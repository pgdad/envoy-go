# Phase 97 — `quic-chain-selection-order` — BRAINSTORM

**Stage:** BRAINSTORM (lifecycle-state **DONE -> 1**). **Subject SELF-PICKED** per the 2026-07-12 standing directive, smallest defensible candidate first, with no banked mid-lifecycle work to advance.

**Tip:** `5fe3c460` (`git rev-parse master` at session start). Fresh worktree `phase-97-brainstorm` off that tip per `feedback_git_worktrees`. Docs-only, ZERO production `.go`.

**One sentence.** On a QUIC/HTTP-3 listener carrying both a `default_filter_chain` and an eligible `filter_chains[]` entry, envoy-go serves the request from the **default chain** and leaves the eligible indexed chain with zero requests — which is precisely the alternative ADR-0080 **§Alternatives (A)** enumerates and **REJECTS**, and which ADR-0080 §Decision 1 and §Decision 2 forbid in both directions.

---

## 0. What this stage refuted

Every stage's job is to refute its predecessor by execution. This one refuted **twelve** claims, four of them load-bearing on the pick itself, and **two of them made by its own measurement agents**.

### 0.1 The router's front-runner is materially LARGER than its adjective, and the adjective was never the problem

`next-prompt.txt` names the HCM `stat_prefix` duplicate-registration panic as *"the strongest banked candidate"* and instructs the reader to re-derive its cost rather than re-read the adjective. Re-derived, the cost floor is not one file:

- **Ten** files register counters whose names interpolate the HCM `stat_prefix` through the **panic-on-duplicate** `NewCounter`/`NewGauge`, not through the `*IfAbsent` seam: `internal/filter/hcm/config.go`, and the `compressor`, `lua`, `jwtauthn`, `fault`, `csrf`, `adaptive_concurrency`, `admission_control` and `tap` HTTP filters, plus `internal/tracing/stats.go`. A repair confined to `hcm/config.go` closes the first trigger reached and leaves nine standing.
- `internal/filter/http/jwtauthn/jwtauthn.go` **already predicted this in a comment** — *"the Registry's `NewCounter` would panic on duplicate name; Task 8 may switch to `NewCounterIfAbsent` if the empirical-scrape closure reveals this is operator-reachable."* It is operator-reachable; that comment has been waiting for a row.
- The repair **must mint a new config-reachable boot panic**. `cmd/envoy-go/main_test.go`'s `TestMain_BootPanicIsVisible` says so in its own header: *"this test's trigger is the ONLY config-reachable in-window panic in the tree, and there is NO fallback … If a future row makes duplicate registration a get-or-create (reference parity) or a clean config reject, this test goes RED — that is intended. RE-POINT THE TRIGGER; DO NOT RELAX THE ASSERTION."* Measured: a five-line prototype in `hcm/config.go` alone turns that test RED after ~31s.
- The row needs **two** fixture directories, not one — a subject-only boot-reject arm for the current behaviour and an ordinary cross-side parity arm for the repaired behaviour — because one fixture directory dispatches to exactly one runner branch. Measured floor per fixture directory, from the two most recent creating commits: **1359** added lines for `0120` and **1393** for `0121`.

That is a plausible §6.1 split trigger before the PLAN is even written. **The candidate is not rejected as unimportant; it is rejected as not-smallest.** Its measurements are banked in §4.1 so the next pick is better informed than this one was.

### 0.2 The "crash versus hang" contradiction is REAL, and BOTH parties to it were cited wrongly

The brief this session handed its first measurement agent asserted a contradiction between `next-prompt.txt` (panic, rc=2) and `docs/envoy-go/ROADMAP.md` **line 228** (a swallowed-panic boot hang). The agent reported the contradiction **manufactured** — that line 228 is about the phase-44/45/46 access-log and tracing rows and *"does not mention this defect at all"*, and that the real text is at line 140.

**Both halves of that refutation are themselves wrong, and the variable is the matcher.** Line 140 does carry row 78's cell, and row 78 records the fixed state correctly. But line 228 **also** carries the stale claim, at byte offset **42927** of a 47234-byte line, inside phase 74's notes:

> a swallowed-panic BOOT HANG (duplicate `stat_prefix` ⇒ `registry.go:107` panics from `hcm/config.go:358`, SWALLOWED by the deferred `<-flusherDone` at `cmd/envoy-go/main.go:299`, so the process neither crashes nor boots)

The agent searched for the panic **message text**; the line spells the **config field**. A matcher blind to the wording returned a clean absence. The controller's own sweep, keyed on `stat_prefix` / `registry.go` / `hcm/config.go`, found it immediately.

⚠️ **That stale carrier sits INSIDE a sentinel window (window 228).** It is recorded here and deliberately **NOT** edited: a BRAINSTORM's measured file scope on `ROADMAP.md` is `+1 / -0`, and no stage should touch a sentinel window for a cosmetic reason. See §9.4.

### 0.3 D10-QUICSEL "needs its own ADR" is REFUTED for the half this row repairs

The banked note (carried in `next-prompt.txt` and in `phases/96-…/BRAINSTORM.md` §0.4) says the accessor split *"needs its own ADR (multi-chain QUIC dispatch vs ADR-0080's fallback semantics)"* and calls it a design question.

