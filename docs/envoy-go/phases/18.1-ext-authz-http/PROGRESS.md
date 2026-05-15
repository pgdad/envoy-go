# Phase 18.1 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..17 PROGRESS.md structure.

- **Phase:** 18.1 — HTTP filter `envoy.filters.http.ext_authz` (HTTP-mode)
- **Branch:** `phase-18.1-ext-authz-http-impl` (fresh worktree at `.worktrees/phase-18.1-ext-authz-http-impl`)
- **Base commit (master tip):** `c4951ae` (phase-18.1 PLAN SHA-fill follow-up; PLAN squash `9f786f7`; SPEC squash `308e9b6`; SPEC SHA-fill `312beec`; BRAINSTORM squash `854fa2c`; BRAINSTORM SHA-fill `6862d2c`)

## Preamble — execution preconditions

All 17 preconditions verified green at cold-start. Worktree branch `phase-18.1-ext-authz-http-impl` (fresh worktree at `.worktrees/phase-18.1-ext-authz-http-impl`, branched from master tip `c4951ae`). Master tail shows PLAN SHA-fill follow-up at `c4951ae`, PLAN squash at `9f786f7`, SPEC SHA-fill follow-up at `312beec`, SPEC squash at `308e9b6`, BRAINSTORM SHA-fill follow-up at `6862d2c`, BRAINSTORM squash at `854fa2c`. Go 1.26.2, golangci-lint v1.64.8, Docker client 28.4.0 + server 28.1.1 present. ADR tail at 0164 (ADR-0164 — the highest §Context-draft / full ADR anchored at the SPEC commit; per ADR-0044 ADR-on-impl convention + phase-13/15/16/17 pattern, the 7 phase-18.1 ADRs ADR-0156/0157/0159/0160/0161/0162/0163 are anchored as §Context drafts at SPEC commit `308e9b6`; §Decision + §Consequences bodies land at impl-time anchor Tasks 2/2/3/4/5/6/7 per the per-ADR table below). No ADR-0125 §(xiv) amendment paragraph — phase 18.1 is the FIRST §9 family-row since phase 13 to REUSE an existing canonical (5th canonical via ADR-0163) rather than extend the ADR-0125 roster. The `grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` command returns 2 matches but these are EXPLANATORY text within ADR-0163 §Context describing the ABSENCE of §(xiv) — confirmed via `grep -cE '^\*\*(xiv)\*\*' docs/envoy-go/DECISIONS.md` returning 0 (no actual amendment paragraph). SPEC at `308e9b6`; PLAN at `9f786f7`. `internal/filter/http/extauthz/` absent (Task 2 lands). `test/helpers/extauthzhttp/` absent (Task 10 lands). `cmd/envoy-go/main.go` registers 12 `httpReg.Register` calls (`router` + 11 filters: `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `jwtauthn`, `localratelimit`, `rbac`) at master tip `c4951ae`; `extauthz` insertion alphabetical-between-`envoygotest`-and-`fault` lands at Task 10. **Note on PLAN precondition 11 pattern**: the PLAN spec says to run `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9])'`; this regex does NOT match `TestDifferential` (the actual function name), so the run returns "no tests to run" (PASS). The actual differential suite is `TestDifferential` with sub-tests `0000-tcp-echo` through `0019-http-jwt-authn`; verified via (a) fixtures 0000–0009 batch: `go test -count=1 ./test/differential/ -run 'TestDifferential/000[0-9]'` → ok (28.1s); (b) fixtures 0010–0019 batch: `go test -count=1 ./test/differential/ -run 'TestDifferential/001[0-9]'` → ok (29.9s). Full 20-at-once run shows intermittent `subj start: subject ready: EOF` failures on different fixtures across runs (transient Docker resource contention; each fixture PASSES when run individually); recorded here, not blocking. Reference Envoy image `envoyproxy/envoy:v1.37.2` present (SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`; ADR-0008 pin; unchanged through phase 18.1). **`ext_authz` proto** present in module closure (`go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3 ExtAuthz | head -5` returns the type's exported fields). **Pre-existing fuzzers**: `grep -rE '^func Fuzz' --include='*.go' .` returns exactly 21 `Fuzz*` functions from phases 02–17 (co-located `fuzz_test.go` files under `internal/...`); full 30s-each run deferred to Task 14 Gate D phase-done verification per the phase-09..17 precedent. `go test -count=1 -short ./...` returns 47 `ok` packages with 0 failures. Working tree pristine (empty `git status --porcelain`).

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §10)

The seven 18.1-landing ADRs anticipated by SPEC §10 (ADR-0156/0157/0159/0160/0161/0162/0163). **§Context drafts ALREADY LANDED at SPEC commit `308e9b6`** per ADR-0044 ADR-on-impl convention. **§Decision + §Consequences bodies AUTHORED AT IMPL-TIME** per the phase-13/15/16/17 pattern. **NO ADR-0125 amendment paragraph required** — ADR-0163 records the explicit 5th-canonical-REUSE classification (NO §(xiv); verified at Task 1 cold-start via `grep -cE '^\*\*(xiv)\*\*' docs/envoy-go/DECISIONS.md` returning 0). Per-ADR Lands-in-task anchors (reproduced verbatim from PLAN §"ADRs introduced by this plan"):

| ADR | Subject (18.1 portion) | Lands-in-task |
|---|---|---|
| **ADR-0156** | `internal/filter/http/extauthz/` package shape — single-token directory (underscore-stripped per ADR-0114; matches `localratelimit/` + `jwtauthn/`) + DECODER-only `HTTPFilter` (`Encoder: nil`; 5th §9 row pure decode-side) + 6-base-counter `filterStats` (`ok`/`denied`/`error`/`disabled`/`failure_mode_allowed`/`invalid`; `disabled` STRUCTURALLY UNREACHABLE under MVP; unconditional allocation at `New()` time) + boot-registration alphabetical between `envoygotest` and `fault` + the deny-path `SendLocalReply` mechanism (status/body/headers from the auth response or `status_on_error`; `content-length` synthesized per ADR-0085) | Task 2 |
| **ADR-0157** | `compiledConfig` shape + the `services`-oneof dual-mode dispatch (a `checkFn` closure; `grpc_service` arm PARSE-REJECTs in 18.1, §Decision amended at 18.2 IMPL) + consumed-vs-deferred field discipline + the error-posture fields (`failure_mode_allow` / `failure_mode_allow_header_add` / `status_on_error` / `validate_mutations`) + `transport_api_version` V3-only PARSE-REJECT + the empty-`services`-oneof factory rejection + the §5.P10 error-classification boundary in `check.go` | Task 2 |
| **ADR-0159** | HTTP-outbound auth-check framework primitive — the thin ext_authz-local client (disposition (b) per SPEC §3.1); `httpAuthClient` wrapping `*http.Client` + the configured `HttpService.server_uri.timeout` + `path_prefix`; composes-against (does NOT reuse) the phase-17 `internal/jwks/Fetcher` outbound-HTTP structure; the (a)-vs-(b) record + the oauth2-triggers-`internal/httpclient/`-generalization forward-pointer | Task 3 |
| **ADR-0160** | `AuthorizationRequest` builder (HTTP-mode portion) — `headers_to_add` + `path_prefix` prepend + the top-level `ExtAuthz.allowed_headers`/`disallowed_headers` request-side filtering (`ListStringMatcher` → exact/prefix/suffix/contains/`ignore_case`/`safe_regex`; `custom` PARSE-REJECT) + the deprecated-`AuthorizationRequest.allowed_headers` honored-if-present disposition | Task 4 |
| **ADR-0161** | Bidirectional header-mutation discipline (HTTP-mode portion) — `AuthorizationResponse.{allowed_upstream_headers, allowed_upstream_headers_to_append, allowed_client_headers}` compilation + allow-path upstream injection (set vs append) + deny-path downstream `allowed_client_headers`-filtered emission + `validate_mutations` gating → `invalid` counter + the deny-path header-set construction (`text/plain` fallback, decision-headers-first ordering) + the `allowed_client_headers_on_success` deferral + the stash-for-HCM revisit note | Task 5 |
| **ADR-0162** | Request-body inclusion — `with_request_body{max_request_bytes, allow_partial_message, pack_as_bytes}` + the phase-13 ADR-0128 decode-side body-buffering reuse + the `allow_partial_message:false` over-limit → `SendLocalReply(413, "Payload Too Large", {connection: close})` edge case (auth NOT called, NO counters) + the `DecodeHeaders`-StopIteration / `DecodeData`-resume interaction | Task 6 |
| **ADR-0163** | Per-route 5th-canonical REUSE classification (explicit no-new-canonical decision; **NO ADR-0125 amendment paragraph** — the FIRST §9 row to REUSE an ADR-0125 canonical) + SHARED-stats discipline + the `CheckSettings` narrower-override surface + the 6-counter stat surface (`http.<HCM_stat_prefix>.ext_authz.*`; HCM-rooted SN2-reuse; RATIFIED-PENDING-IMPL-TIME §18.P6 + §18.P7 closed at Task 8) + the PGV wrinkles (`disabled` `const: true`; `override` oneof PGV-required) | Task 7 |

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The twelve planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — `test/helpers/extauthzhttp/` directory name + helper shape LOCKED per SPEC §12.1.** The in-process HTTP auth server lands at the top-level shared-helper location `test/helpers/extauthzhttp/` (NOT per-fixture) — mirrors the phase-17 `test/helpers/jwksbackend/` precedent, anticipating that the 18.2 gRPC fixture will want a sibling `test/helpers/extauthzgrpc/`. Package name `extauthzhttp`. Helper shape: a `Server` type + `New(ctx, addr, script) (*Server, error)` + `Stop()` + `Addr()`; `Script` supports fixed-response, per-path/method map, and body-inspecting predicate modes. *Anchored: SPEC §12.1 + §7.2.*

