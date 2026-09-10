# 0122-quic-chain-selection

Cross-side differential for phase 97's headline proof
(`quic-chain-selection-order`): on a QUIC/UDP listener carrying **both** an
eligible `filter_chains[0]` **and** a `default_filter_chain`, envoy-go serves
from the **indexed** chain — exactly as reference Envoy (`contrib-v1.37.2`,
Docker) does — instead of letting the last-resort default slot pre-empt it.

This is Tasks 17-18 of the phase-97 IMPL spine. It builds on:

- Tasks 2-8 — the ten-arm `internal/listener/quic_test.go` roster, recorded
  RED `{a,c,d,f,h,j}` / GREEN `{b,e,g,i}` at the un-fixed tip.
- Task 10 — the QUIC path now runs the **same** 8-dimension
  `listenerfilter.SelectChain` the TCP path runs.
- Task 11 — both nondeterministic `chainByName` loops deleted;
  `quicTLSConfig` stays connection-independent.
- Tasks 14-16 — the negative-control matrix that proved each arm
  discriminating.

Fixture `0104-http3-downstream-get` is the tree's other QUIC fixture and the
only other implementor of `fixture.ReferenceListenerIsUDP`; **this fixture
copies `0104`'s shape**, not `0121`'s.

## Shape: no YAML, no PKI

Like `0104` and unlike almost every other fixture, `0122` ships **no `.yaml`
bootstrap files and no `pki/` directory at all**. Both bootstraps are
`const referenceTmpl` / `const subjectTmpl` string literals inside
`driver/driver.go`.

The cert/key are delivered `inline_string:` (indented one level deeper than
the `inline_string:` key, with no PEM text inside a YAML comment) because
**the reference container cannot read `filename:` cert paths** —
`reference_fixture_cert_delivery_to_reference_container`. The pair is the
same `testAlphaCertPEM`/`testAlphaKeyPEM` ECDSA P-256 self-signed pair
(SAN `alpha.envoy-go.test`, `serverAuth` EKU, valid 2026-2046) that
`internal/listener/manager_test.go` and `0104` already prove.

## Topology

**ONE** UDP/QUIC listener (`l_qcs`) carrying **two** chain slots. The
differential harness supports exactly one UDP listener per fixture — proven
by mechanism, not guessed — so a second listener is not an option here.

```yaml
address:
  socket_address: { address: <bind>, port_value: <port>, protocol: UDP }
udp_listener_config:
  quic_options: {}

filter_chains:
  - name: fc_indexed
    # ⚠️ NO filter_chain_match KEY AT ALL — an empty match is UNIVERSALLY
    # ELIGIBLE. That is the whole point: an eligible indexed chain must win.
    transport_socket: { name: envoy.transport_sockets.quic, ... }   # its OWN
    filters:
      - HCM: codec_type HTTP3, stat_prefix chain_indexed,
             route "/" -> direct_response 222 "chain-indexed\n"

default_filter_chain:
  name: fc_default
  transport_socket: { name: envoy.transport_sockets.quic, ... }     # its OWN
  filters:
    - HCM: codec_type HTTP3, stat_prefix chain_default,
           route "/" -> direct_response 222 "chain-default\n"
```

Both chains answer the **same** path with the **same** status and
**different** bodies, so the body alone names the chain that served.

## 🔴 This fixture sits ONE IDENTIFIER away from a known banked defect

**The two `stat_prefix` values must differ, and that is load-bearing, not
cosmetic.** Two HCMs sharing one `stat_prefix` — in one listener, across two
filter chains, *exactly this shape* — **panic envoy-go at boot**:

```
panic: stats: duplicate metric registration: "http.<prefix>.downstream_rq_total"
```

rc=2, in 0 seconds. Changing `chain_default` to `chain_indexed` (or either to
a shared name) does not degrade this fixture into a weaker assertion — it
stops the subject from booting at all. The driver repeats this warning at
`referenceTmpl`'s doc comment so it is not lost if only one file is read.

