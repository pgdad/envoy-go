# 0061-lb-ring-hash

Cross-side `[tcp_proxy]` differential over ONE 3-endpoint cluster with
`lb_policy: RING_HASH` + `ring_hash_lb_config: {}` (defaults) + the tcp_proxy
`hash_policy: [{source_ip: {}}]` on BOTH sides (the 0001 shape: reference
STRICT_DNS / `host.docker.internal`, subject STATIC / 127.0.0.1). This is the
**end-to-end proof of Tasks 3-6** — the `source_ip` hash_policy →
`WithHashKey` → `ringHashLB` pick → per-source-IP affinity — that envoy-go's
Ketama consistent-hash load balancer lands connections with the SAME affinity
shape and moves the SAME cluster stats (cross-equal `ring_hash_lb.*` gauges) as
the reference Envoy `contrib-v1.37.2`.

The **FIRST consistent-hash fixture**; the **FIRST non-zero LB-stat delta**
(+3 `ring_hash_lb.*` gauges → the stat surface moves 1116 → **1119**).

Phase 36 SPEC §7 (the +3 gauges) / §8.1 (the fixture design) / 36.1 PLAN Task 7.

## The workload (subject-side multi-source-IP; AMEND-RH8 / D-RH8)

The driver binds outgoing connections to **4 source IPs** — `127.0.0.2`,
`127.0.0.3`, `127.0.0.4`, `127.0.0.5` — via `net.Dialer{LocalAddr: …}` (proven
feasible on host loopback, SPEC §11.8: on Linux all of `127.0.0.0/8` is loopback,
so binding `127.0.0.2..5` as `LocalAddr` works without `ip addr add`). For EACH
source IP it opens **16 connections** (`burstPerIP`), writes one deterministic
payload (`rh-<s>-<i>\n`), reads the echo, and closes. **16 × 4 = 64** total
connections (the 0059/0060 conservation target). A 750 ms settle then lets the
upstream closes propagate so `upstream_cx_active` quiesces to 0 before the stats
scrape.

The `drive` function is SHARED by `DriveReference` and `DriveSubject` — identical
payloads → byte-identical echo streams, so the runner's `CompareBytes` gate passes
cross-side regardless of which backend served which connection.

### Why the subject shows affinity but the reference collapses

The subject is a **host process**: the driver connects loopback → loopback, so the
subject sees `RemoteAddr` = `127.0.0.x` (DISTINCT per source IP). Each source IP →
one `source_ip` hash key → one ring point → one backend. All 16 connections from a
given source IP land on the SAME backend, so each backend's count is a sum of whole
16-blocks → **≡ 0 mod 16**. 4 keys over 3 backends → **≥ 2 backends nonzero**
(spread).

The reference runs in **Docker**: the driver reaches it at `127.0.0.1:<mappedport>`
but Docker NAT rewrites the in-container source to ONE gateway IP, so the reference
sees a SINGLE `source_ip` key → it pins ALL 64 connections to ONE backend
(single-key affinity, the live D-RH4b observation). Cross-side host identity is
therefore **INFEASIBLE** (AMEND-RH8 /
`reference_differential_hash_key_cross_side_infeasible`) — the reference is asserted
on **conservation + byte-equivalence + the cross-side stats only**, NOT on per-IP
spread.

## The affinity+spread arm (`AssertDistribution`) — the D-S36-4 modular invariant

`AssertDistribution` is **DETERMINISTIC / EXACT** — NOT a σ-band
(`reference_differential_band_sigma_margin` governs RNG-distributed bands; ring_hash
affinity is not one). The runner snapshots `backend.accepts` after Drive (each
accept on the streaming `TCPEcho` backend counts one connection; the 64 conns sum
across the three backends).

**SUBJECT** (affinity + spread + conservation):

- **affinity:** each per-backend count `cᵢ ≡ 0 mod 16` — the consistent-hash
  invariant (one source IP → one key → one ring point → one backend, so a source IP
  contributes all 16 or 0 to a given backend). VIOLATED by a key-scatter break (a
  source IP splitting across backends produces a count NOT a multiple of 16);
- **spread:** `>= 2` backends nonzero — VIOLATED by a collapsed ring (all 64 on one
  backend);
- **conservation:** `c1 + c2 + c3 == 64` (catches drop / double-count).

**REFERENCE** (conservation only): `sum == 64` (Docker NAT → single gateway source
IP → single-key pin → all 64 on ONE backend). The reference's real proof is
byte-equivalence + the cross-side stats.

### D-S36-4 coverage note (the modular invariant vs an adversarial even-split)

The modular invariant is **necessary and overwhelmingly discriminating** against the
realistic break (random scatter), NOT a tight proof against an adversarial
even-split. A key-scatter break draws picks ~uniformly per connection, so a source
IP's 16 connections scatter across the 3 backends; the chance the *aggregate*
per-backend totals all happen to land on an all-multiples-of-16 tuple is `< 1%` (a
`multinomial(64, 1/3)` landing on `{0,16,32,48,64}`-only counts). So the invariant
catches scatter **flake-free**. It would NOT catch a hypothetical adversary that
scattered the keys but kept every backend total a perfect multiple of 16 — but no
realistic LB bug produces that; the seam either preserves the per-key affinity (the
correct path) or scatters per-connection (the break). The affinity assertion is the
load-bearing one: if it fails (counts not multiples of 16), the source-IP binding or
the `source_ip` key path is broken.

## The stats prong (`StatsAsserter`, post-drain) — SPEC §7

The §7 cross-vs-per-side set, EXTENDED with the **3 new `ring_hash_lb.*` gauges**.
Observed (every run, both sides):