**ADR-0080 already answers it, empirically, against the pinned reference.** Read at this tip (`DECISIONS.md:3078`):

- **§Decision 1 — No-match fallback only.** *"`default_filter_chain` is consulted ONLY when no `filter_chains[]` entry's `filter_chain_match` is eligible … If at least one `filter_chains[]` entry is eligible, `default_filter_chain` is NOT consulted."*
- **§Decision 2 — Empty-match chain in `filter_chains[]` BEATS `default_filter_chain`.**
- **§Alternatives (A) — *"`default_filter_chain` ALWAYS preferred (bypass `filter_chains[]` if `default_filter_chain` is set)"* — REJECTED**, on an §11.1 empirical pin against Envoy v1.37.2.

`quicChain()` implements **exactly the rejected alternative (A)**, unconditionally. The **ordering** half needs no new decision; it needs the existing one applied. The **multi-chain SNI-dispatch** half is genuinely open and stays deferred — and is not required to repair the ordering.

⚠️ **§Decision 3 is the WRONG section for this question.** It authorises independent TLS *posture* only. Citing it here is the phase-96 §0.13 mistake ("read the decision, not the example") one section over.

### 0.4 `internal/listener` is FULLY GREEN under the prototype fix — the ordering is UNTESTED

`go test ./internal/listener/... -count=1` returns rc=0 with all three packages `ok` **both before and after** the prototype that inverts QUIC chain selection. No existing test asserts the current behaviour, and none would have caught the repair. That is the coverage finding, not a reassurance: **the guard is absent, in both directions.**

### 0.5 An unrelated boot reject MASKS the front-runner's panic

The controller's first reproduction of the `stat_prefix` panic on the boot path returned **rc=1** with `extract admin: bootstrap: missing admin` — the config had no `admin` block, and envoy-go rejects that **before** reaching filter construction. The panic arm and a plain config error are one exit code apart and read alike in a log tail. **A reproduction that never reached the site under test is not a negative result.** With an `admin` block added, the same config gives rc=2 and the panic, in 0 seconds.

### 0.6 The banked port-race figure is PREDICATE-DEPENDENT, and neither carrier states the predicate

The banked `37 files (31 panic / 5 error / 1 other)` reconciles only under a predicate that admits a **loopback** re-bind. Under the narrow predicate the banked note's own command encodes — probe `127.0.0.1:0`, close, re-bind on a **wildcard** `0.0.0.0:%d` — the answer is **36 = 31 / 5 / 0**. The 37th file is `test/fixtures/0024-http-oauth2/inputs/driver.go`, which re-binds on loopback with a bare `return err`; it is the entire content of the "1 other". **31 and 5 are right. 37 and 1 are right only under a predicate nobody wrote down.**

### 0.7 The banked D8-FCM framing is refuted by the PINNED PROTO's own doc comment

The banked note calls a `filter_chain_match` inside `default_filter_chain` *"PGV-rejected by the reference and silently ignored here"*. The pinned `go-control-plane` listener proto's doc comment on `DefaultFilterChain` reads **"The filter chain match is ignored in this field."** Ignoring it is **proto-mandated**, exactly as phase 96's own SPEC said and contrary to how the banked note frames it. Running the generated validators shows they descend into both slots and reject `source_ports: [0]` in **both** — so most of the residual is the already-banked fact that envoy-go runs no such validation at all, not a default-chain divergence. Exactly **one** shape is default-chain-specific: a CIDR `prefix_len` out of range, which envoy-go's own parser happens to cover inside `filter_chains[]` and does not cover in the default slot.

### 0.8 The banked `manager_test.go:5571` citation has ALREADY drifted, and so has phase 96's correction of it

Phase 96 banked a stale citation at `manager_test.go:5571`. At this tip that line is **`:5832`** — a **+261** shift caused by phase 96's own `+556 / -24` to that file. Phase 96 §0.9's *correction* to a sibling anchor (`:4597`) has likewise rotted to `:4848`. **A banked line number is stale by the time it is banked.** This is `reference_line_shift_after_insert_is_banded` firing against two consecutive authors.

### 0.9 `0118`'s falsified port band is falsified TWICE over

`test/fixtures/0118-runtime-static-layer/driver/driver.go:31` warns *"NOT 10450: that is the TLS/SDS band (0108-0113)"*. Measured per-fixture reference ports: `0103 -> 10443`, `0108 -> 10444` … `0113 -> 10449`. So (i) the band is **10443-10449** once `0103` is counted and **10444-10449** for the stated range, and (ii) **`10450` is held by nothing at all** — its only occurrence anywhere under `test/` is that warning comment.

### 0.10 `ROADMAP.md:136` is WRONG ONCE and RIGHT ONCE, on the same line

