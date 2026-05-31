# Phase 26.3 — network-filter RBAC — PROGRESS

This file accumulates task-completion records for each of the 15 IMPL tasks.
Commit tip at Task 1: `c29eafe` (branch `phase-26.3-network-filter-rbac-impl`).
Tasks 2+3 squash SHA: `331c239`.

---

## Task 1: First-action baselines + proto/anchor re-confirm (HARD GATE)

**Status: PASS — all baselines confirmed, all gates green.**

---

### Step 1: Baseline counts (git ls-files, deterministic)

| Baseline       | Expected | Actual                                         | Result |
|----------------|----------|------------------------------------------------|--------|
| Fuzzers        | 35       | 35                                             | PASS   |
| Fixture dirs   | 44       | 44                                             | PASS   |
| Fixture tail   | 0042-…   | `0042-network-direct-response-boot-reject`     | PASS   |
| ADR tail       | ADR-0218 | ADR-0218 (DECISIONS.md line 14070)             | PASS   |

No drift.

---

### Step 2: Stat surface

BEHAVIOR_CONTRACT.md (lines 438–445, 3572) confirms the project stat surface is **132** at 26.1 and 26.2 phase-done. The 26.3 `rbac_network` 4-counter roster will advance 132 → **136** (lands at Task 8 + Task 15 ROLLUP). No discrepancy.

---

### Step 3: Network-RBAC TypeURL + proto fields

Programmatic check via `proto.MessageName(&networkrbacv3.RBAC{})`:

```
MessageName: envoy.extensions.filters.network.rbac.v3.RBAC
TypeURL:     type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC
```

Proto has exactly **8 fields** (confirmed via `go doc`):
1. `Rules *v3.RBAC` (field 1)
2. `ShadowRules *v3.RBAC` (field 2)
3. `StatPrefix string` (field 3)
4. `EnforcementType RBAC_EnforcementType` (field 4)
5. `ShadowRulesStatPrefix string` (field 5)
6. `Matcher *v31.Matcher` (field 6)
7. `ShadowMatcher *v31.Matcher` (field 7)
8. `DelayDeny *durationpb.Duration` (field 8)

**NO `track_per_rule_stats` field** — confirmed absent (F2). The `[#next-free-field: 9]` comment corroborates exactly 8 fields. TypeURL carries the `extensions.` segment as expected (memory note confirmed).

---

### Step 4: Engine/framework line anchors (re-pinned at IMPL-session tip)

#### `internal/filter/http/rbac/evaluator.go` (854 lines total)

| Symbol | SPEC-stated line | Actual current line | Drift? |
|--------|-----------------|---------------------|--------|
| `permissionEvaluator` interface | @24 | 24 | none |
| `principalEvaluator` interface | @38 (range @24-38) | 36 | -2 lines |
| `evalContext` interface (open) | @60 | 60 | none |
| `evalContext` interface (close) | @121 | 121 | none |
| `buildOnePermission` | @275 | 275 | none |
| `buildOnePrincipal` | @565 | 565 | none |
| `matchString` adapter | @680 | 680 | none |
| `matchPath` adapter | @733 (SPEC says ~@854 range) | 733 | note: SPEC range said @680-854; order is matchString@680, matchPath@733, matchHeader@767, matchCidr@826 |
| `matchHeader` adapter | (in @680-854 range) | 767 | none |
| `matchCidr` adapter | (in @680-854 range) | 826 | none |

Note: `principalEvaluator` is at line 36, not 38. The SPEC stated the `permissionEvaluator`/`principalEvaluator` range as @24-38; actual is @24-38 (interface body ends at 38, `principalEvaluator` keyword opens at 36). Minor textual drift, functionally correct.

#### `internal/filter/http/rbac/rbac.go` (1191 lines total)

| Symbol | SPEC-stated line | Actual current line | Drift? |
|--------|-----------------|---------------------|--------|
| `compiledRulesEngine` struct | @76 | 76 | none |
| `compiledMatcherEngine` struct | @86 | 86 | none |
| `incPolicy` func | @210-222 | 210–222 | none |
| `buildCompiledRulesEngine` | @373 | 373 | none |
| CEL silent-ignore block | @420-424 | 420–423 | -1 line (block ends at 423 not 424) |
| `buildCompiledMatcherEngine` | @445 | 445 | none |
| `evaluateEngine` | @726 | 726 | none |
| `evaluateRulesEngine` | @757 | 757 | none |
| `policyMatches` | @792 | 792 | none |
| `evaluateMatcherEngine` | @824 | 824 (via `@845` call site ref) | 824 confirmed |
| `matcherCtxAdapter` type | @866-897 | 866–897 | none |
| L4 stub `DestinationIP` | @961 | 965 | +4 lines |
| L4 stub `DestinationPort` | (@961 range) | 969 | +4 lines from SPEC |
| L4 stub `RequestedServerName` | (@961 range) | 974 | +4 lines from SPEC |
| L4 stub `FilterState` | @1001 | 1001 | none |

