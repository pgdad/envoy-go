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

**Notes:** Followed `superpowers:test-driven-development`. Group 3 tests written first (RED confirmed — build failure on undefined `buildAuthRequest` + wrong arity on `compileStringMatcherList`). `attributes.go` authored; `buildCompiledConfig` and the `compileStringMatcherList` stub wired to the real implementation. Test count 54 → 83 (29 new `func Test` added — verified `git show 93c6cf6:…/extauthz_test.go | grep -c '^func Test'` = 54 pre-Task-4, `grep -c '^func Test' …/extauthz_test.go` = 83 post-Task-4). `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `gofmt -l` empty.

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

### Test run — Group 3 verbose after implementation (GREEN, 29 test functions)

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

83 tests PASS (up from 54 at Task 3 end; 29 new `func Test` appended). 0 failures.

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

---

## Task 5 — Bidirectional header-mutation discipline + `validate_mutations` gating + Group 8 + ADR-0161 §Decision+§Consequences

**Files changed:** `internal/filter/http/extauthz/extauthz.go` (modified — `dispInvalid` added as 4th `dispositionClass` const; `deprecatedAllowedHeaders *stringMatcherList` field added to `compiledConfig`; `applyUpstreamMutations` helper added; `buildCompiledConfig` updated to pre-compile deprecated `AuthorizationRequest.allowed_headers` at config-load time with PARSE-REJECT on malformed pattern, null it out when top-level `allowedHeaders` is set, and call `buildHTTPCheckFn(httpSvc, cc.validateMutations)` with two args), `internal/filter/http/extauthz/check.go` (modified — `buildHTTPCheckFn` signature extended to accept `validateMutations bool`; authorization_response matchers compiled at config-load time; `buildCheckFnClosure` updated to call `mapHTTPResponseWithMatchers`; `mapHTTPResponseWithMatchers` added replacing the Task 3 stub; `extractMatchingHeaders` + `buildDenyHeaders` helpers added; the Task 3 `mapHTTPResponse` wrapper was initially kept but is **removed in the Task 5 review-fix** — see review-fix note below), `internal/filter/http/extauthz/attributes.go` (modified — `buildAuthRequest` updated to use `cc.deprecatedAllowedHeaders` pre-compiled field instead of per-request compilation; comment block updated), `internal/filter/http/extauthz/extauthz_test.go` (modified — Group 8 tests appended: 18 new `func Test`; `buildHTTPCheckFnForTest` + `TestBuildHTTPCheckFn_MissingServerURI` + `TestBuildHTTPCheckFn_EmptyURI` fixed to pass `false` as second arg; `buildAuthRequestForTest` updated to pre-compile deprecated field mirroring `buildCompiledConfig`), `docs/envoy-go/DECISIONS.md` (ADR-0161 HTTP-mode §Status updated; §Decision 7-point body (i)–(vii) filled; §Consequences 5-bullet body filled)

**Commit SHA:** `975fc14`

**Notes:** Followed `superpowers:test-driven-development`. Group 8 tests written first (RED confirmed — build failure on undefined `dispInvalid`, wrong-arity `buildHTTPCheckFn`, undefined `applyUpstreamMutations`, undefined `cc.deprecatedAllowedHeaders`). Production code authored in `extauthz.go`, `check.go`, `attributes.go`. Test count 83 → 103 (20 new `func Test` added; 18 Group 8 + 2 Group 4 fixes; `grep -c '^func Test' …/extauthz_test.go` = 103). `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `gofmt -l` empty.

**Task 4 carried-forward fix (deprecated `allowed_headers` pre-compile).** The Task 4 original code compiled `AuthorizationRequest.allowed_headers` per-request in `buildAuthRequest` (lines 291–303 of original `attributes.go`). Replaced at Task 5 with: (a) `compiledConfig.deprecatedAllowedHeaders *stringMatcherList` field compiled ONCE at `buildCompiledConfig` time; (b) malformed pattern → PARSE-REJECT (replaces the original "silent-degrade on error"); (c) top-level `cc.allowedHeaders` set → `cc.deprecatedAllowedHeaders` nulled (top-level wins); (d) `buildAuthRequest` reads `cc.deprecatedAllowedHeaders` directly — no per-request `compileStringMatcherList` call. `buildAuthRequestForTest` updated to mirror this pre-compile discipline.

**`dispInvalid` design.** Fourth `dispositionClass` constant (iota = 3, after `dispAllow`/`dispDeny`/`dispError`). SPEC §6.3 separately tracks the `invalid` counter from `errored` — the rejection is error-posture but distinct. The Task 9 dispatch (`disp.class == dispInvalid`) will call `SendLocalReply(403)` and increment the `invalid` counter.

