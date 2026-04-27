# Phase 06.2 — Access log (placeholder)

This directory was created by the planner-time split of phase 06 (see `DECISIONS.md` ADR-0045 and `BOOTSTRAP_PROMPT.md` §6.2). It awaits a SPEC.md drafted by `superpowers:brainstorming` (or `superpowers:writing-plans` per the lifecycle-state-1 routing) at the entry of 06.2's lifecycle.

**Master design document:** `docs/envoy-go/phases/06-observability-baseline/SPEC.md` (this commit). The phase-06 parent SPEC remains the authoritative cross-cutting design for both 06.1 and 06.2; sub-phase SPECs carve coherent slices of the parent's deliverables. The brainstorm-close artifact `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` (master `75a6bf9`) is the upstream design source — 06.2's eventual SPEC distills the brainstorm's access-log scope into formal SPEC shape, just as `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` does for 06.1's stats-Prometheus scope.

**Sub-phase 06.2 scope** (per the parent SPEC §2 + BRAINSTORM §1's split table):

- `internal/accesslog/` — new package: file sink (open / append / fsync discipline; rotation NOT in scope at 06.2 — stays a future phase), Envoy default-format formatter (the `[%START_TIME%] "%REQ(:METHOD)% %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% %PROTOCOL%" %RESPONSE_CODE% ...` shape), async writer (channel-buffered goroutine that drains the format-output to the file sink with bounded backpressure).
- HCM access-log emit hooks: at request finalization, the HCM filter formats one access-log record per request and submits to the async writer. The hook lands in `internal/filter/hcm/filter.go` alongside (but separate from) the 06.1 stat-emit response hook.
- Bootstrap proto: HCM `access_log[]` field is now read; the existing silent-ignore (per ADR-N) is amended for the `envoy.access_loggers.file` typed-config carrier specifically. Other access-log types (`envoy.access_loggers.stdout`, `envoy.access_loggers.tcp_grpc`, `envoy.access_loggers.open_telemetry`) remain silently-ignored — those are Observability-family deliverables.
- `test/differential/0006-access-log/` — NEW fixture: `envoy.yaml`, `envoy-go.yaml`, `expectations.yaml`, `README.md`, `driver/driver.go` + `driver_test.go`, plus a per-fixture access-log file location convention (probably `test/differential/0006-access-log/logs/{subject,reference}.log` or runner-managed temp dirs — the 06.2 SPEC settles). Workload: identical sequential request stream against both proxies; per-side access-log file scraped + parsed; per-record field-mapping equivalence asserted. Format-string diffs (e.g., `START_TIME` non-determinism) handled via the access-log field-mapping ignore-list.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md ## Access log field mapping` subsection: POPULATED with the field-by-field mapping table (Envoy's default-format fields → envoy-go's emitted-field names + format-normalization rules + ignore-list for non-deterministic values like timestamps + connection IDs + durations). Parallel to 06.1's population of `## Stat-name mapping` per ADR-0061.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md ## Equivalence Matrix`: the existing forward-looking row `Access log records | Semantically equal after field-mapping` becomes load-bearing — the 06.2 SPEC settles its concrete definition (the "Semantically equal" predicate) and the new row references the populated `## Access log field mapping` subsection for its allow-list / tolerance column.

**ADRs anticipated in 06.2** (sequential numbering after 06.1's tail; 06.1 lands ADR-0059..ADR-0064 per its SPEC §8, so 06.2's first-free is ADR-0065 if no intervening phase reshuffles; the planner re-verifies next-free at write time per ADR-0004's autonomous-numbering rule). Anticipated topics:
- **Access-log architecture** — async-writer + file-sink discipline; no third-party access-log library (mirrors 06.1's ADR-0059 architectural shape rationale).
- **Format-string fidelity vs. semantic equivalence** — the 06.2 differential equivalence claim's exact shape: byte-equal records vs. field-by-field semantic equality after format-normalization (the parallel of 06.1's ADR-0062 "behavioral delta" choice, settled per the BRAINSTORM's access-log analog of §2.5).
- **Access-log field allow-list** — codifies the per-field ignore-list (timestamps, connection IDs, durations) into `BEHAVIOR_CONTRACT.md`'s populated `## Access log field mapping` subsection.
- **Optional: `internal/accesslog`-emits-stats hook** — if the 06.2 brainstorm decides to emit per-access-log buffer-pressure gauges through 06.1's stats Registry, an ADR records the cross-sub-phase coupling (the BRAINSTORM §1 footnote flagged this as "optional").
- **Async-writer backpressure policy** — what happens when the async writer's buffer fills (drop oldest record? block the request handler? shed records with a counter?). The 06.2 brainstorm decides; the ADR records the choice + rationale.

**Depends on:** 06.1 (lifecycle ordering — 06.1 ships first per BRAINSTORM §1; 06.2 may optionally consume 06.1's `*stats.Registry` for buffer-pressure gauges, but no code-surface dependency is required by the SPEC stub above).

**OUT of 06.2** (deferred to later phases):
- Log rotation (size-based / time-based) — out of 06.2; future phase or operational tooling.
- Other access-log sinks (`envoy.access_loggers.stdout`, `envoy.access_loggers.tcp_grpc`, `envoy.access_loggers.open_telemetry`, gRPC ALS) — Observability-family deliverables.
- Per-route access-log filters (the bootstrap proto's `access_log_filter` field) — phase 07's filter-chain framework.
- Custom format-string parsing (non-default formats) — out of 06.2; the 06.2 SPEC ships only the Envoy default format. Custom-format support is a future phase or a future Observability-family extension.
- Trailers in access logs — gRPC family.

**Differential surface at end of 06.2:** NEW fixture `0006-access-log` differentially green (gate (a) is non-vacuous for the second time on the observability surface). Pre-existing fixtures (`0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing`, `0005-prometheus-stats`) remain green (gate (b)). Conformance gate (c) at the ADR-0051 pin re-runs at 53/53 PASS (06.2 doesn't touch H2 wire code). Fuzz (d) re-runs the seven fuzzers from 06.1 and may add an access-log-format fuzzer (the 06.2 brainstorm settles). Build/lint/test (e) and REVIEW (f) apply normally.

**Phase-done rollup:** at 06.2's phase-done commit, ROADMAP row `06.2` flips `in-progress → done` AND row `06` (parent) flips `in-progress → done` AT THE SAME COMMIT, mirroring the 05 / 05.1 / 05.2 closure pattern recorded in `STATE.md` at master `75a6bf9` and in the parent SPEC §5. The phase-done commit's commit-message body must explicitly name both ROADMAP-row transitions so the rollup is grep-verifiable from `git log`.

---

When `superpowers:brainstorming` next runs against this directory, it produces `SPEC.md` here that authoritatively defines 06.2's scope (this README is illustrative; the SPEC supersedes it). Brainstorming for 06.2 should not start until 06.1 reaches `done` (06.1's `internal/stats.Registry` is the foundation 06.2 may optionally consume for buffer-pressure observability, and 06.1's BEHAVIOR_CONTRACT in-place edit pattern is the precedent 06.2 mirrors when populating `## Access log field mapping`).