## The subject-side cluster surprise

Both routes are pure `direct_response`s — no cluster is ever dialed. The
reference bootstrap indeed carries **no** `clusters:` section (contrib-Envoy
boots fine with zero clusters, and its log says `loading 0 cluster(s)`).
envoy-go does **not**: an omitted/empty `static_resources.clusters`
boot-rejects. The subject template therefore carries a throwaway `c_backend`
STATIC cluster pointed at the runner-allocated (but never dialed) backend
port. `BackendCount() == 1` exists solely to give the runner something to
allocate — the runner rejects `BackendCount() == 0`
(`reference_differential_backendcount_min_one`).

## Workload

One HTTP/3 `GET /health` per side via `helpers.H3RoundTrip`.

Per `reference_http_expectations_tcp_only`, this fixture does **not**
implement `fixture.HTTPExpectations` — that re-drive is HTTP/1-over-TCP only
and cannot reach a QUIC/UDP listener. The HTTP/3 arm drives through the
driver's own `DriveReference` / `DriveSubject` hooks, on the `0104`
precedent. `ReferenceListenerIsUDP() == true` (plus the compile-time
`var _ fixture.ReferenceListenerIsUDP` assertion) is what makes the runner
publish the reference port as `<port>/udp` **and** the admin TCP port, and
pass `ListenerUDPAddr` to `DriveReference`. `--network host` is what does
**not** work for this.

## Asserted

- **Status + body, absolutely, per side** (in-band in `drive()`): `222` and
  `"chain-indexed\n"`. A body of `"chain-default\n"` is reported **by name**
  as the phase-97 defect. This is not redundant with `CompareBytes`: if both
  sides regressed to the default chain they would agree byte-for-byte and the
  cross-side comparison would pass vacuously.
- **Body, cross-side EXACT** via the runner's `CompareBytes`.
- **Stats** (`AssertStats`, a named subset of exactly **two** names per side):
  `http.chain_indexed.downstream_rq_total >= 1` and
  `http.chain_default.downstream_rq_total == 0`, each preceded by an explicit
  **presence** assertion.
- **Admin** `/ready` (`ProbeAdmin`) over HTTP-over-TCP on both sides.

### Why the presence assertion, and why only two names

A scrape map returns the **zero value for a missing key**, so a `== 0` pin on
a name the subject never emits would be **silently vacuous — green, not
red**. Both names were scraped on **both** sides at this tip *before* either
was pinned:

```
ref  http.chain_indexed.downstream_rq_total: 1
ref  http.chain_default.downstream_rq_total: 0
subj http.chain_indexed.downstream_rq_total: 1
subj http.chain_default.downstream_rq_total: 0
```

`chain_default`'s counter is **present at zero** on both sides, not absent —
so the `== 0` pin reads a real counter and is **real, not vacuous**. The
presence check keeps it that way: if a side ever stops emitting that scope,
the presence assertion is what goes red.

Never a whole map. Measured on this exact shape: the reference emits **156**
`http.chain_*` lines across the two HCM scopes while envoy-go emits **10**
(546 vs 29 lines in the whole scrape), and the reference additionally emits a
listener-qualified `listener.<addr>.http.<prefix>.*` scope of **12** lines
that envoy-go emits **zero** of. ⚠️ The listener address token also differs
cross-side — `listener.0.0.0.0_15122.*` vs `listener.127_0_0_1_<port>.*`,
different address *and* different dot-mangling — so any assertion keyed on
the full listener-scoped name is cross-side **infeasible**
(`reference_listener_stat_scope_cross_side_divergence`).

## UNasserted — a CLOSED enumeration

Everything asserted is listed above. These are the **only** exclusions; a
name in neither list reads as asserted.

1. The `listener.<addr>.http.<prefix>.*` scope — **the subject has none**
   (12 lines on the reference, 0 on the subject), and the address token is
   cross-side infeasible.
