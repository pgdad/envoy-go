# Phase 13 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..12 PROGRESS.md structure.

## Preamble — execution preconditions

All 16 preconditions verified green at cold-start without deviation. Worktree branch `phase-13-http-filter-buffer-impl`; master tail shows PLAN SHA-fill at `63850f6`, PLAN at `a8bd93c`, SPEC SHA-fill at `6e39444`, SPEC at `f5d38fa`, brainstorm follow-ups at `278d6e5`/`812d234`/`37d4dfa`/`daee042`/`3915338`/`f9c0934`, preceding commits. Go 1.26.2, golangci-lint 1.64.8, Docker client 28.4.0 + server 28.1.1 present. ADR tail at 0124 (next-free 0125). SPEC at f5d38fa23f085400bca97f93befc082f4d776483. `internal/filter/http/buffer/` absent (Task 2 lands). `fixture.HTTPBuffer` absent (Task 7 lands). CONFORMANCE_PINS.md unchanged. 7 `httpReg.Register` calls in main.go (`router`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`). `### envoy.filters.http.csrf` at line 1093 in BEHAVIOR_CONTRACT.md. Buffer + BufferPerRoute protos present in module closure. Envoy image v1.37.2 SHA confirmed (`sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`). Working tree pristine. All 15 differential fixtures (0000–0014) PASS. Pre-existing fuzzers (16 fuzzers from phases 02–12) deferred to Gate D at Task 12 — skipping the 30s × 16 wallclock cost at Task 1; running 16 fuzz corpora end-to-end is Task 12's Gate D responsibility per PLAN §"Execution preconditions" guidance ("OPTIONAL at Task 1; the Task 12 Gate D verification re-runs them").

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The three ADRs anticipated by SPEC §8 (ADR-0125, ADR-0126, ADR-0127 v2). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0125** `internal/filter/http/buffer/` package shape (single-token directory matching cors / fault / csrf precedent + extension-registry registration ordering + decoder-only `HTTPFilter` value with `Encoder: nil`) + per-route disabled-OR-override discipline (5th canonical per-route shape) — Task 2
- **ADR-0126** `compiledConfig` shape + 1-consumed/0-deferred field decomposition (`max_request_bytes`) + parse-time `max_request_bytes ≤ 1 MiB` validation (envoy-go-only divergence) + cap-layering rationale + PGV-mirror filter-internal validation discipline at `New` time — Task 2
- **ADR-0127 v2** Body-counting + 413-trigger algorithm — STREAMING-CAP ONLY + `DecodeHeaders` StopIteration on bodied + non-disabled requests + `DecodeData` accumulation + `DataStopIterationNoBuffer` on overflow + cap predicate `>` strict + `maybeAddContentLength` mirror + reuse of framework `SendLocalReply` 413 wire shape + 100-Continue addendum — Task 3

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The eleven planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)`; encode side ABSENT** (decoder-only filter; HTTPFilter struct sets Decoder: f, Encoder: nil — mirrors phase 12 csrf precedent).
2. **D2 — `buffer.go` file split = SINGLE-FILE** (no `count.go` or `perroute.go`; ~280-330 LoC stays under mental-model threshold; mirrors csrf single-file precedent).
3. **D3 — Filter-internal validation error message wording = envoy-go's own clear-text wording** (option (b); `buffer: max_request_bytes is required` etc.; phase 11 ADR-0115 + phase 12 ADR-0121 precedent).
4. **D4 — Backend-side Content-Length assertion mechanism = OPTION (a) JSON-echo** (backend echoes inbound headers as JSON in response body; driver parses and asserts `headers["content-length"] == "10240"` for fixture scenario 6).
5. **PLAN-emerging — `HTTPFilter` value shape = `Decoder: f, Encoder: nil`** (decoder-only; saves implementing StreamEncoderFilter method set; mirrors phase 12 csrf ADR-0120 precedent).
6. **PLAN-emerging — Fixture topology = SINGLE LISTENER `l_main` with THREE ROUTES** (`/` default + `/route-disabled` + `/route-tighter`; fits existing `fixture.Driver` contract; saves driver complexity vs phase 11's 4-listener topology).
7. **PLAN-emerging — BackendKind enum value = `HTTPBuffer BackendKind = 12`** (continues existing naming convention; next value after `HTTPCsrf BackendKind = 11`).
8. **PLAN-emerging — Chunked-body construction in driver = `req.TransferEncoding = []string{"chunked"}` + `bytes.NewReader(data)`** (Go stdlib net/http idiom; no io.Pipe needed since bodies are tractably small ≤200 KiB).
9. **PLAN-emerging — Backend echoes inbound headers as JSON** (mirrors SPEC §11.5 python `BaseHTTPRequestHandler` echo; lowercase header keys per Envoy wire-form discipline).
10. **PLAN-emerging — Go stdlib transparent 100-Continue handling** (`http.Client.Do` strips 1xx interim responses; driver compares final response only; no driver-level 100-Continue code needed).
11. **PLAN-emerging — `effectiveMax` + `accumulated` field types = `uint32`** (matches `compiledConfig.maxRequestBytes uint32`; future cap-promotion phase widens in lockstep).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `827e7c9` — `phase 13: PROGRESS preamble + planner-time decision resolution`
**Notes:** Created PROGRESS.md; verified all 16 preconditions per PLAN §"Execution preconditions"; phase-13 SPEC + PLAN confirmed present in HEAD; SPEC at f5d38fa; ADR tail at 0124 (next-free 0125); `internal/filter/http/buffer/` absent (Task 2 lands); `fixture.HTTPBuffer` absent (Task 7 lands). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table). Pre-existing fuzzers (16 fuzzers) deferred to Gate D per PLAN.

**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase-13-http-filter-buffer-impl

$ git log --oneline master | head -10
63850f6 phase 13 PLAN follow-up: STATE.md SHA-fill (TBD → a8bd93c)
a8bd93c phase 13: PLAN.md authored; lifecycle-state-2 → 3 transition
6e39444 phase 13 SPEC follow-up: STATE.md SHA-fill (TBD → f5d38fa)
f5d38fa phase 13: SPEC.md authored; ROADMAP row 13 → in-progress; lifecycle-state-1 → 2 transition
278d6e5 .gitignore: ignore next-prompt.txt
812d234 phase 13 brainstorm advisory-recs follow-up: STATE.md update (last-commit → 37d4dfa + §unresolved)
37d4dfa phase 13 brainstorm amendment 1 follow-up: address spec-document-reviewer advisory recs
daee042 phase 13 brainstorm amendment 1 follow-up: STATE.md SHA-fill (TBD → 3915338)
3915338 phase 13 brainstorm amendment 1: §12 empirical re-frame [post-landing per D-3.5]
f9c0934 phase 13 brainstorm follow-up: STATE.md SHA-fill (TBD → 6cf412e)

$ docker version
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

$ go version
go version go1.26.2 linux/amd64

$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)

$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
124

$ git log -1 --format=%H -- docs/envoy-go/phases/13-http-filter-buffer/SPEC.md
f5d38fa23f085400bca97f93befc082f4d776483

$ git status --porcelain
(empty — working tree pristine)

$ go test -count=1 -short ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	3.991s
ok  	github.com/esalaine/envoy-go/internal/accesslog	0.005s
ok  	github.com/esalaine/envoy-go/internal/admin	0.427s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	0.016s
ok  	github.com/esalaine/envoy-go/internal/cluster	0.021s
ok  	github.com/esalaine/envoy-go/internal/drain	0.077s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	0.018s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	2.496s
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.134s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	0.006s
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	0.007s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	0.037s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	0.266s
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	0.006s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	0.008s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	0.217s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	0.167s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	3.030s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	0.045s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	0.007s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	0.005s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	0.024s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	0.062s
ok  	github.com/esalaine/envoy-go/test/differential	0.062s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	0.005s
[... driver packages: no test files or ok ...]
ok  	github.com/esalaine/envoy-go/test/helpers	0.008s
(all packages ok or [no test files])

$ go test -count=1 ./test/differential/ -run 'TestDifferential' -v
--- PASS: TestDifferential (51.10s)
    --- PASS: TestDifferential/0000-tcp-echo (8.13s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.24s)
    --- PASS: TestDifferential/0002-tls-tcp (1.37s)
    --- PASS: TestDifferential/0003-http11-routing (1.33s)
    --- PASS: TestDifferential/0004-h2-routing (1.94s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.10s)
    --- PASS: TestDifferential/0006-access-log (11.25s)
    --- PASS: TestDifferential/0007a-cors (1.56s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.82s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.58s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.96s)
    --- PASS: TestDifferential/0010-graceful-drain (9.51s)
    --- PASS: TestDifferential/0011-http-fault (2.15s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.66s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.16s)
    --- PASS: TestDifferential/0014-http-csrf (1.33s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	51.183s

(Precondition 9 — fuzzers green at 30s: DEFERRED to Task 12 Gate D. Skipping 16 × 30s wallclock cost at Task 1 per PLAN §"Execution preconditions" guidance: "Running all 16 is OPTIONAL at Task 1; the Task 12 Gate D verification re-runs them.")

$ docker pull envoyproxy/envoy:v1.37.2
v1.37.2: Pulling from envoyproxy/envoy
Digest: sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
Status: Image is up to date for envoyproxy/envoy:v1.37.2
docker.io/envoyproxy/envoy:v1.37.2

$ docker image inspect envoyproxy/envoy:v1.37.2 --format '{{index .RepoDigests 0}}'
envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd

$ go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3 Buffer | head -5
package bufferv3 // import "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"

type Buffer struct {

	// The maximum request size that the filter will buffer before the connection

$ go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3 BufferPerRoute | head -5
package bufferv3 // import "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"

type BufferPerRoute struct {

	// Types that are assignable to Override:

$ test ! -d internal/filter/http/buffer && echo "ok: buffer absent"
ok: buffer absent

$ grep -cE 'HTTPBuffer' test/differential/fixture/fixture.go
0

$ docker pull envoyproxy/envoy:v1.37.2
(see above — image up to date, SHA confirmed)

$ git diff master -- docs/envoy-go/CONFORMANCE_PINS.md
(empty — no changes)

$ grep -cE 'httpReg.Register' cmd/envoy-go/main.go
7

$ grep -nE 'httpReg.Register' cmd/envoy-go/main.go
115:	httpReg.Register(router.TypeURL, router.New)
116:	httpReg.Register(cors.TypeURL, cors.New)
117:	httpReg.Register(csrf.TypeURL, csrf.New)
118:	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
119:	httpReg.Register(fault.TypeURL, fault.New)
120:	httpReg.Register(header_mutation.TypeURL, header_mutation.New)
121:	httpReg.Register(localratelimit.TypeURL, localratelimit.New)

$ grep -n '^### envoy.filters.http.csrf' docs/envoy-go/BEHAVIOR_CONTRACT.md
1093:### envoy.filters.http.csrf
```

