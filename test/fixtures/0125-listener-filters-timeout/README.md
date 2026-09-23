# 0125-listener-filters-timeout

Cross-side differential for phase 100 (`listener-filters-timeout-enforce`):
reference Envoy **enforces** `Listener.listener_filters_timeout`. A connection
whose listener filters have not finished inspecting at the deadline is **closed**
under `continue_on_listener_filters_timeout: false`, or **falls through** to chain
selection under `true`; either way the listener's `downstream_pre_cx_timeout`
counter books it. `0s` **disables** the deadline. The un-fixed envoy-go never
enforces it: a silent client is held until the client acts, and the counter name
does not exist.

## Shape

Like `0123`: no YAML, no PKI, no `inputs/`. `renderBootstrap` in
`driver/driver.go` builds **both** sides from one template; only the bind
address, the admin port and the five listener ports differ cross-side.

Every listener is a **one-line diff** from one base: `tls_inspector`,
`listener_filters_timeout: 1s`, `fc_indexed` matching
`transport_protocol: raw_buffer` (body `INDEXED <listener>`), and a last-resort
`default_filter_chain` (body `DEFAULT <listener>`). All ten HCM stat prefixes are
distinct (a shared prefix panics envoy-go at boot).

| listener | delta | reference port | `downstream_pre_cx_timeout` |
|---|---|---|---|
| `l_false` | `continue…: false` | 15125 | **51** = F1 1 + F2 50 + F3 0 |
| `l_true` | `continue…: true` | 15228 | **1** (T2; T1 books 0) |
| `l_true_tls` | `true`, `fc_indexed` matches `tls` | 15229 | **1** (T3) |
| `l_zero` | `listener_filters_timeout: 0s` | 15230 | **0** |
| `l_nofilt` | no `listener_filters` | 15231 | **0** |

`BackendCount() == 1`: a `c_unused` STATIC cluster no route dials (the runner
rejects 0; envoy-go boot-rejects an absent `clusters` key).

## Arms (every arm on a side runs concurrently — one ~16.5 s drive per side)

| arm | listener | client | pin |
|---|---|---|---|
| F1 | `l_false` | silent, hold 3 s | server closed in [700, 1800] ms, 0 bytes |
| F2 | `l_false` | 50 concurrent silent, hold 3 s | **all 50** closed in [700, 1800] ms, 0 bytes each |
| F3 | `l_false` | GET at 300 ms | 200, `INDEXED l_false` |
| T1 | `l_true` | GET at 0 | 200, `INDEXED l_true` |
| T2 | `l_true` | silent 2500 ms, then GET | 200, `INDEXED l_true` |
| T3 | `l_true_tls` | silent 2500 ms, then GET | 200, `DEFAULT l_true_tls` |
| Z1 | `l_zero` | silent | still open at 16.5 s |
| N1 | `l_nofilt` | silent | still open at 2.8 s |
| S | all five | — | `downstream_pre_cx_timeout` BY VALUE on each side's own `envoy_listener_address` label on `/stats/prometheus`; a **missing** series is a hard failure |

The reference label is `0.0.0.0_<in-container port>`; the subject label is its
bind address with `:` and `.` folded to `_` (`127_0_0_1_<port>`).

## Window — MEASURED through the harness's host `-p` path

The clock starts when the client's dial returns. Pooled over every run of the
PLAN measurement (tip, PB1 x3, NC1 x2, NC2, NC3, NC8):

| side | n | min | max | mean | σ | (mean-700)/σ | (1800-mean)/σ |
|---|---|---|---|---|---|---|---|
| reference (via docker-proxy) | 459 | 1000 | 1004 | 1002.0 | 0.92 | 328 | 867 |
| subject (PB1-based runs) | 347 | 1000 | 1021 | 1001.5 | 4.22 | 71 | 189 |

`[700, 1800]` holds with far more than the 4-5σ the band rule asks for. The
reference's close through docker-proxy was **FIN in 459 of 459** connections
(recorded, never pinned).

**IMPL observation (phase-100 IMPL, Task 10, three solo PB1 runs; added beside
the PLAN figures above, which it does not replace):** one run's reference spread
read n=51, min 1001, **max 1030**, mean 1015.3, **σ 13.72** — wider than every
PLAN run, and still inside `[700, 1800]` (the other two runs' reference maxima
were 1002 and 1003 ms; the subject maxima were 1014, 1021 and 1014 ms).

## Not pinned (SPEC §7.4)

`downstream_cx_total` on a drop listener (reference counts post-filter, subject
pre-filter); the close kind; a partial-byte arm; a half-close arm; any exact
millisecond; any `downstream_listener_filter_*` name.

## Negative controls (measured on PB1)

| NC | mutation | red arms |
|---|---|---|
| NC1 | P0: a second socket clock | F1, F2 (33 / 26 of 50 held open), S `l_false` (17 / 24), CompareBytes; S `l_true`/`l_true_tls` on one run of two |
| NC2 | `pre_cx` `Inc` deleted | S `l_false`, `l_true`, `l_true_tls` |
| NC3 | `0s` fold reverted | Z1 (FIN at 15012 ms), S `l_zero` (1), CompareBytes |
| NC8 | `pre_cx` only under `true` | S `l_false` (0) |

**IMPL observation (phase-100 IMPL, Task 12 fixture runs: NC1 x2, NC2, NC3, NC8; the table
above stays the PLAN's record):** NC2, NC3 and NC8 reddened exactly the arms
listed, NC1 reddened every arm listed for it, and NC1's probabilistic cells
read differently — F2 held open **36 / 37** of 50, S
`l_false` **14 / 13**, and S `l_true` / `l_true_tls` red in **2 of 2** runs
(the PLAN read one of two). Same direction every time; no count is pinned.
