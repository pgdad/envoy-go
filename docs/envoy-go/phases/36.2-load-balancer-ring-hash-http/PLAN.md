# Phase 36.2 Implementation Plan — the HTTP route `hash_policy` producer plane (`RouteAction.hash_policy`)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the HTTP router's `RouteAction.hash_policy` producer — the SECOND data plane feeding the ALREADY-LANDED ring_hash consistent-hash key (after the tcp_proxy `source_ip` plane at 36.1). Per matched route, if `RouteAction.hash_policy[]` is non-empty, the router folds each supported policy's contribution into a `uint64` and threads it via the LANDED `cluster.WithHashKey(ctx, key)` seam into `AcquireH1(ctx)`/`Dial(ctx)`/`DialH2(ctx)` → the LANDED `ringHashLB.Pick`. Two specifiers are SUPPORTED — `header` (`xxHash64` over the matched request-header value) and `connection_properties.source_ip` (byte-identical to the tcp plane — reuse `cluster.HashSourceIP`). Three DEPARTURE-reject at config-build (cookie/query_parameter/filter_state), plus a `regex_rewrite` and a `source_ip==false` connection_properties.

**Architecture:** A proto-free per-route descriptor (`router.HashPolicy`) is PARSED + VALIDATED at config-build (`internal/filter/hcm/config.go buildRouterAction`, the tcp_proxy `NewFilter` fail-fast precedent) and lowered onto `clusterRouteAction`; the existing `asRouterAction()`/`asRouterActionH2()` bridges pass it to the EXTENDED `router.H1ClusterAction(c, hps)` / `router.H2ClusterAction(c, hps)` constructors. Per request, a shared codec-agnostic helper `applyHashKey(ctx, hps, headerVal, remoteAddr) context.Context` folds the descriptor list (`hash = rotl64(prev,1) ^ new`; first contributor verbatim; nullopt-skip; `terminal` short-circuit) and returns `cluster.WithHashKey(ctx, key)` (or `ctx` untouched when nothing contributes); it is called at the FOUR per-request dial entrypoints. The header digest is the seed-chained `XXH64` fold via a new additive `cluster.HashHeaderValues`; the source_ip digest reuses `cluster.HashSourceIP` VERBATIM. NO new package, NO new go.mod dep, NO new stat name, NO new fuzzer, NO new BackendKind, NO LB-policy/seam/manager change.

**Tech Stack:** Go 1.26.x / golangci-lint 1.64.8 (ADR-0009); reference `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227, @ `sha256:7edd5b0f…`); go-control-plane `/envoy` v1.32.4 (ADR-0008 — `RouteAction.HashPolicy` field 15 already in the pinned module, ZERO new dep). Reuses `internal/cluster/` (LANDED `WithHashKey`/`hashKeyFrom` + `HashSourceIP`/`xxHash64`/`ringHashLB` + the manager gate), `internal/filter/hcm/`, `internal/filter/http/router/`, and the differential harness (`DistributionAsserter` + `StatsAsserter` + existing HTTP backends).

---

## D-question resolutions (SPEC §12 — settled at this PLAN)

The SPEC §12 anticipations are CONFIRMED, with two refinements grounded in the as-built code read this session:

- **D-S362-1 (header-hash exported surface) → RESOLVED: a new additive `cluster.HashHeaderValues(values []string) uint64`.** The seed-chained `XXH64` fold over byte-sorted values, collapsing to `xxHash64([]byte(value))` for the single-value case. Keeps `xxHash64`/`ipOnly` unexported and the digest inside `internal/cluster` (sibling to `HashSourceIP`). This is the ONE additive exported `cluster` symbol; the exported `Cluster` surface stays byte-stable. (Task 2.)
- **D-S362-2 (parse placement) → RESOLVED with refinement: the descriptor type `router.HashPolicy` lives in the `router` package (proto-free struct); the proto-read + fail-fast reject (`parseRouteHashPolicies`) lives in `hcm/config.go`.** Refinement vs the SPEC's loose "router-package parse helper": the `router` package currently imports **ZERO** go-control-plane protos (it operates on `*http.Request` / `h2.H2Request` / `cluster.Cluster`). Keeping the proto-read in `hcm/config.go` (where `routev3` is already imported) preserves that proto-free property AND keeps the fail-fast at the config-build boundary (SPEC §3.1). The descriptor stays router-owned (a plain struct in the router package). (Task 3.)
- **D-S362-3 (per-codec header accessor + remote-addr access) → RESOLVED: a `headerVal func(name string)(string,bool)` closure per codec + a UNIFORM ctx-carried downstream remote addr for BOTH codecs.** As-built finding: `*http.Request.RemoteAddr` is **NOT** populated on the HCM H1 codec path (only the chain's remote addr is set, `connection.go:384`) — so source_ip cannot rely on `req.RemoteAddr` on either codec. Introduce `router.WithDownstreamRemoteAddr(ctx, addr string)` + `downstreamRemoteAddrFrom(ctx)` (a `router` ctx key mirroring `cluster.WithHashKey`), set at the H1 dispatch (`hcm/connection.go dispatchRequest`, `downstream` in scope) and H2 dispatch (`hcm/h2dispatch.go`, `chainDispatchAction.downstreamRemoteAddr` in scope). The header accessor is per-codec: H1 `req.Header.Get`; H2 a scan of `req.Headers []hpack.HeaderField` (already lowercased). The header path is LIVE on every `0062` request; the source_ip path is UNIT-tested only (the fixture keys on header). (Task 4.)
- **D-S362-4 (multi-value header fold) → RESOLVED: implement the full seed-chained `XXH64` fold over sorted values** (in `cluster.HashHeaderValues`). ~6 LoC over single-value; keeps the header method faithful. The fixture uses single-value. (Task 2.)
- **D-S362-5 (empty-`header_name` reject wording) → RESOLVED: pin the go-binding PGV form `value length must be at least 1 runes`.** envoy-go hand-rolls against the go module (not the C++ binary) and its other parity rejects mirror the go-binding PGV text. No cross-side fixture pins it (the `0062` fixture is an ACCEPT path), so the exact text is unit-level discretion per ADR-0080. (Task 3.)
- **D-S362-6 (`0062` affinity attribution) → RESOLVED: reuse the 36.1 aggregate-count modular invariant** (bind K repeats per distinct `X-Hash` value → each backend's per-value count is a multiple of K → per-value affinity is provable from aggregate counts), applicable to BOTH sides since the header key is NAT-transparent. NO new BackendKind (existing HTTP backends reused). (Task 5.)
- **D-S362-7 (conformance gate) → RESOLVED: the final task RE-RUNS h2spec/proxy-wasm where Docker is present, ELSE asserts-unaffected with the explicit rationale** "the producer only STUFFS a ctx key before the existing dial; the request/response wire path is byte-unchanged when no `hash_policy` is configured — which is every conformance config." 36.2 DOES touch the H1/H2 router path (NOT zero-touch like 36.1), so the rationale obligation is STRONGER than 36.1's. The real guard is the full 63-dir differential re-verify (Task 7). (Task 7.)
- **ADR-0045 split-gate re-check → NO SPLIT.** Envelope ≈ 150 production LoC across 8 tasks (1 additive `cluster` helper + 1 hcm parse + 1 router descriptor/fold/4-dial-site wiring + 1 fixture), all against the LANDED ADR-0235 seam. Well under the gate; the 36.1/36.2 by-plane split is already CONSUMED (parent SPEC §3.0). No further split.

---

## File Structure

**New files:**
- `test/fixtures/0062-lb-ring-hash-http/` — the differential fixture (envoy.yaml, driver, asserters, README) — Task 5.

**Modified files (production):**
- `internal/cluster/hash.go` — add the additive exported `HashHeaderValues(values []string) uint64` (seed-chained `XXH64` fold). Task 2.
- `internal/filter/http/router/router.go` — add the `HashPolicy` descriptor type + `HashKind` enum; the `WithDownstreamRemoteAddr`/`downstreamRemoteAddrFrom` ctx-carry; the `applyHashKey` helper; extend `H1ClusterAction(c, hps)` + `routerAction.hashPolicies`; rebind ctx at `doH1ClusterAction:504` + `routerAction.do:641`. Tasks 3, 4.
- `internal/filter/http/router/router_h2.go` — extend `H2ClusterAction(c, hps)` + `routerActionH2.hashPolicies`; the H2 hpack `headerVal` shim; rebind ctx at `doH2ClusterAction:57` + `routerActionH2.doH2:214`. Task 4.
- `internal/filter/hcm/config.go` — `parseRouteHashPolicies(r.GetHashPolicy()) ([]router.HashPolicy, error)`; call it in `buildRouterAction`; carry `hashPolicies` onto `clusterRouteAction`. Task 3.
- `internal/filter/hcm/actions.go` — `clusterRouteAction.hashPolicies` field; pass it through `asRouterAction()` → `H1ClusterAction(a.cluster, a.hashPolicies)` + `asRouterActionH2()` → `H2ClusterAction(a.cluster, a.hashPolicies)`. Task 3.
- `internal/filter/hcm/connection.go` — set `router.WithDownstreamRemoteAddr` on the dispatch ctx (H1). Task 4.
- `internal/filter/hcm/h2dispatch.go` — set `router.WithDownstreamRemoteAddr` on the dispatch ctx (H2). Task 4.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md` (ADR-0237), `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/36.2-load-balancer-ring-hash-http/PROGRESS.md` — Tasks 1, 8.

