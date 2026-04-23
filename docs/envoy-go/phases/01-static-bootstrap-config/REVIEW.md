# Phase 01 — Static Bootstrap Config Review

**Reviewer:** superpowers:requesting-code-review (dispatched 2026-04-23)
**Range:** `c6f9d6c..cec4bf7`
**Verdict:** APPROVED

## Summary

Phase 01 delivers what SPEC §1 promises: the phase-00 minimal YAML schema is
retired in favor of a real `envoy.config.bootstrap.v3.Bootstrap` loader under
`internal/bootstrap/` (via the permitted `go-control-plane/envoy` proto-types
module), the first admin-API surface lands as `internal/admin.Server`
serving byte-exact `/ready` equivalent to upstream Envoy v1.37.2, and the
subject binary (`cmd/envoy-go/main.go`) is rewired end-to-end — all without
regressing the phase-00 TCP echo surface. Every one of the 17 PLAN tasks
lands as an atomic commit traceable through `PROGRESS.md` with verbatim
command outputs, and all ten promised ADRs (ADR-0012 through ADR-0021)
appear in `DECISIONS.md` with ADR-0021 explicitly superseding ADR-0007 per
`BOOTSTRAP_PROMPT.md` §4.1 invariant 4. The `## Verification` block at the
tail of `PROGRESS.md` quotes all five phase-done gates and the CI-YAML
validation check verbatim; per the STATE.md review scope I did not re-run
any of those. Reviewer spot-checks (`go build ./...`, file-existence walk,
ADR count grep, supersession grep) were clean: `go build ./...` is exit 0
at `cec4bf7`, `grep -c '^## ADR-' = 21`, and `**Supersedes:** ADR-0007`
appears once at the ADR-0021 entry. No finding rises to Critical or
Important. Five Minors, all deferrable to phase 02+. The phase is safe to
advance to lifecycle-state 6 unconditionally.

## Strengths

- **Bootstrap loader does exactly what SPEC §5.1 says.**
  `internal/bootstrap/bootstrap.go:26-54` is the three-stage
  `yaml.v3 → encoding/json → protojson.Unmarshal` pipeline promised by
  ADR-0012, with `DiscardUnknown: false` (ADR-0016) wired at line 49, the
  `dynamic_resources` / `layered_runtime` rejections guarded at lines
  38-43 *before* any protojson work runs, and every error prefixed with
  `bootstrap: ` per SPEC §5.1. The blank-import of
  `.../filters/network/tcp_proxy/v3` at line 14 is documented in place
  with a comment naming ADR-0016 consequences — a stranger reading the
  file finds the "why" immediately.
- **Admin `/ready` is byte-exact to captured upstream bytes, not to PLAN
  guesses.** `internal/admin/admin.go:68-86` emits exactly the five
  headers and body captured in Task 7's `upstream-ready-observation.md`:
  `Content-Type: text/plain; charset=UTF-8`, `Cache-Control: no-cache,
  max-age=0`, `X-Content-Type-Options: nosniff` (which the PLAN snippet
  *omitted* — the implementer caught this via evidence-vs-snippet
  reconciliation in Task 8), `Server: envoy`, and `LIVE\n` body. The
  reconciliation is itemized verbatim in PROGRESS.md Task 8 as six
  numbered divergences.
- **TDD evidence is strong.** Every task under `internal/bootstrap/`,
  `internal/admin/`, and `test/helpers/` shows a RED compile-fail step
  before the implementation lands; PROGRESS.md Tasks 2, 4, 5, 8, and 11
  all follow the fail-first loop with `undefined: Load`,
  `undefined: AdminSocket`, `undefined: ParseHTTPResponse` errors
  visible in the log before the GREEN step.
- **Phase-00 TCP pump preserved byte-for-byte, per SPEC §5.3.**
  `cmd/envoy-go/main.go:91-119` carries the phase-00 `netConn`, `pump`,
  `halfClose` triple verbatim, with the splice(2) explanatory comment
  intact. The ready sentinel format `envoy-go ready on <addr>\n` is
  unchanged (line 79), so `harness.readyAddr`'s substring parser
  requires no modification — SPEC §5.6's "keep harness-readiness
  orthogonal to the surface under test" gate is respected.
