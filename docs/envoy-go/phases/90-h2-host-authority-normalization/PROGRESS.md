# Phase 90 — `h2-host-authority-normalization` — PROGRESS

Append-only. One entry per lifecycle stage.

---

## BRAINSTORM — done 2026-08-19

**Base master:** `6b4bc7c0` · **Branch:** `phase-90-brainstorm` · **lifecycle-state DONE -> 1**
**Execution style:** subagent-driven per `feedback_execution_style` — three probe agents on disjoint
detached worktrees, disjoint port bands, private scratch each, **each committing nothing and each proving
its tree clean**; the controller ran the sentinel battery, the counts, and **re-derived every load-bearing
agent claim by execution**.

### What landed

- `docs/envoy-go/phases/90-h2-host-authority-normalization/BRAINSTORM.md` (new)
- `docs/envoy-go/phases/90-h2-host-authority-normalization/PROGRESS.md` (this file, new)
- `docs/envoy-go/ROADMAP.md` — row 90 registered `in-progress`, numstat **`1 0`** (a pure insertion),
  239 -> 240 lines, 121 -> 122 data rows, sentinel `want` **121 -> 122** in the SAME commit
- `docs/envoy-go/STATE.md` — rolled IN PLACE
- `next-prompt.txt` — rolled (`git add -f`)

**Docs-only: ZERO production `.go`, ZERO test `.go`.** `DECISIONS.md` and `BEHAVIOR_CONTRACT.md`
**BYTE-UNTOUCHED** — a BRAINSTORM adds no ADR. Next-free stays **`ADR-0312`**; the strict
`^> **STATUS: PROPOSED` guard **STAYS AT 0** and the SPEC re-arms it 0 -> 1.

### The pick

**`h2-host-authority-normalization`**, SELF-PICKED per the 2026-07-12 standing directive. Four candidates
costed at this tip, each by a built-run-and-reverted prototype:

| candidate | production cost | files | packages | verdict |
|---|---|---|---|---|
| **`host`/`:authority`** | **+15 / −1** | **1** | **1** | **PICKED** |
| decode-side trailers | +37 / −4 | 3 | 1 | rejected — Lua hook needs coroutine rework |
| ADR-0310 C1 drain | +161 / −31 | 3 | 1 | deferred — best NEXT row |
| ADR-0310 C2 `max_request_headers_kb` | +76 / −2 | 5 | 3 | deferred — must not go before C1 |
| ADR-0310 C3 SETTINGS | +31 / −19 | 3 | 1 | **REJECTED — measured ANTI-parity** |

### Sentinel — ACTUAL output, both sides of the row add

| | check (1) | check (2) | check (3) |
|---|---|---|---|
| **BEFORE** (`want=121`) | SILENT | SIX at `:199 :205 :211 :221 :227 :235` | SILENT |
| **AFTER** (`want=122`) | `NOT DONE: row 90` | SIX at `:200 :206 :212 :222 :228 :236` | SILENT |

⇒ **SENTINEL DOES NOT FIRE. `stop` NOT created** (verified absent at the git root and in the stage
worktree, both sides). Window COUNT and CONTENT unchanged; only anchors shift +1.

⚠️ **A DRAFT OF BRAINSTORM §8 PREDICTED "(1) SILENT" AFTER THE ADD AND WAS WRONG.** Row 90 is
`in-progress`, so check (1) MUST name it. Corrected to the measured output. **Caught only by running the
gate, not by reasoning about it.**

**All four NCs fired on the post-add file.** ⚠️ **NC-A CHANGES SHAPE while a row is in-progress**: it now
reads `NOT DONE: row 62` **and** `NOT DONE: row 90`, not row 62 ALONE. Both lines are required — row 62
proves the check is live, row 90 proves the add landed. NC-B `GATE FAIL: examined 122 data rows, expected
121`; NC-C residual 2 -> 0 then `NEVER OPENED: gRPC` with WASM silent; NC-D long **5** / short **1** /
union **6**.

### Counts at this close — re-derived mechanically, none copied

`ROADMAP.md` **240 / 122 rows** · `DECISIONS.md` **18277**, tail **ADR-0311**, next-free **ADR-0312**
(`grep -c '^## ADR-0312'` => 0), `^---$` **216**, headings **310**, strict `PROPOSED` guard **0** ·
`BEHAVIOR_CONTRACT.md` **5962** · `STATE_HISTORY.md` **506 -> 508** · `BOOTSTRAP_PROMPT.md` **522** ·
phase dirs **130 -> 131** · fixtures **121**, tail `0119-grpc-unary-trailers`, **`0120` FREE** ·
blank imports **121** (narrowed form; the unnarrowed reads 123 and is REFUTED) · fuzzers **55** ·
BackendKind tail **38** · stat surface **406** · slice-only-writer gate **6** · `-family row` **95 / 67** ·
`gRPC-family row` **2** · `Operational-tooling-family row` **3** · REVIEW.md **37 FILES**.

### Findings the next stage must not re-learn

1. **The phase-89 codec census is prose-contaminated AND structurally blind.** Config-only reads
   HTTP1 **212** / AUTO **2** / HTTP2 **0** / HTTP3 **0**, not 270/6/3; and the YAML view misses **46 of
   121** fixtures that build config in their Go driver. **The H2-capable downstream set is FOUR fixtures**
   — `0004`, `0079`, `0080`, `0119`.
2. **The differential arm CANNOT use `helpers.H2RoundTrip`**, proven at `x/net@v0.34.0`
   `http2/transport.go:2162` (drops a client-set `host`) and `:2146` (always synthesizes `:authority`).
   The instrument exists in-tree at `0119`'s driver.
3. **No existing test pins the defect** — controller-verified with a green baseline first, then green with
   the prototype across **69 packages**.
4. **ADR-0310 C3 is measured ANTI-parity** — the reference advertises no `0x6` either, with or without
   `max_request_headers_kb` set. Three documents file it as deferred parity; all three are wrong.
5. **The "~64 KiB encoded band" in ADR-0310 §Consequences (xi) is not reproducible.** RECORDED, NOT FIXED.
6. **A drift CORRECTION fired and was itself refuted** — the stat figure **406** was reported stale
   ("403"); 403 is a LINE count scoped to `internal/`, the canonical command counts occurrences repo-wide
   and reads 406. **The carried figure was right.**
7. **The ARM-A malformed-row figure reconciles only under an ESCAPE-AWARE field count** (naive reads 17).
8. **`STATE_HISTORY.md`'s "archive labels 202" is not reproducible** by any of six matcher forms. Carry no
   number; use the anchored form.
9. **Every port band this loop has assigned since phase 87 sits inside the kernel ephemeral range**
   (`32768 60999`) — which is also the mechanism behind the "driver-owned receiver port race" flake.

### Probe hygiene

All three probes: nothing committed, nothing pushed, every patched tracked file restored with
`sha256sum -c` verified, `git status --porcelain` and `git diff --stat 6b4bc7c0` both EMPTY, containers
torn down BY NAME, worktrees removed. Foreign containers (`infallible_booth`, `crazy_kare`, `golink-ai`,
`quizzical_goldstine`) deliberately left untouched.

⚠️ **A probe's `pgrep -f` matched and killed a SIBLING probe's process** (PID 2870478). The controller
issued an advisory; the victim discarded that arm entirely and re-ran every dependent arm with
`ss -ltnp` asserted before and after — **all re-run results held**. One flake observed:
`TestAcquireH2Stream_PromoteSkipsDrainingConn`, **6/6 green on retry**, and it fired **without `-race`**,
which widens the recorded `internal/cluster` outlier class.
