# Phase 20 — HTTP filter `envoy.filters.http.oauth2` (single-row landing) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.oauth2.v3.OAuth2` — the canonical Envoy v1.37.2 OAuth 2.0 authentication filter (Standard envelope per SPEC §1: sign-in + refresh + sign-out under the 07.1 framework) — as the THIRTEENTH §9 family-row by shipping the NEW `internal/filter/http/oauth2/` package (16 Go files / ~2850-3110 LoC; 5-cookie envelope; 6-counter wire-exact stat surface per AMEND-4 + S5; deny-path 302+401 only per AMEND-3; listener-scoped-only via HCM-parse-time PARSE-REJECT per SPEC §5.2; THIRD CONSECUTIVE §9 row to skip ADR-0125 amendment per §5.4), the NEW top-level `internal/httpclient/` framework primitive (~150-250 LoC; ADR-0177 — **CLOSES ADR-0159 §Future Work load-bearing forward-pointer**; **FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer** per SPEC §9 item 1), the NEW top-level `internal/sdsfile/` framework primitive (~160-200 LoC + NEW go.mod fsnotify dep; ADR-0178; ~100ms debounce + atomic.Pointer swap), the NEW filter-local AES-256-CBC token-encryption helper at `oauth2/tokens.go` (~150-200 LoC; ADR-0182 + AMEND-1 — algorithm REVISED from BRAINSTORM Q4 AES-GCM to upstream-byte-exact CBC per §20.P5 REFUTED), TWO IN-PLACE §Decision AMENDMENTs at ADR-0150 (jwks Fetcher consumes `*httpclient.Client`; ~40-60 LoC delta) + ADR-0159 (extauthz `httpAuthClient` consumes `*httpclient.Client` + §Future Work CLOSURE-AT-PHASE-20 paragraph; ~50-80 LoC delta), the differential fixture `0024-http-oauth2` (9 wire-level expectations across 8 scenario directories + 3-listener topology per planner-time D10), the NEW test-helper `test/helpers/oauthbackend/` (~250-350 LoC; FIRST in-tree OAuth-server mock), the 26th fuzzer `FuzzOAuth2ConfigParse`, the BEHAVIOR_CONTRACT.md 10-edit bundle per SPEC §13, and DECISIONS.md 9 NEW ADR §Decision + §Consequences bodies (ADR-0177..ADR-0185) + 2 IN-PLACE AMENDMENT bodies — with byte-equivalent wire outcomes against reference Envoy v1.37.2 on every observable axis except the two documented envoy-go-strict departures (token_endpoint non-2xx retry-eligible → 302 challenge per §4.7; POST callback PARSE-REJECT per §2.14). **Single-row landing** per ADR-0045 (SPEC-settled — phase-17 jwt_authn 3855 single-row precedent applies; LoC envelope ~3400-3900 post-empirical-scrape).

