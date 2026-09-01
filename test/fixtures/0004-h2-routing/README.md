# Fixture 0004 — HTTP/2 routing (HCM AUTO + ALPN h2 + upstream H/2 + TLS termination + TLS origination)

**Purpose:** end-to-end exercise the phase-05.2 dataplane (HCM(AUTO) + route match + router → upstream-H/2 client codec + upstream-TLS validation) and prove byte-equivalent decoded response bodies + per-side RR distribution + status-code equivalence between upstream Envoy and envoy-go on a 47-request workload, all over TLS-terminated downstream H/2 + TLS-originated upstream H/2. Since phase 89 the workload also covers **decode-side HTTP-filter header mutations reaching the upstream H/2 request** (ADR-0311). Since phase 90 it also covers **H/2 `host` vs `:authority` normalization on the downstream leg** (ADR-0312). Since phase 92 it also covers **rejection of a malformed upstream LEADING response header block** on the encode direction (ADR-0314).

**Differential surface:** concatenated decoded response bodies for the 9 `/health` direct_response requests (`"OK\n"` x 9) followed by the two phase-87 leading-`//` arms (`"edge-ok"` x 2) are byte-equivalent. The `/api/v1/<n>` router-action bodies are NOT concatenated into the diff stream (RR-pick ordering may diverge between STATIC and STRICT_DNS); routing correctness is covered by per-side `[3,3,3]` distribution + status-200. The 404 catch-all bodies are NOT compared (envoy-go: `not found\n`; Envoy: HTML/JSON local reply per its default config).

**Local-correctness surface:** each proxy's per-cluster accept counts over the 9 router-action requests must be exactly `[3, 3, 3]` (RR witness per ADR-0028's `--concurrency 1` reference pin). Counts are derived from parsing the response body's `"backend-<idx>:"` prefix — the H/2 backend is a subprocess, so the runner's in-process accept counters do NOT see these requests; the driver implements `DistributionAsserter` from the response bytes instead.

**Topology:**

```
client (test, H2RoundTrip helper, fresh dial per request)
    ──TLS+ALPN h2──> [subj listener 127.0.0.1:<subjPort>] ──HCM(AUTO)──> direct_response | router → STATIC c_h2_backend → TLS+h2 → 127.0.0.1:<bN>
    ──TLS+ALPN h2──> [container-mapped <hostPort>]        ──Envoy──>     HCM(AUTO) → direct_response | router → STRICT_DNS c_h2_backend → TLS+h2 → host.docker.internal:<bN>
```

The 3 backends are subprocesses spawned from `test/fixtures/0004-h2-routing/backends/main.go` (one per index 0/1/2; `BACKEND_IDX` env var supplies the idx that flows into response bodies). They listen on TLS with `NextProtos=["h2"]` + `http2.ConfigureServer` driver-side (D-3.2 governs envoy-go runtime, not test backends).

**Routes (declaration order; first-match-wins):**

1. `match.path: "/health"` → direct_response 200 `"OK\n"`
2. **(phase 89)** seven `match.path: "/api/v1/reflect-headers/<a1|a2|a3|a4|a6|a7|a8>"` exact routes → router → cluster `c_h2_backend`, each carrying its own `typed_per_filter_config` `HeaderMutationPerRoute`. Declared **above** the `/api` prefix route so first-match-wins picks them. `a5` has deliberately **no** route — it falls through to the `/api` prefix route, which *is* the no-per-route-mutation case.
3. `match.prefix: "/api"` → router → cluster `c_h2_backend` (3 endpoints, RR)
4. `match.prefix: "//edge"` → direct_response 200 `"edge-ok"` (phase 87 — leading-`//` origin-form routing)
5. `match.prefix: "/"` → direct_response 404 `"not found\n"` (explicit catch-all)