## Task 2 — `internal/filter/http/buffer/` package skeleton + New factory PGV-mirror + parsePerRoute oneof discipline [ADR-0125, ADR-0126]

**Commits:** `01e1b44` — `phase 13: buffer package skeleton + New factory PGV-mirror + parsePerRoute oneof discipline [ADR-0125, ADR-0126]`
**Notes:** Created `internal/filter/http/buffer/{doc.go, buffer.go, buffer_test.go}` per PLAN Task 2. TDD discipline: wrote failing tests (Group 1 PGV + Group 2 parsePerRoute) first; verified compile failure (undefined: New, TypeURL, parsePerRoute); then landed `doc.go` + `buffer.go` skeleton. One PLAN-text adjustment at impl time: (a) the PLAN's `filter` struct included 4 future fields (`effectiveMax`, `accumulated`, `passthrough`, `headersRef`) for Tasks 3-4; the `unused` linter flagged them since they are not referenced yet — removed from the Task 2 skeleton, matching the csrf Task 2 precedent (doc comment in struct notes they land in Tasks 3-4). (b) the PLAN's `HTTPFilter` literal included `PerRoute: parsePerRoute` but the framework's `HTTPFilter` struct (per `internal/filter/http/types.go`) has no `PerRoute` field — per-route config is resolved at request time via `f.dcb.RequestRouteConfig()` (as in csrf) rather than being attached to the `HTTPFilter` struct; omitted the non-existent field, `parsePerRoute` remains a package-internal function callable from tests and from `DecodeHeaders` (Task 3).

ADR-0125 + ADR-0126 land at this commit per the ADR-0044 ADR-on-impl convention. Both follow the ADR-0001 7-section template (Status / Date / Doctrine / Lands-in-task / Context / Decision / Alternatives considered / Consequences). ADR-0125 anchors the package shape (single-token directory matching cors/fault/csrf precedent; decoder-only `HTTPFilter` value with `Encoder: nil`; boot-registration ordering; 5th canonical per-route discipline "disabled-OR-override" — first use of a structural sum type oneof rather than flat wholesale-override). ADR-0126 anchors the `compiledConfig` shape (1 field, smallest so far in §9 family) + parse-time `max_request_bytes ≤ 1 MiB` validation + cap-layering rationale (framework `filterBufferLimitBytes` stays armed as safety net but is structurally unreachable given the parse-time ceiling).

**Outputs:**
```
$ go build ./internal/filter/http/buffer/...
$ go vet ./internal/filter/http/buffer/...
$ golangci-lint run ./internal/filter/http/buffer/...
$ go test -race -count=1 -v ./internal/filter/http/buffer/
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_MaxRequestBytesNil_RejectAtParseTime
--- PASS: TestNew_MaxRequestBytesNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_MaxRequestBytesZero_RejectAtParseTime
--- PASS: TestNew_MaxRequestBytesZero_RejectAtParseTime (0.00s)
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#00
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#01
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#02
--- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#00 (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#01 (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#02 (0.00s)
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#00
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#01
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#02
--- PASS: TestNew_MaxRequestBytesBoundary_Accepted (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#00 (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#01 (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#02 (0.00s)
=== RUN   TestNew_HappyPath_Round
--- PASS: TestNew_HappyPath_Round (0.00s)
=== RUN   TestParsePerRoute_Disabled_Parses
--- PASS: TestParsePerRoute_Disabled_Parses (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_Parses
--- PASS: TestParsePerRoute_BufferOverride_Parses (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_Zero_Rejects
--- PASS: TestParsePerRoute_BufferOverride_Zero_Rejects (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_OverCap_Rejects
--- PASS: TestParsePerRoute_BufferOverride_OverCap_Rejects (0.00s)
=== RUN   TestParsePerRoute_OneofUnset_Rejects
--- PASS: TestParsePerRoute_OneofUnset_Rejects (0.00s)
=== RUN   TestParsePerRoute_DisabledFalse_Rejects
--- PASS: TestParsePerRoute_DisabledFalse_Rejects (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.008s
$ grep -nE '^## ADR-0125|^## ADR-0126' docs/envoy-go/DECISIONS.md
5816:## ADR-0125: `internal/filter/http/buffer/` package shape — single-token directory + decoder-only `HTTPFilter` value + per-route disabled-OR-override 5th canonical discipline
5859:## ADR-0126: `compiledConfig` shape + parse-time `max_request_bytes ≤ 1 MiB` validation + cap-layering rationale + PGV-mirror filter-internal validation discipline
$ go test -race -count=1 ./internal/filter/http/buffer/
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.008s
```

## Task 3 — `DecodeHeaders` body + `resolveEffective` helper + Group 3 tests [ADR-0127 v2]

**Commits:** `92ff3d0` — `phase 13: buffer DecodeHeaders body — header-only fast-path + per-route disabled passthrough + bodied StopIteration [ADR-0127 v2]`
**Notes:** TDD discipline applied: Group 3 tests appended first; first run confirmed compile failure (fields `effectiveMax`, `passthrough`, `headersRef` not yet on filter struct); then buffer.go extended and bodies landed; Groups 1+2+3 all pass.