The banked one-line maintenance item says `ROADMAP.md:136` mis-cites `registry.go:107` where `BEHAVIOR_CONTRACT.md:1042` says `:117`. Ground truth at this tip: `(*Registry).register` panics on duplicate at **:107**; `(*Registry).checkName` panics on charset at **:117**. Line 136 carries **two** `registry.go:107` tokens — the charset one at column 6095 is wrong, the duplicate-panic one at column 10953 is **right**. A whole-line substitution corrupts a correct citation. A **third** wrong live citation nobody banked sits at `DECISIONS.md:2356`, which cites `registry.go:100` for `checkName` — `:100` is a comment line.

### 0.11 The `IsValidName` guard is CHARSET-ONLY, and there is no duplicate guard anywhere

`internal/stats.IsValidName` is used at ~22 call sites to validate user-derived metric names at the input boundary. It answers *"is this name spellable"* and nothing else. `internal/filter/hcm/config.go` validating `stat_prefix` for charset does **not** imply it checks for collision, and no site in the tree checks for collision. **Two separate concerns that share a variable are not one guard.**

### 0.12 The banked `0120/expectations.yaml` complaint is about SCOPE, not ORIGIN

The banked item calls the file's first line — `# Phase 94 fixture 0120-tls-connection-error expectations` — a false attribution. The fixture **was** created by the phase-94 IMPL. What is stale is the file's *scope*: phase 95 rewrote it `+163 / -17`, adding two arms and changing *"four of this fixture's five arms"* to *"six of this fixture's seven arms"*, without touching the header. A correction that fixes the wrong word is not a correction.

---

## 1. The pick, and why it is defensible as "smallest first"

### 1.1 Charter, in one sentence

Make the QUIC/HTTP-3 serving path select its filter chain by the algorithm ADR-0080 and ADR-0081 already mandate — an eligible `filter_chains[]` entry first, `default_filter_chain` only on no-match — so that the chain supplying the **filters** and the chain supplying the **TLS identity** are the same chain, and so that an empty-match indexed chain is not silently bypassed.

### 1.2 Why "smallest defensible" selects it — a trade-off, stated, not a ranking

| axis | this row | the `stat_prefix` crash |
|---|---|---|
| production touch-set, measured | **1 file**, `+7 / -10` at the prototype | **10 files** minimum, plus a new boot-panic trigger |
| governing decision | **already written** (ADR-0080 §Decision 1 + 2, §Alternatives (A)) | needs a parity ADR; the reference merges rather than rejecting |
| existing correct seam | **exists and is used by TCP** (`listenerfilter.SelectChain`) | exists (`*IfAbsent`) but must be threaded through ten packages |
| collateral guard breakage | none observed | breaks `TestMain_BootPanicIsVisible` **by design** |
| fixture directories implied | 0-1 | 2 |
| severity | wrong chain serves; filter and TLS provenance cross-wired | process crash on a config the reference accepts |

The crash is the more severe defect and this document does not pretend otherwise. The standing directive selects on **smallest defensible**, not on severity, and the router's own instruction is to treat every cost as a floor. This row's floor is one file against ten, with the decision already made and the correct algorithm already written and already in use on the sibling path.

⚠️ **The router recommended the other one. That is the phase-96 pattern repeating** — there, too, the router named the right file and the wrong member. The recommendation is recorded, re-derived, and declined on measurement.

### 1.3 What this row does NOT buy — stated plainly

- It does **not** implement SNI-dispatched multi-chain QUIC. That question is genuinely open, genuinely needs an ADR, and stays deferred. This row makes a **single** chain selection correct; it does not make QUIC select per-connection.
- It does **not** repair the `default_filter_chain` no-`transport_socket` reject divergence (D2-QUICTS), which is separately banked and reference-confirmed.
- It does **not** touch the TCP path, which already calls `SelectChain`.
- It buys **no** sentinel progress. See §8.5.

---

## 2. The defect, MEASURED

### 2.1 The rig and its controls, stated BEFORE the result

Subject binary built from the `phase-97-brainstorm` worktree at `5fe3c460` with `-o` into scratch (never into the worktree). Config derived from `test/fixtures/0104-http3-downstream-get`'s own known-good subject template — the only QUIC fixture in the tree — so the YAML shape is not invented. Ports **15702** and **15704** for the listeners and **15703** / **15705** for admin, inside the 15500-15999 ad-hoc band, below the 32768 ephemeral floor and clear of the harness reservations.

The listener carries **two** chains:

- `filter_chains[0]` — **no `filter_chain_match` at all**, therefore universally eligible at Pass 1 of the chain-match algorithm. Carries the QUIC transport socket with the `alpha.envoy-go.test` leaf. HCM `stat_prefix: INDEXED_CHAIN`.
- `default_filter_chain` — HCM `stat_prefix: DEFAULT_CHAIN`, a distinguishable `direct_response`.

**The criterion, stated before the run.** ADR-0080 §Decision 2 says the empty-match indexed chain wins. So a correct implementation books `http.INDEXED_CHAIN.downstream_rq_total: 1` and `http.DEFAULT_CHAIN.downstream_rq_total: 0`. The counters are the discriminator; the response body is the corroborator.

**Two shapes, because one cannot separate two hypotheses.** Shape 1 gives the default chain **no** `transport_socket` (plaintext). Shape 2 gives it its **own** QUIC transport socket. Shape 1 alone would be confounded: phase 96 measured that the reference **rejects** a no-`transport_socket` default chain on a QUIC listener, so a divergence on shape 1 alone is the already-banked D2-QUICTS finding wearing a different hat. **Shape 2 is the parity-relevant arm** and it is the one this row is chartered on.

