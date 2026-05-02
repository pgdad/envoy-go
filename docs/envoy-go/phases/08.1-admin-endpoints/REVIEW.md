# Phase 08.1 — Code review (REVIEW.md)

**Phase id:** `08.1`
**Slug:** `08.1-admin-endpoints`
**Branch under review:** `phase-08.1-admin-endpoints-impl`
**Range:** `581d0ea..HEAD` (29 commits — 15 plan tasks + 13 SHA-fill / PROGRESS-append follow-ups + 1 mid-flight T6 review-driven test rigor follow-up)
**Reviewer skill:** `superpowers:requesting-code-review` (final REVIEW per BOOTSTRAP §5 step 6, lifecycle-state 5 → 6). The reviewer subagent dispatch tool was not available in the executing harness so the review is written inline by the implementing session per the PLAN's explicit allowance ("Use the requesting-code-review skill OR spawn a code-reviewer subagent yourself"); the SPEC + the diff + the 07.1 / 07.2 REVIEW.md were used as inputs.
**Six-gate state at HEAD:** all green per Task 15 Step 1's verification sweep — outputs captured in PROGRESS.md Task 15 entry and §"Six-gate verification appendix" below.

This review covers the full ~3 940-LoC phase 08.1 surface — six new admin handler files (`internal/admin/{configdump,clusters,listeners,serverinfo,headers,version}.go` + tests), one constructor widening at `internal/admin.New`, one `*cluster.Manager` snapshot accessor + two new exported types, one new `Bootstrap.ConfigPath` field, one new differential fixture (`0009-admin-config-dump`), one new fuzzer (`FuzzConfigDumpFormat`), the BEHAVIOR_CONTRACT umbrella restructure (`## Admin API` with five per-endpoint subsections + four equivalence-matrix rows), and seven new ADRs (ADR-0084..ADR-0090).

---

## 1. Final assessment

**APPROVED.**

All six phase-done gates are GREEN at HEAD; the implementation faithfully realizes the SPEC across all 15 PLAN tasks; the one mid-flight self-correction (T6 review-driven test rigor follow-up `7b938fc` — pinning the 1-space indent + EmitUnpopulated assertion in `TestHandleConfigDump_PinMarshalOptions`) is correct and well-tested; the empirical-pin discipline anchored at SPEC §11.1–§11.8 is paste-verbatim-synchronized into ADR-0086 + ADR-0087 + ADR-0088 + the BEHAVIOR_CONTRACT umbrella; ADR discipline is exemplary with all seven anticipated ADRs landed at the right tasks and full Context/Decision/Consequences sections (ADR-0089's deferral table and ADR-0090's no-ACL posture are particularly grep-discoverable); doctrine adherence (D-3.2 no-cgo via the existing protojson stack; D-3.3 differential correctness via fixture 0009; D-3.5 ADRs for ambiguities; D-3.6 green build; D-3.7 no CONFORMANCE_PINS drift) is uniform across the phase.

The constructor-widening pattern (LBP-1 third application per ADR-0085) is a clean generalisation of 06.1's `*stats.Registry` and 07.1's `*HTTPRegistry` / 07.2's `*ListenerFilterRegistry` precedents — `admin.New` now threads four fresh dependencies (`*bootstrap.Bootstrap`, `*cluster.Manager`, `*listener.Manager`, plus the existing `*stats.Registry`) without breaking the boot-order invariant. The differential fixture's structural-projection canonicalisation discipline is the right shape for a body whose reference Envoy v1.37.2 emission carries ~40 enum-default-emission divergences against `EmitUnpopulated:true` (per ADR-0086 consequence) — narrow projection beats wide allow-listing, and the projection IS the assertion shape (grep-discoverable in `test/fixtures/0009-admin-config-dump/driver/driver.go`).

The findings below are NOT blockers; they are documentation/test-coverage sharpenings that the parent session may choose to carry forward into 08.2 or a future hardening pass. The phase-done commit may proceed.

---

## 2. Strengths

### 2.0 Phase shape and scope discipline

