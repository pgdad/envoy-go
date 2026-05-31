# Fixture 0043 — network-rbac (cross-side; R-M-LIVE)

Cross-side differential fixture for the `envoy.filters.network.rbac`
L4 network filter (phase 26.3). It is the **first production mixed
read→terminal network filter chain (R-M-LIVE)**: each listener's filter
chain is `[rbac_network, tcp_proxy]`, so a read filter (rbac_network)
makes an L4 enforcement decision and — when it Continues — hands off to
the terminal `tcp_proxy` which passes bytes through to a TCP echo
backend. Both reference Envoy v1.37.2 (dockerized) and envoy-go boot the
same three-listener bootstrap; the driver asserts byte-exact parity
across the allow / deny / shadow scenarios + the four
`<stat_prefix>.rbac.*` counters.

## Fixture type

Cross-side (`MultiListenerDriver` + `StatsAsserter`). The runner spawns
BOTH proxies, drives traffic against all three listeners, diffs the
response byte streams (`CompareBytes`), diffs the admin `/ready`
responses, then runs `AssertStats` (the asserter-dispatch-mandated
cross-side assertion path — `SubjectAsserter` would NOT run on the
cross-side path and would be a dead vacuous assertion).

## Scenarios (one listener each; L4 RBAC is connection-scoped)

- **l_allow** — rbac ALLOW: Permission `destination_port = <this side's
  listener port>` AND Principal `direct_remote_ip 0.0.0.0/0`. BOTH L4
  accessors are genuinely evaluated. The `direct_remote_ip 0.0.0.0/0`
  principal is the cross-side-stable choice (the reference container
  sees the source IP as the Docker bridge gateway; envoy-go on the host
  sees 127.0.0.1 — a loopback-specific CIDR would diverge). The
  `destination_port` is templated to each side's ACTUAL listener port so
  `conn.LocalAddr().Port` matches on both sides. → ALLOW → tcp_proxy
  passthrough → byte-exact echo. `rbac_allow.rbac.allowed = 1`.

- **l_deny** — rbac default-deny (`rules: {action: ALLOW, policies: {}}`):
  nothing matches → enforced-deny → response-code-details
  `rbac_deny_close` + connection close (NoFlush) BEFORE tcp_proxy sees a
  byte → zero echoed bytes on BOTH sides. `rbac_deny.rbac.denied = 1`.

- **l_shadow** — enforced-ALLOW (same as l_allow) + shadow default-deny.
  Enforced passthrough → byte-exact echo; the shadow walk ticks
  `rbac_shadow.rbac.shadow_ns.shadow_denied = 1` AND writes the shadow
  pair to connection dynamic-metadata (emitted-but-unread at L4 —
  asserted indirectly via the stat here + directly by the Task-11 unit
  test).

## StatsAsserter (deliberate-break-proven LIVE)

`AssertStats` scrapes `/stats/prometheus` from both admin endpoints and
asserts the per-side `<stat_prefix>.rbac.*` counters. Reference Envoy
v1.37.2 surfaces these via a tag-extractor: metric `envoy_rbac_<rest>` +
label `envoy_rbac_prefix="<stat_prefix>"`. envoy-go matches this shape
via the phase-26.3 `flattenToProm` rbac tag-extractor rule (added at
Task 14; previously the network rbac counters were dropped from
`/stats/prometheus` because no flatten rule recognized the
`<stat_prefix>.rbac.*` shape). The deliberate-break proof (Task 14
PROGRESS.md): flipping `rbac_allow.rbac.allowed` to 99 FAILS the test on
BOTH `ref` and `subj` — proving the subject-side assertion is live.

## D-26.3-4 (SNI / mTLS-authenticated scenario)

UNIT-ONLY at 26.3. Driving SNI/mTLS at a raw L4 listener requires a
downstream TLS transport_socket + tls_inspector + client-cert harness,
substantially heavier than the plaintext L4 path and not readily
reusable from the HTTP-oriented 0018 PKI harness. The
`RequestedServerName` + `DownstreamPrincipal` accessor mapping is
UNIT-covered at Task 9 (`internal/filter/network/rbac/evalctx_test.go`).
The cross-side SNI/mTLS differential gap is recorded honestly in
PROGRESS.md Task 14.

## Cluster requirement

`tcp_proxy` needs an upstream cluster — `c_echo` (the runner's TCP echo
backend) doubles as the boot-satisfying cluster (a zero-cluster boot is
rejected by both sides) AND the passthrough target for allow + shadow.