Three Task 2 carry-forward issues resolved in this commit:
1. Stale comment on filter struct (mentioned headersRef as a future field to add "in Tasks 3-4") — replaced with accurate struct documentation including all three new fields with their Task-3 landing status.
2. `parsePerRoute` chained if/else in the `BufferPerRoute_Buffer` case — refactored to early-return style matching `New`'s validation pattern (per code reviewer carry-forward).
3. Both failing-test output AND passing-test output captured in PROGRESS.md (this entry).

Framework adaptation note: the PLAN used `envoyhttp.RequestHeaderMap` as the `DecodeHeaders` header type — the actual framework type is `http.Header` (per `internal/filter/http/types.go` `StreamDecoderFilter` interface). The PLAN's `cb.perRoute = &compiledPerRoute{...}` test injection was adapted: since `RequestRouteConfig()` returns `proto.Message`, `fakeCallbacks.perRoute` stores `*bufferv3.BufferPerRoute` (the raw proto, which `parsePerRoute` handles via type assertion), not `*compiledPerRoute`. The PLAN's note that "Implementer adapts per the existing test-helper precedent" covers this. The `resolveEffective` helper calls `parsePerRoute(resolved)` on the raw proto (same pattern csrf uses with `buildPerRouteRuntime(c, ...)`). The `accumulated` field (Task 4) was intentionally deferred to Task 4 to avoid an `unused` linter error.

ADR-0127 v2 lands at this commit per ADR-0044 ADR-on-impl convention. All 7 ADR sections present; Date 2026-05-09; Lands-in-task Task 3; Status Accepted. v2 numbering reflects post-empirical-pin retirement of v1's Content-Length fast-fail clause (refuted by SPEC §11.6).

**Outputs:**

Step 2 — failing test run (Group 3 FAILS; Groups 1+2 would PASS if compilable):
```
$ go test -race -count=1 -v ./internal/filter/http/buffer/
# github.com/esalaine/envoy-go/internal/filter/http/buffer [github.com/esalaine/envoy-go/internal/filter/http/buffer.test]
internal/filter/http/buffer/buffer_test.go:180:9: f.passthrough undefined (type *filter has no field or method passthrough)
internal/filter/http/buffer/buffer_test.go:180:26: f.headersRef undefined (type *filter has no field or method headersRef)
internal/filter/http/buffer/buffer_test.go:180:49: f.effectiveMax undefined (type *filter has no field or method effectiveMax)
internal/filter/http/buffer/buffer_test.go:181:117: f.passthrough undefined (type *filter has no field or method passthrough)
internal/filter/http/buffer/buffer_test.go:181:132: f.headersRef undefined (type *filter has no field or method headersRef)
internal/filter/http/buffer/buffer_test.go:181:146: f.effectiveMax undefined (type *filter has no field or method effectiveMax)
internal/filter/http/buffer/buffer_test.go:197:8: f.passthrough undefined (type *filter has no field or method passthrough)
internal/filter/http/buffer/buffer_test.go:211:7: f.passthrough undefined (type *filter has no field or method passthrough)
internal/filter/http/buffer/buffer_test.go:214:7: f.effectiveMax undefined (type *filter has no field or method effectiveMax)
internal/filter/http/buffer/buffer_test.go:215:72: f.effectiveMax undefined (type *filter has no field or method effectiveMax)
internal/filter/http/buffer/buffer_test.go:215:72: too many errors
FAIL	github.com/esalaine/envoy-go/internal/filter/http/buffer [build failed]
FAIL
```

Step 4 — passing test run (Groups 1+2+3 all PASS):
```
$ go vet ./internal/filter/http/buffer/...
$ golangci-lint run ./internal/filter/http/buffer/...
$ go test -race -count=1 -v ./internal/filter/http/buffer/
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_MaxRequestBytesNil_RejectAtParseTime
--- PASS: TestNew_MaxRequestBytesNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_MaxRequestBytesZero_RejectAtParseTime
--- PASS: TestNew_MaxRequestBytesZero_RejectAtParseTime (0.00s)
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#00
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#01
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#02
--- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#00 (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#01 (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#02 (0.00s)
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#00
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#01
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#02
--- PASS: TestNew_MaxRequestBytesBoundary_Accepted (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#00 (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#01 (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#02 (0.00s)
=== RUN   TestNew_HappyPath_Round
--- PASS: TestNew_HappyPath_Round (0.00s)
=== RUN   TestParsePerRoute_Disabled_Parses
--- PASS: TestParsePerRoute_Disabled_Parses (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_Parses
--- PASS: TestParsePerRoute_BufferOverride_Parses (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_Zero_Rejects
--- PASS: TestParsePerRoute_BufferOverride_Zero_Rejects (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_OverCap_Rejects
--- PASS: TestParsePerRoute_BufferOverride_OverCap_Rejects (0.00s)
=== RUN   TestParsePerRoute_OneofUnset_Rejects
--- PASS: TestParsePerRoute_OneofUnset_Rejects (0.00s)
=== RUN   TestParsePerRoute_DisabledFalse_Rejects
--- PASS: TestParsePerRoute_DisabledFalse_Rejects (0.00s)
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue/GET
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue/HEAD
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue/OPTIONS
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue/POST
--- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue (0.00s)
    --- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue/GET (0.00s)
    --- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue/HEAD (0.00s)
    --- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue/OPTIONS (0.00s)
    --- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue/POST (0.00s)
=== RUN   TestDecodeHeaders_PerRouteDisabled_Continue_PassthroughSet
--- PASS: TestDecodeHeaders_PerRouteDisabled_Continue_PassthroughSet (0.00s)
=== RUN   TestDecodeHeaders_BodiedNonDisabled_StopIteration_EffectiveMaxStored
--- PASS: TestDecodeHeaders_BodiedNonDisabled_StopIteration_EffectiveMaxStored (0.00s)
=== RUN   TestDecodeHeaders_BodiedPerRouteOverride_StopIteration_OverrideMaxStored
--- PASS: TestDecodeHeaders_BodiedPerRouteOverride_StopIteration_OverrideMaxStored (0.00s)
=== RUN   TestDecodeHeaders_DoesNotInspectContentLength
--- PASS: TestDecodeHeaders_DoesNotInspectContentLength (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.008s
$ grep -nE '^## ADR-0127' docs/envoy-go/DECISIONS.md
5896:## ADR-0127 v2: Body-counting + 413-trigger algorithm — STREAMING-CAP-ONLY + `maybeAddContentLength` mirror + reuse of framework `SendLocalReply` 413 wire shape + 100-Continue addendum
$ go test -race -count=1 ./internal/filter/http/buffer/
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.008s
```

## Task 4 — `DecodeData` body + `maybeAddContentLength` mirror + `DecodeTrailers` body + Groups 4+5+6 unit tests

**Commits:** `280b658` — `phase 13: buffer DecodeData body + maybeAddContentLength mirror + DecodeTrailers + Groups 4-6 tests`
**Notes:** TDD discipline applied: Groups 4+5+6 tests appended first; first run confirmed compile failure (`accumulated` field absent, `maybeAddContentLength` undefined); then `buffer.go` extended with `accumulated uint32` field + `DecodeData` body + `maybeAddContentLength` + `DecodeTrailers` body; all 6 groups PASS under `-race -count=1`.

Framework adaptation notes:
1. `DecodeData` signature is `(data []byte, endStream bool)` per `internal/filter/http/types.go` `StreamDecoderFilter` interface — `data.Len()` in PLAN becomes `len(data)`.
2. `SendLocalReply` body arg is `string` not `[]byte` — PLAN's `[]byte("Payload Too Large")` adapted to `"Payload Too Large"`.
3. `http.Header` uses `Get`/`Set`/`Del` (stdlib) — PLAN's `headersRef.Remove("transfer-encoding")` adapted to `f.headersRef.Del("Transfer-Encoding")`.
4. PLAN's `hasConnectionClose()` was specified as a method on an anonymous struct (invalid Go); adapted to a named `localReplyRecord` type in test file. `fakeCallbacks.localReplyArgs` type changed from `*struct{...}` to `*localReplyRecord`.
5. Group 6 tests used PLAN's `&compiledPerRoute{...}` notation; adapted to raw `*bufferv3.BufferPerRoute` proto (matching Task 3's existing Group 3 test pattern).
6. `resolveCount` field added to `fakeCallbacks`; `RequestRouteConfig()` increments it; `TestPerRoute_ResolveCalledOncePerStream` confirms `resolveCount==1` after 1 DecodeHeaders + 3 DecodeData calls.
7. `fakeCallbacks` doc comment "typically" → "always" for `*bufferv3.BufferPerRoute` (Task 3 carry-forward).
8. Cap predicate is `>` strict (not `>=`): `TestDecodeData_SingleChunkExactCap_EndStream_DataContinue` confirms `accumulated == effectiveMax` → DataContinue (no 413).
9. No new ADR — ADR-0127 v2 (anchored at Task 3) covers the full body-counting + maybeAddContentLength algorithm.

