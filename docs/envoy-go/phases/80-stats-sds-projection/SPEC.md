# SPEC 80 — stats-sds-projection

**Stage:** SPEC. Lifecycle-state `1 -> 2`. **ROW 80 STAYS `in-progress`** — `ROADMAP.md` is BYTE-UNTOUCHED at this stage and the sentinel's `want` stays **112**.
**Base:** master `53855de0`. Worktree `/home/esa/git/envoy-go-wt/phase-80-spec`, branch `phase-80-spec`.
**Family:** TWENTY-THIRD §9 Observability-family row (registered at the BRAINSTORM; not re-registered here).
**Docs-only:** ZERO production `.go`, ZERO test `.go`.
**BYTE-UNTOUCHED at this stage:** `ROADMAP.md` · `BEHAVIOR_CONTRACT.md`.
**File set:** `SPEC.md` (NEW) + `DECISIONS.md` (ADR-0302 §Context) + `STATE.md` + `STATE_HISTORY.md` + `next-prompt.txt` — five files.

**Dispatch:** four investigation agents on disjoint remits (S1 live reference probe / S2 subject-side denominator + validation leg / S3 sink blast radius + AST guards / S4 cost + bookkeeping), each in its own detached worktree with private scratch and a private port band outside the differential harness's 20000-31007 range (S1 42000-42099 with docker, S2 42100-42199, S3 42200-42299, S4 none), plus controller re-derivation.

---

## 0. Sentinel — RE-RUN MECHANICALLY AT THIS TIP. IT DOES NOT FIRE; `stop` WAS NOT CREATED

`ls stop` ⇒ `No such file or directory`. It must not be created.

| check | ACTUAL output at this tip | negative control, observed firing |
|---|---|---|
| **(1)** `want=112` | **`NOT DONE: row 80`** — correct, row 80 is `in-progress` | `want=111` ⇒ `GATE FAIL: examined 112 data rows, expected 111`; row 76 doctored to `in-progress` on a scratch copy ⇒ `NC NOT DONE: row 76` **alongside** row 80 (the doctoring script self-reported `rows doctored: 1`) |
| **(2)** | **FIVE** — `:190 :200 :210 :216 :224` (long form ×4, Operational-tooling short form at `:224`) | union **5** vs long-form-only **4** — the one-arm form remains blind to the short form |
| **(3)** | `NEVER OPENED: gRPC`, `NEVER OPENED: WASM` | invented slug ⇒ `NC NEVER OPENED: ZZZ-nonexistent`; the REGISTERED slug `Observability` correctly printed nothing ⇒ the loop discriminates, it does not merely print |

