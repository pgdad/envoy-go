# Test Gap Analysis — envoy-go

**Date:** 2026-07-11
**Reference Envoy pin:** `envoyproxy/envoy:contrib-v1.37.2` (per `docs/envoy-go/ENVOY_TARGET.md`)
**Scope:** analysis of the tests that verify *proper functionality* against upstream
Envoy's observable behavior, the highest-value gaps, and the enhancements landed in
this pass.

> This document is orthogonal to the phase-driven workflow (like `REVIEW_FINDINGS.md`).
> It does not modify anything under `docs/envoy-go/` or `test/fixtures/`. Its
> implementation touches only test files and CI config.

---

## 1. Baseline (clean checkout, 2026-07-11)

| Signal | Value |
|---|---|
| Go toolchain | go1.26 (module declares `go 1.23`) |
| Go source files | 928 |
| Test files | 408 |
| Fuzz targets | 46 (across 40+ packages) |
| Differential fixtures | 101 (`0000`–`0100`), Docker-based vs real Envoy |
| Conformance suites | h2spec (53/53 sections), proxy-wasm (10 families ported / 6 deferred) |
| Statement coverage (`-short`, `internal/…`+`validate/…`) | **84.5%** |
| Unit suite `go test -short -race ./...` | **PASS**, ~90s wall |
| Differential suite `go test ./test/differential/ -count=1` | **PASS**, 101 fixtures, ~5m21s |

**Nothing failed on a clean checkout.** The suite is deterministic: ephemeral ports
everywhere (`:0`, 176 sites), injectable/fake clocks for every time-sensitive filter
(`internal/clock`, plus `nowNanos` seams in cluster health/outlier), no external
network access, and documented/gated skips only. Test hygiene is high.

### Lowest-coverage implemented packages (blind-spot finder, not a target)

```
47.1%  internal/filter/http/wasm          (wazero host glue; much is error-path)
48.0%  internal/filter/http/rbac
57.7%  internal/matcher
59.3%  internal/filter/network/directresponse
61.1%  internal/filter/http/localratelimit
63.3%  internal/boot
63.6%  internal/filter/http/cors
```

These are mostly error/branch tails, not whole untested features. Coverage was used
only to locate blind spots, not chased as a number.

---

## 2. Gap analysis vs real Envoy (priority order)

### 2.1 Protocol robustness — HTTP/1.1 downstream request validation ⚠️ **HIGHEST VALUE**

The H1 read loop (`internal/filter/hcm/connection.go:163`) delegates request framing
**entirely to Go's `net/http` `http.ReadRequest`**. Go's parser and Envoy's HTTP/1
codec disagree on several adversarial, smuggling-shaped inputs. **Before this pass there
was zero test coverage feeding raw adversarial request bytes** — the only raw-byte H1
negative test was a single `"GARBAGE\r\n\r\n"` → 400 case.

I drove the same 12 raw inputs through both `runConnection` (envoy-go) and the pinned
reference Envoy image (minimal HTTP1 direct-response listener) and diffed the status:

| Raw request shape | envoy-go | Envoy v1.37.2 | Verdict |
|---|---|---|---|
| well-formed `GET /` | 200 | 200 | match ✓ |
| bare-LF line endings | 200 | 200 | match ✓ (both lenient) |
| conflicting duplicate `Content-Length` | 400 | 400 | match ✓ |
| non-numeric `Content-Length` | 400 | 400 | match ✓ |
| negative `Content-Length` | 400 | 400 | match ✓ (RFC-correct) |
| bare CR in header value | 400 | 400 | match ✓ (RFC-correct) |
| space in method token (`GE T`) | 400 | 400 | match ✓ |
| `Transfer-Encoding: chunked` then `identity` | 400 | 501 | both reject (status differs) |
| **`Transfer-Encoding: chunked` + `Content-Length`** | **200** | **400** | **DIVERGENCE** |
| **identical duplicate `Content-Length`** | **200** | **400** | **DIVERGENCE** |
| **whitespace before header colon (`Foo : bar`)** | **200** | **400** | **DIVERGENCE** |
| **unsupported version (`HTTP/9.9`)** | **200** | **426** | **DIVERGENCE** |

