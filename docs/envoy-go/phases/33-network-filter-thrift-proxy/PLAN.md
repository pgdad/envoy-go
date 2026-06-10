# Phase 33 Implementation Plan — `thrift_proxy` network filter (`envoy.filters.network.thrift_proxy`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Per `feedback_execution_style` run subagent-driven; per `feedback_git_worktrees` the IMPL runs in a worktree; per `feedback_subagents_no_push` subagents commit LOCAL-ONLY (the controller squash-merges + pushes at stage-close); per `feedback_pertask_gofmt_lint` each task runs `gofmt -l` + `golangci-lint run` on touched packages before commit.

**Goal:** Land `envoy.filters.network.thrift_proxy` — the project's SECOND terminal routing proxy (after redis_proxy) — at a single-pair single-route-terminal MVP: decode the Thrift **framed × binary** message-begin envelope under `payload_passthrough`, route by method name through a `route_config` exact-match/match-all route to ONE upstream cluster, round-trip each request through the REUSED ADR-0230 upstream-pool seam, answer a routing miss with a LOCAL `UnknownMethod` Thrift exception (zero upstream), and emit the EAGER 25-name `thrift.<stat_prefix>.` stat roster — in ONE flat phase, framework-ZERO-touch, ZERO new go.mod dep.

**Architecture:** A NEW `internal/filter/network/thriftproxy/` package implements `network.TerminalFilter` (the redis_proxy package shape): an in-house Thrift codec (`thrift.go`), a `route_config` method-routing table (`route.go`), the EAGER 25-name roster (`stats.go`), and a `TerminalFilter.Handle` request→reply pump (`filter.go`) that REUSES the ADR-0230 `UpstreamConn` seam UNCHANGED (the FIRST reuse). The differential proof is TWO-pronged: downstream-Thrift-response byte-equivalence PLUS cross-side `StatsAsserter`. The §9-family-CLOSING row.

**Tech Stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); go-control-plane `/envoy v1.32.4` (ADR-0008 — thrift_proxy/v3 is a CORE `/envoy` subpackage where redis_proxy/v3 lives → ZERO new go.mod dep, D-T1). REUSES `internal/filter/network/` (the `TerminalFilter.Handle` seam + the ADR-0230 upstream-pool seam, unchanged), `internal/cluster/`, `internal/filter/network/redisproxy/` (the package SHAPE), `internal/stats/`, the differential harness + `StatsAsserter`.

**Authoritative inputs (do NOT re-litigate; do NOT re-run the empirical probes):**
- `docs/envoy-go/phases/33-network-filter-thrift-proxy/SPEC.md` — §1.1 AMEND-T1..T7; §5/§6 the proto/reject rosters; §7 the 25-name roster + the `thrift.` hoist arm; §8 the `0057`/`0058` fixtures + `TCPThriftResponder`; §10 the spine this PLAN decomposes; **§11 the D-T1..D-T9 empirical-pin block + Appendix A the framed×binary wire format are AUTHORITATIVE**.
- `docs/envoy-go/DECISIONS.md` — ADR-0231 §Context (anchored at the SPEC; the §Decision/§Consequences body lands at this IMPL per ADR-0044) + ADR-0230 (the REUSED seam) + ADR-0229 (the redis precedent).

---

## ADR-0045 split-gate — FINAL re-check at PLAN: NO SPLIT

Re-estimated against the §11 findings + this PLAN's task decomposition (the seam is REUSED → zero new seam LoC):

| Leg | Estimate | Gate | Verdict |
|---|---|---|---|
| Production LoC | ~790–1275 (SPEC §3.0 table) | ≤ ~1500 | UNDER |
| Task count | **16** (this PLAN) | ≤ ~25 | UNDER |

**Both legs comfortably under the gate → SINGLE FLAT ROW 33, no pre-split** (the kafka-31 precedent — a §9 protocol filter with NO new framework seam). The pre-authorized **33.1-codec / 33.2-stats** escape-valve (SPEC §3.0) STAYS UNCONSUMED (the kafka-31 / mongo-29.1 "pre-authorized split stands unconsumed" precedent). At the PLAN-DONE commit ROADMAP row 33 STAYS `in-progress`; it flips `→ done` at the IMPL six-gate (a flat §9 row, NO parent rollup). The phase-33 IMPL six-gate CLOSES the §9 Network-filters family.

---

## SPEC §12 D-question resolutions (settled at this PLAN)

- **D-S33-1 — the `thriftproxy/` file split.** RESOLVED → mirror the redisproxy layout exactly, NO separate `exception.go`:
  | File | Responsibility | redisproxy analogue |
  |---|---|---|
  | `thriftproxy.go` | `TypeURL` (via `proto.MessageName`) + `NewFactory` | `redisproxy.go` + `NewFactory` in `filter.go` |
  | `config.go` | `parseConfig` + the byte-stable reject constants + `IsValidName` guard | `config.go` |
  | `route.go` | the method-routing table + `match()` | `commands.go` |
  | `thrift.go` | the codec: framed frame decode + binary message-begin decode + opaque-body passthrough + the reply classifier + the `AppException` (`UnknownMethod`) encoder | `resp.go` |
  | `stats.go` | the EAGER 25-name roster + inc accessors | `stats.go` |
  | `filter.go` | the `filter` struct + `TerminalFilter.Handle` pump | `filter.go` |
  | `doc.go` | package doc | `doc.go` |
  | `fuzz_test.go` | `FuzzThriftDecode` (the 42nd fuzzer) | `fuzz_test.go` |
  The `AppException` encoder lives in `thrift.go` (SPEC §3.2 lists it there — the `resp.go` decode+encode-in-one-file precedent).
- **D-S33-2 — the `0057` reply-EXCEPTION / reply-ERROR arms.** RESOLVED → the **reply-EXCEPTION arm LANDS as a `0057` fixture arm** (Task 12; exercises `response_exception` from a BACKEND reply, distinct from the local-miss exception — this requires a `TCPThriftResponder` exception-reply mode, Task 11). The **reply-ERROR arm stays UNIT-TESTED** (Task 6 classifier test — a REPLY with field-id ≥ 1 → `response_error`; not worth a third round-trip fixture arm).
- **D-S33-3 — the `request_active` gauge differential treatment.** RESOLVED → **quiesced-to-0 asserted post-workload** cross-side (the redis `downstream_rq_active` precedent). The MVP's synchronous single-flight pump completes each request before reading the next, so `request_active` returns to 0 between calls; NO held-open arm (a held-open thrift arm would require pausing mid-RPC, awkward for the single-flight contract). Unit test (Task 7/8) proves the inc/dec balance; the `0057` `StatsAsserter` (Task 12) asserts `request_active == 0` post-workload on both sides.
- **D-S33-4 — reply-frame success/error classification under `payload_passthrough`.** RESOLVED → for a REPLY msgtype, peek the FIRST result-struct field header: **field-type `STOP`(0x00) OR field-id 0 → `response_success`; field-id ≥ 1 → `response_error`** (SPEC §3.2 / Appendix A). The `TCPThriftResponder` void-success reply (a single `0x00` STOP byte body) pins `response_success`; the reply-ERROR unit test (Task 6) pins `response_error` via a field-id-1 body.
- **D-S33-5 — the `FuzzThriftDecode` corpus seeds.** RESOLVED → six seeds (Task 14): (1) a valid framed-binary CALL (`ping`, seq 1; Appendix A); (2) a valid framed-binary REPLY (void success); (3) a truncated frame (length prefix says 17, only 5 bytes follow); (4) a bad-magic frame (`0x0000` version); (5) an oversized length prefix (`0x7FFFFFFF`); (6) an invalid-msgtype frame (msgtype byte `0x09`).
- **D-S33-6 — the malformed-frame path.** RESOLVED → the MVP counts **`request_decoding_error` and SILENTLY CLOSES** (no local `ProtocolError` exception emission). Rationale: under `payload_passthrough` the only decodable surface is the message-begin; a malformed frame (bad magic / truncated / oversized length / bad msgtype) fails BEFORE a usable `seq_id`/method is recovered, so there is no echo material for a `ProtocolError` reply, and the reference's `ProtocolError`+`FlushWrite`-close is a close-direction framework-surgery boundary (AMEND-T6, deferred project-wide). **NO malformed-frame byte-equivalence fixture arm** (the local `ProtocolError` exception is an explicit coverage-boundary departure, recorded in BEHAVIOR_CONTRACT at Task 16). Unit-tested only (Task 8: a malformed request frame → `request_decoding_error` +1, pump returns, conn closes).

---

## File Structure

**New package `internal/filter/network/thriftproxy/`** (single-token-joined per the `redisproxy`/`kafkabroker` precedent):

```
internal/filter/network/thriftproxy/
  doc.go            # package doc
  thriftproxy.go    # TypeURL + NewFactory
  config.go         # parseConfig + reject constants + IsValidName guard
  route.go          # routing table + match()
  thrift.go         # codec: frame decode + message-begin decode + passthrough + reply classifier + AppException encoder
  stats.go          # EAGER 25-name roster + inc accessors
  filter.go         # filter struct + TerminalFilter.Handle pump
  *_test.go         # per-task unit tests
  fuzz_test.go      # FuzzThriftDecode
```

**Modified framework/registration files (additive, framework-ZERO-touch on the seam):**
- `internal/bootstrap/bootstrap.go` — add the thrift_proxy/v3 blank-import (after the redis_proxy/v3 import at line 119).
- `internal/filter/network/builtins/builtins.go` — the 11th `RegisterBuiltins` entry.
- `internal/stats/name.go` — the `thrift.` SINGLE-label-hoist prom arm (after the `redis.` arm at line ~315).
- `test/differential/fixture/fixture.go` — the `TCPThriftResponder` BackendKind (value 33) + its responder loop.

**New differential fixtures:**
- `test/fixtures/0057-thrift-roundtrip/` (cross-side; driver + README).
- `test/fixtures/0058-thrift-boot-reject/` (boot-reject; driver + README).

**Docs (Task 16 completion bundle):**
- `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md` (ADR-0231 body), `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`.
- `docs/envoy-go/phases/33-network-filter-thrift-proxy/PROGRESS.md` (created Task 1, appended each task).

---

## Counts at master tip (advance at this IMPL)

