# Phase 29.1 IMPL — PROGRESS

Network-filter `mongo_proxy` wire + requests. This file tracks the IMPL-session
baselines (Task 1) and a per-task log filled as Tasks 1–17 land.

## Baselines (Task 1 — first-action gate)

Re-confirmed at the IMPL-session tip.

- **Worktree tip:** `4a538f7 next-prompt.txt + STATE.md: correct PLAN line count 3022→3033 (post reviewer-advisory fold)`
- **Fixtures:** **50** (canonical recipe `ls -d test/fixtures/[0-9]* | wc -l`); tail dir = `test/fixtures/0048-zookeeper-responses`. 29.1 lands `0049` + `0050` → 52.
- **Fuzzers:** **38** (canonical recipe `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`). 29.1 lands the 39th (`FuzzMongoDecode`).
- **Stats surface:** **337** (canonical count — `BEHAVIOR_CONTRACT.md:466` "Phase 28.1 extension — 136 → 337 internal names"; `:464` confirms 28.2 held at 337). 29.1 lands +23 → **360** at Task 17.
- **DECISIONS.md ADR tail:** **ADR-0226** (`grep -oE "ADR-0[0-9]+" docs/envoy-go/DECISIONS.md | sort -u | tail -3` = ADR-0224 / ADR-0225 / ADR-0226). **Next-free = ADR-0227.** 29.1 fills the ADR-0224 §Decision/§Consequences body in place (no new ADR number consumed).

## TypeURL + field/FaultDelay/denominator roster (Task 1 — Step 3 pins)

Against go-control-plane **v1.32.4** in the module cache.

### TypeURL pin

- `proto.MessageName(&mpv3.MongoProxy{})` = **`envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy`** (verified via `go run`; carries the `extensions.` segment).
- `TypeURL` = `type.googleapis.com/` + above = **`type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy`**.

### MongoProxy 5-field accessor set

`extensions/filters/network/mongo_proxy/v3/mongo_proxy.pb.go`:

- `func (x *MongoProxy) GetStatPrefix() string`
- `func (x *MongoProxy) GetAccessLog() string`
- `func (x *MongoProxy) GetDelay() *v3.FaultDelay`  (`v3` = `extensions/filters/common/fault/v3`)
- `func (x *MongoProxy) GetEmitDynamicMetadata() bool`
- `func (x *MongoProxy) GetCommands() []string`

### FaultDelay oneof accessors + wrapper type names

`extensions/filters/common/fault/v3/fault.pb.go`:

- `func (x *FaultDelay) GetFaultDelaySecifier() ...` — the oneof getter. **NOTE the upstream-proto `Secifier` typo** (missing `p`; it is NOT `Specifier`).
- `func (x *FaultDelay) GetFixedDelay() *durationpb.Duration` — type-asserts to `*FaultDelay_FixedDelay`.
- `func (x *FaultDelay) GetHeaderDelay() *FaultDelay_HeaderDelay` — type-asserts to `*FaultDelay_HeaderDelay_`.
- `func (x *FaultDelay) GetPercentage() *v3.FractionalPercent`  (`v3` here = `type/v3`).
- Oneof-wrapper types: **`FaultDelay_FixedDelay`** (struct) and **`FaultDelay_HeaderDelay_`** (struct, trailing underscore — distinct from the `FaultDelay_HeaderDelay` message type returned by `GetHeaderDelay()`).
- Marker method: `isFaultDelay_FaultDelaySecifier()` (typo carried).

### FractionalPercent denominator enum (valid set {0,1,2})

`type/v3/percent.pb.go`:

- `FractionalPercent_HUNDRED FractionalPercent_DenominatorType = 0`
- `FractionalPercent_TEN_THOUSAND FractionalPercent_DenominatorType = 1`
- `FractionalPercent_MILLION FractionalPercent_DenominatorType = 2`

## Per-task log