Input measured at **228 lines / 13** bare `candidates:` hits (against the sentinel's narrower 5), so an empty result could not have read as a zero result.

⚠️ **CHECK (2) IS UNCHANGED AND THIS ROW NARROWS NOTHING — STATED, NOT FORECAST.** The twenty-fifth consecutive phase at which it did not go down. This row's subject is the residue of a closed row's departure and is drawn from no candidate paragraph.

⚠️ **The leak check is NOT live this session** — the SPEC leaves `ROADMAP.md` byte-untouched. It re-arms at the IMPL, which writes row 80's closing cell.

---

## 1. SCOPE — what row 80 does

1. An **`sds.` label-hoisting arm** in `internal/stats.ExtractTags`, reference-faithful (§3).
2. A **boot-boundary reject** for a secret name that cannot form a clean metric name (§4) — replacing today's silent registration skip. ⚠️ **This is an envoy-go-strict DEPARTURE: the reference accepts and serves such names** (§2.4).
3. **`helpText` + `helpTextRoster`** entries for the five projected bases (§5.2 — mandatory, not cosmetic).
4. **`internal/statssink` golden extension** — `goldenRoster` + `goldenTaggedIdx` + the four `goldenOTLPCells` byte pins (§5.1).
5. The **terminal error string + AST guard** move `12 -> 13`, in the same task as the arm (§6).
6. A **fixture-level `/stats/prometheus` name-and-label-shape assertion** on **5** names (§7).
7. **`BEHAVIOR_CONTRACT.md`** — close the narrowed departure, record the new one, repair the live `79.1` forward references (§8).
8. **ADR-0302** — §Context drafted here; §Decision + §Consequences append IN PLACE at the IMPL.

**Explicitly OUT of scope:** the reference's other `sds.*` names (`control_plane.*`, `version`, `version_text`, `update_time`, `update_duration`, `key_rotation_failed`). ⚠️ **CONFIRMED BY EXECUTION rather than inherited:** the reference's `sds.*` family is **10 flat names / 9 prom families** — `version_text` is a TextReadout present on flat and **dropped by the reference itself** on `/stats/prometheus`. The five-counter subset stands as a defensible named subset per `reference_stats_sink_emits_used_only`.

**Also out of scope, and chartered in §10:** the general dynamic-token charset exposure; full hyphen fidelity; the reference's `# HELP`-free exposition format.

---

## 2. WHAT THIS SPEC REFUTES OR REFINES IN THE BRAINSTORM

*A SPEC's job is to refute its predecessor by execution, not to formalize it. Nine findings.*

### 2.1 ⚠️ THE §6 FORK IS NOT A FORK — but the BRAINSTORM's RULE IS INCOMPLETE, and the missing half changes output

**Half one — the fork dissolves.** `flattenToProm` is a thin projection over `ExtractTags` applying `strings.ReplaceAll(residual, ".", "_")` at projection time, so the existing SN1 `cluster.` arm already produces the reference's exact shape. Driven through the **live** `flattenToProm`:

```
cluster.alpha.beta.gamma.update_success  -> envoy_cluster_beta_gamma_update_success  [{envoy_cluster_name alpha}]
cluster.foo.bar.update_success           -> envoy_cluster_bar_update_success         [{envoy_cluster_name foo}]
cluster.a.b.c.d.update_success           -> envoy_cluster_b_c_d_update_success       [{envoy_cluster_name a}]
NC: sds.server_cert.update_success       -> terminal rejection (12 top-level / 4 mid-name)
```

Substitute `sds`/`envoy_xds_resource_name` and row 1 is **byte-identical to the reference**. ⇒ option (a) needs **no new code shape**; option (b) (last-dot) needs a `strings.LastIndex` appearing **nowhere** in `ExtractTags` — every dynamic-token arm (SN1, SN2, SN3, `mongo.`) uses first-dot. **(b) diverges from the reference AND from the file's own four-arm convention.** There is no fidelity-versus-correctness tension. **Do not restore the "wrong-looking but conformant" framing** — it invites a future row to "fix" it.

Independently corroborated by the pinned image's own tag-extractor literal, recovered from `/usr/local/bin/envoy`:

```
^sds\.((<TAG_VALUE>)\.).+     <TAG_VALUE> = [^\.]+     tag name = envoy.xds_resource_name
```

Confirmed on **three discriminating arms** (`foo.bar` ⇒ `envoy_sds_bar_US{…="foo"}`; `a.b.c.d` ⇒ `envoy_sds_b_c_d_US{…="a"}`; `a..b` ⇒ `envoy_sds__b_US{…="a"}`), and `[^.]+`-vs-`(.*?)` separated by the `.lead` arm alone (`sds..lead.US` ⇒ `envoy_sds__lead_US{}` — **empty label set, name polluted**). NC on the verifier: last-dot fails **7/16** arms, `(.*?)` fails **exactly 1** (`lead`).

### ⚠️ Half two — THE LOAD-BEARING CORRECTION: a NAME-FORMATION pre-step the BRAINSTORM never modelled

The reference does **not** form `"sds." + secret + "." + suffix`. It forms:

```
prefix := "sds." + secret + "."
prefix  = replace(prefix, "://" -> "_", ":/" -> "_", ":" -> "_")   // in that order
prefix  = TrimRight(prefix, ".")                                   // ALL trailing dots
statName := prefix + "." + suffix
```

Pinned by three mutually-constraining arms: `trail.` ⇒ `sds.trail.update_success` (**not** `sds.trail..…`); `.lead` ⇒ `sds..lead.update_success` (leading/interior empties **preserved**); `a..b` ⇒ `sds.a..b.update_success`. The colon mapping is pinned by `a:b` ⇒ `sds.a_b.…` and by the SPIFFE arm (`://` collapses to a **single** `_`, so it is a 3-char replacement, not per-char). NC: dropping the trailing-dot strip fails **16/16** arms; dropping the colon mapping fails **2**.

⚠️ **envoy-go forms the name naively** — `fmt.Sprintf("sds.%s.%s", secretName, suffix)` in `RegisterSDSStats`. So for a trailing-dot secret it builds `sds.trailing_dot..update_success`, which **`NamePattern` ACCEPTS** (an interior empty segment passes — `reference_dynamic_stat_name_charset_guard` records exactly this blind spot), registers, and would project to `envoy_sds__update_success` against the reference's `envoy_sds_update_success{…="trail"}`. **A byte-perfect tag rule still produces the wrong metric name.** §4 resolves this at the guard rather than by mirroring the sanitizer (which ADR-0065 forecloses — see §2.2).

### 2.2 ⚠️ THE §5 VALIDATION LEG IS NOT AN SDS DEFECT — it is ADR-0065 non-compliance, and `sds.` is the ONLY offender

The BRAINSTORM presents the silent skip as an `sds.`-local wart with three invented options. **The tree already has a five-times-replicated answer and a landed ADR that mandates it.**

| family | site (symbol anchor) | behavior on an invalid name |
|---|---|---|
| `cluster.` | `internal/cluster/manager.go` `NewManager` guard | `cluster: %q: invalid cluster name (must contain only ASCII letters, digits, underscore, or dot, …)` |
| `http.` (HCM `stat_prefix`) | `internal/filter/hcm/config.go` `parseFilterWithCtx` | `hcm: invalid stat_prefix: %q (…)` |
| network `rbac` | `internal/filter/network/rbac/rbac.go` | `parseRejectStatPrefixInvalid` |
| `kafka_broker` | `internal/filter/network/kafkabroker/config.go` | `errStatPrefixInvalid` — *"config-boundary charset guard (AMEND-K7; the rbac/mongo precedent)"* |
| `thrift_proxy`, `redis_proxy` | `.../config.go` | same shape |
| `listener.` | `internal/listener/manager.go` `normalizeAddr` | PRE-SANITIZES, so the assembled name is always valid |
| **`sds.`** | `internal/xds/stats.go` `RegisterSDSStats` | ⚠️ **SILENTLY SKIPS all five counters** |

**`ADR-0065 §Consequences (e)` is normative and `sds.` violates it, verbatim:**

> *"Future filter / extension authors that introduce metrics derived from user input MUST validate at their input boundary using `stats.IsValidName` (or an equivalent boundary check) — the panic discipline is correct for static names but not for user-input-derived names, and silently relying on `Registry.NewCounter`'s panic to surface invalid input is a design defect."*

`RegisterSDSStats` does neither thing that ADR contemplates: it neither validates at the input boundary nor relies on the panic. It guards-and-skips — a third behavior producing neither an error nor a crash.

⚠️ **AND ADR-0065 FORECLOSES SANITIZATION FOR EXACTLY THIS ROW'S REASON.** Its §Context rejected candidate (A) — sanitizing the dynamic token — because *"Sanitising would silently mutate that label vs upstream Envoy's emission. Two stat_prefixes differing only in invalid chars would collapse to the same Prometheus label value — a silent data-loss bug."* **That applies verbatim the moment this row makes the secret name a hoisted label value.** S2 independently reproduced the collision hazard: `a-b` and `a_b` would alias in `getOrRegister`. ⇒ BRAINSTORM §6 option (c) and every sanitization design — **including mirroring the reference's §2.1 pre-step** — are foreclosed by a landed ADR, not by taste.

### 2.3 ⚠️ THE CHARSET EXPOSURE IS GENERAL — and my own first probe overstated it

```
sds.server-cert.update_success                IsValidName=false
cluster.foo-bar.upstream_rq_total             IsValidName=false
http.ingress-http.downstream_rq_total         IsValidName=false
mongo.my-mongo.cmd.find.total                 IsValidName=false
rbac-allow.rbac.allowed                       IsValidName=false
wasm.my-plugin.executions                     IsValidName=false
listener.0.0.0.0_8080.downstream_cx_total     IsValidName=true    <- normalizeAddr pre-sanitizes
CONTROL cluster.foo_bar.upstream_rq_total     IsValidName=true
```

⚠️ **I first read this as "six families silently skip." That was wrong, and the §2.2 control caught it.** `IsValidName=false` says the *name* is invalid; it says nothing about what the *family does*. Five of those families REJECT; only `sds.` skips. **A probe's input is a claim, and so is its framing.**

What survives, and it is load-bearing: the hyphen is legal in a Prometheus label *value* — only metric NAMES and label KEYS are constrained:

```
cluster.foo-bar.upstream_rq_total     -> envoy_cluster_upstream_rq_total{envoy_cluster_name="foo-bar"}
http.ingress-http.downstream_rq_total -> envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="ingress-http"}
```

⇒ **the registration guard is stricter than the projection requires.**

### 2.4 ⚠️ THE HYPHEN SKIP IS A GENUINE CONFORMANCE DIVERGENCE — the reference ACCEPTS, HOISTS, and SERVES IT VERBATIM

The BRAINSTORM calls the hyphen *"idiomatic"* but never asked what the reference does. **Executed against `envoyproxy/envoy:contrib-v1.37.2`:**

```
flat:  sds.server-cert.update_success: 1
prom:  envoy_sds_update_success{envoy_xds_resource_name="server-cert"} 1
```

`od`-verified: the hyphen (`0x2d`) is preserved **verbatim in the label value** — not sanitized, not dropped, not rejected at config parse. The metric name is the clean `envoy_sds_update_success`, because the whole dot-free secret is hoisted out of it.

⇒ **envoy-go has ZERO counters where the reference has FIVE.** This upgrades the validation leg from *"add a diagnostic"* to *"close a behavioural gap"* — **and it means a design that merely LOGS the skip leaves envoy-go divergent, because the reference does not skip.**

⚠️ **IT ALSO MEANS THE CHOSEN REJECT IS ITSELF A DEPARTURE, AND THE SPEC SAYS SO PLAINLY.** A hyphenated secret boots green on the reference and will boot-FAIL on envoy-go. That is a *deliberate, documented, envoy-go-strict* choice — the same posture `ParseSDSConfig`'s own doc comment already declares for its whole reject roster:

> *"Every reject is ADR-0080-distinct and `"xds: sds:"`-prefixed — **the reference SUPPORTS all these forms, so they are documented envoy-go-strict DEPARTURES.**"*

**Full hyphen fidelity is chartered in §10, not taken here** — it requires either relaxing `NamePattern` (a `checkName`-panics invariant with repo-wide blast radius) or carrying the raw operator bytes alongside the sanitized registry key through the `Registry`/`ExtractTags` seam. Both are out of proportion to this row.

⚠️ **`ADR-0080-distinct` is an established idiom, NOT a stale cite.** ADR-0080's heading is about `default_filter_chain`, but the phrase is used at **8** sites as shorthand for the anti-silent-divergence convention (`DECISIONS.md` glosses it *"ADR-0080 anti-silent-divergence"*). **Do not "fix" it.** Checked precisely because it looked like drift.

⚠️ **The reference's only name reject is the EMPTY name, and it is PGV** — `SdsSecretConfigValidationError.Name: value length must be at least 1 characters`. `reference_pgv_forecloses_go_hazard` records that **envoy-go runs no PGV**, so this cannot be inherited for free; `ParseSDSConfig` already rejects `name == ""` independently.

### 2.5 ⚠️ "5 PER PROCESS" — CONFIRMED, and the mechanism is STRONGER than stated

The BRAINSTORM derived this from *reading* the `seen > 1` guard. **Executed, three two-secret arms, with no SDS server running (so the reject is proven purely config-level):**

| arm | shape | INNER_EXIT | stdout | stderr |
|---|---|---|---|---|
| A2 | ONE listener, ONE `common_tls_context`, cert-SDS **+** plain `validation_context_sds_secret_config` | **1** | 0 B | 115 B — `sds provider: xds: sds: multiple SDS-bound downstream TLS contexts unsupported (MVP takes one)` |
| A3 | **TWO separate listeners**, one cert-SDS secret each | **1** | 0 B | 115 B, byte-identical |
| A5 | cert-SDS **+ CVC's** `validation_context_sds_secret_config` | **1** | 0 B | 115 B, byte-identical |

**Discriminating NC** — a single valid secret, also with no SDS server — fails at a **different stage with a different message** (`listener manager: … dial tcp …: connection refused`, 332 B), so the guard message is specific to `seen > 1` and not generic SDS failure.

⇒ a two-secret config is a **hard boot reject: exit 1, zero stdout, no listener bound**. `RegisterSDSStats` is called **exactly once or the process does not exist**. **A2 also answers a question the BRAINSTORM did not ask: `seen` is counted across ALL listeners**, not per-listener. **Any fixture assertion expects 5.** A two-secret fixture is **not constructible** on the subject — it would be a boot-reject fixture, not a stats fixture.

⚠️ **NEW: `--mode validate` reaches NONE of the SDS provider path.** `validate.Bootstrap` passes a nil provider, so `boot.NewSDSProvider` is never called and every SDS bootstrap yields the tls apply-point message instead. **This forecloses `--mode validate` as a home for the new check.**

### 2.6 ⚠️ THE ONLY EXISTING DIAGNOSTIC IS INVERTED — measured in BOTH directions

Two boots differing by one `sed` substitution (`server_cert` → `server-cert`), same SDS server, same committed PKI:

| | valid name | hyphenated name |
|---|---|---|
| READY sentinel | yes | **yes** |
| stderr at ready | **0 B** | **0 B** |
| `/stats` | 200, 35 lines, **5** `sds.` | 200, 30 lines, **0** `sds.` |
| `/stats/prometheus` | 200, 86 lines, **0** `envoy_sds` | 200, 86 lines, **0** `envoy_sds` — **SAME sha256** |
| shutdown stderr | the phase-79 skip line naming all five | **nothing** (`grep -i 'sds\|secret\|skip\|invalid\|name'` ⇒ NONE) |

Two consequences, both load-bearing:

1. **The skip log fires on the HEALTHY arm and is SILENT on the BROKEN one.** Landing the projection arm alone removes it from the healthy case, after which the silent skip has **no signal in either direction**. ⇒ **the validation leg belongs in this row — confirmed by measurement, not by argument.**
2. ⚠️ **On `/stats/prometheus` today, a fully working registration and a silently-skipped one are BYTE-IDENTICAL** (same sha256, 4768 B). Non-vacuity proven: the same file yields **12** `sds`-containing lines, all `envoy_cluster_*{envoy_cluster_name="sds_cluster"}` — the matcher works, the family is absent. ⇒ **the §7 fixture assertion is a real gate, not a tautology.**

With the reject in place, an `sds.*` name can only be registered-and-projected or absent-because-the-boot-failed-loudly ⇒ **any nonzero `sds.` skip line becomes a bug**, where today zero is ambiguous between "fixed" and "never registered". §6 gates on that.

### 2.7 `incNil` is COMPLETE — an AUDIT, denominator stated

95 printable ASCII bytes 0x20-0x7e, one secret per byte, pointer-level plus a registry-walk delta:

```
denominator=95: allFive=64  allNil=31  partial=0
rejected (31): [space ! " # $ % & ' ( ) * + , - / : ; < = > ? @ [ \ ] ^ ` { | } ~]
```

**`partial == 0` across the whole sweep**, and five increments on the all-nil struct panic nothing with registry size 0. The property is **structural**: validity depends solely on the operator-supplied segment, since `sds.` and all five suffixes are fixed and charset-clean. **CONFIRMED**, and the 31-byte figure independently reproduces the BRAINSTORM's.

⚠️ **The probe caught its own author:** its first draft hand-asserted `1leading_digit` and `trailing_dot.` as invalid; both are **valid** because `sds.` supplies the leading char and the fixed suffix supplies the trailing one. Deriving the expectation from `IsValidName` rather than by hand is what caught it — `reference_probe_input_is_a_claim`.

⚠️ **Minor stale figure, corrected:** `RegisterSDSStats` has **1** non-test call site (`internal/boot/boot.go:201`) and **25** `_test.go` occurrences, not the 15 the BRAINSTORM states. Immaterial to the design.

### 2.8 ⚠️ THE FIXTURE CANNOT DISCRIMINATE THE §6 FORK — all four corpus secrets are DOT-FREE

`server_cert`, `validation_ca`, `rccf_validation_ca`, `edf_validation_ca` contain **no dots**, so first-dot (a) and last-dot (b) produce **identical** output for every fixture in the corpus. **A fixture arm is a vacuous gate for the fork.** The fork needs a dedicated unit test on a dotted name, or it goes unguarded. (Corpus re-derived independently from `test/fixtures/*/envoy*.yaml`; `0024-http-oauth2`'s `client_secret`/`hmac` are `path_config_source` and **EXCLUDED** — including them gives 6×5=30 and would coincidentally "confirm" the banked 30 for the wrong reason. The `0110`→`rccf_validation_ca` / `0111`→`edf_validation_ca` attribution is re-confirmed.)

### 2.9 ⚠️ THE `79.1` ROSTER WAS EXACT AT ITS BRANCH POINT AND THE BRAINSTORM'S OWN COMMIT FALSIFIED IT

BRAINSTORM §1.1 states *"36 occurrences across 8 files"*. Measured at both revisions, occurrences not lines:

| | @ `80cafb7e` (parent) | @ `53855de0` (HEAD) |
|---|---|---|
| `BEHAVIOR_CONTRACT.md` | 4 | 4 |
| `ROADMAP.md` | 3 | **5** (row 80's new cell) |
| `STATE.md` | 2 | **4** |
| phase-79 `{BRAINSTORM,SPEC,PLAN,PROGRESS}.md` | 25 | 25 |
| phase-80 `BRAINSTORM.md` | — | **8** (NEW) |
| `next-prompt.txt` | 2 | **4** |
| **TOTAL** | **36 / 8 files** | **50 / 9 files** |

> **The figure was EXACTLY RIGHT when written and is stale by the commit that wrote it.** `reference_branchpoint_roster_stale_midrow` firing on the stage that cites it. **The IMPL carries 50 / 9, re-derived at ITS tip, not this one.** Zero false positives — all 50 inspected with context (a full audit, not a sample); no version number, no decimal, no `79.10`.

⚠️ **AND A WHOLE CLASS IS MISSING FROM §1.1's TRICHOTOMY.** It classifies live-normative / invariant-blocked / historical, which leaves **11 occurrences in this row's OWN live documents** unclassified — `ROADMAP.md` row 80's cell ×2, `STATE.md` ×4, phase-80 `BRAINSTORM.md` ×8 minus overlap. Those are **editable by this row's own stages** and must not be mistaken for either of the frozen classes.

⚠️ **A CITE-DRIFT SET, re-derived:** row 80's insertion shifted `ROADMAP.md` below `:142` by **+1**, so the BRAINSTORM's `:209` (the uncarved `11-14` parent band) is now **`:210`**, and `ADR-0300 §Consequences (iii)`'s `ROADMAP.md:208` is now **`:210`** too. `:141` and `:140` are above the insertion and still resolve. ⚠️ **`BEHAVIOR_CONTRACT.md`'s pre-79 offset is `+54` ONLY up to old line ~5023 — a second hunk adds six more, so every pre-79 cite at or beyond old `:5024` is stale by `+60`.** A flat `+54` lands six lines short in the tail. **Prefer symbol anchors to any offset.**

⚠️ **ONE INHERITED PREMISE IS WEAKER THAN THE BRAINSTORM CLAIMS, and the conclusion still holds.** §1.1 claim 3 argues *"when this project DOES split, the legs live in the parent's `sub-phases` PROSE, never as separate rows."* **`ROADMAP.md` §Schema `:18` says the opposite in writing** — *"Only `status` and `sub-phases` columns are updated in place. **Sub-phases get their own rows.**"*, and `:20` repeats *"each child gets its own row"*. So claim 3 describes **observed practice diverging from written doctrine**, not doctrine. **The row-id adjudication survives on claims 1 and 2, which are independent and both hold** — §6.2's split machinery was never invoked and no sub-phase row has been created since 32.2. **Record the tension; do not relitigate the id.**

---

## 3. DESIGN DECISION D-SDS-1 — the projection arm

**Shape: SN1, first-dot, single-segment, non-greedy.** An `sds.` case in `ExtractTags`' top-level switch, structurally cloned from the `cluster.` arm:

- `tail := strings.TrimPrefix(internal, "sds.")`; `dot := strings.Index(tail, ".")`; `dot < 0` ⇒ the arm's own `has no <rest> segment` reject (mirroring SN1/SN2/SN3).
- label `{Key: "envoy_xds_resource_name", Value: tail[:dot]}`; `residual = "sds." + tail[dot+1:]`.
- **No dot→underscore pre-transform in the arm** — `flattenToProm` does it at projection time (the phase-79 lesson, written into the SN5 comment).
- Falls through to the SN4 `_Nxx` collapse like SN1. The five suffixes carry no `_Nxx`, so SN4 is inert; left in the common path rather than short-circuited, matching `cluster.`.

Projected bases (**derived by executing `WriteProm` under an experimental arm, not hand-typed**):
`envoy_sds_update_success` · `envoy_sds_update_failure` · `envoy_sds_update_rejected` · `envoy_sds_update_attempt` · `envoy_sds_init_fetch_timeout`, each carrying `envoy_xds_resource_name="<secret>"`.

**Placement:** before the `wasm.` arm. Behaviorally irrelevant (the prefixes are disjoint), but the AST guard reports the detector SET in source order, so the IMPL must expect `sds.` at its chosen index in the guard's `in code:` list.

**Reference-faithful label rendering, pinned by execution and NOT to be re-invented:** label values escape `\`→`\\`, `"`→`\"`, LF→`\n`; **hyphens and raw UTF-8 are emitted verbatim, unescaped**. Metric-name sanitization replaces every character outside `[A-Za-z0-9_]` with exactly one `_` **per Unicode code point, not per byte** (`x.çd` ⇒ `envoy_sds__d_update_success` — two underscores, not three), with no run-collapsing. ⚠️ Most of this is **already** how envoy-go behaves via `flattenToProm` + the Prometheus writer; the IMPL's job is to *verify* it rather than add it, and to record any gap it finds.

---

## 4. DESIGN DECISION D-SDS-2 — the validation leg

**Shape: a boot-boundary reject in `boot.NewSDSProvider`, immediately after `ParseSDSConfig` returns and before `RegisterSDSStats`.**

```
xds: sds: invalid secret name: %q (must contain only ASCII letters, digits, underscore, or dot, and form a valid metric-name segment)
```

### ⚠️ 4.1 THE SITE MOVED — `ParseSDSConfig` is WRONG, and this SPEC's own first draft had it wrong

An earlier draft of this SPEC placed the reject in `xds.ParseSDSConfig`, reasoning from ADR-0065's *"validate where the input enters process state"* and from arms 8/9 being same-shape neighbours. **Executed, that is wrong.** `ParseSDSConfig` has **FOUR** non-test call sites:

```
internal/boot/boot.go:192
internal/tls/config.go:118   (plain VC-SDS)
internal/tls/config.go:178   (CVC-SDS)
internal/tls/config.go:383   (cert-SDS)
```

The three `internal/tls` sites wrap into a **different** operator-facing chain — `return nil, fmt.Errorf("tls: downstream: %w", err)` — so one condition would surface under two distinct substrings, against the ADR-0080 distinctness discipline. **And more decisively: those three paths never register a stat at all** (`RegisterSDSStats` has exactly ONE non-test call site, `boot.go:201`), so a reject there would fail a config for a stat-name reason in contexts where no stat name is ever formed.

⇒ **`boot.NewSDSProvider` is the correct site** — colocated with arm 7 (the `node.id`/`node.cluster` reject), which is the established home for boot-level SDS rejects, and reached by exactly the path that registers the counters. **No new package edge** (`internal/xds` already depends on `internal/stats`; verified `go list -deps ./internal/xds` ⇒ hit, NC `./internal/conv` ⇒ 0 — the first NC I ran was vacuous and was re-run).

### 4.2 ⚠️ GUARD THE SEGMENT, NOT ONLY THE ASSEMBLED NAME

`IsValidName` is a **charset** guard, not a **well-formedness** guard: an interior empty segment passes. Measured — `trailing_dot.` yields `sds.trailing_dot..update_success`, which **registers cleanly** and would project to `envoy_sds__update_success` against the reference's `envoy_sds_update_success{…="trail"}` (§2.1 half two). `reference_dynamic_stat_name_charset_guard` states the rule this row must follow verbatim: ***"Guard on the segments, not only on the assembled name, when empties are reachable."***

⇒ the reject asserts **both**: `stats.IsValidName(assembled)` **and** that the secret name contains no empty segment (no leading dot, no trailing dot, no `..`). ADR-0065 §Consequences (b)'s *"validate the longest, they pass/fail together"* is **tested rather than inherited** and holds for the sds suffix set:

```
server_cert / validation_ca / rccf_validation_ca / edf_validation_ca : all five suffixes agree = true
server-cert                                                          : all five suffixes agree = false
my.dotted.cert                                                       : all five suffixes agree = true
```

⇒ validating **one** assembled name suffices; the IMPL validates the longest (`init_fetch_timeout`) for consistency with the precedent, **plus** the segment check.

**The new reject is a no-op on the differential corpus** — all four corpus secrets pass both legs, so the 120-fixture suite cannot go red for this reason.

### 4.3 What stays, and what the design sidesteps

**The `RegisterSDSStats` guard and `incNil` STAY.** Two-layer defense is the established pattern (`internal/tracing/stats.go` documents its own guard as *"defense-in-depth; the hcm config.go guard already validates"*). `incNil`'s deletion is a proven boot-path SIGSEGV. The IMPL adds a test asserting the skip branch is **unreachable from production config**, which is what converts it from live logic to defense-in-depth.

⚠️ **Both deadlock-prone designs stay excluded** (`reference_registry_walk_lock_inversion`): no counter registered from inside the projection path, no lazy registration at first increment. **The chosen design sidesteps both by construction** — a boot-boundary reject registers nothing and walks nothing. It also **moots the ADR-0300 §Consequences (iii) panic-visibility question**, because the site returns an error into `main.go`'s existing `log.Fatalf`, never panicking. The BRAINSTORM's five-arm insertion-point probe stands as a **correction to ADR-0300**, recorded as such in §8, and is **not relied upon** by this design.

⚠️ **A counter for the skip (option (iv)) is rejected on a NEW ground, not only the deadlock one.** Registered unconditionally it adds a name to all four SDS fixtures' rosters and every sink golden; registered conditionally it is a name that appears only sometimes — exactly the unstable roster the byte-stability guards exist to catch. **And it would itself be `sds.`-rooted, so it would need the new hoisting arm to project — a circularity.**

---

## 5. BLAST RADIUS — and the finding that the existing gates are BLIND

### 5.1 ⚠️ ALL FOUR OTLP GOLDENS ARE INERT, NOT JUST `(F,F)` — the BRAINSTORM understates this

Re-measured at this tip under a real experimental `sds.` arm. Field names verified against the `OTLPMetricsSink` struct — `reportCountersAsDeltas`, `useTagExtractedName`, `emitTagsAsAttributes`, `prefix`; only the middle two consult `ExtractTags`, so the sensitive cross-product is genuinely 2×2:

| cell | BEFORE | AFTER | Δ |
|---|---|---|---|
| **(F,F)** default | 186 B, `envoygo.sds.server_cert.update_success`, no attrs | **186 B, identical** | **0 — CANNOT FAIL** |
| (F,T) | 186 B | 228 B, `+envoy.xds_resource_name=server_cert` | **+42** |
| (T,F) | 186 B | 174 B, `envoygo.sds.update_success` | **−12 — SHRINKS** |
| (T,T) | 186 B | 216 B, both | **+30** |

⚠️ **BUT THE LANDED GOLDEN IS BLIND IN ALL FOUR CELLS, WHICH IS THE ACTUAL HAZARD.** Re-running the four landed `goldenOTLPCells` byte pins with the arm applied:

```
F_F_default_INERT_UNDER_HOIST  got=1134 pinned=1134
F_T_attrs                      got=1200 pinned=1200
T_F_residual_name              got=1118 pinned=1118
T_T_both                       got=1184 pinned=1184
```

**All four unmoved.** `goldenRoster` is **13 entries with ZERO `sds.*` names** (independently verified: `cluster.` ×2, `listener_manager.` ×1, `runtime.` ×2, `access_logs.` ×4, `tracing.` ×4), and `goldenTaggedIdx = {0: true, 1: true}` — the two `cluster.` entries. ⇒ **running the full cross-product is NECESSARY BUT NOT SUFFICIENT.** The roster must be extended first or every cell is vacuous, the inert one included.

**Mandatory IMPL action:** extend `goldenRoster` with at least one `sds.*` entry, extend `goldenTaggedIdx` to mark it tag-bearing, and **re-measure all four `wantBytes` rather than computing them.**

### 5.2 ⚠️ THE HELP TEXT SHIPS SELF-EQUAL AND THE EXISTING GUARD DOES NOT CATCH IT

Under the experimental arm, `WriteProm` emits 839 B / 5 families — with degraded HELP:

```
# HELP envoy_sds_update_success envoy_sds_update_success
# TYPE envoy_sds_update_success counter
envoy_sds_update_success{envoy_xds_resource_name="server_cert"} 7
```

`helpText` has no `envoy_sds_*` key, so `prom.go`'s `if help == "" { help = g.name }` degradation fires. **`TestHelpText_NoSelfEqualHelp` does NOT catch it** — it drives only `helpTextRoster`, which has no sds entries. ⇒ extending `helpText` (**25** keys today) AND `helpTextRoster` (**25** entries today) in lockstep is **mandatory, not cosmetic**. "Phase 79 moved helpText 15 → 25" is CONFIRMED by execution across `895f0be2^`/`895f0be2`/HEAD.

⚠️ Roster entries must use the **two-segment** shape `sds.<secret>.<suffix>`; a single-segment `sds.x` hits the arm's `dot < 0` reject.

⚠️ **THIS IS INTERNAL-CONSISTENCY WORK, NOT CONFORMANCE WORK.** Measured: **the reference emits ZERO `# HELP` lines** (`# TYPE` ×330, `# HELP` ×0 in every dump) while envoy-go emits both. That is a **pre-existing format departure independent of this row** — named here so nobody prices the helpText task as parity work, and chartered in §10.

### 5.3 The other three sinks — AFTER bytes, measured

```
dogstatsd   BEFORE "envoygo.sds.server_cert.update_success:7|c"
            AFTER  "envoygo.sds.update_success:7|c|#envoy.xds_resource_name:server_cert"
graphite    BEFORE "envoygo.sds.server_cert.update_success:7|c"
            AFTER  "envoygo.sds.update_success;envoy.xds_resource_name=server_cert:7|c"
labelMapper BEFORE name="sds.server_cert.update_success" labels=[]
            AFTER  name="sds.update_success" labels=[envoy.xds_resource_name=server_cert]
```

The BRAINSTORM's live UDP capture of the BEFORE bytes reproduces exactly, so its baseline is confirmed against this tree. ⚠️ **DogStatsd tag order stays UNSORTED** — `formatTagSuffix` has no `sort.Slice` (unlike `labelMapper.apply`, which does). With one hoisted label there is no order to observe; **do not sort** (`reference_dogstatsd_tag_order_unsorted`).

⚠️ The sink attribute key is **`envoy.xds_resource_name`** (dotted) while the Prometheus label key is **`envoy_xds_resource_name`** (underscored). Do not conflate them in an assertion.

**"Only `WriteProm` drops" — CONFIRMED.** Consumer roster re-derived by `git grep`: 29 syntactic hits / 6 files; **5 non-test call sites**, each walked one layer up to a `cmd/envoy-go/main.go` construction site and confirmed production-reachable, none a declared-but-dead seam. `label.go` degrades on `err != nil || len(labels) == 0`; `dogstatsd.go`/`graphite.go` fall back to `residual, labels = fam.GetName(), nil`; `otlp.go` degrades on two independent `err == nil` guards. Executed proof: the five real `sds.*` names produce **`WRITEPROM outbytes=0`** (a measured zero — input was 5 registered counters) while all four sinks emit all five in full dotted form.

### 5.4 ⚠️ THE WHOLE-TREE BLAST RADIUS IS **ONE** FAILING TEST — a warning, not a comfort

Clean baseline `go test ./internal/stats/... ./internal/statssink/... -count=1` ⇒ **INNER_EXIT=0**. With the experimental arm ⇒ **INNER_EXIT=1, exactly ONE test fails**: `TestTerminalError_TopLevelCountMatchesCode`. `internal/statssink` stays **fully green**, including all four phase-79 wire goldens; so do `TestExtractTagsTerminalError_ByteStable`, `TestHelpText_*`, `TestWriteProm_SkipLogStackedControl`, `internal/stats/dynamic` and the fuzzers.

⇒ **the tree's existing gates are almost entirely blind to this row. The phase-80 gate must be BUILT, not inherited.** The only thing that comes for free is the `12 -> 13` count.

---

## 6. GUARDS — both sides derived from the code

**`internal/stats/segmentcount_test.go` is the shipped model and it is genuinely two-sided:** `topLevelDetectorsFromSource` AST-walks `ExtractTags` for `strings.HasPrefix(internal, LIT)` / `strings.CutPrefix(internal, LIT)`; the claim side is *parsed out of the live `noRecognizedSegmentErrFmt` constant* by `claimedRe`. **Neither side is hand-written.** Verified firing in BOTH directions:

- **13th detector, error string untouched** ⇒ `terminal message claims 12 top-level segments; ExtractTags has 13.` + `roots ExtractTags accepts but the message does not name: [sds.]`
- **error string bumped to 13, no detector** ⇒ THREE tests fail: `TestExtractTagsTerminalError_ByteStable` (both legs), `TestTerminalError_TopLevelCountMatchesCode` (`claims 13 … has 12`; `roots the message names but ExtractTags does not accept: [sds.]`), and `TestTerminalError_NamedRootsAreAccepted`.

⚠️ Not the vacuous union-strip shape (`reference_gate_command_negative_control`): the count leg moved 12↔13 **and** the set leg named the exact missing/extra member, independently.

### 6.1 ⚠️ THE MUST-MOVE-TOGETHER SET — and the SWEEP NEEDLE IS ITSELF A FINDING

⚠️ **TWO AGENTS DISAGREED ON THE SIZE OF THIS SWEEP AND I ADJUDICATED IT BY EXECUTION. NEITHER NEEDLE IS THE SWEEP.**

- The **numeral** needle `one of the 12` returns **4 hits / 4 files** — but **2 are FALSE POSITIVES** (`phases/09-http-filter-fault/{REVIEW,PROGRESS}.md`, which are about *"the 12 existing fuzzers"* and have nothing to do with `ExtractTags`). Only `internal/stats/name.go` and `internal/stats/name_test.go` are real. **An agent using only this needle concluded "the count lives in exactly TWO files, so no sweep task is owed."**
- The **spelled** needle `\btwelve\b` returns **45 files** — overwhelmingly false positives (twelve filters, twelve fuzzers, twelve anything).
- **Scoping the spelled needle to the detector context** (`twelve` co-occurring with `segment|detector|ExtractTags|top-level|prefix`) returns **26 lines / 20 files**, of which the **LIVE-EDITABLE set is 16 lines across 8 files**.

⇒ **the numeral form and the spelled form must BOTH be swept, and the spelled one must be scoped or it drowns in noise.** This is `reference_stale_cite_recurs_fix_by_pattern` at the needle level: **fix by PATTERN, and validate the pattern before trusting its count.**

**Executing / build-affecting (4) — ONE task with the arm:**
1. `internal/stats/name.go` — the `sds.` arm.
2. `internal/stats/name.go` — const **`noRecognizedSegmentErrFmt`**: `12`→`13`, `sds.` in the pipe list. **The mid-name `4` is UNCHANGED** — `sds.` is a ROOT.
3. `internal/stats/name.go` — the doc block above that const (`12 TOP-LEVEL segment detectors`, `the top-level twelve`) and `ExtractTags`' SN doc comment.
4. `internal/stats/name_test.go` — const **`wantNoRecognizedSegmentErrFmt`**, the hand-written twin, plus its surrounding prose.

⚠️ **THE BRAINSTORM NAMES THE WRONG FILE FOR THE COUNT.** §9 says *"`segmentcount_test.go`'s claimed count moves 12 → 13"*. **`segmentcount_test.go` hard-codes NO count at all** — it AST-derives one side and parses the other out of the live constant. The `12` lives in **`name.go` (production)** and in the **hand-written byte-stable golden `name_test.go`**. The atomicity instruction is right; the file it names is wrong.

**Executing but content-silent unless extended (3) — the vacuity risks of §5.1/§5.2:**
5. `internal/stats/helptext_test.go` — `helpTextRoster`.
6. `internal/stats/name.go` — the `helpText` map.
7. `internal/statssink/golden_bytemirror_test.go` — `goldenRoster`, `goldenTaggedIdx`, all four `goldenOTLPCells.wantBytes`.

**Non-executing prose that goes stale — LIVE-EDITABLE (8 files / 16 lines), derived by the scoped needle:**
8. `internal/stats/promskip_test.go` — the `TWELVE top-level prefix detectors` comment. ⚠️ Its sentinel `promSkipUnprojectableName = "filesystem.flushed_by_timer"` **stays valid** (not sds-rooted), but the file's own editor note requires re-checking it against `flattenToProm`, not against prose.
9. `internal/stats/segmentcount_test.go` — the narrative header.
10. `internal/admin/stats.go` — `not one of the twelve top-level segments ExtractTags recognizes`.
11. ⚠️ **`test/fixtures/0118-runtime-static-layer/` — SIX live hits**: `driver/driver.go` ×2, `expectations.yaml` ×2, `README.md` ×2. **An agent reported this fixture "de-fanged" by phase 79 on the strength of two lines that refuse to quote the constant; the scoped needle shows six that do not refuse.** Phase 79's own headline was that this enumeration lived in FOUR files where the SPEC said one — **do not re-inherit an undercount of the same surface.**
12. `docs/envoy-go/BEHAVIOR_CONTRACT.md` ×2.
13. `docs/envoy-go/STATE.md` ×1.
14. ⚠️ **`next-prompt.txt`** — carries `**TWELVE** non-test top-level segment detectors`. **A recursive `command grep` in this harness does NOT see this file** (gitignored-but-tracked); found only by `git grep`. ⚠️ **`reference_recursive_grep_blind_to_gitignored_tracked_file` firing live inside the very row that documented it.** **The IMPL's sweep MUST use `git grep`.**

**MUST NOT TOUCH (historical / invariant-blocked):** `docs/envoy-go/phases/{04,06.1,56,79}/…`, `STATE_HISTORY.md`, `DECISIONS.md` ×3 (immutable ADR text), and **`ROADMAP.md` ×3** (`:118`, `:134`, `:141` — all in `summary` cells, invariant-blocked by §Schema).

⚠️ **This is the phase-79 defect shape one generation on.** Prefer symbol anchors — every by-line cite written during phase 79 went stale inside phase 79.

### 6.2 The gates this row must ADD

- **G1** — the `12 -> 13` AST guard move (free).
- **G2** — `helpText` set-exactness over the five new bases via the REAL `flattenToProm` (never hand-typed), plus the no-self-equal leg extended to cover them.
- **G3** — `goldenRoster`/`goldenTaggedIdx` extension + all four re-measured OTLP `wantBytes`, run as the **full cross-product** (`reference_probe_must_discriminate`).
- **G4** — the reject: a positive arm (hyphenated secret ⇒ the exact `xds: sds: invalid secret name` string), a **segment arm** (`trailing_dot.`, `.lead`, `a..b`), and a **negative arm** (all four corpus secrets ⇒ accept), so the guard cannot pass by rejecting everything.
- **G5** — the §2.6 invariant, **stacked**: after a clean SDS boot the skip line names **zero** `sds.` entries **AND** the five names are present on `/stats/prometheus`, so "zero skips" cannot be satisfied by "nothing registered".
- **G6** — ⚠️ **a UNIT test on a DOTTED secret name**, because §2.8 proves the fixture cannot discriminate the first-dot/last-dot fork. Without G6 the row's central design decision ships unguarded.

---

## 7. DIFFERENTIAL AND FIXTURE POSTURE

⚠️ **NO FIXTURE PINS `sds.` ABSENCE FROM `/stats/prometheus`.** Unlike phase 77's `0118-runtime-static-layer` — which pinned the exposition asymmetry and went RED on purpose when phase 79 landed — all five SDS fixtures explicitly record the `sds.<secret>.*` counters as **NOT asserted**. ⇒ (i) **no red-on-purpose conversion to do** (a cost reduction against the phase-79 shape); (ii) **no in-tree guard would catch this row failing to land**, so the new assertion is the only end-to-end proof the row worked.

### ⚠️ 7.1 THE HOME IS `0110`, NOT `0103` — this SPEC's own first draft had it wrong

| fixture | admin | secret | `fixture.StatsAsserter`? | `/stats/prometheus` scraper? |
|---|---|---|---|---|
| `0103-xds-sds-server-cert` | yes | `server_cert` | **no** | **no** |
| `0108`, `0109` | yes | `validation_ca` | **no** | **no** |
| **`0110-tls-require-client-cert-false`** | yes | `rccf_validation_ca` | **YES** | **YES** (`scrapeProm`) |
| **`0111-tls-cvc-empty-dynamic-fallback`** | yes | `edf_validation_ca` | **YES** | **YES** |

An earlier draft named `0103` as "the canonical single-secret SDS fixture". It is canonical, and it has **neither** an `AssertStats` nor a prom scraper — so it would cost strictly more than `0110`, which already carries an SDS secret with a valid name, `AssertStats`, and a live cross-side-green `/stats/prometheus` scraper. **Extending `0110` costs ZERO registration gates** (`reference_differential_fixture_three_registration_gates`).

⚠️ **A concrete blocker inside the existing helper, so this is NOT free reuse:** `0110`'s `scrapeProm` keys its map by **metric name only and discards the label set**. It therefore **cannot assert `envoy_xds_resource_name="rccf_validation_ca"`**. Asserting the hoisted label needs a label-aware scraper or a raw-body substring assertion. **Budget it.**

### 7.2 ⚠️ THE ASSERTION IS A NAME+LABEL-SHAPE SUBSET — never set-equality, never value-parity

**BRAINSTORM §9 item 5 calls it a "parity assertion expecting 5 names". That phrasing would produce a RED fixture on arrival** if a planner reads "parity" as set- or value-equality. Three independent reasons:

1. **Set-equality is impossible.** The reference's roster is fuller (`control_plane.*`, `version`, `version_text`, `update_time`, `update_duration`, `key_rotation_failed`) — measured at **10 flat / 9 prom** against envoy-go's 5. Those nine are explicitly out of scope.
2. **Value-parity is structurally impossible.** Subject measures `update_attempt: 1` (two independent shapes); the live reference dump shows **2**. Mechanism is contract-documented: envoy-go is **INITIAL-FETCH ONLY** and does not keep the stream open, while the reference maintains a long-lived subscription. ⇒ **assert `update_attempt`'s NAME and LABEL, never its VALUE**; `update_success` carries the same hazard. Only `init_fetch_timeout` is safely `0 == 0` cross-side — and a `0 == 0` pin is vacuous, so it needs a stacked control or should be left unasserted.
3. **Cross-side is nonetheless VALID for the name+label shape**, with a live in-tree precedent: `0111` already asserts three `listener.<addr>.ssl.*` names cross-side off `/stats/prometheus` *"where the address is a LABEL and the metric NAME is therefore cross-side identical."* **That is exactly the `sds.` shape post-hoist** ⇒ `reference_listener_stat_scope_cross_side_divergence` is **neutralized by the hoist itself**, not a blocker. And `reference_ssl_stats_suppressed_by_fast_failing_upstream` is not live here — the `sds.*` family is accounted at boot, strictly before any downstream connection.

**VERDICT:** a cross-side assertion of exactly **5** names each carrying `envoy_xds_resource_name="<secret>"`, with values asserted **per-side only** (`>= 1`), not cross-side.

⚠️ **THE DENOMINATOR IS 5, NOT 20** (§2.5). A two-secret fixture is not constructible.

⚠️ **THE FULL 120-FIXTURE DIFFERENTIAL IS MANDATORY** for any row touching `internal/stats` — it links into `cmd/envoy-go`, and **h2spec is a FOURTH consumer NOT covered by `./test/differential/`**. Budget ~400-420 s per green attempt; `-race` is a second run. **The SPEC is docs-only and owes none of it**; the PLAN specifies it and the IMPL runs it, with `comm -3` as the load-bearing gate and `^[0-9]{4}[a-z]?-` as the faithful dir predicate (a bare `^[0-9]{4}-` gives 118).

---

## 8. CONTRACT AND ADR EDITS (owed at the IMPL, specified here)

1. **Close the narrowed departure** at the *"THE DEPARTURE IS NARROWED, NOT ELIMINATED"* paragraph — 20 names go from dropped to projected. The four-family decomposition **20 + 4 + 4 + 2 = 30** is the contract's own and is correct. ⚠️ **Do NOT inherit "20 of 30" from `ROADMAP.md`'s row-79 cell**, which says *"THIRTY names across SIX families"* then enumerates **four** summing to **26**.
2. **Repair the live `79.1` forward references** — **4 occurrences on 3 lines, all in `BEHAVIOR_CONTRACT.md`** (the live-normative class of §2.9's 50-occurrence roster) — anchored on the PHRASE, not the line: *"Until 79.1 lands, treat `sds.*` as flat-`/stats`-only"*, the *"DEFERS the `sds.` twenty to row 79.1"* clause, and the *"79.1 takes it to 0 lines"* clause. ⚠️ `ROADMAP.md` row 79's three occurrences are **INVARIANT-BLOCKED** by §Schema (only `status` and `sub-phases` may be edited in place) and are NAMED as a permanent discrepancy; the phase-79 phase documents are **historical record and must NOT be rewritten**.
3. **Rewrite the five-counter subset paragraph**, which says the secret-name segment *"is guarded by `stats.IsValidName` before registration"* — true today, false after this row.
4. **Record the NEW DEPARTURE** (§2.4/§4): envoy-go **boot-fails** on a secret name that cannot form a clean metric name, where **the reference boots green and serves the counters with the name hoisted verbatim into the label value**. State it as an envoy-go-strict departure with the reference behavior quoted, not as a fix.
5. **Update the `TWELVE` count** to THIRTEEN wherever the contract states it.
6. **Correct ADR-0300 §Consequences (iii)** — the over-attribution. Recorded as a named correction; the chosen design does not depend on it.
7. **ADR-0302** — §Context drafted at this SPEC **WITH the retained italic footer**, exact byte form verified by `od -c` against ADR-0300 (no leading/trailing whitespace, no `**`, U+00A7 section signs, newline-terminated), placed as the **last line of §Context**, one blank line after the final `**§Context ¶N …**` paragraph:

   ```
   *(§Decision + §Consequences land at the phase-80 IMPL.)*
   ```

   ⚠️ ADR-0301 omitted it and forced the phase-79 IMPL into a recorded departure. **The footer-carrying block is `ADR-0294 … ADR-0300` — SEVEN blocks.** ⚠️ **ADR-0301's own STATUS line says *"seven blocks"* but then names the range *"ADR-0295 through ADR-0300"*, which is SIX.** Enumerated mechanically: 0294 (phase-72), 0295 (73), 0296 (74), 0297 (75), 0298 (76), 0299 (77), 0300 (78). **Copy the form, not the miscount.** No renumber, no `---` separator. ⚠️ Carry **no whole-file grep count** — that species self-falsified in ADR-0296 ¶3 and twice in ADR-0297. Next-free confirmed mechanically: `^## ADR-0302` ⇒ 0, NC `^## ADR-0301` ⇒ 1; **300 headings, ids 0001-0301, exactly one gap at ADR-0209, zero duplicates** — *"300 ADRs"* and *"tail ADR-0301"* are both true and must not be conflated.

---

## 9. COST

| # | task | notes |
|---|---|---|
| T1 | the `sds.` arm + `noRecognizedSegmentErrFmt` `12->13` + both doc blocks + `wantNoRecognizedSegmentErrFmt` | **ONE task — they cannot split** (§6.1 items 1-4) |
| T2 | the stale-`TWELVE` prose sweep, **by `git grep`** | §6.1 items 8-15 |
| T3 | `helpText` + `helpTextRoster` + the no-self-equal extension | §5.2 |
| T4 | `goldenRoster` + `goldenTaggedIdx` + four re-measured OTLP `wantBytes`, full cross-product | §5.1 — **the T7-analogue, likeliest to split** |
| T5 | the three non-OTLP sink goldens | §5.3 |
| T6 | the boot-boundary reject in `NewSDSProvider`, charset **and** segment legs | §4 |
| T7 | reject gates G4 (positive + segment + negative arms) | §6.2 |
| T8 | the `RegisterSDSStats` skip-unreachable-from-production test | §4.3 |
| T9 | **G6** — the dotted-name unit test for the first-dot fork | §2.8 — without it the central decision ships unguarded |
| T10 | fixture `0110` extension: label-aware scrape + 5-name/label-shape assertion | §7.1/§7.2 — **includes the scraper rework, not free reuse** |
| T11 | G5 stacked skip-line invariant | §6.2 |
| T12 | contract + ADR-0302 §Decision/§Consequences + `79.1` repair + ADR-0300 correction | §8 |
| T13 | ⚠️ **the BREAK ROSTER**, each arm proven to fire its OWN assertion | §9.2 — **the BRAINSTORM omits this entirely** |
| T14 | full differential + `-race` + six-gate | §7 |

⇒ **14 tasks, floor 12 (T5 folds into T4; T8 folds into T7), ceiling 15 if T4 splits.**

**Band: 12-15, budget 13.** ⚠️ Calibration says **three of four recent rows landed AT or ABOVE the SPEC ceiling and none below the floor**, so a budget at 13 in a 12-15 band should be read as *"expect 14"*, not *"expect 13"*.

### 9.1 Why this is ABOVE the BRAINSTORM's 9-12/11

Six findings move it up, all from execution: the golden apparatus is **blind in all four cells** rather than one (T4 grows, likelier to split); the HELP text ships **degraded and unguarded** (T3 mandatory, not cosmetic); the whole-tree blast radius is **one test**, so the gate is built rather than inherited (T7/T8/T11); the fixture **cannot discriminate the central design fork**, forcing T9; the fixture home moved to `0110` where the existing scraper **discards labels** (T10 includes a rework); and **the break roster is missing from the BRAINSTORM's bottom-up altogether** (T13). The reject design is *cheaper* than the BRAINSTORM's open three-option fork — a clone of a five-times-replicated precedent — but that does not offset the gate work.

### 9.2 ⚠️ THE BREAK ROSTER IS A SYSTEMATIC OMISSION, NOT A ROUNDING ERROR

BRAINSTORM §10's bottom-up has **no break-roster cell**. **Every one of the four calibration phases shipped one as its own numbered task** — 76 `## Task 8 — the break roster`, 77 `## Task 8 — the break roster`, 78 `## Task 6 — the break roster: **eight arms**`, 79 `## Task 10 — the break roster`. **4/4.** This alone moves 11 → 12 before any of §9.1's findings apply.

⚠️ And this row's breaks are unusually hazardous, because §5.4 proves the tree is nearly blind to the arm: a break arm that "fires" may be firing an unrelated assertion. `reference_deliberate_break_wrong_assertion`, `reference_break_arm_injection_site_is_a_claim` and `reference_vacuous_break_modes` are all live. **Each arm must name WHICH assertion fired.**

### 9.3 ⚠️ THE LoC TRIGGER IS LIVE — the BRAINSTORM states the rule and never applies it to its own row

Calibration re-derived from primary documents, each band quoted from its SPEC's own heading rather than ticked: **76: ~7-9 → 9 (AT ceiling)** · **77: 11-13 → 12 (INSIDE)** · **78: 7-9 → 10 (ABOVE)** · **79: 10-12 budget 12 → 12 (AT)**. PLAN counts mechanical (`^## Task [0-9]`): 9 / 12 / 10 / 12. **All four cells CONFIRMED.** PLAN squashes carry **0** `.go` files (verified 4/4), so the IMPL squash *is* the row's code. Realized ratios reproduce to the byte: **77: 2.040× · 78: 1.500× · 79: 1.751×** ⇒ ×1.75 central, ×2.04 ceiling (the banked "×2.05" is a rounding **up**, not a measurement). Phase 76 has no LoC estimate in either document (0 hits against non-empty inputs), so no 76 ratio exists — flagged, not interpolated.

**Estimate for THIS row:** production ≈ 55 · test ≈ 690 · fixture ≈ 110 ⇒ **~855 LoC**.

> ⚠️ **855 × 1.75 ≈ 1496 and 855 × 2.04 ≈ 1744.** The row sits **ON the ~1500 line at the central multiplier and OVER it at the ceiling.** ⇒ **THE SPEC STATES A SPLIT POSTURE AND DOES NOT WRITE "≪ ~1500".** The phase-79 SPEC wrote "≪ ~1500" and its own PLAN refuted it; this SPEC declines to repeat that. **If the PLAN's bottom-up lands ≥ ~800 LoC, split at the T4/T10 seam** (goldens+fixture as leg b), which is the natural cut.

⚠️ **THE NORMATIVE §6.1 CITE IS `:287-292`, NOT `:287-291`.** Verified exactly: heading `### 6.1 When to split` at `:285`; *"~25 **numbered** tasks"* at `:289`; *"~1500 **lines of code**"* at `:290`; **`:291` is BLANK and the THIRD trigger lives at `:292`** — *"splitting is triggered *mid-execution* if any single task's sub-steps blow up past ~10 items"*. **Both the BRAINSTORM and the phase-79 PLAN cite short of the very sentence they rely on** (the PLAN cites `:285-290` while calling that trigger "LIVE"). **Prefer the heading anchor to the range.** The canonical `25 tasks`/`1500 LoC` grep genuinely misses §6.1 (those strings occur only at `:225` and `:472`, both flow summaries), and the six-gate cites (`§7.5` heading `:357`, gates (a)-(f) `:360-365`, close `:367`) all verify EXACT. `ADR-0045` quotes rather than states; `ADR-0106` defines nothing about phase-done gates.

---

## 10. WHAT THIS ROW NAMES BUT DOES NOT FIX

- ⚠️ **Full hyphen fidelity.** The reference serves `envoy_sds_update_success{envoy_xds_resource_name="server-cert"}`; envoy-go will boot-fail. Closing this needs either relaxing `NamePattern` (a `checkName`-panics invariant, repo-wide) or carrying raw operator bytes alongside the sanitized registry key through the `Registry`/`ExtractTags` seam. **Chartered.**
- ⚠️ **The general dynamic-token charset exposure (§2.3).** Six families assemble stat names from operator input. This row brings `sds.` into line; it does **not** audit whether the five existing rejects are complete, nor whether other dynamic segments (wire-derived mongo collection names, wasm plugin names) are reachable with an invalid name. `reference_dynamic_stat_name_charset_guard` records the wire-derived half.
- ⚠️ **The `# HELP` format departure (§5.2).** The reference emits **zero** `# HELP` lines; envoy-go emits them for every family. Pre-existing, independent of this row, **newly measured here.**
- ⚠️ **`--mode validate` cannot validate ANY SDS bootstrap** (§2.5). Independent of this row; newly measured.
- ⚠️ **The `listener.`/`stat_prefix` sanitization inconsistency.** ADR-0065 §Context rejected sanitizing a hoisted label value as *"a silent data-loss bug"* — yet `normalizeAddr` sanitizes the `listener.` address, which SN3 hoists into `envoy_listener_address`. **The precedent set is internally inconsistent.** Named so the IMPL is not ambushed.
- The `ADR-0299` `PROPOSED`→`COMPLETE` one-word rider (defensible on this row's IMPL, which already edits `DECISIONS.md`).
- The `STATE_HISTORY.md` archive gap and the `ROADMAP.md` malformed rows 57/69/78 — invariant-blocked or bookkeeping-only.

---

## 11. HAZARDS CARRIED INTO THE PLAN

- **A recursive `grep` in this harness is blind to `next-prompt.txt`** — it fired live in this row (§6.1 item 15). **`git grep` for every sweep; print NAMES, never a count.**
- **A hand-written golden shares the author's mistake** — `wantNoRecognizedSegmentErrFmt` is exactly that twin, which is why it is inside T1.
- **A gate cell that cannot fail** — and, worse here, a whole ROSTER that cannot fail (§5.1).
- **`reference_sds_init_fetch_timeout_dial_budget_flake` is LIVE for this row's subject, in TWO packages.** It did **not** fire at this stage — evidenced by `update_success=1` with `init_fetch_timeout=0` on two independent boot shapes, both reaching READY in 2 ticks. Isolate-re-run and state the classification AND its evidence.
- The full-suite startup flake (`subject ready: EOF` / `bind: address already in use`, failing before any assertion) and the pre-existing `internal/cluster` `-race` outlier. ⚠️ **`0061-lb-ring-hash` is NOT a live flake — a spread failure there is a FINDING.**
- **The Bash cwd reset** — it fired again at this stage (§12).

---

## 12. HYGIENE

Four agents on disjoint remits, each in its own detached worktree with private scratch and a private port band outside 20000-31007; S1's 19 docker containers named `p80s1-<arm>` and torn down **BY NAME**, teardown proof empty. **Zero commits and zero pushes by any agent**; every experimental edit reverted **by explicit path** and verified sha256 byte-identical.

⚠️ **THE BASH CWD RESET FIRED AGAIN — THE TWENTY-FIRST CONSECUTIVE SESSION**, observed live on multiple controller probes (`Shell cwd was reset to /home/esa/git/envoy-go`). Every git command used `git -C <abs-worktree-path>`.

### ⚠️ 12.1 FOUR PROBES WERE THEMSELVES THE UNRELIABLE THING, AND CONTROLS — NOT REVIEW — CAUGHT ALL FOUR

1. **Controller:** the first `go list -deps` NC did not discriminate (the package chosen as a negative also carries the edge); re-run against `./internal/conv` ⇒ 0.
2. **Controller:** the charset sweep was first framed as "six families silently skip" when five of them reject (§2.3). The measurement was sound; the sentence attached to it was not.
3. **S1:** the label-escaping negative control **initially failed to apply and printed `FAILURES=0`** — a silently-unapplied NC reads exactly like "the rule is fine." An `assert old in s` caught it. `reference_gate_command_negative_control` firing inside the gate built to test for it.
4. **S2:** the `incNil` roster's first draft hand-asserted `1leading_digit` and `trailing_dot.` as invalid; both are valid. Deriving the expectation from `IsValidName` rather than by hand caught it.

⚠️ **And one NAMED SUBSTITUTION, flagged rather than hidden:** S1 drove the reference via `path_config_source` (filesystem SDS), not `api_config_source` (gRPC SDS). Justification: `SdsApi` creates the scope `sds.<name>.` identically for both, and tag extraction is a pure string transform on the finished name; corroborated by the `server_cert` arm reproducing the BRAINSTORM's gRPC-path result **byte-identically**. **gRPC SDS was not independently probed against the reference.**

### 12.2 Arms not run, flagged rather than silently dropped

- gRPC-sourced SDS against the reference (substituted, above).
- A secret name that is **all dots** (`...`) against the reference.
- Ordering of `:`→`_` versus the trailing-dot strip at a name that is both colon-bearing and dot-trailing.
- The `0110`/`0111` differential fixtures end-to-end (require the reference container; the extension-point analysis is from source plus the contract's statement that `0111`'s cross-side prom assertion is green today).
- The full 120-fixture differential — **the SPEC owes none of it**; the IMPL runs it.