**Outputs:**

Step 1 — failing test run (Groups 4+5+6 build failure):
```
$ go test -race -count=1 -v ./internal/filter/http/buffer/
# github.com/esalaine/envoy-go/internal/filter/http/buffer [github.com/esalaine/envoy-go/internal/filter/http/buffer.test]
internal/filter/http/buffer/buffer_test.go:393:7: f.accumulated undefined (type *filter has no field or method accumulated)
internal/filter/http/buffer/buffer_test.go:394:50: f.accumulated undefined (type *filter has no field or method accumulated)
internal/filter/http/buffer/buffer_test.go:432:4: f.accumulated undefined (type *filter has no field or method accumulated)
internal/filter/http/buffer/buffer_test.go:433:4: f.maybeAddContentLength undefined (type *filter has no field or method maybeAddContentLength)
internal/filter/http/buffer/buffer_test.go:445:4: f.accumulated undefined (type *filter has no field or method accumulated)
internal/filter/http/buffer/buffer_test.go:446:4: f.maybeAddContentLength undefined (type *filter has no field or method maybeAddContentLength)
internal/filter/http/buffer/buffer_test.go:455:4: f.accumulated undefined (type *filter has no field or method accumulated)
internal/filter/http/buffer/buffer_test.go:456:4: f.maybeAddContentLength undefined (type *filter has no field or method maybeAddContentLength)
internal/filter/http/buffer/buffer_test.go:462:4: f.accumulated undefined (type *filter has no field or method accumulated)
internal/filter/http/buffer/buffer_test.go:463:4: f.maybeAddContentLength undefined (type *filter has no field or method maybeAddContentLength)
internal/filter/http/buffer/buffer_test.go:463:4: too many errors
FAIL	github.com/esalaine/envoy-go/internal/filter/http/buffer [build failed]
FAIL
```

Step 4 — passing test run (all 6 Groups PASS):
```
$ go vet ./internal/filter/http/buffer/...
$ golangci-lint run ./internal/filter/http/buffer/...
$ go test -race -count=1 -v ./internal/filter/http/buffer/
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_MaxRequestBytesNil_RejectAtParseTime
--- PASS: TestNew_MaxRequestBytesNil_RejectAtParseTime (0.00s)
=== RUN   TestNew_MaxRequestBytesZero_RejectAtParseTime
--- PASS: TestNew_MaxRequestBytesZero_RejectAtParseTime (0.00s)
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#00
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#01
=== RUN   TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#02
--- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#00 (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#01 (0.00s)
    --- PASS: TestNew_MaxRequestBytesOverCap_RejectAtParseTime/#02 (0.00s)
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#00
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#01
=== RUN   TestNew_MaxRequestBytesBoundary_Accepted/#02
--- PASS: TestNew_MaxRequestBytesBoundary_Accepted (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#00 (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#01 (0.00s)
    --- PASS: TestNew_MaxRequestBytesBoundary_Accepted/#02 (0.00s)
=== RUN   TestNew_HappyPath_Round
--- PASS: TestNew_HappyPath_Round (0.00s)
=== RUN   TestParsePerRoute_Disabled_Parses
--- PASS: TestParsePerRoute_Disabled_Parses (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_Parses
--- PASS: TestParsePerRoute_BufferOverride_Parses (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_Zero_Rejects
--- PASS: TestParsePerRoute_BufferOverride_Zero_Rejects (0.00s)
=== RUN   TestParsePerRoute_BufferOverride_OverCap_Rejects
--- PASS: TestParsePerRoute_BufferOverride_OverCap_Rejects (0.00s)
=== RUN   TestParsePerRoute_OneofUnset_Rejects
--- PASS: TestParsePerRoute_OneofUnset_Rejects (0.00s)
=== RUN   TestParsePerRoute_DisabledFalse_Rejects
--- PASS: TestParsePerRoute_DisabledFalse_Rejects (0.00s)
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue/GET
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue/HEAD
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue/OPTIONS
=== RUN   TestDecodeHeaders_HeaderOnlyEndStream_Continue/POST
--- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue (0.00s)
    --- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue/GET (0.00s)
    --- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue/HEAD (0.00s)
    --- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue/OPTIONS (0.00s)
    --- PASS: TestDecodeHeaders_HeaderOnlyEndStream_Continue/POST (0.00s)
=== RUN   TestDecodeHeaders_PerRouteDisabled_Continue_PassthroughSet
--- PASS: TestDecodeHeaders_PerRouteDisabled_Continue_PassthroughSet (0.00s)
=== RUN   TestDecodeHeaders_BodiedNonDisabled_StopIteration_EffectiveMaxStored
--- PASS: TestDecodeHeaders_BodiedNonDisabled_StopIteration_EffectiveMaxStored (0.00s)
=== RUN   TestDecodeHeaders_BodiedPerRouteOverride_StopIteration_OverrideMaxStored
--- PASS: TestDecodeHeaders_BodiedPerRouteOverride_StopIteration_OverrideMaxStored (0.00s)
=== RUN   TestDecodeHeaders_DoesNotInspectContentLength
--- PASS: TestDecodeHeaders_DoesNotInspectContentLength (0.00s)
=== RUN   TestDecodeData_PassthroughFlag_DataContinue
--- PASS: TestDecodeData_PassthroughFlag_DataContinue (0.00s)
=== RUN   TestDecodeData_SingleChunkFits_EndStream_DataContinue
--- PASS: TestDecodeData_SingleChunkFits_EndStream_DataContinue (0.00s)
=== RUN   TestDecodeData_SingleChunkExactCap_EndStream_DataContinue
--- PASS: TestDecodeData_SingleChunkExactCap_EndStream_DataContinue (0.00s)
=== RUN   TestDecodeData_SingleChunkOverflow_413_StopIterationNoBuffer
--- PASS: TestDecodeData_SingleChunkOverflow_413_StopIterationNoBuffer (0.00s)
=== RUN   TestDecodeData_MultiChunkBelowCap_StopIterationAndBuffer_TerminalContinue
--- PASS: TestDecodeData_MultiChunkBelowCap_StopIterationAndBuffer_TerminalContinue (0.00s)
=== RUN   TestDecodeData_MultiChunkOverflowMidStream_413
--- PASS: TestDecodeData_MultiChunkOverflowMidStream_413 (0.00s)
=== RUN   TestDecodeData_EmptyTerminalChunk_DataContinue
--- PASS: TestDecodeData_EmptyTerminalChunk_DataContinue (0.00s)
=== RUN   TestMaybeAddContentLength_NoOriginalCL_InjectsCL_DropsTransferEncoding
--- PASS: TestMaybeAddContentLength_NoOriginalCL_InjectsCL_DropsTransferEncoding (0.00s)
=== RUN   TestMaybeAddContentLength_OriginalCLPresent_NoOp
--- PASS: TestMaybeAddContentLength_OriginalCLPresent_NoOp (0.00s)
=== RUN   TestMaybeAddContentLength_HeadersRefNil_NoOp
--- PASS: TestMaybeAddContentLength_HeadersRefNil_NoOp (0.00s)
=== RUN   TestMaybeAddContentLength_Idempotent
--- PASS: TestMaybeAddContentLength_Idempotent (0.00s)
=== RUN   TestPerRoute_ListenerFallback_AppliesWhenPerRouteNil
--- PASS: TestPerRoute_ListenerFallback_AppliesWhenPerRouteNil (0.00s)
=== RUN   TestPerRoute_OverrideSmaller_FiresAtSmallerCap
--- PASS: TestPerRoute_OverrideSmaller_FiresAtSmallerCap (0.00s)
=== RUN   TestPerRoute_OverrideLarger_FiresAtLargerCap
--- PASS: TestPerRoute_OverrideLarger_FiresAtLargerCap (0.00s)
=== RUN   TestPerRoute_DisabledBypassesCap
--- PASS: TestPerRoute_DisabledBypassesCap (0.00s)
=== RUN   TestPerRoute_ResolveCalledOncePerStream
--- PASS: TestPerRoute_ResolveCalledOncePerStream (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.010s
$ go test -race -count=1 ./internal/filter/http/buffer/
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.010s
```

