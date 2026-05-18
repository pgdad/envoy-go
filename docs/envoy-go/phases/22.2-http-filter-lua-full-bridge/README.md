# Phase 22.2 — `http-filter-lua-full-bridge` (placeholder)

**Status:** Sub-phase directory pre-created at the **phase-22 parent BRAINSTORM** per Q12 (see `../22-http-filter-lua/BRAINSTORM.md`). This sub-phase is **not yet opened** — opening happens at the dedicated 22.2 SPEC session after the 22.1 IMPL squash-merges to master + the parent SPEC at `../22-http-filter-lua/SPEC.md` lands.

**Parent row:** `22 | http-filter-lua` (status `in-progress` per ROADMAP).
**This sub-row:** `22.2 | http-filter-lua-full-bridge` (status `planned` per ROADMAP; depends-on `22.1`).

## Anticipated scope (per parent BRAINSTORM §11.2)

Sub-phase 22.2 delivers the full Envoy↔Lua bridge-API delta on top of 22.1's pragmatic-middle. The anticipated 22.2 surface:

- **`:body()` + `:bodyChunks()` body-access surface** — interacts with phase-13 ADR-0128 decode-side body-buffering primitive. 22.2 SPEC settles the exact interaction discipline (including coroutine-vs-goroutine-resume choice).
- **`:trailers()` trailer-access surface** — request + response trailers.
- **`:metadata()` dynamic-metadata bridge** — likely PARSE-REJECT or partial-deferral per the project's cross-phase dynamic-metadata-deferral discipline (deferred at phases 16 / 17 / 18 / 19 / 20).
- **`:connection()` connection-info bridge** — SSL/TLS access integrating with phase-03 TLS primitives.
- **`:httpCall()` outbound HTTP call** — reuses phase-20 `internal/httpclient/` framework primitive at first co-consumer (validates the phase-20 extraction).
- **Crypto / base64 / sha helpers** — `:base64Escape()` + `:base64Decode()` + `:sha256()` + `:sha512()` + `:importPublicKey()` + `:verifySignature()`.
- **`:fileBytes()` file-read helper** — security caveats apply (sandbox concern; 22.2 SPEC settles).
- **`:timestamp()` time helper** — non-deterministic; fixture cross-side byte-exact challenges.
- **Full `:streamInfo()` surface** — adds `:upstreamHost` + `:upstreamCluster` + `:dynamicMetadata` + `:dynamicTypedMetadata` + `:requestedServerName` + `:filterState` + `:downstreamSslConnection` to the 22.1 subset.
- **Additional stat-surface entries** — httpCall counters likely (`httpcall_total` + `httpcall_failures`).
- **Differential fixture `0027-http-lua-full-bridge`** — partial cross-side / REFERENCE-LESS fallback for non-deterministic scenarios per parent BRAINSTORM §6.4.

## Anticipated artefacts (filled in at this sub-phase's own session-set)

- `BRAINSTORM.md` — (optional) sub-phase BRAINSTORM if the parent's design coverage is insufficient for the bridge-API-delta detail
- `SPEC.md` — authored at the dedicated 22.2 SPEC session
- `PLAN.md` — authored at the dedicated 22.2 PLAN session
- `PROGRESS.md` — per-task entries authored during 22.2 IMPL session(s)
- `REVIEW.md` — authored at 22.2 IMPL phase-done

## D-hypothesis (provisional; 22.2 BRAINSTORM re-evaluates)

Anticipated 22.2 IMPL ADRs: ~2-4 NEW ADRs (full-bridge-API shape + httpCall dispatcher + body-buffering-interaction + dynamic-metadata-bridge-deferral). 22.2 BRAINSTORM re-evaluates the WEAK / STRONG / BREAK hypothesis disposition after the 22.1 IMPL outcomes are known.

## Cross-references

- `../22-http-filter-lua/BRAINSTORM.md` — parent BRAINSTORM
- `../22.1-http-filter-lua-vm-and-headers-bridge/README.md` — predecessor sub-phase
- `../22.3-http-filter-lua-multi-script-and-per-route/README.md` — successor sub-phase

**This README is descriptive of anticipated scope, not prescriptive of design.**