### 2.2 The result — the eligible chain is bypassed on BOTH shapes

Client: `test/helpers.H3RoundTrip`, the repo's own H3 round-tripper, `GET /health`, ALPN `h3`, SNI `alpha.envoy-go.test`.

| shape | default chain TLS | observed status / body | `INDEXED_CHAIN` | `DEFAULT_CHAIN` |
|---|---|---|---|---|
| 1 — plaintext default chain | none | `222` / `"from-default-chain"` | **0** | **1** |
| 2 — default chain has its OWN QUIC transport socket | own | `222` / `"from-default-chain"` | **0** | **1** |

Both shapes validate clean (`-mode validate` rc=0, `configuration OK`) and boot clean (`envoy-go listener l_h3 ready`). **The universally-eligible `filter_chains[0]` served zero requests on both.**

**Shape 1 additionally cross-wires TLS and filter provenance within one connection.** The handshake completes — so the TLS identity came from `filter_chains[0]`, the only chain holding a `tlsCfg` — while the filters came from `default_filter_chain`. Two different chains served one connection. That is not a preference disagreement; it is an inconsistency no ADR authorises.

### 2.3 The mechanism — the two accessors name different chains, and neither uses the mandated algorithm

`internal/listener/quic.go`, read by symbol at this tip:

- `quicTLSConfig()` returns `rt.defaultChain.tlsCfg` only when **both** `rt.defaultChain` and its `tlsCfg` are non-nil, and otherwise walks `rt.chainByName` for the first non-nil TLS config.
- `quicChain()` returns `rt.defaultChain` whenever it is non-nil **at all**, regardless of TLS, and otherwise returns an arbitrary entry of `rt.chainByName`.

The predicates differ by exactly the `tlsCfg != nil` conjunct, which is why shape 1 splits them. Both walk `rt.chainByName`, a **map** — and both carry a comment conceding the iteration order is nondeterministic while asserting that is *"harmless here because the minimal QUIC slice supports exactly one chain."* **Nothing enforces that precondition.** Both probe configs carry two chains, validate rc=0 and boot.

`quicChain()` is called once, in `serveQUICConnection`, before the filter build. `quicTLSConfig()` is called twice: unconditionally in `startQUIC` to build the QUIC listener, and again in `serveQUICConnection` as the `http3.Server` TLS config after `quicChain()` has already returned non-nil.

### 2.4 The correct algorithm already exists and QUIC does not call it

The TCP path selects with `listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)` and then resolves the winner through `rt.chainByName[selectedSpec.Name]`. `rt.chainSpecs` is an **ordered slice**; `rt.defaultSpec` is the separate default slot. `SelectChain` is the ADR-0081 eight-dimension algorithm, and ADR-0080 §Consequences (a) states its contract: consult the default chain only when the eligibility set is empty.

**The QUIC path calls neither `SelectChain` nor `chainSpecs`.** It reaches around both into the unordered map. The repair direction is therefore not novel design; it is applying the seam the sibling path already uses.

### 2.5 Arms NOT run — recorded, not glossed

- **The reference was NOT measured on either shape.** Docker was serialized to one agent this session and spent on the front-runner's parity question. Whether the pinned reference accepts a two-chain QUIC listener at all, and which chain it serves, is **UNMEASURED** and is this row's blocking unknown. See §10.
- **No `filter_chain_match` dimension was varied.** Every arm used the empty-match shape, which exercises §Decision 2 only. §Decision 1's no-match fallback direction — an ineligible indexed chain, default chain correctly chosen — was **not** run and must be, because a repair that always prefers `filter_chains[]` would break it.
- **No SNI-varying arm was run**, deliberately: multi-chain dispatch is out of scope.

---

## 3. The mechanism's hazards for the SPEC

### 3.1 `startQUIC` needs ONE TLS config BEFORE any connection exists

`quicTLSConfig()` is called in `startQUIC` to build the QUIC listener, long before a connection or any `ChainMatchInputs` exist. A per-connection `SelectChain` call cannot serve that site. **The repair for this row is therefore a static, Start-time selection that both accessors agree on**, not a per-connection dispatch. Any SPEC that writes "call `SelectChain`" without saying *with what inputs, at what moment* has not specified the fix. This is `reference_registration_time_is_not_construction_time` in a new coat: the two call sites run at different moments and only one of them can see a connection.

### 3.2 A repair that always prefers `filter_chains[]` is WRONG in the other direction

ADR-0080 §Decision 1 requires the default chain when **no** indexed chain is eligible. The prototype in §6 takes the first entry of `chainSpecs` unconditionally, which is correct for the empty-match arm and **unvalidated** for the ineligible-indexed-chain arm. The SPEC owes an arm on both sides of that conditional; a guard that only tests one side of a branch is the vacuous-guard family.

### 3.3 The nondeterministic-map comment must be repaired or repealed, not left

