# BRAINSTORM 87 — h2-double-slash-path-routing

**Stage:** BRAINSTORM (lifecycle-state DONE -> 1). **Date:** 2026-08-12.
**Base master:** `638ef78ad942d6392be7fb8bb7d246d0200039b6` (from `git rev-parse master`), branch `phase-87-brainstorm`.
**Method:** SELF-PICKED per the 2026-07-12 standing directive; no banked mid-lifecycle work existed at this tip (phase 86 CLOSED, row 86 `done`, every chartered row `done` — sentinel check (1) SILENT at `want=118` before this stage). ⚠️ **NAMED DEPARTURE from the investigation-agent pattern: this stage's probes were executed INLINE by the controller** — a tip binary built with `go build -o <scratch>` (never into the repo tree), four `curl --http2-prior-knowledge --path-as-is` executions against it over h2c, two standalone `go run` `net/url` differentials, plus code reads and greps at `638ef78a`. Ports 47400-47402 template values only; nothing bound survives the probe. Every load-bearing claim below is first-hand execution or a direct code read at this tip; carried claims are labeled as carried.

---

## 1. THE HEADLINE

### 1.1 ⚠️ THE H2 DOWNSTREAM CODEC MIS-PARSES A LEADING `//` IN THE ORIGIN-FORM `:path`, AND IT FAILS TWO WAYS — REPRODUCED BY EXECUTION AT THIS TIP

envoy-go's hand-rolled HTTP/2 downstream codec builds the request from the decoded pseudo-headers in `buildRequest` (`internal/filter/hcm/h2/stream.go`), and parses the origin-form `:path` with **`url.Parse`**:

```go
u, err := url.Parse(path)   // internal/filter/hcm/h2/stream.go, in buildRequest()
...
u.Scheme = scheme
u.Host = authority
```

`url.Parse` is a **generic URI parser**, not a request-target parser: a value beginning with `//` is read as a *network-path reference* (`//host/path`), so the authority is peeled off into `u.Host` and the routing path is corrupted. `u.Host` is then overwritten with `:authority`, but the damage is already in `u.Path`. Measured at this tip with a standalone `net/url` differential (`go run`):

| `:path` value | `url.Parse` → `u.Path` | `url.ParseRequestURI` → `.Path` |
|---|---|---|
| `//foo` | **`""`** (Host peeled to `foo`) | `//foo` |
| `//` | **`""`** | `//` |
| `//foo/bar` | **`/bar`** (Host peeled to `foo`) | `//foo/bar` |
| `/a//b` | `/a//b` (mid-path `//` unaffected) | `/a//b` |
| `/foo` | `/foo` | `/foo` |

So a leading `//` produces **two distinct failure modes**, both wrong:
1. **`//foo` → empty path → routing miss (404).** The route table sees `""`, which matches no `prefix: "/"` route.
2. **`//foo/bar` → SILENT MIS-ROUTE to `/bar`.** The leading segment is swallowed as an authority; the request is routed and served **against the wrong path** with no error — strictly worse than the 404, because it is invisible.

### 1.2 Reproduced end-to-end against the running proxy (h2c), with a positive control and an H1 cross-check

Binary built at `638ef78a` into scratch; a one-listener config (`codec_type: HTTP2`, `--allow-h2c`) with a single `direct_response{status:200, body:"routed-ok"}` on `route match {prefix:"/"}`. `curl --http2-prior-knowledge --path-as-is`:

| request | over **H2** (MEASURED) | over **H1** (MEASURED) |
|---|---|---|
| `GET /` (control) | `routed-ok` **200** | — |
| `GET /a//b` (mid-path control) | `routed-ok` **200** | — |
| `GET //foo` | **404**, empty body | `routed-ok` **200** |
| `GET //` | **404**, empty body | — |

⚠️ **The defect is H2-specific.** The identical `//foo` request over **HTTP/1.1 returns `routed-ok` 200** — because the H1 path is served by Go's `net/http` server, which parses the request-target correctly. It is therefore both a **cross-side divergence** (envoy-go H2 vs a reference Envoy, §3) and a **subject-internal H1-vs-H2 inconsistency** (envoy-go's own two codecs disagree on the same request).

### 1.3 The blast radius is exactly ONE production site; H1 and H3 are already correct