**Four confirmed conformance gaps where envoy-go silently *accepts* (200) a request that
Envoy *rejects*.** The most serious is **TE + Content-Length together** — the canonical
HTTP request-smuggling vector (RFC 9112 §6.1, CWE-444): `net/http` resolves the ambiguity
by dropping `Content-Length` and treating the body as chunked, whereas Envoy rejects the
message outright. In a topology where envoy-go fronts (or is fronted by) a component that
frames the *other* way, this is a genuine request-smuggling exposure.

Root cause is architectural: framing validation is outsourced to `net/http`, which is
lenient-by-design, and envoy-go adds no pre-dispatch strict-framing gate. This was not
documented anywhere in `BEHAVIOR_CONTRACT.md` (which only records H1 *response*-side
framing latitude), so it is an untested gap rather than a recorded deviation.

**Landed:** `internal/filter/hcm/connection_robustness_test.go` — deterministic,
no-Docker, drives `runConnection` with each raw shape. Two groups:
`TestH1Robustness_ConformantRejections` (locks in the correct 4xx behavior) and
`TestH1Robustness_KnownDivergencesFromEnvoy` (characterizes the four divergences, asserts
current behavior so CI stays green, logs each as a tracked divergence, and fails loudly —
with a pointer to move the case — if envoy-go is ever hardened to match Envoy).

**Not fixed** (deliberately): making envoy-go reject these changes observable behavior on
a path the differential suite covers and, per this repo's doctrine (`REVIEW_FINDINGS.md`
"Deferred" section, ADR discipline), belongs in a dedicated phase with a new differential
fixture + `BEHAVIOR_CONTRACT` extension + ADR — not a test-only pass. See §4.

### 2.2 Concurrency — `-race` was not in CI

CI's unit job ran `go test -short ./...` **without `-race`**, despite:
- an extensive documented race-fix history (`REVIEW_FINDINGS.md`: h2 flow-control,
  wasm tick/close, extproc, adaptive_concurrency, listener/sds, …), and
- multiple tests whose *only* assertion is that `-race` stays clean
  (`internal/filter/network/chain_test.go` "the 28.1b race surface must be EMPTY",
  `sdsfile` debounce-race group, kafka/mongo codec `-race -count=5` stress tests).

Without `-race` in CI those tests are vacuous there. **Landed:** unit job now runs
`go test -short -race ./...` (verified green locally).

### 2.3 Fuzzing — only 1 of 46 targets got a CI engine run

CI smoke-ran only `FuzzBootstrapLoad` (30s). The seed corpora of the other 45 targets do
run as ordinary unit tests under `go test ./...`, but the mutation *engine* — the part
that finds *new* crashers — ran on nothing else. The riskiest attacker-reachable parsers
(HCM typed_config, access-log format strings, HTTP/2 HPACK/frame codec, TLS context, and
the Kafka/Mongo/RESP wire codecs) had no engine coverage. **Landed:** the `fuzz-bootstrap`
job is now a `fuzz-smoke` matrix of 9 targets × 30s (bounded; each smoke-verified clean
locally).

### 2.4 Negative / error paths — partial (gaps recorded for future work)

Well covered: dial-failure→503, write/read-header-failure→502, circuit-breaker/conn-pool
overflow→503, retry/per-try-timeout taxonomy, buffer-filter 413 overflow, drain
transitions, TLS *config-build* rejections. Gaps (see §4 for ranking):
- **Upstream connection reset *mid-response-body*** (`router.go:625-628`) is a distinct
  code path (returns `ActionResponse{Status: resp.StatusCode}, picked, err` with a partial
  status and no body) exercised by no test.
- **Runtime TLS handshake failures** — partially closed in this pass (see §3): a live
  aborted-handshake test for unmatched-SNI-with-no-catch-all now exists. Still uncovered:
  missing client cert vs `RequireAndVerifyClientCert` (blocked — envoy-go build-rejects
  `require_client_certificate`, so there is no runtime mTLS to drive) and ALPN negotiation
  failure.
- **No downstream `idle_timeout` / `stream_idle_timeout` / `request_timeout`** exists at
  the HCM level at all — a slow/idle client is never timed out (feature gap, not just a
  test gap).
- **No `max_request_headers_kb` / H2 `SETTINGS_MAX_HEADER_LIST_SIZE`** enforcement.