Both accessors' comments assert a precondition ("exactly one chain") that the boot path does not enforce and that both probe configs violate. Whichever way the row goes, that comment becomes false or becomes load-bearing. Leave it and the next author inherits a false invariant — which is exactly the root cause phase 96 traced back twenty-two rows.

---

## 4. Rejected alternatives — every cost RE-DERIVED at this tip

### 4.1 The HCM `stat_prefix` duplicate-registration panic — **REJECTED for size; still the strongest banked candidate, and now MEASURED**

Everything below was measured this session and must not be re-derived:

- **It crashes; it does not hang.** `-mode validate` and normal boot both exit **2** in **0 seconds** with `panic: stats: duplicate metric registration: "http.<prefix>.downstream_rq_total"`, raised from `(*Registry).register` at `registry.go:107` via `hcm/config.go:358`. Reproduced independently by the controller. The stale "boot hang" prose in window 228 pre-dates row 78 and is false at this tip.
- **The negative control boots.** Distinct prefixes: validate rc=0, both listeners bind and serve, clean drain.
- **The reference ACCEPTS and MERGES.** `--mode validate` rc=0. On a real boot with 3 requests to one listener and 5 to the other, the reference emits `http.<prefix>.downstream_rq_total: 8` — merged — **and additionally** a listener-qualified `listener.<addr>.http.<prefix>.*` scope keeping them separate at 3 and 5. **envoy-go has no `listener.<addr>.http.<prefix>.*` surface at all**, which is a second, larger, independent gap.
- ⇒ **The parity-correct repair is get-or-create**, not a config reject. A reject diverges from a config the reference accepts.
- **It is not two-listener-specific.** Two HCMs in **one** listener across two filter chains panic identically. Two `fault` filters, or two `local_ratelimit` filters, in a single chain each panic on their own name.
- **`local_ratelimit` defines both a panicking and an `IfAbsent` stats constructor and the boot path calls the panicking one.** Auditing by which symbol a package mentions would miss it; only the call site decides.
- **The differential shape already exists.** `test/differential/harness.go` defines `SubjectOnlyBootRejectFixture` — reference boots, subject boot-rejects with a pinned stderr substring, request phase skipped — a precise fit for the current behaviour. The symmetric `BootRejectFixture` is the wrong shape here because the reference does not reject.
- **The blast radius on the guard is the real cost.** See §0.1.

### 4.2 An SDS × downstream `alpn_protocols` fixture — **REJECTED; the recorded runner-up, and the "empty intersection" claim CONFIRMED**

Structural sets at this tip: **6** fixtures carry `sds_config`; **7** mention `alpn_protocols`, of which only three carry it in a config. `comm -12` of the two sets is **empty**. ⚠️ A case-insensitive `sds` match is a trap — it also hits `StatsdSink`, inflating the set to 8.

The seams share exactly one code path (`NewDownstreamConfig` → `commonTLSContextToConfig`, which appends the SDS leaf and the ALPN list to the same config object) but are **behaviourally decoupled**: SDS in this tree is initial-fetch-only with no rotation, so the certificate is a build-time snapshot, and the ALPN fallback's `GetConfigForClient` has exactly one assignment site in the tree. **Coverage gap, no predicted divergence, ~1200-1400 added lines.** Lowest value per line on the roster.

### 4.3 The `registry.go:107` citation conflict — **REJECTED as a row; fold-in**

One wrong token on one line, plus a second wrong citation at `DECISIONS.md:2356` the banked note never named. Already recorded in three places. ⚠️ **A naive line-scoped substitution corrupts a correct citation on the same line** (§0.10). Fold into whichever row next edits `ROADMAP.md` outside a sentinel window.

### 4.4 D8-FCM's PGV-strictness nit — **REJECTED; the framing is wrong (§0.7)**

Its actionable residual is a documentation contradiction plus one CIDR shape. A real fix is a validation pass, which is family-sized.

⚠️ **An over-strictness in the opposite direction surfaced while measuring it and is NOT banked anywhere:** `parseChainSpec` boot-rejects any `filter_chain_match.transport_protocol` outside `{"", "tls", "raw_buffer", "quic"}`, while the field is a free string in the proto with no constraint. If the reference accepts an unknown value and simply never matches it, envoy-go rejects a config the reference accepts. **UNMEASURED against a live reference.**

### 4.5 The driver-owned receiver port race — **REJECTED for size; the banked figure re-derived and its PREDICATE named (§0.6)**

A 36-or-37-file mechanical sweep with no production defect behind it. Its trigger has still not fired. Open it when a full run aborts, not on the note.

⚠️ **A distinct and un-banked hazard surfaced beside it:** `0066`, `0067`, `0068` and `0071` deliberately allocate a port, close it, and rely on nothing listening there so a health-check probe **fails**. The port is inside the ephemeral range. If anything grabs it first, the probe **succeeds** and the fixture asserts the opposite of its proposition — a silent wrong **pass**, where the port-race roster produces a loud abort. Different class, worse failure mode.

### 4.6 The doc riders — **REJECTED as rows; each 1-3 prose lines. Carried.**

