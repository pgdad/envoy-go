# 0076-per-try-timeout

Cross-side differential fixture (phase 42.2a, Task 8). Proves the HTTP retry-loop
`per_try_timeout` (a per-ATTEMPT deadline) behaves identically on the envoy-go
(subject) side and the reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`) side.

## Design: the held-host exhaustion-exact

One cluster `c_ptt` over a SINGLE `BlockingHoldResponder` (BackendKind 36 — reused,
no new kind): it accepts each connection but **holds** every `GET /<seg>` open
(never responds) until a `GET /__release` control request. The route `/ptt` carries:

```yaml
retry_policy:
  retry_on: "5xx"
  num_retries: 3
  per_try_timeout: 0.25s
```

With a small `per_try_timeout` T (250ms) over a backend that never answers, EVERY
attempt blocks past T → the per-try-timeout fires on each → a synthesized **504**.
504 is a 5xx, so under `retry_on: "5xx"` each timed-out attempt is retriable; the
loop runs all `num_retries+1` (4) attempts, exhausts the cap, and returns the final
**504** to the client.

A SINGLE host makes this **deterministic and cross-side-EXACT** — no round-robin
spread, so no offset nondeterminism (unlike 0075's recover arm).

## What it asserts (per side, cross-side EXACT)

One sequential `GET /ptt` drives the entire 4-attempt loop inside the proxy (~1.2s;
the `per_try_timeout` is the feature's own timing — there is **no `time.Sleep`** in
the assertion). Then the delta-stats over `c_ptt`:

| stat (delta)                              | want |
|-------------------------------------------|------|
| `upstream_rq_per_try_timeout`             | 4    |
| `upstream_rq_retry`                       | 3    |
| `upstream_rq_retry_limit_exceeded`        | 1    |
| `upstream_rq_retry_success`               | 0    |
| `upstream_rq_total`                       | 4    |

Plus `upstream_rq_total > 0` on the reference side (the decode-ran guard:
`per_try_timeout` cannot fire on an attempt that never connected). The final client
status is **504**. After the assertions, `GET /__release` on the backend control
port (`127.0.0.1:<backendPort>`) drains the parked held attempts and re-arms the
gate for the next side (subject runs fully, then reference).

## Topology / shapes

- 1 `BlockingHoldResponder` (runner-spawned, uniform `BackendKind()`).
- `c_ptt`: subject `STATIC` over `127.0.0.1:<backendPort>`; reference `STRICT_DNS`
  over `host.docker.internal:<backendPort>` (the 0074/0075 bridge shape). Both
  carry the router `http_filter` (a missing router filter makes requests hang).

## Deliberate breaks (Task 9)

- Drop `per_try_timeout` from `/ptt` → attempts block to the client/connect bound;
  no synthesized 504, no `upstream_rq_per_try_timeout` → the delta assertion fails.
- Flip the expected `per_try_timeout` delta (4→3) or the final status (504→503) →
  the assertion bites.

## Run

```
go test ./test/differential/ -run 'TestDifferential/0076' -count=1 -v
```

The `TestDifferential/` prefix is required (a bare `-run '0076'` matches zero
subtests — they are named `TestDifferential/<fixture>`).