| Metric | At master tip | At IMPL six-gate | Canonical recipe |
|---|---|---|---|
| stat surface | 1091 | **1116** (+25 = 24 counters + 1 gauge; histogram deferred) | BEHAVIOR_CONTRACT doc count |
| differential fixtures | 58 (tail `0056`) | **60** (`0057`/`0058`) | `ls -d test/fixtures/[0-9]* \| wc -l` |
| fuzzers | 41 (tail `FuzzRESPDecode`) | **42** (`FuzzThriftDecode`) | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` |
| BackendKind tail | 32 (`TCPRedisResponder`) | **33** (`TCPThriftResponder`) | `test/differential/fixture/fixture.go` enum |
| DECISIONS tail | ADR-0231 (§Context anchored at SPEC) | ADR-0231 (§Decision/§Consequences body in-place) | next-free → ADR-0232 |

---

## Task 1: First-task baselines / anchors gate (no production code)

**Files:**
- Create: `docs/envoy-go/phases/33-network-filter-thrift-proxy/PROGRESS.md`

- [ ] **Step 1: Re-confirm the master-tip counts via the canonical recipes**

Run each and record the output in PROGRESS.md:
```bash
ls -d test/fixtures/[0-9]* | wc -l                                            # expect 58
ls -d test/fixtures/[0-9]* | tail -1                                          # expect 0056-redis-boot-reject
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l           # expect 41
grep -n "BackendKind = 3" test/differential/fixture/fixture.go | tail -1      # expect TCPRedisResponder = 32
grep -c "request_time_ms" docs/envoy-go/SPEC.md 2>/dev/null || true            # (informational)
grep -n "^## ADR-023" docs/envoy-go/DECISIONS.md | tail -1                     # expect ADR-0231 (anchored at SPEC)
```
Expected: 58 / `0056-redis-boot-reject` / 41 / `TCPRedisResponder BackendKind = 32` / DECISIONS tail `ADR-0231`.

- [ ] **Step 2: Re-confirm the TypeURL + ZERO-new-dep pin (do NOT trust the SPEC string; re-derive)**

Run a throwaway `proto.MessageName` check (the SPEC §11.1 recipe):
```bash
cat > /tmp/dt1probe_test.go <<'EOF'
package main
EOF
# Quick in-repo derivation instead (the import is already resolvable via /envoy v1.32.4):
go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3 ThriftProxy 2>&1 | head -3
go list -m github.com/envoyproxy/go-control-plane/envoy 2>/dev/null    # expect v1.32.4
```
Expected: the `thrift_proxy/v3` package resolves (it is a subpackage of the ALREADY-direct `/envoy v1.32.4` module); the TypeURL pin is `type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy` (Task 2 pins it by a `proto.MessageName` test, NEVER hand-typed).

- [ ] **Step 3: Re-pin the as-built anchors against the IMPL-session tip**

Confirm these files/lines still exist (the PLAN's references; ADR-0072 makes registration order behavior-neutral):
```bash
sed -n '37,73p' internal/filter/network/upstreampool.go      # the UpstreamConn seam API (NewUpstreamConn/Send/Reader/Close)
grep -n "redisproxy.TypeURL" internal/filter/network/builtins/builtins.go   # the registration site
grep -n "redis_proxy/v3" internal/bootstrap/bootstrap.go     # the blank-import block (~line 119)
grep -n 'CutPrefix(internal, "redis.")' internal/stats/name.go   # the redis arm (~line 315)
grep -n "RegisterBuiltins" cmd/envoy-go/main.go              # confirms main.go uses builtins (no explicit per-filter list → Task 9 parity is automatic)
```

- [ ] **Step 4: Establish the clean six-gate baseline**

```bash
go build ./... && go vet ./...
golangci-lint run ./...
go test -race -short ./... 2>&1 | tail -20
```
Expected: build/vet clean; lint clean; tests PASS. Record the baseline in PROGRESS.md.

- [ ] **Step 5: Commit**

```bash
git add docs/envoy-go/phases/33-network-filter-thrift-proxy/PROGRESS.md
git commit -m "phase 33 Task 1: baselines/anchors gate — counts re-confirmed (58 fixtures / 41 fuzzers / 1091 stats / BackendKind 32 / DECISIONS ADR-0231), TypeURL + zero-new-dep re-pinned, six-gate baseline green"
```

---

## Task 2: Package skeleton + `TypeURL` + the `/envoy` blank-import

**Files:**
- Create: `internal/filter/network/thriftproxy/doc.go`
- Create: `internal/filter/network/thriftproxy/thriftproxy.go`
- Create: `internal/filter/network/thriftproxy/thriftproxy_test.go`
- Modify: `internal/bootstrap/bootstrap.go` (add the blank-import after line 119)

- [ ] **Step 1: Write the failing test** (`thriftproxy_test.go`)

```go
package thriftproxy

import "testing"

func TestTypeURL(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestTypeURL`
Expected: FAIL (build error — `TypeURL` undefined / package empty).

- [ ] **Step 3: Write minimal implementation**

`thriftproxy.go`:
```go
package thriftproxy

import (
	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
	"google.golang.org/protobuf/proto"
)

// TypeURL is the canonical Any type URL for thrift_proxy's typed_config. Derived
// via proto.MessageName (NEVER a hand-typed docs string —
// reference_network_filter_typeurl_extensions; the redisproxy.go precedent).
// thrift_proxy/v3 is CORE /envoy v1.32.4 (AMEND-T1) — ZERO new go.mod dep.
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&thrift_proxyv3.ThriftProxy{}))
```

`doc.go`:
```go
// Package thriftproxy implements the envoy.filters.network.thrift_proxy network
// filter (ADR-0231): the project's SECOND terminal routing proxy. It decodes the
// Thrift framed×binary message-begin envelope under payload_passthrough, routes by
// method name through a route_config to one upstream cluster, round-trips each
// request through the REUSED ADR-0230 upstream-pool seam (one-conn-per-downstream,
// synchronous single-flight, positional correlation), and answers a routing miss
// with a local UnknownMethod Thrift exception. The 11th network-filter built-in.
package thriftproxy
```

In `bootstrap.go`, after line 119 (the redis_proxy/v3 import):
```go
	// Phase-33 registers the thrift_proxy network-filter extension proto so
	// protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type. Registered transitively
	// by the thriftproxy filter package too; the explicit blank-import here
	// guarantees resolution in any bootstrap-parsing context (e.g. the differential
	// harness). The thrift_proxy/v3 subpackage carries both thrift_proxy.pb.go and
	// route.pb.go (one import registers the route-config descriptors too). ZERO new
	// go.mod dep (AMEND-T1 — CORE /envoy v1.32.4). Per ADR-0016 amendment policy,
	// documented in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
```

- [ ] **Step 4: Run test + confirm zero new dep**

```bash
go test ./internal/filter/network/thriftproxy/ -run TestTypeURL    # PASS
go mod tidy && git diff --exit-code go.mod go.sum                   # expect NO change (zero new dep)
```
Expected: PASS; `go mod tidy` adds nothing (exit 0 on the diff).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/thriftproxy/ internal/bootstrap/
golangci-lint run ./internal/filter/network/thriftproxy/... ./internal/bootstrap/...
git add internal/filter/network/thriftproxy/ internal/bootstrap/bootstrap.go go.mod go.sum
git commit -m "phase 33 Task 2: thriftproxy package skeleton + TypeURL (proto.MessageName pin) + thrift_proxy/v3 blank-import (zero new dep)"
```

---

## Task 3: Config parse — `stat_prefix` / transport / protocol / payload_passthrough + reject roster

**Files:**
- Create: `internal/filter/network/thriftproxy/config.go`
- Create: `internal/filter/network/thriftproxy/config_test.go`

Proto roster: SPEC §5.2. PGV: only `stat_prefix` is a hard gate (required `min_len 1`); `transport`/`protocol` are `defined_only` (the MVP accepts only `{AUTO_TRANSPORT, FRAMED}×{AUTO_PROTOCOL, BINARY}`, else an envoy-go-strict DEPARTURE reject); `route_config` is NOT required (Task 4 parses it).

- [ ] **Step 1: Write the failing test** (`config_test.go`)

Table test over `parseConfig(*thrift_proxyv3.ThriftProxy) (*compiledConfig, error)`:
```go
package thriftproxy

import (
	"testing"

	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		msg     *thrift_proxyv3.ThriftProxy
		wantErr string // "" = expect success
	}{
		{"minimal-defaults-ok", &thrift_proxyv3.ThriftProxy{StatPrefix: "t"}, ""},
		{"framed-binary-ok", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Transport: thrift_proxyv3.TransportType_FRAMED, Protocol: thrift_proxyv3.ProtocolType_BINARY}, ""},
		{"passthrough-ok", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", PayloadPassthrough: true}, ""},
		{"stat-prefix-missing", &thrift_proxyv3.ThriftProxy{}, errStatPrefixRequired},
		{"stat-prefix-invalid", &thrift_proxyv3.ThriftProxy{StatPrefix: "bad name!"}, errStatPrefixInvalid},
		{"transport-unsupported", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Transport: thrift_proxyv3.TransportType_UNFRAMED}, errUnsupportedTransport},
		{"transport-header-unsupported", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Transport: thrift_proxyv3.TransportType_HEADER}, errUnsupportedTransport},
		{"protocol-unsupported", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Protocol: thrift_proxyv3.ProtocolType_COMPACT}, errUnsupportedProtocol},
		{"protocol-twitter-unsupported", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Protocol: thrift_proxyv3.ProtocolType_TWITTER}, errUnsupportedProtocol},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig(tc.msg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.statPrefix == "" {
					t.Fatalf("statPrefix not set")
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestParseConfig`
Expected: FAIL (build error — `parseConfig`/`compiledConfig`/the error constants undefined).

- [ ] **Step 3: Write minimal implementation** (`config.go`)