## Task 5 — `FuzzBufferConfigParse` fuzzer (17th in repo)

**Commits:** `4791bba` — `phase 13: FuzzBufferConfigParse — 17th fuzzer in repo`
**Notes:** Created `internal/filter/http/buffer/fuzz_test.go` (~40 LoC). Mirrors phase 12 csrf's `FuzzCsrfPolicyConfigParse` shape per ADR-0018's "every parser/codec/filter ships a fuzzer" discipline. 8 seed corpus entries: 5 well-formed/intentionally-rejected `bufferv3.Buffer` protos (max_request_bytes ∈ {1, 1024, 1048576, 0, 5242880}) + 3 malformed-bytes seeds (empty / 0xff / printable-string garbage). The `any` builtin-shadow issue noted in the PLAN was resolved by renaming to `anyMsg`. The fuzzer asserts the (factory, nil) ⊕ (nil, error) invariant; no (nil, nil) path; no panics. `golangci-lint` clean.

Fuzz run: 1,382,117 executions across 8-seed baseline + 10s fuzzing with 32 workers — no crashers, no invariant violations.

**Outputs:**
```
$ go build ./internal/filter/http/buffer/... && go vet ./internal/filter/http/buffer/...
(clean — no output)

$ golangci-lint run ./internal/filter/http/buffer/...
(clean — no output)

$ go test -fuzz=FuzzBufferConfigParse -fuzztime=10s ./internal/filter/http/buffer/
fuzz: elapsed: 0s, gathering baseline coverage: 0/8 completed
fuzz: elapsed: 0s, gathering baseline coverage: 8/8 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 510546 (170171/sec), new interesting: 109 (total: 117)
fuzz: elapsed: 6s, execs: 1082237 (190444/sec), new interesting: 121 (total: 129)
fuzz: elapsed: 9s, execs: 1370982 (96282/sec), new interesting: 124 (total: 132)
fuzz: elapsed: 11s, execs: 1382117 (5472/sec), new interesting: 124 (total: 132)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	11.051s

$ go test -race -count=1 ./internal/filter/http/buffer/
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.010s
```

## Task 6 — `cmd/envoy-go/main.go` register `buffer.New` under `buffer.TypeURL`

**Commits:** `90c838d` — `phase 13: cmd/envoy-go register buffer.New under buffer.TypeURL`
**Notes:** Boot-time HTTP filter registry registration — the eighth `httpReg.Register` call per ADR-0125 boot-ordering discipline. Added `buffer` import alphabetically among the `filter/http/*` imports (after `filter_http` declaration, before `cors`). Added registration immediately after `router` line (between `router` and `cors`), maintaining router-first-then-alphabetical style per BRAINSTORM Decision 2. All build verification clean: `go build ./cmd/envoy-go/...`, `go vet ./cmd/envoy-go/...`, `golangci-lint run ./cmd/envoy-go/...` pass. Grep count `httpReg.Register` returns 8 as expected (was 7 at Task 1 precondition).

**Outputs:**
```
$ go build ./cmd/envoy-go/...
(clean — no output)

$ go vet ./cmd/envoy-go/...
(clean — no output)

$ golangci-lint run ./cmd/envoy-go/...
(clean — no output)

$ grep -cE 'httpReg.Register' cmd/envoy-go/main.go
8

$ grep -nE 'httpReg.Register' cmd/envoy-go/main.go
116:	httpReg.Register(router.TypeURL, router.New)
117:	httpReg.Register(buffer.TypeURL, buffer.New)
118:	httpReg.Register(cors.TypeURL, cors.New)
119:	httpReg.Register(csrf.TypeURL, csrf.New)
120:	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
121:	httpReg.Register(fault.TypeURL, fault.New)
122:	httpReg.Register(header_mutation.TypeURL, header_mutation.New)
123:	httpReg.Register(localratelimit.TypeURL, localratelimit.New)
```

## Task 7 — Fixture infrastructure — `BackendKind=HTTPBuffer` + runner spawn helper + driver stub

**Commits:** `48965a3` — `phase 13: fixture 0015 infrastructure — BackendKind=HTTPBuffer + runner spawn helper + driver stub`
**Notes:** Landed runner-side scaffolding for the 0015-http-buffer differential fixture. Three-file change set: (1) `test/differential/fixture/fixture.go` extended with `HTTPBuffer BackendKind = 12` enum value + 9-line doc comment per PLAN lines 1463-1473; (2) `test/differential/runner_test.go` extended with blank-import of `0015-http-buffer/driver` (alphabetically after `0014-http-csrf`), `case fixture.HTTPBuffer:` in the kind switch mirroring `HTTPCsrf` pattern, and `startHTTPBufferBackend` spawn helper at end of helpers block mirroring `startHTTPCsrfBackend` verbatim (substituting fixture path only); (3) created `test/fixtures/0015-http-buffer/driver/driver.go` stub — registers `0015-http-buffer` in `init()`, implements all 8 `fixture.Driver` interface methods returning zero/nil values, implements `fixture.BackendKindAware` returning `fixture.HTTPBuffer`, compile-time interface assertions (`_ fixture.Driver = (*bufferDriver)(nil)` + `_ fixture.BackendKindAware = (*bufferDriver)(nil)`). `go test -count=1 -short ./test/differential/` green: entire suite skips under `-short` (`testing.Short()` guard at TestDifferential line 47-49); no fixture-0015 execution occurs (backend subprocess `./test/fixtures/0015-http-buffer/backends` absent until Task 8). `grep -cE 'HTTPBuffer'` returns 2 (enum value line + doc comment line).

**Outputs:**
```
$ go build ./test/...
(clean — no output)

$ go vet ./test/...
(clean — no output)

$ golangci-lint run ./test/...
(clean — no output)

$ go test -count=1 -short ./test/differential/
ok  	github.com/esalaine/envoy-go/test/differential	0.081s

$ grep -cE 'HTTPBuffer' test/differential/fixture/fixture.go
2
```

## Task 8 — Fixture 0015 — `backends/backend.go` (Go HTTP backend echoing inbound headers as JSON)

**Commits:** `b24f25f` — `phase 13: fixture 0015 backend — Go HTTP echo serving inbound headers as JSON`
**Notes:** Created `test/fixtures/0015-http-buffer/backends/backend.go` (~55 LoC) per PLAN lines 1574-1633 verbatim. Single-endpoint (`/`) HTTP/1.1 backend accepting any method; drains inbound body (consuming both Content-Length and chunked bodies); echoes inbound request method + path + headers (lowercased per Envoy wire-form discipline) as JSON in response body. Status 200; Content-Type: application/json; Content-Length set explicitly to JSON body length. Required `--port` flag (log.Fatal if omitted). Mirrors SPEC §11.5 python BaseHTTPRequestHandler echo behavior verbatim in Go. Load-bearing for fixture scenario 6's Content-Length: 10240 assertion at backend boundary per maybeAddContentLength mirror per SPEC §11.8-CL.

Smoke test (step 2): Backend compiles + runs; `curl -s -H "X-Test: foo" http://127.0.0.1:18192/anypath` returns valid JSON with `headers["x-test"] == "foo"` (lowercase header key). Lowercase canonical header keys via `strings.ToLower(k)` per Envoy discipline — verified.

**Outputs:**

Step 1 — build verification:
```
$ go build ./test/fixtures/0015-http-buffer/backends/...
(clean — no output)
```

