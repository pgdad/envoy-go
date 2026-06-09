# Phase 32.2 SPEC — the full single-route command set + the per-command/splitter/cluster stat roster + the `redis.` Prometheus arm + the differential command matrix + the 41st fuzzer + the parent-row-32 ROLLUP

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 32.2** (`network-filter-redis-commands-and-stats`), the SECOND (final) sub-phase of the phase-32 BRAINSTORM-time 2-way pre-split (32.1 / 32.2). It is authored per the phase-22.2 / 25.x / 28.2 / 29.2 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/32-network-filter-redis-proxy/SPEC.md`) resolved the BRAINSTORM §10 D32-1..D32-8 empirical pins live against the contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` + go-control-plane `/envoy` v1.32.4 + upstream Envoy v1.37.2 source (parent §11; AMEND-R1..R9), and the **32.1 SPEC + IMPL** landed the upstream-pool seam (ADR-0230), the RESP codec, the `redisproxy` package foundation (the 10th built-in), the 10-name eager downstream roster (the 4 gauges CREATED-not-incremented), the PING/AUTH local-reply set, the `TCPRedisResponder` BackendKind (32), the `0055`/`0056` fixtures, and the flat `GET /stats` admin endpoint. This 32.2 SPEC **INHERITS** the parent SPEC's §5 proto roster + §6 PARSE-REJECT roster + §7 stat surface + §11 empirical-pin block AND the 32.1 SPEC's §3.5 codec + §4 seam + §7 roster, **resolves the parent's 32.2-owned D-questions** (D-P32-2 the exact per-command table + the mirrored-vs-per-side upstream counter set; D-P32-6 the full local-reply extent; D-P32-7 the IsValidName disposition; D-P32-8 the gauges' differential assertion design; D-P32-9 the upstream-pool per-side asymmetry), and **runs a focused SPEC-time source pin** (§12 — the exact 180-command `SupportedCommands` table + the success/error semantics + the gauge inc/dec call sites + the local-reply behaviors, transcribed from upstream Envoy v1.37.2 source; NO new docker probes — the parent §11 live-probe block is the authoritative empirical record). It anchors **NO new ADR** (ADR-0229's §Decision/§Consequences 32.2 half completes IN-PLACE; ADR-0230 is ACCEPTED). The next session, per BOOTSTRAP §5, authors the **32.2 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Complete `redis_proxy` to full single-route observability — wire the EAGER 180-command per-command stat roster (`command.<cmd>.{total,success,error}`) + the 2 `splitter.*` counters + the 3 `REDIS_CLUSTER_STATS` counters + the 2 connection-lifecycle gauges' (`downstream_cx_active`/`downstream_rq_active`) inc/dec into the `Handle` pump; add the `ECHO`/`TIME`/`QUIT`/`HELLO` local-reply commands; land the `redis.` LABEL-HOISTED Prometheus tag-extractor arm at `internal/stats/name.go`; extend `0055` with a differential command matrix (and the `TCPRedisResponder` reply table); add the 41st fuzzer `FuzzRESPDecode`; and roll the parent row 32 `in-progress → done` ATOMICALLY with sub-row 32.2 (ADR-0229 §Decision/§Consequences body completes PARTIAL → ACCEPTED).

**Architecture:** A `redisproxy`-package + `internal/stats/name.go` + test-surface change ONLY (the 29.2 "consumer sub-phase" shape — ZERO framework touch; the seam (ADR-0230) is consumed unchanged). The 32.1 `Handle` pump's "else → proxy" branch gains: (a) a supported-commands table lookup (unknown → `splitter.unsupported_command` + `-ERR unknown command` local error; recognized → `command.<cmd>.total` Inc); (b) a minimal arity check (bad-arity → `splitter.invalid_request` + `-invalid request`); (c) the per-command success/error increment on the reply (single-server semantics — ANY reply → `success`, only a pool/transport failure → `error`); (d) the `downstream_rq_active` gauge inc/dec around the request lifecycle. The 32.1 `Handle` entry/exit gains the `downstream_cx_active` gauge inc/dec. The 32.1 local-reply set (PING/AUTH) extends to ECHO/TIME/QUIT/HELLO (the empirically-pinned behaviors — §3.3). `internal/stats/name.go` gains the `redis.<prefix>.<rest>` LABEL-HOISTED arm (the mongo `.rbac.` shape — single `envoy_redis_prefix` label). ZERO changes to `internal/filter/network/` framework files, `upstreampool.go`, `manager.go`, `tcp_proxy`, or HCM.

**Tech Stack:** Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); go-control-plane `/envoy` v1.32.4 (ADR-0008). Consumes the as-built `internal/filter/network/redisproxy/` package (32.1 — extended in place), `internal/filter/network/upstreampool.go` (the seam — UNCHANGED), `internal/stats/` (counters + gauges + `IsValidName` + `name.go` tag-extractor — the `redis.` arm added), the differential harness + `fixture.StatsAsserter` + the flat `GET /stats` scrape (the 32.1 `0055` mechanics). **ZERO new go.mod dependencies** (redis_proxy is CORE `/envoy v1.32.4`; the RESP codec is in-house — AMEND-R1).

**Authored:** 2026-06-09. **Empirical-pin probe date (inherited):** 2026-06-08 (parent SPEC §11; NOT re-executed). **SPEC-time source-pin date:** 2026-06-09 (§12 — upstream Envoy v1.37.2 C++ source transcription; NO docker probes).

---

## 1. Purpose / Mission

Phase 32.2 delivers the redis_proxy command set + the full stat roster + the differential matrix + the family rollup (parent §3.2 item "32.2"):

1. **The full single-route command set.** Every non-local command is routed via `catch_all` to the one upstream cluster and forwarded VERBATIM (the 32.1 "else → proxy" path — already functional). 32.2 adds the OBSERVABILITY around it: the supported-commands table lookup (recognized → per-command stats; unknown → `splitter.unsupported_command`), minimal arity validation (`splitter.invalid_request`), and the per-command success/error increment. The local-reply set extends from PING/AUTH (32.1) to ECHO/TIME/QUIT/HELLO (D-P32-6 full extent — §3.3).

2. **The per-command + splitter + cluster-pool stat roster (ADR-0229 32.2 half).** The EAGER 180-command `command.<cmd>.{total,success,error}` table (D-P32-2 RESOLVED — §12.1; the kafka-176 eager precedent), the 2 `splitter.*` counters, the 3 `REDIS_CLUSTER_STATS` counters, and the 2 connection-lifecycle gauges' inc/dec (`downstream_cx_active`/`downstream_rq_active`; D-P32-8 RESOLVED — §4.4). The 2 `*_bytes_buffered` gauges stay exist-at-0 (a coverage boundary — framework-buffer-managed upstream; §4.4). Stat surface **546 → 1091** (§7).

3. **The `redis.` LABEL-HOISTED Prometheus tag-extractor arm (ADR-0229 32.2 half; AMEND-R4).** A new arm at `internal/stats/name.go` detecting `redis.<prefix>.<rest>` → `envoy_redis_<rest flattened>{envoy_redis_prefix="<prefix>"}` (the mongo `.rbac.` HOIST shape — single label; the dynamic per-command names flatten into the metric name; §5). This replaces the 32.1 flat-`/stats` scrape with the `/stats/prometheus` label-aware comparison upgrade (D-P32-7 — §5.2).

4. **The differential command matrix + the 41st fuzzer.** `0055-redis-roundtrip` extended (same dir — cross-side-XOR-boot-reject constraint) with a command-spread + error/edge arms over the `redis.` `/stats/prometheus` lines + the per-command counters; the `TCPRedisResponder` reply table extended (`$-1` GET-miss, `:<n>` INCR); the deterministic local-reply arms (ECHO/QUIT/HELLO-error). The 41st fuzzer `FuzzRESPDecode` (no-panic / no-mutation / bounded-allocation on the `resp.go` decode path; §9).

5. **The parent-row-32 ROLLUP + the ADR-0229 body completion.** The ADR-0229 §Decision/§Consequences body completes (PARTIAL → ACCEPTED); the BEHAVIOR_CONTRACT 32.2 bundle lands; ROADMAP parent row 32 flips `in-progress → done` ATOMICALLY with sub-row 32.2 `planned → done` (the 18/19/22/24/25/26/28/29 ROLLUP precedent per ADR-0106(d)).

After phase 32.2 the project has a FULLY observable single-route `redis_proxy`: the full command set with per-command/downstream/upstream stat parity, the `redis.` Prometheus arm, a cross-side differential command matrix, and the no-panic fuzzer. The §9 Network-filters family stays OPEN with ONE candidate ({thrift}) remaining — it reuses the 32.1 upstream-pool seam.

### 1.1 Parent AMENDs + 32.1 outputs load-bearing for 32.2