- **H1** — served by `net/http`'s server, which parses the request-line target itself. `//foo` routes correctly (measured, §1.2). No envoy-go path-parse code involved.
- **H3** — `internal/filter/hcm/h3dispatch.go` receives an `*http.Request` already built by the `quic-go`/`http3` library (it only re-injects the `:path` pseudo-header onto `r.Header`; `grep` shows no `url.Parse` in the H3 dispatch path). The library parses the request-target; envoy-go does not.
- **H2** — the ONLY codec envoy-go hand-rolls end to end, and the ONLY one with its own `:path` parse. `buildRequest`'s `url.Parse(path)` is the single defect site. `grep -rn 'url.Parse\|ParseRequestURI' internal/filter/hcm/` finds it in exactly one non-test production location.

### 1.4 Root cause named precisely, so the SPEC does not re-derive it

`url.Parse` implements the full RFC 3986 grammar including *network-path references* (`//authority/path`); a request-target `:path` is RFC 9113 §8.3.1 **origin-form** (`absolute-path [ "?" query ]`), asterisk-form (`*`, OPTIONS), or absolute-form. The right primitive is **`url.ParseRequestURI`**, which parses a request-target and preserves a leading `//` as PATH. Measured at this tip: `url.ParseRequestURI("//foo") = {Path:"//foo"}`, `("*") = {Path:"*"}` (asterisk-form survives), `("/foo?a=b")` splits the query. (The one behavioral wrinkle for the SPEC: `ParseRequestURI` does **not** split a `#fragment` out of the path, where `url.Parse` does — but a `#` in an H2 `:path` is out-of-spec and rare; §4 Q1.)

---

## 2. THE PICK, AND THE REJECTED ALTERNATIVES

