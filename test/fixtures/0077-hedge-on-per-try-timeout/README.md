# 0077-hedge-on-per-try-timeout

Cross-side differential fixture (phase 42.2b, Task 9). Proves the HTTP retry-loop
**hedging** (`hedge_on_per_try_timeout`) behaves identically on the envoy-go
(subject) side and the reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`) side.

## Design: the held-host first-acceptable-wins

One cluster `c_hedge` over a SINGLE `BlockingHoldResponder` (BackendKind 36 — reused,
no new kind): it accepts each connection but **holds** every `GET /<seg>` open
(never responds) until a `GET /__release` control request. The route `/hedge` carries:

```yaml
timeout: 0s            # AMEND-H7: disable the reference's default route timeout
                       #           (envoy-go has none) — REQUIRED on both sides.
retry_policy:
  retry_on: "5xx"
  num_retries: 3
  per_try_timeout: 0.25s
hedge_policy:
  hedge_on_per_try_timeout: true
```

With `hedge_on_per_try_timeout` over a backend that never answers, each per-try
deadline T (250ms) **launches a hedge** (a RETRY) and **leaves the original
running** — it does NOT synthesize a 504, does NOT abandon. So 1 primary + 3 hedges
(`num_retries`) all end up in flight (held); after 3 hedges the retry cap is hit
(`upstream_rq_retry_limit_exceeded == 1`) and the request **blocks** awaiting the
first acceptable result. A `GET /__release` then makes the held attempts answer 200
→ the first acceptable **200** returns downstream.

**THE LOAD-BEARING PROOF (AMEND-H1):** `cluster.c_hedge.upstream_rq_per_try_timeout`
stays **0** — a hedged per-try-timeout is a RETRY, not a `per_try_timeout`. The hedge
executor uses a hedge-trigger timer, NOT the 42.2a abandon-and-count discriminator.

## What it asserts (per side, cross-side EXACT)

The hedged `GET /hedge` **blocks** until `/__release` (all attempts held), so it is
fired in a **goroutine** and the driver **polls** the admin `/stats` to the steady
state (3 hedges launched, cap hit) before releasing — the 0074 concurrent-fire +
poll-to-converge model. There is **no `time.Sleep`** in the assertion path
(poll-to-converge only). Then the delta-stats:

| stat (delta)                              | want | note |
|-------------------------------------------|------|------|
| `cluster.c_hedge.upstream_rq_per_try_timeout`    | 0 | **AMEND-H1** — hedged, not a per_try_timeout |
| `cluster.c_hedge.upstream_rq_retry`              | 3 | numHedges launched |
| `cluster.c_hedge.upstream_rq_retry_limit_exceeded` | 1 | cap hit once |
| `cluster.c_hedge.upstream_rq_total`              | 4 | 1 primary + 3 hedges (counted at attempt entry) |
| `http.ingress_http.downstream_rq_2xx`            | 1 | the single downstream 200 (request recovered) |

Plus `upstream_rq_total > 0` on the reference side (the decode-ran guard: no hedges
can launch on an attempt that never connected). The final client status is **200**.
After the assertions the shared backend is drained by `/__release` for the next side
(subject runs fully, then reference).

## The `upstream_rq_200` H1-loser caveat (ADR-0251 departure, D-S422B-2)

We deliberately **do NOT** assert the UPSTREAM 200-class counter
(`cluster.c_hedge.upstream_rq_200` / `upstream_2xx`) cross-side. On the SUBJECT
(envoy-go H1) side, `doH1ClusterAction` honors only `ctx.Deadline()`, **not**
`ctx.Done()`, so after `/__release` ALL 4 held H1 losers complete with 200 and each
bumps the upstream 200-class counter → the subject **over-counts** (up to 4) AND
races the join. The REFERENCE cancels its in-flight losers, so its `upstream_rq_200`
== 1. They are **not** cross-side equal. The DOWNSTREAM `downstream_rq_2xx` == 1 **is**
equal (one client response on both sides) — that is why the fixture asserts the
downstream class, not the upstream class. Task 12's ADR-0251 body covers this.

## Topology / shapes

- 1 `BlockingHoldResponder` (runner-spawned, uniform `BackendKind()`).
- `c_hedge`: subject `STATIC` over `127.0.0.1:<backendPort>`; reference `STRICT_DNS`
  over `host.docker.internal:<backendPort>` (the 0074/0076 bridge shape). Both
  carry the router `http_filter`.

## Deliberate breaks (Task 10)

- break B: make a hedged per-try-timeout increment `upstream_rq_per_try_timeout`
  (the abandon-and-count path) → the per_try_timeout delta-0 assertion fails.
- Flip the expected `upstream_rq_retry` delta (3→2) or the downstream status
  (200→504) → the assertion bites.

## Run

```
go test ./test/differential/ -run 'TestDifferential/0077-hedge-on-per-try-timeout' -count=1 -v
```

The `TestDifferential/` prefix is required (a bare `-run '0077'` matches zero
subtests — they are named `TestDifferential/<fixture>`).