`0118/driver/driver.go:31`'s falsified band (§0.9) · `0120/expectations.yaml`'s stale **scope** (§0.12) · the drifted `manager_test.go` citation, now at `:5832` (§0.8) · `0108`'s three false `ssl.*` confessions · `0061-lb-ring-hash`'s sigma-margin second occurrence. ⚠️ The `GetConfigForClient` history-class comment count is **contested on purpose — quote no number**.

### 4.7 The stale window-228 carrier — **REJECTED as a row and NOT edited by this stage (§0.2, §9.4)**

---

## 5. Family attribution

**No family ordinal is claimed.** This is a core-listener / QUIC-dispatch **maintenance** row, on the row-85 through row-91, row-95 and row-96 precedent. The HTTP/3 family (opened at phase 61) stays open and its banked candidate list is untouched — this row consumes none of its bullets, and adds none.

---

## 6. The cost FLOOR — a built, run, and reverted prototype

⚠️ **This is a LOWER BOUND.** `reference_measured_prototype_is_a_lower_bound` has fired for fifteen consecutive rows, and at phase 96 it did not merely widen a range — it **inverted a comparison**, a one-line measured edit landing +1285 `.go` additions.

**Production: one file.** `git diff --numstat` on the prototype: `7 10 internal/listener/quic.go`. `quicTLSConfig()` delegates to `quicChain()` so the two can no longer disagree; `quicChain()` walks the ordered `rt.chainSpecs` first and falls back to `rt.defaultChain`. `gofmt -l` output empty; `go vet ./internal/listener/...` rc=0; the binary builds.

**Behaviour, driven on both shapes with the fixed binary:**

| shape | pre-fix | post-fix |
|---|---|---|
| 1 — plaintext default chain | `DEFAULT_CHAIN 1` / `INDEXED_CHAIN 0` | **`DEFAULT_CHAIN 0` / `INDEXED_CHAIN 1`**, body `"from-indexed-chain"` |
| 2 — default chain has own TLS | `DEFAULT_CHAIN 1` / `INDEXED_CHAIN 0` | **`DEFAULT_CHAIN 0` / `INDEXED_CHAIN 1`**, body `"from-indexed-chain"` |

**Existing tests: fully green under the prototype** — see §0.4, which is a finding about missing coverage, not a clean bill.

**The prototype was reverted and the revert PROVEN**: `sha256sum` captured before patching, `git checkout --` after, `sha256sum -c` reporting `OK`, and `git status --porcelain --untracked-files=all` empty. The un-fixed binary was then re-driven and still shows `222` / `"from-default-chain"`.

**What the floor does NOT include**, each of which will grow it: unit arms on both sides of the eligibility conditional (§3.2); the comment repair (§3.3); a decision on whether `startQUIC`'s Listen-time config and `serveQUICConnection`'s per-connection config may ever legitimately differ; and any differential fixture.

**Anticipated counts, every axis re-derived at this tip and each an ANTICIPATION, not a measurement:** stat NAMES **+0** · fixtures **+0 or +1** (123 today) · BackendKinds **+0** (tail 38) · fuzzers **+0** (56 targets / 48 files) · go.mod modules **+0** (67 require entries under a structural extractor) · new packages **0** · ADRs **1** (next-free **ADR-0319**), §Context drafted at the SPEC per ADR-0044.

---

## 7. The differential measurement

### 7.1 There is no existing gate — stated plainly

`0104-http3-downstream-get` is the **only** QUIC/H3 fixture in the tree. It builds a **single** `filter_chains[0]` with no `default_filter_chain`, so it cannot reach the disagreement. No unit test covers it either (§0.4).

### 7.2 What a gate would have to do, and the trap in it

A fixture arm must pin, cross-side, **which chain served** — by a per-chain `stat_prefix` counter, not by response body alone, because a body is one bit and the counters name the chain. ⚠️ **The subject and the reference must be shown to emit the same counter NAMES before either is pinned**: the reference additionally emits a listener-qualified HCM scope that envoy-go does not have (§4.1), so a naive whole-map comparison reddens against correct code. Pin a **named subset**.

⚠️ **`HTTPExpectations` is TCP-only.** An H3 arm drives through the driver's own hooks, on the `0104` precedent.

⚠️ **The QUIC fixture does not use the `10<index>` reference-port band** — `0104` sits at **15104**. Any new fixture must census the band it actually joins rather than inheriting the note.

---

## 8. Sentinel — RUN MECHANICALLY, ACTUAL OUTPUT, BOTH SIDES OF THIS STAGE'S OWN INSERTION

### 8.1 PRE-INSERTION, at `5fe3c460`

- **check (1)**, `want=128`: **SILENT**
- **check (2)**: **SIX**, at `:206 :212 :218 :228 :234 :242`
- **check (3)**: **SILENT**
- `ROADMAP.md` **246** lines, **128** data rows, tail row **96** `done`

### 8.2 The four mandated NCs, PRE-INSERTION — ALL FOUR FIRED