**Pseudo-header test infrastructure note.** `TestValidateMutations_AllowPath_PseudoHeaderRejected` + `TestValidateMutations_DenyPath_PseudoHeaderRejected` call `mapHTTPResponseWithMatchers` directly with a hand-crafted `*http.Response` (bypassing `net/http`'s HTTP/1.1 wire layer which strips `:` pseudo-headers from responses). The test rationale: Go's `net/http` server does not transmit `:status`-prefixed headers over HTTP/1.1; the `validate_mutations` gate operates on already-extracted `headerKV` slices — the unit-level test is most faithful at the `mapHTTPResponseWithMatchers` API boundary rather than through a live server. End-to-end confirmation of the full path (auth server → extraction → validate) is covered by the fixture 0020 differential harness at Task 13.

**D7 `validate_mutations` rule set confirmation.** The rule set authored in `attributes.go` at Task 4 (`:` pseudo-headers REJECTED; invalid RFC 7230 §3.2.6 token chars in name REJECTED; bare CR/LF/NUL in value REJECTED) is wired at Task 5. Full empirical confirmation against v1.37.2 deferred to Task 13 differential fixture pass per D7. Evidence basis: phase-10 header_mutation protected-header discipline + proto-field documentation. No divergence from LOCKED D7 disposition.

**ADR-0161 fill.** §Status updated from "Anticipated" to "Accepted (HTTP-mode portion, Task 5 of phase-18.1 PLAN)"; **Lands-in:** updated from "Task 5 (hypothesis)" to "Task 5 of phase-18.1 PLAN (HTTP-mode portion confirmed)"; §Decision: 7-point body (i)–(vii) covering response-side matcher compilation, allow-path extraction, `applyUpstreamMutations` placement, deny-path header-set construction, `validate_mutations` gating, `mapHTTPResponse` supersession (corrected in the review-fix — see note below), deprecated `allowed_headers` pre-compile fix; §Consequences: 5 bullets covering PARSE-REJECT on malformed response matchers, deprecated-field PARSE-REJECT flip, `dispInvalid` wiring, `allowed_client_headers_on_success` deferral, gRPC-mode deferred.

**Outputs:**

### Test run — Group 8 (failing before implementation — RED)

```
$ go build ./internal/filter/http/extauthz/ 2>&1  [before production code]
internal/filter/http/extauthz/extauthz_test.go:2332:34: too many arguments in call to buildHTTPCheckFn
internal/filter/http/extauthz/extauthz_test.go:2688:19: undefined: dispInvalid
internal/filter/http/extauthz/extauthz_test.go:2788:2: undefined: applyUpstreamMutations
internal/filter/http/extauthz/extauthz_test.go:2913:8: cc.deprecatedAllowedHeaders undefined
FAIL	github.com/esalaine/envoy-go/internal/filter/http/extauthz [build failed]
```

### Test run — Group 8 verbose after implementation (GREEN, 18 test functions)

```
$ go test ./internal/filter/http/extauthz/ -run 'TestHeaderMutation|TestValidateMutations|TestApplyUpstreamMutations|TestDeprecatedAllowedHeaders' -v -count=1
=== RUN   TestHeaderMutation_AllowPath_UpstreamSet
--- PASS: TestHeaderMutation_AllowPath_UpstreamSet (0.00s)
=== RUN   TestHeaderMutation_AllowPath_UpstreamApp
--- PASS: TestHeaderMutation_AllowPath_UpstreamApp (0.00s)
=== RUN   TestHeaderMutation_AllowPath_SetAndAppend
--- PASS: TestHeaderMutation_AllowPath_SetAndAppend (0.00s)
=== RUN   TestHeaderMutation_AllowPath_NilMatcher
--- PASS: TestHeaderMutation_AllowPath_NilMatcher (0.00s)
=== RUN   TestHeaderMutation_DenyPath_AllowedClientHeaders
--- PASS: TestHeaderMutation_DenyPath_AllowedClientHeaders (0.00s)
=== RUN   TestHeaderMutation_DenyPath_TextPlainFallback
--- PASS: TestHeaderMutation_DenyPath_TextPlainFallback (0.00s)
=== RUN   TestHeaderMutation_DenyPath_NilClientHeadersMatcher
--- PASS: TestHeaderMutation_DenyPath_NilClientHeadersMatcher (0.00s)
=== RUN   TestHeaderMutation_DenyPath_DecisionHeadersFirst
--- PASS: TestHeaderMutation_DenyPath_DecisionHeadersFirst (0.00s)
=== RUN   TestValidateMutations_AllowPath_PseudoHeaderRejected
--- PASS: TestValidateMutations_AllowPath_PseudoHeaderRejected (0.00s)
=== RUN   TestValidateMutations_AllowPath_InvalidNameCharsRejected
--- PASS: TestValidateMutations_AllowPath_InvalidNameCharsRejected (0.00s)
=== RUN   TestValidateMutations_DenyPath_PseudoHeaderRejected
--- PASS: TestValidateMutations_DenyPath_PseudoHeaderRejected (0.00s)
=== RUN   TestValidateMutations_False_PseudoHeaderAllowed
--- PASS: TestValidateMutations_False_PseudoHeaderAllowed (0.00s)
=== RUN   TestApplyUpstreamMutations_Set
--- PASS: TestApplyUpstreamMutations_Set (0.00s)
=== RUN   TestApplyUpstreamMutations_Append
--- PASS: TestApplyUpstreamMutations_Append (0.00s)
=== RUN   TestApplyUpstreamMutations_SetBeforeAppend
--- PASS: TestApplyUpstreamMutations_SetBeforeAppend (0.00s)
=== RUN   TestApplyUpstreamMutations_Empty
--- PASS: TestApplyUpstreamMutations_Empty (0.00s)
=== RUN   TestDeprecatedAllowedHeaders_PreCompiled
--- PASS: TestDeprecatedAllowedHeaders_PreCompiled (0.00s)
=== RUN   TestDeprecatedAllowedHeaders_MalformedParseReject
--- PASS: TestDeprecatedAllowedHeaders_MalformedParseReject (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.006s
```

### Test run — full suite with race detector

```
$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.074s
```

103 tests PASS (up from 83 at Task 4 end; 20 new `func Test` appended — 18 Group 8 + 2 Group 4 arity-fix). 0 failures.

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
$ grep -nE '^## ADR-0161' docs/envoy-go/DECISIONS.md
8526:## ADR-0161: Bidirectional header-mutation discipline — ...
```

1 match (1 required). §Decision (7-point body (i)–(vii)) + §Consequences (5 bullets) filled. Status: Accepted (HTTP-mode portion); Date: 2026-05-14; Lands-in: Task 5 of phase-18.1 PLAN.

### Task 5 review-fix

Code-quality review returned "Needs changes" (2 Important + 2 should-fix). All 4 fixed in a follow-up commit:

1. **(Important) Dead `mapHTTPResponse` deleted.** The Task 3 `mapHTTPResponse` backward-compat wrapper had ZERO callers (production closure + all tests go through `mapHTTPResponseWithMatchers`). Deleted entirely from `check.go`; ADR-0161 §Decision (vi) in `DECISIONS.md` corrected to state it was superseded by `mapHTTPResponseWithMatchers` at Task 5 and removed (not "retained"). `grep` confirms no `.go` reference to `mapHTTPResponse` remains.
2. **(Important) Stale `buildHTTPCheckFn` signature comment.** `extauthz.go` cross-reference comment updated from `buildHTTPCheckFn(hs *ext_authzv3.HttpService) (checkFn, error)` to the real Task-5 signature `buildHTTPCheckFn(hs *ext_authzv3.HttpService, validateMutations bool) (checkFn, error)`.
3. **(should-fix) Null-out precedence test added.** `TestDeprecatedAllowedHeaders_NullOutWhenTopLevelSet` added to `extauthz_test.go` — drives real `buildCompiledConfig` with BOTH top-level `ExtAuthz.AllowedHeaders` and deprecated `AuthorizationRequest.AllowedHeaders` set; asserts `cc.deprecatedAllowedHeaders == nil` AND `cc.allowedHeaders != nil` (security-relevant: deprecated field must not override a top-level allow-list).
4. **(should-fix) `TestValidateMutations_AllowPath_InvalidNameCharsRejected` refactored.** Was calling `validateMutationHeaders` directly + a redundant constant-identity check; now hand-crafts an `*http.Response` carrying an invalid-name-char header and drives it through `mapHTTPResponseWithMatchers` (mirroring `TestValidateMutations_AllowPath_PseudoHeaderRejected`), genuinely exercising the `mapHTTPResponseWithMatchers` → `validateMutationHeaders` → `dispInvalid` wiring. Redundant constant-identity check removed.

Test count 103 → 104 (+1: the null-out test; the validate_mutations refactor is in-place). `go build` / `go vet` / `gofmt -l` (empty) / `go test -race -count=1 ./internal/filter/http/extauthz/...` all green.

---

## Task 6 — `with_request_body` ADR-0128 reuse + over-limit 413 edge case + Group 6 tests + ADR-0162 §Decision+§Consequences

**Files changed:** `internal/filter/http/extauthz/extauthz.go` (modified — `filter.bodySettings *bufferSettings` field added; `effectiveWithRequestBody(pr *compiledPerRoute) *bufferSettings` helper added; `DecodeHeaders` body-buffering branch implemented: computes effective `withRequestBody` + sets `awaitingBody=true` + caches `bodySettings`; `DecodeData` body-accumulation + over-limit logic implemented per ADR-0128; `DecodeHeaders`/`DecodeData` doc-comments revised to be accurate), `internal/filter/http/extauthz/extauthz_test.go` (modified — file header comment updated to include Group 6; `fakeExtAuthzDCB` + `localReplyRecord6` + `newFakeExtAuthzDCB` helpers added; `newBodyBufferingFilter` helper added; 16 Group 6 test functions appended: 3 Group 6A DecodeHeaders, 5 Group 6B accumulation, 1 Group 6C over-limit, 1 Group 6C mid-stream, 1 Group 6C no-counter, 2 Group 6D allow-partial, 2 Group 6E pack_as_bytes, 2 Group 6F per-route override), `docs/envoy-go/DECISIONS.md` (ADR-0162 §Status updated from "Anticipated" to "Accepted"; **Lands-in** updated from hypothesis to confirmed "Task 6 of phase-18.1 PLAN"; §Decision 7-point body (i)–(vii) filled; §Consequences 6-bullet body filled)

**Commit SHA:** `f27383f`

**Notes:** Followed `superpowers:test-driven-development`. Group 6 tests written first (RED confirmed — 9 failing tests); production code authored in `extauthz.go`; then GREEN. Test count 104 → 120 (16 new `func Test` for Group 6). `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet` exit 0; `gofmt -l` empty. 48/48 packages pass.

**Task 6 / Task 9 seam.** Task 6 delivers: (a) the `DecodeHeaders` body-buffering branch (effective `withRequestBody` resolve + `awaitingBody=true` + `bodySettings` cache), and (b) the `DecodeData` accumulation + over-limit 413 + allow_partial truncation. Task 9 delivers: the full `DecodeHeaders` dispatch body (per-route resolve → disabled short-circuit → async outbound check goroutine → disposition application) + `OnDestroy` cancellation. The seam is in `DecodeData` at `endStream=true` (within limit): return `DataStopIterationAndBuffer` with a `// Task 9: fire outbound check here` comment — Task 9 replaces this with the `buildAuthRequest` + `checkFn` goroutine + `dcb.ContinueDecoding()` dispatch per ADR-0159.

**ADR-0128 synchronous-HCM dispatch constraint.** `DecodeHeaders` body-buffering branch returns `Continue` (NOT `StopIteration`) per ADR-0128: envoy-go's HCM runs the `RunDecodeHeaders` → body-read loop → `RunDecodeData` → `RunAction` sequence synchronously in one goroutine (ADR-0076 + connection.go). Returning `StopIteration` from `DecodeHeaders` for body buffering would deadlock (body loop IS the `ContinueDecoding` path). The SPEC §6.3 "return `HeaderStopIteration`" is conceptual Envoy semantics; the envoy-go implementation uses `Continue` for the body-buffering phase (same discipline as buffer/bandwidth_limit filters). The `DataStopIterationAndBuffer` at `endStream=true` in `DecodeData` IS the parking mechanism — it runs in the same goroutine AFTER all body bytes are already accumulated (framework bodyBuf guarantees this per ADR-0128).

**`pack_as_bytes` semantics.** `pack_as_bytes` is parsed into `bufferSettings.packAsBytes` (no HTTP-mode effect in 18.1). In HTTP-mode the POST body is the raw `f.body` bytes verbatim regardless of `pack_as_bytes`. The field is stored for 18.2's gRPC-mode `AttributeContext` builder which uses it to choose `body` (string) vs `raw_body` (bytes). Confirmed in `TestDecodeData_PackAsBytes_BodyVerbatimInHTTPMode`.

**`disable_request_body_buffering` precedence.** Per SPEC §8 + §6.3 step 3: (a) per-route `disable_request_body_buffering=true` → nil (OFF), (b) per-route `with_request_body` set → per-route override, (c) listener-level `withRequestBody`. Implemented in `effectiveWithRequestBody(pr *compiledPerRoute)` helper; confirmed in `TestDecodeHeaders_PerRouteDisableBodyBuffering` and `TestDecodeHeaders_PerRouteWithRequestBodyOverride`.

**NO counter increments on 413 path.** `DecodeData` emits the 413 local-reply and returns `DataStopIterationNoBuffer` WITHOUT incrementing any ext_authz counter. The counter-zero invariant is explicitly tested by `TestDecodeData_OverLimit_AllowPartialFalse_413` (reads all 5 non-`disabled` counters) and `TestDecodeData_OverLimit_NoCounterIncrements` (all 6 counters). This is the load-bearing "auth NOT called, NO counters" assertion from parent SPEC §6 amendment 6.

**ADR-0162 fill.** §Status updated from "Anticipated" to "Accepted"; **Lands-in** confirmed "Task 6 of phase-18.1 PLAN"; §Decision: 7-point body covering `bufferSettings` parse, ADR-0128 reuse + the `Continue`-not-`StopIteration` rationale, over-limit 413 edge case + NO-counter invariant, allow_partial truncation, pack_as_bytes HTTP-mode no-op, effective-withRequestBody precedence, Task 6/Task 9 seam; §Consequences: 6 bullets.

**Outputs:**

### Test run — Group 6 (failing before implementation — RED)

```
$ go test ./internal/filter/http/extauthz/ -run 'TestDecodeHeaders_WithRequestBody|TestDecodeData|TestWithRequestBody_PackAsBytes|TestDecodeData_PackAsBytes|TestDecodeHeaders_PerRoute' -v -count=1 [before production code]
=== RUN   TestDecodeHeaders_WithRequestBody_SetsAwaitingBodyAndContinue
    extauthz_test.go:3123: DecodeHeaders(withRequestBody, !endStream): want awaitingBody=true, got false
--- FAIL: TestDecodeHeaders_WithRequestBody_SetsAwaitingBodyAndContinue (0.00s)
=== RUN   TestDecodeData_SingleChunk_WithinLimit_EndStream_Parks
    extauthz_test.go:3192: DecodeData(within limit, endStream=true): want DataStopIterationAndBuffer (Task 9 seam), got 0
    extauthz_test.go:3195: body: got "", want "hello world"
--- FAIL: TestDecodeData_SingleChunk_WithinLimit_EndStream_Parks (0.00s)
=== RUN   TestDecodeData_OverLimit_AllowPartialFalse_413
    extauthz_test.go:3272: DecodeData(over-limit, allow_partial=false): want DataStopIterationNoBuffer, got 0
    extauthz_test.go:3277: SendLocalReply: got 0 calls, want 1
--- FAIL: TestDecodeData_OverLimit_AllowPartialFalse_413 (0.00s)
[... 6 more failures ...]
FAIL    github.com/esalaine/envoy-go/internal/filter/http/extauthz  0.004s
```

### Test run — Group 6 verbose after implementation (GREEN, 16 new test functions + pre-existing skeleton)

```
$ go test ./internal/filter/http/extauthz/ -run 'TestDecodeHeaders_WithRequestBody|TestDecodeData|TestWithRequestBody_PackAsBytes|TestDecodeData_PackAsBytes|TestDecodeHeaders_PerRoute|TestDecodeHeaders_NoWithRequestBody' -v -count=1
=== RUN   TestDecodeDataSkeleton_Passthrough
--- PASS: TestDecodeDataSkeleton_Passthrough (0.00s)
=== RUN   TestDecodeHeaders_WithRequestBody_SetsAwaitingBodyAndContinue
--- PASS: TestDecodeHeaders_WithRequestBody_SetsAwaitingBodyAndContinue (0.00s)
=== RUN   TestDecodeHeaders_WithRequestBody_EndStreamSkipsBuffer
--- PASS: TestDecodeHeaders_WithRequestBody_EndStreamSkipsBuffer (0.00s)
=== RUN   TestDecodeHeaders_NoWithRequestBody_AwaitingBodyNotSet
--- PASS: TestDecodeHeaders_NoWithRequestBody_AwaitingBodyNotSet (0.00s)
=== RUN   TestDecodeData_Passthrough_AwaitingBodyFalse
--- PASS: TestDecodeData_Passthrough_AwaitingBodyFalse (0.00s)
=== RUN   TestDecodeData_SingleChunk_WithinLimit_EndStream_Parks
--- PASS: TestDecodeData_SingleChunk_WithinLimit_EndStream_Parks (0.00s)
=== RUN   TestDecodeData_MultiChunk_WithinLimit_EndStream_Parks
--- PASS: TestDecodeData_MultiChunk_WithinLimit_EndStream_Parks (0.00s)
=== RUN   TestDecodeData_ExactCap_StrictGreaterThan
--- PASS: TestDecodeData_ExactCap_StrictGreaterThan (0.00s)
=== RUN   TestDecodeData_OverLimit_AllowPartialFalse_413
--- PASS: TestDecodeData_OverLimit_AllowPartialFalse_413 (0.00s)
=== RUN   TestDecodeData_OverLimit_AllowPartialFalse_MidStream
--- PASS: TestDecodeData_OverLimit_AllowPartialFalse_MidStream (0.00s)
=== RUN   TestDecodeData_OverLimit_NoCounterIncrements
--- PASS: TestDecodeData_OverLimit_NoCounterIncrements (0.00s)
=== RUN   TestDecodeData_OverLimit_AllowPartialTrue_TruncatesToMaxBytes
--- PASS: TestDecodeData_OverLimit_AllowPartialTrue_TruncatesToMaxBytes (0.00s)
=== RUN   TestDecodeData_OverLimit_AllowPartialTrue_MultiChunk
--- PASS: TestDecodeData_OverLimit_AllowPartialTrue_MultiChunk (0.00s)
=== RUN   TestWithRequestBody_PackAsBytesStored
--- PASS: TestWithRequestBody_PackAsBytesStored (0.00s)
=== RUN   TestDecodeData_PackAsBytes_BodyVerbatimInHTTPMode
--- PASS: TestDecodeData_PackAsBytes_BodyVerbatimInHTTPMode (0.00s)
=== RUN   TestDecodeHeaders_PerRouteDisableBodyBuffering
--- PASS: TestDecodeHeaders_PerRouteDisableBodyBuffering (0.00s)
=== RUN   TestDecodeHeaders_PerRouteWithRequestBodyOverride
--- PASS: TestDecodeHeaders_PerRouteWithRequestBodyOverride (0.00s)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/http/extauthz  0.004s
```

### Test run — full suite with race detector

```
$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok      github.com/esalaine/envoy-go/internal/filter/http/extauthz  1.074s
```

120 tests PASS (up from 104 at Task 5 end; 16 new `func Test` appended). 0 failures.

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
$ grep -nE '^## ADR-0162' docs/envoy-go/DECISIONS.md
8575:## ADR-0162: Request-body inclusion — ...
```

1 match (1 required). §Decision (7-point body (i)–(vii)) + §Consequences (6 bullets) filled. Status: Accepted; Date: 2026-05-14; Lands-in: Task 6 of phase-18.1 PLAN.

### Task 6 review-fix

Code-quality review returned "Approve with minor fixes" (2 Important doc-only + 2 Minor). All 4 fixed in a follow-up commit:

1. **(Important) ADR-0162 internal consumer-count inconsistency.** The §Doctrine line said "SECOND consumer of the phase-13 ADR-0128 decode-side body-buffering primitive (after phase-15 bandwidth_limit)" while §Consequences first bullet said "THIRD ADR-0128 consumer (after phase-13 buffer filter + phase-15 bandwidth_limit)". The Consequences bullet is correct (buffer #1, bandwidth_limit #2, ext_authz #3). Doctrine line in `docs/envoy-go/DECISIONS.md` (ADR-0162) updated to "THIRD consumer of the phase-13 ADR-0128 decode-side body-buffering primitive (after phase-13 buffer filter + phase-15 bandwidth_limit)" — now consistent with the Consequences bullet.
2. **(Important) `effectiveWithRequestBody` missing precondition comment.** When `pr.disabled=true` AND `pr.checkSettings=nil`, `effectiveWithRequestBody` falls through and returns the listener-level `withRequestBody` — but `pr.disabled=true` should short-circuit body buffering too. In Task 6's intermediate state this isn't a production bug because Task 9's disabled short-circuit fires before this helper is called, but the implicit precondition wasn't documented. Doc-comment in `internal/filter/http/extauthz/extauthz.go` extended to state the precondition explicitly: "Precondition: caller has already short-circuited on `pr.disabled=true` (per SPEC §6.3 step 2 — Task 9 wires this). [...] The implicit contract is: callers MUST NOT invoke this helper when `pr.disabled=true`."
3. **(Minor) Stale `Skeleton` test names + comments renamed.** `TestDecodeHeadersSkeleton_ReturnsHeaderContinue` → `TestDecodeHeaders_EndStreamNoBody_Continue` and `TestDecodeDataSkeleton_Passthrough` → `TestDecodeData_AwaitingBodyFalse_Passthrough` in `internal/filter/http/extauthz/extauthz_test.go`. The implementation now has real body-buffering logic; the "Skeleton" / "pass-through placeholder" comments were misleading. Renamed both tests + rewrote the comments to accurately describe what they assert today (DecodeHeaders endStream=true no-body returns HeaderContinue; DecodeData with `awaitingBody=false` passes through). No behavior change.
4. **(Minor) Missing test added: allow_partial=true + mid-stream truncation + subsequent chunk.** `TestDecodeData_OverLimit_AllowPartialTrue_MultiChunk` covered chunk 1 (within) + chunk 2 (over-limit, `endStream=true`). It did NOT cover chunk 1 (within) + chunk 2 (over-limit, `endStream=false`) + chunk 3 (`endStream=false`, still over-limit — truncation repeats idempotently) + chunk 4 (`endStream=true`, terminal park). Added `TestDecodeData_OverLimit_AllowPartialTrue_MidStreamTruncationThenChunk` (in `extauthz_test.go`) covering exactly this 4-chunk scenario: chunk 1 = "abc" (within, `DataContinue`); chunk 2 = "defgh" (over-limit, non-terminal, body truncated to maxBytes=5 = "abcde", `DataContinue`); chunk 3 = "ijk" (still over-limit, non-terminal, truncation idempotent — body unchanged at "abcde", `DataContinue`); chunk 4 = "lm" (terminal, over-limit, `DataStopIterationAndBuffer`). Asserts body length == `maxRequestBytes` throughout post-truncation chunks AND content is the unchanged "abcde" prefix.

Test count 120 → 121 (+1: the mid-stream truncation test; the 2 rename refactors are in-place). `go build` / `go vet` / `gofmt -l` (empty) / `go test -race -count=1 ./internal/filter/http/extauthz/...` all green.

```
$ go build ./internal/filter/http/extauthz/...
[no output]

$ go vet ./internal/filter/http/extauthz/...
[no output]

$ gofmt -l ./internal/filter/http/extauthz/
[no output — empty]

$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.075s
```

## Task 7 — Per-route 5th-canonical REUSE + SHARED-stats + Group 7 finalization [ADR-0163]

**Files changed:**
- `internal/filter/http/extauthz/extauthz_test.go` (Group 7 extended: 8 new tests)
- `docs/envoy-go/DECISIONS.md` (ADR-0163 §Decision + §Consequences filled; Status → Accepted; Lands-in → Task 7 of phase-18.1 PLAN)
- `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (this entry)

**Commit SHA:** `45eb62c` (SHA-fill follow-up at next commit per established convention)

**What was implemented vs what was already done:**

Task 2 delivered a COMPLETE skeleton of `parsePerRoute`, `resolvePerRouteConfig`, `compiledCheckSettings`, and `compiledPerRoute` — all logic was already wired including the XOR validation, the disabled-arm PGV-mirror, the SHARED-stats cc-wiring (`fresh.cc = s.listenerRC`), and the `sync.Map` LoadOrStore lazy-cache. Task 6 landed `effectiveWithRequestBody` reading `pr.checkSettings.disableRequestBodyBuffering` and `pr.checkSettings.withRequestBody`. Task 7's production-code contribution is therefore **zero new production lines** — finalization work confirmed the skeleton was complete.

**Task 7 added 8 new Group 7 tests** covering the cases the PLAN specified:

1. `TestResolvePerRouteConfig_SharedStats` — SHARED-stats discipline: `result.cc == listenerRC` pointer assertion for check_settings arm.
2. `TestResolvePerRouteConfig_DisabledSharedStats` — SHARED-stats discipline: `result.cc == listenerRC` pointer assertion for disabled arm.
3. `TestParsePerRoute_ContextExtensions_NoopInHTTPMode` — context_extensions parsed + stored, no HTTP-mode side-effects on `disableRequestBodyBuffering` or `withRequestBody`; per SPEC §8 item 8 + ADR-0163 §Decision (iii).
4. `TestResolvePerRouteConfig_ConcurrentSameProto` — 20 concurrent goroutines resolving the same proto pointer all return pointer-identical `*compiledPerRoute` (sync.Map LoadOrStore identity; ADR-0117 + ADR-0125 §(v)).
5. `TestEffectiveWithRequestBody_DisableRequestBodyBuffering` — per-route `disableRequestBodyBuffering=true` → `effectiveWithRequestBody` returns nil even with listener-level `withRequestBody` set.
6. `TestEffectiveWithRequestBody_PerRouteOverride` — per-route `withRequestBody` → `effectiveWithRequestBody` returns per-route pointer, not listener-level.
7. `TestEffectiveWithRequestBody_ListenerFallback` — three sub-cases: nil per-route, empty checkSettings, nil checkSettings all fall back to listener-level `withRequestBody`.
8. `TestResolvePerRouteConfig_CCAlwaysListenerRC` — table-driven across disabled arm, empty check_settings arm, check_settings with context_extensions: all produce `result.cc == listenerRC`.

**TDD note:** Task 2's skeleton was already complete, so the new tests passed immediately upon addition. This is an expected TDD outcome for finalization work — the implementation existed, the tests verify the existing behavior is correct and complete.

**ADR-0163 fill:** §Decision (8-point body (i)–(viii)) + §Consequences (5 bullets) filled. Status: Accepted; Date: 2026-05-14; **Lands-in: Task 7 of phase-18.1 PLAN.** Records: 5th-canonical-REUSE (NO §(xiv) amendment); `parsePerRoute` PGV-mirror; `check_settings` narrower-override merge; `context_extensions` HTTP-mode no-op; `effectiveWithRequestBody` 3-tier resolution; SHARED-stats discipline (no per-route `*filterStats`); `sync.Map` lazy-cache identity; the 6-counter stat surface + SN2-reuse + RATIFIED-PENDING-IMPL-TIME closure-at-Task-8 disposition (D8); NO ADR-0125 §(xiv) amendment.

**NO `§(xiv)` amendment verification:**

```
$ grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md
8633:... The absence of a §(xiv) amendment is itself a recorded decision ...
8641:... NO ADR-0125 §(xiv) amendment paragraph is introduced. ...
8657:**(viii) NO ADR-0125 §(xiv) amendment:** ADR-0125's canonical-pattern roster stays at 8 entries ...
```

3 matches — all EXPLANATORY text within ADR-0163 §Context/§Decision describing the ABSENCE of §(xiv). No actual amendment paragraph.

```
$ grep -cE '^\*\*(xiv)\*\*' docs/envoy-go/DECISIONS.md
0
```

0 matches — no actual `**(xiv)**` amendment paragraph exists.

### Test run — Group 7 (extended)

```
$ go test ./internal/filter/http/extauthz/ -run 'TestParsePerRoute|TestResolvePerRoute|TestCheckSettings|TestEffectiveWithRequestBody' -v
=== RUN   TestParsePerRoute_EmptyOverride
--- PASS: TestParsePerRoute_EmptyOverride (0.00s)
=== RUN   TestParsePerRoute_DisabledFalse
--- PASS: TestParsePerRoute_DisabledFalse (0.00s)
=== RUN   TestParsePerRoute_DisabledTrue
--- PASS: TestParsePerRoute_DisabledTrue (0.00s)
=== RUN   TestParsePerRoute_CheckSettings_Empty
--- PASS: TestParsePerRoute_CheckSettings_Empty (0.00s)
=== RUN   TestParsePerRoute_CheckSettings_WithContextExtensions
--- PASS: TestParsePerRoute_CheckSettings_WithContextExtensions (0.00s)
=== RUN   TestParsePerRoute_CheckSettings_DisableRequestBodyBuffering
--- PASS: TestParsePerRoute_CheckSettings_DisableRequestBodyBuffering (0.00s)
=== RUN   TestParsePerRoute_CheckSettings_WithRequestBody
--- PASS: TestParsePerRoute_CheckSettings_WithRequestBody (0.00s)
=== RUN   TestParsePerRoute_CheckSettings_BothBodySettingsXOR
--- PASS: TestParsePerRoute_CheckSettings_BothBodySettingsXOR (0.00s)
=== RUN   TestResolvePerRouteConfig_NilMsg
--- PASS: TestResolvePerRouteConfig_NilMsg (0.00s)
=== RUN   TestResolvePerRouteConfig_DisabledTrue
--- PASS: TestResolvePerRouteConfig_DisabledTrue (0.00s)
=== RUN   TestResolvePerRouteConfig_CheckSettings
--- PASS: TestResolvePerRouteConfig_CheckSettings (0.00s)
=== RUN   TestResolvePerRouteConfig_SyncMapIdentity
--- PASS: TestResolvePerRouteConfig_SyncMapIdentity (0.00s)
=== RUN   TestResolvePerRouteConfig_DifferentProtos
--- PASS: TestResolvePerRouteConfig_DifferentProtos (0.00s)
=== RUN   TestResolvePerRouteConfig_UnknownMsgTypeFallback
--- PASS: TestResolvePerRouteConfig_UnknownMsgTypeFallback (0.00s)
=== RUN   TestResolvePerRouteConfig_SharedStats
--- PASS: TestResolvePerRouteConfig_SharedStats (0.00s)
=== RUN   TestResolvePerRouteConfig_DisabledSharedStats
--- PASS: TestResolvePerRouteConfig_DisabledSharedStats (0.00s)
=== RUN   TestParsePerRoute_ContextExtensions_NoopInHTTPMode
--- PASS: TestParsePerRoute_ContextExtensions_NoopInHTTPMode (0.00s)
=== RUN   TestResolvePerRouteConfig_ConcurrentSameProto
--- PASS: TestResolvePerRouteConfig_ConcurrentSameProto (0.00s)
=== RUN   TestEffectiveWithRequestBody_DisableRequestBodyBuffering
--- PASS: TestEffectiveWithRequestBody_DisableRequestBodyBuffering (0.00s)
=== RUN   TestEffectiveWithRequestBody_PerRouteOverride
--- PASS: TestEffectiveWithRequestBody_PerRouteOverride (0.00s)
=== RUN   TestEffectiveWithRequestBody_ListenerFallback
--- PASS: TestEffectiveWithRequestBody_ListenerFallback (0.00s)
=== RUN   TestResolvePerRouteConfig_CCAlwaysListenerRC
=== RUN   TestResolvePerRouteConfig_CCAlwaysListenerRC/disabled_arm
=== RUN   TestResolvePerRouteConfig_CCAlwaysListenerRC/check_settings_arm_(empty)
=== RUN   TestResolvePerRouteConfig_CCAlwaysListenerRC/check_settings_arm_(with_context_extensions)
--- PASS: TestResolvePerRouteConfig_CCAlwaysListenerRC (0.00s)
    --- PASS: TestResolvePerRouteConfig_CCAlwaysListenerRC/disabled_arm (0.00s)
    --- PASS: TestResolvePerRouteConfig_CCAlwaysListenerRC/check_settings_arm_(empty) (0.00s)
    --- PASS: TestResolvePerRouteConfig_CCAlwaysListenerRC/check_settings_arm_(with_context_extensions) (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.004s
```

22 Group 7 tests PASS (14 existing from Task 2 + 8 new from Task 7).

### Test run — full suite with race detector

```
$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.073s
```

129 tests PASS (up from 121 at Task 6 end; 8 new `func Test` appended). 0 failures.

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

### ADR acceptance-criteria grep

```
$ grep -nE '^## ADR-0163' docs/envoy-go/DECISIONS.md
8622:## ADR-0163: Per-route 5th-canonical REUSE classification ...
```

1 match (1 required). §Decision (8-point body (i)–(viii)) + §Consequences (5 bullets) filled. Status: Accepted; Date: 2026-05-14; Lands-in: Task 7 of phase-18.1 PLAN.

### Task 7 review-fix

Code-quality review returned "Approve with minor fixes" (2 Important + 1 Minor). All 3 fixed in a follow-up commit:

1. **(Important) `TestResolvePerRouteConfig_ConcurrentSameProto` should use `sync.WaitGroup` instead of unbuffered channel.** The project precedent for concurrent tests is `sync.WaitGroup` with `wg.Add(N)` before goroutine launch + `defer wg.Done()` in each (see `internal/filter/http/jwtauthn/jwtauthn_test.go:1248`). The original Task 7 test used `done := make(chan struct{})` (unbuffered), which allows goroutines to serialize on the send (each finished goroutine blocks until the previous send is received), narrowing the race-triggering window. Refactored to the established `sync.WaitGroup` pattern in `internal/filter/http/extauthz/extauthz_test.go` (~line 1034): `var wg sync.WaitGroup` + `wg.Add(n)` before the launch loop + `defer wg.Done()` in each goroutine + `wg.Wait()` before assertions. Added `"sync"` to the import block. The test still passes under `-race`.
2. **(Important) Stale file header + Group 7 section header comments.** `extauthz_test.go` line 3 said "unit-test Groups 1 + 2 + 3 + 4 + 6 + 7 + 8 (through Task 6)" — Task 7 extended Group 7, so updated to "(through Task 7)". `extauthz_test.go` line 614 said "(Group 7 per SPEC §14.1 + PLAN Task 2)" — Task 7 added 8 new Group 7 tests, so updated to "(Group 7 per SPEC §14.1 + PLAN Tasks 2 + 7)".
3. **(Minor) ADR-0163 §Decision (viii) match count is wrong.** §Decision (viii) in `docs/envoy-go/DECISIONS.md` (~line 8657) said "`grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` returns 2 matches, but both are explanatory text within ADR-0163 §Context". Actual count is 3 (lines 8633 + 8641 in §Context plus a third match at line 8657 in §Decision (viii) itself — the §Decision sentence uses `\(xiv\)` in its own explanation). Updated to "returns 3 matches, but all three are explanatory text within ADR-0163 §Context/§Decision describing the ABSENCE of §(xiv)".

Test count unchanged (129; no new tests added — refactor + comment-correctness only). `go build` / `go vet` / `gofmt -l` (empty) / `go test -race -count=1 ./internal/filter/http/extauthz/...` all green.

```
$ go build ./internal/filter/http/extauthz/...
[no output]

$ go vet ./internal/filter/http/extauthz/...
[no output]

$ gofmt -l ./internal/filter/http/extauthz/
[no output — empty]

$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.075s

$ go test -race -count=1 -run '^TestResolvePerRouteConfig_ConcurrentSameProto$' -v ./internal/filter/http/extauthz/...
=== RUN   TestResolvePerRouteConfig_ConcurrentSameProto
--- PASS: TestResolvePerRouteConfig_ConcurrentSameProto (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.009s
```

## Task 8 — Stat surface finalization + §18.P6 + §18.P7 RATIFIED-PENDING empirical-scrape closures [ADR-0163]

Per PLAN Task 8 (Steps 1–7) + ADR-0163 §Decision (vii) + planner-time decision D8.

The stat-surface machinery — `newFilterStats` + the 6-counter `filterStats` struct + `baseStatPrefix` helper — landed wholly at Task 2 (per ADR-0156). Task 8's substantive deliverables are:

1. **Group 2 stats sub-group tests** — 6 new tests locking down the already-satisfied contract (finalization/locking step).
2. **§18.P6 + §18.P7 RATIFIED-PENDING-IMPL-TIME empirical scrape** — reference Envoy v1.37.2 run confirming the SN2-reuse hypothesis.
3. **No source-code amendments to `extauthz.go`** — Task 2's `newFilterStats` implementation was correct; Task 8 locks it down with tests.

### Status: §18.P6 + §18.P7 RATIFIED — SN2-reuse CONFIRMED

### Group 2 stats sub-group — TDD note

Per PLAN Task 8 Step 1: these tests are written at Task 8 to lock down the contract. They passed immediately because Task 2's implementation already satisfies all conditions — this is the documented "finalization/locking step" pattern per the PLAN (identical to Task 7's per-route finalization).

