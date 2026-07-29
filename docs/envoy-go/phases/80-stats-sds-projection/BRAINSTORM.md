# BRAINSTORM 80 — stats-sds-projection

**Stage:** BRAINSTORM. Lifecycle-state `DONE -> 1`. **ROW 80 REGISTERED `in-progress`**; sentinel `want` bumped **111 -> 112** in the same commit.
**Base:** master `80cafb7e`. Worktree `/home/esa/git/envoy-go-wt-p80bs`, branch `phase-80-brainstorm`.
**Family:** **TWENTY-THIRD §9 Observability-family row** (row 79 held TWENTY-SECOND; row 75 TWENTY-FIRST). `TWENTY-THIRD … Observability-family row` was collision-checked ⇒ **0** prior occurrences.
**Docs-only:** ZERO production `.go`, ZERO test `.go` committed.
**BYTE-UNTOUCHED at this stage:** `DECISIONS.md` · `BEHAVIOR_CONTRACT.md`.

**Dispatch:** four investigation agents (A1 reference probe / A2 subject-side reproduction / A3 validation leg / A4 cost + bookkeeping) on disjoint remits, each in its own detached worktree with private scratch and a private port band **outside the differential harness's 20000-31007 range** (A1 41000-41099, A2 41100-41199, A3 41200-41299, A4 none), plus controller re-derivation of every load-bearing claim. Docker used by A1 only; containers named `p80a1-<arm>` and torn down **BY NAME**, teardown proof empty.

---

## 1. THE PICK, SELF-PICKED per the 2026-07-12 standing directive

**The `sds.` Prometheus-projection gap plus its registration-time silent-skip twin.** Twenty registered `sds.<secret>.*` stat names are served on the flat `/stats` and **silently absent from `/stats/prometheus`**; separately, an operator secret name containing a **hyphen** causes all five of that secret's counters to be **skipped at registration with zero diagnostics**. These are the two halves of the same surface and this row takes both.

Chosen over the alternatives in §8 because it is the **smallest defensible candidate** that is (a) already reproduced end-to-end at this tip, (b) the last remaining arm of a departure this project has been narrowing for three phases, and (c) paired with a defect whose only accidental signal **this row would otherwise destroy** (§5.4 — the decisive argument for taking both halves together).

### 1.1 ⚠️ THE ROW ID IS `80`, NOT `79.1` — a doctrine adjudication, decided against three landed documents