**HTTP filters (both sides, identical):** an **empty** `envoy.filters.http.header_mutation` (zero listener-scope mutations — its only job is to make the filter name live so the per-route configs attach; MEASURED: reference Envoy `contrib-v1.37.2` boots this shape, `--mode validate` ⇒ `configuration OK`, rc=0) followed by `envoy.filters.http.router`. Because the listener-level filter declares no mutations, the pre-existing 31 round-trips are byte-untouched.

**Driver request schedule (47 requests per side):**

- 9 × `GET /health` → expect 200, body `"OK\n"` (concatenated into the byte stream)
- 9 × `GET /api/v1/<n>` for n=0..8 → expect 200, body `"backend-<idx>:v1/<n>"` (not concatenated; distribution counted)
- 9 × `GET /missing/<n>` for n=0..8 → expect 404 (body NOT concatenated)
- 1 × `GET //edge` → expect 200, body `"edge-ok"` (concatenated)
- 1 × `GET //edge/health` → expect 200, body `"edge-ok"` (concatenated)
- 1 × `GET /api/v1/reflect` → expect 200, body `"reflect:probe=probe-value,padlen=32000"` (phase 88; not concatenated, not counted into the distribution)
- 1 × `GET /api/v1/emit` → expect 200, response header `x-cont-marker: emitted` plus a 32000-byte `x-cont-pad` (phase 88; not concatenated)
- 8 × the phase-89 decode-mutation arms `A1..A8` against `/api/v1/reflect-headers/<arm>` → expect 200; the reflected header block is asserted in-band, and only a normalized `p89-<arm>:ok` marker is appended to the byte stream
- 3 × the phase-90 authority-normalization arms `P90-P` / `P90-A` / `P90-B` against `/api/v1/reflect-headers/p90{p,a,b}` → expect 200; each is a **hand-built HPACK field list over its own raw `http2.Framer` connection**, and each appends exactly one `p90-<arm>:auth=… host=…` line to the byte stream
- 5 × the phase-92 illegal-RESPONSE-header arms against `/api/v1/p92-{keepalive,upgrade,proxyconn,te-gzip,te-empty}` → each backend path emits exactly **ONE** illegal connection-specific response header, and the driver reads the **downstream** header block back with a raw framer. Each appends one `p92-<arm>:status=… illegal=…` line to the byte stream. ⚠️ **ONE SHAPE PER PATH is load-bearing** — a single path emitting all of them is blind to a fix that catches one and launders another. ⚠️ The transcript records the **SET** of illegal names present, never a single name. ⚠️ `content-length` is deliberately **NOT** in the cross-side line — it is pinned **per side** instead (since phase 93 as a **declared-vs-delivered body-length** pin rather than an arity pin); see the phase-92 departure section below.

The first 27 requests are unchanged from phase 05.2, and the two new arms are appended after them, so the pre-existing transcript prefix stays byte-identical.

**Leading-`//` origin-form arms (phase 87):** a leading `//` in an HTTP/2 `:path` is an ordinary origin-form path, but under the full RFC-3986 URI grammar it reads as a network-path reference whose first segment is an authority. A codec that parses `:path` with `url.Parse` peels the first segment into the host and routes on the remainder. **Both assertions on both arms are load-bearing; neither arm alone catches both failure modes:**

- `GET //edge` — the **status** assertion is load-bearing. Under the defect the path degrades to `""`, a route MISS (an empty path matches neither `//edge` nor the `/` catch-all), so the reply is 404 with an **empty** body — not the catch-all's `not found\n`. A body-only check would compare `""` against `""` and read green.
- `GET //edge/health` — the **body** assertion is load-bearing. Under the same defect the path degrades to `/health`, which matches route 1 and replies 200 `"OK\n"` — a silent mis-route. A status-only check would see 200 and read green.

The two arms are `direct_response` and touch no backend, so the per-side `[3,3,3]` RR distribution over the 9 router-action requests is unchanged.