**Architecture:** The IMPL extends the existing 07.1 HTTP filter framework with TWO NEW top-level framework primitives (`internal/httpclient/` per ADR-0177; `internal/sdsfile/` per ADR-0178) + ONE NEW filter-local helper (AES-256-CBC at `oauth2/tokens.go` per ADR-0182) + TWO IN-PLACE prior-ADR refactors (ADR-0150 jwks Fetcher + ADR-0159 extauthz `httpAuthClient` both refactored to consume `*httpclient.Client` per ADR-0044 in-place edit discipline) + the NEW `internal/filter/http/oauth2/` package (10 production + 6 test files). The NEW oauth2 package follows the phase-17/18.1/18.2/19.1 multi-file split: `oauth2.go` (filter type + factory + filterStats + compile-time assertions) + `compiled_config.go` (`compiledConfig` + `buildCompiledConfig` + PARSE-REJECT path with byte-stable error messages per planner-time D2) + `decode_headers.go` (dispatch + handleUnauthenticated + handlePassThrough + handleValidCookies) + `callback.go` (handleCallback + applyTokenEndpointResponse + handleBadState) + `signout.go` (handleSignout) + `oauth_client.go` (postTokenEndpoint + buildTokenRequestBody + `urlEncode` custom helper per ADR-0185 + AMEND-5) + `cookies.go` (parseAllCookies + formatSetCookie) + `hmac.go` (computeHMAC + hmacValidate + dual-encoding read per AMEND-2 + S4) + `tokens.go` (encryptToken + decryptToken + SHA-256 KDF per AMEND-1) + `stats.go` (6-counter filterStats + SN2 compile-time guards per planner-time D6). The dispatcher in `decode_headers.go` classifies inbound requests into 4 dispositions (signout / callback / pass_through / cookie-validate) per SPEC §6.3 priority order. The callback flow parks the decode goroutine on the phase-09 async-resume primitive while the token_endpoint POST is in flight. The 5-cookie envelope (BearerToken / OauthHMAC / OauthExpires / IdToken-deferred / RefreshToken) is the wire shape per ADR-0181; HMAC composition is 5-input newline-joined per AMEND-2 + S4 dual-encoding (emit Base64; accept BOTH Base64 + HexBase64 on read). AES-256-CBC envelope per AMEND-1: `Base64URL(IV ‖ CT)` with SHA-256(hmac_secret)[:32] KDF + random 16-byte IV + PKCS#7 padding; decryption-failure fall-back returns ciphertext-as-plaintext (downstream HMAC validation rejects naturally; no `cookie_decrypt_failure` counter per §20.P11 RATIFIED-AS-ABSENT). The 6-counter stat surface (`oauth_unauthorized_rq` / `oauth_failure` / `oauth_passthrough` / `oauth_success` / `oauth_refreshtoken_success` / `oauth_refreshtoken_failure`) is HCM-rooted SN2-reuse per ADR-0143. Deny-path emits ONLY 302 or 401 — NO 500 anywhere per AMEND-3. The `internal/httpclient/` primitive's 3 introduction-time consumers (jwks Fetcher post-ADR-0150 AMENDMENT + extauthz httpAuthClient post-ADR-0159 AMENDMENT + oauth2 token_endpoint POST NEW) close the ADR-0159 §Future Work forward-pointer load-bearing — the third-consumer trigger fires exactly as ADR-0159 anticipated at phase 18.1. The `internal/sdsfile/` primitive watches the outer Secret-proto JSON/YAML file via fsnotify; consumes `generic_secret.inline_string` ONLY; PARSE-REJECTs non-filesystem `core.ConfigSource` arms + the deprecated `ConfigSource.path` field 1 per §20.P6 RATIFIED + the inner `secret_file` indirect-arm per §8 item 14; ~100ms debounce per §12 item B7. Listener-scoped-only enforcement via `RegisterPerRouteValidator(reg)` HCM-parse-time PARSE-REJECT hook per SPEC §5.2 + planner-time D2 byte-stable wording. THREE NEW race-test groups land per planner-time D4: `TestWatcher_DebounceRace_*` (sdsfile; Task 3) + `TestRefreshTokenRotation_Concurrent_*` (oauth2; Task 8) + `TestAesKeySwap_Concurrent_*` (oauth2; Task 7 cross-cuts Task 3). The phase-20 SPEC anchored 9 NEW ADRs (ADR-0177..ADR-0185 §Context drafts) + 2 IN-PLACE §Decision AMENDMENT-anticipation paragraphs (ADR-0150 + ADR-0159) at the SPEC commit `4df55be`; the IMPL lands the §Decision + §Consequences bodies + the 2 AMENDMENT bodies + the ADR-0159 §Future Work CLOSURE paragraph at their respective Tasks per ADR-0044. ADR-0044 escape-valve held in reserve for ~0-2 IMPL-time-unanticipated ADRs; PLAN's strong hypothesis per planner-time D11: NO additional ADR fires at phase-20 IMPL (next-free ADR-0186 stays unconsumed at phase-20 phase-done).

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/oauth2/v3` for the filter config + `envoy/extensions/transport_sockets/tls/v3` for `SdsSecretConfig` + `envoy/extensions/transport_sockets/tls/v3.Secret` + `envoy/config/core/v3` for `ConfigSource` + `envoy/extensions/transport_sockets/tls/v3.GenericSecret` for the SDS Secret-proto JSON shape); NEW go.mod direct dep `github.com/fsnotify/fsnotify` v1.7.0+ (latest minor on the v1 line at PLAN-time; the IMPL picks the precise tag at Task 3); stdlib `crypto/aes` + `crypto/cipher` + `crypto/sha256` + `crypto/hmac` + `crypto/rand` + `encoding/base64` + `encoding/hex` + `net/http` + `net/url` + `sync/atomic` (`atomic.Pointer[T]`) + `context` + `time` + `os` + `path/filepath`; stdlib `net/http/httptest` for `test/helpers/oauthbackend/`; reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext upstream token_endpoint cluster fixture (NO TLS-to-token_endpoint fixture coverage — mirrors phase-17 / phase-18.1 fixtures 0019 / 0020 disposition; behavioral verification of TLS for token_endpoint lives in `internal/httpclient/*_test.go` against a TLS-fronted test HTTP server if the IMPL adds such coverage at Task 2, otherwise mocked).

---

## Scope check — why phase 20 ships as one row (single-row settled at SPEC per ADR-0045)

The phase-20 SPEC author settled the split disposition per SPEC §3.8 + ROADMAP row 20 + ADR-0045: **SINGLE-ROW landing** (no sub-rows `20.1`/`20.2`; the BRAINSTORM-anticipated MEDIUM-HIGH lean per Q6 was REVISED at SPEC time per the S1/S2/S5 simplifications). The LoC envelope re-estimated post-empirical-scrape at ~3400-3900 production (slightly tighter than BRAINSTORM's ~3500-4000) — below the ADR-0045 split threshold per the phase-17 jwt_authn at 3855 single-row precedent. The 14-task envelope below (mirrors phase-19.1's 15-task PLAN within one task per the slightly smaller cross-package surface) is comfortably under the ADR-0045 25-task split-gate. **Phase 20 ships as the single row it is** — no further split. The phase-20 phase-done squash-merge **CLOSES row 20** (in-progress → done) at the same commit; there is no parent-row rollup discipline (unlike phase 19's 19.1+19.2 pair).

Net change estimate for phase 20 (mirroring the phase-09..19.2 PLAN component-table convention):

- `internal/filter/http/oauth2/oauth2.go` ~150-220 (filter type + factory + filterStats wiring + compile-time interface assertions; `New` body wires from Tasks 2-10 at Task 5/11 integration)
- `internal/filter/http/oauth2/compiled_config.go` ~280-380 (`compiledConfig` struct shape per SPEC §6.2 + `buildCompiledConfig` body + PARSE-REJECT path with byte-stable error messages per D2; populates `tokenEndpoint` / `authorizationEndpoint` / `redirectURI` / matchers / credentials / SDS Watchers / behavioral knobs / `aesKey atomic.Pointer[[32]byte]` / `httpClient *httpclient.Client` / `stats *filterStats`)
- `internal/filter/http/oauth2/decode_headers.go` ~280-380 (`DecodeHeaders` dispatch per SPEC §6.3 + handleUnauthenticated + handlePassThrough + handleValidCookies + handleRefresh dispatch leg + handleRefreshFailure)
- `internal/filter/http/oauth2/callback.go` ~280-380 (handleCallback + applyTokenEndpointResponse + handleBadState + 4-emission-category dispatch per SPEC §4.2; async-resume continuation per SPEC §6.8)
- `internal/filter/http/oauth2/signout.go` ~80-120 (handleSignout per SPEC §6.9 + Max-Age=0 envelope clearing + `deny_redirect_matcher` integration per ADR-0184)
- `internal/filter/http/oauth2/oauth_client.go` ~220-300 (postTokenEndpoint via `httpClient.Do` per ADR-0177 + buildTokenRequestBody 4-field auth-code + 3-field refresh-token templates per AMEND-5 + `urlEncode` custom helper per §6.7 + §12 item A5)
- `internal/filter/http/oauth2/cookies.go` ~180-250 (parseAllCookies + formatSetCookie + Set-Cookie attribute discipline per §4.5 + §12 item A2)
- `internal/filter/http/oauth2/hmac.go` ~120-180 (computeHMAC + hmacValidate + dual-encoding read per AMEND-2 + S4)
- `internal/filter/http/oauth2/tokens.go` ~150-200 (encryptToken + decryptToken + SHA-256(hmac_secret)[:32] KDF + PKCS#7 padding + Base64URL(IV ‖ CT) envelope per AMEND-1 + ADR-0182 + AMEND-3 decryption-failure fall-back)
- `internal/filter/http/oauth2/stats.go` ~120-180 (6-counter filterStats per ADR-0181 + AMEND-4 + S5 + SN2 compile-time guards per D6 + `newFilterStats` constructor)
- `internal/filter/http/oauth2/oauth2_test.go` ~350-500 (Group 1 PARSE-REJECT tests per §14.1; Group 8 dispatcher dispatch tests per §14.1; Group 9 compile-time invariant tests per §14.1)
- `internal/filter/http/oauth2/cookies_test.go` ~250-350 (Group 4 cookie envelope round-trip tests per §14.1 — includes state-cookie payload shape + OauthExpires format tests per §12 item A3)
- `internal/filter/http/oauth2/hmac_test.go` ~250-350 (Group 2 HMAC composition vector tests per §14.1 + dual-encoding read tests per AMEND-2 + S4)
- `internal/filter/http/oauth2/tokens_test.go` ~300-400 (Group 3 AES-256-CBC encrypt/decrypt vector tests per §14.1 + decryption-failure fall-back semantics tests per AMEND-3 + §12 item B6 + `TestAesKeySwap_Concurrent_*` race group per D4)
- `internal/filter/http/oauth2/oauth_client_test.go` ~250-350 (Group 5 token_endpoint POST body template tests per §14.1 + `urlEncode` vector tests per §12 item A5 + `TestRefreshTokenRotation_Concurrent_*` race group per D4)
- `internal/filter/http/oauth2/fuzz_test.go` ~100 (26th fuzzer `FuzzOAuth2ConfigParse` per §7.4 — corpus seeds per planner-time D7)
- `internal/httpclient/httpclient.go` ~150-250 NEW (Options + RetryPolicy + Client + Do per ADR-0177 § Context)
- `internal/httpclient/httpclient_test.go` ~150-250 NEW (Group 6 httpclient unit tests per §14.1 — Options zero-value + Client.Do + zero-retry default + retry envelope + ctx cancellation)
- `internal/sdsfile/sdsfile.go` ~160-200 NEW (Watcher + New + Start + Current + Close + fsnotify integration + atomic-swap discipline per ADR-0178)
- `internal/sdsfile/sdsfile_test.go` ~250-350 NEW (Group 7 sdsfile unit tests per §14.1 + `TestWatcher_DebounceRace_*` race group per D4 + debounce-edge-case coverage per §12 item B7)
- `internal/jwks/fetcher.go` ~+40/-30 IN-PLACE refactor (ADR-0150 AMENDMENT — `New` constructor signature gains `*httpclient.Client` parameter; `doFetch` inner loop delegates to `Client.Do` per phase-20 SPEC §3.4)
- `internal/jwks/fetcher_test.go` ~+30 (existing tests adapted to construct via `*httpclient.Client`; NO new test surface beyond keeping the existing coverage GREEN)
- `internal/filter/http/extauthz/check.go` ~+50/-40 IN-PLACE refactor (ADR-0159 AMENDMENT — `httpAuthClient` constructor signature gains `*httpclient.Client` parameter; the `checkFn` closure's `hac.client.Do(outReq)` delegates to the new primitive per phase-20 SPEC §3.5)
- `internal/filter/http/extauthz/check_test.go` ~+30 (existing tests adapted)
- `cmd/envoy-go/main.go` ~+1 LoC + +1 import (`httpReg.Register(oauth2.TypeURL, oauth2.New)` inserted alphabetical between `localratelimit` (line 134) and `rbac` (shifts to line 136) per ADR-0100 §2.2)
- `test/helpers/oauthbackend/doc.go` ~25 NEW
- `test/helpers/oauthbackend/oauthbackend.go` ~250-350 NEW (in-process OAuth 2.0 authorization-server mock + Scripted{Authz,Token}Responses + ValidCookieEnvelope + TamperedStateCookie helpers; stdlib `net/http/httptest`-based per SPEC §7.3)
- `test/helpers/oauthbackend/oauthbackend_test.go` ~120-180 NEW (Server lifecycle + scripted sequence + stop + concurrent client)
- `test/differential/fixture/fixture.go` ~+15 (NEW `BackendKind` enum value `HTTPOAuth2 BackendKind = 20` after `HTTPExtProcGRPC = 19`)
- `test/differential/runner_test.go` ~+12 (blank import + switch-case for `HTTPOAuth2`)
- `test/fixtures/0024-http-oauth2/` (NEW DIRECTORY) — `envoy.yaml` ~250-350 + `envoy-go.yaml` ~250-350 + `expectations.yaml` ~120 + `README.md` ~180 + `inputs/driver.go` ~500-700 + `secrets/hmac.json` ~30 + `secrets/client_secret.json` ~30 ≈ ~1360-1760 LoC (per planner-time D3 SDS path settle)
- `go.mod` / `go.sum` +1 direct dep `github.com/fsnotify/fsnotify` (Task 3)
- `docs/envoy-go/DECISIONS.md` — 9 NEW ADR §Decision + §Consequences bodies (ADR-0177..ADR-0185) + 2 IN-PLACE §Decision AMENDMENT bodies (ADR-0150 + ADR-0159) + ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph; ~+700-900 LoC. NO new ADR numbers consumed at IMPL under D11 hypothesis (next-free stays ADR-0186)
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+400-500 (§13 10-edit bundle per SPEC §13 — NEW `### envoy.filters.http.oauth2` subsection ~250-350 LoC + 2 NEW framework-primitive umbrella subsections + stat-table 86→92 extension + ADR-0125 cross-reference paragraph + ADR-0159 CLOSURE paragraph + 2 envoy-go-strict departure records + NEW `### Phase 20 forward-pointer notes` subsection + ADR-0150 REFACTORED-AT-PHASE-20 paragraph)
- `docs/envoy-go/ROADMAP.md` row 20 flips `in-progress → done` at phase-done; per-cell IMPL-done annotation added; ~+1 net
- `docs/envoy-go/STATE.md` rewrite-in-place
- `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (NEW) ~700-900 across 14 task entries
- `docs/envoy-go/phases/20-http-filter-oauth2/REVIEW.md` (NEW) ~300

**Production code: ~1990-2790 LoC** (`internal/filter/http/oauth2/` ~1830-2510 + `internal/httpclient/` ~150-250 + `internal/sdsfile/` ~160-200 + in-place refactors `internal/jwks/` ~10 net + `internal/filter/http/extauthz/` ~10 net) **+ ~250-350 LoC test-helper + ~1990-2790 LoC tests = ~4230-5930 LoC production+test** + ~1360-1760 LoC fixture + ~1100-1400 LoC docs ≈ **~6690-9090 LoC total**. **Task count below is 14** — comfortably under the ADR-0045 25-task split-gate (mirrors phase-19.1's 15-task PLAN one shy). Single-row landing settled.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/httpclient/httpclient.go` | NEW | `Options{Timeout, RetryPolicy, TLSConfig}` struct (zero-value Options is a no-op per SPEC §3.1); `RetryPolicy{Attempts, PerAttemptDelay, RetryOnStatus}` struct (zero = no retries; matches Envoy v1.37.2 wire default per §20.P1 RATIFIED); `Client` wrapping `*http.Client` + `Options`; `New(opts Options) *Client`; `(c *Client) Do(req *http.Request) (*http.Response, error)` — synchronous; honors `ctx` cancellation via `req.WithContext`; applies retries per `RetryPolicy`. **The retry loop:** when `RetryOnStatus` contains the response status code AND `Attempts > 0`, the client sleeps `PerAttemptDelay` and re-issues; max `Attempts` per call. Cross-phase reuse confirmed by 3 introduction-time consumers (jwks + extauthz + oauth2 — closes ADR-0159 §Future Work forward-pointer). ~150-250 LoC. ADR-0177 §Decision + §Consequences at Task 2. |
| `internal/httpclient/httpclient_test.go` | NEW | Group 6 per SPEC §14.1 — Options zero-value + Client.Do happy-path + zero-retry default + retry envelope (3 status codes; verify attempt count) + ctx cancellation mid-Do + TLSConfig wired through + request-error propagation. ~150-250 LoC. ~10-12 tests. |
| `internal/sdsfile/sdsfile.go` | NEW | `Watcher` struct: `atomic.Pointer[[]byte]` + `*fsnotify.Watcher` + per-watcher mutex for debounce-timer state + `path string` + `done chan struct{}`. `New(path string) (*Watcher, error)` — reads the file once, populates initial Pointer, returns; does NOT auto-start. `Start() error` — spawns goroutine reading `w.fsWatcher.Events`; on `fsnotify.Write \| fsnotify.Create \| fsnotify.Rename` events (the standard atomic-rename-via-mv + in-place-truncate-and-rewrite event sequences) the watcher sets a `time.AfterFunc(100*time.Millisecond, w.reload)` debouncer; redundant events within the window collapse to ONE reload per §12 item B7. `Current() []byte` — `atomic.LoadPointer` returns the latest bytes (no copy); concurrent reads safe. `Close() error` — closes done channel + `w.fsWatcher.Close()` + waits for goroutine via `sync.Once` guard. **Honors `core.ConfigSource.PathConfigSource` (oneof arm field 8)** per §20.P6 RATIFIED — the field-8 wrapper wraps `{path, watched_directory}`. **Consumes `generic_secret.inline_string` ONLY** (the inner `secret_file` arm PARSE-REJECTs at the outer compiledConfig parser per §8 item 14 — the sdsfile primitive itself reads the byte-stream verbatim from the filesystem path; the JSON/YAML interpretation of the bytes is the CONSUMER's responsibility, NOT sdsfile's). ~160-200 LoC. ADR-0178 §Decision + §Consequences at Task 3. |
| `internal/sdsfile/sdsfile_test.go` | NEW | Group 7 per SPEC §14.1 + §12 item B7 — initial-load + atomic-rename-via-mv reload + in-place-write-via-truncate reload + debounce-window collapses multiple writes + `Current()` returns latest bytes + `Close()` idempotency + `TestWatcher_DebounceRace_*` (race group per D4 — concurrent `Current()` reads during reload; 3-5 race tests). ~250-350 LoC. ~12-15 tests. |
| `internal/jwks/fetcher.go` | IN-PLACE REFACTOR | ADR-0150 §Decision AMENDMENT — `New` constructor signature gains a `*httpclient.Client` parameter (REPLACES the internal `&http.Client{...}` instantiation). The `doFetch` inner-HTTP-request loop (§Decision (vi)) delegates to `Client.Do` rather than the previous `client.Get(uri)` shape. Timeout + retry-policy + TLS posture preserved verbatim (ADR-0177's `Options` carries the phase-17-pinned semantics). ~+40 LoC inserted, ~-30 LoC removed (net ~+10). Consumers updated at the same Task: the boot-registration site at `cmd/envoy-go/main.go` already builds a single `*httpclient.Client` and threads it into both jwks + extauthz constructors. Per ADR-0044 in-place edit discipline. |
| `internal/jwks/fetcher_test.go` | IN-PLACE REFACTOR | Existing tests adapted to construct the `*Fetcher` via a `*httpclient.Client` test instance; NO new test surface; existing test coverage stays GREEN. ~+30 LoC delta. |
| `internal/filter/http/extauthz/check.go` | IN-PLACE REFACTOR | ADR-0159 §Decision AMENDMENT — `httpAuthClient` constructor signature gains a `*httpclient.Client` parameter (REPLACES the internal `&http.Client{Timeout: hs.server_uri.timeout}` instantiation). The `checkFn` closure's `hac.client.Do(outReq)` delegates to the new primitive's `Do` method. The per-request cancellable semantics + the zero-retry-default discipline + the `OnDestroy`-drives-cancel path PRESERVED VERBATIM (ADR-0177's `Options{Timeout}` carries the `HttpService.server_uri.timeout`; the per-request `ctx` thread is unchanged). ~+50 LoC inserted, ~-40 LoC removed (net ~+10). §Future Work CLOSURE-AT-PHASE-20 paragraph appended to the existing §Consequences "Deferred `internal/httpclient/` generalization + the oauth2 trigger" paragraph per the SPEC §3.5 anchor — records that **the third-consumer trigger fires exactly as anticipated at phase 18.1** + **PHASE 20 IS THE FIRST §9 FAMILY-ROW TO CLOSE A PRIOR-PHASE LOAD-BEARING FORWARD-POINTER**. Per ADR-0044 in-place edit discipline. |
| `internal/filter/http/extauthz/check_test.go` | IN-PLACE REFACTOR | Existing tests adapted; NO new test surface; existing test coverage stays GREEN. ~+30 LoC delta. |
| `internal/filter/http/oauth2/oauth2.go` | NEW | Main file. **Public surface** per SPEC §6.1: `const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.oauth2.v3.OAuth2"` + `func New(message proto.Message) (api.HTTPFilterFactory, error)` + `func RegisterPerRouteValidator(reg api.PerRouteValidatorRegistry)` (the HCM-parse-time PARSE-REJECT hook per §5.2 + D2 byte-stable error wording). **`filter` struct** (per-stream state): `cc *compiledConfig`, `dcb api.DecoderFilterCallbacks`, `parentCtx context.Context`, `flowID string` (for `addFlowCookieDeletionHeaders` on 401 per §4.3), `resumeCh chan struct{}` (for the async token_endpoint POST per phase-09 async-resume). All `Decode*` + `Encode*` methods routed per SPEC §6.3 (DECODER-ONLY per §6.12 — the filter is `var _ api.StreamFilter = (*filter)(nil)` + `var _ api.StreamDecoderFilter = (*filter)(nil)` compile-time-asserted). `New` builds `*compiledConfig` via `buildCompiledConfig` (in `compiled_config.go`) then returns the factory closure. ~150-220 LoC. ADR-0180 §Decision + §Consequences at Task 5 + Task 11. |
| `internal/filter/http/oauth2/compiled_config.go` | NEW | `compiledConfig` struct per SPEC §6.2 (verbatim — endpoints + matchers + credentials + behavioral knobs + `aesKey atomic.Pointer[[32]byte]` + `httpClient *httpclient.Client` + `stats *filterStats`) + `buildCompiledConfig(cfg *anypb.Any, ctx api.HTTPFilterFactoryCtx) (*compiledConfig, error)` body + PARSE-REJECT path with byte-stable error messages per planner-time D2. PARSE-REJECT cases (compile-time invariants per SPEC §6.2): (1) token_endpoint URL invalid; (2) authorization_endpoint empty; (3) redirect_uri empty; (4) client_id empty; (5) SDS configs unreachable (file does NOT exist; non-filesystem ConfigSource oneof arm; deprecated path field; `secret_file` indirect arm); (6) `disable_token_encryption=false` AND `hmac_secret` empty; (7) PKCE fields set (`use_pkce` + `oauth_nonce` + `code_verifier` + `code_verifier_token_expires_in`) per §2.1; (8) basic_auth set per §2.3; (9) `forward_bearer_token` populated → MVP-CONSUMED per AMEND-6 C3 (~10 LoC honor — NOT a PARSE-REJECT); (10) `csrf_token_expires_in` honors proto-default 600s per §20.P12 RATIFIED. ~280-380 LoC. Constructs the `*httpclient.Client` from the operator config (or the default zero-Options). Constructs the `aesKey` from `SHA-256(hmac_secret)[:32]` and stores via `atomic.Pointer[[32]byte].Store`. Constructs the `*sdsfile.Watcher` for `hmac_secret` + `client_secret` (calls `New(path)` then `Start()`; cleanup via factory teardown). |
| `internal/filter/http/oauth2/decode_headers.go` | NEW | `DecodeHeaders` dispatch per SPEC §6.3 priority order: (1) signout_path match → `handleSignout` (calls into `signout.go` at Task 9); (2) redirectPathMatcher match → `handleCallback` (calls into `callback.go` at Task 5); (3) passThroughMatcher match → `handlePassThrough` (bypass + `stats.oauth_passthrough++`); (4) else `handleCookieValidate` → ContinueDecoding (valid envelope; with optional `Authorization: Bearer <decrypted>` injection per `forward_bearer_token=true`) OR `handleRefresh` (expired BearerToken + valid RefreshToken — async refresh-token POST via `oauth_client.go::postTokenEndpoint`) OR `handleUnauthenticated` (category (a) 302 challenge per §4.1). Includes `handleRefreshFailure` (category (a) 302 challenge + `stats.oauth_refreshtoken_failure++` per §4.6). The POST-callback method PARSE-REJECTs at the callback-dispatch level per §2.14 + ADR-0180. ~280-380 LoC. ADR-0180 §Decision + §Consequences at Task 5. |
| `internal/filter/http/oauth2/callback.go` | NEW | `handleCallback` (parks decode goroutine on resume channel; spawns goroutine to POST token_endpoint per `oauth_client.go`; on resume invokes `applyTokenEndpointResponse`) + `applyTokenEndpointResponse` (parses JSON body on 2xx; emits category (b) 302 post-callback-success with the 5-cookie envelope per §4.5; on 5xx emits category (a) 302 challenge per §4.7 + AMEND-3; on 4xx terminal emits category (d) 401 with constant body per §4.3) + `handleBadState` (category (d) 401 with constant body + `addFlowCookieDeletionHeaders(headers, flow_id_)` per AMEND-3). ~280-380 LoC. ADR-0180 + ADR-0183 §Decision + §Consequences at Task 5 + Task 8. |
| `internal/filter/http/oauth2/signout.go` | NEW | `handleSignout` per SPEC §6.9 — emits category (c) 302 + Max-Age=0 for all 5 cookies per §4.5 + `deny_redirect_matcher` integration for the Location header per ADR-0184. NO separate `signout_completed` counter per AMEND-4 + S5 (the 302 emission IS the sign-out completion event). ~80-120 LoC. ADR-0184 §Decision + §Consequences at Task 9. |
| `internal/filter/http/oauth2/oauth_client.go` | NEW | `postTokenEndpoint(ctx, body []byte) (*http.Response, error)` — invokes `cc.httpClient.Do(req)` per ADR-0177. `buildTokenRequestBody(grantType string, params map[string]string) []byte` per AMEND-5 — switches on `grantType`: `authorization_code` emits the 4-field MVP template `grant_type=authorization_code&code={0}&client_id={1}&client_secret={2}&redirect_uri={3}` byte-exact per §20.P10 + ADR-0185; `refresh_token` emits the 3-field template `grant_type=refresh_token&refresh_token={0}&client_id={1}&client_secret={2}`. PKCE-gated 5th field `&code_verifier={4}` (currently absent — gated per S3 + §2.1; the function reserves the parameter slot but only emits when PKCE lands). `urlEncode(value string) string` — custom percent-encoder per AMEND-5 + §20.P10: percent-encodes `:/=&?` (NOT stdlib `url.PathEscape` per §6.7 — different byte-exact behavior); spaces as `%20`; the IMPL Task 10 closes the non-ASCII bytes edge per §12 item A5. ~220-300 LoC. ADR-0185 §Decision + §Consequences at Task 10. |
| `internal/filter/http/oauth2/cookies.go` | NEW | `parseAllCookies(headers api.RequestHeaderMap) (map[string]string, error)` — extracts the 5-cookie envelope (BearerToken / OauthHMAC / OauthExpires / IdToken / RefreshToken) per `cc.cookieNames`. `formatSetCookie(name, value string, attrs SetCookieAttrs) string` — emits `name=value; Path=/; Secure; HttpOnly; SameSite=Lax` per §4.5 (default attributes; the operator's `cookie_configs` per-cookie attribute customization is deferred per §2.5; the `Partitioned` attribute is deferred per AMEND-7). Set-Cookie attribute discipline RATIFIED-PENDING-IMPL-TIME per §12 item A2 — settles at Task 12 fixture-0024 scenario (a). ~180-250 LoC. ADR-0181 §Decision + §Consequences at Task 6. |
| `internal/filter/http/oauth2/hmac.go` | NEW | `computeHMAC(domain, expires, token, idToken, refreshToken, hmacSecret []byte) string` per AMEND-2 — `base64.RawURLEncoding.EncodeToString(hmacSHA256(hmacSecret, StrJoin({domain, expires, token, idToken, refreshToken}, "\n")))`. `hmacValidate(envelope CookieEnvelope, hmacSecret []byte) bool` per AMEND-2 + S4 — dual-encoding: decodes the `OauthHMAC` cookie value via BOTH `base64.RawURLEncoding.DecodeString` AND `hex.DecodeString(base64.RawURLEncoding.DecodeString(...))` (the HexBase64 nested encoding per S4); compares against the recomputed HMAC via `crypto/hmac.Equal` (constant-time). The `id_token + refresh_token` participate as empty strings when absent (per §20.P4 REFUTED). ~120-180 LoC. ADR-0179 §Decision + §Consequences at Task 4. |
| `internal/filter/http/oauth2/tokens.go` | NEW | `encryptToken(plaintext, hmacSecret []byte) string` per AMEND-1 — derives 32-byte AES-256 key via `SHA-256(hmacSecret)[:32]`; reads 16-byte random IV via `crypto/rand`; pads plaintext per PKCS#7 to AES block size (16); encrypts via `crypto/cipher.NewCBCEncrypter`; returns `base64.RawURLEncoding.EncodeToString(IV ‖ CT)`. `decryptToken(envelope string, hmacSecret []byte) []byte` per AMEND-1 + AMEND-3 — `base64.RawURLEncoding.DecodeString` envelope; on decode-error returns the original `envelope` bytes (NOT an error) per AMEND-3 decryption-failure fall-back; on success extracts IV + CT, derives key, decrypts via `crypto/cipher.NewCBCDecrypter`, strips PKCS#7 padding; on any failure (malformed envelope, bad padding, wrong block size, ciphertext-too-short) returns the original ciphertext bytes (the downstream HMAC validation rejects the cookie naturally; NO `cookie_decrypt_failure` counter per §20.P11 RATIFIED-AS-ABSENT + AMEND-4). `disable_token_encryption=true` skip-path (the consumer checks `cc.disableTokenEncryption` before calling `encryptToken` / `decryptToken` — handled at the cookies.go level). ~150-200 LoC. ADR-0182 §Decision + §Consequences at Task 7. |
| `internal/filter/http/oauth2/stats.go` | NEW | `filterStats` struct: 6 counters per ADR-0181 + AMEND-4 + S5 — `oauthUnauthorizedRq` / `oauthFailure` / `oauthPassthrough` / `oauthSuccess` / `oauthRefreshTokenSuccess` / `oauthRefreshTokenFailure`. `newFilterStats(ctx, statPrefix)` registers each as `http.<HCM_stat_prefix>.oauth2.<name>` per ADR-0143 SN2-reuse. SN2 compile-time guards per planner-time D6: a `stat_names_test.go` (lives at this file or alongside) table-driven test asserts the 6 counter wire-names byte-exact at `go test` time; the build-time guard is the constant-string declaration block (`const statNameOauthUnauthorizedRq = "oauth_unauthorized_rq"` etc.) that the `newFilterStats` constructor passes verbatim. ~120-180 LoC. ADR-0181 §Decision + §Consequences at Task 6. |
| `internal/filter/http/oauth2/oauth2_test.go` | NEW | Group 1 PARSE-REJECT tests per §14.1 — ~30-40 table rows covering: PKCE fields set; basic_auth set; non-filesystem ConfigSource arms; deprecated path field; `secret_file` indirect arm; empty token_endpoint URL; empty authorization_endpoint; empty redirect_uri; empty client_id; SDS file does NOT exist; `disable_token_encryption=false` + empty `hmac_secret`; `RegisterPerRouteValidator` route-level placement; etc. Each PARSE-REJECT row asserts the byte-stable error wording per planner-time D2. Group 8 dispatcher dispatch tests per §14.1 — ~8-10 priority-order rows covering signout/callback/pass_through/cookie-validate per SPEC §6.3; POST-callback method PARSE-REJECT. Group 9 compile-time invariant tests per §14.1 — ~5-7 assertions (`var _ api.StreamFilter`; `var _ api.StreamDecoderFilter`; TypeURL constant assertion; stat-name byte-exact assertions). ~350-500 LoC. |
| `internal/filter/http/oauth2/cookies_test.go` | NEW | Group 4 cookie envelope round-trip tests per §14.1 — ~15-20 rows covering: 5-cookie envelope round-trip (parse → format → parse); Set-Cookie attribute discipline per §4.5; per-category emission table per §4.1 (auth-challenge / post-callback / sign-out / 401); state-cookie payload shape per §12 item A3 (epoch-seconds-as-decimal-string for OauthExpires); category (a) cleared-envelope-cookies + state cookie SET to HMAC(state). ~250-350 LoC. |
| `internal/filter/http/oauth2/hmac_test.go` | NEW | Group 2 HMAC composition vector tests per §14.1 + AMEND-2 + S4 — ~15-20 rows covering: 5-input newline-joined composition; id_token + refresh_token empty when absent; dual-encoding read (Base64 + HexBase64 BOTH accepted); constant-time-compare via `crypto/hmac.Equal`; reference-vector emission match. ~250-350 LoC. |
| `internal/filter/http/oauth2/tokens_test.go` | NEW | Group 3 AES-256-CBC encrypt/decrypt vector tests per §14.1 + AMEND-1 + AMEND-3 — ~20-25 rows covering: round-trip (encrypt → decrypt → byte-exact); SHA-256(hmac_secret)[:32] KDF; random 16-byte IV (different per encryption); PKCS#7 padding boundary cases (empty plaintext + 16-byte plaintext + non-block-multiple plaintext); Base64URL envelope shape; decryption-failure fall-back (malformed envelope returns ciphertext-as-plaintext per AMEND-3 + §12 item B6 — exactly the assertion that no counter increments and no error surfaces); `disable_token_encryption=true` skip-path correctness. PLUS `TestAesKeySwap_Concurrent_*` race group per planner-time D4 — covers `atomic.Pointer[[32]byte]` swap during in-flight encrypt/decrypt (~2-3 tests; `-race` clean). ~300-400 LoC. |
| `internal/filter/http/oauth2/oauth_client_test.go` | NEW | Group 5 token_endpoint POST body template tests per §14.1 + AMEND-5 — ~15-20 rows covering: 4-field auth-code template byte-exact (matches upstream `oauth_client.cc` source); 3-field refresh-token template byte-exact; PKCE-gated 5th field absent (assertion that the 4-field template does NOT contain `code_verifier`); `urlEncode` vector tests per §12 item A5 covering `:/=&?` percent-encoded + spaces as `%20` + non-ASCII bytes; stdlib `url.PathEscape` vs the custom helper divergence assertion. PLUS `TestRefreshTokenRotation_Concurrent_*` race group per planner-time D4 — concurrent-request race-vs-rotation per ADR-0183 (no per-stream serialization; latest Set-Cookie wins; counter increment one-per-event; ~3-4 tests; `-race` clean). ~250-350 LoC. |
| `internal/filter/http/oauth2/fuzz_test.go` | NEW | 26th fuzzer `FuzzOAuth2ConfigParse` per SPEC §7.4 + planner-time D7 corpus seeds — must-never-panic across `buildCompiledConfig` + `decodeHeaders` parse + cookie-parse + hmac-validate + decrypt-token + buildTokenRequestBody. Clean at 30s per seed. ~100 LoC. |
| `cmd/envoy-go/main.go` | MODIFY | +1 LoC + +1 import (`oauth2 "github.com/esalaine/envoy-go/internal/filter/http/oauth2"`); `httpReg.Register(oauth2.TypeURL, oauth2.New)` inserted alphabetical between `localratelimit` (line 134) and `rbac` (which shifts to line 136) per ADR-0100 §2.2. PLUS construction of the SINGLE `*httpclient.Client` instance threaded into both `jwks.New(...)` and `extauthz.NewHTTPAuthClient(...)` boot sites per the ADR-0150 + ADR-0159 AMENDMENT — `httpClient := httpclient.New(httpclient.Options{Timeout: ...})` constructed at startup; passed to both. |
| `test/helpers/oauthbackend/doc.go` | NEW | Package doc enumerating: the in-process OAuth 2.0 authorization-server mock; consumed by fixture 0024; stdlib `net/http/httptest`-based per SPEC §7.3; per-scenario scripted authz + token responses. ~25 LoC. |
| `test/helpers/oauthbackend/oauthbackend.go` | NEW | Public surface per SPEC §7.3: `New(t testing.TB) *Server` — spawns httptest.Server on `127.0.0.1:0`; `(*Server).Addr() string`; `(*Server).Script(method, path string, response *http.Response)` — register per-scenario scripted response; `(*Server).Stop()` — closes the test server. Also `ValidCookieEnvelope(t, secret)` helper (produces a valid 5-cookie envelope for assertion fixtures) + `TamperedStateCookie(t, secret)` helper (produces a tampered state cookie for scenarios b2 + f). ~250-350 LoC. |
| `test/helpers/oauthbackend/oauthbackend_test.go` | NEW | Server lifecycle + scripted sequence + stop + concurrent client + scripted-authz-response shape + scripted-token-response shape. ~120-180 LoC. |
| `test/differential/fixture/fixture.go` | MODIFY | +1 enum value `HTTPOAuth2 BackendKind = 20` (after `HTTPExtProcGRPC = 19`). |
| `test/differential/runner_test.go` | MODIFY | +blank import + switch-case for `HTTPOAuth2`. |
| `test/fixtures/0024-http-oauth2/envoy.yaml` | NEW | Reference Envoy config — 3-listener topology per planner-time D10 (l_test_a default-encryption / l_test_b `disable_token_encryption=true` / l_test_c `forward_bearer_token=true`). ~250-350 LoC. |
| `test/fixtures/0024-http-oauth2/envoy-go.yaml` | NEW | envoy-go-side config — mirrors the upstream config; SDS Secret files at `secrets/hmac.json` + `secrets/client_secret.json`. ~250-350 LoC. |
| `test/fixtures/0024-http-oauth2/expectations.yaml` | NEW | 9 wire-level expectations across 8 scenario directories per SPEC §7.1 (a + b1 + b2 + c + d + e + f + g + h + i). ~120 LoC. |
| `test/fixtures/0024-http-oauth2/README.md` | NEW | Scenario matrix narrative; cross-references to SPEC §7. ~180 LoC. |
| `test/fixtures/0024-http-oauth2/inputs/driver.go` | NEW | Test-driver invoking the 9-scenario matrix against both reference Envoy + envoy-go; asserts byte-exact wire shape for each scenario. Consumes `test/helpers/oauthbackend/` for the mock OAuth server. ~500-700 LoC. |
| `test/fixtures/0024-http-oauth2/secrets/hmac.json` | NEW | Secret-proto JSON with `generic_secret.inline_string` populated (per planner-time D3). ~30 LoC. |
| `test/fixtures/0024-http-oauth2/secrets/client_secret.json` | NEW | Secret-proto JSON with `generic_secret.inline_string` populated. ~30 LoC. |
| `go.mod` / `go.sum` | MODIFY | +1 direct dep `github.com/fsnotify/fsnotify` (Task 3). |
| `docs/envoy-go/DECISIONS.md` | MODIFY | 9 ADR §Decision + §Consequences bodies anchored at IMPL Tasks (ADR-0177 at Task 2 + ADR-0178 at Task 3 + ADR-0179 at Task 4 + ADR-0180 at Task 5 + ADR-0181 at Task 6 + ADR-0182 at Task 7 + ADR-0183 at Task 8 + ADR-0184 at Task 9 + ADR-0185 at Task 10). 2 IN-PLACE §Decision AMENDMENT bodies (ADR-0150 + ADR-0159) at Task 2. ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph at Task 2. Cross-references intact per SPEC §15 item 15. NO new ADR numbers consumed at IMPL under D11 hypothesis. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY | §13 10-edit bundle (NEW `### envoy.filters.http.oauth2` subsection + 2 NEW framework-primitive umbrella subsections + stat-table 86→92 extension + ADR-0125 cross-reference paragraph + ADR-0159 CLOSURE paragraph + 2 envoy-go-strict departure records + NEW `### Phase 20 forward-pointer notes` subsection + ADR-0150 REFACTORED-AT-PHASE-20 paragraph). Lands at Task 13 (atomic landing per ADR-0052). |
| `docs/envoy-go/ROADMAP.md` | MODIFY | row 20 per-cell IMPL-done annotation + status flips `in-progress → done` at Task 14. |
| `docs/envoy-go/STATE.md` | MODIFY | rewrite-in-place at Task 14 (advance to post-phase-20 state per BOOTSTRAP §4.1 invariant 1). |
| `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` | NEW | append-only task log; ~700-900 LoC across 14 task entries. |
| `docs/envoy-go/phases/20-http-filter-oauth2/REVIEW.md` | NEW | Task 14 reviewer artifact per `superpowers:requesting-code-review`. ~300 LoC. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + PLAN-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's eight deferred decisions before implementation; this PLAN settles those eight (delegated to the IMPL Task that closes each per SPEC §12 column) plus eleven that emerged at PLAN-drafting time. The resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced here so the implementer at each task can act without re-deriving them:

1. **D1 — Task 2 sub-grouping LOCKED at SINGLE PLAN-TASK with 3 IMPL-internal commits (NEW; surfaces at PLAN-time).** Settle: Task 2 carries the NEW `internal/httpclient/` primitive (ADR-0177) + the ADR-0150 jwks Fetcher refactor + the ADR-0159 extauthz `httpAuthClient` refactor as ONE PLAN task with 3 IMPL-internal commit boundaries: **Task 2a** — NEW `internal/httpclient/` package + tests + ADR-0177 §Decision + §Consequences body + go.mod check + boot-registration site at `cmd/envoy-go/main.go` constructs the SHARED `*httpclient.Client` instance; **Task 2b** — ADR-0150 jwks Fetcher refactor + AMENDMENT body + jwks unit tests adapted + fixture-0019 GREEN regression check; **Task 2c** — ADR-0159 extauthz httpAuthClient refactor + AMENDMENT body + §Future Work CLOSURE-AT-PHASE-20 paragraph + extauthz unit tests adapted + fixture-0020 GREEN regression check. Rationale: the cross-package nature is the only structural complexity; keeping one PLAN task with 3 internal commits preserves the conceptual atomicity (the 3 sub-changes are inseparable per ADR-0177's introduction-time-3-consumer framing) while giving the reviewer 3 distinct commits to evaluate. The 14-task PLAN envelope stays under the 25-task ADR-0045 split-gate. *Anchored: PLAN-time emerge + phase-19.1 Task 4+5 multi-commit precedent.*

2. **D2 — PARSE-REJECT byte-stable error message exact strings LOCKED per SPEC §12 + ADR-0080 discipline (NEW; surfaces at PLAN-time).** Settle: all oauth2 PARSE-REJECT messages use the prefix `"oauth2:"` followed by a colon-delimited subject and reason. Reference strings (the implementer's authoritative list at IMPL Task 2 + Task 5 + Task 11):
   - `"oauth2: typed_per_filter_config not supported at route or virtualHost level; oauth2 is listener-scoped only"` (HCM-parse-time PARSE-REJECT per §5.2 + AMEND-7; emitted at `RegisterPerRouteValidator`).
   - `"oauth2: ApiConfigSource ConfigSource arm not supported; only filesystem PathConfigSource is supported"` (SDS PARSE-REJECT per §2.11 + §20.P6).
   - `"oauth2: Ads ConfigSource arm not supported; only filesystem PathConfigSource is supported"` (SDS PARSE-REJECT per §2.11).
   - `"oauth2: deprecated ConfigSource.path field 1 not supported; use PathConfigSource (oneof arm field 8)"` (SDS PARSE-REJECT per §2.11 + §20.P6 RATIFIED).
   - `"oauth2: generic_secret.secret_file arm not supported; only inline_string is supported"` (SDS PARSE-REJECT per §2.11 + §8 item 14).
   - `"oauth2: OAuth2Credentials.basic_auth not supported; use client_secret_post (token_endpoint POST body)"` (BASIC_AUTH PARSE-REJECT per §2.3 + AMEND-5).
   - `"oauth2: use_pkce + PKCE-related fields not supported in MVP"` (PKCE PARSE-REJECT per §2.1; covers `use_pkce` + `oauth_nonce` + `code_verifier` + `code_verifier_token_expires_in`).
   - `"oauth2: POST callback method not supported; GET-only (envoy-go-strict departure)"` (callback dispatch PARSE-REJECT per §2.14 + §20.P3).
   - `"oauth2: disable_token_encryption=false requires non-empty hmac_secret"` (parse-time invariant per §6.2).
   - `"oauth2: token_endpoint URL invalid: %s"` (compile-time invariant per §6.2 with stdlib `url.Parse` error tail).
   - `"oauth2: authorization_endpoint empty"` (compile-time invariant per §6.2).
   - `"oauth2: redirect_uri empty"` (compile-time invariant per §6.2).
   - `"oauth2: client_id empty"` (compile-time invariant per §6.2).
   Pattern mirrors ext_authz / ext_proc PARSE-REJECT prefixes; operator-grep-friendly `oauth2:` prefix; each message terminated WITHOUT a trailing period. The unit-test Group 1 (Task 2 + Task 11) asserts each byte-exact via `errors.Is` + `err.Error() == expected`. *Anchored: SPEC §12 + ADR-0080 + PLAN-time emerge.*

3. **D3 — SDS Secret file paths in fixture-0024 LOCKED per SPEC §7.2 hypothesis (NEW; surfaces at PLAN-time confirming).** Settle: SDS Secret files live at `test/fixtures/0024-http-oauth2/secrets/hmac.json` + `test/fixtures/0024-http-oauth2/secrets/client_secret.json` per BRAINSTORM §6 hypothesis + SPEC §7.2 reproduction. Each file is a Secret-proto JSON with `generic_secret.inline_string` populated (the inner `secret_file` indirect-arm PARSE-REJECTs per §8 item 14). The reload-during-fixture-run scenario is NOT in the 9-scenario matrix at MVP (the in-process sdsfile race tests at Task 3 + Task 7 cover the reload-during-encrypt-decrypt + reload-during-HMAC-validate race surfaces); a future fixture-extension scenario MAY add a reload-mid-stream assertion if a behavioral delta surfaces. *Anchored: SPEC §7.2 + BRAINSTORM §6 + PLAN-time emerge.*

4. **D4 — Race-test surface roster LOCKED per SPEC §14.2 (NEW; surfaces at PLAN-time).** Settle: THREE race-test groups under `go test -race ./...`:
   - **`TestWatcher_DebounceRace_*`** (sdsfile; lives at `internal/sdsfile/sdsfile_test.go`; lands at Task 3) — 3-5 tests covering: concurrent `Current()` reads during reload; back-to-back rapid writes (~100ms window) collapse to one reload + final-bytes wins; `Close()` during an in-flight reload terminates cleanly without panic.
   - **`TestRefreshTokenRotation_Concurrent_*`** (oauth2; lives at `internal/filter/http/oauth2/oauth_client_test.go`; lands at Task 8) — 3-4 tests covering: concurrent in-flight requests with same expired BearerToken + valid RefreshToken each POST refresh independently (envoy-go-strict no per-stream serialization per ADR-0183); latest Set-Cookie wins via deferred Set-Cookie discipline; counter increment one-per-event (`oauth_refreshtoken_success` += 1 per successful rotation; `oauth_refreshtoken_failure` += 1 per failed; refresh-failure increments `oauth_refreshtoken_failure` + `oauth_unauthorized_rq` if downstream-deny — NOT `oauth_failure` per §4.6).
   - **`TestAesKeySwap_Concurrent_*`** (oauth2; lives at `internal/filter/http/oauth2/tokens_test.go`; lands at Task 7 cross-cuts Task 3) — 2-3 tests covering: `atomic.Pointer[[32]byte]` swap during in-flight encrypt + during in-flight decrypt (via the sdsfile-triggered reload path; the new `aesKey` derived from new `hmac_secret` bytes via SHA-256); concurrent reads observe consistent key bytes via atomic.LoadPointer; the swap discipline guarantees no partial-bytes read.
   Cumulative race-test surface: 8-12 tests across 3 groups; ALL clean under `-race` at Gate C per §14.5. *Anchored: SPEC §14.2 + ADR-0178 + ADR-0182 + ADR-0183 + PLAN-time emerge.*

5. **D5 — Cross-package regression-test command shape LOCKED at single test pattern + Makefile reuse (NEW; surfaces at PLAN-time).** Settle: after Task 2b (jwks refactor) the implementer runs `go test -count=1 ./test/differential/ -run 'Test.*0019'` to verify fixture-0019 (jwt_authn) stays GREEN post-refactor. After Task 2c (extauthz refactor) the implementer runs `go test -count=1 ./test/differential/ -run 'Test.*0020'` (fixture-0020 ext_authz HTTP-mode) AND `go test -count=1 ./test/differential/ -run 'Test.*0021'` (fixture-0021 ext_authz gRPC-mode, untouched but verifies no incidental breakage). At Task 14 Gate D the full regression `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-4])'` runs all 25 fixtures (the 24 pre-existing + the new 0024). Per SPEC §12 item C8 expected outcome: zero regression. *Anchored: SPEC §12 item C8 + SPEC §7.5 + PLAN-time emerge.*

6. **D6 — Stat-name compile-time guard pattern LOCKED at constant-declaration + table-driven assertion (NEW; surfaces at PLAN-time confirming).** Settle: stat names declared as package-level `const` declarations in `stats.go` (one constant per counter; `const statNameOauthUnauthorizedRq = "oauth_unauthorized_rq"` etc.); `newFilterStats(ctx, statPrefix)` reads the constants directly when registering each counter (no string-literal duplication). A table-driven `TestStatNames_Equal_*` test in `oauth2_test.go` asserts the 6 constants byte-exact against the wire-expected names (mirrors phase-17 jwt_authn + phase-18.x ext_authz + phase-19.x ext_proc precedent). The "compile-time" guard is the constant declaration itself: any drift between the constant and a string literal at the registration site fails the build via the constant-pointer convention. *Anchored: SPEC §6.12 + ADR-0143 SN2-reuse + phase-17/18.2/19.x precedent + PLAN-time emerge.*

7. **D7 — Fuzzer corpus seed roster for `FuzzOAuth2ConfigParse` LOCKED per SPEC §7.4 + §14.3 (NEW; surfaces at PLAN-time).** Settle: corpus seeds at `internal/filter/http/oauth2/testdata/fuzz/FuzzOAuth2ConfigParse/` covering:
   - Each consumed OAuth2Config field × valid/invalid variants (~17 consumed fields × 2 ≈ 34 seeds): `token_endpoint`, `authorization_endpoint`, `redirect_uri`, `redirect_path_matcher`, `signout_path`, `forward_bearer_token`, `preserve_authorization_header`, `disable_token_encryption`, `use_refresh_token`, `default_expires_in`, `default_refresh_token_expires_in`, `auth_scopes`, `resources`, `csrf_token_expires_in`, `pass_through_matcher`, `deny_redirect_matcher`, `credentials`.
   - Each OAuth2Credentials field × valid/invalid (4 × 2 = 8 seeds): `client_id`, `token_secret` (the SDS), `hmac_secret` (the SDS), `cookie_names` — INCLUDING `basic_auth` PARSE-REJECT triggers.
   - Each CookieNames field × valid/invalid (5 consumed × 2 = 10 seeds): bearer_token / oauth_hmac / oauth_expires / id_token / refresh_token — PLUS `oauth_nonce` + `code_verifier` PARSE-REJECT triggers per §2.1.
   - SdsSecretConfig path variants (valid filesystem path + 4 PARSE-REJECT variants — ApiConfigSource / Ads / deprecated path field / `secret_file` indirect arm = ~5 seeds).
   - Matcher engine variants (header matcher + path matcher boundary cases — ~5 seeds).
   Total corpus floor: ~62 seeds. Must-never-panic across `buildCompiledConfig` + `decodeHeaders` parse + cookie-parse + hmac-validate + decrypt-token + buildTokenRequestBody. Clean at 30s per seed. *Anchored: SPEC §7.4 + §14.3 + PLAN-time emerge.*

8. **D8 — `httpclient.Client` instance ownership LOCKED at single-process-wide instance constructed at boot-registration (NEW; surfaces at PLAN-time).** Settle: ONE `*httpclient.Client` instance is constructed at `cmd/envoy-go/main.go` (Task 2a) via `httpclient.New(httpclient.Options{...})` with sensible defaults (Timeout 30s; RetryPolicy zero); the instance is threaded into both `jwks.New(...)` (per Task 2b) and `extauthz.NewHTTPAuthClient(...)` (per Task 2c); the oauth2 filter's `compiledConfig.httpClient` field captures the SAME instance via the FactoryCtx. Rationale: matches the underlying `*http.Client`'s designed-for-reuse semantics + minimizes connection-pool fragmentation across consumers. Cross-phase reuse cost ≈ 0 (the Options struct is per-call-context-aware if needed; per-consumer Options can be threaded via a future Wrap helper without breaking the singleton). NO new ADR fires — this is an IMPL-level integration choice. *Anchored: PLAN-time emerge + ADR-0177 §Consequences hint.*

9. **D9 — Task graph parallelization LOCKED per planner-time emerge (NEW).** Settle: Tasks 3 + 4 + 6 + 7 can run in PARALLEL after Task 2 lands (independent surfaces; all depend on Task 2 + Task 1 for the package being established but NOT on each other) — `internal/sdsfile/` (Task 3) + `internal/filter/http/oauth2/hmac.go` (Task 4) + `internal/filter/http/oauth2/cookies.go + stats.go` (Task 6) + `internal/filter/http/oauth2/tokens.go` (Task 7). Tasks 5 (decode_headers + callback dispatch) + 8 (refresh-token rotation) + 9 (signout) + 10 (oauth_client) depend sequentially on Tasks 3 + 4 + 6 + 7. Task 11 (ADR final-state alignment) + Task 12 (fixture 0024) + Task 13 (BEHAVIOR_CONTRACT 10-edit bundle) + Task 14 (6 gates + STATE/ROADMAP advance) are sequential at the tail. **Parallel-dispatch opportunity at Tasks 3+4+6+7** — four agents can run concurrently on disjoint files. **Sequential bottleneck at Tasks 5→8→9→10 + Task 11** — the dispatch + flow handlers + ADR finalization are the critical path. The IMPL session per `superpowers:subagent-driven-development` per project memory `feedback_execution_style.md` exploits the parallel opportunity at Tasks 3+4+6+7. *Anchored: PLAN-time emerge + phase-19.1 task-graph precedent.*

10. **D10 — Fixture 0024 listener topology LOCKED at 3 listeners per SPEC §7.2 (NEW; surfaces at PLAN-time confirming).** Settle: 3 HCM listeners per SPEC §7.2's "2-or-3 listeners; settled by IMPL planner" disposition — `l_test_a` default-encryption (`disable_token_encryption=false`) hosts scenarios a + b1 + b2 + c + d + e + f + g + h; `l_test_b` `disable_token_encryption=true` hosts scenario i; `l_test_c` `forward_bearer_token=true` hosts scenario b1's Authorization-header injection assertion (NOTE: scenario b1 has TWO assertions — the basic cookie passthrough on l_test_a; the Authorization injection-byte-exact on l_test_c). The 3-listener topology mirrors the phase-18.2 fixture 0021 pattern (the listener-scoped-only `forward_bearer_token` per-listener invariant CANNOT be per-route-overridden). *Anchored: SPEC §7.2 + phase-18.2 fixture-0021 precedent + PLAN-time emerge.*

11. **D11 — ADR-0044 escape-valve disposition: PLAN-time HYPOTHESIS that NO additional ADR fires at phase-20 IMPL (NEW; surfaces at PLAN-time).** Per the SPEC-time scrape closure of all 12 §20.P pins (6 RATIFIED + 4 REFUTED + 1 PARTIAL → SPEC-decided + 1 RATIFIED-AS-ABSENT) — the most-likely escape-valve surfaces are REMOVED at SPEC time per BRAINSTORM §11 lesson (h). PLAN's strong hypothesis: NO additional ADR fires at phase-20 IMPL — next-free ADR-0186 stays unconsumed at phase-20 phase-done. The remaining possible IMPL surfaces are: (i) AES-256-CBC PKCS#7 padding-oracle hardening — low-probability per AMEND-3 fall-through semantics (the decryption-failure path returns ciphertext-as-plaintext NOT an error; no oracle surface); if it surfaces, ADR-0182 §Decision is AMENDED in-place at Task 7 per ADR-0044 (NO new ADR); (ii) fsnotify event-debounce edge-cases under multi-rapid-write races — low-probability per ~100ms debounce + Go race-test coverage at Task 3; if a delta surfaces ADR-0178 §Decision is AMENDED in-place; (iii) `urlEncode` charset-edge-case for non-ASCII bytes — low-probability per stdlib UTF-8 escaping at the helper; if a delta surfaces ADR-0185 §Decision is AMENDED in-place at Task 10. If at IMPL time a surface DOES warrant a new ADR (highly unlikely per the SPEC-time scrape closure), it is ADR-0186 + PLAN's D11 hypothesis is recorded as falsified in PROGRESS.md. *Anchored: SPEC §10 C escape-valve note + BRAINSTORM §11 lesson (h) + PLAN-time emerge.*

12. **D12 — fsnotify dependency version LOCKED at latest v1.x minor (NEW; surfaces at PLAN-time).** Settle: Task 3 adds `github.com/fsnotify/fsnotify` as a NEW direct go.mod dep at the latest v1.x minor available at IMPL time (anticipated v1.7.0+ at PLAN-time; the IMPL captures the precise tag at the `go get -u github.com/fsnotify/fsnotify@latest` invocation). The IMPL pins via `go.sum` + the standard module-graph discipline. The choice is the v1.x line because (a) fsnotify v1.x is stable + cross-platform; (b) the v2.x line (if it exists at IMPL time) is opt-in API-changing; (c) no other in-tree dep blocks the version pick. NO new ADR fires (a module-version pin is an IMPL-level choice). *Anchored: PLAN-time emerge.*

13. **D13 — `*sdsfile.Watcher` lifecycle ownership LOCKED at `compiledConfig`-owned + closed at filter teardown (NEW; surfaces at PLAN-time).** Settle: each `*sdsfile.Watcher` instance is OWNED by the `*compiledConfig` (one per SDS config — typically two per filter for `hmac_secret` + `client_secret`); constructed via `sdsfile.New(path)` + `Start()` at `buildCompiledConfig` time (Task 11); closed via `Close()` at compiledConfig teardown (which fires when the filter is unregistered or the process exits). MVP leaks-on-exit discipline (mirrors phase-18.2 D2 + ADR-0158 §Decision (vi)) — no `os.Exit` cleanup hook needed. Concurrent reads from multiple per-stream filter instances are safe via the `atomic.Pointer[[]byte]` discipline. *Anchored: PLAN-time emerge + phase-18.2 D2 precedent.*

14. **D14 — Refresh-token rotation: no per-stream serialization LOCKED per ADR-0183 §Decision (NEW; surfaces at PLAN-time confirming).** Settle: concurrent in-flight requests with the same expired BearerToken + valid RefreshToken each POST the refresh INDEPENDENTLY (no per-stream mutex; no global mutex per (BearerToken, RefreshToken) key). The "race" outcome is benign: each successful POST emits a new BearerToken + RefreshToken envelope via deferred Set-Cookie; the latest Set-Cookie observed by the downstream client wins (standard cookie-overwrite browser semantic). The 6-counter wire-exact roster increments one-per-event: `oauth_refreshtoken_success` += 1 per successful POST; `oauth_refreshtoken_failure` += 1 per failed POST. Race tests at `TestRefreshTokenRotation_Concurrent_*` (per D4) validate the behavior. *Anchored: ADR-0183 §Decision + SPEC §4.6 + SPEC §14.2 + PLAN-time emerge.*

15. **D15 — POST callback method PARSE-REJECT byte-stable wording LOCKED per D2 + §2.14 + §20.P3 (NEW; surfaces at PLAN-time).** Settle: when the callback dispatch (in `decode_headers.go` at Task 5) detects a POST request matching the `redirect_path_matcher`, the dispatcher emits a category (d) 401 with the constant body `"OAuth flow failed."` (per §4.3) + the byte-stable error `"oauth2: POST callback method not supported; GET-only (envoy-go-strict departure)"` LOGGED (not in the response body) — the response body matches the bad-state-401 wire shape per §4.3 + AMEND-3 to keep the 401 wire body single-source-of-truth. The downstream client receives the standard 401 + `"OAuth flow failed."` body; the operator observes the log line for diagnostics. NO new counter (the standard `oauth_unauthorized_rq` increments per §4.6). *Anchored: SPEC §2.14 + §4.3 + §20.P3 + D2 + PLAN-time emerge.*

16. **D16 — Wire-shape byte-confirmation items in SPEC §12 A1-A5 LOCKED at fixture-0024 scenario coverage (NEW; surfaces at PLAN-time).** Settle: each of the 5 wire-shape items from SPEC §12 closes at Task 12 fixture-0024 scenarios as follows: (A1) 401 Content-Type + no-trailing-newline closes at scenario (f) bad_state_401 + scenario (h) token_endpoint_4xx_401; (A2) Set-Cookie attribute byte-exact upstream defaults closes at scenario (a) sign_in_happy_path 5-cookie envelope emission; (A3) state-cookie payload byte-exact shape + OauthExpires format closes at scenario (a) — verify epoch-seconds-as-decimal-string for OauthExpires + the state-cookie payload bytes; (A4) HCM `SendLocalReply` Content-Type default closes via the scenario (f) + (h) Content-Type assertion against the corresponding reference Envoy capture; (A5) `urlEncode` charset closes at scenario (a) — the token_endpoint POST body capture asserts byte-exact match against reference Envoy v1.37.2's emission for the matched request. The IMPL captures both reference Envoy AND envoy-go responses per scenario; differential harness asserts byte-equivalent. *Anchored: SPEC §12 items A1-A5 + PLAN-time emerge.*

17. **D17 — Library-behavioral items in SPEC §12 B6 + B7 LOCKED at unit-test + race-test coverage (NEW; surfaces at PLAN-time).** Settle: (B6) AES-256-CBC PKCS#7 padding decrypt-failure semantics closes at Task 7 unit tests (`tokens_test.go` Group 3 decryption-failure fall-back rows per AMEND-3) + Task 12 fixture-0024 decrypt-failure path coverage via scenario (b2) cookie_passthrough_tampered_envelope; (B7) fsnotify event-debounce window precise behavior closes at Task 3 unit tests (`sdsfile_test.go` debounce-window collapses multiple writes rows) + race-tested via `TestWatcher_DebounceRace_*` at Task 3. Both items report RATIFIED at Task 14 PROGRESS log. *Anchored: SPEC §12 items B6 + B7 + PLAN-time emerge.*

18. **D18 — Cross-phase regression matrix item C8 LOCKED per D5 + Task 14 6-gate (NEW; surfaces at PLAN-time confirming).** Settle: SPEC §12 item C8 (cross-package regression matrix for ADR-0150 + ADR-0159 in-place AMENDMENTs) closes at Task 2b + Task 2c regression checks (per D5) + Task 14 Gate D full 25-fixture regression run. Expected outcome per SPEC §12: zero regression (refactor is pure thin-wrapper-substitution). RATIFIED at Task 14 PROGRESS log. *Anchored: SPEC §12 item C8 + D5 + PLAN-time emerge.*

19. **D19 — Boot-registration position LOCKED at line-135 between localratelimit and rbac per §3.7 (NEW; surfaces at PLAN-time confirming).** Settle: `cmd/envoy-go/main.go` gains the `httpReg.Register(oauth2.TypeURL, oauth2.New)` call at line 135 alphabetically between `localratelimit` (line 134) and `rbac` (which shifts from line 135 to 136). The 15th `httpReg.Register` call after phase 19's 14 calls. Per ADR-0072 + ADR-0100 §2.2 — registration order does not affect runtime behavior; stylistic discipline only. Plus the `*httpclient.Client` singleton construction per D8 — `httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})` constructed at startup before the boot-registration block; threaded into both `jwks.New(httpClient, ...)` (Task 2b refactor) + `extauthz.NewHTTPAuthClient(httpClient, ...)` (Task 2c refactor) + captured by the oauth2 factory via FactoryCtx (Task 11). *Anchored: SPEC §3.7 + ADR-0100 §2.2 + D8 + PLAN-time emerge.*

---

## ADRs introduced/landed by this plan

The phase-20-landing ADRs per SPEC §10 + the 2 IN-PLACE AMENDMENTs — **§Context drafts already at the SPEC commit `4df55be`** (re-anchored at SHA-fill follow-up `9cb4292`) per ADR-0044 ADR-on-impl convention; **§Decision + §Consequences land at each ADR's Lands-in-Task at phase-20 IMPL**. The 2 IN-PLACE §Decision AMENDMENT-anticipation paragraphs at ADR-0150 + ADR-0159 anchor at the SPEC commit; **AMENDMENT bodies + ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph land at IMPL Task 2** per ADR-0044. PLAN's strong hypothesis per D11: **NO conditional impl-time-unanticipated ADR fires at phase-20 IMPL** (next-free ADR-0186 stays unconsumed at phase-20 phase-done).

| ADR | Subject (phase-20 portion) | Lands-in-Task |
|---|---|---|
| **ADR-0177** | NEW `internal/httpclient/` framework primitive (Options + RetryPolicy + Client.Do); **CLOSES ADR-0159 §Future Work forward-pointer load-bearing** (third-consumer trigger fired per Q2 EXTRACT NOW); cross-phase-reusable for any future outbound-HTTP consumer. 3 introduction-time consumers: jwks Fetcher (post-ADR-0150 AMENDMENT) + extauthz httpAuthClient (post-ADR-0159 AMENDMENT) + oauth2 token_endpoint POST (NEW) | Task 2 (2a sub-commit) |
| **ADR-0178** | NEW `internal/sdsfile/` framework primitive (Watcher + fsnotify + atomic-swap); `generic_secret.inline_string` only; PARSE-REJECT non-filesystem `core.ConfigSource` oneof arms + the deprecated `ConfigSource.path` field 1 + the inner `secret_file` indirect-arm; NEW go.mod dep `github.com/fsnotify/fsnotify`; ~100ms debounce; cross-phase-reusable for any future filesystem-SDS consumer | Task 3 |
| **ADR-0179** | oauth2 HMAC cookie composition — 5-input newline-joined `HMAC-SHA256(hmac_secret, StrJoin({domain, expires, token, id_token, refresh_token}, "\n"))` per AMEND-2 + §20.P4 REFUTED; id_token + refresh_token participate as empty strings when absent; dual-encoding read per S4 (emit Base64; accept BOTH Base64 + HexBase64); constant-time-compare via `crypto/hmac.Equal` | Task 4 |
| **ADR-0180** | oauth2 state-machine + deny-path wire shape (302+401 only; NO 500 anywhere per AMEND-3 + §20.P9 REFUTED) + listener-scoped-only enforcement via HCM-parse-time PARSE-REJECT per §5.2 + the explicit NO-ADR-0125-AMENDMENT REUSE-by-absence classification (THIRD CONSECUTIVE §9 row after phase 18 + phase 19 to skip ADR-0125 roster extension; absence-as-lesson stronger form) | Task 5 |
| **ADR-0181** | oauth2 cookie envelope (5-of-7 `CookieNames` consumed at MVP) + 6-counter stat surface (86 → 92 names per AMEND-4 + S5; wire-exact upstream — `oauth_unauthorized_rq` / `oauth_failure` / `oauth_passthrough` / `oauth_success` / `oauth_refreshtoken_success` / `oauth_refreshtoken_failure`; HCM-rooted SN2-reuse per ADR-0143); **CLOSES §20.P11 envoy-go-strict departure flag as RATIFIED-AS-ABSENT** (no `cookie_decrypt_failure` counter); the `Partitioned` cookie attribute deferred per AMEND-7 + §8 item 15 | Task 6 |
| **ADR-0182** | NEW filter-local AES-256-CBC token-encryption helper at `oauth2/tokens.go` per AMEND-1 + §20.P5 REFUTED (algorithm swap from BRAINSTORM Q4-anticipated AES-GCM to upstream-byte-exact AES-256-CBC); SHA-256(hmac_secret)[:32] key derivation; random 16-byte IV per encryption (prepended); PKCS#7 padding; Base64URL(IV ‖ CT) envelope; `disable_token_encryption=true` skip-path (plaintext storage; explicit MVP-CONSUMED per S2 NO-runtime-gate decision); decryption-failure fall-back returns ciphertext-as-plaintext per AMEND-3 (no `cookie_decrypt_failure` counter per §20.P11 RATIFIED-AS-ABSENT) | Task 7 |
| **ADR-0183** | oauth2 refresh-token rotation timing + race-vs-rotation discipline — `default_refresh_token_expires_in` semantics + concurrent-request-with-same-expired-BearerToken-plus-valid-RefreshToken disposition (envoy-go-strict: no per-stream serialization per D14; each in-flight request POSTs the refresh independently; the LATEST `Set-Cookie` envelope wins via the deferred Set-Cookie discipline); counter increment matrix (refresh-failure → 302 challenge, NOT also `oauth_failure` per AMEND-3 + §4.6) | Task 8 |
| **ADR-0184** | oauth2 sign-out flow — `signout_path` handling + full envelope clearing (Max-Age=0 for all 5 cookies) + `deny_redirect_matcher` integration; category (c) 302 emission per §4.1; NO separate `signout_completed` counter per AMEND-4 + S5 (sign-out completion IS the 302 emission; 6-counter wire-exact upstream) | Task 9 |
| **ADR-0185** | oauth2 token_endpoint POST body templates per AMEND-5 + §20.P10 RATIFIED — byte-exact 4-field auth-code template for MVP + 3-field refresh-token template; PKCE-gated 5th field for future per S3; spaces as `%20`; PercentEncoding charset includes `:/=&?`; NEW `urlEncode` custom helper at `oauth2/oauth_client.go` (stdlib `url.PathEscape` does NOT match upstream byte-exact behavior) | Task 10 |

### IN-PLACE §Decision AMENDMENTs (per ADR-0044)

| ADR | AMENDMENT scope | Lands-in-Task |
|---|---|---|
| **ADR-0150** | `internal/jwks/Fetcher` refactor — §Decision body gains AMENDMENT paragraph (consumes `*httpclient.Client`); §Consequences body gains cross-phase-consumer disposition paragraph; ~40-60 LoC delta. AMENDMENT body lands at IMPL Task 2 (sub-commit 2b) paired with ADR-0177 introduction | Task 2 (2b sub-commit) |
| **ADR-0159** | `extauthz/check.go::httpAuthClient` refactor — §Decision body gains AMENDMENT paragraph (consumes `*httpclient.Client`); **§Future Work gains CLOSED-AT-PHASE-20 paragraph** (FIRST §9 family-row to CLOSE prior-phase load-bearing forward-pointer per SPEC §9 item 1); ~50-80 LoC delta. AMENDMENT body + §Future Work CLOSURE paragraph land at IMPL Task 2 (sub-commit 2c) paired with ADR-0177 introduction | Task 2 (2c sub-commit) |

The implementer at each impl-anchor task AUTHORS the ADR §Decision + §Consequences bodies in DECISIONS.md (the §Context drafts are already at the SPEC commit per ADR-0044), includes the ADR in the commit message, and verifies via `grep -nE '^## ADR-0XX' docs/envoy-go/DECISIONS.md` returning the expected single match per ADR.

**NO in-place ADR-0125 amendment required by phase 20** (ADR-0180 records the explicit no-amendment REUSE-by-absence decision — THIRD CONSECUTIVE §9 row after phase 18 + phase 19 to skip; phase 20's REUSE-by-absence is a stronger form — there is no per-route surface at all per §20.P7 RATIFIED, so the listener-scoped-only enforcement is itself a parse-time PARSE-REJECT discipline rather than a roster-REUSE classification).

**ADR-0044 escape-valve held in reserve per D11** — `ADR-0186` is reserved for any phase-20-IMPL-unanticipated surface. If at IMPL time a surface DOES warrant a new ADR (highly unlikely per the SPEC-time scrape closure of all 12 §20.P pins), it is ADR-0186 + the PLAN's D11 hypothesis is recorded as falsified in PROGRESS.md. If ADR-0177..ADR-0185 require IMPL-time §Decision AMENDMENTs (e.g., AES-256-CBC padding-oracle hardening; fsnotify debounce edge-case; urlEncode charset edge-case), the AMENDMENT lands in-place — NO new ADR number consumed.

---

## Task graph (sequential vs parallelizable)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task graph:

- **Task 1** (PROGRESS.md preamble + 17-precondition verification) — sequential prerequisite for everything; sets up the append-only log.
- **Task 2** (NEW `internal/httpclient/` + paired ADR-0150 + ADR-0159 in-place refactors) — sequential after Task 1; 3 IMPL-internal commit boundaries (2a / 2b / 2c per D1). Establishes the framework primitive + closes the ADR-0159 §Future Work forward-pointer.
- **Tasks 3, 4, 6, 7** — **PARALLELIZABLE** (independent surfaces; all depend on Task 2 for `*httpclient.Client` being available + Task 1 PROGRESS log but NOT on each other per D9):
  - **Task 3** — NEW `internal/sdsfile/` package + ADR-0178 + go.mod fsnotify dep + sdsfile unit tests + race tests (`TestWatcher_DebounceRace_*`).
  - **Task 4** — `internal/filter/http/oauth2/hmac.go` + tests + ADR-0179.
  - **Task 6** — `internal/filter/http/oauth2/cookies.go` + `stats.go` + tests + ADR-0181.
  - **Task 7** — `internal/filter/http/oauth2/tokens.go` + tests + ADR-0182 + `TestAesKeySwap_Concurrent_*` race tests.
- **Task 5** (NEW `internal/filter/http/oauth2/decode_headers.go` + `callback.go` + dispatcher + ADR-0180) — depends on Task 4 (HMAC helpers for cookie validation) + Task 6 (cookie envelope reader) + Task 7 (decryption helpers) for the cookie-validate path.
- **Task 8** (refresh-token rotation continuation in `callback.go` + race tests) — depends on Task 5 (dispatch wiring) + Task 7 (AES helpers); produces ADR-0183.
- **Task 9** (NEW `internal/filter/http/oauth2/signout.go` + ADR-0184) — depends on Task 5 (dispatch wiring) + Task 6 (cookie envelope writer for Max-Age=0).
- **Task 10** (NEW `internal/filter/http/oauth2/oauth_client.go` + `urlEncode` helper + ADR-0185) — depends on Task 2 (`*httpclient.Client` available) + Task 5 (callback dispatch wiring for the `postTokenEndpoint` call site).
- **Task 11** (full filter integration in `oauth2.go` + `compiled_config.go` + boot-registration at `cmd/envoy-go/main.go` + ADR final-state alignment in DECISIONS.md) — depends on Tasks 3-10 (consumes all prior surfaces); produces a fully-functional `api.HTTPFilterFactory` from `New()`.
- **Task 12** (differential fixture `0024-http-oauth2` + NEW `test/helpers/oauthbackend/` + 26th fuzzer `FuzzOAuth2ConfigParse` + RATIFIED-PENDING-IMPL-TIME pin closures per D16 + D17) — depends on Task 11 (full filter integration); CLOSES SPEC §12 items A1-A5 + B6 + B7 + cross-package C8 (with help from D5).
- **Task 13** (BEHAVIOR_CONTRACT.md 10-edit bundle per SPEC §13) — depends on Task 12 (some §13 paragraphs reference the fixture-0024 wire-shape closures from Task 12).
- **Task 14** (six-gate phase-done verification A-F per §7.5 + cross-package regression matrix per §12 item C8 + STATE.md re-advance + ROADMAP row 20 flip in-progress → done + REVIEW.md authoring per `superpowers:requesting-code-review`) — depends on everything.

**Parallel-dispatch opportunity at Tasks 3+4+6+7** — four agents can run concurrently on disjoint files. **Sequential bottleneck at Task 2 → Task 5 → Task 11** — the framework primitives + the dispatcher + the integration are the critical path. **Tasks 8+9+10** can be partially parallelized (different files; Task 10 + Task 9 are file-disjoint and depend only on Task 5; Task 8 cross-cuts callback.go which Task 5 owns) — but the IMPL is simpler if 8+9+10 run sequentially after Task 5 + Task 7.

---

## Execution preconditions

Before Task 1 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`). The expected sequence (executed by the orchestrating session before invoking the IMPL session, OR by the IMPL session at cold-start if standalone):

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-20-http-filter-oauth2-impl \
                 -b phase-20-http-filter-oauth2-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-20-http-filter-oauth2-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 17 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-20-http-filter-oauth2-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -8` shows the phase-20-PLAN.md squash commit + its SHA-fill follow-up at the head, with the phase-20-SPEC.md squash commit `4df55be` + its SHA-fill follow-up `9cb4292` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `185` (ADR-0185 — the highest ADR anchored as of master tip per the phase-20 SPEC commit). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0177' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0177 §Context already at SPEC commit per ADR-0044). Same for ADR-0178 through ADR-0185. `grep -nE '^## ADR-0186' docs/envoy-go/DECISIONS.md` returns 0 (ADR-0186 stays unconsumed at phase-20 IMPL under D11 hypothesis).
6. **2 IN-PLACE §Decision AMENDMENT-anticipation paragraphs present.** `grep -nE 'AMENDMENT body \+ §Consequences refresh land at phase-20 IMPL Task 2' docs/envoy-go/DECISIONS.md` returns ≥1 match in the ADR-0150 §Decision body block AND ≥1 match in the ADR-0159 §Decision body block — confirms the SPEC-time AMENDMENT-anticipation paragraphs anchored.
7. **NO ADR-0125 §(xv) amendment.** `grep -nE '\(xv\)' docs/envoy-go/DECISIONS.md` returns 0 matches — phase 20 lands NO ADR-0125 amendment (ADR-0180 records the explicit no-amendment REUSE-by-absence decision; THIRD CONSECUTIVE §9 row after phase 18 + phase 19 to skip). If `(xv)` returns ≥1, investigate before proceeding.
8. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/20-http-filter-oauth2/SPEC.md` returns `4df55be` (or descendant). If different, re-read SPEC.
9. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/20-http-filter-oauth2/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
10. **Pristine tree.** `git status --porcelain` returns empty.
11. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
12. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-3])'` returns every fixture 0000–0023 PASS — the 24 pre-existing fixtures are the regression baseline. Phase 20 adds the 25th (`0024-http-oauth2` per Task 12).
13. **Pre-existing fuzzers run clean at 30s.** The 25 fuzzers from phases 02–19.2 run clean. Phase 20 adds the 26th (`FuzzOAuth2ConfigParse` per Task 12).
14. **Reference Envoy image present.** `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin; unchanged).
15. **`envoy.extensions.filters.http.oauth2.v3` proto package reachable.** `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/oauth2/v3 OAuth2 | head -10` returns the `OAuth2` config without an `import path failed` error. If it fails, `go mod download`.
16. **Pre-existing `internal/filter/http/oauth2/` directory does NOT exist.** `test ! -d internal/filter/http/oauth2 && echo "ok: oauth2 absent"` returns success.
17. **Pre-existing `internal/httpclient/` + `internal/sdsfile/` + `test/helpers/oauthbackend/` directories do NOT exist + `github.com/fsnotify/fsnotify` is NOT yet in go.mod.** `test ! -d internal/httpclient && test ! -d internal/sdsfile && test ! -d test/helpers/oauthbackend && ! grep -q 'github.com/fsnotify/fsnotify' go.mod && echo "ok: phase-20-new-surfaces absent"` returns success.

If all 17 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0177..ADR-0185 §Context drafts are at the SPEC commit `4df55be` (re-anchored at SHA-fill follow-up `9cb4292`); the 2 IN-PLACE §Decision AMENDMENT-anticipation paragraphs at ADR-0150 + ADR-0159 are at the same commit; ADR-0186 is CONDITIONAL (PLAN hypothesis per D11: it does NOT fire at phase-20 IMPL). The PROGRESS preamble ANTICIPATES the 9 NEW ADR landings + the 2 IN-PLACE AMENDMENT landings (each with its Lands-in-Task anchor reproduced from this PLAN's per-ADR table) and records the 19 planner-time decisions D1-D19.

**Precondition:** worktree exists at `phase-20-http-filter-oauth2-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 17 preconditions report green.
**Artifact:** `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (new file).
**Acceptance:** all 17 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` with: (a) Preamble summarizing the 17-precondition verification (verbatim command outputs captured); (b) the 9-NEW-ADR + 2-IN-PLACE-AMENDMENT table from `## ADRs introduced/landed by this plan` reproduced verbatim; (c) the 19 planner-time decisions D1-D19 reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Task 1 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 1: PROGRESS.md preamble + 17-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
# expect: a 40-char SHA (Task 1 commit)
```

---

## Task 2: NEW `internal/httpclient/` + paired ADR-0150 + ADR-0159 in-place refactors (3 IMPL-internal sub-commits per D1)

**Files (sub-commit 2a — NEW `internal/httpclient/`):**
- Create: `internal/httpclient/httpclient.go` (~150-250 LoC)
- Create: `internal/httpclient/httpclient_test.go` (~150-250 LoC)
- Modify: `cmd/envoy-go/main.go` (+1 import; construct shared `*httpclient.Client` instance)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0177 §Decision + §Consequences body — EXTENDS the SPEC-commit §Context draft per ADR-0044)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 2a entry)

**Files (sub-commit 2b — ADR-0150 jwks Fetcher refactor):**
- Modify: `internal/jwks/fetcher.go` (~+40/-30; constructor signature gains `*httpclient.Client` parameter; `doFetch` delegates to `Client.Do`)
- Modify: `internal/jwks/fetcher_test.go` (~+30; tests adapted)
- Modify: `cmd/envoy-go/main.go` (thread `httpClient` into `jwks.New(...)`)
- Modify: `docs/envoy-go/DECISIONS.md` (~+50 LoC: ADR-0150 §Decision AMENDMENT body + §Consequences cross-phase-consumer paragraph; per ADR-0044 in-place edit discipline)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 2b entry)

**Files (sub-commit 2c — ADR-0159 extauthz refactor + §Future Work CLOSURE):**
- Modify: `internal/filter/http/extauthz/check.go` (~+50/-40; `httpAuthClient` constructor signature gains `*httpclient.Client` parameter; `hac.client.Do(outReq)` delegates to new primitive)
- Modify: `internal/filter/http/extauthz/check_test.go` (~+30; tests adapted)
- Modify: `cmd/envoy-go/main.go` (thread `httpClient` into `extauthz.NewHTTPAuthClient(...)`)
- Modify: `docs/envoy-go/DECISIONS.md` (~+80 LoC: ADR-0159 §Decision AMENDMENT body + §Future Work CLOSURE-AT-PHASE-20 paragraph per SPEC §3.5 + §9 item 1 — **FIRST §9 family-row to CLOSE prior-phase load-bearing forward-pointer**)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 2c entry)

This task lands the foundational framework primitive that all 3 introduction-time consumers (jwks + extauthz + oauth2) depend on. The 3 sub-commits preserve reviewer-clarity (per D1): 2a anchors ADR-0177 + the singleton instance; 2b refactors jwks; 2c refactors extauthz + closes the ADR-0159 §Future Work forward-pointer load-bearing — the **FIRST §9 family-row to demonstrate prior-phase forward-pointer-and-close discipline functioning across phase boundaries** per BRAINSTORM §11 Lesson (d).

**Precondition:** Task 1 complete; preconditions 16 + 17 verified ABSENT.
**Artifact:** `internal/httpclient/` package + paired refactors in jwks + extauthz; `cmd/envoy-go/main.go` constructs shared `*httpclient.Client`; ADR-0177 §Decision + §Consequences body + ADR-0150 + ADR-0159 §Decision AMENDMENT bodies + ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph anchored.
**Acceptance:** `go build ./...` clean after each sub-commit; `go vet ./...` clean; `golangci-lint run ./internal/httpclient/...` clean after 2a; `go test -count=1 ./internal/httpclient/...` clean after 2a (Group 6 — 10-12 tests pass); fixture-0019 GREEN after 2b (`go test -count=1 ./test/differential/ -run 'Test.*0019'`); fixture-0020 + fixture-0021 GREEN after 2c; `grep -cE '^## ADR-0177' docs/envoy-go/DECISIONS.md` returns `1` AND the §Decision body is non-empty; ADR-0150 + ADR-0159 §Decision body each gains an AMENDMENT paragraph dated 2026-05-17 cross-referenced to phase 20 + ADR-0177; ADR-0159 §Future Work gains a CLOSED-AT-PHASE-20 paragraph.

### Sub-commit 2a — NEW `internal/httpclient/` package

- [ ] **Step 2a.1: Write failing tests** in `internal/httpclient/httpclient_test.go` per SPEC §14.1 Group 6 — Options zero-value (timeout 0 = no deadline; zero RetryPolicy = no retries; nil TLSConfig); Client.Do happy-path 200 OK; zero-retry default (verify single attempt even on 5xx); retry envelope (3 status codes; verify attempt count == Attempts + 1); ctx cancellation mid-Do (`context.WithTimeout` 1ms → expect `context.DeadlineExceeded`); TLSConfig wired through; request-error propagation.

- [ ] **Step 2a.2: Run tests to verify they fail**

```bash
go test ./internal/httpclient/... 2>&1 | head -20
# Expect: FAIL with "no such package" or similar
```

- [ ] **Step 2a.3: Author `internal/httpclient/httpclient.go`** per the File-structure table row above + SPEC §3.1 + ADR-0177 §Context. Includes the 4 type/function declarations: `Options`, `RetryPolicy`, `Client`, `New`, `(*Client).Do`. The retry loop honors `ctx.Err()` after every sleep (do NOT retry if context cancelled).

- [ ] **Step 2a.4: Run tests to verify they pass**

```bash
go test -count=1 ./internal/httpclient/... 
# Expect: PASS — 10-12 tests
```

- [ ] **Step 2a.5: Modify `cmd/envoy-go/main.go`** — add the import `httpclient "github.com/esalaine/envoy-go/internal/httpclient"`; construct the singleton via `httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})` BEFORE the boot-registration block. Sub-commit 2a leaves the jwks + extauthz boot-sites UNTOUCHED (they refactor at 2b + 2c respectively).

- [ ] **Step 2a.6: Author ADR-0177 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft at the SPEC commit `4df55be`. §Decision body covers: the Options + RetryPolicy + Client + Do shape; the synchronous semantics; the per-Client wraps-`*http.Client` discipline; the cross-phase reuse intent (3 consumers at introduction time); the zero-Options zero-cost no-op default. §Consequences body covers: cross-phase-reusability for future outbound-HTTP consumers (future ext_authz mTLS, future jwt_authn alternative-issuer fetch, future ratelimit gRPC TLS); the closure of ADR-0159 §Future Work forward-pointer load-bearing (the third-consumer trigger fires exactly as anticipated at phase 18.1 — **FIRST §9 family-row to demonstrate prior-phase forward-pointer-and-close discipline functioning across phase boundaries**); the introduction-time-3-consumer pattern as a reference for future framework-primitive extractions.

- [ ] **Step 2a.7: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 2a.8: Append PROGRESS.md Task 2a entry** — record build/test output + `git log -1 --format=%H` for the upcoming Task 2a commit.

- [ ] **Step 2a.9: Commit sub-commit 2a**

```bash
git add internal/httpclient/httpclient.go \
        internal/httpclient/httpclient_test.go \
        cmd/envoy-go/main.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 2a: NEW internal/httpclient/ framework primitive + ADR-0177

Lands the Options + RetryPolicy + Client.Do synchronous wrapper over
*http.Client per ADR-0177. Shared *httpclient.Client singleton instance
constructed at cmd/envoy-go/main.go boot for use by jwks + extauthz + oauth2.
Group 6 tests (10-12 tests) all pass. ADR-0177 §Decision + §Consequences
anchored (EXTENDS SPEC-commit §Context draft per ADR-0044).

3 introduction-time consumers anticipated: jwks Fetcher (Task 2b refactor) +
extauthz httpAuthClient (Task 2c refactor) + oauth2 token_endpoint POST
(Task 10 NEW consumer). Closes ADR-0159 §Future Work forward-pointer
load-bearing at Task 2c."
```

### Sub-commit 2b — ADR-0150 jwks Fetcher refactor

- [ ] **Step 2b.1: Modify `internal/jwks/fetcher.go`** — `New` constructor signature gains `*httpclient.Client` parameter (REPLACES the internal `&http.Client{...}` instantiation); `doFetch` inner HTTP-request loop delegates to `client.Do` rather than the existing inline shape. Preserve verbatim: timeout, retry-policy, TLS posture semantics (ADR-0177's `Options` carries the phase-17-pinned semantics).

- [ ] **Step 2b.2: Modify `internal/jwks/fetcher_test.go`** — adapt existing tests to construct the `*Fetcher` via a `*httpclient.Client` test instance (e.g., `httpclient.New(httpclient.Options{Timeout: 5 * time.Second})`). NO new test surface beyond keeping existing coverage GREEN.

- [ ] **Step 2b.3: Modify `cmd/envoy-go/main.go`** — thread the shared `httpClient` into `jwks.New(httpClient, ...)` constructor call.

- [ ] **Step 2b.4: Verify all jwks tests + fixture-0019 GREEN**

```bash
go test -count=1 ./internal/jwks/...
# Expect: PASS — existing test coverage preserved
go test -count=1 ./test/differential/ -run 'Test.*0019'
# Expect: PASS — fixture-0019 (jwt_authn) GREEN post-refactor (per D5)
```

- [ ] **Step 2b.5: Author ADR-0150 §Decision AMENDMENT body in DECISIONS.md** — APPENDS an AMENDMENT paragraph dated 2026-05-17 to the existing §Decision body block + a cross-phase-consumer disposition paragraph to the existing §Consequences body block per SPEC §3.4 + the SPEC-commit AMENDMENT-anticipation paragraph. The AMENDMENT paragraph documents: the `Fetcher` no longer owns its own `*http.Client`; takes a `*httpclient.Client` constructor argument; the `doFetch` inner loop delegates to `Client.Do`; ~40-60 LoC delta. The cross-phase-consumer paragraph documents the 3-consumer view (jwks + extauthz + oauth2) — closes the implicit forward-pointer to a future httpclient primitive per §9 item 2.

- [ ] **Step 2b.6: Append PROGRESS.md Task 2b entry.**

- [ ] **Step 2b.7: Commit sub-commit 2b**

```bash
git add internal/jwks/fetcher.go \
        internal/jwks/fetcher_test.go \
        cmd/envoy-go/main.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 2b: ADR-0150 jwks Fetcher refactor — consumes *httpclient.Client

Per ADR-0044 in-place edit discipline + the phase-20 SPEC §3.4. The Fetcher
constructor signature gains a *httpclient.Client parameter (replaces the
internal http.Client instantiation); doFetch delegates to Client.Do. Timeout
+ retry-policy + TLS posture preserved verbatim (ADR-0177's Options carries
the phase-17-pinned semantics). Fixture-0019 (jwt_authn) GREEN post-refactor.
ADR-0150 §Decision AMENDMENT body + §Consequences cross-phase-consumer
paragraph anchored."
```

### Sub-commit 2c — ADR-0159 extauthz refactor + §Future Work CLOSURE-AT-PHASE-20 paragraph

- [ ] **Step 2c.1: Modify `internal/filter/http/extauthz/check.go`** — `httpAuthClient` constructor signature gains `*httpclient.Client` parameter (REPLACES the internal `&http.Client{Timeout: hs.server_uri.timeout}` instantiation); the `checkFn` closure's `hac.client.Do(outReq)` delegates to the new primitive's `Do` method. Preserve verbatim: per-request cancellable semantics, zero-retry-default discipline, `OnDestroy`-drives-cancel path (ADR-0177's `Options{Timeout}` carries `HttpService.server_uri.timeout`; per-request `ctx` thread unchanged).

- [ ] **Step 2c.2: Modify `internal/filter/http/extauthz/check_test.go`** — adapt existing tests to construct the `*httpAuthClient` via a `*httpclient.Client` test instance. NO new test surface.

- [ ] **Step 2c.3: Modify `cmd/envoy-go/main.go`** — thread the shared `httpClient` into the extauthz HTTP-mode boot site (most-likely a parameter to `extauthz.New` or a similar factory; the exact integration depends on the existing extauthz factory shape — IMPL settles).

- [ ] **Step 2c.4: Verify all extauthz tests + fixture-0020 + fixture-0021 GREEN**

```bash
go test -count=1 ./internal/filter/http/extauthz/...
# Expect: PASS — existing test coverage preserved
go test -count=1 ./test/differential/ -run 'Test.*0020'
# Expect: PASS — fixture-0020 (ext_authz HTTP-mode) GREEN post-refactor (per D5)
go test -count=1 ./test/differential/ -run 'Test.*0021'
# Expect: PASS — fixture-0021 (ext_authz gRPC-mode) GREEN; untouched but
# verifies no incidental breakage
```

- [ ] **Step 2c.5: Author ADR-0159 §Decision AMENDMENT body + §Future Work CLOSURE-AT-PHASE-20 paragraph in DECISIONS.md** — APPENDS an AMENDMENT paragraph dated 2026-05-17 to the existing §Decision body block per SPEC §3.5 + the SPEC-commit AMENDMENT-anticipation paragraph. The AMENDMENT paragraph documents: the `httpAuthClient` no longer owns its own `*http.Client`; takes a `*httpclient.Client` constructor argument; the `hac.client.Do(outReq)` delegates to `Client.Do`; ~50-80 LoC delta. The §Future Work CLOSURE-AT-PHASE-20 paragraph is appended to the existing §Consequences "Deferred `internal/httpclient/` generalization + the oauth2 trigger" paragraph — records that the third-consumer trigger fires exactly as anticipated at phase 18.1; the 3-consumer view (jwks + ext_authz + oauth2) is achieved; the generalization fires per ADR-0177 introduction; **PHASE 20 IS THE FIRST §9 FAMILY-ROW TO CLOSE A PRIOR-PHASE LOAD-BEARING FORWARD-POINTER** — a structurally important demonstration that the ADR-0044 §Future-Work forward-pointer-and-close discipline functions across phase boundaries (per SPEC §9 item 1 + BRAINSTORM §11 Lesson (d)).

- [ ] **Step 2c.6: Append PROGRESS.md Task 2c entry** — record the FIRST-§9-family-row-CLOSURE milestone explicitly.

- [ ] **Step 2c.7: Commit sub-commit 2c**

```bash
git add internal/filter/http/extauthz/check.go \
        internal/filter/http/extauthz/check_test.go \
        cmd/envoy-go/main.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 2c: ADR-0159 extauthz refactor + §Future Work CLOSURE-AT-PHASE-20

Per ADR-0044 in-place edit discipline + the phase-20 SPEC §3.5 + §9 item 1.
The httpAuthClient constructor gains a *httpclient.Client parameter (replaces
the internal http.Client instantiation); the checkFn closure delegates to
Client.Do. Per-request cancellable semantics + zero-retry-default discipline
+ OnDestroy cancel path preserved verbatim. Fixture-0020 + fixture-0021 GREEN
post-refactor.

ADR-0159 §Decision AMENDMENT body + §Future Work CLOSURE-AT-PHASE-20
paragraph anchored. PHASE 20 IS THE FIRST §9 FAMILY-ROW TO CLOSE A
PRIOR-PHASE LOAD-BEARING FORWARD-POINTER per SPEC §9 item 1 + BRAINSTORM §11
Lesson (d) — structurally important demonstration that the ADR-0044
§Future-Work forward-pointer-and-close discipline functions across phase
boundaries."
```

---

## Task 3: NEW `internal/sdsfile/` package + ADR-0178 + go.mod fsnotify dep

**Files:**
- Create: `internal/sdsfile/sdsfile.go` (~160-200 LoC)
- Create: `internal/sdsfile/sdsfile_test.go` (~250-350 LoC; Group 7 unit tests + `TestWatcher_DebounceRace_*` race group per D4)
- Modify: `go.mod` / `go.sum` (+1 direct dep `github.com/fsnotify/fsnotify` per D12)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0178 §Decision + §Consequences body — EXTENDS the SPEC-commit §Context draft per ADR-0044)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 3 entry)

This task lands the second NEW top-level framework primitive — the filesystem-path SDS Secret reader with fsnotify-driven hot-reload + atomic-swap discipline. Cross-phase-reusable for any future filesystem-SDS consumer. **Parallelizable with Tasks 4 + 6 + 7** per D9 (disjoint files; depends only on Task 2 being landed).

**Precondition:** Task 2 complete (3 sub-commits landed); the singleton `*httpclient.Client` is constructed in `cmd/envoy-go/main.go`.
**Artifact:** `internal/sdsfile/` package + go.mod fsnotify dep; ADR-0178 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/sdsfile/...` clean; `go test -count=1 ./internal/sdsfile/...` clean (Group 7 — 12-15 tests pass); `go test -race -count=1 ./internal/sdsfile/...` clean (race tests pass); `grep -cE '^## ADR-0178' docs/envoy-go/DECISIONS.md` returns `1` AND §Decision body non-empty; `go list -m github.com/fsnotify/fsnotify` returns the pinned version.

- [ ] **Step 1: `go get -u github.com/fsnotify/fsnotify@latest`** — adds the latest v1.x minor tag per D12; verify `go list -m github.com/fsnotify/fsnotify` reports the pinned version.

- [ ] **Step 2: Write failing tests** in `internal/sdsfile/sdsfile_test.go` per SPEC §14.1 Group 7 + §12 item B7:
  - `TestWatcher_New_LoadsInitialBytes` — points at a tempfile; verify `Current()` returns the initial bytes
  - `TestWatcher_Start_ObservesAtomicRename` — `os.Rename`-based update of the watched file → `Current()` returns new bytes (after ~150ms debounce wait)
  - `TestWatcher_Start_ObservesInPlaceTruncate` — `os.WriteFile`-based in-place rewrite → `Current()` returns new bytes
  - `TestWatcher_Debounce_CollapsesRapidWrites` — 5 back-to-back writes within 50ms → ONE reload; `Current()` returns the LAST bytes per §12 item B7
  - `TestWatcher_Close_Idempotent` — `Close()` called twice → no panic
  - `TestWatcher_Close_DuringInFlightReload_NoPanic` — `Close()` during a reload-in-progress → no panic
  - `TestWatcher_DebounceRace_ConcurrentCurrent` — N goroutines invoking `Current()` during a reload-in-flight → all observe consistent bytes (no torn read)
  - `TestWatcher_DebounceRace_ConcurrentWrites` — multiple writers + multiple `Current()` readers under `-race` → no races
  - `TestWatcher_DebounceRace_CloseRacesReload` — `Close()` racing a reload → clean termination

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/sdsfile/... 2>&1 | head -20
# Expect: FAIL with "no such package"
```

- [ ] **Step 4: Author `internal/sdsfile/sdsfile.go`** per the File-structure table row above + SPEC §3.2 + ADR-0178 §Context. The Watcher uses `atomic.Pointer[[]byte]` for the in-memory bytes; `*fsnotify.Watcher` for the file event stream; `time.AfterFunc(100 * time.Millisecond, w.reload)` for the debouncer (reset on each event); `sync.Mutex` for the debounce-timer state; `sync.Once` for `Close()` idempotency; goroutine reading `w.fsWatcher.Events` + `w.fsWatcher.Errors` with `select` on `done` channel for clean shutdown. The `reload` method does `os.ReadFile(w.path)` + `atomic.StorePointer(&w.bytes, &newBytes)`.

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test -count=1 ./internal/sdsfile/...
# Expect: PASS — 12-15 tests
go test -race -count=1 ./internal/sdsfile/...
# Expect: PASS — race tests under -race
```

- [ ] **Step 6: Author ADR-0178 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft at the SPEC commit `4df55be`. §Decision body covers: the Watcher + New + Start + Current + Close surface; the `atomic.Pointer[[]byte]` swap discipline; the ~100ms debounce window per §12 item B7; the fsnotify event-subset (Write + Create + Rename) covered; PARSE-REJECT discipline (the inner `secret_file` indirect-arm + the non-filesystem ConfigSource arms PARSE-REJECT at the consumer's compiledConfig parser per §2.11 + §8 item 14 — the sdsfile primitive itself takes a filesystem path and reads it verbatim). §Consequences body covers: cross-phase-reusability (future jwt_authn TLS-trust-store reload, future ext_authz mTLS, future ratelimit gRPC TLS); the NEW go.mod direct dep `github.com/fsnotify/fsnotify` (v1.7.0+ per D12); the MVP consumer is oauth2 (hmac_secret + client_secret); the leaks-on-exit lifecycle ownership per D13.

- [ ] **Step 7: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 8: Append PROGRESS.md Task 3 entry.**

- [ ] **Step 9: Commit**

```bash
git add internal/sdsfile/sdsfile.go \
        internal/sdsfile/sdsfile_test.go \
        go.mod go.sum \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 3: NEW internal/sdsfile/ framework primitive + ADR-0178

Lands the Watcher{New, Start, Current, Close} fsnotify-driven filesystem-path
SDS Secret reader with ~100ms debounce + atomic.Pointer[[]byte] swap per
ADR-0178. NEW go.mod direct dep github.com/fsnotify/fsnotify v1.7.0+ per D12.
Group 7 unit tests (12-15 tests) + TestWatcher_DebounceRace_* race group
(3-5 tests per D4) all pass under -race. ADR-0178 §Decision + §Consequences
anchored.

Closes SPEC §12 item B7 RATIFIED-PENDING-IMPL-TIME (fsnotify event-debounce
window precise behavior). MVP consumer: oauth2 (Task 11 integration).
Cross-phase-reusable for any future filesystem-SDS consumer."
```

---

## Task 4: NEW `internal/filter/http/oauth2/hmac.go` + ADR-0179

**Files:**
- Create: `internal/filter/http/oauth2/hmac.go` (~120-180 LoC)
- Create: `internal/filter/http/oauth2/hmac_test.go` (~250-350 LoC; Group 2 vector tests + dual-encoding read per AMEND-2 + S4)
- Modify: `docs/envoy-go/DECISIONS.md` (~+80 LoC: ADR-0179 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 4 entry)

Per AMEND-2 + §20.P4 REFUTED: 5-input newline-joined HMAC composition + dual-encoding read per S4. Cross-task-parallelizable with Tasks 3 + 6 + 7 per D9.

**Precondition:** Task 2 complete; `internal/filter/http/oauth2/` package directory may or may not exist at start (this Task may be the first to create it; if so, the package skeleton is established here OR at Task 6/7 — whichever lands first per the parallel-dispatch schedule).
**Artifact:** `hmac.go` + tests; ADR-0179 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/oauth2/...` clean; `go test -count=1 ./internal/filter/http/oauth2/... -run 'TestComputeHMAC|TestValidateHMAC'` clean (15-20 vector tests pass); `grep -cE '^## ADR-0179' docs/envoy-go/DECISIONS.md` returns `1` AND §Decision body non-empty.

- [ ] **Step 1: If `internal/filter/http/oauth2/` does NOT exist, create it with a stub `oauth2.go` containing just `package oauth2`** — needed for the Go compiler to recognize the package. (The full `oauth2.go` body lands at Task 11.)

- [ ] **Step 2: Write failing vector tests** in `internal/filter/http/oauth2/hmac_test.go` per SPEC §14.1 Group 2:
  - `TestComputeHMAC_FullEnvelope` — domain + expires + token + idToken + refreshToken non-empty → known-vector base64 output
  - `TestComputeHMAC_NoIdToken_NoRefreshToken` — id_token + refresh_token both empty strings → distinct vector output (verifies the empty-string contribution per §20.P4 REFUTED)
  - `TestComputeHMAC_OnlyRefreshTokenAbsent` — id_token non-empty + refresh_token empty
  - `TestComputeHMAC_KnownVectorsMatchUpstream` — table-driven row matching upstream test vectors (the IMPL captures the reference Envoy vector at first call)
  - `TestValidateHMAC_Base64EncodingAccepted` — envelope OauthHMAC encoded as Base64URL → validates GREEN
  - `TestValidateHMAC_HexBase64EncodingAccepted` — envelope OauthHMAC encoded as Base64URL-of-hex → validates GREEN (dual-encoding per S4)
  - `TestValidateHMAC_TamperedHmac_Rejected` — tampered OauthHMAC → validates RED
  - `TestValidateHMAC_TamperedToken_Rejected` — tampered BearerToken → validates RED
  - `TestValidateHMAC_TamperedDomain_Rejected` — request host mismatched against cookie state → validates RED
  - `TestValidateHMAC_ConstantTimeCompare` — verify `crypto/hmac.Equal` is used (test by inspection or via timing-side-channel sanity check; bypass if timing tests are flaky)
  - 5-10 additional vector rows covering boundary cases (empty domain, max-length expires, unicode in token, etc.)

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/filter/http/oauth2/... -run 'TestComputeHMAC|TestValidateHMAC' 2>&1 | head -20
# Expect: FAIL with "undefined: computeHMAC" or similar
```

- [ ] **Step 4: Author `internal/filter/http/oauth2/hmac.go`** per the File-structure table row above + SPEC §6.4 + §6.5 + ADR-0179 §Context. The composition: `HMAC-SHA256(hmac_secret, []byte(domain + "\n" + expires + "\n" + token + "\n" + idToken + "\n" + refreshToken))`. The dual-encoding read tries `base64.RawURLEncoding.DecodeString` first; on parse failure tries nested `base64.RawURLEncoding.DecodeString(...) → hex.DecodeString(...)`. Comparison via `crypto/hmac.Equal`.

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test -count=1 ./internal/filter/http/oauth2/... -run 'TestComputeHMAC|TestValidateHMAC'
# Expect: PASS — 15-20 vector tests
```

- [ ] **Step 6: Author ADR-0179 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft. §Decision body covers: 5-input newline-joined composition; id_token + refresh_token empty when absent; dual-encoding read (Base64 + HexBase64 BOTH accepted); constant-time-compare via `crypto/hmac.Equal`. §Consequences body covers: AMEND-2 corrects BRAINSTORM Q9 3-input hypothesis (REFUTED per §20.P4); dual-encoding-read covers operator-configurable encoding drift (some upstream deployments emit HexBase64 due to historical config; this widens compatibility per S4); the byte-exact upstream match for the emitted Base64 envelope keeps wire-compat.

- [ ] **Step 7: Append PROGRESS.md Task 4 entry.**

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/oauth2/hmac.go \
        internal/filter/http/oauth2/hmac_test.go \
        $(ls internal/filter/http/oauth2/oauth2.go 2>/dev/null || true) \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 4: oauth2/hmac.go — 5-input HMAC composition + dual-encoding read + ADR-0179

Per AMEND-2 + §20.P4 REFUTED + S4. 5-input newline-joined
HMAC-SHA256(hmac_secret, StrJoin({domain, expires, token, id_token,
refresh_token}, '\n')) with id_token + refresh_token empty when absent.
Dual-encoding read accepts BOTH Base64 + HexBase64 on validation. Emits
Base64. Constant-time compare via crypto/hmac.Equal. Group 2 vector tests
(15-20 tests) pass. ADR-0179 §Decision + §Consequences anchored."
```

---

## Task 5: NEW `internal/filter/http/oauth2/decode_headers.go` + `callback.go` (dispatcher + 4-emission-category handlers) + ADR-0180

**Files:**
- Create: `internal/filter/http/oauth2/decode_headers.go` (~280-380 LoC)
- Create: `internal/filter/http/oauth2/callback.go` (~280-380 LoC; handleCallback structure — applyTokenEndpointResponse continuation body at Task 8 + Task 10 integration)
- Modify: `internal/filter/http/oauth2/oauth2.go` (~+30 LoC; register `RegisterPerRouteValidator` per SPEC §6.1)
- Modify: `docs/envoy-go/DECISIONS.md` (~+200 LoC: ADR-0180 §Decision + §Consequences body — covers state-machine + deny-path + listener-scoped + NO-ADR-0125-AMENDMENT classification)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 5 entry)

This task lands the dispatcher core + the 4-emission-category handlers per SPEC §4.1 + §6.3. Depends on Task 4 (HMAC helpers) + Task 6 (cookie envelope reader) + Task 7 (decryption helpers); the IMPL sequences Task 5 AFTER 3+4+6+7 land. The HCM-parse-time PARSE-REJECT for route-level placement per §5.2 + D2 byte-stable wording is registered here via `RegisterPerRouteValidator`.

**Precondition:** Tasks 3 + 4 + 6 + 7 complete; `internal/filter/http/oauth2/{hmac.go, cookies.go, tokens.go, stats.go}` available; `internal/sdsfile/` + `internal/httpclient/` available.
**Artifact:** `decode_headers.go` + `callback.go` with full dispatcher + 4-category handlers; `oauth2.go` registers `RegisterPerRouteValidator`; ADR-0180 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/oauth2/... -run 'TestDispatcher|TestHandleUnauthenticated|TestHandlePassThrough|TestHandleValidCookies|TestHandleBadState'` clean (8-10 dispatcher tests + per-handler tests pass); per-route TPFC PARSE-REJECT byte-stable wording verified per D2 + D15.

- [ ] **Step 1: Write failing tests** in `internal/filter/http/oauth2/oauth2_test.go` per SPEC §14.1 Group 8 dispatcher dispatch tests:
  - `TestDispatcher_SignoutPath_Highest_Priority` — signout_path match → `handleSignout` dispatched
  - `TestDispatcher_CallbackPath_GET_HandlesCallback` — GET to redirect_path_matcher → `handleCallback` dispatched
  - `TestDispatcher_CallbackPath_POST_ParseRejects` — POST to redirect_path_matcher → category (d) 401 per §2.14 + D15
  - `TestDispatcher_PassThroughMatcher_Hits_Bypasses` — header match → `handlePassThrough` + `oauth_passthrough++`
  - `TestDispatcher_ValidCookieEnvelope_ContinuesDecoding` — valid envelope → ContinueDecoding (with optional Authorization injection per `forward_bearer_token=true`)
  - `TestDispatcher_ExpiredBearerToken_ValidRefreshToken_DispatchesRefresh` — expired BearerToken + valid RefreshToken → `handleRefresh` dispatched (the async POST itself lands at Task 8)
  - `TestDispatcher_Unauthenticated_EmitsCategory_A_302_Challenge` — no valid envelope → category (a) 302 with state cookie
  - `TestHandleBadState_EmitsCategory_D_401_With_Constant_Body` — bad state cookie → category (d) 401 with `"OAuth flow failed."` body + `addFlowCookieDeletionHeaders` per AMEND-3
  - `TestHandlePassThrough_NoOauth2Emission_IncrementsCounter` — bypass + counter increment
  - `TestRegisterPerRouteValidator_PARSE_REJECTS_RouteLevel_Placement` — `RegisterPerRouteValidator` registers a validator that PARSE-REJECTs route-level TPFC per §5.2 + D2; the byte-stable wording matches the D2 reference string

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `internal/filter/http/oauth2/decode_headers.go`** per SPEC §6.3 + ADR-0180. Includes the `DecodeHeaders` dispatch priority order: (1) signout_path match → `handleSignout`; (2) redirectPathMatcher match → `handleCallback` (GET-only — POST PARSE-REJECTs per D15); (3) passThroughMatcher match → `handlePassThrough`; (4) else `handleCookieValidate` → ContinueDecoding OR `handleRefresh` OR `handleUnauthenticated`. Includes `handleUnauthenticated` (emits category (a) 302 + state cookie); `handlePassThrough` (bypass + counter); `handleValidCookies` (ContinueDecoding + Authorization-header injection per `forward_bearer_token=true`); `handleRefreshFailure` (category (a) 302 challenge + `oauth_refreshtoken_failure++`).

- [ ] **Step 4: Author `internal/filter/http/oauth2/callback.go`** per SPEC §6.8 + ADR-0180 (handleCallback structure — the actual async-resume continuation lands at Task 8 for refresh + Task 10 for token-endpoint POST integration). Includes `handleCallback` (parses callback URL params + state cookie verification + dispatches to `applyTokenEndpointResponse`); `applyTokenEndpointResponse` skeleton (the continuation body fully wired at Task 8 + Task 10); `handleBadState` (emits category (d) 401 with constant body + `addFlowCookieDeletionHeaders`).

- [ ] **Step 5: Modify `internal/filter/http/oauth2/oauth2.go`** — implement `RegisterPerRouteValidator(reg api.PerRouteValidatorRegistry)` per SPEC §6.1 + §5.2 + D2. The validator REJECTS any non-empty TPFC at route or virtualHost level with the D2 byte-stable wording `"oauth2: typed_per_filter_config not supported at route or virtualHost level; oauth2 is listener-scoped only"`. (Per ADR-0072 + ADR-0100 §2.2 — registered at the same boot site as the filter factory.)

- [ ] **Step 6: Run tests to verify they pass.**

- [ ] **Step 7: Author ADR-0180 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft. §Decision body covers: the 3-flow state machine (sign-in / refresh / sign-out + pass_through); the dispatch priority order per §6.3; the 4-emission-category wire shape (302 auth-challenge / 302 post-callback / 302 sign-out / 401 with constant body); NO 500 anywhere per AMEND-3 + §20.P9 REFUTED; the constant 401 body `"OAuth flow failed."` (18 bytes per §20.P9); the listener-scoped-only enforcement via `RegisterPerRouteValidator` HCM-parse-time PARSE-REJECT per §5.2 + D2; the explicit NO-ADR-0125-AMENDMENT REUSE-by-absence classification per §5.4 — THIRD CONSECUTIVE §9 row (after phase 18 + phase 19) to skip; phase 20's REUSE-by-absence is a stronger form (the absence itself is the lesson — no per-route surface at all per §20.P7 RATIFIED). §Consequences body covers: cross-phase reuse intent (the 4-emission-category pattern is reproducible for any future single-flow filter with similar wire shape); the AMEND-3 + §20.P9 + §20.P11 closures recorded; the THIRD CONSECUTIVE NO-ADR-0125-AMENDMENT strengthens the ADR-0125 roster-not-monotonic lesson.

- [ ] **Step 8: Append PROGRESS.md Task 5 entry.**

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/oauth2/decode_headers.go \
        internal/filter/http/oauth2/callback.go \
        internal/filter/http/oauth2/oauth2.go \
        internal/filter/http/oauth2/oauth2_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 5: oauth2/decode_headers.go + callback.go + ADR-0180 (state-machine + deny-path)

Lands the 3-flow state machine dispatcher per SPEC §6.3 + the 4-emission-
category handlers per SPEC §4.1 (302 auth-challenge / 302 post-callback /
302 sign-out / 401 with constant body) + the HCM-parse-time PARSE-REJECT
hook via RegisterPerRouteValidator per §5.2 + D2. NO 500 anywhere per
AMEND-3 + §20.P9 REFUTED. POST callback method PARSE-REJECTs per §2.14 +
D15. Group 8 dispatcher dispatch tests (8-10 tests) pass.

ADR-0180 §Decision + §Consequences anchored — including the explicit
NO-ADR-0125-AMENDMENT REUSE-by-absence classification per §5.4. THIRD
CONSECUTIVE §9 row to skip ADR-0125 amendment (REUSE-by-absence stronger
form than 5th-canonical REUSE)."
```

---

## Task 6: NEW `internal/filter/http/oauth2/cookies.go` + `stats.go` + ADR-0181

**Files:**
- Create: `internal/filter/http/oauth2/cookies.go` (~180-250 LoC)
- Create: `internal/filter/http/oauth2/cookies_test.go` (~250-350 LoC; Group 4 round-trip tests per §14.1)
- Create: `internal/filter/http/oauth2/stats.go` (~120-180 LoC; 6-counter filterStats + SN2 guards per D6)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0181 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 6 entry)

Cross-task-parallelizable with Tasks 3 + 4 + 7 per D9.

**Precondition:** Task 2 complete.
**Artifact:** `cookies.go` + `stats.go` + tests; ADR-0181 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `go test -count=1 ./internal/filter/http/oauth2/... -run 'TestParseAllCookies|TestFormatSetCookie|TestFilterStats|TestStatNames_Equal'` clean (15-20 round-trip tests + 6-counter assertion tests pass); `grep -cE '^## ADR-0181' docs/envoy-go/DECISIONS.md` returns `1` AND §Decision body non-empty.

- [ ] **Step 1: Write failing tests** in `internal/filter/http/oauth2/cookies_test.go` per SPEC §14.1 Group 4:
  - `TestParseAllCookies_FullEnvelope` — all 5 cookies present → returns map[string]string
  - `TestParseAllCookies_MissingIdToken` — IdToken absent (MVP-deferred per §2.2) → map omits idToken key
  - `TestParseAllCookies_MissingRefreshToken` — RefreshToken absent → map omits refreshToken key
  - `TestFormatSetCookie_DefaultAttributes` — name=value emits `name=value; Path=/; Secure; HttpOnly; SameSite=Lax` per §4.5 + D16 A2
  - `TestFormatSetCookie_MaxAgeZero` — sign-out clearing → `name=; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=0`
  - `TestRoundTrip_5CookieEnvelope` — emit → parse → byte-exact map equality
  - `TestPerCategory_Emission_Table_A_AuthChallenge` — category (a) emits cleared envelope cookies + state cookie SET to HMAC(state)
  - `TestPerCategory_Emission_Table_B_PostCallback` — category (b) emits BearerToken=encrypted_access_token + OauthHMAC=HMAC + OauthExpires=epoch + RefreshToken=encrypted_refresh_token
  - `TestPerCategory_Emission_Table_C_Signout` — category (c) emits Max-Age=0 for all 5 cookies
  - `TestPerCategory_Emission_Table_D_401` — category (d) emits cleared envelope via `addFlowCookieDeletionHeaders`
  - `TestStateCookiePayloadShape_EpochSecondsDecimalString` — OauthExpires format per §12 item A3 + D16 A3

- [ ] **Step 2: Write failing tests** in `internal/filter/http/oauth2/oauth2_test.go` (or `stats_test.go` if split) per D6 + §6.12:
  - `TestStatNames_Equal_OauthUnauthorizedRq` — `statNameOauthUnauthorizedRq == "oauth_unauthorized_rq"` byte-exact assertion
  - 5 more per-counter byte-exact assertions
  - `TestNewFilterStats_Registers6Counters` — `newFilterStats(ctx, "")` registers 6 distinct counters in stat-prefix `http.<statPrefix>.oauth2.<name>`

- [ ] **Step 3: Run tests to verify they fail.**

- [ ] **Step 4: Author `internal/filter/http/oauth2/cookies.go`** per the File-structure table row above + SPEC §6.4 + §6.5. `parseAllCookies(headers)` iterates the `Cookie:` header values and matches against `cc.cookieNames` (the operator-configurable names; defaults to upstream-canonical `BearerToken` / `OauthHMAC` / `OauthExpires` / `IdToken` / `RefreshToken` per ADR-0181 §Context). `formatSetCookie(name, value, attrs)` emits `name=value; <attrs>` with the §4.5 defaults. `addFlowCookieDeletionHeaders(headers, flow_id_)` emits Max-Age=0 Set-Cookie headers for all flow cookies per AMEND-3.

- [ ] **Step 5: Author `internal/filter/http/oauth2/stats.go`** per the File-structure table row above + SPEC §6.11 + ADR-0143 + D6. Declares the 6 byte-exact `const statName*` declarations + the `filterStats` struct + `newFilterStats(ctx, statPrefix) *filterStats` constructor.

- [ ] **Step 6: Run tests to verify they pass.**

- [ ] **Step 7: Author ADR-0181 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft. §Decision body covers: the 5-cookie envelope (BearerToken / OauthHMAC / OauthExpires / IdToken / RefreshToken); the Set-Cookie attribute defaults per §4.5 + §12 item A2 (Secure / HttpOnly / SameSite=Lax / Path=/); the 6-counter stat surface wire-exact upstream per AMEND-4 + S5 + §20.P8 REFUTED; the HCM-rooted SN2-reuse per ADR-0143 (`http.<HCM_stat_prefix>.oauth2.<counter>`); the per-category emission table per §4.5 + §4.1. §Consequences body covers: AMEND-4 reduces 86 → 92 names (NOT 94 per BRAINSTORM §5 over-count); CLOSES §20.P11 envoy-go-strict departure flag as RATIFIED-AS-ABSENT (no `cookie_decrypt_failure` counter); the `Partitioned` cookie attribute deferred per AMEND-7 + §8 item 15; the `cookie_configs` per-cookie attribute customization deferred per §2.5; cross-phase reuse intent (the 5-cookie envelope + 6-counter pattern is reusable for any future Standard-OAuth2 evolution).

- [ ] **Step 8: Append PROGRESS.md Task 6 entry.**

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/oauth2/cookies.go \
        internal/filter/http/oauth2/cookies_test.go \
        internal/filter/http/oauth2/stats.go \
        internal/filter/http/oauth2/oauth2_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 6: oauth2/cookies.go + stats.go + ADR-0181 (envelope + 6-counter surface)

Lands the 5-cookie envelope (BearerToken / OauthHMAC / OauthExpires /
IdToken / RefreshToken) + the 6-counter wire-exact stat surface per AMEND-4
+ S5 + §20.P8 REFUTED + ADR-0143 SN2-reuse. Set-Cookie attribute defaults
per §4.5 (Secure / HttpOnly / SameSite=Lax / Path=/). Per-category emission
table per §4.1 (categories a/b/c/d). 6-counter byte-exact compile-time
guards per D6. Group 4 round-trip tests (15-20 tests) + Group 9 stat-name
assertion tests pass.

ADR-0181 §Decision + §Consequences anchored. CLOSES §20.P11 envoy-go-strict
departure flag as RATIFIED-AS-ABSENT. Stat surface 86 → 92 names."
```

---

## Task 7: NEW `internal/filter/http/oauth2/tokens.go` + ADR-0182 + `TestAesKeySwap_Concurrent_*` race group

**Files:**
- Create: `internal/filter/http/oauth2/tokens.go` (~150-200 LoC)
- Create: `internal/filter/http/oauth2/tokens_test.go` (~300-400 LoC; Group 3 vector tests per §14.1 + AMEND-3 fall-back semantics tests + `TestAesKeySwap_Concurrent_*` race group per D4)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0182 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 7 entry)

Per AMEND-1 + §20.P5 REFUTED. Cross-task-parallelizable with Tasks 3 + 4 + 6 per D9. Race-test group `TestAesKeySwap_Concurrent_*` per D4 cross-cuts Task 3 (the sdsfile-triggered reload path drives the key swap via `atomic.Pointer[[32]byte]` on `compiledConfig.aesKey`).

**Precondition:** Task 2 complete (the singleton `*httpclient.Client` is unrelated but Task 2 establishes the worktree state).
**Artifact:** `tokens.go` + tests; ADR-0182 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `go test -count=1 ./internal/filter/http/oauth2/... -run 'TestEncryptToken|TestDecryptToken'` clean (20-25 vector tests pass); `go test -race -count=1 ./internal/filter/http/oauth2/... -run 'TestAesKeySwap_Concurrent'` clean (race tests pass); `grep -cE '^## ADR-0182' docs/envoy-go/DECISIONS.md` returns `1` AND §Decision body non-empty.

- [ ] **Step 1: Write failing vector tests** in `internal/filter/http/oauth2/tokens_test.go` per SPEC §14.1 Group 3 + AMEND-1 + AMEND-3:
  - `TestEncryptToken_RoundTrip_ByteExact` — encrypt → decrypt → byte-exact plaintext
  - `TestEncryptToken_RandomIV_DistinctOutputs` — same plaintext + same secret → distinct envelopes (IV is random)
  - `TestEncryptToken_KDF_Sha256TruncatedTo32` — known hmac_secret → SHA-256[:32] key (verify via known-vector AES-256-CBC ciphertext)
  - `TestEncryptToken_PKCS7Padding_BlockBoundary` — plaintext length = 16 (one full block) → output is 16 IV + 16 CT + 16 padding-block = 48 raw bytes
  - `TestEncryptToken_PKCS7Padding_EmptyPlaintext` — plaintext length = 0 → output is 16 IV + 16 padding-only block
  - `TestEncryptToken_Base64URLEnvelope` — output envelope is valid base64URL (raw, no padding)
  - `TestDecryptToken_HappyPath` — valid envelope → plaintext bytes
  - `TestDecryptToken_MalformedBase64_ReturnsCiphertextAsPlaintext_NoError` — invalid base64 → returns input bytes per AMEND-3 (no error, no counter increment per §12 item B6)
  - `TestDecryptToken_BadPadding_ReturnsCiphertextAsPlaintext_NoError` — valid base64 + valid IV but garbage CT → returns input bytes per AMEND-3
  - `TestDecryptToken_TruncatedEnvelope_ReturnsCiphertextAsPlaintext_NoError` — envelope < 16 bytes → returns input bytes per AMEND-3
  - `TestDecryptToken_WrongHmacSecret_GarbageOutputsLikely_NoError` — different hmac_secret → garbage plaintext but no error (downstream HMAC validation catches it)
  - `TestAesKeySwap_Concurrent_DuringEncrypt` — atomic.Pointer key swap during in-flight encrypt: 2 goroutines encrypting + 1 swap goroutine → no torn output
  - `TestAesKeySwap_Concurrent_DuringDecrypt` — atomic.Pointer key swap during in-flight decrypt: 2 goroutines decrypting + 1 swap goroutine → no torn output
  - `TestAesKeySwap_Concurrent_ReadAfterSwapObservesNewKey` — encrypt with key1 → swap to key2 → decrypt with key1 returns fall-back (key mismatch) per AMEND-3

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `internal/filter/http/oauth2/tokens.go`** per the File-structure table row above + SPEC §3.3 + §6.4 + §6.5 + ADR-0182 §Context. The `encryptToken(plaintext, hmacSecret []byte) string` derives key via `sha256.Sum256(hmacSecret)` truncated to first 32 bytes; reads random IV via `crypto/rand.Read(iv)`; pads via PKCS#7 to AES block size; encrypts via `cipher.NewCBCEncrypter(block, iv)`; returns `base64.RawURLEncoding.EncodeToString(iv ‖ ct)`. The `decryptToken(envelope string, hmacSecret []byte) []byte` does the reverse; on ANY error (decode error, length-mismatch, bad padding) returns the original envelope bytes (raw string conversion to []byte) per AMEND-3 fall-back. **Crucially: no error is returned**; the caller's HMAC validation step naturally rejects the bytes if decryption produced garbage.

- [ ] **Step 4: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/oauth2/... -run 'TestEncryptToken|TestDecryptToken'
# Expect: PASS — 20-25 vector tests
go test -race -count=1 ./internal/filter/http/oauth2/... -run 'TestAesKeySwap_Concurrent'
# Expect: PASS — 2-3 race tests under -race
```

- [ ] **Step 5: Author ADR-0182 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft. §Decision body covers: the AES-256-CBC algorithm (per AMEND-1 + §20.P5 REFUTED — swap from BRAINSTORM Q4 AES-GCM to upstream-byte-exact CBC); the SHA-256(hmac_secret)[:32] KDF; the random 16-byte IV per encryption (prepended); PKCS#7 padding; the Base64URL(IV ‖ CT) envelope; the `disable_token_encryption=true` skip-path; the AMEND-3 decryption-failure fall-back (returns ciphertext-as-plaintext; downstream HMAC validation rejects naturally; NO `cookie_decrypt_failure` counter per §20.P11 RATIFIED-AS-ABSENT). §Consequences body covers: filter-local discipline (NOT extracted to a shared package at phase 20 — second-consumer trigger deferral; no other in-tree filter needs AES-CBC at MVP); the b-disposition rationale per ADR-0159 (b)-pattern; AMEND-1 RECORDS the algorithm-swap-from-BRAINSTORM as load-bearing; cross-phase reuse intent (the SHA-256-truncated-to-32 KDF + random-IV-prepended envelope is a reusable pattern for future filter-local AES-CBC needs).

- [ ] **Step 6: Append PROGRESS.md Task 7 entry — RATIFIES SPEC §12 item B6 (AES-256-CBC PKCS#7 padding decrypt-failure semantics) per D17.**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/oauth2/tokens.go \
        internal/filter/http/oauth2/tokens_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 7: oauth2/tokens.go — AES-256-CBC + ADR-0182 + race tests

Per AMEND-1 + §20.P5 REFUTED. NEW filter-local AES-256-CBC token-encryption
helper. SHA-256(hmac_secret)[:32] KDF; random 16-byte IV per encryption
(prepended); PKCS#7 padding; Base64URL(IV ‖ CT) envelope. Decryption-failure
fall-back returns ciphertext-as-plaintext per AMEND-3 (downstream HMAC
validation rejects naturally; no cookie_decrypt_failure counter per §20.P11
RATIFIED-AS-ABSENT). Group 3 vector tests (20-25 tests) +
TestAesKeySwap_Concurrent_* race group (2-3 tests under -race) pass.

ADR-0182 §Decision + §Consequences anchored. SPEC §12 item B6 RATIFIED
per D17."
```

---

## Task 8: Refresh-token rotation continuation in `callback.go` + race tests + ADR-0183

**Files:**
- Modify: `internal/filter/http/oauth2/callback.go` (~+100 LoC; handleRefresh + applyRefreshTokenResponse + deferred Set-Cookie discipline per ADR-0183)
- Modify: `internal/filter/http/oauth2/decode_headers.go` (~+30 LoC; refresh-token rotation dispatch leg)
- Modify: `internal/filter/http/oauth2/oauth_client_test.go` (~+200 LoC; `TestRefreshTokenRotation_Concurrent_*` race group per D4)
- Modify: `docs/envoy-go/DECISIONS.md` (~+100 LoC: ADR-0183 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 8 entry)

Per ADR-0183 + §4.6 + §14.2 race-detector. Depends on Task 5 (dispatcher) + Task 7 (AES helpers). The race-tests at `TestRefreshTokenRotation_Concurrent_*` per D4 + D14 land here.

**Precondition:** Task 5 + Task 7 complete.
**Artifact:** Refresh-token rotation flow wired end-to-end with race-safe deferred Set-Cookie discipline; ADR-0183 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `go test -count=1 ./internal/filter/http/oauth2/... -run 'TestHandleRefresh|TestApplyRefreshTokenResponse'` clean (refresh-flow tests pass); `go test -race -count=1 ./internal/filter/http/oauth2/... -run 'TestRefreshTokenRotation_Concurrent'` clean (race tests pass); `grep -cE '^## ADR-0183' docs/envoy-go/DECISIONS.md` returns `1`.

- [ ] **Step 1: Write failing tests** for refresh-flow:
  - `TestHandleRefresh_SuccessfulPost_ContinuesDecodingWithDeferredSetCookie` — happy path: refresh POST 2xx → CONTINUE + new Set-Cookie envelope emitted on response per §4.6
  - `TestHandleRefresh_FailedPost_Emits_302_Challenge_With_oauth_refreshtoken_failure_Counter` — refresh POST 5xx → category (a) 302 challenge + `oauth_refreshtoken_failure++` per §4.6 (NOT also `oauth_failure` per AMEND-3 + §4.6)
  - `TestHandleRefresh_4xxFailure_Emits_302_Challenge_With_oauth_refreshtoken_failure_Counter` — refresh POST 4xx → category (a) 302 challenge + `oauth_refreshtoken_failure++`
  - `TestRefreshTokenRotation_Concurrent_2RequestsSameCookies_BothPost` — 2 concurrent in-flight requests with same expired BearerToken + valid RefreshToken → both POST refresh independently (no per-stream serialization per D14 + ADR-0183) → both succeed → both emit new envelope (latest Set-Cookie wins via deferred Set-Cookie)
  - `TestRefreshTokenRotation_Concurrent_CounterIncrementOnePerEvent` — N concurrent rotations → `oauth_refreshtoken_success` counter == N
  - `TestRefreshTokenRotation_Concurrent_MixedSuccessAndFailure` — concurrent rotations mix 2xx + 5xx → respective counters increment correctly

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Modify `internal/filter/http/oauth2/callback.go`** — add `handleRefresh(headers)` that issues the async POST per ADR-0183 + §6.8 + `applyRefreshTokenResponse(resp)` that handles the response (success → deferred Set-Cookie + ContinueDecoding; failure → category (a) 302 + `oauth_refreshtoken_failure++`). The dispatch goroutine parks the decode goroutine on the phase-09 async-resume primitive while the POST is in flight. No per-stream serialization per D14.

- [ ] **Step 4: Modify `internal/filter/http/oauth2/decode_headers.go`** — wire the refresh-token rotation dispatch leg in `handleCookieValidate`: when envelope valid + BearerToken expired + RefreshToken present + valid → dispatch to `handleRefresh` (rather than `handleUnauthenticated`).

- [ ] **Step 5: Run tests to verify they pass.**

- [ ] **Step 6: Author ADR-0183 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft. §Decision body covers: the refresh-token rotation flow per §6.8; the deferred Set-Cookie discipline (Set-Cookie emitted on the success-leg response BEFORE ContinueDecoding); concurrent-request race-vs-rotation per D14 (no per-stream serialization; each in-flight request POSTs independently; latest Set-Cookie wins); the counter increment matrix per §4.6 (refresh-failure → `oauth_refreshtoken_failure` only, NOT also `oauth_failure`). §Consequences body covers: the envoy-go-strict simplification (no shared cache across concurrent rotations; the natural last-writer-wins of the cookie envelope handles the race benignly); the race-detector coverage at `TestRefreshTokenRotation_Concurrent_*` per D4; cross-phase reuse intent (the deferred Set-Cookie + no-per-stream-serialization pattern is reusable for any future cookie-refresh-on-success flow).

- [ ] **Step 7: Append PROGRESS.md Task 8 entry.**

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/oauth2/callback.go \
        internal/filter/http/oauth2/decode_headers.go \
        internal/filter/http/oauth2/oauth_client_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 8: refresh-token rotation in callback.go + ADR-0183 + race tests

Per ADR-0183 + §4.6 + §14.2. handleRefresh + applyRefreshTokenResponse wire
the deferred Set-Cookie discipline. No per-stream serialization per D14 —
each in-flight request POSTs the refresh independently; latest Set-Cookie
wins. Counter increment one-per-event: oauth_refreshtoken_success on 2xx;
oauth_refreshtoken_failure on non-2xx (NOT also oauth_failure per AMEND-3 +
§4.6). TestRefreshTokenRotation_Concurrent_* race group (3-4 tests under
-race) passes. ADR-0183 §Decision + §Consequences anchored."
```

---

## Task 9: NEW `internal/filter/http/oauth2/signout.go` + ADR-0184

**Files:**
- Create: `internal/filter/http/oauth2/signout.go` (~80-120 LoC)
- Modify: `internal/filter/http/oauth2/oauth2_test.go` (~+100 LoC; sign-out tests)
- Modify: `docs/envoy-go/DECISIONS.md` (~+80 LoC: ADR-0184 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 9 entry)

Per ADR-0184 + §4.1 category (c) + §4.5 + §6.9. Depends on Task 5 (dispatcher) + Task 6 (cookie envelope writer).

**Precondition:** Task 5 + Task 6 complete.
**Artifact:** `signout.go` with `handleSignout`; ADR-0184 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `go test -count=1 ./internal/filter/http/oauth2/... -run 'TestHandleSignout'` clean (sign-out tests pass); `grep -cE '^## ADR-0184' docs/envoy-go/DECISIONS.md` returns `1`.

- [ ] **Step 1: Write failing tests**:
  - `TestHandleSignout_EmitsCategory_C_302_With_MaxAge0_AllCookies` — signout_path matched → category (c) 302 + 5 Set-Cookie headers each with Max-Age=0
  - `TestHandleSignout_DenyRedirectMatcher_Honored_When_Match` — request matches `deny_redirect_matcher` → Location header per matcher; no counter increment per §4.6 + AMEND-4 (no `signout_completed` counter)
  - `TestHandleSignout_NoSeparateCounter_For_Signout` — sign-out emission does NOT increment any of the 6 counters

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `internal/filter/http/oauth2/signout.go`** per the File-structure table row above + SPEC §6.9 + ADR-0184 §Context. The `handleSignout(headers)` emits category (c) 302 + the 5-cookie Max-Age=0 envelope per §4.5 + the `deny_redirect_matcher` integration for the Location header. NO counter increment per AMEND-4 + S5 (the 302 emission IS the sign-out completion event).

- [ ] **Step 4: Run tests to verify they pass.**

- [ ] **Step 5: Author ADR-0184 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft. §Decision body covers: the signout_path handling at dispatch priority 1 per §6.3; the full envelope clearing (Max-Age=0 for all 5 cookies); the `deny_redirect_matcher` integration; the category (c) 302 emission per §4.1. §Consequences body covers: NO separate `signout_completed` counter per AMEND-4 + S5 + ADR-0181 (sign-out completion IS the 302 emission; operator observability via downstream access-logs records the 302 emission with the signout_path-match URL; no per-filter counter needed); phase 20 mirrors upstream wire-compat fully.

- [ ] **Step 6: Append PROGRESS.md Task 9 entry.**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/oauth2/signout.go \
        internal/filter/http/oauth2/oauth2_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 9: oauth2/signout.go + ADR-0184 (sign-out flow)

Per ADR-0184 + §4.1 category (c) + §4.5 + §6.9. handleSignout emits category
(c) 302 with Max-Age=0 envelope clearing for all 5 cookies + deny_redirect_
matcher integration for the Location header. NO separate signout_completed
counter per AMEND-4 + S5 (the 302 emission IS the sign-out completion
event). Sign-out tests pass. ADR-0184 §Decision + §Consequences anchored."
```

---

## Task 10: NEW `internal/filter/http/oauth2/oauth_client.go` + `urlEncode` helper + ADR-0185

**Files:**
- Create: `internal/filter/http/oauth2/oauth_client.go` (~220-300 LoC)
- Create: `internal/filter/http/oauth2/oauth_client_test.go` if not yet created (~250-350 LoC; Group 5 template byte-exact tests + `urlEncode` vector tests per §12 item A5 + D16 A5)
- Modify: `docs/envoy-go/DECISIONS.md` (~+100 LoC: ADR-0185 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 10 entry)

Per ADR-0185 + AMEND-5 + §20.P10 RATIFIED. Depends on Task 2 (`*httpclient.Client` available) + Task 5 (callback dispatch wiring).

**Precondition:** Task 2 + Task 5 complete.
**Artifact:** `oauth_client.go` with `postTokenEndpoint` + `buildTokenRequestBody` + `urlEncode`; ADR-0185 §Decision + §Consequences body anchored.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `go test -count=1 ./internal/filter/http/oauth2/... -run 'TestBuildTokenRequestBody|TestUrlEncode'` clean (15-20 template tests + ~5 urlEncode vector tests pass); `grep -cE '^## ADR-0185' docs/envoy-go/DECISIONS.md` returns `1`.

- [ ] **Step 1: Write failing tests**:
  - `TestBuildTokenRequestBody_AuthCode_4FieldByteExact` — `grantType=authorization_code`, params={code, client_id, client_secret, redirect_uri} → byte-exact `grant_type=authorization_code&code=<urlEncoded>&client_id=<urlEncoded>&client_secret=<urlEncoded>&redirect_uri=<urlEncoded>` per AMEND-5 + §20.P10 (matches upstream `oauth_client.cc` source)
  - `TestBuildTokenRequestBody_RefreshToken_3FieldByteExact` — `grantType=refresh_token`, params={refresh_token, client_id, client_secret} → byte-exact 3-field template
  - `TestBuildTokenRequestBody_AuthCode_PKCEAbsent_NoCodeVerifier` — assertion that the 4-field auth-code template does NOT contain `code_verifier` (gated per S3 + §2.1)
  - `TestUrlEncode_PercentEncodes_ColonSlashEqualsAmpersandQuestion` — `:/=&?` chars all percent-encoded
  - `TestUrlEncode_SpacesAsPercent20` — spaces as `%20` (NOT `+`)
  - `TestUrlEncode_StdlibPathEscapeDivergence` — assert byte-exact divergence from `url.PathEscape` for known input
  - `TestUrlEncode_NonAsciiBytes_UTF8Escaped` — non-ASCII inputs percent-encoded per stdlib UTF-8 escaping per §12 item A5 + D16 A5

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `internal/filter/http/oauth2/oauth_client.go`** per the File-structure table row above + SPEC §6.7 + ADR-0185 §Context. `postTokenEndpoint(ctx, body)` constructs the `http.Request` (POST + Content-Type `application/x-www-form-urlencoded`) and invokes `cc.httpClient.Do(req)`. `buildTokenRequestBody` switches on grantType + emits the byte-exact template via the `urlEncode` helper. `urlEncode(value)` iterates the input bytes, percent-encoding `:/=&?` PLUS spaces (as `%20`); other chars per stdlib character-classification.

- [ ] **Step 4: Run tests to verify they pass.**

- [ ] **Step 5: Author ADR-0185 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft. §Decision body covers: the 4-field auth-code template byte-exact (MVP); the 3-field refresh-token template byte-exact; the PKCE-gated 5th field for future per S3; spaces as `%20`; the PercentEncoding charset includes `:/=&?` per §20.P10; the NEW `urlEncode` custom helper (NOT stdlib `url.PathEscape` per the wire-divergence assertion test). §Consequences body covers: AMEND-5 RECORDS the empirical-scrape disposition; phase-20 SPEC §12 item A5 RATIFIES-AT-IMPL via the `urlEncode` vector tests + the fixture-0024 scenario (a) token_endpoint POST body capture; future PKCE-enabling phase consumes the 5th-field gating.

- [ ] **Step 6: Append PROGRESS.md Task 10 entry — RATIFIES SPEC §12 item A5 (urlEncode charset for non-ASCII bytes) per D16.**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/oauth2/oauth_client.go \
        internal/filter/http/oauth2/oauth_client_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 10: oauth2/oauth_client.go + urlEncode + ADR-0185

Per AMEND-5 + §20.P10 RATIFIED. NEW oauth_client.go carrying
postTokenEndpoint via *httpclient.Client.Do (ADR-0177 consumer) +
buildTokenRequestBody 4-field auth-code template (MVP) + 3-field refresh-
token template byte-exact per upstream oauth_client.cc + NEW urlEncode custom
helper (percent-encodes :/=&? + spaces as %20). PKCE-gated 5th field for
future per S3. Group 5 template tests (15-20 tests) + urlEncode vector tests
pass.

ADR-0185 §Decision + §Consequences anchored. SPEC §12 item A5 RATIFIED
per D16."
```

---

## Task 11: Full filter integration in `oauth2.go` + `compiled_config.go` + boot-registration + ADR final-state alignment

**Files:**
- Modify: `internal/filter/http/oauth2/oauth2.go` (~+120 LoC; full `New` factory body + filter type wiring + filterStats hookup; replaces any stub from Task 4-9 with the full integration)
- Create: `internal/filter/http/oauth2/compiled_config.go` (~280-380 LoC; `compiledConfig` + `buildCompiledConfig` + PARSE-REJECT path)
- Modify: `cmd/envoy-go/main.go` (~+1 line; `httpReg.Register(oauth2.TypeURL, oauth2.New)` insertion + `RegisterPerRouteValidator` registration; pass `httpClient` via FactoryCtx if not already)
- Modify: `internal/filter/http/oauth2/oauth2_test.go` (~+200 LoC; Group 1 PARSE-REJECT tests — ~30-40 rows covering all PARSE-REJECT cases per D2)
- Modify: `docs/envoy-go/DECISIONS.md` (final-state alignment for all 9 NEW ADRs + 2 IN-PLACE AMENDMENTs; cross-references intact per SPEC §15 item 15)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 11 entry)

This task wires everything from Tasks 2-10 into a fully-functional `api.HTTPFilterFactory` from `New()`. Boot-registration inserts at line 135 alphabetical between `localratelimit` and `rbac` per D19. The PARSE-REJECT path with byte-stable error messages per D2 lands here.

**Precondition:** Tasks 2-10 complete.
**Artifact:** Fully-functional oauth2 filter; boot-registration alphabetical; PARSE-REJECT path complete with byte-stable wording; ADR cross-references intact.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/oauth2/...` ALL clean (full unit-test surface ~95-130 tests pass); `grep -nE 'httpReg.Register\(oauth2.TypeURL' cmd/envoy-go/main.go` returns the boot-registration line at line 135; PARSE-REJECT byte-stable wording matches D2 reference strings; `grep -cE '^## ADR-0177' through `grep -cE '^## ADR-0185' docs/envoy-go/DECISIONS.md` each return `1` AND §Decision + §Consequences bodies all non-empty.

- [ ] **Step 1: Author `internal/filter/http/oauth2/compiled_config.go`** per the File-structure table row above + SPEC §6.2 + §6.10. Includes the full `compiledConfig` struct per SPEC §6.2 + the `buildCompiledConfig(message proto.Message) (*compiledConfig, error)` body covering all PARSE-REJECT cases per D2: (1) `Unmarshal` the proto; (2) PARSE-REJECT non-filesystem ConfigSource arms + deprecated path field + `secret_file` indirect arm; (3) PARSE-REJECT BASIC_AUTH set; (4) PARSE-REJECT PKCE fields set; (5) Validate token_endpoint URL via `url.Parse`; (6) Validate authorization_endpoint non-empty; (7) Validate redirect_uri non-empty; (8) Validate client_id non-empty; (9) Construct `*sdsfile.Watcher` for hmac_secret + client_secret (calls `New` then `Start`); (10) Validate `disable_token_encryption=false` AND `hmac_secret` non-empty; (11) Construct `aesKey` via `SHA-256(hmac_secret)[:32]` and store via `atomic.Pointer[[32]byte].Store`; (12) Capture `forward_bearer_token` per AMEND-6 C3; (13) Capture `preserve_authorization_header` + `use_refresh_token` + `default_expires_in` + `default_refresh_token_expires_in` + `auth_scopes` + `resources` + `csrf_token_expires_in`; (14) Compile matchers (redirect_path_matcher + signout_path + pass_through_matcher + deny_redirect_matcher); (15) Construct `*filterStats` via `newFilterStats(ctx, statPrefix)`; (16) Wire `httpClient` from FactoryCtx; (17) Return the full `*compiledConfig`.

- [ ] **Step 2: Modify `internal/filter/http/oauth2/oauth2.go`** — replace the stub `New` with the full factory body: `New(message proto.Message) (api.HTTPFilterFactory, error) { cc, err := buildCompiledConfig(message); if err != nil { return nil, err }; return func() api.HTTPFilter { return &filter{cc: cc, parentCtx: ...} }, nil }`. Wire compile-time interface assertions per SPEC §6.12 (`var _ api.StreamFilter = (*filter)(nil)` + `var _ api.StreamDecoderFilter = (*filter)(nil)`).

- [ ] **Step 3: Modify `cmd/envoy-go/main.go`** — add the import `oauth2 "github.com/esalaine/envoy-go/internal/filter/http/oauth2"` (alphabetical); add `httpReg.Register(oauth2.TypeURL, oauth2.New)` at line 135 (between `localratelimit` at line 134 and `rbac` which shifts to line 136) per ADR-0100 §2.2 + D19; ensure `oauth2.RegisterPerRouteValidator(perRouteReg)` is called for the HCM-parse-time PARSE-REJECT hook per §5.2 + D2. Pass `httpClient` (from Task 2a's singleton) into the oauth2 factory's FactoryCtx if not already.

- [ ] **Step 4: Add Group 1 PARSE-REJECT tests** in `internal/filter/http/oauth2/oauth2_test.go` covering ~30-40 PARSE-REJECT rows per D2 reference strings. Each row asserts `err != nil && err.Error() == "<expected D2 string>"`.

- [ ] **Step 5: Verify all unit tests + build + vet + lint clean**

```bash
go test -count=1 ./internal/filter/http/oauth2/...
# Expect: PASS — full ~95-130 test surface
go test -count=1 ./internal/httpclient/...
go test -count=1 ./internal/sdsfile/...
# Expect: PASS — Tasks 2 + 3 surfaces still GREEN
go build ./... && go vet ./... && golangci-lint run
# Expect: all clean
```

- [ ] **Step 6: Final-state ADR alignment in DECISIONS.md** — verify all 9 NEW ADR §Decision + §Consequences bodies are present + non-empty + cross-references intact per SPEC §15 item 15; verify 2 IN-PLACE AMENDMENT bodies are present at ADR-0150 + ADR-0159 + the ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph is present.

- [ ] **Step 7: Append PROGRESS.md Task 11 entry — record D11 hypothesis (NO additional ADR fires at IMPL) status: HOLDING as of Task 11.**

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/oauth2/oauth2.go \
        internal/filter/http/oauth2/compiled_config.go \
        internal/filter/http/oauth2/oauth2_test.go \
        cmd/envoy-go/main.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 11: oauth2 full filter integration + boot-registration + ADR alignment

Wires Tasks 2-10 into a fully-functional api.HTTPFilterFactory. NEW
compiled_config.go with buildCompiledConfig + PARSE-REJECT path per D2
byte-stable wording. Boot-registration alphabetical at line 135 between
localratelimit and rbac per D19 + ADR-0100 §2.2. RegisterPerRouteValidator
hooked for HCM-parse-time PARSE-REJECT per §5.2. Group 1 PARSE-REJECT tests
(30-40 rows) pass. All 9 NEW ADR §Decision + §Consequences bodies +
2 IN-PLACE AMENDMENT bodies aligned. D11 hypothesis (NO additional ADR at
IMPL) HOLDING."
```

---

## Task 12: Differential fixture `0024-http-oauth2` + NEW `test/helpers/oauthbackend/` + 26th fuzzer + RATIFIED-PENDING-IMPL-TIME pin closures

**Files:**
- Create: `test/helpers/oauthbackend/doc.go` (~25 LoC)
- Create: `test/helpers/oauthbackend/oauthbackend.go` (~250-350 LoC)
- Create: `test/helpers/oauthbackend/oauthbackend_test.go` (~120-180 LoC)
- Create: `test/fixtures/0024-http-oauth2/envoy.yaml` (~250-350 LoC)
- Create: `test/fixtures/0024-http-oauth2/envoy-go.yaml` (~250-350 LoC)
- Create: `test/fixtures/0024-http-oauth2/expectations.yaml` (~120 LoC)
- Create: `test/fixtures/0024-http-oauth2/README.md` (~180 LoC)
- Create: `test/fixtures/0024-http-oauth2/inputs/driver.go` (~500-700 LoC)
- Create: `test/fixtures/0024-http-oauth2/secrets/hmac.json` (~30 LoC; per D3)
- Create: `test/fixtures/0024-http-oauth2/secrets/client_secret.json` (~30 LoC; per D3)
- Create: `internal/filter/http/oauth2/fuzz_test.go` (~100 LoC; 26th fuzzer per D7)
- Create: `internal/filter/http/oauth2/testdata/fuzz/FuzzOAuth2ConfigParse/` (corpus seeds per D7; ~60 seeds)
- Modify: `test/differential/fixture/fixture.go` (+1 enum value `HTTPOAuth2 = 20`)
- Modify: `test/differential/runner_test.go` (+blank import + switch-case)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 12 entry — RATIFIES SPEC §12 items A1-A5 + B7 per D16 + D17)

Per SPEC §7 + §14.3 + §14.4. Lands the 9-wire-expectation differential fixture + the NEW `test/helpers/oauthbackend/` helper + the 26th fuzzer. CLOSES SPEC §12 items A1-A5 + B7 per D16 + D17.

**Precondition:** Task 11 complete (full filter integration GREEN).
**Artifact:** Fixture 0024 + helper package + 26th fuzzer + 9-scenario byte-exact GREEN against reference Envoy v1.37.2.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `go test -count=1 ./test/helpers/oauthbackend/...` clean (helper tests pass); `go test -count=1 ./test/differential/ -run 'Test.*0024'` clean (9 wire-level expectations GREEN); `go test -fuzz=FuzzOAuth2ConfigParse -fuzztime=30s ./internal/filter/http/oauth2/` clean (no panics); SPEC §12 items A1-A5 + B7 + B6 + C8 status RATIFIED at PROGRESS log.

- [ ] **Step 1: Author `test/helpers/oauthbackend/`** — `doc.go` + `oauthbackend.go` + `oauthbackend_test.go` per SPEC §7.3 + the File-structure table rows above. The httptest.Server hosts the mock authorization_endpoint + token_endpoint per scenario script.

- [ ] **Step 2: Author the SDS Secret files** per D3:
  - `test/fixtures/0024-http-oauth2/secrets/hmac.json` — Secret-proto JSON with `generic_secret.inline_string` populated (a fixed-byte HMAC secret for fixture stability)
  - `test/fixtures/0024-http-oauth2/secrets/client_secret.json` — Secret-proto JSON with `generic_secret.inline_string` populated (a fixed-byte client secret)

- [ ] **Step 3: Author `test/fixtures/0024-http-oauth2/envoy.yaml`** — reference Envoy config with 3-listener topology per D10 (l_test_a default-encryption / l_test_b `disable_token_encryption=true` / l_test_c `forward_bearer_token=true`); each listener wires the `envoy.filters.http.oauth2` filter with the appropriate `OAuth2Config`; SDS Secret files referenced via `core.ConfigSource.PathConfigSource` (oneof arm field 8) per §20.P6. Routes to upstream + mock token_endpoint server (oauthbackend).

- [ ] **Step 4: Author `test/fixtures/0024-http-oauth2/envoy-go.yaml`** — mirror the upstream config for envoy-go side; SDS Secret files at the same paths.

- [ ] **Step 5: Author `test/fixtures/0024-http-oauth2/expectations.yaml`** — per SPEC §7.1 9 wire-level expectations across 8 scenario directories (a + b1 + b2 + c + d + e + f + g + h + i): for each scenario, the expected wire-shape (status code + Set-Cookie envelope + Location header + body bytes) for both reference Envoy + envoy-go, asserted byte-exact-pinned.

- [ ] **Step 6: Author `test/fixtures/0024-http-oauth2/inputs/driver.go`** — test-driver invoking the 9-scenario matrix against both reference Envoy + envoy-go; consumes `test/helpers/oauthbackend/` for the mock OAuth server; asserts byte-exact wire shape per `expectations.yaml`.

- [ ] **Step 7: Author `test/fixtures/0024-http-oauth2/README.md`** — scenario matrix narrative; cross-references SPEC §7.1; documents the 3-listener topology per D10.

- [ ] **Step 8: Modify `test/differential/fixture/fixture.go`** — add the `HTTPOAuth2 BackendKind = 20` enum value (after `HTTPExtProcGRPC = 19`).

- [ ] **Step 9: Modify `test/differential/runner_test.go`** — add the blank import + switch-case for `HTTPOAuth2`.

- [ ] **Step 10: Author `internal/filter/http/oauth2/fuzz_test.go`** per D7 — 26th fuzzer `FuzzOAuth2ConfigParse` covering buildCompiledConfig + decode-path parse + cookie-parse + hmac-validate + decrypt-token + buildTokenRequestBody. Must-never-panic.

- [ ] **Step 11: Author corpus seeds** at `internal/filter/http/oauth2/testdata/fuzz/FuzzOAuth2ConfigParse/` per D7 — ~60 seeds covering OAuth2Config + OAuth2Credentials + CookieNames + SdsSecretConfig variants + matcher engine variants.

- [ ] **Step 12: Run the differential fixture against both stacks**

```bash
go test -count=1 ./test/differential/ -run 'Test.*0024'
# Expect: PASS — 9 wire-level expectations byte-exact across both stacks
go test -count=1 ./test/helpers/oauthbackend/...
# Expect: PASS — helper tests
go test -fuzz=FuzzOAuth2ConfigParse -fuzztime=30s ./internal/filter/http/oauth2/
# Expect: clean — no panics at 30s per seed
```

- [ ] **Step 13: Append PROGRESS.md Task 12 entry** — RATIFIES SPEC §12 items A1-A5 + B7 + B6 + C8 (with cross-package matrix from D5):
  - A1 (401 Content-Type + no-trailing-newline) — RATIFIED at fixture-0024 scenarios (f) + (h)
  - A2 (Set-Cookie attribute byte-exact upstream defaults) — RATIFIED at scenario (a)
  - A3 (state-cookie payload byte-exact shape + OauthExpires format) — RATIFIED at scenario (a)
  - A4 (HCM SendLocalReply Content-Type default) — RATIFIED at scenarios (f) + (h) Content-Type assertion
  - A5 (urlEncode charset for non-ASCII bytes) — RATIFIED at scenario (a) token_endpoint POST body capture
  - B6 (AES-256-CBC PKCS#7 padding decrypt-failure semantics) — RATIFIED at scenario (b2)
  - B7 (fsnotify event-debounce window) — RATIFIED at Task 3 unit-tests + Task 7 race tests
  - C8 (cross-package regression matrix) — RATIFIED at Task 2b/2c regression checks + this Task's full 25-fixture run via D5 cmd shape

- [ ] **Step 14: Commit**

```bash
git add test/helpers/oauthbackend/ \
        test/fixtures/0024-http-oauth2/ \
        test/differential/fixture/fixture.go \
        test/differential/runner_test.go \
        internal/filter/http/oauth2/fuzz_test.go \
        internal/filter/http/oauth2/testdata/ \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 12: fixture 0024-http-oauth2 + oauthbackend helper + 26th fuzzer + pin closures

Lands the 9-wire-expectation differential fixture (3-listener topology per
D10) + NEW test/helpers/oauthbackend/ in-process OAuth-server mock (~250-350
LoC) + 26th fuzzer FuzzOAuth2ConfigParse (~60 corpus seeds per D7;
must-never-panic; clean at 30s per seed).

Cross-package regression matrix GREEN: 24 pre-existing fixtures + new 0024
all GREEN at Gate D regression run.

CLOSES SPEC §12 items A1-A5 + B6 + B7 + C8 RATIFIED-PENDING-IMPL-TIME at
PROGRESS log per D16 + D17 + D18."
```

---

## Task 13: BEHAVIOR_CONTRACT.md 10-edit bundle per SPEC §13

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (~+400-500 LoC; 10-edit bundle per SPEC §13.A-§13.F)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 13 entry)

Per SPEC §13.F — all 10 edits land at the SAME IMPL commit per ADR-0052; none mutate pre-phase-20 paragraphs (in-place-by-append discipline).

**Precondition:** Task 12 complete (some §13 paragraphs reference the fixture-0024 wire-shape closures from Task 12).
**Artifact:** BEHAVIOR_CONTRACT.md 10-edit bundle landed atomically.
**Acceptance:** `grep -cE '^### envoy.filters.http.oauth2' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns `1` (NEW subsection); 6-counter stat-table rows present; ADR-0159 CLOSURE-AT-PHASE-20 paragraph present; ADR-0150 REFACTORED-AT-PHASE-20 paragraph present; 2 envoy-go-strict departure records present; NEW `### Phase 20 forward-pointer notes` subsection present.

- [ ] **Step 1: §13.A.1 — NEW `### envoy.filters.http.oauth2` subsection** inserted after `### envoy.filters.http.ext_proc`. ~250-350 LoC subsections per SPEC §13.A.1 (filter scope + populated-vs-deferred field map + sign-in/refresh/sign-out/pass-through wire shapes + cookie envelope discipline + token_endpoint POST body template + stat-name mapping + per-route discipline + 2 envoy-go-strict departures).

- [ ] **Step 2: §13.B.2 — Stat-name mapping 86-name → 92-name table extension** with 6 new oauth2 counter rows; table caption updated.

- [ ] **Step 3: §13.B.3 — NEW `## HTTP outbound framework primitive (per phase 20 ADR-0177)` subsection** after the existing JWKS framework primitive subsection. Documents `internal/httpclient/` + 3 consumers.

- [ ] **Step 4: §13.B.4 — NEW `## Filesystem-SDS framework primitive (per phase 20 ADR-0178)` subsection** after item B3. Documents `internal/sdsfile/` + MVP consumer + cross-phase reuse forward-pointer.

- [ ] **Step 5: §13.B.5 — CLOSURE-AT-PHASE-20 paragraph appended to `## HTTP outbound auth-check framework note (per phase 18.1 ADR-0159)`** documenting the third-consumer-trigger closure (FIRST §9 family-row to close prior-phase load-bearing forward-pointer).

- [ ] **Step 6: §13.B.6 — Per-route canonical patterns cross-reference table update** — caption "updated through phase 19.2" → "updated through phase 20"; phase-20 cross-reference paragraph added documenting the REUSE-by-absence (THIRD CONSECUTIVE §9 row).

- [ ] **Step 7: §13.C.7 + §13.C.8 — NEW envoy-go-strict departure records** for `token_endpoint POST non-2xx retry-eligible → 302 challenge` simplification per §4.7 + AMEND-3 + `POST callback method PARSE-REJECT` per §20.P3 + §2.14.

- [ ] **Step 8: §13.D.9 — NEW `### Phase 20 forward-pointer notes` subsection** placed immediately after `### Phase 19.2 forward-pointer notes`. Documents 6 forward-pointers per SPEC §13.D.9.

- [ ] **Step 9: §13.E.10 — REFACTORED-AT-PHASE-20 paragraph appended to `## JWKS framework primitive (per phase 17 ADR-0150)`** documenting the `internal/jwks/Fetcher` refactor.

- [ ] **Step 10: Verify edit-bundle completeness** — `grep -cE '^### envoy.filters.http.oauth2' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns `1`; all 10 edit items present.

- [ ] **Step 11: Append PROGRESS.md Task 13 entry.**

- [ ] **Step 12: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md
git commit -m "phase 20 Task 13: BEHAVIOR_CONTRACT.md 10-edit bundle per SPEC §13

Lands all 10 edits atomically per ADR-0052 (in-place-by-append): NEW
envoy.filters.http.oauth2 subsection (250-350 LoC); NEW HTTP outbound
framework primitive (per phase 20 ADR-0177) umbrella; NEW Filesystem-SDS
framework primitive (per phase 20 ADR-0178) umbrella; stat-table 86 to 92
extension; ADR-0159 CLOSURE-AT-PHASE-20 paragraph; ADR-0150 REFACTORED-AT-
PHASE-20 paragraph; per-route canonical patterns cross-reference update
(THIRD CONSECUTIVE phase-9 row); 2 envoy-go-strict departure records
(token_endpoint non-2xx to 302; POST callback PARSE-REJECT); NEW Phase 20
forward-pointer notes subsection."
```

---

## Task 14: Six-gate phase-done verification + cross-package regression matrix + STATE.md re-advance + ROADMAP row 20 flip + REVIEW.md

**Files:**
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place to post-phase-20 state per BOOTSTRAP §4.1 invariant 1)
- Modify: `docs/envoy-go/ROADMAP.md` (row 20 status flips `in-progress → done` + per-cell IMPL-done annotation)
- Create: `docs/envoy-go/phases/20-http-filter-oauth2/REVIEW.md` (~300 LoC; per `superpowers:requesting-code-review`)
- Append: `docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md` (Task 14 entry — final task; 6-gate outputs captured verbatim)

Per SPEC §7.5 + §14.5 + §15. The 6 phase-done gates A/B/C/D/E/F MUST be GREEN for the row-20 status flip. The 18-item SPEC §15 acceptance checklist is the reviewer's authoritative validation per `superpowers:requesting-code-review` invocation.

**Precondition:** Tasks 1-13 complete.
**Artifact:** All 6 gates green; STATE.md + ROADMAP advanced; REVIEW.md authored; phase-done ready for squash-merge.
**Acceptance:** All 6 gates GREEN (captured verbatim in PROGRESS.md Task 14 entry); SPEC §15 all 18 items checked + green; STATE.md updated with post-phase-20 state (`lifecycle-state: phase 20 IMPL done`; `next-skill: superpowers:brainstorming` for next-phase; `last-commit: <TBD — SHA-fill follow-up after squash-merge>`; `next-free ADR: ADR-0186`); ROADMAP row 20 status `done`; REVIEW.md authored.

- [ ] **Step 1: Gate A — build** — `go build ./...` clean. Capture output verbatim in PROGRESS.md.

- [ ] **Step 2: Gate B — vet + lint** — `go vet ./...` + `golangci-lint run` clean; no new suppressions. Capture output verbatim.

- [ ] **Step 3: Gate C — race** — `go test -race -count=1 ./...` clean; zero data-race violations across all packages including the new `TestWatcher_DebounceRace_*` + `TestRefreshTokenRotation_Concurrent_*` + `TestAesKeySwap_Concurrent_*` race groups per D4. Capture output verbatim.

- [ ] **Step 4: Gate D — differential + cross-package regression matrix per D5** — `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-4])'` clean (all 25 fixtures GREEN: 24 pre-existing + new 0024). Per SPEC §12 item C8: fixture-0019 (jwt_authn) + fixture-0020 (ext_authz HTTP-mode) GREEN post-refactor (RATIFIED at Task 2b + 2c regression checks); fixture-0021 (ext_authz gRPC-mode) untouched. Capture output verbatim.

- [ ] **Step 5: Gate E — fuzz** — `go test -fuzz=FuzzOAuth2ConfigParse -fuzztime=30s ./internal/filter/http/oauth2/` clean (no panics). 25 pre-existing fuzzers re-run clean at 30s per seed via `go test -fuzz=Fuzz -fuzztime=30s ./...` or per-package. Capture output verbatim.

- [ ] **Step 6: Gate F — h2spec** — `make test-h2spec` 53/53 PASS at ADR-0051 pin. Capture output verbatim.

- [ ] **Step 7: Update STATE.md** to post-phase-20 state per BOOTSTRAP §4.1 invariant 1:
  - `active-phase`: next-phase identifier (TBD by user; placeholder `"to-be-determined-at-next-session"`)
  - `lifecycle-state`: `phase 20 IMPL done; awaiting next-phase identification`
  - `next-skill`: `superpowers:brainstorming` (the next-phase initial skill; OR per the user's next-phase direction)
  - `last-commit`: `<TBD — SHA-fill follow-up after squash-merge>` placeholder
  - `last-updated`: today's date
  - `next-free ADR`: `ADR-0186` (UNCHANGED — D11 hypothesis HOLDS; no additional ADR consumed at IMPL)
  - Verbose summary captures: 14 tasks landed; 9 NEW ADRs anchored (ADR-0177..ADR-0185); 2 IN-PLACE AMENDMENT bodies (ADR-0150 + ADR-0159); ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph (FIRST §9 family-row to close prior-phase forward-pointer); 26th fuzzer; 25/25 differential fixtures green; all 6 phase-done gates green; SPEC §15 18 items all GREEN.

- [ ] **Step 8: Update ROADMAP.md row 20** — status flips `in-progress → done`; per-cell IMPL-done annotation appended documenting the 14-task IMPL landing + the 6-gate green outputs + the ADR-0159 closure milestone + the SPEC §15 18-item acceptance.

- [ ] **Step 9: Author REVIEW.md** per `superpowers:requesting-code-review` — 300 LoC reviewer artifact covering: the 6-gate outputs verbatim; the SPEC §15 18-item checklist verification with cite-to-PROGRESS-entry per item; the D1-D19 planner-decision-disposition record (which decisions HELD, which were AMENDED at IMPL); the next-phase handoff state.

- [ ] **Step 10: Append final PROGRESS.md Task 14 entry** with all 6 gate outputs verbatim + the SPEC §15 18-item closure checklist + the D11 final hypothesis status (HOLDING — ADR-0186 stays unconsumed at phase-20 IMPL phase-done).

- [ ] **Step 11: Verify nothing left uncommitted**

```bash
git status --porcelain
# Expect: empty
```

- [ ] **Step 12: Commit (Task 14 final IMPL-worktree commit)**

```bash
git add docs/envoy-go/STATE.md \
        docs/envoy-go/ROADMAP.md \
        docs/envoy-go/phases/20-http-filter-oauth2/PROGRESS.md \
        docs/envoy-go/phases/20-http-filter-oauth2/REVIEW.md
git commit -m "phase 20 Task 14: 6-gate phase-done verification + STATE/ROADMAP advance + REVIEW

All 6 phase-done gates GREEN: A build / B vet+lint / C race / D differential
(25/25 fixtures incl. new 0024) / E fuzz (26 fuzzers clean) / F h2spec 53/53
PASS. Cross-package regression matrix per SPEC §12 item C8 GREEN
(fixture-0019 + fixture-0020 + fixture-0021).

SPEC §15 18-item acceptance checklist all GREEN. D11 hypothesis HOLDING:
ADR-0186 stays unconsumed at IMPL phase-done. STATE.md re-advanced to
post-phase-20 state. ROADMAP row 20 flipped in-progress → done. REVIEW.md
authored per superpowers:requesting-code-review."
```

---

## Phase-done squash-merge + push to origin

After Task 14 completes:

1. **Squash-merge to master** (from the master worktree):

```bash
cd /home/esa/git/envoy-go  # the master worktree
git merge --squash phase-20-http-filter-oauth2-impl
# Resolve commit message — body must include the 14-task summary + the 9-NEW-ADR
# + 2-IN-PLACE-AMENDMENT roster + the closes-row-20 + ADR-0159-closure milestone.
git commit -m "$(cat <<'EOF'
Squash merge phase-20-http-filter-oauth2-impl

Closes ROADMAP row 20 (in-progress → done) — THIRTEENTH §9 family-row.

14 tasks landed. 9 NEW ADRs anchored (ADR-0177 internal/httpclient/ +
ADR-0178 internal/sdsfile/ + ADR-0179 HMAC composition + ADR-0180
state-machine + ADR-0181 envelope + 6-counter surface + ADR-0182
AES-256-CBC + ADR-0183 refresh-token rotation + ADR-0184 sign-out +
ADR-0185 token_endpoint POST templates). 2 IN-PLACE §Decision AMENDMENT
bodies anchored (ADR-0150 jwks Fetcher refactor + ADR-0159 extauthz
httpAuthClient refactor + §Future Work CLOSURE-AT-PHASE-20 paragraph).
FIRST §9 family-row to CLOSE prior-phase load-bearing forward-pointer
(ADR-0159 §Future Work third-outbound-HTTP-consumer trigger → closed
per Q2 EXTRACT NOW). 26th fuzzer FuzzOAuth2ConfigParse clean at 30s.
25/25 differential fixtures GREEN (0000-0024). All 6 phase-done gates
GREEN. SPEC §15 18-item acceptance checklist all GREEN.

THIRD CONSECUTIVE §9 row to skip ADR-0125 amendment (REUSE-by-absence
stronger form per phase-20 SPEC §5.4 + ADR-0180). Stat surface 86 → 92
names per AMEND-4 + S5. Deny-path 302+401 only — NO 500 anywhere per
AMEND-3.
EOF
)"
```

2. **SHA-fill follow-up** (per the phase-09..19.2 convention):

```bash
# Update STATE.md last-commit field with the real squash SHA (was TBD at Task 14):
# Edit docs/envoy-go/STATE.md replacing "<TBD — SHA-fill follow-up after squash-merge>"
# with the actual squash commit SHA from `git log -1 --format=%H master`.
git add docs/envoy-go/STATE.md
git commit -m "phase 20 IMPL follow-up: STATE.md SHA-fill (TBD → <squash SHA> post-squash)"
```

3. **Push to origin** (per project memory `feedback_push_to_origin.md` — always-push-to-origin without asking):

```bash
git push origin master
```

4. **Worktree cleanup** (optional but tidy):

```bash
git worktree remove /home/esa/git/envoy-go/.worktrees/phase-20-http-filter-oauth2-impl
# Keep the branch alive for reference; do NOT delete unless cleanup is explicit
```

---

## Remember
- Exact file paths always.
- Complete code shapes are in the SPEC §6 references — the PLAN points to SPEC §6 rather than reproducing the full code (per the SPEC-vs-PLAN division of labor); the per-Task File-structure table rows + per-Task Step bodies above describe the IMPL surface in implementer-actionable detail.
- Exact commands with expected output for each Step.
- Reference relevant skills with @ syntax where applicable: `@superpowers:subagent-driven-development` (recommended IMPL execution per project memory `feedback_execution_style.md`), `@superpowers:executing-plans` (alternative inline), `@superpowers:systematic-debugging` (when race-test flakes surface at Tasks 3 / 7 / 8), `@superpowers:test-driven-development` (every code task is Write-failing-test → Run-FAIL → Implement → Run-PASS → Commit), `@superpowers:requesting-code-review` (Task 14), `@superpowers:verification-before-completion` (the 6 phase-done gates at Task 14).
- DRY, YAGNI, TDD, frequent commits.

End of phase 20 PLAN.
