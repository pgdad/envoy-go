# 0126-listener-filters-timeout-envelope

Cross-side differential for phase 101 (`listener-filters-timeout-envelope-lift`):
reference Envoy **accepts** `Listener.listener_filters_timeout` outside envoy-go's
old `[1s, 60s]` envelope and enforces it as a millisecond count **truncated** from
the Duration. `0.5s` closes a silent client at ~500 ms; `4294968s` does **not**
wrap (a `uint32` of milliseconds would wrap it to ~705 ms); `0.0005s` truncates to
0, which **disables** the deadline. The un-fixed envoy-go boot-rejects every one of
these values (`listener_filters_timeout 500ms is outside the supported [1s, 60s]
envelope`), so at the tip the whole fixture is RED at subject boot.

`0125` is left byte-untouched: putting these values in its bootstrap would mask
every landed `0125` arm behind the same boot reject (SPEC §7.1).

## Shape

Cloned from `0125`: no YAML, no PKI, no `inputs/`. `renderBootstrap` in
`driver/driver.go` builds **both** sides from one template; only the bind address,
the admin port and the six listener ports differ cross-side.

Every listener is a **one-line diff** from one base: `tls_inspector`,
`fc_indexed` matching `transport_protocol: tls` (body `INDEXED <listener>`), and a
last-resort `default_filter_chain` (body `DEFAULT <listener>`). Every client is
plaintext, so `fc_indexed` never matches and every served body is `DEFAULT`.
All twelve HCM stat prefixes are distinct (a shared prefix panics envoy-go at boot).
Durations are spelled in seconds (`0.5s`), never `500ms` (protojson).

| listener | delta | reference port | `downstream_pre_cx_timeout` |
|---|---|---|---|
| `l_half` | `0.5s`, `continue…: false` | 15126 | **1** (H1) |
| `l_half_true` | `0.5s`, `true` | 15232 | **1** (T1 falls through) |
| `l_one_true` | `1s`, `true` — the MIRROR of `l_half_true` | 15233 | **0** (T2's GET beats it) |
| `l_wrap` | `4294968s`, `false` | 15234 | **0** |
| `l_subms` | `0.0005s`, `false` | 15235 | **0** |
| `l_nofilt_120` | no `listener_filters`, `120s`, `false` | 15236 | **0** |

`BackendCount() == 1`: a `c_unused` STATIC cluster no route dials (the runner
rejects 0; envoy-go boot-rejects an absent `clusters` key).

## Arms (every arm on a side runs concurrently — one ~1.5 s drive per side)

| arm | listener | client | pin |
|---|---|---|---|
| H1 | `l_half` | silent, hold 2000 ms | server closed in [350, 900] ms, 0 bytes |
| W1 | `l_wrap` | silent, hold 1500 ms | still open at 1500 ms |
| M1 | `l_subms` | silent, hold 1500 ms | still open at 1500 ms |
| T1 | `l_half_true` | silent 800 ms, then GET | 200, `DEFAULT l_half_true` |
| T2 | `l_one_true` | silent 800 ms, then GET | 200, `DEFAULT l_one_true` |
| N1 | `l_nofilt_120` | GET at 0 | 200, `DEFAULT l_nofilt_120` |
| S | all six | — | `downstream_pre_cx_timeout` BY VALUE on each side's own `envoy_listener_address` label on `/stats/prometheus`; a **missing** series is a hard failure |

The reference label is `0.0.0.0_<in-container port>`; the subject label is its
bind address with `:` and `.` folded to `_` (`127_0_0_1_<port>`).

**T1/T2 pin the counter, not only the body.** Under `true` the body cannot tell
"fell through at 0.5 s" from "the GET arrived first": both are `DEFAULT`. The body
pin only guards chain selection; the discriminator is S's `l_half_true` **1**
beside the mirror `l_one_true` **0**. Both sides served `DEFAULT l_half_true` and
`DEFAULT l_one_true` on every run (PLAN measurement: R1-R3, MEAS1-3, NC1, NC4).

## Window — MEASURED through the harness's host `-p` path

The clock starts when the client's dial returns. Pooled over three runs of a
measurement variant of this driver (H1 plus 30 concurrent replicates on
`l_half`, `wantPreCx` 31; never committed), shape A applied:

| side | n | min | max | mean | σ | (mean-350)/σ | (900-mean)/σ |
|---|---|---|---|---|---|---|---|
| reference (via docker-proxy) | 93 | 500 | 508 | 502.89 | 2.54 | 60 | 156 |
| subject (shape A) | 93 | 500 | 501 | 500.95 | 0.23 | 669 | 1769 |

The three plain runs (R1-R3, one H1 per side each) read reference 501 / 501 / 502
ms and subject 501 / 500 / 501 ms. `[350, 900]` holds with far more than the 4-5σ
the band rule asks for, and stays clear of an immediate close and of a mis-parse to
the 1 s mirror (≥ 1000 ms on both sides in `0125`). Against `0125`'s worst observed
reference spread (σ 13.72, max 1030 ms at 1 s), the margins are still 11σ low and
29σ high. The reference's close through docker-proxy was **FIN in 93 of 93**
(recorded, never pinned).

## Not pinned (SPEC §7.7)

Any reject message; a close KIND; an exact millisecond; `downstream_cx_total` on
a drop listener (the reference counts post-filter); any
`downstream_listener_filter_*` name; the 1 ms immediate-request outcome.

## Negative controls (measured on shape A, one run each)

| NC | mutation in `parseListenerFiltersTimeout` | red arms | green arms |
|---|---|---|---|
| tip | none (no shape A) | subject boot reject (`500ms is outside the supported [1s, 60s] envelope`, 3 attempts); no arm driven on either side | — |
| NC1 | narrow the return to `uint32` | W1 (subject FIN at 704 ms), S `l_wrap` (subject 1), CompareBytes (`W1 … open_at_1500ms=false`) | H1, M1, T1, T2, N1, every other S cell |
| NC4 | round instead of truncate (`+ 500000` before `/1e6`) | M1 (subject FIN at 1 ms), S `l_subms` (subject 1), CompareBytes (`M1 … open_at_1500ms=false`) | H1, W1, T1, T2, N1, every other S cell |