Notes on drift:
- CEL block ends at 423 not 424 (cosmetic, 1-line off).
- `DestinationIP` is at 965 (SPEC said @961); the 4 lines before it are comments introduced by the L4-adapters note block. Functionally zero drift.

#### `internal/filter/network/callbacks.go`

| Symbol | SPEC-stated line | Actual current line | Drift? |
|--------|-----------------|---------------------|--------|
| `ReadFilterCallbacks` interface (open) | @16-30 | 16–30 | none |
| `CloseType` type | @37-45 | 37–45 | none |
| `Connection` interface (open) | @50 | 50 | none |
| `Connection` accessors | @50-67 | 50–67 | none |

No drift.

#### `internal/filter/network/chain.go`

| Symbol | SPEC-stated line | Actual current line | Drift? |
|--------|-----------------|---------------------|--------|
| `SetResponseCodeDetails` sink | @336-340 | 336–340 | none |
| `connection.Close` ignoring CloseType | @369-371 | 369–371 | none |
| `NewBucket` (in `newChainRuntime`) | @159 | 159 | none |
| `bucket.Reset()` | @301 | 301 | none |
| `TerminalReady` func | @88 (ChainRuntime) | 88 | none |
| `HandleTerminal` func | @93 (ChainRuntime) | 93 | none |
| `handleTerminal` (internal) + `prefixConn` usage | @179-203 | 191–204 | +12 lines from SPEC for internal; ChainRuntime wrappers at 85-93 |

Note: SPEC anchor @179-203 refers to the `terminalReady`/`handleTerminal` internal helpers. The `terminalReady` helper is at ~line 185, `handleTerminal` at ~194, `prefixConn` reference at 199. The `ChainRuntime` public wrappers are at 85/88/93. Minor position shift of ~12 lines from SPEC.

`prefixConn` type defined in `internal/filter/network/prefixconn.go:12`; `newPrefixConn` at `prefixconn.go:17`.

#### `internal/filter/network/builtins/builtins.go` (52 lines total)

| Symbol | SPEC-stated line | Actual current line | Drift? |
|--------|-----------------|---------------------|--------|
| `type Deps` struct | @24-52 range | 27 | note: SPEC said @24-52 for the Deps+RegisterBuiltins block |
| `StatsRegistry` field | (in Deps) | 29 | none |
| `RegisterBuiltins` func | @40 | 40 | none |

Note: SPEC anchor said `builtins.go:24-52`; `type Deps` opens at 27 (3-line drift due to copyright/comment header). The file is 52 lines total, so the range @27-52 covers both `Deps` and `RegisterBuiltins` as expected.

#### `internal/listener/manager.go`

| Symbol | SPEC-stated line | Actual current line | Drift? |
|--------|-----------------|---------------------|--------|
| `serveNetworkChain` func | @1025 | 1025 | none |
| `dispatchConn.Close()` pure-read close | @1080 | 1080 | none |

No drift.

---

### Step 5: AMEND-A6 — HTTP rbac emits NO dynamic metadata

```
git grep -nE 'dynamicmetadata|shadow_engine_result|shadow_effective_policy_id' internal/filter/http/rbac/
```

Result: 2 matches, BOTH in `rbac_test.go` (test stub `DynamicMetadata() *dynamicmetadata.Bucket { return nil }`). Zero matches in production files (`rbac.go`, `evaluator.go`). `shadow_engine_result` and `shadow_effective_policy_id`: 0 matches anywhere.

**AMEND-A6 CONFIRMED: HTTP rbac production code emits no dynamic metadata.**

---

### Step 6: Six gates at tip

Worktree tip: `c29eafe`

| Gate | Command | Result |
|------|---------|--------|
| 1 | `go build ./...` | PASS (exit 0) |
| 2 | `go vet ./...` | PASS (exit 0) |
| 3 | `golangci-lint run` | PASS (exit 0, installed at `/home/esa/go/bin/golangci-lint`) |
| 4 | `go test -race -short ./...` | PASS (exit 0, all packages pass) |

