# envoy-go Conformance Tool Image Pins

This file records the exact container image digests used in automated conformance
test gates.  Pinning by digest (not by tag) guarantees reproducibility: the same
image runs in every CI invocation, local developer run, and review environment.

## Refresh procedure

Per doctrine D-3.7, pins are changed only via a dedicated phase. All pins are
append-only: a new pin supersedes the old one for the same tool, and the old pin
is kept for audit purposes. To execute a pin refresh:

1. `docker pull <image>:<new-tag>`; capture the SHA256 with `docker inspect --format='{{index .RepoDigests 0}}'`.
2. Append a new row to the pin table with the new tag and digest, preserving the prior row for audit.
3. Run the conformance gate: `go test -run TestH2Spec ./test/conformance/h2spec/` (or equivalent for the tool).
4. Confirm all tests PASS at the new digest.
5. Mark the prior row with a "superseded by <digest>" annotation.
6. Land the change as a single commit on a dedicated pin-refresh phase branch.

---

## h2spec — HTTP/2 RFC 9113 conformance

| Field              | Value                                                                |
|--------------------|----------------------------------------------------------------------|
| **Tool**           | [summerwind/h2spec](https://github.com/summerwind/h2spec)            |
| **Image ref**      | `summerwind/h2spec`                                                  |
| **Tag at pin time**| `latest` (resolves to v2.6.0; v2.6.0 tag not published separately)  |
| **Digest**         | `sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0` |
| **Full ref**       | `summerwind/h2spec@sha256:5f4a65c30cae8569558ced048b4bfe0dcf01a221e36767ae504ccd8348a7aeb0` |
| **Pulled**         | 2026-04-25                                                           |
| **Gate file**      | `test/conformance/h2spec/h2spec.go`                                  |
| **ADR**            | ADR-0051                                                             |

### Sections tested (threshold)

| Section | Description                             | Cases |
|---------|-----------------------------------------|-------|
| 3.5     | HTTP/2 Connection Preface               | 2     |
| 4.1     | Frame Format                            | 3     |
| 4.2     | Frame Size                              | 3     |
| 4.3     | Header Compression and Decompression    | 3     |
| 5.1     | Stream States                           | 13    |
| 5.1.1   | Stream Identifiers                      | 2     |
| 5.1.2   | Stream Concurrency                      | 1     |
| 5.3.1   | Stream Dependencies                     | 2     |
| 5.4.1   | Connection Error Handling               | 2     |
| 5.5     | Extending HTTP/2                        | 2     |
| 7       | Error Codes                             | 2     |
| 8.1     | HTTP Request/Response Exchange          | 1     |
| 8.1.2   | HTTP Header Fields                      | 1     |
| 8.1.2.1 | Pseudo-Header Fields                   | 4     |
| 8.1.2.2 | Connection-Specific Header Fields      | 2     |
| 8.1.2.3 | Request Pseudo-Header Fields           | 7     |
| 8.1.2.6 | Malformed Requests and Responses       | 2     |
| 8.2     | Server Push                             | 1     |
| **Total** |                                       | **53** |

Section 6.6 (PUSH_PROMISE) is excluded because phase 05.1 disables server push per ADR-0047 / 05.1 SPEC §2.1; the section's tests are conformance-irrelevant for this surface. Per ADR-0055 (phase 05.2), the exclusion stays — the flow-control discipline tightening does not change the server-push posture.

### First run result (2026-04-25)

```
53 tests, 53 passed, 0 skipped, 0 failed
```

---

## h2spec — CORRECTED STRICT SCOPE (phase 85, ADR-0307)

⚠️ **The 18-suite table above records what the gate ACTUALLY RAN from 2026-04-25 to
2026-08-10 — it does not record what ADR-0051 §2 declared the gate ran.** The nine
section-6 selectors were declared in SLASH form (`http2/6/1` … `http2/6/5`,
`http2/6/7` … `http2/6/10`); h2spec addresses sections with **DOTTED** numbers, and
an unmatched positional argument is a **silent no-op** — it selects nothing and
reports no error. So every `http2/6.x` sub-suite ran **zero** cases, the first-run
record below measured **53 of the gate's 95 declared cases**, and the pin table and
ADR-0051 contradicted each other for roughly eighty phases because both ended in
"53". The selectors were repaired to dot form at the phase-85 IMPL. **The table
below is the TRUE scope at the same pinned digest.**

### Sections tested (corrected strict set, 31 suites)

| Section | Description                             | Cases |
|---------|-----------------------------------------|-------|
| 3.5     | HTTP/2 Connection Preface               | 2     |
| 4.1     | Frame Format                            | 3     |
| 4.2     | Frame Size                              | 3     |
| 4.3     | Header Compression and Decompression    | 3     |
| 5.1     | Stream States                           | 13    |
| 5.1.1   | Stream Identifiers                      | 2     |
| 5.1.2   | Stream Concurrency                      | 1     |
| 5.3.1   | Stream Dependencies                     | 2     |
| 5.4.1   | Connection Error Handling               | 2     |
| 5.5     | Extending HTTP/2                        | 2     |
| 6.1     | DATA                                    | 3     |
| 6.2     | HEADERS                                 | 4     |
| 6.3     | PRIORITY                                | 2     |
| 6.4     | RST_STREAM                              | 3     |
| 6.5     | SETTINGS                                | 3     |
| 6.5.2   | Defined SETTINGS Parameters             | 5     |
| 6.5.3   | Settings Synchronization                | 2     |
| 6.7     | PING                                    | 4     |
| 6.8     | GOAWAY                                  | 1     |
| 6.9     | WINDOW_UPDATE                           | 3     |
| 6.9.1   | The Flow-Control Window                 | 3     |
| 6.9.2   | Initial Flow-Control Window Size        | 3     |
| 6.10    | CONTINUATION                            | 6     |
| 7       | Error Codes                             | 2     |
| 8.1     | HTTP Request/Response Exchange          | 1     |
| 8.1.2   | HTTP Header Fields                      | 1     |
| 8.1.2.1 | Pseudo-Header Fields                    | 4     |
| 8.1.2.2 | Connection-Specific Header Fields       | 2     |
| 8.1.2.3 | Request Pseudo-Header Fields            | 7     |
| 8.1.2.6 | Malformed Requests and Responses        | 2     |
| 8.2     | Server Push                             | 1     |
| **Total** |                                       | **95** |

The thirteen `6.x` rows are the 42 cases that were silently unrun; the other
eighteen rows are byte-for-byte the table above. This set is pinned in
`test/conformance/h2spec/h2spec.go` as the 31-entry `expectedSuites` roster
(keyed on the `<testsuite>` **`package`** attribute — ⚠️ `id` values COLLIDE
across families, e.g. `id="6.1"` appears on both `http2/6.1` and `hpack/6.1`),
mapped to the per-suite minimum case counts above, and enforced by three guard
layers: (1) every declared selector must match ≥ 1 case; (2) roster membership
**in both directions** — every rostered suite must run, and every `http2/*` suite
that runs must be rostered; (3) per-suite minimums. ⚠️ **The roster and its
minimums are properties of the PINNED DIGEST, not of envoy-go — they change only
via the refresh procedure at `:7-18`, never by an in-place edit to make a red
gate green.**

⚠️ **The section 6.6 (PUSH_PROMISE) exclusion recorded above is VACUOUS**: the
pinned image ships **no `http2/6.6` suite at all** (`--dryrun -S http2/6` lists
none), so the exclusion excludes nothing and never did. The `h2spec.go` comment
is RETAINED — rewritten to state the measured vacuity — as documentation of
ADR-0051's intent; re-adding an `http2/6.6` selector would match zero cases and
trip guard layer 1.

### Run record (2026-08-10) — the repaired gate at the phase-85 IMPL

```
95 tests, 94 passed, 1 skipped, 0 failed
```

The one skip is invariantly **6.9.2/2** ("window size to be negative") — the same
case in every run made at this pin. Measured five times at the phase-85 IMPL tip
with zero variance (~6.6 s inner, ~9 s warm). Subject: envoy-go at the phase-85
IMPL, booted from the harness's synthetic h2c bootstrap. The gate is enrolled in
the `differential` job of `.github/workflows/ci.yml` and runs per push from this
commit forward; before it, the conformance gate ran in **no** CI job at all.

### Reference observation (2026-08-09) — RECORDED, NEVER COPIED INTO EXPECTATIONS

The pinned reference `envoyproxy/envoy:contrib-v1.37.2`, booted on the SAME
bootstrap shape, ran the corrected strict set **twice** at **95 tests, 82 passed,
1 skipped, 12 failed** on both runs. Its section-6 failures are **four —
{6.3/1, 6.7/2, 6.9.1/2, 6.9.1/3}** — **fully DISJOINT** from the four the subject
carried before this repair ({6.5.2/1, 6.5.2/3, 6.5.2/4, 6.9.2/1}); each side
passes the other's failures. The reference's **twelfth failing slot FLIPS within
section 8 across runs** (8.1.2.1/3 on one run, 8.1/1 on the other) while the
subject is zero-variance at n≥3.

⚠️ **Method caveat**: the reference was measured container-to-container over a
docker bridge network with host-gateway addressing, not over the subject
harness's path — the two are not a controlled A/B on transport. ⚠️ **These are
OBSERVATIONS, recorded here for audit only. RFC 9113 MUSTs bind the subject even
where upstream Envoy fails them; no reference result is ever copied into a gate
expectation, a roster entry or a minimum.**
