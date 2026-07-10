# Phase 56.1 Implementation Plan — the HTTP tap filter, headers leg: `internal/headermatch` + `internal/matchpredicate` + `internal/filter/http/tap` (a dual-sided end-of-stream observer) + an in-package byte-exact `filePerTapSink` + the `0099-http-tap-headers` differential

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller verifies each commit, re-runs gates on the frozen HEAD, performs deliberate-break/liveness verification ITSELF, and squashes + pushes at stage-close (`feedback_subagent_autocommit_claudemd` · `feedback_push_to_origin`).

**Goal:** When an HCM `http_filters[]` entry carries an `envoy.extensions.filters.http.tap.v3.Tap` typed_config, envoy-go compiles the `TapConfig.match` (f4) `MatchPredicate` tree into a tri-state node tree at config time, evaluates it incrementally over request headers (`DecodeHeaders`) and response headers (`EncodeHeaders`), and — **at stream end, on a match, and never before** — assembles a `data/tap/v3.TraceWrapper` carrying an `HttpBufferedTrace` and writes it as ONE byte-exact protojson document to a per-stream `file_per_tap` sink file. Proven cross-side against `envoyproxy/envoy:contrib-v1.37.2` by the `0099-http-tap-headers` differential, whose bodyless `GET` → `204` shape makes both sides emit structurally headers-only traces.

**Architecture:** Tap is the **20th production HTTP filter** (21st registered). It is a *usage pattern*, not a new framework primitive: it plugs into the landed `StreamDecoderFilter` + `StreamEncoderFilter` interfaces via the two-step ADR-0071 factory, and adds ZERO new dispatch layers, ZERO new callback methods, and ZERO new go.mod modules. It adds two library packages (`internal/headermatch`, `internal/matchpredicate`) and the filter package `internal/filter/http/tap` (which houses the unexported `filePerTapSink`). It is the FIRST Observability-family row to integrate at the HTTP-filter layer rather than the bootstrap-sink layer, and the FIRST filter whose emit decision is deferred to stream end across BOTH directions. Byte-identical when no tap filter is configured. ANCHORS ADR-0273; the Observability family STAYS OPEN.

**Tech Stack:** Go; the in-tree HTTP-filter seam (`internal/filter/http`); `internal/stats` (one static counter); `google.golang.org/protobuf/encoding/protojson` (the byte-exact renderer); the resolved `go-control-plane/envoy v1.32.4` tap/matcher/route/core protos; the Docker-bridge differential harness. ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Global Constraints

Every task's requirements implicitly include this section. Values are copied verbatim from SPEC-56.1.

- **Framework-zero-touch (PRODUCTION).** No change to any landed filter, to the HTTP-filter dispatch layer, to the callback surface, or to the trailer seams. Do NOT attempt the never-done HCM "Task 18". *(The `test/differential` harness is NOT production framework — see Task 13 and D-TAP-DIRMOUNT.)*
- **The pinned marshal, byte-exact:** `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitDefaultValues: true}` + a trailing `"\n"`. `EmitUnpopulated: true` is **WRONG** (it emits `"body": null`). Indent is a **single space**. A byte-stability unit test is MANDATORY.
- **Exactly ONE new stat:** `http.<stat_prefix>.tap.rq_tapped` (counter). The `.tap.` segment is HARDCODED — it is NOT the `http_filters[]` entry name. Registered at filter-parse (not per-stream) so it reads `0` with no taps. Increments on the **MATCH decision**, not on a successful file write. **ZERO new stat-name (SN) rule.** Stat surface 1200 → 1201.
- **`reference_dynamic_stat_name_charset_guard` is N/A** — no tap stat name is wire-derived, so no `stats.IsValidName` guard is needed.
- **Every unsupported arm gets an EXPLICIT reject**, never a silent ignore and never a fall-through `default` (`reference_strict_reject_sibling_typeurl_gap`, ADR-0080). See §6 of the SPEC and Task 7.
- **ONE SHARED `*tapFilter` VALUE** in both `HTTPFilter.Decoder` and `HTTPFilter.Encoder`. An Encoder-only `OnDestroy` is UNREACHABLE when a Decoder is present.
- **NEVER mutate the header map handed to `EncodeHeaders`.** Build a COPY. A synthetic `:status` added to that map is emitted literally on the wire.
- **Lowercase every emitted header key.** Go canonicalizes `x-tap` → `X-Tap`; the reference emits lowercase.
- **The trace is an END-OF-STREAM artifact.** Never early-emit, even when a request arm is already TRUE at decode.
- **Per-task gates (every code task):** `gofmt -l` (must print nothing) + `golangci-lint run` on touched packages + `go vet ./...` + `go build ./...` (`feedback_pertask_gofmt_lint`).
- **COMMIT BEFORE YOU BREAK.** `git restore <file>` reverts to HEAD, not to "before the break". Finish → gate → commit → only then break, restore, re-run with `-count=1`.
- **Deliberate-break runs ALWAYS use `-count=1`** (`reference_differential_break_protocol_count1`) and the FULL subtest selector `-run 'TestDifferential/0099-http-tap-headers'` (`reference_differential_run_selector` — a bare `-run '0099'` matches ZERO subtests and reports a vacuous PASS).
- **`t.Errorf` per independent property; `t.Fatalf` only for a broken precondition** (`reference_fatalf_makes_assertions_unreachable`).

---

## Orientation — read before Task 1 (the zero-context brief)

You are extending a Go reimplementation of Envoy. This row adds Envoy's **HTTP tap filter**: a per-stream observer that watches a request/response pair, decides whether it "matches" a configured predicate, and if so writes a JSON "trace" of the stream to a file.

Three things make it unusual, and all three are load-bearing:

1. **The trace is assembled across BOTH directions and emitted only at the very end.** Request headers arrive at `DecodeHeaders`; response headers arrive at `EncodeHeaders`; the file is written at `OnDestroy` (stream teardown). The real Envoy *never* writes early — even when the predicate is already satisfiable at request time, it still waits and emits the whole stream. So the predicate tree needs three states per node (True / False / Undetermined) to *resolve* correctly, but the *timing* of emission is unconditional: always at stream end.

2. **`OnDestroy` fires exactly once — and only via the Decoder field.** `FilterChain.Destroy()` loops `if f.Decoder != nil { f.Decoder.OnDestroy() } else if f.Encoder != nil { f.Encoder.OnDestroy() }`. The branches are mutually exclusive. If you decompose tap into two values (`Decoder: &decodeSide{}, Encoder: &encodeSide{}`), the encoder's `OnDestroy` will **never run** and your emit will silently never fire. Tap installs **one shared `*tapFilter` pointer in both fields**.

3. **The response `:status` is not in the header map, and adding it leaks onto the wire.** HCM hands the encoder chain a header map that deliberately omits `:status`, then merges that same map back into the response it writes to the socket. If you write `":status"` into it, a literal `:status: 204` header goes out on the wire (HTTP/1) or a duplicate `:status` pseudo-header (HTTP/2). Take the status from `EncoderFilterCallbacks.ResponseStatus()` and inject it into a **copy**.

The **differential harness** boots BOTH the real reference Envoy (in a Docker container) and the in-process subject (envoy-go) against equivalent bootstraps, drives the same traffic at both, and asserts equivalence. For `0099` the reference writes its tap files inside the container, so the fixture bind-mounts a host directory over the container's output directory (Task 13 adds directory-mount support to the harness — it currently only mounts single files).

### Key source seams (verified at PLAN time against master `0f82eb75`; re-confirm line numbers before editing — files evolve)

**The filter framework** (`internal/filter/http/`):
- `types.go:54-60` — `StreamDecoderFilter interface { DecodeHeaders(http.Header, bool) FilterHeadersStatus; DecodeData([]byte, bool) FilterDataStatus; DecodeTrailers(http.Header) FilterTrailersStatus; SetDecoderCallbacks(DecoderFilterCallbacks); OnDestroy() }`
- `types.go:65-71` — `StreamEncoderFilter interface { EncodeHeaders(...); EncodeData(...); EncodeTrailers(...); SetEncoderCallbacks(EncoderFilterCallbacks); OnDestroy() }`
- `types.go:77-81` — `HTTPFilter struct { Name string; Decoder StreamDecoderFilter; Encoder StreamEncoderFilter }`
- `types.go:245` — `type HTTPFilterFactory func(tc *anypb.Any, ctx FactoryCtx) (FilterInstanceFactory, error)`
- `types.go:249` — `type FilterInstanceFactory func() HTTPFilter`
- `types.go:253-304` — `FactoryCtx struct` with SIX fields: `Registry *HTTPRegistry` (`:254`), `Stats *stats.Registry` (`:260`), `StatPrefix string` (`:264`), `ClusterManager *cluster.Manager` (`:276`), `HTTPClient *httpclient.Client` (`:287`), `NodeServiceCluster string` (`:301`). **There is NO `BaseDir` field** (that is the *network*-filter `FactoryCtx` — do not confuse them). Tap uses only `Stats` + `StatPrefix`.
- `chain.go:292` — `destroyOnce sync.Once`; `chain.go:665-679` — `Destroy()`, whose loop at `:668-672` is:
  ```go
  if f.Decoder != nil {
      f.Decoder.OnDestroy()      // chain.go:669
  } else if f.Encoder != nil {   // chain.go:670 — ELSE IF
      f.Encoder.OnDestroy()      // chain.go:671
  }
  ```
  `chain.go:669`/`:671` are the ONLY production callers of an HTTP-filter `OnDestroy`. `Destroy()` has exactly two production callers: `internal/filter/hcm/connection.go:447` (H1) and `internal/filter/hcm/h2dispatch.go:383` (H2), both `defer`red.
- `callbacks.go:103` — `DownstreamRemoteAddr() net.Addr` (decoder); `callbacks.go:113` — `DownstreamLocalAddr() net.Addr` (decoder). Same two on the encoder at `:390`/`:399`.
- `callbacks.go:488` — `ResponseStatus() int`. **Encoder side ONLY.** Seeded by `chain.SetEncodeResponseStatus` (`chain.go:1178`), called at `connection.go:737` (H1), `h2dispatch.go:574` (H2), `chain.go:1250` (local replies).
- **There is NO accessor anywhere exposing the per-direction header-arrival instant.** Grep-proven: `FilterChain` has no time field and `chain.go` does not import `"time"`. This is why `record_headers_received_time` is rejected (D-TAP-RECORDFLAGS).
- `registry.go:35` — `func (r *HTTPRegistry) Register(typeURL string, f HTTPFilterFactory)` (panics if frozen or duplicate).

**The HCM header seams** (`internal/filter/hcm/`):
- Request pseudo-headers injected into the filter-visible map, by **direct raw-map assignment** (NOT `.Set`, which would canonicalize away the leading colon):
  - H1: `:method` `connection.go:481-483`; `:authority` `:495-497`; `:path` `:506-517`.
  - H2: `:method` `h2dispatch.go:466-468`; `:authority` `:478-480`; `:path` `:488-494`.
  - **`:scheme` is injected on NEITHER path.** The H2 codec parses it into a local var and discards it (`h2/stream.go:393-398`). envoy-go filters can never see `:scheme`.
- The encode block, H1 `connection.go:731-757`:
  ```go
  733  merged := resp.Headers.ToHTTPHeader()
  736  // ... :status is not present ...
  737  chain.SetEncodeResponseStatus(status)
  738  if _, err := chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0); err != nil { ... }
  741  resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)
  ```
  then `writeH1Reply(bw, resp.Status, resp.Headers, resp.Body)` at `:766`. H2 is identical in shape: `h2dispatch.go:570` `merged`, `:574` seed, `:575` run, `:580` reconcile, `:602` `writeH2Reply`.
- `ReconcileOrderedHeaders` (`internal/filter/http/types.go:206-238`) does **no** pseudo-header filtering — an unknown key falls into the `newKeys` branch (`:221-236`) and is appended. `writeH1Reply` (`internal/filter/hcm/codec.go:74-119`) emits every field as `"%s: %s\r\n"` after `canon := textproto.CanonicalMIMEHeaderKey(hf.Name)` (`:85`), and **`CanonicalMIMEHeaderKey(":status") == ":status"`** (empirically confirmed — the leading colon is an invalid token byte, so canonicalization is a no-op). **Hence the leak is real.**
- Trailers: `chain.go:454` `RunDecodeTrailers` and `chain.go:621` `RunEncodeTrailers` exist but have **ZERO production callers**. `connection.go:562-568` and `h2dispatch.go:501-504` carry the "Task 18" comments. *(Those comments claim "the FilterChain does not yet expose a RunDecodeTrailers method" — that clause is stale/false; the method exists. What remains true is that HCM never invokes it.)*

**Precedents to copy:**
- **Both-sided shared instance** — `internal/filter/http/compressor/compressor.go:300-307`:
  ```go
  return func() envoyhttp.HTTPFilter {
      f := &filter{config: compiled, stats: fStats}
      return envoyhttp.HTTPFilter{
          Name:    filterName,
          Decoder: f,
          Encoder: f, // SAME *filter per ADR-0129 §Decision (iv).
      }
  }, nil
  ```