- **AMEND-R3** (the per-command roster is EAGER + table-bounded ~180; `command.<cmd>.{total,success,error}`; the `latency` histogram + `*_fault` counters DEFERRED; `enable_command_stats` no-op in v1.37.2) — the load-bearing input to §3.2 + §4.1 + §12.1.
- **AMEND-R4** (Prometheus LABEL-HOISTED `envoy_redis_<leaf>{envoy_redis_prefix="<sp>"}` — the mongo `.rbac.` shape) — §5.
- **AMEND-R5** (PING/AUTH [+ECHO/TIME/QUIT/HELLO] local-reply; zero upstream; count `downstream_rq_total` but NO `command.*`) — §3.3 + §4.2.
- **AMEND-R6** (the `upstream_cx_*`/`upstream_rq_*` traffic stats live under `cluster.<name>.*`, reused via the seam; only the 3 `REDIS_CLUSTER_STATS` are redis-specific upstream/pool counters) — §4.3 + §8.3.
- **AMEND-R7** (the fixed 15-name roster = 11 counters + 4 gauges; the 32.1 subset was 10; 32.2 adds the 2 `splitter.*` + 3 `REDIS_CLUSTER_STATS` = 5 fixed + the 2 lifecycle gauges' inc/dec) — §4.
- **AMEND-R9** (NO close-direction counters — close-direction-framework-zero-touch) — confirmed; §2.7.
- **32.1 outputs consumed:** the `resp.go` codec (`decodeRequest` returns the UPPERCASED cmd + raw frame; `decodeReply`); the `Handle` pump (`filter.go` — the local-vs-proxy dispatch + the cx/rq counter + byte-count increments); the eager 10-name roster (`stats.go` — the 4 gauges exist-at-0; the 2 `downstream_cx_drain_close`/`downstream_cx_protocol_error` counters exist-at-0); the `commands.go` `isLocalReply`/`localReply` (PING/AUTH); the `UpstreamConn` seam (UNCHANGED); the `0055` driver + `TCPRedisResponder` + the flat `/stats` scrape; the `internal/stats/name.go` arms (the kafka `kafka.` INLINE arm = the contrast; the mongo `.rbac.` hoist = the shape for `redis.`).

### 1.2 32.2-SPEC-additive contributions (what this document pins beyond the parent + 32.1)

- **§12.1 D-P32-2 RESOLVED: the EXACT 180-command table.** Transcribed from upstream Envoy v1.37.2 `supported_commands.h` + `command_splitter_impl.cc` (§12 — the 8 iterated `SupportedCommands` groups; full enumerated list). 180 distinct commands × 3 slots (`total`/`success`/`error`) = 540 per-command counters. The `latency` histogram + `error_fault`/`delay_fault` counters DEFERRED (ADR-0060 / faults).
- **§4.2 / §12.2 the success/error semantics.** Single-server (MVP) commands increment `command.<cmd>.success` on ANY reply (incl. a backend `-ERR` reply AND the `$-1`/`*-1` null sentinels); only a pool/transport FAILURE (dial error, decode error, conn dropped before a reply) increments `command.<cmd>.error`. `total` Incs once at dispatch. (Fragmented MGET/MSET reply-type-based stats are DEFERRED with multi-key fragmentation — §2.2.)
- **§4.4 / §12.3 D-P32-8 RESOLVED: the gauge inc/dec split + the differential design.** Only `downstream_cx_active` (Handle entry/exit) + `downstream_rq_active` (request received/reply sent) get filter-driven inc/dec. The 2 `*_bytes_buffered` gauges are connection-buffer-layer state set via upstream `setConnectionStats` — envoy-go's synchronous single-flight terminal holds no persistent buffer → they stay exist-at-0 (a coverage boundary). The differential mirror: `downstream_cx_active` LIVE via a held-open idle-connection arm (==1 both sides while held; ==0 post-close — the mongo `op_query_active` D-P9 precedent); the other three quiesced-zero (presence + ==0 post-workload) + unit-tested inc/dec.
- **§3.3 / §12.4 D-P32-6 RESOLVED: the full local-reply extent, empirically refined.** ECHO (echoes arg as bulk; arity≠2 → `splitter.invalid_request`) + QUIT (`+OK\r\n` then connection close) are deterministic differential arms; **TIME** (`*2\r\n…` from local wall-clock — NON-DETERMINISTIC) is a local-reply but UNIT-test-only (NOT a byte-equivalence arm); **HELLO** splits — its `>2-args` (`-ERR HELLO options…not supported`) + `bad/non-2 proto` (`-NOPROTO unsupported protocol version`, incl. `HELLO 3`) paths are deterministic local errors, but `HELLO 2`/no-arg falls through to the PROXIED path (it is a `ClusterScopeCommand` — server-identity-dependent reply; NOT a local reply, NOT a differential arm).
- **§5 / §12.5 the `redis.` LABEL-HOISTED arm.** The exact `internal/stats/name.go` site + the single-label hoist shape (contrast the kafka INLINE arm + the mongo MULTI-label hoist).
- **§5.2 / D-P32-7 RESOLVED: IsValidName satisfied BY CONSTRUCTION.** The 180-command table is STATIC (the kafka eager posture) → every `command.<cmd>.*` name is table-bounded → `stats.IsValidName` holds at table-build time WITHOUT a per-wire-byte guard (`reference_dynamic_stat_name_charset_guard` does NOT bite — a wire command not in the table never reaches `NewCounterIfAbsent`; it routes to `splitter.unsupported_command`). A `TestCommandRoster_AllValidNames` test asserts the property.
- **§8 / D-P32-9 RESOLVED: the upstream-pool per-side asymmetry pin.** The MVP's one-conn-per-downstream model vs the reference's shared-per-host pool → `cluster.<name>.upstream_cx_total` diverges ONLY when MULTIPLE downstream connections are live concurrently. The 32.2 command matrix uses ONE fresh connection per arm (sequential) → both sides dial once per arm; `upstream_cx_total` equality holds per arm. A concurrent-downstream scenario is NOT exercised (pinned per-side as a coverage boundary, not asserted equal).

---

## 2. Non-purposes

Phase 32.2 does NOT extend any subsystem beyond the minimum needed to complete the single-route command + stat surface under ADR-0229.

- **2.1 Multi-cluster routing OUT OF SCOPE.** `prefix_routes.routes[]` longest-prefix routing stays parse-accepted-behavior-deferred (only `catch_all_route` consumed — parent §2.1; the 32.1 config parse is unchanged). A future routing sub-phase.
- **2.2 Multi-key command fragmentation OUT OF SCOPE.** MGET/MSET/DEL/EXISTS/TOUCH/UNLINK split-and-collate (the `hashMultipleSumResultCommands`/`mget`/`mset` upstream handlers) is DEFERRED — these commands are PROXIED VERBATIM as single-server commands (no shard split, no reply collation). Their per-command counters exist + increment via the single-server path; the fragmented reply-type-based success/error accounting (parent §12.1 / the upstream `SplitKeysSumResultRequest` path) is NOT replicated. A future fragmentation sub-phase (it needs the seam's deferred deep pending queue + fan-out, §4.4 of the 32.1 SPEC).
- **2.3 Command-latency histograms OUT OF SCOPE.** `command.<cmd>.latency` (180 histograms) deferred per ADR-0060 (project-wide histogram deferral) — NOT created, NOT counted. The `error_fault`/`delay_fault` per-command counters (360 counters) defer WITH `faults` (parse-accepted-behavior-deferred). Coverage-boundary record in the 32.2 bundle.
- **2.4 The deferred active features stay deferred (parse-accept; parent §2.1).** sharding/`enable_redirection`/`enable_hashtagging`/`dns_cache_config`; downstream AUTH enforcement (the configured-password path — AUTH still answers the no-password error, AMEND-R5); `faults`; request mirroring; replica `read_policy`; the full `ConnPoolSettings` beyond `op_timeout`; `custom_commands`. Each parse-accepts standalone (the 32.1 config parse is UNCHANGED — §6). Their stat counters exist-at-0 (the 3 `REDIS_CLUSTER_STATS` — §4.3).
- **2.5 `op_timeout` enforcement OUT OF SCOPE.** Parsed + stored at 32.1; still NOT wired to a dial/round-trip deadline at 32.2 (the synchronous single-flight pump against a hermetic responder exercises no timeout path). Deferred to a future latency/pipelined sub-phase. BEHAVIOR_CONTRACT note (carried from 32.1).
- **2.6 The seam is UNCHANGED.** `internal/filter/network/upstreampool.go` (ADR-0230) is CONSUMED, not modified — the one-conn-per-downstream synchronous single-flight model is unchanged. The deferred shared-pool / deep-queue / two-goroutine pipelined model (32.1 SPEC §4.4) stays deferred (thrift / a latency sub-phase). NO ADR-0223 per-conn mutex.
- **2.7 Close-direction machinery NOT consumed (AMEND-R9).** No close-direction-keyed counters in `redis_proxy`; the 29.3 `CloseDirection` seam is untouched. `reference_close_direction_framework_gap` does NOT bite. `downstream_cx_drain_close` (the drain counter, exist-at-0 from 32.1) gets NO increment — the synchronous pump has no drain-close decision path at the MVP (QUIT's connection close is a clean local close, NOT a `cx_drain_close` increment — §3.3). `downstream_cx_protocol_error` (exist-at-0 from 32.1) gets its increment WIRED at 32.2 (the decode-error path — §4.5).
- **2.8 Runtime-key gating unmirrored** — envoy-go has no runtime layer; the filter behaves at proto-configured values (the §9 family boundary).
- **2.9 No new go.mod dep; no new built-in; no new BackendKind; no new fixture DIR; RESP2 only.** mongoproxy... redisproxy is already the 10th built-in; the `TCPRedisResponder` (32) is extended (reply table), not re-numbered; `0055` is extended (no new dir); RESP3 (`HELLO 3`) is rejected locally (`-NOPROTO`, §3.3) — envoy-go speaks RESP2. The framework is UNTOUCHED.
- **2.10 No close-direction / drain / connection-rate-limit / unknown-connection behavior.** The 3 `REDIS_CLUSTER_STATS` are CREATED (roster parity) but stay exist-at-0 (no drain, no redirection, no rate-limiting in the MVP — §4.3 / §12.6).

---

## 3. The command set + dispatch + local-reply extension (`redisproxy` package; 32.2)

The 32.1 `Handle` pump (`filter.go:106-137`) dispatches each decoded request: `isLocalReply(cmd)` → local; else → proxy. 32.2 enriches BOTH branches.

### 3.1 File touch (extends the 32.1 split)

| File | 32.2 change |
|---|---|
| `commands.go` | the static 180-command supported-commands table (`supportedCommands` set) + the extended local-reply set (ECHO/TIME/QUIT/HELLO) + the local-reply arity/validation helpers + the unknown-command + bad-arity dispatch decision |
| `stats.go` | the EAGER 180-command per-command roster (`command.<cmd>.{total,success,error}`) + the 2 `splitter.*` + 3 `REDIS_CLUSTER_STATS` counters + the per-command + splitter inc accessors + the 2 lifecycle gauges' inc/dec accessors |
| `filter.go` | the `Handle` pump wiring: the table lookup + arity check + the per-command total/success/error increment + the `splitter.*` increments + the `cx_active`/`rq_active` gauge inc/dec + the `downstream_cx_protocol_error` increment + the QUIT connection-close |
| `resp.go` | UNCHANGED (the decoder is whole; the fuzzer (§9) exercises it) |
| `config.go` | UNCHANGED (no new parse arms — §6) |
| `doc.go` | the 32.2 forward-pointers resolved (the command set + roster landed) |
| `fuzz_test.go` | NEW — `FuzzRESPDecode` (the 41st; §9) |
| `*_test.go` | per-file unit tests (§15) |

(The exact split is the 32.2 PLAN/IMPL's; this is the SPEC-anticipated shape. `internal/stats/name.go` gains the `redis.` arm — §5; the `0055` driver + `fixture.go` `TCPRedisResponder` extend — §8.)

### 3.2 The supported-commands table + per-command dispatch (`commands.go` + `filter.go`)

The static table `supportedCommands` (a `map[string]struct{}` or sorted slice; lower-cased names) holds the EXACT 180 commands of §12.1 (the 8 iterated `SupportedCommands` groups). Command-name matching is ASCII-case-insensitive (the decoder uppercases; the table is lower-cased — compare via `strings.ToLower` once at dispatch, or store the table UPPERCASED to match the decoder's output directly — D-S32.2-1). The pump's proxied-branch dispatch (AMEND-R3; §12.2):

1. **`isLocalReply(cmd)`** (PING/AUTH/ECHO/TIME/QUIT/HELLO-error-paths, §3.3) → local reply; NO `command.*` increment (these are NOT in the per-command table — §12.1 confirms PING/AUTH/ECHO/TIME/QUIT are singleton accessors outside the iterated groups; HELLO's local-ERROR paths return before any per-command total).
2. **command NOT in `supportedCommands`** → `splitter.unsupported_command` Inc + write the local error `-ERR unknown command '<NAME>', with args beginning with: \r\n` (byte-stable; the upstream wording, parent §11.5); ZERO upstream; NO `command.*` (none exists for an unknown name — the IsValidName-by-construction guarantee, §5.2). `<NAME>` is the AS-RECEIVED (not upper-cased) command token (the upstream echoes the wire bytes — D-S32.2-2 pins the exact casing/args-suffix wording against the reference).
3. **command in the table but BAD ARITY** → `splitter.invalid_request` Inc + write `-invalid request\r\n` (byte-stable; `ResponseValues::InvalidRequest`, parent §11.5); ZERO upstream; NO `command.<cmd>.total` (the arity check precedes the per-command total Inc — §12.2). The minimal arity rule: a command with `< 2` array elements that is NOT in `commandsWithoutMandatoryArgs` → invalid (the upstream `command_splitter_impl.cc` rule; the exact `commandsWithoutMandatoryArgs` set is transcribed at IMPL — D-S32.2-3).
4. **command in the table, valid arity (the data-command path)** → `command.<cmd>.total` Inc → proxy via the seam (`up.Send(ctx, raw)` → `decodeReply`) → on a reply (ANY RESP type, incl. `-ERR`/`$-1`/`*-1`) `command.<cmd>.success` Inc; on a pool/transport FAILURE (Send error, decodeReply error, conn closed before a reply) `command.<cmd>.error` Inc + close (§4.2). The reply is written downstream VERBATIM (unchanged from 32.1).

The `<cmd>` stat segment is `strings.ToLower(cmd)` (the upstream `AsciiStrToLower`; the table is lower-cased so the segment is a table member → IsValidName by construction — §5.2).

### 3.3 The extended local-reply set (`commands.go`; D-P32-6 full extent — §12.4)

`isLocalReply` extends from {PING, AUTH} to {PING, AUTH, ECHO, TIME, QUIT, HELLO-error-forms}. Each empirically-pinned (§12.4):

| Command | Local reply | Determinism | Differential arm? |
|---|---|---|---|
| **PING** (any args) | `+PONG\r\n` (arg ignored — no echo) | DETERMINISTIC | yes (32.1 landed; a `PING foo` no-echo arm at 32.2) |
| **AUTH** (no pwd configured) | `-ERR Client sent AUTH, but no password is set\r\n` | DETERMINISTIC | yes (32.1 landed) |
| **ECHO `<msg>`** (arity exactly 2) | `$<len>\r\n<msg>\r\n` (echoes the arg as a bulk string) | DETERMINISTIC | **yes** (a valid ECHO arm) |
| **ECHO** (arity ≠ 2) | `-invalid request\r\n` + `splitter.invalid_request` Inc | DETERMINISTIC | **yes** (a wrong-arity ECHO arm) |
| **TIME** | `*2\r\n$<n>\r\n<unix_secs>\r\n$<m>\r\n<unix_micros>\r\n` (from `time.Now()` — wall-clock) | **NON-DETERMINISTIC** | **no** — local-reply, UNIT-test-only (asserts the 2-element-array-of-bulk SHAPE, not exact bytes) |
| **QUIT** | `+OK\r\n` then CLOSE the downstream connection (after the write flushes) | DETERMINISTIC (reply + active close) | **yes** (a QUIT arm — asserts `+OK\r\n` + that the connection closes) |
| **HELLO** with `> 2` array elements | `-ERR HELLO options like AUTH and SETNAME are not supported\r\n` | DETERMINISTIC | **yes** (a HELLO-options arm) |
| **HELLO** with a non-`2`/non-numeric proto arg (incl. `HELLO 3`) | `-NOPROTO unsupported protocol version\r\n` | DETERMINISTIC | **yes** (a HELLO-3 RESP3-rejection arm) |
| **HELLO 2** / **HELLO** (no arg) | NOT a local reply — falls through to the PROXIED path (HELLO is a `ClusterScopeCommand`; `command.hello.total` Inc + forwarded upstream → the backend's server-info reply) | NON-DETERMINISTIC (server identity) | **no** — proxied; NOT exercised in the differential (the `TCPRedisResponder` does not speak HELLO) |

Implementation note: ECHO/TIME need the request's ARGUMENT (not just the command name) — the local-reply path needs the parsed argument array OR a re-parse of `raw`. The 32.1 `decodeRequest` returns only `(cmd, raw)`; 32.2 either (a) extends `decodeRequest` to also return the parsed argument slice (a small additive change — D-S32.2-4), or (b) the local-reply path re-parses `raw` for ECHO/TIME/HELLO arg-count. Option (a) is cleaner (the arity check + ECHO arg + HELLO arg-count all want the parsed array). The QUIT close path: `localReply` signals "close-after-write" (a small dispatch enum or a second return value — the 32.1 `localReply(cmd) []byte` shape extends to carry a close flag — D-S32.2-5).

ECHO/TIME/QUIT are NOT in the per-command stat table (singleton accessors — §12.1) → NO `command.*` increment (AMEND-R5). HELLO IS in the table (ClusterScopeCommands) → `command.hello.*` is CREATED eagerly, but the HELLO-error LOCAL paths do NOT increment it (they return before the per-command total — only the proxied `HELLO 2` path does).

---

## 4. The 32.2 stat roster (cross-reference parent §7 + the 32.1 §7 subset)

### 4.1 The EAGER per-command roster (`stats.go`; AMEND-R3; D-P32-2 RESOLVED — §12.1)

`redis.<stat_prefix>.command.<cmd>.{total,success,error}` over the STATIC 180-command table (§12.1) — **540 counters**, created EAGERLY at config parse (the kafka-176 / D-P32-1 eager precedent; `reg.NewCounterIfAbsent` idempotent across listeners sharing a `stat_prefix`). The roster build iterates the same `supportedCommands` table the dispatch uses (one source of truth). The `latency` histogram + `error_fault`/`delay_fault` counters are NOT created (ADR-0060 / faults — §2.3). A byte-stable `TestCommandRoster_MatchesUpstream` test pins the 180 names against a golden list transcribed from §12.1; a `TestCommandRoster_AllValidNames` test pins the IsValidName-by-construction property (§5.2).

### 4.2 Per-command increment semantics (the single-server MVP path; §12.2)

| Slot | Incremented when |
|---|---|
| `command.<cmd>.total` | the command is DISPATCHED to the proxy path (valid arity, in the table) — once per request, BEFORE `up.Send` |
| `command.<cmd>.success` | a reply frame is received from the upstream (ANY RESP type — incl. `-ERR` error replies AND `$-1`/`*-1` null sentinels; the single-server `updateStats(true)` semantics, §12.2) |
| `command.<cmd>.error` | the upstream round-trip FAILS — `up.Send` error, `decodeReply` error, or the upstream conn closes before a reply (the single-server `onFailure` path) |

This is the load-bearing semantic pin: a backend ERROR REPLY counts `success` (the command round-tripped), NOT `error`; only a TRANSPORT/POOL failure counts `error`. (The fragmented MGET/MSET reply-type-based accounting is DEFERRED — §2.2.)

### 4.3 The 2 splitter + 3 cluster-pool fixed counters (AMEND-R6/R7; §12.6)

- **`splitter.invalid_request`** (counter) — Inc on a bad-arity / malformed / non-array request (§3.2 item 3; the ECHO-wrong-arity arm).
- **`splitter.unsupported_command`** (counter) — Inc on a command not in the table (§3.2 item 2; the UNKNOWN-command arm).
- **`upstream_cx_drained`** / **`max_upstream_unknown_connections_reached`** / **`connection_rate_limited`** (REDIS_CLUSTER_STATS; counters) — CREATED eager (roster parity) but stay exist-at-0 in the MVP (no drain / no redirection / no connection-rate-limiting — §12.6). These are the ONLY redis-specific upstream/pool counters under `redis.<sp>.`; the standard `upstream_cx_total`/`upstream_cx_active`/`upstream_rq_total` traffic stats are the CLUSTER's own under `cluster.<name>.*` (the seam reuses them — AMEND-R6; UNCHANGED from 32.1). The HTTP-shaped `cluster.<name>.upstream_rq_2xx..5xx` have no RESP analog (stay 0 per-side; not asserted).

These 5 fixed names complete the parent §7.2 fixed-15 roster (10 landed at 32.1; +5 here). All created eager at config parse.

### 4.4 The 2 lifecycle gauges' inc/dec + the 2 buffered-gauge coverage boundary (D-P32-8 RESOLVED — §12.3)

The 4 gauges were CREATED eager at 32.1 (exist-at-0). 32.2 wires the inc/dec:

| Gauge | 32.2 inc/dec | Differential |
|---|---|---|
| `downstream_cx_active` | Inc at `Handle` entry (after `incCxTotal`); Dec at `Handle` return (defer) — one per live downstream connection | **LIVE-mirrored** via a held-open idle-connection arm (==1 both sides while held; ==0 post-close) — the mongo `op_query_active` D-P9 precedent (§8.2) |
| `downstream_rq_active` | Inc after `decodeRequest` succeeds (request received); Dec after the reply is written / the local reply is sent (request complete) | quiesced-zero (the synchronous single-flight pump completes each request before looping → no stable non-zero cross-side scrape point); asserted ==0 post-workload + UNIT-tested inc/dec |
| `downstream_cx_rx_bytes_buffered` | **exist-at-0 (coverage boundary)** — upstream sets this via `setConnectionStats` at the connection-buffer layer; envoy-go's synchronous terminal holds no persistent read buffer (the `bufio.Reader` is transient) | quiesced-zero (==0 both sides) |
| `downstream_cx_tx_bytes_buffered` | **exist-at-0 (coverage boundary)** — as above, write-buffer occupancy | quiesced-zero (==0 both sides) |

**The D-P32-8 finding (§12.3):** in upstream Envoy the 2 `*_bytes_buffered` gauges are NOT driven by the redis filter — they are handed to `Network::ConnectionImpl` via `setConnectionStats` and reflect the connection's read/write buffer occupancy (framework-managed). envoy-go's `redis_proxy` terminal owns the `net.Conn` directly with a transient `bufio.Reader` and a synchronous single-flight write — there is no persistent buffered state to track. So these 2 gauges stay exist-at-0 (a faithful coverage boundary, NOT a parity gap — at every quiesced scrape point upstream's buffered gauges are also 0). The 2 LIFECYCLE gauges (`cx_active`/`rq_active`) ARE filter-driven and get real inc/dec. `downstream_cx_active` is phase-32's differentially-mirrored gauge (the project's 2nd mirrored-gauge family after mongo's `op_query_active`, AMEND-R7).

### 4.5 The 2 deferred 32.1 counters

- **`downstream_cx_protocol_error`** (exist-at-0 from 32.1) — 32.2 WIRES its increment: a `decodeRequest` decode error (a malformed frame — bad type byte, overflow, truncated line) Incs it before `Handle` returns (the 32.1 pump returned silently on a decode error; 32.2 adds the Inc). A unit test drives a malformed frame → +1.
- **`downstream_cx_drain_close`** (exist-at-0 from 32.1) — stays exist-at-0 (the drain-close decision path is not in the MVP; QUIT's close is a clean local close, NOT a drain — §2.7). Coverage boundary.

---

## 5. The `redis.` LABEL-HOISTED Prometheus tag-extractor arm (`internal/stats/name.go`; AMEND-R4; D-P32-7)

### 5.1 The arm shape

A new arm in `flattenToProm` (`internal/stats/name.go`) detecting `redis.<prefix>.<rest>`:

```
redis.<sp>.downstream_cx_total        → envoy_redis_downstream_cx_total{envoy_redis_prefix="<sp>"}
redis.<sp>.command.get.total          → envoy_redis_command_get_total{envoy_redis_prefix="<sp>"}
redis.<sp>.splitter.unsupported_command → envoy_redis_splitter_unsupported_command{envoy_redis_prefix="<sp>"}
redis.<sp>.downstream_cx_active       → envoy_redis_downstream_cx_active{envoy_redis_prefix="<sp>"}  (# TYPE gauge)
```

The detection (the mongo `.rbac.` SINGLE-label hoist generalized to a `redis.` ROOT; the `strings.CutPrefix(internal, "redis.")` shape, NOT the `.rbac.`/`.zookeeper.` mid-string-segment shape): strip the `redis.` prefix → split off the dot-free `<prefix>` segment → label `envoy_redis_prefix="<prefix>"` + base `envoy_redis_` + `<rest>` flattened (dot→underscore). The per-command dynamic names flatten identically (`command.get.total` → `envoy_redis_command_get_total`) — collapsing distinct commands onto the metric NAME is NOT done (unlike the mongo `cmd`/`collection` VALUE hoist — redis hoists ONLY the `stat_prefix`, AMEND-R4: `envoy_redis_command_get_total{envoy_redis_prefix="redisprobe"}`, the command name is IN the metric name, not a label). SHAPE-based detection (dot-free `<prefix>` segment) — an allowlist is impossible given the 180 dynamic command names (the mongo/zookeeper/kafka permissive precedent). KEEP-IN-SYNC comment → `internal/filter/network/redisproxy/stats.go`.

### 5.2 Contrast with the kafka INLINE + mongo MULTI-label arms; the IsValidName-by-construction pin (D-P32-7)

| Filter | Arm shape | Labels |
|---|---|---|
| kafka (`kafka.<sp>.<rest>`) | INLINE — `envoy_kafka_<sp>_<rest>` (prefix flattened INTO the name) | EMPTY |
| mongo (`mongo.<sp>.<rest>`) | MULTI-label HOIST — `envoy_mongo_<rest collapsed>` + {prefix, cmd?, collection?, callsite?} | 1–4 (dynamic VALUE tokens hoisted) |
| **redis (`redis.<sp>.<rest>`)** | **SINGLE-label HOIST — `envoy_redis_<rest>` + {prefix}** | **1 (only `envoy_redis_prefix`)** |

redis is the SIMPLEST hoist (one label; the command name stays in the metric name, no VALUE collapse). **D-P32-7 RESOLVED:** because the 180-command table is STATIC and the `<cmd>` segment is always a table member, every `command.<cmd>.*` name is known at table-build time → `stats.IsValidName` holds BY CONSTRUCTION (the kafka eager posture; NO per-wire-byte guard needed — a wire command not in the table routes to `splitter.unsupported_command` and NEVER reaches `NewCounterIfAbsent`). `reference_dynamic_stat_name_charset_guard` does NOT bite (contrast the mongo dynamic `collection`/`callsite` names, which ARE wire-derived and DO need the guard). A `TestCommandRoster_AllValidNames` unit test asserts `IsValidName` over the 180-name table.

### 5.3 The 32.1→32.2 differential scrape upgrade

The 32.1 `0055` driver scraped the FLAT `GET /stats` text (internal names; no `redis.` arm needed). 32.2 upgrades the per-command/gauge assertions to the `/stats/prometheus` label-aware comparison (the mongo `0049`/`0051` label-aware scrape mechanics; the kafka `0053` precedent) — proving the `redis.` arm's `envoy_redis_<leaf>{envoy_redis_prefix="<sp>"}` form matches the reference's tag-extracted lines (R7). The flat-`/stats` counters (the 6 from 32.1) may stay flat-scraped; the NEW per-command + gauge assertions use the prometheus label-aware path (D-S32.2-6 — the exact scrape mix is the IMPL's; both endpoints are available to the in-band driver).

---

## 6. Config parse / PARSE-REJECT (cross-reference parent §6 + 32.1 §6; NO new arms)

The 32.1 `config.go` parse is COMPLETE (all PGV arms + the runtime no-upstream check + the unknown-cluster tolerance + the `IsValidName(stat_prefix)` guard + the deferred-field parse-accept). 32.2 adds NO new parse code and NO new PARSE-REJECT arm — the full command set is a `Handle`-time concern (the catch_all routing is unchanged). The 6 `redis_proxy:` byte-stable reject constants (`config.go:19-26`) are UNCHANGED. The `0056-redis-boot-reject` fixture is UNCHANGED (the `stat_prefix` arm). No new boot-reject differential.

---

## 7. Stat surface accounting — 546 → **1091**

| Component | Count | Cumulative |
|---|---|---|
| 32.1 baseline (10 fixed: 6 counters + 4 gauges) | — | **546** |
| + 2 `splitter.*` counters (§4.3) | +2 | 548 |
| + 3 `REDIS_CLUSTER_STATS` counters (§4.3) | +3 | 551 |
| + 180-command per-command roster × 3 slots (`total`/`success`/`error`; §4.1) | +540 | **1091** |

**= 1091** (the project's largest single-phase stat jump; the bounded-static-table kafka precedent). NOT counted (deferred): the 180 `command.<cmd>.latency` histograms (ADR-0060); the 360 `command.<cmd>.{error_fault,delay_fault}` counters (defer with `faults`). The BEHAVIOR_CONTRACT stat table gains the +545 rows in the 32.2 bundle (§10). The 2 lifecycle gauges' inc/dec + the 2 deferred counters' wiring (`downstream_cx_protocol_error`) add NO new stat NAMES (already created at 32.1) — they wire increments only.

(Sanity: 546 + 2 + 3 + 540 = 1091; matches parent §7.5 / the ADR-0229 §Consequences anticipated ~1091.)

---

## 8. Differential command matrix + the `TCPRedisResponder` extension (+0 dirs; cross-reference parent §8.1 + 32.1 §8)

Per `reference_differential_fixture_dispatch_constraint`: the command matrix extends the EXISTING cross-side `0055-redis-roundtrip` dir (cross-side-XOR-boot-reject — `0055` is the cross-side dir; NO new dir). Per `reference_differential_asserter_dispatch`: every subject-side stat assertion uses `fixture.StatsAsserter` (the as-built `0055` `AssertStats`); proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). Fixture count STAYS **58** (no new dir — parent §8.4 "extends `0055`").

### 8.1 The command-matrix arms (added to the `0055` `driveProxy` workload)

Each arm uses a FRESH connection (the 32.1 per-arm precedent — sequential, so the one-conn-per-downstream model dials once per arm → `upstream_cx_total` equality holds, D-P32-9). All arms drive both sides identically; the reply bytes + the per-command/splitter stats are the cross-side proofs.

| Arm | Request | Expected reply (byte-equivalence) | Stat assertions |
|---|---|---|---|
| **GET-hit** (32.1, kept) | `GET foo` (after `SET foo bar`) | `$3\r\nbar\r\n` | `command.get.{total,success}` +1 |
| **GET-miss** | `GET nope` | `$-1\r\n` (null bulk) | `command.get.success` (NOT error — null counts success, §4.2) |
| **INCR** | `INCR ctr` | `:1\r\n` | `command.incr.{total,success}` +1 |
| **DEL** | `DEL foo` | `:1\r\n` | `command.del.{total,success}` +1 (proxied single-server; NOT fragmented — §2.2) |
| **ECHO** (local) | `ECHO hi` | `$2\r\nhi\r\n` | NO `command.*`; NO upstream (local-reply) |
| **ECHO wrong-arity** (local) | `ECHO` (arity 1) | `-invalid request\r\n` | `splitter.invalid_request` +1; NO upstream |
| **QUIT** (local) | `QUIT` | `+OK\r\n` + connection close | NO `command.*`; connection closes |
| **HELLO-3** (local) | `HELLO 3` | `-NOPROTO unsupported protocol version\r\n` | NO `command.*`; NO upstream |
| **HELLO-options** (local) | `HELLO 2 AUTH u p` (>2 args) | `-ERR HELLO options like AUTH and SETNAME are not supported\r\n` | NO `command.*`; NO upstream |
| **UNKNOWN** | `BOGUSCMD x` | `-ERR unknown command 'BOGUSCMD', with args beginning with: \r\n` | `splitter.unsupported_command` +1; NO upstream |
| **PING-with-arg** (local) | `PING hello` | `+PONG\r\n` (arg ignored — no echo) | `downstream_rq_total` +1; NO upstream |
| **cx_active held-open** | (open conn, `PING`, hold across scrape) | `+PONG\r\n` | `downstream_cx_active` ==1 both sides while held; ==0 after close (§8.2) |

**EXCLUDED from byte-equivalence** (NON-DETERMINISTIC — §12.4): TIME (wall-clock; unit-test-only), HELLO 2 / HELLO-no-arg (proxied → backend server-info). These are NOT differential arms.

The exact arm set + the byte-equivalence verdict construction (the 32.1 `emitArmBytes` reply-concatenation pattern) is the IMPL's; the SPEC pins the arm taxonomy + the per-arm stat names + the determinism classification. Each NEW assertion is proven live via the R6 deliberate-break (`-count=1`) recorded in the driver comments + README + PROGRESS.md.

### 8.2 The `downstream_cx_active` held-open gauge arm (D-P32-8 / D-P32-9; §4.4)

The differentially-mirrored gauge needs a LIVE non-zero observation. The driver holds an idle connection open across the `AssertStats` scrape: open a connection to each side's listener, send `PING` (keeps the connection in `Handle`'s serve loop, `cx_active`==1), and DO NOT close it before the prometheus scrape — assert `envoy_redis_downstream_cx_active{envoy_redis_prefix="redis_r"}` ==1 on BOTH sides; then close → (a follow-up scrape, or the post-workload settle) ==0. This requires the driver to hold the ref + subj idle connections as state across `DriveX`→`AssertStats` (the 32.1 driver "carries no mutable cross-arm state" — 32.2 adds the held-connection pair; D-S32.2-7 pins the exact mechanism — a driver field holding the two idle conns, closed in a cleanup). The mongo `op_query_active` 29.2 unanswered-query held-arm is the precedent. `downstream_rq_active` + the 2 buffered gauges are asserted quiesced-zero (==0 both sides post-workload; §4.4) — they need no held arm.

### 8.3 Upstream-pool stats (AMEND-R6; D-P32-9)

The proxied arms assert `cluster.redis_cluster.upstream_cx_total` / `upstream_rq_total` per-arm (each fresh sequential connection dials once → equality holds — D-P32-9). A CONCURRENT-downstream scenario (where the reference's shared-per-host pool diverges from envoy-go's one-conn-per-downstream) is NOT exercised at 32.2 — pinned per-side as a coverage boundary (NOT asserted equal; the BEHAVIOR_CONTRACT 32.2 bundle records it; §12.6).

### 8.4 The `TCPRedisResponder` reply-table extension (`test/differential/fixture/fixture.go`)

The 32.1 `TCPRedisResponder` (BackendKind 32) returns `+OK\r\n` (SET) + `$3\r\nbar\r\n` (GET). 32.2 extends its canned-reply table: `$-1\r\n` (GET-miss — a key the responder treats as absent), `:1\r\n` (INCR), `:1\r\n` (DEL). FIFO/positional (no correlation id — unchanged). The responder is keyed by the decoded command name (it already parses RESP request frames). PING/AUTH/ECHO/TIME/QUIT/HELLO-errors NEVER reach the backend (local-reply — AMEND-R5); HELLO 2 would reach it but is NOT a differential arm (§3.3). The exact reply table is pinned at the 32.2 IMPL.

### 8.5 Fixture-dir count + conformance

Fixtures STAY **58** (`0055` extended, NO new dir; `0056` unchanged). The full 58-dir suite re-runs byte-exact green at the six-gate (the seam + the redisproxy filter are ADDITIVE — they activate only for a redis_proxy terminal). No new conformance harness; h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected (phase 32 touches no HTTP/h2/proxy-wasm path).

---

## 9. The 41st fuzzer `FuzzRESPDecode` (`internal/filter/network/redisproxy/fuzz_test.go`)

The 41st fuzzer (parent §3.1; the fuzzer-count tail `FuzzKafkaDecode` = 40 → 41). It feeds arbitrary bytes through BOTH `resp.go` decode entry points via a `bufio.Reader` over a `bytes.Reader` and asserts (the kafka `FuzzKafkaDecode` shape, adapted to the reader-based redis codec):

1. **No panic** — `decodeRequest(bufio.NewReader(bytes.NewReader(data)))` + `decodeReply(bufio.NewReader(bytes.NewReader(data)))` never panic on arbitrary bytes (a panic fails the fuzz run).
2. **No mutation** — the input `data` slice is never mutated (captured-copy compare; the decoder reads, never writes back — the redis analogue of the kafka chain-buffer-no-mutation invariant).
3. **Bounded allocation** — a crafted length header (`$<huge>\r\n` / `*<huge>\r\n`) NEVER allocates beyond the `maxBulkLen` (512 MiB) / `maxArrayLen` (1 Mi) guards (`resp.go:14-17`): the overflow guards reject an over-cap declared length with `errProtocol` BEFORE `make()`; an in-cap-but-absent body errors at `io.ReadFull` (`ErrUnexpectedEOF`) without hanging. The fuzzer asserts the decode returns (error or success) without unbounded growth. (NOTE: a `$536870911\r\n`-with-no-body input DOES `make([]byte, n+2)` up to the 512 MiB cap before the `io.ReadFull` error — the guard CAPS this; the fuzzer documents the cap as the bound. D-S32.2-8: whether to tighten `maxBulkLen` for the fuzz corpus is an IMPL note — the cap matches the upstream `proto_max_bulk_size` analog, so it stays.)

Seeds: a valid inline `PING\r\n`; a valid array `*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n`; each reply value type (`+OK\r\n`, `-ERR x\r\n`, `:1\r\n`, `$3\r\nbar\r\n`, `$-1\r\n`, `*2\r\n$1\r\na\r\n$1\r\nb\r\n`, `*-1\r\n`); a partial frame (`$10\r\nshort`); an overflow length (`$999999999999\r\n`); a bad type byte (`?xyz\r\n`); a reply-type-first-byte request (`+OK\r\n` to `decodeRequest` → `errProtocol`); a non-numeric integer (`:abc\r\n`). Per `reference_dynamic_stat_name_charset_guard`: the fuzzer ALSO confirms no stat-name is built from wire bytes (the decode path touches no registry — the per-command stat lookup is table-bounded in `filter.go`, not `resp.go`; the fuzzer scope is the codec only).

---

## 10. Behavior-contract delta + the parent-row-32 ROLLUP (per ADR-0052 atomic landing + ADR-0106(d))

ONE atomic bundle at the 32.2 IMPL final task:

- **Extend the `### envoy.filters.network.redis_proxy` subsection** (from the 32.1 bundle): the full command set (catch_all proxy of the 180-command table; the unknown → `splitter.unsupported_command` + bad-arity → `splitter.invalid_request` dispatch); the per-command `command.<cmd>.{total,success,error}` roster + the single-server success-on-any-reply / error-on-transport-failure semantics; the ECHO/TIME/QUIT/HELLO local-reply set (TIME non-deterministic; HELLO split — error-local vs proxied-happy-path); the 2 lifecycle gauges' inc/dec (`cx_active` mirrored; `rq_active` quiesced) + the 2 buffered-gauge coverage boundary; the `downstream_cx_protocol_error` wiring.
- **NEW `### Prometheus exposition — the redis. tag-extractor arm` note** (the `envoy_redis_<leaf>{envoy_redis_prefix="<sp>"}` single-label hoist; the kafka-INLINE / mongo-MULTI contrast; §5).
- **Coverage-boundary / departure records (the 32.2 bundle):** the `command.<cmd>.latency` histograms unmirrored (ADR-0060); the `*_fault` per-command counters deferred (faults); the 2 `*_bytes_buffered` gauges exist-at-0 (framework-buffer-managed upstream — §4.4); the 3 `REDIS_CLUSTER_STATS` exist-at-0 (no drain/redirection/rate-limit — §4.3); TIME non-deterministic (unit-test-only); HELLO 2 proxied (server-identity-dependent — not a differential arm); the multi-key fragmentation deferred (MGET/MSET proxied single-server — §2.2); the one-conn-per-downstream per-side pooling divergence (D-P32-9 — concurrent-downstream not exercised); `op_timeout` parsed-not-consumed (carried from 32.1); `enable_command_stats` no-op (parse-accepted); runtime-keys-at-defaults; close-direction-zero-touch (AMEND-R9).
- **Stat table:** 546 → **1091** (+545 rows: 2 splitter + 3 cluster + 540 per-command).
- **The parent-row-32 family ROLLUP note** (ADR-0106(d) — the §9 in-row rollup; the 18/19/22/24/25/26/28/29 precedent).

---

## 11. ADR map (NO new ADR; the ADR-0229 body completes)

Per ADR-0044 (§Context at SPEC; §Decision/§Consequences at IMPL): at THIS 32.2-SPEC commit, **NO new ADR §Context is anchored** (DECISIONS.md tail STAYS **ADR-0230**; next-free **ADR-0231** — unchanged). ADR-0229's §Decision/§Consequences body has a PARTIAL status (the 32.1 half landed); its **32.2 half completes IN-PLACE at the 32.2 IMPL** (status PARTIAL → ACCEPTED — the full command set + the per-command/splitter/cluster roster + the `redis.` arm + the 2 lifecycle gauges + the differential command matrix + the 41st fuzzer + the ROLLUP). ADR-0230 (the seam) is ACCEPTED and UNCHANGED (the seam is consumed, not extended). No ADR number is consumed at the 32.2 SPEC OR the 32.2 IMPL.

- **ADR-0229** *(32 — filter + parent umbrella; §Context at the parent SPEC; §Decision/§Consequences 32.1 half landed; 32.2 half completes at the 32.2 IMPL)* — the 32.2 body content: §4 the full command set + dispatch, §5 the per-command/splitter/cluster roster + the single-server semantics, §6 the `redis.` arm, §7 the 2 lifecycle gauges, §8 the differential matrix, §9 the fuzzer, §10 the ROLLUP. The §Consequences gains: stat surface 546 → 1091; fuzzers 40 → 41; fixtures 58 (extended, no new dir); BackendKind tail 32 (unchanged); the §9 family candidate count 2 → 1 ({thrift}).

Next-free after phase-32 phase-done = **ADR-0231** (unchanged; no further ADR anticipated).

---

## 12. SPEC-time empirical-pin block (upstream Envoy v1.37.2 source transcription; NO docker probes)

The 32.2 SPEC does NOT re-execute the parent §11 D32-1..D32-8 live-probe block (resolved once at the parent SPEC against the contrib reference image; inherited). It runs NO new docker probes. The only in-session evidence was a focused SPEC-time transcription of the upstream Envoy **v1.37.2** C++ source (the parent §11 corpus item 3, re-fetched this session) to pin the EXACT per-command table + the stat semantics + the local-reply behaviors that the parent §11 deferred to the sub-phase SPEC (D-P32-2/6/7/8). Source files (all at tag v1.37.2; fetched via raw.githubusercontent.com):

- `source/extensions/filters/network/common/redis/supported_commands.h`
- `source/extensions/filters/network/redis_proxy/command_splitter_impl.{cc,h}`
- `source/extensions/filters/network/redis_proxy/proxy_filter.{cc,h}`
- `source/extensions/filters/network/redis_proxy/conn_pool_impl.{cc,h}`
- `source/extensions/filters/network/common/redis/utility.cc`

### 12.1 D-P32-2 RESOLVED — the EXACT 180-command per-command stat table

`command_splitter_impl.cc InstanceImpl::InstanceImpl()` eagerly `addHandler()`s a `command.<lowercased-name>.*` stat block for each command in EXACTLY these 8 iterated `SupportedCommands` groups (the other groups — `multiKeyCommands`, `writeCommands`, `commandsWithoutMandatoryArgs` — are used for read/write classification + arity validation, NOT per-command stat registration; the singletons `auth`/`echo`/`ping`/`time`/`quit` are handled inline + NOT registered):

| Group (`supported_commands.h`) | Count | Handler |
|---|---|---|
| `simpleCommands()` | 152 | `simple_command_handler_` |
| `evalCommands()` (`eval`, `evalsha`) | 2 | `eval_command_handler_` |
| `objectCommands()` (`object`) | 1 | `object_command_handler_` |
| `hashMultipleSumResultCommands()` (`del`, `exists`, `touch`, `unlink`) | 4 | `split_keys_sum_result_handler_` |
| `mget()` / `mset()` / `scan()` / `infoShard()`=`info.shard` | 4 | the dedicated handlers |
| `randomShardCommands()` (`cluster`, `randomkey`) | 2 | `random_shard_handler_` |
| `transactionCommands()` (`multi`, `exec`, `discard`, `watch`, `unwatch`) | 5 | `transaction_handler_` |
| `ClusterScopeCommands()` (`script`, `flushall`, `flushdb`, `slowlog`, `config`, `info`, `keys`, `select`, `role`, `hello`) | 10 | `cluster_scope_handler_` |
| **Total distinct** (no cross-group overlap; `msetnx` is in `simpleCommands`, counted once) | **180** | |

The deduplicated 180-name list (lower-cased; the golden for the `stats.go` table + the `TestCommandRoster_MatchesUpstream` test):

```
append, bf.add, bf.card, bf.exists, bf.info, bf.insert, bf.loadchunk, bf.madd, bf.mexists,
bf.reserve, bf.scandump, bitcount, bitfield, bitop, bitpos, cluster, config, copy, decr, decrby,
del, discard, dump, eval, evalsha, exec, exists, expire, expireat, flushall, flushdb, geoadd,
geodist, geohash, geopos, georadius, georadius_ro, georadiusbymember, georadiusbymember_ro,
geosearch, geosearchstore, get, getbit, getdel, getex, getrange, getset, hdel, hello, hexists,
hget, hgetall, hincrby, hincrbyfloat, hkeys, hlen, hmget, hmset, hrandfield, hscan, hset, hsetnx,
hstrlen, hvals, incr, incrby, incrbyfloat, info, info.shard, keys, lindex, linsert, llen, lmove,
lpop, lpos, lpush, lpushx, lrange, lrem, lset, ltrim, mget, mset, msetnx, multi, object, persist,
pexpire, pexpireat, pfadd, pfcount, pfmerge, psetex, pttl, publish, randomkey, rename, renamenx,
restore, role, rpop, rpoplpush, rpush, rpushx, sadd, scan, scard, script, sdiff, sdiffstore,
select, set, setbit, setex, setnx, setrange, sinter, sinterstore, sismember, slowlog, smembers,
smismember, smove, sort, sort_ro, spop, srandmember, srem, sscan, strlen, substr, sunion,
sunionstore, touch, ttl, type, unlink, unwatch, watch, xack, xadd, xautoclaim, xclaim, xdel, xlen,
xpending, xrange, xrevrange, xtrim, zadd, zcard, zcount, zdiff, zdiffstore, zincrby, zinter,
zinterstore, zlexcount, zmscore, zpopmax, zpopmin, zrandmember, zrange, zrangebylex, zrangebyscore,
zrangestore, zrank, zrem, zremrangebylex, zremrangebyrank, zremrangebyscore, zrevrange,
zrevrangebylex, zrevrangebyscore, zrevrank, zscan, zscore, zunion, zunionstore
```

NOTE the `.`-containing names (`bf.add`…`bf.scandump`, `info.shard`) — these flatten to `command_bf_add` / `command_info_shard` in Prometheus (dot→underscore, §5) and pass `IsValidName` (the segment chars are `[a-z._]` — all valid; §5.2). The IMPL transcribes this exact list (a generated-or-hand-checked Go literal; the byte-stable test is the guard against transcription error). **D-P32-2 ANSWER: the FULL 180-command table is adopted (roster parity, the kafka-176 eager precedent — NOT a core subset).**

### 12.2 The per-command success/error semantics

`SplitRequestBase::updateStats(bool success)` (`command_splitter_impl.cc`): `success ? success_.inc() : error_.inc()`. The `success` flag:

- **SingleServerRequest** (the MVP path for ALL single-key proxied commands): `onResponse()` → `updateStats(true)` UNCONDITIONALLY (ANY upstream reply, INCLUDING a Redis `-ERR` error reply, counts `success`); `onFailure()` → `updateStats(false)` (a pool/connection failure → `error`). `total_.inc()` fires once at dispatch (`makeRequest`).
- **Fragmented requests** (MGET/MSET/SplitKeysSumResult/Scan): reply-type-based (`updateStats(error_count == 0)`). **DEFERRED at 32.2** (multi-key fragmentation is parse-accepted-behavior-deferred — §2.2; these commands proxy single-server in envoy-go's MVP → success-on-any-reply).

**envoy-go MVP rule (§4.2):** `total` at dispatch; `success` on ANY decoded reply (incl. `-ERR`, `$-1`, `*-1`); `error` ONLY on a transport/pool failure (Send/decode error, conn closed before reply). This is faithful to the SingleServerRequest path that all MVP commands take.

### 12.3 D-P32-8 — the 4 gauges' inc/dec call sites

`proxy_filter.{cc,h}`:
- `downstream_cx_active` (Accumulate) — `inc()` in `ProxyFilter` ctor (per downstream connection); `dec()` in `~ProxyFilter` (connection destroy). → envoy-go: Inc at `Handle` entry, Dec at `Handle` return (defer).
- `downstream_rq_active` (Accumulate) — `inc()` in `PendingRequest` ctor (per decoded command, `onRespValue`); `dec()` in `~PendingRequest` (response flushed in `onResponse`, or popped on connection close). → envoy-go: Inc after `decodeRequest`, Dec after the reply/local-reply is written.
- `downstream_cx_rx_bytes_buffered` / `downstream_cx_tx_bytes_buffered` (Accumulate) — **NOT inc/dec'd by the filter**; handed to `Network::ConnectionImpl` via `callbacks_->connection().setConnectionStats({…, rx_buffered, …, tx_buffered, …})` — driven by the generic connection read/write-buffer accounting. → envoy-go: exist-at-0 (no persistent buffer in the synchronous single-flight terminal — §4.4 coverage boundary).

(Also pinned: `downstream_cx_total` inc in ctor [32.1]; `downstream_rq_total` inc in PendingRequest ctor [32.1]; `downstream_cx_protocol_error` inc in `onData` catch [32.2 — §4.5]; `downstream_cx_drain_close` inc in `onResponse` drain decision [exist-at-0 in the MVP — §4.5].)

### 12.4 D-P32-6 — the local-reply command behaviors

(The full table is §3.3.) Source pins: PING handled in `command_splitter_impl.cc makeRequest` (`+PONG`; arg ignored); AUTH/QUIT in `proxy_filter.cc` (`onAuth` no-password → `-ERR Client sent AUTH, but no password is set`; `onQuit` → `+OK` + `connection().close(FlushWrite)`); ECHO/TIME/HELLO in `command_splitter_impl.cc makeRequest` inline (ECHO arity-2 echoes arg as BulkString, else `onInvalidRequest`; TIME → 2-element array from `dispatcher.timeSource().systemTime()` — WALL-CLOCK; HELLO `>2` args → options-not-supported error, non-2/non-numeric proto → `NOPROTO` error, `HELLO 2`/no-arg falls through to upstream dispatch). The `ResponseValues` constants (`command_splitter_impl.h`): `UnsupportedProtocol = "NOPROTO unsupported protocol version"`, `InvalidRequest = "invalid request"`. `makeError(s)` (`utility.cc`) → `-<s>\r\n`.

### 12.5 D-P32-7 / AMEND-R4 — the `redis.` Prometheus arm

Live-probed at the parent SPEC (`envoy_redis_command_get_total{envoy_redis_prefix="redisprobe"} 2`; `envoy_redis_downstream_cx_total{envoy_redis_prefix="redisprobe"} 8`). Single-label hoist (only `envoy_redis_prefix`); the command name stays IN the metric name. IsValidName satisfied by construction (§5.2 — the 180-name table is static).

### 12.6 The 3 REDIS_CLUSTER_STATS firing conditions

`conn_pool_impl.{cc,h}` (`RedisClusterStats`, cluster-scoped pool path): `upstream_cx_drained` (a draining client closes), `max_upstream_unknown_connections_reached` (the ASK/MOVED redirect unknown-host cap), `connection_rate_limited` (the per-second token-bucket rejects). ALL stay exist-at-0 in the MVP (no drain, no redirection, no rate-limiting — §4.3). Created for roster parity.

---

## 13. SPEC-time D-questions — parent resolutions + 32.2-additive PLAN/IMPL questions

### 13.1 Parent D-questions RESOLVED at this SPEC

- **D-P32-2 (per-command roster + the mirrored-vs-per-side upstream counter set) — RESOLVED** (§12.1 / §4): the FULL eager 180-command table (`command.<cmd>.{total,success,error}` = 540 counters); `total`/`success`/`error` only (latency histogram + `*_fault` deferred). The upstream traffic stats are the cluster's own `cluster.<name>.*` (mirrored per-arm with one-conn-per-downstream — D-P32-9); the 3 `REDIS_CLUSTER_STATS` exist-at-0.
- **D-P32-6 (local-reply extent) — RESOLVED** (§3.3 / §12.4): PING/AUTH (32.1) + ECHO/TIME/QUIT/HELLO (32.2); TIME unit-test-only (non-deterministic); HELLO split (error-local vs proxied-happy-path).
- **D-P32-7 (IsValidName disposition) — RESOLVED: by construction** (§5.2): the static 180-table → table-bounded names → no per-wire-byte guard (the kafka eager posture).
- **D-P32-8 (the 4 gauges' differential design) — RESOLVED** (§4.4 / §12.3): `cx_active` filter-driven + LIVE-mirrored (held-open arm); `rq_active` filter-driven + quiesced-zero; the 2 `*_bytes_buffered` exist-at-0 (framework-buffer-managed-upstream coverage boundary).
- **D-P32-9 (upstream-pool per-side asymmetry) — RESOLVED** (§8.3): per-arm sequential connections → `upstream_cx_total` equality holds; a concurrent-downstream scenario is NOT exercised (per-side coverage boundary).

### 13.2 32.2-additive D-questions for PLAN / IMPL resolution

- **D-S32.2-1 (the table representation + case).** `map[string]struct{}` vs sorted slice; UPPERCASED (to match `decodeRequest`'s output) vs lower-cased (the stat segment) — likely store UPPERCASED for the dispatch lookup + lower-case once for the stat name. **Resolution at:** IMPL (the `commands.go` task).
- **D-S32.2-2 (the unknown-command error wording).** The exact `-ERR unknown command '<name>', with args beginning with: …\r\n` form — the casing of `<name>` (as-received vs upper) + the args-suffix. Pin byte-stable against the reference (the `0055` UNKNOWN arm captures the reference bytes). **Resolution at:** IMPL.
- **D-S32.2-3 (the arity-validation rule + `commandsWithoutMandatoryArgs` set).** The minimal "<2 args and not in commandsWithoutMandatoryArgs → invalid_request" rule + the exact set (transcribed from `supported_commands.h`). **Resolution at:** IMPL (the `commands.go` task). The differential's ECHO-wrong-arity arm is the live proof.
- **D-S32.2-4 (decodeRequest argument exposure).** Whether `decodeRequest` is extended to return the parsed argument slice (for ECHO/TIME echo + arity) or the local-reply path re-parses `raw`. **Resolution at:** IMPL (the `commands.go`/`resp.go` task). Anticipated: extend the return (additive).
- **D-S32.2-5 (the QUIT close signal).** How `localReply` signals close-after-write (a second return value / a dispatch enum). **Resolution at:** IMPL.
- **D-S32.2-6 (the prometheus vs flat-`/stats` scrape mix).** Which assertions use `/stats/prometheus` (the `redis.` arm + per-command + gauges) vs the flat `/stats` (the 32.1 counters). **Resolution at:** IMPL (the `0055` task).
- **D-S32.2-7 (the held-open gauge-arm mechanism).** How the driver holds the ref + subj idle connections across `AssertStats` (driver fields + cleanup). **Resolution at:** IMPL (the `0055` task).
- **D-S32.2-8 (the fuzzer allocation cap).** Whether to bound `maxBulkLen` for the fuzz corpus (the cap matches the upstream `proto_max_bulk_size` analog → stays). **Resolution at:** IMPL (the fuzzer task).

---

## 14. Per-task structure (~12-15 tasks; the SPEC-anticipated spine) + ADR-0045 split-gate re-check

The 32.2 PLAN authors the exact bite-sized TDD tasks (the PLAN may merge/split); this is the SPEC-anchored spine:

| # | Task | Lands |
|---|---|---|
| 1 | First-action baselines/anchors gate: re-pin fixtures **58** + fuzzers **40** + stat surface **546** + BackendKind tail **32** + DECISIONS tail **ADR-0230** (next-free **ADR-0231**) + the §12.1 180-command table source-transcription re-check + a clean `go mod tidy` (ZERO new dep), against the live IMPL-session tip | §7 / §12 |
| 2 | `commands.go`: the static 180-command `supportedCommands` table + `TestCommandRoster_MatchesUpstream` (golden 180) + `TestCommandRoster_AllValidNames` (IsValidName by construction) | §3.2 / §4.1 / §5.2 / §12.1 |
| 3 | `commands.go`: the extended local-reply set (ECHO/TIME/QUIT/HELLO) + the decodeRequest arg exposure (D-S32.2-4) + the QUIT close signal (D-S32.2-5) + unit tests (incl. TIME shape-only, HELLO split) | §3.3 / §12.4 |
| 4 | `stats.go`: the EAGER 540 per-command counters + the 2 `splitter.*` + 3 `REDIS_CLUSTER_STATS` (eager) + the per-command total/success/error + splitter inc accessors | §4.1 / §4.3 |
| 5 | `stats.go`: the 2 lifecycle gauges' inc/dec accessors (`cx_active`/`rq_active`) + the `downstream_cx_protocol_error` accessor | §4.4 / §4.5 |
| 6 | `filter.go`: the `Handle` pump wiring — table lookup + unknown→`splitter.unsupported_command`+error, bad-arity→`splitter.invalid_request`+error, valid→`command.<cmd>.total`+proxy+success/error; the `cx_active`/`rq_active` gauge inc/dec; the `cx_protocol_error` increment; the QUIT close; the `doc.go` 32.2 forward-pointers resolved; unit tests (single command stat; unknown; bad-arity; success-on-error-reply; error-on-transport-failure; gauge inc/dec) | §3.2 / §4.2 / §4.4 / §4.5 |
| 7 | `internal/stats/name.go`: the `redis.` LABEL-HOISTED arm + unit tests (the fixed names + the dynamic command names + the gauge `# TYPE` line; contrast kafka/mongo) | §5 |
| 8 | `FuzzRESPDecode` (the 41st) — no-panic / no-mutation / bounded-allocation over `decodeRequest`+`decodeReply`; the seed corpus | §9 |
| 9 | `TCPRedisResponder` reply-table extension (`$-1` GET-miss, `:1` INCR/DEL) | §8.4 |
| 10 | `0055` driver: the command-matrix arms (GET-hit/miss, INCR, DEL, ECHO valid/wrong-arity, QUIT, HELLO-3, HELLO-options, UNKNOWN, PING-with-arg) + the per-command/splitter stat assertions via `/stats/prometheus` | §8.1 / §5.3 |
| 11 | `0055` driver: the `cx_active` held-open gauge arm (==1 live both sides) + the `rq_active`/buffered quiesced-zero assertions + the R6 deliberate-break liveness recording (`-count=1`) | §8.2 / §4.4 |
| 12 | Completion bundle: ADR-0229 §Decision/§Consequences 32.2 half (PARTIAL → ACCEPTED) in-place (ADR-0044) + the BEHAVIOR_CONTRACT 32.2 bundle (§10) + STATE.md + ROADMAP parent row 32 + sub-row 32.2 ATOMIC ROLLUP (`in-progress → done` / `planned → done`) + next-prompt.txt (the next-phase cold-start) + the six-gate (incl. the FULL 58-dir differential suite) | §10 / §11 / §15 |

### 14.1 ADR-0045 split-gate — SPEC-level re-check

Production-LoC estimate (production code; fixture drivers + unit tests + the 180-name table literal EXCLUDED per the 26.x–31 accounting basis — the table literal is a generated/transcribed data block, not logic):

| Deliverable | Production LoC |
|---|---|
| `commands.go` (the 180-table build + the extended local-reply set + arity/validation + dispatch helpers) | ~120–180 (+ the ~180-line table literal, excluded) |
| `stats.go` (the eager per-command roster build + 5 fixed counters + the inc accessors + the gauge inc/dec) | ~90–140 |
| `filter.go` (the pump wiring — table lookup + splitter + per-command + gauges + protocol_error + QUIT close) | ~90–140 |
| `internal/stats/name.go` (the `redis.` arm) | ~25–40 |
| `FuzzRESPDecode` (test — excluded) | — |
| **Total (production basis)** | **~325–500** |

**Verdict: fits as ONE sub-phase** (well under the ~1500 LoC gate; the ~12-15-task spine under the ~25-task gate — matches parent §15's 32.2 estimate ~400-650 / ~10-15 tasks). The fixture drivers (`0055` extension + `TCPRedisResponder`) + the unit/fuzz tests are excluded. **The 32.2 PLAN remains the FINAL gate-check** (parent §3.0): if the bite-sized TDD decomposition exceeds ~25 tasks / ~1500 LoC, a 32.2a/32.2b escape-valve split is available (anticipated axis: 32.2a = the command set + the per-command/splitter/cluster roster + the gauges [Tasks 2–6]; 32.2b = the `redis.` arm + the fuzzer + the differential matrix + the ROLLUP [Tasks 7–12]). The 2-way parent pre-split holds; no further split anticipated.

---

## 15. Test surface + 32.2 IMPL acceptance checklist

### 15.1 Test surface (per parent §14, scoped to 32.2)

- **Layer A — redisproxy unit tests:** the 180-command table (`TestCommandRoster_MatchesUpstream` golden + `TestCommandRoster_AllValidNames`); the extended local-reply set (ECHO echo + arity; TIME shape-only [2-element array of bulk]; QUIT `+OK`+close; HELLO `>2`→options-error, `HELLO 3`→NOPROTO, `HELLO 2`→proxied); the pump dispatch (unknown→`splitter.unsupported_command`+`-ERR unknown command`; bad-arity→`splitter.invalid_request`+`-invalid request`; valid→`command.<cmd>.total`; success-on-any-reply incl. `-ERR`/`$-1`; error-on-transport-failure); the 2 lifecycle gauges' inc/dec (held connection → `cx_active`==1, close → 0; request lifecycle → `rq_active` inc/dec); `downstream_cx_protocol_error` on a malformed frame.
- **Layer A — stats/name.go unit tests:** the `redis.` arm (fixed names + dynamic `command.<cmd>.total` + the gauge `# TYPE` line; the `bf.add`/`info.shard` dot-flatten; contrast the kafka/mongo arms).
- **Layer C — fuzz:** `FuzzRESPDecode` (the 41st) — no-panic / no-mutation / bounded-allocation (§9).
- **Layer D — differential:** `0055` extended (the command matrix + the gauge held-open arm + the `/stats/prometheus` label-aware assertions) + the FULL 58-dir back-compat suite → 58/58 green. Every new assertion proven live via a recorded `-count=1` deliberate-break (R6).
- **Layer E — race:** `go test ./... -race -short` across `internal/filter/network/redisproxy/...` + `internal/stats/...`.
- Per-task `gofmt -l` + `golangci-lint run` on touched packages (`feedback_pertask_gofmt_lint`).

### 15.2 Six-gate checklist (per the 26–31 precedent)

`go build ./...` / `go vet ./...` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact (58 dirs incl. the back-compat gate) / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — phase 32.2 touches no HTTP path). All outputs quoted into PROGRESS.md (run honestly).

### 15.3 32.2 IMPL acceptance checklist

1. The full command set lands per §3 (the 180-table dispatch + unknown/bad-arity splitter handling + the ECHO/TIME/QUIT/HELLO local-reply set); the per-command/splitter/cluster roster + the 2 lifecycle gauges' inc/dec + the `cx_protocol_error` wiring land per §4.
2. The `redis.` LABEL-HOISTED Prometheus arm lands per §5; `TestCommandRoster_AllValidNames` proves IsValidName-by-construction (no per-wire-byte guard).
3. `FuzzRESPDecode` (the 41st) lands per §9; counts: fuzzers 40 → 41, stat surface 546 → 1091, fixtures 58 (extended, no new dir), BackendKind tail 32 (unchanged).
4. `0055` extended with the command matrix + the gauge held-open arm + the `/stats/prometheus` assertions; the `TCPRedisResponder` reply table extended; every new assertion proven live (R6 `-count=1`).
5. The ADR-0229 §Decision/§Consequences 32.2 half completes in-place (PARTIAL → ACCEPTED; DECISIONS.md tail STAYS ADR-0230; no new number); the BEHAVIOR_CONTRACT 32.2 bundle lands (§10).
6. Six gates green (§15.2); STATE.md advanced; **ROADMAP parent row 32 `in-progress → done` ATOMICALLY with sub-row 32.2 `planned → done`** (the §9 in-row ROLLUP per ADR-0106(d) — the 18/19/22/24/25/26/28/29 precedent); next-prompt.txt rewritten for the next-phase cold-start (the §9 family stays OPEN — {thrift} remains).

---

## 16. Stage-close handoff

Per ADR-0004/0005: this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, ROADMAP sub-row 32.2 flips **`planned → in-progress` AT THIS SPEC COMMIT** (ADR-0106 / the 26.x/28.x/29.x precedent — the parent row 32 STAYS `in-progress`; the `→ done` ROLLUP is the 32.2 IMPL's) AND the stale ROADMAP row-32.2 text is corrected (the "DYNAMIC per-command-seen creation likely" / "inline-vs-label-hoist pinned at SPEC" hypotheses are SUPERSEDED — D32-2 RESOLVED EAGER table-bounded + LABEL-HOISTED at the parent SPEC). NO new ADR §Context is anchored (DECISIONS.md tail STAYS ADR-0230; next-free ADR-0231). STATE.md advances to lifecycle-state-for-32.2-PLAN with `next-skill = superpowers:writing-plans` scoped to the **32.2 PLAN** (`docs/envoy-go/phases/32.2-network-filter-redis-commands-and-stats/PLAN.md`). The SPEC is squash-merged to master + pushed (`feedback_push_to_origin`; the controller squash-merges + pushes at stage-close, subagents local-only per `feedback_subagents_no_push`); next-prompt.txt is rewritten for the 32.2-PLAN cold-start. Per `feedback_execution_style` the 32.2 IMPL runs `superpowers:subagent-driven-development` (fresh subagent per task + two-stage review); per `feedback_git_worktrees`/`feedback_subagents_no_push`/`feedback_push_to_origin`/`feedback_pertask_gofmt_lint`/`reference_differential_break_protocol_count1` the established worktree/push/lint/break discipline applies.

---

## Appendix A — Cross-references to parent SPEC + the 32.1 SPEC

| 32.2 SPEC § | Source § | Relationship |
|---|---|---|
| §1 Purpose | parent §1 + §3.2 (32.2) + 32.1 §1 | refines |
| §1.1 AMENDs | parent §1.1 (R3/R4/R5/R6/R7/R9) | inherits the 32.2-load-bearing subset |
| §1.2 Additive pins | — | NEW (the D-P32-2/6/7/8/9 resolutions; the success/error + gauge + local-reply refinements) |
| §3 command set + local-reply | parent §3.2 + §11.6 + 32.1 §3.6 | refines into the dispatch + the extended set |
| §4 stat roster | parent §7.2 + 32.1 §7.2 | refines (the +5 fixed + 540 per-command + the gauge inc/dec) |
| §5 the `redis.` arm | parent §7.4 (AMEND-R4) | PINS the site + the single-label shape; resolves D-P32-7 |
| §6 config / reject | parent §6 + 32.1 §6 | INHERITS (no new arm) |
| §7 stat surface | parent §7.5 | refines (546 → 1091) |
| §8 differential matrix | parent §8.1 + 32.1 §8.1 | refines (the command matrix + the gauge arm + the prometheus upgrade) |
| §9 fuzzer | parent §3.1 / §14 | PINS `FuzzRESPDecode` (the 41st) |
| §10 behavior contract + ROLLUP | parent §9 (32.2 bundle) | refines + the parent-row ROLLUP |
| §11 ADR map | parent §10 (ADR-0229 body) | completes the ADR-0229 body (no new number) |
| §12 empirical pins | parent §11 (inherited) + the source transcription | resolves D-P32-2/6/7/8 |
| §13 D-questions | parent §12 | resolves D-P32-2/6/7/8/9; adds D-S32.2-1..8 |
| §14 Tasks + split-gate | parent §15 (32.2 row) | NEW (task spine); gate re-check |

## Appendix B — Phase 32.2 ADR landing summary

- **ADR-0229** (the `redis_proxy` filter + parent-row-32 umbrella) — §Context anchored at the parent SPEC; §Decision/§Consequences 32.1 half landed at the 32.1 IMPL (status PARTIAL); the **32.2 half completes IN-PLACE at the 32.2 IMPL** (status PARTIAL → ACCEPTED — the full command set + the per-command/splitter/cluster roster + the `redis.` arm + the 2 lifecycle gauges + the differential command matrix + the 41st fuzzer + the parent-row-32 ROLLUP). NO new ADR number consumed (DECISIONS.md tail STAYS ADR-0230; next-free ADR-0231).
- **ADR-0230** (the upstream connection-pool / cluster-routing seam) — ACCEPTED; CONSUMED unchanged at 32.2 (the seam is not extended).
- DECISIONS.md tail = **ADR-0230** at the 32.2 SPEC commit (unchanged); next-free after phase-32 phase-done = **ADR-0231**.
