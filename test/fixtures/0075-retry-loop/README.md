# 0075-retry-loop

Cross-side `[http_connection_manager + router]` differential proving the HTTP
**retry loop** behaves IDENTICALLY on the envoy-go (subject) side and the
reference-Envoy side, on BOTH sides (the 0069 HTTP shape: reference `STRICT_DNS`
/ `host.docker.internal`, subject `STATIC` / `127.0.0.1`).

Two clusters + two routes, each a distinct retry behavior:

- **EXHAUSTION** (`/exhaust` → `c_exhaust`): one host, the always-503 responder;
  `retry_policy{retry_on:"5xx", num_retries:3}`. A single `GET /exhaust` attempts
  once + retries 3 times against the same 503 host, exhausts the cap, returns 503.
- **RECOVER** (`/recover` → `c_recover`): two hosts (the 503 responder + a healthy
  HTTPEcho), `ROUND_ROBIN`; `retry_policy{retry_on:"5xx", num_retries:1}`. A
  `/recover` request that first picks the 503 host retries ONCE → the retry
  re-picks the OTHER host (echo) → 200; an echo-first request 200s immediately.

Phase 42.1 SPEC / PLAN Task 9.

## Topology: 1 ALWAYS-503 + 1 HEALTHY (all runner-spawned)

| endpoint | backing                          | clusters            | role                              |
|----------|----------------------------------|---------------------|-----------------------------------|
| 0        | runner HTTP503Responder backend0 | c_exhaust, c_recover | always 503; the retry target      |
| 1        | runner HTTPEcho backend1         | c_recover            | 200s `/`; the recover landing host |

`BackendCount()` returns **2**; the runner selects the kinds via
`PerHostBackendKind` (`BackendKindAt(0)` → `HTTP503Responder`, `BackendKindAt(1)`
→ `HTTPEcho`).

## Clusters + routes (identical on both sides — NAT-transparent static config)

```yaml
clusters:
  - name: c_exhaust            # ROUND_ROBIN, 1 host: [backend0 (503)]
  - name: c_recover            # ROUND_ROBIN, 2 hosts: [backend0 (503), backend1 (echo)]
routes:
  - { prefix: "/exhaust", cluster: c_exhaust, retry_policy: { retry_on: "5xx", num_retries: 3 } }
  - { prefix: "/recover", cluster: c_recover, retry_policy: { retry_on: "5xx", num_retries: 1 } }
```

The HCM `stat_prefix` is `ingress_http` (single-sourced; it keys the
`http.ingress_http.downstream_rq_2xx` recover-arm assertion).

## The driver: drive BOTH sides + delta the retry stats (the 0069 pattern)

The runner's hooks are `DriveReference`/`DriveSubject` (the byte-equiv stream, run
**first**) then `AssertStats` (run **last**, the only hook holding **both** admin
addrs). All measured work runs inside `AssertStats`. The `Drive` hooks stash their
listener addrs and return a fixed `"READY\n"` for the runner's `CompareBytes`
gate.

The retry loop is **synchronous** — the `GET` returns only after all retries
complete (the backoff is delay-only: it changes WHEN, never WHETHER/HOW-MANY). So
the assertion is **count-based** with **NO sleep**
(`reference_differential_band_sigma_margin`).

Per side (`addr=listener`, `adminAddr`):

1. baseline `scrapeStats(adminAddr)`.
2. **EXHAUSTION** — 1 `GET /exhaust` → assert **503**.
3. **RECOVER** — `recoverReqs` (8) `GET /recover` → assert **200** each (every 200
   from host1, the echo).
4. final `scrapeStats(adminAddr)`; delta-assert:

| stat                                                  | assertion        | scope            |
|-------------------------------------------------------|------------------|------------------|
| `cluster.c_exhaust.upstream_rq_retry`                 | `delta == 3`     | cross-side EXACT |
| `cluster.c_exhaust.upstream_rq_retry_limit_exceeded`  | `delta == 1`     | cross-side EXACT |
| `cluster.c_exhaust.upstream_rq_total`                 | `delta == 4`     | cross-side EXACT |
| `cluster.c_exhaust.upstream_rq_total`                 | `> 0` (ref)      | decode-ran guard |
| `http.ingress_http.downstream_rq_2xx`                 | `delta == 8`     | cross-side INVARIANT |
| `cluster.c_recover.upstream_rq_retry_limit_exceeded`  | `delta == 0`     | cross-side INVARIANT |
| `cluster.c_recover.upstream_rq_retry`                 | `delta > 0`      | **subject only** |
| `cluster.c_recover.upstream_rq_retry_success`         | `== retry delta` | **subject only** |

## Why the recover-arm exact retry count is subject-side only

The reference **randomizes** the round-robin initial offset
(`reference_round_robin_offset_randomized`), so the NUMBER of `/recover` requests
that pick the 503 host FIRST (== the number that retry) is NOT cross-side
deterministic. What IS offset-invariant: **every** `/recover` request recovers to
a downstream 200. So the cross-side recover assertion is the offset-invariant
`downstream_rq_2xx == K` + `retry_limit_exceeded == 0`; the exact
`retry_success == retry` equality (every retry recovered) is **subject-side**.

The EXHAUSTION arm has a single 503 host (no round-robin spread → no offset
nondeterminism), so it IS cross-side-exact.

## The key behavioral parity this fixture proves

With 2 hosts `ROUND_ROBIN` + `num_retries:1`, a `/recover` request that first
picks the 503 host **must retry onto a FRESH host** (the healthy echo) → 200. This
holds on BOTH sides because the retry re-selects the upstream host via the cluster
LB each attempt (envoy-go's `doH1ClusterAction` re-picks via `AcquireH1`;
reference Envoy's retry host-selection re-picks too). If a side pinned the SAME
503 host on retry, the all-recover invariant would break — **that parity IS the
assertion**.

## Run it

```bash
go test ./test/differential/ -run 'TestDifferential/0075' -count=1 -v 2>&1 | tail -40
```

Use `-run 'TestDifferential/0075'` (NOT `-run '0075'`, which matches zero
subtests — `reference_differential_run_selector`). Docker must be available (the
reference image `envoyproxy/envoy:contrib-v1.37.2`).

## Non-additions

- **NO new BackendKind authored here** — `HTTP503Responder` (= 35) and `HTTPEcho`
  (= 1) are both reused via `BackendKindAt`.
- **NO `DistributionAsserter`** — the runner's per-backend accept counters are not
  used; the per-side status/idx classification + the cross-side delta-stats live
  in `AssertStats` (`reference_differential_asserter_dispatch`).
- **NO new fuzzer / boot-reject dir** — the retry parse/classify/backoff logic is
  UNIT-covered in `internal/filter/http/router` + `internal/filter/hcm`.