**CONTINUATION arms (phase 88):** both arms send/receive a 32000-byte header value. MEASURED at this fixture's pad alphabet, that value HPACK-encodes to **23792 B** — past the 16384 B RFC 9113 §6.5.2 default `SETTINGS_MAX_FRAME_SIZE` — so the header block **must** travel as `HEADERS` + `CONTINUATION`. Negative controls at the same alphabet: 1024 B → 774 B encoded and 16000 B → 11902 B encoded, both in **one** frame. **The flip is at the frame-split boundary, not at a header-size threshold.**

- `GET /api/v1/reflect` — request direction. The backend reports back the **LENGTH** of the `x-cont-pad` header it received. The length is the load-bearing datum: the CONTINUATION-discard defect is **partial** (fields encoded before the split point survive), so asserting the small `x-cont-probe` header's **presence** would read green on a broken codec.
- `GET /api/v1/emit` — response direction. Same length assertion on the response header block emitted by the backend.

Neither arm asserts **which** headers survive (that is x/net encoder field ordering, not a contract), neither body enters the differential byte stream, and both are appended after the 9 `/api/v1/<n>` requests so the per-side `[3,3,3]` RR distribution over those 9 is unchanged. ⚠️ Changing the pad alphabet or size changes the encoded size and moves the split point — **re-measure**, do not assume the byte count carries.

## Decode-side filter-mutation arms (phase 89, ADR-0311)

Before phase 89, decode-side HTTP-filter header mutations **never reached the upstream H/2 request** — additions were lost and removals ignored — while H/1, H/3 and the H/2 *encode* side all worked. The cause was two containers with no write-back: the decode chain mutates an `http.Header` map while the upstream carrier is a separate ordered `[]hpack.HeaderField`. `reconcileH2DecodeDelta` (in `internal/filter/hcm/h2dispatch.go`) now re-emits the delta onto the carrier. These arms are the cross-side witness.

Each arm sends one request to `/api/v1/reflect-headers/<arm>`; the backend replies with a **sorted** `name: value` block of the request headers it actually received.

| arm | route mutation | client sends | asserted |
|---|---|---|---|
| A1 | append `x-p89-added: a1` (`APPEND_IF_EXISTS_OR_ADD`) | — | `x-p89-added` present **exactly once**, value `a1`; 200 |
| A2 | `remove: "x-p89-removed"` | `x-p89-removed: seed` | name **absent**; 200 |
| A3 | `x-p89-changed: new` (`OVERWRITE_IF_EXISTS_OR_ADD`) | `x-p89-changed: old` | **exactly one** value, `new`; 200 |
| A4 | append `x-p89-dup: v1` (`APPEND_IF_EXISTS_OR_ADD`) | `x-p89-dup: c0` | **both** values, `[c0 v1]` — the cross-side pin for the APPEND rule |
| A5 | *(no per-route config; falls through to the `/api` prefix route)* | — | 200; the reconcile is a no-op on an empty delta |
| A6 | append with a canonical-MIME config key `X-P89-Case: c1` | — | 200 and `x-p89-case: c1` present |
| A7a | append `x-p89-benign: kept` **and** `te: trailers` (`te` exactly once) | — | both present; 200 |
| A8 | the A1 mutation on a **POST with a non-empty body** | body bytes | mutation lands; 200 |

**Two reference write rules, both reproduced.** MEASURED against `envoyproxy/envoy:contrib-v1.37.2` with a raw-framer recorder (one `hpack.Decoder` per connection, `hpack_errors=0`): `OVERWRITE_IF_EXISTS_OR_ADD` removes **every** occurrence and re-appends **one** copy at the tail, while `APPEND_IF_EXISTS_OR_ADD` leaves the existing field **at its position** and appends only the new value at the tail. A3 pins the first rule; **A4 pins the second** — collapsing the two into a single remove-everywhere-then-tail-append rule relocates an otherwise-untouched original.