**Untouched (the LANDED-at-36.1 surface — confirm NO diff at Task 7):** `internal/cluster/loadbalancer.go`, `cluster.go` (the `WithHashKey`/`Dial`/`AcquireH1` threading — CONSUMED), `ringhash.go`, `manager.go`, `internal/filter/tcpproxy/filter.go`.

---

## Task 1: First-task baselines/anchors gate + PROGRESS.md

**Files:**
- Create: `docs/envoy-go/phases/36.2-load-balancer-ring-hash-http/PROGRESS.md`
- Read-only: the as-built anchors below.

No production code. Re-confirm every count via the canonical recipe so the IMPL starts from a verified baseline, and re-pin the as-built line anchors (line numbers drift — re-grep, do not trust this doc's numbers blindly).

- [ ] **Step 1: Re-confirm the five counts.**

```bash
cd <worktree>
ls -d test/fixtures/[0-9]* | wc -l          # expect 63 (tail 0061-lb-ring-hash)
ls -d test/fixtures/[0-9]* | tail -1        # expect test/fixtures/0061-lb-ring-hash
grep -rho 'Fuzz[A-Za-z0-9_]*' --include=*_test.go | sort -u | wc -l   # expect 42 (tail FuzzThriftDecode)
grep -rn 'TCPThriftResponder' test/ | head   # BackendKind tail 33 present
grep -c '^### ADR-' docs/envoy-go/DECISIONS.md   # DECISIONS tail ADR-0236 (next-free ADR-0237)
```
Expected: fixtures **63**, fuzzers **42**, BackendKind tail **33**, DECISIONS tail **ADR-0236**. Stat surface **1119** — re-confirm via the canonical stat-surface recipe documented in `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the DOC count); record the exact command + output in PROGRESS.md.

- [ ] **Step 2: Re-pin the as-built code anchors** (re-grep; record the CURRENT line numbers in PROGRESS.md):

```bash
grep -n 'func WithHashKey\|func hashKeyFrom\|func HashSourceIP' internal/cluster/cluster.go internal/cluster/hash.go
grep -n 'func buildRouterAction\|routev3 "' internal/filter/hcm/config.go
grep -n 'type clusterRouteAction\|func (a \*clusterRouteAction) asRouterAction' internal/filter/hcm/actions.go
grep -n 'func H1ClusterAction\|type routerAction\|func doH1ClusterAction\|func (a \*routerAction) do' internal/filter/http/router/router.go
grep -n 'func H2ClusterAction\|type routerActionH2\|func doH2ClusterAction\|func (r \*routerActionH2) doH2' internal/filter/http/router/router_h2.go
grep -n 'SetDownstreamRemoteAddr' internal/filter/hcm/connection.go internal/filter/hcm/h2dispatch.go
```
Confirm: `WithHashKey`/`HashSourceIP`/`xxHash64` LANDED; `buildRouterAction` returns `&clusterRouteAction{cluster: c}`; the four dial sites (`AcquireH1`, `Dial`, two `DialH2`) present; `chain.SetDownstreamRemoteAddr` at connection.go (H1 dispatch) + h2dispatch.go (H2 dispatch).

- [ ] **Step 3: Create PROGRESS.md** with the baseline counts, the pinned anchors, the 8-task checklist, and the D-resolution summary (mirror the 36.1 `PROGRESS.md` shape). Commit.

```bash
git add docs/envoy-go/phases/36.2-load-balancer-ring-hash-http/PROGRESS.md
git commit -m "phase 36.2 IMPL Task 1: baselines/anchors gate + PROGRESS.md"
```

---

## Task 2: The header-hash digest — `cluster.HashHeaderValues` (D-S362-1, D-S362-4)

**Files:**
- Modify: `internal/cluster/hash.go`
- Test: `internal/cluster/hash_test.go`

The header contribution is the seed-chained `XXH64` fold over byte-sorted values (SPEC §11.2 / `hash_policy.cc:43-77` + `hash.cc:7-12`): `seed=0`; for each value (sorted), `seed = XXH64(value, seed)`. The single-value case collapses to `xxHash64([]byte(value))` (the fixture path). This is the ONE additive exported `cluster` symbol.

**Note on the seed-chained XXH64:** the LANDED `xxHash64(b []byte)` hardcodes `seed=0`. The fold needs `XXH64(value, seed)` with a non-zero seed for the 2nd+ value. Generalize by adding an internal `xxHash64Seed(b []byte, seed uint64) uint64` (the existing body with `var seed uint64` replaced by the parameter — the accumulator-init `seed + prime64_1 + prime64_2` etc. already reference the `seed` var) and have the existing `xxHash64(b)` delegate as `xxHash64Seed(b, 0)`. This is a behavior-neutral refactor (all existing `xxHash64` callers unchanged); confirm the existing `hash_test.go` XXH64 vectors still pass.

- [ ] **Step 1: Write the failing test.** Anchor on the published XXH64 single-value vector already pinned in `hash_test.go` (e.g. `xxHash64("")==0xEF46DB3751D8E999`) plus a seed-chain + sort assertion:

```go
func TestHashHeaderValues(t *testing.T) {
	// Single value collapses to xxHash64([]byte(value)) (the fixture path).
	if got, want := HashHeaderValues([]string{"alpha"}), xxHash64([]byte("alpha")); got != want {
		t.Fatalf("single-value: got %#x want %#x", got, want)
	}
	// Empty list → 0 (caller treats the contribution as "no value"; applyHashKey
	// only calls this when the header is present, so this is a defensive pin).
	if got := HashHeaderValues(nil); got != 0 {
		t.Fatalf("empty: got %#x want 0", got)
	}
	// Seed-chained over BYTE-SORTED values: XXH64("b", XXH64("a", 0)).
	want := xxHash64Seed([]byte("b"), xxHash64Seed([]byte("a"), 0))
	if got := HashHeaderValues([]string{"b", "a"}); got != want { // input unsorted → sorted internally
		t.Fatalf("multi-value: got %#x want %#x", got, want)
	}
	// Sort is byte-wise: {"a","b"} and {"b","a"} produce the SAME key.
	if HashHeaderValues([]string{"a", "b"}) != HashHeaderValues([]string{"b", "a"}) {
		t.Fatal("multi-value fold must be sort-order-independent")
	}
}
```

- [ ] **Step 2: Run it — expect FAIL** (`HashHeaderValues`/`xxHash64Seed` undefined):

```bash
go test ./internal/cluster/ -run TestHashHeaderValues -count=1
```

- [ ] **Step 3: Implement.** In `hash.go`, refactor `xxHash64` to delegate to a seeded variant, then add `HashHeaderValues`:

```go
// xxHash64 computes the XXH64 digest of b with the seed fixed to 0 (Envoy's
// XX_HASH default). Delegates to the seeded variant.
func xxHash64(b []byte) uint64 { return xxHash64Seed(b, 0) }

// xxHash64Seed is the XXH64 reference algorithm with a caller-supplied seed —
// the seed-chained header fold (HashHeaderValues) feeds each value's output as
// the next value's seed (source/common/common/hash.cc:7-12). seed==0 reproduces
// xxHash64's incumbent behavior byte-for-byte.
func xxHash64Seed(b []byte, seed uint64) uint64 {
	// (body of the former xxHash64, with the leading `var seed uint64` removed —
	// the parameter `seed` is now the accumulator base used by v1..v4 init and
	// the n<32 short path. The rest is byte-identical.)
	n := len(b)
	var h uint64
	// ... existing stripe loop / tail / avalanche, referencing `seed` ...
}

// HashHeaderValues returns the ring_hash consistent-hash key for an HTTP route
// header hash_policy: the seed-chained XXH64 fold over the BYTE-SORTED header
// values (Envoy HeaderHashMethod — source/common/http/hash_policy.cc:43-77).
// A single value collapses to xxHash64([]byte(value)); an empty list → 0.
// Exported so the http/router producer computes the digest without reaching
// cluster's unexported xxHash64 (the ONE additive 36.2 exported symbol;
// the exported Cluster surface stays byte-stable). ADR-0237.
func HashHeaderValues(values []string) uint64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]string(nil), values...)
	sort.Strings(sorted) // byte-wise std::sort parity
	var seed uint64
	for _, v := range sorted {
		seed = xxHash64Seed([]byte(v), seed)
	}
	return seed
}
```
Add `"sort"` to the import block.

- [ ] **Step 4: Run — expect PASS**, and confirm the incumbent XXH64 vectors still pass (the seed-refactor is behavior-neutral):

```bash
go test ./internal/cluster/ -run 'TestHashHeaderValues|TestXXHash|TestHash' -count=1
gofmt -l internal/cluster/hash.go && golangci-lint run ./internal/cluster/...
```
Expected: PASS; `gofmt -l` prints nothing; lint clean.

- [ ] **Step 5: Commit.**

```bash
git add internal/cluster/hash.go internal/cluster/hash_test.go
git commit -m "phase 36.2 IMPL Task 2: cluster.HashHeaderValues seed-chained XXH64 fold (D-S362-1/4)"
```

---

## Task 3: The config-build parse + DEPARTURE-reject + descriptor wiring (D-S362-2, D-S362-5)

**Files:**
- Modify: `internal/filter/http/router/router.go` (add the `HashPolicy` descriptor type + `HashKind`)
- Modify: `internal/filter/hcm/config.go` (`parseRouteHashPolicies` + call in `buildRouterAction`)
- Modify: `internal/filter/hcm/actions.go` (`clusterRouteAction.hashPolicies` + the two bridges)
- Test: `internal/filter/hcm/config_test.go` (the §6 reject matrix), `internal/filter/http/router/router_test.go` (extend the constructors)

This task wires PARSE + DESCRIPTOR end-to-end but the per-request fold (`applyHashKey`) is Task 4 — here the extended constructors just STORE `hps` (behavior-neutral: an empty/ignored `hashPolicies` leaves every path byte-stable). The reject matrix is the deliverable.

- [ ] **Step 1: Define the proto-free descriptor** in `router.go` (router stays proto-free — D-S362-2):

```go
// HashKind enumerates the supported RouteAction.hash_policy specifiers (36.2).
type HashKind uint8

