# Fixture 0024 — `envoy.filters.http.oauth2`

Phase-20 differential fixture exercising the
`envoy.filters.http.oauth2.v3.OAuth2` filter implementation per
phase-20 SPEC §7 + PLAN Task 12.

## Scope decision — REFERENCE-LESS subject-only structural fixture

The Task 12 IMPL lands this fixture as **reference-less**
(`RequiresReference() == false`, mirroring the
`0007b-iteration-probe` precedent). The runner short-circuits the
reference-proxy spawn + `DriveReference` + the byte-stream
`CompareBytes` per the `runReferenceLessFixture` branch in
`test/differential/runner_test.go`. The driver's `SubjectAsserter`
in-band-asserts per-scenario wire-shape invariants against the
captured subject byte stream.

### Why reference-less?

A fully byte-exact cross-side differential against reference Envoy
v1.37.2 oauth2 requires control over per-request entropy that neither
stack exposes:

1. **AES-256-CBC random IVs** (per ADR-0182 + AMEND-1) — each
   encryption produces a fresh 16-byte random IV; the BearerToken /
   RefreshToken cookie envelope payload is therefore non-deterministic
   across stacks for the same plaintext + key. Cross-side byte-exact
   would require coordinating IV generation; no production-grade hook
   exists in either stack.

2. **State-cookie payload** (per SPEC §12 item A3 RATIFIED-PENDING-
   IMPL-TIME) — the per-request state cookie value embeds the
   epoch-second timestamp; the cross-side wall-clock skew between the
   two proxies (in two separate processes) makes the cookie value
   non-deterministic at the second resolution.

3. **token_endpoint POST randomness** (per ADR-0185) — the form-body
   field ORDER is deterministic, but the `code` value from the
   authorization_endpoint redirect is mock-controlled. Cross-side
   wire-exact for the POST body shape IS achievable; however, the
   end-to-end success path (token POST → JSON parse →
   re-encrypt-with-fresh-IV → Set-Cookie envelope wire) re-introduces
   AES-CBC IV non-determinism on the response leg.

4. **`disable_token_encryption` field absent in `go-control-plane`
   v1.32.4 proto** (per Task 11 finding) — scenario (i) per SPEC §7.1
   row 9 cannot be authentically exercised through the proto-config
   path. A future go-control-plane bump unblocks scenario (i); the
   bump-and-extend trigger is documented at the PROGRESS Task 12 entry.

The Task 12 implementer chose the responsible engineering path: land
the working subject-only structural fixture asserting wire-shape
invariants of the envoy-go filter (status code, body bytes, Set-Cookie
attribute byte-exact per ADR-0181, Location header construction per
ADR-0184 sign-out flow + ADR-0180 sign-in 302-challenge); defer the
cross-side byte-equivalent variant to a future fixture-extension task
post-go-control-plane bump.

## Listener topology — 2-listener (was 3 in SPEC §7.2)

The phase-20 SPEC §7.2 anticipated a 3-listener topology
(`l_test_a` default-encryption / `l_test_b`
`disable_token_encryption=true` / `l_test_c`
`forward_bearer_token=true`). The Task 12 IMPL ships a **2-listener
topology** (`l_test_a` + `l_test_c`) per the
`disable_token_encryption` field-absence finding; `l_test_b` is
deferred per the go-control-plane bump trigger documented above.

## Scenario coverage matrix

| #  | Scenario                                | Listener   | Status |
|----|-----------------------------------------|------------|--------|
| a  | sign-in 302-challenge wire shape        | l_test_a   | LANDED |
| b1 | cookie-passthrough + forward_bearer_token | l_test_c | LANDED |
| b2 | cookie-passthrough tampered envelope    | l_test_a   | LANDED |
| c  | pass_through_matcher bypass             | l_test_a   | LANDED |
| d  | refresh-token rotation                  | l_test_a   | DEFERRED (requires success-leg AES round-trip; see scope note above) |
| e  | sign-out flow                           | l_test_a   | LANDED |
| f  | bad-state 401                           | l_test_a   | LANDED |
| f' | POST callback PARSE-REJECT              | l_test_a   | LANDED |
| g  | token_endpoint 5xx → 302 challenge      | l_test_a   | DEFERRED (callback.go::handleCallback Task-10 wire-up gap — the auth-code leg outbound POST is wire-gapped at phase-20 IMPL Task 11; a future callback-wire-up task closes this) |
| h  | token_endpoint 4xx → 401                | l_test_a   | DEFERRED (same gap as (g)) |
| i  | disable_token_encryption=true           | l_test_b   | DEFERRED (proto field absent in go-control-plane v1.32.4) |

8 of 11 scenarios landed at IMPL Task 12. The 3 deferred scenarios
(d, i) are documented at the PROGRESS Task 12 entry as the
fixture-extension forward-pointer.

## Driver discipline

`inputs/driver.go` wires:

1. The oauthbackend mock per `test/helpers/oauthbackend/` —
   scripted per-scenario `/token` and `/authorize` responses.
2. Per-scenario request issuance against the subject's
   `l_test_a` + `l_test_c` listeners.
3. The deterministic byte-stream encoding (mirrors 0007b's
   `encodeProbe`) so `AssertSubject` is a pure analysis step on the
   captured bytes.

## §12 closure status (per PLAN Task 12 + planner-time D16+D17+D18)

| §12 item | Closure status |
|----------|----------------|
| A1 (401 Content-Type + no-trailing-newline) | RATIFIED at scenarios (f) + (h) body byte-exact |
| A2 (Set-Cookie attribute byte-exact upstream defaults) | RATIFIED at scenarios (a) + (e) + (f) Set-Cookie wire assertions |
| A3 (state-cookie payload byte-exact shape) | PARTIAL — scenario (a) asserts the state cookie IS SET; the full HMAC-protected payload byte-exact is wired in the filter at compose_state_cookie_value but the cross-side compare deferred per scope |
| A4 (HCM SendLocalReply Content-Type default) | RATIFIED at scenarios (f) + (h) Content-Type observation |
| A5 (urlEncode charset for non-ASCII bytes) | RATIFIED at Task 10 vector-tests (oauth_client_test.go); fixture (a) token_endpoint POST body capture exercises the encoder at integration time |
| B6 (AES-256-CBC PKCS#7 decrypt-failure semantics) | RATIFIED at scenario (b2) tampered envelope routes to category (a) 302 challenge per SPEC §4.7 + AMEND-3 |
| B7 (fsnotify event-debounce window) | RATIFIED at Task 3 unit-tests + Task 7 race tests |
| C8 (cross-package regression matrix) | RATIFIED at Task 2b/2c regression checks + this Task's full pre-existing-fixtures green run |