- **Admin address is threaded through `SubjectProxy` by pre-allocation,
  not scraped from stdout.** `test/differential/harness.go:215-224`
  records the pre-allocated `subjAdminAddr` on the proxy struct;
  `runner_test.go:71-74` allocates the port via `freeTCPPort(t)` and
  interpolates it into the subject bootstrap before `StartSubjectProxy`
  runs. This preserves the sentinel's byte-exact format while still
  plumbing the admin address into the differential diff.
- **Fixture `envoy-go.yaml` inline comment block matches README and
  SPEC §5.4 word-for-word.**
  `test/fixtures/0000-tcp-echo/envoy-go.yaml:1-11` enumerates the three
  benign divergences from `envoy.yaml` (STATIC vs STRICT_DNS, no
  `dns_lookup_family`, `127.0.0.1` vs `0.0.0.0` + `host.docker.internal`),
  each linked to its SPEC §5.4 item number. A D-3.4 context-isolation
  win.
- **BEHAVIOR_CONTRACT Admin API subsection lands in the promised
  structural slot.** `docs/envoy-go/BEHAVIOR_CONTRACT.md:69-111` is
  under a new `## Admin API — /ready` H2, positioned between
  `## Timing tolerances` (line 61) and `## Test harness host networking`
  (line 114) — exactly where SPEC §5.5 placed it. The previously empty
  `## Header allow-list` section gains its first canonical table row
  (`date` scoped to `/ready`).
- **ADR-0013 reflects *actual* reality, not the PLAN hint.** PLAN.md
  line 65's "representative: v0.13.x" was the author's guess before the
  module-split observation; Task 1 discovered the
  `github.com/envoyproxy/go-control-plane` →
  `.../envoy` nested-module split and pinned `v1.32.4` on the nested
  module. ADR-0013's Context section (`DECISIONS.md:313-320`) explains
  the split in enough detail that a future reviewer who only reads the
  ADR understands why the parent module is *not* in `go.mod`'s
  direct-require block — that absence is the D-3.2 proto-types-only
  boundary rendered in `go.mod`.
- **ADR-0021's supersession of ADR-0007 is exemplary.** The explicit
  `**Supersedes:** ADR-0007` header appears at `DECISIONS.md:559`;
  ADR-0007 itself is unedited; `grep -c '^## ADR-' = 21` matches the
  expected ADR-0001..ADR-0021 inclusive range; and PROGRESS Task 13
  notes "`git diff --numstat -- docs/envoy-go/DECISIONS.md` showing
  `35 insertions, 0 deletions`" as append-only evidence. The deleted
  `cmd/envoy-go/config.go` and `config_test.go` paths confirmed absent
  via `ls cmd/envoy-go/` → only `main.go` and `main_test.go`.
- **Fuzz target is structurally minimal and forward-compatible.**
  `internal/bootstrap/fuzz_test.go` (per PROGRESS Task 6) seeds only
  `sampleBootstrap`, the reference fixture bytes, and five degenerate
  inputs; the fuzz body asserts only "no panic, no hang", deliberately
  avoiding a prefix-match on `bootstrap: ` that would double-count
  unit-test coverage. The 30-second CI budget (ADR-0018) is declared
  outside the differential job's 5-minute wall-clock so the two lanes
  run in parallel.
- **CI workflow `fuzz-bootstrap` job resolves correctly.**
  `.github/workflows/ci.yml:43-55` declares `needs: lint-vet-test`
  which matches the existing `lint-vet-test` job at line 8. PROGRESS
  verification block confirms `yaml.safe_load(...)` parses the file and
  both `fuzz-bootstrap.needs` and `differential.needs` resolve to the
  `lint-vet-test` job.
- **Every §10 deferred decision settled; no phase-exit loose ends.**
  ADR-0012 / 0013 / 0014 / 0015 / 0016 / 0017 / 0018 / 0019 / 0020 /
  0021 map 1:1 to SPEC §10 items 1–9 plus the ADR-0007 supersession. No
  "resolved-at-implementation-time" choice was swept into PROGRESS prose
  without a matching ADR entry.

## Findings

### Critical (must fix before lifecycle-state 6)

*None.*

### Important (should fix, reviewer's judgment call)

*None.*

### Minor (nice-to-have, can defer)