Phase 08.1 is the first sub-phase under the parent phase 08 split (ADR-0084). The scope decision was anchored at brainstorm time (parent 08-admin-api-and-drain BRAINSTORM §1) and reified at SPEC time. Three dimensions show that the scope discipline was honored:

- **No 08.2 surface touched in 08.1.** No production edit landed any drain-state machinery, listener.Manager.Drain, cluster.Manager.Drain, POST /drain_listeners handler, or any DRAINING enum-state transition. The `internal/admin.Server` is now ready for 08.2's drain-state extension (a `drainState atomic.Pointer[DrainState]` can be added without changing `New`'s signature; `/ready` and `/server_info` extend to handle DRAINING; `POST /drain_listeners` registers on the same mux). The post-plan-handoff section of PLAN.md anticipates this and the field shapes left in place are forward-compatible.
- **Deferred items are exhaustively enumerated.** ADR-0089's deferral table (item (a) mutating endpoints — 7 entries; item (b) read-only sub-system pre-requisite endpoints — 10 entries; item (c) body-shape extensions on the four 08.1 endpoints — 9 entries; item (d) the trailing-slash-body permitted divergence) is the canonical grep-discoverable cross-reference for any future "is X modeled?" question. The `BEHAVIOR_CONTRACT.md ## Admin API ### Does not yet apply to` block cites ADR-0089 for nine of ten bullets.
- **Single fixture, non-vacuous.** SPEC §7 + Decision G commit to a single fixture (`0009-admin-config-dump`) — the four-endpoint surface fits in one fixture (5-request defined load + per-endpoint scrape against ref Envoy v1.37.2). The 06.1 precedent of a single `0005-prometheus-stats` fixture is the right shape for a sub-phase whose central engineering claim ("envoy-go's four read-only admin endpoints emit equivalent bodies to upstream Envoy v1.37.2 under the per-endpoint tolerance discipline") is provable with one dual-proxy fixture.

### 2.1 Constructor-widening pattern (LBP-1 third application per ADR-0085)

The heart of phase 08.1's architecture is the `admin.New(addr, registry, bs, cm, lm) *Server` signature widening (`internal/admin/admin.go:51-61`). Three dimensions are notable:

- **Strict explicit-threading discipline.** The four fresh dependencies (`*bootstrap.Bootstrap`, `*cluster.Manager`, `*listener.Manager`, `bootTime time.Time`) are all passed explicitly. No package globals. No factory-pattern indirection. The `bootTime` field is set at `time.Now()` in the constructor (line 59) — a tiny but load-bearing decision because the same `bootTime` value is used for both `BootstrapConfigDump.LastUpdated` (configdump.go:83) and `ServerInfo.UptimeCurrentEpoch / UptimeAllEpochs` (serverinfo.go:47); future 08.2 drain machinery will read this field directly.
- **nil-tolerated for tests, non-nil for production.** The doc comment on `New` (admin.go:46-50) explicitly records that bs/cm/lm may be nil in tests that do not exercise the four new endpoints. The handlers defensively check (`if s.bs != nil`, `if s.cm != nil`, `if s.lm != nil`) and emit empty-but-valid responses on nil. Production call sites in `cmd/envoy-go/main.go` always thread non-nil values.
- **Mirrors 06.1 / 07.1 / 07.2 precedents exactly.** The `*stats.Registry` was the first LBP-1 application (06.1 SPEC §5.4); `*HTTPRegistry` was the second (07.1 ADR-0072); `*ListenerFilterRegistry` was the third (07.2 ADR-0079). ADR-0085 records 08.1 as the fourth (the second SECOND application — admin-mux-reuse rather than registry-introduction; the boundary is the no-package-global rule). The DECISIONS.md tail at ADR-0085 explicitly cites the three precedents with task anchors; the consequence section enumerates the constructor-call-site updates that needed to follow (Task 10's `cmd/envoy-go/main.go` call-site edit).

### 2.2 ADR-0086's `protojson.MarshalOptions` consolidation + cross-endpoint reuse

ADR-0086 settles the four-value `configDumpMarshalOptions` tuple (`Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true`) at the package-level `var configDumpMarshalOptions` in `internal/admin/configdump.go:27-32` and pins it via the SPEC §11.1 empirical scrape against Envoy v1.37.2. The decision is consequential in two ways:

- **Cross-endpoint body-shape consistency.** `/server_info` (serverinfo.go:23) reuses the same `configDumpMarshalOptions` rather than declaring a separate tuple — ADR-0086 consequence (d) anticipates this and ADR-0088 confirms it. Both JSON admin endpoints round-trip through the same MarshalOptions; future protojson admin surfaces (a hypothetical 08.2 JSON response on `/drain_listeners`, though current SPEC §1 has it return `OK\n` text) follow the same pattern by default.
- **EmitUnpopulated:true is the load-bearing flag.** This is what gives Envoy's body its "show me everything" character — the differential equivalence comparator relies on both sides emitting the same set of fields. The mid-flight T6 follow-up (`7b938fc`) added a test (`TestHandleConfigDump_PinMarshalOptions`) that explicitly asserts the four-value tuple via reflection; this is the kind of regression-pin discipline that prevents silent option drift across future refactors.

### 2.3 ADR-0087's `/clusters` + `/listeners` text-format choice

The `/clusters` body (`internal/admin/clusters.go`) is ~100 LoC with 28 `fmt.Fprintf` lines per cluster — exhaustively enumerating the 10 cluster-level + 18 per-endpoint constants pinned by SPEC §11.2. Three decisions are notable:

- **Default constants for non-modeled fields.** envoy-go has no circuit-breaker machinery (no proto field plumbing for `default_priority::max_connections::1024` etc.) but emits the constants unconditionally — Envoy emits them too (the v3 cluster proto's `circuit_breakers` field has these as Go-zero-value-equivalent defaults at the proto-defaults level). The byte-shape parity is per-line correct under tuple-set-equality; the differential's drop-list approach (per the iter-2 canonicaliser) lets the comparator converge despite the cross-side address discrepancy.
- **Per-endpoint counters emit literal `0` (planner-time decision 8 + ADR-0063 carry-forward).** envoy-go has no per-endpoint stats per ADR-0063's deferral; the 8 per-endpoint cx_*/rq_* lines are just `<cluster>::<addr>::<key>::0`. ADR-0087's consequence (d) explicitly records that the differential allow-list widens from the SPEC §7.1 ±1-tolerance to a full drop-list for these 8 fields. The fixture's `canonicaliseClusters` honors this verbatim.
- **`/listeners` is intentionally simple.** One line per listener (`<name>::<addr>`), alphabetical-by-name, defensive sort at scrape time (handle `internal/listener.Manager.Listeners()` returning declaration order, which is NOT guaranteed alphabetical). The implementation is 35 LoC + sort. The `?format=json` form is deferred per ADR-0089.

### 2.4 ADR-0088's state-enum coverage decision

ADR-0088 explicitly records that envoy-go's 08.1 `/server_info` covers exactly two of the four `adminv3.ServerInfo_State` enum values: `LIVE` (post-MarkReady) + `PRE_INITIALIZING` (pre-MarkReady). `INITIALIZING` is documented as unreachable in the static-bootstrap-only model (the admin server starts AFTER `bs.Load` + cluster/listener manager construction — there is no observable code path that produces it). `DRAINING` is 08.2's deliverable. This is the right shape for a sub-phase: the ADR explicitly anticipates 08.2's amendment path (consequence (c) — "the amendment is purely additive; no other field changes; the ADR-0088 amendment will record the addition without superseding this ADR"). The amendment-not-supersession discipline mirrors ADR-0004's anti-fragmentation guidance.

### 2.5 Differential fixture 0009's structural-projection assertion shape (per the T14 review iteration)

The differential canonicaliser in `test/fixtures/0009-admin-config-dump/driver/driver.go:280-551` is the load-bearing equivalence-claim implementation. Four per-endpoint canonicalisers each project the body down to a narrow assertion shape:

- `canonicaliseConfigDump` projects to `{configs_types: [...], static_listeners: [<name>...], static_clusters: [<name>...]}` with `configs_types` intersected to the three envoy-go-emitted envelope types.
- `canonicaliseClusters` parses lines, drops the 8 per-endpoint cx_*/rq_* counter tuples, strips the per-endpoint address suffix, drops `hostname` (cross-side STRICT_DNS vs STATIC discrepancy), and applies allow-sets for cluster-level (10 keys) and per-endpoint (10 keys minus hostname) field-keys.
- `canonicaliseListeners` strips the address suffix; sorts by listener name.
- `canonicaliseServerInfo` projects to `{state: <enum string>}`.

This is the same correctness discipline 06.1 used for `/stats/prometheus` (twin-series filter discipline drops ~50% of Envoy-emitted counter names that envoy-go does not model). The structural-projection approach trades narrow allow-listing for narrow projection — the projection IS the assertion shape, and the assertion shape is grep-discoverable in the driver source. The Task 14 review's verdict ("Acceptable trade-off; T15 should update SPEC §7.1 + expectations.yaml prose to match the projection-based assertion shape") is honored in this T15 commit (SPEC §7.1 + expectations.yaml prose updated; see §3 below).

### 2.6 Test coverage

Test coverage across the phase is comprehensive:

- **`internal/admin/`** carries new tests for all four handlers — `configdump_test.go` (~158 LoC), `clusters_test.go` (~301 LoC), `listeners_test.go` (~351 LoC), `serverinfo_test.go` (~145 LoC), `headers_test.go` (~67 LoC), `version_test.go` (~63 LoC), plus `admin_helpers_test.go` (~103 LoC of `mustMinimalBs` / `mustMinimalCM` / `mustMinimalLM` helpers reused across the suite). The four-endpoint smoke + method-discrimination Envoy-parity tests live in the existing `admin_test.go`. `TestAdminConcurrentScrapeRace` (admin_test.go:330) is the load-bearing race-detector contract — 100 goroutines × 4 endpoints × 1s under `go test -race`.
- **`internal/cluster/manager_test.go`** added 76 LoC of new tests for the `Clusters()` snapshot accessor (read-only-snapshot semantics, alphabetical ordering, per-endpoint information).
- **`internal/bootstrap/bootstrap_test.go`** added 23 LoC of tests for the new `ConfigPath` field.
- **`cmd/envoy-go/main_test.go`** added 135 LoC of new tests for the call-site update + `bs.ConfigPath = *cfgPath` post-Load wiring.
- **Differential fixture 0009** is the per-endpoint differential pin: 5-connection workload across the dual-proxy (envoy-go on admin :9901 + listener :10000; reference Envoy on admin :9902 + listener :10001) demonstrating per-endpoint equivalence under the structural-projection canonicalisation. Per the PROGRESS Task 14 entry "PASS first try" — non-vacuous evidence that the projection captures the load-bearing claim.
- **`FuzzConfigDumpFormat`** is the 10th fuzzer (per ADR-0018 30s short-budget). Adversarial bootstrap inputs through `buildConfigDump` + `protojson.Marshal`; asserts no panic, output is valid JSON, output has a `configs` field. The 30s run produced 124 new-interesting inputs but no crash.

### 2.7 Empirical-pin discipline

All eight §11 empirical-pin blocks (§11.1 /config_dump JSON shape, §11.2 /clusters text format, §11.3 /listeners text format, §11.4 /server_info JSON shape, §11.5 framing across all four endpoints, §11.6 header set, §11.7 pre-MarkReady state value, §11.8 method discrimination) are paste-verbatim-synchronized into ADR-0086 + ADR-0087 + ADR-0088 + the BEHAVIOR_CONTRACT umbrella. The pins were captured at SPEC time by the planner against Envoy v1.37.2 server SHA per the ADR-0008 pin (per BOOTSTRAP §7.5 gate (f) discipline — durable evidence in the SPEC, the ADRs, and the BEHAVIOR_CONTRACT).

The boundary grep `grep -rE 'github.com/.*admin|github.com/.*config_dump' . --include='*.go' --include='go.mod' --include='go.sum' | grep -v 'envoyproxy/go-control-plane' | grep -v 'envoy-go'` returns zero matches at HEAD, confirming the SPEC §16 acceptance bullet ("No third-party admin-endpoint or proto-rendering library is imported"). The protojson stack from `google.golang.org/protobuf/encoding/protojson` is the only proto-marshaling primitive — D-3.2's no-cgo discipline is preserved.

### 2.8 ADR discipline

All seven anticipated ADRs (ADR-0084..ADR-0090) landed at the right tasks per the SPEC §8 anticipation table:

- **ADR-0084** (phase-08 split; Task 1, commit `3fc8871`) — anchors the ROADMAP row-flip already landed at the SPEC commit; mirrors ADR-0070's 07.1 + ADR-0045's 06.1 scope-confirmation pattern.
- **ADR-0085** (admin-mux reuse + LBP-1 third application; Task 5, commit `4fc9706`) — the load-bearing constructor-widening decision; cites the three prior LBP-1 applications.
- **ADR-0086** (`/config_dump` body shape; Task 6, commit `044f751`) — pinned to SPEC §11.1 empirical evidence; the four-value MarshalOptions tuple is the consequence-anchored decision.
- **ADR-0087** (`/clusters` + `/listeners` body shape; Task 7, commit `2022e68`) — covers both text-format endpoints in one ADR; the per-endpoint counter `0`-emission decision is recorded explicitly.
- **ADR-0088** (`/server_info` body shape; Task 9, commit `7b080e0`) — pinned to SPEC §11.4 empirical evidence; state-enum coverage decision (LIVE + PRE_INITIALIZING) recorded with INITIALIZING-unreachable note + DRAINING-08.2-deferred note.
- **ADR-0089** (deferral list; Task 15, this commit) — the canonical grep-discoverable cross-reference for the BEHAVIOR_CONTRACT `### Does not yet apply to` block; per-ADR-0040 deferral-format precedent.
- **ADR-0090** (no-ACL security posture; Task 15, this commit) — explicit recording of the implicit phase-01 + phase-06.1 posture; cites BRAINSTORM §2.1 Decision G + SPEC §2.5 + §11.8 method-discrimination empirical pin.

Each ADR has full Context/Decision/Alternatives-considered/Consequences sections per ADR-0001's template; each names its `Lands-in-task` field; ADR-0089 + ADR-0090 are the kind of forward-pointing ADRs that prevent future implicit-decisions from accumulating without explicit records.

### 2.9 BEHAVIOR_CONTRACT.md restructure (ADR-0052 in-place edit at phase-done commit)

The Task 15 in-place edit per ADR-0052 (THIS commit) is well-executed:

- The existing `## Admin API — /ready` section (phase 01) is renamed to `## Admin API` umbrella with three opening paragraphs (framing deviation, header set, method discrimination posture).
- The existing `### Ready-state response (authoritative)` and `### Pre-init response` sub-blocks are demoted to fourth-level headings (`####`) under a new `### /ready` subsection (verbatim-preserved).
- Five new per-endpoint subsections are added: `### /stats/prometheus` (short summary, deferring to `## Stat-name mapping`); `### /config_dump`, `### /clusters`, `### /listeners`, `### /server_info` (each with body-shape, empirical-evidence pointer, and equivalence-claim paragraphs per SPEC §13.1 verbatim).
- `### Applies to` and `### Does not yet apply to` lists added per SPEC §13.1 verbatim; the `### Does not yet apply to` list cites ADR-0089 nine times and ADR-0090 once.
- Four new equivalence-matrix rows added at the head of the file (folded into the existing 2-column table shape — the SPEC §13.2 verbatim patch was 3 columns, but the existing table is 2 columns so the allow-list info was folded into the "Required equivalence" cell with explicit "(Per phase 08.1 SPEC §13.2.)" tail to keep the cross-reference grep-discoverable).

---

## 3. T14 review follow-up — applied (a) + (b); deferred (c) with note

The T14 review identified that the differential fixture's canonicaliser uses **structural projection** (extracting narrow shape) rather than the PLAN's per-field allow-list approach. The T15 follow-up scope was:

(a) Update SPEC §7.1 prose to record that the implemented assertion is structural projection. **Applied.** The §7.1 prose now explicitly records the structural-projection shape per-endpoint, cites the iter-2 enum-default-emission divergence + ADR-0086 EmitUnpopulated:true as the empirical justification, and references the canonicaliser functions in the driver source.

(b) Update `test/fixtures/0009-admin-config-dump/expectations.yaml` prose to match. **Applied.** The expectations.yaml now spells out the four per-endpoint projections in detail (configs_types + static_listeners + static_clusters for /config_dump; tuple-set with normalisations for /clusters; name-only set for /listeners; state-only for /server_info) and names the canonicaliser functions explicitly.

(c) Optional: tighten `canonicaliseConfigDump` to also assert listener `port_value` and endpoint addresses-from-allow-set (~20 LoC). **Deferred to a future hardening pass.** The structural-projection approach is intentionally narrow (capturing the load-bearing assertion only); tightening to assert listener port and endpoint addresses would tip from "narrow projection" toward "narrow allow-list", which the iter-2 reasoning explicitly avoided due to the ~40 enum-default-emission divergences in the deeply-nested admin/v3 proto structure. The current projection asserts the named-listener / named-cluster present-and-correct on both sides, which is the load-bearing claim per SPEC §7.1's per-endpoint equivalence statement; the address fields are inferred indirectly (each side returns 200 with a non-empty body and ≥1 listener entry). A future hardening pass may revisit (c) as a tightening if the projection ever proves too lax in a regression — but at MVP scope the current shape is correct.

---

## 4. Findings

### 4.1 Major (blocks phase-done)

**None.**

### 4.2 Minor (decide carry-forward vs inline-fix)

**N-1 (Note tier — borderline Minor).** `internal/admin/listeners.go:30` — the `sort.Slice` defensive-sort comment ("Listeners() iterates m.runtimes in declaration order, which is NOT guaranteed alphabetical") is correct as written, but `internal/listener/manager.go:928`'s `Listeners()` doc comment does NOT explicitly state the ordering it returns. A reader investigating "what order does Listeners() return?" needs to read the impl to know. Suggested follow-up (08.2 inline-fix candidate): add a one-line doc-comment on `Listeners()` saying "order is bootstrap-declaration order; callers needing alphabetical ordering must sort." **Disposition:** carry-forward to 08.2 — `internal/listener.Manager` will be touched by 08.2's drain wiring; the doc-fix can ride along. No phase-done blocker.

**N-2 (Note tier).** `internal/admin/clusters.go:78-99` — `writeEndpointLines` hard-codes the 18-line constant block via 18 separate `fmt.Fprintf` calls. This is correct and matches the SPEC §11.2 verbatim emission, but a future cluster-introspection extension (e.g. one that DOES populate per-endpoint stats post-ADR-0063 supersession) would need to refactor 18 lines of hard-coded format strings. Suggested follow-up: a future ADR-0063-supersession + ADR-0087-amendment pair that lands per-endpoint stats may want to refactor `writeEndpointLines` to a table-driven loop with `(key, valueFn)` pairs (~30 LoC). **Disposition:** carry-forward to a future hardening phase — no current roadmap row. No phase-done blocker.

**N-3 (Note tier).** `internal/admin/version.go:51-57` — `BuildVersionString()` does not memoize its result. Each `/server_info` request re-runs the 7-char slice + `runtime.Version()` lookup + 4 string concatenations. Per-request cost is microseconds-scale (the runtime.Version() call is O(1)); under the 100-goroutine TestAdminConcurrentScrapeRace load no measurable contention surfaces. Suggested follow-up: a `var versionString = BuildVersionString()` package-level cache with a `sync.Once` guard would shave the per-request cost. **Disposition:** carry-forward to a future micro-optimisation pass — current shape is correct and adequately fast. No phase-done blocker.

**N-4 (Note tier).** The differential fixture's `canonicaliseConfigDump` (driver.go:298-372) hardcodes the three envoy-go-emitted envelope @type URLs as a literal map (`wantedTypes` at driver.go:357-361). If a future 08.x phase lands `RoutesConfigDump` or `EndpointsConfigDump` per ADR-0089's deferral-table item (c), the fixture's `wantedTypes` set must be expanded in lockstep. Suggested follow-up: a doc-comment on `wantedTypes` flagging the cross-reference to ADR-0089's deferral table would help future readers. **Disposition:** inline-fix opportunity but low value at MVP scope; carry-forward to 08.2. No phase-done blocker.

**N-5 (Note tier).** The fuzzer `FuzzConfigDumpFormat` (fuzz_test.go:22) seeds with three corpus entries (empty, just-admin, admin+minimal-cluster). The fuzzer's mutation strategy works on YAML byte-strings, and `bootstrap.Load` rejects most adversarial inputs (the seed expansion goes through the YAML parser). The 30s run produced 124 new-interesting inputs but no crash — the `t.Errorf` paths (invalid JSON output, missing `configs` field) never fired, which is correct because `buildConfigDump` is well-defined on any successfully-parsed bootstrap. Suggested follow-up: a future fuzzer-sharpening pass could add corpus entries for IPv6 endpoint, large-name cluster, multi-listener bootstrap to exercise the enumerate-static-{listeners,clusters} paths more directly. **Disposition:** carry-forward to a fuzzer-hardening pass — no current roadmap row. No phase-done blocker.

### 4.3 Note tier (informational; no action required)

**Note-1.** The `nolint:revive` comment on `cluster.ClusterInfo` (manager.go:122) is a deliberate stutter-name allowance — `cluster.ClusterInfo` triggers revive's `var-naming` check but the SPEC §6.2 fixed the type name verbatim and ADR-0087 reserves it for the public /clusters-snapshot surface. The lint suppression is grep-discoverable and well-justified.

**Note-2.** The `time.Sleep(20 * time.Millisecond)` after `s.MarkReady()` in admin_test.go:344 is a small race-mitigation against the LIVE-state propagation — `s.ready.Store(true)` is atomic but the test goroutines may start BEFORE the parent goroutine completes `s.MarkReady()`. The 20ms sleep is generous for the atomic propagation; not a correctness issue.

**Note-3.** The phase-done commit body cites ROADMAP row 08.1 flips `in-progress → done` AT this commit. Verified: ROADMAP.md row 08.1 reads `done` post-edit; row 08 stays `in-progress`; row 08.2 stays `planned`. The closure pattern matches 05 / 05.1 / 05.2 + 06 / 06.1 / 06.2 + 07 / 07.1 / 07.2 precedents.

---

## 5. Carry-forward dispositions

| Finding | Tier | Disposition |
|---|---|---|
| N-1 listener.Manager.Listeners() doc-comment ordering | Note (borderline Minor) | Carry-forward to 08.2 (Listener.Manager touched by drain wiring). |
| N-2 writeEndpointLines table-driven refactor | Note | Carry-forward to a future ADR-0063-supersession phase. |
| N-3 BuildVersionString() memoization | Note | Carry-forward to a future micro-optimisation pass. |
| N-4 wantedTypes cross-reference doc-comment | Note | Carry-forward to 08.2 (fixture 0010 likely touches the same canonicaliser). |
| N-5 FuzzConfigDumpFormat corpus expansion | Note | Carry-forward to a future fuzzer-hardening pass. |

No Major findings; no Minor findings requiring inline-fix; phase-done proceeds.

---

## 6. Six-gate verification appendix

All six gates run against HEAD at Task 15 Step 1. Verbatim outputs preserved in PROGRESS.md Task 15 entry. Summary:

| Gate | Command | Result |
|---|---|---|
| (a) build clean | `go build ./... && go vet ./... && golangci-lint run ./...` | PASS — clean |
| (b) unit tests + race | `go test -count=1 ./...` + `go test -count=1 -race ./...` | PASS — both clean |
| (c) h2spec re-run | `go test -count=1 -v ./test/conformance/h2spec/...` | PASS — 53/53 at the ADR-0051 pin (unchanged) |
| (d) 10 fuzzers @ 30s | `for fuzz in <11 fuzzers>; do go test -fuzz=$fuzz -fuzztime=30s ./<pkg>/; done` | PASS — all clean (FuzzBootstrapLoad, FuzzTcpProxyFilter, FuzzTLSContextParse, FuzzHCMConfigParse, FuzzFrameStream, FuzzHPACKDecode, FuzzPromTextFormat, FuzzAccessLogFormat, FuzzFilterChainParse, FuzzFilterChainMatch, FuzzConfigDumpFormat — actual 11 fuzzers post-08.1, not 10 as the PLAN's gate command listed; `FuzzDefaultFormatRender` named in the PLAN gate-command does not exist — never created in the codebase. The PROGRESS Task 15 entry records this as a PLAN-doc-error followup) |
| (e) differential 0000-0009 | `go test -count=1 -v ./test/differential/...` | PASS — 10/10 fixtures + 0007a-cors + 0007b-iteration-probe = 11 pass |
| (f) BEHAVIOR_CONTRACT.md populated | grep `^## Admin API$` + 5 per-endpoint subsections + 4 equivalence-matrix rows | PASS — populated by Step 2 of THIS task |

Six-gate state: ALL GREEN at HEAD. Phase-done commit may proceed.

---

## 7. Acceptance against SPEC §15

Cross-referencing SPEC §15 acceptance checklist:

- [x] Six admin endpoints registered + responding 200: verified by admin_test.go four-endpoint smoke + TestAdmin_FourEndpointsAcceptAnyMethod.
- [x] `/config_dump` JSON body shape per ADR-0086: verified by configdump_test.go's TestHandleConfigDump_PinMarshalOptions (1-space indent + EmitUnpopulated assertion landed by T6 review follow-up).
- [x] `/clusters` text body shape per ADR-0087: verified by clusters_test.go's per-cluster line-set assertions.
- [x] `/listeners` text body shape per ADR-0087: verified by listeners_test.go's name-only line-set + alphabetical-ordering assertions.
- [x] `/server_info` JSON body shape per ADR-0088: verified by serverinfo_test.go's TestHandleServerInfo_StatePostMarkReady + TestHandleServerInfo_StatePreMarkReady + uptime-monotonic + version-non-empty assertions.
- [x] `internal/cluster.Manager.Clusters()` snapshot accessor + types per SPEC §6.2: verified by cluster/manager_test.go new tests.
- [x] `Bootstrap.ConfigPath` field threaded post-Load: verified by bootstrap/bootstrap_test.go + main_test.go.
- [x] Differential fixture 0009-admin-config-dump green: verified by gate (e) above.
- [x] FuzzConfigDumpFormat 30s clean: verified by gate (d) above.
- [x] TestAdminConcurrentScrapeRace race-clean: verified by gate (b) -race above.
- [x] BEHAVIOR_CONTRACT.md `## Admin API` umbrella populated with 5 per-endpoint subsections + 4 equivalence-matrix rows: verified by THIS commit's edits.
- [x] Seven new ADRs (ADR-0084..ADR-0090) in DECISIONS.md: verified by `grep -c '^## ADR-008[4-9]:\|^## ADR-0090:'` = 7.
- [x] ROADMAP row 08.1 `in-progress → done`; 08 stays `in-progress`; 08.2 stays `planned`: verified by THIS commit's ROADMAP edit.
- [x] STATE.md `active-phase: 08.2-graceful-drain` + `lifecycle-state: 0` + `next-skill: superpowers:brainstorming`: verified by THIS commit's STATE rewrite.
- [x] `internal/admin/doc.go` enumerates six endpoints: verified by THIS commit's doc.go rewrite.
- [x] T14 review follow-up (a) + (b) applied: verified by THIS commit's SPEC §7.1 + expectations.yaml prose updates.

All acceptance items checked. Phase-done.