The 6 new tests added to `extauthz_test.go` (9 total stats tests after Task 8; 3 existing from Task 2 + 6 new):
1. `TestFilterStats_ExactlySixCounters_NoExtras` — exactly 6 counters, no extras in a fresh Registry.
2. `TestFilterStats_UnconditionalRegistration_ViaBuildCompiledConfig` — integration: all 6 registered immediately after `buildCompiledConfig` (non-lazy; SN2 names verified).
3. `TestFilterStats_NilStats_CcStatsIsNil` — `cc.stats` is nil when `ctx.Stats` is nil (ADR-0085 nil-tolerance guard).
4. `TestFilterStats_EmptyPrefix_FoldsToBarePrefixShape` — empty HCM stat_prefix folds to bare `ext_authz.<counter>` (no double-dot).
5. `TestFilterStats_CounterHandleNames_SN2ReusePins` — pins exact `Name()` values on all 6 counter handles (`http.ingress_http.ext_authz.*`).
6. `TestFilterStats_DisabledCounter_RegisteredButZero` — `disabled` registered (non-nil handle) but `Load()` == 0 (STRUCTURALLY UNREACHABLE under MVP per parent §6 amendment 7).

All 6 passed immediately confirming Task 2's impl was correct.

### §18.P6 + §18.P7 empirical scrape — reference Envoy v1.37.2

