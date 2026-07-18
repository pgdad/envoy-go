# Test Gap Analysis — envoy-go

> **Second pass appended 2026-07-17** — see the dated section at the end of this
> document. The first-pass §2.5 statement "xDS absent by design" is superseded:
> phases 57–66 landed an SDS subsystem, a QUIC/HTTP-3 listener, and more.

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
  aborted-handshake test for unmatched-SNI-with-no-catch-all now exists. Missing-client-cert
  rejection vs `RequireAndVerifyClientCert` is also covered: `require_client_certificate` has
  been a supported runtime feature since phase 16/ADR-0147 (this bullet's "build-rejects"
  claim was stale against that row, pre-dating phase 67 — corrected here in passing, not new
  drift this row created), and verify-if-presented mTLS at false/absent landed at phase
  67/ADR-0289. Runtime mTLS is drivable and exercised by fixtures 0018/0108/0109/0110. Still
  uncovered: ALPN negotiation failure.
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
   is now covered (§3). Missing-client-cert rejection is also now covered: `require_client_certificate`
   has been a supported runtime feature since phase 16/ADR-0147 (this item's "build-rejects"
   framing was stale against that row, pre-dating phase 67 — corrected here in passing) and
   verify-if-presented at false/absent landed at phase 67/ADR-0289 — see fixtures
   0018/0108/0109/0110. Still open: ALPN negotiation failure. Low-medium effort.

4. **Downstream timeout coverage** — but the feature (`idle_timeout` /
   `stream_idle_timeout` / `request_timeout`) does not exist yet; the test and the feature
   land together. Thread `internal/clock` into router/retry/hedge so those timeout tests
   become deterministic (they currently use real wall-clock sleeps). Medium effort.

5. **Oversized-header enforcement** (`max_request_headers_kb`, H2 header-list-size) — feature
   + test. Medium effort.

6. **Flakiness pre-emption** (optional): convert the ~10 goroutine-ordering `time.Sleep`
   sites in `hcm/h2`/`tcpproxy` tests to `sync`/channel handshakes if CI ever flakes.

---

# Second pass — 2026-07-17

**Branch:** `test-review-20260717` (from `facb0faa`).
**Reference Envoy pin:** unchanged, `envoyproxy/envoy:contrib-v1.37.2`.
**Scope:** the 58 commits since the first pass (`39e2140e..facb0faa`) landed phases
57–66 — a MAJOR new surface the 2026-07-11 analysis predates entirely: an xDS/SDS
subsystem (`internal/xds`, SotW gRPC stream, SDS `tls_certificate` /
`validation_context` / `combined_validation_context`), a QUIC/HTTP-3 downstream
listener (quic-go v0.54.1, the first external module), a Graphite statsd sink, and
four tracing rows. §2.5's "xDS absent by design" is now FALSE. This pass is the
same kind of orthogonal test-quality overlay as the first: test files + CI config
only, no production code, no `docs/envoy-go/` state, no fixture changes.

## 5. Baseline (2026-07-17, clean checkout of facb0faa)

| Signal | First pass | This pass |
|---|---|---|
| Go source files | 928 | 976 |
| Test files | 408 | 426 (430 after this pass) |
| Fuzz targets | 46 | 55 |
| Differential fixtures | 101 | 111 (`0000`–`0109`) |
| Unit suite `go test -short -race ./...` | PASS ~90s | **PASS** (124 pkgs; 16s wall / 1m42s CPU on this host) |
| Differential suite | PASS ~5m21s | **PASS** (see caveat below) |

**Environment finding (not a code failure):** during the first baseline run the
local Docker Desktop daemon died mid-suite; the differential package's
`TestHostGatewayIP` pre-check hung for the full 10-minute package timeout against
the half-dead socket, failing the run. After a daemon restart the suite is green.
The differential harness could bound its Docker liveness probe (a dial timeout on
the socket) so a dying daemon fails in seconds, not 10 minutes — noted in §8.

## 6. Gap analysis of the new surface

Sources: code inspection of `internal/xds`, `internal/boot`, `internal/tls`,
`internal/listener/quic.go`, `internal/filter/hcm/h3dispatch.go`, plus the phase
docs' own recorded coverage boundaries (`docs/envoy-go/phases/6*/PROGRESS*.md`).

### 6.1 What was already well covered (credit where due)