Four in-tree documents forward-reference **"row 79.1"** as the thing that closes this gap (`ROADMAP.md` row 79's cell ×3, `BEHAVIOR_CONTRACT.md` ×4, `STATE.md` ×2, phase-79 `PROGRESS.md` ×3; 36 occurrences across 8 files). **That label is itself a doctrinal error, and this stage does not inherit it.** Three independent lines converge, each executed:

1. **`BOOTSTRAP_PROMPT.md` §6.2 was never invoked.** A split requires the parent row to become `status = in-progress` with its `sub-phases` column listing the children, plus **an ADR explaining the split**. Phase 79 crossed neither §6.1 threshold (it shipped **12** tasks against ~25, well under the LoC gate), authored no split ADR, and closed `done` with an **empty** `sub-phases` field.
2. **No sub-phase ROW has been created since 32.2.** All **31** dotted row ids in the file are `≤ 32.2`; the last **47** rows (33-79) are flat. Verified by extracting every row id mechanically.
3. **When this project DOES split, the legs live in the parent's `sub-phases` PROSE, never as separate rows.** Rows **40 / 42 / 43** are `done` with `sub-phases` cells enumerating legs `40.1-40.3`, `42.1/42.2a/42.2b`, `43.1/43.2a/43.2b` — and **not one of those legs has a row of its own**. Phases 46 and 61 likewise have sub-phase *directories* cited in-tree with a single flat row.

⇒ Chartering `79.1` would require either **reopening closed row 79** (falsifying `ADR-0301 COMPLETE`, four documents, and re-opening sentinel check (1) on a row recorded closed) **or** inventing a row shape no row in the file has: a sub-phase child of a `done` parent whose `sub-phases` field is empty. **Row 80 requires no fabrication.** Recorded as an adjudication so the SPEC can check it rather than re-derive it.

⚠️ **THE COST OF THIS CHOICE IS NAMED, NOT HIDDEN.** The `79.1` forward references go stale. `BEHAVIOR_CONTRACT.md:158`, `:5078` and `:5080` are **live normative statements** (`"Until 79.1 lands, treat sds.* as flat-/stats-only"`) and are **MANDATORY edits at this row's IMPL**. **`ROADMAP.md` row 79's three occurrences are INVARIANT-BLOCKED** — §Schema `:18` permits updating only `status` and `sub-phases` on an existing row — so they are **NAMED as a permanent documentary discrepancy**, the same bind as the malformed row 78 (§7.2). The phase-79 phase documents (25 occurrences) are **historical record and must NOT be rewritten.**

---

## 2. Sentinel — RE-RUN MECHANICALLY AT THIS TIP. IT DOES NOT FIRE; `stop` WAS NOT CREATED

`ls stop` ⇒ `No such file or directory`, and it must not be created. Run on `master` @ `80cafb7e`, clean tree.

| check | ACTUAL output | negative control, observed firing |
|---|---|---|
| **(1)** | **NOTHING**, and no `GATE FAIL` at `want=111` | `want=110` ⇒ `GATE FAIL: examined 111 data rows, expected 110`; `want=112` ⇒ `… expected 112`; row 79 doctored to `in-progress` on a scratch copy ⇒ `NOT DONE: row 79` (doctoring script self-reported `rows doctored: 1`) |
| **(2)** | **FIVE** — `:189 :199 :209 :215 :223` | union **5** vs long-form-only **4**: the one-arm form is still blind to the Operational-tooling short form |
| **(3)** | `NEVER OPENED: gRPC`, `NEVER OPENED: WASM` | invented slug ⇒ `NEVER OPENED: ZZZ-nonexistent`; **registered** slug `Observability` correctly prints nothing ⇒ the loop discriminates, it does not merely print |

Input measured at **227 lines / 1 007 766 bytes / 13** bare `candidates:` hits (against the sentinel's narrower 5), so an empty result could not read as a zero result.

⚠️ **CHECK (2) IS UNCHANGED AND THIS ROW NARROWS NOTHING — STATED, NOT FORECAST.** The twenty-fourth consecutive phase at which it did not go down. The candidate sentence keeps its text until a row CLOSES, and this row's subject is not drawn from any of the five paragraphs — it is the residue of a *closed* row's departure.

### 2.1 The `want` bump was REHEARSED WITH CONTROLS BEFORE THE EDIT

On a scratch copy carrying a synthetic well-formed row 80: `want=112` ⇒ **only** `NOT DONE: row 80`; `want=111` ⇒ `NOT DONE: row 80` **plus** `GATE FAIL: examined 112 data rows, expected 111`. The id field parses as `80` and the row does not join the malformed set (§7.2).

### 2.2 The leak-check was rehearsed on the new cell ALONE, both matchers negative-controlled

New-cell phrase count **0**; family slugs printed: **`Observability-family row` only**, which is **REGISTERED 50× elsewhere** in `ROADMAP.md`, so it silences nothing. NCs: doctoring the cell to carry the short phrase ⇒ **1**; doctoring it to carry `gRPC-family row` ⇒ printed, **and re-running check (3) on the doctored file showed `gRPC` vanish while `WASM` remained** — i.e. the leak is proven to **silence check (3) BY MENTION**, not merely to be greppable. **`grep` cannot tell a mention from a use.**

⚠️ **No sentinel matcher string is written into `ROADMAP.md` by this stage.** Writing them in *this* file is safe; writing them *there* is not.

---

## 3. THE DEFECT — REPRODUCED BY EXECUTION, NOT DESCRIBED

### 3.1 The projection drop: 20/20 dropped against a discriminating 8/8 control

Every one of the 20 names returns the terminal `ExtractTags` rejection verbatim:

```
stats: name "sds.server_cert.update_success" has no recognized top-level segment (want one of the
12: cluster.|http.|listener.|server.|runtime.|access_logs.|tracing.|wasm.|mongo.|kafka.|redis.|thrift.)
and no recognized mid-name segment (want one of the 4: .http_local_rate_limit.|.http_bandwidth_limit.|.rbac.|.zookeeper.)
```

**The control is the load-bearing half** — in the same registry and the same `WriteProm` call, 8/8 projected, including all three roots phase 79 landed:

```
cluster.upstream_svc.upstream_rq_total  -> "cluster.upstream_rq_total"  labels=[{envoy_cluster_name upstream_svc}]
http.ingress_http.downstream_rq_total   -> "http.downstream_rq_total"   labels=[{envoy_http_conn_manager_prefix ingress_http}]
server.live                             -> "server.live"                labels=[]
runtime.num_keys / access_logs.… / tracing.…  -> byte-mirrored, labels=[]
TALLY: subject dropped 20/20 ; control projected 8/8
```

⇒ **The residual live drop at this tip is EXACTLY the 20 `sds.` names.** The other 10 of the banked 30 are closed by phase 79.

### 3.2 End-to-end: PRESENT in flat, ABSENT in prom

`cmd/envoy-go` booted on `test/fixtures/0103-xds-sds-server-cert/envoy-go.yaml` against a real in-process `sdsserver`, with one real TLS handshake driven so the counters are non-zero:

| endpoint | HTTP | bytes | lines | `sds` result |
|---|---|---|---|---|
| `/stats` | 200 | 1297 | **35** | **5 `sds.*` lines PRESENT** |
| `/stats/prometheus` | 200 | 4726 | **86** | **0 `envoy_sds*` lines** |

⚠️ **A TRAP NAMED SO THE SPEC DOES NOT FALL IN IT:** `grep sds` on the prom output returns **12** lines, every one `envoy_cluster_*{envoy_cluster_name="sds_cluster"}` — the SDS *cluster's* stats, not the per-secret counters. **Assert on `envoy_sds`, never on `sds`.**

The phase-79 skip log, verbatim from the live subject, **one line per `WriteProm` call** (a second scrape emitted a second identical line):

```
stats: WriteProm skipped 5 registered metric name(s) with no recognized top-level segment:
sds.server_cert.init_fetch_timeout, sds.server_cert.update_attempt, sds.server_cert.update_failure,
sds.server_cert.update_rejected, sds.server_cert.update_success
```

### 3.3 ⚠️ THE DENOMINATOR IS 20 PER CORPUS AND **5 PER PROCESS** — do not conflate them

Mechanically re-extracted over all `test/fixtures/*/{envoy-go,envoy}.yaml`, classified by `api_config_source` vs `path_config_source`:

| secret | fixture(s) |
|---|---|
| `server_cert` | `0103-xds-sds-server-cert` |
| `validation_ca` | `0108-xds-sds-validation-context`, `0109-xds-sds-combined-validation-context` |
| `rccf_validation_ca` | `0110-tls-require-client-cert-false` |
| `edf_validation_ca` | `0111-tls-cvc-empty-dynamic-fallback` |

**4 distinct `api_config_source` secrets × 5 counters = 20. CONFIRMED.** `0024-http-oauth2`'s `client_secret`/`hmac` are `path_config_source` (served by `internal/sdsfile.Watcher`, never reaching `boot.go:201`) and are **EXCLUDED** — ⚠️ **including them gives 6×5=30 and would coincidentally "confirm" the banked 30-name total for the wrong reason.**

⚠️ **BUT ONE PROCESS REGISTERS EXACTLY 5.** `boot.NewSDSProvider`'s `seen > 1` guard rejects more than one SDS-bound downstream TLS context, so `RegisterSDSStats` is called **at most once per boot** (**1** non-test call site, `internal/boot/boot.go:201`; 15 test call sites). **Any end-to-end or fixture assertion must expect 5, not 20.** The 20 is a corpus figure and has never been a single-process figure.

---

## 4. INHERITED CLAIMS REFUTED OR CONFIRMED BY EXECUTION

### 4.1 ⚠️ CONFIRMED against a LIVE reference — the hoisting shape, and the contract's own "owed re-pin" is DISCHARGED

`BEHAVIOR_CONTRACT.md:5078` says `sds.` needs a label-hoisting arm, *"an `envoy_xds_resource_name` promotion, itself owed a re-pin against a live reference."* **Probed against the pinned `envoyproxy/envoy:contrib-v1.37.2`, two distinct secret names in one dump:**

```
# TYPE envoy_sds_update_success counter
envoy_sds_update_success{envoy_xds_resource_name="server_cert"} 1
envoy_sds_update_success{envoy_xds_resource_name="validation_ca"} 1
envoy_sds_update_attempt{envoy_xds_resource_name="server_cert"} 2
envoy_sds_init_fetch_timeout{envoy_xds_resource_name="server_cert"} 0
```

**LABEL-HOISTED. Key `envoy_xds_resource_name` — CONFIRMED, not guessed** (`envoy_sds_secret=` ⇒ **0** hits; `envoy_xds_resource_name=` ⇒ **66**). Two distinct secret names sharing one metric name is the discrimination the question needs; a single name cannot tell a label from a constant. Negative-controlled in the same dumps against a known-hoisting family (`envoy_cluster_upstream_cx_total{envoy_cluster_name=…}`) and a known-byte-mirror family (`envoy_server_live{}`), so the method is proven able to see both a label and its absence.

⚠️ **THIS IS THE OPPOSITE OF THE PHASE-79 RESULT AND THAT MATTERS.** Phase 79 probed the same question for `runtime.`/`access_logs.`/`tracing.` and found **all three byte-mirror**, refuting its own BRAINSTORM. Here the inherited claim **HOLDS**. The lesson is that the probe decides, not the pattern.

### 4.2 ⚠️ THE DECISIVE NEW FINDING — the reference hoist is SINGLE-SEGMENT and NON-GREEDY

Second arm, secret name `alpha.beta.gamma`:

```
flat:  sds.alpha.beta.gamma.update_success: 1
prom:  envoy_sds_beta_gamma_update_success{envoy_xds_resource_name="alpha"} 1
```

The reference extracts **only the first dot-delimited segment** and **leaks the remainder into the metric NAME**. Its rule is effectively `^sds\.([^.]+)\.` → hoist group 1, drop the matched span, then dots→underscores on the residual. **It is not greedy-to-last-dot and does not treat the whole secret name as one unit.** This makes the arm structurally **SN1-shaped** (`<root>.<dynamic>.<rest>`), identical in form to `cluster.<n>.<rest>`.

### 4.3 ⚠️ A FALSE DRIFT-CORRECTION, BANKED IN FIVE DOCUMENTS — the SPEC was RIGHT

The phase-80 brief, `STATE.md:18`, `PLAN.md:174`, `PLAN.md:585` and `PROGRESS.md:111` all assert *"the phase-79 SPEC's secret→fixture attribution is SWAPPED — `edf_validation_ca` is `0110`, `rccf_validation_ca` is `0111`."* **That correction is itself wrong.** Executed, and independently re-derived by the controller from the fixtures:

| fixture | secret — and the strongest evidence is EXECUTABLE GO |
|---|---|
| `0110-tls-require-client-cert-false` | **`rccf_validation_ca`** — `0110/driver/driver.go:52`, plus `envoy.yaml:90`, `envoy-go.yaml:83`, README, expectations (7 sites) |
| `0111-tls-cvc-empty-dynamic-fallback` | **`edf_validation_ca`** — `0111/driver/driver.go:56`, plus `envoy.yaml:90`, `envoy-go.yaml:85`, README, expectations (9 sites) |

**Negative control both ways: `edf_` in `0110` ⇒ 0; `rccf_` in `0111` ⇒ 0.** The mnemonics corroborate (`rccf` = **r**equire-**c**lient-**c**ert-**f**alse; `edf` = **e**mpty-**d**ynamic-**f**allback), as does an independent third document, `phases/68-tls-cvc-empty-dynamic-fallback/PROGRESS.md:18`. **`SPEC.md:114` states it CORRECTLY.**

⇒ **`reference_a_drift_correction_is_itself_a_claim` firing live, and it propagated into four documents plus the stage brief.** The two rolling documents (`STATE.md`, `next-prompt.txt`) are corrected at this stage; `PLAN.md`/`PROGRESS.md` are historical record and are **named, not rewritten**.

### 4.4 ⚠️ "THREE OF FOUR SINKS ARE NO-OP BY CONSTRUCTION" IS TRUE OF A BYTE-MIRROR ARM AND FALSE OF THIS ONE

`ExtractTags` has **five** production consumers: `internal/stats/prom.go` plus `internal/statssink/{label,graphite,dogstatsd,otlp}.go`. **Only `WriteProm` drops on error**; the four sinks degrade gracefully and emit the full dotted name untagged. Captured live over real loopback UDP:

```
labelMapper: 1 families out; name="sds.server_cert.update_success" labels=0
dog_statsd datagram (42 B): "envoygo.sds.server_cert.update_success:7|c"
graphite   datagram (42 B): "envoygo.sds.server_cert.update_success:7|c"
```

A hoisting arm shortens the name to `envoygo.sds.update_success` and adds tags ⇒ **these wire bytes MOVE.** This row is **emphatically not sink-neutral**, which is exactly the asymmetry phase 79 cited when deferring it — and phase 79's own hoisting NC already moved all four sink goldens at two injection sites. **The deferral rationale is confirmed, and the cost it predicted is this row's.**

---

## 5. THE SECOND HALF — the registration-time silent skip

### 5.1 ⚠️ THE TRIGGER IS IDIOMATIC: A HYPHEN

`NamePattern = ^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$` (`internal/stats/registry.go:48`). An exhaustive ASCII 0x20-0x7e sweep gives **31 rejected printable bytes**, hyphen among them. Controller-re-derived independently:

```
secret="server_cert"                            IsValidName(...)=true
secret="server-cert"                            IsValidName(...)=false   <- HYPHEN
secret="my-tls-cert"                            IsValidName(...)=false   <- HYPHEN
secret="spiffe://cluster.local/ns/default/sa/x" IsValidName(...)=false   <- SPIFFE URI
secret="my.dotted.cert"                         IsValidName(...)=true    <- DOTS ACCEPTED
```

**The most idiomatic Kubernetes/Envoy secret naming convention silently destroys the stat family.** Nothing on the config path validates it — `xds.ParseSDSConfig` (`internal/xds/config.go:22-30`) rejects only `name == ""`.

### 5.2 Live boot on `server-cert`: green listener, working TLS, 5 of 36 stats gone, ZERO bytes of stderr

```
ready=true ctx.Err=<nil>
STDOUT: envoy-go listener l_sds_tls ready on 127.0.0.1:41212 / envoy-go ready
STDERR: (0 bytes)
/stats: 31 lines, sds.* = 0     [control with server_cert: 36 lines, sds.* = 5]
```

Not a panic, not a boot reject — a **silent skip**. TLS still terminates on the SDS-fetched leaf, so nothing else signals.

### 5.3 `incNil` is COMPLETE — and proven load-bearing rather than assumed

Pointer-level, per field, with a registry-walk delta: valid names ⇒ all five non-nil, delta **5**; every rejected name ⇒ all five nil, delta **0**, **all-or-nothing, never partial**. The five unexported fields are read in exactly one file, only inside the five `incXxx` methods, all routing through `incNil`; **12** production consumers, all `p.stats.incXxx()`; no getter, no `reflect`, no `unsafe`. **Arm F deleted the `if c != nil` guard and produced a boot-time SIGSEGV on the production path** (`FetchInitialCertificate` → `incUpdateAttempt` → `Counter.Inc`), with a valid-name control proving the crash is the nil path and not the edit. Confirms `reference_nil_stats_counter_inc_crashes_goroutine` in this exact code path.

⚠️ **One vacuity flagged for the SPEC:** the `if s != nil` **receiver** guards in all five methods are **dead in production** — one call site, always non-nil. Exercised only by `TestRegisterSDSStats_NilReceiverSafe`.

### 5.4 ⚠️ THE ARGUMENT FOR TAKING BOTH HALVES IN ONE ROW — this row would otherwise DESTROY the only signal

**The phase-79 skip log is an INVERTED signal.** It fires on the **valid** arm (names exist, don't project) and is **silent** on the **invalid** arm (names never registered — nothing left to skip). So today the only in-tree diagnostic **goes quiet exactly when the counters are missing**, and its silence is indistinguishable from *"the projection gap was fixed."*

⇒ **Landing the projection arm alone REMOVES that log for the valid case, after which the silent skip has no signal at all in either direction.** A stat that silently does not exist would then read identically to a stat that is always zero. **That is why the validation leg is in-scope, not a follow-on.**

### 5.5 The `Registry.Walk` deadlock — reproduced, NOT reachable today, and the SPEC must keep it that way

`RegisterSDSStats` invoked inside `reg.Walk`'s callback **DEADLOCKS** (reproduced live at a 3 s bound): `Walk` holds `r.mu.RLock` across the iteration (`registry.go:138-144`) while `getOrRegister` takes `r.mu.Lock` (`:179`); Go's `RWMutex` is not reentrant. Confirms `reference_registry_walk_lock_inversion`.

**Not reachable at this tip** — all three non-test `Walk` sites (`internal/admin/stats.go:56`, `internal/stats/prom.go:59`, `internal/statssink/mapping.go:24`) have pure accumulator closures, and phase 79's log is deliberately emitted *after* the walk returns. ⚠️ **Two designs the SPEC must therefore EXCLUDE:** (a) instrumenting the skip with a **counter registered from inside the projection path** — the exact phase-79 deadlock, which is why D-SPP-3 resolved to a log; (b) **lazy registration at first increment**, since `NewCounterIfAbsent` is permitted post-`Freeze` and an increment could then fire under a scrape's `RLock`.

### 5.6 ⚠️ A LANDED ADR CONSEQUENCE IS OVER-ATTRIBUTED — refuted by execution at five insertion points

`ADR-0300 §Consequences (iii)` records that this candidate is *unblocked because a boot panic is now visible* after phase 78. **For this specific insertion point that is false, and the visibility never depended on phase 78.** Per ADR-0300 §Consequences (ii) — verbatim: *"A future row applying that method must vary the INSERTION POINT across the pre-anchor and post-anchor windows: a single pre-anchor injection reports 'caught' and conceals the case that is genuinely uncovered"* — the insertion point was varied across **five** arms:

| arm | insertion site | wall | exit | panic printed |
|---|---|---|---|---|
| A | inside `RegisterSDSStats`, invalid-name branch | 10 ms | **2** | **YES** |
| B | `main.go` after the `NewSDSProvider` err-check (pre-anchor) | 9 ms | **2** | **YES** |
| C | after `defer lm.Stop()` (the (ii) residual window) | 11 ms | **2** | **YES** |
| D | after the `flusherDone` defer, before the ready sentinels | 10 ms | **2** | **YES** |
| **E** | site D **with phase-78's leading `cancel()` deleted** | **12.003 s** | **-1** | **NO** |

**Mechanism:** every `defer` in `main()` is at lines **186, 295, 328, 331, 336, 384** — all strictly **after** line 156, where `boot.NewSDSProvider` is called. **A registration-time-validation panic unwinds an EMPTY defer chain and prints for the plainest possible reason.** Arms C/D/E show where phase 78's fix *is* load-bearing; arm A shows this site is not one of them.

⚠️ **Hang-vs-exit was discriminated by OUTPUT, never by status** (`reference_timeout_exit_124_shared_by_healthy_and_hung`): arm E printed only nine statsd flush-noise lines (**1269 B**, reproducing ADR-0300 §Context (f) byte-for-byte) with no panic text, while the valid-name control exited at 12.013 s with **68 B of ready sentinels** — the same status, opposite meanings.

---

## 6. THE DESIGN FORK the SPEC must adjudicate

**Dots are ACCEPTED in a secret name** (`IsValidName("sds.my.dotted.cert.update_success")` ⇒ `true`), and a dotted name is **fully reachable from config** — executed end-to-end: booted with secret `my.dotted.cert`, boot succeeded, TLS verified, five `sds.my.dotted.cert.*` lines on flat `/stats`, **0** on prom. So the SN1 split is **not well-defined by construction** and the SPEC must choose:

| option | behavior on `my.dotted.cert` | note |
|---|---|---|
| **(a) first-dot split** | label `="my"`, base `envoy_sds_dotted_cert_update_success` | ⚠️ **This is what the LIVE REFERENCE does** (§4.2). Reference-faithful; "wrong-looking" but conformant. |
| **(b) last-dot split** | label `="my.dotted.cert"`, base `envoy_sds_update_success` | Well-defined because all five suffixes are fixed and dot-free. **DIVERGES from the reference.** |
| **(c) registration-time dot-free guard** | registration skipped | Behavioral change: silently drops names that boot fine today — the very defect §5 is repairing. |

⚠️ **A2 independently reasoned that (a) is "wrong" and (b) is correct — without having probed the reference, which it explicitly flagged as unverified. A1 supplies the missing fact and it inverts the conclusion.** Under this project's differential-conformance discipline **(a) is the presumptive answer**, and the SPEC must state it as a *fidelity* decision rather than a correctness one. **This is a genuine cross-agent synthesis: neither agent could reach it alone, and their blind axes were complementary.**

---

## 7. FINDINGS OUTSIDE THE SUBJECT, recorded because they were found by execution

### 7.1 ⚠️ BROKEN-GATE SHAPE **SIXTEEN** — two defects that CANCEL in the metric the gate measures

`ROADMAP.md` row 78 carries **one unescaped inner `|`** *and* **no trailing `|`**. In an `awk -F'|'` field count those **cancel**: `NF=8`, byte-identical to a well-formed row. Consequently **no single predicate finds every malformed row**:

| predicate | finds | verdict |
|---|---|---|
| escape-**blind** `NF!=8` (the naive reach) | 17 rows | **16 false positives** (escaped `\|`), and **MISSES 78** |
| escape-**aware** `NF!=8` | 57, 69 | **MISSES 78** — the cancellation |
| escape-aware last-field-non-empty | 78 | misses 57, 69 |
| **the disjunction of the last two** | **57, 69, 78** | correct |

### 7.2 ⚠️ THE BANKED CLAIM IS TRUE UNDER ITS OWN PREDICATE AND FALSE UNDER THE ONE THAT MATTERS — and two agents split on it

**Both halves are executed and they are compatible; the disagreement is which predicate defines "malformed."**

- Under **"no trailing `|`"** (`awk '/^\| *[0-9]/ && !/\|$/'`) **row 78 is genuinely the ONLY one — 1 of 111**, NC-confirmed by stripping row 79's trailing pipe on a scratch copy ⇒ 2. **The banked claim holds as stated.**
- Under **"summary survives GFM render"**, it does **not** hold: **three** rows lose content. ⚠️ **And this is the predicate the banked claim's own justification appeals to** — it says *"so GFM drops the phase-78 IMPL summary from the rendered table."* That consequence was **inferred from the column count and never rendered**; this stage rendered it.

Rows **57**, **69** and **78** all carry unescaped `|` in the summary cell (row 57: `` `|#k:v` ``, a statsd separator; row 69: `w==nil || w.GetValue()`, a Go operator; row 78: a bare ` | ` before `**IMPL done**`). Confirmed by two independent methods that agree — a sed-normalized `awk` pass and a character-walk splitter that honours `\` escapes. **Render consequence EXECUTED with two renderers** (pandoc `-f gfm` and python-markdown), measuring rendered-vs-source cell length against **well-formed controls**:

| row | src summary chars | rendered | verdict |
|---|---|---|---|
| 57 | 5676 | 4799 | **TRUNCATED** (~689 source chars lost) |
| 69 | 11532 | 7278 | **TRUNCATED** (~4032 lost) |
| **74** (control, carries **escaped** pipes) | 18875 | 18043 | **INTACT** |
| **76** (control, well-formed) | 17072 | 16204 | **INTACT** |
| 78 | 5363 | 2369 | **TRUNCATED** (~2877 lost) |

Row 78 is unique only in **also** dropping its trailing `|`. All three are **INVARIANT-BLOCKED** from repair by §Schema `:18`. ⚠️ **Row 74 is the important control:** it carries escaped pipes and renders intact, which is what proves escape-awareness is required rather than optional.

⚠️ **The right gate is therefore a CONJUNCTION, and `NF==8` is not part of it.** `NF==8` **passes on row 78** (§7.1). The two assertable predicates are `!/\|$/` (finds 78) and escape-aware excess-cell detection (finds 57, 69). ⚠️ **`ROADMAP.md:141` additionally misstates its own denominator — it says *"the ONLY MALFORMED ROW of 110"* where there are 111 data rows**, stale by one because it was written before row 79's own row existed.

### 7.3 ⚠️ FOUR OF MY OWN PROBES WERE THE UNRELIABLE THING, AND THEIR CONTROLS CAUGHT THEM

Recorded because it is the same species as phase 79's two controller probe defects. (i) A `grep -o '\|'` pipe tally contradicted the field audit — `grep -o` counting was unsound, the character walk authoritative. (ii) A `<tr>`-anchored `<td>` extractor returned **empty** and would have read as "0 cells" — pandoc emits `<tr class="odd">`; **an empty output is not a zero result**. (iii) A needle-based render probe reported `DROPPED` for **well-formed control row 76**, which is what exposed it; replaced by a length comparison. (iv) My own escape-blind audit produced **16 false positives** and I briefly drafted a refutation from it. **In every case the control, not review, caught it.**

### 7.4 A non-finding, recorded so it is not inherited

A2's invented control `listener.0.0.0.0_41101.downstream_cx_total` projected with `envoy_listener_address="0"` — **an artifact of hand-invented input.** Real listener names are pre-sanitized (`listener.___41113.…`, no dots) and project correctly. **There is no listener defect here** — the same phantom-defect trap as phase 78's `reference_probe_input_is_a_claim`.

### 7.5 ⚠️ THE MOST REUSABLE FINDING OF THIS STAGE — A RECURSIVE `grep` IN THIS HARNESS IS BLIND TO `next-prompt.txt`

**Found INDEPENDENTLY by the controller and by A4, from opposite directions, and it invalidates a class of audit this project performs at every stage.**

In this Bash environment `grep` is a **shell function**, not GNU grep: it execs `claude -G --ignore-files --hidden -I --exclude-dir=.git …` in ugrep-compatible mode. **`--ignore-files` honours `.gitignore`**, and `.gitignore:2` lists **`next-prompt.txt`** — which is nevertheless **TRACKED**. Therefore:

```
grep -rn 'want=111' <repo>            -> 8 hits / 6 files   (next-prompt.txt ABSENT)
command grep -rn 'want=111' <repo>    -> 11 hits / 7 files  (next-prompt.txt PRESENT, :17 :41 :46)
grep -c  'want=111' <repo>/next-prompt.txt -> 3             (direct file access works)
```

⚠️ **THIS IS LIVE FOR EVERY "AUDIT WHERE THE NUMBER LIVES" SWEEP**, because the sentinel's live `want` figure and the entire router live in exactly the file directory-recursion drops. A stage that sweeps recursively for a figure and repairs what it finds will **silently leave the router stale** — which is the precise mechanism the phase-79 headline warned about, one layer lower.

⚠️ **`git check-ignore` IS NOT A VALID PROBE FOR THIS.** It reports `next-prompt.txt` as *"not ignored"* (exit 1) because **git exempts tracked files from `.gitignore`**, while ugrep does not. **Two tools disagree for a principled reason, and the one you would reach for to check reassures you wrongly.**

**Reliable forms:** `command grep -r`, `git grep`, an explicit file list, or a recursive grep handed the FILE path directly (no directory traversal). ⚠️ **`reference_next_prompt_tracked_despite_gitignore` records that the file is tracked despite `.gitignore`; it does NOT record that this makes it invisible to the session's own recursive grep.** That is the new half.

⚠️ **AND MY OWN DIAGNOSTIC CONCEALED IT ONCE.** A control probe reported `grep -rl 'AUTONOMOUS LOOP CONTROL' <repo>` ⇒ **1 file**, which I read as "the file is visible." The matching file was `phases/61-http3-downstream-listener/PLAN-61.3.md`; `next-prompt.txt` was being skipped the whole time. **I printed a COUNT where only the NAME discriminates** — `reference_output_volume_is_not_output_content`, committed inside the probe built to test for exactly this class of blindness.

---

## 8. REJECTED ALTERNATIVES, each costed at THIS tip

Banked costs go stale (`reference_deferred_candidate_cost_restale`); these were re-derived, and **five banked figures did not reproduce** (flagged inline). The deferred-candidate menu is `ROADMAP.md:190 :200 :210 :216 :224` **after this row's insertion shifted every one by +1** — cite them by content, not by line.

⚠️ **THE MENU IS NOT COSTED, AND CALLING IT "THE SELF-PICK MENU" OVERSTATES IT.** `grep -obE '[0-9]+(-[0-9]+)? tasks'` returns **no match** on four of the five paragraphs: HTTP/3 (7 uncosted candidates), xDS (9 uncosted), Runtime (4 uncosted) and Operational-tooling (3 uncosted) carry **zero** task bands. **Only the Observability line carries costs** — six bands, tied to ADR-0276/0277/0283/0284/0285 plus the `11-14` projection parent. Any stage claiming the menu is costed is wrong for 4 of 5 lines.

| candidate | cost | why not now |
|---|---|---|
| **The `sds.` projection + validation row** | **CHOSEN** | reproduced end-to-end; closes a three-phase-old departure; the two halves are coupled (§5.4) |
| The `STATE_HISTORY.md` archive gap — ⚠️ **banked 40 REFUTED: it is 36 for phases 67-75, and 58 overall** | **4-6** | pure bookkeeping; **this stage honours the rule for its own eviction** and leaves the backfill named. ⚠️ **Defensible ONLY if it lands the set-difference as a recurrence guard** (`comm -13` of present-vs-git bullets, must print empty); a bare 36-bullet paste re-drifts at the next eviction and 58 is the real hole. ⚠️ **NEW: `phase 77 PLAN done` is in NEITHER file — the cap already leaks, at the most recent phase but one** |
| The documented **public import path** defect (ZERO in compiled Go; `go build ./...` exit 0) | ⚠️ **NOT DEFENSIBLE AS A PHASE** | banked 36/SEVEN files is **stale — 33/SEVEN at this tip** (36 reproduces exactly at the two older tips; the drift is `next-prompt.txt` being rewritten). **It cannot reach a clean end state:** `DECISIONS.md:142` is an immutable ADR that must *name* the wrong path, so the count can never reach 0; **7 of the 32 sit inside CLOSED rows' summary cells** which §Schema `:18` forbids editing; the 8 root `PROGRESS.md` hits are **pasted `go test` output**; and any `count==0` guard is unsatisfiable by construction. **The problem is not its size — it is that there is no green.** |
| A **mechanical stat-surface recount** to replace the documentary 1207 | **8-11**, unchanged | strictly larger; and a `+0` row is the cheap place, which this row is **not**. Every banked figure reproduces (**210** sites, **27**+1 ledger rows, **5** fan-outs, zookeeper **201** by execution, the **+3** unattributed gap) ⚠️ **except "2 method-value registrations" — it is 4 lines / 6 arguments**, which *strengthens* the case that grep cannot do this. ⚠️ **Defensible only if reframed as an EXECUTABLE enumerator** (dump `Registry` keys after a boot); a grep-based recount covers ~0.8 % of the surface and stays rejected |
| **`ADR-0299`'s STATUS still `PROPOSED`** | **1** | ⚠️ **CONFIRMED and it is the ONLY one — 13 `COMPLETE`, 1 `PROPOSED`**, while its §Decision and §Consequences demonstrably landed at the phase-77 IMPL squash. **The single word is the entire defect.** **Defensible as a RIDER** on any row whose close already edits `DECISIONS.md` — which this row's does — with a guard of shape *"a `PROPOSED` STATUS must have no §Decision."* **Not defensible standalone:** a 1-task row cannot carry a four-stage lifecycle |
| Row 78 GFM malformation (and rows **57/69** under the render predicate) | **1 to fix, 0 permitted** | **INVARIANT-BLOCKED** by §Schema `:18`, which permits in-place edits to `status` and `sub-phases` **only** (§7.2). **Defensible as a GUARD, not a fix:** land `awk '/^\| *[0-9]/ && !/\|$/'` and record the rows as permanent known exceptions |
| Symmetric bind hardening (`mustAllocatePort()`) | ⚠️ **8-12, NOT the banked 4-6** | **still rejected: it CANNOT be verified by a green suite run** — the prior instance needed `-count=6`. ⚠️ **AND THE BANKED NARROWING IS REFUTED IN THE UNSAFE DIRECTION:** *"only `fixture.HTTPSH2` can close-then-rebind race"* is wrong — **26 of 29** backend-startup arms call the close-then-rebind `freeTCPPort`, and only the in-process `TCPEcho`/`HTTPEcho` arm (which HOLDS its listener) is immune. The subject has a retry loop; the backends have none. **Do not adopt it on the "only HTTPSH2" framing.** Real surface: 26 arms + 16 helper definitions under **4** names in fixtures (**6** repo-wide) |
| Opening the **gRPC** family | 16-22+, ⚠️ **band NOT re-derivable** | **HARD-BLOCKED, now evidence-backed rather than asserted**: `\.RunEncodeTrailers(` ⇒ **0** non-test / 1 test, `\.RunDecodeTrailers(` ⇒ **0** non-test / 1 test; both declared (`chain.go:455`, `:622`) and neither reachable from production. **NC: `\.RunEncodeHeaders(` non-test ⇒ 4**, so the pattern shape does match live call sites. The 16-22+ band has no in-tree figure to re-measure — **flagged as not re-derived**; only its blocking premise is verified |
| The **WASM** row-summary rider | ~1 | **DECLINED ON THE MERITS, do not re-adopt as cheap** — `ROADMAP.md:76` declares phase 25 the FINAL §9 HTTP-filters-family row, and writing the marker would **silence check (3) BY MENTION** (proven in §2.2) |
| Normalizing the Operational-tooling short-form paragraph | ~1 | named, deliberately untaken; it would narrow check (2) by matcher rather than by work |

---

## 9. SCOPE, and what the SPEC must settle

**In scope:** (1) an `sds.` SN1-shaped hoisting arm in `ExtractTags`, reference-faithful per §4.2/§6; (2) `helpText` entries for the five names (phase 79 moved it 15 → 25); (3) the registration-time validation leg (§5), excluding the two deadlock-prone designs of §5.5; (4) the four `internal/statssink` consumers' goldens, which **will** move (§4.4); (5) a fixture-level `/stats/prometheus` parity assertion expecting **5** names, not 20 (§3.3); (6) `BEHAVIOR_CONTRACT.md` — close the departure and repair the three live `79.1` references (§1.1); (7) **ADR-0302** §Context.

**Explicitly OUT of scope:** the reference's other nine `sds.*` names (`control_plane.*`, `version`, `version_text`, `update_time`, `update_duration`, `key_rotation_failed`) — the existing five-counter subset is a defensible named subset per `reference_stats_sink_emits_used_only`, and the two TextReadouts are **dropped by the reference itself** on `/stats/prometheus`.

**Guards must DERIVE both sides from the code** — phase 79's headline lesson, paid for at the cost of a wrong number reaching five sites behind a green byte-stable guard. The shipped model is `internal/stats/segmentcount_test.go`, which walks `ExtractTags`' **AST**. ⚠️ **A hand-written golden shares the author's mistake**, and this row adds a **thirteenth** top-level detector — so `segmentcount_test.go`'s claimed count moves **12 → 13** and the terminal error string moves with it. **Both must move in the SAME task**, or this row reproduces phase 79's stale-on-arrival defect one generation on.

⚠️ **PREFER SYMBOL ANCHORS TO LINE CITES.** Every by-line cite written during phase 79 went stale *inside* phase 79.

**Cost band: 9-12 tasks, budget 11.** Derived in §10, and it agrees with an independent calibration re-derivation that recommends planning the banked `~7-9` **UP to 9-11**.

---

## 10. COST

*(Calibration and LoC re-derivation are recorded here; the SPEC re-derives independently.)*

Bottom-up: the arm + error-string/AST-guard pair (2) · helpText + its guard (1) · the validation leg + its boot-visibility guard (2) · four sink goldens (2, plausibly 3 — `otlp` alone carries the inert-cell hazard of broken-gate shape fifteen) · fixture parity assertion (1) · contract + ADR + row/`79.1` repair (2) · gates (1) ⇒ **11**, with a floor of 9 if the sink goldens land as one task and a ceiling of 12-13 if `otlp` splits.

### 10.1 The banked figure REPRODUCES — and it is not where the router said

⚠️ **The banked `~7-9` lives at `ROADMAP.md:141` (row 79's own cell), NOT at `:209`.** Located by byte offset: *"**DEFERRED TO A BANKED ROW 79.1 (~7-9):** the `sds.` LABEL-HOISTING arm (20 names; `envoy_xds_resource_name` measured ONCE, the SPEC must re-pin it against a live dockerized reference) and the registration-time-validation endgame"* at byte **4268** of a 10 719-char line. **`:209` carries the UNCARVED parent band `11-14`** at byte 46223, one of **six** bands on that line. ⚠️ **Byte offsets and character offsets differ by ~1 % on these multibyte lines — do not cross-compare them.**

⚠️ **`:141` misreports its own calibration twice, and `:209` carries an arithmetic defect.** `:141` claims *"three-for-three at or above the SPEC ceiling"* — it is **two-for-three** (phase 77's PLAN landed **12** against a **11-13** band, i.e. BELOW the ceiling) — and attributes row 79 a *"SPEC band 9-11, budget 11"* when the SPEC §9 heading explicitly **overrode that to 10-12, budget 12**. Separately `:209` says *"THIRTY live registered names across SIX families"* and then enumerates **four** families summing to **26** (it uses 2 each for `access_logs.`/`tracing.` where the contract at `:5078` uses 4 each). **The correct decomposition is the contract's: 20 + 4 + 4 + 2 = 30, of which phase 79 closed 10.** ⇒ **do not inherit "20 of 30" from `:209`.**

### 10.2 Calibration re-derived from primary documents

| phase | SPEC band | PLAN count | verdict |
|---|---|---|---|
| 76 | ~7-9 | **9** | **AT ceiling** |
| 77 | 11-13 | **12** | INSIDE |
| 78 | 7-9 | **10** | **ABOVE ceiling** |
| 79 | 10-12, budget 12 | **12** | **AT ceiling** |

**Three of four landed at or above the SPEC ceiling; none landed below the floor.** ⇒ the banked `~7-9` should be planned **UP to 9-11**, and **11 is inside that once the ceiling bias is applied.** This row's SPEC should expect to re-scope upward the way row 79's did (banked 11-14 → measured 7-9 → SPEC 10-12 → PLAN 12).

### 10.3 The LoC multiplier — a THIRD data point, and the gate is closer than it looks

PLAN commits carry **0** `.go` files (verified for all four), so the IMPL squash *is* the row's code:

| phase | estimate | realized `.go` added | net | ratio |
|---|---|---|---|---|
| 77 | ~700 | **1428** | 1406 | **2.04×** |
| 78 | ~330 | **495** | 471 | **1.50×** |
| **79** | **~875** | **1532** | **1446** | **1.75×** ⬅ new |

**Multiplier to carry: ×1.75 central, ×2.05 ceiling** (×1.70 / ×2.01 on the net basis §6.1 actually specifies). ⚠️ **Both 77 and 79 landed net just UNDER the 1500 gate (1406 and 1446).** ⇒ **a row estimated at ≥ ~750 LoC trips §6.1's LoC trigger once the multiplier is applied.** Phase 76 has **no** LoC estimate in either its SPEC or PLAN, so no 76 ratio exists — flagged rather than interpolated.

⚠️ **The NORMATIVE split gate is `BOOTSTRAP_PROMPT.md` §6.1, heading `:285`, body `:287-291`** — the canonical grep MISSES it because §6.1 spells the figures *"~25 **numbered** tasks"* / *"~1500 **lines of code**"* while `25 tasks`/`1500 LoC` occur only at `:225` and `:472` (both flow summaries). It carries **THREE** triggers, the third being **mid-execution** when any task's sub-steps exceed ~10 — which **FIRED at phase 79's T7**. The sink-golden task is this row's T7-analogue and the likeliest to trip it. ⚠️ **`ADR-0045` QUOTES the figures inside quotation marks, attributing them to `BOOTSTRAP_PROMPT.md` §5 — citing the numerals to ADR-0045 alone is a laundered cite.** `ADR-0106` is a family-shape ADR and defines nothing about phase-done gates; the six-gate is §7.5, heading `:357`, gates (a)-(f) `:360-365`, close `:367` — all three cites verified exact.

⚠️ **THE FULL 120-FIXTURE DIFFERENTIAL IS MANDATORY** — this row touches `internal/stats`, which links into `cmd/envoy-go` at `test/differential/harness.go:240`, `:594` and `test/conformance/h2spec/h2spec_test.go:210`; **h2spec is a FOURTH consumer NOT covered by `./test/differential/`**. Budget ~400-420 s per green attempt; `-race` is a second run. The `comm -3` cross-check is the load-bearing gate (120 with one fixture renamed and another skipped still reads 120), and the faithful dir predicate is `^[0-9]{4}[a-z]?-` — a bare `^[0-9]{4}-` gives **118**.

⚠️ **LIVE FLAKES, from the index and not from a brief's list:** the full-suite startup flake (`subject ready: EOF` **and** `bind: address already in use`, both failing **before any assertion** — it fired at the phase-79 IMPL on `0084-otlp-access-log`) · **`reference_sds_init_fetch_timeout_dial_budget_flake`, TWO packages, LIVE for this row's subject** · the pre-existing `internal/cluster` `-race` outlier. **Isolate-re-run, then state the classification AND its evidence.** ⚠️ **`0061-lb-ring-hash` is NOT a live flake — a spread failure there is a FINDING.** No flake fired at this stage: A2's and A3's boots were clean first-run, evidenced by `update_success=1` with `init_fetch_timeout=0`.

---

## 11. Hygiene

Four agents, disjoint remits, each in its own detached worktree with **private scratch** and a **private port band outside 20000-31007**; A1's docker containers named and torn down **BY NAME** with teardown proof empty. **Zero commits and zero pushes by any agent**; every experimental edit reverted **by explicit path**, with `cmd/envoy-go/main.go` and `internal/xds/stats.go` verified **sha256 byte-identical** across A3's six edit/revert cycles. The controller's one experimental file (`internal/stats/zz_ctl_p80_test.go`) was removed by explicit path and the tree verified clean.

⚠️ **THE BASH CWD RESET FIRED AGAIN — THE TWENTIETH CONSECUTIVE SESSION**, observed live **three times** at this stage (`Shell cwd was reset to /home/esa/git/envoy-go`). Every git command used `git -C <abs-worktree-path>`. ⚠️ **It also corrupted an agent's output once**: A1's arm-2 harness `READY` line was lost to a cwd-reset redirect and a stray zero-byte file landed in the main repo root — removed by explicit path. A1 flagged this rather than claiming it had read the line, and substituted independent liveness evidence.
