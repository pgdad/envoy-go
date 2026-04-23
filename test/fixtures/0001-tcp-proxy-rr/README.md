# Fixture 0001 — TCP proxy + round-robin (3 endpoints)

**Purpose:** end-to-end exercise the phase-02 dataplane (listener manager + STATIC cluster + round-robin LB + TCP proxy filter) and prove the per-proxy round-robin distribution is exact across a 9-request workload against a 3-endpoint cluster.

**Differential surface:** concatenated response bodies (9 echo round-trips) are byte-equivalent between upstream Envoy and envoy-go.

**Local-correctness surface:** each proxy's per-backend accept counts must be exactly `[3, 3, 3]` (asserted via `AssertDistribution` per the new BEHAVIOR_CONTRACT TCP proxy subsection).

**Topology:**

```
client (test) ──> [proxy listener 127.0.0.1:<subjPort>] ──RR──> [backend 1 / 2 / 3 on host:0.0.0.0:<random>]
client (test) ──> [container-mapped <hostPort>     ──Envoy──> [host.docker.internal:<random> 1/2/3 (V4_ONLY)]
```

Same client driver targets both proxies; the host-side backends serve both runs (with per-side counter snapshots taken between drives so the runner can credit each proxy's distribution independently).

**STATIC vs STRICT_DNS divergence (ADR-0010, ADR-0027):** the reference Envoy runs inside a Docker container and reaches host-side backends via `host.docker.internal` (which requires `STRICT_DNS` + `dns_lookup_family: V4_ONLY` per ADR-0010). The envoy-go subject runs as a host subprocess and dials literal 127.0.0.1 endpoints. The cluster *behaviour* is equivalent — three echo endpoints in round-robin order — but the *config shape* diverges by ADR. Same pattern fixture 0000 carries.

**Why distribution is not a differential dimension (BEHAVIOR_CONTRACT § TCP proxy):** upstream Envoy's RR LB is per-worker-thread with a randomized starting offset; the cross-proxy sequence of endpoint selections is not reproducible. Each proxy is asserted RR-correct in its own right (3/3/3 per proxy); cross-proxy sequence equivalence is explicitly NOT asserted.

**Run locally:**

```bash
go test ./test/differential/ -run TestDifferential/0001-tcp-proxy-rr -v
```

**Re-baseline:** if upstream Envoy's pin (ADR-0008) bumps and the differential gate fails, follow ADR-0008 §"refresh procedure" to re-record evidence and supersede the failing ADR if the bytes change.