The phase-driven TDD process left the new surface in strong shape at the UNIT
level: SDS provider fetch classification (success / timeout / mgmt-down /
rejected, for both resource types), the `NewSDSProvider` pre-scan arms (all three
SDS shapes, the seen>1 compose-two reject, node-required, ADS reject), the
phase-66 E3 security reject — *(RETIRED at phase 67/ADR-0289.)* Previously, a CVC
listener with `require_client_certificate` false/absent CANNOT boot — pinned twice,
by message and by the `ClientAuth != NoClientCert` property test; phase 67 lifts
verify-if-presented across all three validation shapes and retires E3, so the
property test now pins `ClientAuth == VerifyClientCertIfGiven` for that shape, and
the pin-twice discipline (property test + differential fixture, now `0110`) carries
forward — all four CVC sub-field rejects
re-pointed at `default_validation_context`, malformed-served-secret rejection
(wrong name / wrong oneof / bad PEM / unsupported sub-fields), and an
`FuzzDiscoveryResponseParse` fuzzer over both Secret parse chains. The three
Docker differential fixtures 0103/0108/0109 pin the cross-side behavior.

### 6.2 Gaps found (and what this pass did about each)

1. **No in-process end-to-end SDS test existed.** Every joint was tested in
   isolation; nothing proved they COMPOSE — `bootstrap.Load → cluster manager →
   NewSDSProvider → Construct → lm.Start → live TLS handshake`. In particular:
   (a) nothing asserted a live handshake actually serves the SDS-delivered leaf;
   (b) nothing drove live mTLS against the SDS-delivered CA (the phase-67
   brainstorm's "Go cert-withholding vacuous-green driver trap" — a Go client
   silently withholds its cert when the server's CA advertisement doesn't match,
   so a naive test goes green without testing anything);
   (c) **the phase-66 Design A "pool substitution" had NO behavioral test**: the
   ADR-0287 equivalence theorem (P3/P5) says the SDS pool REPLACES the inline
   default CA, and its own code comment admits "if either ever diverges, this
   equivalence is SILENTLY falsified with no test to catch it";
   (d) the fail-closed boot posture (ADR-0280 departure; the phase-67 drift
   question D-RCCF-FETCHFAIL-POSTURE) was pinned only at the `internal/tls` unit
   level, never at the real apply-point.
   **Landed:** `internal/boot/boot_sds_e2e_test.go` — four test groups covering
   exactly (a)–(d): serial-exact SDS leaf on a live handshake + echo round-trip
   through tcp_proxy; mTLS accept/reject with FORCE-SENT client certs
   (`GetClientCertificate`) so reject tests cannot go vacuously green; CVC
   substitution (a leaf signed by the INLINE DEFAULT CA is REFUSED); silent
   SDS server (config-driven 200ms `initial_fetch_timeout`) and unreachable SDS
   server both FAIL the boot with no listener bound.

2. **A RECORDED coverage gap in the provider's error classification.**
   `phases/65-.../PROGRESS.md:122` honestly records that a deliberate break
   swapping the `errValidation`-before-`ctx.Err()` classification ordering fired
   NO test — no test created the discriminating condition (a validation failure
   arriving after the fetch deadline expired). **Landed:**
   `internal/xds/provider_classification_test.go` — a fake stream blocks on the
   provider's own ctx.Done(), then delivers a validation-failing response; the
   error must classify `update_rejected`, not `init_fetch_timeout`. Verified
   discriminating by re-running the recorded break (both new tests fire).

3. **QUIC listener had zero negative-path tests.** All three committed phase-61
   tests are positive-path; ALPN mismatch existed only as a TEMPORARY
   deliberate-break during 61.1, and nothing ever fed the UDP socket non-QUIC
   bytes. For a public-facing UDP listener the containment property matters: a
   failed handshake or garbage datagram must not exit the accept loop.
   **Landed:** `internal/listener/quic_negative_test.go` — ALPN-h2-only client
   refused AND the listener still serves h3 afterwards; garbage datagrams (short
   junk / 1200B noise / QUIC-long-header-shaped junk) neither crash nor wedge
   the listener (before/after control GETs guard against vacuous passes).

4. **First-pass ranked item 3 (ALPN negotiation failure, TCP TLS)** — still
   open from 2026-07-11. **Landed:** extended
   `internal/listener/tls_handshake_negative_test.go` with a live-handshake
   abort test (`no_application_protocol`) plus an overlapping-ALPN control that
   asserts the negotiated protocol.

5. **First-pass ranked item 2 (upstream mid-response-body death)** — the
   router's distinct `io.ReadAll` failure path (partial status + non-nil action
   error) had no test. **Landed:**
   `internal/filter/hcm/upstream_midbody_reset_test.go`, FIN and RST flavors.
   **Characterized behavior:** envoy-go's fully-buffered proxy has written
   nothing downstream when the body read fails and HCM gates the wire-write on
   `actionErr == nil`, so the downstream connection closes with ZERO response
   bytes and the connection loop exits. Reference Envoy (streaming) forwards
   the 200 header block and truncates. Both terminate without a deliverable
   complete response (no smuggling/desync class), but the wire shape differs —
   an architectural consequence of buffered proxying, recorded here as a
   characterized divergence like the four §2.1 H1 rows. If envoy-go ever
   streams responses, these tests fail loudly (the re-pin signal).