2. All `ssl.*` counters (14 listener-scoped on the reference, 5 on the
   subject). QUIC handshake accounting is `0121`'s subject; this row asserts
   chain **selection**.
3. All **histograms** — `scrapeStats` skips every line whose value does not
   `ParseUint`.
4. All `tracing.*` — no tracing provider is configured on either side.
5. The remaining `http.chain_*` names outside the two pinned ones — envoy-go
   emits only stats it actually increments
   (`reference_stats_sink_emits_used_only`).
6. Response header set equality, and QUIC/H3 transport-level framing (ALPN
   negotiation cadence, QUIC version negotiation, 0-RTT).

## Why the INELIGIBLE arm does not ride here

Phase 97's roster also covers the arm where the indexed chain is
**ineligible** and there is no default slot — where the reference closes the
connection (`no filter chain found`) while the un-fixed envoy-go served it.
That arm cannot ride in this directory for two independent reasons:

- **One fixture directory dispatches to exactly one runner branch**
  (`reference_differential_fixture_dispatch_constraint`), so a second
  scenario cannot be selected from the same directory.
- **The harness supports exactly one UDP listener per fixture**, so a second
  listener carrying the ineligible shape is not constructible here.

Those arms live in `internal/listener/quic_test.go`'s ten-arm roster (Tasks
2-16), where each was proven RED at the un-fixed tip, GREEN after the repair,
and discriminating under the Task 14-16 negative-control matrix.

## Reference port 15122 — censused, not inherited

`git grep -hoE '\b15[0-9]{3}\b' -- 'test/fixtures/*/driver/*'
'test/fixtures/*/inputs/*' | sort -u` reads **28** distinct literals at this
tip: `15000`-`15011`, `15042`-`15056`, `15104`, `15360`. ⚠️ **`15360` is not
a port** — it is `0017-http-bandwidth-limit`'s `request_*_total_size` byte
figure — so `15104` (fixture `0104`) is the only reference **port** at or
above `15100`. The producing convention is `15000 + <fixture index>`, a
derived observation **no document states**. `0122` therefore takes `15122`,
verified free (`git grep -n 15122` reads zero under `test/`; `ss -uan` and
`ss -tan` read zero live sockets).

## Live evidence

Reference run **by digest**
(`envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`,
verified against `docs/envoy-go/ENVOY_TARGET.md` lines 3-4) with
`-p 15122:15122/udp`; subject booted from the rendered `subjectTmpl`:

```
ref  H3 GET /health -> status=222 body="chain-indexed\n"
subj H3 GET /health -> status=222 body="chain-indexed\n"
```

Both sides served from the **indexed** chain. `AssertStats` logs the live
values and presence flags via `log.Printf` (`fixture.TB` has no `Logf` —
`reference_fixture_tb_has_no_logf`):

```
0122-quic-chain-selection: ref  http.chain_indexed.downstream_rq_total=1(present=true) http.chain_default.downstream_rq_total=0(present=true)
0122-quic-chain-selection: subj http.chain_indexed.downstream_rq_total=1(present=true) http.chain_default.downstream_rq_total=0(present=true)
```

## ⚠️ This fixture is INERT until its blank import lands

`TestDifferential` looks the fixture up in `fixture.DriverRegistry` by
directory name and **`t.Skipf`s** — a silent, green skip — when no driver is
registered. Registration happens through the driver package's `init()`, which
only runs if `test/differential/runner_test.go` blank-imports
`.../test/fixtures/0122-quic-chain-selection/driver`. That import is a
**separate, later task**. Until it lands, this fixture contributes nothing to
the suite and passes vacuously by skipping.

The registered name is byte-identical to the directory name (checked
mechanically: both `sha256
de2d8507dc66d59d613e31a1fe7602aeff3d09fc002921d7bb3c47e39d4dc80a`), so the
lookup will hit as soon as the import is added.
