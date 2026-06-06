# Phase 30 SPEC — reference-image pin-refresh to `envoyproxy/envoy:contrib-v1.37.2`

> **For agentic workers:** this is the phase SPEC for **phase 30** (`pin-refresh-envoy-contrib`), a **flat infrastructure row** (NOT a §9 Network-filters-family row — ROADMAP row 30, depends-on 29). It is authored per `superpowers:writing-plans` scoped to SPEC authoring (the project's BRAINSTORM → SPEC precedent), executing the BRAINSTORM §6 **D30-1..D30-6 empirical pins IN-SESSION** and anchoring the **ADR-0227 §Context** draft. The next session, per `BOOTSTRAP_PROMPT.md` §5, authors the **phase-30 PLAN** (bite-sized TDD-style tasks) from this SPEC. Steps in §10 use the SPEC-anticipated task-spine shape; the bite-sized checkbox PLAN is the next session's deliverable.

**Goal:** Swap the SHA-pinned differential reference image from the standard Envoy variant to the **contrib variant of the same upstream version** — `envoyproxy/envoy:v1.37.2` → **`envoyproxy/envoy:contrib-v1.37.2`** — re-baseline all **54** existing differential fixtures byte-identical-PASS against it, and supersede ADR-0008 with **ADR-0227**. ZERO production LoC; ZERO new go.mod deps; ZERO new package/stat/fixture/fuzzer/BackendKind. The motivation is downstream: unblock the contrib-only `kafka_broker` §9 subject (phase 31).

**Architecture:** A pure pin-refresh per `ENVOY_TARGET.md` doctrine **D-3.7** (*"the pin is changed only via a dedicated phase that re-baselines the differential suite"*). The ONLY runtime-load-bearing change is `docs/envoy-go/ENVOY_TARGET.md`: `test/differential/harness.go::parseEnvoyTarget` reads `**Tag:**`/`**SHA256:**` from it, and `StartReferenceProxy(ctx, pin, …)` boots `pin.SHA256` as the testcontainers image, so editing the two pin lines atomically repoints every reference-booting fixture. No Go code changes; the contrib image is the standard image plus extra compiled-in extensions (a behavioral superset), proven by the unchanged 54-fixture differential surface.

**Tech Stack:** Docker / testcontainers-go (the differential harness — unchanged); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (this phase, ADR-0227, superseding the ADR-0008 standard-variant pin); Go 1.26.2; go-control-plane `/envoy` v1.32.4 (ADR-0008; UNCHANGED — phase 30 adds no go.mod dep). The contrib digest is captured via `docker pull` + `docker inspect --format='{{index .RepoDigests 0}}'` per the `ENVOY_TARGET.md` refresh procedure.

**Authored:** 2026-06-06. **Empirical-pin probe date (D30-1..D30-6):** 2026-06-06 (THIS SPEC session — §5 + §11). **Baseline-anchor re-pin date:** 2026-06-06 (this SPEC session, master tip `b448178` — §11.1).

---

## 1. Purpose / Mission

Phase 30 delivers the project's **FIRST reference-image pin change since ADR-0008** (the unchanged standard-variant v1.37.2 pin since phase 02), as the dedicated re-baselining phase mandated by D-3.7. It is a flat infrastructure row — no feature, no family membership, no production code.

### 1.1 What phase 30 delivers as a self-contained whole

1. **The reference image swap** — `docs/envoy-go/ENVOY_TARGET.md` `**Tag:**` → `envoyproxy/envoy:contrib-v1.37.2` + `**SHA256:**` → the captured contrib digest (`sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8` — §5.1) + refreshed `**Upstream release notes:**` URL + `**Pinned in:**` → ADR-0227 + `**Last verified:**` date. This single file is the ONLY runtime-load-bearing change (§3.1).
2. **The 54-fixture re-baseline** — the full `ls -d test/fixtures/[0-9]* | wc -l` = 54 differential suite byte-identical-PASS against the contrib image (`-count=1`), plus the conformance gates (h2spec 53/53, proxy-wasm 10/10) re-run as a standing sanity gate (image-independent — §5.3). The divergence policy (D-3.7 step 3): any divergence is investigated → envoy-go fixed to match, OR recorded as a `BEHAVIOR_CONTRACT.md` extension with its own ADR. Expected (and observed — §5.2): **none**.
3. **ADR-0227** superseding ADR-0008 (the contrib-variant pin; same upstream version v1.37.2; the 54-fixture superset-parity evidence; the contrib filters it unblocks). §Context drafted at this SPEC; §Decision/§Consequences body at the phase-30 IMPL per ADR-0044 (§6).
4. **The `BEHAVIOR_CONTRACT.md` current-pin lines** updated to the new digest + ADR-0227 (the two `### Applies to` `ENVOY_TARGET pin` bullets — §7); the frozen historical references left untouched.
5. STATE / ROADMAP advance + next-prompt rewrite (this SPEC commit opens the phase-30 PLAN cold-start); at IMPL the standard six-gate + completion bundle.

### 1.2 The corrected image identity — AMEND-D30 (the D30-1 finding)

The phase-30 BRAINSTORM (§1, §2.2, §3.1) and ROADMAP row 30 assume the contrib variant ships as **`envoyproxy/envoy-contrib:v1.37.2`** — a separate Docker Hub repository. The D30-1 empirical pin (BLOCKING; §5.1) **refutes** that assumption:

- The `envoyproxy/envoy-contrib` repository **exists but its release line stops at v1.35.0** — it carries **no v1.37.x tag** (Docker Hub `name=1.37` filter → 0 results; the highest semver tag is `v1.35.0`). A `docker pull envoyproxy/envoy-contrib:v1.37.2` fails with `not found`.
- The contrib variant for v1.37.2 ships instead as a **`contrib-`-prefixed tag inside the standard `envoyproxy/envoy` repository**: **`envoyproxy/envoy:contrib-v1.37.2`** (alongside `contrib-debug-v1.37.2` and `contrib-distroless-v1.37.2`). This is the current Envoy image-publishing convention (confirmed live for the v1.36/v1.37/v1.38 lines).

**AMEND-D30:** the phase-30 target image is **`envoyproxy/envoy:contrib-v1.37.2`** (NOT `envoyproxy/envoy-contrib:v1.37.2`). This SPEC supersedes the BRAINSTORM §1/§2.2/§3.1 + ROADMAP row 30 image-reference string. The phase **premise is unchanged** — same upstream version (`v1.37.2`), contrib variant, unblocks the contrib-only `kafka_broker` extension; only the reference string is corrected. The ROADMAP row 30 stale image string is corrected in place at this SPEC commit (the project precedent of correcting stale BRAINSTORM-time row text at the SPEC — e.g. the 29.1 row's `.mongo.` correction).

### 1.3 Not a family-row landing

Phase 30 lands no §9 family row; the §9 Network-filters candidate roster `redis / kafka_broker / thrift` is UNCHANGED by phase 30. The `kafka_broker` §9 row is **phase 31** (the BRAINSTORM §4 forward-pointer), which depends on phase 30 and adds the `/contrib v1.32.4` go.mod dep with its first consumer. After phase 30 the remaining §9 candidates are still `redis / kafka_broker / thrift` (`kafka_broker` now *unblocked* but not yet landed).

### 1.4 ADR-0045 split readiness — NO split (well under the gate)

Per ADR-0045 §6 the split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Phase 30 ships **zero production LoC** and an anticipated **~4–6 task spine** (§10). No split; no pre-split sub-phases.

---

## 2. Non-purposes

Phase 30 does NOT do anything beyond the image swap + re-baseline.

- **2.1 No kafka_broker code, no `/contrib` go.mod dep, no new filter package, no new stat, no new fixture, no new fuzzer, no new BackendKind.** Counts are UNCHANGED across phase 30 (54 fixtures / 39 fuzzers / 360 stats / BackendKind tail 30 / DECISIONS tail advances ADR-0226 → ADR-0227 at the IMPL when the §Decision/§Consequences body lands — at THIS SPEC the §Context draft lands and the tail becomes ADR-0227 per the §Context-anchor precedent; see §6). The `/contrib` dependency lands in **phase 31.1** with its first consumer (an unused module dep cannot survive `go mod tidy` — BRAINSTORM §2.4).
- **2.2 No rewrite of historical SHA references.** The many `envoyproxy/envoy:v1.37.2` + old-digest mentions in prior phases' `PROGRESS.md`/`SPEC.md`/`PLAN.md` (and the fixture-driver comments + the zookeeper `stats.go` probe-date headers) are durable records of what was true at those phases — they stay frozen. Only the live `ENVOY_TARGET.md` + the two current-pin `BEHAVIOR_CONTRACT.md` lines change (§3.4 + §7).
- **2.3 No `harness.go` / `harness_test.go` code change.** The `v1.34.0` / `sha256:abc123def456` strings in `harness_test.go` are a *parser* unit-test sample (`TestParseEnvoyTarget_PullsTagAndDigest` feeds a hardcoded sample doc) — they have NO coupling to the live pin (the live pin is v1.37.2) and stay as-is (§8 / D30-5).
- **2.4 No version bump.** The upstream version stays `v1.37.2`; only the *variant* changes (standard → contrib). Keeping the version fixed isolates the variable — any 54-fixture divergence would be attributable to the core↔contrib build difference, not a version jump (BRAINSTORM §2.2).

---

## 3. Change-surface inventory

### 3.1 The one runtime-load-bearing edit

`docs/envoy-go/ENVOY_TARGET.md` — `parseEnvoyTarget` (the `tagLineRE` / `sha256LineRE` regexes on `**Tag:**` + `**SHA256:**`) reads it; `StartReferenceProxy` / `StartReferenceProxyWithMounts` / `tryStartReferenceProxy` use `pin.SHA256` as the testcontainers `Image`. Editing the Tag + SHA256 lines repoints all reference-booting fixtures atomically. Also update the release-notes URL (the v1.37.2 URL is unchanged — same upstream version — but the `Pinned in:` becomes ADR-0227 and `Last verified:` becomes the IMPL date). **At THIS SPEC the file is NOT edited on the landing branch** (the edit lands at IMPL per the BRAINSTORM §1.1 / D-3.7 "capture-at-IMPL" discipline); the SPEC session performed the edit on a *throwaway* basis in the SPEC worktree to drive the D30-2 re-baseline, then discarded it.

### 3.2 Code deliberately UNTOUCHED

`test/differential/harness.go` + `test/differential/harness_test.go` — the `EnvoyPin` struct comment (`e.g. envoyproxy/envoy:v1.34.0`) and the `TestParseEnvoyTarget_*` unit tests use a hardcoded `envoyproxy/envoy:v1.34.0` / `sha256:abc123def456` **sample document** to test the *parser*, not the live pin. No behavioral coupling; left as-is to minimize churn (§8 / D30-5). (Optional cosmetic refresh of the sample is explicitly declined — leave it.)

### 3.3 Docs touched (at IMPL unless noted)

`docs/envoy-go/ENVOY_TARGET.md` (the pin — §3.1, IMPL); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the two current-pin lines → new digest + ADR-0227 — §7, IMPL); `docs/envoy-go/DECISIONS.md` (ADR-0227 §Context at THIS SPEC; §Decision/§Consequences body at IMPL — §6); `docs/envoy-go/ROADMAP.md` (row 30 stale image-string correction at THIS SPEC — §1.2; row 30 `in-progress → done` at IMPL); `docs/envoy-go/STATE.md` + `next-prompt.txt` (advance at THIS SPEC for the PLAN cold-start; re-advance at PLAN/IMPL); the phase dir (`SPEC.md` + `README.md` at THIS SPEC; `PLAN.md` next session; `PROGRESS.md` at IMPL).

### 3.4 Frozen (NOT rewritten)

Every historical `PROGRESS.md`/`SPEC.md`/`PLAN.md` mention of `envoyproxy/envoy:v1.37.2` + the old `c5e8a68e…` digest; the fixture-driver comments (`test/fixtures/0035/0037/0039/.../inputs/driver.go`) naming the ADR-0008 image; the `internal/filter/network/zookeeperproxy/stats.go` + `stats_test.go` probe-date headers (`envoyproxy/envoy:v1.37.2 admin /stats dump; probe date 2026-06-02`); and the phase-06.2 access-log empirical-evidence block in `BEHAVIOR_CONTRACT.md` (§543–558 — a verbatim capture-provenance record paste-synchronized with the 06.2 SPEC, valid as captured; see §7.3). These are durable per-phase records of what was true then — frozen per D-3.7.

---

## 4. Framework / production touchpoints — NONE (a pinned property of this phase)

Phase 30 touches ZERO Go production or test code. No `internal/` change, no `cmd/` change, no `test/` Go change (the harness is unchanged; the fixtures are unchanged; the re-baseline is a *run*, not an edit). The only landing-branch deltas are documentation: `ENVOY_TARGET.md` (the pin), `BEHAVIOR_CONTRACT.md` (the two current-pin lines), `DECISIONS.md` (ADR-0227), `ROADMAP.md`/`STATE.md`/`next-prompt.txt` (lifecycle), and the phase dir. This zero-LoC property is the reason the phase has no TDD red/green cycle — its "test" is the full differential + conformance gate run against the new image (§5 / §10).

---

## 5. The empirical re-baseline (D30-1..D30-3 EXECUTED at this SPEC session)

Per the BRAINSTORM §6, the SPEC author executes the empirical pins IN-SESSION. The ENVOY_TARGET.md pin edit + the authoritative re-baseline land at IMPL (D-3.7); this SPEC session ran the re-baseline on a throwaway worktree pin to ground the SPEC's central claim and the ADR-0227 evidence.

### 5.1 D30-1 (BLOCKING) — the contrib image identity + digest

- **Finding (AMEND-D30, §1.2):** `envoyproxy/envoy-contrib:v1.37.2` does NOT exist (the `envoyproxy/envoy-contrib` repo stops at v1.35.0). The contrib variant for v1.37.2 is **`envoyproxy/envoy:contrib-v1.37.2`**.
- **Pull:** `docker pull envoyproxy/envoy:contrib-v1.37.2` → success (image id `7edd5b0fd763`, **299 MB** vs the standard `envoyproxy/envoy:v1.37.2`'s 263 MB — consistent with the extra compiled-in contrib extensions; tag pushed 2026-04-10, the v1.37.2 release window).
- **Digest (the `ENVOY_TARGET.md` `**SHA256:**` value):** `docker inspect --format='{{index .RepoDigests 0}}' envoyproxy/envoy:contrib-v1.37.2` →

  ```
  envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8
  ```

- The new pin lines for IMPL:

  ```
  **Tag:** `envoyproxy/envoy:contrib-v1.37.2`
  **SHA256:** `envoyproxy/envoy@sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`
  ```

### 5.2 D30-2 — the full 54-fixture differential re-baseline against the contrib image

Method: in the SPEC worktree, the `ENVOY_TARGET.md` `**SHA256:**` was temporarily repointed to the contrib digest and the full suite run with `go test ./test/differential/... -count=1` (the `-count=1` defeats go-test result caching per `reference_differential_break_protocol_count1`); the throwaway edit was then discarded. The contrib image booted every reference-side fixture (the harness `StartReferenceProxy`/`StartReferenceProxyWithMounts`/`tryStartReferenceProxy` paths) exactly as the standard image did.

**Result:** **byte-identical PASS — ZERO divergence.** `ok github.com/esalaine/envoy-go/test/differential 185.458s` + `ok …/test/differential/fixture 0.001s`, process **EXIT=0**. All 54 fixture directories — the cross-side `StatsAsserter` fixtures, the reference-less wire-shape oracles, the symmetric + subject-only boot-reject fixtures, the access-log fixture `0006`, and every HTTP/H2/TLS/network-filter fixture — passed against `envoyproxy/envoy:contrib-v1.37.2` exactly as they pass against the standard `envoyproxy/envoy:v1.37.2`. (Run wall-clock ~3 min — the harness runs fixtures in parallel.)

**Divergence classification (D-3.7 step 3):** **none.** The contrib image is a behavioral **superset** of the standard image — the identical Envoy v1.37.2 source plus extra compiled-in contrib extensions (image 299 MB vs 263 MB) — so every byte the standard image emitted on the existing fixture surface, the contrib image emits identically. No envoy-go fix, no `BEHAVIOR_CONTRACT.md` behavior extension, and no revert is needed. This empirically confirms the BRAINSTORM §2.3 "expected divergences: none" prediction.

### 5.3 D30-3 — conformance image-independence (proven analytically + by grep)

The conformance suites are **image-independent** — they exercise the envoy-go *subject* (its own h2c listener / its own wasm host), NOT the pinned reference container. Evidence: `grep -rln "StartReferenceProxy\|ENVOY_TARGET\|parseEnvoyTarget\|EnvoyPin\|pin.SHA256\|envoyproxy/envoy" test/conformance/` returns **nothing** — neither `test/conformance/h2spec/` nor `test/conformance/proxy-wasm/` references the reference image, the pin file, or the pin parser. The swap therefore carries no correctness dependency on conformance; h2spec 53/53 + proxy-wasm 10/10 are re-run at the IMPL six-gate only as a standing sanity gate (no change expected, none possible from the image swap). Re-run result at this SPEC session: **green** — `go test ./test/conformance/... -count=1` → `ok …/test/conformance/h2spec 2.604s` + `ok …/test/conformance/proxy-wasm 0.272s`, EXIT=0 (h2spec 53/53, proxy-wasm 10/10; run in the SPEC worktree with the contrib pin in effect, confirming — as expected — that the pin has no bearing on conformance).

### 5.4 Divergence policy (D-3.7 step 3) — restated for the IMPL

If the IMPL six-gate re-baseline surfaces ANY fixture divergence (a fixture that PASSed on the standard image but FAILs on the contrib image), the IMPL MUST classify it before landing: either (a) fix envoy-go to match the contrib image's behavior (with the fix's own evidence), or (b) record the divergence as a `BEHAVIOR_CONTRACT.md` extension with its own ADR (next-free after ADR-0227 ≈ ADR-0228), or (c) revert the pin. The SPEC-session re-baseline (§5.2) is the advance evidence that case (a)/(b)/(c) is not expected; the IMPL re-run is the authoritative gate.

---

## 6. ADR-0227 — the pin-refresh (supersedes ADR-0008)

A new **ADR-0227** records the pin change and supersedes ADR-0008 (the original standard-variant v1.37.2 pin). Per ADR-0044, the **§Context draft lands at THIS SPEC** (in `DECISIONS.md`, appended after ADR-0226); the **§Decision/§Consequences body lands at the phase-30 IMPL** alongside the `ENVOY_TARGET.md` edit. The DECISIONS.md tail advances to **ADR-0227** at this SPEC §Context anchor (the parent-29 / 28 precedent: a §Context-only ADR draft DOES advance the tail at its anchoring commit; the body fills in place later). Next-free after phase 30 ≈ **ADR-0228** (the ADR-0209 escape-valve reserve carried from the §9 family stays unconsumed).

**ADR-0227 §Context (the drafted wording):** records (i) the contrib-variant choice — same upstream version v1.37.2, the contrib build (a behavioral superset: the standard image plus extra compiled-in extensions); (ii) the corrected image identity (AMEND-D30: `envoyproxy/envoy:contrib-v1.37.2`, NOT a separate `envoy-contrib` repo); (iii) the motivation (unblock Envoy contrib network filters — `kafka_broker` first, phase 31; `sip_proxy`/`mysql_proxy`/`postgres_proxy`/`rocketmq_proxy`/`generic_proxy` latent); (iv) the D-3.7 dedicated-re-baselining-phase mandate; (v) the 54-fixture superset-parity evidence (§5.2) + the conformance image-independence (§5.3). The exact §Context text is in `DECISIONS.md` at this commit.

---

## 7. `BEHAVIOR_CONTRACT.md` current-pin lines (D30-4)

The live digest `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` appears in `BEHAVIOR_CONTRACT.md` at exactly **three** sites; D30-4 classifies each:

### 7.1 The two CURRENT-PIN lines to update (at IMPL)

Both are `### Applies to` bullets stating the live contract surface; both must move to the contrib digest + ADR-0227:

- **Line 728** (the admin-API `## ...` section's `### Applies to`):
  `- ENVOY_TARGET pin v1.37.2 at \`sha256:c5e8a68e…\` (ADR-0008).`
- **Line 804** (the graceful-drain `### Applies to`):
  `- ENVOY_TARGET pin v1.37.2 at \`sha256:c5e8a68e…\` (ADR-0008).`

Each becomes (IMPL): `- ENVOY_TARGET pin v1.37.2 (contrib) at \`sha256:7edd5b0f…\` (ADR-0227, superseding ADR-0008).` (exact wording an IMPL micro-decision; the digest + ADR reference are load-bearing).

### 7.2 The FROZEN site (NOT updated)

- **Line 548** — inside the phase-06.2 `### Empirical evidence (verbatim excerpt from reference-Envoy /tmp/envoy-access.log)` code block: `reference image v1.37.2 at ENVOY_TARGET.md SHA c5e8a68e…; captured 2026-04-30 by phase 06.2 PLAN Task 3 step 3`. This is a **capture-provenance record** — it documents the image under which the verbatim access-log excerpt was captured, paste-synchronized with the 06.2 SPEC §11 block (line 558's drift-prohibition note). Because the access-log format is unchanged under the contrib image (same upstream version — proven by fixture `0006-access-log` re-baselining green in §5.2), the captured evidence remains valid as-is; the provenance line is frozen per §3.4 / D-3.7. The line 558 drift-prohibition fires only if a future image bump *alters the format* — which this same-version variant swap does not.

### 7.3 D30-4 resolution

**Two** current-pin lines change (728, 804); **one** site is frozen (548). The IMPL edits exactly the two; the BEHAVIOR_CONTRACT 30 bundle also adds a short phase-30 note recording the pin-refresh (the variant swap + the superset-parity outcome) per the ADR-0052 atomic-landing discipline.

---

## 8. The `harness_test.go` parser sample + no-other-hardcode confirmation (D30-5)

- **The parser sample is left as-is.** `harness_test.go::TestParseEnvoyTarget_PullsTagAndDigest` feeds a hardcoded `envoyproxy/envoy:v1.34.0` / `sha256:abc123def456` sample doc and asserts the parser extracts them. This tests `parseEnvoyTarget`, not the live pin (the live pin is v1.37.2, unrelated to the v1.34.0 sample). No change (§2.3 / §3.2).
- **No other runtime path hardcodes the live tag/digest.** `grep -rn "envoy:v1.37.2\|c5e8a68e…\|envoy-contrib" --include="*.go"` finds only: (a) fixture-driver *comments* (`0035/0037/0039`) citing the ADR-0008 image as documentation; (b) the zookeeper `stats.go`/`stats_test.go` *comment* headers recording a probe date. None is runtime-load-bearing; all are frozen historical references (§3.4). The single runtime read of the pin is `parseEnvoyTarget(ENVOY_TARGET.md)` → `pin.SHA256` → the testcontainers `Image`. **D30-5 RESOLVED:** `ENVOY_TARGET.md` is the sole runtime-load-bearing pin site.

---

## 9. `ENVOY_TARGET.md` refresh-procedure compliance record (D30-6)

The `ENVOY_TARGET.md` §"Refresh procedure" 6-step recipe, mapped to phase 30's execution:

| Step | Recipe | Phase-30 execution |
|---|---|---|
| 1 | Pick a new candidate tag per the selection criteria (stable, current within 6 months, no API transition in flight). | `envoyproxy/envoy:contrib-v1.37.2` — same upstream version as the current pin (a variant swap, the minimal change); stable; well within the window; no API transition. The candidate is fixed by the motivation (the contrib variant is required for `kafka_broker`); the selection is the variant, not a version bump. |
| 2 | `docker pull <tag>`; capture the SHA256 via `docker inspect --format='{{index .RepoDigests 0}}'`. | DONE at this SPEC (§5.1): digest `sha256:7edd5b0f…`. (The tag form is `contrib-v1.37.2`; AMEND-D30 corrected the repo/tag shape vs the BRAINSTORM.) |
| 3 | Run all differential fixtures against the new image; investigate any divergence — fix envoy-go, or extend `BEHAVIOR_CONTRACT.md` (with an ADR), or revert. | DONE at this SPEC (§5.2): full 54-fixture suite `-count=1`. Divergence classification: **none** (54/54 byte-identical PASS — §5.2). The authoritative re-run is the IMPL six-gate. |
| 4 | Update `ENVOY_TARGET.md` with the new tag, SHA, release-notes URL, and `Last verified` date. | **IMPL** (§3.1): the landing-branch edit. (The SPEC ran it throwaway only.) |
| 5 | Append a new ADR superseding ADR-0008 (and any contract-extension ADRs from step 3). | ADR-0227 §Context at THIS SPEC (§6); §Decision/§Consequences body + any contract-extension ADR (none expected) at IMPL. |
| 6 | Land as a single commit on the pin-refresh phase branch. | **IMPL** — the `ENVOY_TARGET.md` + `BEHAVIOR_CONTRACT.md` + ADR-0227 body + ROADMAP/STATE atomic landing under the six-gate (ADR-0052). |

**D30-6 RESOLVED:** ADR-0227 supersedes ADR-0008 with the wording in §6; the 6-step compliance is recorded above (steps 1–3 + 5-§Context done at SPEC; steps 4 + 5-body + 6 at IMPL).

---

## 10. Per-task structure (~4–6 tasks; the SPEC-anticipated task spine)

The phase-30 PLAN (next session) decomposes this into bite-sized tasks. The anticipated spine (zero production LoC → no TDD red/green; the "test" is the gate run):

1. **T1 — Baselines / anchors gate.** Verify master-tip counts (54 fixtures / 39 fuzzers / 360 stats / BackendKind 30 / DECISIONS tail ADR-0227-§Context) + the contrib image present locally (`docker inspect` digest matches §5.1) + a clean six-gate on the UNCHANGED (standard-image) pin as the regression baseline.
2. **T2 — Flip `ENVOY_TARGET.md`.** Edit the Tag + SHA256 + `Pinned in:` (ADR-0227) + `Last verified:` lines to the contrib pin (§3.1 / §5.1). (Release-notes URL unchanged — same version.)
3. **T3 — Re-baseline the 54-fixture differential suite.** `go test ./test/differential/... -count=1` against the flipped pin → byte-identical PASS (the authoritative re-run; §5.2). Investigate + classify any divergence per §5.4.
4. **T4 — Conformance sanity.** Re-run h2spec (53/53) + proxy-wasm (10/10) — image-independent (§5.3); confirm green.
5. **T5 — ADR-0227 body + `BEHAVIOR_CONTRACT.md` pin lines.** Land the ADR-0227 §Decision/§Consequences body in place (§6); update the two current-pin lines (728, 804) + add the BEHAVIOR_CONTRACT 30 note (§7); leave the frozen site (548) untouched.
6. **T6 — Completion bundle.** The six-gate (`go build`/`go vet`/`golangci-lint`/`go test -race -short`/the full differential/conformance) GREEN LIVE; STATE/ROADMAP advance (row 30 `in-progress → done`); next-prompt rewrite (phase-31 `kafka_broker` BRAINSTORM cold-start); PROGRESS.md.

### 10.1 ADR-0045 split-gate — SPEC-level re-check

~4–6 tasks / **0 production LoC** → far under the `> ~25 tasks OR > ~1500 LoC` gate. **NO split.**

---

## 11. SPEC-time empirical-pin block

### 11.1 D-S1 — master-tip baselines + as-built anchors VERIFIED at this SPEC session (2026-06-06)

- master tip `b448178` (`next-prompt.txt: name the live SHA-fill tip b0f6ca3 …`); substantive predecessor `d671263` (29.3 IMPL). origin/master in sync.
- differential fixtures **54** (`ls -d test/fixtures/[0-9]* | wc -l` = 54; tail `0052-mongo-fault-delay`); fuzzers **39**; stat surface **360**; BackendKind tail **30** (`TCPMongoResponder`); DECISIONS tail **ADR-0226** at master tip (→ **ADR-0227** at this SPEC §Context anchor).
- current pin `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e…` (ADR-0008); present locally (image id `c5e8a68e52f4`, 263 MB).

### 11.2 D30-1..D30-6 — the empirical pins (executed at this session)

- **D30-1 (BLOCKING):** RESOLVED — §5.1. The contrib image is `envoyproxy/envoy:contrib-v1.37.2` (AMEND-D30); digest `sha256:7edd5b0f…`.
- **D30-2:** EXECUTED — §5.2. Full 54-fixture suite `-count=1` against the contrib image.
- **D30-3:** RESOLVED — §5.3. Conformance image-independent (grep evidence); re-run sanity-green.
- **D30-4:** RESOLVED — §7. Two current-pin lines (728, 804) to update; one frozen (548).
- **D30-5:** RESOLVED — §8. `harness_test.go` sample left as-is; `ENVOY_TARGET.md` the sole runtime pin site.
- **D30-6:** RESOLVED — §6 + §9. ADR-0227 supersedes ADR-0008; the 6-step compliance record.

---

## 12. SPEC-time D-questions — resolutions + PLAN/IMPL carries

### 12.1 RESOLVED at this SPEC

- **AMEND-D30 (image identity):** `envoyproxy/envoy:contrib-v1.37.2` (§1.2 / §5.1) — supersedes the BRAINSTORM `envoy-contrib` repo assumption.
- **D30-1..D30-6:** §11.2.
- **The SPEC does NOT land the `ENVOY_TARGET.md` edit** (D-3.7 capture-at-IMPL; the SPEC ran it throwaway) — §3.1 / §5.

### 12.2 Carried to PLAN / IMPL

- **D-S30-1 (the authoritative re-baseline is the IMPL six-gate).** The SPEC-session §5.2 run is advance evidence; the IMPL re-runs the full differential + conformance on the landing-branch pin as the load-bearing gate (the 28/29 six-gate precedent).
- **D-S30-2 (the BEHAVIOR_CONTRACT 30 note wording).** The exact phase-30 contract note + the two current-pin line edits are an IMPL micro-decision within §7's pin.
- **D-S30-3 (next-prompt successor).** At the phase-30 IMPL, next-prompt opens the **phase-31 `kafka_broker` BRAINSTORM** cold-start (the BRAINSTORM §4 forward-pointer) — NOT a SPEC (phase 31 is a new family row needing its own brainstorm).

---

## 13. Counts (UNCHANGED by phase 30)

- differential fixtures **54** (tail `0052-mongo-fault-delay`); phase 30 adds none.
- fuzzers **39**; phase 30 adds none.
- stat surface **360**; phase 30 adds none.
- BackendKind tail **30** (`TCPMongoResponder`); phase 30 adds none.
- DECISIONS.md tail **ADR-0227** at this SPEC (§Context anchor; §Decision/§Consequences body at IMPL); next-free after phase 30 ≈ **ADR-0228**.
- reference pin: `envoyproxy/envoy:v1.37.2` @ `sha256:c5e8a68e…` (ADR-0008) → `envoyproxy/envoy:contrib-v1.37.2` @ `sha256:7edd5b0f…` (ADR-0227, this phase; the landing-branch edit at IMPL).
- conformance h2spec 53/53 + proxy-wasm 10/10 (image-independent; re-run as sanity at the six-gate).
- Toolchain: Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008) — `/contrib` v1.32.4 confirmed resolvable for phase 31 (NOT added at phase 30).

---

## 14. Stage-close handoff

This SPEC settles phase 30 as a pure pin-refresh: swap `envoyproxy/envoy:v1.37.2` → **`envoyproxy/envoy:contrib-v1.37.2`** (AMEND-D30 — the corrected image identity vs the BRAINSTORM's `envoy-contrib` repo assumption), re-baseline all 54 fixtures byte-identical green against it (§5.2), supersede ADR-0008 with ADR-0227 — zero production LoC, zero new go.mod deps, zero new package/stat/fixture/fuzzer/BackendKind, NO split. The D30-1..D30-6 empirical pins are executed (§5/§11); the ADR-0227 §Context is anchored (§6); the BEHAVIOR_CONTRACT current-pin line set is pinned (§7); the no-other-hardcode confirmation is recorded (§8); the refresh-procedure compliance is mapped (§9).

The next session authors `docs/envoy-go/phases/30-pin-refresh-envoy-contrib/PLAN.md` (`superpowers:writing-plans` — the bite-sized task spine of §10) from this SPEC. ROADMAP row 30 stays `in-progress` through PLAN + IMPL (it flips `in-progress → done` at the IMPL phase-done — no parent rollup; phase 30 is a flat infra row). At this SPEC commit: ROADMAP row 30 stale image-string corrected in place (§1.2); STATE + next-prompt advanced for the PLAN cold-start.