**⚠️ NO ARM ASSERTS HEADER ORDER, AND THAT IS A DELIBERATE SCOPE LIMIT, NOT AN OVERSIGHT.** Two independent reasons, either of which alone is sufficient:

1. `helpers.H2RoundTrip` sets client headers with `req.Header.Add` onto a Go map, so the relative order of **different** client-sent names is randomized per request by map iteration. Any assertion over client-sent header order would be a flake, not a pin. (Duplicates of a *single* name stay adjacent and in order — which is why A4's `[c0 v1]` *is* a pin.)
2. This fixture's backend is `net/http` + `http2.ConfigureServer`, and x/net's H/2 server folds the HEADERS block into an `http.Header` **map** (`server.go`) before any handler runs. Wire order is destroyed before it can be observed, and no hook exposes `MetaHeadersFrame.Fields`. Recovering it would need a raw-framer backend — a new `BackendKind` for one assertion.

Wire **order** is therefore pinned at the **unit** layer (`internal/filter/hcm` reconcile tests). This fixture pins presence / absence / value / **count** / status.

**⚠️ Honest scope on three arms** — do not read them as more than they are:

- **A5's "no `:`-prefixed name in the block" check is a FREE VACUOUS INVARIANT.** A `:`-prefixed name in the regular-header region is an RFC 9113 §8.3 protocol error the backend's codec rejects before any handler runs, so the check can never fire on a reachable input. It is kept because it costs nothing and documents intent; it does **not** pin `h2ReconcileSkipKey`'s pseudo-header clause (that is pinned at the unit layer).
- **A6 does not prove the wire name was lowercased.** The backend canonicalizes every received name, so the reflected block reads `X-P89-Case` either way. The real discriminator is that an uppercase name on an H/2 request is a protocol error: a proxy emitting it verbatim fails the round-trip, and A6 then goes red on the **status** check, not the header check.
- **A8 exercises the `hasBody` / `RunDecodeData` branch but does not discriminate the reconcile's placement relative to it.** Every mutation here originates in `DecodeHeaders`, so moving the reconcile above the `hasBody` block would leave A8 green. That placement is pinned at the unit layer by `TestWriteH2_Reconcile_DecodeDataMutationIsApplied`.

**⚠️ The reflected body never enters the differential transcript.** It carries `x-request-id` (a random UUID) plus reference-only `x-forwarded-proto` and `x-envoy-expected-rq-timeout-ms`, so the two sides can never compare equal byte-for-byte. Each arm parses the block and asserts named headers **in-band** (`return nil, fmt.Errorf(...)`, so the first failing arm aborts the Drive); only the normalized `p89-<arm>:ok` marker is appended to the cross-side byte stream. For the same reason no arm may assert that its mutated header is the **last** field on the wire: the reference adds `x-forwarded-proto` / `x-request-id` *before* the filter chain and `x-envoy-expected-rq-timeout-ms` from the *router*, i.e. after `header_mutation`.

### ⚠️ Phase-89 INTENTIONAL DEPARTURE — arm A7b is deliberately omitted

`connection`, `transfer-encoding` and `te: gzip` **cannot** be cross-side arms here, and no amount of fixture work will make them green:

- Reference Envoy accepts all three as `header_mutation` config (rc=0 — its protected set is exactly `:`-prefixed + `host`, the same set as envoy-go's `isProtectedHeader`) and then **forwards them verbatim onto the H/2 upstream wire**. Against this fixture's conformant backend (`net/http` + `x/net/http2`, whose `checkValidHTTP2RequestHeaders` rejects the connection-specific set and any `te` other than `trailers`) each yields **400 with zero backend delivery**, attributed by Envoy's own `cluster.*.upstream_rq_400: 1`.
- envoy-go **drops** them (`h2.IsIllegalH2RequestHeader`, value-aware so `te: trailers` survives) and answers **200 with delivery** — decision **D-89-VALIDATE**, frozen.

That is a real, measured, **intentional divergence**, not parity, so it is pinned at the **unit** layer instead (`…/11_rfc9113_illegal_pairs_dropped_te_trailers_kept`). Dropping rather than rejecting is the deliberate choice: rejecting would turn a config-legal filter mutation into a client-facing 5xx, whereas dropping matches what the pre-fix tip effectively did (these mutations never reached the upstream at all).

A related reference behavior is also **not** reproduced: reference Envoy comma-coalesces repeated `te` into one field (`te: gzip,trailers`, measured) while custom names are not coalesced (`x-p89-dup` arrives as two fields, measured); envoy-go emits one field per value for every name. `te` is therefore appended **at most once** (A7a only) — a coalesced `te: trailers,trailers` would be rejected by the backend.

## H/2 `host` vs `:authority` normalization arms (phase 90, ADR-0312)

Before phase 90 the upstream H/2 request could carry **two** authorities or **none that is usable**, where reference Envoy always forwards exactly one. `buildH2Request` (`internal/filter/hcm/h2/stream.go`) filled `H2Request.Authority` from `:authority` alone and its `default:` arm appended every non-pseudo field — **including a regular `host`** — onto the ordered upstream carrier. The rule is now **promote on ABSENT** (`host` becomes the authority only when `:authority` was absent from the field list — never when it was present-and-empty), **suppress always** (a regular `host` never reaches the upstream carrier, nor the decode map), **first occurrence wins** as the promotion source.

The three arms are driven by a **raw `http2.Framer`** — the `0119-grpc-unary-trailers` instrument shape — over **one fresh TLS(ALPN h2) connection per arm, each with its own per-connection `hpack.Decoder`**. `helpers.H2RoundTrip` **cannot** express any of them, on three independent mechanisms: it has no `req.Host` surface, x/net's H/2 transport **silently drops** a client-supplied `host` entry, and `:authority` can neither be set nor emptied nor injected through it. Every arm sends a **fixed literal** authority (`p90.example` / `p90host.example`), never the dial address, so the two sides' bytes can agree by construction.

| arm | client sends (wire order) | asserted |
|---|---|---|
| P90-P | `:authority: p90.example`, no `host` | **positive control** — `x-observed-authority: p90.example`, `host` absent; must pass on both sides even PRE-fix |
| P90-A | `:authority: p90.example` **and** `host: p90host.example` | `:authority` wins; the regular `host` must **not** survive onto the upstream carrier |
| P90-B | `host: p90host.example` only, no `:authority` | `host` is **promoted** to the authority and then **suppressed** as a regular field |

The backend emits `x-observed-authority: <r.Host>` **after** its sorted reflected block (never folded into the sorted names — a lexical sort would relocate it and re-baseline every phase-89 arm). The driver renders `ABSENT` as `<absent>`, distinct from present-and-empty (`""`); that distinction is **load-bearing for P90-B**, where the pre-fix subject emits a present-and-**empty** `:authority`.

**⚠️ This block is deliberately NOT fail-fast.** The phase-87/88/89 arms assert in-band with `return nil, fmt.Errorf(...)`, so the first failing arm aborts the whole Drive and every later arm is unreachable. These arms follow 0119's discipline instead: every failure is recorded **in** the transcript (`p90-<arm>:ERR …`), all arms **always** run, and the runner's cross-side byte compare **is** the assertion. Exactly one `p90-<arm>:auth=… host=…` line is emitted per arm, always.

**⚠️ NO YAML EDIT.** Routes 2-8 are *exact* matches for `a1`-`a4`/`a6`-`a8`, so `/api/v1/reflect-headers/p90{p,a,b}` falls through to the `/api` prefix route into the backend's `reflect-headers/` subtree handler — exactly as `a5` already does by design. Both `*.yaml` files are **byte-untouched** by phase 90, and the per-backend counter increment lives only inside the `/api/v1/<n>` loop, so the per-side `[3,3,3]` RR distribution is unchanged.

### ⚠️ Phase-90 INTENTIONAL SCOPE LIMITS — two arms are deliberately absent

- **Arm C (`:authority` PRESENT-AND-EMPTY) is a deferred follow-on.** `:authority` absent and `:authority` present-and-empty **both** satisfy `rp.authority == ""` in x/net's H/2 server and both take the fallback to the `Host` header, landing byte-identical in `r.Host` **and** `r.Header`. No backend edit can recover the distinction; a raw-framer **backend** (a new `BackendKind`) would be required, and this row does not buy it.
- **Arm E (first-occurrence-wins: two regular `host` fields, no `:authority`) was built, run, and REMOVED. It is not differentiable in principle — do not re-add it.** MEASURED against the pinned image (`envoyproxy/envoy:contrib-v1.37.2`): a **second** regular `host` field on the H/2 downstream leg is rejected at the codec layer (`Invalid HTTP header field was received: frame type: 1, stream: 1, name: [host]`, details `http2.invalid.header.field`). The client is sent **no GOAWAY and no RST_STREAM** — the connection is simply closed and the arm reads a bare EOF, so the reference line is `p90-E:ERR read-frame: …` while a correct subject serves 200. **The rejection is by ARITY, not by value** (two *identical* `host` values are refused the same way) and holds with `:authority` also present. Testing first-occurrence-wins **requires** two `host` fields, so no subject-side change can ever make the two sides' bytes agree. The axis is pinned at the **unit** layer instead — `TestAuthorityNormalization/E_dup_host_first_wins` (`internal/filter/hcm/h2/authority_norm_test.go`) is the sole first-wins discriminator in the tree; do not delete it. Matching the reference's reject here is out of charter: it is reference-side admission control, the same family as the deferred arm-C validity reject and the same class as **D-90-DUP**.

## Illegal-RESPONSE-header rejection arms (phase 92, ADR-0314)

The five arms drive a backend path that emits exactly one illegal connection-specific **response** header, and read the **downstream** header block back off the wire with a raw `http2.Framer`. What the transcript records is what the proxy chose to forward. The row's contract is that the malformed upstream leading block is **rejected** and the illegal header is **not laundered downstream**.

**What CONVERGES cross-side (the row's contract, byte-compared on all five arms):**

| observable | reference | subject |
|---|---|---|
| `status` | 502 | 502 |
| `illegal` (sorted SET of illegal names in the downstream block) | `<none>` | `<none>` |

**What does NOT converge, and is pinned PER SIDE instead:** the **body length** of the 502 local reply — **reference 87, subject 12**, on all five arms. Both sides now emit exactly one `content-length` (arity **1 vs 1**), so arity is no longer the discriminator: at phase 93 the departure moved from **arity** to **length**. The per-side instrument in `driver/driver.go` records, for each side and each arm, the **declared** `content-length` value and the **delivered** sum of DATA-frame bytes; it asserts `declared == delivered` per side (RFC 9110 §8.6 — **not** relaxed) and pins the per-side lengths at their measured values. It reports **every** mismatching arm rather than the first.

⚠️ **The per-side `content-length` pin is asserted in `AssertDistribution`, deliberately BELOW the cross-side byte compare — not in `DriveReference` / `DriveSubject`.** The runner `t.Fatalf`s a Drive error at step 5/6 but only `t.Errorf`s `AssertDistribution` at step 8, *after* `CompareBytes` at step 7. **MEASURED (at phase 92, on the then-arity form of this pin — the placement finding is form-independent and still binds the phase-93 declared-vs-delivered pin):** with the production guard reverted, a Drive-level pin reported **only** `p92 subj content-length fields: …` and the row's own `status` / `illegal` divergence was **never reached** — the out-of-charter pin **masked** the charter regression it was supposed to sit beside. Below the gate, one run reports both. ⚠️ Do not move this assertion back into the Drive path.

### ⚠️ Phase-92 DOCUMENTED DEPARTURE — H/2 local replies carried no `content-length` (CLOSED at phase 93)

**Root cause (pre-phase-93 state; retained as history).** `h2LocalReplyHeaders()` (`internal/filter/http/router/router_h2.go`) returned `Content-Type` / `Date` / `Server` and **no** `Content-Length`. Its H/1 sibling `localReplyHeaders(bodyLen int)` (`internal/filter/http/router/router.go`) **did** emit one, and even took a `bodyLen` the H/2 version did not have. The asymmetry dated from **phase 07.1** and was **not** a phase-92 regression: phase 92 merely reused the existing helper. ⚠️ **CLOSED at phase 93 (ADR-0315):** the H/2 helper is now `h2LocalReplyHeaders(bodyLen int)` and emits `Content-Length` immediately after `Content-Type`, so the two codec siblings are symmetric.

**Why phase 92 is the row that exposed it.** It is a **compensating-defect unmasking**. *Pre*-fix the subject forwarded the backend's 200 carrying `content-length: 6` (arity 1) while the reference already answered 502 with a `content-length` (arity 1) — **1 vs 1, the two defects cancelled** and the field was a cross-side invariant. The fix replaces the forwarded 200 with a **locally generated** 502, and at that tip envoy-go's H/2 local reply carried no `content-length`, so the arity became **0 vs 1**. The observable only became a discriminator once the row's own defect was fixed. **Resolution (phase 93):** the subject now emits `content-length: 12` on its H/2 502, so the arity reads **1 vs 1** again — this time because both sides are *correct*, not because two defects cancel. What remains per-side-divergent is the body **length** (87 vs 12), which is local-reply **composition**, not a contract.

**What phase 92 BANKED, and what phase 93 actually did.** `h2LocalReplyHeaders()` has **seven** call sites across the 502/503/504 H/2 paths (`retry.go` 504; `router_h2.go` 503 x3 and 502 x3) — a count re-derived at the phase-93 tip and still correct. Phase 92 banked the change as unchartered and unpriced for *that* row; **phase 93 chartered and priced it (ADR-0315)**, so that clause is superseded, not still true.

⚠️ **How the seven sites changed is narrower than the phase-92 wording implied.** All seven did change — but each only **gained an argument**: four pass `0` (the 503/504 sites, which carry no body) and three pass `len(bad502Body)` (= **12**). No site needed bespoke per-site treatment, and no site's **body bytes** moved. The emitted **value** is not a new degree of freedom either: `writeH2Reply` (`internal/filter/hcm/h2dispatch.go`) overrides a **present** `content-length` inline to `len(body)`, so the wire value is recomputed regardless of what the call site passed. What changed on the wire is the header's **presence** — an *absent* `content-length` was never injected by that override, which is exactly why the subject's arity measured **0**. The argument must still be correct at the call site, because the header carrier is read by `RunEncodeHeaders` and `emitAccessLogH2` **before** `writeH2Reply` corrects it; a placeholder would lie to the encode chain and the access log even though the wire stayed right.

**Why the axis is EXCLUDED from the cross-side stream rather than left red.** Local-reply **composition** is already an established excluded cross-side axis in this fixture — the 404 catch-all bodies are not compared for exactly the same reason (envoy-go: `not found\n`; Envoy: HTML/JSON local reply per its default config). Leaving the byte stream red would have conflated a **pre-existing, out-of-charter** departure with the row's own contract, so a later real regression on `status` / `illegal` would have landed on an already-failing gate.

**Why the per-side pin is mandatory.** Dropping the observable outright would have hidden the departure. Pinning it per side at its **measured** values keeps it visible in **both** directions: if either side's local-reply body length moves (subject 12, reference 87), or either side's declared `content-length` stops matching the bytes it actually delivers, the pin **reddens** and a human must consciously re-derive it. It was this pin, held across the phase-92→93 boundary, that made the subject's 0 → 1 arity change a *deliberate* re-baseline rather than a silent one.

⚠️ **NEVER THE BODY BYTES CROSS-SIDE; THE LENGTH ONLY, AND ONLY PER SIDE.** (Phase 92 stated this as *"ARITY, NEVER VALUE"*; phase 93 **narrows** the rule rather than deleting it, because the row now pins `declared == delivered`, which *is* a value assertion.) The two 502 bodies differ by construction — the reference's own local-reply body (87 B) against envoy-go's `bad gateway\n` (12 B) — so the body **bytes** are not a cross-side contract, and the ratified relaxation stands (`BEHAVIOR_CONTRACT.md:1993`: *"Status is asserted; body is relaxed"*). What **is** asserted is per side: the declared `content-length` must equal the delivered DATA-frame byte count, and each side's length is pinned at its measured value. Arity remains the only observable a duplicate-`content-length` regression would move on its own, so the per-side instrument reads the **declared field** and not merely a body byte count — that shape stays in view.

**STATIC vs STRICT_DNS divergence (ADR-0027 inherited):** subject is host-side STATIC; reference is container-side STRICT_DNS with `dns_lookup_family: V4_ONLY` per ADR-0010.

**`--concurrency 1` reference pin (ADR-0028 inherited):** keeps RR distribution deterministic on the reference side.

**Per-request fresh dial (ADR-0039 inherited; settled SPEC §10 #13 H/2):** the driver's `H2RoundTrip` helper creates a fresh `*http2.Transport` + `*http2.ClientConn` per call (no caching), so each request consumes one RR slot end-to-end.

**ALPN-h2 e2e shape:** the driver advertises `NextProtos=["h2"]` only; the listener offers `["h2","http/1.1"]` (so HCM `codec_type: AUTO` selects H/2). Upstream cluster's `transport_socket.alpn_protocols=["h2"]` plus `typed_extension_protocol_options.HttpProtocolOptions.explicit_http_config.http2_protocol_options{}` pins the upstream codec to H/2 (per ADR-0056 — `Cluster.UseH2()`).

**ADR-0057 closure of ADR-0035 H/2 leg:** ADR-0035 carved out the `phase-05` H/2 expansion. Phase 05.1 settled the downstream H/2 leg via the H/2 server codec + h2spec gate. This fixture (with its driver landing at Task 14) closes the upstream H/2 leg by exercising the full chain end-to-end on a non-trivial workload: HCM(AUTO) → router → upstream H/2 client codec → 3 TLS h2 backends. ADR-0057 lands at Task 14 alongside the driver registration.

**Header allow-list (ADR-0044 inherited):**

`date`, `server`, `content-type`, `content-length`, `transfer-encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id` — values not compared.

**Per-side `[3,3,3]` RR rule:** the 9 `/api/v1/<n>` requests on a 3-endpoint RR cluster must distribute exactly 3 to each backend (subject + reference, independently). The driver counts by parsing `"backend-<idx>:"` prefixes from decoded response bodies.

**PKI regeneration procedure:**

```bash
cd test/fixtures/0004-h2-routing
go run ./pki/gen
git diff --exit-code pki/ && echo ok   # expect: ok (deterministic)
```

The committed PEMs are the authoritative source. CI does NOT run the generator. Re-run only to rotate (and update the `notBefore` / `notAfter` constants in `pki/gen/main.go`).

**Run locally (will become available once Task 14 lands the driver):**

```bash
go test ./test/differential/ -run 'TestDifferential/0004-h2-routing' -v -timeout=10m
```

Until then, the fixture content (this directory) is committed; the runner does not yet pick it up because no driver package is blank-imported in `test/differential/runner_test.go`.

**Re-baseline:** per ADR-0008 §"refresh procedure". If upstream Envoy's pin bumps and the gate fails on the response-body concatenation, supersede the failing ADR if the bytes change.
