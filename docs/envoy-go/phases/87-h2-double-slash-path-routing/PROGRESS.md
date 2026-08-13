# PROGRESS — phase 87 (h2-double-slash-path-routing)

## BRAINSTORM — done 2026-08-12

## What landed

Docs-only: **ZERO production `.go`, ZERO test `.go`.** `docs/envoy-go/phases/87-h2-double-slash-path-routing/BRAINSTORM.md` (new) · `ROADMAP.md` **row 87 registered `in-progress`** (236 -> 237 lines, 118 -> 119 data rows; the row sits at `:149`) · `next-prompt.txt` sentinel **`want` 118 -> 119 in the SAME commit as the row** (via the roll to the phase-87 SPEC) · `STATE.md` rolled **IN PLACE** · `STATE_HISTORY.md` appended (the unique-oldest §Recent entry evicted at the five-entry cap), verified STRICTLY APPEND-ONLY. `DECISIONS.md` and `BEHAVIOR_CONTRACT.md` **BYTE-UNTOUCHED**; this stage adds **NO ADR** (next-free stays ADR-0309; strict `PROPOSED` guard stays 0 — the phase-87 SPEC re-arms it).

## Method

**SELF-PICKED** per the 2026-07-12 standing directive; no banked mid-lifecycle work existed (phase 86 CLOSED, all 118 rows `done` — check (1) was SILENT at `want=118`, the third silent reading in project history, before this stage re-opened it at row 87). ⚠️ **NAMED DEPARTURE: no investigation agents this stage** — the probes (a tip binary built with `-o` into session scratch; four `curl --http2-prior-knowledge --path-as-is` executions over h2c; two `net/url` `go run` differentials; code reads and greps at `638ef78a`) were run INLINE by the controller; nothing in the repo tree was edited by any probe. Ports 47400-47402 template values only.

## The stage's headline

**envoy-go's hand-rolled H2 downstream codec mis-parses a leading `//` in the origin-form `:path`, and it fails two ways — reproduced by execution at this tip.** `internal/filter/hcm/h2/stream.go`'s `buildRequest` parses `:path` with `url.Parse` (a generic RFC-3986 URI parser), which reads a leading `//` as a network-path reference and peels the authority into `u.Host`, corrupting `u.Path`. Measured end-to-end over h2c (positive control `GET /` -> `routed-ok` 200): `GET //foo` -> **404** (empty path, routing miss); `GET //foo/bar` -> **silent mis-route to `/bar`** (leading segment swallowed as authority, served against the wrong path with no error); mid-path `/a//b` -> 200 (unaffected). ⚠️ **The defect is H2-specific**: the identical `//foo` over HTTP/1.1 returns `routed-ok` 200 (H1 is served by Go `net/http`), and H3 delegates request-target parsing to the `quic-go`/`http3` library — so the blast radius is exactly ONE production site (the `url.Parse(path)` call in `buildRequest`). The fix primitive is `url.ParseRequestURI` (measured at this tip: preserves `//foo`, keeps asterisk-form `*`, splits the query), to be settled at the SPEC by prototype.

## The pick

**Row 87 `h2-double-slash-path-routing`, a core-HCM / HTTP-routing MAINTENANCE row claiming NO family ordinal** (the row-85/row-86 precedent — a maintenance row repairs a landed deliverable and does not extend a charter). Provenance: the phase-74 BRAINSTORM sweep's *"Newly surfaced this session and NONE chartered:"* prose (historical, NOT a live `candidates:` sentence, deliberately untouched), carried in `STATE.md`/`next-prompt.txt`'s documentary-defects list — OUTSIDE the six family windows, so nothing narrows at row-done. Rejected with re-derived costs (BRAINSTORM §2.1): the CONTINUATION two-sided repair (strongest KNOWN product defect but 2-4x row 85, gate does not exist — the natural next LARGE row, charter as a split phase); the stat-surface recount (cheapest but changes nothing real; rides a future +0 row); `ssl.connection_error` (+444 whole-`.go` floor); `test/conformance/grpc/` (9/26 reachable); REVIEW.md restoration (process-not-product); the D-86-CONN `client.Close` gate (~10 test-only lines, too thin — a fold-in); hygiene fold-ins (thin, test/process only).

## Sentinel (measured, before AND after the ROADMAP edit)

BEFORE at `want=118` (repo tip): (1) SILENT · (2) SIX `:196 :202 :208 :218 :224 :232` · (3) SILENT. AFTER at `want=119` (worktree, row registered): input **237 lines / 119 data rows** · (1) **`NOT DONE: row 87`** (the single expected line while open) · (2) SIX, anchors shifted uniformly +1 to **`:197 :203 :209 :219 :225 :233`** (the single inserted line at `:149` sits above all six windows) · (3) SILENT. Conjunction fails ⇒ **does NOT fire**; `stop` NOT created (checked at the repo root AND the worktree, both sides). All four doctoring NCs fired both sides:
- **row-62 doctoring** (`NC LANDED? [ in-progress ]` inspected first): BEFORE -> `NOT DONE: row 62` alone (the silent side still LOOKS); AFTER -> `NOT DONE: row 62` AND `NOT DONE: row 87`.
- **want off-by-one**: BEFORE `want=117` -> `GATE FAIL: examined 118 data rows, expected 117`; AFTER `want=118` -> `GATE FAIL: examined 119 data rows, expected 118` (plus `NOT DONE: row 87`).
- **check-(3) doctoring** (residual 2 -> 0 confirmed first): `NEVER OPENED: gRPC` fired, `WASM` correctly silent.
- **check-(2) one-arm**: long arm alone **5**, short arm alone **1**, union **6** (a one-arm strip is not an NC for the union).

Leak axes (whole-file count, BEFORE -> AFTER): lines **236 -> 237** · data rows **118 -> 119** · union **6 -> 6** · `-family row` **95/67 -> 95/67** (the row's MAINTENANCE phrasing deliberately does not match check (3)'s `<slug>-family row` pattern — rows 76/78/85/86 precedent) · `gRPC-family row` **2 -> 2** · `Operational-tooling-family row` **3 -> 3** · new-slug occurrences **2**, both inside row 87's own line (the `| h2-double-slash-path-routing |` id cell and the phase-dir path). The ARM-A malformed-row figure {119, 131} is UNAFFECTED — those rows sit above the `:149` insertion point (not re-run with the fragile escape-aware command; the binding leak-invariance proof is the whole-file counts above).

## NEXT

**SPEC** — dispose the seven §4 open questions BY EXECUTION, centered on a compiling test-green prototype of the `buildRequest` fix (Q1 — `url.ParseRequestURI` vs manual construction; enumerate origin/asterisk/`#fragment` forms) and the reference-container re-verification of the `//foo` / `//foo/bar` expectation (Q4); the differential proof shape and literal-`:path` injection (Q5, the load-bearing cost question — `HTTPExpectations` is H1-only, an H2 arm needs `Drive` hooks); draft `ADR-0309 §Context` STATUS `PROPOSED` (re-arms the strict guard 0 -> 1); enumerate cost by prototype (the §4.1 floor `~2-10 prod / ~120-400 test` is a FLOOR — the tenth consecutive lower-bound firing closed the phase-86 lineage).