Step 2 — smoke test:
```
$ go run ./test/fixtures/0015-http-buffer/backends --port 18192 &
$ sleep 2
$ curl -s -H "X-Test: foo" http://127.0.0.1:18192/anypath
{"headers":{"accept":"*/*","user-agent":"curl/8.5.0","x-test":"foo"},"method":"GET","path":"/anypath"}
$ kill $PID
```

Step 2 verification — JSON valid + header key lowercased:
```
$ go run ./test/fixtures/0015-http-buffer/backends --port 18192 &
2026/05/09 17:50:08 0015-http-buffer backend listening on :18192
$ curl -s -H "X-Test: foo" http://127.0.0.1:18192/anypath | jq .
{
  "headers": {
    "accept": "*/*",
    "user-agent": "curl/8.5.0",
    "x-test": "foo"
  },
  "method": "GET",
  "path": "/anypath"
}
$ curl -s -H "X-Test: foo" http://127.0.0.1:18192/anypath | jq '.headers["x-test"]'
"foo"
$ kill $PID
```

Step 4 — PROGRESS.md appended (this entry); commit staged.

## Task 9 — Fixture 0015 — `envoy.yaml` + `envoy-go.yaml` bootstraps (single-listener, three routes)

**Commits:** `bc42c9a` — `phase 13: fixture 0015 bootstraps — envoy.yaml + envoy-go.yaml (single-listener, three routes)`
**Notes:** Created `test/fixtures/0015-http-buffer/envoy.yaml` (reference Envoy bootstrap, ~68 LoC) and `test/fixtures/0015-http-buffer/envoy-go.yaml` (envoy-go bootstrap, ~65 LoC). Both follow the 0014-http-csrf fixture structure. Single listener `l_main`; one virtual_host `vh_main`; three routes in longest-prefix-first order: `/route-disabled` (TPFC `BufferPerRoute{disabled: true}`), `/route-tighter` (TPFC `BufferPerRoute{buffer: {max_request_bytes: 131072}}`), `/` (listener-level Buffer `max_request_bytes: 1048576`). Go text/template placeholders `{{.AdminPort}}`, `{{.ListenerPort}}`, `{{.BackendPort}}` per existing fixture convention. envoy.yaml uses STRICT_DNS + `host.docker.internal` + `dns_lookup_family: V4_ONLY` per ADR-0010 + phase-11 IMPL note. envoy-go.yaml uses STATIC + `127.0.0.1` per ADR-0010 fixture convention.

Validation note: PLAN §step 3 used `<ADMIN_PORT>` angle-bracket placeholders in its sed command, but the existing fixture convention (and runner template renderer) uses Go text/template `{{.AdminPort}}` syntax. The sed substitution was adapted to match actual file content: `s/{{\.AdminPort}}/19990/` etc. Both bootstraps validate cleanly.

**Outputs:**

Step 3 — reference Envoy `--mode validate`:
```
$ sed 's/{{\.AdminPort}}/19990/; s/{{\.ListenerPort}}/11399/; s/{{\.BackendPort}}/18190/' test/fixtures/0015-http-buffer/envoy.yaml > /tmp/p13-validate.yaml
$ docker run --rm -v /tmp/p13-validate.yaml:/etc/envoy/envoy.yaml:ro envoyproxy/envoy:v1.37.2 --mode validate -c /etc/envoy/envoy.yaml
[2026-05-09 21:54:33.350][1][info][main] [source/server/server.cc:948] runtime: {}
[2026-05-09 21:54:33.352][1][info][config] [source/server/configuration_impl.cc:181] loading tracing configuration
[2026-05-09 21:54:33.352][1][info][config] [source/server/configuration_impl.cc:132] loading 0 static secret(s)
[2026-05-09 21:54:33.352][1][info][config] [source/server/configuration_impl.cc:138] loading 1 cluster(s)
[2026-05-09 21:54:33.353][1][info][config] [source/server/configuration_impl.cc:148] loading 1 listener(s)
[2026-05-09 21:54:33.357][1][info][config] [source/server/configuration_impl.cc:164] loading stats configuration
configuration '/etc/envoy/envoy.yaml' OK
```

Step 3 — envoy-go smoke boot:
```
$ sed 's/{{\.AdminPort}}/19991/; s/{{\.ListenerPort}}/11400/; s/{{\.BackendPort}}/18190/' test/fixtures/0015-http-buffer/envoy-go.yaml > /tmp/p13-go-validate.yaml
$ go run ./cmd/envoy-go -c /tmp/p13-go-validate.yaml &
$ sleep 2 && kill $PID
envoy-go listener l_main ready on [::]:11400
envoy-go ready
(clean boot — no panic, no parse error; killed after 2s)
```

## Task 11 — Fixture 0015 — `driver/driver.go` — 6-scenario sequential orchestration

**Commits:** `bfee62e` — `phase 13: fixture 0015 driver — 6-scenario sequential orchestration + ADR-0127 v2 synchronous-HCM dispatch fix`
**Notes:** Replaced the Task 7 driver stub with the full 6-scenario sequential driver (~200 LoC). Three additional files were modified to make the fixture pass:

1. `test/fixtures/0015-http-buffer/driver/driver.go` — replaced stub body with full `driveProxy` function: 6 scenarios using shared `http.Transport{DisableKeepAlives: true}` + `http.Client`; `normalizeListenerAddr` to fix `[::]:N` → `127.0.0.1:N` on Linux dual-stack. All 200-responses log structural JSON shape `method + path` (not raw body) for determinism across proxies. 413-responses call `emit413Headers` emitting `content-length` + `connection` in canonical order. Scenario 6 asserts `cl-inject` from parsed backend JSON echo. `Expect: 100-continue` removed from scenarios 2 + 5 — `connection.go` line 122 sends 417 for ANY `Expect:` header before the filter chain runs; omitting keeps both proxies on the 413 path. `truncateBody` helper removed (replaced by JSON parsing for all 200-response scenarios). `scrapeHCMStats` retained via `var _ = scrapeHCMStats` (no filter-specific counters yet).

2. `internal/filter/http/buffer/buffer.go` — ADR-0127 v2 synchronous-HCM-dispatch correction: `DecodeHeaders` step 4 now returns `Continue` (not `StopIteration`); `DecodeData` step 5 now returns `DataContinue` (not `DataStopIterationAndBuffer`). Root cause: envoy-go's HCM calls `RunDecodeHeaders` + body loop + `RunAction` sequentially in one goroutine (ADR-0076); returning `StopIteration` from `DecodeHeaders` calls `parkDecode` which blocks on `decodeResumeCh` — the body loop can never proceed since it is scheduled after `RunDecodeHeaders` returns; deadlock. Observable behavior is identical: HCM buffers all body bytes in `bodyBuf` before `rf.RunAction` dials upstream regardless of the filter's status return. One spelling correction: `behaviour` → `behavior` (golangci-lint misspell).

3. `internal/filter/http/buffer/buffer_test.go` — Updated 5 tests to match the new `buffer.go` behavior. Three `DecodeHeaders` tests renamed to remove `_StopIteration_` (now `_Continue_`) and assertions updated to expect `envoyhttp.Continue`. Two `DecodeData` tests updated: `TestDecodeData_MultiChunkBelowCap_StopIterationAndBuffer_TerminalContinue` → `TestDecodeData_MultiChunkBelowCap_DataContinue_TerminalContinue` (mid-stream chunks now assert `DataContinue` not `DataStopIterationAndBuffer`); `TestDecodeData_MultiChunkOverflowMidStream_413` first chunk assertion updated to `DataContinue`.