Byte-stable reject constants (ADR-0080; prefix `thrift_proxy: ` mirrors `redis_proxy: `). `compiledConfig` stores `statPrefix`, the parsed routing table (a `*routeTable`, Task 4 — for now a placeholder field the route-table task fills), and `payloadPassthrough bool`.
```go
package thriftproxy

import (
	"errors"

	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"

	"github.com/esalaine/envoy-go/internal/stats"
)

// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §6). DO NOT CHANGE these strings.
// errStatPrefixRequired is the ONLY fixture-proven arm (the 0058 boot-reject,
// §6.2); the rest are unit-test-only. errUnsupportedTransport/Protocol are
// envoy-go-strict DEPARTURE rejects (the reference parse-accepts those enum
// values — AMEND-T7), NOT cross-side boot-rejects.
const (
	errStatPrefixRequired   = "thrift_proxy: stat_prefix is required"
	errStatPrefixInvalid    = "thrift_proxy: stat_prefix is not a valid metric name"
	errUnsupportedTransport = "thrift_proxy: unsupported transport (only AUTO_TRANSPORT and FRAMED are supported)"
	errUnsupportedProtocol  = "thrift_proxy: unsupported protocol (only AUTO_PROTOCOL and BINARY are supported)"
)

// compiledConfig is the boot-parsed, per-listener-shared thrift_proxy config
// (ADR-0079 two-step factory). The roster (stats.go) is NOT here — it attaches to
// the filter struct at NewFactory (filter.go).
type compiledConfig struct {
	statPrefix         string
	payloadPassthrough bool
	routes             *routeTable // Task 4; nil-tolerant → all-miss
}

// parseConfig validates + compiles the proto (SPEC §5/§6). Only stat_prefix is a
// hard PGV gate (+ IsValidName-guarded at the config boundary). transport/protocol
// accept {AUTO,FRAMED}×{AUTO,BINARY}; any other DEFINED enum value is an
// envoy-go-strict departure reject. route_config is NOT required (absent → all
// requests routing-miss → local exception). Deferred fields (thrift_filters,
// max_requests_per_connection, trds, access_log, header_keys_preserve_case)
// parse-accept standalone. Pure — no registry.
func parseConfig(msg *thrift_proxyv3.ThriftProxy) (*compiledConfig, error) {
	sp := msg.GetStatPrefix()
	if sp == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	if !stats.IsValidName(sp) {
		return nil, errors.New(errStatPrefixInvalid)
	}
	switch msg.GetTransport() {
	case thrift_proxyv3.TransportType_AUTO_TRANSPORT, thrift_proxyv3.TransportType_FRAMED:
	default:
		return nil, errors.New(errUnsupportedTransport)
	}
	switch msg.GetProtocol() {
	case thrift_proxyv3.ProtocolType_AUTO_PROTOCOL, thrift_proxyv3.ProtocolType_BINARY:
	default:
		return nil, errors.New(errUnsupportedProtocol)
	}
	rt, err := parseRouteConfig(msg.GetRouteConfig()) // Task 4
	if err != nil {
		return nil, err
	}
	return &compiledConfig{statPrefix: sp, payloadPassthrough: msg.GetPayloadPassthrough(), routes: rt}, nil
}
```
**NOTE:** Task 3 introduces the `routes *routeTable` field + the `parseRouteConfig` call, but Task 4 implements `routeTable`/`parseRouteConfig`. To keep Task 3 self-contained + green, add a TEMPORARY stub in `route.go` (`type routeTable struct{}` + `func parseRouteConfig(*routev3...) (*routeTable, error) { return &routeTable{}, nil }`) that Task 4 REPLACES. (Alternatively, fold the route-table parse stub into Task 3's commit and complete it in Task 4 — the executor's choice; the cleanest is the stub.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestParseConfig -v`
Expected: PASS (all 9 sub-tests).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/thriftproxy/
golangci-lint run ./internal/filter/network/thriftproxy/...
git add internal/filter/network/thriftproxy/config.go internal/filter/network/thriftproxy/config_test.go internal/filter/network/thriftproxy/route.go
git commit -m "phase 33 Task 3: thrift_proxy config parse — stat_prefix required + IsValidName guard + transport/protocol departure rejects + payload_passthrough (byte-stable reject arms)"
```

---

## Task 4: The `route_config` method-routing table + match

**Files:**
- Create/replace: `internal/filter/network/thriftproxy/route.go` (replaces the Task 3 stub)
- Create: `internal/filter/network/thriftproxy/route_test.go`

Proto roster: SPEC §5.4. `RouteConfiguration{Routes []*Route}`; `Route{Match *RouteMatch (required), Route *RouteAction (required)}`; `RouteMatch.match_specifier` oneof required (`MethodName`/`ServiceName`); `RouteAction.cluster_specifier` oneof required (`Cluster` min_len 1). The MVP consumes `method_name` exact-match (empty `""` = match-all) → `cluster`. Other specifiers (`service_name`, `weighted_clusters`, `cluster_header`) are parse-accepted-deferred (the route is RECORDED but its non-`method_name`/non-`cluster` fields are ignored — or, simplest, skip routes whose specifiers the MVP doesn't consume). PLAN call: the MVP records ONLY `method_name`→`cluster` routes; a route with a `service_name` match or a non-`cluster` action is a **boot-reject** PARITY arm (the route-table parse rejects what it cannot honor — fail-fast; SPEC §6.2 lists `thrift-proxy-route-*` unit-tested arms).

- [ ] **Step 1: Write the failing test** (`route_test.go`)

```go
package thriftproxy

import (
	"testing"

	routev3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
)

func mkRouteConfig(rs ...*routev3.Route) *routev3.RouteConfiguration {
	return &routev3.RouteConfiguration{Routes: rs}
}
func mkRoute(method, cluster string) *routev3.Route {
	return &routev3.Route{
		Match: &routev3.RouteMatch{MatchSpecifier: &routev3.RouteMatch_MethodName{MethodName: method}},
		Route: &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: cluster}},
	}
}

func TestRouteMatch(t *testing.T) {
	rt, err := parseRouteConfig(mkRouteConfig(mkRoute("Ping", "c1"), mkRoute("", "fallback")))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// exact match wins for "Ping"
	if c, ok := rt.match("Ping"); !ok || c != "c1" {
		t.Fatalf("match(Ping) = %q,%v want c1,true", c, ok)
	}
	// match-all ("") catches everything else
	if c, ok := rt.match("Other"); !ok || c != "fallback" {
		t.Fatalf("match(Other) = %q,%v want fallback,true", c, ok)
	}
}

func TestRouteMiss(t *testing.T) {
	rt, _ := parseRouteConfig(mkRouteConfig(mkRoute("Ping", "c1")))
	if c, ok := rt.match("Other"); ok {
		t.Fatalf("match(Other) = %q,%v want \"\",false", c, ok)
	}
	// nil route config → all-miss (AMEND-T7)
	rt2, err := parseRouteConfig(nil)
	if err != nil {
		t.Fatalf("nil route config should parse OK: %v", err)
	}
	if _, ok := rt2.match("anything"); ok {
		t.Fatalf("nil route config should all-miss")
	}
}

func TestRouteParseRejects(t *testing.T) {
	tests := []struct {
		name    string
		rc      *routev3.RouteConfiguration
		wantErr string
	}{
		{"no-match", mkRouteConfig(&routev3.Route{Route: &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c"}}}), errRouteMatchRequired},
		{"no-action", mkRouteConfig(&routev3.Route{Match: &routev3.RouteMatch{MatchSpecifier: &routev3.RouteMatch_MethodName{MethodName: "m"}}}), errRouteActionRequired},
		{"empty-cluster", mkRouteConfig(&routev3.Route{Match: &routev3.RouteMatch{MatchSpecifier: &routev3.RouteMatch_MethodName{MethodName: "m"}}, Route: &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: ""}}}), errRouteClusterRequired},
		{"service-name-unsupported", mkRouteConfig(&routev3.Route{Match: &routev3.RouteMatch{MatchSpecifier: &routev3.RouteMatch_ServiceName{ServiceName: "s"}}, Route: &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c"}}}), errRouteMatchUnsupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseRouteConfig(tc.rc); err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
```
(NOTE: confirm the generated Go type names at IMPL — `RouteMatch_MethodName`, `RouteAction_Cluster`, etc. — via `go doc .../thrift_proxy/v3 RouteMatch`. The thrift_proxy/v3 package holds `route.pb.go` in the SAME package as `ThriftProxy`, so the import alias is the same `thrift_proxyv3`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestRoute`
Expected: FAIL (`routeTable`/`parseRouteConfig`/the error constants undefined or the stub returns empty).

- [ ] **Step 3: Write minimal implementation** (`route.go`)

```go
package thriftproxy

import (
	"errors"

	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
)

const (
	errRouteMatchRequired   = "thrift_proxy: route_config.routes[].match is required"
	errRouteActionRequired  = "thrift_proxy: route_config.routes[].route is required"
	errRouteClusterRequired = "thrift_proxy: route_config.routes[].route.cluster is required"
	errRouteMatchUnsupported = "thrift_proxy: only method_name route matches are supported"
)

// routeEntry is one method_name→cluster route. An empty methodName is match-all.
type routeEntry struct {
	methodName string
	cluster    string
}

// routeTable is the ordered method-routing table (SPEC §3.2). match() returns the
// FIRST entry whose methodName equals the request method, else the FIRST match-all
// (empty methodName) entry, else ("",false). First-match ordering mirrors the
// reference's sequential route scan.
type routeTable struct {
	entries []routeEntry
}

func parseRouteConfig(rc *thrift_proxyv3.RouteConfiguration) (*routeTable, error) {
	rt := &routeTable{}
	if rc == nil {
		return rt, nil // absent route_config → all-miss (AMEND-T7)
	}
	for _, r := range rc.GetRoutes() {
		m := r.GetMatch()
		if m == nil {
			return nil, errors.New(errRouteMatchRequired)
		}
		mn, ok := m.GetMatchSpecifier().(*thrift_proxyv3.RouteMatch_MethodName)
		if !ok {
			return nil, errors.New(errRouteMatchUnsupported) // service_name/headers deferred
		}
		a := r.GetRoute()
		if a == nil {
			return nil, errors.New(errRouteActionRequired)
		}
		cl, ok := a.GetClusterSpecifier().(*thrift_proxyv3.RouteAction_Cluster)
		if !ok || cl.Cluster == "" {
			return nil, errors.New(errRouteClusterRequired)
		}
		rt.entries = append(rt.entries, routeEntry{methodName: mn.MethodName, cluster: cl.Cluster})
	}
	return rt, nil
}

// match returns the cluster for method: an exact methodName match first, then a
// match-all (empty methodName) entry. First-match wins within each tier.
func (rt *routeTable) match(method string) (string, bool) {
	var fallback string
	haveFallback := false
	for _, e := range rt.entries {
		if e.methodName == method {
			return e.cluster, true
		}
		if e.methodName == "" && !haveFallback {
			fallback, haveFallback = e.cluster, true
		}
	}
	if haveFallback {
		return fallback, true
	}
	return "", false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestRoute -v`
Expected: PASS (all route sub-tests).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/thriftproxy/
golangci-lint run ./internal/filter/network/thriftproxy/...
git add internal/filter/network/thriftproxy/route.go internal/filter/network/thriftproxy/route_test.go
git commit -m "phase 33 Task 4: thrift_proxy route_config method-routing table (exact-match + match-all → cluster; first-match; route PGV reject arms)"
```

---

## Task 5: Framed-transport frame decode + binary-protocol message-begin decode

**Files:**
- Create: `internal/filter/network/thriftproxy/thrift.go`
- Create: `internal/filter/network/thriftproxy/thrift_test.go`

Wire format: SPEC Appendix A. FRAMED = 4-byte BE frame-length prefix (signed int32, `> 0` and `<= maxFrameSize`) + that many payload bytes. BINARY strict message-begin (`MinMessageBeginLength = 12`): magic `0x8001` + zero byte + msgtype byte (`Call=1`/`Reply=2`/`Exception=3`/`Oneway=4`) + i32 name-len + name + i32 `seq_id`. Under `payload_passthrough` the decoder reads ONLY the message-begin then keeps the rest of the frame as opaque body bytes.

`decodeFrame(r *bufio.Reader) (*thriftMessage, error)` returns the decoded message-begin + the RAW full frame bytes (length prefix + payload — forwarded VERBATIM upstream/downstream). A `thriftMessage` holds `{msgType uint8, method string, seqID int32, raw []byte, body []byte}`.

- [ ] **Step 1: Write the failing test** (`thrift_test.go`)

```go
package thriftproxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"testing"
)

// framedBinaryCall builds a framed-binary CALL frame (Appendix A).
func framedBinaryCall(msgType uint8, method string, seqID int32) []byte {
	var p bytes.Buffer
	p.Write([]byte{0x80, 0x01, 0x00, msgType})
	_ = binary.Write(&p, binary.BigEndian, int32(len(method)))
	p.WriteString(method)
	_ = binary.Write(&p, binary.BigEndian, seqID)
	p.WriteByte(0x00) // empty struct STOP
	var f bytes.Buffer
	_ = binary.Write(&f, binary.BigEndian, int32(p.Len()))
	f.Write(p.Bytes())
	return f.Bytes()
}

func TestDecodeFrame_Call(t *testing.T) {
	in := framedBinaryCall(msgTypeCall, "ping", 1)
	r := bufio.NewReader(bytes.NewReader(in))
	m, err := decodeFrame(r)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if m.msgType != msgTypeCall || m.method != "ping" || m.seqID != 1 {
		t.Fatalf("got %+v", m)
	}
	if !bytes.Equal(m.raw, in) {
		t.Fatalf("raw frame not preserved verbatim")
	}
}

func TestDecodeFrame_Errors(t *testing.T) {
	tests := []struct{ name string; in []byte }{
		{"empty", nil},
		{"truncated-length", []byte{0x00, 0x00}},
		{"truncated-payload", append([]byte{0x00, 0x00, 0x00, 0x11}, 0x80, 0x01)},
		{"bad-magic", func() []byte { b := framedBinaryCall(msgTypeCall, "x", 1); b[4] = 0x00; b[5] = 0x00; return b }()},
		{"bad-msgtype", func() []byte { b := framedBinaryCall(0x09, "x", 1); return b }()},
		{"zero-length", []byte{0x00, 0x00, 0x00, 0x00}},
		{"oversized-length", []byte{0x7f, 0xff, 0xff, 0xff}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(tc.in))
			if _, err := decodeFrame(r); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestDecodeFrame`
Expected: FAIL (`decodeFrame`/`thriftMessage`/`msgTypeCall` undefined).

- [ ] **Step 3: Write minimal implementation** (`thrift.go`)

```go
package thriftproxy

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

const (
	maxFrameSize  = 100 * 1024 * 1024 // 100 MiB frame guard (the reference default order)
	binaryVersion = 0x8001            // strict binary protocol version magic

	msgTypeCall      uint8 = 1
	msgTypeReply     uint8 = 2
	msgTypeException uint8 = 3
	msgTypeOneway    uint8 = 4

	minMessageBeginLength = 12 // magic(2)+zero(1)+msgtype(1)+namelen(4)+seqid(4) with empty name
)

// errDecode is the internal framing error (never byte-compared — the differential
// proof is the request_decoding_error COUNT, not the message; the redisproxy
// errProtocol precedent). errInvalidType is a DISTINCT sentinel for an
// out-of-range message type (→ request_invalid_type, NOT request_decoding_error)
// so Task 8 can switch on errors.Is(err, errInvalidType).
var (
	errDecode      = errors.New("thrift_proxy: frame decode error")
	errInvalidType = errors.New("thrift_proxy: invalid message type")
)

// thriftMessage is one decoded framed-binary message-begin plus the RAW frame
// (forwarded VERBATIM up/downstream — §8) and the opaque passthrough body.
type thriftMessage struct {
	msgType uint8
	method  string
	seqID   int32
	raw     []byte // the full frame: 4-byte length prefix + payload
	body    []byte // the opaque struct bytes after the message-begin (passthrough)
}

// decodeFrame reads ONE framed-binary frame (Appendix A) from r and decodes the
// message-begin, keeping the raw frame + the opaque body. Blocks on partial
// frames; never panics on arbitrary bytes; bounds-checks the length prefix BEFORE
// allocating. A frame-boundary io.EOF (no bytes) is returned verbatim (clean end).
func decodeFrame(r *bufio.Reader) (*thriftMessage, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err // io.EOF here = clean end between frames
	}
	frameLen := int32(binary.BigEndian.Uint32(lenBuf[:]))
	if frameLen <= 0 || int64(frameLen) > maxFrameSize {
		return nil, errDecode
	}
	payload := make([]byte, frameLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, errDecode // a mid-frame short read is a framing error
	}
	if len(payload) < minMessageBeginLength {
		return nil, errDecode
	}
	if binary.BigEndian.Uint16(payload[0:2]) != binaryVersion {
		return nil, errDecode // bad magic / non-strict / wrong version
	}
	mt := payload[3]
	if mt < msgTypeCall || mt > msgTypeOneway {
		return nil, errInvalidType // distinct sentinel → request_invalid_type (Task 8 errors.Is)
	}
	nameLen := int32(binary.BigEndian.Uint32(payload[4:8]))
	if nameLen < 0 || 8+int64(nameLen)+4 > int64(len(payload)) {
		return nil, errDecode
	}
	method := string(payload[8 : 8+nameLen])
	off := 8 + nameLen
	seqID := int32(binary.BigEndian.Uint32(payload[off : off+4]))
	off += 4
	raw := make([]byte, 0, 4+len(payload))
	raw = append(raw, lenBuf[:]...)
	raw = append(raw, payload...)
	return &thriftMessage{msgType: mt, method: method, seqID: seqID, raw: raw, body: payload[off:]}, nil
}
```
**Design note (D-S33-6):** `decodeFrame` returns `errDecode` for ALL malformed frames EXCEPT an out-of-range message type, for which it returns the DISTINCT `errInvalidType` sentinel (declared in the var block above, returned at the `mt` range check after the magic/length checks pass). This lets Task 8 map the two failure classes to different counters: `errDecode` → `request_decoding_error` (silent close, D-S33-6); `errInvalidType` → `request_invalid_type`. The `TestDecodeFrame_Errors` table above still passes (both sentinels are non-nil errors); Task 8 switches on `errors.Is(err, errInvalidType)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestDecodeFrame -v`
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/thriftproxy/
golangci-lint run ./internal/filter/network/thriftproxy/...
git add internal/filter/network/thriftproxy/thrift.go internal/filter/network/thriftproxy/thrift_test.go
git commit -m "phase 33 Task 5: framed-transport frame decode + binary-protocol message-begin decode (bounds-checked length prefix; bad-magic/bad-msgtype/truncated/oversized errors)"
```

---

## Task 6: Reply classifier + opaque passthrough + the `AppException` (`UnknownMethod`) encoder

**Files:**
- Modify: `internal/filter/network/thriftproxy/thrift.go`
- Modify: `internal/filter/network/thriftproxy/thrift_test.go`

The reply classifier (D-S33-4): for a REPLY msgtype, peek the FIRST result-struct field header in `body` — field-type `STOP`(0x00) OR field-id 0 → success; field-id ≥ 1 → error. The `AppException` encoder (Appendix A): an `AppException` TStruct `{1: string message, 2: i32 type}`, framed-binary EXCEPTION (msgtype 3), echoing the request's method name + `seq_id`. The exact byte layout is in SPEC Appendix A (live-captured) — pin it byte-for-byte.

- [ ] **Step 1: Write the failing test** (append to `thrift_test.go`)

```go
func framedBinaryReply(method string, seqID int32, body []byte) []byte {
	var p bytes.Buffer
	p.Write([]byte{0x80, 0x01, 0x00, msgTypeReply})
	_ = binary.Write(&p, binary.BigEndian, int32(len(method)))
	p.WriteString(method)
	_ = binary.Write(&p, binary.BigEndian, seqID)
	p.Write(body)
	var f bytes.Buffer
	_ = binary.Write(&f, binary.BigEndian, int32(p.Len()))
	f.Write(p.Bytes())
	return f.Bytes()
}

func TestClassifyReply(t *testing.T) {
	// void success: body is a single STOP byte
	m := mustDecode(t, framedBinaryReply("ping", 1, []byte{0x00}))
	if got := classifyReply(m); got != replySuccess {
		t.Fatalf("void reply class = %v want success", got)
	}
	// error: first field id 1 (type STRING 0x0b, id 0x0001), then value+STOP
	errBody := []byte{0x0b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 'x', 0x00}
	m2 := mustDecode(t, framedBinaryReply("ping", 1, errBody))
	if got := classifyReply(m2); got != replyError {
		t.Fatalf("field-id-1 reply class = %v want error", got)
	}
}

func TestEncodeUnknownMethodException(t *testing.T) {
	// SPEC Appendix A live-captured layout for method "somethingelse", seq 1.
	got := encodeUnknownMethod("somethingelse", 1)
	want := []byte{
		0x00, 0x00, 0x00, 0x4b, // frame len (75 = payload bytes; verify by recompute at IMPL)
		0x80, 0x01, 0x00, 0x03, // version + EXCEPTION(3)
		0x00, 0x00, 0x00, 0x0d, // name-len 13
		's', 'o', 'm', 'e', 't', 'h', 'i', 'n', 'g', 'e', 'l', 's', 'e',
		0x00, 0x00, 0x00, 0x01, // seq_id 1
		0x0b, 0x00, 0x01, 0x00, 0x00, 0x00, 0x23, // STRING id 1, len 0x23=35
		'n', 'o', ' ', 'r', 'o', 'u', 't', 'e', ' ', 'f', 'o', 'r', ' ', 'm', 'e', 't', 'h', 'o', 'd', ' ',
		'\'', 's', 'o', 'm', 'e', 't', 'h', 'i', 'n', 'g', 'e', 'l', 's', 'e', '\'',
		0x08, 0x00, 0x02, 0x00, 0x00, 0x00, 0x01, // I32 id 2, value 1 (UnknownMethod)
		0x00, // STOP
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("exception bytes mismatch:\n got %x\nwant %x", got, want)
	}
}
```
(NOTE: `mustDecode` is a test helper that wraps `decodeFrame` over a `bufio.Reader`. The exact frame-length / field-lengths in the `want` literal MUST be verified at IMPL by recomputing from the message string `no route for method 'somethingelse'` (35 bytes = `0x23`) — the payload assembles to 75 bytes (`0x4b`: 4 magic/msgtype + 4 name-len + 13 name + 4 seq_id + 3 field-1 header + 4 msg-len + 35 msg + 3 field-2 header + 4 i32 value + 1 STOP); the SPEC Appendix A captured the payload bytes live for this exact string.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/thriftproxy/ -run 'TestClassifyReply|TestEncodeUnknownMethod'`
Expected: FAIL (`classifyReply`/`replySuccess`/`encodeUnknownMethod` undefined).

- [ ] **Step 3: Write minimal implementation** (append to `thrift.go`)

```go
type replyClass int

const (
	replySuccess replyClass = iota
	replyError
)

// classifyReply peeks the first result-struct field header in a REPLY body
// (D-S33-4): field-type STOP (empty struct = void) OR field-id 0 → success;
// field-id >= 1 → error. Under payload_passthrough the body is opaque bytes; this
// reads only the leading field header (1 type byte + 2 id bytes).
func classifyReply(m *thriftMessage) replyClass {
	if len(m.body) == 0 || m.body[0] == 0x00 { // STOP → void success
		return replySuccess
	}
	if len(m.body) < 3 {
		return replySuccess // malformed-short → treat as success (no field id readable)
	}
	fieldID := int16(binary.BigEndian.Uint16(m.body[1:3]))
	if fieldID == 0 {
		return replySuccess
	}
	return replyError
}

// encodeUnknownMethod builds the local UnknownMethod AppException EXCEPTION frame
// (Appendix A): framed-binary EXCEPTION(3) echoing method + seqID, carrying the
// TStruct {1: string "no route for method '<method>'", 2: i32 1}. Byte-identical
// to the reference's miss-path reply (the 0057 miss-arm byte-equivalence proof).
func encodeUnknownMethod(method string, seqID int32) []byte {
	msg := "no route for method '" + method + "'"
	var p []byte
	p = append(p, 0x80, 0x01, 0x00, msgTypeException)
	p = appendI32(p, int32(len(method)))
	p = append(p, method...)
	p = appendI32(p, seqID)
	// field 1: STRING (0x0b) id 1
	p = append(p, 0x0b, 0x00, 0x01)
	p = appendI32(p, int32(len(msg)))
	p = append(p, msg...)
	// field 2: I32 (0x08) id 2, value 1 (AppExceptionType::UnknownMethod)
	p = append(p, 0x08, 0x00, 0x02)
	p = appendI32(p, 1)
	// STOP
	p = append(p, 0x00)
	frame := appendI32(nil, int32(len(p)))
	return append(frame, p...)
}

func appendI32(b []byte, v int32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/filter/network/thriftproxy/ -run 'TestClassifyReply|TestEncodeUnknownMethod' -v`
Expected: PASS. If `TestEncodeUnknownMethodException` fails on the frame-len byte, recompute `want` from the assembled bytes (the test is the source of truth for the layout; the SPEC Appendix A hex is the live capture).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/thriftproxy/
golangci-lint run ./internal/filter/network/thriftproxy/...
git add internal/filter/network/thriftproxy/thrift.go internal/filter/network/thriftproxy/thrift_test.go
git commit -m "phase 33 Task 6: reply classifier (success/error first-field peek) + UnknownMethod AppException encoder (byte-identical to the reference miss-path reply)"
```

---

## Task 7: The EAGER 25-name stat roster

**Files:**
- Create: `internal/filter/network/thriftproxy/stats.go`
- Create: `internal/filter/network/thriftproxy/stats_test.go`

Roster: SPEC §7.2 — 24 counters + 1 gauge (`request_active`), all under `thrift.<stat_prefix>.`, created EAGER at `NewFactory` via `NewCounterIfAbsent`/`NewGaugeIfAbsent` (idempotent across listeners sharing a `stat_prefix`). The `request_time_ms` histogram is NOT created (ADR-0060). The 2 close-direction counters + `downstream_response_drain_close` are created but NEVER incremented (AMEND-T6).

- [ ] **Step 1: Write the failing test** (`stats_test.go`)

```go
package thriftproxy

import (
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestStatRoster(t *testing.T) {
	reg := stats.NewRegistry()
	st := newThriftStats(reg, "tp")
	_ = st
	// all 24 counters + 1 gauge present-at-0 under thrift.tp.
	for _, suf := range counterSuffixes {
		if reg.LookupCounter("thrift.tp."+suf) == nil {
			t.Errorf("counter thrift.tp.%s not created", suf)
		}
	}
	if reg.LookupGauge("thrift.tp.request_active") == nil {
		t.Errorf("gauge thrift.tp.request_active not created")
	}
	if len(counterSuffixes) != 24 {
		t.Fatalf("counter roster = %d, want 24", len(counterSuffixes))
	}
	// the histogram is NOT created (ADR-0060)
	if reg.LookupHistogram("thrift.tp.request_time_ms") != nil {
		t.Errorf("request_time_ms histogram must NOT be created (ADR-0060)")
	}
	// idempotent across a second prefix-sharing instance
	_ = newThriftStats(reg, "tp")
}
```
(NOTE: confirm the exact `stats.Registry` lookup API names at IMPL — `LookupCounter`/`LookupGauge`/`LookupHistogram` or the project's actual accessor; grep `internal/stats/registry.go`. If no Lookup exists, assert via `reg.NewCounterIfAbsent` returning the SAME pointer twice, or via the admin `/stats` snapshot.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestStatRoster`
Expected: FAIL (`newThriftStats`/`counterSuffixes` undefined).

- [ ] **Step 3: Write minimal implementation** (`stats.go`)

```go
package thriftproxy

import "github.com/esalaine/envoy-go/internal/stats"

// counterSuffixes is the fixed 24-counter roster under thrift.<stat_prefix>.
// Pinned name-for-name against ALL_THRIFT_FILTER_STATS + the 5 router counters
// (SPEC §7.2 / §11.3). The 2 close-direction counters + downstream_response_drain
// _close are created but NEVER incremented (AMEND-T6); the request_internal_error/
// shadow_request_submit_failure/upstream_rq_maintenance_mode/downstream_cx_max
// _requests members exist-at-0 (deferred features).
var counterSuffixes = []string{
	"request",
	"request_call",
	"request_oneway",
	"request_passthrough",
	"request_decoding_error",
	"request_invalid_type",
	"request_internal_error",
	"response",
	"response_reply",
	"response_success",
	"response_error",
	"response_exception",
	"response_passthrough",
	"response_decoding_error",
	"response_invalid_type",
	"route_missing",
	"unknown_cluster",
	"no_healthy_upstream",
	"shadow_request_submit_failure",
	"upstream_rq_maintenance_mode",
	"cx_destroy_local_with_active_rq",  // exist-at-0, never incremented (AMEND-T6)
	"cx_destroy_remote_with_active_rq", // exist-at-0, never incremented (AMEND-T6)
	"downstream_cx_max_requests",       // exist-at-0 (max_requests_per_connection deferred)
	"downstream_response_drain_close",  // exist-at-0, never incremented (AMEND-T6)
}

// thriftStats holds the EAGER fixed roster (24 counters + 1 gauge) created under
// thrift.<prefix>. The request_time_ms histogram is DEFERRED (ADR-0060).
type thriftStats struct {
	prefix   string
	counters map[string]*stats.Counter
	active   *stats.Gauge
}

func newThriftStats(reg *stats.Registry, statPrefix string) *thriftStats {
	ts := &thriftStats{
		prefix:   "thrift." + statPrefix + ".",
		counters: make(map[string]*stats.Counter, len(counterSuffixes)),
	}
	for _, suf := range counterSuffixes {
		ts.counters[suf] = reg.NewCounterIfAbsent(ts.prefix + suf)
	}
	ts.active = reg.NewGaugeIfAbsent(ts.prefix + "request_active")
	return ts
}

func (ts *thriftStats) inc(suf string)   { ts.counters[suf].Inc() }
func (ts *thriftStats) incActive()       { ts.active.Inc() }
func (ts *thriftStats) decActive()       { ts.active.Dec() }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestStatRoster -v`
Expected: PASS (roster of 24 counters + the gauge; no histogram).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/thriftproxy/
golangci-lint run ./internal/filter/network/thriftproxy/...
git add internal/filter/network/thriftproxy/stats.go internal/filter/network/thriftproxy/stats_test.go
git commit -m "phase 33 Task 7: EAGER 25-name thrift stat roster (24 counters + request_active gauge; histogram deferred; 3 never-incremented close/drain counters)"
```

---

## Task 8: The `TerminalFilter.Handle` request→reply pump

**Files:**
- Modify: `internal/filter/network/thriftproxy/thriftproxy.go` (add `NewFactory` + the `filter` struct)
- Create: `internal/filter/network/thriftproxy/filter.go`
- Create: `internal/filter/network/thriftproxy/filter_test.go`

The pump (SPEC §3.3): per decoded message-begin — count the request (`request`/`request_call|oneway`/`request_passthrough`/`request_active` inc); match the route; on a MISS write the local `UnknownMethod` exception (`route_missing`+`response_exception`, keep conn open); on a HIT round-trip the raw frame through the REUSED `UpstreamConn` seam, classify the reply, count the response, forward the raw reply. Mirror the redisproxy `filter.go` structure (the `dialSource` closure, the `ensureUpstream` lazy-dial, the per-request `request_active` defer-balance). The cluster is resolved per the matched route's cluster name (NOT a single catch_all — the route table can name different clusters; the MVP fixture uses one).

- [ ] **Step 1: Write the failing test** (`filter_test.go`)

Use a `newTestFilter` helper that injects a fake `dialSource` (an in-memory pipe to a canned framed-binary REPLY responder), the redisproxy `filter_test.go` precedent. Test arms:
```go
package thriftproxy

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// fakeBackend serves one canned framed-binary REPLY echoing the request method+seq.
// ... (a net.Pipe-based helper; the redisproxy filter_test.go shape) ...

func TestHandle_RouteHit(t *testing.T) {
	// route "ping" → cluster; drive a framed-binary CALL("ping"); expect a REPLY
	// forwarded downstream byte-equivalent + request/request_call/request_passthrough/
	// response/response_reply/response_success/response_passthrough each +1 + request_active==0.
}

func TestHandle_RouteMiss(t *testing.T) {
	// route "ping" → cluster; drive CALL("other"); expect the local UnknownMethod
	// exception bytes downstream + route_missing/response_exception each +1 +
	// NO upstream dial + cx_destroy_*/downstream_response_drain_close stay 0.
}

func TestHandle_DecodingError(t *testing.T) {
	// drive a malformed frame; expect request_decoding_error +1, pump returns, no reply.
}

func TestHandle_InvalidType(t *testing.T) {
	// drive a frame with msgtype 0x09; expect request_invalid_type +1.
}
```
(Flesh out each arm at IMPL with the `net.Pipe`/`bufio` plumbing — model on `redisproxy/filter_test.go` `newTestFilter` + the `TCPRedisResponder`-in-test pattern. Assert counter values via the registry lookup used in Task 7.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestHandle`
Expected: FAIL (`NewFactory`/`filter`/`Handle` undefined).

- [ ] **Step 3: Write minimal implementation**

`filter.go` — the `filter` struct (mirror redisproxy: `network.Marker`, `cfg`, `st`, `cm *cluster.Manager`, `dialSource func(ctx, cluster string) (network.UpstreamDialFunc, func(), error)`) + `Handle`:
```go
func (f *filter) Handle(ctx context.Context, downstream net.Conn) {
	defer func() { _ = downstream.Close() }()
	dr := bufio.NewReader(downstream)
	var up *network.UpstreamConn
	defer func() {
		if up != nil {
			_ = up.Close()
		}
	}()
	for {
		m, err := decodeFrame(dr)
		if err != nil {
			if errors.Is(err, errInvalidType) {
				f.st.inc("request_invalid_type")
				return
			}
			if !errors.Is(err, io.EOF) {
				f.st.inc("request_decoding_error")
			}
			return
		}
		if !f.serveRequest(ctx, downstream, m, &up) {
			return
		}
	}
}
```
`serveRequest`:
- `f.st.inc("request")`; `request_call` (Call) / `request_oneway` (Oneway); `if f.cfg.payloadPassthrough { f.st.inc("request_passthrough") }`; `f.st.incActive(); defer f.st.decActive()`.
- `cluster, ok := f.cfg.routes.match(m.method)`:
  - **MISS:** `f.st.inc("route_missing"); f.st.inc("response_exception"); downstream.Write(encodeUnknownMethod(m.method, m.seqID))`; return `true` (keep conn open — AMEND-T6; do NOT FlushWrite-close).
  - **HIT:** lazily `ensureUpstream(cluster)` (resolve `f.cm.Get(cluster)` → dial closure + `IncUpstreamRqTotal`); on an unresolvable cluster `f.st.inc("unknown_cluster")`, return false; on no healthy host `f.st.inc("no_healthy_upstream")`. `up.Send(ctx, m.raw)`; `reply, err := decodeFrame(up.Reader())`; on decode error `f.st.inc("response_decoding_error")`, return false. Classify: `f.st.inc("response")`; for REPLY `response_reply` + (`response_success`|`response_error`), for EXCEPTION `response_exception`, else `response_invalid_type`; `if payloadPassthrough { response_passthrough }`. `downstream.Write(reply.raw)`; return true.

`NewFactory(cm *cluster.Manager, reg *stats.Registry)` in `thriftproxy.go` — the redisproxy two-dep shape: unmarshal → `parseConfig` → `newThriftStats` → `f.dialSource = f.resolveCluster`.

**`UpstreamConn` reuse note:** `NewUpstreamConn(dial, onRequest)` is one-conn-per-downstream. The MVP routes ALL frames on a connection to whatever cluster each matches; since the fixture uses ONE cluster, a single `UpstreamConn` per downstream conn suffices. If a connection's frames matched DIFFERENT clusters, the MVP would need one `UpstreamConn` per cluster — but the single-route MVP fixture never exercises that; key `ensureUpstream` by the resolved cluster name and reuse if unchanged (simplest correct: store `up` + its cluster; re-dial if a later frame names a different cluster — defensive, but the fixture stays single-cluster).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/filter/network/thriftproxy/ -run TestHandle -v`
Expected: PASS (hit, miss, decoding-error, invalid-type; `request_active` balanced to 0).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/thriftproxy/
golangci-lint run ./internal/filter/network/thriftproxy/...
git add internal/filter/network/thriftproxy/filter.go internal/filter/network/thriftproxy/thriftproxy.go internal/filter/network/thriftproxy/filter_test.go
git commit -m "phase 33 Task 8: TerminalFilter.Handle request→reply pump (route hit round-trip via the REUSED UpstreamConn seam; route miss local UnknownMethod exception keeping conn open; decoding-error/invalid-type accounting; request_active balanced)"
```

---

## Task 9: Register as the 11th built-in + boot smoke

**Files:**
- Modify: `internal/filter/network/builtins/builtins.go`
- Create/modify: a boot-smoke test (the kafka/redis registration-test precedent — likely `internal/filter/network/builtins/builtins_test.go` or `internal/bootstrap/*_test.go`)

- [ ] **Step 1: Write the failing test**

A registration test asserting `thriftproxy.TypeURL` resolves to a factory after `RegisterBuiltins`, mirroring the redis_proxy registration test. If a `builtins_test.go` enumerates the expected registered TypeURLs, add `thriftproxy.TypeURL` to that set; else add a focused test:
```go
func TestThriftProxyRegistered(t *testing.T) {
	reg := network.NewRegistry()
	RegisterBuiltins(reg, Deps{ClusterManager: cluster.NewManager(...), StatsRegistry: stats.NewRegistry()})
	if _, ok := reg.Lookup(thriftproxy.TypeURL); !ok {
		t.Fatalf("thrift_proxy not registered")
	}
}
```
(Confirm the `network.Registry` lookup API + the existing builtins-test shape at IMPL.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/filter/network/builtins/ -run TestThriftProxyRegistered`
Expected: FAIL (not registered).

- [ ] **Step 3: Write minimal implementation**

In `builtins.go`: add the import `"github.com/esalaine/envoy-go/internal/filter/network/thriftproxy"`, update the package doc + the `RegisterBuiltins` doc comment (eleven built-ins), and add the 11th entry after the redis_proxy line:
```go
	// thrift_proxy: the 11th built-in (33; ADR-0231). The project's SECOND
	// terminal routing proxy. Like redisproxy it needs BOTH deps.ClusterManager
	// (route cluster → Cluster.Dial via the REUSED ADR-0230 upstream-pool seam)
	// AND deps.StatsRegistry (the thrift.<sp> roster). The FIRST row to REUSE a
	// prior framework seam unchanged. The §9 family-CLOSING built-in.
	reg.Register(thriftproxy.TypeURL, thriftproxy.NewFactory(deps.ClusterManager, deps.StatsRegistry))
```
`cmd/envoy-go/main.go` needs NO change — it registers network filters solely via `builtins.RegisterBuiltins` (confirmed Task 1 Step 3; no explicit per-filter list).

- [ ] **Step 4: Run test + boot smoke**

```bash
go test ./internal/filter/network/builtins/ -run TestThriftProxyRegistered -v   # PASS
go build ./... && go test ./internal/bootstrap/... -short                        # bootstrap parses a thrift_proxy listener
```
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/network/builtins/
golangci-lint run ./internal/filter/network/builtins/...
git add internal/filter/network/builtins/
git commit -m "phase 33 Task 9: register thrift_proxy as the 11th network-filter built-in (two-dep factory: ClusterManager + StatsRegistry)"
```

---

## Task 10: The `thrift.` SINGLE-label-hoist Prometheus arm

**Files:**
- Modify: `internal/stats/name.go` (add the `thrift.` arm after the `redis.` arm at line ~315)
- Modify: `internal/stats/name_test.go`

Shape (SPEC §7.4): `thrift.<prefix>.<rest>` → metric `envoy_thrift_<rest flattened>` + label `envoy_thrift_prefix="<prefix>"`. The redis-arm shape generalized to a `thrift.` root. The roster is fixed (no dynamic command names) → shape-based detection over a dot-free `<prefix>` segment is unambiguous.

- [ ] **Step 1: Write the failing test** (`name_test.go`)

```go
func TestThriftPromArm(t *testing.T) {
	tests := []struct {
		in        string
		wantBase  string
		wantLabel string // value of envoy_thrift_prefix
	}{
		{"thrift.thriftprobe.request", "envoy_thrift_request", "thriftprobe"},
		{"thrift.thriftprobe.response_reply", "envoy_thrift_response_reply", "thriftprobe"},
		{"thrift.tp.route_missing", "envoy_thrift_route_missing", "tp"},
		{"thrift.tp.request_active", "envoy_thrift_request_active", "tp"},
	}
	for _, tc := range tests {
		base, labels, err := promName(tc.in) // confirm the exact function name at IMPL
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if base != tc.wantBase {
			t.Errorf("%s base = %q want %q", tc.in, base, tc.wantBase)
		}
		if !hasLabel(labels, "envoy_thrift_prefix", tc.wantLabel) {
			t.Errorf("%s missing envoy_thrift_prefix=%q", tc.in, tc.wantLabel)
		}
	}
}
```
(Confirm the exact `name.go` entry-point function name + the `Label` helper at IMPL — the redis-arm test in `name_test.go` is the template.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stats/ -run TestThriftPromArm`
Expected: FAIL (`thrift.` falls through to the "no recognized top-level segment" error).

- [ ] **Step 3: Write minimal implementation** (`name.go`, after the `redis.` arm block at line ~324)

```go
		// Phase-33 thrift_proxy SINGLE-label HOIST (ADR-0231; AMEND-T3; the redis.
		// ADR-0229 shape generalized to a thrift. ROOT prefix). Internal name
		// thrift.<stat_prefix>.<rest> → envoy_thrift_<rest flattened>
		// {envoy_thrift_prefix="<stat_prefix>"}. The roster is FIXED (no dynamic
		// command names — the method drives ROUTING, not a counter), so shape
		// validation (dot-free <prefix>) is unambiguous. KEEP-IN-SYNC:
		// internal/filter/network/thriftproxy/stats.go (the roster name builders).
		if rest, ok := strings.CutPrefix(internal, "thrift."); ok {
			if idx := strings.IndexByte(rest, '.'); idx > 0 {
				prefix, tail := rest[:idx], rest[idx+1:]
				if !strings.ContainsRune(prefix, '.') {
					labels = append(labels, Label{Key: "envoy_thrift_prefix", Value: prefix})
					base = "envoy_thrift_" + strings.ReplaceAll(tail, ".", "_")
					return base, labels, nil
				}
			}
		}
```
(Insert BEFORE the final `return "", nil, fmt.Errorf(...)` fall-through, alongside the `redis.` arm.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/stats/ -run TestThriftPromArm -v`
Expected: PASS (all 4 cases; the `request_active` gauge flattens identically).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/stats/
golangci-lint run ./internal/stats/...
git add internal/stats/name.go internal/stats/name_test.go
git commit -m "phase 33 Task 10: thrift. SINGLE-label-hoist Prometheus arm (envoy_thrift_<leaf>{envoy_thrift_prefix=<sp>} — the redis-32 tag-extractor shape)"
```

---

## Task 11: The `TCPThriftResponder` BackendKind (value 33)

**Files:**
- Modify: `test/differential/fixture/fixture.go` (the BackendKind enum + the responder loop)

The new backend (SPEC §8.3): per connection it loops reading a framed-binary CALL, parses method+msgtype+`seq_id`, and writes a framed-binary REPLY (msgtype 2) echoing the SAME method + RECEIVED `seq_id` with a void-success body (single STOP byte). seq_id-AGNOSTIC. A second mode (D-S33-2) replies an EXCEPTION (msgtype 3) for the `0057` reply-EXCEPTION arm.

- [ ] **Step 1: Write the failing test** (a fixture-package unit test, the `TCPRedisResponder` test precedent)

```go
func TestTCPThriftResponder_EchoesReply(t *testing.T) {
	// start the responder on a net.Pipe / loopback; send framed CALL("ping", seq 7);
	// expect a framed REPLY msgtype 2, method "ping", seq 7, void-success body.
}
```
(If `fixture.go` responders are exercised only through the runner, add the focused test in the fixture package; else the `0057` driver (Task 12) is the live proof — but a unit test here de-risks Task 12.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./test/differential/fixture/ -run TestTCPThriftResponder`
Expected: FAIL (`TCPThriftResponder` undefined).

- [ ] **Step 3: Write minimal implementation**

Add the enum constant (after `TCPRedisResponder BackendKind = 32`):
```go
	// TCPThriftResponder is a framed-binary Thrift canned-response TCP backend (33;
	// 33 SPEC §8.3). Per connection it reads a framed-binary CALL (4-byte BE
	// frame-len + binary message-begin), parses method+msgtype+seq_id, and writes a
	// framed-binary REPLY (msgtype 2) echoing the SAME method + RECEIVED seq_id with
	// a void-success body (single STOP 0x00). seq_id-AGNOSTIC (echoes whatever it
	// receives — AMEND-T5; the reference sends 0, the subject passes the original
	// through, the DOWNSTREAM reply carries the original on both sides). An optional
	// exception mode replies an EXCEPTION (msgtype 3) for the 0057 reply-EXCEPTION
	// arm (D-S33-2). NEW BackendKind per reference_differential_fixture_dispatch
	// _constraint; the TCPRedisResponder = 32 precedent.
	TCPThriftResponder BackendKind = 33
```
Wire its responder loop into the runner's backend dispatch (the switch that starts `TCPRedisResponder`/`TCPKafkaResponder` backends). The reply builder mirrors Task 5's `framedBinaryReply` (do NOT import the filter package — duplicate the tiny frame builder in the fixture package, the `TCPRedisResponder` self-contained precedent).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./test/differential/fixture/ -run TestTCPThriftResponder -v`
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l test/differential/fixture/
golangci-lint run ./test/differential/fixture/...
git add test/differential/fixture/fixture.go
git commit -m "phase 33 Task 11: TCPThriftResponder BackendKind (value 33) — framed-binary canned REPLY echoing method + received seq_id (seq_id-agnostic); optional exception mode"
```

---

## Task 12: The `0057-thrift-roundtrip` cross-side fixture

**Files:**
- Create: `test/fixtures/0057-thrift-roundtrip/driver/driver.go`
- Create: `test/fixtures/0057-thrift-roundtrip/README.md`

Cross-side (SPEC §8.1): chain `[thrift_proxy {stat_prefix, transport: FRAMED, protocol: BINARY, payload_passthrough: true, route_config: method "Ping" → cluster}]` as the TERMINAL on BOTH sides; backend = `TCPThriftResponder`. Model on `test/fixtures/0055-redis-roundtrip/driver/driver.go` (cross-side, `StatsAsserter`, single-listener bootstrap, prometheus scrape). The working YAML is SPEC §11.2 (reusable verbatim). Arms:
1. **route-HIT round-trip** — CALL `Ping` seq 1 → round-trips → REPLY void-success forwarded downstream. Cross-assert: downstream reply bytes byte-identical; `request`/`request_call`/`request_passthrough`/`response`/`response_reply`/`response_success`/`response_passthrough` +1; `cluster.<name>.upstream_cx_total`/`upstream_rq_total` +1 (cross-equal — D-T9b); `request_active`==0 post-workload (D-S33-3).
2. **route-MISS local-exception** — CALL `Pong` → local `UnknownMethod` exception, NO dial. Cross-assert: downstream EXCEPTION bytes byte-identical; `route_missing`/`response_exception` +1; cluster `upstream_cx_total`/`upstream_rq_total` stay 0. **Per-side (NOT cross-asserted):** `cx_destroy_local_with_active_rq`/`downstream_response_drain_close` (reference moves them; subject==0 — AMEND-T6).
3. **reply-EXCEPTION** (D-S33-2) — backend replies an EXCEPTION msgtype → `response_exception` +1 (distinct from the local miss). Requires the `TCPThriftResponder` exception mode (Task 11).

Per `reference_differential_asserter_dispatch`: cross-side MUST use `fixture.StatsAsserter` (NOT `SubjectAsserter`). Per `reference_differential_reference_parses_full_message`: send fully-valid frames; pin per-side `*.failure` values where the reference adds an abandon-at-close failure. Per fixture caveat (SPEC §8 (i)): verify the round-trip RAN via `cluster.<name>.upstream_cx_rx_bytes_total > 0` AND `request_call > 0` (thrift_proxy emits no listener `downstream_cx_rx_bytes_total`).

- [ ] **Step 1: Write the driver skeleton + the HIT arm (the failing differential)**

Author `driver.go`: the frame builders (shared with the unit tests — duplicate the tiny builder, the redis driver precedent), both bootstraps (thrift_proxy terminal; STRICT_DNS/STATIC cluster → `TCPThriftResponder`), `BackendKind() → fixture.TCPThriftResponder`, the HIT arm driving CALL `Ping` and returning the captured reply bytes for the runner's `CompareBytes`, and a stub `AssertStats`.

- [ ] **Step 2: Run the fixture to verify the HIT arm round-trips cross-side**

Run: `go test ./test/differential/ -run 0057 -count=1 2>&1 | tail -30`
Expected: the HIT arm PASSES byte-equivalence (or surfaces a real subject bug to fix). Confirm via the reference `/stats` that `cluster.<name>.upstream_cx_rx_bytes_total > 0`.

- [ ] **Step 3: Add the MISS arm + the reply-EXCEPTION arm + fill `AssertStats`**

Add arm 2 (CALL `Pong` → local exception bytes byte-identical; the per-side `cx_destroy`/`drain_close` assertions) and arm 3 (the exception-mode backend reply). Fill `AssertStats` with the cross-side counter assertions (HIT roster +1; MISS `route_missing`/`response_exception` +1; the per-side boundary; `request_active`==0).

- [ ] **Step 4: Run the full fixture + prove each asserted counter LIVE**

Run: `go test ./test/differential/ -run 0057 -count=1 -v 2>&1 | tail -40`
Expected: all arms PASS. Then the deliberate-break liveness proof (Task 15 consolidates, but spot-check here): temporarily break ONE asserted counter (e.g. comment out a `response_success` inc), run with `-count=1`, confirm the assertion FAILS (proving it is live, not vacuous — `reference_differential_break_protocol_count1` + the `0030` lesson), then restore.

- [ ] **Step 5: Write the README + gofmt + lint + commit**

`README.md`: the arm taxonomy, the byte-equivalence + `StatsAsserter` prongs, the per-side `cx_destroy` coverage boundary, the deliberate-break liveness record (the `0030` lesson).
```bash
gofmt -l test/fixtures/0057-thrift-roundtrip/driver/
golangci-lint run ./test/fixtures/0057-thrift-roundtrip/...
git add test/fixtures/0057-thrift-roundtrip/
git commit -m "phase 33 Task 12: 0057-thrift-roundtrip cross-side fixture (route-HIT round-trip + route-MISS local-exception + reply-EXCEPTION arms; downstream byte-equivalence + StatsAsserter; per-side cx_destroy boundary)"
```

---

## Task 13: The `0058-thrift-boot-reject` fixture + unit-tested reject arms

**Files:**
- Create: `test/fixtures/0058-thrift-boot-reject/driver/driver.go`
- Create: `test/fixtures/0058-thrift-boot-reject/README.md`

Boot-reject (SPEC §8.2): missing `stat_prefix` → both sides reject at boot (the `thrift-proxy-stat-prefix-required` arm; boot-stderr substring parity per §6.2). Per `reference_differential_fixture_dispatch_constraint`: a boot-reject fixture is a SEPARATE dir from the cross-side `0057`. Model on `test/fixtures/0056-redis-boot-reject/`. The route/route-action/thrift-filter-name PGV arms + the un-chosen transport/protocol DEPARTURE arms are UNIT-TESTED (already covered by Task 3/Task 4 `config_test.go`/`route_test.go`) — the load-bearing fixture arm is the missing-`stat_prefix` one.

- [ ] **Step 1: Write the driver** (the `0056` boot-reject precedent)

A `thrift_proxy` listener config OMITTING `stat_prefix`; assert both sides fail to boot with a `stat_prefix`-related stderr substring (per-side, NOT exact cross-impl equality — the C++ `value length must be at least 1 characters` vs the subject's `thrift_proxy: stat_prefix is required`).

- [ ] **Step 2: Run the fixture to verify it fails appropriately**

Run: `go test ./test/differential/ -run 0058 -count=1 2>&1 | tail -20`
Expected: the boot-reject arm PASSES (both sides reject at boot).

- [ ] **Step 3: Confirm the unit-tested reject arms are present**

Re-run Task 3 + Task 4 reject tables (already green); confirm `errStatPrefixRequired`/`errUnsupportedTransport`/`errUnsupportedProtocol`/`errRoute*` are all exercised:
```bash
go test ./internal/filter/network/thriftproxy/ -run 'TestParseConfig|TestRouteParseRejects' -v
```
Expected: PASS.

- [ ] **Step 4: Verify the new fixture count**

Run: `ls -d test/fixtures/[0-9]* | wc -l`
Expected: **60** (58 + `0057` + `0058`).

- [ ] **Step 5: Write the README + gofmt + lint + commit**

```bash
gofmt -l test/fixtures/0058-thrift-boot-reject/driver/
golangci-lint run ./test/fixtures/0058-thrift-boot-reject/...
git add test/fixtures/0058-thrift-boot-reject/
git commit -m "phase 33 Task 13: 0058-thrift-boot-reject fixture (missing stat_prefix; per-side stderr parity) + the route/departure reject arms unit-tested"
```

---

## Task 14: The 42nd fuzzer `FuzzThriftDecode`

**Files:**
- Create: `internal/filter/network/thriftproxy/fuzz_test.go`

SPEC §14 + D-S33-5: no-panic / no-mutation / bounded-allocation over `decodeFrame` + `classifyReply`. Mirror `redisproxy/fuzz_test.go`.

- [ ] **Step 1: Write the fuzzer**

```go
package thriftproxy

import (
	"bufio"
	"bytes"
	"testing"
)

// FuzzThriftDecode is the 42nd fuzzer (SPEC §14). It feeds arbitrary bytes through
// decodeFrame (+ classifyReply on success) via a bufio.Reader over a bytes.Reader
// and asserts: (1) no panic; (2) no mutation of the input slice; (3) bounded
// allocation — a crafted 4-byte length prefix never allocates beyond maxFrameSize
// before the bounds guard rejects it (thrift.go). The codec touches NO registry
// (reference_dynamic_stat_name_charset_guard — the roster is fixed in stats.go).
func FuzzThriftDecode(f *testing.F) {
	seeds := [][]byte{
		framedBinaryCall(msgTypeCall, "ping", 1),       // valid CALL
		framedBinaryReply("ping", 1, []byte{0x00}),     // valid void REPLY
		{0x00, 0x00, 0x00, 0x11, 0x80, 0x01, 0x00},     // truncated payload
		{0x00, 0x00, 0x00, 0x0c, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, // bad magic
		{0x7f, 0xff, 0xff, 0xff},                        // oversized length prefix
		func() []byte { b := framedBinaryCall(0x09, "x", 1); return b }(), // invalid msgtype
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		orig := append([]byte(nil), data...)
		r := bufio.NewReader(bytes.NewReader(data))
		if m, err := decodeFrame(r); err == nil && m != nil {
			_ = classifyReply(m)
		}
		if !bytes.Equal(orig, data) {
			t.Fatalf("decoder mutated its input")
		}
	})
}
```

- [ ] **Step 2: Run the fuzzer briefly**

Run: `go test ./internal/filter/network/thriftproxy/ -run FuzzThriftDecode -v` (seed corpus) then `go test ./internal/filter/network/thriftproxy/ -fuzz FuzzThriftDecode -fuzztime 20s`
Expected: no panic, no mutation, no new crashers.

- [ ] **Step 3: Verify the fuzzer count**

Run: `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`
Expected: **42**.

- [ ] **Step 4: gofmt + lint**

```bash
gofmt -l internal/filter/network/thriftproxy/
golangci-lint run ./internal/filter/network/thriftproxy/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/filter/network/thriftproxy/fuzz_test.go
git commit -m "phase 33 Task 14: the 42nd fuzzer FuzzThriftDecode (no-panic / no-mutation / bounded-allocation over decodeFrame + classifyReply; 6 corpus seeds)"
```

---

## Task 15: Full differential re-verify + deliberate-break liveness proofs

**Files:** none (verification task; record evidence in PROGRESS.md)

- [ ] **Step 1: Full differential — the 58 prior dirs byte-exact + the 2 new green**

Run: `go test ./test/differential/ -count=1 2>&1 | tail -40`
Expected: all 60 dirs PASS (the 58 prior byte-exact back-compat + `0057`/`0058`). Per `reference_differential_break_protocol_count1` ALWAYS `-count=1` (the cache serves stale PASS).

- [ ] **Step 2: Deliberate-break liveness proofs (`-count=1`)**

For EACH load-bearing `0057` assertion, prove it is LIVE (not vacuous — the `0030` lesson): temporarily break the production path, run with `-count=1`, confirm the fixture FAILS, restore, re-confirm PASS. Cover at minimum: (a) a `response_success` mis-count; (b) the MISS-arm downstream byte-equivalence (corrupt one exception byte); (c) the cross-side counter parity. Record each break→fail→restore→pass in the `0057` README + PROGRESS.md.

- [ ] **Step 3: The six gates**

```bash
go build ./... && go vet ./...
golangci-lint run ./...
go test -race -short ./... 2>&1 | tail -20
go test ./test/differential/ -count=1 2>&1 | tail -5
# h2spec 53/53 + proxy-wasm 10/10 asserted-UNAFFECTED (image-independent; phase 33 touches no HTTP/h2/proxy-wasm path)
```
Expected: all green; the conformance gates re-run asserted-unaffected per SPEC §8.4.

- [ ] **Step 4: Verify the stat surface delta**

Confirm the BEHAVIOR_CONTRACT count advances 1091 → 1116 (+25) once Task 16 lands the contract edit; here, spot-check the live roster via a thrift_proxy `/stats` snapshot shows 24 counters + 1 gauge present-at-0.

- [ ] **Step 5: Record evidence + commit**

```bash
git add docs/envoy-go/phases/33-network-filter-thrift-proxy/PROGRESS.md
git commit -m "phase 33 Task 15: full differential re-verify (60 dirs byte-exact) + deliberate-break liveness proofs (-count=1) + six gates green"
```

---

## Task 16: Completion bundle — BEHAVIOR_CONTRACT + ADR-0231 body + STATE/ROADMAP + six-gate evidence

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the 33 subsection; 1091 → 1116)
- Modify: `docs/envoy-go/DECISIONS.md` (the ADR-0231 §Decision/§Consequences body, in-place per ADR-0044)
- Modify: `docs/envoy-go/STATE.md` (counts + active-phase → `phase 33 done`)
- Modify: `docs/envoy-go/ROADMAP.md` (row 33 `in-progress → done`; the §9 family CLOSES)
- Modify: `docs/envoy-go/phases/33-network-filter-thrift-proxy/PROGRESS.md` (final record)

Per ADR-0052 the contract/ADR/STATE/ROADMAP edits land ATOMICALLY with the final task (the IMPL six-gate is the landing gate).

- [ ] **Step 1: BEHAVIOR_CONTRACT 33 subsection**

Add a NEW `### envoy.filters.network.thrift_proxy` subsection (SPEC §9): the single-pair single-route terminal envelope; the `route_config` method-routing; the REUSED ADR-0230 round-trip; the local `UnknownMethod` exception; the EAGER 25-name roster under `thrift.<stat_prefix>.`; the 11th built-in; the `thrift.` SINGLE-label-hoist prom arm. Update the stat table 1091 → **1116** (+25). Record the coverage-boundary / departure items (SPEC §7.6 / §9): the `request_time_ms` histogram unmirrored (ADR-0060); the 2 close-direction counters + `downstream_response_drain_close` created-but-never-incremented (asserted per-side on the `0057` miss arm); the un-chosen transport×protocol DEPARTURE reject; the non-router thrift_filters / full route_config richness / full struct parsing parse-accepted-deferred; the upstream `seq_id` per-side difference; the malformed-frame silent-close (no local `ProtocolError`, D-S33-6); runtime-keys-at-defaults.

- [ ] **Step 2: ADR-0231 §Decision/§Consequences body**

Complete the ADR-0231 body IN-PLACE (the §Context was anchored at the SPEC; per ADR-0044 NO new ADR number). §Decision: the as-built filter (the package layout; the REUSED seam; the local-reply semantics; the roster; the prom arm). §Consequences: the §9 family CLOSES; the ADR-0230 seam's redis-scoped YAGNI sizing VALIDATED at its first reuse (zero seam churn); the deferred multiplexing surface stays deferred. DECISIONS tail STAYS ADR-0231 (next-free → ADR-0232).

- [ ] **Step 3: STATE + ROADMAP**

STATE.md: active-phase → `phase 33 (network-filter-thrift-proxy) done`; counts 1116 / 60 / 42 / BackendKind 33 / DECISIONS tail ADR-0231; next-skill → (the §9 family is CLOSED — the next phase is a NEW family/row, TBD). ROADMAP.md: row 33 `in-progress → done` (a flat §9 row, NO parent rollup); the §9 candidate-roster paragraph records the family CLOSURE (0 candidates remain after thrift).

- [ ] **Step 4: The six-gate evidence + final counts**

```bash
go build ./... && go vet ./... && golangci-lint run ./...
go test -race -short ./... 2>&1 | tail -5
go test ./test/differential/ -count=1 2>&1 | tail -5
ls -d test/fixtures/[0-9]* | wc -l                                   # 60
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l  # 42
```
Expected: all green; counts 60 / 42; the BEHAVIOR_CONTRACT count 1116.

- [ ] **Step 5: Commit (the atomic landing)**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/STATE.md docs/envoy-go/ROADMAP.md docs/envoy-go/phases/33-network-filter-thrift-proxy/PROGRESS.md
git commit -m "phase 33 Task 16: completion bundle — BEHAVIOR_CONTRACT 33 subsection (1091→1116) + ADR-0231 §Decision/§Consequences body + STATE/ROADMAP row 33 in-progress→done (§9 Network-filters family CLOSES) + six-gate evidence"
```

---

## Post-IMPL (controller, NOT a subagent task)

Per `feedback_subagents_no_push` + `feedback_push_to_origin`: after all 16 tasks land local-only and the six gates are green, the CONTROLLER squash-merges the IMPL branch to master and pushes to origin (subagents commit local-only; the controller pushes at stage-close). Then the worktree cleanup + branch delete (`superpowers:finishing-a-development-branch`).