const (
	HashKindHeader   HashKind = iota // xxHash64 over the matched request-header value
	HashKindSourceIP                 // cluster.HashSourceIP over the downstream client IP
)

// HashPolicy is a lowered, proto-free RouteAction.hash_policy entry. The proto
// read + DEPARTURE-reject happens at the hcm config-build boundary
// (parseRouteHashPolicies); this descriptor is router-owned because the
// per-request fold (applyHashKey) lives here. ADR-0237.
type HashPolicy struct {
	Kind       HashKind
	HeaderName string // lowercased; set for HashKindHeader only
	Terminal   bool   // RouteAction_HashPolicy.terminal — short-circuit once acc is non-empty
}
```

- [ ] **Step 2: Write the failing reject-matrix test** in `hcm/config_test.go` (table-driven; the §6 roster). Build a `*routev3.RouteAction` per row and assert `parseRouteHashPolicies` accept/reject + the descriptor shape:

```go
func TestParseRouteHashPolicies(t *testing.T) {
	hdr := func(name string) *routev3.RouteAction_HashPolicy {
		return &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_Header_{
			Header: &routev3.RouteAction_HashPolicy_Header{HeaderName: name}}}
	}
	srcip := func(v bool) *routev3.RouteAction_HashPolicy {
		return &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_ConnectionProperties_{
			ConnectionProperties: &routev3.RouteAction_HashPolicy_ConnectionProperties{SourceIp: v}}}
	}
	cookie := &routev3.RouteAction_HashPolicy{PolicySpecifier: &routev3.RouteAction_HashPolicy_Cookie_{
		Cookie: &routev3.RouteAction_HashPolicy_Cookie{Name: "sess"}}}
	// ... query_parameter, filter_state, header+regex_rewrite, header+terminal rows ...

	tests := []struct {
		name    string
		in      []*routev3.RouteAction_HashPolicy
		wantErr string // "" = accept
		want    []router.HashPolicy
	}{
		{"nil", nil, "", nil},
		{"header", []*routev3.RouteAction_HashPolicy{hdr("x-hash")}, "", []router.HashPolicy{{Kind: router.HashKindHeader, HeaderName: "x-hash"}}},
		{"header-upper-lowered", []*routev3.RouteAction_HashPolicy{hdr("X-Hash")}, "", []router.HashPolicy{{Kind: router.HashKindHeader, HeaderName: "x-hash"}}},
		{"source_ip", []*routev3.RouteAction_HashPolicy{srcip(true)}, "", []router.HashPolicy{{Kind: router.HashKindSourceIP}}},
		{"header-terminal", []*routev3.RouteAction_HashPolicy{hdrTerminal("x-hash")}, "", []router.HashPolicy{{Kind: router.HashKindHeader, HeaderName: "x-hash", Terminal: true}}},
		{"empty-header-name", []*routev3.RouteAction_HashPolicy{hdr("")}, "value length must be at least 1 runes", nil},
		{"regex-rewrite", []*routev3.RouteAction_HashPolicy{hdrRegex("x-hash")}, "regex_rewrite is not supported", nil},
		{"source_ip-false", []*routev3.RouteAction_HashPolicy{srcip(false)}, "connection_properties without source_ip", nil},
		{"cookie", []*routev3.RouteAction_HashPolicy{cookie}, "is not supported (only header, connection_properties.source_ip)", nil},
		// ... query_parameter, filter_state ...
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRouteHashPolicies(tt.in)
			if tt.wantErr == "" {
				if err != nil { t.Fatalf("accept expected, got %v", err) }
				if !reflect.DeepEqual(got, tt.want) { t.Fatalf("descriptor: got %+v want %+v", got, tt.want) }
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want err containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
```

- [ ] **Step 3: Run — expect FAIL** (`parseRouteHashPolicies` undefined):

```bash
go test ./internal/filter/hcm/ -run TestParseRouteHashPolicies -count=1
```

- [ ] **Step 4: Implement `parseRouteHashPolicies` in `hcm/config.go`** (the tcp_proxy `NewFilter` fail-fast precedent; `routev3` already imported). Lowercase the header name (`strings.ToLower`) to match the H2 hpack fields + Envoy's case-insensitive lookup:

```go
// parseRouteHashPolicies lowers RouteAction.hash_policy[] into proto-free
// router.HashPolicy descriptors, fail-fast-rejecting unsupported specifiers
// at config-build (the tcp_proxy NewFilter source_ip precedent). header +
// connection_properties.source_ip are SUPPORTED; cookie/query_parameter/
// filter_state + a configured regex_rewrite + a source_ip==false
// connection_properties DEPARTURE-reject (the reference validate-ACCEPTS the
// three specifiers — recorded departures, ADR-0080); an empty header_name
// PARITY-rejects the PGV min_len=1. No hash_policy → nil (byte-stable). §6/ADR-0237.
func parseRouteHashPolicies(policies []*routev3.RouteAction_HashPolicy) ([]router.HashPolicy, error) {
	if len(policies) == 0 {
		return nil, nil
	}
	out := make([]router.HashPolicy, 0, len(policies))
	for _, hp := range policies {
		switch spec := hp.GetPolicySpecifier().(type) {
		case *routev3.RouteAction_HashPolicy_Header_:
			h := spec.Header
			if h.GetHeaderName() == "" {
				return nil, fmt.Errorf("router: hash_policy header_name: value length must be at least 1 runes")
			}
			if h.GetRegexRewrite() != nil {
				return nil, fmt.Errorf("router: hash_policy header_name %q: regex_rewrite is not supported", h.GetHeaderName())
			}
			out = append(out, router.HashPolicy{
				Kind:       router.HashKindHeader,
				HeaderName: strings.ToLower(h.GetHeaderName()),
				Terminal:   hp.GetTerminal(),
			})
		case *routev3.RouteAction_HashPolicy_ConnectionProperties_:
			if !spec.ConnectionProperties.GetSourceIp() {
				return nil, fmt.Errorf("router: hash_policy connection_properties without source_ip is not supported")
			}
			out = append(out, router.HashPolicy{Kind: router.HashKindSourceIP, Terminal: hp.GetTerminal()})
		default:
			return nil, fmt.Errorf("router: hash_policy specifier %T is not supported (only header, connection_properties.source_ip)", hp.GetPolicySpecifier())
		}
	}
	return out, nil
}
```
Wire it into `buildRouterAction` (replace the final `return &clusterRouteAction{cluster: c}, nil`):

```go
	hps, err := parseRouteHashPolicies(r.GetHashPolicy())
	if err != nil {
		return nil, err
	}
	return &clusterRouteAction{cluster: c, hashPolicies: hps}, nil
```

- [ ] **Step 5: Carry the descriptor onto `clusterRouteAction`** (`hcm/actions.go`) and pass it through both bridges:

```go
type clusterRouteAction struct {
	cluster      *cluster.Cluster
	hashPolicies []router.HashPolicy // 36.2: route RouteAction.hash_policy producer (ADR-0237)
}
// ...
func (a *clusterRouteAction) asRouterAction() router.Action {
	return router.H1ClusterAction(a.cluster, a.hashPolicies)
}
func (a *clusterRouteAction) asRouterActionH2() router.H2Action {
	return router.H2ClusterAction(a.cluster, a.hashPolicies)
}
```
The legacy `clusterRouteAction.do` also calls `router.H1ClusterAction(a.cluster)` (actions.go:212) — update to `router.H1ClusterAction(a.cluster, a.hashPolicies)`.

- [ ] **Step 6: Extend the constructors (store-only at this task)** in `router.go` / `router_h2.go`:

```go
// router.go
func H1ClusterAction(c *cluster.Cluster, hps []HashPolicy) Action {
	a := &routerAction{cluster: c, hashPolicies: hps}
	return func(ctx context.Context, req *http.Request) (ActionResponse, cluster.Endpoint, error) {
		return doH1ClusterAction(ctx, a, req)
	}
}
type routerAction struct {
	cluster      *cluster.Cluster
	filter       *Filter
	hashPolicies []HashPolicy // 36.2
}
// router_h2.go
func H2ClusterAction(c *cluster.Cluster, hps []HashPolicy) H2Action {
	a := &routerActionH2{cluster: c, hashPolicies: hps}
	return func(ctx context.Context, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
		return doH2ClusterAction(ctx, a, req)
	}
}
type routerActionH2 struct {
	cluster      *cluster.Cluster
	filter       *Filter
	hashPolicies []HashPolicy // 36.2
}
```
Fix all OTHER `H1ClusterAction(`/`H2ClusterAction(` callers (production + tests) to pass `nil` — grep the blast radius:
```bash
grep -rn 'H1ClusterAction(\|H2ClusterAction(' internal/ | grep -v '_test.go:.*nil)'
```

- [ ] **Step 7: Run — expect PASS** (reject matrix green; constructor ripple compiles; all prior router/hcm tests green):

```bash
go build ./...
go test ./internal/filter/hcm/ ./internal/filter/http/router/ -run 'HashPolic|ClusterAction' -count=1
gofmt -l internal/filter/hcm/config.go internal/filter/hcm/actions.go internal/filter/http/router/router.go internal/filter/http/router/router_h2.go
golangci-lint run ./internal/filter/hcm/... ./internal/filter/http/router/...
```
Expected: PASS; `gofmt -l` empty; lint clean.

- [ ] **Step 8: Commit.**

```bash
git add internal/filter/hcm/config.go internal/filter/hcm/actions.go internal/filter/http/router/router.go internal/filter/http/router/router_h2.go internal/filter/hcm/config_test.go internal/filter/http/router/router_test.go
git commit -m "phase 36.2 IMPL Task 3: parseRouteHashPolicies + DEPARTURE-reject matrix + descriptor wiring (D-S362-2/5)"
```

---

## Task 4: The per-request fold + the 4 dial-site ctx rebinds (D-S362-3, D-S362-4)

**Files:**
- Modify: `internal/filter/http/router/router.go` (the `applyHashKey` helper; the ctx-carry; the H1 `headerVal` shim; rebind at `doH1ClusterAction` + `routerAction.do`)
- Modify: `internal/filter/http/router/router_h2.go` (the H2 hpack `headerVal` shim; rebind at `doH2ClusterAction` + `routerActionH2.doH2`)
- Modify: `internal/filter/hcm/connection.go` (set the ctx-carry, H1 dispatch)
- Modify: `internal/filter/hcm/h2dispatch.go` (set the ctx-carry, H2 dispatch)
- Test: `internal/filter/http/router/router_test.go`

- [ ] **Step 1: Write the failing fold + dispatch tests.** Pin the fold algorithm directly (SPEC §3.2) against `cluster.HashHeaderValues`/`HashSourceIP`, plus per-codec end-to-end key-stuffing:

```go
func TestApplyHashKey(t *testing.T) {
	hv := func(m map[string]string) func(string) (string, bool) {
		return func(n string) (string, bool) { v, ok := m[n]; return v, ok }
	}
	hdr := func(name string) HashPolicy { return HashPolicy{Kind: HashKindHeader, HeaderName: name} }
	src := HashPolicy{Kind: HashKindSourceIP}

	// 1. single header → cluster.HashHeaderValues([]string{value}) stuffed.
	ctx := applyHashKey(context.Background(), []HashPolicy{hdr("x-hash")}, hv(map[string]string{"x-hash": "alpha"}), "")
	key, ok := cluster.HashKeyForTest(ctx) // test-only accessor; see Step 3
	if !ok || key != cluster.HashHeaderValues([]string{"alpha"}) {
		t.Fatalf("header key: ok=%v key=%#x", ok, key)
	}
	// 2. absent header → no contribution → ctx untouched (no key).
	if _, ok := cluster.HashKeyForTest(applyHashKey(context.Background(), []HashPolicy{hdr("x-hash")}, hv(nil), "")); ok {
		t.Fatal("absent header must leave ctx keyless")
	}
	// 3. two policies fold rotl64(prev,1)^new, first verbatim.
	got, _ := cluster.HashKeyForTest(applyHashKey(context.Background(), []HashPolicy{hdr("x-hash"), src},
		hv(map[string]string{"x-hash": "alpha"}), "10.0.0.7:5555"))
	first := cluster.HashHeaderValues([]string{"alpha"})
	want := bits.RotateLeft64(first, 1) ^ cluster.HashSourceIP("10.0.0.7:5555")
	if got != want { t.Fatalf("fold: got %#x want %#x", got, want) }
	// 4. terminal short-circuits: header(terminal)+src → only header contributes.
	ht := HashPolicy{Kind: HashKindHeader, HeaderName: "x-hash", Terminal: true}
	got2, _ := cluster.HashKeyForTest(applyHashKey(context.Background(), []HashPolicy{ht, src},
		hv(map[string]string{"x-hash": "alpha"}), "10.0.0.7:5555"))
	if got2 != first { t.Fatalf("terminal: got %#x want %#x (header only)", got2, first) }
	// 5. terminal on a nullopt policy does NOT short-circuit (acc still empty).
	htAbsent := HashPolicy{Kind: HashKindHeader, HeaderName: "absent", Terminal: true}
	got3, _ := cluster.HashKeyForTest(applyHashKey(context.Background(), []HashPolicy{htAbsent, src},
		hv(nil), "10.0.0.7:5555"))
	if got3 != cluster.HashSourceIP("10.0.0.7:5555") {
		t.Fatalf("terminal-on-nullopt must not short-circuit; got %#x", got3)
	}
	// 6. no policies → ctx untouched.
	if _, ok := cluster.HashKeyForTest(applyHashKey(context.Background(), nil, hv(nil), "")); ok {
		t.Fatal("no policy → keyless")
	}
}
```
Add an H1 + H2 dispatch-level test asserting the key reaches `Dial`/`AcquireH1`/`DialH2` via a fake `*cluster.Cluster` (or assert the ctx-carry through a seam test — reuse the 36.1 `cluster_test.go` hashKeyFrom witness pattern). **NOTE — `cluster.HashKeyForTest`**: `hashKeyFrom` is unexported. Add a tiny test-only export in `internal/cluster/export_test.go` is NOT visible cross-package; instead add `cluster.HashKeyForTest(ctx)(uint64,bool)` as an exported wrapper in a `cluster` non-test file guarded by intent, OR assert the fold purely in the `router` package by having `applyHashKey` return `(context.Context, uint64, bool)` for testability. **Decision (pin at Step 3):** make `applyHashKey` return `(ctx, key uint64, has bool)` and have the dial sites use `ctx`; the test asserts `key`/`has` directly — no cluster test-export needed.

- [ ] **Step 2: Run — expect FAIL** (`applyHashKey` undefined):

```bash
go test ./internal/filter/http/router/ -run TestApplyHashKey -count=1
```

- [ ] **Step 3: Implement the ctx-carry + the fold** in `router.go`. Revised signature returns the key for testability (the dial sites ignore `key`/`has` and just use `ctx`):

```go
import "math/bits" // add

type downstreamRemoteAddrCtxKey struct{}

// WithDownstreamRemoteAddr carries the downstream client's remote address
// ("host:port") for the source_ip hash_policy producer. HCM dispatch sets it
// before invoking the action (H1: connection.go; H2: h2dispatch.go) since
// neither *http.Request (HCM codec path) nor h2.H2Request carries a remote
// addr. ADR-0237.
func WithDownstreamRemoteAddr(ctx context.Context, addr string) context.Context {
	return context.WithValue(ctx, downstreamRemoteAddrCtxKey{}, addr)
}
func downstreamRemoteAddrFrom(ctx context.Context) string {
	v, _ := ctx.Value(downstreamRemoteAddrCtxKey{}).(string)
	return v
}

// applyHashKey folds the route's hash_policy list into a ring_hash key and
// returns ctx carrying it (cluster.WithHashKey). If nothing contributes, ctx is
// returned unchanged (the LB's no-hash fallback). Mirrors Envoy
// HashPolicyImpl::generateHash (SPEC §3.2 / hash_policy.cc:259-270):
// rotl64(prev,1)^new fold, first contributor verbatim, nullopt policies skipped
// entirely, a terminal policy short-circuiting once the accumulator is non-empty.
// headerVal is a codec-agnostic accessor; remoteAddr is the ctx-carried
// downstream addr (empty when unset). Returns (ctx, key, has) for testability;
// the dial sites use only ctx.
func applyHashKey(ctx context.Context, hps []HashPolicy, headerVal func(name string) (string, bool), remoteAddr string) (context.Context, uint64, bool) {
	var acc uint64
	var has bool
	for _, hp := range hps {
		var nh uint64
		var ok bool
		switch hp.Kind {
		case HashKindHeader:
			if v, present := headerVal(hp.HeaderName); present {
				nh, ok = cluster.HashHeaderValues([]string{v}), true
			}
		case HashKindSourceIP:
			if remoteAddr != "" {
				nh, ok = cluster.HashSourceIP(remoteAddr), true
			}
		}
		if ok {
			if has {
				acc = bits.RotateLeft64(acc, 1) ^ nh
			} else {
				acc, has = nh, true
			}
		}
		if hp.Terminal && has {
			break // hash_policy.cc:270 — checks the accumulator
		}
	}
	if !has {
		return ctx, 0, false
	}
	return cluster.WithHashKey(ctx, acc), acc, true
}
```
**Multi-value header note (D-S362-4):** the H1/H2 `headerVal` shims below return the FIRST value (single-value fixture path). To support genuine multi-value headers faithfully, `applyHashKey`'s header arm would call `cluster.HashHeaderValues(allValues)` — but that requires a `headerValues func(name string)([]string,bool)` accessor. Since the fixture + the live reference run are single-value and H1's `req.Header.Get`/H2's first-match are single-value by construction, keep the single-value `headerVal` closure; the FULL multi-value fold already lives in `cluster.HashHeaderValues` (Task 2) and is exercised by its unit test. Record this boundary in PROGRESS.md (the producer feeds single-value; the digest supports multi-value).

- [ ] **Step 4: Rebind ctx at the 4 dial sites.** H1 `doH1ClusterAction` (before `AcquireH1`, ~:509) and `routerAction.do` (before `Dial`, ~:662):

```go
// doH1ClusterAction — after a.cluster.IncUpstreamRqTotal(), before AcquireH1:
ctx, _, _ = applyHashKey(ctx, a.hashPolicies, func(n string) (string, bool) {
	v := req.Header.Get(n)
	return v, v != ""
}, downstreamRemoteAddrFrom(ctx))
```
H2 `doH2ClusterAction` (before `DialH2`, ~:62) and `routerActionH2.doH2` (before `DialH2`, ~:233) use an hpack scan shim:

```go
// router_h2.go — h2HeaderVal scans the (lowercased) hpack fields.
func h2HeaderVal(req h2.H2Request) func(name string) (string, bool) {
	return func(name string) (string, bool) {
		for _, f := range req.Headers { // field names already lowercased
			if f.Name == name {
				return f.Value, true
			}
		}
		return "", false
	}
}
// doH2ClusterAction — after IncUpstreamRqTotal(), before DialH2:
ctx, _, _ = applyHashKey(ctx, a.hashPolicies, h2HeaderVal(req), downstreamRemoteAddrFrom(ctx))
```
(`req.Header.Get` already lowercases per Go's textproto canonicalization-then-lookup; the descriptor's `HeaderName` is lowercased at parse, and `http.Header.Get` is case-insensitive — so H1 matching is correct. The H2 hpack fields are lowercased by the codec; the descriptor is lowercased — direct `==` is correct.)

- [ ] **Step 5: Set the ctx-carry at the two HCM dispatch sites.** H1 — in `connection.go dispatchRequest`, where `downstream` is in scope (guard nil downstream like the chain seed at :383):

```go
if downstream != nil {
	ctx = router.WithDownstreamRemoteAddr(ctx, downstream.RemoteAddr().String())
}
```
**Carrier note:** insert this reassignment on the live `ctx` BEFORE `RunAction(ctx)` — the action dispatch threads the function-scope `ctx` straight into the closure. Do NOT rely on `chain.SetRequestCtx` (connection.go ~:343, which captures `ctx` earlier in the function and would NOT carry the addr); the live `ctx` variable is the carrier, not the chain-stored ctx.

H2 — in `h2dispatch.go`, wrap the ctx threaded to the action with `c.downstreamRemoteAddr` (the `chainDispatchAction` field). At each H2 action-invoke point (`RunAction(ctx)` and the no-match `c.action(ctx, h2req)`), if `c.downstreamRemoteAddr != nil`:

```go
if c.downstreamRemoteAddr != nil {
	ctx = router.WithDownstreamRemoteAddr(ctx, c.downstreamRemoteAddr.String())
}
```
Re-grep the exact lines at IMPL (`grep -n 'RunAction(ctx)\|c.action(ctx' internal/filter/hcm/h2dispatch.go`); the H1 `dispatchRequest` already imports `router` (it builds router actions), and h2dispatch.go does too.

- [ ] **Step 6: Run — expect PASS** (fold + dispatch tests; all prior tests green):

```bash
go build ./...
go test ./internal/filter/http/router/ ./internal/filter/hcm/ ./internal/cluster/ -count=1
gofmt -l internal/filter/http/router/ internal/filter/hcm/
golangci-lint run ./internal/filter/http/router/... ./internal/filter/hcm/...
```
Expected: PASS; gofmt empty; lint clean. (The `ctx, _, _ =` triple-assignment must not trip `ineffassign`/`staticcheck` — if it does, use a named throwaway or change the dial sites to `ctx, _, _ = applyHashKey(...)` which is a genuine ctx reassignment, not ineffective.)

- [ ] **Step 7: Commit.**

```bash
git add internal/filter/http/router/ internal/filter/hcm/connection.go internal/filter/hcm/h2dispatch.go
git commit -m "phase 36.2 IMPL Task 4: applyHashKey fold + 4 dial-site ctx rebinds + uniform remote-addr carry (D-S362-3/4)"
```

---

## Task 5: The `0062-lb-ring-hash-http` differential fixture (D-S362-6)

**Files:**
- Create: `test/fixtures/0062-lb-ring-hash-http/` (envoy.yaml, driver.go or the fixture's driver convention, README.md)
- Reference: `test/fixtures/0061-lb-ring-hash/` (the 36.1 sibling — mirror its structure, asserters, modular-invariant affinity proof)

Mirror `0061` exactly, swapping the tcp `source_ip` plane for an HTTP listener + a route-level `hash_policy: header`.

- [ ] **Step 1: Author `envoy.yaml`** — an HTTP listener (HCM + router) routing to a 3-endpoint `RING_HASH` cluster (`ring_hash_lb_config: {}` defaults; STRICT_DNS or the fixture's static-3-host convention), with a route-level `hash_policy: [{header: {header_name: "x-hash"}}]`. Backends: the existing HTTP backends (NO new BackendKind — tail STAYS 33). Copy the `0061` cluster block (the RING_HASH config is identical) + a `0060`/HTTP-fixture listener block.

- [ ] **Step 2: Author the driver + asserters** mirroring `0061`'s `DistributionAsserter` + `StatsAsserter`. The workload: N≥4 distinct `X-Hash` values × K repeats each (K=16 per the 36.1 modular-invariant convention; pick the smallest K that keeps the spread stable — re-use 0061's K). Assertions:
  - **Affinity (BOTH sides, modular invariant — D-S362-6):** each backend's total request count ≡ 0 (mod K). Because each distinct header value is internally deterministic (one backend) and sent K times, every backend's aggregate count is a sum of per-value K-multiples → ≡ 0 mod K. Holds on BOTH subject and reference (the `X-Hash` header is NAT-transparent). A scattered key breaks the invariant (a value's K repeats split across backends → a non-multiple).
  - **Spread (BOTH sides):** distinct values collectively cover `>= 2` backends per side.
  - **Cross-side byte-equivalence** of the HTTP responses (standard prong).
  - **Cross-side `StatsAsserter`** (SPEC §7): the 3 `ring_hash_lb.{size=1026,min=342,max=342}` gauges cross-equal + `membership_total=3` + quiesced `upstream_cx_active=0` + **`upstream_rq_total` cross-equal** (the HTTP-plane rq increment — the new prong vs `0061`).
  - **NOT asserted:** cross-side host IDENTITY (the two rings are built over different endpoint address strings — AMEND-RH8 / `reference_differential_hash_key_cross_side_infeasible`).

- [ ] **Step 3: Author `README.md`** documenting the workload, the pinned property (per-value affinity + spread + byte-equiv + cross-equal stats — NOT host identity), the deliberate-break liveness (Task 6), and the K choice.

- [ ] **Step 4: Run `0062` green** (Docker required — per `reference_docker_probe_bridge_network`, use a bridge network + STRICT_DNS backend hostname; confirm decode ran via `upstream_cx_rx_bytes_total > 0`). Use the selector `-run 'TestDifferential/0062'` (NOT bare `0062` — `reference_differential_run_selector`):

```bash
go test ./test/... -run 'TestDifferential/0062-lb-ring-hash-http' -count=1 -v
```
Expected: PASS, with both-side affinity + spread + cross-equal stats green.

- [ ] **Step 5: Commit.**

```bash
git add test/fixtures/0062-lb-ring-hash-http/
git commit -m "phase 36.2 IMPL Task 5: 0062-lb-ring-hash-http differential fixture (NAT-transparent both-side affinity; D-S362-6)"
```

---

## Task 6: Deliberate-break liveness (`-count=1`) + flake check

**Files:** none committed (temporary edits reverted). Follow `reference_differential_break_protocol_count1` (go test caching serves a stale PASS — ALWAYS `-count=1` after breaking production code) and `feedback_subagent_worktree_detach` (use `git restore`, never `checkout <sha>`/`amend`, to revert breaks).

Prove EACH `0062` assertion is live by breaking the production code it guards and confirming the SPECIFIC prong fails:

- [ ] **Step 1: Scatter the key** — make `applyHashKey` return `ctx, 0, false` unconditionally (or have the header arm skip). Run `-count=1` → a value's K repeats spread across backends → the per-value affinity (modular-invariant) leg FAILS on BOTH sides. Restore (`git restore`).
- [ ] **Step 2: Collapse the spread** — force the cluster to one backend (or stub the ring to a single point). Run `-count=1` → the spread (`>= 2`) leg FAILS. Restore.
- [ ] **Step 3: Drop/corrupt the rq Inc** — skip `IncUpstreamRqTotal` on one dial site (or corrupt the asserted `upstream_rq_total` expectation). Run `-count=1` → the `StatsAsserter` rq prong FAILS. Restore.
- [ ] **Step 4: Corrupt a gauge** — perturb a `ring_hash_lb.*` expected value. Run `-count=1` → the cross-equal gauge prong FAILS. Restore.
- [ ] **Step 5: Flake check** — run `0062` ≥20× clean (affinity is deterministic; spread `>= 2` over N≥4 values / 3 backends is overwhelmingly stable). Record the 4 break outcomes + the 20-run result in the fixture README + PROGRESS.md.

```bash
for i in $(seq 1 20); do go test ./test/... -run 'TestDifferential/0062-lb-ring-hash-http' -count=1 || echo "FLAKE run $i"; done
```
Expected: 20/20 PASS; each break fails the named prong only.

- [ ] **Step 6: Commit** the README/PROGRESS liveness notes (production code is unchanged — restored).

```bash
git add test/fixtures/0062-lb-ring-hash-http/README.md docs/envoy-go/phases/36.2-load-balancer-ring-hash-http/PROGRESS.md
git commit -m "phase 36.2 IMPL Task 6: 0062 deliberate-break liveness (4 prongs) + 20-run flake check"
```

---

## Task 7: Full differential re-verify + the six-gate (D-S362-7)

**Files:** none (verification only).

- [ ] **Step 1: Confirm the LANDED-at-36.1 surface is byte-unchanged** (the producer must not have touched the seam):

```bash
git diff master --stat -- internal/cluster/loadbalancer.go internal/cluster/ringhash.go internal/cluster/manager.go internal/filter/tcpproxy/filter.go
```
Expected: EMPTY (no diff). `cluster.go` shows only the consume (no `WithHashKey`/`Dial` body change); `hash.go` shows only the additive `HashHeaderValues` + the seed-refactor.

- [ ] **Step 2: Full 64-dir differential** — all 63 prior dirs byte-exact through the constructor signature change (the action closures are behavior-neutral when `hashPolicies` is empty) + `0062` green:

```bash
go test ./test/... -count=1   # full differential (Docker present)
```
Expected: 64/64 PASS.

- [ ] **Step 3: `-race -short` + build/vet/gofmt/lint/tidy:**

```bash
go test -race -short ./... -count=1
go build ./... && go vet ./...
gofmt -l . | grep -v '^$' && echo "GOFMT DRIFT" || echo "gofmt clean"
golangci-lint run ./...
go mod tidy -diff   # expect EMPTY — ZERO new dep (D-RH1)
```
Expected: race-clean; build/vet clean; gofmt empty; lint clean; tidy-diff EMPTY.

- [ ] **Step 4: h2spec / proxy-wasm (D-S362-7).** 36.2 TOUCHES the H1/H2 router path → NOT zero-touch-by-construction. Where the h2spec/proxy-wasm Docker harness is present, RE-RUN: expect **h2spec 53/53** + **proxy-wasm 10/10**. Where impractical, ASSERT-unaffected with the explicit rationale: *the producer only STUFFS a ctx key before the existing dial; with no `hash_policy` configured (every conformance config), `applyHashKey` returns `ctx` unchanged and the request/response wire path is byte-identical — the full 64-dir differential (Step 2, which includes all H1/H2 fixtures) is the real guard.* Record which path was taken in PROGRESS.md.

- [ ] **Step 5: Commit** the gate evidence into PROGRESS.md.

```bash
git add docs/envoy-go/phases/36.2-load-balancer-ring-hash-http/PROGRESS.md
git commit -m "phase 36.2 IMPL Task 7: full 64-dir differential + six-gate + h2spec/proxy-wasm disposition (D-S362-7)"
```

---

## Task 8: Completion bundle (ADR-0052 atomic landing)

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/36.2-load-balancer-ring-hash-http/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT.md** — extend the `### Load balancer — ring_hash (RING_HASH)` subsection (created at 36.1) with the 36.2 HTTP-plane delta (SPEC §9): the supported specifiers (`header` `xxHash64`, `connection_properties.source_ip` reusing the tcp compute), the `rotl64(prev,1)^new` fold (first verbatim, nullopt-skip, terminal short-circuit), the single-value collapse vs the multi-value seed-chained fold; the DEPARTURE-rejected specifiers (cookie/query_parameter/filter_state + regex_rewrite + source_ip-false) + the empty-`header_name` parity reject; the NAT-transparent TRUE cross-side affinity; the `upstream_rq_total` cross-equal on the HTTP plane; the stat-surface note STAYS 1119 (the FIRST LB producer reusing the prior plane's stat surface entirely); the NO-new-fuzzer/BackendKind family expectations.

- [ ] **Step 2: DECISIONS.md** — append the full **ADR-0237** (status ACCEPTED) using the SPEC §13 §Context draft VERBATIM as §Context; author §Decision (the producer wiring choices: proto-free `router.HashPolicy` descriptor parsed at the hcm boundary; the additive `cluster.HashHeaderValues`; the uniform ctx-carried remote addr; the 4 dial-site rebinds; the unit-level reject roster) + §Consequences (byte-stable exported surface +1 additive symbol; NO seam/manager/stat change; the conformance-path-touch caveat). Tail ADR-0236 → **ADR-0237**.

- [ ] **Step 3: STATE.md / ROADMAP.md** — advance per ADR-0106 (flat family row, NO parent rollup): active-phase → `phase 36.2 (load-balancer-ring-hash-http) done`; ROADMAP row 36 → `36.1 done, 36.2 done` (the Load-balancing family STAYS OPEN — 5 candidates remain: maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds). Update the counts block: stat surface **1119** (unchanged); fixtures **64**; fuzzers **42**; BackendKind tail **33**; DECISIONS tail **ADR-0237**.

- [ ] **Step 4: Final six-gate evidence** recorded in PROGRESS.md (surface 1119 unchanged, fixtures 64, tidy-diff empty). Verify the counts one last time:

```bash
ls -d test/fixtures/[0-9]* | wc -l   # 64
grep -c '^### ADR-' docs/envoy-go/DECISIONS.md   # tail ADR-0237
```

- [ ] **Step 5: Commit** the completion bundle (ADR-0052 atomic landing — the doc bundle + the final PROGRESS in ONE commit).

```bash
git add docs/envoy-go/
git commit -m "phase 36.2 IMPL Task 8: completion bundle — BEHAVIOR_CONTRACT + ADR-0237 + STATE/ROADMAP row 36.2 done (fixtures 64, surface 1119, ADR-0237)"
```

The controller then squash-merges the 8 task commits to master + pushes (per `feedback_subagents_no_push` + `feedback_push_to_origin`); the next-prompt roll-forward routes the next cold-start past 36.2 (the Load-balancing family's next candidate, or the next ROADMAP family).

---

## Execution notes (honor throughout)

- **TDD per task** (`superpowers:test-driven-development`): failing test → run-fail → minimal impl → run-pass → commit.
- **Subagent-driven** (`feedback_execution_style`): fresh subagent per task, two-stage (spec + code-quality) review between tasks; work in a worktree (`feedback_git_worktrees`); subagents commit LOCAL-ONLY (`feedback_subagents_no_push`) + the controller pushes at stage-close (`feedback_push_to_origin`).
- **Per-task `gofmt -l` + `golangci-lint` on touched pkgs** (`feedback_pertask_gofmt_lint`) — not just `go vet`.
- **Worktree hygiene** (`feedback_subagent_worktree_detach` + `_path_targeting`): pin the canonical worktree paths; dispatch a GIT HYGIENE block (`git restore`, no `checkout <sha>`/`amend`) for the Task-6 breaks; the controller re-verifies the branch + the clean main repo after each task.
- **Differential discipline**: `-count=1` after any production break (`reference_differential_break_protocol_count1`); `-run 'TestDifferential/0062-…'` not bare `0062` (`reference_differential_run_selector`); `StatsAsserter` for cross-side stats (`reference_differential_asserter_dispatch`); the affinity leg is DETERMINISTIC (not a σ-band — `reference_differential_band_sigma_margin` governs RNG bands only); Docker bridge network + decode-verify (`reference_docker_probe_bridge_network`).
- **ADR cadence**: ADR-0237 body lands IN-PLACE at this IMPL (ADR-0044); the atomic six-gate landing (ADR-0052); the flat family row (ADR-0106); byte-stable parse-reject wording (ADR-0080); the contrib reference image (ADR-0227).