- **NC-A** (row 62 doctored to `in-progress` in a scratch copy, `want=128`): substitution inspected first — `NC LANDED? [ in-progress ]` — then **ONE** line, `NOT DONE: row 62`.
- **NC-B** (`want=127` on the real file): **ONE** line, `GATE FAIL: examined 128 data rows, expected 127`.
- **NC-C** (`gRPC-family row` substituted in a scratch copy): residual **0**, and `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D**: `-family row` reads **96** occurrences and **68** lines, with `--` before the pattern.

### 8.3 The check-(2) positive control

Both phrases substituted, and the substitution asserted rather than assumed: residual **0**, `candidatesXX` **6**.

### 8.4 The escape-aware malformed set

`sed 's/\\|//g' | awk -F'|'` with **no file argument to awk**: exactly **two** rows off the 8-field schema — file line **119** (row **57**, NF **9**) and file line **131** (row **69**, NF **10**). The naive form reads **17** lines and is not the same question.

### 8.5 Provenance of the pick against the six windows — and the instrument NC'd

A per-line case-insensitive fixed-string sweep of all six windows for this row's own tokens (`quic`, `default_filter_chain`, `quicChain`, `chain selection`) and for the front-runner's tokens (`stat_prefix`, `registry.go`, `hcm/config.go`).

**Instrument controls run first:** a token known present (`quic`) reads **3** in window 206 and 0 elsewhere; a fabricated token reads **0** in all six.

**Result:** the front-runner's tokens appear in window **228** only, at byte offset **42927** — roughly 28 000 bytes **after** that window's match phrase at offset **14608**, inside phase 74's "newly surfaced" list and not inside any candidate clause. This row's own subject touches **no** candidate clause in any window.

⇒ **ZERO sentinel progress, PROVEN and not assumed. Check (2) stays SIX.**

⚠️ **The margin is ONE.** Checks (1) and (3) are both silent. **Do not tidy a candidate line, and do not "fix" the six.**

⚠️ **Check (2) matches the WHOLE FILE, not only those six lines.** A new row that spelled either match phrase in its own summary cell would mint a seventh hit and read as a finding. Row 97's cell says "BANKED, NOT CHARTERED" instead, on the row-96 precedent.

### 8.6 POST-INSERTION — measured on the other side of this stage's own row

**NC shapes change across a row ADD and were RE-MEASURED, never inherited.** The row is an ADD, so the denominator moved: `want` **128 -> 129**, `ROADMAP.md` **246 -> 247**, row 97 at file line **159**.

- **check (1)**, `want=129`: **ONE** line, `NOT DONE: row 97`. It was SILENT before this stage and is not silent now — that is the row being registered `in-progress`, and it is correct.
- **check (2)**: **SIX**, now at `:207 :213 :219 :229 :235 :243` — every window shifted by exactly **+1** by the insertion above them.
- **check (3)**: **SILENT**.
- **NC-A** (`want=129`): substitution inspected — `NC LANDED? [ in-progress ]` — then **TWO** lines, `NOT DONE: row 62` and `NOT DONE: row 97`. It read ONE before this stage.
- **NC-B** (`want=128` on the real file): **TWO** lines, `NOT DONE: row 97` and `GATE FAIL: examined 129 data rows, expected 128`. It read ONE before this stage.
- **NC-C**: residual **0**, `NEVER OPENED: gRPC   <- NC FIRED`.
- **NC-D**: **96** occurrences / **68** lines, unchanged.
- **check-(2) positive control**: **6** substitutions asserted, residual **0**.

**Per-line md5 of the six windows at their NEW line numbers, trailing newline INCLUDED** (`sed -n 'Np' f | md5sum`, first 12 hex) — **ALL SIX BYTE-IDENTICAL to the pre-insertion reading and to the phase-96 IMPL close record:**

`207 10d7807bf02d` · `213 4a92f7e62fc6` · `219 2a7eb298b9fd` · `229 242e53c6f7a3` · `235 b2680e6f4fbf` · `243 6caa1c3ce0e7`

⚠️ **The digest is METHOD-SENSITIVE** — the six match only with the trailing newline included. State the method whenever quoting it.

**The escape-aware malformed set is UNCHANGED at exactly two rows** — file lines **119** (row 57, NF 9) and **131** (row 69, NF 10). **Row 97 did NOT join it**: its summary cell was field-counted under **both** the naive and the escape-aware forms **before** installation, reading **NF=8** on both, with every pipe reworded away rather than escaped. Method note 24 fired against the phase-96 IMPL's own author, who wrote a Go operator into a summary cell; this stage spells such operators out in words.

⇒ **THE SENTINEL DOES NOT FIRE. `stop` WAS EVALUATED AND DELIBERATELY NOT CREATED** — verified absent at the git root and in the stage worktree.

---

## 9. Findings the next stage must not re-learn

### 9.1 An absence claim is only as good as its matcher's vocabulary

A measurement agent searched a document for a **message string** and reported a stale claim absent; the claim was present under the **config-field** spelling, 42 000 bytes into a 47 000-byte line. Both greps were correctly executed. **The variable was the vocabulary, not the tree.** When an agent reports an absence, ask what it spelled before believing what it did not find. `reference_agreeing_measurements_do_not_generalize`'s sibling: a single disagreeing measurement has a variable, and the variable is findable.

### 9.2 A green package under a behaviour-inverting patch is a COVERAGE finding, not a pass

`go test ./internal/listener/...` is green before and after a patch that reverses which filter chain serves every QUIC request. **Run your fix and your un-fix through the same suite and compare; identical greens mean the suite is blind.**

### 9.3 An unrelated boot reject masks the site under test

A missing `admin` block produced rc=1 from a config written to produce rc=2 from a panic. **Prove your reproduction reached the site**, by the message and not by the exit code.

### 9.4 A cosmetic defect INSIDE a sentinel window is not a cosmetic edit

Window 228 carries a false present-tense claim about the front-runner. Repairing it means editing a line the termination sentinel matches, on a margin of one. **Record it; do not tidy it.** When a row does repair it, the repair must preserve that window's candidate clause byte-for-byte and re-run all three checks and all four NCs on both sides of the edit.

### 9.5 The two halves of a banked "needs an ADR" verdict can have different answers

D10-QUICSEL was banked as one design question. It is two: an **ordering** half ADR-0080 already settles, and a **dispatch** half that is genuinely open. **Split a banked verdict before pricing it** — pricing the pair together priced the cheap half at the expensive half's rate for two rows.

### 9.6 The fix shape still encodes a parity answer nobody has measured

Phase 96's §9.6 fires again, unchanged. The prototype makes the eligible indexed chain win because **ADR-0080 says so for TCP**. Whether the pinned reference accepts a two-chain QUIC listener at all, and which chain it serves, is unmeasured. **Measure the reference, then choose.**

### 9.7 Method findings, each found by execution

- A `direct_response` status of **111** came back as **200** with the body intact on the H3 path, while **222** came back as **222**. Unexplained and **not investigated** — recorded as an observation, not a claim. Choose non-1xx sentinel statuses in probes.
- The `10<index>` fixture reference-port band does not cover the QUIC family; `0104` sits at **15104**.
- The Bash tool's cwd reset to the repo root repeatedly, exactly as `reference_bash_cwd_reset_commits_to_main` warns. Every git command used `git -C <abs-path>`.

### 9.8 Defects found in passing — RECORDED, deliberately NOT fixed by this stage

`parseChainSpec`'s `transport_protocol` over-strictness (§4.4) · the health-check fixtures' dead-port false-green (§4.5) · envoy-go's absent `listener.<addr>.http.<prefix>.*` scope (§4.1) · duplicate listener **addresses** accepted by envoy-go's validate mode and rejected by the reference's (found while measuring the front-runner; envoy-go binds nothing in validate mode and has no duplicate-address check) · the third wrong `registry.go` citation at `DECISIONS.md:2356` (§0.10).

---

## 10. What the SPEC owes

1. **THE BLOCKING UNKNOWN: measure the pinned reference on both probe shapes.** Does `envoyproxy/envoy:contrib-v1.37.2` accept a QUIC listener carrying both a `default_filter_chain` and an eligible `filter_chains[]` entry, and which chain serves? Pin by digest, verified against `ENVOY_TARGET.md` lines 3-4. Include a plaintext negative control. **The repair shape depends on the answer and must not be chosen before it.**
2. **Specify the selection moment.** `startQUIC` needs a TLS config before any connection exists (§3.1). Say what is selected, from what, and when — for **both** call sites of `quicTLSConfig()` and the one of `quicChain()`.
3. **Arms on BOTH sides of the eligibility conditional** (§3.2): an eligible indexed chain must win; an **ineligible** indexed chain must leave the default chain winning. A guard that tests one side of a branch is vacuous.
4. **Decide the fate of the "exactly one chain" comments** (§3.3) — enforce the precondition at boot, or delete the claim. Do not leave it.
5. **State whether a fixture is chartered**, and if so census its port band rather than inheriting one (§7.2), and name all three registration gates including the blank import that is silently green if missed.
6. **Re-derive every figure in this document at the SPEC's own publishing commit**, in the same commit as the edits that move it. This document's figures are true at `5fe3c460` and expire when the SPEC edits anything.
7. **Draft ADR-0319 §Context only.** A SPEC adds §Context; §Decision and §Consequences land at the IMPL per ADR-0044. ⚠️ Drafting a house-form `STATUS: PROPOSED` line **RE-ARMS** the guard that is currently disarmed and reading 0.
8. **Do NOT touch `ROADMAP.md` or `BEHAVIOR_CONTRACT.md`** — measured across four consecutive SPEC commits, a SPEC leaves both byte-untouched.
9. **Carry the front-runner's measurements forward intact** (§4.1). They cost a Docker-serialized agent and must not be re-derived.

---

## 11. Probe hygiene

Every probe config, binary and log lives in the session scratch directory; **nothing was written into any worktree except the prototype patch and one temporary test file**, both removed. The prototype revert is proven by `sha256sum -c` returning `OK` on `internal/listener/quic.go` and by `git status --porcelain --untracked-files=all` reading empty afterwards. The temporary H3 probe test file under `test/helpers/` was deleted after each run and the tree re-checked. No Docker container was created by the controller. Ports **15702-15705** only, released at each teardown by killing PIDs captured at launch — never by a pattern match, which would kill the tool call.