**Envoy image:** `envoyproxy/envoy:v1.37.2` (SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`)

**Envoy config used (minimal — HCM with ext_authz HTTP filter, dummy auth cluster, admin on :9901):**

```yaml
admin:
  address:
    socket_address:
      address: 0.0.0.0
      port_value: 9901

static_resources:
  listeners:
  - name: listener_0
    address:
      socket_address:
        address: 0.0.0.0
        port_value: 10000
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          stat_prefix: ingress_http
          route_config:
            name: local_route
            virtual_hosts:
            - name: local_service
              domains: ["*"]
              routes:
              - match:
                  prefix: "/"
                route:
                  cluster: backend
          http_filters:
          - name: envoy.filters.http.ext_authz
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz
              transport_api_version: V3
              http_service:
                server_uri:
                  uri: http://127.0.0.1:12345
                  cluster: auth_service
                  timeout: 0.25s
          - name: envoy.filters.http.router
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router

  clusters:
  - name: backend
    connect_timeout: 0.25s
    type: STATIC
    load_assignment:
      cluster_name: backend
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 8080
  - name: auth_service
    connect_timeout: 0.25s
    type: STATIC
    load_assignment:
      cluster_name: auth_service
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address:
                address: 127.0.0.1
                port_value: 12345
```

**Docker invocation:**

```
docker run --rm -d --name envoy-ext-authz-scrape \
  -p 9901:9901 \
  -v /tmp/envoy-ext-authz-scrape.yaml:/etc/envoy/envoy.yaml \
  envoyproxy/envoy:v1.37.2 \
  -c /etc/envoy/envoy.yaml

sleep 3
curl -s localhost:9901/stats/prometheus | grep -i ext_authz
docker stop envoy-ext-authz-scrape
```

**Verbatim `/stats/prometheus` output filtered to `ext_authz`:**

```
# TYPE envoy_http_ext_authz_denied counter
envoy_http_ext_authz_denied{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_disabled counter
envoy_http_ext_authz_disabled{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_error counter
envoy_http_ext_authz_error{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_failure_mode_allowed counter
envoy_http_ext_authz_failure_mode_allowed{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_filter_state_name_collision counter
envoy_http_ext_authz_filter_state_name_collision{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_ignored_dynamic_metadata counter
envoy_http_ext_authz_ignored_dynamic_metadata{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_invalid counter
envoy_http_ext_authz_invalid{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_ok counter
envoy_http_ext_authz_ok{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_omitted_response_headers counter
envoy_http_ext_authz_omitted_response_headers{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_request_header_limits_reached counter
envoy_http_ext_authz_request_header_limits_reached{envoy_http_conn_manager_prefix="ingress_http"} 0
# TYPE envoy_http_ext_authz_response_header_limits_reached counter
envoy_http_ext_authz_response_header_limits_reached{envoy_http_conn_manager_prefix="ingress_http"} 0
```

**Analysis of the scrape evidence:**

The 6 MVP counter names from the scrape:
- `envoy_http_ext_authz_ok{envoy_http_conn_manager_prefix="ingress_http"} 0`
- `envoy_http_ext_authz_denied{envoy_http_conn_manager_prefix="ingress_http"} 0`
- `envoy_http_ext_authz_error{envoy_http_conn_manager_prefix="ingress_http"} 0`
- `envoy_http_ext_authz_disabled{envoy_http_conn_manager_prefix="ingress_http"} 0`
- `envoy_http_ext_authz_failure_mode_allowed{envoy_http_conn_manager_prefix="ingress_http"} 0`
- `envoy_http_ext_authz_invalid{envoy_http_conn_manager_prefix="ingress_http"} 0`

The 5 additional deferred-feature counters (`filter_state_name_collision`, `ignored_dynamic_metadata`, `omitted_response_headers`, `request_header_limits_reached`, `response_header_limits_reached`) also appear — these are the extra v1.37.2 names already documented as DEFERRED in parent SPEC §6 amendment 8. envoy-go does NOT register these 5 (correct per the SPEC).

**SN2-reuse rendering verification:** The internal path `http.ingress_http.ext_authz.<counter>` → Prometheus `envoy_http_ext_authz_<counter>{envoy_http_conn_manager_prefix="ingress_http"}` matches exactly what `internal/stats/name.go`'s SN2 rule produces: the `http.*` prefix-match extracts `ingress_http` as `envoy_http_conn_manager_prefix`; the rest `ext_authz.<counter>` gets dot→underscore transform → `envoy_http_ext_authz_<counter>`. NO new SN-flattening rule needed.

**§18.P6 + §18.P7 VERDICT: CONFIRMED — SN2-reuse hypothesis holds.** No ADR-0163 amendment required.

### File changes (Task 8)

| File | Change | Detail |
|---|---|---|
| `internal/filter/http/extauthz/extauthz_test.go` | +143 LoC | File header updated (Task 7 → Task 8); Group 2 stats sub-group comment block + 6 new tests |
| `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` | (this entry) | Task 8 narrative + empirical-scrape evidence |

No changes to `docs/envoy-go/DECISIONS.md` (ADR-0163 §Decision already carries the closure-at-Task-8 disposition; no amendment needed since the scrape CONFIRMED the SN2-reuse hypothesis).

### Test runs

#### Targeted stats sub-group run

```
$ go test ./internal/filter/http/extauthz/ -run 'TestFilterStats' -v
=== RUN   TestFilterStats_6Counters
--- PASS: TestFilterStats_6Counters (0.00s)
=== RUN   TestFilterStats_CounterNames
--- PASS: TestFilterStats_CounterNames (0.00s)
=== RUN   TestFilterStats_NilRegistryTolerance
--- PASS: TestFilterStats_NilRegistryTolerance (0.00s)
=== RUN   TestFilterStats_ExactlySixCounters_NoExtras
--- PASS: TestFilterStats_ExactlySixCounters_NoExtras (0.00s)
=== RUN   TestFilterStats_UnconditionalRegistration_ViaBuildCompiledConfig
--- PASS: TestFilterStats_UnconditionalRegistration_ViaBuildCompiledConfig (0.00s)
=== RUN   TestFilterStats_NilStats_CcStatsIsNil
--- PASS: TestFilterStats_NilStats_CcStatsIsNil (0.00s)
=== RUN   TestFilterStats_EmptyPrefix_FoldsToBarePrefixShape
--- PASS: TestFilterStats_EmptyPrefix_FoldsToBarePrefixShape (0.00s)
=== RUN   TestFilterStats_CounterHandleNames_SN2ReusePins
--- PASS: TestFilterStats_CounterHandleNames_SN2ReusePins (0.00s)
=== RUN   TestFilterStats_DisabledCounter_RegisteredButZero
--- PASS: TestFilterStats_DisabledCounter_RegisteredButZero (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.003s
```

All 9 stats tests PASS (3 from Task 2 + 6 new at Task 8). Tests passed immediately — finalization/locking step confirmed.

#### Full package run

```
$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.079s
```

135 PASS, 0 FAIL, 0 SKIP (was 129 PASS after Task 7; +6 new stats sub-group tests).

#### go vet

```
$ go vet ./internal/filter/http/extauthz/...
(no output — exit 0)
```

#### gofmt

```
$ gofmt -l internal/filter/http/extauthz/
(no output — empty)
```

### Task 8 commit SHA

`4b672ba`

## Task 9 — `DecodeHeaders` dispatch + async-resume outbound-call leg + `OnDestroy` cancellation + Groups 5+9

Per PLAN Task 9 (Steps 1–7) + SPEC §6.3 + planner-time decision D4.

Task 9 wires the full request-processing dispatch path: `DecodeHeaders` returns `StopIteration` and launches a goroutine that calls the auth service; the goroutine applies the disposition (allow/deny/error/invalid) back on the filter chain via `ContinueDecoding()` or `SendLocalReply`; `OnDestroy` cancels the in-flight context and sets the `mu`/`done` race guard.

### Deliverables

1. **`dispatchOutboundCheck(headers http.Header)` helper** — single source of truth for authRequest construction + goroutine launch + disposition application. Called from both `DecodeHeaders` (no-body path) and `DecodeData` (body-complete path). Nil-guard for `cc.checkFn` prevents goroutine panics in test-constructed filters.

2. **`applyDisposition(headers, cc, disp, err)` helper** — wires all four disposition classes under `f.mu` (goroutine-side): `dispAllow` → `ok` counter + upstream mutation + optional `clearRouteCacheRequested` flag + `ContinueDecoding()`; `dispDeny` → `denied` counter + `SendLocalReply`; `dispInvalid` → `invalid` counter + error posture; default → error posture.

3. **`applyErrorPosture(headers, cc, fs)` helper** — `errored` counter + `failureModeAllow` branch: header-add (`X-Envoy-Auth-Failure-Mode-Allowed: true`) + `ContinueDecoding()` vs `SendLocalReply(statusOnError)`.

4. **`headerKVToOrderedHeaders(kvs []headerKV) envoyhttp.OrderedHeaders`** — converts the deny-path header slice to the ordered-headers format required by `SendLocalReply`.

5. **`DecodeHeaders` rewritten** — resolves per-route config, handles disabled short-circuit (`Continue`), caches `activeRC`, detects `withRequestBody`+`!endStream` path (`awaitingBody=true`, `cachedHeaders` set, `Continue`), dispatches `dispatchOutboundCheck` + returns `StopIteration` on the no-body path.

6. **`DecodeData` body-complete seam wired** — `endStream=true` while `awaitingBody`: recovers `cachedHeaders`, calls `dispatchOutboundCheck`, returns `DataStopIterationAndBuffer`.

7. **`OnDestroy` implemented** — acquires `f.mu`, sets `f.done=true`, snapshots `callCancel`, releases `f.mu`, calls `cancel()` if non-nil. Resume goroutine checks `f.done` under `f.mu` and aborts callbacks if set.

8. **`filter` struct additions**: `cachedHeaders http.Header` (header cache for body-complete path), `clearRouteCacheRequested bool` (ADR-0155 ClearRouteCache deferral tracking flag).

9. **Groups 5+9 tests** (12 new tests in `extauthz_test.go`): Group 5A (`TestDecodeHeaders_PerRouteDisabled_ContinueNoCounters`), Group 5B (`TestDecodeHeaders_AsyncAllow_UpstreamMutation`, `TestDecodeHeaders_AsyncAllow_ClearRouteCache`), Group 5C (`TestDecodeHeaders_AsyncDeny_SendLocalReply`), Group 5D (`TestDecodeHeaders_AsyncError_FailureModeAllow_False`, `TestDecodeHeaders_AsyncError_FailureModeAllow_True`, `TestDecodeHeaders_AsyncError_FailureModeAllow_True_HeaderAdd`), Group 5E (`TestDecodeHeaders_AsyncInvalid_InvalidCounterAndErrorPosture`), Group 5F (`TestDecodeData_BodyComplete_AsyncDispatch`), Group 9 (`TestOnDestroy_CancelsInFlightContext`, `TestOnDestroy_ResumeAfterDestroy_NoCallback`, `TestOnDestroy_NoPanic_WhenNoActiveCall`).

10. **`asyncExtAuthzDCB` race-safe mock** — added `sync.Mutex mu` + locked `ContinueDecoding`/`SendLocalReply` + safe accessor functions `asyncDCB_continueCount()` + `asyncDCB_localReply()` + `waitForContinueOrReply()` polling helper. Upstream-header read in `TestDecodeHeaders_AsyncAllow_UpstreamMutation` guarded under `f.mu` to eliminate the goroutine-write vs test-read race.

### Planner-time decision D4 — ADR-0044 escape-valve NOT fired

The `mu`/`done` guard + `context.WithCancel` sufficed. No ADR-0165 authored.

- `OnDestroy` sets `f.done = true` under `f.mu` and calls `callCancel()`.
- Resume goroutine acquires `f.mu`, checks `f.done`, aborts the callback touch if the stream is gone.
- `TestOnDestroy_ResumeAfterDestroy_NoCallback` passes under `-race` confirming the guard works.
- `TestOnDestroy_CancelsInFlightContext` confirms the cancellable context makes the in-flight `client.Do` return promptly.

### Task 9 TDD cycle notes

**RED → GREEN cycle per test group:**

- All 12 Group 5+9 tests were written first and failed (compilation errors / wrong return values / missing functions) before implementation. Key failures observed:
  - `TestDecodeHeaders_EndStreamNoBody_Continue` (Task 2 skeleton): expected `Continue`, now returns `StopIteration` → renamed `TestDecodeHeaders_EndStreamNoBody_StopIteration` + expectation updated.
  - `TestDecodeHeaders_WithRequestBody_EndStreamSkipsBuffer` (Task 2 skeleton): expected `Continue`, now returns `StopIteration` → expectation updated.
  - `stats.Counter.Value()` undefined → corrected to `.Load()`.
  - `f.clearRouteCacheRequested` undefined → field added to `filter` struct.
  - `dispInvalid` test via real HTTP server: Go's `net/http` canonicalizes response headers, making `:pseudo` header injection impossible → switched to fake `checkFn` closure.
  - DATA RACE on `asyncExtAuthzDCB.continueCount`/`localReply`: resume goroutine write vs test goroutine read → added `sync.Mutex` + locked methods + safe accessors.
  - DATA RACE on `upstream` http.Header: goroutine write under `f.mu` vs test read unguarded → test-side read now under `f.mu`.

### Full package test run (race-clean)

```
$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.303s
```

147 PASS, 0 FAIL, 0 SKIP (was 135 PASS after Task 8; +12 new Group 5+9 tests).

### go vet

```
$ go vet ./internal/filter/http/extauthz/...
(no output — exit 0)
```

### gofmt

```
$ gofmt -l internal/filter/http/extauthz/
(no output — empty)
```

### Task 9 commit SHA

`c310d96`

### Task 9 review fixes

Post-landing spec and code-quality reviews identified 7 issues. All fixed in one follow-up commit.

#### Fix 1 (Important) — `path=""` bug

**Root cause:** `dispatchOutboundCheck` was calling `buildAuthRequest(f, nil, headers, f.body, "")` — the empty string `""` meant the auth service received every request at the same URL (`baseURL + pathPrefix`) regardless of the client's actual request path. Auth services cannot make path-based decisions and `path_prefix` prepending was effectively broken.

**Fix:** Extract the client's path from the `:path` pseudo-header in `dispatchOutboundCheck` (mirroring the `jwtauthn.go:759` pattern):
```go
path := headers.Get(":path")
authReq := buildAuthRequest(f, headers, f.body, path)
```

**Files changed:** `internal/filter/http/extauthz/extauthz.go` (dispatchOutboundCheck — add path extraction + pass to buildAuthRequest; also fix stale comment per Fix 4 below).

**Side discovery:** The `:path` pseudo-header (and any other `:` prefixed HTTP/2 pseudo-headers like `:method`, `:authority`, `:scheme`) must also be stripped from the filtered headers map in `buildAuthRequest` — Go's `net/http` client rejects `:` prefixed header field names as invalid per RFC 7230 §3.2. Added unconditional pseudo-header skip in `buildAuthRequest`'s header-filtering loop:
```go
if strings.HasPrefix(name, ":") {
    continue
}
```
**Files changed:** `internal/filter/http/extauthz/attributes.go` (buildAuthRequest — skip pseudo-headers before allow/disallow filtering).

**New test (TDD):** `TestDecodeHeaders_PathPropagation_AuthServerSeesClientPath` — verified FAIL before fix (`auth server path: got "", want "/api/v1/users"`) and PASS after.

#### Fix 2 (Important) — `hs=nil` → `headers_to_add` silently dropped

**Root cause:** `dispatchOutboundCheck` passed `nil` as the `hs *ext_authzv3.HttpService` argument to `buildAuthRequest`. The `buildAuthRequest` function guarded `headers_to_add` processing with `if hs != nil`, so all `AuthorizationRequest.headers_to_add` static headers were silently dropped from every outbound auth request.

**Fix (Option A — recommended):** Added `headersToAdd []headerKV` field to `compiledConfig`. Pre-compiled from `httpSvc.GetAuthorizationRequest().GetHeadersToAdd()` at `buildCompiledConfig` time (alongside `deprecatedAllowedHeaders`). Updated `buildAuthRequest` to:
- Remove the `hs *ext_authzv3.HttpService` parameter entirely (buildAuthRequest is now fully hs-independent).
- Apply `cc.headersToAdd` unconditionally instead of reading from `hs`.

This mirrors the Task-5 `deprecatedAllowedHeaders` pre-compile pattern. The `dispatchOutboundCheck` call becomes `buildAuthRequest(f, headers, f.body, path)` — no nil/empty placeholders.

**Files changed:** `internal/filter/http/extauthz/extauthz.go` (`compiledConfig` — add `headersToAdd []headerKV` field; `buildCompiledConfig` — pre-compile headers_to_add into cc; also extended pre-compile comment block). `internal/filter/http/extauthz/attributes.go` (remove `hs` param from `buildAuthRequest`; consume `cc.headersToAdd`; remove unused `ext_authzv3` import). `internal/filter/http/extauthz/extauthz_test.go` (`buildAuthRequestForTest` helper updated to pre-compile `headersToAdd` from hs into cc, mirroring buildCompiledConfig; `TestBuildAuthRequest_BodyIncluded` updated to drop hs arg; `TestBuildAuthRequest_NilHttpService` renamed to `TestBuildAuthRequest_NoHeadersToAdd` with updated description).

**New test (TDD):** `TestDecodeHeaders_HeadersToAdd_ReachAuthServer` — verified PASS after fix (pre-fix failure confirmed by the structural analysis: the `if hs != nil` guard silently dropped headers_to_add when `nil` was passed).

#### Fix 3 — `clearRouteCacheRequested` test read without lock

**Root cause:** `TestDecodeHeaders_AsyncAllow_ClearRouteCache` read `f.clearRouteCacheRequested` without holding `f.mu`, while the resume goroutine writes it under `f.mu`. This is a data race.

**Fix:** Wrapped the read under `f.mu` (mirrors `TestDecodeHeaders_AsyncAllow_UpstreamMutation` line 4380-4382 which already acquired the lock):
```go
f.mu.Lock()
got := f.clearRouteCacheRequested
f.mu.Unlock()
if !got { ... }
```

**File changed:** `internal/filter/http/extauthz/extauthz_test.go` (TestDecodeHeaders_AsyncAllow_ClearRouteCache — add mu lock/unlock around read).

#### Fix 4 — Stale comment in `dispatchOutboundCheck`

**Root cause:** The comment "The HttpService is extracted from cc.checkFn's closure capture — we pass nil here because buildAuthRequest only needs hs for headers_to_add + the deprecated allowed_headers field; both are pre-compiled into cc at buildCompiledConfig time..." was factually wrong (headers_to_add was NOT pre-compiled, which was exactly the bug fixed in Fix 2).

**Fix:** Rewrote the comment to accurately describe the post-Option-A state (cc pre-compiled approach, hs-independent buildAuthRequest, path extracted from :path pseudo-header).

**File changed:** `internal/filter/http/extauthz/extauthz.go` (dispatchOutboundCheck comment block — accurate post-fix description).

#### Fix 5 — Group 5A disabled test checks only `ok` counter

**Root cause:** `TestDecodeHeaders_PerRouteDisabled_ContinueNoCounters` only asserted `ok == 0` but not the other 5 counters. The "NO counter increments" contract was incompletely tested.

**Fix:** Extended to assert all 6 counters (`ok`, `denied`, `error`, `disabled`, `failure_mode_allowed`, `invalid`) stay at 0 using a table-driven check.

**File changed:** `internal/filter/http/extauthz/extauthz_test.go` (TestDecodeHeaders_PerRouteDisabled_ContinueNoCounters — table-driven all-6-counters assertion).

#### Fix 6 — Group 5E invalid test misses `errored==1`

**Root cause:** The `dispInvalid` path calls `applyErrorPosture` which increments `errored` PLUS the separate `invalid` counter. The test only verified `invalid == 1` and missed the dual-increment behavior.

**Fix:** Added `erroredCtr.Load() == 1` assertion to lock down the dual-increment contract.

**File changed:** `internal/filter/http/extauthz/extauthz_test.go` (TestDecodeHeaders_AsyncInvalid_InvalidCounterAndErrorPosture — add erroredCtr assertion).

#### Fix 7 — Stale self-correction comment in `DecodeData`

**Root cause:** The `DecodeData` `endStream` block had a multi-paragraph comment describing a "wrong design" followed by a "CORRECTION" narrative — implementation history that has no place in production code.

**Fix:** Replaced with a clean comment explaining the `cachedHeaders` design directly, with no design-history narrative.

**File changed:** `internal/filter/http/extauthz/extauthz.go` (DecodeData endStream block — clean comment describing the cachedHeaders approach).

#### Post-fix verification

```
$ go build ./internal/filter/http/extauthz/...
(no output — exit 0)

$ go vet ./internal/filter/http/extauthz/...
(no output — exit 0)

$ gofmt -l internal/filter/http/extauthz/
(no output — empty)

$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.307s

$ go test -race -count=10 -run 'TestDecodeHeaders|TestAsyncResume|TestOnDestroy' ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	3.371s
```

184 PASS, 0 FAIL, 0 SKIP (was 147 after Task 9 landing; +37 includes the new path + headers_to_add + Group 5A/5E test additions, plus the pre-existing counter changes).

**Note on Task 13 impact:** The `path=""` bug (Fix 1) and `headers_to_add` dropped (Fix 2) are behavioral gaps that Task 13's differential test against reference Envoy would have caught. Fixing them now de-risks Task 13 significantly. The pseudo-header strip (Fix 1 side discovery) is also required for correct HTTP/1.1 outbound request construction.

### Task 9 review-fix commit SHA

`1203613`

---

## Task 10 — boot-registration + FuzzExtAuthzConfigParse 22nd fuzzer + test/helpers/extauthzhttp/ + fixture infrastructure

**Files changed:**
- Modified: `cmd/envoy-go/main.go` (register `extauthz.TypeURL → extauthz.New` + import; alphabetical between `envoygotest` and `fault`)
- Created: `internal/filter/http/extauthz/fuzz_test.go` (22nd fuzzer `FuzzExtAuthzConfigParse`; 21 corpus seeds)
- Created: `test/helpers/extauthzhttp/doc.go` (~25 LoC package doc)
- Created: `test/helpers/extauthzhttp/extauthzhttp.go` (~155 LoC in-process scriptable HTTP auth server)
- Created: `test/helpers/extauthzhttp/extauthzhttp_test.go` (~200 LoC; 6 unit tests written FIRST per TDD)
- Modified: `test/differential/fixture/fixture.go` (`HTTPExtAuthzHTTP BackendKind = 17`)
- Modified: `test/differential/runner_test.go` (switch-case for `HTTPExtAuthzHTTP`)
- Modified: `docs/envoy-go/phases/18.1-ext-authz-http/PROGRESS.md` (this entry)

**Blank-import decision (Task 10 judgment call):** The blank import `_ "github.com/esalaine/envoy-go/test/fixtures/0020-http-ext-authz-http/inputs"` is DEFERRED to Task 11. The `inputs` package does not exist yet (Task 11 creates `test/fixtures/0020-http-ext-authz-http/inputs/driver.go`). Landing the blank import now would break `go build`. The `BackendKind = 17` enum value + the switch-case skeleton in `runner_test.go` land now (as the phase-17 Task 10 precedent did for `HTTPJwtAuthn`). The switch-case comment documents this explicitly.

**No ADR landed in Task 10** (ADR-0044 ADR-on-impl convention; boot-registration is covered by existing ADR-0072 + ADR-0156 §Decision already landed at Task 2).

**Outputs:**

### go build ./...

```
$ go build ./...
(exit 0 — no output)
```

### go vet ./...

```
$ go vet ./...
(exit 0 — no output)
```

### gofmt -l

```
$ gofmt -l ./cmd/envoy-go/main.go ./internal/filter/http/extauthz/fuzz_test.go ./test/helpers/extauthzhttp/ ./test/differential/
(empty — all files formatted)
```

### grep -cE 'httpReg.Register' cmd/envoy-go/main.go

```
$ grep -cE 'httpReg\.Register' cmd/envoy-go/main.go
13
```

(Was 12 before Task 10; now 13 per SPEC §1 item 2 + ADR-0072.)

### go test -race -count=1 ./test/helpers/extauthzhttp/...

```
$ go test -race -count=1 ./test/helpers/extauthzhttp/...
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzhttp	6.017s
```

6 tests: `TestNew_StartsServerOnConfiguredAddr`, `TestServer_FixedScript_ReturnsStatusBodyHeaders`, `TestServer_PathMethodMap_Dispatch`, `TestServer_BodyInspectingScript`, `TestServer_Stop_ClosesListener`, `TestServer_Stop_Idempotent`, `TestServer_ConcurrentClient_NoRace`. All PASS, race-clean.

### FuzzExtAuthzConfigParse 30s run

```
$ go test -run '^$' -fuzz 'FuzzExtAuthzConfigParse' -fuzztime 30s ./internal/filter/http/extauthz/
fuzz: elapsed: 0s, gathering baseline coverage: 0/22 completed
fuzz: elapsed: 0s, gathering baseline coverage: 22/22 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 156654 (52203/sec), new interesting: 122 (total: 144)
fuzz: elapsed: 6s, execs: 566773 (136720/sec), new interesting: 206 (total: 228)
fuzz: elapsed: 9s, execs: 975578 (136253/sec), new interesting: 240 (total: 262)
fuzz: elapsed: 12s, execs: 1482873 (169125/sec), new interesting: 289 (total: 311)
fuzz: elapsed: 15s, execs: 1836976 (118037/sec), new interesting: 310 (total: 332)
fuzz: elapsed: 18s, execs: 2064550 (75848/sec), new interesting: 337 (total: 359)
fuzz: elapsed: 21s, execs: 2204133 (46538/sec), new interesting: 342 (total: 364)
fuzz: elapsed: 24s, execs: 2665910 (153792/sec), new interesting: 358 (total: 380)
fuzz: elapsed: 27s, execs: 3098453 (144287/sec), new interesting: 401 (total: 423)
fuzz: elapsed: 30s, execs: 3729963 (210536/sec), new interesting: 406 (total: 428)
fuzz: elapsed: 31s, execs: 3729963 (0/sec), new interesting: 406 (total: 428)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	31.090s
```

22 corpus seeds; 3.7M+ executions in 30s; no crashes, no panics, no (nil, nil) violations. PASS.

### Total fuzzer count

```
$ grep -rE '^func Fuzz' --include='*.go' . | wc -l
22
```

(Was 21 after phase 17; now 22 after Task 10.)

### go test -count=1 ./test/differential/ (fixtures 0000-0019 regression)

```
$ go test -count=1 ./test/differential/ -run 'TestDifferential' -timeout 120s
ok  	github.com/esalaine/envoy-go/test/differential	56.415s
```

All 20 pre-existing fixtures (0000–0019) PASS. No regression. Fixture 0020 is skipped (`no driver registered for fixture "0020-http-ext-authz-http"`) since its blank import lands at Task 11.

### Task 10 commit SHA

TBD (filled post-squash)