### Task 1 — First-action baselines/anchors gate (no code change) — DONE
All counts re-confirmed at tip: fixtures 50 (tail `0048-zookeeper-responses`), fuzzers 38, stats 337, DECISIONS tail ADR-0226 (next-free ADR-0227). TypeURL = `envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy`. Field/FaultDelay/denominator roster pinned above. `PROGRESS.md` created + committed local-only.

### Task 2 — `mongoproxy` package skeleton + TypeURL + config parse — DONE
Skeleton + TypeURL (`type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy`) + config parse (5 fields, AMEND-B7 default commands {delete,insert,update}, alias table) landed; commit `d682ee6`.

### Task 3 — FaultDelay PGV validation + PARSE-REJECT constants — DONE
Replaced the Task-2 `parseDelay` stub with the real FaultDelay PGV validator: oneof `fault_delay_secifier` required when delay present (`errDelaySpecifierRequired`), `fixed_delay` must be > 0s (`errDelayFixedDelayTooSmall`), `header_delay` parse-accepted, percentage denominator constrained to the defined enum set {0,1,2} (`errDelayDenominatorInvalid`); the four byte-stable PARSE-REJECT constants guarded by `TestParseRejectConstants_ByteStable`. The four Task-2 `//nolint:unused` directives + their announcing block comment removed (parseDelay now writes `delayConfigured`/`fixedDelay`/`delayPercentNum`/`delayPercentDenom`). gofmt/lint/test clean.

### Task 4 — bson.go part 1 — primitives + scalars — DONE
`bson.go` part 1: `errBSON`, `bsonElem`, `bsonDoc`+`first()`/`find()`, `asInt64`, the `bsonReader` cursor (readByte/readBytes/readInt32/readInt64/readCString, all underflow-checked), `parseBSON`/`parseDocument` framing (docLen>=5, end-bound, terminator-position checks), `parseElementValue` for the 7 fixed-width scalar types (0x01/0x08/0x09/0x0A/0x10/0x11/0x12) + throw-on-unknown default. `//nolint:unused` added on `find`+`asInt64` (consumed by the codec) — REMOVED at Task 8. commit `f757eae`.

### Task 5 — bson.go part 2 — variable/nested + lookups — DONE
Inserted the 7 variable-length/nested cases ABOVE the default: String 0x02 + Symbol 0x0E (`readString`), Document 0x03 + Array 0x04 (recurse `parseDocument`), Binary 0x05 (len+subtype+bytes), ObjectId 0x07 (12 bytes), Regex 0x0B (two cstrings → `[2]string`). No recursion-depth guard (upstream wire-parity; bounded by the codec readBuf cap). commit `3b37d81`.

### Task 6 — stats.go — 23-stat eager roster + dynamic helpers — DONE
`rosterSuffixes()` (the EXACT 22 upstream-macro suffixes; `delays_injected` PLURAL — AMEND-B3), `mongoStats` + `newMongoStats` (eager 22 counters + `op_query_active` gauge under `mongo.<sp>.`), `inc(suffix)` (panic on unknown — closed roster), dynamic helpers `cmdTotal`/`collectionQuery`/`callsiteQuery`. `//nolint:unused` on `inc` — REMOVED at Task 7. `compiledConfig.stats` field NOT re-added (deferred to Task 10). commit `0f16aac`. **The Task-6 code-quality review surfaced a latent panic: `collectionQuery`/`callsiteQuery` feed wire-derived segments into `NewCounterIfAbsent` which PANICS on names failing `nameRE` → the Task-13 fuzzer would crash. User-approved fix "Guard + skip" applied at Task 8 (see below). Memory: `reference_dynamic_stat_name_charset_guard`.**

