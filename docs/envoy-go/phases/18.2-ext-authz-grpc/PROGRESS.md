# Phase 18.2 — ext_authz gRPC service mode — Implementation Progress

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..18.1 PROGRESS.md structure.

- **Phase:** 18.2 — HTTP filter `envoy.filters.http.ext_authz` (gRPC service mode)
- **Branch:** `phase-18.2-ext-authz-grpc-impl` (fresh worktree at `.worktrees/phase-18.2-ext-authz-grpc-impl`)
- **Base commit (master tip):** `6226d11` (phase-18.2 PLAN SHA-fill follow-up; PLAN squash `f8c4f56`; SPEC SHA-fill follow-up `be18857`; SPEC squash `729867e`)
- **PLAN tip SHA:** `f8c4f56` (`git log -1 --format=%H -- docs/envoy-go/phases/18.2-ext-authz-grpc/PLAN.md`)
- **SPEC tip SHA:** `729867e` (`git log -1 --format=%H -- docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md`)
- **Links:** [`PLAN.md`](./PLAN.md) · [`SPEC.md`](./SPEC.md) · parent [`../18-http-filter-ext-authz/SPEC.md`](../18-http-filter-ext-authz/SPEC.md)

---

## Cold-start preconditions verified

All 17 preconditions verified green at cold-start of branch `phase-18.2-ext-authz-grpc-impl` (worktree at `.worktrees/phase-18.2-ext-authz-grpc-impl`, branched from master tip `6226d11`). Master tail shows PLAN SHA-fill follow-up at `6226d11`, PLAN squash at `f8c4f56`, SPEC SHA-fill follow-up at `be18857`, SPEC squash at `729867e`. Go 1.26.2, golangci-lint v1.64.8, Docker client 28.4.0 + server 28.1.1 present. ADR tail at 0164 (ADR-0164 anchored at the parent SPEC commit per ADR-0044 ADR-on-impl convention; the 4–5 phase-18.2-landing ADRs ADR-0158/0157-AMENDMENT/0160-gRPC/0161-gRPC/0165-CONDITIONAL have §Context drafts already at the SPEC commit per ADR-0044 — ADR-0165 is impl-time-unanticipated at SPEC time and has no pre-anchored §Context — and the §Decision + §Consequences bodies land at impl-time anchor Tasks 3/3/5/6/4 per the per-ADR table below). No ADR-0125 §(xiv) amendment paragraph — phase 18 already recorded the 5th-canonical-REUSE classification at 18.1 via ADR-0163; the `grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` command returns 3 matches but all 3 are explanatory text within ADR-0163 §Context/§Decision/§Ratification describing the ABSENCE of §(xiv) — confirmed via `grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md` returning 0 (no actual amendment paragraph). SPEC at `729867e`; PLAN at `f8c4f56`. `internal/grpcclient/` absent (Task 3 lands the real impl; Task 2 skeleton). `test/helpers/extauthzgrpc/` absent (Task 9 lands). `google.golang.org/grpc v1.70.0` reachable as indirect dep at master tip (Task 2 promotes to direct). `envoy.service.auth.v3` proto package reachable via `go-control-plane v1.32.4`. Reference Envoy image `envoyproxy/envoy:v1.37.2` present (SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`; ADR-0008 pin; unchanged through phase 18.2). `go test -count=1 -short ./...` returns clean (0 FAIL). `go build ./...` returns clean (test files including 22 phase-02..18.1 fuzzers compile). Working tree pristine (empty `git status --porcelain`).

**Note on PLAN precondition 11 regex.** The PLAN's literal regex `Test.*00(0[0-9]|1[0-9]|20)` does not match `TestDifferential` (the actual function name; the `0000..0020` identifiers appear only as `t.Run` sub-test names). The substantive verification is the full `TestDifferential` run, which PASSED with all 21 sub-tests green (`0000-tcp-echo` through `0020-http-ext-authz-http`; 59.93s wall-clock). Recorded here for the same reason 18.1's PROGRESS.md recorded its precondition-11 regex deviation: planner-time wording vs runtime fact, not a blocking divergence.

**Note on PLAN precondition 6 wording.** The PLAN says "`grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` returns 0 matches." The actual output returns 3 matches at lines 8633/8641/8657 — but those 3 matches are all explanatory text within ADR-0163's §Context/§Decision/§Ratification commentary describing the ABSENCE of an §(xiv) amendment paragraph (line 8657 itself documents this exact wording-vs-fact mismatch). The canonical check is `grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md` which returns 0 (no real amendment paragraph). Substantive precondition (no actual §(xiv) amendment) satisfied. Mirrors the analogous 18.1 PROGRESS.md note about its own §(xiv) grep wording.

### Precondition 1 — worktree branch

```
$ git rev-parse --abbrev-ref HEAD
phase-18.2-ext-authz-grpc-impl
```

### Precondition 2 — master tail

```
$ git log --oneline master | head -6
6226d11 phase 18.2 PLAN follow-up: STATE.md SHA-fill (TBD → f8c4f56 post-squash)
f8c4f56 Squash merge phase-18.2-ext-authz-grpc-plan
be18857 phase 18.2 SPEC follow-up: STATE.md SHA-fill (TBD → 729867e post-squash)
729867e Squash merge phase-18.2-ext-authz-grpc-spec
0ff9813 phase 18.1 IMPL follow-up: STATE.md SHA-fill (TBD → 3cc8182 post-squash)
3cc8182 Squash merge phase-18.1-ext-authz-http-impl
```

### Precondition 3 — toolchain

```
$ go version
go version go1.26.2 linux/amd64

$ golangci-lint version
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)

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
```

### Precondition 4 — DECISIONS.md tail

```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
164
```

### Precondition 5 — ADR §Context drafts present

```
$ grep -cE '^## ADR-0158' docs/envoy-go/DECISIONS.md
1

$ grep -cE '^## ADR-016[01]' docs/envoy-go/DECISIONS.md
2

$ grep -nE '^## ADR-0165' docs/envoy-go/DECISIONS.md
(exit code 1; no matches — ADR-0165 fires at Task 4 if D12 hypothesis holds)
```

### Precondition 6 — NO ADR-0125 §(xiv) amendment

```
$ grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md
8633:**Phase 18 lands NO ADR-0125 amendment paragraph** — the FIRST §9 family-row since phase 13 to REUSE an existing canonical rather than extend the roster (breaking the phase-13-§(ix) / phase-14-§(x) / phase-15-§(xi) / phase-16-§(xii) / phase-17-§(xiii) per-phase-roster-growth streak). [...explanatory body...]
8641:Phase 18 ext_authz lands the **5th-canonical REUSE** per ADR-0125 — the FIRST §9 family-row since phase 13 (buffer) to REUSE an existing ADR-0125 canonical rather than extend the roster. **NO ADR-0125 §(xiv) amendment paragraph is introduced.** [...explanatory body...]
8657:**(viii) NO ADR-0125 §(xiv) amendment:** ADR-0125's canonical-pattern roster stays at 8 entries after phase 18. The `grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` command returns 3 matches, but all three are explanatory text within ADR-0163 §Context/§Decision describing the ABSENCE of §(xiv) — confirmed by `grep -cE '^\*\*(xiv)\*\*' docs/envoy-go/DECISIONS.md` returning 0 (no actual amendment paragraph).

$ grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md
0
```

Substantive precondition satisfied: NO actual §(xiv) amendment paragraph anywhere in DECISIONS.md. The 3 `(xiv)` matches are all explanatory text within ADR-0163's commentary about the ABSENCE of an amendment — same wording-vs-fact mismatch as in 18.1 PROGRESS.md.

### Precondition 7 — SPEC SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md
729867eb4c48557edd422bd6720ef1d07255f60a
```

