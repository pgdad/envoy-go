# 0000-tcp-echo

The fixture now exercises two independent equivalence observations against
both proxies (upstream Envoy reference, envoy-go subject). On the TCP path
the driver pumps ten deterministic payloads through each proxy's listener to
the same echo backend and asserts the full response byte streams are
byte-exact. On the HTTP path the driver probes each proxy's admin
`/ready` endpoint once the proxy is ready and asserts the on-the-wire
responses match byte-for-byte on three dimensions: the status line is
exact, the response body is `LIVE\n` byte-exact, and the header set is
equal modulo the BEHAVIOR_CONTRACT allow-list. Both observations run as
independent byte-pair comparisons in the same fixture run; the fixture
passes iff both pass.

The subject's `envoy-go.yaml` carries three benign divergences from the
reference `envoy.yaml`, each recorded inline in the subject YAML and
traced to SPEC §5.4: (1) the cluster `type` is `STATIC` on the subject
versus `STRICT_DNS` on the reference — the subject reaches the backend
via a literal IP address because phase 01 has no DNS resolver (SPEC §5.4
#1); (2) the subject cluster omits `dns_lookup_family` while the
reference keeps `V4_ONLY` per ADR-0010 — again because the subject has
no DNS resolver and the field would be inert (SPEC §5.4 #2); (3) the
subject binds and dials `127.0.0.1` whereas the reference binds
`0.0.0.0` and egresses to `host.docker.internal` — the subject runs as a
host subprocess with no docker bridge, so the reference's host-gateway
hostname does not apply (SPEC §5.4 #3).

## Driver

`driver/driver.go` opens a TCP connection to each proxy's listener, sends
ten payloads `ping-N-<uuid>\n` for N ∈ {0..9}, half-closes write, reads to
EOF or 1s idle, returns the concatenated response stream. The same driver
also implements `ProbeAdmin`, which dials each proxy's admin socket over
raw TCP, writes `GET /ready HTTP/1.1\r\nHost: <addr>\r\nConnection: close\r\n\r\n`,
and returns the full response bytes (status line + headers + body) for
the diff to split and compare.

## Expectations

`expectations.yaml` enumerates every BEHAVIOR_CONTRACT §7.2 dimension.
Three are applicable in phase 01: `response-status` (exact-match, scoped
to `admin-/ready`), `response-body` (byte-exact, scoped to `tcp-echo +
admin-/ready`), and `response-headers` (set-equal-modulo-allow-list,
scoped to `admin-/ready`). The remaining six are not-applicable with
phase-01-specific justifications: `response-trailers` (no trailers on
`/ready`; no HTTP layer on the TCP path), `http2-http3-framing` (admin
is HTTP/1.1; TCP has no framing), `access-log` (deferred to phase 06),
`stats` (deferred to phase 06), `xds` (static config; no xDS), and
`timing` (not opt-in). The header allow-list consumed by the
`response-headers` dimension is sourced from
`docs/envoy-go/BEHAVIOR_CONTRACT.md § "Admin API — /ready"`, which names
the three allow-listed headers (`Date`, `Content-Length`,
`Transfer-Encoding`) and the rationale for each.