6. **Fuzz-smoke matrix lagged the new trust boundary.** The SDS Secret parser
   consumes bytes from the management server — a genuine trust boundary feeding
   X509 material into listener trust anchors — but only its seed corpus ran in
   CI. **Landed:** `FuzzDiscoveryResponseParse` added to the `fuzz-smoke`
   matrix (9 → 10 targets; local 20s engine run clean, ~1.5M execs).

### 6.3 Judged and deliberately NOT done this pass

- **Hardening H1 request framing (first-pass item 1)** — still open, still the
  right call to defer: it is a behavior change on a differential-covered path
  requiring an ADR + `BEHAVIOR_CONTRACT` extension per repo doctrine. The four
  divergences remain characterized in `connection_robustness_test.go`.
- **The D-RCCF-FETCHFAIL-POSTURE reference-side probe** — phase 67's own SPEC
  obligation (a discriminating server-cert-fetch-failure probe against the
  reference, reconciling BC:900/ADR-0286/config.go against the
  init-hold-then-fail-closed finding). That is doc/contract reconciliation work
  belonging to the phase machinery, not a test overlay. envoy-go's OWN posture
  (boot-FAIL, both resource types) is now integration-pinned (§6.2.1d).
- **H3 differential POST-body / access-log `Protocol=HTTP/3` cross-side
  assertions** (recorded deferrals in PROGRESS-61.3) — need harness surgery in
  the phase workflow's territory (`test/differential/harness_h3_test.go`).
- **Downstream idle/stream/request timeouts and `max_request_headers_kb`
  (first-pass items 4–5)** — feature + test land together; too large for a
  test-only pass, unchanged ranking.
- **`quicAcceptLoop` accept-error backoff (M6-1)** — the deferred robustness
  row's business; the error path is currently believed unreachable outside
  listener close (quic-go's Accept only errors terminally).

## 7. Verification

- `gofmt` clean, `go vet ./...` clean, `golangci-lint run` clean over every
  touched package, `go build ./...` clean.
- `go test -short -race -count=1 ./...`: **PASS**, 124 packages, 0 failures.
- `go test ./test/differential/ -count=1`: **PASS** after the Docker daemon
  restart (111 fixtures; test-only changes cannot affect it, run for the
  before/after record).
- The new classification tests were proven discriminating via the recorded
  deliberate break (ordering swap → both fire → restored green). The SDS e2e
  reject arms are guarded against vacuous passes by force-sent client certs and
  control dials; the QUIC negatives by before/after control GETs.
- No production code changed. Fixtures untouched (111). Fuzz targets unchanged
  (55 — a matrix row is not a new fuzzer).

## 8. Re-ranked remaining recommended work

1. **Harden H1 request framing to Envoy semantics + differential fixture**
   (unchanged #1; ADR + BEHAVIOR_CONTRACT + phase machinery required — the four
   characterized divergences include the TE+CL smuggling vector).
2. *(COMPLETED at phase 67/ADR-0289.)* The D-RCCF-FETCHFAIL-POSTURE drift is resolved:
   the discriminating reference probe (P1 — {server-cert, validation-context} ×
   {silent, unreachable}, all four cells identical) ran at the phase-67 SPEC, and
   all FIVE living serve-anyway sites — not three — are reconciled at the phase-67
   IMPL: `BEHAVIOR_CONTRACT.md:900` (B2), `internal/tls/config.go:115-117` (B11),
   the DECISIONS.md ADR-0286 D-SDSVC-FETCHTIMEOUT bullet (B12),
   `internal/xds/provider.go:91-93` (B16), and `internal/tls/config_test.go:999`
   (B17). The envoy-go-side posture was already locked by `boot_sds_e2e_test.go`
   either way.
3. **Downstream idle/stream/request timeout support + tests** (feature+test;
   thread `internal/clock` for determinism) — unchanged.
4. **Oversized-header enforcement** (`max_request_headers_kb`, H2
   `SETTINGS_MAX_HEADER_LIST_SIZE`) — unchanged.
5. **H3 differential depth**: POST-body arm and a cross-side
   `Protocol=HTTP/3` access-log assertion for fixture 0104's family; also a
   standing SDS-rotation story once watch/rotation lands (today's SDS is
   initial-fetch-only by design).
6. **Differential-harness Docker liveness probe timeout** (§5 environment
   finding): bound the socket probe so a dying daemon fails the suite in
   seconds instead of eating the 10-minute package timeout.
7. **Flakiness pre-emption** (unchanged, still optional): the ~10
   goroutine-ordering `time.Sleep` sites in `hcm/h2`/`tcpproxy` tests; none
   have flaked in CI to date.