**PICKED — `h2-double-slash-path-routing`.** Registered as a **core-HCM / HTTP-routing MAINTENANCE row claiming NO family ordinal** (the row-85 / row-86 precedent — a maintenance row repairs a landed deliverable and does not extend a charter; it is NOT one of the six feature-family windows' rows). It repairs the **phase-05.1 downstream H2 codec** (`internal/filter/hcm/h2`), a landed MVP-trunk deliverable. ⚠️ **Provenance: the phase-74 BRAINSTORM sweep**, recorded in the ROADMAP phase-74 Observability-window paragraph's *"Newly surfaced this session and NONE chartered:"* prose (`a pre-existing H2 //-path routing bug (internal/filter/hcm/h2/stream.go uses url.Parse, not url.ParseRequestURI)`) and carried in `STATE.md`/`next-prompt.txt`'s documentary-defects list. That *"newly surfaced"* prose is a **historical record of a past session's findings, NOT a live `deferred candidates:` sentence** — it does not match check (2)'s matcher and it names no family ordinal — so, exactly like row 86 left the Op-tooling window's "RTDS/SDS validate companion" byte-untouched, **nothing narrows in any family-window sentence at row-done** (the phase-85/86 provenance shape). The phase-74 *"newly surfaced"* mention stays byte-untouched (rewriting a past session's record to say "chartered" would falsify its history).

Why it is the smallest **defensible** candidate at this tip:

1. **It is a genuine user-facing production defect, reproduced end to end at this tip with a positive control and an H1 cross-check** (§1.2) — not a doc contradiction, not a hygiene item, not a process artifact.
2. **It is the smallest such defect on the board** — a single production edit site (§1.3), against the CONTINUATION repair's two-sided multi-file scope (§2.1).
3. **It is cleanly differential-provable** — a leading-`//` request has an unambiguous, byte-comparable cross-side answer (reference routes it; subject 404s or mis-routes), unlike the CONTINUATION repair (h2spec MEASURED not to cover it) or the stat recount (no wire surface).
4. **The silent mis-route case (`//foo/bar → /bar`) is a correctness hazard, not merely an availability one** — a request served against the wrong path is worse than a rejected one, which raises this above the pure-404 framing the phase-74 sweep recorded.

### 2.1 Rejected alternatives, re-derived at this tip

| rejected alternative | re-derived cost at this tip | why rejected |
|---|---|---|
| **CONTINUATION two-sided repair** (server discard at `internal/filter/hcm/h2/conn.go`'s `dispatchFrame` `*http2.ContinuationFrame` arm — CONFIRMED still a bare `return nil` behind the "shouldn't happen" comment at this tip; client blindness; the client-side SETTINGS gap in `h2/client.go`, all named in ADR-0307 §Consequences vii) | est. **2-4x row 85** (row 85 realized **+1046 net `.go`**); needs its own gates on BOTH sides, and h2spec is MEASURED not to cover either (ADR-0307 §Consequences vii names all four as deferred) | The strongest KNOWN product defect and the natural next LARGE row — but explicitly not the *smallest defensible* (the standing directive picks smallest first). Its gate does not exist yet. A future session should charter it as a SPLIT phase; deferral does not orphan it (ADR-0307, the gRPC window, and STATE all carry it). |
| **Stat-surface mechanical recount** (the 1205-vs-1207 contradiction, WIDENED, both figures DOC-SOURCED) | ~0 production; one read-only census + doc reconciliation | Cheapest on the board but changes no behavior and repairs no defect; it "rides a future +0 row" (BRAINSTORM-85/86). This row lands production lines and has a wire surface — a different shape. Remains available. |
| **`ssl.connection_error`** (Observability window) | floor **+444 net `.go`** (carried from BRAINSTORM-84 §2.2's whole-`.go` measure; NOT re-measured here) | Larger; a live Observability-family row, not a maintenance repair. |
| **`test/conformance/grpc/`** | test-only ~400-1100 lines; **9 of 26** interop cases reachable (carried) | ~65% vacuous at birth; a later gRPC-family row's job. |
| **REVIEW.md restoration** (37 of 127 phase dirs carry one; none since 25.3) | n/a | Process-not-product; retro-writing fabricates review acts. Standing named departure. |
| **`D-86-CONN client.Close` gate** (the UNGATED invariant landed at the 86 IMPL — `internal/boot/boot.go`'s `_ = client.Close()`, deleting it stays green) | ~10 test-only lines riding `validate_test.go`'s goroutine-count idiom | Too thin to be a standalone row (the 86 IMPL named it a fold-in candidate); it repairs no user-visible behavior. A future maintenance row can fold it in. |
| **hygiene fold-ins** (`harness_test.go` stale port inventory — CONFIRMED still `backendPortBandBase=11000` with a stale comment at this tip; xDS cycle-guard not automated) | thin, test/process only | No product behavior; not a defensible standalone row. |

---

## 3. THE CROSS-SIDE EXPECTATION (carried; the SPEC verifies by reference container)

The reference is `envoyproxy/envoy:contrib-v1.37.2` (`ENVOY_TARGET.md`). Envoy's HTTP/2 downstream, with **default** path handling — `merge_slashes: false`, `path_with_escaped_slashes_action` default, `normalize_path` default — preserves a leading `//` and routes origin-form `//foo` against a `prefix: "/"` route (any path starting with `/` matches, and `//foo` does). Expected reference answers: `GET //foo` → **200** (routed), `GET //foo/bar` → **200** routed against `//foo/bar` (NOT `/bar`). ⚠️ **This is a carried expectation, not executed at this BRAINSTORM** — the named-departure inline probes covered the subject side and the `net/url` root cause; the reference-container run is the load-bearing SPEC probe (Q4). The phase-86 precedent: the BRAINSTORM reproduced the subject arms by execution and deferred the reference container to the SPEC. If the reference turns out to merge or normalize by default (contradicting the above), the row's contract changes shape and the SPEC records it — that is exactly why it is a SPEC probe and not a BRAINSTORM claim.

---

## 4. THE SEVEN OPEN QUESTIONS THE SPEC OWES (disposed by execution)

- **Q1 — the fix primitive.** `url.ParseRequestURI` (preserves `//foo`, keeps asterisk-form `*`, splits query — all measured at this tip) vs a manual `&url.URL{Path: …, RawQuery: …}` construction. Enumerate every `:path` form envoy-go's H2 must accept: origin-form (the common case), asterisk-form `*` (OPTIONS — `ParseRequestURI("*")` = `{Path:"*"}`, survives), and whether absolute-form / CONNECT authority-form reach `buildRequest` at all (today `:path == ""` is already rejected upstream as `empty :path`, so CONNECT is out of scope). Settle the `#fragment` divergence (`ParseRequestURI` keeps it in Path; `url.Parse` splits it) — decide by reference behavior, not by taste. **BUILD the fix as a compiling, test-green prototype and MEASURE it** (the standing lower-bound lesson — the tenth consecutive firing closed the 86 lineage; every budget below is a FLOOR).
- **Q2 — query and RawQuery preservation.** Confirm the chosen primitive keeps `/foo?a=b` → `{Path:"/foo", RawQuery:"a=b"}` and that downstream routing + access logging see the same path/query they see today for the non-`//` common case (a zero-regression anchor: the 121-fixture differential suite must stay byte-stable).
- **Q3 — the two failure arms are BOTH proven.** The RED anchors are `//foo` (404 today) AND `//foo/bar` (mis-route to `/bar` today). The mis-route arm is the stronger one and a substring/status assertion alone cannot catch it — the proof must assert the **routed path** (echo it via a header or an access-log field, or route `//foo/bar` and `/bar` to DISTINCT responses so the mis-route is visible cross-side).
- **Q4 — reference-container re-verification of §3 BY EXECUTION.** One `--rm` container per shape (name `b87-*`, tear down BY NAME), confirm `//foo` → 200 and `//foo/bar` routed against the full path on the reference H2 downstream at the pin. If default path-normalization alters this, record it and reshape the contract.
- **Q5 — the differential proof shape, the load-bearing COST question.** `fixture.HTTPExpectations` is TCP/H1-only (`reference_differential_http_expectations_tcp_only`), so an H2 arm needs `Drive` hooks. The hard sub-question: **how to inject a literal `:path = //foo`** — `golang.org/x/net/http2`'s `Transport` derives `:path` from `req.URL.RequestURI()` and may normalize it, so the driver may need a raw HEADERS-frame writer or a `curl --http2-prior-knowledge --path-as-is` subprocess (which WORKED at this BRAINSTORM). Decide: a NEW H2 fixture, or an extension of `0004-h2-routing`. This is where the cost lives.
- **Q6 — placement of the unit RED anchors.** `buildRequest` is unit-testable directly (feed `[]hpack.HeaderField` with `:path=//foo`, assert `req.URL.Path == "//foo"`); place per-arm unit tests in `internal/filter/hcm/h2/stream_test.go` (or the existing h2 test file) plus an optional seed in the existing `internal/filter/hcm/h2/fuzz_test.go`. The differential fixture is the end-to-end proof; the unit arms are the fast RED.
- **Q7 — counts.** Anticipated: stat surface **+0** (no new counter), fuzzers **+0 or +1** (a `//foo` seed is an `f.Add`, not a new `func Fuzz` — `reference_fuzzer_count_docs_drift`), BackendKind **+0**, go.mod **+0** (`net/url` already imported), fixtures **+0 or +1** (Q5). ONE ADR anticipated (**ADR-0309** from the tail — the H2 request-target parse correction; its §Context re-arms the strict `PROPOSED` guard 0 -> 1 at the SPEC). The `BEHAVIOR_CONTRACT.md ## HTTP/2` section likely gains an origin-form-path parity rule (the contract-edit rides the ADR per ADR-0052).

### 4.1 Cost floor (re-derived; a FLOOR, not an estimate)

Production **~2-10 net `.go`** (the single-site `url.Parse` → `url.ParseRequestURI` swap plus, plausibly, a small `parseRequestTarget` helper handling the origin/asterisk forms and a comment naming RFC 9113 §8.3.1); test **~120-400 net `.go`** (unit arms on `buildRequest` for the two failure modes + the common-case + asterisk-form regression pins, plus the differential fixture driver/yaml/README if Q5 lands a new fixture). ⚠️ **This is a FLOOR — the SPEC must ENUMERATE by prototype.** The phase-86 lineage closed with the TENTH consecutive `reference_measured_prototype_is_a_lower_bound` firing (test side overran the PLAN ceiling by 356 lines), cause under-ENUMERATION every time. The SPEC's prototype must include the differential harness plumbing (Q5), not just the one-line fix, before quoting a band.

---

## 5. SENTINEL (measured, BEFORE and AFTER the ROADMAP edit)

See PROGRESS.md §Sentinel for the recorded both-sides output. Summary: the sentinel does NOT fire on either side; `stop` NOT created; check (1) transitions from SILENT (want=118) to `NOT DONE: row 87` (want=119); check (2) stays SIX; check (3) stays SILENT; every leak axis invariant by whole-file count.

## 6. NEXT

**SPEC** — dispose Q1-Q7 by execution, centered on a compiling test-green prototype of the `buildRequest` fix (Q1) and the reference-container re-verification (Q4); draft `ADR-0309 §Context` STATUS `PROPOSED` (re-arms the strict guard 0 -> 1); enumerate the cost by prototype (the §4.1 floor is a floor).