2. **D2 — `httpAuthClient` retry discipline LOCKED at ZERO retry per SPEC §12.2.** The thin ext_authz-local HTTP client does NO retry — single-attempt-then-error. Rationale: `HttpService` has no retry-policy field in the proto (unlike the JWKS fetcher's `RetryPolicy`); a connect failure / timeout maps straight to the **error** disposition per §5.P10. The 18.1 IMPL at Task 3 confirms against the §5.P10 error-classification boundary. Impl-time alternative (a fixed single-retry) NOT selected — MVP single-attempt. *Anchored: SPEC §12.2 + parent SPEC §5.P10.*

3. **D3 — `extauthz_test.go` single-file LOCKED per SPEC §12.3.** All 9 unit-test groups stay in one `extauthz_test.go` for 18.1 (mirrors `rbac/rbac_test.go`). Impl-time MAY split `check_test.go` if the file exceeds ~1800 LoC — an IMPL-cohesion call recorded at Task 3/Task 4 if it triggers. *Anchored: SPEC §12.3.*

4. **D4 — Async-resume-after-`OnDestroy` race guard LOCKED at the phase-09-fault pattern + an explicit `mu`/`done` guard + a cancellable `context.Context` per SPEC §12.4.** The outbound check runs in a plain goroutine (phase-09 fault precedent: `StopIteration` returned synchronously; the goroutine performs the cancellable call; `cb.ContinueDecoding()` or the deny `SendLocalReply` path on completion). The per-filter `mu sync.Mutex` + `done bool` guard: `OnDestroy` sets `done = true` under `mu` and calls `callCancel()`; the resume goroutine acquires `mu`, checks `done`, and aborts the callback touch if the stream is gone. The per-request `context.Context` (cancelled at `OnDestroy`) makes the in-flight `client.Do` return promptly. **This is the most-likely ADR-0044 escape-valve surface** (parent SPEC §7 + 18.1 SPEC §10) — if the `mu`/`done` guard proves insufficient or the HCM-dispatch interaction needs a framework primitive, it lands as **ADR-0165** at Task 9. Lands at Task 9. *Anchored: SPEC §12.4 + §6.3 + parent SPEC §7.*

5. **D5 — `safe_regex` RegexMatcher engine subset LOCKED at the phase-09/12 RE2 subset per SPEC §12.5.** The `allowed_headers`/`disallowed_headers` (and the response-side matcher fields) `safe_regex` arm reuses the phase-09/12 RegexMatcher-subset discipline — the `google_re2` engine arm is honored (Go `regexp`, RE2-compatible); other `RegexMatcher` engine arms PARSE-REJECT envoy-go-strict. The 18.1 IMPL at Task 4 confirms the exact subset against reference Envoy v1.37.2. *Anchored: SPEC §12.5 + parent SPEC §5.P8.*

6. **D6 — Deprecated `AuthorizationRequest.allowed_headers` disposition LOCKED at honored-if-present per SPEC §12.6.** When the deprecated `AuthorizationRequest.allowed_headers` (#1) IS present in a config, envoy-go honors it proto-faithful for backward-compat (mirrors the phase-17 amendment-4 "deprecated-but-honored" disposition). The top-level `ExtAuthz.allowed_headers` (#17) is the primary path. The 18.1 IMPL at Task 4 confirms against v1.37.2 whether it still honors the deprecated field or PARSE-IGNOREs it; if v1.37.2 PARSE-IGNOREs, the IMPL flips to silent-ignore + records the flip in PROGRESS.md + ADR-0160 §Decision. *Anchored: SPEC §12.6 + parent SPEC §6 amendment 2.*

7. **D7 — `validate_mutations` validation rule set LOCKED at the phase-10 header_mutation protected-header discipline per SPEC §12.7.** When `validate_mutations: true`, the auth service's allow-path upstream-injection headers + the deny-path client headers are validated: `:`-prefixed pseudo-headers REJECTED; invalid header-name characters REJECTED; invalid header-value characters REJECTED. A rejection drives the **invalid** disposition + the `invalid` counter (treated as the error posture per SPEC §6.3). The 18.1 IMPL at Task 5 pins the exact rule set against v1.37.2 `validate_mutations` behavior. *Anchored: SPEC §12.7 + the phase-10 header_mutation precedent.*

8. **D8 — RATIFIED-PENDING-IMPL-TIME pin closures assigned to concrete tasks (NEW; surfaces at PLAN-time per SPEC §11).** The three 18.1 RATIFIED-PENDING-IMPL-TIME pins close as follows: **§18.P6** (the 6-counter stat surface) + **§18.P7** (the Prometheus tag-extractor SN2-reuse) close at **Task 8** via an empirical scrape of reference Envoy v1.37.2's `/stats/prometheus` output for fixture 0020's listener config (the canonical RATIFIED-PENDING closure step per phase-16 §10 lesson (c) + phase-17 §11.P7 precedent); if divergent, amend ADR-0163 §Decision in-place at Task 8. The **§18.P11 deny-path header-ordering byte-shape** closes at **Task 13** via the fixture-harness differential diff (the auth-service-supplied decision headers first, framework housekeeping `content-length`/`date`/`server: envoy` after). *Anchored: SPEC §11 + parent SPEC §5.P6/P7/P11.*

9. **D9 — Counter-delta byte-equivalence assertion convention carried forward per planner-time decision (NEW; surfaces at PLAN-time).** Fixture 0020's driver scrapes `/stats/prometheus` before + after each scenario; asserts byte-equivalence against reference Envoy's expected delta in `expectations.yaml`. The 5 reachable counters (`ok`/`denied`/`error`/`failure_mode_allowed`/`invalid`) are asserted; the `disabled` counter is NOT asserted (STRUCTURALLY UNREACHABLE under MVP per parent SPEC §6 amendment 7 — it publishes 0 always). The per-route `disabled` scenario (scenario 6) asserts NO `ext_authz` counter increments at all. *Anchored: SPEC §7 + parent SPEC §6 amendment 7 + the phase-16/17 ADR-0145 precedent.*

10. **D10 — BEHAVIOR_CONTRACT §13.1 insertion at alphabetical-after-`csrf` position per SPEC §13.1 + ADR-0100 §2.2 (NEW; surfaces at PLAN-time).** The `### envoy.filters.http.ext_authz` subsection inserts alphabetically between `### envoy.filters.http.csrf` and the next subsection. The IMPL at Task 14 verifies the current BEHAVIOR_CONTRACT.md subsection ordering and, if it is landing-chronological rather than alphabetical, falls back to the observed convention + records the fallback in PROGRESS.md. *Anchored: SPEC §13.1 + ADR-0100 §2.2.*

11. **D11 — ADR-0044 escape-valve disposition: NO pre-reserved task slot (NEW; surfaces at PLAN-time).** Per the phase-13/14/15/16/17 precedent (the impl-time-unanticipated ADR landed at a follow-up task or folded into a main task), this PLAN does NOT pre-reserve an explicit task slot. The most-likely surface is the async-resume-after-`OnDestroy` race guard (D4) → if it needs an ADR-lift it lands as **ADR-0165** at Task 9 or as a follow-up commit between Task 13 and Task 14. *Anchored: SPEC §10 + parent SPEC §7.*

12. **D12 — Fixture 0020 is plaintext-only; NO PKI, NO TLS (NEW; surfaces at PLAN-time).** Unlike the phase-16 rbac mTLS fixture or the phase-17 jwt_authn RSA/ECDSA PKI fixture, fixture 0020 wires a plain HTTP/1.1 listener + a plain-HTTP auth server. No `pki/gen.go`. The TLS-to-auth-service plumbing is an 18.2 concern (parent SPEC §5.P13 RATIFIED-PENDING-IMPL-TIME). *Anchored: SPEC §7.2.*

---

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Files changed:** `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (new)
**Commit SHA:** `5e880d606103842d35909498894f84e46ef8e240`
**Notes:** Created PROGRESS.md; verified all 17 preconditions per PLAN §"Execution preconditions"; phase-18.1 SPEC + PLAN confirmed present in HEAD; SPEC at `308e9b6`, PLAN at `9f786f7`; ADR tail at 0164 (the 7 phase-18.1 ADRs ADR-0156/0157/0159/0160/0161/0162/0163 §Context drafts ALREADY landed at SPEC commit `308e9b6` per ADR-0044 ADR-on-impl convention; §Decision + §Consequences bodies land at impl-time anchor Tasks 2/2/3/4/5/6/7 per the per-ADR table above — mirroring phase-13/15/16/17 pattern); `internal/filter/http/extauthz/` absent (Task 2 lands); `test/helpers/extauthzhttp/` absent (Task 10 lands). No ADR-0125 §(xiv) amendment paragraph (`grep -cE '^\*\*(xiv)\*\*' docs/envoy-go/DECISIONS.md` returns 0 — the 2 `grep -nE '\(xiv\)'` matches are explanatory text within ADR-0163 §Context describing the absence). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention). Pre-existing fuzzers (21 fuzzers from phases 02–17 across co-located `fuzz_test.go` files) deferred to Task 14 Gate D phase-done verification per PLAN. **Note on PLAN precondition 11 wording**: the PLAN's regex `Test.*00(0[0-9]|1[0-9])` does not match `TestDifferential`; actual fixture differential run uses `TestDifferential/000[0-9]` + `TestDifferential/001[0-9]` patterns; both batches PASS; full 20-at-once run has transient Docker resource contention (different fixture EOF on each attempt; each individual fixture PASSES); documented above, not blocking.

**Outputs:**

### Precondition 1 — branch name

```
$ git rev-parse --abbrev-ref HEAD
phase-18.1-ext-authz-http-impl
```

### Precondition 2 — master tail

```
$ git log --oneline master | head -6
c4951ae phase 18.1 plan follow-up: STATE.md SHA-fill (TBD → 9f786f7 post-squash)
9f786f7 Squash merge phase-18.1-ext-authz-http-plan
312beec phase 18 spec follow-up: STATE.md SHA-fill (TBD → 308e9b6 post-squash)
308e9b6 Squash merge phase-18-http-filter-ext-authz-spec
6862d2c phase 18 brainstorm follow-up: STATE.md SHA-fill (TBD → 854fa2c post-squash)
854fa2c Squash merge phase-18-http-filter-ext-authz-brainstorm
```

### Precondition 3 — toolchain (go + golangci-lint + docker)

```
$ go version && golangci-lint version && docker version 2>&1 | head -30
go version go1.26.2 linux/amd64
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
Client: Docker Engine - Community
 Version:           28.4.0
 API version:       1.49 (downgraded from 1.51)
 Go version:        go1.24.7
 Git commit:        d8eb465
 Built:             Wed Sep  3 20:57:32 2025
 OS/Arch:           linux/amd64
 Context:           desktop-linux

Server: Docker Desktop 4.41.2 (191736)
 Engine:
  Version:          28.1.1
  API version:      1.49 (minimum version 1.24)
  Go version:       go1.23.8
  Git commit:       01f442b
  Built:            Fri Apr 18 09:52:57 2025
  OS/Arch:          linux/amd64
  Experimental:     false
 containerd:
  Version:          1.7.27
  GitCommit:        05044ec0a9a75232cad458027ca83437aae3f4da
 runc:
  Version:          1.2.5
  GitCommit:        v1.2.5-0-g59923ef
 docker-init:
  Version:          0.19.0
  GitCommit:        de40ad0
```

### Precondition 4 — DECISIONS.md ADR tail

```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
164
```

### Precondition 5 — ADR §Context drafts present

```
$ grep -cE '^## ADR-015[6-9]|^## ADR-016[0-4]' docs/envoy-go/DECISIONS.md
9

$ grep -cE '^Lands-in: Task [0-9]+ of phase-18.1' docs/envoy-go/DECISIONS.md
0

$ grep -cE 'Lands-in.*Task.*phase-18.1' docs/envoy-go/DECISIONS.md
7
```

**GREEN with note on second command:** The PLAN's second command uses `^` (line-start anchor) and returns 0; the actual DECISIONS.md format for `Lands-in:` fields is `**Lands-in:** Task N of phase-18.1 PLAN` (bold markdown + `PLAN` suffix), so the line does not start bare with `Lands-in:`. The corrected command (relaxed pattern, no `^` anchor, dropping the literal `: ` and trailing suffix) returns 7 matches (≥6 required by PLAN). Not blocking.

### Precondition 6 — NO ADR-0125 §(xiv) amendment

```
$ grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md
8503:**Phase 18 lands NO ADR-0125 amendment paragraph** — the FIRST §9 family-row since phase 13 to REUSE an existing canonical rather than extend the roster (breaking the phase-13-§(ix) / phase-14-§(x) / phase-15-§(xi) / phase-16-§(xii) / phase-17-§(xiii) per-phase-roster-growth streak). This is BRAINSTORM §11 lesson (d)'s inverse confirmation: the ADR-0125 roster grows when a filter's per-route shape is genuinely structurally novel, and stays flat when it is a textbook instance of an existing pattern. The absence of a §(xiv) amendment is itself a recorded decision — ADR-0163 records the explicit 5th-canonical-REUSE classification (the same way phase-13 §(ix) recorded the 5th canonical's *introduction*). Two minor PGV wrinkles vs the bare buffer/compressor 5th canonical are recorded (parent SPEC §6 amendment 3): `ExtAuthzPerRoute.disabled` is PGV `const: true` (envoy-go PARSE-REJECTs `disabled: false`) and the `override` oneof is PGV-required (envoy-go PARSE-REJECTs an empty `ExtAuthzPerRoute`); these do not constitute a new canonical — buffer's `BufferPerRoute` has the same disabled-bool-in-required-oneof structure.
8515:LANDS AT phase-18.1 IMPL per ADR-0044. Records: ADR-0125's roster staying at 8 (NO §(xiv) growth); the 5th-canonical-REUSE as a notable data point (the roster does not grow monotonically per phase); the stat-table 71 → 77; the §18.P6/§18.P7 RATIFIED-PENDING closures at the 18.1 stat-surface task.

$ grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md
0
```

**GREEN with note:** `grep -nE '\(xiv\)'` returns 2 matches but both are EXPLANATORY text within ADR-0163 §Context describing the ABSENCE of §(xiv) — not actual amendment paragraphs. `grep -cE '^\*\*\(xiv\)\*\*'` returns 0 confirming no actual amendment paragraph. Not blocking.

### Precondition 7 — SPEC SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/18.1-ext-authz-http/SPEC.md
308e9b62cfc42648e16e4347f3bff74a7c7a3c9d
```

(Matches `308e9b6` expected by PLAN.)

### Precondition 8 — PLAN SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/18.1-ext-authz-http/PLAN.md
9f786f732e805d38d73d1fb93772b7eb0426e15a
```

### Precondition 9 — pristine tree

```
$ git status --porcelain
(empty — no output)
```

**Note:** The PLAN spec says an untracked `.claude/` entry is acceptable; actual tree is fully pristine with no untracked entries. GREEN.

### Precondition 10 — short `go test ./...` pass

```
$ go test -count=1 -short ./... 2>&1 | grep -cE '^ok'
47

$ go test -count=1 -short ./... 2>&1 | grep -cE '^(FAIL|---\s+FAIL)'
0

$ go test -count=1 -short ./... 2>&1 | grep -vE '^\?' | head -50
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.871s
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.009s
ok  	github.com/esalaine/envoy-go/internal/admin	0.428s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.020s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.022s
ok  	github.com/esalaine/envoy-go/internal/drain	0.078s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.022s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	2.494s
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.134s
ok  	github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit	0.390s
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	0.008s
ok  	github.com/esalaine/envoy-go/internal/filter/http/compressor	0.014s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	0.006s
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	0.010s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	0.038s
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.265s
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	0.007s
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	0.077s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	0.008s
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	0.009s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	0.217s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.168s
ok  	github.com/esalaine/envoy-go/internal/jwks	1.612s
ok  	github.com/esalaine/envoy-go/internal/jwt	0.044s
ok  	github.com/esalaine/envoy-go/internal/listener	3.033s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	0.044s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	0.006s
ok  	github.com/esalaine/envoy-go/internal/matcher	0.005s
ok  	github.com/esalaine/envoy-go/internal/stats	0.005s
ok  	github.com/esalaine/envoy-go/internal/tls	0.023s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	0.064s
ok  	github.com/esalaine/envoy-go/test/differential	0.064s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	0.005s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	0.005s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	0.006s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	0.004s
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	0.003s
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	0.004s
ok  	github.com/esalaine/envoy-go/test/fixtures/0016-http-compressor/inputs	0.007s
ok  	github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki	0.006s
ok  	github.com/esalaine/envoy-go/test/helpers	0.010s
ok  	github.com/esalaine/envoy-go/test/helpers/echobackend	0.007s
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend	0.006s
```

### Precondition 11 — pre-existing differential suite green

**Note on PLAN regex:** The PLAN's command `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9])'` returns "no tests to run" (PASS) because the regex does not match `TestDifferential`. The correct sub-test patterns are:

```
$ go test -count=1 ./test/differential/ -run 'TestDifferential/000[0-9]' -timeout 300s
ok  	github.com/esalaine/envoy-go/test/differential	28.104s

$ go test -count=1 ./test/differential/ -run 'TestDifferential/001[0-9]' -timeout 300s
ok  	github.com/esalaine/envoy-go/test/differential	29.913s
```

Both fixture batches (0000–0009 and 0010–0019) PASS. Full 20-at-once run has transient Docker resource-contention failures (`subj start: subject ready: EOF`; different fixture each attempt); each individual fixture passes cleanly in isolation. **GREEN** (transient contention noted; not blocking per project precedent — see phase-17 PROGRESS precondition 11 note).

### Precondition 12 — pre-existing fuzzers ≥21

```
$ grep -rE '^func Fuzz' --include='*.go' . | wc -l
21
```

21 fuzzers from phases 02–17 present (co-located `fuzz_test.go` files under `internal/...`). Full 30s-each dedicated `-fuzz=… -fuzztime=30s` runs deferred to Task 14 Gate D phase-done verification per the phase-09..17 precedent. **GREEN with deferred fuzzer runs documented.**

Fuzz functions found:
- `internal/bootstrap/fuzz_test.go`: `FuzzBootstrapLoad`
- `internal/accesslog/fuzz_test.go`: `FuzzAccessLogFormat`
- `internal/stats/fuzz_test.go`: `FuzzPromTextFormat`
- `internal/listener/listenerfilter/fuzz_test.go`: `FuzzFilterChainMatch`
- `internal/drain/fuzz_test.go`: `FuzzDrainTransitions`
- `internal/filter/tcpproxy/fuzz_test.go`: `FuzzTcpProxyFilter`
- `internal/filter/http/fuzz_test.go`: `FuzzFilterChainParse`
- `internal/filter/http/header_mutation/fuzz_test.go`: `FuzzHeaderMutationConfigParse`
- `internal/filter/http/csrf/fuzz_test.go`: `FuzzCsrfPolicyConfigParse`
- `internal/filter/http/localratelimit/fuzz_test.go`: `FuzzLocalRateLimitConfigParse`
- `internal/filter/http/compressor/fuzz_test.go`: `FuzzCompressorConfigParse`
- `internal/filter/http/fault/fuzz_test.go`: `FuzzFaultConfigParse`
- `internal/filter/http/bandwidthlimit/fuzz_test.go`: `FuzzBandwidthLimitConfigParse`
- `internal/filter/http/jwtauthn/fuzz_test.go`: `FuzzJwtAuthnConfigParse`
- `internal/filter/http/buffer/fuzz_test.go`: `FuzzBufferConfigParse`
- `internal/filter/http/rbac/fuzz_test.go`: `FuzzRBACConfigParse`
- `internal/filter/hcm/fuzz_test.go`: `FuzzHCMConfigParse`
- `internal/filter/hcm/h2/fuzz_test.go`: `FuzzFrameStream`
- `internal/filter/hcm/h2/fuzz_test.go`: `FuzzHPACKDecode`
- `internal/admin/fuzz_test.go`: `FuzzConfigDumpFormat`
- `internal/tls/fuzz_test.go`: `FuzzTLSContextParse`

### Precondition 13 — reference Envoy image SHA

```
$ docker image inspect envoyproxy/envoy:v1.37.2 --format '{{.Id}}'
sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
```

### Precondition 14 — ext_authz proto package present

```
$ go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3 ExtAuthz | head -5
package ext_authzv3 // import "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_authz/v3"

type ExtAuthz struct {

	// External authorization service configuration.
```

### Precondition 15 — `internal/filter/http/extauthz/` absent

```
$ test ! -d internal/filter/http/extauthz && echo "ok: extauthz absent"
ok: extauthz absent
```

### Precondition 16 — `test/helpers/extauthzhttp/` absent

```
$ test ! -d test/helpers/extauthzhttp && echo "ok: extauthzhttp absent"
ok: extauthzhttp absent
```

### Precondition 17 — `cmd/envoy-go/main.go` registers 12 `httpReg.Register` calls

```
$ grep -cE 'httpReg.Register' cmd/envoy-go/main.go
12
```

The 12 registrations: `router` + 11 filters: `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `jwtauthn`, `localratelimit`, `rbac`. `extauthz` (alphabetical between `envoygotest` and `fault`) lands at Task 10.

---

## Task 2 — extauthz package skeleton (doc.go + extauthz.go + extauthz_test.go) + Groups 1+2+7 + ADR-0156 §Decision+§Consequences + ADR-0157 §Decision+§Consequences

**Files changed:** `internal/filter/http/extauthz/doc.go` (new), `internal/filter/http/extauthz/extauthz.go` (new, 600 LoC), `internal/filter/http/extauthz/extauthz_test.go` (new, ~580 LoC), `docs/envoy-go/DECISIONS.md` (ADR-0156 + ADR-0157 §Decision+§Consequences filled)
**Commit SHA:** `b528060`
**Notes:** Followed `superpowers:test-driven-development`. Groups 1 (10 tests), 2 (9 tests), 7 (10 tests) pass; all 38 tests green. `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet ./internal/filter/http/extauthz/...` exit 0. `go test -count=1 -short ./...` grew from 47 to 48 `ok` packages (0 failures). ADR-0156 §Decision 7-point body (i)–(vii) + §Consequences filled at Task 2. ADR-0157 §Decision 7-point body (i)–(vii) + §Consequences filled at Task 2. **Build-order fix** applied during TDD: initial `buildCompiledConfig` called `buildHTTPCheckFn` stub before `transport_api_version` + `with_request_body` structural checks — caused 3 tests to fail with the wrong error message (`TestNew_NonV3TransportApiVersion`, `TestNew_NonAutoTransportApiVersion`, `TestNew_WithRequestBodyMaxBytesZero`); fixed by restructuring the build order to: (1) nil/grpc_service PARSE-REJECT, (2) `transport_api_version` check, (3) `with_request_body` validation, (4) `buildHTTPCheckFn` stub call. This gives deterministic error ordering where structural checks fire before service-specific dispatch. `errHTTPCheckFnStub` sentinel used by Group 1 `TestNew_HttpService_ValidConfig_Task2Stub` to distinguish stub path from real parse rejections via `errors.Is(err, errHTTPCheckFnStub)` — tightened to `factory != nil` at Task 3. No ADR-0044 escape-valve triggered. Boot-registration deferred to Task 10 (13 → 14 entries). `cmd/envoy-go/main.go` still at 12 `httpReg.Register` calls.

**Outputs:**

### Test run — Groups 1+2+7 (38 tests, race detector)

```
$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.011s

$ go vet ./internal/filter/http/extauthz/...
(no output — exit 0)
```

### Test run — full suite (48 packages, short mode)

```
$ go test -count=1 -short ./... 2>&1 | grep -cE '^ok'
48

$ go test -count=1 -short ./... 2>&1 | grep -cE '^(FAIL|---\s+FAIL)'
0
```

### ADR acceptance-criteria grep

```
$ grep -nE '^## ADR-0156|^## ADR-0157' docs/envoy-go/DECISIONS.md
8305:## ADR-0156: `internal/filter/http/extauthz/` package shape — ...
8360:## ADR-0157: `compiledConfig` shape + the `services`-oneof dual-mode dispatch — ...
```

2 matches (≥2 required). Both §Decision + §Consequences bodies filled. Status: Accepted.

### Task 2 review-fix (6 issues from code-quality review)

**I1** — `resolvePerRouteConfig` signature changed from `interface{}` to `proto.Message` (extauthz.go:487); `"google.golang.org/protobuf/proto"` + `matcherv3` imports added.
**I2** — `TestResolvePerRouteConfig_UnknownMsgTypeFallback` fixed to pass `&corev3.GrpcService{}` (a valid `proto.Message` that is NOT `*ExtAuthzPerRoute`) so the `!ok` branch genuinely fires (extauthz_test.go:905–921).
**I3** — `compileStringMatcherList` parameter changed from `interface{}` to `*matcherv3.ListStringMatcher` (extauthz.go:397).
**M1** — Comment on `errHTTPCheckFnStub` corrected from "Exported as a package-level error" to "Accessible to package-level tests" (extauthz.go:41).
**M2** — ADR-0157 §Decision (iii) ordering description corrected: "fires BEFORE the `services`-oneof dispatch" → "fires AFTER the `services`-oneof presence/grpc PARSE-REJECTs" with the actual 4-step ordering (DECISIONS.md ~line 8385).
**M3** — ADR-0156 §Consequences test count corrected: 39 → 38 (DECISIONS.md ~line 8346); confirmed via `grep -c '^func Test' internal/filter/http/extauthz/extauthz_test.go` = 38.
All 38 tests still green post-fix. M4/M5 not touched (forward-pointing notes only).

## Task 3 — check.go HTTP-outbound auth-check primitive (`httpAuthClient` + `buildHTTPCheckFn` + the checkFn closure + the HTTP-response → `checkDisposition` mapping + the §5.P10 error-classification boundary) + extauthz_test.go Group 4 + ADR-0159 §Decision+§Consequences

**Files changed:** `internal/filter/http/extauthz/check.go` (new, 289 LoC), `internal/filter/http/extauthz/extauthz.go` (modified — Task 2 `buildHTTPCheckFn` stub removed, replaced by a cross-reference comment; `errHTTPCheckFnStub` retired to a nil-match audit-trail placeholder), `internal/filter/http/extauthz/extauthz_test.go` (modified — Group 4 appended; the 4 Task-2 M4 stub-tolerant tests tightened in place), `docs/envoy-go/DECISIONS.md` (ADR-0159 §Decision+§Consequences filled)
**Commit SHA:** `21f0ac0` (SHA-filled by the follow-up commit `phase 18.1 Task 3 follow-up: PROGRESS.md SHA-fill`)
**Notes:** Followed `superpowers:test-driven-development` — Group 4 tests written first. The HTTP-outbound auth-check primitive lands per ADR-0159 disposition (b): a thin ext_authz-local `httpAuthClient` in `check.go`, NOT a generalized `internal/httpclient/` package. `httpAuthClient` wraps `*http.Client` (`&http.Client{Timeout: durationpbToGo(server_uri.timeout)}`; ZERO retry per planner-time decision D2) + the parsed base URL + `path_prefix`. The closure builds the outbound POST (`path_prefix` prepended via the double-slash-avoiding `joinPaths`; the `authRequest` headers copied as-is — request-side filtering STUBBED until Task 4; the body as a `bytes.Reader` when non-empty), threads `ctx` through `http.NewRequestWithContext` so `OnDestroy`'s cancel aborts the in-flight call, calls `client.Do`, and maps the response. `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `go test -count=1 -short ./...` still 48 `ok` / 0 fail. Test count 38 → 52. No ADR-0044 escape-valve triggered. Boot-registration still deferred to Task 10.

### `buildHTTPCheckFn` signature — deviation from the PLAN nominal

The PLAN File-structure-table nominal signature was `buildHTTPCheckFn(hs *ext_authzv3.HttpService, ar *authRequestCfg, validateMutations bool) (checkFn, error)`. The signature settled at Task 3 is **`buildHTTPCheckFn(hs *ext_authzv3.HttpService) (checkFn, error)`** — taking only the `HttpService`. Rationale: the `authRequestCfg` type + `buildAuthRequest` land at Task 4, and `validate_mutations` gating over the extracted header sets lands at Task 5; neither type exists at Task 3. The reduced signature matches the existing `buildCompiledConfig` call-site (Task 2 already wired it to pass only `hs`). The PLAN Task-3 description explicitly sanctioned this ("check what Task 2's `buildCompiledConfig` currently passes to the stub `buildHTTPCheckFn`"). When Task 4/Task 5 land, the request-side-filtered headers reach the closure via the `*authRequest` argument and the `validate_mutations` gating via the (Task-5) compiled `authorization_response` matcher triple captured in the closure — no `buildHTTPCheckFn` signature change is anticipated. Recorded in ADR-0159 §Decision (iii).

### §5.P10 error-classification boundary — as implemented

`mapHTTPResponse` switches on `resp.StatusCode`:
- **HTTP `200` → `dispAllow`** — header extraction STUBBED at Task 3 (`upstreamSet`/`upstreamApp` left nil; `allowed_upstream_headers` / `allowed_upstream_headers_to_append` extraction lands at Task 5).
- **HTTP `401` or `403` → `dispDeny`** — the recognized deny-status set per parent SPEC §5.P10. The response body is read verbatim into `denyBody` per §5.P11; `denyStatus` set to the status code. `allowed_client_headers` extraction STUBBED at Task 3 (`denyHeaders` left nil; lands at Task 5).
- **any other status → `dispError`** — returns `checkDisposition{class: dispError}` + a descriptive error.

Before `mapHTTPResponse` is reached: a `client.Do` error (connect failure / timeout / `ctx` cancelled) → `dispError` + the wrapped transport error; a `NewRequestWithContext` build error → `dispError`; an IO error reading the deny body → `dispError`. ZERO retry — a single attempt then the error disposition (D2 — `HttpService` has no retry-policy proto field).

### Task-2 M4 stub-test tightening

The 4 Task-2 M4 stub-tolerant tests were **tightened in place** (not via separate `_RealImpl` variants): `TestNew_StatusOnError_Default`, `TestNew_StatusOnError_Explicit`, `TestCompiledConfig_FailureModeAllowConsumed`, `TestCompiledConfig_WithRequestBodyConsumed` — their stub-era `if cc != nil { ... } else if err == nil { ... }` wrappers were replaced with unconditional assertions (`if err != nil { t.Fatalf }` + `if cc == nil { t.Fatal }` + the field assertion) now that `buildHTTPCheckFn` is real. The WIP had initially added separate `_RealImpl` duplicate functions; those 4 duplicates were removed in favor of tightening the named originals in place per the Task 3 instruction (a short NOTE comment marks the tightening). `TestNew_HttpService_ValidConfig_Task2Stub` was likewise tightened in place to assert `factory != nil`; `TestNew_HttpService_ValidConfig_RealImpl` is kept as a distinct Group-4 factory smoke-test.

### Group 4 coverage note — `headers_to_add` / deprecated `allowed_headers`

The PLAN Task-3 Step-1 enumeration lists `headers_to_add` appended + deprecated `AuthorizationRequest.allowed_headers` honored-if-present among the Group 4 cases. Those two are **request-side** `AuthorizationRequest`-builder concerns — they are produced by `buildAuthRequest`, which is STUBBED at Task 3 and lands at Task 4 (ADR-0160). At Task 3 the closure copies the `authRequest` headers as-is; `TestCheckFn_HeadersForwarded` covers the header-forwarding mechanism (the closure faithfully transmits whatever `authRequest.headers` contains). The proto-field-level `headers_to_add` / deprecated-`allowed_headers` assertions land with Group 3 / the `buildAuthRequest` tests at Task 4 — recorded here so the PLAN-vs-impl Group-4 delta is on disk.

**Outputs:**

### Test run — full suite + Group 4 (race detector)

```
$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.068s

$ go vet ./internal/filter/http/extauthz/...
(no output — exit 0)
```

### Test run — Group 4 verbose (`TestCheckFn|TestBuildHTTPCheckFn`, 13 tests)

```
$ go test ./internal/filter/http/extauthz/ -run 'TestCheckFn|TestBuildHTTPCheckFn' -v
=== RUN   TestBuildHTTPCheckFn_MissingServerURI
--- PASS: TestBuildHTTPCheckFn_MissingServerURI (0.00s)
=== RUN   TestBuildHTTPCheckFn_EmptyURI
--- PASS: TestBuildHTTPCheckFn_EmptyURI (0.00s)
=== RUN   TestBuildHTTPCheckFn_ValidConfig_ReturnsNonNilFn
--- PASS: TestBuildHTTPCheckFn_ValidConfig_ReturnsNonNilFn (0.00s)
=== RUN   TestCheckFn_Allow_Status200
--- PASS: TestCheckFn_Allow_Status200 (0.00s)
=== RUN   TestCheckFn_Deny_Status401
--- PASS: TestCheckFn_Deny_Status401 (0.00s)
=== RUN   TestCheckFn_Deny_Status403
--- PASS: TestCheckFn_Deny_Status403 (0.00s)
=== RUN   TestCheckFn_Error_UnrecognizedStatus
--- PASS: TestCheckFn_Error_UnrecognizedStatus (0.00s)
=== RUN   TestCheckFn_Error_ConnectFailure
--- PASS: TestCheckFn_Error_ConnectFailure (0.00s)
=== RUN   TestCheckFn_Error_Timeout
--- PASS: TestCheckFn_Error_Timeout (0.05s)
=== RUN   TestCheckFn_Error_ContextCancelled
--- PASS: TestCheckFn_Error_ContextCancelled (0.00s)
=== RUN   TestCheckFn_PathPrefix_Prepended
--- PASS: TestCheckFn_PathPrefix_Prepended (0.00s)
=== RUN   TestCheckFn_HeadersForwarded
--- PASS: TestCheckFn_HeadersForwarded (0.00s)
=== RUN   TestCheckFn_WithRequestBody
--- PASS: TestCheckFn_WithRequestBody (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.056s
```

### Test run — Groups 1+2+7 regression (no regression)

```
$ go test ./internal/filter/http/extauthz/ -run 'TestNew_|TestCompiledConfig_|TestParsePerRoute|TestResolvePerRoute|TestFilterStats' -v 2>&1 | tail -40
=== RUN   TestNew_HttpService_ValidConfig_Task2Stub
--- PASS: TestNew_HttpService_ValidConfig_Task2Stub (0.00s)
=== RUN   TestNew_StatusOnError_Default
--- PASS: TestNew_StatusOnError_Default (0.00s)
=== RUN   TestNew_StatusOnError_Explicit
--- PASS: TestNew_StatusOnError_Explicit (0.00s)
=== RUN   TestNew_StatPrefix_Consumed
--- PASS: TestNew_StatPrefix_Consumed (0.00s)
=== RUN   TestFilterStats_6Counters
--- PASS: TestFilterStats_6Counters (0.00s)
=== RUN   TestFilterStats_CounterNames
--- PASS: TestFilterStats_CounterNames (0.00s)
=== RUN   TestFilterStats_NilRegistryTolerance
--- PASS: TestFilterStats_NilRegistryTolerance (0.00s)
=== RUN   TestCompiledConfig_FieldFinal
--- PASS: TestCompiledConfig_FieldFinal (0.00s)
=== RUN   TestCompiledConfig_FailureModeAllowConsumed
--- PASS: TestCompiledConfig_FailureModeAllowConsumed (0.00s)
=== RUN   TestCompiledConfig_WithRequestBodyConsumed
--- PASS: TestCompiledConfig_WithRequestBodyConsumed (0.00s)
=== RUN   TestParsePerRoute_EmptyOverride
--- PASS: TestParsePerRoute_EmptyOverride (0.00s)
... (all Group 1/2/7 tests PASS) ...
=== RUN   TestNew_HttpService_ValidConfig_RealImpl
--- PASS: TestNew_HttpService_ValidConfig_RealImpl (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.004s
```

### Test run — full suite (48 packages, short mode)

```
$ go test -count=1 -short ./... 2>&1 | grep -cE '^ok'
48

$ go test -count=1 -short ./... 2>&1 | grep -cE '^(FAIL|---\s+FAIL)'
0
```

### ADR acceptance-criteria grep

```
$ grep -nE '^## ADR-0159' docs/envoy-go/DECISIONS.md
8436:## ADR-0159: HTTP-outbound auth-check framework primitive — ...
```

1 match (1 required). §Decision (5-point body (i)–(v)) + §Consequences filled. Status: Accepted; Date: 2026-05-14; Lands-in: Task 3 of phase-18.1 PLAN.

### Task 3 review-fix (6 issues from code-quality review)

**Issue 1 (Important)** — Dead `httpAuthClient.baseURL` field + double-strip eliminated. `buildCheckFnClosure` signature changed from `(hac *httpAuthClient, serverURI string)` to `(hac *httpAuthClient)` (check.go:124); the closure now uses `hac.baseURL + joinPaths(hac.pathPrefix, req.path)` directly (check.go:132) instead of calling `buildTargetURL(serverURI, ...)` → `stripPath(serverURI)` per request. `buildTargetURL` repurposed to take a pre-stripped `base` string (check.go:213–231) — `stripPath` runs exactly once per checkFn lifetime (at `buildHTTPCheckFn` time, check.go:99). Call site in `buildHTTPCheckFn` updated: `buildCheckFnClosure(hac)` (check.go:112). `uri` local variable is still used for `stripPath(uri)` in the `hac` struct literal.

**Issue 2 (Minor)** — `errHTTPCheckFnStub` dead-code var removed. The `errors.New(...)` allocation (extauthz.go:53) and its factually incorrect comment claiming tests reference it via `errors.Is` are replaced by a single one-line tombstone comment (extauthz.go:42–44).

**Issue 3 (Minor)** — `doc.go` gofmt-broken bullet continuations fixed. Two orphaned fragments in the `# ADR anchors` section repaired: ADR-0156 bullet now ends `…mechanism + boot-registration ordering. Lands Task 2.` (doc.go:170–171 collapsed); ADR-0160 bullet now starts `…headers_to_add +` so the next line is a proper continuation (doc.go:180–182). `go doc ./internal/filter/http/extauthz/ | tail -40` confirms all ADR bullets render as single coherent items.

**Issue 4 (Minor)** — `stripPath` + `joinPaths` + `buildTargetURL` unit tests added. Three table-driven tests in `extauthz_test.go` (appended after Group 4): `TestStripPath` (7 cases: URI with path, without path, single-segment path, HTTPS, trailing slash, no scheme separator, empty), `TestJoinPaths` (7 cases: both non-empty, double-slash avoidance, no-slash-added, empty prefix, empty path, both empty, prefix-trailing-slash-empty-path), `TestBuildTargetURL` (5 cases: base+prefix+path, no prefix, double-slash avoidance, both empty, serverURI-with-path-component pre-stripped). No live server needed.

**Issue 5 (Minor)** — Duplicate test cleaned up. `TestNew_HttpService_ValidConfig_Task2Stub` deleted (extauthz_test.go:284–293 replaced with a tombstone comment). `TestNew_HttpService_ValidConfig_RealImpl` renamed to `TestNew_HttpService_ValidConfig` (extauthz_test.go:1279). One test, cleanly named. NOTE comment updated accordingly.

**Issue 6 (Minor)** — Latent unsynchronized `sas.received` write removed. `received *http.Request` field removed from `scriptableAuthServer` struct (extauthz_test.go:943). `sas.received = r` write removed from both `newScriptableAuthServer` handler (extauthz_test.go:942) and `newSlowAuthServer` handler (extauthz_test.go:961). Nothing reads `received`; the field and both writes were dead and would constitute a data-race if a future test read the field unsynchronized.

All 6 fixes: `go build` exit 0, `go vet` exit 0, `gofmt -l` empty, `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0 (ok 1.069s).

## Task 4 — attributes.go AuthorizationRequest builder + request-side header filtering + Group 3 + ADR-0160 §Decision+§Consequences

**Files changed:** `internal/filter/http/extauthz/attributes.go` (new, ~195 LoC; review-fix: trailing `buildAuthRequest` call-site comment corrected), `internal/filter/http/extauthz/extauthz.go` (modified — placeholder `stringMatcherList` type replaced by comment cross-reference; `compileStringMatcherList` stub replaced by comment cross-reference; `buildCompiledConfig` step 6 updated to handle real `(sml, error)` return; `matcherv3` import removed; review-fix: `DecodeHeaders` skeleton gained the Task-9 `buildAuthRequest` call-site forward-pointer), `internal/filter/http/extauthz/check.go` (modified at review-fix — stale "request-side header filtering STUBBED at Task 3" header comment replaced by the `buildAuthRequest` call-site-boundary note; closure inline comments corrected), `internal/filter/http/extauthz/extauthz_test.go` (modified — Group 3 appended; `matcherv3` import added), `docs/envoy-go/DECISIONS.md` (ADR-0160 HTTP-mode §Decision+§Consequences filled; review-fix: §Decision (vii) added)
**Commit SHA:** `26e2e48` (original landing); review-fix commit SHA below.

**Notes:** Followed `superpowers:test-driven-development`. Group 3 tests written first (RED confirmed — build failure on undefined `buildAuthRequest` + wrong arity on `compileStringMatcherList`). `attributes.go` authored; `buildCompiledConfig` and the `compileStringMatcherList` stub wired to the real implementation. Test count 52 → 83. `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `gofmt -l` empty.

**Deviation from PLAN Step 4 — `buildAuthRequest` wiring (review-fix, Option B chosen).** PLAN Task 4 Step 4 says "Wire the real `buildAuthRequest` into `check.go`'s `checkFn` closure (replace the Task 3 stub)." The original Task 4 landing did NOT do this — `buildAuthRequest` was authored + unit-tested but not called from any production path; `check.go`'s closure still copied `req.headers` as-is. The spec-compliance review flagged this. **Resolution: the PLAN Step 4 wording is imprecise; `buildAuthRequest` genuinely belongs at the Task 9 `DecodeHeaders` call site, not in the `check.go` closure.** Architectural reasoning: (1) `buildAuthRequest(f *filter, hs, headers, body, path)` needs the per-stream `*filter` carrying the *per-route-resolved* `f.activeRC` (`cc.allowedHeaders`/`cc.disallowedHeaders`) AND the real client request headers from `dcb.RequestHeaders()`. (2) `buildHTTPCheckFn` runs at config-load time — it has only `hs`, no per-stream `f` (not even the `compiledConfig`, since it is called mid-construction of `cc`), no request headers. (3) The `checkFn` closure is mode-agnostic (the type is shared with 18.2's gRPC-mode closure) and by contract RECEIVES an already-built `*authRequest` — handing it `f` + raw headers would couple the mode-agnostic type to the per-stream filter shape and duplicate `buildAuthRequest`'s job inside the closure, making the code worse. (4) Only `DecodeHeaders` (Task 9) has `f` (with resolved `activeRC`), `dcb.RequestHeaders()`, the buffered body, and the path coexisting — so that is the correct call site. Option A (restructuring to run the filtering at the check.go layer) was rejected as architecturally unsound. The review-fix therefore documents the justified deviation: the stale "STUBBED" header comment in `check.go` is replaced by a `buildAuthRequest` call-site-boundary note; `extauthz.go`'s `DecodeHeaders` skeleton gains an explicit forward-pointer that Task 9 MUST call `buildAuthRequest` before invoking the `checkFn`; the trailing comment in `attributes.go` is corrected (it previously claimed `buildAuthRequest` is "called from check.go's closure"); ADR-0160 §Decision gains point (vii) recording the call-site boundary. No production-code behavior change — `buildAuthRequest` is consumed in production at Task 9 as originally planned. Post-fix: `go build` / `go vet` / `gofmt -l` (empty) / `go test -race -count=1` all green (83 tests, unchanged — Option B is documentation + comment-correctness only, no new code path).