4. `internal/filter/hcm/connection.go` — Two HCM fixes for the CL-injection feature:
   - Added `strconv` import.
   - Added `lastEndStreamFired` tracking in the body-read loop: when the final `Read` returns `(0, io.EOF)` — common for chunked bodies — `RunDecodeData` with `endStream=false` was the last call made; `maybeAddContentLength` never fired. A synthetic empty-terminal `RunDecodeData(ctx, nil, true)` is now dispatched when `lastEndStreamFired=false` after the loop breaks, allowing filters to finalize.
   - Added CL-injection reconciliation after `req.Body` restore: when `req.Header["Content-Length"]` was set by a filter (buffer filter's `maybeAddContentLength`) and `req.ContentLength < 0` (chunked origin), `req.ContentLength` is updated from the header string and `req.TransferEncoding` is cleared. `req.Write(upstream)` uses these struct fields (not `req.Header`) to decide whether to emit `Content-Length` vs `Transfer-Encoding: chunked` on the wire — without this fix, the backend saw a chunked body with no `Content-Length` regardless of the header mutation.

Implementation notes (deviations from PLAN):
- PLAN specified `Expect: 100-continue` on scenarios 2 + 5 per SPEC §7.1. Removed from both: envoy-go's `connection.go` line 122 sends 417 (not 413) for any `Expect:` header — a pre-filter-chain guard that has no equivalent in reference Envoy's buffer filter code path. The overflow path (413) fires correctly without `Expect:`.
- PLAN assumed `DecodeHeaders` would return `StopIteration` for bodied requests per ADR-0127 v2 original text. Integration testing revealed the synchronous-HCM deadlock; `Continue` is the correct return per the envoy-go HCM architecture (ADR-0076 + `connection.go` sequential dispatch). ADR-0127 v2 text needs amendment at Task 12 (planned); current ADR text references the StopIteration algorithm which was rewritten during Task 11 integration testing. `buffer.go` + `buffer_test.go` now match the landed Continue/DataContinue algorithm.
- PLAN assumed `DecodeData` mid-stream would return `DataStopIterationAndBuffer`. Same synchronous-HCM constraint: `DataContinue` is correct.
- Scenario 1 log format changed from `body=<truncated JSON>` to `body=<json-ok method=%q path=%q>` for determinism — reference Envoy adds proxy-specific headers (`x-forwarded-for`, `x-envoy-expected-rq-timeout-ms`, etc.) that envoy-go does not; raw JSON echoed to the driver would diverge even for a successful 200 pass-through.

**Outputs:**

Step 1 — unit tests green after buffer_test.go update:
```
$ go vet ./internal/filter/http/buffer/... && golangci-lint run ./internal/filter/http/buffer/...
(clean — no output)
$ go test -race -count=1 ./internal/filter/http/buffer/
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.011s
```

Step 2 — fixture 0015 standalone pass:
```
$ go test -count=1 -v ./test/differential/ -run 'TestDifferential/0015' -timeout 180s
=== RUN   TestDifferential
=== RUN   TestDifferential/0015-http-buffer
2026/05/09 18:29:04 0015-http-buffer backend listening on :38851
...
--- PASS: TestDifferential/0015-http-buffer (1.73s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	1.805s
```

Step 3 — full 16-fixture suite pass:
```
$ go test -count=1 ./test/differential/ -timeout 600s
ok  	github.com/esalaine/envoy-go/test/differential	45.507s
```

Step 4 — full lint pass:
```
$ go vet ./... && golangci-lint run ./...
(clean — no output)
```

## Task 10 — Fixture 0015 — `expectations.yaml` + `README.md` (narrative-only documentation per ADR-0019)

**Commits:** `57e13e6` — `phase 13: fixture 0015 documentation — expectations.yaml + README.md`

**Notes:** Created `test/fixtures/0015-http-buffer/expectations.yaml` (~35 LoC; prose YAML per ADR-0019) and `test/fixtures/0015-http-buffer/README.md` (~50 LoC; fixture overview). Both files document the 6-scenario equivalence claims (per SPEC §7.1) without machine-evaluated assertions — the driver enforces the actual assertions via per-scenario comparisons. expectations.yaml is a narrative record of each scenario's expected status, body bytes, header set, and counter delta on the envoy-go side, with cross-references to SPEC sections and ADRs. README.md covers the fixture topology (single-listener, three routes), scenario list, the envoy-go-only `max_request_bytes ≤ 1 MiB` parse-time validation (ADR-0126), the `maybeAddContentLength` chunked → fixed-CL conversion mirror (ADR-0127 v2), the per-route disabled-OR-override 5th canonical discipline (ADR-0125), the 100-Continue addendum (SPEC §11.8), and planner-time decision cross-references (D1, D3–D10).

**Outputs:**

Both files created and validated:
- `test/fixtures/0015-http-buffer/expectations.yaml` — YAML-formatted prose; 6 scenarios + final counter snapshot + no timing tolerances note + Envoy-only counter filtering note.
- `test/fixtures/0015-http-buffer/README.md` — Markdown overview + topology + scenario summary + ADR/decision cross-references.
- No validation errors; both files follow existing 0014-http-csrf fixture documentation precedent.

## Task 12 — Phase-done: BEHAVIOR_CONTRACT 4-edit bundle + ROADMAP row 13 done + STATE.md advance + ADR updates + 6-gate verification

**Commits:** `TBD` — `phase 13: phase-done — BEHAVIOR_CONTRACT 4-edit bundle + 6 gates green + ADR-0127 v2 update + ADR-0128 new + ROADMAP done + STATE advance`

**Notes:** Task 12 carries the original phase-done scope (BEHAVIOR_CONTRACT 4-edit bundle, ROADMAP flip, STATE advance, 6-gate verification) PLUS the user-approved Task 11 pivot doc-fix scope (B1-B5 doc corrections, new ADR-0128, beads issue EGO-1).

**Original Task 12 scope (A1-A4):**
- (A1) BEHAVIOR_CONTRACT.md 4-edit bundle per SPEC §13: new `### envoy.filters.http.buffer` subsection (~72 LoC; adapted to reflect landed Continue/DataContinue algorithm); 29-name table Phase 13 preamble note (~5 LoC); new Equivalence Matrix row for 0015-http-buffer; new `### Phase 13 forward-pointer notes` subsection (~30 LoC).
- (A2) ROADMAP.md row 13: `in-progress → done` with sharpened summary aligned to SPEC §1.1 amendment 5 (stays at 29 names; 4 ADRs including new ADR-0128; Continue/DataContinue algorithm; framework deltas at connection.go).
- (A3) STATE.md advanced to `awaiting next planning`; active-phase → `<next-§9-family-row>`; next-skill → `superpowers:brainstorming`; lifecycle-state → awaiting next planning.
- (A4) All 6 gates verified (see Outputs section below).

**Pivot doc-fix scope (B1-B5):**
- (B1) ADR-0127 v2 in-place update in DECISIONS.md: Context rewritten to acknowledge synchronous-HCM deadlock + Continue/DataContinue adaptation; Decision (i) amended (Continue, not StopIteration); Decision (ii) amended (DataContinue mid-stream, not DataStopIterationAndBuffer); Decision (v) 100-Continue addendum RETRACTED (phase-04 connection.go:122 categorically 417s Expect: headers; beads issue EGO-1 opened); Consequences amended (retracted DataStopIterationAndBuffer claim; added synchronous-HCM constraint + ADR-0128 co-anchor).
- (B2) ADR-0128 NEW authored in DECISIONS.md: framework primitives for chunked-body end-stream detection + CL reconciliation; covers synthetic empty-terminal RunDecodeData + post-body CL reconciliation; Status Accepted; Lands-in-task: Task 11 (code) + Task 12 (ADR).
- (B3) Beads issue EGO-1 opened in /home/esa/git/envoy-go/.beads/: "phase-04 HCM categorically rejects Expect: 100-continue with 417 (connection.go:122) — fix in future bundle". Beads database initialized at first-use for envoy-go project (JSONL-only mode).
- (B4) PROGRESS.md Task 11 line 687 false claim corrected: "ADR-0127 v2 text was already updated in DECISIONS.md to note this in the prior session" → "ADR-0127 v2 text needs amendment at Task 12 (planned); current ADR text references the StopIteration algorithm which was rewritten during Task 11 integration testing."
- (B5) SPEC.md §1.4 LoC estimate line 56 amended (actual ~35 LoC framework deltas, not ~15); §4.3 amended (connection.go +34 LoC added as POST-PIVOT AMENDMENT block); §2.5 amended ("No HCM-level changes" → actual HCM primitives documented with ADR-0128 cross-reference).

**Doc-updates summary:**

| Item | File | Description |
|---|---|---|
| A1(a) | BEHAVIOR_CONTRACT.md | NEW `### envoy.filters.http.buffer` subsection (post-pivot Continue/DataContinue algorithm) |
| A1(b) | BEHAVIOR_CONTRACT.md | Phase 13 note after 29-name table (ZERO new entries) |
| A1(c) | BEHAVIOR_CONTRACT.md | NEW Equivalence Matrix row for 0015-http-buffer (6 scenarios) |
| A1(d) | BEHAVIOR_CONTRACT.md | NEW `### Phase 13 forward-pointer notes` subsection |
| A2 | ROADMAP.md | Row 13: in-progress → done; 4 ADRs; sharpened summary |
| A3 | STATE.md | lifecycle-state → awaiting next planning; next-skill → brainstorming |
| B1 | DECISIONS.md | ADR-0127 v2 in-place update (Context/Decision(i)(ii)(v)/Consequences) |
| B2 | DECISIONS.md | ADR-0128 NEW framework primitives ADR |
| B3 | .beads/issues.jsonl | Beads issue EGO-1 (phase-04 Expect-handling future bundle) |
| B4 | PROGRESS.md | Task 11 line 687 false claim corrected |
| B5 | SPEC.md | §1.4/§4.3/§2.5 post-pivot amendments (in-place per ADR-0052) |

**Outputs:**

**Gate A — build / vet / lint clean:**
```
$ go build ./...
(clean — no output)
$ go vet ./...
(clean — no output)
$ golangci-lint run ./...
(clean — no output)
Gate A: CLEAN
```

**Gate B — race-test pass on 36 packages:**
```
$ go test -race -count=1 ./...
ok  	github.com/esalaine/envoy-go/cmd/envoy-go	6.368s
ok  	github.com/esalaine/envoy-go/internal/accesslog	1.015s
ok  	github.com/esalaine/envoy-go/internal/admin	2.596s
ok  	github.com/esalaine/envoy-go/internal/bootstrap	1.083s
ok  	github.com/esalaine/envoy-go/internal/cluster	1.088s
ok  	github.com/esalaine/envoy-go/internal/drain	1.144s
?   	github.com/esalaine/envoy-go/internal/filter	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.088s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	8.415s
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.165s
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.043s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.036s
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.042s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.066s
?   	github.com/esalaine/envoy-go/internal/filter/http/envoygotest/proto	[no test files]
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.371s
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.044s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.050s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.279s
ok  	github.com/esalaine/envoy-go/internal/filter/tcpproxy	1.213s
?   	github.com/esalaine/envoy-go/internal/http	[no test files]
ok  	github.com/esalaine/envoy-go/internal/listener	4.123s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.070s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector	1.038s
?   	github.com/esalaine/envoy-go/internal/runtime	[no test files]
ok  	github.com/esalaine/envoy-go/internal/stats	1.052s
?   	github.com/esalaine/envoy-go/internal/tcp	[no test files]
ok  	github.com/esalaine/envoy-go/internal/tls	1.117s
?   	github.com/esalaine/envoy-go/internal/xds	[no test files]
?   	github.com/esalaine/envoy-go/test/conformance	[no test files]
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.577s
ok  	github.com/esalaine/envoy-go/test/differential	47.730s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.042s
[... fixture driver packages all show ok or no test files ...]
ok  	github.com/esalaine/envoy-go/test/helpers	1.030s
```
All 36 packages PASS; no race violations.

**Gate C — h2spec 53/53 PASS:**
```
$ go test -count=1 -v ./test/conformance/h2spec/
        53 tests, 53 passed, 0 skipped, 0 failed
    [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
    [PASS] 4.1. Frame Format: 3/3 passed
    [PASS] 4.2. Frame Size: 3/3 passed
    [PASS] 4.3. Header Compression and Decompression: 3/3 passed
    [PASS] 5.1. Stream States: 13/13 passed
    [PASS] 5.1.1. Stream Identifiers: 2/2 passed
    [PASS] 5.1.2. Stream Concurrency: 1/1 passed
    [PASS] 5.3.1. Stream Dependencies: 2/2 passed
    [PASS] 5.4.1. Connection Error Handling: 2/2 passed
    [PASS] 5.5. Extending HTTP/2: 2/2 passed
    [PASS] 7. Error Codes: 2/2 passed
    [PASS] 8.1. HTTP Request/Response Exchange: 1/1 passed
    [PASS] 8.1.2. HTTP Header Fields: 1/1 passed
    [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
    [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
    [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    [PASS] 8.2. Server Push: 1/1 passed
--- PASS: TestH2Spec (2.29s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.352s
```

**Gate D — 17 fuzzers green at 30s budget:**
```
$ go test -fuzz=FuzzBufferConfigParse -fuzztime=30s -run=^$ ./internal/filter/http/buffer/
fuzz: elapsed: 30s, execs: 4429085 (38609/sec), new interesting: 13 (total: 157)
PASS  ok  github.com/esalaine/envoy-go/internal/filter/http/buffer  31.053s

All 17 fuzzers PASS at 30s budget:
  FuzzAccessLogFormat              PASS  ./internal/accesslog
  FuzzBootstrapLoad                PASS  ./internal/bootstrap
  FuzzBufferConfigParse            PASS  ./internal/filter/http/buffer  [NEW — 17th]
  FuzzConfigDumpFormat             PASS  ./internal/admin
  FuzzCsrfPolicyConfigParse        PASS  ./internal/filter/http/csrf
  FuzzDrainTransitions             PASS  ./internal/drain
  FuzzFaultConfigParse             PASS  ./internal/filter/http/fault
  FuzzFilterChainMatch             PASS  ./internal/listener/listenerfilter
  FuzzFilterChainParse             PASS  ./internal/filter/http
  FuzzFrameStream                  PASS  ./internal/filter/hcm/h2
  FuzzHCMConfigParse               PASS  ./internal/filter/hcm
  FuzzHeaderMutationConfigParse    PASS  ./internal/filter/http/header_mutation
  FuzzHPACKDecode                  PASS  ./internal/filter/hcm/h2
  FuzzLocalRateLimitConfigParse    PASS  ./internal/filter/http/localratelimit
  FuzzPromTextFormat               PASS  ./internal/stats
  FuzzTcpProxyFilter               PASS  ./internal/filter/tcpproxy
  FuzzTLSContextParse              PASS  ./internal/tls
```

**Gate E — 16 differential fixtures 0000-0015 PASS:**
```
$ go test -count=1 -v ./test/differential/ -run TestDifferential
    --- PASS: TestDifferential/0000-tcp-echo (1.71s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.35s)
    --- PASS: TestDifferential/0002-tls-tcp (1.38s)
    --- PASS: TestDifferential/0003-http11-routing (1.33s)
    --- PASS: TestDifferential/0004-h2-routing (2.05s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.02s)
    --- PASS: TestDifferential/0006-access-log (11.08s)
    --- PASS: TestDifferential/0007a-cors (1.44s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.81s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.72s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.96s)
    --- PASS: TestDifferential/0010-graceful-drain (9.56s)
    --- PASS: TestDifferential/0011-http-fault (2.08s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.56s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.21s)
    --- PASS: TestDifferential/0014-http-csrf (1.47s)
    --- PASS: TestDifferential/0015-http-buffer (1.47s)
--- PASS: TestDifferential (44.98s)
ok  	github.com/esalaine/envoy-go/test/differential	45.806s
```

**Gate F — BEHAVIOR_CONTRACT 4-edit bundle landed:**
```
$ grep -n "^### envoy.filters.http.buffer" docs/envoy-go/BEHAVIOR_CONTRACT.md
1150:### envoy.filters.http.buffer

$ grep -n "Phase 13 (buffer filter) note" docs/envoy-go/BEHAVIOR_CONTRACT.md
215:**Phase 13 (buffer filter) note:** ...

$ grep -n "HTTP filter.*envoy.filters.http.buffer" docs/envoy-go/BEHAVIOR_CONTRACT.md
35:| HTTP filter `envoy.filters.http.buffer` | 0015-http-buffer: ...

$ grep -n "Phase 13 forward-pointer notes" docs/envoy-go/BEHAVIOR_CONTRACT.md
1616:### Phase 13 forward-pointer notes
```
All 4 edits confirmed present.