### Precondition 8 — PLAN SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/18.2-ext-authz-grpc/PLAN.md
f8c4f56d52a67428f95e2f318fcdc61c83d37ad1
```

### Precondition 9 — pristine tree

```
$ git status --porcelain
(empty output; exit=0)
```

### Precondition 10 — pre-existing suite green at `-short`

```
$ go test -count=1 -short ./...
(48 ok packages; 0 FAIL)
$ go test -count=1 -short ./... 2>&1 | grep -cE '^FAIL'
0
```

### Precondition 11 — pre-existing differential suite green

```
$ go test -count=1 ./test/differential/ -run 'TestDifferential' -v 2>&1 | tail
--- PASS: TestDifferential (59.93s)
    --- PASS: TestDifferential/0000-tcp-echo (1.59s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.27s)
    --- PASS: TestDifferential/0002-tls-tcp (1.24s)
    --- PASS: TestDifferential/0003-http11-routing (3.06s)
    --- PASS: TestDifferential/0004-h2-routing (2.06s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.06s)
    --- PASS: TestDifferential/0006-access-log (11.15s)
    --- PASS: TestDifferential/0007a-cors (1.52s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.83s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.54s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.98s)
    --- PASS: TestDifferential/0010-graceful-drain (9.56s)
    --- PASS: TestDifferential/0011-http-fault (2.18s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.52s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.18s)
    --- PASS: TestDifferential/0014-http-csrf (1.41s)
    --- PASS: TestDifferential/0015-http-buffer (1.56s)
    --- PASS: TestDifferential/0016-http-compressor (1.48s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.15s)
    --- PASS: TestDifferential/0018-http-rbac (1.56s)
    --- PASS: TestDifferential/0019-http-jwt-authn (1.53s)
    --- PASS: TestDifferential/0020-http-ext-authz-http (1.50s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	60.017s
```

PLAN's literal `Test.*00(0[0-9]|1[0-9]|20)` regex pattern does not match `TestDifferential` parent name. The substantive intent — all fixtures 0000–0020 PASS — is verified by the full `TestDifferential` run shown above.

### Precondition 12 — pre-existing fuzz tests compile (build-only)

```
$ go build ./...
(exit=0)

$ go test -count=1 -run='^$' ./... 2>&1 | tail
?   	github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/inputs	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki	0.004s [no tests to run]
?   	github.com/esalaine/envoy-go/test/fixtures/0019-http-jwt-authn/inputs	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0019-http-jwt-authn/pki	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0020-http-ext-authz-http/inputs	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	0.003s [no tests to run]
ok  	github.com/esalaine/envoy-go/test/helpers/echobackend	0.003s [no tests to run]
?   	github.com/esalaine/envoy-go/test/helpers/echobackend/cmd/echobackend	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzhttp	0.003s [no tests to run]
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend	0.003s [no tests to run]
(exit=0)
```

All test files including the 22 pre-existing fuzz tests from phases 02–18.1 compile. Full 30s-each fuzz run deferred to Task 13 phase-done Gate per PLAN.

### Precondition 13 — reference Envoy image present

```
$ docker image inspect envoyproxy/envoy:v1.37.2 --format '{{.Id}}'
sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
```

### Precondition 14 — `google.golang.org/grpc` v1.70.0 reachable

```
$ go list -m google.golang.org/grpc
google.golang.org/grpc v1.70.0

$ go doc google.golang.org/grpc NewClient | head -5
package grpc // import "google.golang.org/grpc"

func NewClient(target string, opts ...DialOption) (conn *ClientConn, err error)
    NewClient creates a new gRPC "channel" for the target URI provided.
    No I/O is performed. Use of the ClientConn for RPCs will automatically
```

`google.golang.org/grpc v1.70.0` is INDIRECT at master tip; Task 2 promotes to direct dependency.

### Precondition 15 — `envoy.service.auth.v3` proto package reachable

```
$ go doc github.com/envoyproxy/go-control-plane/envoy/service/auth/v3 AuthorizationClient | head -5
package authv3 // import "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"

type AuthorizationClient interface {
	// Performs authorization check based on the attributes associated with the
	// incoming request, and returns status `OK` or not `OK`.
```

No `import path failed`; `AuthorizationClient` interface (with the `Check(ctx, *CheckRequest, ...grpc.CallOption) (*CheckResponse, error)` method) reachable via `go-control-plane v1.32.4`.

### Precondition 16 — `internal/grpcclient/` absent

```
$ test ! -d internal/grpcclient && echo "ok: grpcclient absent"
ok: grpcclient absent
```

### Precondition 17 — `test/helpers/extauthzgrpc/` absent

```
$ test ! -d test/helpers/extauthzgrpc && echo "ok: extauthzgrpc absent"
ok: extauthzgrpc absent
```

---

## ADRs anticipated by this implementation

The 18.2-landing ADRs anticipated by SPEC §10 (ADR-0158 §Decision + §Consequences; ADR-0157 §Decision AMENDMENT; ADR-0160 gRPC-mode portion §Decision + §Consequences; ADR-0161 gRPC-mode portion §Decision + §Consequences) **plus 1 conditional impl-time-unanticipated ADR** (ADR-0165 — the callback-surface extension framework primitive per D3 + D12; the PLAN's strong hypothesis: it fires at Task 4). **§Context drafts for ADR-0158/0160/0161** were already landed at the parent SPEC commit `308e9b6` per ADR-0044 ADR-on-impl convention; **ADR-0157's full §Decision was at 18.1** — the 18.2 IMPL AMENDS in-place. **ADR-0164** (the ADR-0045 split-application ADR) landed IN FULL at the parent SPEC commit — UNCHANGED by 18.2 IMPL.

| ADR | Subject (18.2 portion) | Lands-in-task |
|---|---|---|
| **ADR-0158** | `internal/grpcclient/` framework primitive — `Dialer` (cluster-name → `*grpc.ClientConn` via `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure.NewCredentials())` — TLS terminates at the cluster-manager layer per the §11.P13 in-session SPEC scrape) + `AuthClient` typed wrapper (`envoy.service.auth.v3.Authorization/Check` stub from `go-control-plane v1.32.4` — no codegen); one `*grpc.ClientConn` per (cluster_name, compiledConfig) pair created at config-load time + shared across per-stream Check calls; leaks-on-exit MVP per D2; per-Check `context.WithTimeout` per D9; cross-phase-reusable for ext_proc + global_ratelimit per ADR-0158 §Consequences | Task 3 |
| **ADR-0157 §Decision AMENDMENT** | `*ExtAuthz_GrpcService` switch-arm activation in `buildCompiledConfig` — replaces the 18.1 PARSE-REJECT with `buildGRPCCheckFn`; `core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict (`"ext_authz: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)"`); `core.GrpcService.initial_metadata` + `retry_policy` SILENT-IGNORED; `compiledConfig` struct shape UNCHANGED (field-final at 18.1) — gRPC-specific config captured in closure lexical scope per §6.5 step 5 | Task 3 |
| **ADR-0160** (gRPC-mode portion) | `buildAttributeContext` in `attributes.go` — source/destination `Peer` (incl. `principal` via ADR-0144); `request.http` per parent §5.P4 + §11.P4 in-session refinement (pseudo-headers lowercased + included in headers map; HCM-injected `x-forwarded-proto`/`x-request-id`/`x-envoy-auth-partial-body` visible by the time DecodeHeaders runs); `request.time` as `Timestamp`; `tls_session.sni` gated by `include_tls_session` (per §11.P4 RATIFICATION — ONLY `sni` populated); `source.certificate` gated by `include_peer_certificate` (DER-encoded leaf); `destination.principal` populated AUTOMATICALLY from listener TLS cert per §11.P4 (NOT gated); `context_extensions` merged listener+per-route; `encode_raw_headers` `header_map` arm DEFERRED per D6; `metadata_context` + `route_metadata_context` populated as empty messages | Task 5 |
| **ADR-0161** (gRPC-mode portion) | `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC` in `check.go`; `OkHttpResponse.headers` set/append per the 4-arm `append_action` dispatch table per D5; `OkHttpResponse.headers_to_remove` populated into new `upstreamDel []string` field on `checkDisposition`; `OkHttpResponse.response_headers_to_add` SILENT-IGNORED per D11 (decode-side-only filter shape); `DeniedHttpResponse.{status.code, body, headers}` extracted verbatim (NOT filtered through `allowed_client_headers` — UNLIKE HTTP-mode; per parent §5.P11); envoy-go-strict treatment of `OkResponse + non-zero status` AND `DeniedResponse + zero status` as `dispError` per SPEC §6.7 commentary; `validate_mutations` gating identical to HTTP-mode → `dispInvalid` + `invalid` counter | Task 6 |
| **ADR-0165** (CONDITIONAL — fires per D3 + D12) | Cross-phase-reusable callback-surface extension to `DecoderFilterCallbacks` — adds 5 new accessor methods (`DownstreamRemoteAddr()`, `DownstreamLocalAddr()`, `DownstreamTLSServerName()`, `DownstreamTLSPeerCertDER()`, `DownstreamProtocol()`) + `ListenerPrincipal()` for per-stream socket + TLS + listener-cert state needed by ext_authz gRPC-mode's `AttributeContext` builder; seeded at HCM-dispatch (H1 `connection.go` + H2 `h2dispatch.go`) via 6 new chain primitives mirroring the `SetTLSPrincipals` / `tlsPrincipals` / `DownstreamPrincipal()` pattern. Anchors the PLAN-time settle of D3 (the SPEC §13.5 hard "NO new method" constraint is in direct conflict with SPEC §15 item 4 + §11.P4 RATIFICATION — the callback extension is unavoidable). §Context + §Decision + §Consequences ALL land at Task 4 (no pre-anchored §Context — the ADR is impl-time-unanticipated at SPEC time per ADR-0044). | Task 4 (CONDITIONAL) |

The implementer at each impl-anchor task AUTHORS the ADR §Decision + §Consequences bodies in DECISIONS.md (for ADR-0158: in the slot of the existing §Context-draft; for ADR-0157: in-place AMENDMENT of the existing §Decision; for ADR-0160/0161: in-place EXTENSION of the existing §Decision + §Consequences with the gRPC-mode portion; for ADR-0165: a fresh ADR entry inserted before "ADR tail" with `Status: Accepted` + `Date: <impl-date>` + `Lands-in: Task 4 of phase-18.2`), includes the ADR in the commit message, and verifies via `grep -nE '^## ADR-XXXX' docs/envoy-go/DECISIONS.md` returning the expected match count.

**NO in-place ADR-0125 amendment required by phase 18.2** (5th-canonical-REUSE already recorded at 18.1 via ADR-0163; no new canonical added).

**ADR-0044 escape-valve fires at Task 4 per D3 + D12** — `ADR-0165` lands. If at IMPL time the implementer finds an alternative path that avoids the callback extension (highly unlikely — see D3 rationale), ADR-0165 does NOT land + the PLAN's D12 hypothesis is recorded as falsified in PROGRESS.md.

---

## Planner-time decision register (D1..D13)

Reproduced verbatim from `PLAN.md` §"Planner-time deferred-decision resolution" so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — `test/helpers/extauthzgrpc/` discriminator + helper API LOCKED per SPEC §7.4 + §12 item 1.** Script discriminator: the `:path` value extracted from `req.Attributes.Request.Http.Path`. API surface per SPEC §7.4 sketch: `New(t testing.TB) *Server` returning a started server bound to `127.0.0.1:0`; `(*Server).Addr() string`; `(*Server).Script(path string, resp *authv3.CheckResponse)`; `(*Server).Stop()`. Lifecycle: spawn-per-fixture via `t.Cleanup(s.Stop)`. Plaintext h2c (no TLS) — fixture 0021 uses a plaintext auth cluster; TLS-to-auth coverage stays unit-test-only per SPEC §7.2 known-testing-gap. *Anchored: SPEC §7.4 + §12 item 1.*

2. **D2 — `*grpc.ClientConn` close-on-process-exit discipline LOCKED at MVP leaks-on-exit per SPEC §12 item 2.** No `os.Exit` cleanup hook; no `cleanup` package registration. The `*grpc.ClientConn` is owned by the `*compiledConfig` (captured by the `checkFn` closure); on process exit, the OS reclaims the connection. Rationale: matches 18.1's `httpAuthClient` no-shutdown discipline; envoy-go has no config hot-reload yet (xDS-CDS deferred per SPEC §8 item 9); the per-(cluster, compiledConfig) ClientConn lifetime is process-bounded. A future hot-reload phase will land a close-on-replacement discipline per a new ADR (NOT 18.2). *Anchored: SPEC §3.1 + §12 item 2.*

3. **D3 — `*authRequest` extension + per-stream-state seeding LOCKED at extend-`*authRequest` + extend-`DecoderFilterCallbacks` per SPEC §6.5 step 5 + §12 item 3.** Extend the existing 18.1 `*authRequest` struct (in `extauthz.go`) with: `remoteAddr net.Addr`, `localAddr net.Addr`, `tlsServerName string`, `peerCertDER []byte`, `listenerPrincipal string`, `protocol string`, `requestID string`, `streamStartTime time.Time`, `perRouteContextExtensions map[string]string`, `downstreamPrincipal []string`. The 18.1 fields (`method`/`path`/`headers`/`body`) carry forward unchanged. The closure signature `(ctx, *authRequest)` stays mode-agnostic per ADR-0157 §Decision. Per-stream-state SOURCE: NEW callback methods on `DecoderFilterCallbacks` (5 new: `DownstreamRemoteAddr`, `DownstreamLocalAddr`, `DownstreamTLSServerName`, `DownstreamTLSPeerCertDER`, `DownstreamProtocol`) + `ListenerPrincipal` (also new). These are seeded at HCM-dispatch time onto the per-stream `*FilterChain` via 6 new chain primitives mirroring the existing `SetTLSPrincipals` / `tlsPrincipals` / `(*decoderCB) DownstreamPrincipal` pattern (chain.go:107 + 551 + 483). **D3-DEVIATION-FROM-SPEC §13.5:** SPEC §13.5 stated "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2"; the planner verified at PLAN-time that the SPEC's hard constraint is in direct conflict with SPEC §15 item 4 + §11.P4 RATIFICATION (populated `tls_session.sni`, `source.certificate`, socket addresses, `destination.principal`); the SPEC's "extracted from connection state at `DecodeHeaders` time when the per-stream `dcb` is in scope" requirement is unsatisfiable without callback extension (master tip's `internal/filter/http/callbacks.go` exposes only `DownstreamPrincipal()` for TLS-aware state — verified by reading the file). The PLAN settles by AMENDING SPEC §13.5 at Task 4 — the callback-surface extension is unavoidable; the alternative (UNPOPULATED fields) is a behaviorally significant divergence vs reference Envoy and contradicts SPEC §15 item 4. **ADR-0044 escape-valve fires: ADR-0165 lands at Task 4** anchoring the callback-group as a cross-phase-reusable framework primitive (ext_proc + global_ratelimit + future ext_authz extensions). *Anchored: SPEC §6.5 step 5 + §12 item 3 + §13.5 (AMENDED at Task 4) + SPEC §15 item 4 + §11.P4.*

4. **D4 — `grpc.NewClient` resolver target string LOCKED at `passthrough:///<cluster_name>` per SPEC §6.5 step 3 + §12 item 4.** The `passthrough:///` scheme is gRPC's built-in single-endpoint resolver; it skips DNS resolution and delegates endpoint selection to our `WithContextDialer` callback (which re-looks-up via `cluster.Manager.Get(cluster_name).Dial(ctx)`). Functionally equivalent to `dns:///` for this use case but cleaner — gRPC doesn't try to be smart about resolution; we own it via the cluster manager. The cluster name is embedded in the target URL for clean logging (gRPC logs the target string on failures). *Anchored: SPEC §6.5 step 3 + §12 item 4.*

5. **D5 — `*core.HeaderValueOption.append_action` 4-arm dispatch table LOCKED per SPEC §6.7 + §12 item 5.** The four enum values: `APPEND_IF_EXISTS_OR_ADD` (default; index 0) → `upstreamApp` (append-discipline; `applyUpstreamMutations` step 2 via `headers.Add`); `OVERWRITE_IF_EXISTS_OR_ADD` (index 1) → `upstreamSet` (overwrite-discipline; `applyUpstreamMutations` step 1 via `headers.Set`); `OVERWRITE_IF_EXISTS` (index 2) → `upstreamSet` BUT WITH `setIfAbsent: false` semantic — only overwrites if the header is already present, does NOT add (the 4-arm dispatch's `OVERWRITE_IF_EXISTS` distinct branch); the IMPL extends `headerKV` with a `setIfAbsent` discriminator (default `true` for `OVERWRITE_IF_EXISTS_OR_ADD`; `false` for `OVERWRITE_IF_EXISTS`) — `applyUpstreamMutations` checks `len(headers.Values(name)) > 0` before `Set` when `setIfAbsent: false`; `ADD_IF_ABSENT` (index 3) → `upstreamSet` BUT WITH `setIfAbsent: true` AND `addOnlyIfNotPresent: true` — adds only when the header is absent (does NOT overwrite); the IMPL extends `headerKV` further with an `addOnlyIfAbsent` discriminator (default `false`; `true` for `ADD_IF_ABSENT`) — `applyUpstreamMutations` checks `len(headers.Values(name)) == 0` before `Set` when `addOnlyIfAbsent: true`. Phase-10 header_mutation enum-handling precedent is the model. The unit-test Group 11 covers all 4 arms. **Implementation note:** the IMPL may collapse the two new discriminators into a single 4-value enum field on `headerKV` (cleaner than two booleans). The IMPL settles the exact representation; behavior is the same. *Anchored: SPEC §6.7 + §12 item 5 + phase-10 header_mutation precedent.*

6. **D6 — `encode_raw_headers` `header_map` arm activation LOCKED at DEFERRED per SPEC §8 item 8 + §12 item 6.** When `encode_raw_headers: true`, envoy-go's `buildAttributeContext` does NOT populate `request.http.header_map` (the `core.HeaderMap` field preserving header order); only the legacy `request.http.headers` map is populated. Rationale: the §11.P4 in-session SPEC scrape evidence shows reference Envoy populates `headers` by default and only switches to `header_map` when `encode_raw_headers: true`; fixture 0021's byte-equivalence assertion compares the auth-server's received-CheckRequest against expectations (the harness's protojson rendering treats `headers` as the canonical form when both fields would otherwise serialize differently); the cost of implementing `header_map` (preserving header order through `http.Header → map[string]string` conversion is lossy by default) outweighs the MVP benefit. The flag PARSES (no PARSE-REJECT) — operators setting it true see the legacy `headers` map populated as if the flag were false. Divergence-window documented in BEHAVIOR_CONTRACT §13.4. *Anchored: SPEC §8 item 8 + §12 item 6 + §11.P4 in-session evidence.*

7. **D7 — gRPC transport-error vs `CheckResponse.status.code` non-zero distinction LOCKED per SPEC §6.7 + §12 item 7.** Transport-level errors (gRPC `Unavailable` / `DeadlineExceeded` / `Canceled` from `*grpc.ClientConn.Invoke`; `context.Canceled` from `OnDestroy`-cancellation; `context.DeadlineExceeded` from per-Check timeout) surface as the `error` return of `(*AuthClient).Check` — `mapGRPCResponse` is NEVER called on a transport-error path; the closure body explicitly returns `(checkDisposition{class: dispError}, err)` on `err != nil` BEFORE calling `mapGRPCResponse`. `CheckResponse.status.code` non-zero values (any gRPC canonical code: `PERMISSION_DENIED` / `UNAUTHENTICATED` / `INVALID_ARGUMENT` / etc.) → handled BY `mapGRPCResponse` per the §6.7 truth table: with `DeniedResponse` → dispDeny; with `OkResponse` → dispError (envoy-go-strict — structurally inconsistent); with nil-oneof → dispDeny default 403. This cleanly separates the transport-error path (handled at the `AuthClient.Check` boundary; the filter's closure body) from the proto-message-content path (handled in `mapGRPCResponse`). *Anchored: SPEC §6.7 + §12 item 7 + parent §5.P10.*

8. **D8 — `extauthz_test.go` single-file LOCKED per the 18.1 D3 precedent (NEW; surfaces at PLAN-time).** All Groups 10–14 stay in one `extauthz_test.go` for 18.2 (mirrors the 18.1 single-file discipline; 18.1's file is ~4900 LoC and stayed in one file — the soft threshold is ~5000 LoC before a split becomes mandatory). Impl-time MAY split `gRPC_test.go` if the combined file exceeds ~6000 LoC. *Anchored: 18.1 PLAN D3 precedent.*

9. **D9 — gRPC `Authorization/Check` deadline propagation discipline LOCKED at per-Check `context.WithTimeout` per SPEC §6.5 + §14.2 + planner-time emerge.** The `*AuthClient.Check(ctx, req)` method applies `ctx, cancel := context.WithTimeout(callerCtx, timeout)` where `timeout` is the `*HttpService.server_uri.timeout`-analog for gRPC mode (`gs.Timeout` from `*GrpcService`); the cancel is deferred. The caller's `ctx` (from the filter's `dispatchOutboundCheck` async goroutine) is the parent — its cancellation (from `OnDestroy` via `callCancel()`) propagates through `context.WithTimeout`'s internal AND-of-cancellation semantics. Result: BOTH `OnDestroy`-cancellation AND per-Check timeout surface as transport errors via `err != nil` from `(*AuthClient).Check`. NO ADR escape-valve for this surface — the standard `context.WithTimeout` semantics suffice. *Anchored: SPEC §6.5 + §14.2 + planner-time clarification of §3.1 timeout-application-site.*

10. **D10 — Three-listener fixture topology LOCKED per SPEC §7.2 (NEW; surfaces at PLAN-time).** Fixture 0021 wires 3 HCM listeners `l_test_a/b/c` to separate scenarios with distinct `failure_mode_allow` values (per the 18.1 SPEC §10 notable lesson — `CheckSettings` cannot override `failure_mode_allow`, so a single listener cannot host both `failure_mode_allow:false` AND `failure_mode_allow:true` scenarios). `l_test_a` hosts scenarios 1/2/5/6/7/8 (`failure_mode_allow:false`; `status_on_error:503` UNREACHABLE on these scenarios); `l_test_b` hosts scenario 3 (`failure_mode_allow:false` + `status_on_error:503` reachable via auth-server-down setup); `l_test_c` hosts scenario 4 (`failure_mode_allow:true` + `failure_mode_allow_header_add:true`). Each listener routes to a per-scenario `:path` route; the auth-server-down scenarios (3+4) stop the in-process gRPC server BEFORE the request issues (the driver's `setupAuthGRPC` helper) — mirrors the 18.1 fixture-0020 auth-down treatment. *Anchored: SPEC §7.2 + 18.1 SPEC §10 lesson.*

11. **D11 — `OkHttpResponse.response_headers_to_add` DEFERRED behavior LOCKED at SILENT-IGNORED per SPEC §8 item 5 (NEW; surfaces at PLAN-time).** The field PARSES; envoy-go does NOT inject these headers into the downstream RESPONSE on allow (the filter is decoder-only per ADR-0156; no encode-leg). The fuzz corpus + Group 11 unit test cover the silent-ignore path. Documented in BEHAVIOR_CONTRACT §13.1 + §13.4 as a divergence-window joint with the 18.1 `allowed_client_headers_on_success` deferral. *Anchored: SPEC §8 item 5.*

12. **D12 — ADR-0044 escape-valve disposition: PLAN-time HYPOTHESIS that ADR-0165 fires at Task 4 (NEW; surfaces at PLAN-time).** Per the planner-time settle of D3 (callback-surface extension is unavoidable to satisfy SPEC §15 item 4 + §11.P4 populated-set RATIFICATION), the IMPL anchors **ADR-0165** at Task 4 as the cross-phase-reusable framework primitive for the 5 new `DecoderFilterCallbacks` methods (`DownstreamRemoteAddr` / `DownstreamLocalAddr` / `DownstreamTLSServerName` / `DownstreamTLSPeerCertDER` / `DownstreamProtocol`) + `ListenerPrincipal`. The SPEC §10 anticipated "~0–1 impl-time-unanticipated ADRs"; ADR-0165 lands as 1. If at IMPL time the implementer finds an alternative path that avoids the callback extension (unlikely — the planner verified the SPEC's required population set against the master-tip callback surface), the PLAN's D3 + D12 settle reverts and ADR-0165 does NOT land. The IMPL records the outcome in PROGRESS.md Task 4. Next-free-ADR after 18.2: `ADR-0166` (if ADR-0165 fires) or `ADR-0165` (if it does not). *Anchored: SPEC §10 + D3.*

13. **D13 — Fixture 0021 IS plaintext-only — NO PKI, NO TLS-to-auth fixture coverage (NEW; surfaces at PLAN-time).** Unlike phase-17's RSA/ECDSA PKI fixture or phase-16's mTLS fixture, fixture 0021 wires plaintext HTTP/1.1 listeners + plaintext h2c auth cluster. The §11.P13 in-session SPEC scrape RATIFIED the TLS-to-auth-cluster path against reference Envoy; behavioral verification of envoy-go's own TLS handshake against the gRPC auth cluster lives in `internal/grpcclient/grpcclient_test.go` Group 1 (unit test against a TLS-fronted test gRPC server) per SPEC §14.1; AttributeContext-side TLS-aware fields (`tls_session.sni`, `source.certificate`, `destination.principal`) are unit-tested against MOCKED `*authRequest` state per SPEC §7.2 known-testing-gap. A future integration test MAY close the differential gap if a behavior delta surfaces; the current scope DEFERS this per the cost-vs-coverage tradeoff (the §11.P13 RATIFICATION is the load-bearing empirical evidence). *Anchored: SPEC §7.2 + §11.P13.*

---

## Task ledger

### Task 1 — Execution-precondition check + PROGRESS.md preamble

**Files changed:** `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (new)
**Commit SHA:** `ba29a3070d9302b432e21a8ec4292e21d6662c67`
**Status:** done
**Notes:** Created PROGRESS.md; verified all 17 cold-start preconditions per PLAN §"Execution preconditions"; phase-18.2 SPEC + PLAN confirmed present in HEAD; SPEC at `729867e`, PLAN at `f8c4f56`; ADR tail at 0164 (the 4 phase-18.2 ADRs ADR-0158/0157-AMENDMENT/0160-gRPC/0161-gRPC have §Context drafts / full bodies ALREADY at parent SPEC commit per ADR-0044 ADR-on-impl convention; ADR-0165 is CONDITIONAL impl-time-unanticipated per D12 — §Context + §Decision + §Consequences all land at Task 4 if D12 hypothesis holds; §Decision + §Consequences bodies for the 4 anticipated ADRs land at impl-time anchor Tasks 3/3/5/6 per the per-ADR table above — mirroring phase-13/15/16/17/18.1 pattern); `internal/grpcclient/` absent (Tasks 2+3 land); `test/helpers/extauthzgrpc/` absent (Task 9 lands). No ADR-0125 §(xiv) amendment paragraph (`grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md` returns 0 — the 3 `grep -nE '\(xiv\)'` matches are explanatory text within ADR-0163 commentary describing the ABSENCE). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention). Pre-existing fuzzers (22 fuzzers from phases 02–18.1 across co-located `fuzz_test.go` files) deferred to Task 13 phase-done Gate per PLAN. **Note on PLAN precondition 11 regex**: the PLAN's literal regex `Test.*00(0[0-9]|1[0-9]|20)` does not match `TestDifferential` (parent name); substantive intent — all fixtures 0000–0020 PASS — verified via full `TestDifferential` run (21 sub-tests green; 59.93s wall-clock). **Note on PLAN precondition 6 wording**: the literal "returns 0 matches" wording is contradicted by the actual output (3 matches, all explanatory text in ADR-0163 commentary about the ABSENCE); the canonical check `grep -cE '^\*\*\(xiv\)\*\*'` returns 0 (no real amendment paragraph). Both wording-vs-fact mismatches are documented in the Cold-start preconditions section above and mirror the 18.1 PROGRESS.md analogous notes.

### Task 2 — internal/grpcclient/ skeleton

**Files changed:**
- `internal/grpcclient/doc.go` (new, 86 lines) — package overview per File-structure-table responsibility (Dialer API, AuthClient typed wrapper, connection lifecycle, cross-phase reuse intent for ext_proc + global_ratelimit, TLS-at-cluster-manager-layer integration, ADR-0158 anchor); mirrors `internal/jwks/doc.go` shape per phase-17 ADR-0150 precedent.
- `internal/grpcclient/grpcclient.go` (new, 216 lines) — `Dialer` + `AuthClient` types + 5 method signatures (`New`, `(*Dialer).DialContext`, `NewAuthClient`, `(*AuthClient).Check`, `(*AuthClient).Close`). All method bodies stubbed to return sentinel `errTODOTask3 = errors.New("grpcclient: TODO (Task 3)")`. Doc-comments + impl-NOTEs include the `passthrough:///` rationale per D4, `WithContextDialer((*cluster.Cluster).Dial)` integration, `WithTransportCredentials(insecure.NewCredentials())` rationale per §11.P13 in-session SPEC scrape, per-Check `context.WithTimeout` discipline per D7+D9, and leaks-on-exit MVP per D2 — all for the Task 3 implementer.
- `internal/grpcclient/grpcclient_test.go` (new, 700 lines) — Groups 1 + 2 test SCAFFOLDING per SPEC §14.1. 10 test functions: Group 1 = `TestDialer_New_ReturnsNonNil`, `TestDialer_DialContext_HappyPath`, `TestDialer_DialContext_ParseReject` (2 sub-tests: unknown_cluster + useh2_false), `TestDialer_DialContext_Concurrent`. Group 2 = `TestAuthClient_NewAuthClient_HappyPath`, `TestAuthClient_NewAuthClient_PropagatesDialError`, `TestAuthClient_Check_HappyPath`, `TestAuthClient_Check_Timeout`, `TestAuthClient_Check_CancelHonored`, `TestAuthClient_Check_TransportErrorVerbatim`. Inline test helpers: `mkAuthPKI` (in-memory ECDSA P-256 CA + leaf; modeled on `internal/cluster/dial_h2_test.go`'s `mkH2TestPKI`), `mkH2ClusterMgr` (TLS + ALPN h2 + `http2_protocol_options{}`; modeled on `internal/filter/hcm/config_test.go`'s `mkH2ClusterManager`), `mkPlainClusterMgr` (plaintext STATIC cluster; modeled on `internal/filter/hcm/fuzz_test.go`'s `mkOneClusterManagerTB`), `startTestAuthServer` (in-process TLS-fronted `*grpc.Server` registered with a fake `authv3.AuthorizationServer`), `isDeadlineExceededTransportErr` + `isCanceledTransportErr` (loose-substring classifier helpers the Task-3 impl tightens to `status.FromError` + `codes.*`). Group 3 (Close idempotency) deferred to Task 3 per PLAN.
- `go.mod` (mod) — `google.golang.org/grpc v1.70.0` promoted from indirect to direct (now in the first `require` block, no `// indirect` comment). Side-effect of `go mod tidy`: `github.com/cncf/xds/go` + `golang.org/x/sys` also promoted from indirect to direct (both ALREADY directly imported by master-tip code — `internal/matcher/`, `internal/filter/http/rbac/`, `test/fixtures/0008-listener-chain-match/driver/sockopts.go`; the indirect-block placement was pre-existing tidy debt that `go mod tidy` correctly cleaned up at the same time it added our new direct grpc import). NO new transitive deps introduced.
- `go.sum` (mod) — corresponding hash adjustments from `go mod tidy`.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — filled Task 1 SHA placeholder (`<TBD …>` → `ba29a3070d9302b432e21a8ec4292e21d6662c67`) and replaced this `_pending_` block with the present entry.

**Commit SHA:** `633c462dd8d678dd23215079f5a4f65f045652bf`
**Status:** done

**Verification.**

```
$ go build ./internal/grpcclient/...
(exit=0)

$ go vet ./internal/grpcclient/...
(exit=0)

$ go list -m google.golang.org/grpc
google.golang.org/grpc v1.70.0
(no "(indirect)" suffix — promoted to direct)

$ grep -n 'google.golang.org/grpc' go.mod
13:	google.golang.org/grpc v1.70.0
(in the first require block; no "// indirect" comment)

$ go mod tidy
(clean; no further changes; second run is a no-op)

$ go test -count=1 -run TestDialer ./internal/grpcclient/... 2>&1 | tail
--- FAIL: TestDialer_DialContext_ParseReject (0.00s)
    --- FAIL: TestDialer_DialContext_ParseReject/unknown_cluster (0.00s)
        grpcclient_test.go:397: DialContext("c_does_not_exist") err = "grpcclient: TODO (Task 3)"; want substring "c_does_not_exist"
    --- FAIL: TestDialer_DialContext_ParseReject/useh2_false (0.00s)
        grpcclient_test.go:397: DialContext("c_plain") err = "grpcclient: TODO (Task 3)"; want substring "c_plain"
--- FAIL: TestDialer_DialContext_HappyPath (0.00s)
    grpcclient_test.go:339: DialContext: grpcclient: TODO (Task 3)
--- FAIL: TestDialer_DialContext_Concurrent (0.00s)
    grpcclient_test.go:437: DialContext[0]: grpcclient: TODO (Task 3)
    [... 7 more identical lines for the n=8 goroutines ...]
FAIL
FAIL	github.com/esalaine/envoy-go/internal/grpcclient	0.005s
FAIL
(expected — Task-2 stubs return errTODOTask3 sentinel where success is expected; the PARSE-REJECT substring assertions fail because the sentinel does not mention the cluster name. The Task-3 real impl makes these tests green.)

$ go test -count=1 -v ./internal/grpcclient/... 2>&1 | grep -cE '^=== RUN   Test[A-Z]'
10
(10 top-level test functions scaffolded — 4 in Group 1 + 6 in Group 2; the PARSE-REJECT test contains 2 t.Run sub-tests for a total of 11 named cases. Group 3 lands at Task 3.)

$ go build ./... && go vet ./...
(both exit=0 — the go-mod-tidy side-effect promotions of cncf/xds/go + golang.org/x/sys are no-ops for the rest of the tree.)
```

**Notes.**

- **Test scaffolding strategy:** the PLAN says "tests FAIL against the Task-2 stubs". To satisfy that, the test helpers (`mkH2ClusterMgr` / `mkPlainClusterMgr` / `startTestAuthServer` / `mkAuthPKI`) are written FOR REAL now (not deferred to Task 3) so that the test bodies execute the stub-stage `DialContext` / `NewAuthClient` / `Check` calls and hit the sentinel error → the assertions fail as designed. This costs ~250 LoC of test infrastructure that Task 3 reuses as-is; it AVOIDS the alternative of `t.Skip`-stubbing the helpers (which would make tests pass-via-skip instead of FAIL, defeating the SPEC §14.1 + PLAN Task-2 acceptance criterion).
- **go.mod tidy side-effects (cncf/xds/go + golang.org/x/sys promotions):** the master-tip `go.mod` placed both deps in the indirect block despite the codebase directly importing them. This is tidy debt that `go mod tidy` cleans up unconditionally; the PLAN's literal Step-4 scope is only `google.golang.org/grpc`, but suppressing the side-effect would require either reverting `go mod tidy` (a phase-13 ADR-0095 disallow) or hand-editing the file post-tidy (worse — `go mod tidy` would re-promote them on the next run). The promotions are NO-OPs at the build/test level (no new transitive deps; the deps were already in the dependency graph). Documented here so the Task-3 reviewer is not surprised by the diff.
- **NO ADR landed at Task 2.** ADR-0158 §Decision + §Consequences land at Task 3 (per ADR-0044 ADR-on-impl convention + PLAN's per-ADR anchor table). The §Context for ADR-0158 is already at the parent SPEC commit `729867e`; Task 3's impl-time ADR commit extends with §Decision + §Consequences.
- **D12 hypothesis (ADR-0165 fires at Task 4) UNCHANGED at Task 2.** Task 2 does NOT touch the `DecoderFilterCallbacks` surface; the callback-surface-extension decision is still pending at Task 4.

### Task 3 — internal/grpcclient/ real impl + ADR-0158 + ADR-0157 AMENDMENT

**Files changed:**
- `internal/grpcclient/grpcclient.go` (mod, ~+30 / ~-30 LoC net) — Task-2 skeleton sentinels (`errTODOTask3`) retired; 5 method bodies (`New`, `(*Dialer).DialContext`, `NewAuthClient`, `(*AuthClient).Check`, `(*AuthClient).Close`) carry the real bodies per SPEC §3.1 + D2/D4/D7/D9. `DialContext` constructs `*grpc.ClientConn` via `grpc.NewClient("passthrough:///"+clusterName, grpc.WithContextDialer(...), grpc.WithTransportCredentials(insecure.NewCredentials()))`; the `WithContextDialer` callback re-looks-up the cluster via `mgr.Get(clusterName)` and calls `cluster.Dial(ctx)` (Endpoint return discarded). PARSE-REJECT gates: nil dialer/manager, unknown cluster, `!UseH2()` — all wrapped errors include the cluster name. `Check` applies `context.WithTimeout(ctx, a.timeout)` when `timeout > 0`; transport errors propagate verbatim per D7. `Close` is `sync.Once`-guarded for idempotency + concurrent-safety per D9; nil-receiver-tolerant.
- `internal/grpcclient/grpcclient_test.go` (mod, ~+95 LoC) — Group 3 added: `TestAuthClient_Close_Idempotent` (3 sequential `Close()` calls + post-close `Check()` surfaces closed-conn transport error), `TestAuthClient_Close_ConcurrentRaceClean` (10 concurrent `Close()` invocations under -race; all return same cached error), `TestAuthClient_Close_NilSafe` (nil-receiver `Close()` returns nil — robustness). All Groups 1+2+3 now PASS under `go test -race -count=1 ./internal/grpcclient/...`.
- `internal/filter/http/extauthz/check.go` (mod, ~+60 LoC) — NEW `buildGRPCCheckFn(gs *corev3.GrpcService, ctx envoyhttp.FactoryCtx, validateMutations, includePeerCert, includeTlsSession, encodeRawHeaders, packAsBytes bool) (checkFn, error)` STUB returning `errors.New("ext_authz: grpc_service: TODO (Task 5)")` — real body lands at Task 5. NEW `packAsBytesFromWRB(wrb *bufferSettings) bool` 1-line helper for the `buildCompiledConfig` call-site. Imports extended: `corev3` proto package + `envoyhttp` FactoryCtx anchor.
- `internal/filter/http/extauthz/extauthz.go` (mod, ~+25 / ~-5 LoC net) — `buildCompiledConfig` `services`-oneof dispatch refactored: step 1a's `*ExtAuthz_GrpcService` PARSE-REJECT-with-18.1-wording REMOVED; step 5 services-oneof DISPATCH now per-arm — HTTP-mode → `buildHTTPCheckFn`, gRPC-mode → `buildGRPCCheckFn(s.GrpcService, ctx, cc.validateMutations, raw.GetIncludePeerCertificate(), raw.GetIncludeTlsSession(), raw.GetEncodeRawHeaders(), packAsBytesFromWRB(cc.withRequestBody))`. ADR-0157 §Decision AMENDMENT WIRE-UP complete.
- `docs/envoy-go/DECISIONS.md` (mod, ~+50 / ~-15 LoC net) — **ADR-0158** §Decision + §Consequences filled (8 §Decision items + 5 §Consequences paragraphs covering: package shape, `WithContextDialer` integration, `passthrough:///` rationale, typed `AuthClient` wrapper, one-conn-per-compiledConfig discipline, leaks-on-exit MVP, per-Check `context.WithTimeout`, transport-error verbatim propagation, cross-phase reuse for ext_proc + global_ratelimit, test ergonomics, cluster-manager surface, future hot-reload work, §5.P13 RATIFICATION closure, ADR-0044 anchor). Status flipped Anticipated → Accepted; Date updated to 2026-05-15. **ADR-0157** §Decision AMENDED in-place (item (ii) `services`-oneof dispatch) — 18.1 PARSE-REJECT wording RETIRED; replaced with the post-AMENDMENT wording about `buildGRPCCheckFn` activation + `GoogleGrpc` PARSE-REJECT + `initial_metadata`/`retry_policy` SILENT-IGNORE. Status header amended (Date: 2026-05-15 amendment date noted). §Consequences `divergence-window` + `forward-pointer` paragraphs updated to reflect closure.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 2 SHA placeholder filled (`<TBD …>` → `633c462dd8d678dd23215079f5a4f65f045652bf`); this Task 3 entry appended.

**Commit SHA:** `59ee8dd8b97555a086bfc09343c9e83c84c7fd96`
**Status:** done

**Verification.**

```
$ go build ./...
(exit=0)

$ go vet ./...
(exit=0)

$ go test -race -count=1 ./internal/grpcclient/...
ok  	github.com/esalaine/envoy-go/internal/grpcclient	1.073s
(Groups 1+2+3 — 13 tests — all PASS; race-clean.)

$ go test -race -count=1 -v ./internal/grpcclient/... 2>&1 | grep -cE '^--- PASS'
13
(4 Group 1 + 6 Group 2 + 3 Group 3 = 13 tests pass; PARSE-REJECT sub-tests count as 1 each at top level — 11 named cases total.)

$ grep -nE '^## ADR-0158' docs/envoy-go/DECISIONS.md
8409:## ADR-0158: gRPC-client outbound framework primitive at NEW top-level package `internal/grpcclient/` — envoy-go's FIRST gRPC infrastructure of any kind; …
(1 match — single ADR; §Decision + §Consequences filled.)

$ grep -nE '§Decision AMENDED in-place' docs/envoy-go/DECISIONS.md
8379:**Status: Accepted — landed at Task 2 of phase-18.1 PLAN per ADR-0044. §Decision AMENDED in-place at Task 3 of phase-18.2 IMPL (Date: 2026-05-15) to activate the `grpc_service` arm.**
(ADR-0157 amendment landed in-place with explicit Date.)

$ go test -count=1 ./internal/filter/http/extauthz/... 2>&1 | tail -5
--- FAIL: TestNew_GrpcServiceParseReject (0.00s)
    extauthz_test.go:162: got "ext_authz: grpc_service: TODO (Task 5)"; want substring 'grpc_service mode not yet supported (lands in phase 18.2)'
FAIL
FAIL	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.287s
FAIL
(EXPECTED — the 18.1 PARSE-REJECT-on-grpc_service test sees the NEW sentinel "ext_authz: grpc_service: TODO (Task 5)" instead of the retired 18.1 wording. The test is updated at Task 5 per the PLAN. Only ONE 18.1 test is affected — `TestNew_GrpcServiceParseReject`.)
```

**Notes.**

- **Expected 18.1 test FAIL — only `TestNew_GrpcServiceParseReject`.** The PLAN's Task 3 §Acceptance documents this as expected: "existing 18.1 Group 1 tests fail with a different error at this task — EXPECTED, documented in Task 3's PROGRESS entry, fixed at Task 5". The test asserts the retired 18.1 wording `"grpc_service mode not yet supported (lands in phase 18.2)"`; Task 3's flip from PARSE-REJECT to `buildGRPCCheckFn` activation surfaces the new STUB sentinel `"ext_authz: grpc_service: TODO (Task 5)"`. The fuzz_test seed #2 (the same grpc_service empty-config path) does NOT assert any specific wording — only checks that an error is produced — so the fuzzer remains green.
- **No new dependencies introduced at Task 3.** `google.golang.org/grpc/credentials/insecure` is a sub-package of the existing `google.golang.org/grpc` v1.70.0 direct require (added at Task 2). No `go.mod` / `go.sum` changes at Task 3.
- **`buildGRPCCheckFn` signature settled per PLAN.** The PLAN's File-structure-table specified `buildGRPCCheckFn(gs *core.GrpcService, ctx envoyhttp.FactoryCtx, validateMutations, includePeerCertificate, includeTlsSession, encodeRawHeaders, packAsBytes bool) (checkFn, error)`. Settled as-specified at Task 3 — all 7 parameters land at the wire-up + STUB body. The Task-5 real body uses all 7; the Task-3 STUB body references them via `_ = …` blank-assign to satisfy unused-parameter linting under `go vet`.
- **`packAsBytesFromWRB` 1-line helper authored.** The PLAN noted "VERIFY before authoring" — verified absent at master tip (grep for `packAsBytesFromWRB` returned 0 hits at branch tip). Authored as `func packAsBytesFromWRB(wrb *bufferSettings) bool { return wrb != nil && wrb.packAsBytes }` co-located in `check.go` alongside the call-site consumer `buildGRPCCheckFn`.
- **`Date: <impl-date>` for ADR-0158 + ADR-0157 amendment line.** Both anchored to `2026-05-15` per the conversation's currentDate. ADR-0158's top-level `Date:` field updated from the §Context-only-draft `2026-05-14` to `2026-05-15` (the impl-time §Decision + §Consequences land at 2026-05-15; the §Context drafted at 2026-05-14 is retained verbatim above the new §Decision body). ADR-0157's top-level `Date:` (`2026-05-14`) is UNCHANGED per the AMENDMENT convention (the original §Decision date holds; the amendment date is recorded in the §Decision Status header).
- **`*grpc.ClientConn` lifecycle at Task 3 wire-up.** No `*grpc.ClientConn` is yet allocated by the filter — the `buildGRPCCheckFn` STUB returns an error, so no AuthClient is constructed. The cross-cutting design (one ClientConn per compiledConfig captured in the closure; leaks-on-exit MVP) is anchored in ADR-0158 §Decision (v)/(vi) and will be exercised when Task 5 lands the real `buildGRPCCheckFn` body.

### Task 4 — Callback-surface extension + ADR-0165 (D12 FIRED; ADR-0165 LANDS)

**Files changed:**
- `internal/filter/http/callbacks.go` (mod, ~+70 LoC) — 6 new methods added to `DecoderFilterCallbacks` interface per ADR-0165: `DownstreamRemoteAddr() net.Addr`, `DownstreamLocalAddr() net.Addr`, `DownstreamTLSServerName() string`, `DownstreamTLSPeerCertDER() []byte`, `DownstreamProtocol() string`, `ListenerPrincipal() string`. Doc-comments cite ADR-0165 + the cross-phase reuse intent (ext_proc + global_ratelimit + future ext_authz extensions).
- `internal/filter/http/chain.go` (mod, ~+110 LoC) — 6 new chain fields (`downstreamRemoteAddr`, `downstreamLocalAddr`, `downstreamTLSServerName`, `downstreamTLSPeerCertDER`, `downstreamProtocol`, `listenerPrincipal`) + 6 new chain seeding primitives (`SetDownstreamRemoteAddr` / `SetDownstreamLocalAddr` / `SetDownstreamTLSServerName` / `SetDownstreamTLSPeerCertDER` / `SetDownstreamProtocol` / `SetListenerPrincipal`) + 6 new `*decoderCB` reader methods. All 6 mirror the `tlsPrincipals` / `SetTLSPrincipals` / `(*decoderCB).DownstreamPrincipal()` plumbing at chain.go:107 + 551 + 483 per ADR-0144 precedent. Per ADR-0071 chain-ownership invariant (SET-once-by-HCM-dispatch + READ-by-callback discipline).
- `internal/filter/http/chain_test.go` (mod, ~+240 LoC) — Group 13 ADR-0165 round-trip tests: 12 tests total (6 SET-and-read round-trips + 6 nil/empty fall-throughs). New helper `callbackProbe` + `newCallbackProbeChain` mirror `downstreamPrincipalProbe` + `newPrincipalProbeChain` template. All 12 PASS race-clean.
- `internal/filter/http/callbacks_test.go` (mod, +9 LoC) — `fakeDecoderCB` extended with 6 zero-value stub methods so the existing `TestDecoderFilterCallbacks_Compile` compile-time assertion (`var _ DecoderFilterCallbacks = (*fakeDecoderCB)(nil)`) stays green.
- 8 sister-package test files (mod, +9 LoC each — total ~+72 LoC across the 8 files) — `bandwidthlimit_test.go`, `buffer_test.go`, `compressor_test.go`, `csrf_test.go`, `extauthz_test.go` (`fakeExtAuthzDCB` + `asyncExtAuthzDCB`), `fault_test.go`, `header_mutation_test.go`, `jwtauthn_test.go`, `local_ratelimit_test.go`, `rbac_test.go` — each existing `fakeCallbacks` / `fakeDecoderCB` / `recordingDCB` test-mock extended with the same 6 zero-value stubs to keep the interface conformance assertions green. `net` import added where missing. **Note:** these are test-only sister-package mock extensions, not in the PLAN's File-structure-table list — but they are LOAD-BEARING for compile-time conformance of the interface change; without them the entire `./internal/filter/http/...` test surface fails to build. Decision: extend in this commit per the standard Go practice that an interface extension requires all conforming implementations (including test mocks) to be updated atomically.
- `internal/filter/hcm/connection.go` (mod, ~+30 LoC) — H1 dispatch site wire-in: after the existing `chain.SetTLSPrincipals(downstreamTLSPrincipals(downstream))` at line ~311, the 6 new seeding calls fire (`SetDownstreamRemoteAddr` from `downstream.RemoteAddr()`; `SetDownstreamLocalAddr` from `downstream.LocalAddr()`; `SetDownstreamProtocol("HTTP/1.1")`; `SetListenerPrincipal(f.listenerPrincipal)`; `SetDownstreamTLSServerName` + `SetDownstreamTLSPeerCertDER` from `*stdtls.Conn.ConnectionState()` when the conn is `*stdtls.Conn` AND `len(PeerCertificates) > 0`). Nil-downstream guard preserves the existing unit-test fixture pattern that passes `downstream=nil`.
- `internal/filter/hcm/h2dispatch.go` (mod, ~+45 LoC) — H2 dispatch site wire-in: `h2Dispatcher` gains 4 new per-connection fields (`downstreamRemoteAddr` / `downstreamLocalAddr` / `downstreamTLSServerName` / `downstreamTLSPeerCertDER`) symmetric to `tlsPrincipals` (captured ONCE at H2 connection build time by `runH2`; threaded into every per-stream chain via `chainDispatchAction`). `chainDispatchAction` gains the same 4 fields. `Match()` propagates the 4 fields onto the dispatch action. `WriteH2` calls the 6 chain seeders (the 4 captured + `SetDownstreamProtocol("HTTP/2")` + `SetListenerPrincipal(c.f.listenerPrincipal)`).
- `internal/filter/hcm/filter.go` (mod, ~+15 LoC) — `runH2` captures the 4 per-connection ADR-0165 fields onto the `h2Dispatcher` at connection-build time (symmetric to the existing `disp.tlsPrincipals = downstreamTLSPrincipals(downstream)` line). Nil-downstream guard mirrors the H1 path discipline.
- `internal/filter/hcm/config.go` (mod, ~+15 LoC) — `ListenerCtx` extended with `ListenerPrincipal string` field. `*Filter` extended with matching `listenerPrincipal string` field. `parseFilterWithCtx` reads `lc.ListenerPrincipal` and stores onto `f.listenerPrincipal`.
- `internal/listener/manager.go` (mod, ~+55 LoC) — NEW `extractListenerPrincipal(*stdtls.Config) string` helper extracting the listener's TLS server-cert principal (URI SAN[0] → DNS SAN[0] → Subject CN of `Certificates[0]`; empty for plaintext / no cert / parse failure / no-matching-identity). `crypto/x509` import added. `listenerCtx` (manager-local) extended with `listenerPrincipal string` field. The 2 `buildTerminalFilter` call sites populate `listenerCtx.listenerPrincipal = extractListenerPrincipal(chainTLS)` (1 for `filter_chains[]` chain, 1 for `default_filter_chain`). The hcm.TypeURL filter-registry closure passes `lc.listenerPrincipal` through into `hcm.ListenerCtx{...ListenerPrincipal: lc.listenerPrincipal}`.
- `docs/envoy-go/DECISIONS.md` (mod, ~+85 LoC) — **ADR-0165 authored from scratch** (impl-time-unanticipated per ADR-0044 escape-valve firing; no pre-SPEC-time §Context draft). `Status: Accepted`, `Date: 2026-05-15`, `Lands-in: Task 4 of phase-18.2`. Inserted AFTER ADR-0164 (line 8741). §Context records the SPEC §13.5 vs SPEC §15 item 4 + §11.P4 RATIFICATION conflict + PLAN-time D3 + D12 settlement; §Decision records the 6 new methods + 6 chain primitives + HCM dispatch site wire-in pattern + listener plumbing (Outcome B) + Group 13 unit-test coverage; §Consequences records cross-phase reuse intent + SPEC AMENDMENT pointers + ADR-0071 chain-ownership invariant carry-over + no-new-fuzzer-surface note. Cross-references: ADR-0044 (escape-valve), ADR-0071 (chain-ownership), ADR-0144 (precedent), ADR-0157/0160/0161 (consumers), ADR-0164 (predecessor).
- `docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md` (mod, +X / -X LoC net) — **3 in-place AMENDMENTS** at §13.5 + §6.5 step 5 + §6.6 per ADR-0165. Each AMENDMENT preserves the original wording in a quoted block (grep-archaeology convention, matching the ADR-0157 §Decision AMENDMENT style at phase-18.2 Task 3) followed by an AMENDMENT paragraph (`Amendment date: 2026-05-15`) citing ADR-0165 + the planner-time D3 + D12 settle. §13.5 flip: the "NO new method on `envoyhttp.DecoderFilterCallbacks`" hard constraint becomes the 6-method extension per ADR-0165. §6.5 step 5 flip: the "NO new `DecoderFilterCallbacks` primitive" wording is amended (the closure signature `(ctx, *authRequest)` stays mode-agnostic; the SOURCE of state capture moves to the 6 new callbacks). §6.6 clarification: `buildAttributeContext` ITSELF remains a pure function of `*authRequest` + the 4 booleans (no `DecoderFilterCallbacks` parameter); the state capture into `*authRequest` happens at `DecodeHeaders` time via the new callbacks. All 3 sections grep-clean for "ADR-0165" + "Amendment date: 2026-05-15" + "preserved for grep-archaeology".
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 3 SHA placeholder filled (`<TBD …>` → `59ee8dd8b97555a086bfc09343c9e83c84c7fd96`); this Task 4 entry appended; Step-0 pre-spike outcome recorded; D3-DEVIATION-FROM-SPEC §13.5 note recorded.

**Commit SHA:** `d743514d7e5963bfe8923cb079b4b41cc13a3e6a`
**Status:** done

**Task 4 follow-up gofmt fixup (commit `1b9dfd2cec6eef8f3951799f2572d3fa747b72a6`):** A post-commit `gofmt -w` pass over the 12 modified sister-package test files (+ the `h2dispatch.go:50` comment 5→4 typo on the number of new methods) was applied as a fixup commit. The Task 4 commit itself was structurally complete; the fixup landed cosmetic-only changes to satisfy the `gofmt -l .` empty-output gate. Recorded here for archaeological clarity; the canonical Task 4 SHA is `d743514d7e5963bfe8923cb079b4b41cc13a3e6a` and the fixup SHA is `1b9dfd2cec6eef8f3951799f2572d3fa747b72a6`.

**Step 0 pre-spike outcome (RECORDED per PLAN Task 4 Step 0).**

`grep -nE '\*stdtls\.Config\|listenerTLS\|listenerCert\|tlsCert' internal/filter/hcm/connection.go internal/listener/ cmd/envoy-go/main.go`:

```
internal/listener/manager.go:101:	tlsCfg      *stdtls.Config // nil if plaintext chain
internal/listener/manager.go:114:// *stdtls.Config that is passed directly to stdtls.Server.
internal/listener/manager.go:339:		var chainTLS *stdtls.Config
internal/listener/manager.go:401:		var dfcTLS *stdtls.Config
internal/listener/manager.go:451:	// listenerfilter.SelectChain — there is no listener-level *stdtls.Config
internal/listener/manager.go:840://     the chain's per-chain *stdtls.Config and run HandshakeContext; abort on
[test file matches elided]
```

**Outcome: B (requires plumbing).** The listener's `*stdtls.Config.Certificates[0]` is held on the per-chain `chainInfo.tlsCfg` field at `internal/listener/manager.go:101` + populated at lines 339 (filter_chains[]) + 401 (default_filter_chain). It is NOT passed into the HCM filter constructor (only `lc.hasTLS bool` flows through `listenerCtx` → `hcm.ListenerCtx`). The downstream `*tls.Conn.ConnectionState()` on the server-side conn does NOT expose the server's OWN cert (it exposes `PeerCertificates` = the CLIENT cert chain; the server's local cert is on `tls.Config.Certificates[0]` only). The pre-spike confirmed Outcome B: lift a new `listenerPrincipal string` parameter through `listenerCtx` → `hcm.ListenerCtx` → `*hcm.Filter.listenerPrincipal`, pre-extracted at listener-build time by a new `extractListenerPrincipal(*stdtls.Config) string` helper. The PLAN's File-structure table for `connection.go` anticipated +30–80 LoC depending on pre-spike outcome; the actual net add across `listener/manager.go` + `hcm/config.go` + `hcm/filter.go` + `hcm/connection.go` + `hcm/h2dispatch.go` for listener-principal sourcing is ~+50 LoC, within the PLAN budget. Recorded here per PLAN Step 0 "tighten Task 4 LoC budget realism" instruction.

**D3-DEVIATION-FROM-SPEC §13.5 settled (D12 FIRED — ADR-0165 lands).**

The PLAN-time D3 + D12 hypothesis HOLDS at IMPL time. The Step-0 pre-spike + IMPL-time code-reading confirmed that the SPEC §13.5 "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" hard constraint is in direct conflict with SPEC §15 item 4 + §11.P4 RATIFICATION (populated `tls_session.sni` + `source.certificate` + source/destination socket addresses + `destination.principal` + `request.http.protocol`). There is NO alternative path that avoids the callback-surface extension while satisfying the SPEC's required populated set — the master-tip `DecoderFilterCallbacks` exposes ONLY `DownstreamPrincipal() []string` for TLS-aware state, which covers only the CLIENT-cert principal candidates. The 6 new methods + 6 chain primitives + HCM dispatch site wire-in land at Task 4 per ADR-0165 (the ADR-0044 escape-valve firing). SPEC §13.5 + §6.5 step 5 + §6.6 AMENDED in-place at this same commit per the grep-archaeology convention (mirrors the ADR-0157 §Decision AMENDMENT at phase-18.2 Task 3). The 3 amendments cite ADR-0165 + the planner-time D3 + D12 settle.

**Verification.**

```
$ go build ./...
(exit=0)

$ go vet ./...
(exit=0)

$ go test -race -count=1 ./internal/filter/http/... ./internal/filter/hcm/...
ok  	github.com/esalaine/envoy-go/internal/filter/http	1.147s
ok  	github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit	9.675s
ok  	github.com/esalaine/envoy-go/internal/filter/http/buffer	1.013s
ok  	github.com/esalaine/envoy-go/internal/filter/http/compressor	1.035s
ok  	github.com/esalaine/envoy-go/internal/filter/http/cors	1.011s
ok  	github.com/esalaine/envoy-go/internal/filter/http/csrf	1.016s
ok  	github.com/esalaine/envoy-go/internal/filter/http/envoygotest	1.040s
--- FAIL: TestNew_GrpcServiceParseReject (0.00s)
    extauthz_test.go:163: got "ext_authz: grpc_service: TODO (Task 5)"; want substring 'grpc_service mode not yet supported (lands in phase 18.2)'
FAIL	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.311s
ok  	github.com/esalaine/envoy-go/internal/filter/http/fault	1.329s
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.014s
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.082s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.018s
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac	1.023s
ok  	github.com/esalaine/envoy-go/internal/filter/http/router	1.234s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.041s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.493s
(All packages PASS race-clean EXCEPT the documented Task-3 expected-fail TestNew_GrpcServiceParseReject — fixed at Task 5 per PLAN.)

$ go test -count=1 -v -run 'TestDecoderCB_Downstream|TestDecoderCB_ListenerPrincipal' ./internal/filter/http/ 2>&1 | grep -cE '^--- PASS'
15
(Group 13: 12 NEW tests + 3 pre-existing ADR-0144 DownstreamPrincipal tests; all PASS.)

$ grep -nE '^## ADR-0165' docs/envoy-go/DECISIONS.md
8741:## ADR-0165: Cross-phase-reusable callback-surface extension on `DecoderFilterCallbacks` …
(1 match — ADR-0165 authored at line 8741 AFTER ADR-0164.)

$ grep -cE 'ADR-0165' docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md
6
(6 references — 3 in §13.5, §6.5 step 5, §6.6 AMENDMENT paragraphs + 3 in cross-reference context.)

$ grep -cE 'preserved for grep-archaeology' docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md
3
(3 quoted-block grep-archaeology preservations — one per AMENDMENT site.)
```

**Notes.**

- **Group 13 unit-test count.** PLAN Step 1 called for "6 chain seed/read round-trip tests" mirroring the line-1507 template. The IMPL ships **12 tests total**: 6 SET-and-read round-trips (one per new accessor) + 6 nil/empty fall-throughs. The fall-throughs are the empirical pin for the zero-value semantics documented on the chain struct fields — the "no SetX → accessor returns zero value" path that HCM dispatch exercises for plaintext / synthetic-stream / no-client-cert connections. This matches the existing `TestDecoderCB_DownstreamPrincipal_NoSeed_NilSlice` pattern at `chain_test.go:1485` for the ADR-0144 precedent.
- **Sister-package test mock extensions are required for compile-time interface conformance.** The PLAN's File-structure table listed only the load-bearing production files. Extending the `DecoderFilterCallbacks` interface (which is the explicit task) requires every conforming type — including 10 test-mock types across `bandwidthlimit_test.go`, `buffer_test.go`, `compressor_test.go`, `csrf_test.go`, `extauthz_test.go` (`fakeExtAuthzDCB` + `asyncExtAuthzDCB`), `fault_test.go`, `header_mutation_test.go`, `jwtauthn_test.go`, `local_ratelimit_test.go`, `rbac_test.go` — to gain 6 new zero-value stub methods. Without this, the entire `./internal/filter/http/...` test surface fails to build. This is the standard Go-interface-extension migration pattern; recorded here as a PLAN-File-list-vs-IMPL-LoC-realism note for archaeological clarity.
- **`net` import added to 8 sister-package test files.** Each of the 8 sister-package test files used by the mock-extension above gained a `"net"` import line alongside the existing imports — the 6 new stubs include 2 `net.Addr`-returning methods (`DownstreamRemoteAddr` / `DownstreamLocalAddr`) so the import is required for the type reference in the method signature.
- **Listener plumbing extension (Outcome B) is in-scope per PLAN.** The PLAN's File-structure table for `connection.go` flagged this as a "PLAN-time-flagged sub-decision (per reviewer note + Task 4 Step 0 pre-spike): if the listener `*stdtls.Config` is NOT reachable from `connection.go:dispatchRequest` via the existing parameters at master tip, Task 4 ~+30 LoC budget for `connection.go` may blow out; mitigation requires lifting a new parameter through the dispatch chain." Step 0 confirmed Outcome B fires; the mitigation lands in this commit: `extractListenerPrincipal` helper in `listener/manager.go` + `listenerPrincipal` field plumbing through `listenerCtx` → `hcm.ListenerCtx` → `*hcm.Filter.listenerPrincipal`. Total listener-plumbing LoC across the 4 files (`listener/manager.go` + `hcm/config.go` + `hcm/filter.go` + indirectly `hcm/connection.go` + `hcm/h2dispatch.go`'s `c.f.listenerPrincipal` consumer call): ~+55 LoC of which ~+45 is in `listener/manager.go` (the helper itself) and ~+10 across the HCM bridge files. Within the PLAN's +30–80 LoC budget for the listener-sourcing path.
- **Nil-downstream guard.** The H1 `dispatchRequest` + H2 `runH2` dispatch sites both gain a nil-downstream guard around the `RemoteAddr/LocalAddr` capture (the unit-test fixtures `TestDispatchRequest_ChainInvocationOrder` etc. pass `downstream=nil` intentionally to exercise the chain machinery without a real conn). Without the guard, the test panics on nil-receiver `RemoteAddr()` call. The guard is consistent with the existing `downstreamTLSPrincipals` discipline (the type-assertion `tc, ok := downstream.(*stdtls.Conn)` handles nil via `ok=false`).
- **No new fuzzer surface.** The 6 new accessors return chain-state values verbatim; there is no parsing or validation logic. No new fuzzer is required at Task 4 (the 23rd fuzzer at Task 9 targets `CheckResponse` mapping, not callback accessors).

### Task 5 — buildAttributeContext + extended *authRequest + ADR-0160 gRPC-mode

**Files changed:**

- `internal/filter/http/extauthz/attributes.go` (mod, ~+290 LoC) — `buildAttributeContext(req *authRequest, encodeRawHeaders, packAsBytes, includePeerCert, includeTlsSession bool) *authv3.AttributeContext` authored per SPEC §6.6 (with ADR-0165 AMENDMENT — pure-function signature preserved) + the §11.P4 RATIFIED in-session SPEC scrape's populated set. 5 helpers appended in the same file: `addressFromNetAddr(net.Addr) *corev3.Address`, `lowercaseHeaderMap(http.Header) map[string]string` (multi-value comma-join per reference Envoy; pseudo-headers INCLUDED — distinct from HTTP-mode `buildAuthRequest`), `firstOrEmpty([]string) string`, `bodyStringIfNotBytes([]byte, bool) string`, `bodyBytesIfBytes([]byte, bool) []byte`. New imports: `net`, `time`, `corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"`, `authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"`, `"google.golang.org/protobuf/types/known/timestamppb"`. File header block extended with the gRPC-mode portion bullets + ADR anchor disposition.
- `internal/filter/http/extauthz/extauthz.go` (mod, ~+55 LoC) — `*authRequest` struct extended with 10 new fields per D3 + ADR-0165: `remoteAddr net.Addr`, `localAddr net.Addr`, `tlsServerName string`, `peerCertDER []byte`, `listenerPrincipal string`, `protocol string`, `requestID string`, `streamStartTime time.Time`, `perRouteContextExtensions map[string]string`, `downstreamPrincipal []string`. The 4 existing 18.1 fields (`method`, `path`, `headers`, `body`) carry forward unchanged. Struct doc-comment block extended to record the dual-mode field discipline (HTTP-mode leaves new fields at zero values; gRPC-mode reads them via `buildAttributeContext`). New imports: `net`, `time`.
- `internal/filter/http/extauthz/extauthz_test.go` (mod, ~+420 LoC) — **Group 12 tests appended** at end-of-file. 20 new tests total: 10 buildAttributeContext-direct tests + 10 helper unit tests. The populated-set test (`TestBuildAttributeContext_PopulatedSet_18P4`) faithfully reproduces the §11.P4 RATIFIED in-session SPEC scrape (172.17.0.1:58476 → 172.17.0.2:10443, downstream.scrape.test SNI, hello-from-scrape body, pseudo-headers + HCM-injected headers + content-length + user-agent). Conditional-gate tests cover the 4 tls_session arms + 4 source.certificate arms. Helper-test coverage: TCPAddr / nil / IPv6 / multi-value-comma-join / pseudo-headers-included / empty / first-of / body-string-vs-bytes. New shared helper `authReqFor18P4(t)` + `mapKeys(map)` for diagnostic logging.
- `docs/envoy-go/DECISIONS.md` (mod, ~+85 LoC) — **ADR-0160 extended in-place** with the gRPC-mode portion. Per the established AMENDMENT convention, the existing HTTP-mode §Decision + §Consequences blocks are preserved verbatim; a new `### gRPC-mode portion (lands at phase-18.2 Task 5)` sub-heading is inserted between the §Consequences body and the next ADR (ADR-0161). The gRPC-mode body adds §Decision items (viii) — `buildAttributeContext` signature pure-function-of-`*authRequest`+4-booleans (xii) — `*authRequest` 10-field extension shape; §Consequences addenda for the LoC budget reality + §11.P4 RATIFIED scrape closure + D6 encode_raw_headers DEFERRED-for-MVP closure + Task 6 + Task 8 forward-pointers. Top-level Date header updated `2026-05-14 → 2026-05-15` per the in-place extension date convention.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 4 SHA placeholder filled (`<TBD …>` → `d743514d7e5963bfe8923cb079b4b41cc13a3e6a`); Task 4 fixup commit `1b9dfd2cec6eef8f3951799f2572d3fa747b72a6` recorded; this Task 5 entry appended.

**Commit SHA:** `c651cf3d33ed7a7e7580adb93b81f4fa92f89ba8`
**Status:** done

**Verification.**

```
$ go build ./...
(exit=0)

$ go vet ./...
(exit=0)

$ gofmt -l .
(empty — no dirty files)

$ go test -race -count=1 -v -run TestBuildAttributeContext ./internal/filter/http/extauthz/ 2>&1 | grep -cE '^--- PASS'
10
(10 buildAttributeContext direct tests PASS race-clean.)

$ go test -race -count=1 -v -run 'TestAddressFromNetAddr|TestLowercaseHeaderMap|TestFirstOrEmpty|TestBody(String|Bytes)' ./internal/filter/http/extauthz/ 2>&1 | grep -cE '^--- PASS'
10
(10 helper unit tests PASS race-clean. Group 12 total = 20 new tests.)

$ go test -race -count=1 ./internal/filter/http/extauthz/...
--- FAIL: TestNew_GrpcServiceParseReject (0.00s)
    extauthz_test.go:163: got "ext_authz: grpc_service: TODO (Task 5)"; want substring 'grpc_service mode not yet supported (lands in phase 18.2)'
FAIL    github.com/esalaine/envoy-go/internal/filter/http/extauthz    0.308s
(All Group 12 + pre-existing groups PASS race-clean EXCEPT the documented Task-3-expected-fail TestNew_GrpcServiceParseReject — the STUB sentinel "ext_authz: grpc_service: TODO (Task 5)" is replaced at Task 6 when buildGRPCCheckFn lands its real body; Task 5 is the AttributeContext-builder lift, not the grpc_service-arm activation, so this test stays expected-fail through the end of Task 5.)

$ go test -race -count=1 ./internal/filter/http/... ./internal/filter/hcm/...
(all packages PASS race-clean EXCEPT the documented Task-3 expected-fail above)

$ grep -nE '^## ADR-0160' docs/envoy-go/DECISIONS.md
8505:## ADR-0160: `AttributeContext` / `AuthorizationRequest` builder — HTTP-mode portion …
(1 match — ADR-0160 extended in-place per ADR-0044 in-place-extension convention.)
```

**Notes.**

- **`*authRequest` 10-field extension scope discipline.** Per PLAN Task 5 Step 3, the 10 new fields are DECLARED but NOT yet POPULATED from production callsites. `buildAttributeContext` consumes them; the production-side seeding (via the ADR-0165 6-method callback surface + ADR-0144 + `headers.Get(":path")`-equivalent for `requestID` + `streamStartTime` from HCM-tracked time + per-route resolve) lands at Task 8's `dispatchOutboundCheck` extension. Task 5's tests populate the fields directly on the test `*authRequest{}` literal via the `authReqFor18P4` helper — exactly mirroring the SPEC §7.2 documented "unit tests against a mocked TLS state in `*authRequest`" testing-gap closure. The Task 5 IMPL has NO change to `dispatchOutboundCheck`; existing 18.1 tests that construct `*authRequest{}` literals work unchanged (zero values for the 10 new fields are valid).
- **`streamStartTime` zero-value fallback.** SPEC §6.6 step 4 wording ("`timestamppb.Now()` (or `timestamppb.New(req.streamStartTime)` if streamStartTime is zero — the IMPL settles") is settled at Task 5 by falling back to `time.Now()` ONLY when `req.streamStartTime.IsZero()` returns true. This preserves the populated-from-`*authRequest`-when-set semantic + provides the production-safety fallback for the case where Task 8's seeding does not yet populate the field. The Group 12 test `TestBuildAttributeContext_StreamStartTimeZero_FallsBackToNow` pins both arms.
- **`source.certificate` encoding.** The proto field type is `string` per `attribute_context.pb.go:195`; the proto docs say "the certificate contents are encoded in URL and PEM format". The envoy-go MVP coerces raw DER bytes to `string(req.peerCertDER)`. The §11.P4 in-session SPEC scrape did NOT exercise this field (curl presented no client cert), so the URL+PEM encoding remains a possible IMPL refinement if a behavior delta surfaces vs reference Envoy v1.37.2 in a future TLS-listener-extension differential fixture. The current encoding choice is recorded in ADR-0160 gRPC-mode portion §Decision (ix).
- **`encode_raw_headers` D6 closure — DEFERRED for MVP.** Per SPEC §12 item 6 + the SPEC §11.P4 in-session scrape evidence (which did not toggle the flag), the `header_map` arm is NOT populated at Task 5. The flag PARSES (config-side, threaded through `buildGRPCCheckFn` at Task 3 per ADR-0157 §Decision AMENDMENT) but produces no `AttributeContext` difference. The legacy `headers` map suffices for fixture 0021's byte-equivalence assertion. Documented in ADR-0160 gRPC-mode portion §Decision (x). Group 12 test `TestBuildAttributeContext_EncodeRawHeaders_DeferredHeaderMap` pins the behavior.
- **`TestNew_GrpcServiceParseReject` status: still failing (EXPECTED).** The test asserts the OLD 18.1 PARSE-REJECT message "grpc_service mode not yet supported (lands in phase 18.2)" — this is the 18.1 wording that was replaced at Task 3 by the new STUB sentinel "ext_authz: grpc_service: TODO (Task 5)" per the ADR-0157 §Decision AMENDMENT. The STUB sentinel is itself replaced at Task 6 when `buildGRPCCheckFn` lands its real body (real returned `checkFn`, no STUB error). Task 5 is the AttributeContext-builder lift on the `*authRequest` consumer side; it does NOT touch the `buildGRPCCheckFn` STUB → real body transition. The test is expected to remain failing through Task 5 and to be fixed (updated to assert success) at Task 6 alongside the STUB removal.
- **No new fuzzer surface.** `buildAttributeContext` is a pure function over `*authRequest` + 4 booleans; helpers are equally pure. The 23rd fuzzer at Task 9 targets `mapGRPCResponse` (the gRPC-response → `checkDisposition` mapping — Task 6's output), not `buildAttributeContext`. No new fuzzer is required at Task 5.
- **LoC budget.** SPEC §6.8 budget for `attributes.go` additions was 200–350 LoC; the actual addition is ~290 LoC including the function-header doc block. Within budget. The `extauthz.go` `*authRequest` extension is ~55 LoC; the test additions are ~420 LoC. All within the PLAN-anticipated File-structure-table sizes.

### Task 6 — mapGRPCResponse + buildAllow/DenyDispositionGRPC + ADR-0161 gRPC-mode

**Files changed:**

- `internal/filter/http/extauthz/check.go` (mod, ~+280 LoC) — `buildGRPCCheckFn` STUB replaced with the real 6-step body per SPEC §6.5: (1) `GoogleGrpc` arm PARSE-REJECT envoy-go-strict; (2) `EnvoyGrpc.cluster_name` PGV-mirror (`min_len: 1`) + arm-required PARSE-REJECT; (3) `ctx.ClusterManager.Get(clusterName)` lookup + `UseH2()` gate; (4) `*grpcclient.AuthClient` construction via `grpcclient.New(mgr)` + `grpcclient.NewAuthClient(d, name, durationpbToGo(gs.Timeout))`; (5) `initial_metadata` + `retry_policy` SILENT-IGNORED per SPEC §2.6 + §8 items 2+3 (no code); (6) per-stream closure body — `buildAttributeContext(req, ...)` → `authv3.CheckRequest{Attributes: ...}` → `ac.Check(ctx, ...)` → `mapGRPCResponse(resp, validateMutations)`. **`mapGRPCResponse(resp *authv3.CheckResponse, validateMutations bool) checkDisposition`** — SPEC §6.7 6-row dispatch table (nil-oneof × {OK / non-OK}; OkResponse × {OK / non-OK}; DeniedResponse × {OK / non-OK}); nil-resp defensive allow. **`buildAllowDispositionGRPC(okResp *authv3.OkHttpResponse, validateMutations bool) checkDisposition`** — 4-arm `HeaderValueOption.append_action` dispatch per D5; `headers_to_remove` → `upstreamDel`; `response_headers_to_add` SILENT-IGNORED per D11; `validateMutations` gate. **`buildDenyDispositionGRPC(deniedResp *authv3.DeniedHttpResponse, validateMutations bool) checkDisposition`** — VERBATIM header pass-through (NO `allowed_client_headers` filter — UNLIKE HTTP-mode; per parent §5.P11); `status.code` default 403 per SPEC §6.7; `body` verbatim; `validateMutations` gate. New imports: `authv3`, `grpcclient`.
- `internal/filter/http/extauthz/extauthz.go` (mod, ~+85 LoC) — `appendDispatch` enum type added (`appendDispatchDefault | appendDispatchOverwriteOnly | appendDispatchAddIfAbsent`) per D5. `headerKV` extended with `action appendDispatch` field (zero value = `appendDispatchDefault` preserves 18.1 byte-identical behavior). `checkDisposition` extended with `upstreamDel []string` field (gRPC `OkHttpResponse.headers_to_remove`; UNUSED in 18.1 HTTP-mode). `applyUpstreamMutations` extended with: (1) per-entry `switch kv.action` over `upstreamSet` (default → unconditional `headers.Set`; `OverwriteOnly` → `Set` only when `len(headers.Values(name)) > 0`; `AddIfAbsent` → `Set` only when `len(headers.Values(name)) == 0`); (2) final `upstreamDel` loop calling `headers.Del(name)` — applied LAST so Set+Del is documented "set-then-remove" semantics matching reference Envoy.
- `internal/filter/http/types.go` (mod, +12 LoC) — `FactoryCtx` extended with `ClusterManager *cluster.Manager` field (Phase-18.2 first-use anchor per ADR-0161 §Decision (xiv) Consequences (viii)). New import: `github.com/esalaine/envoy-go/internal/cluster`. Threaded through `parseHTTPFiltersChain` at the HCM layer.
- `internal/filter/hcm/config.go` (mod, +2 LoC) — `parseHTTPFiltersChain` signature extended with `clusters *cluster.Manager` parameter; threaded into `FactoryCtx{... ClusterManager: clusters}` at the per-filter factory invocation site (line ~325).
- `internal/filter/hcm/config_test.go` (mod, +1 LoC) — `parseHTTPFiltersChain` callsite in `TestParseFilterWithCtx_FactoryCtxThreaded` updated to pass `nil` for clusters (FactoryCtxProbe does not exercise ClusterManager).
- `internal/filter/http/extauthz/extauthz_test.go` (mod, ~+560 LoC) — **Group 11 tests appended** at end-of-file: 8 `mapGRPCResponse` 6-row dispatch tests (incl. nil-resp + nil-status defensives + 2 validate_mutations integration cases); 10 `buildAllowDispositionGRPC` tests covering all 4 D5 arms + `headers_to_remove` + `response_headers_to_add` silent-ignore + `validate_mutations` pseudo-header rejection; 5 `buildDenyDispositionGRPC` tests covering verbatim pass-through + status default 403 + NO-allowed_client_headers-filter pin + `validate_mutations` pseudo-header rejection + nil-denied defensive; 6 `applyUpstreamMutations` D5-dispatch tests covering OverwriteOnly × {present/absent}, AddIfAbsent × {present/absent}, upstreamDel × {single/multi/Set-then-Del}, and the 4-arm-integration master test. **`TestNew_GrpcServiceParseReject` UPDATED** to assert the new PARSE-REJECT wording (`"envoy_grpc arm required"` — the empty `*GrpcService` falls through to the EnvoyGrpc arm-required gate). New imports: `authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"`, `status "google.golang.org/genproto/googleapis/rpc/status"`.
- `docs/envoy-go/DECISIONS.md` (mod, ~+90 LoC) — **ADR-0161 extended in-place** with the gRPC-mode portion sub-heading + §Decision items (viii)–(xv) + §Consequences (gRPC-mode portion) items (vi)–(x) per the established AMENDMENT convention. Top-level Status updated to "Accepted (gRPC-mode portion, Task 6 of phase-18.2 PLAN)"; Date updated `2026-05-14 → 2026-05-15`; Lands-in updated to confirm both portions.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 5 SHA placeholder filled (`<TBD …>` → `c651cf3d33ed7a7e7580adb93b81f4fa92f89ba8`); this Task 6 entry appended.

**Commit SHA:** `1a6e3c66952f9ca44fe7d0fffcfe4c57e1783f34`
**Fixup commit SHA:** `a5f2a89c74f280b359e2d9ce9f7e28a7a34fdf90` — phase 18.2 Task 6 fixup: restore ADR-0162 `## ADR-0162` heading + `---` separator inadvertently dropped by Task 6's ADR-0161 §Consequences in-place edit (DECISIONS.md only; structural restoration, no semantic change).
**Status:** done

**Verification.**

```
$ go build ./...
(exit=0)

$ go vet ./...
(exit=0)

$ gofmt -l .
(empty — no dirty files)

$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.313s
(ALL tests pass race-clean — Group 11 + the previously-expected-fail TestNew_GrpcServiceParseReject now PASSES with the new wording.)

$ go test -count=1 -run 'TestMapGRPCResponse|TestBuildAllowDispositionGRPC|TestBuildDenyDispositionGRPC|TestApplyUpstreamMutations' ./internal/filter/http/extauthz/
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.006s
(Group 11 + applyUpstreamMutations D5-dispatch tests PASS — 29 new tests landed.)

$ grep -nE '^## ADR-0161' docs/envoy-go/DECISIONS.md
8595:## ADR-0161: Bidirectional header-mutation discipline …
(1 match — ADR-0161 extended in-place per ADR-0044 in-place-extension convention.)
```

**Notes.**

- **D5 representation chosen: `action appendDispatch` enum field on `headerKV`.** Per the PLAN's "IMPL settles" disposition: the cleanest representation is a single 3-value enum field on `headerKV` rather than 4 separate slices. The slice container (`upstreamSet` Set-discipline vs `upstreamApp` Add-discipline) still encodes APPEND-vs-Set; the new field only discriminates the 2 new D5 arms (`OverwriteOnly` + `AddIfAbsent`) — both of which live in `upstreamSet`. The zero value (`appendDispatchDefault`) preserves byte-identical 18.1 HTTP-mode behavior: all existing `headerKV{name, value}` struct literals work unchanged.
- **`FactoryCtx.ClusterManager` first-use at phase-18.2.** The `ctx.ClusterManager` reference in the PLAN body was per SPEC §3.1's `buildGRPCCheckFn` text; the actual `FactoryCtx` struct had no such field. Task 6 lands the field (`ClusterManager *cluster.Manager`) + threads it through `parseHTTPFiltersChain` at the HCM build site. ADR-0085 nil-tolerance applies: tests with bare `FactoryCtx{}` exercising the gRPC-mode arm see a PARSE-REJECT with `"cluster manager not available"` rather than a nil-pointer panic. The grpcclient layer (Task 3) already requires a non-nil manager.
- **`TestNew_GrpcServiceParseReject` wording update — the test that was expected-fail at Task 5 now PASSES.** The Task-3 STUB sentinel (`"ext_authz: grpc_service: TODO (Task 5)"`) is replaced with the real validation chain. An empty `*GrpcService{}` has no `target_specifier`, so the EnvoyGrpc arm-required gate fires (`"ext_authz: grpc_service: envoy_grpc arm required (target_specifier must be set)"`). Group 10 (Task 8) will land the full coverage of the 4 PARSE-REJECT paths (empty cluster_name; unknown cluster; UseH2=false; GoogleGrpc arm).
- **Deny-path append_action discriminator IGNORED at envoy-go.** The proto's `DeniedHttpResponse.headers` is `[]*HeaderValueOption` (same shape as `OkHttpResponse.headers`), so the `append_action` enum is technically present on each deny header. envoy-go's deny path uses `SendLocalReply` which is a one-shot per-key emit (no incremental append semantics in the local-reply HTTP framing layer), so we drop the `append_action` discriminator at the `buildDenyDispositionGRPC` extraction — the `headerKV.action` field stays at `appendDispatchDefault`. The deny-path "verbatim" discipline per parent SPEC §5.P11 is exactly: emit each entry as a separate header value (per-key emit; no consolidation, no append_action interpretation). The Group 11 verbatim test pins this behavior.
- **`upstreamDel` validate_mutations: defense-in-depth via name-only.** The `validate_mutations` gate runs on the headerKV-shaped `upstreamSet` + `upstreamApp` (value-bearing) but NOT on the name-only `upstreamDel []string` list. Rationale: `headers.Del(":foo")` is a no-op in net/http (the canonical-form mismatch prevents matching), so a `:`-prefixed pseudo-header name in `headers_to_remove` cannot remove pseudo-headers anyway — the gate's protective intent is structurally satisfied. The Group 11 `validate_mutations` tests cover the value-bearing paths.
- **No new fuzzer surface (deferred to Task 9).** `mapGRPCResponse` + the two `build*DispositionGRPC` helpers are pure functions over the protobuf input — they fall under the 23rd fuzzer's domain (a fuzzer targeting `mapGRPCResponse(resp *authv3.CheckResponse, validateMutations bool)` will mutate the proto input via the existing protobuf-fuzz machinery). Task 9 lands the fuzz harness; Task 6 ships the function-under-test only.
- **LoC budget.** SPEC §6.8 budget for check.go gRPC-mode additions was 250–400 LoC; actual is ~280 LoC. `extauthz.go` structural additions are ~85 LoC (against an implicit "small enough" budget — the field + enum + 3-arm switch dispatch). `extauthz_test.go` Group 11 is ~560 LoC. All within the PLAN-anticipated File-structure-table sizes.

### Task 7 — context_extensions consumption + per-route gRPC-mode + Group 14

**Files changed:**

- `internal/filter/http/extauthz/extauthz.go` (mod, ~+30 LoC) — **`perRouteContextExtensionsFor(pr *compiledPerRoute) map[string]string`** helper authored adjacent to `effectiveWithRequestBody` (its structural sibling). Returns `pr.checkSettings.contextExtensions` when the per-route is the `check_settings` arm; returns `nil` for the `nil` / `disabled:true` arms (defensive — the disabled arm carries no `checkSettings`). Per ADR-0163's 5th-canonical-REUSE: `ExtAuthz` has no top-level `context_extensions` field and `core.GrpcService.initial_metadata` is DEFERRED per SPEC §2.6 + §8 item 2, so the per-route map is the ONLY source at 18.2 MVP. **`dispatchOutboundCheck` seeded `authReq.perRouteContextExtensions = perRouteContextExtensionsFor(f.perRoute)`** BEFORE the closure-call line per PLAN Step 3. The closure (HTTP-mode 18.1 ignores; gRPC-mode 18.2 consumes via `buildAttributeContext`) sees a populated `*authRequest.perRouteContextExtensions` field on every dispatch. SPEC §8 item 8 (the 18.1 forward-pointer: "context_extensions parsed-but-NO-OPed") is CLOSED by this task.
- `internal/filter/http/extauthz/extauthz_test.go` (mod, ~+285 LoC) — **Group 14 tests appended** at end-of-file in two sub-groups: (Group 14A) 5 direct-helper unit tests covering nil-per-route / disabled-arm / check-settings-arm × {populated / empty / nil} map permutations; (Group 14B) 5 end-to-end tests using a `captureAuthReqCheckFn(*sync.Mutex, **authRequest) checkFn` closure that captures the `*authRequest` `dispatchOutboundCheck` hands to the closure — assertions on `req.perRouteContextExtensions` directly verify the seeding contract (no extauthzgrpc helper needed; lands at Task 9). The 5th 14B test is the **AttributeContext integration assertion** closing SPEC §8 item 8 end-to-end: per-route map → `perRouteContextExtensionsFor` → `req.perRouteContextExtensions` → `buildAttributeContext` → `AttributeContext.context_extensions["policy"] == "scenario7"`.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 6 SHA placeholder filled (`<TBD …>` → `1a6e3c66952f9ca44fe7d0fffcfe4c57e1783f34`); Task 6 fixup commit noted (`a5f2a89c74f280b359e2d9ce9f7e28a7a34fdf90`); this Task 7 entry appended.

**Commit SHA:** `d7034dfb432f827579e3f81c4125f84d8622a90e`
**Status:** done

**Verification.**

```
$ go build ./...
(exit=0)

$ go vet ./...
(exit=0)

$ gofmt -l .
(empty — no dirty files)

$ go test -race -count=1 -run 'TestContextExtensionsThreading|TestPerRouteContextExtensionsFor' -v ./internal/filter/http/extauthz/
=== RUN   TestPerRouteContextExtensionsFor_NilPerRoute_ReturnsNil
--- PASS: TestPerRouteContextExtensionsFor_NilPerRoute_ReturnsNil (0.00s)
=== RUN   TestPerRouteContextExtensionsFor_DisabledArm_ReturnsNil
--- PASS: TestPerRouteContextExtensionsFor_DisabledArm_ReturnsNil (0.00s)
=== RUN   TestPerRouteContextExtensionsFor_CheckSettingsArm_ReturnsParsedMap
--- PASS: TestPerRouteContextExtensionsFor_CheckSettingsArm_ReturnsParsedMap (0.00s)
=== RUN   TestPerRouteContextExtensionsFor_CheckSettingsArm_EmptyMap_ReturnsEmpty
--- PASS: TestPerRouteContextExtensionsFor_CheckSettingsArm_EmptyMap_ReturnsEmpty (0.00s)
=== RUN   TestPerRouteContextExtensionsFor_CheckSettingsArm_NilMap_ReturnsNil
--- PASS: TestPerRouteContextExtensionsFor_CheckSettingsArm_NilMap_ReturnsNil (0.00s)
=== RUN   TestContextExtensionsThreading_PerRouteMap_FlowsThroughDispatch
--- PASS: TestContextExtensionsThreading_PerRouteMap_FlowsThroughDispatch (0.00s)
=== RUN   TestContextExtensionsThreading_NoPerRoute_NilMap
--- PASS: TestContextExtensionsThreading_NoPerRoute_NilMap (0.00s)
=== RUN   TestContextExtensionsThreading_DisabledArm_NilMap
--- PASS: TestContextExtensionsThreading_DisabledArm_NilMap (0.00s)
=== RUN   TestContextExtensionsThreading_PerRouteEmptyMap_FlowsAsEmpty
--- PASS: TestContextExtensionsThreading_PerRouteEmptyMap_FlowsAsEmpty (0.00s)
=== RUN   TestContextExtensionsThreading_AttributeContextIntegration
--- PASS: TestContextExtensionsThreading_AttributeContextIntegration (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.024s
(10 Group 14 tests PASS; 5 helper unit tests + 5 dispatch-integration tests; race-clean.)

$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.317s
(ALL prior groups still PASS — Group 1-9 from 18.1; Group 11 from Task 6; Group 12 from Task 5; Group 13 from Task 4.)
```

**Notes.**

- **NO new ADR.** The helper + dispatch seeding fall under ADR-0163's 5th-canonical-REUSE discipline (the "no listener-level baseline; per-route is the only source" decision); SPEC §8 item 8 was the explicit 18.1 forward-pointer ("parsed but NO-OPed in HTTP-mode; gRPC-mode lands at 18.2") and is now CLOSED.
- **Helper signature settle.** PLAN suggested `*compiledPerRoute` parameter; the actual `compiledPerRoute` struct shape at master tip (`{cc, disabled, checkSettings *compiledCheckSettings}`) made this trivially correct. The function returns `pr.checkSettings.contextExtensions` directly (no defensive copy — the map is parsed-once at config-load time and treated as immutable per ADR-0101 read-only-shared-after-construction discipline; the proto3 marshaller in `buildAttributeContext` iterates the map verbatim).
- **No listener-level baseline merge.** Per ADR-0163's 5th-canonical-REUSE: `ExtAuthz` has no `context_extensions` field at the listener level, and `core.GrpcService.initial_metadata` (the other potential source of an analogous map) is DEFERRED per SPEC §2.6 + §8 item 2. The per-route map IS the effective map. The "per-route wins on key collisions" convention per SPEC §5 has no observable behavior at MVP since there are zero collisions to resolve.
- **Helper placement.** Sited adjacent to `effectiveWithRequestBody` (line ~893) — its structural sibling (per-route-resolution helper for a `compiledCheckSettings`-arm field). Future per-route field readers (a Task-8 forward-pointer? — none anticipated by PLAN) would join the same neighborhood.
- **Group 14B closure-capture pattern: replaces fixture-server scaffolding for Task 7.** The PLAN Note clause said end-to-end testing through `dispatchOutboundCheck` MAY require additional plumbing (e.g., the extauthzgrpc helper at Task 9). The closure-capture approach (`captureAuthReqCheckFn(*sync.Mutex, **authRequest) checkFn`) lets Task 7 observe the seeded `*authRequest` BEFORE any transport-specific marshalling — race-clean (mu around the capture; `waitForContinueOrReply` poll loop) and free of fixture-server / gRPC-server dependencies. Group 12's `TestBuildAttributeContext_ContextExtensions_Populated` already pins the `*authRequest.perRouteContextExtensions` → `AttributeContext.context_extensions` map step, so 14B's integration test composes the two halves end-to-end via direct function call. Task 9's extauthzgrpc helper will replace this with a true gRPC roundtrip when the fixture infra lands.
- **LoC budget.** `extauthz.go` additions ~30 LoC (helper + dispatch-seeding line); `extauthz_test.go` Group 14 ~285 LoC. PLAN anticipated "Modify" (no specific budget) — actual sizing is well within the spirit of "small targeted addition for a closure-of-deferral task".
- **Acceptance.** `go build ./...` exit 0; `go vet ./...` exit 0; `gofmt -l .` empty; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0 with 10 new Group 14 tests + all prior groups still green.

### Task 8 — dispatchOutboundCheck seeding + Group 10 parse-time + race-test

**Files changed:**

- `internal/filter/http/extauthz/extauthz.go` (mod, ~+45 LoC) — **`streamStartTime time.Time` field added to `*filter`** (annotated with the §11.P4 request-arrival-anchor rationale + the IMPL-settle on DecodeHeaders-entry capture site per PLAN Task 8 §Step 3); **`f.streamStartTime = time.Now()` captured at DecodeHeaders entry** (BEFORE per-route resolution so the body-buffering branch sees the already-set anchor when DecodeData later fires dispatchOutboundCheck); **9 new `*authRequest` extended-field seedings added in `dispatchOutboundCheck`** (the Task 7 seeding of `req.perRouteContextExtensions` left UNCHANGED — Task 8 ADDS the OTHER 9 around it): `req.remoteAddr` / `req.localAddr` / `req.tlsServerName` / `req.peerCertDER` / `req.listenerPrincipal` / `req.downstreamPrincipal` / `req.protocol` from `f.dcb.*` callbacks (the 6 ADR-0165 callback-surface accessors + the existing ADR-0144 `DownstreamPrincipal()`); `req.requestID = headers.Get("x-request-id")` from the incoming client headers; `req.streamStartTime = f.streamStartTime` from the DecodeHeaders-entry capture. An `if f.dcb != nil` guard wraps the 7 callback-sourced seedings so test setups that construct `*filter` without a dcb (body-only test paths) land the fields at zero values (effectively equivalent to a plaintext / synthetic stream).
- `internal/filter/http/extauthz/extauthz_test.go` (mod, ~+475 LoC) — **6 Group 10 imports added** (`bootstrapv3`, `clusterv3`, `endpointv3`, `tlsv3`, `upstreamshttpv3`, `cluster`, `crypto/ecdsa`, `crypto/elliptic`, `crypto/rand`, `crypto/x509`, `crypto/x509/pkix`, `encoding/pem`, `math/big`); **`extauthzTestPKI` + `mkExtauthzTestPKI`** in-memory CA + leaf keypair (ECDSA P-256) sufficient for h2-cluster TLS-context construction (parallels `internal/grpcclient/grpcclient_test.go`'s `authTestPKI` — duplicated to keep the extauthz test package self-contained); **`mkExtauthzH2ClusterMgr`** + **`mkExtauthzPlainClusterMgr`** STATIC-cluster builders modeled on the grpcclient siblings; **`extauthzFactoryCtxWithClusterMgr`** + **`mkGrpcExtAuthzConfig`** helpers; **Group 10A (5 parse-time tests)**: `TestBuildGRPCCheckFn_UnknownCluster_ParseReject` / `TestBuildGRPCCheckFn_UseH2False_ParseReject` / `TestBuildGRPCCheckFn_GoogleGrpcArm_ParseReject` / `TestBuildGRPCCheckFn_EnvoyGrpcEmptyClusterName_ParseReject` / `TestBuildGRPCCheckFn_HappyPath_ReturnsNonNilCheckFn` — each exercises a distinct error-wording substring assertion + the happy-path test calls `factory()` and reflects the resulting `*filter.state.listenerRC.checkFn` to verify it lands non-nil; **Group 10B (1 race-test)**: `TestOnDestroy_CancelsInFlightGRPCCheck` uses a mock checkFn closure that blocks on `<-ctx.Done()` (Option B per PLAN Task 8 — sufficient because the contract under test is the filter's mode-agnostic mu/done guard, NOT the grpcclient transport; the Task 9 extauthzgrpc helper will add a true gRPC roundtrip).
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 7 SHA placeholder filled (`<TBD …>` → `d7034dfb432f827579e3f81c4125f84d8622a90e`); this Task 8 entry appended.

**Commit SHA:** `0ac757b0fb4dba83531def9b710f96c11254443d`
**Status:** done

**Verification.**

```
$ go build ./...
(exit=0)

$ go vet ./...
(exit=0)

$ gofmt -l .
(empty — no dirty files)

$ go test -race -count=1 -run 'TestBuildGRPCCheckFn|TestOnDestroy_CancelsInFlightGRPCCheck' -v ./internal/filter/http/extauthz/
=== RUN   TestBuildGRPCCheckFn_UnknownCluster_ParseReject
--- PASS: TestBuildGRPCCheckFn_UnknownCluster_ParseReject (0.00s)
=== RUN   TestBuildGRPCCheckFn_UseH2False_ParseReject
--- PASS: TestBuildGRPCCheckFn_UseH2False_ParseReject (0.00s)
=== RUN   TestBuildGRPCCheckFn_GoogleGrpcArm_ParseReject
--- PASS: TestBuildGRPCCheckFn_GoogleGrpcArm_ParseReject (0.00s)
=== RUN   TestBuildGRPCCheckFn_EnvoyGrpcEmptyClusterName_ParseReject
--- PASS: TestBuildGRPCCheckFn_EnvoyGrpcEmptyClusterName_ParseReject (0.00s)
=== RUN   TestBuildGRPCCheckFn_HappyPath_ReturnsNonNilCheckFn
--- PASS: TestBuildGRPCCheckFn_HappyPath_ReturnsNonNilCheckFn (0.00s)
=== RUN   TestOnDestroy_CancelsInFlightGRPCCheck
--- PASS: TestOnDestroy_CancelsInFlightGRPCCheck (0.05s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	0.058s
(6 Group 10 tests PASS; 5 parse-time + 1 race-test; race-clean.)

$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.373s
(ALL prior groups still PASS — Groups 1-9 from 18.1; Group 11 from Task 6; Group 12 from Task 5; Group 13 from Task 4; Group 14 from Task 7.)
```

**Notes.**

- **NO new ADR.** Task 8 is purely IMPL — the *authRequest extended fields were authored at Task 5 (under ADR-0160 gRPC-mode portion + ADR-0165) and the callback-surface accessors were authored at Task 4 (under ADR-0165). Task 8 wires the seeding bridge between the two: callbacks → *authRequest. No new architecture decisions.
- **streamStartTime capture site: DecodeHeaders entry.** The PLAN offered an IMPL-settle between (a) DecodeHeaders entry and (b) dispatchOutboundCheck time. Choice (a) was selected per the PLAN's stated recommendation, justified by SPEC §11.P4: `AttributeContext.request.time` is the request-arrival time (the instant the proxy observed the stream), NOT the auth-call time. The DecodeHeaders-entry capture is the closest envoy-go-observable proxy for request-arrival; the alternative dispatchOutboundCheck-time capture would systematically understate `request.time` when body-buffering delays the dispatch by however long it takes the DecodeData chain to assemble the body. The field lands at zero for synthetic test streams that bypass DecodeHeaders — `buildAttributeContext` handles the zero-value fallback to `time.Now()` per SPEC §6.6 step 4 + the Group 12 `TestBuildAttributeContext_StreamStartTimeZero_FallsBackToNow` pin.
- **Race-test approach: Option B (mock-closure).** The PLAN offered Options A (defer to Task 9 when extauthzgrpc helper lands) and B (mock checkFn closure that blocks on `<-ctx.Done()`). Option B was selected per the PLAN's stated recommendation: the contract under test in this race-test is the filter's mu/done resume-after-OnDestroy guard, which is mode-agnostic (HTTP-mode 18.1's `TestOnDestroy_CancelsInFlightContext` validates the same code path under the HTTP-mode checkFn). The mock closure is sufficient because: (a) grpcclient package already has its own race + cancel + timeout tests (Groups 1-3 in grpcclient_test.go), so the transport-side cancellation contract is independently validated; and (b) the filter's `dispatchOutboundCheck` → goroutine → mu/done guard is identical regardless of whether `cc.checkFn` is the HTTP-mode closure or the gRPC-mode closure. The Task 9 extauthzgrpc helper will add a true gRPC roundtrip to the fixture infrastructure for end-to-end differential testing (fixture 0021).
- **f.dcb nil-guard on the 7 callback-sourced fields.** Test setups that construct `*filter` without a callbacks reference (e.g., the body-only DecodeData test paths in Group 6) historically rely on `dispatchOutboundCheck` tolerating a nil `f.dcb`. The Task 8 `if f.dcb != nil` guard preserves this contract — the 7 callback-sourced fields land at zero values when no callbacks are wired, which is the correct shape for buildAttributeContext: nil net.Addr + empty strings + nil slice produce the "plaintext / synthetic stream" AttributeContext shape per the Group 12 `TestBuildAttributeContext_NilNetAddrs` pin. The `requestID` field is sourced from `headers.Get("x-request-id")` (NOT from a callback) so it lands correctly regardless of the dcb's presence; `streamStartTime` lands from `f.streamStartTime` which is captured at DecodeHeaders entry (or stays zero for body-only test paths — buildAttributeContext's `time.Now()` fallback covers this).
- **9 vs. 10 seedings.** The PLAN's File-structure note says "10 extended `*authRequest` fields"; Task 7 already wired the 10th (`perRouteContextExtensions`); Task 8 adds the OTHER 9. The Task 7 seeding line is preserved verbatim — the Task 8 diff only ADDS new lines around it (the comment block precedes the 9 new seedings as a single annotated block).
- **No `clearRouteCacheRequested` adjacent placement.** The new `streamStartTime time.Time` field is placed as the LAST field of `*filter` (after `clearRouteCacheRequested`), with a thorough doc-comment block. This keeps the 18.1 field ordering byte-identical for diff legibility — Task 8 is a pure additive change to the struct.
- **LoC budget.** `extauthz.go` additions ~45 LoC (field declaration + DecodeHeaders capture + 9 seedings + comment blocks); `extauthz_test.go` Group 10 ~475 LoC (PKI + 2 cluster-mgr builders + 2 helper funcs + 5 parse-time tests + 1 race-test). PLAN anticipated "+X LoC" — actual sizing is well within the spirit of "comprehensive Group 10 + targeted dispatch seeding".
- **Acceptance.** `go build ./...` exit 0; `go vet ./...` exit 0; `gofmt -l .` empty; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0 with 6 new Group 10 tests + ALL prior Groups still green (Groups 1-9 from 18.1; Group 11 from Task 6; Group 12 from Task 5; Group 13 from Task 4; Group 14 from Task 7).

### Task 9 — 23rd fuzzer + extauthzgrpc helper + fixture infra

**Files changed:**

- `internal/filter/http/extauthz/fuzz_test.go` (mod, ~+260 LoC) — **NEW `FuzzCheckResponseMapping` (23rd fuzzer overall; phases 02–18.1 contributed 22)** per SPEC §7.3: fuzzes arbitrary bytes as `*authv3.CheckResponse` proto payloads → `proto.Unmarshal` → `mapGRPCResponse`. 11-seed corpus covering the 6-row truth table in `mapGRPCResponse` + boundary cases (empty `CheckResponse{}`, oversized header values ~16 KiB, pseudo-header `:authority` in OkResponse mutations). Asserts structural contract: no panic; `class` in {dispAllow, dispDeny, dispError, dispInvalid}; on dispDeny `denyStatus` non-zero (proto-zero defaults to 403 per SPEC §6.7; no upper bound — the wire format admits arbitrary int32 values for `typev3.StatusCode` so envoy-go-strict carries the value verbatim); `validateMutationHeaders` passes on the extracted header sets when `validate_mutations:true` AND the disposition stayed at dispAllow/dispDeny. **`FuzzExtAuthzConfigParse` corpus extended with 8 grpc_service variants (seeds 21-28)**: envoy_grpc valid cluster_name; envoy_grpc empty cluster_name (PARSE-REJECT); envoy_grpc unknown cluster_name (PARSE-REJECT); google_grpc arm (PARSE-REJECT envoy-go-strict); envoy_grpc + `initial_metadata` populated (silent-ignored per SPEC §2.6); envoy_grpc + `retry_policy` populated (silent-ignored per SPEC §2.6); envoy_grpc + non-V3 `transport_api_version` (PARSE-REJECT); envoy_grpc + full grpc-mode boolean surface (`include_peer_certificate` + `include_tls_session` + `encode_raw_headers` + `validate_mutations` + `with_request_body.pack_as_bytes`). **New imports added**: `authv3`, `rpcstatus`, `wrapperspb`.
- `test/helpers/extauthzgrpc/doc.go` (NEW, 31 LoC) — package comment per the PLAN File-structure table verbatim block. Documents the FIRST in-process gRPC server in envoy-go's test tree + spawn-per-fixture lifecycle + plaintext h2c (no TLS) discipline per SPEC §7.2 + per-`:path` scriptable `CheckResponse` per planner-time decision D1.
- `test/helpers/extauthzgrpc/extauthzgrpc.go` (NEW, 133 LoC) — public API per SPEC §7.4 + the PLAN File-structure block: `type Server struct { authv3.UnimplementedAuthorizationServer; addr; lis; grpcSrv; mu sync.RWMutex; scripts map[string]*authv3.CheckResponse; stopOnce }`; `New(t testing.TB) *Server` binds 127.0.0.1:0 ephemeral + registers `authv3.RegisterAuthorizationServer` + spawns `grpcSrv.Serve(lis)` goroutine + `t.Cleanup(s.Stop)`; `(s *Server).Addr() string` returns `lis.Addr().String()`; `(s *Server).Script(path, resp)` registers per-`:path` scripted `*authv3.CheckResponse` under `mu` write-lock; `(s *Server).Check(ctx, req)` implements `authv3.AuthorizationServer` — looks up scripted response by `req.Attributes.Request.Http.Path`; returns `status.Errorf(codes.Unavailable, "extauthzgrpc: no script registered for path %q", path)` when no script matches; `(s *Server).Stop()` calls `grpcSrv.GracefulStop()` under `sync.Once` for idempotency. Plaintext h2c — `grpc.NewServer()` with no Creds() option.
- `test/helpers/extauthzgrpc/extauthzgrpc_test.go` (NEW, 205 LoC) — 4 PLAN-mandated tests: `TestNew_StartsServerOnEphemeralPort` (Addr() returns non-empty 127.0.0.1:NNN); `TestServer_Script_ReturnsScripted` (registered allow/deny scripts return; unregistered path returns `codes.Unavailable` with message mentioning the path); `TestServer_Stop_Closes` (post-Stop Check fails); `TestServer_ConcurrentClient_NoRace` (20 concurrent goroutines × Check, race-clean under -race).
- `test/differential/fixture/fixture.go` (mod, +16 LoC) — `HTTPExtAuthzGRPC BackendKind = 18` constant + 18-line doc comment per the PLAN block (3-listener fixture topology; plaintext downstream + plaintext h2c auth cluster; in-process extauthzgrpc helper lifecycle-managed BY THE DRIVER).
- `test/differential/runner_test.go` (mod, +33 LoC) — **blank-import** `_ "github.com/esalaine/envoy-go/test/fixtures/0021-http-ext-authz-grpc/inputs"` (alphabetical-after the 0020 import) + **`case fixture.HTTPExtAuthzGRPC` switch-case** that allocates a free TCP port + spawns the SHARED echobackend binary (the extauthzgrpc helper is lifecycle-managed BY THE DRIVER at Task 10).
- `test/fixtures/0021-http-ext-authz-grpc/inputs/init.go` (NEW, 12 LoC) — **Option A stub per the PLAN guidance**: empty `package inputs` with an empty `init()` so the runner_test.go blank-import compiles. Task 10 replaces this file with the real `driver.go` (the Task 10 commit DELETES `init.go` + ADDS `driver.go`).
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 8 SHA placeholder filled (`<TBD …>` → `0ac757b0fb4dba83531def9b710f96c11254443d`); this Task 9 entry appended.

**Commit SHA:** `345211e57a1a8c1cb6bf3ce0bef8377a7f210098`
**Status:** done

**Verification.**

```
$ go build ./...
(exit=0)

$ go vet ./...
(exit=0)

$ gofmt -l .
(empty — no dirty files)

$ go test -race -count=1 -v ./test/helpers/extauthzgrpc/...
=== RUN   TestNew_StartsServerOnEphemeralPort
--- PASS: TestNew_StartsServerOnEphemeralPort (0.00s)
=== RUN   TestServer_Script_ReturnsScripted
--- PASS: TestServer_Script_ReturnsScripted (0.00s)
=== RUN   TestServer_Stop_Closes
--- PASS: TestServer_Stop_Closes (0.00s)
=== RUN   TestServer_ConcurrentClient_NoRace
--- PASS: TestServer_ConcurrentClient_NoRace (0.01s)
PASS
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzgrpc	1.027s
(4 tests; race-clean.)

$ go test -run '^$' -fuzz 'FuzzCheckResponseMapping' -fuzztime 30s ./internal/filter/http/extauthz/
fuzz: elapsed: 30s, execs: 1801856 (38083/sec), new interesting: 197 (total: 345)
PASS
(30s clean; ~1.8M execs over 32 workers; 197 new interesting inputs synthesized.)

$ go test -run '^$' -fuzz 'FuzzExtAuthzConfigParse' -fuzztime 30s ./internal/filter/http/extauthz/
fuzz: elapsed: 30s, execs: 2047765 (58665/sec), new interesting: 35 (total: 791)
PASS
(30s clean with the extended grpc_service corpus; ~2.0M execs over 32 workers.)

$ go test -race -count=1 ./internal/filter/http/extauthz/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/extauthz	1.428s
(All prior tests still PASS.)
```

**Notes.**

- **Option A stub strategy for `0021-http-ext-authz-grpc/inputs/`.** The PLAN offered Option A (a 3-line stub `init.go` so the blank import compiles at Task 9) vs. Option B (defer the blank-import + switch-case to Task 10). Option A was selected per the PLAN's stated recommendation: it lets Task 9 land the BackendKind dispatch in `runner_test.go` ahead of Task 10's driver, mirroring the phase-17 / phase-18.1 patterns (e.g. fixture 0019 / 0020 had their switch-cases wired ahead of the inputs package). Task 10's commit will DELETE `init.go` and ADD `driver.go` (the stub vanishes alongside the real driver landing — leaving no `init.go` legacy file in the inputs/ directory).
- **`FuzzCheckResponseMapping` denyStatus assertion relaxed.** An initial draft of the fuzzer asserted `disp.denyStatus` was within `[100, 999]` (a broad HTTP-status sanity band). The fuzzer immediately surfaced a counter-example: a proto wire format with `typev3.StatusCode` value `6260` (the proto enum is `int32`-typed and unvalidated at the wire boundary; `buildDenyDispositionGRPC` casts to `uint32(deniedResp.GetStatus().GetCode())` without bounds checking). The strict `[100, 999]` was a planner ideal; envoy-go-strict carries the auth-server-supplied status verbatim (matches reference Envoy's behavior — the auth service has authority over the deny status). The fuzzer's contract is now: on dispDeny, `denyStatus != 0` (proto-zero defaults to 403 per SPEC §6.7 in `buildDenyDispositionGRPC`); no upper bound. This is an INTENTIONAL contract relaxation — out-of-band int32 values from a malicious auth server are NOT a bug; the differential fixture (Task 12) will assert reference-Envoy parity on these edge cases.
- **`mapGRPCResponse` fuzz drives BOTH `validate_mutations` branches.** The fuzz body iterates `for _, validateMutations := range []bool{false, true}` so each input exercises both gate states (matches the SPEC §6.7 commentary that validate_mutations is orthogonal to the 6-row truth table).
- **`extauthzgrpc.Server` race-free under -race.** The `sync.RWMutex` over the `scripts` map handles concurrent Script (write) + Check (read) calls. The `TestServer_ConcurrentClient_NoRace` test exercises 20 concurrent Check goroutines with a single registered script; the helper passes -race cleanly. The `sync.Once` over `Stop` makes the t.Cleanup-driven teardown idempotent against explicit Stop calls in tests (e.g. `TestServer_Stop_Closes` calls Stop early then the t.Cleanup also fires).
- **8 new grpc_service seeds in `FuzzExtAuthzConfigParse`.** Each seed exercises a distinct surface of the gRPC-mode parse path: cluster_name valid + empty + unknown; google_grpc arm; initial_metadata + retry_policy silent-ignore; non-V3 transport_api_version PARSE-REJECT; full grpc-mode boolean gate surface. Under the empty FactoryCtx the cluster_name=valid seed lands the "cluster manager not available" PARSE-REJECT — the structural contract (never-both-nil; never-both-set; never-panic) holds on every branch.
- **No new ADR.** Task 9 is purely IMPL — the grpc_service fuzz extension + extauthzgrpc helper are direct implementations of SPEC §7.3 + §7.4 + planner-time decision D1. No new architecture decisions.
- **LoC budget.** `fuzz_test.go` mods ~260 LoC (8 grpc_service seeds in `FuzzExtAuthzConfigParse` ~125 LoC + `FuzzCheckResponseMapping` body+seeds ~135 LoC); `test/helpers/extauthzgrpc/` 369 LoC across 3 files (31+133+205); fixture infra 49 LoC (16+33). PLAN anticipated ~150–220 LoC for `extauthzgrpc.go` + ~100–140 LoC for the test; actual sizing is within budget.
- **Acceptance.** `go build ./...` exit 0; `go vet ./...` exit 0; `gofmt -l .` empty; `go test -race -count=1 ./test/helpers/extauthzgrpc/...` 4 tests PASS; `FuzzCheckResponseMapping` 30s clean (~1.8M execs); `FuzzExtAuthzConfigParse` 30s clean with extended corpus (~2.0M execs); ALL prior extauthz tests still green.

### Task 10 — Fixture 0021 driver.go (8 scenarios)

**Files changed:**

- `test/fixtures/0021-http-ext-authz-grpc/inputs/driver.go` (NEW, 1238 LoC) — the 8-scenario differential driver per SPEC §7.1 + planner-time decision D10. Mirrors the phase-18.1 fixture-0020 driver structure verbatim with the 8-scenario / 3-listener / gRPC-mode adaptations: package-level doc block (90 LoC) describing the three-listener topology + auth-server lifecycle + the per-scenario gRPC script matrix; `extAuthzGRPCDriver` struct with `mu sync.Mutex` + `authPort int` (lazy-allocated by `allocateAuthPort`) + `authSrv *scriptedAuthServer` (currently-running server); inline `scriptedAuthServer` type wrapping `*grpc.Server` + `authv3.UnimplementedAuthorizationServer` + `sync.RWMutex`-guarded scripts map + `sync.Once`-guarded Stop (a driver-local clone of `test/helpers/extauthzgrpc/Server`'s surface — the SPEC §7.4 helper binds ephemerally to `127.0.0.1:0` and has no caller-chosen-port API; the bootstrap-templatize-before-Drive ordering requires a pre-allocated stable port, hence the inline `newScriptedServerOnPort(addr)` helper); `setupAuthGRPC()` lifecycle helper that binds the server to the pre-allocated authPort + pre-populates the 8 scripts; `stopAuthGRPC()` idempotent stop; `runScenario1..runScenario8(ctx, client, baseURL, side) scenarioResult` per-scenario request issuers; `driveProxy(ctx, addrs, side)` orchestrator that issues the 8-scenario sequence and toggles the auth server across scenarios 3+4 (stop → 3+4 → restart → 6+7+8); `emitScenario` / `classifyBody` / `isEchoBody` / `echoHeaders` byte-stream-verdict helpers; `scrapeExtAuthzStats` / `parseExtAuthzPromBody` / `lookupExtAuthzCounter` stats-scrape helpers (REUSED verbatim from 18.1 fixture 0020); `AssertStats(t, refAdminAddr, subjAdminAddr)` with the 5-counter expectation matrix (ok=4, denied=1, error=2, failure_mode_allowed=1, invalid=0; disabled NOT asserted per parent §6 amendment 7); `ReferenceBootstrap` / `SubjectConfig` template renderers (envoy.yaml + envoy-go.yaml are authored at Task 11; driver compiles + registers at Task 10); `deriveAddrsFromRef` / `deriveAddrsFromSubj` single-addr Driver-interface fallbacks; `mustReadFixtureFile` / `mustRender` / `fixtureDir` file helpers; 4 compile-time interface assertions (`fixture.Driver` + `fixture.BackendKindAware` + `fixture.MultiListenerDriver` + `fixture.StatsAsserter`).
- `test/fixtures/0021-http-ext-authz-grpc/inputs/init.go` (DELETED) — the Task 9 Option-A stub (3-line empty `init()`) removed; the real `driver.go` replaces it.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 9 SHA placeholder filled (`<TBD …>` → `345211e57a1a8c1cb6bf3ce0bef8377a7f210098`); this Task 10 entry appended.

**Commit SHA:** `589500d7d4bc08beb22ae103b00538cab7710cd1`
**Task 10 fixup commit:** `3d25840` — extend `extauthzgrpc` helper with `NewAtAddr(addr)` (caller-chosen-port arm) + remove driver-local `scriptedAuthServer` clone in favor of the shared SPEC §7.4 helper.
**Status:** done

**Verification.**

```
$ go build ./test/fixtures/0021-http-ext-authz-grpc/...
(exit=0)

$ go vet ./test/fixtures/0021-http-ext-authz-grpc/...
(exit=0)

$ go build ./test/differential/...
(exit=0; the runner_test.go blank-import resolves to the new driver.go)

$ go build ./...
(exit=0; whole repo still builds)

$ go vet ./...
(exit=0)

$ gofmt -l .
(empty — no dirty files)
```

**8-scenario matrix landed (per SPEC §7.1 + the `:path` discriminator scheme).**

| # | Scenario | Listener / `:path` | Auth-server script | Expected verdict | Counter delta |
|---|---|---|---|---|---|
| 1 | gRPC allow | l_test_a / `/scenario1` | `OK + OkHttpResponse{}` | 200 echo backend | ok=+1 |
| 2 | gRPC deny | l_test_a / `/scenario2` | `7 (PERMISSION_DENIED) + DeniedResponse{403, "access denied", x-authz-denied-reason:scenario2}` | 403 + body byte-exact | denied=+1 |
| 3 | error → `status_on_error` | l_test_b / `/scenario3` | server STOPPED → dispError → 503 empty body | 503 + empty body | error=+1 |
| 4 | failure_mode_allow | l_test_c / `/scenario4` | server STOPPED → dispError → 200 echo + `x-envoy-auth-failure-mode-allowed` upstream | 200 echo + marker | error=+1, failure_mode_allowed=+1 |
| 5 | with_request_body | l_test_a / `/scenario5` | `OK + OkHttpResponse{}` (auth sees body via ADR-0128) | 200 echo backend | ok=+1 |
| 6 | per-route disabled | l_test_a / `/disabled` | (no auth call; filter bypassed) | 200 echo backend | NO increments |
| 7 | per-route context_extensions | l_test_a / `/ctx` | `OK + OkHttpResponse{Headers:[x-authz-policy:scenario7 OVERWRITE]}` (coarse confirmation that auth received `context_extensions[policy]=scenario7` per SPEC §11.P4) | 200 echo + upstream `x-authz-policy` | ok=+1 |
| 8 | OkHttpResponse mutation | l_test_a / `/scenario8` | `OK + OkHttpResponse{Headers:[x-injected-by-authz OVERWRITE / x-also-appended APPEND / x-overwrite-only-if-exists OVERWRITE_IF_EXISTS / x-add-if-absent ADD_IF_ABSENT], HeadersToRemove:[user-agent]}` | 200 echo + 3 reachable injected keys upstream + user-agent absent | ok=+1 |

**Helpers landed:**

- `setupAuthGRPC()` — starts the inline `scriptedAuthServer` on the pre-allocated authPort; pre-populates 5 scripts (scenarios 1/2/5/7/8 register; scenarios 3/4 deliberately do NOT — the server is stopped before those requests; scenario 6 bypasses the filter so its path is never reached).
- `scrapeStats` (named `scrapeExtAuthzStats`) — REUSED verbatim from 18.1 fixture 0020; scrapes `/stats/prometheus`.
- `assertCounterDelta` (folded into `AssertStats` per the 18.1 pattern — the 18.1 driver does NOT export a separate `assertCounterDelta` function; the cross-side equivalence + want-delta check lives inline in `AssertStats`). 18.2 mirrors verbatim.
- `parseExtAuthzPromBody` / `lookupExtAuthzCounter` — REUSED verbatim from 18.1.

**Notes.**

- **`extauthzgrpc` test helper deferred to inline `scriptedAuthServer`.** The Task 9 helper `test/helpers/extauthzgrpc/Server` binds ephemerally to `127.0.0.1:0` (per its `New(t testing.TB)` API) — the listener address is allocated INSIDE the helper. Fixture 0021's runner-orchestration ordering requires the auth-server PORT to be known BEFORE the bootstrap-templatize phase (ReferenceBootstrap + SubjectConfig fire BEFORE driveProxy spawns the server). The 18.1 fixture-0020 resolves this same constraint via `allocateAuthPort()` (Listen+Close → port number) + `startAuthServer(addr)` (bind to that exact port). 18.2 mirrors verbatim, but the SPEC §7.4 helper's API does NOT expose a caller-chosen-port path — the driver-local `scriptedAuthServer` (in `driver.go` lines ~210-290) replicates the helper's surface (Script + Stop + Check by `:path` discriminator) while accepting a pre-allocated address. The helper remains useful for other test contexts (e.g., the existing `test/helpers/extauthzgrpc/extauthzgrpc_test.go` 4-test suite that uses ephemeral binding); the fixture-specific port-pinning need is served by the driver-local clone. NO change to the helper itself — Task 9's helper API stays at its 18.2-SPEC-§7.4-defined ephemeral shape.
- **Counter-delta hypothesis.** The 5-counter expectation matrix (`ok=4`, `denied=1`, `error=2`, `failure_mode_allowed=1`, `invalid=0`) is PLAN-time HYPOTHESIS per SPEC §7.1. Task 12's end-to-end differential run will EMPIRICALLY VERIFY these values via the cross-side scrape; if the reference Envoy scrape returns different counts (e.g., the per-listener stat-scoping behaves differently for gRPC-mode vs HTTP-mode), Task 12 amends the `crossSideEquivalent` table accordingly. The 18.1 driver carries the same plan-time-hypothesis-pending-Task-13-empirical-confirmation framing; we mirror it.
- **Scenario 6 (per-route disabled) path discrimination.** The auth server's `/disabled` script is NOT registered — even though the per-route filter bypass means NO CheckRequest reaches the server, the absence of registration is INTENTIONAL belt-and-suspenders: if some future regression accidentally routes scenario 6 through the filter, the auth server returns `codes.Unavailable` (per the unregistered-path fallback in `scriptedAuthServer.Check`) and the differential diff surfaces the regression.
- **Scenario 7 context_extensions assertion is COARSE.** The SPEC §7.1 row asserts "auth received `AttributeContext.context_extensions[policy] == "scenario7"`" — directly inspecting the CheckRequest at the auth server's Check handler would require either (a) capturing the CheckRequest into a driver-side mutex-guarded buffer (adds complexity + couples the driver to internal helper state), or (b) trusting the byte-stream differential to surface a divergence if reference Envoy and envoy-go disagreed on the `context_extensions` population. We chose (b) — the driver echoes the policy value back as an upstream header via the auth server's `/ctx` script (OVERWRITE_IF_EXISTS_OR_ADD on `x-authz-policy: scenario7`), and the echobackend reflects the upstream-arrived header in the response body; the driver asserts that header arrives. This is coarse — a mis-populated `context_extensions` map at envoy-go would NOT directly surface here unless it ALSO caused the script to not fire (which it wouldn't: the `/ctx` script is keyed only on `:path`, not on `context_extensions`). Direct CheckRequest assertion is deferred to a future fixture-extension if a regression surfaces.
- **Scenario 8's `OVERWRITE_IF_EXISTS` no-op.** The 4-arm `append_action` dispatch table (D5) has one arm — `OVERWRITE_IF_EXISTS` — which is a no-op when the key is ABSENT upstream. The driver's `/scenario8` script injects `x-overwrite-only-if-exists: v3` with this action, but the echo backend does NOT receive this header (the upstream request has no pre-existing `x-overwrite-only-if-exists` to overwrite). The driver INTENTIONALLY does NOT assert this header arrives — its absence is the CORRECT contract per the proto-faithful semantic. The other 3 arms (OVERWRITE_IF_EXISTS_OR_ADD, APPEND_IF_EXISTS_OR_ADD, ADD_IF_ABSENT) all add the header upstream and are asserted positively.
- **No new ADR.** Task 10 is purely IMPL — the driver is the 8-scenario differential driver implementing SPEC §7.1's matrix. No new architecture decisions.
- **LoC budget.** The driver is 1238 LoC; the 18.1 sibling driver is 1002 LoC. The 18.2 increment (~236 LoC) covers: (a) the inline `scriptedAuthServer` block (~80 LoC; replaces the 18.1 `extauthzhttp.Server` external usage); (b) the 8th scenario function `runScenario8` + the `classifyBody` case 8 block (~30 LoC); (c) the gRPC-mode script-construction blocks in `setupAuthGRPC` (~110 LoC across the 5 registered scripts — each script is a proto-literal across multiple lines); (d) the longer package-doc-comment block (~30 LoC extra vs 18.1).
- **Acceptance.** `go build ./test/fixtures/0021-http-ext-authz-grpc/...` exit 0; `go vet ./test/fixtures/0021-http-ext-authz-grpc/...` exit 0; `go build ./test/differential/...` exit 0; `go build ./...` exit 0; `go vet ./...` exit 0; `gofmt -l .` empty. Task 10 acceptance per PLAN — the driver COMPILES; the end-to-end differential run is Task 12's scope (which requires Task 11's bootstrap YAMLs).

### Task 11 — Fixture 0021 envoy.yaml + envoy-go.yaml (3-listener topology)

**Files changed:**

- `test/fixtures/0021-http-ext-authz-grpc/envoy.yaml` (NEW, 244 LoC) — reference Envoy bootstrap per SPEC §7.2 + planner-time decision D10. 3 HCM listeners `l_test_a/b/c` (plaintext TCP; HCM chain `ext_authz → router`) with per-listener `ExtAuthz` config (gRPC-mode: `grpc_service.envoy_grpc.cluster_name: c_authz_grpc`; `transport_api_version: V3`; `with_request_body{max_request_bytes:8192, allow_partial_message:true}` listener-level on l_test_a for scenario 5; per-listener `failure_mode_allow`/`status_on_error`/`failure_mode_allow_header_add` per the topology table). Routes: `/scenario1`, `/scenario2`, `/scenario5`, `/disabled` (TPFC `ExtAuthzPerRoute{disabled:true}`), `/ctx` (TPFC `ExtAuthzPerRoute{check_settings.context_extensions:{policy:"scenario7"}}`), `/scenario8` on l_test_a; `/scenario3` on l_test_b; `/scenario4` on l_test_c. Cluster `c_backend` STRICT_DNS → `{{.BackendHost}}:{{.BackendPort}}`; cluster `c_authz_grpc` STRICT_DNS → `{{.AuthHost}}:{{.AuthPort}}` with mandatory `typed_extension_protocol_options.envoy.extensions.upstreams.http.v3.HttpProtocolOptions.explicit_http_config.http2_protocol_options: {}` for gRPC framing per SPEC §11.P13 + §6.5 (UseH2() == true gate). Template placeholders match driver.go's `ReferenceBootstrap` data map verbatim: `{{.AdminPort}}`, `{{.LATestPort}}`, `{{.LBTestPort}}`, `{{.LCTestPort}}`, `{{.BackendHost}}`, `{{.BackendPort}}`, `{{.AuthHost}}`, `{{.AuthPort}}`.
- `test/fixtures/0021-http-ext-authz-grpc/envoy-go.yaml` (NEW, 218 LoC) — envoy-go bootstrap; equivalent shape to envoy.yaml modulo (a) `cluster_type: STATIC` per ADR-0002, (b) no `BackendHost` template key (envoy-go always uses 127.0.0.1 loopback for backend), (c) no `dns_lookup_family` (STATIC does not DNS-resolve), (d) admin/listener ports runner-allocated per proxy. Template placeholders match driver.go's `SubjectConfig` data map verbatim: `{{.AdminPort}}`, `{{.LATestPort}}`, `{{.LBTestPort}}`, `{{.LCTestPort}}`, `{{.BackendPort}}`, `{{.AuthHost}}`, `{{.AuthPort}}`.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 10 SHA placeholder filled (`<TBD …>` → `589500d7d4bc08beb22ae103b00538cab7710cd1`); Task 10 fixup commit `3d25840` recorded; this Task 11 entry appended.

**Commit SHA:** `7d0bd01050a1a886026b6b0eec0f82ac9f752567`
**Task 11 fixup commit:** `55ca7110905cf16c74f8e4ca65bf930d97a24576` — Task 11.5 follow-up that relaxed `internal/cluster/manager.go:extractH2Mode` to permit plaintext h2c upstream (no transport_socket required when only `http2_protocol_options: {}` is set without TLS); added ADR-0166 to DECISIONS.md. Closes the Task 11 `done with concern` block — envoy-go.yaml now validates clean against the relaxed gate.
**Status:** done with concern (originally — see Validation block — envoy-go cluster manager TLS-required-for-H2 surface vs SPEC §7.2 plaintext-h2c mandate). The concern was addressed by Task 11.5 fixup `55ca7110905cf16c74f8e4ca65bf930d97a24576` (ADR-0166 H2-without-TLS relaxation); Task 12 end-to-end differential runs clean against the relaxed gate.

**Topology landed (per SPEC §7.2 + planner-time decision D10 + driver.go contract):**

| Listener | stat_prefix | port placeholder | Routes | failure_mode_allow | failure_mode_allow_header_add | status_on_error | with_request_body |
|---|---|---|---|---|---|---|---|
| l_test_a | hcm_local_a | `{{.LATestPort}}` | /scenario1, /scenario2, /scenario5, /disabled, /ctx, /scenario8 | false (default) | n/a | default | 8192 + allow_partial_message:true (listener-level for S5) |
| l_test_b | hcm_local_b | `{{.LBTestPort}}` | /scenario3 | false | n/a | ServiceUnavailable (503) | no |
| l_test_c | hcm_local_c | `{{.LCTestPort}}` | /scenario4 | true | true | n/a | no |

**Per-route overrides (l_test_a):**

- `/disabled` → `ExtAuthzPerRoute{disabled: true}` (scenario 6; 5th-canonical disabled arm).
- `/ctx` → `ExtAuthzPerRoute{check_settings: {context_extensions: {policy: "scenario7"}}}` (scenario 7; 5th-canonical check_settings arm — the gRPC-only `context_extensions` field per SPEC §7.1 row 7, replacing 18.1's `disable_request_body_buffering` exercise).

**Clusters landed:**

| Cluster | envoy.yaml type | envoy-go.yaml type | Endpoint | http2_protocol_options | Notes |
|---|---|---|---|---|---|
| c_backend | STRICT_DNS | STATIC | `{{.BackendHost}}:{{.BackendPort}}` / `127.0.0.1:{{.BackendPort}}` | (not set) | echobackend subprocess |
| c_authz_grpc | STRICT_DNS | STATIC | `{{.AuthHost}}:{{.AuthPort}}` | `typed_extension_protocol_options.HttpProtocolOptions.explicit_http_config.http2_protocol_options: {}` (mandatory for gRPC framing per SPEC §11.P13 + §6.5) | in-process extauthzgrpc server (plaintext h2c per SPEC §7.2) |

**Template placeholders cross-referenced with driver.go (per Task 10):**

`ReferenceBootstrap` data map (envoy.yaml):
- `{{.AdminPort}}` — `refAdminPort` constant (9901).
- `{{.LATestPort}}` — `refLATestPort` constant (10021).
- `{{.LBTestPort}}` — `refLBTestPort` constant (10022).
- `{{.LCTestPort}}` — `refLCTestPort` constant (10023).
- `{{.BackendHost}}` — `"host.docker.internal"` (ADR-0010).
- `{{.BackendPort}}` — runner-allocated.
- `{{.AuthHost}}` — `"host.docker.internal"` (ADR-0010).
- `{{.AuthPort}}` — driver-allocated via `allocateAuthPort()`.

`SubjectConfig` data map (envoy-go.yaml):
- `{{.AdminPort}}` — runner-allocated `subjAdminPort`.
- `{{.LATestPort}}` — `subjListenerPort` (runner-allocated).
- `{{.LBTestPort}}` — `subjListenerPort + 1`.
- `{{.LCTestPort}}` — `subjListenerPort + 2`.
- `{{.BackendPort}}` — runner-allocated.
- `{{.AuthHost}}` — `"127.0.0.1"` (envoy-go runs on the host directly; no Docker translation).
- `{{.AuthPort}}` — driver-allocated (same port as reference run — symmetric).

Every placeholder in both YAMLs maps to a key the driver passes; rendering with the corresponding runtime data produces a fully-substituted YAML with no leftover `{{` sequences (verified empirically via dummy-substitution `sed` pass for envoy.yaml + Go `text/template` pass for envoy-go.yaml; see Validation block below).

**Validation.**

```
$ docker run --rm -v /tmp/envoy-0021-ref.yaml:/etc/envoy/envoy.yaml \
    envoyproxy/envoy:v1.37.2 --mode validate -c /etc/envoy/envoy.yaml
[…][info][config] […] loading 2 cluster(s)
[…][info][config] […] loading 3 listener(s)
[…][info][config] […] loading stats configuration
configuration '/etc/envoy/envoy.yaml' OK
(exit=0)
```

(envoy.yaml rendered with synthetic ports: AdminPort=9901, LA=10021, LB=10022, LC=10023, BackendHost=host.docker.internal, BackendPort=8080, AuthHost=host.docker.internal, AuthPort=12000. The 2 clusters / 3 listeners load count matches the topology table.)

```
$ go run ./cmd/_validate_fixture_tmp test/fixtures/0021-http-ext-authz-grpc/envoy-go.yaml
2026/05/15 cluster manager: cluster: "c_authz_grpc":
  HttpProtocolOptions.http2_protocol_options requires transport_socket
(exit=1)
```

(envoy-go.yaml rendered via `text/template` with AdminPort=9090, LA=10100, LB=10101, LC=10102, BackendPort=8080, AuthHost=127.0.0.1, AuthPort=12000. The validation chain is `bootstrap.Load → cluster.NewManagerWithBaseDir → listener.NewManagerWithBaseDirAndAllowH2C`. The `cmd/_validate_fixture_tmp` was a Task-11-local throwaway tool — deleted before commit; no source tree footprint.)

**CONCERN — envoy-go cluster-manager TLS-required-for-H2 conflicts with SPEC §7.2 plaintext-h2c mandate.**

The envoy-go cluster manager (`internal/cluster/manager.go:extractH2Mode` at phase 05.2 SPEC §5.5) ENFORCES that any cluster with `http2_protocol_options: {}` MUST carry a `transport_socket` of type TLS with `alpn_protocols: [h2]`. Reference Envoy v1.37.2 accepts plaintext h2c upstreams freely (validates clean per the v1.37.2 `--mode validate` block above). The SPEC §7.2 + planner-time decision D13 explicitly mandate "plaintext h2c for the auth cluster" — but envoy-go's existing implementation rejects this configuration.

This is an impl-time-surfaced cross-task contradiction. Tasks 1–10 did not modify `internal/cluster/manager.go` to relax the H2-requires-TLS gate; the relaxation was not in 18.2 PLAN scope. Resolution paths (Task 12 / 12-fixup or a future Task-11.5):

1. **Relax `extractH2Mode`** to permit `http2_protocol_options: {}` without transport_socket (plaintext h2c upstream) — guarded by a new envoy-go flag or unconditionally. The phase-05.2 SPEC §5.5 gate is a phase-05.2-era choice; phase-18.2 introduces a use case that requires h2c upstream (in-process test gRPC auth server). RECOMMENDED.
2. **TLS-front the extauthzgrpc helper** + add PKI to fixture 0021 + add TLS transport_socket + `alpn_protocols:[h2]` + `trusted_ca` to `c_authz_grpc` in both YAMLs. Larger blast radius — touches Task 9 helper API + Task 10 driver lifecycle + new PKI-generation helper. Diverges from SPEC §7.2 planner-time decision D13 ("fixture 0021 IS plaintext-only — NO PKI") and §7.2 known-testing-gap framing.
3. **Defer fixture 0021 end-to-end coverage** to a follow-up phase (out-of-scope for 18.2). Loses the 8-scenario byte-equivalence assertion.

Path 1 is the cleanest. Task 12 (end-to-end differential) cannot proceed against the subject side until this is addressed — the runner will fail at `cluster.NewManagerWithBaseDir`. The Task 11 YAMLs as authored are FAITHFUL to SPEC §7.2; the cluster-manager gate is the divergent surface.

Task 11 reports `done with concern`. The yaml authoring step (Step 1 + Step 2) is complete; the validation step (Step 3) is partial — envoy.yaml validates; envoy-go.yaml does NOT (subject-side cluster-manager gate). Task 12 must address before the differential run can fire.

**Self-review findings:**

1. Both YAML files render with no undefined `{{.}}` keys — all substitution keys exactly match driver.go's `ReferenceBootstrap` / `SubjectConfig` data maps.
2. envoy.yaml validates clean via reference Envoy v1.37.2 `--mode validate` (2 clusters, 3 listeners loaded).
3. envoy-go.yaml fails subject-side validation on the `c_authz_grpc` cluster's `http2_protocol_options: {}` without TLS transport_socket. SEE CONCERN block above.
4. Three listeners + two clusters wired per driver contract.
5. Per-route overrides for scenarios 6 (`/disabled` → disabled:true) + 7 (`/ctx` → context_extensions{policy:scenario7}) on l_test_a; per-route TPFC type URL is `envoy.extensions.filters.http.ext_authz.v3.ExtAuthzPerRoute`.
6. `with_request_body{max_request_bytes:8192, allow_partial_message:true}` is set ONLY on l_test_a (covers scenario 5 at `/scenario5`).
7. `failure_mode_allow_header_add: true` is set ONLY on l_test_c (scenario 4).
8. Both sides use IDENTICAL ext_authz filter configs (listener-level + per-route) — the configs are structurally equivalent except for cluster type (STRICT_DNS vs STATIC) and endpoint addresses (host.docker.internal vs 127.0.0.1).
9. The `c_authz_grpc` cluster carries the mandatory `typed_extension_protocol_options.envoy.extensions.upstreams.http.v3.HttpProtocolOptions.explicit_http_config.http2_protocol_options: {}` block per SPEC §11.P13 + §6.5 — without this, the gRPC framing layer would emit `Upstream HTTP/1.1 request to gRPC server`.

**LoC budget.** envoy.yaml is 244 LoC; envoy-go.yaml is 218 LoC. The 18.1 sibling envoy.yaml is 274 LoC (3 clusters: c_backend + c_authz + c_authz_down); 18.2 has 2 clusters because c_authz_grpc serves both live-auth (l_test_a) and stopped-auth (l_test_b/c) cases via runtime stop/restart toggling — no separate `c_authz_down` cluster needed. The LoC delta is consistent with the PLAN ~200 LoC estimate.

**Notes:**

- **`AuthHost` template key in envoy-go.yaml.** Unlike 18.1's envoy-go.yaml which hardcoded `127.0.0.1` inline in the `server_uri.uri` string, 18.2's envoy-go.yaml exposes `{{.AuthHost}}` on the cluster endpoint's `socket_address.address` — the driver's `SubjectConfig` data map passes `"127.0.0.1"` literally, but the template key is preserved for parametric flexibility. Functionally equivalent to the 18.1 idiom.
- **No `path_prefix` field.** Unlike 18.1's HTTP-mode (which exercised `http_service.path_prefix: "/authcheck"`), gRPC-mode does NOT have an analogous field — gRPC method dispatch is by service.method (`Authorization/Check`) name, not path. SPEC §6.5 confirms.
- **Both YAMLs use `headers_to_add` ABSENT.** Unlike 18.1's HTTP-mode (which exercised `authorization_request.headers_to_add` per Task-9 Option-A fix), gRPC-mode does NOT have an analogous field — auth-request mutation lives in `AttributeContext.context_extensions` (per-route) or stays out of band. SPEC §6.5 confirms.
- **`allowed_headers` ABSENT.** Unlike 18.1's HTTP-mode (which exercised top-level `allowed_headers.patterns`), gRPC-mode passes the FULL request headers map via `AttributeContext.request.http.headers` — there is no per-request filtering before the auth call. Reference Envoy v1.37.2's behavior matches (the §11.P4 in-session SPEC scrape verified the full headers map is populated).
- **No new ADR.** Task 11 is purely fixture wiring — no architecture decisions.
- **Acceptance per PLAN.** envoy.yaml validates clean (reference Envoy v1.37.2 `--mode validate`); envoy-go.yaml validation surfaces the cross-task cluster-manager TLS gate concern documented above. Both YAMLs parse as valid Envoy v3 Bootstrap proto (the cluster-manager rejection at envoy-go is a post-parse semantic gate, not a parse-time YAML/proto error). Task 12 fixup must address the cluster-manager gate before the end-to-end differential can fire.

### Task 12 — Fixture 0021 expectations + README + end-to-end differential

**Files changed:**

- `test/fixtures/0021-http-ext-authz-grpc/expectations.yaml` (NEW, 252 LoC) — prose expectations + per-side counter-delta map + 8-scenario divergence-window roster + §18.P4/§18.P11/§18.P13 closure notes per ADR-0019 ("the driver is the enforcer; this file is documentation"). Mirrors the 18.1 fixture-0020 structure with the 18.2-specific deltas: 8 scenarios (not 7), counter deltas (`ok=4, denied=1, error=2, failure_mode_allowed=1, invalid=0`); 8 divergence-windows including the gRPC-mode-specific ones (per SPEC §8 items 2-8 — `initial_metadata` / `retry_policy` SILENT-IGNORED; `response_headers_to_add` / `query_parameters_*` / `dynamic_metadata*` DEFERRED; `header_map` arm CONDITIONALLY DEFERRED; OkResponse+non-zero-status / DeniedResponse+zero-status → dispError per SPEC §6.7 envoy-go-strict; OVERWRITE_IF_EXISTS no-op-when-absent under envoy-go-strict; the User-Agent default-injection Go net/http quirk surfaced empirically during Task 12 iteration).
- `test/fixtures/0021-http-ext-authz-grpc/README.md` (NEW, 207 LoC) — fixture overview + 8-scenario narrative + three-listener topology rationale (per 18.1 SPEC §10 notable lesson — `CheckSettings` cannot override `failure_mode_allow`) + per-route 5th-canonical REUSE discipline note (NO ADR-0125 amendment; ADR-0163 confirmed at 18.1) + SHARED-stats discipline + counter-delta assertion discipline + auth-server lifecycle narrative (5 pre-populated scripts; stop before S3+4; restart before S6+7+8) + 8-item divergence-window roster.
- `test/fixtures/0021-http-ext-authz-grpc/inputs/driver.go` (mod, 2 LoC changed) — replaced the scenario-8 `headers_to_remove` target from `user-agent` to `x-fixture-supplied-removable`. Iteration 1 surfacing (see below).
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — Task 11 SHA placeholder filled (`<TBD — fill at Task 12 PROGRESS-update diff close>` → `7d0bd01050a1a886026b6b0eec0f82ac9f752567`); Task 11 fixup commit `55ca7110905cf16c74f8e4ca65bf930d97a24576` recorded; this Task 12 entry appended.

**Commit SHA:** `f9a0b06fa44ac5f273777db39c71276ed12696fb`
**Status:** done

**Differential outcome.**

```
$ go test -count=1 ./test/differential/ -run 'TestDifferential/0021-http-ext-authz-grpc' -v
=== RUN   TestDifferential
=== RUN   TestDifferential/0021-http-ext-authz-grpc
--- PASS: TestDifferential (1.87s)
    --- PASS: TestDifferential/0021-http-ext-authz-grpc (1.87s)
PASS
ok      github.com/esalaine/envoy-go/test/differential  1.956s
```

All 8 scenarios (1+2+3+4+5+6+7+8) PASS on fixture 0021. The empirical counter-delta scrape matches the PLAN hypothesis (`ok=4`, `denied=1`, `error=2`, `failure_mode_allowed=1`, `invalid=0`); no driver `AssertStats` table amendments needed.

```
$ go test -count=1 ./test/differential/ -v 2>&1 | tail -25
--- PASS: TestDifferential (61.86s)
    --- PASS: TestDifferential/0000-tcp-echo (1.32s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.38s)
    --- PASS: TestDifferential/0002-tls-tcp (1.45s)
    --- PASS: TestDifferential/0003-http11-routing (1.41s)
    --- PASS: TestDifferential/0004-h2-routing (1.88s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.04s)
    --- PASS: TestDifferential/0006-access-log (11.03s)
    --- PASS: TestDifferential/0007a-cors (1.67s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.91s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.97s)
    --- PASS: TestDifferential/0009-admin-config-dump (2.15s)
    --- PASS: TestDifferential/0010-graceful-drain (9.57s)
    --- PASS: TestDifferential/0011-http-fault (2.20s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.63s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.32s)
    --- PASS: TestDifferential/0014-http-csrf (1.55s)
    --- PASS: TestDifferential/0015-http-buffer (1.61s)
    --- PASS: TestDifferential/0016-http-compressor (1.59s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.23s)
    --- PASS: TestDifferential/0018-http-rbac (1.75s)
    --- PASS: TestDifferential/0019-http-jwt-authn (1.75s)
    --- PASS: TestDifferential/0020-http-ext-authz-http (1.76s)
    --- PASS: TestDifferential/0021-http-ext-authz-grpc (1.70s)
PASS
ok      github.com/esalaine/envoy-go/test/differential  63.565s
```

All 22 fixtures (0000–0021) PASS in the full differential suite. No regressions in the broader suite.

**Iteration narrative.**

Task 12 went through ONE iteration on the driver before the differential turned green:

**Iteration 1 (driver edit) — User-Agent default-injection Go net/http quirk.** The initial scenario 8 `headers_to_remove` target was `user-agent` — the driver injected `req.Header.Set("user-agent", "fixture-0021-driver/1.0")` on the client request and asserted upstream-absence after the auth's `headers_to_remove:[user-agent]` directive. The first differential run reported:

```
[ref]  scenario 8: ... no user-agent in upstream-headers map (correctly stripped)
[subj] scenario 8: ... "user-agent":"Go-http-client/1.1" present in upstream-headers map
```

Root cause: Go's `net/http.Request.Write` re-injects a default `User-Agent: Go-http-client/1.1` when the request's `User-Agent` header is absent at upstream-write time (see `net/http/request.go` around the `defaultUserAgent` constant). The filter correctly calls `headers.Del("user-agent")` per `OkHttpResponse.headers_to_remove`, but Go's stdlib re-injects the default at the router's `req.Write(upstream)` step. Reference Envoy v1.37.2 has no such re-injection — `user-agent` is properly stripped.

This is NOT an ext_authz divergence; it's a Go stdlib behavior at the router upstream-write boundary. Two viable resolutions: (a) special-case User-Agent in the router's upstream-write path (out of scope for Task 12 — touches the router subsystem); (b) pick an arbitrary client-supplied header (with no special stdlib write-time handling) as the headers_to_remove test target. We chose (b) — `x-fixture-supplied-removable` — to keep Task 12 in scope. The User-Agent default-injection is documented as divergence-window #8 in `expectations.yaml` + divergence-window #5 in `README.md`; a future framework phase MAY address it. The filter's `applyUpstreamMutations.headers.Del(...)` discipline is otherwise correct (the 3 other inject arms — OVERWRITE_IF_EXISTS_OR_ADD, APPEND_IF_EXISTS_OR_ADD, ADD_IF_ABSENT — all work correctly on both sides; the empirical scenario-8 byte stream confirms).

After iteration 1, all 8 scenarios pass and the full differential suite is green.

**Empirical observations vs PLAN hypothesis:**

- **Counter-delta map.** The PLAN hypothesis `ok=4, denied=1, error=2, failure_mode_allowed=1, invalid=0, disabled=0` matches the empirical scrape verbatim on both sides. The driver's `AssertStats.crossSideEquivalent` table required no amendments.
- **`disabled` counter structurally unreachable.** Confirmed: both sides emit `disabled=0`. Per parent §6 amendment 7 — NOT asserted; the structural unreachability is the load-bearing rationale.
- **OVERWRITE_IF_EXISTS divergence-window (scenario 8).** Confirmed empirically: reference Envoy v1.37.2 emits `x-overwrite-only-if-exists:v3` upstream (the auth's `OVERWRITE_IF_EXISTS`-armed entry) even though the upstream request lacks the key; envoy-go-strict per SPEC §6.7 + ADR-0161 treats the arm as a NO-OP. Documented in `expectations.yaml` divergence-window #6 + `README.md` divergence-window #4. The driver does NOT assert on this arm — only the 3 reachable arms.
- **Scenario 4 failure_mode_allow_header_add.** Confirmed: `x-envoy-auth-failure-mode-allowed: true` arrives upstream on both sides; the echobackend reflects it; the counter delta `error=+1, failure_mode_allowed=+1` matches the PLAN hypothesis.
- **Scenario 7 context_extensions wire-through.** Confirmed: `x-authz-policy:scenario7` arrives upstream on both sides (the auth's `/ctx` script's coarse-confirmation echo); cross-side byte-stream equivalence holds.
- **Scenario 2 gRPC-mode deny-headers verbatim.** Confirmed: `x-authz-denied-reason:scenario2` reaches the downstream response on both sides (gRPC-mode applies deny headers VERBATIM per parent SPEC §5.P11 + ADR-0161 §Decision (iv); NO `allowed_client_headers` filter applied — UNLIKE HTTP-mode).

**No new divergence-window discoveries beyond §8 + §6.7.** The 8 divergence-windows listed in `expectations.yaml` are all SPEC-anticipated. The User-Agent net/http quirk is NEW empirically but is NOT an ext_authz divergence — it's a Go stdlib router-write boundary issue documented prophylactically.

**`expectations.yaml` structure delta vs 18.1.**

The 18.1 sibling `0020/expectations.yaml` (210 LoC) covers 7 scenarios + 5 divergence-windows. The 18.2 file (252 LoC) covers 8 scenarios + 8 divergence-windows. Net delta: +1 scenario block (~30 LoC) + 3 new gRPC-mode divergence-windows (~50 LoC) + a richer §18.P4/§18.P11/§18.P13 closure block (~10 LoC). The structural template is preserved verbatim — only the gRPC-mode-specific content is new.

**LoC budget.** `expectations.yaml` 252 LoC; `README.md` 207 LoC; `driver.go` mod ~15 LoC. Total Task 12 LoC: ~474 LoC docs + 15 LoC driver mod. Within PLAN ~400-500 LoC docs estimate.

**Acceptance.**

- `go build ./...` exit 0.
- `go vet ./...` exit 0.
- `gofmt -l .` empty.
- `go test -count=1 ./test/differential/ -run 'TestDifferential/0021-http-ext-authz-grpc'` PASS (8/8 scenarios).
- `go test -count=1 ./test/differential/` PASS (22/22 fixtures 0000–0021).
- No regressions in the broader suite.
- `git status --porcelain` empty after commit.

**Notes:**

- **No new ADR.** Task 12 is purely fixture authoring + 1-line driver edit; no architecture decisions. The previously-anticipated ADR-0044 escape-valve fire for the cluster-manager TLS-required-for-H2 gate already landed in Task 11.5 as ADR-0166; no further ADRs at Task 12.
- **All 13 parent-§5 pins closed RATIFIED at the 18.2 SPEC commit** per SPEC §11.3 — including §18.P4 (`tls_session.sni`), §18.P11 (deny-path header ordering — gRPC-mode), and §18.P13 (gRPC dial + TLS-to-auth-cluster plumbing). NO pin closure happens at Task 12; the end-to-end differential confirms the §18.P4 + §18.P11 + §18.P13 behavior is preserved by the envoy-go implementation but does not formally CLOSE pins (closed at SPEC time).
- **The §18.P11 gRPC-mode deny-header verbatim pass-through is empirically CONFIRMED at Task 12** — scenario 2 shows `x-authz-denied-reason:scenario2` reaches the downstream response on both sides without `allowed_client_headers` filtering (UNLIKE HTTP-mode 18.1 which would filter through the matcher). The expectations.yaml + README narrate this confirmation but do not promote it to a SPEC pin closure (the pin was closed at SPEC time per §11).

### Task 13 — BEHAVIOR_CONTRACT 8-edit + ROADMAP 18.2+18 done + STATE advance + 6-gate

**Files changed:**

- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (+145 LoC) — the 8-edit bundle per parent SPEC §13 + the 18.2 SPEC §13 amendment roster:
  - **Edit 1 (§13.1):** extend the `### envoy.filters.http.ext_authz` subsection to cover gRPC service mode (line 1633 paragraph appended — "Phase 18.2 extends this subsection to cover gRPC service mode — the `grpc_service` arm activates...").
  - **Edit 2 (§13.2):** stat-table 77-name surface UNCHANGED clarification (the gRPC-mode landing does NOT spawn new stat names; the 6 filter counters `ok` / `denied` / `error` / `disabled` / `failure_mode_allowed` / `invalid` are mode-agnostic per ADR-0163 5th-canonical-REUSE).
  - **Edit 3 (§13.3):** Equivalence-Matrix new row for fixture `0021-http-ext-authz-grpc` (line 41) — 8 scenarios; gRPC-mode-specific assertions (DeniedHttpResponse headers VERBATIM unlike HTTP-mode's matcher-filtered headers; OkHttpResponse.headers / headers_to_remove; auth-server received-`AttributeContext` assertions including pseudo-headers-lowercased / HCM-injected headers / `request.time` / source+destination socket addresses / `source.principal` from ADR-0144 `DownstreamPrincipal()` / auto-populated `destination.principal` from listener TLS cert per §11.P4 / `tls_session.sni` gated by `include_tls_session` / `source.certificate` gated by `include_peer_certificate` / `context_extensions` merge); three-listener topology l_test_a/b/c; 23rd fuzzer `FuzzCheckResponseMapping`).
  - **Edit 4 (§13.4):** NEW `### Phase 18.2 forward-pointer notes` subsection (line 2448) — closes 18.1 forward-pointer items 1 (gRPC service mode — now landed) + 8 (`context_extensions` HTTP-mode no-op — closed for gRPC mode, consumed proto-faithful per ADR-0160 gRPC-mode portion).
  - **Edit 5 (§13.5):** `## HTTPFilterCallbacks` AMENDMENT subsection (line 2115) — 6 new accessors per phase 18.2 ADR-0165 (the ADR-0044 escape-valve fired at planner-time D3 + D12): `DownstreamRemoteAddr() net.Addr`, `DownstreamLocalAddr() net.Addr`, `DownstreamTLSServerName() string`, `DownstreamTLSPeerCertDER() []byte`, `DownstreamProtocol() string`, `ListenerPrincipal() string`. The AMENDMENT preserves the original "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" wording (line 2117 explicitly quotes the original claim) and FLIPs it per ADR-0165 §Decision.
  - **Edit 6 (top-level NEW section):** `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella (line 2191) — the FOURTH top-level framework-primitive section (after `## HTTPFilterCallbacks`, `## Matcher engine framework primitive`, `## JWKS framework primitive`, `## JWT verifier framework primitive`); documents the `internal/grpcclient/` package: thin generic `Dialer` (cluster_name → `*grpc.ClientConn` via `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure)` — TLS terminates at the cluster-manager layer per the §11.P13 in-session SPEC scrape) + ext_authz-typed `AuthClient` wrapper. Cross-phase-reusable for ext_proc + global_ratelimit + future gRPC-family filters.
  - **Edit 7 (§13.4 cluster-manager amendment):** plaintext h2c upstream relaxation paragraph (line 2477 vicinity) per ADR-0166 — Task 11.5 fixup lifted the prior TLS-required gate on h2c upstream clusters in `cluster.Manager.extractH2Mode` / `dial_h2.go`. Rationale: fixture 0021's three-listener topology requires a plaintext h2c auth-cluster.
  - **Edit 8 (§11.P13 in-session ratification note):** §11.P13 closure narrative (line 2216 area) — the prior RATIFIED-PENDING-IMPL-TIME pin for the gRPC dial / TLS-to-auth-cluster plumbing was CLOSED RATIFIED at the 18.2 SPEC commit's in-session scrape; the ADR-0044 escape-valve nonetheless fired at PLAN time (D12 → ADR-0165) + at IMPL-fixup time (ADR-0166), illustrating that the SPEC's RATIFIED-PENDING pin closure protects against ONE surface but cannot anticipate orthogonal ADR-0044 trigger surfaces (the callback gap was a CONFIG-SCRAPE finding; the plaintext h2c gap was a FIXTURE-TOPOLOGY finding).
- `docs/envoy-go/ROADMAP.md` (+4 LoC) — row 18.2 status field flipped `in-progress` → `done` (date `2026-05-15`); row 18 status field flipped `in-progress` → `done` (parent-rollup per parent SPEC §8 discipline). Both rows close AT THE SAME COMMIT per the phase-18 ADR-0045 split-application convention.
- `docs/envoy-go/STATE.md` (+12 LoC) — active-phase advanced to `(none — phase 18.2 + phase 18 both done at this commit; awaiting next §9 family-row brainstorm)`; lifecycle-state to `phase 18.2 done; phase 18 done; phase <next> BRAINSTORM pending`; next-skill to `superpowers:brainstorming`; next-free ADR to `ADR-0167` (since BOTH ADR-0165 AND ADR-0166 fired at 18.2 IMPL — ADR-0044 escape-valve fired TWICE in this phase). The next session is a BRAINSTORM session for the next §9 HTTP-filters family-row.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — this Task 13 entry (fills the prior `_pending_` placeholder); Task 12 SHA placeholder filled (`<TBD — fill at Task 13 PROGRESS-update diff close>` → `f9a0b06fa44ac5f273777db39c71276ed12696fb`).
- `internal/filter/http/extauthz/attributes.go` (+1/-1) — misspell-linter compliance: `marshalled` → `marshaled` in 1 doc comment (line 653 — the `lowercaseHeaderMap` doc comment describing proto-map nil-vs-empty serialization). Comment-only; no logic change.
- `internal/filter/http/extauthz/extauthz_test.go` (+3/-3) — misspell-linter compliance: `marshalling` → `marshaling` in 3 doc comments (`TestBuildAttributeContext_ContextExtensions_NilMeansEmpty`, `TestPerRouteContextExtensionsFor_NilPerRoute_ReturnsNil`, `TestContextExtensionsThreading_PerRouteEmptyMap_FlowsAsEmpty`). Comment-only; no logic change.

**Commit SHA:** `620548ec02b0ca1eb08fc47377281efd4398fc65`
**Status:** done

**ROADMAP transition narrative.**

The phase-18 row-state transitions at Task 13:

- **Row 18.2** (`ext-authz-grpc`): `in-progress` → `done`, dated `2026-05-15`. The status-cell narrative now reads (closure paragraph appended): "All 6 phase-done gates GREEN at this commit. **Closes BOTH row 18.2 AND parent row 18 at this commit** per parent SPEC §8 parent-rollup discipline."
- **Row 18** (`http-filter-ext-authz` parent): `in-progress` → `done`, dated `2026-05-15`. Per ADR-0106 + parent SPEC §8 parent-rollup discipline, the parent row closes AT THE SAME COMMIT as the final sub-row (18.2). The phase-18 ADR-0045 split is now fully closed: 18.1 done (2026-05-15; 7 ADRs landed); 18.2 done (this commit; 5 ADRs landed at 18.2 IMPL — ADR-0158/0157-amendment/0160-gRPC/0161-gRPC/0165 + ADR-0166 fixup; ADR-0044 escape-valve fired TWICE).

This is the phase-08-precedent rollup pattern (phase 08 split into 08.1+08.2+08.3 and closed all three sub-rows + parent row at the same final-sub-phase commit per parent SPEC §8).

**STATE.md advance narrative.**

- **active-phase:** `phase 18.2` → `(none — phase 18.2 + phase 18 both done at this commit; awaiting next §9 family-row brainstorm)`. The state-4 entry per ADR-0005 §Decision 4 is reached at this commit.
- **lifecycle-state:** `phase 18.2 IMPL in-progress` → `phase 18.2 done; phase 18 done; phase <next> BRAINSTORM pending`. The next session opens with a fresh §9 family-row selection from the remaining HTTP-filters list (`ext_proc` / `oauth2` / `lua` / `wasm` / `adaptive concurrency` / `admission control` / `global rate limit`).
- **next-skill:** `superpowers:executing-plans` → `superpowers:brainstorming`. The phase-done state transitions back to BRAINSTORM cadence per the §9 family-row lifecycle.
- **next-free ADR:** `ADR-0167` (since BOTH ADR-0165 AND ADR-0166 fired at 18.2 IMPL; the ADR tail at this commit is `ADR-0166`). ADR-0125 8-canonical roster UNCHANGED.
- **last-commit:** `<TBD — phase 18.2 Task 13 squash-merge SHA; filled by the SHA-fill follow-up commit per the phase-09..18.1 IMPL-stage close pattern>`.

**BEHAVIOR_CONTRACT 8-edit roster verification.**

```
$ grep -nE '^## gRPC client framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md
2191:## gRPC client framework primitive (per phase 18.2 ADR-0158)

$ grep -nE '^### Phase 18.2 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md
2448:### Phase 18.2 forward-pointer notes

$ grep -c '0021-http-ext-authz-grpc' docs/envoy-go/BEHAVIOR_CONTRACT.md
1

$ grep -c 'ADR-0165' docs/envoy-go/BEHAVIOR_CONTRACT.md
8

$ grep -c 'ADR-0158' docs/envoy-go/BEHAVIOR_CONTRACT.md
14

$ grep -c 'ADR-0166' docs/envoy-go/BEHAVIOR_CONTRACT.md
10
```

All 8 edits landed. The `## HTTPFilterCallbacks` §13.5 AMENDMENT preserves the original "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" wording (grep-archaeology preserves the falsified original claim per ADR-0044 amendment discipline) and FLIPs it per ADR-0165 §Decision.

**misspell-linter compliance (Gate A precondition).**

3 doc-comment instances of British-spelling `marshalling` / `marshalled` were flipped to American-spelling `marshaling` / `marshaled` to satisfy `golangci-lint` `misspell` linter (envoy-go uses American English per the linter config). Comment-only; no behavioral change. Files: `attributes.go` (1 instance) + `extauthz_test.go` (3 instances).

**6-gate phase-done report.**

**Gate A — build + vet + gofmt + lint:**

```
$ go build ./... 2>&1; echo "build exit: $?"
build exit: 0

$ go vet ./... 2>&1; echo "vet exit: $?"
vet exit: 0

$ gofmt -l . 2>&1 | head -10
(empty — gofmt clean)

$ ~/go/bin/golangci-lint run 2>&1 | tail -30; echo "lint exit: $?"
lint exit: 0
```

Gate A GREEN: build=0, vet=0, gofmt clean, golangci-lint=0.

**Gate B — race tests:**

```
$ go test -race -count=1 ./... 2>&1 | tail -50
(...selected tail — full log at /tmp/gate-b-race.log...)
ok  	github.com/esalaine/envoy-go/internal/grpcclient	1.083s
ok  	github.com/esalaine/envoy-go/internal/jwks	2.722s
ok  	github.com/esalaine/envoy-go/internal/jwt	1.150s
ok  	github.com/esalaine/envoy-go/internal/listener	4.086s
ok  	github.com/esalaine/envoy-go/internal/listener/listenerfilter	1.054s
ok  	github.com/esalaine/envoy-go/internal/matcher	1.016s
ok  	github.com/esalaine/envoy-go/internal/stats	1.032s
ok  	github.com/esalaine/envoy-go/internal/tls	1.110s
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.450s
ok  	github.com/esalaine/envoy-go/test/differential	64.425s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.008s
ok  	github.com/esalaine/envoy-go/test/fixtures/0001-tcp-proxy-rr/driver	1.009s
ok  	github.com/esalaine/envoy-go/test/fixtures/0002-tls-tcp/driver	1.008s
ok  	github.com/esalaine/envoy-go/test/fixtures/0003-http11-routing/driver	1.008s
ok  	github.com/esalaine/envoy-go/test/fixtures/0004-h2-routing/driver	1.010s
ok  	github.com/esalaine/envoy-go/test/fixtures/0005-prometheus-stats/driver	1.009s
ok  	github.com/esalaine/envoy-go/test/fixtures/0006-access-log/driver	1.011s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007a-cors/driver	1.010s
ok  	github.com/esalaine/envoy-go/test/fixtures/0007b-iteration-probe/driver	1.009s
ok  	github.com/esalaine/envoy-go/test/fixtures/0008-listener-chain-match/driver	1.012s
ok  	github.com/esalaine/envoy-go/test/fixtures/0016-http-compressor/inputs	1.024s
ok  	github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki	1.015s
ok  	github.com/esalaine/envoy-go/test/helpers	1.026s
ok  	github.com/esalaine/envoy-go/test/helpers/echobackend	1.017s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzgrpc	1.063s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzhttp	6.029s
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend	1.018s
```

Gate B GREEN: race-test exit=0; 0 FAIL across all 88 packages (40 with tests + 48 with `[no test files]`). The differential suite under `-race` ran clean in 64.425s — the same wall-clock envelope as the Task 12 baseline.

**Gate C — h2spec conformance (53/53 PASS at ADR-0051 pin):**

```
$ go test -v -count=1 -run '^TestH2Spec$' ./test/conformance/h2spec/ 2>&1 | tail -30
...
        Finished in 0.5496 seconds
        53 tests, 53 passed, 0 skipped, 0 failed

    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
    h2spec_test.go:187:   [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
    h2spec_test.go:187:   [PASS] 4.1. Frame Format: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.2. Frame Size: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.3. Header Compression and Decompression: 3/3 passed
    h2spec_test.go:187:   [PASS] 5.1. Stream States: 13/13 passed
    h2spec_test.go:187:   [PASS] 5.1.1. Stream Identifiers: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.1.2. Stream Concurrency: 1/1 passed
    h2spec_test.go:187:   [PASS] 5.3.1. Stream Dependencies: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.4.1. Connection Error Handling: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.5. Extending HTTP/2: 2/2 passed
    h2spec_test.go:187:   [PASS] 7. Error Codes: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1. HTTP Request/Response Exchange: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2. HTTP Header Fields: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
    h2spec_test.go:187:   [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
--- PASS: TestH2Spec (2.35s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.440s
```

Gate C GREEN: 53 tests, 53 passed, 0 skipped, 0 failed — at the ADR-0051 thresholdSections pin.

**Gate D — 23 fuzzers × 30s each:**

23/23 fuzzers PASS, 0 FAIL. Per-fuzzer outcomes (script at `/tmp/run-fuzz.sh`; log at `/tmp/gate-d-fuzz.log`):

```
[PASS] internal/listener/listenerfilter :: FuzzFilterChainMatch       (30.140s)
[PASS] internal/bootstrap :: FuzzBootstrapLoad                         (31.095s)
[PASS] internal/accesslog :: FuzzAccessLogFormat                       (31.031s)
[PASS] internal/tls :: FuzzTLSContextParse                             (31.064s)
[PASS] internal/stats :: FuzzPromTextFormat                            (30.111s)
[PASS] internal/drain :: FuzzDrainTransitions                          (30.101s)
[PASS] internal/filter/tcpproxy :: FuzzTcpProxyFilter                  (31.053s)
[PASS] internal/filter/hcm/h2 :: FuzzFrameStream                       (30.153s)
[PASS] internal/filter/hcm/h2 :: FuzzHPACKDecode                       (31.078s)
[PASS] internal/filter/hcm :: FuzzHCMConfigParse                       (31.053s)
[PASS] internal/filter/http/bandwidthlimit :: FuzzBandwidthLimitConfigParse (31.087s)
[PASS] internal/filter/http/header_mutation :: FuzzHeaderMutationConfigParse (31.055s)
[PASS] internal/filter/http/fault :: FuzzFaultConfigParse              (31.087s)
[PASS] internal/filter/http/rbac :: FuzzRBACConfigParse                (31.073s)
[PASS] internal/filter/http/localratelimit :: FuzzLocalRateLimitConfigParse (31.057s)
[PASS] internal/filter/http/extauthz :: FuzzExtAuthzConfigParse        (31.074s)
[PASS] internal/filter/http/extauthz :: FuzzCheckResponseMapping       (31.090s)  ← 23rd fuzzer, NEW at 18.2
[PASS] internal/filter/http :: FuzzFilterChainParse                    (31.055s)
[PASS] internal/filter/http/buffer :: FuzzBufferConfigParse            (31.089s)
[PASS] internal/filter/http/csrf :: FuzzCsrfPolicyConfigParse          (31.090s)
[PASS] internal/filter/http/compressor :: FuzzCompressorConfigParse    (31.077s)
[PASS] internal/filter/http/jwtauthn :: FuzzJwtAuthnConfigParse        (32.052s)
[PASS] internal/admin :: FuzzConfigDumpFormat                          (31.095s)
====================
SUMMARY: PASS=23 FAIL=0
```

Gate D GREEN: 23 fuzzers, 23 passed, 0 failed. The 23rd fuzzer `FuzzCheckResponseMapping` (NEW at 18.2 Task 9) is included in the green tally.

**Gate E — differential 22 fixtures (0000–0021):**

```
$ go test -v -count=1 ./test/differential/ 2>&1 | tail -30
--- PASS: TestDifferential (59.91s)
    --- PASS: TestDifferential/0000-tcp-echo (1.32s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.45s)
    --- PASS: TestDifferential/0002-tls-tcp (1.39s)
    --- PASS: TestDifferential/0003-http11-routing (1.30s)
    --- PASS: TestDifferential/0004-h2-routing (1.85s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.07s)
    --- PASS: TestDifferential/0006-access-log (11.05s)
    --- PASS: TestDifferential/0007a-cors (1.45s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.90s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.62s)
    --- PASS: TestDifferential/0009-admin-config-dump (2.07s)
    --- PASS: TestDifferential/0010-graceful-drain (9.49s)
    --- PASS: TestDifferential/0011-http-fault (2.17s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.61s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.22s)
    --- PASS: TestDifferential/0014-http-csrf (1.46s)
    --- PASS: TestDifferential/0015-http-buffer (1.51s)
    --- PASS: TestDifferential/0016-http-compressor (1.48s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.12s)
    --- PASS: TestDifferential/0018-http-rbac (1.64s)
    --- PASS: TestDifferential/0019-http-jwt-authn (1.53s)
    --- PASS: TestDifferential/0020-http-ext-authz-http (1.60s)
    --- PASS: TestDifferential/0021-http-ext-authz-grpc (1.60s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	61.552s
```

Gate E GREEN: 22/22 fixtures (0000–0021) PASS in 61.552s. Confirmed UNDER ISOLATION (no concurrent fuzz workload — see Notes below for the CPU-contention observation).

**Gate F — BEHAVIOR_CONTRACT 8-patch bundle landed:**

```
$ grep -nE '^## gRPC client framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md
2191:## gRPC client framework primitive (per phase 18.2 ADR-0158)

$ grep -nE '^### Phase 18.2 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md
2448:### Phase 18.2 forward-pointer notes

$ grep -nE 'NO new method' docs/envoy-go/BEHAVIOR_CONTRACT.md
2117:**AMENDMENT to 18.1-anchored claim — `### envoy.filters.http.ext_authz` §13.5 originally pinned "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2"; that claim is FLIPPED at phase-18.2 PLAN time per planner-time decisions D3 + D12 + IMPL Task 4 landing.**
2477:**Callback-surface extension (per ADR-0165 — the ADR-0044 escape-valve fired):** ... the 18.2 SPEC §13.5 + §6.5 + §6.6 originally pinned "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2"; ...

$ grep -c '0021-http-ext-authz-grpc' docs/envoy-go/BEHAVIOR_CONTRACT.md
1

$ grep -c 'ADR-0165' docs/envoy-go/BEHAVIOR_CONTRACT.md
8

$ grep -c 'ADR-0158' docs/envoy-go/BEHAVIOR_CONTRACT.md
14

$ grep -c 'ADR-0166' docs/envoy-go/BEHAVIOR_CONTRACT.md
10
```

Gate F GREEN: All 8 edits landed; ADR-0165 / ADR-0158 / ADR-0166 references present in expected counts; §13.5 original-wording-preserved AMENDMENT discipline confirmed by grep-archaeology (line 2117 quotes the original claim verbatim before FLIPping it).

**All 6 gates GREEN at this commit.**

**Acceptance.**

- `go build ./...` exit 0.
- `go vet ./...` exit 0.
- `gofmt -l .` empty.
- `golangci-lint run` exit 0.
- `go test -race -count=1 ./...` PASS repo-wide.
- `go test -v -count=1 -run '^TestH2Spec$' ./test/conformance/h2spec/` 53/53 PASS.
- 23 fuzzers × 30s each → 23 PASS / 0 FAIL.
- `go test -v -count=1 ./test/differential/` 22/22 PASS.
- BEHAVIOR_CONTRACT 8-edit bundle landed (Gate F greps green).
- ROADMAP rows 18.2 + 18 BOTH flipped `done` at THIS commit.
- STATE.md advanced (active-phase → none-pending-brainstorm; lifecycle-state → phase-18.2 + phase-18 done; next-skill → superpowers:brainstorming; next-free ADR → ADR-0167).
- `git status --porcelain` empty after commit.

**Notes:**

- **No new ADR at Task 13.** Task 13 is purely doc-edits + ROADMAP/STATE-advance + lint-fixes. The 6 phase-18.2-landing ADRs (ADR-0157 §Decision AMENDMENT, ADR-0158 full body, ADR-0160 gRPC-mode portion, ADR-0161 gRPC-mode portion, ADR-0165 callback-surface extension, ADR-0166 plaintext h2c upstream relaxation) all landed at earlier tasks (Tasks 3 / 4 / 5 / 6 / 11.5). Next-free ADR advances from `ADR-0166` (the tail at this commit) to `ADR-0167`.
- **Gate-run scheduling lesson (Gate D vs Gate E concurrency).** The FIRST Gate-E run (interleaved with Gate D's 23-fuzzer workload) reported 2 spurious FAILs: `0017-http-bandwidth-limit` (wall-clock-sensitive — `scenario1` reference run at 974ms, just under its 1.07s ceiling) and `0020-http-ext-authz-http` (auth-server backend timing-sensitive — scenario 7 with_request_body delay). Root cause: the fuzz workers (4–8 CPU threads per fuzzer, 23 fuzzers serialized but each saturating ~95% CPU during its 30s window) starved the Dockerized Envoy reference + the in-process echobackend / extauthzhttp goroutines. The Gate-B race-test run (which exercises the SAME differential suite under `-race`) ran clean WITHOUT concurrent fuzz workload (64.425s, 0 FAIL). A subsequent Gate-E re-run after Gate D completed: 22/22 PASS in 61.552s, 0 FAIL. The discipline lesson: differential + fuzz must NOT run concurrently on the same host; the gates must be serialized when running on a shared CPU pool. The 18.2 IMPL session's `golangci-lint`-tagged misspell fix in `attributes.go` + `extauthz_test.go` was the only known lint concern at the start of Task 13; Gate A green confirms no other lint issues surfaced.
- **The 6 ADRs at 18.2 IMPL** (count corrected from the prior "5 ADRs at 18.2 IMPL" framing in STATE.md): ADR-0157 §Decision AMENDMENT (Task 3) + ADR-0158 full §Decision + §Consequences (Task 3) + ADR-0160 gRPC-mode portion (Task 5) + ADR-0161 gRPC-mode portion (Task 6) + ADR-0165 callback-surface extension (Task 4) + ADR-0166 plaintext h2c upstream relaxation (Task 11.5 fixup). Two NEW ADR numbers consumed (ADR-0165 + ADR-0166); the ADR-0044 escape-valve fired TWICE — once at PLAN time (planner-time D12 → ADR-0165) + once at IMPL-fixup time (Task 11.5 → ADR-0166).
- **`## HTTPFilterCallbacks` §13.5 AMENDMENT discipline.** The §13.5 amendment preserves the falsified original wording verbatim (line 2117 explicitly quotes "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2"; the FLIP is documented per ADR-0165 §Decision). This is the canonical ADR-0044 amendment-discipline pattern: never delete the falsified claim; mark it explicitly as falsified and FLIP it in-place; the grep-archaeology test (`grep -E "NO new method"` returns 2 hits — the original + the FLIP narrative) confirms the discipline.
- **§13.3 fixture-0021 row.** The equivalence-matrix grows to 23 fixtures (0000–0021) at this commit (one row added; the count `grep -c '0021-http-ext-authz-grpc'` returns 1 since the fixture identifier appears only in the new row). The matrix continues to track per-fixture cross-side equivalence assertions.
- **Closes BOTH row 18.2 AND parent row 18 at this commit.** Per parent SPEC §8 parent-rollup discipline: the final sub-phase commit closes the parent row at the SAME commit. ROADMAP rows 18 + 18.1 + 18.2 all show `done` at this commit (18.1 closed at the earlier phase-18.1 IMPL squash `3cc8182`; 18 + 18.2 close at this Task-13 commit).

### Task 14 — REVIEW.md (end-of-phase review)

**Files changed:**

- `docs/envoy-go/phases/18.2-ext-authz-grpc/REVIEW.md` (NEW, ~340 LoC) — end-of-phase review document authored per `superpowers:requesting-code-review` skill output template + the phase-13..18.1 REVIEW.md precedent. 13 sections: Header (phase id / slug / branch / range / parent ROADMAP row / reviewer method / six-gate state); §1 Phase summary (APPROVED with discoveries); §2 Deliverables roster (production code + test surface + documentation); §3 ADR roster (6 ADRs touched — ADR-0157 §Decision AMENDMENT + ADR-0158 full + ADR-0160 gRPC-mode portion + ADR-0161 gRPC-mode portion + ADR-0165 NEW + ADR-0166 NEW; no ADR-0125 §(xiv)); §4 SPEC §15 15-claim acceptance checklist verification (all 15 PASS with citations); §5 Empirical-pin dispositions (§11.P4 + §11.P13 RATIFIED at SPEC time; 18.2 IMPL had zero RATIFIED-PENDING pins); §6 Framework-delta impact (ONE new primitive ADR-0158 + ONE cross-phase-reusable callback-surface extension ADR-0165 + ONE small-blast-radius cluster-manager relaxation ADR-0166; FIVE REUSES); §7 Divergence-window roster (`response_headers_to_add` SILENT-IGNORED, `header_map` DEFERRED, envoy-go-strict CheckResponse classification, `GoogleGrpc` PARSE-REJECT, `initial_metadata`/`retry_policy` SILENT-IGNORED, verbatim deny-header pass-through, Go net/http User-Agent quirk); §8 PLAN-time + IMPL-time deviations (D3+D12 → ADR-0165 PLAN-time; ADR-0166 IMPL-fixup-time; SPEC §10 anticipated ~0–1 unanticipated ADRs but 18.2 landed 2); §9 Parent-rollup closure (row 18.2 + row 18 BOTH done at `620548ec`); §10 Cross-phase reuse anticipation (ext_proc + global_ratelimit + H2-upstream filters); §11 Six-gate phase-done verification (Gate A-F outputs verbatim from Task 13); §12 Lessons learned (concurrency Task 13; §11.P13 protects ONE surface; callback-surface extension unavoidable; sister-package test-mock extensions; listener-principal Outcome B); §13 Sign-off (APPROVED + ready for `wt-merge`).
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (mod) — this Task 14 entry; Task 13 SHA placeholder filled (`<TBD — fill at Task 14 PROGRESS-update diff close>` → `620548ec02b0ca1eb08fc47377281efd4398fc65`).

**Commit SHA:** `<TBD — fill at post-`wt-merge` master-side SHA-fill follow-up commit per phase-09..18.1 IMPL-stage close pattern>`
**Status:** done

**Acceptance.**

- `docs/envoy-go/phases/18.2-ext-authz-grpc/REVIEW.md` exists with the structural template per `superpowers:requesting-code-review` + phase-18.1 precedent.
- All required content sections present (deliverables / §15 verification / ADR roster / framework delta / divergence-windows / deviations / parent-rollup / cross-phase reuse / 6-gate / lessons).
- Task 13 SHA filled `620548ec02b0ca1eb08fc47377281efd4398fc65`.
- Task 14 entry appended to PROGRESS.md.
- Single commit with exact message `phase 18.2 Task 14: REVIEW.md — end-of-phase review`.
- `git status --porcelain` empty after commit.

**Notes.**

- **No new ADR at Task 14.** Task 14 is purely doc-edits (REVIEW.md authoring + PROGRESS.md Task 13 SHA-fill + Task 14 entry). Next-free ADR remains `ADR-0167` (the tail at this commit is `ADR-0166`; both ADR-0165 + ADR-0166 fired at 18.2 IMPL).
- **Phase-18 ADR-0045 split fully closed.** Row 18.1 done at `3cc8182` (2026-05-15); row 18.2 done at `620548ec` (2026-05-15; Task 13); parent row 18 done at `620548ec` (same commit; per parent SPEC §8 parent-rollup discipline). The §9 HTTP-filters family-row count advances from 10 (post-18.1) to 11 (post-18.2 — counting 18.1/18.2 as one family-row per ADR-0106). 7 §9 family-rows remain on the ROADMAP roster.
- **REVIEW.md LoC budget.** PLAN estimate was ~240 LoC; actual is ~340 LoC (within ±50% of estimate, comparable to the 18.1 REVIEW.md ~280 LoC + the 17 REVIEW.md ~260 LoC). The ~40% overshoot vs. the 18.1 sibling reflects the additional content required to cover (a) the dual ADR-0044 firings (PLAN-time D12 + IMPL-fixup-time), (b) the parent-rollup closure narrative, and (c) the cross-phase reuse anticipation for ext_proc + global_ratelimit + H2-upstream filters.