**`stringMatcherList` type location:** Moved from the Task 2 placeholder in `extauthz.go` to `attributes.go` — co-located with `compileStringMatcherList` + `matchAny` per the rbac/evaluator.go precedent of keeping a compiled type alongside its constructor and methods. The placeholder in `extauthz.go` is replaced by a comment cross-reference.

**`buildAuthRequest` signature:** `buildAuthRequest(f *filter, hs *ext_authzv3.HttpService, headers http.Header, body []byte, path string) *authRequest`. Filter carries `f.activeRC` for `cc.allowedHeaders`/`cc.disallowedHeaders`; `hs` carries `AuthorizationRequest`; `headers` from DecodeHeaders (Task 9 wires from dcb; tests pass directly); `body` is nil at Task 4 (Task 6 wires); `path` stored as-is (path_prefix prepend done in check.go's closure). Method always POST.

**Header name matching convention:** Header names in `http.Header` are in canonical Title-Case form (e.g. `Authorization`); `buildAuthRequest` lowercases them before calling `matchAny` (e.g. `authorization`) to match Envoy's internal lowercase-header convention. Matchers compiled against lowercase patterns (as typical in Envoy configs) therefore match canonical-form headers correctly.

**D5 confirmation (safe_regex engine-arm subset):** The `google_re2` arm is the only valid arm in the v1.37.2 go-control-plane proto for `RegexMatcher`; Go's `regexp` package is RE2-compatible. `compileOneStringMatcher` compiles the regex via `regexp.Compile` (same as rbac/evaluator.go matchString + internal/matcher compileStringMatcher). Nil `RegexMatcher` → PARSE-REJECT; invalid regex → PARSE-REJECT. Full empirical confirmation against a running v1.37.2 deferred to Task 13 differential fixture pass per Task 4 instruction. Evidence basis: go-control-plane proto field documentation + phase-09/12 RegexMatcher-subset precedent. **No divergence from LOCKED D5 disposition.**

**D6 confirmation (deprecated `AuthorizationRequest.allowed_headers` disposition):** The go-control-plane generated Go code for `AuthorizationRequest.AllowedHeaders` carries the `// Deprecated: Marked as deprecated in ...` annotation at v1.37.2 proto level — the field is deprecated-but-present (not removed). `buildAuthRequest` implements honored-if-present: when `cc.allowedHeaders` (top-level primary, `ExtAuthz.allowed_headers` #17) is nil AND `hs.AuthorizationRequest.AllowedHeaders` (#1) is non-nil, the deprecated field is compiled and used as the effective allow-list; when top-level is set, deprecated field is silently ignored. Full empirical confirmation against a running v1.37.2 deferred to Task 13 differential fixture pass. Evidence basis: proto-field annotation + phase-17 amendment-4 "deprecated-but-honored" precedent. **No flip to silent-ignore — field is not removed in v1.37.2. LOCKED D6 disposition confirmed.**

**`validateMutationHeaders` authored but unconsumed:** D7 rule set authored in `attributes.go` — `:` pseudo-headers REJECTED; invalid RFC 7230 token chars in header name REJECTED; bare CR/LF/NUL in header value REJECTED. Group 3 unit tests cover it. NOT wired into the disposition path — Task 5 (ADR-0161) wires it.

**ADR-0160 fill:** §Status updated from "Anticipated" to "Accepted (HTTP-mode portion)"; **Lands-in:** updated from "Task 4 (hypothesis)" to "Task 4 of phase-18.1 PLAN (HTTP-mode portion)"; §Decision: 6-point body (i)–(vi) covering `stringMatcherList` shape, D5 regex-subset confirmation, D6 deprecated-field confirmation, header-name lowercased matching convention, `buildAuthRequest` signature, `validateMutationHeaders` authored-but-unconsumed; §Consequences: 5 bullets covering `attributes.go` LoC, `extauthz.go` updates, D5/D6 confirmations, D7 authoring, gRPC-mode portion deferred.

**Outputs:**

### Test run — Group 3 (failing before implementation — RED)

```
$ go test ./internal/filter/http/extauthz/ -run 'TestCompileStringMatcherList|TestBuildAuthRequest|TestValidateMutation' -v 2>&1 | head -20
# github.com/esalaine/envoy-go/internal/filter/http/extauthz [github.com/esalaine/envoy-go/internal/filter/http/extauthz.test]
internal/filter/http/extauthz/extauthz_test.go:1581:9: undefined: buildAuthRequest
internal/filter/http/extauthz/extauthz_test.go:1591:17: assignment mismatch: 2 variables but compileStringMatcherList returns 1 value
...
FAIL	github.com/esalaine/envoy-go/internal/filter/http/extauthz [build failed]
```

### Test run — Group 3 verbose after implementation (GREEN, 31 test functions)

```
$ go test ./internal/filter/http/extauthz/ -run 'TestCompileStringMatcherList|TestBuildAuthRequest|TestValidateMutation' -v
=== RUN   TestCompileStringMatcherList_NilInput
--- PASS: TestCompileStringMatcherList_NilInput (0.00s)
=== RUN   TestCompileStringMatcherList_Exact
--- PASS: TestCompileStringMatcherList_Exact (0.00s)
=== RUN   TestCompileStringMatcherList_ExactIgnoreCase
--- PASS: TestCompileStringMatcherList_ExactIgnoreCase (0.00s)
=== RUN   TestCompileStringMatcherList_Prefix
--- PASS: TestCompileStringMatcherList_Prefix (0.00s)
=== RUN   TestCompileStringMatcherList_PrefixIgnoreCase
--- PASS: TestCompileStringMatcherList_PrefixIgnoreCase (0.00s)
=== RUN   TestCompileStringMatcherList_Suffix
--- PASS: TestCompileStringMatcherList_Suffix (0.00s)
=== RUN   TestCompileStringMatcherList_SuffixIgnoreCase
--- PASS: TestCompileStringMatcherList_SuffixIgnoreCase (0.00s)
=== RUN   TestCompileStringMatcherList_Contains
--- PASS: TestCompileStringMatcherList_Contains (0.00s)
=== RUN   TestCompileStringMatcherList_ContainsIgnoreCase
--- PASS: TestCompileStringMatcherList_ContainsIgnoreCase (0.00s)
=== RUN   TestCompileStringMatcherList_SafeRegex_GoogleRE2
--- PASS: TestCompileStringMatcherList_SafeRegex_GoogleRE2 (0.00s)
=== RUN   TestCompileStringMatcherList_SafeRegex_InvalidRegex
--- PASS: TestCompileStringMatcherList_SafeRegex_InvalidRegex (0.00s)
=== RUN   TestCompileStringMatcherList_SafeRegex_NilRegexMatcher
--- PASS: TestCompileStringMatcherList_SafeRegex_NilRegexMatcher (0.00s)
=== RUN   TestCompileStringMatcherList_Custom_ParseReject
--- PASS: TestCompileStringMatcherList_Custom_ParseReject (0.00s)
=== RUN   TestCompileStringMatcherList_MultiplePatterns
--- PASS: TestCompileStringMatcherList_MultiplePatterns (0.00s)
=== RUN   TestCompileStringMatcherList_EmptyPatterns
--- PASS: TestCompileStringMatcherList_EmptyPatterns (0.00s)
=== RUN   TestBuildAuthRequest_NilAllowedHeaders_AllPass
--- PASS: TestBuildAuthRequest_NilAllowedHeaders_AllPass (0.00s)
=== RUN   TestBuildAuthRequest_AllowedHeaders_FiltersHeaders
--- PASS: TestBuildAuthRequest_AllowedHeaders_FiltersHeaders (0.00s)
=== RUN   TestBuildAuthRequest_DisallowedHeaders_OverridesAllowed
--- PASS: TestBuildAuthRequest_DisallowedHeaders_OverridesAllowed (0.00s)
=== RUN   TestBuildAuthRequest_DisallowedHeaders_NilAllowed
--- PASS: TestBuildAuthRequest_DisallowedHeaders_NilAllowed (0.00s)
=== RUN   TestBuildAuthRequest_HeadersToAdd_Appended
--- PASS: TestBuildAuthRequest_HeadersToAdd_Appended (0.00s)
=== RUN   TestBuildAuthRequest_DeprecatedAllowedHeaders_HonoredIfPresent
--- PASS: TestBuildAuthRequest_DeprecatedAllowedHeaders_HonoredIfPresent (0.00s)
=== RUN   TestBuildAuthRequest_TopLevelAllowedHeadersTakesPrecedence
--- PASS: TestBuildAuthRequest_TopLevelAllowedHeadersTakesPrecedence (0.00s)
=== RUN   TestBuildAuthRequest_PathCarried
--- PASS: TestBuildAuthRequest_PathCarried (0.00s)
=== RUN   TestBuildAuthRequest_BodyIncluded
--- PASS: TestBuildAuthRequest_BodyIncluded (0.00s)
=== RUN   TestValidateMutationHeaders_ValidHeaders
--- PASS: TestValidateMutationHeaders_ValidHeaders (0.00s)
=== RUN   TestValidateMutationHeaders_PseudoHeaderReject
--- PASS: TestValidateMutationHeaders_PseudoHeaderReject (0.00s)
=== RUN   TestValidateMutationHeaders_InvalidHeaderNameChars
--- PASS: TestValidateMutationHeaders_InvalidHeaderNameChars (0.00s)
=== RUN   TestValidateMutationHeaders_InvalidHeaderValueChars
--- PASS: TestValidateMutationHeaders_InvalidHeaderValueChars (0.00s)
=== RUN   TestValidateMutationHeaders_EmptySlice
--- PASS: TestValidateMutationHeaders_EmptySlice (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.003s
```

### Test run — full suite with race detector

```
$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.070s
```

83 tests PASS (up from 52 at Task 3 end). 0 failures.

### Test run — go vet

```
$ go vet ./internal/filter/http/extauthz/...
(no output — exit 0)
```

### Test run — gofmt

```
$ gofmt -l internal/filter/http/extauthz/
(no output — empty)
```

### Test run — full suite (48 packages, short mode)

```
$ go test -count=1 -short ./... 2>&1 | grep -cE '^ok'
48

$ go test -count=1 -short ./... 2>&1 | grep -cE '^(FAIL|---\s+FAIL)'
0
```

### ADR acceptance-criteria grep

```
$ grep -nE '^## ADR-0160' docs/envoy-go/DECISIONS.md
8479:## ADR-0160: `AttributeContext` / `AuthorizationRequest` builder — ...
```

1 match (1 required). §Decision (6-point body (i)–(vi)) + §Consequences (5 bullets) filled. Status: Accepted (HTTP-mode portion); Date: 2026-05-14; Lands-in: Task 4 of phase-18.1 PLAN.