- **Filter TypeURL + factory + stat registration** — `internal/filter/http/fault/fault.go:23` (`const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.fault.v3.HTTPFault"`), `:84` (`func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error)`), `:97` (`fs := registerFaultStats(ctx.Stats, ctx.StatPrefix)`), `:242` (`p := "http." + prefix + ".fault."`), `:244` (`reg.NewCounter(p + "aborts_injected")`). Note `registerFaultStats` is nil-tolerant: `fault.go:239-241` returns an all-nil struct when `reg == nil`.
- **`internal/stats/registry.go:84`** — `func (r *Registry) NewCounter(name string) *Counter` (panics if frozen/invalid/duplicate). Tap uses `NewCounter`, not `NewCounterIfAbsent` (registration happens at HCM-build time, pre-Freeze).
- **Registration** — `internal/filter/http/builtins/builtins.go:44-63` registers exactly **20** filters (`router`, `adaptive_concurrency`, `admission_control`, `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `extauthz`, `extproc`, `fault`, `header_mutation`, `jwtauthn`, `localratelimit`, `lua`, `oauth2`, `ratelimit`, `rbac`, `wasm`), one of which — `envoygotest` — is test-support. Lines `:64-68` register 5 per-route validators (not filters). The HTTP registry is frozen at **`internal/boot/boot.go:65`** (`httpReg.Freeze()`), immediately after `httpbuiltins.RegisterBuiltins(httpReg)` at `:64`.
  > **`cmd/envoy-go/main.go` does NOT register HTTP filters.** `builtins.go`'s own doc comment says it "mirrors the registration calls in cmd/envoy-go/main.go" — that comment is **STALE**; grep of `main.go` for `Register(`/`RegisterBuiltins` finds nothing. `main.go:322`'s `Freeze()` targets the **stats** registry. **Task 12 touches `builtins.go` ONLY.**
- **Parse-reject unit test** — `internal/filter/http/fault/fault_test.go:53-77`; `mustAny(t, m proto.Message) *anypb.Any` at `:29` (wraps `anypb.New`); malformed-bytes reject at `:45` (`&anypb.Any{TypeUrl: "...", Value: []byte{0xff,0xff,0xff}}`); nil reject at `:38`.
- **Config-parse fuzzer** — `internal/filter/http/fault/fuzz_test.go:23-45`. Fuzzers live in a `fuzz_test.go` in the **same package**. Seeds are `[][]byte` fed via `f.Add`; the invariant asserted is exclusivity: never `(nil, nil)` and never `(factory, err)`.
- **Stat-surface delta guard** — `internal/statssink/registration_test.go:11` (`countMetrics(reg)` walks the registry) and `:51` (`TestNoNewStat_...`). **The absolute "1200" is a live reference-Envoy probe figure, NOT an in-tree assertion.** What the tree enforces is the *delta*. Tap's guard asserts the tap factory registers exactly **+1** counter.

**The differential harness** (`test/differential/`):
- `fixture/fixture.go:15-52` — the required interface is `Driver` (NOT "Fixture"): `BackendCount() int` (`:20`), `SubjectListenerName() string` (`:25`), `ReferenceBootstrap(backendPorts []int) string` (`:30`), `SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string` (`:34`), `ReferenceListenerPort() int` (`:38`), `DriveReference(ctx, addr) ([]byte, error)` (`:42`), `DriveSubject(ctx, addr) ([]byte, error)` (`:46`), `ProbeAdmin(ctx, refAdminAddr, subjAdminAddr) (refBytes, subjBytes []byte, err error)` (`:51`). Registered from the driver package's `init()` via `fixture.RegisterFixture(name, d)` (`:84`); **the name must equal the fixture directory name**.
- `fixture/fixture.go:64` — `TB interface { Errorf; Fatalf; Helper }`. **No `Logf`** (`reference_fixture_tb_has_no_logf`).
- **Asserter dispatch (`reference_differential_asserter_dispatch`)** — `StatsAsserter{AssertStats(t TB, refAdminAddr, subjAdminAddr string)}` (`fixture.go:75`) fires on the **cross-side** path (`runner_test.go:1304-1306`). `SubjectAsserter{AssertSubject(t TB, subjBytes []byte)}` (`fixture.go:688`) fires **ONLY** on the reference-less path (`runner_test.go:2039-2041`). **`0099` is cross-side, so ALL trace assertions go in `AssertStats`** — `AssertSubject` would never run.
- `fixture/fixture.go:631` — `type HostMount struct { HostPath, ContainerPath string }`; `:642` — `ReferenceLogMounter{ ReferenceHostMounts() []HostMount }`. The runner pre-creates each `HostPath` as a **FILE** (`runner_test.go:1148-1150`, `os.OpenFile(hm.HostPath, os.O_CREATE|os.O_WRONLY, 0o666)`), then `harness.go:176-178` turns each into a Docker `HostConfig.Binds` entry `"<hostPath>:<containerPath>"`. **This is why Task 13 exists**: `file_per_tap` writes N files with unpredictable `<trace_id>` names, so it needs a *directory* mount, which the current struct cannot express.
- `fixture/fixture.go:148` — `HTTPStatusHeader BackendKind = 3`: an out-of-process HTTP/1.1 backend (`test/fixtures/0005-prometheus-stats/backends/main.go`) that reads the `X-Backend-Status` request header and returns that status. **Empirically verified**: with `X-Backend-Status: 204` it emits `HTTP/1.1 204 No Content` + `Connection: close` + `Content-Type: text/plain` + `Date`, **no `Content-Length`, and zero body bytes** (Go's `net/http` discards a body write on a 204). This is exactly the AMEND-TAP-BODY shape — **so `0099` needs NO new BackendKind; the tail stays 38.**
- `runner_test.go:222-224` — `BackendCount()` < 1 is a `t.Fatalf` (`reference_differential_backendcount_min_one`).
- `runner_test.go:170-183` — the subtest name is the **exact fixture directory name**. Selector: `-run 'TestDifferential/0099-http-tap-headers'`.
- `harness.go:170` — `StartReferenceProxyWithMounts(ctx, pin, bootstrap, hostMounts, listenerPorts...)`; bind mounts MUST go through `HostConfig.Binds` (`:176-178`, `:187`) because testcontainers-go v0.27.0 silently drops `MountTypeBind` entries in `mapToDockerMounts`. The reference reaches host backends via `host.docker.internal` (ExtraHosts `host-gateway`).
- **The container output dir must NOT be sticky.** `test/fixtures/0006-access-log/driver/driver.go:53-56` records the lesson: `/tmp` in the envoy image is sticky + world-writable, so under `fs.protected_regular=2` (Ubuntu CI) envoy (uid 101) gets EACCES opening a host-owned bind-mounted file with `O_CREAT`. `0006` uses `/envoy-go-test/`. `0099` follows suit.
- The reference bootstrap is passed **inline** via `--config-yaml` (`harness.go:107`), never a mounted file.

### Proto facts (verified at PLAN time against `go-control-plane/envoy v1.32.4` in the module cache — re-confirm at IMPL)

**Citation hazard (AMEND-TAP-PROTOPATHS):** three distinct files are named `common.pb.go` and all three declare `package tapv3`. There are TWO distinct `MatchPredicate` messages. **Always alias explicitly.** Recommended aliases used throughout this plan:

```go
import (
    httptapv3    "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/tap/v3"
    commontapv3  "github.com/envoyproxy/go-control-plane/envoy/extensions/common/tap/v3"
    taptapv3     "github.com/envoyproxy/go-control-plane/envoy/config/tap/v3"
    cmatcherv3   "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
    routev3      "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
    tmatcherv3   "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
    typev3       "github.com/envoyproxy/go-control-plane/envoy/type/v3"
    datatapv3    "github.com/envoyproxy/go-control-plane/envoy/data/tap/v3"
    corev3       "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)
```

| Message | Field (Go) | # | Go type | Getter |
|---|---|---|---|---|
| `httptapv3.Tap` | `CommonConfig` | 1 | `*commontapv3.CommonExtensionConfig` | `GetCommonConfig()` |
| | `RecordHeadersReceivedTime` | 2 | `bool` | `GetRecordHeadersReceivedTime()` |
| | `RecordDownstreamConnection` | 3 | `bool` | `GetRecordDownstreamConnection()` |
| `commontapv3.CommonExtensionConfig` | oneof `ConfigType` (iface `isCommonExtensionConfig_ConfigType`) — **2 arms only, no `TapdsConfig`** | | | `GetConfigType()` |
| | `CommonExtensionConfig_AdminConfig{AdminConfig *commontapv3.AdminConfig}` | 1 | | `GetAdminConfig()` |
| | `CommonExtensionConfig_StaticConfig{StaticConfig *taptapv3.TapConfig}` | 2 | | `GetStaticConfig()` |
| `taptapv3.TapConfig` | `MatchConfig` **(the ONLY deprecated field)** | 1 | `*taptapv3.MatchPredicate` ← **package-local, NOT `cmatcherv3`** | `GetMatchConfig()` |
| | `OutputConfig` | 2 | `*taptapv3.OutputConfig` | `GetOutputConfig()` |
| | `TapEnabled` | 3 | `*corev3.RuntimeFractionalPercent` | `GetTapEnabled()` |
| | `Match` | 4 | `*cmatcherv3.MatchPredicate` | `GetMatch()` |
| `taptapv3.OutputConfig` | `Sinks` | 1 | `[]*taptapv3.OutputSink` | `GetSinks()` |
| | `MaxBufferedRxBytes` / `MaxBufferedTxBytes` | 2 / 3 | `*wrapperspb.UInt32Value` | — |
| | `Streaming` | 4 | `bool` | `GetStreaming()` |
| `taptapv3.OutputSink` | `Format` | 1 | `taptapv3.OutputSink_Format` | `GetFormat()` |
| | oneof `OutputSinkType` (iface `isOutputSink_OutputSinkType`) | | | `GetOutputSinkType()` |
| | `OutputSink_StreamingAdmin` | 2 | `*taptapv3.StreamingAdminSink` | `GetStreamingAdmin()` |
| | `OutputSink_FilePerTap` | 3 | `*taptapv3.FilePerTapSink` | `GetFilePerTap()` |
| | `OutputSink_StreamingGrpc` | 4 | `*taptapv3.StreamingGrpcSink` | `GetStreamingGrpc()` |
| | `OutputSink_BufferedAdmin` | 5 | `*taptapv3.BufferedAdminSink` | `GetBufferedAdmin()` |
| | `OutputSink_CustomSink` | 6 | `*corev3.TypedExtensionConfig` | `GetCustomSink()` |
| `taptapv3.FilePerTapSink` | `PathPrefix` | 1 | `string` | `GetPathPrefix()` |
| `datatapv3.TraceWrapper` | oneof `Trace` (iface `isTraceWrapper_Trace`) — **4 arms** | | | `GetTrace()` |
| | `TraceWrapper_HttpBufferedTrace` | 1 | `*datatapv3.HttpBufferedTrace` | `GetHttpBufferedTrace()` |
| | `TraceWrapper_HttpStreamedTraceSegment` / `_SocketBufferedTrace` / `_SocketStreamedTraceSegment` | 2/3/4 | | |
| `datatapv3.HttpBufferedTrace` | `Request` / `Response` | 1 / 2 | `*datatapv3.HttpBufferedTrace_Message` | `GetRequest()` / `GetResponse()` |
| | `DownstreamConnection` | 3 | `*datatapv3.Connection` | `GetDownstreamConnection()` |
| `datatapv3.HttpBufferedTrace_Message` | `Headers` | 1 | `[]*corev3.HeaderValue` | `GetHeaders()` |
| | `Body` | 2 | `*datatapv3.Body` | `GetBody()` |
| | `Trailers` | 3 | `[]*corev3.HeaderValue` | `GetTrailers()` |
| | `HeadersReceivedTime` | 4 | `*timestamppb.Timestamp` | `GetHeadersReceivedTime()` |
| `datatapv3.Connection` | `LocalAddress` / `RemoteAddress` | 1 / 2 | `*corev3.Address` | `GetLocalAddress()` / `GetRemoteAddress()` |
| `corev3.HeaderValue` | `Key` / `Value` / `RawValue` | 1 / 2 / 3 | `string` / `string` / `[]byte` | `GetKey()` / `GetValue()` / `GetRawValue()` |

`taptapv3.OutputSink_Format` enum constants: `OutputSink_JSON_BODY_AS_BYTES = 0`, `OutputSink_JSON_BODY_AS_STRING = 1`, `OutputSink_PROTO_BINARY = 2`, `OutputSink_PROTO_BINARY_LENGTH_DELIMITED = 3`, `OutputSink_PROTO_TEXT = 4`.

**`cmatcherv3.MatchPredicate`** — oneof `Rule`, interface `isMatchPredicate_Rule`, getter `GetRule()`. **Exactly 10 arms:**

| Wrapper struct | # | Payload | Getter | 56.1 |
|---|---|---|---|---|
| `MatchPredicate_OrMatch` | 1 | `*cmatcherv3.MatchPredicate_MatchSet` | `GetOrMatch()` | **ACCEPT** |
| `MatchPredicate_AndMatch` | 2 | `*cmatcherv3.MatchPredicate_MatchSet` | `GetAndMatch()` | **ACCEPT** |
| `MatchPredicate_NotMatch` | 3 | `*cmatcherv3.MatchPredicate` | `GetNotMatch()` | **ACCEPT** |
| `MatchPredicate_AnyMatch` | 4 | `bool` | `GetAnyMatch()` | **ACCEPT** |
| `MatchPredicate_HttpRequestHeadersMatch` | 5 | `*cmatcherv3.HttpHeadersMatch` | `GetHttpRequestHeadersMatch()` | **ACCEPT** |
| `MatchPredicate_HttpRequestTrailersMatch` | 6 | `*cmatcherv3.HttpHeadersMatch` | `GetHttpRequestTrailersMatch()` | **REJECT** |
| `MatchPredicate_HttpResponseHeadersMatch` | 7 | `*cmatcherv3.HttpHeadersMatch` | `GetHttpResponseHeadersMatch()` | **ACCEPT** |
| `MatchPredicate_HttpResponseTrailersMatch` | 8 | `*cmatcherv3.HttpHeadersMatch` | `GetHttpResponseTrailersMatch()` | **REJECT** |
| `MatchPredicate_HttpRequestGenericBodyMatch` | 9 | `*cmatcherv3.HttpGenericBodyMatch` | `GetHttpRequestGenericBodyMatch()` | **REJECT** |
| `MatchPredicate_HttpResponseGenericBodyMatch` | 10 | `*cmatcherv3.HttpGenericBodyMatch` | `GetHttpResponseGenericBodyMatch()` | **REJECT** |

> **`or_match` is field 1; `and_match` is field 2.** (A first-draft SPEC error, caught and fixed. Do not re-swap them.)

`cmatcherv3.MatchPredicate_MatchSet` has `Rules []*cmatcherv3.MatchPredicate` (f1, `GetRules()`). `cmatcherv3.HttpHeadersMatch` has `Headers []*routev3.HeaderMatcher` (f1, `GetHeaders()`).

**`routev3.HeaderMatcher`** — `Name` (f1, `GetName()`), `InvertMatch` (f8, `GetInvertMatch()`), `TreatMissingHeaderAsEmpty` (f14, `GetTreatMissingHeaderAsEmpty()`); oneof `HeaderMatchSpecifier`, interface `isHeaderMatcher_HeaderMatchSpecifier`, getter `GetHeaderMatchSpecifier()`. **Exactly 8 arms, 5 deprecated (still accepted):**

| Wrapper | # | Payload | Getter | Deprecated |
|---|---|---|---|---|
| `HeaderMatcher_ExactMatch` | 4 | `string` | `GetExactMatch()` | **yes** |
| `HeaderMatcher_RangeMatch` | 6 | `*typev3.Int64Range` | `GetRangeMatch()` | no |
| `HeaderMatcher_PresentMatch` | 7 | `bool` | `GetPresentMatch()` | no |
| `HeaderMatcher_PrefixMatch` | 9 | `string` | `GetPrefixMatch()` | **yes** |
| `HeaderMatcher_SuffixMatch` | 10 | `string` | `GetSuffixMatch()` | **yes** |
| `HeaderMatcher_SafeRegexMatch` | 11 | `*tmatcherv3.RegexMatcher` | `GetSafeRegexMatch()` | **yes** |
| `HeaderMatcher_ContainsMatch` | 12 | `string` | `GetContainsMatch()` | **yes** |
| `HeaderMatcher_StringMatch` | 13 | `*tmatcherv3.StringMatcher` | `GetStringMatch()` | no |

**`tmatcherv3.StringMatcher`** — oneof `MatchPattern`, iface `isStringMatcher_MatchPattern`: `StringMatcher_Exact` (1), `StringMatcher_Prefix` (2), `StringMatcher_Suffix` (3), `StringMatcher_SafeRegex` (5, `*tmatcherv3.RegexMatcher`), `StringMatcher_Contains` (7), `StringMatcher_Custom` (8, xds `TypedExtensionConfig`). Non-oneof `IgnoreCase bool` (f6, `GetIgnoreCase()`). **No field 4.**

**`tmatcherv3.RegexMatcher`** — `Regex string` (f2, **`GetRegex()`**); oneof `EngineType` with the single arm `RegexMatcher_GoogleRe2{GoogleRe2 *tmatcherv3.RegexMatcher_GoogleRE2}` (f1, deprecated). **Casing trap:** the wrapper/field is `GoogleRe2`, the message type is `RegexMatcher_GoogleRE2`.

**`typev3.Int64Range`** — `Start int64` (f1, `GetStart()`), `End int64` (f2, `GetEnd()`). Envoy semantics: `[start, end)`.

All the above are present at `envoy v1.32.4`. **ZERO new go.mod modules.**

### Discipline (honor on EVERY task)

- Run in the worktree pinned by the controller. Give worktree-relative paths. After each task the controller verifies `git -C /home/esa/git/envoy-go status --porcelain` is EMPTY (`feedback_subagent_worktree_path_targeting`).
- Revert a deliberate break with `git restore <file>` ONLY. Never `git checkout <sha>`, never `git commit --amend`. Re-verify `git branch --show-current` after every break (`feedback_subagent_worktree_detach`).
- Never run the unit suite and a `-race` suite concurrently. A `subject ready: EOF` on an UNRELATED fixture is the known startup flake (`reference_differential_fullsuite_startup_flake`) — isolate-re-run to discriminate.
- **Do not verify a citation against the document that made it.** Re-derive every `file:line` from source (`feedback_brief_citations_not_evidence`).

---

## D-question resolutions (the SPEC §12 PLAN pins — settled here)

The SPEC left six. Source archaeology at PLAN time surfaced a **seventh** (D-TAP-DIRMOUNT) that the SPEC's "a per-side temp dir" hand-wave concealed. All seven are pinned below.

### D-TAP-DEPTHCAP → **32**, enforced at compile, exercised by the fuzzer

`internal/matchpredicate`'s compiler is recursive over an attacker-influenceable proto (`not_match` chains, `and_match`/`or_match` nesting). Cap **depth at 32**, counted as *nesting depth of `MatchPredicate` nodes*, root = depth 1. Compiling a node at depth > 32 returns `ErrDepthExceeded` (a boot-reject), never a panic and never a silent truncation. 32 is deep enough for any realistic predicate and shallow enough to bound `FuzzMatchPredicateCompile`'s stack. **Task 6 MUST include a depth-33 seed proving the cap fires** (a cap that is never exercised is not a cap).

### D-TAP-TRACEID → a **process-local monotonic `atomic.Uint64`**, starting at 1

The id appears only in the filename (`<path_prefix>_<trace_id>.json`) and is **never asserted cross-side** (SPEC §8.2 — filenames embed a proxy-internal id that cannot agree between sides). A monotonic counter is sufficient, has no error path, and makes the sink's unit tests deterministic. `crypto/rand` would add an error path and non-determinism for zero benefit. **Rejected alternative:** matching the reference's random-looking 64-bit id — pointless, since we never compare it.

Collision safety comes free from monotonicity; the sink additionally opens with `O_EXCL` so a collision would surface as an error rather than a silent overwrite.

### D-TAP-RECORDFLAGS → **honor `record_downstream_connection`; REJECT `record_headers_received_time: true`**

- **`record_downstream_connection` (f3): HONORED at 56.1.** Both address fields are plumbable: `DecoderFilterCallbacks.DownstreamLocalAddr() net.Addr` (`callbacks.go:113`) and `DownstreamRemoteAddr() net.Addr` (`:103`), captured in `SetDecoderCallbacks`. When true, populate `HttpBufferedTrace.DownstreamConnection` (f3) with `Connection{LocalAddress, RemoteAddress}` as `corev3.Address{Address: &corev3.Address_SocketAddress{...}}`.
- **`record_headers_received_time` (f2): REJECTED when `true`.** `Message.headers_received_time` needs the exact per-direction header-arrival instant. **No landed accessor exposes it** — grep-proven: neither callbacks interface returns a `time.Time`, `FilterChain` has no time field, and `chain.go` does not import `"time"`. Per ADR-0080 an unhonorable field gets an **explicit reject**, not a silent ignore. `false` (the default) is accepted.

This is a slight strengthening of the SPEC's "defer/UNassert" recommendation: "defer" in an ADR-0080 tree *means* reject. The baseline `0099` fixture leaves both flags unset, so neither choice affects the differential; `record_downstream_connection` is covered by unit tests (Task 11 is folded into Task 9).

### D-TAP-SUBSET → the exact cross-side-deterministic subsets

`0099` drives `GET /tap` with request headers `x-tap: {yes|no}` and `x-backend-status: 204`.

**Asserted `request.headers` ⊇ (set-compared, lowercased):**
```
:method          GET
:path            /tap
x-tap            yes
x-backend-status 204
```
**Asserted `response.headers` ⊇ (set-compared, lowercased):**
```
:status          204
content-type     text/plain
```
**UNasserted (coverage boundaries + non-determinism), each with its reason:**

| Key | Why unasserted |
|---|---|
| `:authority` | the two sides listen on different ports ⇒ `127.0.0.1:<port>` differs |
| `:scheme` | never plumbed into envoy-go's filter-visible map (AMEND-TAP-PSEUDO) |
| `x-request-id`, `x-forwarded-proto`, `x-envoy-expected-rq-timeout-ms` | reference-only request headers |
| `date`, `server`, `x-envoy-upstream-service-time` | reference-only / non-deterministic response headers |
| `connection` | hop-by-hop; strip behavior is not pinned cross-side |
| `user-agent` | driver HTTP client differs per side |
| header ORDER | envoy-go sorts; the reference emits codec order (§3.6) — the comparison is a SET |
| the trace FILENAME | embeds the proxy-internal `<trace_id>` |

> **`content-type: text/plain` is NOT yet proven end-to-end — confirm it live at the IMPL.** What *is* verified (raw-socket probe) is only that the `0005` backend emits `Content-Type: text/plain` on its bodyless 204. Nothing has verified that the header survives **reference Envoy's** encoder or **envoy-go's** on a 204; the SPEC's §11.9 probe used `mccutchen/go-httpbin`, not this backend, and only counted response headers. Envoy special-cases bodyless responses.
>
> This fails **safe**: if either proxy strips it, Task 14 Step 2 ("run the fixture green") goes RED before any break is attempted, not silently vacuous. **If it is stripped, move `content-type` to the UNasserted list** — and note the consequence honestly: `response.headers` then rests on `:status: 204` alone, which is still a genuine cross-side property (the subject *synthesizes* it from `ResponseStatus()` while the reference emits a real pseudo-header), but it is a one-key assertion. Do **not** weaken any other assertion to compensate, and do **not** reach for `response_headers_to_add` as a substitute without first probing where each side applies route-level response headers relative to the encoder filter chain.

### D-TAP-EMITSITE → ONE shared value, **NO defensive guard**, proven by three unit tests

`FilterChain.Destroy()` is `destroyOnce`-guarded (`chain.go:666`) and its loop is `if Decoder != nil {…} else if Encoder != nil {…}` (`chain.go:668-672`). The branches are mutually exclusive, so a both-sided filter's `OnDestroy` fires **exactly once**. A `sync.Once` would be dead code that no test could drive to its second call. **Do not add one.** Instead, Task 9 lands three tests:

1. the factory returns an `HTTPFilter` whose `Decoder` and `Encoder` hold the **same pointer** (`reflect`/interface identity);
2. driving a chain to `Destroy()` invokes the tap emit **exactly once**;
3. a *regression* test pinning the hazard: a filter installed as `HTTPFilter{Decoder: dec, Encoder: enc}` with two **distinct** values sees `enc.OnDestroy()` **never called** — proving why tap must not decompose.

### D-TAP-PATHPREFIX → `os.MkdirAll(filepath.Dir(path_prefix), 0o755)` at **filter-parse time**, reject on error

`path_prefix` is a *prefix*, not a directory: `/envoy-go-test/taps/out` ⇒ dir `/envoy-go-test/taps`, basename prefix `out`. The sink `MkdirAll`s the parent **once, at parse time** (sink construction), and a failure is a **parse reject** — a bad output path is a config error and should fail at boot, not silently drop every trace at stream end. The reference's behavior here was not probed; this is a documented **DEPARTURE** (unprobed), recorded in `BEHAVIOR_CONTRACT`.

### D-TAP-DIRMOUNT (**NEW — the SPEC did not anticipate this**) → add `HostMount.Dir bool`

`file_per_tap` writes **one file per matching stream**, named `<path_prefix>_<trace_id>.json` — names the test cannot predict. The reference proxy runs **inside Docker**, so its output is invisible to the host test process unless the *parent directory* is bind-mounted. The existing seam cannot express this: `fixture.HostMount` is `{HostPath, ContainerPath}` and the runner unconditionally pre-creates `HostPath` as a **file** (`runner_test.go:1148-1150`). Pointing it at a directory would fail (`os.OpenFile` with `O_WRONLY` on a directory returns `EISDIR`).

**Pin:** add a third field `Dir bool` to `fixture.HostMount`. The runner branches: `Dir` ⇒ `os.MkdirAll(hm.HostPath, 0o777)` + `os.Chmod(hm.HostPath, 0o777)`; otherwise the existing file pre-create, byte-for-byte unchanged. `0o777` because envoy runs as **uid 101** inside the container and must create files in the mounted dir; the test process (a different uid) then reads them (envoy writes `0o644`) and unlinks them at cleanup (permitted — the *directory* is test-owned and writable).

The container-side prefix is `/envoy-go-test/taps/out` — under the **non-sticky** `/envoy-go-test/` dir, per the `0006-access-log` EACCES lesson (`driver.go:53-56`). `/tmp` in the envoy image is sticky + world-writable and breaks under `fs.protected_regular=2`.

**This is TEST-HARNESS surgery, not production framework surgery.** The Global-Constraints "framework-zero-touch" pin scopes to the HTTP-filter framework and HCM; `test/differential/` is neither. Zero behavior change for the one existing `ReferenceLogMounter` (`0006`, which leaves `Dir` false) — Task 13 proves that with a `0006` regression run.

### FINAL ADR-0045 split-gate re-check (SPEC §3.0's escape valve, re-armed and now discharged)

The SPEC's sketch was ~14 tasks and its escape valve reads: *"if the PLAN's decomposition exceeds ~15 tasks, re-open ADR-0045 before writing code."*

The honest first decomposition came to **16**: the sketch's 14, minus its item 11 (the reject/byte-stability unit tests, which belong *inside* the tasks that introduce the rejects and the renderer), plus three genuinely new tasks — a PROGRESS scaffold (house convention), the `record_downstream_connection` plumbing, and the newly-discovered `HostMount.Dir` harness surgery.

Two **semantic** folds bring it to **15**:
- `record_downstream_connection` folds into **Task 9** (trace assembly) — populating `HttpBufferedTrace.DownstreamConnection` *is* part of assembling the trace; it is one deliverable, not two.
- the reject-roster and byte-stability unit tests fold into **Task 7** and **Task 10** respectively — the tests that prove a behavior belong in the task that introduces it (TDD).

Neither fold hides work; each merges a step into the deliverable that necessitates it. **15 ≤ ~15: the gate HOLDS.** A further 56.1a/56.1b by-layer sub-split was **considered and rejected** — the BRAINSTORM's *(Q6)* already ruled it out on the ground that it would strand `internal/matchpredicate` as dead library code with no differential surface, and the phase-46.1a/46.1b precedent does not transfer (there, *both* halves had an observable emit surface; here the library half has none).

**The margin is now zero.** Recorded honestly: 56.1 consumes the entire ADR-0045 allowance. **56.2 must not absorb spillover from this leg** — if any task below grows a second deliverable during the IMPL, split it into a new task and re-open the gate rather than widening 56.2.

---

## File structure (decomposition locked here)

**New packages (2) + the filter package:**

| Path | Responsibility |
|---|---|
| `internal/headermatch/headermatch.go` | Exported `Matcher` — compiles ONE `routev3.HeaderMatcher` at config time; `Match(http.Header) bool` at request time. Owns `invert_match` + `treat_missing_header_as_empty`. |
| `internal/headermatch/stringmatch.go` | The package-private `stringMatcher` — compiles ONE `tmatcherv3.StringMatcher` (5 arms + `ignore_case`). No shared evaluator exists in the tree; this one is tap's. |
| `internal/headermatch/headermatch_test.go` | Table-driven: all 8 `HeaderMatcher` arms × present/absent/inverted/treat-missing. |
| `internal/headermatch/stringmatch_test.go` | Table-driven: 5 `StringMatcher` arms × `ignore_case` on/off. |
| `internal/matchpredicate/node.go` | The tri-state `Value` (`Undetermined`/`True`/`False`) + the node interface + the 6 node types. |
| `internal/matchpredicate/compile.go` | `Compile(*cmatcherv3.MatchPredicate) (*Tree, error)` — 6 accept arms, 4 explicit reject arms, the depth-32 cap. |
| `internal/matchpredicate/eval.go` | `(*Tree).FeedRequestHeaders`, `.FeedResponseHeaders`, `.Resolve()` — incremental evaluation; still-Undetermined ⇒ False at resolve. |
| `internal/matchpredicate/*_test.go` | Compiler + evaluator unit tests. |
| `internal/matchpredicate/fuzz_test.go` | `FuzzMatchPredicateCompile` (fuzzers 52 → **53**). |
| `internal/filter/http/tap/tap.go` | `TypeURL`, `New` (the ADR-0071 two-step factory), the `*tapFilter` value, `DecodeHeaders`/`EncodeHeaders`/`OnDestroy`. |
| `internal/filter/http/tap/config.go` | Config parse + the full §6 reject roster + `rq_tapped` registration. |
| `internal/filter/http/tap/trace.go` | Header copy/lowercase/sort; `:status` synthesis on a COPY; `TraceWrapper` assembly; `record_downstream_connection`. |
| `internal/filter/http/tap/sink.go` | The unexported `filePerTapSink` + the trace-id source + the pinned protojson render. |
| `internal/filter/http/tap/*_test.go` | Parse rejects, wire-leak regression, emit-once + shared-value, byte-stability. |
| `internal/filter/http/tap/doc.go` | Package doc, mirroring `compressor/doc.go`'s both-sided note. |

**Modified:**

| Path | Change |
|---|---|
| `internal/filter/http/builtins/builtins.go:44-63` | `+ reg.Register(tap.TypeURL, tap.New)` (alphabetical: after `router`? no — the block is alphabetical *except* `router` leads; insert `tap` between `rbac` and `wasm`). |
| `test/differential/fixture/fixture.go:631` | `HostMount` gains `Dir bool`. |
| `test/differential/runner_test.go:1146-1158` | Branch the mount pre-create on `hm.Dir`. |
| `test/differential/runner_test.go` (import block `:26-125`) | Blank-import the `0099` driver package. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | The §9 bundle. |
| `docs/envoy-go/DECISIONS.md` | ADR-0273 body. |
| `docs/envoy-go/{STATE,ROADMAP}.md`, `phases/56-http-tap-filter/{README,PROGRESS-56.1}.md` | The ADR-0052 bundle. |

**New (fixture):**

| Path | Responsibility |
|---|---|
| `test/fixtures/0099-http-tap-headers/envoy.yaml` | Reference bootstrap (container paths, `host.docker.internal` backend). |
| `test/fixtures/0099-http-tap-headers/envoy-go.yaml` | Subject bootstrap (host paths, loopback backend). |
| `test/fixtures/0099-http-tap-headers/driver/driver.go` | The `Driver` + `BackendKindAware` + `ReferenceLogMounter` + `StatsAsserter`. |

**Framework identifiers (verified at PLAN time):** module is `github.com/pgdad/envoy-go`. Filters import the seam as `envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"`. The status constants are `envoyhttp.Continue` / `envoyhttp.StopIteration` (`types.go:21,23`), `envoyhttp.DataContinue` (`:32`), `envoyhttp.TrailersContinue` (`:46`) — **there is no `FilterHeadersStatusContinue`**. Counters expose `Inc()` (`internal/stats/counter.go:22`) and `Load() uint64` (`:30`).

---

## Task 1: Re-record the baselines in `PROGRESS-56.1.md` + confirm the final ADR-0045 split re-check

**Files:**
- Modify: `docs/envoy-go/phases/56-http-tap-filter/PROGRESS-56.1.md` (**already scaffolded at the PLAN stage** — task checklist, baselines-as-of-`0f82eb75`, the discharged split re-check, and the empty deliberate-break ledger)

**Interfaces:**
- Consumes: nothing.
- Produces: the baseline block every later task reconciles against.

- [ ] **Step 1: Re-run the baselines against THIS IMPL's cold-start HEAD**

The PLAN recorded them against master `0f82eb75`. Do not assume they still hold. Run each and **replace** the `Baseline Counts` block with the literal output:

```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                                  # expect 100
grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l          # expect 52
grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1                    # expect ADR-0272
grep -n 'BackendKind = ' test/differential/fixture/fixture.go | tail -1            # expect 38
```

- [ ] **Step 2: Confirm the split re-check still holds**

`PROGRESS-56.1.md` already carries the discharged ADR-0045 re-check (15 tasks, margin zero). Read it. **If your IMPL is about to add a 16th task, stop and re-open the gate** — do not proceed and do not widen 56.2.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/56-http-tap-filter/PROGRESS-56.1.md
git commit -m "phase 56.1 T1: re-record baselines against the IMPL cold-start HEAD; confirm the ADR-0045 gate"
```

---

## Task 2: `internal/headermatch` — the private `stringMatcher` (5 arms + `ignore_case`) [TDD]

**Files:**
- Create: `internal/headermatch/stringmatch.go`
- Test: `internal/headermatch/stringmatch_test.go`

**Interfaces:**
- Consumes: `tmatcherv3.StringMatcher`, `tmatcherv3.RegexMatcher`.
- Produces: `func newStringMatcher(sm *tmatcherv3.StringMatcher) (*stringMatcher, error)` and `func (s *stringMatcher) match(v string) bool` — both package-private, consumed by Task 3.

**Semantics (Envoy):** `ignore_case` applies to `exact`/`prefix`/`suffix`/`contains` and is **ignored for `safe_regex`**. `safe_regex` is a **full match** (anchored both ends). The `custom` arm (f8) is an **explicit reject**.

- [ ] **Step 1: Write the failing test**

`internal/headermatch/stringmatch_test.go`:

```go
package headermatch

import (
	"testing"

	tmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

func TestStringMatcher_Arms(t *testing.T) {
	re := func(s string) *tmatcherv3.RegexMatcher {
		return &tmatcherv3.RegexMatcher{
			EngineType: &tmatcherv3.RegexMatcher_GoogleRe2{GoogleRe2: &tmatcherv3.RegexMatcher_GoogleRE2{}},
			Regex:      s,
		}
	}
	cases := []struct {
		name  string
		sm    *tmatcherv3.StringMatcher
		value string
		want  bool
	}{
		{"exact_hit", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}}, "yes", true},
		{"exact_miss", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}}, "no", false},
		{"exact_case_sensitive", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}}, "YES", false},
		{"exact_ignore_case", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}, IgnoreCase: true}, "YES", true},
		{"prefix_hit", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Prefix{Prefix: "ab"}}, "abc", true},
		{"prefix_ignore_case", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Prefix{Prefix: "ab"}, IgnoreCase: true}, "ABC", true},
		{"suffix_hit", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Suffix{Suffix: "bc"}}, "abc", true},
		{"contains_hit", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Contains{Contains: "b"}}, "abc", true},
		{"contains_ignore_case", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Contains{Contains: "b"}, IgnoreCase: true}, "ABC", true},
		{"safe_regex_full_match", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_SafeRegex{SafeRegex: re("a.c")}}, "abc", true},
		{"safe_regex_is_anchored", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_SafeRegex{SafeRegex: re("b")}}, "abc", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := newStringMatcher(tc.sm)
			if err != nil {
				t.Fatalf("newStringMatcher: %v", err)
			}
			if got := m.match(tc.value); got != tc.want {
				t.Errorf("match(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestStringMatcher_Rejects(t *testing.T) {
	cases := []struct {
		name string
		sm   *tmatcherv3.StringMatcher
	}{
		{"nil", nil},
		{"no_arm", &tmatcherv3.StringMatcher{}},
		{"custom_arm", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_Custom{}}},
		{"bad_regex", &tmatcherv3.StringMatcher{MatchPattern: &tmatcherv3.StringMatcher_SafeRegex{
			SafeRegex: &tmatcherv3.RegexMatcher{Regex: "a(("},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newStringMatcher(tc.sm); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/headermatch/ -count=1
```
Expected: FAIL — `undefined: newStringMatcher`.

- [ ] **Step 3: Implement `stringmatch.go`**

```go
// Package headermatch evaluates Envoy `config/route/v3.HeaderMatcher` protos
// against a request/response header map.
//
// CONTRACT: every Match/match call takes a LOWERCASE-KEYED http.Header. Use
// Lowercase to normalize before matching. Raw map indexing is used throughout
// (never http.Header.Get), because Go's textproto canonicalization does not
// preserve the leading colon of pseudo-headers such as ":status".
package headermatch

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	tmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
)

type smKind int

const (
	smExact smKind = iota
	smPrefix
	smSuffix
	smContains
	smSafeRegex
)

// stringMatcher is a compiled type/matcher/v3.StringMatcher.
type stringMatcher struct {
	kind       smKind
	s          string // already case-folded when ignoreCase
	re         *regexp.Regexp
	ignoreCase bool
}

func newStringMatcher(sm *tmatcherv3.StringMatcher) (*stringMatcher, error) {
	if sm == nil {
		return nil, errors.New("headermatch: nil string_match")
	}
	out := &stringMatcher{ignoreCase: sm.GetIgnoreCase()}
	fold := func(s string) string {
		if out.ignoreCase {
			return strings.ToLower(s)
		}
		return s
	}
	switch p := sm.GetMatchPattern().(type) {
	case *tmatcherv3.StringMatcher_Exact:
		out.kind, out.s = smExact, fold(p.Exact)
	case *tmatcherv3.StringMatcher_Prefix:
		out.kind, out.s = smPrefix, fold(p.Prefix)
	case *tmatcherv3.StringMatcher_Suffix:
		out.kind, out.s = smSuffix, fold(p.Suffix)
	case *tmatcherv3.StringMatcher_Contains:
		out.kind, out.s = smContains, fold(p.Contains)
	case *tmatcherv3.StringMatcher_SafeRegex:
		// ignore_case is ignored for safe_regex (Envoy semantics).
		re, err := regexp.Compile("^(?:" + p.SafeRegex.GetRegex() + ")$")
		if err != nil {
			return nil, fmt.Errorf("headermatch: safe_regex: %w", err)
		}
		out.kind, out.re = smSafeRegex, re
	case *tmatcherv3.StringMatcher_Custom:
		return nil, errors.New("headermatch: string_match.custom is not supported")
	case nil:
		return nil, errors.New("headermatch: string_match has no match_pattern set")
	default:
		return nil, fmt.Errorf("headermatch: unsupported string_match arm %T", p)
	}
	return out, nil
}

func (m *stringMatcher) match(v string) bool {
	if m.kind == smSafeRegex {
		return m.re.MatchString(v)
	}
	if m.ignoreCase {
		v = strings.ToLower(v)
	}
	switch m.kind {
	case smExact:
		return v == m.s
	case smPrefix:
		return strings.HasPrefix(v, m.s)
	case smSuffix:
		return strings.HasSuffix(v, m.s)
	case smContains:
		return strings.Contains(v, m.s)
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/headermatch/ -count=1
```
Expected: `ok`.

- [ ] **Step 5: Gates + commit**

```bash
gofmt -l internal/headermatch/            # must print NOTHING
golangci-lint run ./internal/headermatch/
go vet ./... && go build ./...
git add internal/headermatch/
git commit -m "phase 56.1 T2: internal/headermatch stringMatcher (5 arms + ignore_case, custom rejected)"
```

---

## Task 3: `internal/headermatch` — the exported `Matcher` (8 arms + `invert_match` + `treat_missing_header_as_empty`) + `Lowercase` [TDD]

**Files:**
- Create: `internal/headermatch/headermatch.go`
- Test: `internal/headermatch/headermatch_test.go`

**Interfaces:**
- Consumes: `newStringMatcher`/`(*stringMatcher).match` from Task 2.
- Produces:
  - `func New(hm *routev3.HeaderMatcher) (*Matcher, error)`
  - `func (m *Matcher) Match(h http.Header) bool` — **`h` MUST be lowercase-keyed**
  - `func Lowercase(h http.Header) http.Header` — returns a fresh lowercase-keyed COPY; **never mutates `h`**

**Semantics:** multi-value headers are joined with `","` before evaluation (Envoy). `present_match: true` ⇒ matches iff present; `present_match: false` ⇒ matches iff absent. `treat_missing_header_as_empty` makes a missing header evaluate as `""` for the value arms (it does **not** apply to `present_match`). `invert_match` negates the final result. `range_match` parses the joined value as a base-10 `int64` and tests `[start, end)`; a missing or unparsable value is `false`.

- [ ] **Step 1: Write the failing test**

`internal/headermatch/headermatch_test.go`:

```go
package headermatch

import (
	"net/http"
	"testing"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	tmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func hdr(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h[kv[i]] = append(h[kv[i]], kv[i+1]) // raw: keys are already lowercase
	}
	return h
}

func TestLowercase_CopiesAndLowers(t *testing.T) {
	src := http.Header{"X-Tap": {"yes"}, ":method": {"GET"}}
	got := Lowercase(src)
	if _, ok := got["x-tap"]; !ok {
		t.Errorf("want lowercased key x-tap; got %v", got)
	}
	if _, ok := got[":method"]; !ok {
		t.Errorf("want pseudo-header :method preserved; got %v", got)
	}
	got["x-new"] = []string{"1"}
	if _, leaked := src["x-new"]; leaked {
		t.Errorf("Lowercase must return a COPY; source was mutated")
	}
}

func TestMatcher_Arms(t *testing.T) {
	cases := []struct {
		name string
		hm   *routev3.HeaderMatcher
		h    http.Header
		want bool
	}{
		{"exact_hit", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "yes"}}, hdr("x-tap", "yes"), true},
		{"exact_miss", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "yes"}}, hdr("x-tap", "no"), false},
		{"pseudo_status_exact", &routev3.HeaderMatcher{Name: ":status",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "204"}}, hdr(":status", "204"), true},
		{"present_true_hit", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: true}}, hdr("x-tap", "no"), true},
		{"present_true_absent", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: true}}, hdr(), false},
		{"present_false_absent", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: false}}, hdr(), true},
		{"prefix", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_PrefixMatch{PrefixMatch: "ye"}}, hdr("x-tap", "yes"), true},
		{"suffix", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_SuffixMatch{SuffixMatch: "es"}}, hdr("x-tap", "yes"), true},
		{"contains", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ContainsMatch{ContainsMatch: "e"}}, hdr("x-tap", "yes"), true},
		{"safe_regex", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_SafeRegexMatch{SafeRegexMatch: &tmatcherv3.RegexMatcher{Regex: "y.s"}}}, hdr("x-tap", "yes"), true},
		{"string_match", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_StringMatch{StringMatch: &tmatcherv3.StringMatcher{
				MatchPattern: &tmatcherv3.StringMatcher_Exact{Exact: "yes"}}}}, hdr("x-tap", "yes"), true},
		{"range_in", &routev3.HeaderMatcher{Name: "x-n",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_RangeMatch{RangeMatch: &typev3.Int64Range{Start: 1, End: 10}}}, hdr("x-n", "5"), true},
		{"range_end_exclusive", &routev3.HeaderMatcher{Name: "x-n",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_RangeMatch{RangeMatch: &typev3.Int64Range{Start: 1, End: 10}}}, hdr("x-n", "10"), false},
		{"range_unparsable", &routev3.HeaderMatcher{Name: "x-n",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_RangeMatch{RangeMatch: &typev3.Int64Range{Start: 1, End: 10}}}, hdr("x-n", "abc"), false},
		{"multi_value_joined", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "a,b"}},
			http.Header{"x-tap": {"a", "b"}}, true},
		{"invert", &routev3.HeaderMatcher{Name: "x-tap", InvertMatch: true,
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "yes"}}, hdr("x-tap", "no"), true},
		{"missing_no_treat", &routev3.HeaderMatcher{Name: "x-tap",
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: ""}}, hdr(), false},
		{"missing_treat_as_empty", &routev3.HeaderMatcher{Name: "x-tap", TreatMissingHeaderAsEmpty: true,
			HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: ""}}, hdr(), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := New(tc.hm)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := m.Match(tc.h); got != tc.want {
				t.Errorf("Match() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatcher_Rejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		hm   *routev3.HeaderMatcher
	}{
		{"nil", nil},
		{"empty_name", &routev3.HeaderMatcher{HeaderMatchSpecifier: &routev3.HeaderMatcher_PresentMatch{PresentMatch: true}}},
		{"no_arm", &routev3.HeaderMatcher{Name: "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.hm); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/headermatch/ -count=1
```
Expected: FAIL — `undefined: New`, `undefined: Lowercase`.

- [ ] **Step 3: Implement `headermatch.go`**

```go
package headermatch

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

// Lowercase returns a fresh http.Header whose keys are all lowercase. The input
// is never mutated, and no value slice is aliased. Pseudo-header keys (":method")
// are already lowercase and pass through unchanged.
//
// Two source keys that fold to the same lowercase key (e.g. "X-Tap" and "x-tap")
// have their values MERGED, in map-iteration order. That order is not
// deterministic, but HCM never produces such a pair: it canonicalizes ordinary
// headers and writes pseudo-headers by raw assignment.
func Lowercase(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		lk := strings.ToLower(k)
		// append onto a nil/existing slice copies the elements; it never aliases v.
		out[lk] = append(out[lk], v...)
	}
	return out
}

type hmKind int

const (
	hmString hmKind = iota // exact/prefix/suffix/contains/safe_regex/string_match, all via stringMatcher
	hmPresent
	hmRange
)

// Matcher is a compiled config/route/v3.HeaderMatcher.
type Matcher struct {
	name         string // lowercased
	kind         hmKind
	sm           *stringMatcher
	presentWant  bool
	rangeStart   int64
	rangeEnd     int64
	invert       bool
	treatMissing bool
}

// New compiles one HeaderMatcher. All 8 HeaderMatchSpecifier arms are accepted
// (the 5 deprecated ones included); an unset oneof is an error.
func New(hm *routev3.HeaderMatcher) (*Matcher, error) {
	if hm == nil {
		return nil, errors.New("headermatch: nil HeaderMatcher")
	}
	if hm.GetName() == "" {
		return nil, errors.New("headermatch: HeaderMatcher.name is required")
	}
	m := &Matcher{
		name:         strings.ToLower(hm.GetName()),
		invert:       hm.GetInvertMatch(),
		treatMissing: hm.GetTreatMissingHeaderAsEmpty(),
	}
	switch a := hm.GetHeaderMatchSpecifier().(type) {
	case *routev3.HeaderMatcher_ExactMatch: // f4 (deprecated)
		m.kind, m.sm = hmString, &stringMatcher{kind: smExact, s: a.ExactMatch}
	case *routev3.HeaderMatcher_PrefixMatch: // f9 (deprecated)
		m.kind, m.sm = hmString, &stringMatcher{kind: smPrefix, s: a.PrefixMatch}
	case *routev3.HeaderMatcher_SuffixMatch: // f10 (deprecated)
		m.kind, m.sm = hmString, &stringMatcher{kind: smSuffix, s: a.SuffixMatch}
	case *routev3.HeaderMatcher_ContainsMatch: // f12 (deprecated)
		m.kind, m.sm = hmString, &stringMatcher{kind: smContains, s: a.ContainsMatch}
	case *routev3.HeaderMatcher_SafeRegexMatch: // f11 (deprecated)
		// NOTE: this arm carries a bare *tmatcherv3.RegexMatcher, NOT a
		// StringMatcher — compile it directly. safe_regex is a FULL match.
		if a.SafeRegexMatch == nil {
			return nil, errors.New("headermatch: nil safe_regex_match")
		}
		re, err := regexp.Compile("^(?:" + a.SafeRegexMatch.GetRegex() + ")$")
		if err != nil {
			return nil, fmt.Errorf("headermatch: safe_regex_match: %w", err)
		}
		m.kind, m.sm = hmString, &stringMatcher{kind: smSafeRegex, re: re}
	case *routev3.HeaderMatcher_StringMatch: // f13
		sm, err := newStringMatcher(a.StringMatch)
		if err != nil {
			return nil, err
		}
		m.kind, m.sm = hmString, sm
	case *routev3.HeaderMatcher_PresentMatch: // f7
		m.kind, m.presentWant = hmPresent, a.PresentMatch
	case *routev3.HeaderMatcher_RangeMatch: // f6
		if a.RangeMatch == nil {
			return nil, errors.New("headermatch: nil range_match")
		}
		m.kind = hmRange
		m.rangeStart, m.rangeEnd = a.RangeMatch.GetStart(), a.RangeMatch.GetEnd()
	case nil:
		return nil, errors.New("headermatch: HeaderMatcher has no header_match_specifier set")
	default:
		return nil, fmt.Errorf("headermatch: unsupported header_match_specifier arm %T", a)
	}
	return m, nil
}

// Match reports whether h satisfies the matcher. h MUST be lowercase-keyed
// (see Lowercase). Raw map indexing is used so pseudo-headers are visible.
func (m *Matcher) Match(h http.Header) bool {
	vals, present := h[m.name]
	var res bool
	switch {
	case m.kind == hmPresent:
		res = present == m.presentWant
	case !present && !m.treatMissing:
		res = false
	default:
		v := ""
		if present {
			v = strings.Join(vals, ",")
		}
		res = m.evalValue(v)
	}
	if m.invert {
		return !res
	}
	return res
}

func (m *Matcher) evalValue(v string) bool {
	switch m.kind {
	case hmString:
		return m.sm.match(v)
	case hmRange:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return false
		}
		return n >= m.rangeStart && n < m.rangeEnd
	}
	return false
}
```

> **Two traps in the arms above.** (i) `safe_regex_match` (f11) carries a bare `*tmatcherv3.RegexMatcher`, **not** a `StringMatcher` — it is compiled directly, and `ignore_case` does not apply to it. (ii) The four deprecated string arms (`exact`/`prefix`/`suffix`/`contains`) carry a bare `string` with **no** `ignore_case` field, so they construct a `stringMatcher` literal with `ignoreCase` left false — they must NOT be routed through `newStringMatcher`, which expects a `*tmatcherv3.StringMatcher`. Only the f13 `string_match` arm goes through `newStringMatcher`.
>
> The imports for `headermatch.go` are therefore: `errors`, `fmt`, `net/http`, `regexp`, `strconv`, `strings`, and `routev3` — **no `tmatcherv3`** (that import lives only in `stringmatch.go`).

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/headermatch/ -count=1 -v
```
Expected: `ok`, all subtests PASS.

- [ ] **Step 5: Gates + commit**

```bash
gofmt -l internal/headermatch/ && golangci-lint run ./internal/headermatch/ && go vet ./... && go build ./...
git add internal/headermatch/
git commit -m "phase 56.1 T3: internal/headermatch Matcher (8 arms + invert_match + treat_missing_header_as_empty) + Lowercase"
```

---

## Task 4: `internal/matchpredicate` — node types + `Compile` (6 accept / 4 explicit reject / depth cap 32) [TDD]

**Files:**
- Create: `internal/matchpredicate/node.go`, `internal/matchpredicate/compile.go`
- Test: `internal/matchpredicate/compile_test.go`

**Interfaces:**
- Consumes: `headermatch.New`, `headermatch.Matcher`.
- Produces:
  - `type Value uint8` with `Undetermined`, `True`, `False`
  - `const MaxDepth = 32`
  - `var ErrDepthExceeded, ErrTrailersUnsupported, ErrGenericBodyUnsupported error`
  - `func Compile(mp *cmatcherv3.MatchPredicate) (*Program, error)` — `*Program` is **immutable and safe to share across streams**
  - `func (p *Program) NewEvaluator() *Evaluator` (implemented in Task 5)

**Design pin (a deliberate, semantics-preserving simplification).** The SPEC describes "incremental evaluation". The compiled `Program` holds **no mutable state**: each node is a pure function of the two header maps, which the `Evaluator` records as it goes. Evaluation happens once, at `Resolve()`. This is semantically identical to incremental evaluation (the tree is a pure function of its inputs), makes `Program` shareable across concurrent streams with no cloning and no locking, and makes early emission **structurally impossible** — there is no intermediate value to read. The tri-state remains load-bearing: if `EncodeHeaders` never ran, response leaves are `Undetermined` and `Resolve()` maps that to `false`.

- [ ] **Step 1: Write the failing test**

`internal/matchpredicate/compile_test.go`:

```go
package matchpredicate

import (
	"errors"
	"testing"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

func reqHdr(name, exact string) *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestHeadersMatch{
		HttpRequestHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
			Name: name, HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: exact},
		}}},
	}}
}

func anyMatch() *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_AnyMatch{AnyMatch: true}}
}

// nest builds depth levels of not_match wrapping an any_match leaf.
// depth==1 returns the bare leaf.
func nest(depth int) *cmatcherv3.MatchPredicate {
	p := anyMatch()
	for i := 1; i < depth; i++ {
		p = &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_NotMatch{NotMatch: p}}
	}
	return p
}

func TestCompile_AcceptsSixArms(t *testing.T) {
	accepts := map[string]*cmatcherv3.MatchPredicate{
		"any_match":  anyMatch(),
		"not_match":  {Rule: &cmatcherv3.MatchPredicate_NotMatch{NotMatch: anyMatch()}},
		"or_match":   {Rule: &cmatcherv3.MatchPredicate_OrMatch{OrMatch: &cmatcherv3.MatchPredicate_MatchSet{Rules: []*cmatcherv3.MatchPredicate{anyMatch(), anyMatch()}}}},
		"and_match":  {Rule: &cmatcherv3.MatchPredicate_AndMatch{AndMatch: &cmatcherv3.MatchPredicate_MatchSet{Rules: []*cmatcherv3.MatchPredicate{anyMatch(), anyMatch()}}}},
		"http_request_headers_match":  reqHdr("x-tap", "yes"),
		"http_response_headers_match": {Rule: &cmatcherv3.MatchPredicate_HttpResponseHeadersMatch{
			HttpResponseHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
				Name: ":status", HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "204"}}}}}},
	}
	for name, mp := range accepts {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(mp); err != nil {
				t.Errorf("Compile(%s) = %v, want nil", name, err)
			}
		})
	}
}

func TestCompile_RejectsFourArms(t *testing.T) {
	hh := &cmatcherv3.HttpHeadersMatch{}
	gb := &cmatcherv3.HttpGenericBodyMatch{}
	cases := map[string]struct {
		mp   *cmatcherv3.MatchPredicate
		want error
	}{
		"http_request_trailers_match":  {&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestTrailersMatch{HttpRequestTrailersMatch: hh}}, ErrTrailersUnsupported},
		"http_response_trailers_match": {&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpResponseTrailersMatch{HttpResponseTrailersMatch: hh}}, ErrTrailersUnsupported},
		"http_request_generic_body_match":  {&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestGenericBodyMatch{HttpRequestGenericBodyMatch: gb}}, ErrGenericBodyUnsupported},
		"http_response_generic_body_match": {&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpResponseGenericBodyMatch{HttpResponseGenericBodyMatch: gb}}, ErrGenericBodyUnsupported},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Compile(tc.mp)
			if !errors.Is(err, tc.want) {
				t.Errorf("Compile(%s) err = %v, want %v", name, err, tc.want)
			}
		})
	}
}

func TestCompile_StructuralRejects(t *testing.T) {
	for name, mp := range map[string]*cmatcherv3.MatchPredicate{
		"nil":         nil,
		"no_rule":     {},
		"empty_rules": {Rule: &cmatcherv3.MatchPredicate_AndMatch{AndMatch: &cmatcherv3.MatchPredicate_MatchSet{}}},
		"nil_not":     {Rule: &cmatcherv3.MatchPredicate_NotMatch{}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Compile(mp); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestCompile_DepthCap(t *testing.T) {
	if _, err := Compile(nest(MaxDepth)); err != nil {
		t.Errorf("depth %d must compile, got %v", MaxDepth, err)
	}
	_, err := Compile(nest(MaxDepth + 1))
	if !errors.Is(err, ErrDepthExceeded) {
		t.Errorf("depth %d: err = %v, want ErrDepthExceeded", MaxDepth+1, err)
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/matchpredicate/ -count=1
```
Expected: FAIL — `undefined: Compile`, `undefined: MaxDepth`, ….

- [ ] **Step 3: Implement `node.go`**

```go
// Package matchpredicate compiles a config/common/matcher/v3.MatchPredicate
// proto tree into an immutable, evaluable node tree, then evaluates it against
// a stream's request and response headers.
//
// Six of the ten oneof arms are supported. The four unsupported arms
// (request/response trailers, request/response generic_body) are EXPLICIT
// compile-time rejects, never a silent fall-through (ADR-0080).
//
// A compiled *Program holds NO mutable state and is safe to share across
// concurrent streams. Per-stream state lives in an *Evaluator.
package matchpredicate

import "net/http"

// Value is the tri-state of a predicate node.
type Value uint8

const (
	// Undetermined means the inputs this node depends on have not arrived yet.
	Undetermined Value = iota
	// True means the node matched.
	True
	// False means the node did not match.
	False
)

// state carries the per-stream inputs a node evaluates against.
type state struct {
	reqHdrs  http.Header // lowercase-keyed
	respHdrs http.Header // lowercase-keyed
	reqSeen  bool
	respSeen bool
}

type node interface{ eval(st *state) Value }

// anyNode is `any_match`: always True. Envoy constructs an always-true matcher
// whenever the arm is SET, regardless of the bool's value; we mirror that.
type anyNode struct{}

func (anyNode) eval(*state) Value { return True }

type notNode struct{ child node }

func (n notNode) eval(st *state) Value {
	switch n.child.eval(st) {
	case True:
		return False
	case False:
		return True
	}
	return Undetermined
}

type andNode struct{ children []node }

func (n andNode) eval(st *state) Value {
	sawUndetermined := false
	for _, c := range n.children {
		switch c.eval(st) {
		case False:
			return False // short-circuit: one False decides the conjunction
		case Undetermined:
			sawUndetermined = true
		}
	}
	if sawUndetermined {
		return Undetermined
	}
	return True
}

type orNode struct{ children []node }

func (n orNode) eval(st *state) Value {
	sawUndetermined := false
	for _, c := range n.children {
		switch c.eval(st) {
		case True:
			return True // short-circuit: one True decides the disjunction
		case Undetermined:
			sawUndetermined = true
		}
	}
	if sawUndetermined {
		return Undetermined
	}
	return False
}

// headersNode matches ALL of its matchers against one direction's headers
// (Envoy's HttpHeadersMatch is a conjunction).
type headersNode struct {
	matchers []headerMatcher
	response bool // false => request headers, true => response headers
}

func (n headersNode) eval(st *state) Value {
	h, seen := st.reqHdrs, st.reqSeen
	if n.response {
		h, seen = st.respHdrs, st.respSeen
	}
	if !seen {
		return Undetermined
	}
	for _, m := range n.matchers {
		if !m.Match(h) {
			return False
		}
	}
	return True
}
```

> `headerMatcher` is an interface alias so the package does not leak its dependency into node signatures:
> ```go
> type headerMatcher interface{ Match(http.Header) bool }
> ```
> `*headermatch.Matcher` satisfies it.

- [ ] **Step 4: Implement `compile.go`**

```go
package matchpredicate

import (
	"errors"
	"fmt"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"

	"github.com/pgdad/envoy-go/internal/headermatch"
)

// MaxDepth caps MatchPredicate nesting (root = depth 1). The compiler is
// recursive over an attacker-influenceable proto; without a cap a deeply
// nested not_match chain would exhaust the stack. Exercised by
// FuzzMatchPredicateCompile.
const MaxDepth = 32

var (
	// ErrDepthExceeded is returned when nesting exceeds MaxDepth.
	ErrDepthExceeded = errors.New("matchpredicate: nesting depth exceeds MaxDepth")
	// ErrTrailersUnsupported rejects the two trailer arms (f6/f8): envoy-go's
	// HTTP filters cannot observe trailers (the never-done HCM "Task 18").
	ErrTrailersUnsupported = errors.New("matchpredicate: http_{request,response}_trailers_match is not supported")
	// ErrGenericBodyUnsupported rejects the two generic_body arms (f9/f10).
	ErrGenericBodyUnsupported = errors.New("matchpredicate: http_{request,response}_generic_body_match is not supported")
)

// Program is an immutable compiled predicate tree, safe to share across streams.
type Program struct{ root node }

// Compile builds a Program from a MatchPredicate proto, rejecting every
// unsupported arm explicitly.
func Compile(mp *cmatcherv3.MatchPredicate) (*Program, error) {
	root, err := compileNode(mp, 1)
	if err != nil {
		return nil, err
	}
	return &Program{root: root}, nil
}

func compileNode(mp *cmatcherv3.MatchPredicate, depth int) (node, error) {
	if depth > MaxDepth {
		return nil, fmt.Errorf("%w (max %d)", ErrDepthExceeded, MaxDepth)
	}
	if mp == nil {
		return nil, errors.New("matchpredicate: nil MatchPredicate")
	}
	switch r := mp.GetRule().(type) {
	case *cmatcherv3.MatchPredicate_AnyMatch: // f4
		return anyNode{}, nil
	case *cmatcherv3.MatchPredicate_NotMatch: // f3
		child, err := compileNode(r.NotMatch, depth+1)
		if err != nil {
			return nil, err
		}
		return notNode{child: child}, nil
	case *cmatcherv3.MatchPredicate_OrMatch: // f1
		kids, err := compileSet(r.OrMatch, depth)
		if err != nil {
			return nil, err
		}
		return orNode{children: kids}, nil
	case *cmatcherv3.MatchPredicate_AndMatch: // f2
		kids, err := compileSet(r.AndMatch, depth)
		if err != nil {
			return nil, err
		}
		return andNode{children: kids}, nil
	case *cmatcherv3.MatchPredicate_HttpRequestHeadersMatch: // f5
		return compileHeaders(r.HttpRequestHeadersMatch, false)
	case *cmatcherv3.MatchPredicate_HttpResponseHeadersMatch: // f7
		return compileHeaders(r.HttpResponseHeadersMatch, true)

	// ---- EXPLICIT rejects (never a fall-through default) ----
	case *cmatcherv3.MatchPredicate_HttpRequestTrailersMatch: // f6
		return nil, ErrTrailersUnsupported
	case *cmatcherv3.MatchPredicate_HttpResponseTrailersMatch: // f8
		return nil, ErrTrailersUnsupported
	case *cmatcherv3.MatchPredicate_HttpRequestGenericBodyMatch: // f9
		return nil, ErrGenericBodyUnsupported
	case *cmatcherv3.MatchPredicate_HttpResponseGenericBodyMatch: // f10
		return nil, ErrGenericBodyUnsupported

	case nil:
		return nil, errors.New("matchpredicate: MatchPredicate has no rule set")
	default:
		return nil, fmt.Errorf("matchpredicate: unsupported rule arm %T", r)
	}
}

func compileSet(ms *cmatcherv3.MatchPredicate_MatchSet, depth int) ([]node, error) {
	if ms == nil || len(ms.GetRules()) == 0 {
		// An empty conjunction/disjunction has no unambiguous meaning; reject it.
		// NOTE: Envoy's PGV declares min_items:2 on MatchSet.rules. That bound was
		// NOT probed, so we reject only the unambiguous 0 case and accept 1.
		return nil, errors.New("matchpredicate: {or,and}_match.rules must be non-empty")
	}
	kids := make([]node, 0, len(ms.GetRules()))
	for i, r := range ms.GetRules() {
		k, err := compileNode(r, depth+1)
		if err != nil {
			return nil, fmt.Errorf("rules[%d]: %w", i, err)
		}
		kids = append(kids, k)
	}
	return kids, nil
}

func compileHeaders(hm *cmatcherv3.HttpHeadersMatch, response bool) (node, error) {
	if hm == nil || len(hm.GetHeaders()) == 0 {
		return nil, errors.New("matchpredicate: http_*_headers_match.headers must be non-empty")
	}
	ms := make([]headerMatcher, 0, len(hm.GetHeaders()))
	for i, h := range hm.GetHeaders() {
		m, err := headermatch.New(h)
		if err != nil {
			return nil, fmt.Errorf("headers[%d]: %w", i, err)
		}
		ms = append(ms, m)
	}
	return headersNode{matchers: ms, response: response}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/matchpredicate/ -count=1 -v
```
Expected: `ok`; `TestCompile_DepthCap` PASSes both halves.

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l internal/matchpredicate/ && golangci-lint run ./internal/matchpredicate/ && go vet ./... && go build ./...
git add internal/matchpredicate/
git commit -m "phase 56.1 T4: internal/matchpredicate node types + Compile (6 accept, 4 explicit rejects, depth cap 32)"
```

---

## Task 5: `internal/matchpredicate` — the `Evaluator` (feed + tri-state `Resolve`) [TDD]

**Files:**
- Create: `internal/matchpredicate/eval.go`
- Test: `internal/matchpredicate/eval_test.go`

**Interfaces:**
- Consumes: `*Program` from Task 4.
- Produces:
  - `func (p *Program) NewEvaluator() *Evaluator`
  - `func (e *Evaluator) FeedRequestHeaders(h http.Header)` — `h` lowercase-keyed
  - `func (e *Evaluator) FeedResponseHeaders(h http.Header)`
  - `func (e *Evaluator) Value() Value` — the tri-state, for tests
  - `func (e *Evaluator) Resolve() bool` — **still-`Undetermined` ⇒ `false`**

- [ ] **Step 1: Write the failing test**

`internal/matchpredicate/eval_test.go`:

```go
package matchpredicate

import (
	"net/http"
	"testing"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

func respHdr(name, exact string) *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpResponseHeadersMatch{
		HttpResponseHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
			Name: name, HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: exact}}}},
	}}
}

// The 0099 predicate.
func andReqResp(t *testing.T) *Program {
	t.Helper()
	p, err := Compile(&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_AndMatch{
		AndMatch: &cmatcherv3.MatchPredicate_MatchSet{Rules: []*cmatcherv3.MatchPredicate{
			reqHdr("x-tap", "yes"), respHdr(":status", "204"),
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return p
}

func TestEvaluator_TriState_RequestOnlyIsUndetermined(t *testing.T) {
	e := andReqResp(t).NewEvaluator()
	if got := e.Value(); got != Undetermined {
		t.Errorf("before any feed: Value() = %v, want Undetermined", got)
	}
	e.FeedRequestHeaders(http.Header{"x-tap": {"yes"}})
	if got := e.Value(); got != Undetermined {
		t.Errorf("after request feed only: Value() = %v, want Undetermined", got)
	}
	// A never-arriving response resolves the whole tree to false.
	if e.Resolve() {
		t.Errorf("Resolve() with no response = true, want false")
	}
}

func TestEvaluator_AndMatch_BothArms(t *testing.T) {
	e := andReqResp(t).NewEvaluator()
	e.FeedRequestHeaders(http.Header{"x-tap": {"yes"}})
	e.FeedResponseHeaders(http.Header{":status": {"204"}})
	if !e.Resolve() {
		t.Errorf("Resolve() = false, want true")
	}
}

func TestEvaluator_AndMatch_RequestArmFalse_ShortCircuitsToFalseEarly(t *testing.T) {
	e := andReqResp(t).NewEvaluator()
	e.FeedRequestHeaders(http.Header{"x-tap": {"no"}})
	// One False decides a conjunction even while the response arm is Undetermined.
	if got := e.Value(); got != False {
		t.Errorf("Value() = %v, want False", got)
	}
	if e.Resolve() {
		t.Errorf("Resolve() = true, want false")
	}
}

// The `orshort` probe, in unit form: a TRUE request arm resolves an or_match to
// True even though the response arm never becomes true. This pins that the
// tri-state governs RESOLUTION only; it must NOT be read as a licence to emit
// early (emission is unconditionally at stream end -- see the tap filter).
func TestEvaluator_OrMatch_RequestArmTrue(t *testing.T) {
	p, err := Compile(&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_OrMatch{
		OrMatch: &cmatcherv3.MatchPredicate_MatchSet{Rules: []*cmatcherv3.MatchPredicate{
			reqHdr("x-tap", "yes"), respHdr(":status", "999"),
		}},
	}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	e := p.NewEvaluator()
	e.FeedRequestHeaders(http.Header{"x-tap": {"yes"}})
	if got := e.Value(); got != True {
		t.Errorf("Value() = %v, want True", got)
	}
	e.FeedResponseHeaders(http.Header{":status": {"204"}})
	if !e.Resolve() {
		t.Errorf("Resolve() = false, want true")
	}
}

func TestEvaluator_NotMatch_UndeterminedPassesThrough(t *testing.T) {
	p, err := Compile(&cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_NotMatch{
		NotMatch: respHdr(":status", "204")}})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	e := p.NewEvaluator()
	if got := e.Value(); got != Undetermined {
		t.Errorf("Value() = %v, want Undetermined", got)
	}
	e.FeedResponseHeaders(http.Header{":status": {"500"}})
	if !e.Resolve() {
		t.Errorf("not(:status==204) on a 500 = false, want true")
	}
}

// A Program must be reusable across streams with no cross-talk.
func TestProgram_IsImmutableAcrossEvaluators(t *testing.T) {
	p := andReqResp(t)
	a := p.NewEvaluator()
	a.FeedRequestHeaders(http.Header{"x-tap": {"yes"}})
	a.FeedResponseHeaders(http.Header{":status": {"204"}})
	if !a.Resolve() {
		t.Fatalf("evaluator a should match")
	}
	b := p.NewEvaluator()
	b.FeedRequestHeaders(http.Header{"x-tap": {"no"}})
	b.FeedResponseHeaders(http.Header{":status": {"204"}})
	if b.Resolve() {
		t.Errorf("evaluator b must NOT match; Program leaked state")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/matchpredicate/ -run TestEvaluator -count=1
```
Expected: FAIL — `p.NewEvaluator undefined`.

- [ ] **Step 3: Implement `eval.go`**

```go
package matchpredicate

import "net/http"

// Evaluator carries one stream's evaluation state for a shared Program.
type Evaluator struct {
	p  *Program
	st state
}

// NewEvaluator mints per-stream state. The Program is not mutated.
func (p *Program) NewEvaluator() *Evaluator { return &Evaluator{p: p} }

// FeedRequestHeaders records the request headers. h MUST be lowercase-keyed.
func (e *Evaluator) FeedRequestHeaders(h http.Header) {
	e.st.reqHdrs, e.st.reqSeen = h, true
}

// FeedResponseHeaders records the response headers. h MUST be lowercase-keyed.
func (e *Evaluator) FeedResponseHeaders(h http.Header) {
	e.st.respHdrs, e.st.respSeen = h, true
}

// Value returns the current tri-state of the tree.
func (e *Evaluator) Value() Value { return e.p.root.eval(&e.st) }

// Resolve collapses the tri-state to a match decision. A still-Undetermined
// tree resolves to false: this is a total-function guard, near-unreachable for
// HTTP (envoy-go always synthesizes a response, even a local-reply 5xx, so
// EncodeHeaders always runs), NOT an observed reference behavior.
func (e *Evaluator) Resolve() bool { return e.Value() == True }
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/matchpredicate/ -count=1 -v
go test ./internal/matchpredicate/ -count=1 -race
```
Expected: `ok` for both.

- [ ] **Step 5: Gates + commit**

```bash
gofmt -l internal/matchpredicate/ && golangci-lint run ./internal/matchpredicate/ && go vet ./... && go build ./...
git add internal/matchpredicate/
git commit -m "phase 56.1 T5: internal/matchpredicate Evaluator (feed + tri-state Resolve; Undetermined => false)"
```

---

## Task 6: `FuzzMatchPredicateCompile` — the compiler fuzzer (fuzzers 52 → 53) [fuzz]

**Files:**
- Create: `internal/matchpredicate/fuzz_test.go`

**Interfaces:**
- Consumes: `Compile`, `MaxDepth`, `ErrDepthExceeded`.
- Produces: nothing consumed by later tasks.

**Invariant fuzzed:** `Compile` must return **exactly one** of `(*Program, nil)` or `(nil, error)` — never `(nil, nil)`, never both, and **never panic or overflow the stack**. The seed corpus MUST include a depth-33 `not_match` chain so the cap is genuinely exercised (`D-TAP-DEPTHCAP`).

- [ ] **Step 1: Write the fuzzer**

`internal/matchpredicate/fuzz_test.go`:

```go
package matchpredicate

import (
	"errors"
	"testing"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	"google.golang.org/protobuf/proto"
)

// mustMarshal renders a MatchPredicate to wire bytes for the seed corpus.
func mustMarshal(t *testing.T, mp *cmatcherv3.MatchPredicate) []byte {
	t.Helper()
	b, err := proto.Marshal(mp)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	return b
}

// TestFuzzSeed_DepthCapFires is a non-fuzz guard that the depth-33 seed really
// trips the cap. A cap that is never exercised is not a cap (D-TAP-DEPTHCAP).
func TestFuzzSeed_DepthCapFires(t *testing.T) {
	deep := nest(MaxDepth + 1)
	if _, err := Compile(deep); !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("depth %d: err = %v, want ErrDepthExceeded", MaxDepth+1, err)
	}
	// And it survives a round-trip through the wire form the fuzzer feeds.
	var back cmatcherv3.MatchPredicate
	if err := proto.Unmarshal(mustMarshal(t, deep), &back); err != nil {
		t.Fatalf("round-trip unmarshal: %v", err)
	}
	if _, err := Compile(&back); !errors.Is(err, ErrDepthExceeded) {
		t.Errorf("after round-trip: err = %v, want ErrDepthExceeded", err)
	}
}

func FuzzMatchPredicateCompile(f *testing.F) {
	// Structured seeds.
	for _, mp := range []*cmatcherv3.MatchPredicate{
		anyMatch(),
		reqHdr("x-tap", "yes"),
		{Rule: &cmatcherv3.MatchPredicate_NotMatch{NotMatch: anyMatch()}},
		{Rule: &cmatcherv3.MatchPredicate_AndMatch{AndMatch: &cmatcherv3.MatchPredicate_MatchSet{
			Rules: []*cmatcherv3.MatchPredicate{anyMatch(), anyMatch()}}}},
		nest(MaxDepth),     // at the cap: must compile
		nest(MaxDepth + 1), // over the cap: must reject, must not overflow
		nest(512),          // far over: the cap must bound recursion long before the stack does
	} {
		b, err := proto.Marshal(mp)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(b)
	}
	// Unstructured seeds.
	for _, b := range [][]byte{nil, {}, {0x00}, {0xff, 0xff, 0xff, 0xff}, []byte("not-a-proto")} {
		f.Add(b)
	}

	f.Fuzz(func(t *testing.T, b []byte) {
		var mp cmatcherv3.MatchPredicate
		if err := proto.Unmarshal(b, &mp); err != nil {
			return // not our concern: the filter parse layer rejects bad wire bytes
		}
		prog, err := Compile(&mp)
		if prog == nil && err == nil {
			t.Fatalf("Compile returned (nil, nil) for %x — must return exactly one", b)
		}
		if prog != nil && err != nil {
			t.Fatalf("Compile returned both Program and error for %x — exclusive", b)
		}
	})
}
```

- [ ] **Step 2: Run the seed corpus + a bounded fuzz burst**

```bash
go test ./internal/matchpredicate/ -run 'TestFuzzSeed_DepthCapFires|FuzzMatchPredicateCompile' -count=1
go test ./internal/matchpredicate/ -run FuzzMatchPredicateCompile -fuzz FuzzMatchPredicateCompile -fuzztime=30s
```
Expected: `ok`; no crashers; **no stack overflow on the depth-512 seed** (that is the cap doing its job).

- [ ] **Step 3: Reconcile the fuzzer count** (`reference_fuzzer_count_docs_drift`)

```bash
grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l   # expect 53
```
If the number is not 53, **stop and reconcile the documented running total before proceeding** — do not adjust the doc to match a miscount, and do not adjust the count to match the doc. Record the literal output in `PROGRESS-56.1.md`.

- [ ] **Step 4: Gates + commit**

```bash
gofmt -l internal/matchpredicate/ && golangci-lint run ./internal/matchpredicate/ && go vet ./... && go build ./...
git add internal/matchpredicate/fuzz_test.go
git commit -m "phase 56.1 T6: FuzzMatchPredicateCompile (+depth-33/512 seeds exercising the cap); fuzzers 52 -> 53"
```

---

## Task 7: `internal/filter/http/tap` — config parse + the FULL §6 reject roster + `rq_tapped` (+1 delta guard) [TDD]

**Files:**
- Create: `internal/filter/http/tap/config.go`
- Test: `internal/filter/http/tap/config_test.go`

**Interfaces:**
- Consumes: `matchpredicate.Compile`, `newFilePerTapSink` (Task 10 — stub it in this task as `func newFilePerTapSink(prefix string) (*filePerTapSink, error)` returning a value with only `pathPrefix` set; Task 10 fills in `write`).
- Produces:
  - `const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.tap.v3.Tap"`
  - `const filterName = "envoy.filters.http.tap"`
  - `type config struct { prog *matchpredicate.Program; sink *filePerTapSink; rqTapped *stats.Counter; recordConn bool }`
  - `func parseConfig(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (*config, error)`

**Format pin (a PLAN decision the SPEC left implicit).** `OutputSink.format` defaults to `JSON_BODY_AS_BYTES` (**enum value 0** — the proto default, emitted when the field is omitted). At 56.1 the `body` field is **never populated**, so `JSON_BODY_AS_BYTES` and `JSON_BODY_AS_STRING` render **byte-identically**. Therefore **both JSON formats are ACCEPTED** (rejecting the proto default would be a gratuitous departure), and the **three `PROTO_*` formats are explicit rejects**. 56.2, which introduces bodies, is where the two JSON arms become distinguishable and must diverge.

**The reject roster (SPEC §6), each an explicit arm — never a fall-through `default`:**

| Condition | Parity |
|---|---|
| `record_headers_received_time: true` | DEPARTURE (D-TAP-RECORDFLAGS) |
| `common_config.admin_config` | DEPARTURE |
| `common_config` unset / no `config_type` | — |
| `static_config.match_config` set at all | DEPARTURE |
| `static_config.tap_enabled` set | DEPARTURE |
| neither `match` nor `match_config` | **PARITY** |
| `output_config.streaming: true` | DEPARTURE |
| `sinks` count != 1 | **PARITY** (PGV exactly-1) |
| `format` ∈ {`PROTO_BINARY`, `PROTO_BINARY_LENGTH_DELIMITED`, `PROTO_TEXT`} | DEPARTURE |
| sink arm `streaming_admin` | **PARITY** (without admin) |
| sink arm `buffered_admin` | **PARITY** (without admin) |
| sink arm `streaming_grpc` | DEPARTURE (**the reference ABORTS its own process**, exit 139) |
| sink arm `custom_sink` | **PARITY** (for an unregistered impl) |
| `file_per_tap.path_prefix` empty | — |
| any rejected `MatchPredicate` arm (4) | via `matchpredicate.Compile` |

- [ ] **Step 1: Write the failing test**

`internal/filter/http/tap/config_test.go` (abridged to the load-bearing arms; the implementer adds one case per roster row):

```go
package tap

import (
	"path/filepath"
	"testing"

	cmatcherv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/matcher/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	taptapv3 "github.com/envoyproxy/go-control-plane/envoy/config/tap/v3"
	commontapv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/tap/v3"
	httptapv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/tap/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

func mustAny(t *testing.T, m proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func reqTapYes() *cmatcherv3.MatchPredicate {
	return &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestHeadersMatch{
		HttpRequestHeadersMatch: &cmatcherv3.HttpHeadersMatch{Headers: []*routev3.HeaderMatcher{{
			Name: "x-tap", HeaderMatchSpecifier: &routev3.HeaderMatcher_ExactMatch{ExactMatch: "yes"}}}},
	}}
}

func fileSink(prefix string) *taptapv3.OutputSink {
	return &taptapv3.OutputSink{
		Format:         taptapv3.OutputSink_JSON_BODY_AS_STRING,
		OutputSinkType: &taptapv3.OutputSink_FilePerTap{FilePerTap: &taptapv3.FilePerTapSink{PathPrefix: prefix}},
	}
}

// validTap returns a minimal accepted Tap config writing under dir.
func validTap(dir string) *httptapv3.Tap {
	return &httptapv3.Tap{CommonConfig: &commontapv3.CommonExtensionConfig{
		ConfigType: &commontapv3.CommonExtensionConfig_StaticConfig{StaticConfig: &taptapv3.TapConfig{
			Match:        reqTapYes(),
			OutputConfig: &taptapv3.OutputConfig{Sinks: []*taptapv3.OutputSink{fileSink(filepath.Join(dir, "out"))}},
		}},
	}}
}

func newCtx() (envoyhttp.FactoryCtx, *stats.Registry) {
	reg := stats.NewRegistry()
	return envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "hcm_probe"}, reg
}

func TestNew_AcceptsMinimalConfig(t *testing.T) {
	ctx, _ := newCtx()
	if _, err := New(mustAny(t, validTap(t.TempDir())), ctx); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNew_RegistersExactlyOneCounter_ReadingZero(t *testing.T) {
	ctx, reg := newCtx()
	before := countMetrics(reg)
	if _, err := New(mustAny(t, validTap(t.TempDir())), ctx); err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := countMetrics(reg) - before; got != 1 {
		t.Errorf("registered %d metrics, want exactly 1", got)
	}
	var found *stats.Counter
	reg.Walk(func(m stats.Metric) {
		if m.Name() == "http.hcm_probe.tap.rq_tapped" {
			if c, ok := m.(*stats.Counter); ok {
				found = c
			}
		}
	})
	if found == nil {
		t.Fatalf("counter http.hcm_probe.tap.rq_tapped not registered")
	}
	if got := found.Load(); got != 0 {
		t.Errorf("with no taps the counter must read 0; got %d", got)
	}
}

func countMetrics(reg *stats.Registry) int {
	n := 0
	reg.Walk(func(stats.Metric) { n++ })
	return n
}

// The `.tap.` segment is HARDCODED — it is NOT the http_filters[] entry name.
func TestNew_StatSegmentIsHardcodedNotFilterName(t *testing.T) {
	ctx, reg := newCtx()
	if _, err := New(mustAny(t, validTap(t.TempDir())), ctx); err != nil {
		t.Fatalf("New: %v", err)
	}
	names := map[string]bool{}
	reg.Walk(func(m stats.Metric) { names[m.Name()] = true })
	if !names["http.hcm_probe.tap.rq_tapped"] {
		t.Errorf("want http.hcm_probe.tap.rq_tapped; got %v", names)
	}
}

func TestNew_RejectRoster(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "out")

	withStatic := func(mut func(*taptapv3.TapConfig)) *httptapv3.Tap {
		tp := validTap(dir)
		mut(tp.GetCommonConfig().GetStaticConfig())
		return tp
	}

	cases := map[string]*httptapv3.Tap{
		"record_headers_received_time": func() *httptapv3.Tap {
			tp := validTap(dir)
			tp.RecordHeadersReceivedTime = true
			return tp
		}(),
		"admin_config": {CommonConfig: &commontapv3.CommonExtensionConfig{
			ConfigType: &commontapv3.CommonExtensionConfig_AdminConfig{
				AdminConfig: &commontapv3.AdminConfig{ConfigId: "x"}}}},
		"common_config_unset": {},
		"match_config_set": withStatic(func(sc *taptapv3.TapConfig) {
			sc.MatchConfig = &taptapv3.MatchPredicate{Rule: &taptapv3.MatchPredicate_AnyMatch{AnyMatch: true}}
		}),
		"tap_enabled_set": withStatic(func(sc *taptapv3.TapConfig) {
			sc.TapEnabled = &corev3.RuntimeFractionalPercent{DefaultValue: &typev3.FractionalPercent{
				Numerator: 100, Denominator: typev3.FractionalPercent_HUNDRED}}
		}),
		"neither_match_nor_match_config": withStatic(func(sc *taptapv3.TapConfig) { sc.Match = nil }),
		"streaming_true":                 withStatic(func(sc *taptapv3.TapConfig) { sc.OutputConfig.Streaming = true }),
		"zero_sinks":                     withStatic(func(sc *taptapv3.TapConfig) { sc.OutputConfig.Sinks = nil }),
		"two_sinks": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks = []*taptapv3.OutputSink{fileSink(prefix), fileSink(prefix + "2")}
		}),
		"format_proto_binary": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].Format = taptapv3.OutputSink_PROTO_BINARY
		}),
		"format_proto_binary_length_delimited": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].Format = taptapv3.OutputSink_PROTO_BINARY_LENGTH_DELIMITED
		}),
		"format_proto_text": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].Format = taptapv3.OutputSink_PROTO_TEXT
		}),
		"sink_streaming_admin": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_StreamingAdmin{StreamingAdmin: &taptapv3.StreamingAdminSink{}}
		}),
		"sink_buffered_admin": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_BufferedAdmin{BufferedAdmin: &taptapv3.BufferedAdminSink{}}
		}),
		"sink_streaming_grpc": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_StreamingGrpc{StreamingGrpc: &taptapv3.StreamingGrpcSink{}}
		}),
		"sink_custom": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_CustomSink{CustomSink: &corev3.TypedExtensionConfig{Name: "x"}}
		}),
		"sink_no_arm": withStatic(func(sc *taptapv3.TapConfig) { sc.OutputConfig.Sinks[0].OutputSinkType = nil }),
		"empty_path_prefix": withStatic(func(sc *taptapv3.TapConfig) {
			sc.OutputConfig.Sinks[0].OutputSinkType = &taptapv3.OutputSink_FilePerTap{FilePerTap: &taptapv3.FilePerTapSink{}}
		}),
		"match_trailer_arm": withStatic(func(sc *taptapv3.TapConfig) {
			sc.Match = &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestTrailersMatch{
				HttpRequestTrailersMatch: &cmatcherv3.HttpHeadersMatch{}}}
		}),
		"match_generic_body_arm": withStatic(func(sc *taptapv3.TapConfig) {
			sc.Match = &cmatcherv3.MatchPredicate{Rule: &cmatcherv3.MatchPredicate_HttpRequestGenericBodyMatch{
				HttpRequestGenericBodyMatch: &cmatcherv3.HttpGenericBodyMatch{}}}
		}),
	}
	for name, tp := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, _ := newCtx()
			f, err := New(mustAny(t, tp), ctx)
			if err == nil {
				t.Errorf("expected reject, got nil error")
			}
			if f != nil {
				t.Errorf("expected nil factory on reject, got %v", f)
			}
		})
	}
}

// Both JSON formats are accepted at 56.1 (indistinguishable without a body).
func TestNew_AcceptsBothJSONFormats(t *testing.T) {
	for _, f := range []taptapv3.OutputSink_Format{
		taptapv3.OutputSink_JSON_BODY_AS_BYTES,  // the proto default (0)
		taptapv3.OutputSink_JSON_BODY_AS_STRING,
	} {
		tp := validTap(t.TempDir())
		tp.GetCommonConfig().GetStaticConfig().GetOutputConfig().GetSinks()[0].Format = f
		ctx, _ := newCtx()
		if _, err := New(mustAny(t, tp), ctx); err != nil {
			t.Errorf("format %v: New = %v, want nil", f, err)
		}
	}
}

func TestNew_NilAndGarbageTypedConfig(t *testing.T) {
	ctx, _ := newCtx()
	if _, err := New(nil, ctx); err == nil {
		t.Errorf("nil typed_config: want error")
	}
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	if _, err := New(bad, ctx); err == nil {
		t.Errorf("garbage typed_config: want error")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/filter/http/tap/ -count=1
```
Expected: FAIL — `undefined: New`, `undefined: TypeURL`.

- [ ] **Step 3: Implement `config.go`**

```go
package tap

import (
	"errors"
	"fmt"

	taptapv3 "github.com/envoyproxy/go-control-plane/envoy/config/tap/v3"
	commontapv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/tap/v3"
	httptapv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/tap/v3"
	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/matchpredicate"
	"github.com/pgdad/envoy-go/internal/stats"
)

// TypeURL is the tap filter's typed_config @type.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.tap.v3.Tap"

const filterName = "envoy.filters.http.tap"

// config is the immutable, per-listener compiled tap configuration. It is
// shared by every *tapFilter minted for a stream.
type config struct {
	prog       *matchpredicate.Program
	sink       *filePerTapSink
	rqTapped   *stats.Counter
	recordConn bool
}

// New is the ADR-0071 two-step factory: it parses + compiles once, then returns
// a closure that mints one *tapFilter per stream.
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	cfg, err := parseConfig(tc, ctx)
	if err != nil {
		return nil, err
	}
	return func() envoyhttp.HTTPFilter {
		f := &tapFilter{cfg: cfg}
		return envoyhttp.HTTPFilter{
			Name:    filterName,
			Decoder: f,
			Encoder: f, // SAME *tapFilter: chain.Destroy()'s `else if` makes an
			            // encoder-only OnDestroy unreachable (the compressor precedent).
		}
	}, nil
}

func parseConfig(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (*config, error) {
	if tc == nil {
		return nil, errors.New("tap: typed_config required")
	}
	var t httptapv3.Tap
	if err := tc.UnmarshalTo(&t); err != nil {
		return nil, fmt.Errorf("tap: unmarshal Tap: %w", err)
	}

	// f2: no landed accessor exposes the per-direction header-arrival instant.
	if t.GetRecordHeadersReceivedTime() {
		return nil, errors.New("tap: record_headers_received_time is not supported " +
			"(no per-direction header-arrival instant is exposed to HTTP filters)")
	}

	cc := t.GetCommonConfig()
	if cc == nil {
		return nil, errors.New("tap: common_config required")
	}
	switch a := cc.GetConfigType().(type) {
	case *commontapv3.CommonExtensionConfig_StaticConfig:
		// accepted below
	case *commontapv3.CommonExtensionConfig_AdminConfig:
		return nil, errors.New("tap: common_config.admin_config is not supported")
	case nil:
		return nil, errors.New("tap: common_config has no config_type set")
	default:
		return nil, fmt.Errorf("tap: unsupported common_config arm %T", a)
	}
	sc := cc.GetStaticConfig()
	if sc == nil {
		return nil, errors.New("tap: static_config required")
	}

	if sc.GetMatchConfig() != nil {
		return nil, errors.New("tap: match_config (deprecated) is not supported; use match")
	}
	if sc.GetTapEnabled() != nil {
		return nil, errors.New("tap: tap_enabled is not supported")
	}
	if sc.GetMatch() == nil {
		// PARITY with the reference: "Neither match nor match_config is set in TapConfig".
		return nil, errors.New("tap: neither match nor match_config is set in TapConfig")
	}
	prog, err := matchpredicate.Compile(sc.GetMatch())
	if err != nil {
		return nil, fmt.Errorf("tap: match: %w", err)
	}

	oc := sc.GetOutputConfig()
	if oc == nil {
		return nil, errors.New("tap: output_config required")
	}
	if oc.GetStreaming() {
		return nil, errors.New("tap: output_config.streaming=true is not supported")
	}
	// PARITY with PGV: OutputConfig.sinks must contain exactly 1 item.
	if n := len(oc.GetSinks()); n != 1 {
		return nil, fmt.Errorf("tap: output_config.sinks must contain exactly 1 item(s), got %d", n)
	}
	s := oc.GetSinks()[0]

	switch f := s.GetFormat(); f {
	case taptapv3.OutputSink_JSON_BODY_AS_STRING, taptapv3.OutputSink_JSON_BODY_AS_BYTES:
		// Indistinguishable at 56.1: `body` is never populated. 56.2 diverges.
	case taptapv3.OutputSink_PROTO_BINARY,
		taptapv3.OutputSink_PROTO_BINARY_LENGTH_DELIMITED,
		taptapv3.OutputSink_PROTO_TEXT:
		return nil, fmt.Errorf("tap: output_config.sinks[0].format %v is not supported", f)
	default:
		return nil, fmt.Errorf("tap: unknown output_config.sinks[0].format %v", f)
	}

	switch a := s.GetOutputSinkType().(type) {
	case *taptapv3.OutputSink_FilePerTap:
		// accepted below
	case *taptapv3.OutputSink_StreamingAdmin:
		return nil, errors.New("tap: streaming_admin sink is not supported")
	case *taptapv3.OutputSink_BufferedAdmin:
		return nil, errors.New("tap: buffered_admin sink is not supported")
	case *taptapv3.OutputSink_StreamingGrpc:
		return nil, errors.New("tap: streaming_grpc sink is not supported")
	case *taptapv3.OutputSink_CustomSink:
		return nil, errors.New("tap: custom_sink is not supported")
	case nil:
		return nil, errors.New("tap: output_config.sinks[0] has no output_sink_type set")
	default:
		return nil, fmt.Errorf("tap: unsupported output_sink_type %T", a)
	}
	prefix := s.GetFilePerTap().GetPathPrefix()
	if prefix == "" {
		return nil, errors.New("tap: file_per_tap.path_prefix required")
	}
	sink, err := newFilePerTapSink(prefix)
	if err != nil {
		return nil, fmt.Errorf("tap: %w", err)
	}

	cfg := &config{prog: prog, sink: sink, recordConn: t.GetRecordDownstreamConnection()}
	// Registered at filter-parse (not per-stream) so it reads 0 with no taps.
	// The `.tap.` segment is HARDCODED, not the http_filters[] entry name.
	if ctx.Stats != nil {
		cfg.rqTapped = ctx.Stats.NewCounter("http." + ctx.StatPrefix + ".tap.rq_tapped")
	}
	return cfg, nil
}
```

Also add a minimal `tap.go` stub so the package compiles (Tasks 8–9 fill it):

```go
package tap

type tapFilter struct{ cfg *config }
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/filter/http/tap/ -count=1 -v
```
Expected: `ok`; every `TestNew_RejectRoster` subtest PASSes.

- [ ] **Step 5: Prove the reject roster is LIVE, not vacuous**

A reject test that would pass even without the reject is worthless. For **three** representative rows, temporarily delete the guard, confirm the specific subtest FAILS, then `git restore`:

```bash
# e.g. delete the `if sc.GetTapEnabled() != nil` block
go test ./internal/filter/http/tap/ -run 'TestNew_RejectRoster/tap_enabled_set' -count=1   # must FAIL
git restore internal/filter/http/tap/config.go
git branch --show-current   # confirm still on the impl branch
```
Repeat for `sink_streaming_grpc` and `neither_match_nor_match_config`. Record each in `PROGRESS-56.1.md`.

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l internal/filter/http/tap/ && golangci-lint run ./internal/filter/http/tap/ && go vet ./... && go build ./...
git add internal/filter/http/tap/
git commit -m "phase 56.1 T7: tap config parse + full PARITY/DEPARTURE reject roster + rq_tapped (+1 delta guard)"
```

---

## Task 8: The dual-sided capture — `:status` synthesis on a COPY, lowercase, sort + the wire-leak regression [TDD]

**Files:**
- Create: `internal/filter/http/tap/tap.go` (replacing the Task 7 stub), `internal/filter/http/tap/trace.go`
- Test: `internal/filter/http/tap/tap_test.go`

**Interfaces:**
- Consumes: `config` (Task 7), `headermatch.Lowercase`.
- Produces:
  - `func (f *tapFilter) DecodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus`
  - `func (f *tapFilter) EncodeHeaders(http.Header, bool) envoyhttp.FilterHeadersStatus`
  - `func (f *tapFilter) SetDecoderCallbacks(envoyhttp.DecoderFilterCallbacks)` / `SetEncoderCallbacks(...)`
  - `func toHeaderValues(h http.Header) []*corev3.HeaderValue` — lowercased, **sorted by (key, value)**, `RawValue` nil

> **THE TRAP.** `EncodeHeaders` receives the very map HCM merges back into the wire response (`connection.go:738` → `:741` → `writeH1Reply`). `textproto.CanonicalMIMEHeaderKey(":status") == ":status"` (verified), and `writeH1Reply` emits every field verbatim. **Writing `:status` into that map emits a literal `:status: 204` header on the wire.** Build a copy.

- [ ] **Step 1: Write the failing test**

`internal/filter/http/tap/tap_test.go`:

```go
package tap

import (
	"net/http"
	"reflect"
	"testing"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// stubEncCB satisfies EncoderFilterCallbacks by embedding the interface: only
// ResponseStatus is implemented. Any other method call nil-panics, which is the
// point — tap must not reach for anything else on the encode side.
type stubEncCB struct {
	envoyhttp.EncoderFilterCallbacks
	status int
}

func (s stubEncCB) ResponseStatus() int { return s.status }

func newTapFilter(t *testing.T) *tapFilter {
	t.Helper()
	ctx, _ := newCtx()
	factory, err := New(mustAny(t, validTap(t.TempDir())), ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	f, ok := hf.Decoder.(*tapFilter)
	if !ok {
		t.Fatalf("Decoder is %T, want *tapFilter", hf.Decoder)
	}
	return f
}

// THE WIRE-LEAK REGRESSION. The map handed to EncodeHeaders is merged back into
// the wire response; a synthetic :status added to it would be emitted literally.
func TestEncodeHeaders_NeverMutatesTheWireBoundMap(t *testing.T) {
	f := newTapFilter(t)
	f.SetEncoderCallbacks(stubEncCB{status: 204})

	wire := http.Header{"Content-Type": {"text/plain"}}
	before := wire.Clone()

	if got := f.EncodeHeaders(wire, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders = %v, want Continue", got)
	}
	if !reflect.DeepEqual(wire, before) {
		t.Errorf("EncodeHeaders MUTATED the wire-bound map:\n got %v\nwant %v", wire, before)
	}
	if _, leaked := wire[":status"]; leaked {
		t.Errorf(":status leaked into the wire-bound header map")
	}
	// ...but the captured copy DOES carry it, lowercased.
	if got := f.respHdrs[":status"]; len(got) != 1 || got[0] != "204" {
		t.Errorf("captured response headers :status = %v, want [204]", got)
	}
	if got := f.respHdrs["content-type"]; len(got) != 1 || got[0] != "text/plain" {
		t.Errorf("captured response headers content-type = %v, want [text/plain]", got)
	}
}

func TestDecodeHeaders_LowercasesAndCopies(t *testing.T) {
	f := newTapFilter(t)
	req := http.Header{"X-Tap": {"yes"}, ":method": {"GET"}, ":path": {"/tap"}}
	before := req.Clone()

	if got := f.DecodeHeaders(req, true); got != envoyhttp.Continue {
		t.Errorf("DecodeHeaders = %v, want Continue", got)
	}
	if !reflect.DeepEqual(req, before) {
		t.Errorf("DecodeHeaders MUTATED its input map")
	}
	if got := f.reqHdrs["x-tap"]; len(got) != 1 || got[0] != "yes" {
		t.Errorf("captured x-tap = %v, want [yes]", got)
	}
	if got := f.reqHdrs[":method"]; len(got) != 1 || got[0] != "GET" {
		t.Errorf("captured :method = %v, want [GET]", got)
	}
}

func TestToHeaderValues_SortedByKeyThenValue_RawValueNil(t *testing.T) {
	h := http.Header{"b": {"2"}, "a": {"z", "y"}, ":status": {"204"}}
	got := toHeaderValues(h)
	type kv struct{ k, v string }
	var flat []kv
	for _, hv := range got {
		if hv.GetRawValue() != nil {
			t.Errorf("RawValue must be nil (protojson EmitDefaultValues renders \"\"); got %v", hv.GetRawValue())
		}
		flat = append(flat, kv{hv.GetKey(), hv.GetValue()})
	}
	want := []kv{{":status", "204"}, {"a", "y"}, {"a", "z"}, {"b", "2"}}
	if !reflect.DeepEqual(flat, want) {
		t.Errorf("toHeaderValues = %v, want %v", flat, want)
	}
}

// A missing/zero ResponseStatus must not synthesize a bogus :status.
func TestEncodeHeaders_NoStatusWhenCallbacksAbsent(t *testing.T) {
	f := newTapFilter(t)
	f.EncodeHeaders(http.Header{"Content-Type": {"text/plain"}}, true)
	if _, ok := f.respHdrs[":status"]; ok {
		t.Errorf(":status must be absent when no encoder callbacks are set")
	}
}
```

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/filter/http/tap/ -run 'TestEncodeHeaders|TestDecodeHeaders|TestToHeaderValues' -count=1
```
Expected: FAIL — `f.EncodeHeaders undefined`.

- [ ] **Step 3: Implement `tap.go`**

```go
package tap

import (
	"net/http"
	"strconv"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/headermatch"
)

// tapFilter is ONE value installed as BOTH HTTPFilter.Decoder and
// HTTPFilter.Encoder. FilterChain.Destroy()'s loop is
// `if Decoder != nil {…} else if Encoder != nil {…}`, so an encoder-only
// OnDestroy is UNREACHABLE and the stream-end emit would silently never fire.
// See doc.go.
type tapFilter struct {
	cfg *config

	decCB envoyhttp.DecoderFilterCallbacks
	encCB envoyhttp.EncoderFilterCallbacks

	// Lowercase-keyed COPIES; used for BOTH matching and emission.
	reqHdrs  http.Header
	respHdrs http.Header
	sawReq   bool
	sawResp  bool
}

func (f *tapFilter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.decCB = cb }
func (f *tapFilter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.encCB = cb }

// DecodeHeaders captures a lowercased COPY of the request headers. It never
// emits: a tap trace is an end-of-stream artifact (AMEND-TAP-NOEARLYEMIT).
func (f *tapFilter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	f.reqHdrs = headermatch.Lowercase(headers)
	f.sawReq = true
	return envoyhttp.Continue
}

// EncodeHeaders captures a lowercased COPY of the response headers and injects
// the synthetic :status from the ADR-0196 accessor INTO THE COPY.
//
// NEVER write into `headers`: HCM merges that very map back into the response
// it writes to the socket (connection.go:738 -> :741 -> writeH1Reply), and
// textproto canonicalization does not strip a leading colon, so a synthetic
// ":status" would be emitted as a literal header on the wire.
func (f *tapFilter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	lc := headermatch.Lowercase(headers)
	if f.encCB != nil {
		if st := f.encCB.ResponseStatus(); st > 0 {
			lc[":status"] = []string{strconv.Itoa(st)}
		}
	}
	f.respHdrs = lc
	f.sawResp = true
	return envoyhttp.Continue
}

// Tap observes headers only; the data and trailer hooks are inert pass-throughs.
func (f *tapFilter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *tapFilter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *tapFilter) DecodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
func (f *tapFilter) EncodeTrailers(http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}
```

- [ ] **Step 4: Implement `trace.go` (the header rendering half)**

```go
package tap

import (
	"net/http"
	"sort"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// toHeaderValues renders a lowercase-keyed header map as sorted HeaderValues.
//
// Sorting by (key, value) is a DOCUMENTED departure from the reference's codec
// order; it is invisible to the differential, which compares SETS. A sort is
// required regardless: http.Header is a Go map with no stable iteration order.
//
// RawValue is left nil: protojson's EmitDefaultValues renders it as "" — which
// is exactly what the reference emits.
func toHeaderValues(h http.Header) []*corev3.HeaderValue {
	out := make([]*corev3.HeaderValue, 0, len(h))
	for k, vs := range h {
		for _, v := range vs {
			out = append(out, &corev3.HeaderValue{Key: k, Value: v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GetKey() != out[j].GetKey() {
			return out[i].GetKey() < out[j].GetKey()
		}
		return out[i].GetValue() < out[j].GetValue()
	})
	return out
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/filter/http/tap/ -count=1 -v
```
Expected: `ok`.

- [ ] **Step 6: Prove the wire-leak test is LIVE**

```bash
# Temporarily change EncodeHeaders to write into `headers` instead of `lc`:
#   headers[":status"] = []string{strconv.Itoa(st)}
go test ./internal/filter/http/tap/ -run TestEncodeHeaders_NeverMutatesTheWireBoundMap -count=1  # must FAIL
git restore internal/filter/http/tap/tap.go
git branch --show-current
```
Confirm the failure names the mutation (not some other assertion), and record it in `PROGRESS-56.1.md`.

- [ ] **Step 7: Gates + commit**

```bash
gofmt -l internal/filter/http/tap/ && golangci-lint run ./internal/filter/http/tap/ && go vet ./... && go build ./...
git add internal/filter/http/tap/
git commit -m "phase 56.1 T8: dual-sided capture (:status on a COPY, lowercase, sorted) + the wire-leak regression test"
```

---

## Task 9: `OnDestroy` — the stream-end emit, trace assembly, `record_downstream_connection`, and the ONE-SHARED-VALUE pins [TDD]

**Files:**
- Modify: `internal/filter/http/tap/tap.go`, `internal/filter/http/tap/trace.go`
- Test: `internal/filter/http/tap/emit_test.go`
- Test (framework, test-only): `internal/filter/http/chain_test.go`

**Interfaces:**
- Consumes: `config.prog`, `config.sink`, `config.rqTapped`, `config.recordConn`; `toHeaderValues`; `DecoderFilterCallbacks.DownstreamLocalAddr()/DownstreamRemoteAddr()`.
- Produces: `func (f *tapFilter) OnDestroy()`; `func (f *tapFilter) buildTrace() *datatapv3.TraceWrapper`.

**The counter/write coupling, pinned:** `rq_tapped` increments **on the MATCH decision**, before and independent of the sink write. A write failure does not decrement it and does not fail the stream.

- [ ] **Step 1: Write the failing tests**

`internal/filter/http/tap/emit_test.go`:

```go
package tap

import (
	"net"
	"net/http"
	"path/filepath"
	"testing"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

type stubDecCB struct {
	envoyhttp.DecoderFilterCallbacks
	local, remote net.Addr
}

func (s stubDecCB) DownstreamLocalAddr() net.Addr  { return s.local }
func (s stubDecCB) DownstreamRemoteAddr() net.Addr { return s.remote }

func tcp(t *testing.T, s string) net.Addr {
	t.Helper()
	a, err := net.ResolveTCPAddr("tcp", s)
	if err != nil {
		t.Fatalf("ResolveTCPAddr: %v", err)
	}
	return a
}

func globTraces(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, "out_*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return m
}

// driveStream runs one request/response pair through f and tears it down.
func driveStream(f *tapFilter, xtap string, status int) {
	f.SetEncoderCallbacks(stubEncCB{status: status})
	f.DecodeHeaders(http.Header{"X-Tap": {xtap}, ":method": {"GET"}}, true)
	f.EncodeHeaders(http.Header{"Content-Type": {"text/plain"}}, true)
	f.OnDestroy()
}

func TestOnDestroy_EmitsOneTraceOnMatch(t *testing.T) {
	dir := t.TempDir()
	ctx, reg := newCtx()
	factory, err := New(mustAny(t, validTap(dir)), ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	hf := factory()
	driveStream(hf.Decoder.(*tapFilter), "yes", 204)

	if got := len(globTraces(t, dir)); got != 1 {
		t.Errorf("trace files = %d, want 1", got)
	}
	if got := counterValue(t, reg, "http.hcm_probe.tap.rq_tapped"); got != 1 {
		t.Errorf("rq_tapped = %d, want 1", got)
	}
}

func TestOnDestroy_EmitsNothingOnNoMatch(t *testing.T) {
	dir := t.TempDir()
	ctx, reg := newCtx()
	factory, _ := New(mustAny(t, validTap(dir)), ctx)
	driveStream(factory().Decoder.(*tapFilter), "no", 204)

	if got := len(globTraces(t, dir)); got != 0 {
		t.Errorf("trace files = %d, want 0 (no file on no-match)", got)
	}
	if got := counterValue(t, reg, "http.hcm_probe.tap.rq_tapped"); got != 0 {
		t.Errorf("rq_tapped = %d, want 0", got)
	}
}

// The trace is the WHOLE stream: request headers are present even though the
// predicate only names a request arm, and the response is captured too.
func TestBuildTrace_CarriesBothDirections_NoBody_EmptyTrailers(t *testing.T) {
	dir := t.TempDir()
	ctx, _ := newCtx()
	factory, _ := New(mustAny(t, validTap(dir)), ctx)
	f := factory().Decoder.(*tapFilter)
	f.SetEncoderCallbacks(stubEncCB{status: 204})
	f.DecodeHeaders(http.Header{"X-Tap": {"yes"}, ":method": {"GET"}}, true)
	f.EncodeHeaders(http.Header{"Content-Type": {"text/plain"}}, true)

	bt := f.buildTrace().GetHttpBufferedTrace()
	if bt.GetRequest() == nil || len(bt.GetRequest().GetHeaders()) == 0 {
		t.Errorf("request must be populated")
	}
	if bt.GetResponse() == nil || len(bt.GetResponse().GetHeaders()) == 0 {
		t.Errorf("response must be populated")
	}
	if bt.GetRequest().GetBody() != nil || bt.GetResponse().GetBody() != nil {
		t.Errorf("body must NEVER be populated at 56.1")
	}
	if len(bt.GetRequest().GetTrailers()) != 0 || len(bt.GetResponse().GetTrailers()) != 0 {
		t.Errorf("trailers must NEVER be populated (the framework coverage boundary)")
	}
	if bt.GetDownstreamConnection() != nil {
		t.Errorf("downstream_connection must be absent when record_downstream_connection is unset")
	}
}

func TestBuildTrace_RecordDownstreamConnection(t *testing.T) {
	dir := t.TempDir()
	tp := validTap(dir)
	tp.RecordDownstreamConnection = true
	ctx, _ := newCtx()
	factory, err := New(mustAny(t, tp), ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	f := factory().Decoder.(*tapFilter)
	f.SetDecoderCallbacks(stubDecCB{local: tcp(t, "10.0.0.1:10000"), remote: tcp(t, "10.0.0.2:38216")})
	f.SetEncoderCallbacks(stubEncCB{status: 204})
	f.DecodeHeaders(http.Header{"X-Tap": {"yes"}}, true)
	f.EncodeHeaders(http.Header{}, true)

	conn := f.buildTrace().GetHttpBufferedTrace().GetDownstreamConnection()
	if conn == nil {
		t.Fatalf("downstream_connection must be populated when the flag is set")
	}
	if got := conn.GetLocalAddress().GetSocketAddress().GetAddress(); got != "10.0.0.1" {
		t.Errorf("local address = %q, want 10.0.0.1", got)
	}
	if got := conn.GetLocalAddress().GetSocketAddress().GetPortValue(); got != 10000 {
		t.Errorf("local port = %d, want 10000", got)
	}
	if got := conn.GetRemoteAddress().GetSocketAddress().GetAddress(); got != "10.0.0.2" {
		t.Errorf("remote address = %q, want 10.0.0.2", got)
	}
}

// D-TAP-EMITSITE (i): both HTTPFilter fields must hold the SAME pointer.
func TestFactory_InstallsOneSharedValueInBothFields(t *testing.T) {
	ctx, _ := newCtx()
	factory, _ := New(mustAny(t, validTap(t.TempDir())), ctx)
	hf := factory()
	d, okD := hf.Decoder.(*tapFilter)
	e, okE := hf.Encoder.(*tapFilter)
	if !okD || !okE {
		t.Fatalf("Decoder=%T Encoder=%T, want both *tapFilter", hf.Decoder, hf.Encoder)
	}
	if d != e {
		t.Errorf("Decoder and Encoder must be the SAME *tapFilter value; " +
			"a two-value split makes the encoder OnDestroy unreachable (chain.go:670)")
	}
}

// D-TAP-EMITSITE (ii): driving a real FilterChain to Destroy() emits once.
//
// CRITICAL: this test drives the INTERFACE VALUES hf.Decoder / hf.Encoder — it
// must NEVER downcast hf.Decoder to *tapFilter and poke that. With a downcast,
// a two-value split (Decoder: &tapFilter{}, Encoder: f) would still feed BOTH
// header sets into whatever hf.Decoder happens to be, the emit would fire, and
// the test would pass — proving nothing. Driving through the two interfaces is
// exactly what makes the split break bite: the decoder value would hold only
// the request headers, its response arm would stay Undetermined, and Destroy()
// (which reaches only the Decoder branch) would emit nothing.
func TestChainDestroy_EmitsExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	ctx, reg := newCtx()
	factory, _ := New(mustAny(t, validTap(dir)), ctx)
	hf := factory()

	hf.Encoder.SetEncoderCallbacks(stubEncCB{status: 204})
	hf.Decoder.DecodeHeaders(http.Header{"X-Tap": {"yes"}}, true)
	hf.Encoder.EncodeHeaders(http.Header{}, true)

	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{hf}, nil)
	chain.Destroy()
	chain.Destroy() // idempotent (destroyOnce)

	if got := len(globTraces(t, dir)); got != 1 {
		t.Errorf("trace files = %d, want exactly 1", got)
	}
	if got := counterValue(t, reg, "http.hcm_probe.tap.rq_tapped"); got != 1 {
		t.Errorf("rq_tapped = %d, want 1", got)
	}
}

func counterValue(t *testing.T, reg *stats.Registry, name string) uint64 {
	t.Helper()
	var v uint64
	found := false
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			if c, ok := m.(*stats.Counter); ok {
				v, found = c.Load(), true
			}
		}
	})
	if !found {
		t.Fatalf("counter %s not registered", name)
	}
	return v
}
```

> Imports for `emit_test.go`: `net`, `net/http`, `path/filepath`, `testing`, `envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"`, and `"github.com/pgdad/envoy-go/internal/stats"`. (`os` is not needed — `t.TempDir()` handles cleanup.)

`internal/filter/http/chain_test.go` — **D-TAP-EMITSITE (iii)**, the regression pin. Add:

```go
// A both-sided filter (the SAME value in Decoder and Encoder) has OnDestroy
// invoked exactly once, via the Decoder branch of Destroy()'s if/else-if.
func TestDestroy_BothSidedFilterOnDestroyFiresOnce(t *testing.T) {
	f := &recordingFilter{}
	chain := NewFilterChain([]HTTPFilter{{Name: "both", Decoder: f, Encoder: f}}, nil)
	chain.Destroy()
	if got := f.destroyed.Load(); got != 1 {
		t.Errorf("OnDestroy fired %d times, want exactly 1", got)
	}
}

// THE HAZARD, pinned: with a Decoder present, an ENCODER-ONLY value's OnDestroy
// is UNREACHABLE (chain.go:670 is an `else if`). Any filter that hangs a
// stream-end side effect off a distinct encoder value silently never fires it.
func TestDestroy_EncoderOnlyOnDestroyUnreachableWhenDecoderPresent(t *testing.T) {
	dec := &recordingFilter{}
	enc := &recordingFilter{}
	chain := NewFilterChain([]HTTPFilter{{Name: "split", Decoder: dec, Encoder: encodeRecorder{f: enc}}}, nil)
	chain.Destroy()
	if got := dec.destroyed.Load(); got != 1 {
		t.Errorf("decoder OnDestroy fired %d times, want 1", got)
	}
	if got := enc.destroyed.Load(); got != 0 {
		t.Errorf("encoder OnDestroy fired %d times, want 0 — "+
			"if this ever becomes 1, the tap filter's ONE-SHARED-VALUE constraint can be relaxed", got)
	}
}

// And an encoder-ONLY filter (no Decoder) DOES get OnDestroy, via the else-if.
func TestDestroy_EncoderOnlyFilterWithNoDecoderFires(t *testing.T) {
	enc := &recordingFilter{}
	chain := NewFilterChain([]HTTPFilter{{Name: "enc-only", Encoder: encodeRecorder{f: enc}}}, nil)
	chain.Destroy()
	if got := enc.destroyed.Load(); got != 1 {
		t.Errorf("encoder-only OnDestroy fired %d times, want 1", got)
	}
}
```

> `recordingFilter` and `encodeRecorder` already exist in `chain_test.go` (`OnDestroy` at `:70` and `:161`). Its counter field is **`destroyed atomic.Int32`** (`chain_test.go:39`), not `Int64` — `f.destroyed.Load() != 1` still compiles against the untyped constant. `recordingFilter` satisfies **both** `StreamDecoderFilter` and `StreamEncoderFilter`, so `HTTPFilter{Decoder: f, Encoder: f}` type-checks. Re-confirm before use.

- [ ] **Step 2: Run and watch them fail**

```bash
go test ./internal/filter/http/tap/ -run 'TestOnDestroy|TestBuildTrace|TestFactory_Installs|TestChainDestroy' -count=1
go test ./internal/filter/http/ -run TestDestroy_ -count=1
```
Expected: FAIL — `f.OnDestroy undefined` / `f.buildTrace undefined`; the `chain_test.go` additions should **already pass** (they pin existing behavior — if `TestDestroy_EncoderOnlyOnDestroyUnreachableWhenDecoderPresent` FAILS, STOP: the framework changed and this leg's central constraint must be re-derived).

- [ ] **Step 3: Implement `OnDestroy` + `buildTrace` in `trace.go`**

```go
package tap

import (
	"net"
	"strconv"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	datatapv3 "github.com/envoyproxy/go-control-plane/envoy/data/tap/v3"
)

// OnDestroy is the ONE stream-end hook. It resolves the predicate tree and, on
// a match, assembles and emits the buffered trace.
//
// Emission is UNCONDITIONALLY at stream end. Never emit from DecodeHeaders or
// EncodeHeaders: the reference assembles the WHOLE stream and emits once, even
// when a request arm is already true at decode (AMEND-TAP-NOEARLYEMIT).
//
// This fires EXACTLY ONCE per stream: FilterChain.Destroy() is destroyOnce-
// guarded and its `if Decoder != nil {…} else if Encoder != nil {…}` loop
// reaches this value only through the Decoder branch. No guard is needed.
func (f *tapFilter) OnDestroy() {
	ev := f.cfg.prog.NewEvaluator()
	if f.sawReq {
		ev.FeedRequestHeaders(f.reqHdrs)
	}
	if f.sawResp {
		ev.FeedResponseHeaders(f.respHdrs)
	}
	if !ev.Resolve() {
		return // no match: no counter, no file
	}
	// rq_tapped counts the MATCH DECISION, not a successful write.
	if f.cfg.rqTapped != nil {
		f.cfg.rqTapped.Inc()
	}
	// A sink write failure is deliberately non-fatal and does not affect the
	// counter: the reference increments on the tap decision independently of
	// sink-write success.
	_ = f.cfg.sink.write(f.buildTrace())
}

func (f *tapFilter) buildTrace() *datatapv3.TraceWrapper {
	bt := &datatapv3.HttpBufferedTrace{}
	if f.sawReq {
		// Body and Trailers stay nil: no body at 56.1 (a zero-length body message
		// OMITS the field), and trailers are structurally invisible to envoy-go's
		// filters. EmitDefaultValues renders nil repeated Trailers as [].
		bt.Request = &datatapv3.HttpBufferedTrace_Message{Headers: toHeaderValues(f.reqHdrs)}
	}
	if f.sawResp {
		bt.Response = &datatapv3.HttpBufferedTrace_Message{Headers: toHeaderValues(f.respHdrs)}
	}
	if f.cfg.recordConn && f.decCB != nil {
		bt.DownstreamConnection = &datatapv3.Connection{
			LocalAddress:  addrProto(f.decCB.DownstreamLocalAddr()),
			RemoteAddress: addrProto(f.decCB.DownstreamRemoteAddr()),
		}
	}
	return &datatapv3.TraceWrapper{
		Trace: &datatapv3.TraceWrapper_HttpBufferedTrace{HttpBufferedTrace: bt},
	}
}

// addrProto renders a net.Addr as a core.Address{socket_address}. A nil or
// unsplittable address yields nil (the field is then omitted by protojson).
func addrProto(a net.Addr) *corev3.Address {
	if a == nil {
		return nil
	}
	host, portStr, err := net.SplitHostPort(a.String())
	if err != nil {
		return nil
	}
	p, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return nil
	}
	return &corev3.Address{Address: &corev3.Address_SocketAddress{
		SocketAddress: &corev3.SocketAddress{
			Address:       host,
			PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: uint32(p)},
		},
	}}
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/filter/http/tap/ -count=1 -v
go test ./internal/filter/http/ -count=1        # the whole framework package, incl. the new chain tests
go test ./internal/filter/http/tap/ -count=1 -race
```
Expected: `ok` for all three.

- [ ] **Step 5: Prove the ONE-SHARED-VALUE constraint is LIVE**

Two breaks, both applied to `New()` in `config.go`. **Do not add a third that drops `Decoder` entirely** — `TestFactory_InstallsOneSharedValueInBothFields` uses a comma-ok assertion and would report a clean failure, but any test that dereferences `hf.Decoder` would *panic on a nil interface conversion*, which is a crash, not a proof. The encoder-only-with-no-decoder fact is pinned where it belongs: `chain_test.go`'s `TestDestroy_EncoderOnlyFilterWithNoDecoderFires`.

```bash
# Break 1 — split the value:  Decoder: f, Encoder: &tapFilter{cfg: cfg}
go test ./internal/filter/http/tap/ -run TestFactory_InstallsOneSharedValueInBothFields -count=1
#   must FAIL with "Decoder and Encoder must be the SAME *tapFilter value"
git restore internal/filter/http/tap/config.go

# Break 2 — the REAL hazard:  Decoder: &tapFilter{cfg: cfg}, Encoder: f
#   A DIFFERENT value in Decoder — the exact two-value split the SPEC forbids.
#   TestChainDestroy_EmitsExactlyOnce drives hf.Decoder.DecodeHeaders and
#   hf.Encoder.EncodeHeaders through the INTERFACES, so the request headers land
#   on the decoder value and the response headers on `f`. Destroy() reaches only
#   the Decoder branch (chain.go:668), whose response arm is still Undetermined
#   => no match => no file. `f`'s OnDestroy is never invoked.
go test ./internal/filter/http/tap/ -run TestChainDestroy_EmitsExactlyOnce -count=1
#   must FAIL: "trace files = 0, want exactly 1" AND "rq_tapped = 0, want 1"
git restore internal/filter/http/tap/config.go
git branch --show-current
```

Confirm **which** assertion fired in each (`reference_deliberate_break_wrong_assertion`) and record the literal text in `PROGRESS-56.1.md`.

> **Why Break 2 only bites if the test drives the interfaces.** An earlier draft of this plan had `TestChainDestroy_EmitsExactlyOnce` do `f := hf.Decoder.(*tapFilter)` and then feed *both* header sets into `f`. Under the split, `hf.Decoder` **is** the fresh value, so it received both header sets, matched, and emitted — the test PASSED and the break proved nothing. Adversarial review caught this. If you refactor the test, preserve the interface-driven shape.

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l internal/filter/http/tap/ internal/filter/http/ && golangci-lint run ./internal/filter/http/... && go vet ./... && go build ./...
git add internal/filter/http/tap/ internal/filter/http/chain_test.go
git commit -m "phase 56.1 T9: stream-end OnDestroy emit + trace assembly + record_downstream_connection + ONE-SHARED-VALUE pins"
```

---

## Task 10: `filePerTapSink` — per-stream file, trace-id, the pinned protojson render + byte-stability golden [TDD]

**Files:**
- Create: `internal/filter/http/tap/sink.go`
- Test: `internal/filter/http/tap/sink_test.go`

**Interfaces:**
- Consumes: `*datatapv3.TraceWrapper`.
- Produces: `func newFilePerTapSink(pathPrefix string) (*filePerTapSink, error)`; `func (s *filePerTapSink) write(tw *datatapv3.TraceWrapper) error`.

**The pinned marshal (byte-exact — do not "clean up"):**
```go
var marshalOpts = protojson.MarshalOptions{
	Multiline:         true,
	Indent:            " ",   // ONE space
	UseProtoNames:     true,  // snake_case, not camelCase
	EmitDefaultValues: true,  // NOT EmitUnpopulated
}
```
`EmitUnpopulated: true` is **WRONG**: it emits `"body": null`, `"headers_received_time": null`, `"downstream_connection": null`, which the reference never does. `EmitDefaultValues` reproduces the reference's `"raw_value": ""` (scalar default) and `"trailers": []` (empty repeated) while **omitting nil message fields**. The document ends with a trailing `"\n"`.

- [ ] **Step 1: Write the failing test**

`internal/filter/http/tap/sink_test.go`:

```go
package tap

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	datatapv3 "github.com/envoyproxy/go-control-plane/envoy/data/tap/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

func goldenTrace() *datatapv3.TraceWrapper {
	return &datatapv3.TraceWrapper{Trace: &datatapv3.TraceWrapper_HttpBufferedTrace{
		HttpBufferedTrace: &datatapv3.HttpBufferedTrace{
			Request:  &datatapv3.HttpBufferedTrace_Message{Headers: []*corev3.HeaderValue{{Key: ":method", Value: "GET"}}},
			Response: &datatapv3.HttpBufferedTrace_Message{Headers: []*corev3.HeaderValue{{Key: ":status", Value: "204"}}},
		},
	}}
}

// The exact wire shape the reference produces. If protojson's output ever
// drifts (a toolchain change, protojson's detrand), THIS test must fail loudly
// rather than be "fixed" by regenerating the golden.
func TestMarshal_ByteExactGolden(t *testing.T) {
	got, err := marshalOpts.Marshal(goldenTrace())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := `{
 "http_buffered_trace": {
  "request": {
   "headers": [
    {
     "key": ":method",
     "value": "GET",
     "raw_value": ""
    }
   ],
   "trailers": []
  },
  "response": {
   "headers": [
    {
     "key": ":status",
     "value": "204",
     "raw_value": ""
    }
   ],
   "trailers": []
  }
 }
}`
	if string(got) != want {
		t.Errorf("protojson drift.\n got:\n%s\nwant:\n%s", got, want)
	}
	// Positive pins on the two properties the differential depends on.
	if bytes.Contains(got, []byte(`"body"`)) {
		t.Errorf(`"body" must be ABSENT (nil message field omitted by EmitDefaultValues)`)
	}
	if !bytes.Contains(got, []byte(`"trailers": []`)) {
		t.Errorf(`"trailers": [] must be present (nil repeated rendered by EmitDefaultValues)`)
	}
	if !bytes.Contains(got, []byte(`"raw_value": ""`)) {
		t.Errorf(`"raw_value": "" must be present`)
	}
}

// EmitUnpopulated is the wrong option and must be provably different.
func TestMarshal_EmitUnpopulatedWouldEmitNullBody(t *testing.T) {
	wrong := protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}
	b, err := wrong.Marshal(goldenTrace())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"body": null`)) {
		t.Fatalf("expected EmitUnpopulated to emit \"body\": null; the option semantics changed, re-derive AMEND-TAP-JSON")
	}
}

func TestMarshal_StableAcrossRepeatedCalls(t *testing.T) {
	first, err := marshalOpts.Marshal(goldenTrace())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for i := 0; i < 100; i++ {
		b, err := marshalOpts.Marshal(goldenTrace())
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if !bytes.Equal(b, first) {
			t.Fatalf("protojson output is not byte-stable within a process (iteration %d)", i)
		}
	}
}

func TestSink_WritesOneFilePerTrace_WithTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	s, err := newFilePerTapSink(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatalf("newFilePerTapSink: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.write(goldenTrace()); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "out_*.json"))
	if len(files) != 3 {
		t.Fatalf("files = %d, want 3 (one DISCRETE file per trace)", len(files))
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if !bytes.HasSuffix(b, []byte("\n")) {
			t.Errorf("%s: must end with a trailing newline", filepath.Base(f))
		}
		if !strings.HasPrefix(filepath.Base(f), "out_") || !strings.HasSuffix(f, ".json") {
			t.Errorf("%s: want <prefix>_<trace_id>.json", filepath.Base(f))
		}
	}
}

// D-TAP-PATHPREFIX: the parent directory is created at sink construction.
func TestSink_MkdirAllsParentAtConstruction(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	if _, err := newFilePerTapSink(filepath.Join(dir, "out")); err != nil {
		t.Fatalf("newFilePerTapSink must MkdirAll the parent: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("parent dir %s was not created", dir)
	}
}

func TestSink_RejectsUncreatableParent(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// parent of <file>/sub/out is <file>/sub — cannot be created under a regular file
	if _, err := newFilePerTapSink(filepath.Join(f, "sub", "out")); err == nil {
		t.Errorf("expected a parse-time reject for an uncreatable path_prefix parent")
	}
}
```

> **Implementer:** the `want` golden above is written from the pinned option set. **Do not hand-adjust it to make the test pass.** Run the test once, and if the actual bytes differ, STOP and re-derive: either a proto field number/name changed, or protojson's rendering drifted — both are findings that belong in `PROGRESS-56.1.md` and possibly in the ADR, not a silent golden update.

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/filter/http/tap/ -run 'TestMarshal|TestSink' -count=1
```
Expected: FAIL — `undefined: marshalOpts`, `newFilePerTapSink` missing its real body.

- [ ] **Step 3: Implement `sink.go`**

```go
package tap

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	datatapv3 "github.com/envoyproxy/go-control-plane/envoy/data/tap/v3"
	"google.golang.org/protobuf/encoding/protojson"
)

// marshalOpts is BYTE-EXACT against the reference's file_per_tap JSON.
// Measured at SPEC time: a reference trace round-tripped through these options
// reproduces the file byte-for-byte (modulo the trailing newline we append).
//
// EmitUnpopulated is WRONG: it emits "body": null / "headers_received_time":
// null / "downstream_connection": null, which the reference never does.
// EmitDefaultValues renders "raw_value": "" and "trailers": [] while OMITTING
// nil message fields — exactly the reference.
var marshalOpts = protojson.MarshalOptions{
	Multiline:         true,
	Indent:            " ",
	UseProtoNames:     true,
	EmitDefaultValues: true,
}

// traceIDs is the process-local monotonic trace-id source (D-TAP-TRACEID). The
// id appears only in the filename and is NEVER asserted cross-side, so a
// counter suffices; crypto/rand would add an error path for no benefit.
var traceIDs atomic.Uint64

// filePerTapSink writes ONE protojson document to a NEW file per emitted trace,
// named <path_prefix>_<trace_id>.json.
//
// internal/accesslog.AsyncFileSink is a SHAPE precedent only: it opens ONE
// append-only file for the process lifetime, whereas file_per_tap opens a
// DISCRETE FILE PER STREAM.
type filePerTapSink struct{ pathPrefix string }

// newFilePerTapSink creates the path_prefix's parent directory (D-TAP-PATHPREFIX).
// A path whose parent cannot be created is a CONFIG error and fails at parse
// time, rather than silently dropping every trace at stream end. This is a
// documented DEPARTURE: the reference's behavior here was not probed.
func newFilePerTapSink(pathPrefix string) (*filePerTapSink, error) {
	if err := os.MkdirAll(filepath.Dir(pathPrefix), 0o755); err != nil {
		return nil, fmt.Errorf("file_per_tap: create path_prefix parent: %w", err)
	}
	return &filePerTapSink{pathPrefix: pathPrefix}, nil
}

func (s *filePerTapSink) write(tw *datatapv3.TraceWrapper) error {
	b, err := marshalOpts.Marshal(tw)
	if err != nil {
		return fmt.Errorf("file_per_tap: marshal: %w", err)
	}
	b = append(b, '\n')

	name := fmt.Sprintf("%s_%d.json", s.pathPrefix, traceIDs.Add(1))
	// O_EXCL: a trace-id collision must surface, never silently overwrite.
	f, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("file_per_tap: open %s: %w", name, err)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return fmt.Errorf("file_per_tap: write %s: %w", name, err)
	}
	return f.Close()
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/filter/http/tap/ -count=1 -v
go test ./internal/filter/http/tap/ -count=1 -race
```
Expected: `ok`. **If `TestMarshal_ByteExactGolden` fails, do NOT edit the golden** — follow the Step-1 note.

- [ ] **Step 5: Gates + commit**

```bash
gofmt -l internal/filter/http/tap/ && golangci-lint run ./internal/filter/http/tap/ && go vet ./... && go build ./...
git add internal/filter/http/tap/
git commit -m "phase 56.1 T10: filePerTapSink (per-stream file, monotonic trace-id, MkdirAll parent) + byte-exact protojson golden"
```

---

## Task 11: `doc.go` + the `builtins.go` registration arm (20 → 21 registered; 19 → 20 production)

**Files:**
- Create: `internal/filter/http/tap/doc.go`
- Modify: `internal/filter/http/builtins/builtins.go` (the bulk block `:44-63`, and the package doc comment)
- Test: `internal/filter/http/builtins/builtins_test.go` (extend if a registration test exists; else add one)

**Interfaces:**
- Consumes: `tap.TypeURL`, `tap.New`.
- Produces: a registered `envoy.filters.http.tap` arm.

> **`cmd/envoy-go/main.go` does NOT register HTTP filters** — `builtins.go`'s doc comment claiming it "mirrors the registration calls in cmd/envoy-go/main.go" is **stale**. The HTTP registry is built, populated and frozen entirely inside `internal/boot/boot.go` (`:63` `NewHTTPRegistry`, `:64` `RegisterBuiltins`, `:65` `Freeze`). **Do not touch `main.go`.** While you are here, fix the stale doc comment.

- [ ] **Step 1: Write the failing test**

Add to `internal/filter/http/builtins/builtins_test.go`:

```go
func TestRegisterBuiltins_RegistersTap(t *testing.T) {
	reg := filter_http.NewHTTPRegistry()
	RegisterBuiltins(reg)
	if _, ok := reg.Lookup(tap.TypeURL); !ok {
		t.Errorf("tap.TypeURL %q is not registered", tap.TypeURL)
	}
}
```

> Confirm the registry's lookup method name before writing this (`grep -n 'func (r \*HTTPRegistry)' internal/filter/http/registry.go`); use whatever the existing builtins tests use.

- [ ] **Step 2: Run it and watch it fail**

```bash
go test ./internal/filter/http/builtins/ -run TestRegisterBuiltins_RegistersTap -count=1
```
Expected: FAIL — tap not registered.

- [ ] **Step 3: Add the registration arm**

In `internal/filter/http/builtins/builtins.go`, add the import and insert the line **between `rbac` and `wasm`** (the block is alphabetical after the leading `router`):

```go
	reg.Register(rbac.TypeURL, rbac.New)
	reg.Register(tap.TypeURL, tap.New)
	reg.Register(wasm.TypeURL, wasm.New)
```

Update the package doc comment: **"twenty built-in HTTP filters" → "twenty-one built-in HTTP filters"** (20 → 21 registered; 19 → 20 production, since `envoygotest` is test-support). Fix the stale "mirrors the registration calls in cmd/envoy-go/main.go" clause — `main.go` performs no HTTP-filter registration.

- [ ] **Step 4: Write `doc.go`**

```go
// Package tap implements Envoy's HTTP tap filter (envoy.filters.http.tap):
// a per-stream, dual-sided observer that compiles a
// config/common/matcher/v3.MatchPredicate tree into a tri-state node tree at
// config time, evaluates it over request headers (DecodeHeaders) and response
// headers (EncodeHeaders), and — at STREAM END, on a match — writes a
// buffered data/tap/v3.TraceWrapper as one byte-exact protojson document to a
// per-stream file_per_tap sink file.
//
// Three constraints are load-bearing:
//
//  1. ONE SHARED VALUE. The same *tapFilter is installed in BOTH
//     HTTPFilter.Decoder and HTTPFilter.Encoder. FilterChain.Destroy()'s loop is
//     `if f.Decoder != nil { … } else if f.Encoder != nil { … }`, so a
//     both-sided filter's OnDestroy fires exactly once — and an ENCODER-ONLY
//     value's OnDestroy is UNREACHABLE whenever a Decoder is present. Splitting
//     tap into two values would silently never emit. (Same shape as the
//     compressor filter.)
//
//  2. NEVER MUTATE THE ENCODE HEADER MAP. :status is not carried in the map
//     handed to EncodeHeaders; it comes from EncoderFilterCallbacks.ResponseStatus()
//     (ADR-0196). HCM merges that same map back into the wire response, and Go's
//     header canonicalization does not strip a leading colon — so a synthetic
//     ":status" written into it would be emitted as a literal header on the wire.
//     Tap injects it into a COPY.
//
//  3. NEVER EARLY-EMIT. The trace is an end-of-stream artifact covering the
//     WHOLE stream, even when a request arm is already true at decode. The
//     tri-state (match/no-match/undetermined) exists to RESOLVE the tree, not to
//     decide when to emit.
//
// Trailers are a documented COVERAGE BOUNDARY: envoy-go's HTTP filters cannot
// observe them (the never-done HCM "Task 18"), so the two trailer match arms are
// boot-rejected and Message.trailers is never populated. This is invisible in
// the differential: protojson's EmitDefaultValues renders empty repeated
// trailers as [] byte-identically on both sides.
package tap
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/filter/http/builtins/ -count=1
go build ./... && go test ./internal/... -count=1
```
Expected: `ok`.

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l internal/filter/http/ && golangci-lint run ./internal/filter/http/... && go vet ./... && go build ./...
git add internal/filter/http/
git commit -m "phase 56.1 T11: register envoy.filters.http.tap in builtins (20 -> 21 registered) + doc.go + fix stale main.go doc claim"
```

---

## Task 12: Harness — `fixture.HostMount.Dir` + the runner's directory pre-create (D-TAP-DIRMOUNT) [TDD]

**Files:**
- Modify: `test/differential/fixture/fixture.go` (the `HostMount` struct, `:631`)
- Modify: `test/differential/runner_test.go` (the mount pre-create loop, `:1146-1158`, **and its sibling at `:2110`**)
- Test: `test/differential/fixture/fixture_test.go` (or an existing test file in that package)

**Interfaces:**
- Consumes: nothing.
- Produces: `type HostMount struct { HostPath, ContainerPath string; Dir bool }`.

**Why.** `file_per_tap` writes one file per matching stream with an unpredictable `<trace_id>` name. The reference proxy runs inside Docker, so the host test process can only see those files if the **parent directory** is bind-mounted. The current seam cannot express that: the runner unconditionally pre-creates `HostPath` as a file with `os.OpenFile(..., os.O_CREATE|os.O_WRONLY, 0o666)`, and `O_WRONLY` on a directory returns `EISDIR`.

**Scope note.** This is TEST-HARNESS surgery. The Global-Constraints "framework-zero-touch" pin scopes to the production HTTP-filter framework and HCM; `test/differential/` is neither. **Zero behavior change for the one existing `ReferenceLogMounter` (`0006-access-log`), which leaves `Dir` false.**

- [ ] **Step 1: Find every pre-create site**

```bash
grep -n "ReferenceHostMounts" test/differential/runner_test.go
```
Expected: **two** call sites (~`:1146` and ~`:2110`). Both must branch on `Dir`. Missing the second one leaves a latent `EISDIR` for any future reference-less dir-mounting fixture.

- [ ] **Step 2: Write the failing test**

`test/differential/fixture/fixture_test.go`:

```go
package fixture

import "testing"

func TestHostMount_DirFieldExists(t *testing.T) {
	m := HostMount{HostPath: "/tmp/x", ContainerPath: "/envoy-go-test/taps", Dir: true}
	if !m.Dir {
		t.Errorf("HostMount.Dir must be settable")
	}
	if (HostMount{HostPath: "/tmp/y", ContainerPath: "/c"}).Dir {
		t.Errorf("HostMount.Dir must default to false (file mount), preserving the 0006 behavior")
	}
}
```

- [ ] **Step 3: Run it and watch it fail**

```bash
go test ./test/differential/fixture/ -run TestHostMount_DirFieldExists -count=1
```
Expected: FAIL — `unknown field Dir`.

- [ ] **Step 4: Add the field**

`test/differential/fixture/fixture.go`:

```go
// HostMount describes a bind-mount from the test host into the reference
// container. The runner pre-creates HostPath before starting the container so
// Docker binds the intended kind of inode.
//
// Dir=false (the default) mounts a single FILE — the 0006-access-log shape,
// where the proxy appends to one known path.
//
// Dir=true mounts a DIRECTORY — required when the proxy creates files whose
// names the test cannot predict (e.g. the tap filter's file_per_tap sink, which
// writes <path_prefix>_<trace_id>.json once per matching stream). The directory
// is created 0o777 because the reference envoy runs as uid 101 inside the
// container and must be able to create files in it.
//
// ContainerPath must NOT live under a sticky, world-writable directory such as
// /tmp: with fs.protected_regular=2 (Ubuntu CI) envoy gets EACCES creating files
// there. Use a dedicated non-sticky dir, e.g. /envoy-go-test/.
type HostMount struct {
	HostPath      string
	ContainerPath string
	Dir           bool
}
```

- [ ] **Step 5: Branch the runner's pre-create (BOTH sites)**

Replace the body of each pre-create loop:

```go
		for _, hm := range hostMounts {
			if hm.Dir {
				// Directory mount: envoy (uid 101) creates files inside it.
				if ferr := os.MkdirAll(hm.HostPath, 0o777); ferr != nil {
					t.Fatalf("ref mount mkdir %s: %v", hm.HostPath, ferr)
				}
				if ferr := os.Chmod(hm.HostPath, 0o777); ferr != nil {
					t.Fatalf("ref mount chmod %s: %v", hm.HostPath, ferr)
				}
				continue
			}
			// Pre-create the host file so Docker bind-mounts a file (not a dir).
			f, ferr := os.OpenFile(hm.HostPath, os.O_CREATE|os.O_WRONLY, 0o666)
			if ferr != nil {
				t.Fatalf("ref mount pre-create %s: %v", hm.HostPath, ferr)
			}
			_ = f.Close()
			if ferr = os.Chmod(hm.HostPath, 0o666); ferr != nil {
				t.Fatalf("ref mount chmod %s: %v", hm.HostPath, ferr)
			}
		}
```

`harness.go`'s `StartReferenceProxyWithMounts` needs **no change**: it already renders each mount as a `HostConfig.Binds` entry `"<hostPath>:<containerPath>"`, which Docker resolves to a dir mount when `hostPath` is a directory.

- [ ] **Step 6: Run tests + the `0006` regression**

```bash
go test ./test/differential/fixture/ -count=1
go test ./test/differential/ -run 'TestDifferential/0006-access-log' -count=1
```
Expected: `ok` for both. **The `0006` run is the regression gate** — it is the only existing `ReferenceLogMounter`, and it must still bind a FILE.

- [ ] **Step 7: Gates + commit**

```bash
gofmt -l test/differential/ && golangci-lint run ./test/differential/... && go vet ./... && go build ./...
git add test/differential/
git commit -m "phase 56.1 T12: fixture.HostMount.Dir + runner directory pre-create (D-TAP-DIRMOUNT); 0006 file-mount unchanged"
```

---

## Task 13: Fixture `0099-http-tap-headers` — the YAMLs + the driver (GET → 204, N=3 match + M=2 non-match)

**Files:**
- Create: `test/fixtures/0099-http-tap-headers/envoy.yaml` (reference), `test/fixtures/0099-http-tap-headers/envoy-go.yaml` (subject), `test/fixtures/0099-http-tap-headers/driver/driver.go`
- Modify: `test/differential/runner_test.go` (blank-import the driver package)

**Interfaces:**
- Consumes: `fixture.Driver`, `fixture.BackendKindAware`, `fixture.ReferenceLogMounter` (with `Dir: true` from Task 12), `fixture.HTTPStatusHeader`.
- Produces: the registered fixture `"0099-http-tap-headers"`. `AssertStats` lands in Task 14.

**Design (AMEND-TAP-BODY).** The backend is the existing `HTTPStatusHeader` kind (`BackendKind = 3`): it reads `X-Backend-Status` and returns that status. Driving `GET /tap` with `x-backend-status: 204` yields a **bodyless request** and a **bodyless 204 response** — verified: Go's `net/http` emits `204 No Content` with `Content-Type: text/plain`, **no `Content-Length`, zero body bytes**. A zero-length body message OMITS the `body` field entirely, so both sides emit **structurally headers-only traces**. **BackendKind stays 38 (+0).** `BackendCount()` returns `1` (the runner rejects `0`).

**Predicate:** `and_match{ http_request_headers_match(x-tap exact "yes"), http_response_headers_match(:status exact "204") }` — one arm per direction, so the trace can only be emitted after BOTH have been observed. This is what makes deliberate break (a) bite.

- [ ] **Step 1: Write `envoy.yaml` (the reference bootstrap template)**

```yaml
# Reference bootstrap for 0099-http-tap-headers.
# The tap sink writes into /envoy-go-test/taps, a NON-STICKY container dir
# bind-mounted from a host temp dir (fixture.HostMount{Dir: true}). /tmp would
# EACCES under fs.protected_regular=2 because envoy runs as uid 101.
admin:
  address:
    socket_address: {address: 0.0.0.0, port_value: 9901}
static_resources:
  listeners:
  - name: listener_0
    address:
      socket_address: {address: 0.0.0.0, port_value: {{.ListenerPort}}}
    filter_chains:
    - filters:
      - name: envoy.filters.network.http_connection_manager
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
          stat_prefix: tap_probe
          route_config:
            name: local_route
            virtual_hosts:
            - name: vh
              domains: ["*"]
              routes:
              - match: {prefix: "/"}
                route: {cluster: backend}
          http_filters:
          - name: envoy.filters.http.tap
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.tap.v3.Tap
              common_config:
                static_config:
                  match:
                    and_match:
                      rules:
                      - http_request_headers_match:
                          headers:
                          - name: x-tap
                            string_match: {exact: "yes"}
                      - http_response_headers_match:
                          headers:
                          - name: ":status"
                            string_match: {exact: "204"}
                  output_config:
                    sinks:
                    - format: JSON_BODY_AS_STRING
                      file_per_tap:
                        path_prefix: {{.TapPrefix}}
          - name: envoy.filters.http.router
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
  - name: backend
    connect_timeout: 1s
    type: STRICT_DNS
    load_assignment:
      cluster_name: backend
      endpoints:
      - lb_endpoints:
        - endpoint:
            address:
              socket_address: {address: {{.BackendHost}}, port_value: {{.BackendPort}}}
```

`envoy-go.yaml` is the same document with: `port_value: {{.ListenerPort}}` (the runner-allocated subject port), the admin `port_value: {{.AdminPort}}`, `address: {{.BackendHost}}` = `127.0.0.1`, and `path_prefix: {{.TapPrefix}}` = the **host** subject dir. Keep `stat_prefix: tap_probe` identical on both sides so the stat name matches.

- [ ] **Step 2: Write the driver**

`test/fixtures/0099-http-tap-headers/driver/driver.go`:

```go
// Package driver implements the 0099-http-tap-headers differential fixture.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
)

const (
	refListenerPort = 10000
	refTapDir       = "/envoy-go-test/taps" // non-sticky container dir (see HostMount.Dir)
	refTapPrefix    = refTapDir + "/out"
	statPrefix      = "tap_probe"

	nMatch    = 3 // requests carrying x-tap: yes  -> tapped
	nNonMatch = 2 // requests carrying x-tap: no   -> NOT tapped
)

type tapDriver struct {
	refDir  string // host dir bind-mounted onto refTapDir
	subjDir string // host dir the subject writes into directly
}

func newTapDriver() *tapDriver {
	base := os.TempDir()
	tag := fmt.Sprintf("envoy-go-0099-%d-%d", os.Getpid(), time.Now().UnixNano())
	return &tapDriver{
		refDir:  filepath.Join(base, tag+"-reference"),
		subjDir: filepath.Join(base, tag+"-subject"),
	}
}

func init() { fixture.RegisterFixture("0099-http-tap-headers", newTapDriver()) }

// Compile-time interface assertions.
var (
	_ fixture.Driver             = (*tapDriver)(nil)
	_ fixture.BackendKindAware   = (*tapDriver)(nil)
	_ fixture.ReferenceLogMounter = (*tapDriver)(nil)
	_ fixture.StatsAsserter      = (*tapDriver)(nil)
)

// BackendCount returns 1: the runner rejects 0. The single HTTPStatusHeader
// backend is the real upstream (it returns the 204 both sides proxy).
func (d *tapDriver) BackendCount() int                 { return 1 }
func (d *tapDriver) BackendKind() fixture.BackendKind  { return fixture.HTTPStatusHeader }
func (d *tapDriver) SubjectListenerName() string       { return "listener_0" }
func (d *tapDriver) ReferenceListenerPort() int        { return refListenerPort }

// ReferenceHostMounts bind-mounts a host DIRECTORY over the container's tap
// output dir: file_per_tap creates <prefix>_<trace_id>.json per stream, and the
// test cannot predict those names (D-TAP-DIRMOUNT).
func (d *tapDriver) ReferenceHostMounts() []fixture.HostMount {
	return []fixture.HostMount{{HostPath: d.refDir, ContainerPath: refTapDir, Dir: true}}
}

func (d *tapDriver) subjPrefix() string { return filepath.Join(d.subjDir, "out") }

func mustRender(t string, data any) string {
	tmpl := template.Must(template.New("cfg").Parse(t))
	var b bytes.Buffer
	if err := tmpl.Execute(&b, data); err != nil {
		panic(err)
	}
	return b.String()
}

func (d *tapDriver) ReferenceBootstrap(backendPorts []int) string {
	return mustRender(referenceTmpl, map[string]any{
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"TapPrefix":    refTapPrefix,
	})
}

func (d *tapDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return mustRender(subjectTmpl, map[string]any{
		"ListenerPort": subjListenerPort,
		"AdminPort":    subjAdminPort,
		"BackendHost":  "127.0.0.1",
		"BackendPort":  backendPorts[0],
		"TapPrefix":    d.subjPrefix(),
	})
}

func (d *tapDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}
func (d *tapDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.drive(ctx, addr)
}

// drive sends nMatch matching + nNonMatch non-matching BODYLESS GETs. The
// backend echoes X-Backend-Status, so every response is a bodyless 204 --
// which is what makes both sides emit structurally headers-only traces.
func (d *tapDriver) drive(ctx context.Context, addr string) ([]byte, error) {
	c := &http.Client{Timeout: 5 * time.Second}
	var out bytes.Buffer
	send := func(xtap string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/tap", nil)
		if err != nil {
			return err
		}
		req.Header.Set("x-tap", xtap)
		req.Header.Set("x-backend-status", "204")
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		fmt.Fprintf(&out, "%s %d\n", xtap, resp.StatusCode)
		return nil
	}
	for i := 0; i < nMatch; i++ {
		if err := send("yes"); err != nil {
			return nil, err
		}
	}
	for i := 0; i < nNonMatch; i++ {
		if err := send("no"); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func (d *tapDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
	ref, err := fetch(ctx, "http://"+refAdminAddr+"/stats")
	if err != nil {
		return nil, nil, err
	}
	subj, err := fetch(ctx, "http://"+subjAdminAddr+"/stats")
	if err != nil {
		return nil, nil, err
	}
	return ref, subj, nil
}

func fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(resp.Body)
}

// referenceTmpl / subjectTmpl are the two YAML documents from Step 1.
const referenceTmpl = `...`
const subjectTmpl = `...`
```

> Embed the two YAML documents as the `referenceTmpl` / `subjectTmpl` consts (the `0006-access-log` driver does exactly this at `driver.go:568`/`:625`), **or** `os.ReadFile` the `.yaml` files the way `0018-http-rbac` does (`driver.go:176`/`:216`). Pick whichever the neighbouring fixtures do; do not invent a third mechanism.

- [ ] **Step 3: Blank-import the driver in the runner**

Add to `test/differential/runner_test.go`'s import block (~`:26-125`), in numeric order:

```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0099-http-tap-headers/driver"
```

**Without this the subtest silently `t.Skipf`s** (`runner_test.go:179`) — a vacuous green.

- [ ] **Step 4: Confirm the fixture is discovered and NOT skipped**

```bash
go test ./test/differential/ -run 'TestDifferential/0099-http-tap-headers' -count=1 -v 2>&1 | tail -20
```
Expected: the subtest RUNS (it may still fail — `AssertStats` lands in Task 14). **If you see `--- SKIP`, the blank import is missing.** Also confirm the fixture count:

```bash
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l   # expect 101
```

- [ ] **Step 5: Gates + commit**

```bash
gofmt -l test/ && golangci-lint run ./test/... && go vet ./... && go build ./...
git add test/fixtures/0099-http-tap-headers/ test/differential/runner_test.go
git commit -m "phase 56.1 T13: fixture 0099-http-tap-headers (GET->204 headers-only, 3 match + 2 non-match); fixtures 100 -> 101"
```

---

## Task 14: `0099` `AssertStats` — the glob-and-decode assertions + FIVE deliberate breaks + flake/race gates

**Files:**
- Modify: `test/fixtures/0099-http-tap-headers/driver/driver.go` (add `AssertStats`)

**Interfaces:**
- Consumes: `fixture.TB` (`Errorf`/`Fatalf`/`Helper` only — **no `Logf`**).
- Produces: the cross-side assertions.

> **`AssertStats`, not `AssertSubject`.** `SubjectAsserter` fires **only** on the reference-less path (`runner_test.go:2039-2041`); `0099` is cross-side, so `AssertSubject` would **never run** — a vacuous green (`reference_differential_asserter_dispatch`). All trace assertions go in `AssertStats` (`runner_test.go:1304-1306`), which receives both admin addresses.

**Assertion roster** — each an independent `t.Errorf` property (`reference_fatalf_makes_assertions_unreachable`; `Fatalf` only for a broken precondition such as an unreadable file):

1. reference trace count == `nMatch` (3); subject trace count == `nMatch`.
2. `http.tap_probe.tap.rq_tapped` == 3 on **both** sides.
3. per trace: `request.headers` ⊇ `{:method: GET, :path: /tap, x-tap: yes, x-backend-status: 204}`.
4. per trace: `response.headers` ⊇ `{:status: 204, content-type: text/plain}`.
5. per trace (**decoded**): `request.body` and `response.body` **ABSENT** (`GetBody() == nil`).
5b. per trace (**RAW BYTES**): the file contains no `"body"` key at all.
6. per trace: `len(request.trailers) == 0` and `len(response.trailers) == 0`.
7. per trace: `downstream_connection` **ABSENT**.

> **Why (5) and (5b) are BOTH required — the trap that nearly shipped.** A decode-based check **cannot distinguish an omitted field from an explicit `null`**. `protojson.Unmarshal` of `"body": null` yields a **nil** `Body`, exactly like an omitted `body`. Verified by execution:
> ```
> BAD file contains "body": null:                       true
> after decoding it: req.Body==nil? true  resp.Body==nil? true
> would assertion(5) `GetBody()!=nil` fire?             false
> ```
> So a regression to `EmitUnpopulated` renders `"body": null` on the wire and **assertion (5) stays green**. Only the raw-bytes assertion (5b) catches it. Conversely, (5b) alone would not catch a genuinely-populated body (which renders `"body": {...}` — a *different* substring, still containing `"body"`, so (5b) does fire — but (5) names the defect precisely). Keep both; they have different deliberate breaks.

- [ ] **Step 1: Write `AssertStats`**

```go
func (d *tapDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	// (1) trace counts. The reference writes at stream end but the container's
	// filesystem lands asynchronously from the host's view; poll it. The subject
	// writes in-process and is already done.
	refFiles := pollTraces(t, d.refDir, nMatch, 30*time.Second)
	subjFiles := pollTraces(t, d.subjDir, nMatch, 5*time.Second)
	if got := len(refFiles); got != nMatch {
		t.Errorf("reference trace count = %d, want %d (no trace for the %d non-matching streams)", got, nMatch, nNonMatch)
	}
	if got := len(subjFiles); got != nMatch {
		t.Errorf("subject trace count = %d, want %d (no trace for the %d non-matching streams)", got, nMatch, nNonMatch)
	}

	// (2) rq_tapped on both sides.
	const statName = "http." + statPrefix + ".tap.rq_tapped"
	if got := scrapeCounter(t, ctx, refAdminAddr, statName); got != nMatch {
		t.Errorf("reference %s = %d, want %d", statName, got, nMatch)
	}
	if got := scrapeCounter(t, ctx, subjAdminAddr, statName); got != nMatch {
		t.Errorf("subject %s = %d, want %d", statName, got, nMatch)
	}

	// (3)-(7) per-trace payload assertions on BOTH sides.
	wantReq := map[string]string{
		":method": "GET", ":path": "/tap", "x-tap": "yes", "x-backend-status": "204",
	}
	wantResp := map[string]string{":status": "204", "content-type": "text/plain"}
	assertSide(t, "reference", refFiles, wantReq, wantResp)
	assertSide(t, "subject", subjFiles, wantReq, wantResp)
}

func assertSide(t fixture.TB, side string, files []string, wantReq, wantResp map[string]string) {
	t.Helper()
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: read %s: %v", side, path, err) // broken precondition
		}
		var tw datatapv3.TraceWrapper
		if err := protojson.Unmarshal(b, &tw); err != nil {
			t.Fatalf("%s: decode %s: %v", side, filepath.Base(path), err) // broken precondition
		}
		bt := tw.GetHttpBufferedTrace()
		if bt == nil {
			t.Fatalf("%s: %s: trace is not an http_buffered_trace", side, filepath.Base(path))
		}
		name := side + "/" + filepath.Base(path)

		// (5b) RAW BYTES: no "body" key at all. A decode cannot tell an OMITTED
		// field from an explicit `null` -- protojson unmarshals `"body": null`
		// straight back to a nil Body -- so a regression to EmitUnpopulated is
		// invisible to assertion (5) below. This is the only check that sees it.
		if bytes.Contains(b, []byte(`"body"`)) {
			t.Errorf("%s: raw trace must contain NO \"body\" key "+
				"(EmitDefaultValues omits nil message fields; EmitUnpopulated would render \"body\": null)", name)
		}

		// (3) request header subset. D-TAP-SUBSET: :authority, :scheme,
		// x-request-id, x-forwarded-proto, x-envoy-*, date, server, connection and
		// user-agent are UNasserted coverage boundaries.
		assertSubset(t, name+" request.headers", bt.GetRequest().GetHeaders(), wantReq)
		// (4) response header subset.
		assertSubset(t, name+" response.headers", bt.GetResponse().GetHeaders(), wantResp)

		// (5) bodies ABSENT -- the GET->204 shape makes this a POSITIVE assertion.
		if bt.GetRequest().GetBody() != nil {
			t.Errorf("%s: request.body must be ABSENT (bodyless GET); got %v", name, bt.GetRequest().GetBody())
		}
		if bt.GetResponse().GetBody() != nil {
			t.Errorf("%s: response.body must be ABSENT (204 No Content); got %v", name, bt.GetResponse().GetBody())
		}

		// (6) trailers == [] -- cross-side EXACT despite envoy-go never seeing a
		// trailer, because EmitDefaultValues renders empty repeated as [].
		if n := len(bt.GetRequest().GetTrailers()); n != 0 {
			t.Errorf("%s: request.trailers must be empty; got %d", name, n)
		}
		if n := len(bt.GetResponse().GetTrailers()); n != 0 {
			t.Errorf("%s: response.trailers must be empty; got %d", name, n)
		}

		// (7) downstream_connection ABSENT (both record_* flags unset).
		if bt.GetDownstreamConnection() != nil {
			t.Errorf("%s: downstream_connection must be ABSENT; got %v", name, bt.GetDownstreamConnection())
		}
	}
}

// assertSubset checks want ⊆ got, comparing as a SET: header ORDER is an
// UNasserted boundary (envoy-go sorts; the reference emits codec order).
func assertSubset(t fixture.TB, what string, got []*corev3.HeaderValue, want map[string]string) {
	t.Helper()
	have := make(map[string]string, len(got))
	for _, hv := range got {
		have[hv.GetKey()] = hv.GetValue()
	}
	for k, v := range want {
		gv, ok := have[k]
		if !ok {
			t.Errorf("%s: missing key %q (have %v)", what, k, keysOf(have))
			continue
		}
		if gv != v {
			t.Errorf("%s: key %q = %q, want %q", what, k, gv, v)
		}
	}
}
```

> `assertSide` needs `bytes` in its import set (for the (5b) raw-bytes check), alongside `os`, `path/filepath`, `protojson`, `corev3`, `datatapv3`.
>
> Also write `pollTraces(t, dir, want int, deadline time.Duration) []string` (glob `filepath.Join(dir, "out_*.json")` until `len == want` or the deadline expires, returning whatever it last saw — **never `Fatalf` on a short count**, so assertion (1) reports the real number), `scrapeCounter(t, ctx, adminAddr, name) int` (fetch `/stats`, find the `"<name>: <n>"` line; `Fatalf` if the line is absent — a missing counter is a broken precondition, and on the reference side its absence would mean the filter never parsed), and `keysOf(map[string]string) []string` (sorted, for readable failures).
>
> **The `/stats` line format is confirmed identical on both sides.** envoy-go's admin `writeFlat` (`internal/admin/stats.go:37-40`) emits one `"<name>: <value>\n"` line per metric, matching reference Envoy's plain `/stats`. Scrape plain `/stats`, **not** `/stats/prometheus` (whose names are mangled by `ExtractTags`).
>
> **Temp-dir lifetime:** `newTapDriver()` allocates `refDir`/`subjDir` under `os.TempDir()` at `init()`, so they outlive the test — this **leaks two directories per run**. That matches the `0006-access-log` precedent (`driver.go:70-77`), and `fixture.TB` has no `Cleanup` method, so a driver cannot register one. Accept the leak, consistent with `0006`; do not invent a cleanup mechanism this row. The reference's trace files land owned by uid 101 mode `0o644`; the host dir is `0o777` and test-owned, so they remain readable and unlinkable.

- [ ] **Step 2: Run the fixture green**

```bash
go test ./test/differential/ -run 'TestDifferential/0099-http-tap-headers' -count=1 -v
```
Expected: PASS. **Never use a bare `-run '0099'`** — it matches zero subtests and reports a vacuous PASS (`reference_differential_run_selector`).

- [ ] **Step 3: Commit BEFORE breaking**

```bash
git add test/fixtures/0099-http-tap-headers/
git commit -m "phase 56.1 T14: 0099 AssertStats -- glob-and-decode cross-side trace assertions"
```
`git restore` reverts to **HEAD**, not to "before the break". Commit first, always.

- [ ] **Step 4: The SEVEN deliberate breaks (each `-count=1`; confirm WHICH assertion fired)**

For each: apply the break, run, **read the failure text and confirm the assertion that fired is the intended one** (`reference_deliberate_break_wrong_assertion` — a break failing does NOT prove your assertion is live; it can abort earlier and MASK the intended one). Then `git restore <file>` and `git branch --show-current`.

All breaks edit **subject-side production code**, so it is always the **subject** trace that violates the assertion; the reference trace stays conformant. Confirm the failure text names `subject/...`, not `reference/...`.

| # | Break | Where | Must fire |
|---|---|---|---|
| **(a)** | Move the emit block from `OnDestroy` into the end of `DecodeHeaders` | `tap.go` | (1) *and* (2): **0** subject traces / `rq_tapped` 0 — the response arm is `Undetermined` at decode, so nothing matches. Proves the emit site is the stream end. |
| **(a′)** | `bt.Response = nil` in `buildTrace` | `trace.go` | (4): `response.headers` missing `:status`. **Isolates** the response-capture property, which (a) masks by producing no file at all. |
| **(b)** | Predicate always-true: `func (e *Evaluator) Resolve() bool { return true }` | `internal/matchpredicate/eval.go` | (1) **and** (2), independently: **5** traces (not 3) and `rq_tapped` 5. |
| **(c)** | `EmitUnpopulated: true` (drop `EmitDefaultValues`) | `sink.go` `marshalOpts` | **(5b) ONLY** — the raw-bytes check. Assertion (5) does **NOT** fire: `"body": null` decodes to a nil `Body`. If (5) fires, something else is wrong. |
| **(c′)** | Fabricate a body: `bt.Request.Body = &datatapv3.Body{BodyType: &datatapv3.Body_AsString{AsString: "x"}}` | `trace.go` | **(5)** *and* (5b): a real `"body": {...}` is both non-nil after decode and present in the raw bytes. This is the break that proves the decode-based body-absence check is live. |
| **(d)** | Populate `Trailers: toHeaderValues(f.reqHdrs)` on the request `Message` | `trace.go` | (6): subject `request.trailers` non-empty. Proves the trailers boundary is live. |
| **(e)** | Populate `bt.DownstreamConnection` unconditionally (ignore `cfg.recordConn`) | `trace.go` | (7): subject `downstream_connection` non-nil. Proves the record-flag gate is live (it becomes reachable only once T9's plumbing lands). |

```bash
# the pattern, per break:
go test ./test/differential/ -run 'TestDifferential/0099-http-tap-headers' -count=1 2>&1 | tail -30
git restore <the file>
git branch --show-current    # must still be the impl branch
```

**Break (c) is the subtle one, and it nearly shipped as a vacuous proof.** An earlier draft claimed it would fire assertion (5). It does not: `protojson.Unmarshal` maps `"body": null` to a nil `Body`, exactly like an omitted field, and `EmitUnpopulated` is a *superset* of `EmitDefaultValues` (so `raw_value: ""` and `trailers: []` still render). Under that draft the entire fixture stayed **green** under break (c) — no assertion fired at all. Adversarial review caught it by *executing* the round-trip. Hence: (c) proves (5b); (c′) proves (5).

Break (a) fires (1) and (2) but **cannot** exercise (3)–(7) — there is no trace to inspect. That is why (a′) exists.

**Assertion (3)** (request-header subset) is exercised on every green run (a missing request key fires it) and additionally by (a′)'s sibling if you wish to isolate it: drop `:path` from the `DecodeHeaders` copy and confirm (3) names the missing `":path"` key.

Record, for every break, the **literal failing assertion text** in `PROGRESS-56.1.md` — not a paraphrase.

- [ ] **Step 5: Flake gate + race gate + full suite**

Never run these concurrently (`feedback_pertask_gofmt_lint` / the unloaded-suite rule).

```bash
# 20 consecutive isolated runs -- the fixture must not flake
for i in $(seq 1 20); do
  go test ./test/differential/ -run 'TestDifferential/0099-http-tap-headers' -count=1 >/dev/null || echo "FLAKE on run $i"
done

# full package race (a later task's goroutine can make earlier tests racy)
go test ./internal/... -race -count=1

# the full 101-dir differential, UNLOADED
go test ./test/differential/ -count=1 -timeout 40m
```
A `subject ready: EOF` on an **unrelated** fixture is the known startup flake (`reference_differential_fullsuite_startup_flake`) — isolate-re-run that fixture to discriminate before treating it as a regression.

- [ ] **Step 6: Gates + commit**

```bash
gofmt -l . && go vet ./... && go build ./...
git add -A && git status --porcelain    # must be EMPTY after commit
git commit -m "phase 56.1 T14: 0099 five deliberate-break proofs + flake/race/full-suite gates"
```

---

## Task 15: Docs bundle — ADR-0273 + `BEHAVIOR_CONTRACT` + `STATE`/`ROADMAP`/`README`/`PROGRESS` + fuzzer reconcile (the ADR-0052 atomic landing)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (append **ADR-0273**)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Modify: `docs/envoy-go/phases/56-http-tap-filter/README.md`, `PROGRESS-56.1.md`

**Interfaces:** none (docs).

- [ ] **Step 1: Write ADR-0273**

Take the **§Context** verbatim from `SPEC-56.1.md` §13 (it was drafted there per ADR-0044) and add the `§Decision` + `§Consequences` bodies, which land at the IMPL. The §Decision must record, at minimum:

- the two library packages (`internal/headermatch` — a fresh exported 8-arm HeaderMatcher, **MIGRATE NOBODY**; `internal/matchpredicate` — the tri-state tree, depth cap **32**) and the in-package `filePerTapSink`;
- **ONE SHARED `*tapFilter`** in both `HTTPFilter` fields, and why (`chain.go:668-672`'s `else if`);
- `:status` from `EncoderFilterCallbacks.ResponseStatus()` injected into a **COPY** (the `ReconcileOrderedHeaders` wire-leak trap);
- emission unconditionally at stream end; the tri-state resolves, it does not gate timing;
- byte-exact `protojson` with `EmitDefaultValues` + trailing `\n`;
- `rq_tapped` increments on the **match decision**, not the write;
- the **PARITY vs DEPARTURE** reject roster (notably: `streaming_grpc` **ABORTS the reference process**, exit 139 — envoy-go's reject is strictly safer);
- the **seven** resolved D-questions, including the two the SPEC did not settle: `record_headers_received_time` is **rejected** (no accessor exposes the header-arrival instant) and `HostMount.Dir` (the harness could not bind-mount a directory);
- the trailers and `:scheme` **coverage boundaries**;
- the **`path_prefix` MkdirAll-at-parse** departure (unprobed against the reference);
- both JSON formats accepted at 56.1 (indistinguishable without a body); the 3 `PROTO_*` rejected.

- [ ] **Step 2: `BEHAVIOR_CONTRACT.md`** — add the SPEC §9 bundle: the buffered end-of-stream-artifact model; the tri-state + depth cap; the `http.<stat_prefix>.tap.rq_tapped` counter (ONE name **shape**, the `fault.aborts_injected` row at `:208` is the format precedent); the byte-exact `file_per_tap` rendering; the **trailers** and **`:scheme`** coverage boundaries; the full §6 reject roster with PARITY/DEPARTURE dispositions; the `path_prefix` MkdirAll departure.

- [ ] **Step 3: Reconcile every count**

```bash
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                                # 101
grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l        # 53
grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1                  # ADR-0273
grep -n 'BackendKind = ' test/differential/fixture/fixture.go | tail -1          # 38 (+0)
go mod tidy -diff                                                                # must print NOTHING
```
Stat surface **1200 → 1201** (H2 cluster; non-H2 **1196 → 1197**) — a documented reference figure, enforced in-tree by the Task-7 **+1 delta guard**, not by an absolute assertion. Fuzzer count: reconcile per `reference_fuzzer_count_docs_drift` **before** editing any doc that states the total.

- [ ] **Step 4: `ROADMAP.md` row 56 — STAYS `in-progress`**

Row 56 flips `done` **only at the 56.2 IMPL** (`reference_roadmap_split_phase_row_done` + ADR-0106 — no parent rollup). Update its summary to `56.1 IMPL done, 56.2 planned`. The **Observability family STAYS OPEN** (its deferred-candidate list is non-empty).

- [ ] **Step 5: `STATE.md`** — set `active-phase: phase 56.1 (http-tap-filter, headers leg) IMPL done`, demote the previous entry to `prior active-phase`, and record the exit counts.

- [ ] **Step 6: Final verification on the frozen HEAD**

```bash
go build ./... && go vet ./... && gofmt -l . && golangci-lint run ./...
go test ./internal/... -count=1
go test ./internal/... -race -count=1
go test ./test/differential/ -count=1 -timeout 40m
git -C /home/esa/git/envoy-go status --porcelain   # the MAIN checkout must be EMPTY
```

- [ ] **Step 7: Commit**

```bash
git add docs/
git commit -m "phase 56.1 T15: ADR-0273 + BEHAVIOR_CONTRACT + STATE/ROADMAP/README/PROGRESS; row 56 stays in-progress"
```

---

## Final review + handoff

Before declaring the leg done, the **controller** (not a task subagent) re-performs:

1. **Every deliberate break, itself** — Task 7 Step 5 (3 rejects), Task 8 Step 6 (wire leak), Task 9 Step 5 (2 shared-value breaks), Task 14 Step 4 (7 fixture breaks). Each with `-count=1`, each confirming **which** assertion fired — and, for the fixture breaks, that it fired on the **subject** side.

   Pay particular attention to break (c): it must fire **(5b) only**. If it also fires (5), or fires nothing, stop — the body-absence proof is compromised.
2. **The full suite on the frozen HEAD**, unloaded, after the last commit.
3. **Citation re-derivation** — every `file:line` in ADR-0273 and `BEHAVIOR_CONTRACT` re-checked **against source**, never against this PLAN or the SPEC (`feedback_brief_citations_not_evidence`).
4. **The ADR-0045 margin** — if the IMPL grew past 15 tasks, say so plainly and re-open the gate rather than quietly widening 56.2.
5. **Squash + push** (`feedback_subagent_autocommit_claudemd` · `feedback_push_to_origin`), then roll `next-prompt.txt` forward to **phase 56.2 (bodies)** and fold it into the squash (`reference_next_prompt_tracked_despite_gitignore`).

**Exit counts (anticipated):** stat surface **1201** · fixtures **101** · fuzzers **53** · BackendKind tail **38** · DECISIONS tail **ADR-0273** (next-free ADR-0274) · new Go packages **2** + the filter package · new go.mod modules **0**. Row 56 **stays `in-progress`**; the Observability family **stays OPEN**.