### 2.5 xDS / dynamic config — absent by design

`internal/xds/` is a `doc.go` placeholder; config is bootstrap-static. There is no
LDS/CDS/RDS/EDS handling and therefore no test for dynamic updates, invalid updates, or
removing in-use resources. Only **secret** (cert) reload exists (`internal/sdsfile`,
fsnotify-based, well-tested in isolation) — but its integration with a *live* listener's
`tls.Config` under load is not demonstrated by any test. This is a roadmap item, not a
regression; noted for completeness.

### 2.6 Test quality — high; no rot to clean up

Audited: no tautological tests in the sampled filters (expected values are
algebraically/spec-derived, byte-exact goldens are anchored to the differential harness),
ports are ephemeral, timing filters use fake clocks, skips are gated and documented, no
internet access. The only residual flakiness surface is a handful of goroutine-ordering
`time.Sleep` sites in `hcm/h2` and `tcpproxy` tests and two self-described "racy-by-design"
extauthz `OnDestroy` tests — candidates to convert to channel/`sync` handshakes only *if*
intermittent CI failures ever appear. Not worth pre-emptive churn.

---

## 3. What was implemented in this pass

| Change | File | Type | Risk |
|---|---|---|---|
| HTTP/1.1 request-validation robustness suite (12 shapes; 4 divergences pinned) | `internal/filter/hcm/connection_robustness_test.go` (new) | New tests, deterministic, no Docker | none (test-only) |
| Live TLS handshake abort on unmatched SNI with no catch-all (accept-loop path, not just chain selection) | `internal/listener/tls_handshake_negative_test.go` (new) | New test, deterministic, no Docker | none (test-only) |
| `-race` on the CI unit job | `.github/workflows/ci.yml` | CI hardening | none |
| Fuzz smoke matrix: 1 → 9 riskiest parsers | `.github/workflows/ci.yml` | CI hardening | none |

**Verification:** `gofmt` clean, `go build ./...` clean, `go vet ./internal/filter/hcm/`
clean, `golangci-lint` clean, `go test -short -race ./...` **PASS** (exit 0). No
production code changed; no differential fixture or `docs/envoy-go/` doc touched.

---

## 4. Ranked remaining recommended work

Ordered by (a) risk of silent incorrectness vs Envoy, (b) breadth of code exercised,
(c) cost.

1. **Harden H1 request framing to Envoy semantics + pin it differentially.** Add a
   pre-dispatch strict-framing gate (reject TE+CL, repeated Content-Length,
   OWS-before-colon, unsupported version) and a new `01xx-http11-request-validation`
   differential fixture with a raw-TCP driver comparing status against reference Envoy.
   This is a *behavior change* → needs an ADR + `BEHAVIOR_CONTRACT` extension per repo
   doctrine. **Highest value: closes a request-smuggling exposure and converts the four
   §2.1 divergences from tracked to fixed.** Medium effort; reuses the differential harness
   (no new infra).

2. **Upstream mid-response-body reset test** (`router.go:625`). Backend sends headers then
   half-closes/resets mid-body; assert envoy-go's downstream-visible behavior against
   reference Envoy (differential, because the correct outcome — truncated 200 vs 502 vs
   stream-reset — must be pinned to Envoy, not guessed). Low-medium effort.

3. **Remaining runtime TLS handshake-failure tests.** The unmatched-SNI-no-catch-all abort
   is now covered (§3). Still open: ALPN negotiation failure; and missing-client-cert
   rejection — the latter first needs `require_client_certificate` to be a supported runtime
   feature (it currently build-rejects), so feature + test land together. Low-medium effort.

4. **Downstream timeout coverage** — but the feature (`idle_timeout` /
   `stream_idle_timeout` / `request_timeout`) does not exist yet; the test and the feature
   land together. Thread `internal/clock` into router/retry/hedge so those timeout tests
   become deterministic (they currently use real wall-clock sleeps). Medium effort.

5. **Oversized-header enforcement** (`max_request_headers_kb`, H2 header-list-size) — feature
   + test. Medium effort.

6. **Flakiness pre-emption** (optional): convert the ~10 goroutine-ordering `time.Sleep`
   sites in `hcm/h2`/`tcpproxy` tests to `sync`/channel handshakes if CI ever flakes.