### Task 7 — codec.go part 1 — framing + reassembly + dispatch — DONE
`decoder` struct (`chainConsumed` high-water vs `Buffer.TotalAppended()` per D-S29.1-4, own `readBuf`, `sniffing`, `queries`), `newDecoder`, `decodeOnData`, `nextMessage` (msgLen includes 16-byte header; `msgLen<16`→error catches negatives; partial→wait), `decodeMessage` (7-opcode dispatch: Query/Insert/GetMore/KillCursors/Command body-decoded; Reply(1)/CommandReply(2011) recognized-not-decoded; default incl. OP_MSG 2013/Update/Delete → `decoderError`), `decoderError` (at-most-once + sniffing-off lifetime, D-S29.1-6), `fail()`, 5 STUB body decoders. Removed Task-6 `inc` nolint; added forward-ref nolints on `activeQuery`/`queries`/`fail` (REMOVED at Task 8). Dropped the PLAN test-helper's dead `cfg.stats = ms` (field lands at Task 10). commit `54be390`. Carry: Task-10 must feed `buf.Bytes()`+`buf.TotalAppended()` atomically (trusted-caller contract).

### Task 8 — codec.go part 2 — OP_QUERY decode — DONE (with user-approved deviation)
Full OP_QUERY decode (flags+collection+$cmd command+query-shape+$maxTimeMS+$comment callsite double-count AMEND-C3+active-query append) + `queryShape`/`maxTimeLessThanOne`/`callsiteName` + `remaining()` in bson.go. **DEVIATION (user-approved): `stats.IsValidName` guard around the 3 wire/config-derived dynamic-stat increments (`cmdTotal`/`collectionQuery`/`callsiteQuery`); skip un-nameable segments; fixed counters + active-query append always run. Live guard test `TestDecodeQuery_InvalidCollectionNameSkipsDynamicNoPanic` (proven live: RED panic without guard → GREEN with). Divergence (envoy-go omits the dynamic stat for un-nameable segments upstream would emit) → record as a BEHAVIOR_CONTRACT coverage boundary at Task 17.** Removed 5 now-consumed nolints (find/asInt64/activeQuery/queries/fail). commit `1666555`. Carried advisory → Task 15: include a maxTime≥1 fixture arm to prove `op_query_no_max_time` SUPPRESSION is live.

### Task 9 — codec.go part 3 — INSERT/GET_MORE/KILL_CURSORS/COMMAND — DONE
The four remaining stubs replaced with validate-and-consume bodies (each increments only its primary op counter after structural validation; malformed → `d.fail()`; none append to the active-query list). DoS-safe (empirically verified: huge/negative killCursors `n` and `remaining()>0` loops are bounded by body length). No dynamic stat names → guard N/A. commit `f19fa9d`. Carried advisory → Task 13: optional huge-n / per-opcode malformed tests + a negative-n parity comment.

### Task 10 — filter.go — NewFactory + both-directions glue — DONE
`NewFactory(reg)` (boot-parse + eager roster per stat_prefix) + per-connection `*filter` (BOTH ReadFilter+WriteFilter; OnData feeds decoder via `buf.Bytes()`+`buf.TotalAppended()` atomically; OnWrite pure no-op `Continue` stub; OnNewConnection no-op; OnDestroy nils `dec`). **Re-added the deferred `compiledConfig.stats *mongoStats` field to config.go** (restores the PLAN Task-2 end-state; `mongoStats` now exists). cb/wcb stored-unused, lint-clean (zookeeper precedent). Trusted-caller contract (atomic buffer feed) verified against real Buffer + both chain.go call sites. commit `8bbe37b`.

### Task 11 — 8th built-in registration + bootstrap blank-import + boot smoke — DONE
`reg.Register(mongoproxy.TypeURL, mongoproxy.NewFactory(deps.StatsRegistry))` (8th built-in) + "seven→eight" doc + `mongo_proxy/v3` bootstrap blank-import + all-eight registration test + boot-smoke (23 stats eager at 0; both directions). **§4 zero-touch regression gate: full `go test ./internal/...` stayed green.** commits `c85753e` + `7431e80` (comment-wrap nit fix).

### Task 12 — mongo. four-rule name.go tag-extractor arm — DONE
The AMEND-C1 multi-label tag-extractor in `name.go` `flattenToProm` (`default` branch, after `.zookeeper.`): prefix/cmd/collection/callsite hoisting via `hoistMongoDynamicSegments`; labels SORTED LOCALLY by key → **`prom.go` BYTE-UNTOUCHED (D-S29.1-5)**. Byte-exact `TestWriteProm_MongoCallsiteLineByteExact` proves the 3-label §11.2 line. KEEP-IN-SYNC round-trip with stats.go builders verified. The PLAN's wrong `mongo.a.b.cmd.x.total`-errors sub-test corrected to the permissive fall-through (per PLAN line 2497 note). Full `internal/stats` suite green. Panic-proof on malformed input. commit `efc0759`.