1. **ADR-0015's Decision block lists `transfer-encoding: chunked` in the
   "admin server MUST emit" bullet set, with the exception deferred to a
   later paragraph.** `docs/envoy-go/DECISIONS.md:481-486`: the MUST list
   names `transfer-encoding: chunked (see framing exception below)` and
   the actual waiver is at line 486 ("*phase-01 subject is permitted to
   emit `Content-Length: 5` instead…*"). The inline
   `(see framing exception below)` parenthetical prevents the
   skim-failure mode (a stranger reading just the bullet list does not
   walk away believing the subject emits chunked), so this is not a
   D-3.4 context-isolation defect — but the composition is still
   awkward: a strict reader asks "is this a MUST or a MAY?", and the
   answer requires two paragraphs. The implementation at
   `internal/admin/admin.go:75-85` emits `Content-Length` (not chunked),
   the BEHAVIOR_CONTRACT subsection at `BEHAVIOR_CONTRACT.md:86`
   records the deviation explicitly, and the runner's allow-list at
   `test/differential/runner_test.go:133-137` drops both
   `Content-Length` and `Transfer-Encoding` from the set-equal check,
   so the gate is self-consistent. Fix (deferrable): when a future ADR
   supersedes ADR-0015 (e.g., when phase-02 wires chunked framing into
   admin), collapse the MUST-list + exception paragraph into a single
   paragraph that states "*upstream emits `transfer-encoding: chunked`;
   phase-01 subject emits `Content-Length: 5`; harness normalises both
   before body comparison*". No action required in phase 01 because the
   behavioural contract is unambiguous once all three artefacts
   (ADR, BEHAVIOR_CONTRACT, runner) are read together — and the
   BEHAVIOR_CONTRACT subsection, which is the authoritative reference
   per D-3.3, is prose-coherent on its own.

2. **ADR numbering remains out of physical order in `DECISIONS.md`, and
   phase 01 worsens it.** Phase 00's REVIEW.md Minor Finding 5 flagged
   `0001..0006, 0008, 0007, 0009, 0010, 0011`. Phase 01 appends
   `0013, 0012, 0016, 0017, 0018, 0015, 0014, 0019, 0020, 0021` — ten
   more entries out of strict increasing order, visible via
   `grep -n '^## ADR-' docs/envoy-go/DECISIONS.md`. PROGRESS Task 11
   acknowledges the inversion ("higher-numbered ADRs — 0015-0018 —
   were authored earlier and live mid-document due to earlier-task
   insertions"). Per `BOOTSTRAP_PROMPT.md` §4.1 invariant 4 the
   *numbering* is the authoritative ordering (not physical position),
   so this is compliant, but it makes log-tail navigation tedious.
   The phase-00 REVIEW recommended "future ADRs keep strict
   chronological-numerical order"; phase 01 did not honor that.
   Recommend phase 02 adopt the mechanical discipline of appending
   ADRs at the file's physical tail with the next sequential number.
   Not a doctrine violation; no action required.

3. **`Transfer-Encoding` may never appear in `HTTPResponse.Headers` as a
   key, making its allow-list entry a no-op for the reference side.**
   `test/helpers/http_response.go:25-46` uses `http.ReadResponse` to
   parse, and Go's `net/http` package moves `Transfer-Encoding` from
   `resp.Header` to the dedicated `resp.TransferEncoding` slice — so
   by the time the parser canonicalises into `hdrs`, `Transfer-Encoding`
   has already been stripped from the map. The allow-list entry
   `"Transfer-Encoding": {}` in `runner_test.go:136` is therefore
   defensive-but-dead for upstream responses (upstream emits
   `transfer-encoding: chunked`; `net/http` promotes it; the key never
   lands in the map; the allow-list never needs to filter it). The
   differential gate passes because both `diffHeaders` iterations
   agree that upstream's post-parsing header set lacks the key. No
   bug today; if a future phase swaps `http.ReadResponse` for a more
   literal parser, the allow-list semantics shift silently. Fix
   (future phase): document this behaviour in a code comment next to
   the allow-list, or test both framing modes in
   `test/helpers/http_response_test.go`.

4. **`Content-Length` on the subject's `/ready` response is always `5`
   (or `17` pre-init); blanket allow-listing it hides a whole category
   of framing regressions once a second admin endpoint appears.**
   `runner_test.go:135` allow-lists `Content-Length` globally. If a
   future phase introduces a second admin endpoint with a
   different-length body (e.g., phase-08 `/server_info`), the same
   allow-list enters the runner by inheritance, and a subject regression
   that emits the wrong `Content-Length` for a length-framed body would
   silently pass at the header check. The body-bytes comparison
   currently catches length mismatches indirectly (a short body with a
   wrong `Content-Length` would fail body-equal first via
   `CompareBytes`), so there is no phase-01 defect. Fix (phase-08):
   scope the allow-list to `/ready` when the second endpoint lands, or
   compute `Content-Length == strconv.Itoa(len(body))` as a derived
   cross-check.

5. **`runFixture` and `compareAdminResponses` carry unused parameters.**
   `test/differential/runner_test.go:46`:
   `func runFixture(t *testing.T, root string, pin *EnvoyPin, _ string, d FixtureDriver)` — the `_ string` is the fixture-name slot
   passed from `TestDifferential/t.Run(fx, …)`.
   `test/differential/runner_test.go:107`:
   `func compareAdminResponses(refRaw, subjRaw []byte, _ fixture.Driver)` — the driver parameter exists so a future
   fixture-specific allow-list can be read from the driver; today
   nothing reads it. Both parameters exist for reasonable future
   callers but today appear as `_` — a reader has to trace them to the
   call sites to find out why. Either use them (fixture name could land
   in a log-line or a hex-dump header; driver could expose a
   per-fixture allow-list per ADR-0019's forward-looking note) or drop
   them to keep the signature honest. Defer to the phase
   (phase 06 / 08 per ADR-0019) that productises dimension-aware diff.

## Axis-by-axis assessment

### Axis 1: PLAN.md fidelity

Walked all 17 tasks against the diff and PROGRESS.md. Every task
produced the expected code/doc and an atomic commit with a matching
PROGRESS.md entry. Deviations from PLAN text were either
PROGRESS-documented (Task 1's `v1.32.4` pin vs PLAN's `v0.13.x` hint;
Task 12's incidental touch of `test/differential/harness_test.go`) or
ADR'd at landing time.

| Task | PLAN intent | Commit(s) | PROGRESS | ADR for deviation | Verdict |
|---|---|---|---|---|---|
| 1 | go-control-plane + protojson deps | `52fbd95` | yes, with version-pin transcript | ADR-0013 | pass |
| 2 | `bootstrap.Load` happy + reject dynamic/runtime | `d98c5fa` | yes, RED+GREEN | ADR-0012, ADR-0016 | pass |
| 3 | Load error-path tests | `f3ad272` | yes (locks behaviour) | — | pass |
| 4 | `AdminSocket` extractor | `0176329` | yes | — | pass |
| 5 | `FirstListenerSocket` + `FirstClusterEndpointSocket` | `399a1b9` | yes | ADR-0017 | pass |
| 6 | `FuzzBootstrapLoad` + CI | `5c81c56` | yes, fuzz engine execs quoted | ADR-0018 | pass |
| 7 | empirical upstream `/ready` observation | `90957e1` | yes, pre-init-unobservable declaration | ADR-0015 | pass |
| 8 | `admin.Server` + /ready ready-state | `cb6bed3` | yes, six evidence-vs-PLAN divergences itemised | ADR-0014 | pass |
| 9 | admin pre-init + atomicity + Close idempotent | `c2cb3fb` | yes (`freeAddr` helper deleted) | — | pass |
| 10 | BEHAVIOR_CONTRACT Admin API subsection | `0979230` | yes, H2 placement confirmed | — | pass |
| 11 | `test/helpers/http_response.go` | `3a2218b` | yes, RED+GREEN | ADR-0019 | pass |
| 12 | `main.go` + fixture cutover | `08e09a9` | yes, `harness_test.go` extra-touch disclosed | ADR-0020 | pass |
| 13 | delete phase-00 `config.go`/`config_test.go` | `739b1ba` | yes, ADR-0007 append-only verified | ADR-0021 | pass |
| 14 | admin probe wired through runner | `0c17076` | yes, zero reconciliation iterations | — | pass |
| 15 | fixture `expectations.yaml` extended | `b1e086b` | yes | — | pass |
| 16 | fixture README refresh | `f4dca1b` | yes | — | pass |
| 17 | green local gate sweep | `f43f66f` | yes, five-gate transcripts | — | pass |

No task silently skipped. No scope-creep introduced.

### Axis 2: SPEC.md §12 acceptance checklist

| Item | Status | Evidence |
|---|---|---|
| All §4.1 paths exist with described contents | PASS | `internal/bootstrap/{bootstrap.go,bootstrap_test.go,fuzz_test.go}`, `internal/admin/{admin.go,admin_test.go}`, `phases/01-static-bootstrap-config/{SPEC,PLAN,PROGRESS,upstream-ready-observation}.md` all present; `ls` confirmed. |
| All §4.2 files reflect modifications | PASS | `cmd/envoy-go/main.go` (bootstrap-based), `cmd/envoy-go/main_test.go` (bootstrap-shaped YAML), `test/differential/harness.go` (SubjectProxy.adminAddr + AdminAddr()), `test/fixtures/0000-tcp-echo/{envoy-go.yaml,expectations.yaml,README.md,driver/driver.go}` (rewritten per §5.4), BEHAVIOR_CONTRACT admin subsection, DECISIONS ten new ADRs, go.mod/go.sum updated — all land in `c6f9d6c..cec4bf7`. |
| All §4.3 files are deleted | PASS | `cmd/envoy-go/` contains only `main.go` and `main_test.go`; `git log cmd/envoy-go/config.go` returns "unknown revision or path not in the working tree." Task 13 commit `739b1ba` performs the `git rm`. |
| `internal/bootstrap` parses fixture `envoy-go.yaml` + `envoy.yaml` | PASS | `FuzzBootstrapLoad` seeds include reference `envoy.yaml` bytes (PROGRESS Task 6); `TestLoad_HappyPath` parses the `sampleBootstrap` mirroring the fixture shape; all three extractors return valid `host:port` tuples. |
| `internal/bootstrap` rejects §8 error inputs with `bootstrap: ` prefix | PASS | `bootstrap_test.go` covers: missing admin, missing listener, missing cluster, empty endpoints, dynamic_resources, layered_runtime, YAML syntax error, unknown top-level field, empty document, zero listeners, two listeners. 13 tests, all assertions include the `bootstrap: ` prefix check. |
| `internal/admin` serves `/ready` byte-exact to §5.2 / BEHAVIOR_CONTRACT | PASS | `handleReady` at `admin.go:68-86` emits the five-header set documented in `upstream-ready-observation.md` + `LIVE\n` body; Task 8 PROGRESS enumerates six divergences-from-PLAN reconciled to upstream evidence. Framing deviation (length vs chunked) is documented (see Minor 1). |
| `cmd/envoy-go` starts from fixture `envoy-go.yaml` in the right order | PASS | `main.go:24-79` — open config → `bootstrap.Load` → extract three tuples → `admin.New` → `admin.Start` → `net.Listen` → `MarkReady` → print ready sentinel. Matches SPEC §5.2 lifecycle items 1–6. |
| Fixture `0000-tcp-echo` green under extended `expectations.yaml` | PASS | PROGRESS `## Verification`: `--- PASS: TestDifferential/0000-tcp-echo (1.16s)` under `-count=1` after `go clean -testcache`. |
| Phase-00 TCP echo surface remains green (gate b) | PASS | Same differential invocation; TCP byte-exact at `runner_test.go:85-91` unchanged semantically from phase 00. |
| `go vet`, `golangci-lint`, `go test ./...`, `go test ./test/differential/...` green | PASS (per PROGRESS `## Verification`) | All four exit 0 under `go clean -testcache`. Reviewer sanity re-ran `go build ./...` (clean exit) — not a heavy-gate re-run, just a dependency-resolution check. |
| `FuzzBootstrapLoad` runs clean at ADR-0018 budget | PASS | `ok …/internal/bootstrap 31.075s` with final `PASS`; 1,446,175 execs across 32 workers; zero crashes. |
| `BEHAVIOR_CONTRACT.md` populated Admin API subsection with justifying ADR | PASS | `BEHAVIOR_CONTRACT.md:69-111` cites ADR-0014 + ADR-0015; subsection is substantive, not a `_to be filled_` placeholder. |
| ADRs for every settled §10 decision | PASS | §10 items 1→ADR-0012; 2→ADR-0013; 3→ADR-0014; 4→ADR-0015; 5→ADR-0016; 6→ADR-0017; 7→ADR-0018; 8→ADR-0019; 9→ADR-0020; plus ADR-0021 supersession of ADR-0007. All 10 appear in `DECISIONS.md` (grep-verified). |
| `STATE.md` advances + `ROADMAP.md` row 01 → done | PARTIAL (expected, per lifecycle) | `STATE.md` is at `lifecycle-state: 5` (this review satisfies it); `ROADMAP.md` row 01 still shows `in-progress`. Per SPEC §3 and the state machine, the advance to `done` is the next-session action *after* this REVIEW lands approved — same treatment phase-00's REVIEW applied (see its row 7). |
| `PROGRESS.md` full log + gates verbatim | PASS | 17 task entries, each with `**Commits:**`, `**Notes:**`, `**Outputs:**`. The final `## Verification` section quotes all five phase-done gates plus the CI-YAML validation check. |
| `REVIEW.md` approved | IN FLIGHT | This document. |

### Axis 3: Doctrine D-3.1–D-3.7

| Doctrine | Status | Evidence |
|---|---|---|
| D-3.1 Superpowers-first process | PASS | Brainstorming seeded SPEC.md; writing-plans produced PLAN.md; TDD evidence in PROGRESS task-by-task (RED compile failures in Tasks 2/4/5/8/11); verification-before-completion section at PROGRESS tail; this REVIEW.md is the requesting-code-review output. Unexpected-state events (module split in Task 1; pre-init unobservable in Task 7; stdlib `http.Server.Close` idempotency in Task 9) resolved via systematic-debugging + (where cross-phase) ADR. |
| D-3.2 Hybrid implementation stance | PASS | Permitted foundations only: `go-control-plane/envoy` (proto types only — grep-verified: `internal/bootstrap/bootstrap.go` imports `envoy/config/bootstrap/v3` + blank-imports `envoy/extensions/filters/network/tcp_proxy/v3`, no `pkg/...` helpers), `google.golang.org/protobuf/encoding/protojson`, `gopkg.in/yaml.v3`, `net/http` (admin server), `testing.F` (fuzz), stdlib. No `httputil.ReverseProxy`, no Traefik/Caddy/fasthttp, no cgo, no GPL. ADR-0013 makes the "parent module not imported" boundary legible in `go.mod`. |
| D-3.3 Differential correctness beats internal fidelity | PASS | Fixture `0000-tcp-echo` byte-compares upstream Envoy v1.37.2 against the envoy-go subprocess on *two* independent observations: TCP echo bytes (phase-00 surface, unchanged assertion) and admin `/ready` (new surface — status exact, body byte-exact, headers set-equal modulo the BEHAVIOR_CONTRACT allow-list). `runner_test.go:81-104` wires both observations; reviewer verified no mocks. |
| D-3.4 Context isolation | PASS | SPEC, PLAN, PROGRESS, ADRs are stranger-readable. ADR-0015's composition is slightly awkward (Minor 1) but the inline `(see framing exception below)` forward-reference keeps a stranger reader from taking the MUST list at face value, and the authoritative BEHAVIOR_CONTRACT subsection is prose-coherent on its own. Phase-00's Finding 1 (ADR promising a BEHAVIOR_CONTRACT update that never landed) has no analogue here — every ADR that promises a cross-file note has the note actually in place: ADR-0014 and ADR-0015's `BEHAVIOR_CONTRACT.md` Admin API subsection is present at `BEHAVIOR_CONTRACT.md:69-111`; ADR-0019's `test/helpers/http_response.go` is present; ADR-0021's `cmd/envoy-go/config.go` deletion is effected. |
| D-3.5 Decisions are written, not remembered | PASS | Ten new ADRs (0012–0021) in-session. ADR append-only discipline maintained — no landed ADR edited in place (Task 13 PROGRESS verifies `git diff --numstat` for `DECISIONS.md` shows append-only deltas). ADR-0021 explicitly supersedes ADR-0007 via `**Supersedes:** ADR-0007` per §4.1 invariant 4; grep confirms the header appears exactly once. |
| D-3.6 Every phase is a green build | PASS | All five phase-done gates exit 0 at the verification head per PROGRESS `## Verification`. Reviewer sanity check: `go build ./...` clean at `cec4bf7`. |
| D-3.7 Version pinning | PASS | Envoy: tag `v1.37.2` + SHA256 (unchanged from phase 00). Go: `go 1.23.0` in `go.mod` (unchanged). golangci-lint: `v1.64.8` (unchanged). NEW pins: `github.com/envoyproxy/go-control-plane/envoy@v1.32.4` (ADR-0013), `google.golang.org/protobuf@v1.36.11` (promoted indirect→direct, no standalone ADR per ADR-0013 rationale — `protojson` is canonical and version is not behaviour-salient at this granularity). ADR-0013 describes the refresh procedure mirroring ADR-0008. |

### Axis 4: Phase-done gate (SPEC §3 / BOOTSTRAP_PROMPT §7.5)

Per STATE.md review scope (iv), the heavy gates were NOT re-run; the
`## Verification` block at `PROGRESS.md:828-962` is cited as evidence.
Gate-by-gate, verbatim from that block:

| Gate | Verbatim PROGRESS evidence | Verdict |
|---|---|---|
| (a) new/changed differential fixtures green | `--- PASS: TestDifferential/0000-tcp-echo (1.16s)` under `-count=1` after `go clean -testcache` | PASS |
| (b) pre-existing differential fixtures green | Same invocation; TCP echo byte-exact assertion in `runner_test.go:85-91` still green | PASS |
| (c) conformance threshold | PASS vacuously — SPEC §3 declares threshold 0 for phase 01 | PASS |
| (d) new fuzzer clean short-budget | `ok …/internal/bootstrap 31.075s` with final `PASS`; 1,446,175 execs, zero crashes | PASS |
| (e) vet + lint + test clean | `go vet ./...` exit 0 empty; `golangci-lint run ./...` exit 0 empty; `go test ./... -timeout 10m` all `ok`, zero `FAIL` | PASS |
| (f) REVIEW.md approved | this document | IN FLIGHT → PASS on commit |

Cheap reviewer sanity checks (not a re-run of the heavy gates):

- `go build ./...` at `cec4bf7`: exit 0, no output.
- File-existence walk over SPEC §4.1 / §4.2 / §4.3 paths: every created
  file present; both deleted files absent.
- ADR presence grep: `grep -c '^## ADR-' docs/envoy-go/DECISIONS.md`
  returns `21`, matching the expected ADR-0001..ADR-0021 range.
- Supersession grep: `**Supersedes:** ADR-0007` appears exactly once,
  at ADR-0021's `DECISIONS.md:559`. ADR-0007 itself is present at
  `DECISIONS.md:193` unedited (`Status: Accepted`).
- CI-YAML validation cross-checked: `.github/workflows/ci.yml:43-55`
  declares `fuzz-bootstrap` with `needs: lint-vet-test`; `yaml.safe_load`
  confirms the file parses and the dependency resolves.

## Recommendations (non-blocking)

- Phase 02's writing-plans session: mirror this phase's reality-vs-PLAN
  reconciliation discipline (Task 8 PROGRESS's six divergences,
  Task 12's `harness_test.go`-not-in-PLAN disclosure, Task 1's module-
  split pin correction). The discipline made this review tractable and
  should be the project's new baseline.
- When a future ADR supersedes ADR-0015, collapse the chunked-vs-length
  discussion into the Decision block (Minor 1). Future ADRs documenting
  a permitted divergence between subject and upstream should state it in
  the Decision, not in a downstream "Framing exception" footnote.
- The ADR-number-vs-file-order drift (Minor 2) is cumulative across two
  phases. Strict append-in-increasing-number order is doctrine invariant
  4 in spirit; recommend phase 02 adopt the simplest mechanical rule —
  append ADRs at the file's physical tail, assign the next sequential
  number, do not re-insert between existing entries.
- When phase 06/08 productises dimension-aware diff (per ADR-0019's
  forward-looking note), revisit the allow-list in
  `test/differential/runner_test.go` so `Content-Length` /
  `Transfer-Encoding` (Minors 3, 4) become per-endpoint scoped rather
  than globally waived. Likely co-lands with the `expectations.yaml`
  consumer.
- The unused `_ string` / `_ fixture.Driver` parameters (Minor 5) are
  candidates for a small tidy-up in whichever phase first needs to log
  a fixture name in the diff hex-dump header or read a driver-owned
  allow-list. Not worth a standalone cleanup commit in phase 02.

## Approval line

I approve phase 01 for advancement to lifecycle-state 6 unconditionally.
The five Minor findings are deferrable to phase 02+; none affects
correctness of the differential gate, the byte-exactness of the admin
`/ready` contract, or the append-only integrity of the ADR log. The
phase's exit criteria (SPEC §3 gates a–e green at the verification head;
BEHAVIOR_CONTRACT Admin API subsection populated; ADR-0007 superseded
by ADR-0021 with an explicit naming header; all §10 deferred decisions
settled as ADRs) are all met.

**Verdict:** APPROVED.