| stat                                            | reference | subject | disposition                       |
|-------------------------------------------------|-----------|---------|-----------------------------------|
| `cluster.c_echo.upstream_cx_total`              | 64        | 64      | **cross-equal** == 64             |
| `cluster.c_echo.membership_total`               | 3         | 3       | **cross-equal** == 3              |
| `cluster.c_echo.upstream_cx_active`             | 0         | 0       | **cross-equal** == 0 (quiesced)   |
| `cluster.c_echo.ring_hash_lb.size`              | 1026      | 1026    | **cross-equal** == 1026           |
| `cluster.c_echo.ring_hash_lb.min_hashes_per_host` | 342     | 342     | **cross-equal** == 342            |
| `cluster.c_echo.ring_hash_lb.max_hashes_per_host` | 342     | 342     | **cross-equal** == 342            |
| `cluster.c_echo.upstream_rq_total`              | 64        | 0       | **PER-SIDE**                      |

The **3 `ring_hash_lb.*` gauges are cross-equal** because they depend ONLY on
`(minimum_ring_size, hash_function, host count, weights)`, NOT on endpoint
ADDRESSES — so they are IDENTICAL on subject and reference for a 3-equal-host default
RING_HASH cluster (`size = 1026`, `min = max = 342`). This makes them a STRONG
cross-side prong (unlike the per-side affinity distribution, which diverges by ring
layout — AMEND-RH8). With the default `minimum_ring_size = 1024` and 3 equal-weight
hosts, the Ketama build rounds up to the smallest size that gives each host its
fair share: `342 × 3 = 1026 >= 1024`.

`upstream_rq_total` is PER-SIDE (NOT cross-equal): the reference's `tcp_proxy`
charges one rq per cx (rq-per-cx) → 64; envoy-go's tcpproxy path NEVER calls
`IncUpstreamRqTotal` (a pre-existing documented boundary the 0059/0060 fixtures
already pin) → 0.

## Deliberate-break liveness (Task 8 — `-count=1`)

Each break was applied ONE AT A TIME, run with `-count=1`
(`reference_differential_break_protocol_count1` — go-test caching defeated;
every break confirmed `--- FAIL`, never a stale cached PASS), observed to FAIL
the EXPECTED prong, then `git restore`d (production code + driver byte-identical
after each revert). Every assertion prong is proven LIVE (the `0030`
dead-assertion lesson — no vacuous green).

| # | Break | Where | Expected prong | Observed `--- FAIL` line |
|---|-------|-------|----------------|--------------------------|
| (i) | scatter the key — `Pick` draws random unconditionally (ignore `hashKey`) | `internal/cluster/ringhash.go` `Pick` | SUBJECT **affinity** (`count % 16 == 0`) | `distribution: subject affinity: backend[0]=14 not a multiple of 16 (key scattered? a source IP split across backends)` |
| (ii) | collapse the ring — force every pick to `ring[0]` (`m = 0`) | `internal/cluster/ringhash.go` `Pick` | SUBJECT **spread** (`>= 2` nonzero) | `distribution: subject spread: only 1 backend(s) nonzero, want >= 2 (ring collapsed?)` |
| (iii) | corrupt a cross-equal stat want (`upstream_cx_total` 64 → 99) | `driver/driver.go` `AssertStats` | **stats** cross-equal `upstream_cx_total` | `ref cluster.c_echo.upstream_cx_total = 64, want 99` + `subj cluster.c_echo.upstream_cx_total = 64, want 99` |
| (iv) | corrupt the size gauge (`rh.size` → `rh.size + 1`) | `internal/cluster/manager.go` `registerClusterMetrics` | the **`ring_hash_lb.*` gauge** prong (cross-equal + want) | `cross-side mismatch cluster.c_echo.ring_hash_lb.size: ref=1026 subj=1027` + `subj cluster.c_echo.ring_hash_lb.size = 1027, want 1026` |

Note on (ii): the ring-collapse keeps each per-backend count a multiple of 16
(all 64 land on ONE backend, 64 % 16 == 0), so AFFINITY still holds — the
**spread** leg is the one that bites. This proves the spread assertion is
non-vacuous independent of the affinity leg.

Note on (iv): the corrupted size gauge bit BOTH sub-prongs of the gauge check —
the cross-side `ref != subj` mismatch AND the `subj != want` value check — which
confirms both halves of the cross-equal stats loop are live.

### Flake check

`go test ./test/differential/ -run 'TestDifferential/0061' -count=20` → **20/20
PASS** (66 s; 20 fresh reference containers). The affinity leg is DETERMINISTIC
(fixed ring + fixed source-IP keys → never flakes); the spread leg (`>= 2`) is
overwhelmingly stable (4 source-IP keys over 3 backends). No assertion loosened.

## Firsts / non-additions

- **FIRST consistent-hash fixture** — affinity by hash key (source IP), proven via
  the aggregate-count modular invariant (D-S36-4).
- **FIRST non-zero LB-stat delta** — the +3 `ring_hash_lb.*` gauges move the stat
  surface 1116 → **1119** (REFUTING the BRAINSTORM's anticipated zero delta).
- **NO new BackendKind** — reuses `TCPEcho` (0); the backend tail STAYS 33. An LB
  phase exercises WHERE connections land, not what the backend speaks.
- **NO new fuzzer** — ring_hash decodes no untrusted wire bytes (the hash key
  derives from a source IP, not a wire frame; the ring-lookup property test is
  UNIT-level, D-RH7).
- **NO boot-reject dir** — the lb-policy / config reject arms land UNIT-LEVEL in
  `manager_test.go`.