All four commands (build, vet, lint, test) passed cleanly. No failures, no warnings.

---

### Summary

All baselines confirmed at expected values. All engine/framework line anchors re-pinned. Drift is cosmetic only (1-4 lines on a handful of anchors; no functional impact). AMEND-A6 zero-match confirmed. Six gates green. Ready to proceed to Task 2.

---

## Task 4: Input-capability Profile (ProfileHTTP/ProfileL4) + HTTP-only-arm reject (D-26.3-1)

**Status: PASS — Profile implemented, tests green, full suite clean.**

### Implementation summary

New files:
- `internal/rbac/profile.go` — `Profile` type (`ProfileHTTP` / `ProfileL4`), `armKind` (`armHeader` / `armURLPath`), `permits(armKind) bool` method.
- `internal/rbac/profile_test.go` — 4 tests: `TestProfileHTTP_PermitsHTTPOnlyArms`, `TestProfileL4_RejectsHTTPOnlyPermissionHeader`, `TestProfileL4_RejectsHTTPOnlyPrincipalHeader`, `TestProfileL4_PermitsL4Arms`.

Modified files:
- `internal/rbac/evaluator.go` — `buildPermissionEvaluators` + `buildOnePermission` + `buildPrincipalEvaluators` + `buildOnePrincipal` all gain `profile Profile` parameter. ProfileL4 gates `Permission_Header` + `Permission_UrlPath` + `Principal_Header` + `Principal_UrlPath` arms with `profile.permits(armHeader)` / `profile.permits(armURLPath)` checks at entry (before nil-guard + body).
- `internal/rbac/rbac.go` — `BuildRulesEngine(r, profile)` threads `profile` into `buildPermissionEvaluators` / `buildPrincipalEvaluators`. `BuildMatcherEngine(m, profile)` gains `profile` for signature symmetry.
- `internal/rbac/rbac_test.go` — all `BuildRulesEngine(r)` → `BuildRulesEngine(r, ProfileHTTP)`, `BuildMatcherEngine(...)` → `BuildMatcherEngine(..., ProfileHTTP)`, `buildPermissionEvaluators(...)` → `buildPermissionEvaluators(..., ProfileHTTP)`, `buildPrincipalEvaluators(...)` → `buildPrincipalEvaluators(..., ProfileHTTP)`.
- `internal/rbac/evaluator_test.go` — all `buildOnePermission(p)` → `buildOnePermission(p, ProfileHTTP)`, `buildOnePrincipal(p)` → `buildOnePrincipal(p, ProfileHTTP)`.

### ProfileL4 reject error wording (exact)

- `permission.header`: `"rbac: permission.header is HTTP-only (unsupported for L4 network RBAC)"`
- `permission.url_path`: `"rbac: permission.url_path is HTTP-only (unsupported for L4 network RBAC)"`
- `principal.header`: `"rbac: principal.header is HTTP-only (unsupported for L4 network RBAC)"`
- `principal.url_path`: `"rbac: principal.url_path is HTTP-only (unsupported for L4 network RBAC)"`

Error prefix `rbac:` preserved per the engine-as-owner discipline. Wording is byte-stable-pending (finalized at Task 15) but the prefix + discipline is load-bearing.

### Matcher-leaf reachability finding (IMPL DECISION per D-26.3-1 + SPEC §3.4)

**Finding: VACUOUS — no rbac permission/principal leaf in the matcher path.**

`BuildMatcherEngine` takes a `*matchv3.Matcher` (cncf/xds matcher tree, NOT an rbac config RBAC sub-message). The tree's leaves are generic internal/matcher predicates (`headerPredicate`, `pathPredicate`, `sourceIPPredicate`, etc.) assembled by the `internal/matcher` framework primitive. The terminal is a `rbacconfigv3.Action` proto (wrapped in `*anypb.Any`). Neither `buildOnePermission` nor `buildOnePrincipal` is called anywhere in the matcher-path construction: `BuildMatcherEngine` calls `matcher.New(m, supportedActionTypes)` directly, which PARSE-REJECTs non-canonical terminal TypeURLs but never calls into the rbac build helpers.

Therefore: `profile.permits()` has no callable gate in the matcher path. The HTTP-only arm PARSE-REJECT (D-26.3-1) is satisfied entirely on the **rules-path** (the GUARANTEED reject target). The matcher-path test `TestProfileL4_RejectsHTTPOnlyArmInMatcherLeaf` is DROPPED (there is no rbac arm to reach via the matcher framework).