### Task 13 — 39th fuzzer FuzzMongoDecode — DONE
`FuzzMongoDecode` (4 invariants: no-panic; chain-bytes-unmutated R3; sniffing-off idempotence AMEND-B6; bounded readBuf). **VALIDATION GATE for the Task-8 IsValidName guard: a 60s / 16M-exec fuzz found NO crash** (no `stats: invalid metric name` panic) — the guard holds against un-nameable wire-derived collection/callsite bytes. Fuzzer count now 39. No testdata/fuzz committed. commit `a4c1bdc`.

### Task 14 — 0049 driver part 1 — bootstraps + builders + MultiListener — DONE
The `0049-mongo-requests` cross-side SKELETON: self-contained little-endian mongo wire/BSON builders (D-S29.1-3, shared verbatim with the future 29.2 `0051` driver), the two-listener `[mongo_proxy, tcp_proxy]` bootstraps (`l_default`/mongo_a + `l_commands`/mongo_b), the `MultiListenerDriver` plumbing, and the TCPSink wiring (request-only scope — no response bytes traverse the chain). `Drive*` no-op skeletons + `AssertStats` stub; compile-only. commit `3556326`.

### Task 15 — 0049 driver part 2 — label-aware StatsAsserter + arms 1-5 — DONE
The label-aware `StatsAsserter`: Prometheus parse generalized to `(metric + sorted-label-set) → value` via `canonicalize` (label-order-independent keying), with a precondition that label values are comma/brace-free (true for mongo's identifier-only tags). Arms 1-5 (plain scatter-get query; $cmd commands-list semantics on `l_commands` — isMaster in-list vs foo unknown_command; query-shape variants PrimaryKey/MultiGet + the four flag-bit counters; other request opcodes insert/getMore/killCursors/command; $comment callsite AMEND-C3 double-count). Cumulative per-prefix `want` reasoned per §11.2 and re-verified live cross-side (ref ≡ subj). Cross-side PASS. commit `21c4ad1`.

### Task 16 — 0049 arms 6-9 + 0050-mongo-boot-reject — DONE
Arms 6-9 on `0049` + the sibling `0050-mongo-boot-reject` fixture. **Arm 6** (unsupported opcode 2013 on a FRESH `l_default` conn) → `decoding_error` +1; the first decode error turns sniffing OFF for the connection lifetime (AMEND-B6), so the follow-up VALID OP_QUERY on the SAME conn increments NOTHING — `op_query{mongo_a}` stays at **5, not 6** (the dropped-query proof). **Arm 7** (garbage-BSON: a well-framed OP_QUERY with a bad element type `0x13` on a SEPARATE FRESH conn) fails the BSON walk BEFORE `op_query` increments → `decoding_error` +1; cumulative `decoding_error{mongo_a}` = **2** (a6+a7), at-most-once per connection lifetime (D-S29.1-6). **Arm 8** (assertion-only): exists-at-zero response-side counters present-and-0 both sides/both prefixes; the `op_query_active` gauge `# TYPE … gauge` line present (new `scrapeTypeLine` helper); `cx_destroy_*_with_active_rq` PRESENT, value not compared (AMEND-C2 — the 29.2 increment). **Arm 9** = the R4 deliberate-break (below). `0050-mongo-boot-reject` (the 0047 symmetric `BootRejectFixture` template): a `[mongo_proxy, tcp_proxy]` chain whose mongo `typed_config` OMITS `stat_prefix` → both sides reject at config-load on the shared `stat_prefix` substring (reference PGV `MongoProxyValidationError.StatPrefix` via the echoed bootstrap; envoy-go `mongo_proxy: stat_prefix is required`). Minimal `c_unused` cluster satisfies the zero-cluster boot reject. Runner `0050` blank-import added.

**Bug fix (prior-agent interrupted state):** the `op_query{mongo_a}` expectation had been left at the wrong `want 6`; the ARM-ACCOUNTING table comment already correctly said `5`. The cross-side run had BOTH ref and subj reporting 5 (they AGREE — the reference is ground truth), so `want 6` was the error. Corrected to `5` per the AMEND-B6 dropped-query reasoning; cross-side re-run converged ref ≡ subj ≡ 5 on every keyed counter. No other cell changed (arms 6/7 contribute only to `decoding_error`).

**R4 deliberate-break liveness proof (arm 9; `-count=1` per `reference_differential_break_protocol_count1`, reverted after each):**
- **Break 1 — flip `op_query{mongo_a}` `5 → 99`:** cross-side FAILS on both runner paths; BOTH report the true value `5`:
  ```
  runner_test.go:1082: ref envoy_mongo_op_query{envoy_mongo_prefix="mongo_a"} = 5, want 99
  runner_test.go:1082: subj envoy_mongo_op_query{envoy_mongo_prefix="mongo_a"} = 5, want 99
  --- FAIL: TestDifferential/0049-mongo-requests (4.15s)
  ```
- **Break 2 — disable the §7.4 `name.go` mongo tag-extractor arm:** EVERY `envoy_mongo_*` label-keyed lookup goes ABSENT on the subject side → every keyed assertion fails (the §7.4 arm is load-bearing):
  ```
  runner_test.go:1082: subj: counter envoy_mongo_op_query{envoy_mongo_prefix="mongo_a"} ABSENT (creation / name-shape / label-extraction failure)
  runner_test.go:1082: subj: counter envoy_mongo_op_query_scatter_get{envoy_mongo_prefix="mongo_a"} ABSENT (creation / name-shape / label-extraction failure)
  ... (every envoy_mongo_* keyed lookup ABSENT — op_query family, decoding_error,
      collection.*, cmd.*, the exists-at-zero response-side counters, the gauge) ...
  runner_test.go:1082: subj: gauge envoy_mongo_op_query_active{envoy_mongo_prefix="mongo_a"} ABSENT (creation failure)
  --- FAIL: TestDifferential/0049-mongo-requests
  ```
After BOTH reverts: cross-side `0049` + `0050` GREEN again; `internal/stats/name.go` UNMODIFIED (`git status --short` clean for it). Fixtures 50 → **52**. commit pending (this commit).

### Task 17 — Completion bundle + six-gate — DONE

**Docs authored (Steps 1–4):**
- **BEHAVIOR_CONTRACT.md:** NEW `### envoy.filters.network.mongo_proxy` subsection (after zookeeper_proxy) — request-side decode semantics; the exactly-7-opcode envelope + OP_MSG-not-decoded (upstream parity); sniffing-off-on-error connection-lifetime; the 23-stat EAGER roster + D-P1 boot-window departure; the `mongo.<stat_prefix>.` scope; the four-rule Prometheus tag-extraction table; the dynamic cmd/collection/callsite families + commands-remembering + alias normalization; the USER-APPROVED `stats.IsValidName` guard divergence (coverage boundary; fuzz-validated); the AMEND-C2 `cx_destroy_*` presence-only boundary; the OP_REPLY/OP_COMMANDREPLY recognized-not-decoded 29.1 boundary; forward-pointers to 29.2/29.3. PLUS the `**Phase 29.1 extension — 337 → 360 internal names:**` paragraph (23 new mongo names; roster defined by `mongoproxy/stats.go::rosterSuffixes()` + R2 golden) + the Stat-surface summary roll.
- **DECISIONS.md:** ADR-0224 §Decision (7 numbered items: TypeURL+config, BSON, codec, eager-roster+IsValidName-guard, filter-glue+active-query-list, builtin+bootstrap+name.go-arm, fixtures+fuzzer) + §Consequences (the §9 fourth row; consumer #2 of ADR-0221; the D-P1 boot-window + IsValidName + sub-phase boundaries; tag-extracted-vs-flat; histogram deferral) filled IN-PLACE. DECISIONS tail STAYS ADR-0226 (no new number).
- **STATE.md:** active-phase → `phase 29.1 IMPL done`; next-skill → `superpowers:writing-plans` (the 29.2 SPEC); counts fixtures 52 / fuzzers 39 / stats 360; DECISIONS tail ADR-0226 (next-free ADR-0227); conformance line → "as of the 29.1 six-gate, 2026-06-04"; last-commit → placeholder "the 29.1 IMPL squash SHA (filled post-merge by the controller)".
- **ROADMAP.md:** sub-row 29.1 `in-progress → done (2026-06-04)`; parent row 29 STAYS `in-progress`; 29.2/29.3 STAY `planned`.
- **next-prompt.txt:** rewritten for the 29.2-SPEC cold-start (response side + correlation + gauge inc/dec + cx_destroy value parity + emit_dynamic_metadata + fixture 0051 + TCPMongoResponder BackendKind 30 + the ADR-0225 body at 29.2).

**Verification pins:** `ls -d test/fixtures/[0-9]* | wc -l` = **52** (tail `0050-mongo-boot-reject`); `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` = **39**; BEHAVIOR_CONTRACT extension-paragraph total reads **360**.

**THE SIX-GATE (run honestly; tails quoted):**

- **Gate 1 — `go build ./...`:** `build exit: 0` (clean, no output).
- **Gate 2 — `go vet ./...`:** `vet exit: 0` (clean, no output).
- **Gate 3 — `golangci-lint run`:** `lint exit: 0` (clean, no output).
- **Gate 4 — `go test ./... -race -short`:** `test exit: 0`; no `FAIL`/`panic` lines across the tree; `internal/filter/network/mongoproxy` + `internal/stats` both `ok`. Tail:
  ```
  ok  	github.com/esalaine/envoy-go/test/helpers/oauthbackend	1.009s
  ok  	github.com/esalaine/envoy-go/test/helpers/ratelimitgrpc	1.033s
  test exit: 0
  ```
- **Gate 5 — full 52-dir differential (`go test ./test/differential/ -count=1`):** PASS on RETRY. The first run FAILED on `0027-http-lua-full-bridge` with `subj start: subject ready: EOF` — the documented transient container-startup flake (`reference_docker_probe_bridge_network` env-class, NOT an assertion failure; the `0050-mongo-boot-reject` container output preceding it is the EXPECTED both-sides `MongoProxyValidationError.StatPrefix` / `mongo_proxy: stat_prefix is required` boot-reject, working as designed). Retried once per protocol → GREEN. Final:
  ```
  ok  	github.com/esalaine/envoy-go/test/differential	162.601s
  differential exit: 0
  ```
  This is the FULL 52-dir suite incl. the 50 pre-existing dirs (the §4 zero-touch / R1 back-compat proof — mongoproxy merely registered perturbs nothing) + the new `0049-mongo-requests` (cross-side, 9 arms) + `0050-mongo-boot-reject`.
- **Gate 6a — h2spec (`go test -run TestH2Spec ./test/conformance/h2spec/`):** PASS, **53/53**. Tail:
  ```
        53 tests, 53 passed, 0 skipped, 0 failed
    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
  --- PASS: TestH2Spec (2.55s)
  ```
- **Gate 6b — proxy-wasm (`go test -run TestProxyWasmConformance ./test/conformance/proxy-wasm/`):** PASS, **10/10 families** (security, runtime, wasm_vm, bytecode_util, logging, stop_iteration, shared_data, pairs_util, endianness, + the http/network family). Tail:
  ```
  ok  	github.com/esalaine/envoy-go/test/conformance/proxy-wasm	0.246s
  proxywasm exit: 0
  ```

**ALL SIX GATES GREEN.** The asserted-unaffected HTTP path (h2spec 53/53 + proxy-wasm 10/10) re-confirmed — 29.1 touches no HTTP path (§4 zero-touch). Phase 29.1 IMPL COMPLETE.
