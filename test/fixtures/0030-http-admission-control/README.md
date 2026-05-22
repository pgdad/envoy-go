# Fixture 0030 — http-admission-control

Differential CROSS-SIDE fixture for the
`envoy.filters.http.admission_control` HTTP filter (phase 23).
4 scenarios per SPEC §7.1 + AMEND-2.

## Fixture type

CROSS-SIDE (RequiresReference=true): reference Envoy v1.37.2 +
envoy-go are both spawned. The byte streams from DriveReference +
DriveSubject are compared via CompareBytes. The cross-side
byte-exact guarantee relies on AMEND-2 RNG-independence at P_reject=0:
the always-200 echobackend causes P=0 on both sides, so no RNG
dependency exists.

## 4 scenarios

| # | Scenario | Listener | Filter config | Requests | Expected outcome | Asserted by |
|---|---|---|---|---|---|---|
| (a) | parse_ok | `l_test_a` | full config; enabled=true default | 1x GET / | HTTP 200 (status byte-exact on both sides) | runner `CompareBytes` |
| (b) | all_admit_healthy | `l_test_a` | full config; enabled=true default | 5x GET / | all 200; CROSS-SIDE byte-exact (P=0, RNG-independent per AMEND-2) | runner `CompareBytes` |
| (c) | stat_surface | `l_test_a` | full config | 9x GET / total (a+b+c) + subject /stats scrape | 3 counters present under `hcm_a`; rq_rejected=0; rq_failure=0; **rq_success > 0** | `StatsAsserter.AssertStats` |
| (d) | pass_through_disabled | `l_test_d` | `enabled.default_value=false` | 1x GET / on `l_test_d` | HTTP 200; filter skipped; `hcm_d` rq_rejected=0 AND rq_success=0 AND rq_failure=0 | `StatsAsserter.AssertStats` |

### Assertion wiring (why these interfaces)

This is a CROSS-SIDE fixture (live `DriveReference` + the runner's
`CompareBytes` is the load-bearing (b) leg per SPEC §7.1). The runner takes
the cross-side path and **never** calls `SubjectAsserter.AssertSubject` (that
interface fires only on the reference-less path). So:

- **(a)/(b)** status correctness is the byte-exact `CompareBytes` itself —
  both sides emit identical `status=200` lines. No separate status assertion
  is needed (or possible to wire onto the cross-side path).
- **(c)/(d)** subject-only counter checks run in
  `StatsAsserter.AssertStats(t, refAdminAddr, subjAdminAddr)`, which the
  runner invokes on the cross-side path (step 10) with both admin addresses.
  `AssertStats` dials `l_test_d` (exercising the disabled pass-through), then
  scrapes the subject `/stats/prometheus` once and checks `hcm_a` + `hcm_d`.

## Listener topology

Two listeners in ONE bootstrap to host the enabled config (scenarios a-c)
and the disabled config (scenario d). A single bootstrap (no
MultiListenerDriver) avoids the `freeTCPPort` combined-run flake per 22.2
REVIEW §7.4 — a documented extension of SPEC §7.3's single-listener intent:

- **`l_test_a`** — admission_control (full config; enabled=true by
  default per AMEND-4 INVERSION) + router → c_backend (echobackend
  always-200). Exercises scenarios (a)/(b)/(c). Cross-side CompareBytes
  applies to the HTTP probe bytes from l_test_a.
- **`l_test_d`** — admission_control (enabled.default_value=false) +
  router → c_backend. Exercises scenario (d). Port = subjListenerPort+1
  for subject; refLDTestPort=10031 for reference container. Scenario (d)
  assertion runs entirely in `StatsAsserter.AssertStats` (the cross-side
  CompareBytes covers l_test_a only; scenario (d) emits a placeholder in
  the cross-side stream so the byte stream stays identical on both sides).

## AMEND-2 RNG-independence rationale

The rejection decision is `float64(10000)*P_reject > float64(r%10000)`.
When P_reject=0 (healthy backend, all-success window), this is
`0 > [0..9999]` which is false for every `r` — no random number is
consulted in the admit/reject branch. Both reference Envoy v1.37.2 and
envoy-go produce identical wire shapes (HTTP 200 pass-through) on every
request. The cross-side byte-exact comparison (runner's CompareBytes) is
therefore deterministic and RNG-independent.

## Backend

Reuses the shared echobackend at
`test/helpers/echobackend/cmd/echobackend` (phase-14 Task 10 / D7
settlement). Returns 200 OK with a JSON body for every request.

## Stats surface

3 counters per SPEC §6.6 + AMEND-3 (NO gauges, NO histograms):
- `http.hcm_a.admission_control.rq_rejected`
- `http.hcm_a.admission_control.rq_success`
- `http.hcm_a.admission_control.rq_failure`

Prometheus Rule SN2 flattening: `envoy_http_admission_control_{stat}`
with label `envoy_http_conn_manager_prefix="hcm_a"`.

## Reference container ports

| Listener | Container port |
|---|---|
| `l_test_a` | 10030 |
| `l_test_d` | 10031 |
| admin | 9901 |

## Cross-references

- SPEC §7.1 (4-scenario matrix)
- SPEC §7.3 (single-listener intent; this fixture uses two listeners in one
  bootstrap to host the enabled + disabled configs — see Listener topology)
- AMEND-2 (P=0 RNG-independence ⇒ cross-side byte-exact all-admit)
- AMEND-3 (3-counter stat surface; NO gauges)
- AMEND-4 (enabled absent ⇒ true; enabled.default_value=false ⇒ off)
- ADR-0010 (host.docker.internal for reference container)
- ADR-0194 (algorithm + formula + integer-modulo decision)
- fixture-0031 (phase 23) — sibling boot-reject fixture
- fixture-0025 (phase 21) — nearest 4-scenario subject-asserter precedent
- fixture-0026 (phase 22.1) — nearest cross-side + BootRejectFixture precedent