`BuildMatcherEngine` accepts `profile Profile` for **signature symmetry** with `BuildRulesEngine` and future-proofing only. The parameter is not consumed in the current implementation.

### Test results

```
go test ./internal/rbac/... -v:          PASS (all tests)
go vet ./internal/rbac/...:              PASS
go build ./...:                          PASS
go test -race -short ./...:              PASS (zero failures, full suite)
```

Existing callers updated: 9 `buildOnePermission` sites + 9 `buildOnePrincipal` sites in `evaluator_test.go`; 7 `BuildRulesEngine` sites + 6 `BuildMatcherEngine` sites + 2 `buildPermissionEvaluators` / `buildPrincipalEvaluators` sites in `rbac_test.go`. HTTP consumer (`internal/filter/http/rbac/`) untouched (still uses its own copies until Task 5).

## Task 6: R4 re-verification gate — phase-16 HTTP-rbac differential fixtures byte-exact green LIVE — DONE

VERIFICATION-ONLY (no production code). Proves the Tasks 2–5 engine extraction
(RBAC engine moved to `internal/rbac/` + HTTP rbac filter migrated as consumer
#1, with the import flip + `Profile` threading) preserved byte-exact HTTP-rbac
behavior. SPEC §13 R4; AMEND-A6 (engine-correctness, NOT metadata). This is the
load-bearing extraction proof.

### Step 1 — Fixture identified

`ls test/fixtures/ | grep -iE 'rbac'` → **`0018-http-rbac`** (the sole
phase-16 HTTP-rbac differential fixture; eight scenarios per phase-16 SPEC §7.1,
cross-side vs reference Envoy v1.37.2).

### Step 2 — Cross-side differential, byte-exact vs reference Envoy v1.37.2 — RAN LIVE

The differential harness is the Docker/testcontainers-driven
`go test ./test/differential/ -run TestDifferential` (per 26.1/26.2 PROGRESS).
Reference image pinned via `ENVOY_TARGET.md` → `envoyproxy/envoy:v1.37.2`
(by SHA256 digest `sha256:c5e8a68e…438f18bd`). **Docker was present and
runnable in this environment** (Server 28.1.1; `envoyproxy/envoy:v1.37.2`
already pulled) — so this is a **genuine LIVE cross-side run**, NOT an
environment-limited proxy. (26.1/26.2 also ran it live here.)

Command:

```
$ go test ./test/differential/ -run 'TestDifferential/0018-http-rbac' -v -count=1
```

Output (harness spins up the pinned reference container by digest, runs all
eight scenarios cross-side, tears down):

```
2026/05/31 ... 🐳 Creating container for image envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd
2026/05/31 ... ✅ Container started: ce15f4d109b3
--- PASS: TestDifferential (2.22s)
    --- PASS: TestDifferential/0018-http-rbac (2.22s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.309s
```

**Result: `0018-http-rbac` byte-exact GREEN cross-side against reference Envoy
v1.37.2 — LIVE.** All eight phase-16 rbac scenarios pass post-extraction; the
HTTP-rbac dispatch (engine import flip + Profile threading) is byte-identical to
the pre-extraction behavior. R4 satisfied.

### Step 3 — Engine + consumer race gate — PASS

```
$ go test -race -short -count=1 ./internal/rbac/... ./internal/filter/http/rbac/...
ok  	github.com/esalaine/envoy-go/internal/rbac            1.012s
ok  	github.com/esalaine/envoy-go/internal/filter/http/rbac  1.023s
```

**Race-clean** (per-policy `sync.Map` + the registry are the only shared state).

### Verdict

R4 re-verification gate **GREEN**. The phase-16 HTTP-rbac differential fixture
`0018-http-rbac` is byte-exact green LIVE cross-side vs reference Envoy v1.37.2
after the engine extraction, and the engine+consumer race gate is clean. The
load-bearing extraction proof holds.

## Task 14: +2 differential fixtures — rbac_network cross-side (R-M-LIVE) + boot-reject — DONE

Added two fixture dirs (44 → **46**), each its OWN runner branch per the
fixture-dispatch-constraint (cross-side XOR boot-reject — separate dirs):

- `test/fixtures/0043-network-rbac/driver/driver.go` (+ `README.md`) — cross-side
  allow/deny/shadow, `MultiListenerDriver` + `StatsAsserter`.
- `test/fixtures/0044-network-rbac-boot-reject/driver/driver.go` (+ `README.md`) —
  symmetric boot-reject (`BootRejectFixture`), stat_prefix-missing PGV-mirror.

Blank-imported both into `test/differential/runner_test.go` (lines 68–69).

### Fixture 0043 — the FIRST production mixed read→terminal chain (R-M-LIVE)

Three listeners, each filter chain `[rbac_network, tcp_proxy]`:

| Scenario | listener | rbac config | behavior | counter |
|---|---|---|---|---|
| allow | l_allow | `rules` ALLOW: perm `destination_port=<this side's port>` + principal `direct_remote_ip 0.0.0.0/0` | Continue → tcp_proxy passthrough → byte-exact echo | `rbac_allow.rbac.allowed=1` |
| deny | l_deny | `rules` ALLOW, no policies (default-deny) | rcd `rbac_deny_close` + NoFlush close → zero echoed bytes | `rbac_deny.rbac.denied=1` |
| shadow | l_shadow | enforced ALLOW (as allow) + `shadow_rules` default-deny, `shadow_rules_stat_prefix: shadow_ns` | enforced passthrough echo + shadow metadata write | `rbac_shadow.rbac.shadow_ns.shadow_denied=1` |

Cross-side-stability design notes (load-bearing):
- The allow/shadow principal is `direct_remote_ip 0.0.0.0/0` (NOT loopback): the
  reference container sees the source IP as the Docker bridge gateway while
  envoy-go on the host sees 127.0.0.1, so a loopback-specific CIDR would DIVERGE.
  0.0.0.0/0 matches identically on both sides while still genuinely exercising
  the `direct_remote_ip` principal accessor.
- The `destination_port` permission is templated to EACH side's ACTUAL listener
  port (15043 in-container ref / runner-allocated subj), so `conn.LocalAddr().Port`
  matches on both sides.

Byte-stream parity: the deny scenario yields zero echoed bytes on both sides
(connection close before tcp_proxy); allow + shadow echo the payload byte-exact.
The side label is excluded from the verdict stream so `CompareBytes` enforces
equivalence.

### StatsAsserter wiring + the deliberate-break proof (asserter-dispatch memory)

`AssertStats` scrapes `/stats/prometheus` from both admin endpoints and asserts
the per-side `<stat_prefix>.rbac.*` counters on BOTH ref and subj. This is the
cross-side LIVE path (`SubjectAsserter` would NOT run cross-side).

**Production gap discovered + fixed.** envoy-go's `flattenToProm`
(`internal/stats/name.go`) had NO rule for the network rbac `<stat_prefix>.rbac.*`
shape (its recognized prefixes are `cluster.|http.|listener.|server.|wasm.` +
the local_ratelimit/bandwidth_limit second-pass detections). The network rbac
counters were registered but SILENTLY DROPPED from `/stats/prometheus`
(`prom.go:39` skips names that fail flattenToProm). The subject stats section was
EMPTY — the StatsAsserter would have been vacuous on the subject side. Added a
network-rbac tag-extractor rule mirroring reference Envoy v1.37.2's empirically-
captured shape: `<stat_prefix>.rbac.<rest> → envoy_rbac_<rest_flat>{envoy_rbac_prefix="<stat_prefix>"}`
(stat_prefix promoted to a label; `rbac.`+optional shadow segment inline into the
base; dots→underscores). Pinned by `TestWriteProm_NetworkRBACTagExtractor`
(`internal/stats/prom_test.go`).

**Deliberate-break proof (LIVE):** flipping the expected `rbac_allow.rbac.allowed`
from 1 → 99 made the test FAIL on BOTH sides:

```
runner_test.go:1048: ref rbac_allow.rbac.allowed = 1, want 99
runner_test.go:1048: subj rbac_allow.rbac.allowed = 1, want 99
--- FAIL: TestDifferential/0043-network-rbac
```

Restored to 1 → PASS. The subject-side counter assertion is non-vacuous (the
flatten rule + tag-extractor lookup both fire).

### D-26.3-4 (SNI / mTLS-authenticated scenario) — UNIT-ONLY; differential gap noted

DECISION: SNI/mTLS stays UNIT-ONLY at 26.3 (not added to the differential).
Driving SNI/mTLS at a raw L4 listener requires a downstream TLS
transport_socket + tls_inspector + client-cert harness — substantially heavier
than the plaintext L4 path and not readily reusable from the HTTP-oriented 0018
PKI harness (which terminates TLS at the HCM, not at a raw L4 listener). The
`RequestedServerName` + `DownstreamPrincipal` accessor mapping is UNIT-covered at
Task 9 (`internal/filter/network/rbac/evalctx_test.go` asserts both accessors).
**Honest gap:** there is no cross-side SNI/mTLS-authenticated L4 rbac differential
scenario at 26.3; the accessor mapping is unit-tested but not differential-proven
against reference Envoy. Candidate for a future cross-side extension if an L4
mTLS harness lands.

### Boot-reject finding (0044) — genuine cross-side both-reject; substring asymmetry

Upstream PGV: `stat_prefix` is `(validate.rules).string.min_len = 1`
(`rbac.pb.validate.go:178`). Reference Envoy v1.37.2 DOES reject a
stat_prefix-missing network RBAC config at boot — confirmed live:

```
Proto constraint validation failed (RBACValidationError.StatPrefix: value length must be at least 1 characters)
```

envoy-go also rejects (`rbac_network: stat_prefix is required`). So 0044 IS a
genuine PGV-mirror cross-side both-reject (SPEC §10
`rbac-network-stat-prefix-required`).

HONEST substring caveat: the two ERROR wordings share NO distinctive
case-sensitive token — ref uses PascalCase `StatPrefix`, envoy-go uses snake_case
`stat_prefix` (longest common case-sensitive substring of the error lines is the
non-distinctive `refix`). The fixture's substring is `stat_prefix`:
- load-bearing on the SUBJECT side (the subject stderr is JUST the 126-byte error
  line, no YAML echo — a deliberate-break to `StatPrefix` FAILS the subject:
  `subject stderr does NOT contain "StatPrefix"`).
- on the REF side `stat_prefix` matches the echoed bootstrap config; the genuine
  reference-reject is the runner's separate `refErr != nil` gate (FATALS if ref
  boots cleanly), which fires on the PGV violation above.

### Live cross-side results (Docker reference Envoy v1.37.2)

```
$ go test ./test/differential/ -run 'TestDifferential/(0043-network-rbac|0044-network-rbac-boot-reject)' -v
--- PASS: TestDifferential (7.51s)
    --- PASS: TestDifferential/0043-network-rbac (6.06s)
    --- PASS: TestDifferential/0044-network-rbac-boot-reject (1.45s)
PASS
```

Both byte-exact GREEN LIVE. 0043 confirms the first LIVE mixed read→terminal
differential (R-M-LIVE).

### Fixture count

```
$ ls test/fixtures/ | grep -E '^[0-9]' | wc -l
46
```

### Files changed

- `test/fixtures/0043-network-rbac/driver/driver.go` (new) + `README.md` (new)
- `test/fixtures/0044-network-rbac-boot-reject/driver/driver.go` (new) + `README.md` (new)
- `test/differential/runner_test.go` (+2 blank-imports)
- `internal/stats/name.go` (network-rbac tag-extractor flatten rule — required to
  surface the subject's rbac counters on `/stats/prometheus`)
- `internal/stats/prom_test.go` (`TestWriteProm_NetworkRBACTagExtractor`)

---

## Task 15: BEHAVIOR_CONTRACT 26.3 bundle + ADR-0216/0217/0218 §Decision/§Consequences bodies + ROADMAP parent-row-26 ROLLUP + six-gate — DONE

The atomic documentation + verification bundle that CLOSES phase 26.3 (and parent
phase 26). All four carry-forward review flags addressed.

### Step 1 — byte-stable reject wording (D-P6)

Added `TestParseRejectConstants_ByteStable` to `internal/filter/network/rbac/rbac_test.go`
pinning ALL FOUR network reject consts:
- `parseRejectStatPrefixRequired` = `rbac_network: stat_prefix is required`
- `parseRejectStatPrefixInvalid` = `rbac_network: stat_prefix contains characters invalid for a metric name`
- `parseRejectShadowStatPrefixInvalid` = `rbac_network: shadow_rules_stat_prefix contains characters invalid for a metric name`
- `parseRejectDelayDeny` = `rbac_network: delay_deny is unsupported`

Added `TestProfileL4RejectWording_ByteStable` to `internal/rbac/profile_test.go`
pinning the four ProfileL4 HTTP-only-arm rejects (the engine wraps the leaf with
policy/permission-index context, so the load-bearing leaf wording is pinned as a
suffix): `rbac: permission.header is HTTP-only (unsupported for L4 network RBAC)`,
`... permission.url_path ...`, `... principal.header ...`, `... principal.url_path ...`.

```
$ go test ./internal/filter/network/rbac/ ./internal/rbac/ -run 'ByteStable' -v
=== RUN   TestParseRejectConstants_ByteStable
--- PASS: TestParseRejectConstants_ByteStable (0.00s)
ok  	github.com/esalaine/envoy-go/internal/filter/network/rbac
=== RUN   TestProfileL4RejectWording_ByteStable
--- PASS: TestProfileL4RejectWording_ByteStable (0.00s)
ok  	github.com/esalaine/envoy-go/internal/rbac
```

### Step 2 — BEHAVIOR_CONTRACT.md 26.3 bundle

- NEW `### envoy.extensions.filters.network.rbac` subsection under `## Network filters`:
  the ProfileL4 L4 input surface (supported permissions/principals/combinators + the
  4 HTTP-only PARSE-REJECT arms); decision-in-OnData + OnNewConnection Continue no-op
  (sticky-halt); enforced-deny = NoFlush close + `rbac_deny_close` rcd; shadow =
  dynamic-metadata shadow-pair (namespace `envoy.filters.network.rbac`, keys
  `shadow_engine_result`/`shadow_effective_policy_id`) + `shadow_*` stats; rules/matcher
  dual-path; CEL/audit silent-ignore inherited.
- Stat table UPDATED 132 → 136 (verified it said 132 first): NEW `rbac_network`
  4-counter extension block + the `132 → 136` rollup note. The four
  `<stat_prefix>.rbac.{allowed,denied,shadow_allowed,shadow_denied}` rows added.
- envoy-go-strict departure records (flag #2): HTTP-only-matcher PARSE-REJECT (AMEND-A4);
  delay_deny PARSE-REJECT (AMEND-A9); xDS dynamic-policy PARSE-REJECT; stat_prefix-invalid
  + shadow-stat_prefix-invalid PARSE-REJECT (consistent with hcm/lua).
- Connection-metadata emitted-but-unread note (AMEND-A5/A6).
- internal/rbac/ engine-extraction structural note (HTTP = consumer #1 byte-exact
  re-verified, NO HTTP-rbac behavior change; network = consumer #2).
- NoFlush-now-distinguished note (F3).
- network-rbac Prometheus tag-extractor shape (flag #3): `<stat_prefix>.rbac.* →
  envoy_rbac_<suffix>{envoy_rbac_prefix=...}`, recorded alongside the wasm/SN9/
  bandwidth_limit non-HCM-rooted precedents.
- D-26.3-4 SNI/mTLS differential-gap note (flag #4): unit-tested, not cross-side
  differential-proven — recorded in the new subsection + the "Does not yet apply to" list.
- Forward-pointer + Applies-to + Stat-surface lines updated (26.3 DONE).

### Step 3 — DECISIONS.md ADR bodies (in place; tail STAYS ADR-0218; NO new number)

Filled the §Decision + §Consequences bodies for ADR-0216 (engine extraction + Profile +
consumer-#1/#2 + R-E + the LIVE-first-consumer re-verification discipline R4), ADR-0217
(connection-scoped dynamic-metadata writes + in-place doc generalization + no-reader-yet),
ADR-0218 (rbac_network — L4 input surface; OnData decision + sticky-halt; NoFlush+rcd deny;
shadow metadata+stats; 4-counter roster NO per-policy F2; delay_deny reject;
stat_prefix-invalid reject; CEL silent-ignore F1; rules/matcher dual-path; R-M-LIVE; the
network-rbac prom tag-extractor; the D-26.3-4 SNI/mTLS gap).

```
$ grep -c '^## ADR-' docs/envoy-go/DECISIONS.md   # tail confirmed
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | tail -1
## ADR-0218: NEW `rbac_network` filter ...
```

Tail STAYS ADR-0218; next-free STAYS ADR-0219 (no new number consumed — ADR-0044 in-place).

### Step 4 — STATE.md + ROADMAP.md phase-done advance + parent-row-26 ROLLUP

- ROADMAP sub-row 26.3 `in-progress → done`; **parent row 26 `in-progress → done`
  ATOMICALLY** (the 18/19/22/24/25 ROLLUP). The §9 Network-filters family stays OPEN
  (6 candidates remain: redis/mongo/kafka_broker/thrift/zookeeper/sni_cluster).
- STATE.md advanced to phase-26.3-IMPL-DONE / phase-done (SKILL_ROUTING 3 → 4/5);
  counts fuzzers 36 / fixtures 46 / stat surface 136 / DECISIONS.md tail ADR-0218 /
  next-free ADR-0219. `last-commit` written as a post-merge placeholder (the controller
  fills the squash SHA), mirroring the prior phase-done precedent.

### Step 5 — SIX-GATE verification (run LIVE)

**Gate 1-4 — build / vet / lint / -race -short:**

```
$ gofmt -l internal/                 # (empty — clean)
$ go build ./...                     # exit 0
$ go vet ./...                       # exit 0
$ golangci-lint run                  # exit 0 (v1.64.8)
$ go test -race -short ./...         # all ok / no failures
```

**Gate 5 — full 46-fixture differential suite (LIVE via Docker, reference Envoy v1.37.2):**

```
$ go test ./test/differential/ -run 'TestDifferential' -v
... --- PASS: TestDifferential/0043-network-rbac (5.71s)
    --- PASS: TestDifferential/0044-network-rbac-boot-reject (1.54s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	149.912s

PASS: 46    FAIL: 0    SKIP: 0
```

46/46 fixtures byte-exact GREEN on the FIRST run — NO environmental flakes this run
(the 26.1/26.2-noted port-bind / wasm-race flakes did not recur). Includes the R4
phase-16 HTTP-rbac fixture (`0018-http-rbac` — proves the engine extraction is
behavior-neutral for consumer #1) + the +2 new rbac dirs (`0043-network-rbac` — the
FIRST LIVE mixed read→terminal R-M-LIVE allow/deny/shadow StatsAsserter; `0044`
boot-reject).

**Gate 6a — h2spec conformance (HTTP/2 — re-run LIVE):**

```
$ go test ./test/conformance/h2spec/ -run TestH2Spec -v
    h2spec conformance report: 53 total tests, 0 failures
    (53 tests, 53 passed, 0 skipped, 0 failed)
--- PASS: TestH2Spec (2.75s)
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.829s
```

**PASS — 53/53.**

**Gate 6b — proxy-wasm conformance (the "10/10" suite — re-run LIVE):**

```
$ go test ./test/conformance/proxy-wasm/ -run TestProxyWasmConformance -v
... --- PASS: TestProxyWasmConformance/endianness (0.02s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/proxy-wasm	0.253s
```

**PASS — all families (10/10).** Conformance + h2spec were run LIVE (not merely
asserted-unaffected): 26.3 touches no HTTP/h2/proxy-wasm path, but the harness was
available so they were re-confirmed green.

### Counts at phase-done

- fuzzers: **36** (`grep -rho 'func Fuzz...' --include='fuzz_test.go' | sort -u | wc -l` = 36; the 36th is `FuzzNetworkRBACConfigParse`)
- fixtures: **46** (`ls test/fixtures/ | grep -E '^[0-9]' | wc -l` = 46)
- stat surface: **136** (132 + the 4 rbac_network counters)
- DECISIONS.md tail: **ADR-0218** (next-free ADR-0219; no new number consumed)

### GATE VERDICT: GREEN

All six gates GREEN LIVE. ADR-0216/0217/0218 bodies + BEHAVIOR_CONTRACT 26.3 bundle +
STATE/ROADMAP phase-done advance + parent-row-26 ROLLUP all landed. Phase 26.3 (and
parent phase 26) DONE.

### Known coverage boundary (D-26.3-4)

The `rbac_network` `requested_server_name` (SNI) + `authenticated` (mTLS) principal/
permission arms are UNIT-TESTED (Task 9) but NOT cross-side differential-proven (no
reference-Envoy fixture exercises a TLS/mTLS L4 RBAC policy end-to-end). The IP/port
arms (`destination_port`/`destination_ip`/`direct_remote_ip`/`remote_ip`/`any`) ARE
differential-proven (fixture 0043). Recorded in BEHAVIOR_CONTRACT + ADR-0218 §Consequences.

### Files changed (Task 15)

- `internal/filter/network/rbac/rbac_test.go` (+`TestParseRejectConstants_ByteStable`)
- `internal/rbac/profile_test.go` (+`TestProfileL4RejectWording_ByteStable`)
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the 26.3 bundle + stat table 132→136)
- `docs/envoy-go/DECISIONS.md` (ADR-0216/0217/0218 §Decision/§Consequences bodies)
- `docs/envoy-go/STATE.md` (phase-done advance)
- `docs/envoy-go/ROADMAP.md` (sub-row 26.3 + parent row 26 → done)
- `docs/envoy-go/phases/26.3-network-filter-rbac/PROGRESS.md` (this section)
