# Phase 55 PLAN — the plain-statsd `tcp_cluster_name` transport

> **For agentic workers:** REQUIRED SUB-SKILL: use `superpowers:subagent-driven-development` to implement this plan task-by-task in a FRESH worktree. Subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller verifies each commit, re-runs gates on the frozen HEAD, performs the deliberate-break/liveness verification ITSELF, and squashes + pushes at stage-close.

> **Lifecycle stage:** PLAN (lifecycle-state 2 → 3). Docs-only. Worktree `.worktrees/phase-55-plan`, branch `phase-55-stats-sink-statsd-tcp-plan`. Row 55 STAYS `in-progress`.

**Goal:** Lift the phase-48 strict-reject of `StatsdSink.statsd_specifier.tcp_cluster_name` into a genuine accept-and-honor path: a `TCPStatsdSink` that emits the unchanged phase-48 statsd line protocol over one long-lived, newline-delimited TCP connection obtained from a named cluster via a new unaccounted `Cluster.DialSink`.

**Architecture:** A bounded-channel + writer-goroutine sink (the `MetricsServiceSink`/ADR-0262 shape) — `Submit` never blocks the `Flusher`; a single writer goroutine owns the dial, the `pending []byte` buffer, and the write. `delta.apply` runs in the writer (ADR-0263). On a write error the writer retains the unwritten suffix **realigned to the last complete line boundary ≤ n**, so complete lines that landed are never re-sent and the one straddling line — which the dead connection's stream parser discards at EOF — is re-sent whole. `internal/statssink` does NOT import `internal/cluster`; the dial is a `func(context.Context) (net.Conn, error)` seam that `main.go` closes over.

**Tech Stack:** Go 1.23.0 · `dto "github.com/prometheus/client_model/go"` · `envoyproxy/go-control-plane` v1.37.0 protos · reference `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227).

---

## Global Constraints

Every task's requirements implicitly include this section.

- **The SPEC is authoritative.** `docs/envoy-go/phases/55-stats-sink-statsd-tcp/SPEC.md`. Where the BRAINSTORM disagrees (Q2, Q3, Q4 rationale), the SPEC wins.
- **Wire format is adopted verbatim from the reference** (`reference_wire_format_both_sides_see_same_bytes`). Every line, **including the last of a flush**, is `\n`-**TERMINATED**, never `\n`-separated. No write ever contains `\n\n`.
- **Zero stat-surface delta.** 1200 → 1200. `DialSink` increments NOTHING. No sink self-stats. Any new `stats.NewCounter*` call in this phase is a plan violation.
- **Byte-stable rejects** (ADR-0080). Every reject is a `fmt.Errorf` beginning `"bootstrap: "`, with a `stats_sinks[%d]: ` segment. Tests assert by **substring** (`errSubs []string`), the established `bootstrap_test.go:2111-2123` pattern.
- **`internal/statssink` must NOT import `internal/cluster`.** Enforce with an explicit grep in the Task-5 gate.
- **No new packages, no new go.mod modules.** `go mod tidy -diff` must stay empty.
- **Counts at IMPL exit:** stat surface **1200 (+0)** · fixtures **99 → 100** · fuzzers **52 (+0)** · `BackendKind` tail **38 (+0)** · DECISIONS tail **ADR-0272** (next-free ADR-0273). Reconcile the documented fuzzer count against `grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l` per `reference_fuzzer_count_docs_drift`.
- **Per-task gate** (`feedback_pertask_gofmt_lint`), run on every touched package, not just `go vet`:
  ```sh
  gofmt -l <touched dirs>            # must print NOTHING
  golangci-lint run <touched pkgs>
  go vet <touched pkgs>
  go build ./...
  go test <touched pkgs>             # add -count=1 whenever a deliberate break was just reverted
  ```
- **`-race` obligation** (`reference_full_suite_race_after_background_mutator`). `TCPStatsdSink` is the statsd sinks' FIRST background mutator. A `-run`-subset `-race` will MISS the class of failure it introduces. The merge gate runs a **FULL-package** `-race` on `internal/statssink`, `internal/cluster`, and `test/differential`.
- **Differential `-run` selector** (`reference_differential_run_selector`): subtests are `TestDifferential/<fixture>`. `go test -run '0098'` matches ZERO subtests and reports a vacuous PASS. Always `-run 'TestDifferential/0098-stats-sink-statsd-tcp'`.
- **Deliberate breaks always run with `-count=1`** (`reference_differential_break_protocol_count1`) — Go's test cache serves a stale PASS otherwise.

---

## D-question resolutions (SPEC §12) — decided here, with evidence

All five are resolved against the as-built source, not argued from prose. Line numbers were re-verified against master tip `708c0dcc`.

### D-TCP-CLOSE (LOAD-BEARING) → **mutex-guarded `conn` field + `cancel()` for the dial + a per-write `SetWriteDeadline`**

`cancel()` cannot interrupt a blocked `net.Conn.Write` — that is precisely where the `MetricsServiceSink` precedent (a context-bound gRPC stream) stops transferring. Three candidates were considered:

| Candidate | Verdict |
|---|---|
| `atomic.Pointer[net.Conn]` | Rejected. `net.Conn` is an *interface*; `atomic.Pointer[net.Conn]` stores a pointer-to-interface-value, which is awkward, and it still needs a separate `closed` flag to stop the writer redialing after `Close`. Two atomics where one mutex suffices. |
| ctx-watching closer goroutine | Rejected. A third goroutine whose only job is `<-ctx.Done(); conn.Close()` needs the same guarded `conn` field anyway, and adds a lifetime to reason about. |
| **`sync.Mutex` guarding `{conn, closed}`** | **CHOSEN.** |

The mutex guards the *field*, never the I/O: the writer copies the conn out under the lock and does the blocking `Write` **unlocked**. `net.Conn.Close()` is safe to call concurrently with an in-flight `Write` and unwedges it with an error — that is the unwedge mechanism. The `closed` flag, read under the same lock in `redial`, stops the writer from dialing a fresh conn after `Close` (which would leak it). `cancel()` remains, but its job is narrowed to aborting an in-flight `DialContext`.

**This is `-race`-PROVEN at Task 9, not argued** — a `blockingConn` test double whose `Write` parks until `Close` is called, asserted to return from `Close` within `closeDrainGrace + ε` under `-race`.

### D-TCP-CONFIG-SHAPE → **tagged union: add `TCPClusterName string` to `StatsdSinkConfig`**

Rejected: a separate `Bootstrap.StatsdTCPSinkConfigs` slice. The three landed slices (`StatsSinkConfigs` / `StatsdSinkConfigs` / `DogStatsdSinkConfigs`, `bootstrap.go:399/408/417`) are separated because they are three distinct **TypeURLs**. `tcp_cluster_name` and `address` are two arms of **one oneof inside one TypeURL** (`stats.pb.go:571-577`). A second slice would (a) split one TypeURL's parse across two result fields and (b) **destroy the documented "in declaration order" property** that each slice carries, since interleaved UDP/TCP statsd sinks would land in different slices. The tagged union keeps one sink entry ↔ one config element, keeps declaration order, and keeps `FuzzStatsdSinkConfigParse` + `TestStatsdSink_*` on a single struct.

Invariant: **exactly one of `UDPAddress` / `TCPClusterName` is non-empty.** Documented on the struct; `main.go` branches on `TCPClusterName != ""`.

### D-TCP-PENDING-CAP → **`maxPendingBytes = 1 << 20` (1 MiB); `dropOldestLines` drops whole LINES**

*Value.* The reference is unbounded (§11.5: ≥45 snapshots / ~200 KB retained, no cap reached), so there is nothing to mirror. envoy-go's `snapshot()` walks the whole frozen registry (all **1200** stats — unlike the reference's used-only emission, AMEND-TCP-USEDONLY), so one snapshot is ≈ 1200 lines × ~50 B ≈ **60 KB**. 1 MiB therefore retains ≈ **16 snapshots**, the same order as `MetricsServiceSink`'s `defaultChannelCapacity = 8` in-flight batches, and 1 MiB is the tree's existing in-process buffer bound (ADR-0076's retry body buffer). Justified by symmetry with both, not picked arbitrarily.

*Granularity.* **Whole LINES, dropped from the front.** `pending` is a flat `\n`-terminated byte buffer with **no snapshot delimiters** — dropping whole snapshots would require carrying per-flush offsets, new state for no benefit. Dropping whole lines is what the buffer's structure supports and it preserves the load-bearing invariant that **a partial line is never written to the wire**. Overflow costs the dropped lines' counter increments permanently: a documented, deliberate departure from the reference's unbounded buffer (statsd is lossy by design; `MetricsServiceSink` already drops whole batches on a full channel).

### D-TCP-NODE-PLACEMENT → **parse-time, in `parseStatsdSinkConfig`. VERIFIED reachable.**

The SPEC said "verify `result.Proto` is populated at that point before committing to it." **Verified, with one correction the SPEC's phrasing invites:**

- `parseStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error` — `bootstrap.go:562`. Its third parameter `result` is the **`*bootstrap.Bootstrap` wrapper**, whose field `Proto *bootstrapv3.Bootstrap` is declared at `bootstrap.go:351`.
- `Load` builds it at `bootstrap.go:467` as `result := &Bootstrap{Proto: bs, Stats: stats.NewRegistry()}` — i.e. `result.Proto` **is** `bs`, the same pointer, fully unmarshalled *before* `parseStatsSinks(bs, result)` runs at `:471`.
- ⇒ `result.Proto.GetNode().GetId()` and `.GetCluster()` are reachable and populated at the reject site. Both accessors are nil-safe.

**CORRECTION to a misreading the SPEC's shorthand invites:** there is **no `Proto` field on `StatsdSinkConfig`** (`bootstrap.go:297-300` has exactly `UDPAddress` and `Prefix`). `result.Proto` is the *`Bootstrap` wrapper's* field. Do not add a `Proto` field to `StatsdSinkConfig`.

Parse-time placement means the landed `FuzzStatsdSinkConfigParse` and the `TestStatsdSink_Rejects` table cover the new arm for free.

### D-TCP-DIALNAME → **`func (c *Cluster) DialSink(ctx context.Context) (net.Conn, error)`, sharing a NEW private `dialAndTLS`**

*Name.* `DialSink` over `DialUnaccounted`: the latter names the *mechanism* (no accounting) rather than the *policy scope* (stats sinks only), which invites misuse by any caller who wants to dodge the permit. `DialSink`'s doc comment forbids non-sink callers.

*Signature.* `(net.Conn, error)` — two returns, **not** `Dial`'s three. `Dial` surfaces the picked `Endpoint` (`cluster.go:536`: `func (c *Cluster) Dial(ctx context.Context) (net.Conn, Endpoint, error)`) solely so callers can feed `RecordUpstreamResult` for outlier detection. A stats sink does no outlier recording, so the endpoint is dead weight.

*Shared helper.* **Yes.** `dialPicked` (`cluster.go:576`) already carries an `ownPermit bool` whose doc says it "leaves room for a future permit-less caller without forking the body" — but as written it *unconditionally* does `c.upstreamCxTotal.Inc()` / `c.upstreamCxActive.Inc()` (`:600-601`) and wraps in `connWithGauge{dec: c.connDec(release)}` (whose `dec` Decs the gauge **and**, on a pooled cluster, calls `pool.releaseConn()` — underflowing a permit `DialSink` never took). So `ownPermit=false` is **not** sufficient. Task 1 extracts the accounting-free core — TCP-dial under `connect_timeout`, then TLS handshake under `ctx` — into `dialAndTLS(ctx, ep) (net.Conn, error)`, called by **both** `dialPicked` and `DialSink`. The extraction is a pure refactor: the two error strings (`"cluster: dial: %w"`, `"cluster: tls: handshake: %w"`) are preserved byte-for-byte.

`DialSink` uses `PickEndpoint()` (`cluster.go:506`), which **releases the pick immediately** — its documented contract — so there is no release closure to hold and the returned conn is **bare** (no `connWithGauge` wrap: there is no gauge to Dec and no permit to release). `least_request` load-invisibility is the accepted, already-documented cost of every `PickEndpoint` caller.

---

## FINAL ADR-0045 split-gate re-check

The SPEC re-armed the escape valve: *"if your decomposition exceeds ~14 tasks, revisit the ADR-0045 split gate."*

**This decomposition is exactly 14 tasks. The gate is touched, not exceeded. NO SPLIT.** Recorded reasoning:

- Still **one** subsystem (the statsd sink transport), **one** new production file (`internal/statssink/statsd_tcp.go`), **one** ADR (0272), **one** fixture (`0098`).
- The two candidate splits the BRAINSTORM already rejected remain rejected for the same reasons: a `transport`/`differential` split would strand a lifted strict-reject with no differential proof (exactly what ADR-0080 exists to prevent); a `sink`/`reconnect-hardening` split would ship a sink that mishandles a peer restart to defer ~15 LoC.
- Tasks 12–14 are the standard landing bundle (breaks, ADR + contract, six-gate), present in every phase; the *mechanism* budget is Tasks 1–11.

**Escape valve:** if, during IMPL, Task 7 (line-aligned resume) or Task 9 (`-race` `Close`) forces a task to split in two — pushing the total past 15 — STOP and re-open the split gate as `55.1` (transport + rejects) / `55.2` (fixture + breaks) before proceeding.

---

## File structure

| File | Disposition |
|---|---|
| `internal/cluster/cluster.go` | MODIFY — extract `dialAndTLS`; add `DialSink` (Task 1) |
| `internal/cluster/cluster_test.go` | MODIFY — `TestDialSink_*` (Task 1) |
| `test/helpers/statsdrecv/statsdrecv.go` | MODIFY — extract `ingestLine`; add TCP listener, `ConnCount()`, `UnparsedCount()` (Task 2) |
| `test/helpers/statsdrecv/statsdrecv_test.go` | MODIFY — TCP + split-read tests (Task 2) |
| `internal/bootstrap/bootstrap.go` | MODIFY — `TCPClusterName` field; lift reject; node + unknown-cluster rejects (Tasks 3, 4) |
| `internal/bootstrap/bootstrap_test.go` | MODIFY — flip the `tcp_cluster_name` reject case; add accept + new reject cases (Tasks 3, 4) |
| `internal/bootstrap/statsd_fuzz_test.go` | MODIFY — reseed (Tasks 3, 4) |
| `internal/bootstrap/statssink_fuzz_test.go` | MODIFY — reseed (Task 3) |
| **`internal/statssink/statsd_tcp.go`** | **CREATE** — `TCPStatsdSink` (Tasks 5–9) |
| **`internal/statssink/statsd_tcp_test.go`** | **CREATE** — unit tests incl. the `-race` `Close` proof (Tasks 5–9) |
| `cmd/envoy-go/main.go` | MODIFY — the TCP build arm (Task 10) |
| **`test/fixtures/0098-stats-sink-statsd-tcp/`** | **CREATE** — `envoy.yaml`, `envoy-go.yaml`, `README.md`, `driver/driver.go` (Task 11) |
| `test/differential/runner_test.go` | MODIFY — one blank-import line (Task 11) |
| `docs/envoy-go/DECISIONS.md` | MODIFY — ADR-0272 (Task 13) |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY — the phase-55 delta (Task 13) |
| `docs/envoy-go/{ROADMAP,STATE}.md` | MODIFY — row 55 → `done` (Task 14) |

**No new package. No new go.mod module. No new `BackendKind`. No new fuzzer.**

---

## Three traps (from the SPEC + the memory bank) — do not walk into these

1. **`statsdrecv` is a DATAGRAM parser and the stream hazard is MEASURED, not hypothetical.** `ingest` (`statsdrecv.go:131`) splits a UDP datagram on `\n`, where every line is complete *by construction*. The probe's ~200 KB post-reconnect write arrived in `recv()` chunks capped at 65536 bytes, **splitting lines mid-token**. Task 2 must read line-at-a-time through `bufio` with a carried remainder and **DISCARD an incomplete trailing line at EOF** — §3.5's no-duplication argument *depends* on that discard. Per `reference_line_parser_extension_delimiter_reuse`, Task 2 traces one concrete split-read byte-for-byte.
2. **The `bootstrap.go:556-561` ordering constraint survives the lift with INVERTED meaning.** `GetAddress()` returns nil for BOTH a missing oneof AND a `tcp_cluster_name` arm, which is why the reject ran FIRST. After the lift the `tcp_cluster_name` arm must be **DISPATCHED** before the nil-`address` reject fires, or a valid TCP config is rejected as "missing `statsd_specifier`". Task 3 pins this with a dedicated regression test.
3. **`TCPStatsdSink` is the statsd sinks' FIRST background mutator.** A `-run`-subset `-race` MISSES the class of failure it introduces. Task 14's merge gate runs a **FULL-package** `-race` on `internal/statssink`, `internal/cluster`, AND `test/differential`.

---

## Task 1: `Cluster.dialAndTLS` extraction + `Cluster.DialSink`

**Files:**
- Modify: `internal/cluster/cluster.go:576-607` (extract `dialAndTLS` out of `dialPicked`), and add `DialSink` after `Dial` (`:559`)
- Test: `internal/cluster/cluster_test.go` (append)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func (c *Cluster) DialSink(ctx context.Context) (net.Conn, error)` — consumed by Task 10's `main.go` dial closure. Private: `func (c *Cluster) dialAndTLS(ctx context.Context, ep Endpoint) (net.Conn, error)`.

**Why this is the first task:** it is the only production surface added outside `internal/statssink`, it is independently testable, and Task 5's dial seam is typed against its signature.

- [ ] **Step 1: Write the failing tests**

Append to `internal/cluster/cluster_test.go`. These reuse the existing helpers `mkTestCluster` (`:46`), `listenTCP` (`:71`), `listenTLS` (`:91`), `endpointFromAddr` (`:110`), and `attachConnPool` (`connpool_test.go:378`).

```go
// TestDialSink_NoCxAccounting pins AMEND-TCP-CXSTATS: the reference's statsd TCP
// connection reports upstream_cx_total: 0 / upstream_cx_active: 0. DialSink must
// leave BOTH counters untouched, before and after the conn's Close.
func TestDialSink_NoCxAccounting(t *testing.T) {
	ln := listenTCP(t)
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(ln.Addr()))

	conn, err := c.DialSink(context.Background())
	if err != nil {
		t.Fatalf("DialSink: %v", err)
	}
	if got := c.upstreamCxTotal.Load(); got != 0 {
		t.Errorf("upstream_cx_total after DialSink = %d, want 0", got)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("upstream_cx_active after DialSink = %d, want 0", got)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("upstream_cx_active after Close = %d, want 0 (never Inc'd, must not go negative)", got)
	}
	if got := c.upstreamCxTotal.Load(); got != 0 {
		t.Errorf("upstream_cx_total after Close = %d, want 0", got)
	}
}

// TestDialSink_TakesNoConnPermit is the decisive AMEND-TCP-CXSTATS test. The probe
// showed the reference STILL connects and flushes with max_connections: 0 on the
// stats cluster. envoy-go's connPool returns errConnPoolOverflow on the FIRST
// acquire at maxConnections=0/maxPending=0 (connpool_test.go:295 "(f)"), so Dial
// FAILS on such a cluster while DialSink MUST succeed. This test would pass
// vacuously if DialSink merely skipped the Inc but still took the permit.
func TestDialSink_TakesNoConnPermit(t *testing.T) {
	ln := listenTCP(t)
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(ln.Addr()))
	attachConnPool(c, 0, 0) // max_connections: 0, max_pending_requests: 0

	// Control: Dial must be refused by the permit.
	if _, _, err := c.Dial(context.Background()); !errors.Is(err, errConnPoolOverflow) {
		t.Fatalf("Dial with max_connections=0: got %v, want errConnPoolOverflow", err)
	}
	// DialSink bypasses the permit entirely.
	conn, err := c.DialSink(context.Background())
	if err != nil {
		t.Fatalf("DialSink with max_connections=0: %v (must bypass the permit)", err)
	}
	_ = conn.Close()
}

// TestDialSink_ReturnsBareConn: no connWithGauge wrapper — there is no gauge to
// Dec and no permit to release, so wrapping would be a lie (and connDec would
// underflow the pool).
func TestDialSink_ReturnsBareConn(t *testing.T) {
	ln := listenTCP(t)
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(ln.Addr()))
	conn, err := c.DialSink(context.Background())
	if err != nil {
		t.Fatalf("DialSink: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, wrapped := conn.(*connWithGauge); wrapped {
		t.Fatal("DialSink returned a *connWithGauge; want the bare net.Conn")
	}
}

// TestDialSink_TLS: DialSink honors upstream TLS, like Dial.
func TestDialSink_TLS(t *testing.T) {
	srvCfg, cliCfg := tlsPairForTest(t) // see Step 3 note
	ln := listenTLS(t, srvCfg)
	c := mkTestCluster("c_statsd", cliCfg, endpointFromAddr(ln.Addr()))
	conn, err := c.DialSink(context.Background())
	if err != nil {
		t.Fatalf("DialSink over TLS: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, ok := conn.(*stdtls.Conn); !ok {
		t.Fatalf("DialSink over TLS returned %T, want *tls.Conn", conn)
	}
}

// TestDialSink_CtxCanceled: a canceled ctx short-circuits before the pick.
func TestDialSink_CtxCanceled(t *testing.T) {
	ln := listenTCP(t)
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(ln.Addr()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.DialSink(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("DialSink with canceled ctx: got %v, want context.Canceled", err)
	}
}

// TestDialSink_DialFailure surfaces the same wrapped error string as Dial.
func TestDialSink_DialFailure(t *testing.T) {
	ln := listenTCP(t)
	// endpointFromAddr takes a net.Addr (cluster_test.go:110), NOT a string.
	// The net.Addr VALUE stays usable after the listener closes.
	addr := ln.Addr()
	_ = ln.Close() // nothing is listening now
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(addr))
	_, err := c.DialSink(context.Background())
	if err == nil {
		t.Fatal("DialSink to a closed port: want error, got nil")
	}
	if !strings.Contains(err.Error(), "cluster: dial: ") {
		t.Errorf("error %q should carry the byte-stable prefix %q", err, "cluster: dial: ")
	}
}
```

> **Note on `tlsPairForTest`:** the existing `TestCluster_Dial_TLS` (`cluster_test.go:150`) already builds a server/client `*stdtls.Config` pair inline with `ServerName: "alpha.envoy-go.test"`. **Read `cluster_test.go:150-217` and reuse whatever it uses.** If it builds the pair inline rather than through a helper, extract that block into `tlsPairForTest(t *testing.T) (srv, cli *stdtls.Config)` and have `TestCluster_Dial_TLS` call it too — do NOT duplicate the cert-generation code (DRY).

- [ ] **Step 2: Run the tests to verify they fail**

```sh
go test ./internal/cluster/ -run 'TestDialSink' -count=1 -v
```
Expected: FAIL — `c.DialSink undefined (type *Cluster has no field or method DialSink)`.

- [ ] **Step 3: Extract `dialAndTLS` (pure refactor — no behavior change)**

Replace the body of `dialPicked` (`cluster.go:576-607`). The two `fmt.Errorf` strings move verbatim; nothing else about `dialPicked` changes.

```go
// dialAndTLS is the accounting-FREE dial core shared by dialPicked and DialSink:
// TCP-dial to ep bounded by connect_timeout, then the upstream TLS handshake
// bounded by ctx. It touches NO counter, takes NO permit, and holds NO LB
// release — every one of those is the caller's business. Extracted at phase 55
// so DialSink (the unaccounted stats-sink dial, AMEND-TCP-CXSTATS) cannot drift
// from Dial's dial/TLS semantics.
func (c *Cluster) dialAndTLS(ctx context.Context, ep Endpoint) (net.Conn, error) {
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", ep.Addr())
	if err != nil {
		return nil, fmt.Errorf("cluster: dial: %w", err)
	}
	if c.upstreamCfg == nil {
		return raw, nil
	}
	conn := stdtls.Client(raw, c.upstreamCfg)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("cluster: tls: handshake: %w", err)
	}
	return conn, nil
}

func (c *Cluster) dialPicked(ctx context.Context, ep Endpoint, release func(), ownPermit bool) (net.Conn, Endpoint, error) {
	final, err := c.dialAndTLS(ctx, ep)
	if err != nil {
		if ownPermit {
			c.releaseConnSlot()
		}
		release()
		return nil, ep, err
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	// Compose release (+ pool permit release for pooled clusters) into the
	// existing connWithGauge dec closure. The connWithGauge sync.Once guards the
	// gauge Dec, the LB release AND the pool release, so double-Close cannot
	// double-release. The struct is unchanged.
	return &connWithGauge{Conn: final, dec: c.connDec(release)}, ep, nil
}
```

Also update `dialPicked`'s doc comment (`cluster.go:561-575`): its last sentence currently reads *"the flag documents the single axis of variation and leaves room for a future permit-less caller without forking the body."* Replace that sentence with:

```
// ownPermit remains the single axis of variation for the ACCOUNTED callers
// (Dial, dialPooledH2To — both pass true). The unaccounted stats-sink caller
// (DialSink) does NOT route through here: it needs no counter Inc and no
// connWithGauge wrap, so it shares only the dialAndTLS core.
```

- [ ] **Step 4: Add `DialSink`** (insert after `Dial`, i.e. after `cluster.go:559`)

```go
// DialSink dials one endpoint of c for a STATS SINK and returns the bare conn.
//
// Unlike Dial it takes NO max_connections permit (ADR-0252) and increments NO
// upstream_cx_* counter — mirroring the reference, whose statsd TCP connection
// bypasses conn-pool accounting entirely: with max_connections: 0 on the stats
// cluster it still connects and still flushes, and reports upstream_cx_total: 0
// / upstream_cx_active: 0 (SPEC-55 §11.4, AMEND-TCP-CXSTATS). It DOES honor the
// LB pick, connect_timeout, and upstream TLS.
//
// The pick is released IMMEDIATELY (PickEndpoint's contract), so the sink's
// long-lived connection is invisible to least_request — the accepted, already-
// documented cost of every PickEndpoint caller.
//
// STATS SINKS ONLY. Any data-plane caller wanting a connection MUST use Dial:
// bypassing the permit and the gauges anywhere else silently breaks circuit
// breaking and upstream_cx_active.
func (c *Cluster) DialSink(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ep, err := c.PickEndpoint()
	if err != nil {
		return nil, err
	}
	return c.dialAndTLS(ctx, ep)
}
```

- [ ] **Step 5: Run the tests to verify they pass, and that the refactor broke nothing**

```sh
go test ./internal/cluster/ -run 'TestDialSink' -count=1 -v          # all PASS
go test ./internal/cluster/ -count=1                                  # FULL package: every existing Dial/AcquireH1/H2-pool test still passes
go test ./internal/cluster/ -count=1 -race                            # full-package -race
```
Expected: PASS. In particular `TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose` (`:344`), `TestDial_CloseIdempotent` (`:390`), `TestDial_ReleasesOnDialFailure` (`:577`) must be **unchanged and green** — they are the guard that `dialAndTLS` was a pure extraction.

- [ ] **Step 6: Per-task gate**

```sh
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/... && go vet ./internal/cluster/... && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add internal/cluster/cluster.go internal/cluster/cluster_test.go
git commit -m "phase 55 Task 1: Cluster.DialSink — the unaccounted stats-sink dial (AMEND-TCP-CXSTATS)

Extract the accounting-free dial/TLS core out of dialPicked into dialAndTLS
(pure refactor; both fmt.Errorf strings preserved byte-for-byte) and add
DialSink: LB pick + connect_timeout + upstream TLS, NO max_connections permit,
NO upstream_cx_* Inc, NO connWithGauge wrap.

TestDialSink_TakesNoConnPermit pins the decisive property against a
max_connections:0 cluster, where Dial returns errConnPoolOverflow."
```

---

## Task 2: `statsdrecv` — TCP stream listener, `ConnCount()`, `UnparsedCount()`

**Files:**
- Modify: `test/helpers/statsdrecv/statsdrecv.go` (`Server` struct `:41-54`; `NewAtAddr` `:86-106`; `ingest` `:131-193`; `Reset` `:277-287`; `Addr` `:291-293`; `Close` `:298-300`)
- Test: `test/helpers/statsdrecv/statsdrecv_test.go` (append)

**Interfaces:**
- Consumes: nothing.
- Produces, all consumed by Task 11's driver:
  - `func NewTCPAtAddr(addr string) (*Server, error)`
  - `func (s *Server) ConnCount() int`
  - `func (s *Server) UnparsedCount() int`
  - existing `DeltaSum`, `Gauge`, `SeenCount`, `Addr`, `Close` keep their signatures and now work for both transports.

### Why `UnparsedCount()` exists — the vacuous-break defense

SPEC §8.2 break **(b)** is "emit `\n`-SEPARATED instead of `\n`-TERMINATED → the counter-subset lookups miss." **That assertion is not reliably live.** With no trailing terminator, the last line of flush *N* concatenates with the first line of flush *N+1* into one line, e.g.

```
sdpfx.x.y:7|csdpfx.a.b:1|g
```

The receiver's parser (`ingest`, first-pipe-then-colon) reads `head = "sdpfx.x.y:7"`, parses name and value **successfully**, then takes `typ = "csdpfx.a.b:1"` → falls into `default: continue`. So exactly **two** lines are lost per flush boundary: the last of flush *N* and the first of flush *N+1*. `snapshot()` walks the registry in a stable order, so those are the *same two names every flush* — and **if neither is one of the three `subsetNames`, break (b) passes vacuously.** That is the `reference_differential_asserter_dispatch` failure class this project has been bitten by before.

`UnparsedCount()` makes the break **deterministic and order-independent**: it counts every line the receiver could not account for — a structural parse failure *or* an unknown metric type. envoy-go emits only `|c` and `|g` (no histograms, ADR-0060), so the **subject's** count is exactly `0` under correct framing, and `> 0` under break (b), regardless of registry order. It is a **subject-exact** assertion: the reference emits `|ms` timer lines (35 of them, §11.3), which are legitimately unaccounted, so its count is RECORDED, never asserted.

- [ ] **Step 1: Write the failing tests**

Append to `test/helpers/statsdrecv/statsdrecv_test.go`.

```go
// dialAndWrite opens a TCP conn to srv and writes b in one Write.
func dialAndWrite(t *testing.T, srv *statsdrecv.Server, b []byte) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := c.Write(b); err != nil {
		t.Fatalf("write: %v", err)
	}
	return c
}

// waitSeen polls until SeenCount(name) >= want, or fails after 2s.
func waitSeen(t *testing.T, srv *statsdrecv.Server, name string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for srv.SeenCount(name) < want {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: SeenCount(%q) = %d, want >= %d", name, srv.SeenCount(name), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTCPBasicCounterAndGauge(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	c := dialAndWrite(t, srv, []byte("a.b:3|c\na.b:4|c\ng.h:9|g\n"))
	defer func() { _ = c.Close() }()

	waitSeen(t, srv, "a.b", 2)
	waitSeen(t, srv, "g.h", 1)

	if sum, ok := srv.DeltaSum("a.b"); !ok || sum != 7 {
		t.Errorf("DeltaSum(a.b) = %v,%v; want 7,true", sum, ok)
	}
	if v, ok := srv.Gauge("g.h"); !ok || v != 9 {
		t.Errorf("Gauge(g.h) = %v,%v; want 9,true", v, ok)
	}
	if n := srv.UnparsedCount(); n != 0 {
		t.Errorf("UnparsedCount = %d, want 0", n)
	}
}

// TestTCPConnCount: one long-lived conn ⇒ ConnCount()==1; a second dial ⇒ 2.
func TestTCPConnCount(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	c1 := dialAndWrite(t, srv, []byte("x.y:1|c\n"))
	defer func() { _ = c1.Close() }()
	waitSeen(t, srv, "x.y", 1)
	if n := srv.ConnCount(); n != 1 {
		t.Fatalf("ConnCount after 1 dial = %d, want 1", n)
	}

	c2 := dialAndWrite(t, srv, []byte("x.y:1|c\n"))
	defer func() { _ = c2.Close() }()
	waitSeen(t, srv, "x.y", 2)
	if n := srv.ConnCount(); n != 2 {
		t.Fatalf("ConnCount after 2 dials = %d, want 2", n)
	}
}

// TestTCPSplitReadMidToken is THE trap-1 test. The probe measured a ~200 KB
// post-reconnect write arriving in <=65536-byte recv() chunks that split lines
// MID-TOKEN. Here the split is forced and traced byte-for-byte.
//
// The full logical stream is:
//
//	"sdpfx.cluster.c_backend.upstream_rq_total:7|c\nsdpfx.server.live:1|g\n"
//
// Chunk 1 is the first 50 bytes:
//
//	"sdpfx.cluster.c_backend.upstream_rq_total:7|c\nsdpfx"
//
// i.e. a COMPLETE first line (44 bytes + '\n' at index 44) followed by the
// 5-byte fragment "sdpfx" — the second line split mid-NAME, before its ':'.
// A datagram parser would see "sdpfx" as a whole line, fail to find ':', and
// drop it. A stream parser MUST carry "sdpfx" as a remainder and only emit the
// second line once chunk 2 supplies ".server.live:1|g\n".
func TestTCPSplitReadMidToken(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	const line1 = "sdpfx.cluster.c_backend.upstream_rq_total:7|c\n"
	const line2 = "sdpfx.server.live:1|g\n"
	full := line1 + line2
	const split = 50 // len(line1) == 45; 50 lands 5 bytes into line2's name

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte(full[:split])); err != nil {
		t.Fatalf("write chunk 1: %v", err)
	}
	waitSeen(t, srv, "sdpfx.cluster.c_backend.upstream_rq_total", 1)
	// The remainder "sdpfx" must NOT have been ingested as a line.
	if n := srv.UnparsedCount(); n != 0 {
		t.Fatalf("UnparsedCount after the mid-token chunk = %d, want 0 "+
			"(the %q remainder must be carried, not parsed)", n, full[len(line1):split])
	}
	if _, ok := srv.Gauge("sdpfx.server.live"); ok {
		t.Fatal("sdpfx.server.live must not be visible before its line completes")
	}

	if _, err := conn.Write([]byte(full[split:])); err != nil {
		t.Fatalf("write chunk 2: %v", err)
	}
	waitSeen(t, srv, "sdpfx.server.live", 1)
	if v, ok := srv.Gauge("sdpfx.server.live"); !ok || v != 1 {
		t.Errorf("Gauge(sdpfx.server.live) = %v,%v; want 1,true", v, ok)
	}
	if n := srv.UnparsedCount(); n != 0 {
		t.Errorf("UnparsedCount = %d, want 0", n)
	}
}

// TestTCPDiscardsIncompleteTrailingLineAtEOF pins the property that SPEC §3.5's
// no-DUPLICATION argument depends on: a connection that dies mid-line must
// DISCARD the straddling line, so the sink can safely re-send it whole.
func TestTCPDiscardsIncompleteTrailingLineAtEOF(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	conn := dialAndWrite(t, srv, []byte("done.line:5|c\npartial.li"))
	waitSeen(t, srv, "done.line", 1)
	_ = conn.Close() // EOF with "partial.li" buffered and unterminated

	time.Sleep(100 * time.Millisecond) // let the stream goroutine observe EOF
	if _, ok := srv.DeltaSum("partial.li"); ok {
		t.Fatal("the incomplete trailing line was ingested; it MUST be discarded at EOF")
	}
	if n := srv.UnparsedCount(); n != 0 {
		t.Errorf("UnparsedCount = %d, want 0 (a discarded partial line is not an unparsed line)", n)
	}
	if sum, ok := srv.DeltaSum("done.line"); !ok || sum != 5 {
		t.Errorf("DeltaSum(done.line) = %v,%v; want 5,true", sum, ok)
	}
}

// TestTCPUnparsedCountCatchesConcatenatedLines is the LIVENESS proof for the
// differential's break (b): \n-SEPARATED framing concatenates the last line of
// one flush with the first of the next, producing an unknown metric type.
func TestTCPUnparsedCountCatchesConcatenatedLines(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	// "a.b:7|c" with NO terminator, immediately followed by "c.d:1|g\n".
	conn := dialAndWrite(t, srv, []byte("a.b:7|cc.d:1|g\n"))
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(2 * time.Second)
	for srv.UnparsedCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("UnparsedCount stayed 0; the concatenated line was not detected")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := srv.DeltaSum("a.b"); ok {
		t.Error("a.b must NOT be accounted: its type token is the corrupted \"cc.d:1\"")
	}
}

// TestUDPPathUnchanged: the datagram accessors keep working; the TCP path leaves
// them unpopulated (documented in SPEC §3.9, not silently divergent).
func TestUDPPathUnchanged(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()
	c, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Write([]byte("u.v:2|c\nw.x:3|c\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitSeen(t, srv, "u.v", 1)
	if got := srv.MaxLinesInAnyDatagram(); got != 2 {
		t.Errorf("MaxLinesInAnyDatagram = %d, want 2", got)
	}
	if n, ok := srv.LinesInDatagram("u.v"); !ok || n != 2 {
		t.Errorf("LinesInDatagram(u.v) = %d,%v; want 2,true", n, ok)
	}
	if got := srv.ConnCount(); got != 0 {
		t.Errorf("ConnCount on a UDP receiver = %d, want 0 (connectionless)", got)
	}
}

func TestTCPLeavesDatagramAccessorsUnpopulated(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()
	c := dialAndWrite(t, srv, []byte("p.q:1|c\nr.s:2|c\n"))
	defer func() { _ = c.Close() }()
	waitSeen(t, srv, "r.s", 1)
	if got := srv.MaxLinesInAnyDatagram(); got != 0 {
		t.Errorf("MaxLinesInAnyDatagram on a TCP receiver = %d, want 0 (a stream has no datagrams)", got)
	}
	if _, ok := srv.LinesInDatagram("p.q"); ok {
		t.Error("LinesInDatagram must be unpopulated on the TCP path")
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```sh
go test ./test/helpers/statsdrecv/ -count=1
```
Expected: FAIL — `undefined: statsdrecv.NewTCPAtAddr`, `srv.ConnCount undefined`, `srv.UnparsedCount undefined`.

- [ ] **Step 3: Extract `ingestLine` from `ingest` (behavior-preserving)**

`ingest` currently does the per-line parse **and** the datagram bookkeeping (`maxLinesInDatagram`, `linesInDatagram`) inside one loop. Split them. `ingestLine` assumes `s.mu` is **already held** and returns the accounted name.

Replace `statsdrecv.go:131-193` with:

```go
// ingestLine parses ONE complete statsd/DogStatsd line
// (<name>:<value>|<type>[|#tag1:val1,...]) and updates the value accumulators.
// It assumes s.mu is HELD by the caller. It returns (name, true) when the line
// was accounted, and ("", false) when it was not — either a structural parse
// failure or an unknown metric type; both bump unparsed (except a blank line,
// which is neither).
//
// The first-pipe-then-colon split (phase 49): neither name nor value contains
// '|', so the FIRST '|' unambiguously separates "name:value" from "type[|#tags]"
// — a tagged line's tag suffix contains its OWN colons, which a last-colon split
// would mis-take for the name/value boundary. This degenerates to the exact
// prior behavior on a tagless line (line-parser-extension delimiter-reuse gotcha).
func (s *Server) ingestLine(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", false // a blank line is not an unparsed line
	}
	pipe1 := strings.IndexByte(line, '|')
	if pipe1 < 0 {
		s.unparsed++
		return "", false
	}
	head := line[:pipe1] // "name:value" — no '|' precedes here
	colon := strings.LastIndexByte(head, ':')
	if colon < 0 {
		s.unparsed++
		return "", false
	}
	name := head[:colon]
	val, err := strconv.ParseFloat(head[colon+1:], 64)
	if err != nil {
		s.unparsed++
		return "", false
	}
	rest := line[pipe1+1:] // "type[|#tag1:val1,...]"
	typ := rest
	var lineTags map[string]string
	if pipe2 := strings.IndexByte(rest, '|'); pipe2 >= 0 {
		typ = rest[:pipe2]
		tagPart := strings.TrimPrefix(rest[pipe2+1:], "#")
		lineTags = make(map[string]string)
		for _, pair := range strings.Split(tagPart, ",") {
			if c := strings.IndexByte(pair, ':'); c >= 0 {
				lineTags[pair[:c]] = pair[c+1:]
			}
		}
	}
	switch typ {
	case "c":
		s.deltaSums[name] += val
		sig := tagSignature(lineTags)
		byTag, ok := s.sumsByTags[name]
		if !ok {
			byTag = make(map[string]float64)
			s.sumsByTags[name] = byTag
		}
		byTag[sig] += val
		s.seen[name]++
	case "g":
		s.gauges[name] = val
		s.seen[name]++
	default:
		// An unknown metric type: the reference's |ms timers (legitimate), or a
		// corrupted type token from \n-SEPARATED framing (the break-(b) signal).
		s.unparsed++
		return "", false
	}
	if lineTags != nil {
		s.tags[name] = lineTags
	}
	return name, true
}

// ingest parses each newline-delimited line in one UDP DATAGRAM, where every
// line is complete BY CONSTRUCTION, and additionally records the datagram
// batching signals. The TCP stream path uses ingestLine directly and leaves
// maxLinesInDatagram / linesInDatagram unpopulated (SPEC-55 §3.9).
func (s *Server) ingest(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for _, line := range lines {
		name, ok := s.ingestLine(line)
		if !ok {
			continue
		}
		if n := len(lines); n > s.maxLinesInDatagram {
			s.maxLinesInDatagram = n
		}
		s.linesInDatagram[name] = len(lines)
	}
}
```

- [ ] **Step 4: Add the TCP fields, constructor, accept/stream loops, and accessors**

Add to the `Server` struct (`:41-54`):

```go
	lis   net.Listener // TCP mode: nil in UDP mode
	conns []net.Conn   // TCP mode: accepted conns, hard-closed by Close

	connCount int // TCP mode: total accepted connections (the long-lived-conn proof)
	unparsed  int // lines the receiver could not account (parse failure or unknown type)
```

New constructor (place next to `NewAtAddr`):

```go
// NewTCPAtAddr binds a TCP listener on the caller-supplied host:port (e.g.
// "0.0.0.0:<port>" so a Docker reference-Envoy can reach the host, or
// "127.0.0.1:0" for an ephemeral loopback port) and starts an accept loop.
// Lifecycle is the caller's responsibility via Close.
//
// Unlike the UDP receiver this is a STREAM parser: a TCP read carries no line
// framing (the reference's ~200 KB post-reconnect write arrived in <=65536-byte
// chunks that split lines mid-token, SPEC-55 §11.5), so each connection is read
// line-at-a-time through bufio with a carried remainder, and an INCOMPLETE
// TRAILING LINE AT EOF IS DISCARDED — the property the sink's line-aligned
// resume relies on to avoid duplication (SPEC-55 §3.5).
func NewTCPAtAddr(addr string) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("statsdrecv: listen tcp %q: %w", addr, err)
	}
	s := &Server{
		lis:             lis,
		deltaSums:       make(map[string]float64),
		sumsByTags:      make(map[string]map[string]float64),
		gauges:          make(map[string]float64),
		seen:            make(map[string]int),
		tags:            make(map[string]map[string]string),
		linesInDatagram: make(map[string]int),
	}
	go s.acceptLoop()
	return s, nil
}

// acceptLoop accepts until the listener is closed, counting every connection and
// serving each on its own stream goroutine.
func (s *Server) acceptLoop() {
	for {
		conn, err := s.lis.Accept()
		if err != nil {
			return // listener closed
		}
		s.mu.Lock()
		s.connCount++
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		go s.streamLoop(conn)
	}
}

// streamLoop reads complete, '\n'-terminated lines. bufio.Reader.ReadString
// accumulates across as many underlying Reads as it takes to find the delimiter,
// so a chunk boundary that lands mid-token is transparently rejoined.
//
// A non-nil error from ReadString means NO delimiter was found: whatever bytes
// it returns are an INCOMPLETE TRAILING LINE, and they are DISCARDED. That is
// exactly what a real statsd server does at EOF, and it is what makes the sink's
// re-send of the straddling line non-duplicating.
func (s *Server) streamLoop(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return // EOF / reset: `line` holds an unterminated remainder — DISCARD it
		}
		s.mu.Lock()
		s.ingestLine(line) // TrimSpace inside strips the '\n' (and any '\r')
		s.mu.Unlock()
	}
}

// ConnCount returns the total number of TCP connections accepted over this
// receiver's lifetime. 0 for a UDP receiver (connectionless). The statsd TCP
// sink must hold ONE long-lived connection, so a correct subject yields 1; a
// per-flush redial yields one per flush.
func (s *Server) ConnCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connCount
}

// UnparsedCount returns the number of ingested lines the receiver could not
// account for: a structural parse failure, or a known-good structure carrying an
// unknown metric type (the reference's |ms timers — legitimate; or a corrupted
// type token produced by \n-SEPARATED rather than \n-TERMINATED framing).
//
// envoy-go emits only |c and |g (no histograms, ADR-0060), so a correct SUBJECT
// yields exactly 0. The reference legitimately yields >0 (|ms), so this is a
// SUBJECT-EXACT signal — never assert it cross-side.
func (s *Server) UnparsedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unparsed
}
```

Update `Addr` (`:291-293`), `Close` (`:298-300`), and `Reset` (`:277-287`):

```go
// Addr returns the receiver's bound address, for either transport.
func (s *Server) Addr() string {
	if s.lis != nil {
		return s.lis.Addr().String()
	}
	return s.conn.LocalAddr().String()
}

// Close stops the receiver. For UDP it closes the socket. For TCP it HARD-closes
// the listener and every accepted connection — never waiting for the peer to
// hang up, since the statsd sink holds its connection open for the process
// lifetime (the metricsservice.Close hard-stop precedent). Idempotent.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		if s.conn != nil {
			_ = s.conn.Close()
		}
		if s.lis != nil {
			_ = s.lis.Close()
		}
		s.mu.Lock()
		conns := s.conns
		s.conns = nil
		s.mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
}
```

In `Reset`, add `s.unparsed = 0` alongside `s.maxLinesInDatagram = 0`. Do **NOT** reset `connCount`: it is a transport fact over the receiver's lifetime, not a value accumulator. Say so in the `Reset` doc comment.

Add `"bufio"` to the import block.

- [ ] **Step 5: Run to verify all pass**

```sh
go test ./test/helpers/statsdrecv/ -count=1 -v
go test ./test/helpers/statsdrecv/ -count=1 -race
```
Expected: PASS, including every pre-existing test (`TestStatsdRecvBasic`, `TestStatsdRecvTagged*`, `TestMaxLinesInAnyDatagram`) — they are the guard that `ingestLine` was a pure extraction.

- [ ] **Step 6: Per-task gate**

```sh
gofmt -l test/helpers/statsdrecv/ && golangci-lint run ./test/helpers/statsdrecv/... && go vet ./test/helpers/statsdrecv/... && go build ./...
```

- [ ] **Step 7: Commit**

```bash
git add test/helpers/statsdrecv/
git commit -m "phase 55 Task 2: statsdrecv gains a TCP STREAM listener, ConnCount, UnparsedCount

Extract ingestLine (assumes mu held) out of the datagram-shaped ingest, then add
NewTCPAtAddr: an accept loop + per-conn bufio stream loop that reads complete
'\n'-terminated lines and DISCARDS an incomplete trailing line at EOF — the
property SPEC §3.5's no-duplication argument depends on.

UnparsedCount is the subject-exact liveness signal for the differential's
break (b): \n-SEPARATED framing concatenates flush boundaries into a corrupted
type token. A plain subset-lookup assertion would have been vacuous whenever the
boundary names fell outside the asserted subset."
```

---

## Task 3: Bootstrap — lift the `tcp_cluster_name` reject; dispatch the arm BEFORE the nil-`address` reject

**Files:**
- Modify: `internal/bootstrap/bootstrap.go` — `StatsdSinkConfig` (`:289-300`), its `StatsdSinkConfigs` field doc (`:399-408`), `parseStatsdSinkConfig` (`:554-579`)
- Modify: `internal/bootstrap/bootstrap_test.go` — the `tcp_cluster_name` reject case (`:2074-2083`)
- Modify: `internal/bootstrap/statsd_fuzz_test.go` (`:47-52`)
- Modify: `internal/bootstrap/statssink_fuzz_test.go` (`:110-115`)

**Interfaces:**
- Consumes: nothing.
- Produces: `bootstrap.StatsdSinkConfig{UDPAddress, TCPClusterName, Prefix string}` — consumed by Task 4 (more rejects) and Task 10 (`main.go`).

> **Six sites reference `tcp_cluster_name`, not one.** `grep -rn 'tcp_cluster_name\|TcpClusterName' --include='*.go' .` finds: `bootstrap.go:292` (struct doc), `:402` (field doc), `:556`/`:559` (function doc), `:567-568` (the reject); `statsd_fuzz_test.go:47,52`; **`statssink_fuzz_test.go:110,115`** (a SECOND seed the SPEC does not mention); `bootstrap_test.go:2075-2082`. All must be updated in this task or the build/tests will lie.

- [ ] **Step 1: Write the failing tests**

In `bootstrap_test.go`, **delete** the `tcp_cluster_name` sub-case at `:2074-2083` from the `TestStatsdSink_Rejects` table (it is no longer a reject) and add these new tests:

```go
// TestStatsdSink_AcceptTCPClusterName is the LIFT: phase 48 strict-rejected this.
func TestStatsdSink_AcceptTCPClusterName(t *testing.T) {
	bs, err := Load(strings.NewReader(statsBootstrap(`stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      tcp_cluster_name: mc
      prefix: myprefix
`)))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.StatsdSinkConfigs); got != 1 {
		t.Fatalf("len(StatsdSinkConfigs) = %d, want 1", got)
	}
	cfg := bs.StatsdSinkConfigs[0]
	if cfg.TCPClusterName != "mc" {
		t.Errorf("TCPClusterName = %q, want %q", cfg.TCPClusterName, "mc")
	}
	if cfg.UDPAddress != "" {
		t.Errorf("UDPAddress = %q, want \"\" (the tagged-union invariant: exactly one arm is set)", cfg.UDPAddress)
	}
	if cfg.Prefix != "myprefix" {
		t.Errorf("Prefix = %q, want %q", cfg.Prefix, "myprefix")
	}
}

func TestStatsdSink_AcceptTCPClusterNameDefaultPrefix(t *testing.T) {
	bs, err := Load(strings.NewReader(statsBootstrap(`stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      tcp_cluster_name: mc
`)))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := bs.StatsdSinkConfigs[0].Prefix; got != "envoy" {
		t.Errorf("Prefix = %q, want the %q default", got, "envoy")
	}
}

// TestStatsdSink_TCPArmDispatchedBeforeNilAddressReject is TRAP 2, pinned.
//
// bootstrap.go's ordering comment records that GetAddress() returns nil for BOTH
// a missing statsd_specifier AND a tcp_cluster_name arm — which is why the
// tcp_cluster_name REJECT had to run first. After the lift the meaning INVERTS:
// the tcp_cluster_name arm must be DISPATCHED (accept-and-return) before the
// nil-address reject fires, or a perfectly valid TCP config is rejected as
// "missing statsd_specifier".
//
// This test fails with exactly that misleading error if the arms are reordered.
func TestStatsdSink_TCPArmDispatchedBeforeNilAddressReject(t *testing.T) {
	_, err := Load(strings.NewReader(statsBootstrap(`stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      tcp_cluster_name: mc
`)))
	if err != nil {
		t.Fatalf("a tcp_cluster_name sink must NOT be rejected; got %v "+
			"(if this says \"statsd_specifier is required\", the nil-address reject "+
			"ran before the tcp_cluster_name dispatch)", err)
	}
}

// The missing-oneof reject must STILL fire — the lift must not turn it into an
// accept with two empty arms.
func TestStatsdSink_StillRejectsMissingSpecifier(t *testing.T) {
	_, err := Load(strings.NewReader(statsBootstrap(`stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      prefix: x
`)))
	if err == nil {
		t.Fatal("Load: want a reject for a missing statsd_specifier, got nil")
	}
	for _, sub := range []string{"bootstrap:", "statsd", "statsd_specifier"} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("error should contain %q: %q", sub, err.Error())
		}
	}
}
```

> **`statsBootstrap` already supplies a node.** Its fixed header (`bootstrap_test.go:1695`) is `node: { id: mc-node, cluster: mc-cluster }` and a cluster named `mc` is **not** present (`clusters: []`). Task 4 adds the unknown-cluster reject, at which point `tcp_cluster_name: mc` would start failing. **Therefore Task 4 must add a real `mc` cluster to `statsBootstrap`'s header** (or these tests must switch to a cluster the header declares). Read `statsBootstrap` at `:1695` before starting Task 4 and handle it there; in Task 3 there is no cluster check yet, so `mc` parses fine.

- [ ] **Step 2: Run to verify they fail**

```sh
go test ./internal/bootstrap/ -run 'TestStatsdSink' -count=1
```
Expected: FAIL — `cfg.TCPClusterName undefined (type StatsdSinkConfig has no field or method TCPClusterName)`, and `TestStatsdSink_AcceptTCPClusterName` reports the `UDP-only` reject.

- [ ] **Step 3: Add the tagged-union field**

Replace `bootstrap.go:289-300`:

```go
// StatsdSinkConfig is the parsed statsd stats-sink config from one top-level
// stats_sinks[] StatsdSink entry (ADR-0265; the TCP transport ADR-0272). The sink
// (the StatsdSink / TCPStatsdSink + Flusher) is constructed in cmd/envoy-go/main.go
// after Load returns; this struct carries only the parse-time data.
//
// It is a TAGGED UNION over the StatsdSink.statsd_specifier oneof, whose two arms
// are the ONLY two the proto has (stats.pb.go:571-577): EXACTLY ONE of UDPAddress
// and TCPClusterName is non-empty. A missing oneof (both empty) is a
// REFERENCE-PARITY reject, never a parsed value. socket_address.protocol is
// accepted-and-IGNORED on the UDP arm (dial UDP regardless — rejecting the
// proto-default TCP(0) would reject the omit case).
type StatsdSinkConfig struct {
	UDPAddress     string // address.socket_address host:port (an IP literal:port; net.ResolveUDPAddr-resolvable). Empty ⇔ the TCP arm.
	TCPClusterName string // tcp_cluster_name: a named cluster the sink dials via Cluster.DialSink (ADR-0272). Empty ⇔ the UDP arm.
	Prefix         string // StatsdSink.prefix, default "envoy" when empty
}
```

Update the `StatsdSinkConfigs` field doc at `:399-408`: replace the sentence beginning *"Per ADR-0265/ADR-0080: tcp_cluster_name …"* with:

```
	// Per ADR-0272 each element is a TAGGED UNION over statsd_specifier: exactly
	// one of UDPAddress (the UDP arm) / TCPClusterName (the TCP arm) is set.
	// Both arms share this single slice so declaration order is preserved.
```

- [ ] **Step 4: Rewrite `parseStatsdSinkConfig` to dispatch the TCP arm first**

Replace `bootstrap.go:554-579`:

```go
// parseStatsdSinkConfig parses one statsd stats sink typed_config and appends a
// StatsdSinkConfig to result.StatsdSinkConfigs (ADR-0265, ADR-0272).
//
// ORDERING (load-bearing, and INVERTED from phase 48): GetAddress() returns nil
// for BOTH a missing statsd_specifier AND a tcp_cluster_name arm. At phase 48,
// tcp_cluster_name was a REJECT, so it had to be checked FIRST to produce a
// distinct message. It is now an ACCEPT, so it must be DISPATCHED first for the
// same reason — otherwise the shared nil-address tail would reject a valid TCP
// config as "statsd_specifier is required". TestStatsdSink_TCPArmDispatchedBefore
// NilAddressReject pins this.
//
// The TCP arm carries two additional REFERENCE-PARITY rejects (node required;
// unknown cluster) — see parseStatsdTCPArm.
func parseStatsdSinkConfig(tc *anypb.Any, idx int, result *Bootstrap) error {
	var sd metricsconfigv3.StatsdSink
	if err := tc.UnmarshalTo(&sd); err != nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: statsd typed_config: %w", idx, err)
	}
	prefix := sd.GetPrefix()
	if cn := sd.GetTcpClusterName(); cn != "" {
		return parseStatsdTCPArm(cn, prefix, idx, result)
	}
	addr, prefix, err := parseUDPSinkAddressAndPrefix(sd.GetAddress().GetSocketAddress(), prefix, "statsd", "statsd_specifier", idx)
	if err != nil {
		return err
	}
	result.StatsdSinkConfigs = append(result.StatsdSinkConfigs, StatsdSinkConfig{
		UDPAddress: addr,
		Prefix:     prefix,
	})
	return nil
}

// parseStatsdTCPArm handles the tcp_cluster_name arm. Task 4 adds the two
// reference-parity rejects here; for now it only defaults the prefix and appends.
func parseStatsdTCPArm(clusterName, prefix string, idx int, result *Bootstrap) error {
	if prefix == "" {
		prefix = "envoy"
	}
	result.StatsdSinkConfigs = append(result.StatsdSinkConfigs, StatsdSinkConfig{
		TCPClusterName: clusterName,
		Prefix:         prefix,
	})
	return nil
}
```

> Note the `prefix` shadowing: `parseUDPSinkAddressAndPrefix` returns the defaulted prefix, so the UDP arm's `prefix` is re-bound from its return. `gofmt`/`golangci-lint` will flag an unused shadow if you get this wrong — do not silence it, fix it.

- [ ] **Step 5: Reseed both fuzzers**

`statsd_fuzz_test.go:47-52` — change the comment and add a companion. Replace:

```go
	// tcp_cluster_name (reject)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      tcp_cluster_name: statsd
`))
```
with:
```go
	// tcp_cluster_name (ACCEPT since phase 55 / ADR-0272 — the node in `head`
	// satisfies the node-required arm; the cluster is unknown, so this seed
	// exercises the unknown-cluster reject added in Task 4)
	f.Add([]byte(head + `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      tcp_cluster_name: statsd
`))
```

`statssink_fuzz_test.go:110-115` — same treatment. Read `:100-120` first; its `// statsd tcp_cluster_name (UDP-only reject)` comment is now false.

**Neither fuzzer asserts an outcome** (`f.Fuzz` only asserts "`Load` must not panic"), so no expectation flips — only the comments, which would otherwise be a lie that survives forever. `grep -c '^func Fuzz'` is unchanged: **52**.

- [ ] **Step 6: Run to verify all pass**

```sh
go test ./internal/bootstrap/ -count=1
go test ./internal/bootstrap/ -run 'FuzzStatsdSinkConfigParse' -count=1 -fuzz='FuzzStatsdSinkConfigParse' -fuzztime=20s
```
Expected: PASS; the fuzz run finds no new crashers.

- [ ] **Step 7: Per-task gate + commit**

```sh
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./...
grep -rn 'UDP-only' internal/bootstrap/    # must print NOTHING: the claim is retired
```

```bash
git add internal/bootstrap/
git commit -m "phase 55 Task 3: lift the statsd tcp_cluster_name reject; dispatch the arm first

StatsdSinkConfig becomes a tagged union over the two-arm statsd_specifier oneof
(D-TCP-CONFIG-SHAPE). The ordering constraint at bootstrap.go:556-561 survives
with INVERTED meaning: GetAddress() is nil for both a missing oneof and the TCP
arm, so the TCP arm must now be DISPATCHED before the nil-address reject fires.
Pinned by TestStatsdSink_TCPArmDispatchedBeforeNilAddressReject.

All SIX tcp_cluster_name sites updated, including the second fuzz seed in
statssink_fuzz_test.go that the SPEC did not name. Fuzzer count unchanged (52)."
```

---

## Task 4: Bootstrap — the node-required and unknown-cluster rejects (ADR-0080)

**Files:**
- Modify: `internal/bootstrap/bootstrap.go` — `parseStatsdTCPArm`
- Modify: `internal/bootstrap/bootstrap_test.go` — `statsBootstrap` header (`:1695`); new reject tests
- Modify: `internal/bootstrap/statsd_fuzz_test.go` — new seeds

**Interfaces:**
- Consumes: `parseStatsdTCPArm(clusterName, prefix string, idx int, result *Bootstrap) error` from Task 3.
- Produces: two reject arms. No new exported surface.

**The two rejects, both reference-PARITY (SPEC §6, probed at §11.6):**

| Condition | Reference message | envoy-go message |
|---|---|---|
| TCP arm + `node.id` or `node.cluster` missing | `tcp statsd: node 'id' and 'cluster' are required. Set it either in 'node' config or via --service-node and --service-cluster options.` | `bootstrap: stats_sinks[%d]: statsd tcp_cluster_name requires node.id and node.cluster to be set` |
| TCP arm + unknown cluster | `tcp statsd: unknown cluster 'c_nonexistent'` | `bootstrap: stats_sinks[%d]: statsd tcp_cluster_name %q names an unknown cluster` |

The UDP arm keeps booting with **no node at all** — the reference control probe confirmed the node requirement is TCP-SPECIFIC. A dedicated test pins that the lift did not leak the requirement onto the UDP arm.

**Why parse-time and not `main.go`:**
- *Node:* purely intra-bootstrap data. `result.Proto.GetNode()` is reachable and populated (see the D-TCP-NODE-PLACEMENT resolution above), so the landed `FuzzStatsdSinkConfigParse` and the `TestStatsdSink_Rejects` table cover the arm for free.
- *Unknown cluster:* `result.Proto.GetStaticResources().GetClusters()` is the complete cluster set — envoy-go has **no CDS** (the xDS family has never been opened). Parse-time *is* boot time in envoy-go (`Load` runs at boot), so this is reference-parity, and it is unit- and fuzz-testable without a second differential fixture dir. Per `reference_differential_fixture_dispatch_constraint` a boot-reject fixture cannot share `0098`'s directory, so a `main.go`-only check would have gone **untested**. `main.go` keeps a defensive `cm.Get` fatal (Task 10) for the can't-happen case; a `// when CDS lands, this static check must move to the cluster manager` comment marks the future.

**Reject precedence when BOTH arms are invalid was NOT probed.** envoy-go checks node first. No test depends on the order — each test makes exactly one arm invalid. Record this as an unpinned detail in the ADR, do not claim parity for it.

- [ ] **Step 1: Fix `statsBootstrap` so Task 3's accept tests keep passing**

Read `bootstrap_test.go:1695-1710`. Its header declares `clusters: []`. Add a cluster named `mc` so `tcp_cluster_name: mc` resolves:

```go
func statsBootstrap(topLevel string) string {
	return `node: { id: mc-node, cluster: mc-cluster }
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters:
    - name: mc
      connect_timeout: 1s
      type: STATIC
      load_assignment:
        cluster_name: mc
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: {address: 127.0.0.1, port_value: 9999}
` + topLevel
}
```

> Verify against the real `:1695` body before editing — if the metrics_service tests already rely on `clusters: []` being empty, adding `mc` may change an existing assertion. Run the FULL `./internal/bootstrap/` suite after this step, before writing any new test.

- [ ] **Step 2: Write the failing tests**

```go
// TestStatsdSink_TCPRejectsMissingNode: AMEND-TCP-NODE. Either field alone fails.
func TestStatsdSink_TCPRejectsMissingNode(t *testing.T) {
	const sink = `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      tcp_cluster_name: mc
`
	const clusters = `
static_resources:
  listeners: []
  clusters:
    - name: mc
      connect_timeout: 1s
      type: STATIC
      load_assignment:
        cluster_name: mc
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: {address: 127.0.0.1, port_value: 9999}
`
	const admin = `
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
`
	cases := []struct {
		name string
		node string
	}{
		{"no node at all", ""},
		{"node.id only", "node: { id: sd-node }\n"},
		{"node.cluster only", "node: { cluster: sd-cluster }\n"},
		{"node with empty id", "node: { id: \"\", cluster: sd-cluster }\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(strings.NewReader(tc.node + admin + clusters + sink))
			if err == nil {
				t.Fatalf("Load: want a node-required reject, got nil")
			}
			for _, sub := range []string{"bootstrap:", "stats_sinks[0]", "tcp_cluster_name", "node.id", "node.cluster"} {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("error should contain %q: %q", sub, err.Error())
				}
			}
		})
	}
}

// TestStatsdSink_TCPBothNodeFieldsBoots: the positive control.
func TestStatsdSink_TCPBothNodeFieldsBoots(t *testing.T) {
	if _, err := Load(strings.NewReader(statsBootstrap(`stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      tcp_cluster_name: mc
`))); err != nil {
		t.Fatalf("Load with node.id + node.cluster: %v", err)
	}
}

// TestStatsdSink_UDPArmNeedsNoNode is the CONTROL PROBE, mirrored: the reference
// boots a UDP statsd sink with NO node at all. The node requirement is
// TCP-SPECIFIC and must not leak onto the UDP arm.
func TestStatsdSink_UDPArmNeedsNoNode(t *testing.T) {
	const cfg = `admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters: []
stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      address: { socket_address: { address: 127.0.0.1, port_value: 8125 } }
`
	if _, err := Load(strings.NewReader(cfg)); err != nil {
		t.Fatalf("a UDP statsd sink must boot with NO node: %v", err)
	}
}

// TestStatsdSink_TCPRejectsUnknownCluster: reference parity, not envoy-go-strict.
func TestStatsdSink_TCPRejectsUnknownCluster(t *testing.T) {
	_, err := Load(strings.NewReader(statsBootstrap(`stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      tcp_cluster_name: c_nonexistent
`)))
	if err == nil {
		t.Fatal("Load: want an unknown-cluster reject, got nil")
	}
	for _, sub := range []string{"bootstrap:", "stats_sinks[0]", "tcp_cluster_name", "c_nonexistent", "unknown cluster"} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("error should contain %q: %q", sub, err.Error())
		}
	}
}
```

- [ ] **Step 3: Run to verify they fail**

```sh
go test ./internal/bootstrap/ -run 'TestStatsdSink_TCPRejects|TestStatsdSink_UDPArmNeedsNoNode' -count=1
```
Expected: FAIL — `Load: want a node-required reject, got nil` (both reject tests), `TestStatsdSink_UDPArmNeedsNoNode` already PASSes.

- [ ] **Step 4: Implement both rejects in `parseStatsdTCPArm`**

```go
// parseStatsdTCPArm handles the tcp_cluster_name arm of statsd_specifier
// (ADR-0272) and applies its two REFERENCE-PARITY rejects (ADR-0080), both
// probed against envoyproxy/envoy:contrib-v1.37.2 (SPEC-55 §11.6):
//
//  1. node.id AND node.cluster must BOTH be set. The reference refuses to boot
//     otherwise ("tcp statsd: node 'id' and 'cluster' are required."); either
//     field alone fails with the identical message. The UDP address arm boots
//     with no node at all — the requirement is TCP-SPECIFIC (control probe).
//     Neither value appears in any emitted line: this is a validation, not a
//     naming input.
//  2. tcp_cluster_name must name a declared cluster ("tcp statsd: unknown
//     cluster 'c_nonexistent'"). static_resources.clusters is the COMPLETE
//     cluster set today; when CDS lands this check must move to the cluster
//     manager. main.go keeps a defensive cm.Get fatal for the can't-happen case.
//
// Precedence when BOTH are invalid was not probed; envoy-go checks node first.
// No test depends on the order.
func parseStatsdTCPArm(clusterName, prefix string, idx int, result *Bootstrap) error {
	node := result.Proto.GetNode()
	if node.GetId() == "" || node.GetCluster() == "" {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: statsd tcp_cluster_name requires node.id and node.cluster to be set (the UDP address sink does not)", idx)
	}
	known := false
	for _, c := range result.Proto.GetStaticResources().GetClusters() {
		if c.GetName() == clusterName {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: statsd tcp_cluster_name %q names an unknown cluster", idx, clusterName)
	}
	if prefix == "" {
		prefix = "envoy"
	}
	result.StatsdSinkConfigs = append(result.StatsdSinkConfigs, StatsdSinkConfig{
		TCPClusterName: clusterName,
		Prefix:         prefix,
	})
	return nil
}
```

- [ ] **Step 5: Add fuzz seeds**

In `statsd_fuzz_test.go`, after the existing `tcp_cluster_name` seed, add two seeds that exercise the new arms. `head` already carries `node: { id: sd-node, cluster: sd-cluster }` and `clusters: []`, so the existing seed now hits the **unknown-cluster** reject. Add a **no-node** seed with its own header:

```go
	// tcp_cluster_name with a KNOWN cluster (accept)
	f.Add([]byte(`node: { id: sd-node, cluster: sd-cluster }
admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters:
    - name: c_statsd
      connect_timeout: 1s
      type: STATIC
      load_assignment:
        cluster_name: c_statsd
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: {address: 127.0.0.1, port_value: 8125}
stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      tcp_cluster_name: c_statsd
`))
	// tcp_cluster_name with NO node (reject — AMEND-TCP-NODE)
	f.Add([]byte(`admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters: []
stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdType + `
      tcp_cluster_name: c_statsd
`))
```

Fuzzer count stays **52** — these are seeds on an existing `FuzzStatsdSinkConfigParse`, not a new `func Fuzz`.

- [ ] **Step 6: Run to verify all pass**

```sh
go test ./internal/bootstrap/ -count=1                    # FULL package: the statsBootstrap header change must break nothing
go test ./internal/bootstrap/ -fuzz='FuzzStatsdSinkConfigParse' -fuzztime=30s
grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l   # 52
```

- [ ] **Step 7: Deliberate-break liveness check (the subagent runs it; the CONTROLLER re-performs it)**

Prove each new reject is live, not vacuous:

```sh
# (a) delete the node check → TestStatsdSink_TCPRejectsMissingNode MUST fail
# (b) make `known` always true → TestStatsdSink_TCPRejectsUnknownCluster MUST fail
# (c) move the node check into the UDP arm → TestStatsdSink_UDPArmNeedsNoNode MUST fail
go test ./internal/bootstrap/ -count=1     # after EACH break; then `git restore` and re-run
```

**GIT HYGIENE** (`feedback_subagent_worktree_detach`): revert with `git restore <file>` ONLY. Never `git checkout <sha>`, never `git commit --amend`. After every break, re-verify `git branch --show-current` prints `phase-55-...`, not a detached HEAD.

- [ ] **Step 8: Per-task gate + commit**

```bash
git add internal/bootstrap/
git commit -m "phase 55 Task 4: statsd TCP arm — node-required + unknown-cluster rejects (ADR-0080)

Both are reference PARITY, probed at SPEC §11.6, and both live in
parseStatsdTCPArm at PARSE time: result.Proto.GetNode() and
result.Proto.GetStaticResources() are reachable there (Load builds
&Bootstrap{Proto: bs} at bootstrap.go:467 before parseStatsSinks runs), so
FuzzStatsdSinkConfigParse and the TestStatsdSink_* table cover them for free.
A main.go-only check would have been untestable: per the one-fixture-dir/one-
runner-branch constraint, a boot-reject fixture cannot share 0098's directory.

TestStatsdSink_UDPArmNeedsNoNode mirrors the reference's control probe: the node
requirement is TCP-SPECIFIC and must not leak onto the UDP arm."
```

---

## Task 5: `TCPStatsdSink` skeleton — dial seam, bounded channel, writer goroutine, delta-in-writer

**Files:**
- Create: `internal/statssink/statsd_tcp.go`
- Create: `internal/statssink/statsd_tcp_test.go`

**Interfaces:**
- Consumes: `emitStatsdLines` (`udp.go:58`), `deltaState`/`newDeltaState` (`delta.go:22,39`), `defaultChannelCapacity`/`closeDrainGrace`/`dropLogIntervalNanos` (`sink.go:33-48`).
- Produces, consumed by Task 10:
  - `type DialFunc func(ctx context.Context) (net.Conn, error)`
  - `func NewTCPStatsdSink(dial DialFunc, prefix string) *TCPStatsdSink`
  - `func (s *TCPStatsdSink) Submit(batch []*dto.MetricFamily)` / `func (s *TCPStatsdSink) Close() error` (satisfying `Sink`, `sink.go:18-21`)

**Exact signatures this task must match** (verified, character-for-character):
```go
func emitStatsdLines(batch []*dto.MetricFamily, nameAndTags func(fam *dto.MetricFamily) (name, tagSuffix string), emit func(line string))
func (d *deltaState) apply(abs []*dto.MetricFamily) []*dto.MetricFamily   // returns a NEW slice; does NOT mutate abs
```

This task builds everything **except** the write path (Task 7), the cap (Task 8), and `Close`'s unwedge (Task 9). `flush` here only serializes into `pending`.

- [ ] **Step 1: Write the failing tests**

Create `internal/statssink/statsd_tcp_test.go`:

```go
package statssink

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// fakeConn is a net.Conn test double that records every Write and can be told to
// accept only n bytes then error (Task 7), or to block until Close (Task 9).
type fakeConn struct {
	mu      sync.Mutex
	writes  [][]byte
	closed  bool
	acceptN int   // 0 ⇒ accept everything
	errAfter error // non-nil ⇒ return (acceptN, errAfter)
}

func (c *fakeConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, errors.New("fakeConn: write on closed conn")
	}
	if c.errAfter != nil {
		n := c.acceptN
		if n > len(b) {
			n = len(b)
		}
		c.writes = append(c.writes, append([]byte(nil), b[:n]...))
		return n, c.errAfter
	}
	c.writes = append(c.writes, append([]byte(nil), b...))
	return len(b), nil
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeConn) written() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([][]byte, len(c.writes))
	copy(out, c.writes)
	return out
}

func (c *fakeConn) Read([]byte) (int, error)         { return 0, errors.New("unused") }
func (c *fakeConn) LocalAddr() net.Addr              { return nil }
func (c *fakeConn) RemoteAddr() net.Addr             { return nil }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

// compile-time seam check
var _ net.Conn = (*fakeConn)(nil)
var _ Sink = (*TCPStatsdSink)(nil)

// waitWrites polls until conn has recorded at least n writes.
func waitWrites(t *testing.T, c *fakeConn, n int) [][]byte {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		w := c.written()
		if len(w) >= n {
			return w
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out: %d writes, want >= %d", len(w), n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestTCPStatsdSink_DialsLazilyOnFirstFlush(t *testing.T) {
	var dials int32
	conn := &fakeConn{}
	var mu sync.Mutex
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		dials++
		mu.Unlock()
		return conn, nil
	}, "p")

	mu.Lock()
	d := dials
	mu.Unlock()
	if d != 0 {
		t.Fatalf("dials before first Submit = %d, want 0 (lazy dial)", d)
	}

	s.Submit([]*dto.MetricFamily{counterFam("a.b", 3)})
	waitWrites(t, conn, 1)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if dials != 1 {
		t.Fatalf("dials = %d, want exactly 1 (one long-lived connection)", dials)
	}
}

// TestTCPStatsdSink_SubmitNeverBlocks: the Flusher calls Submit SERIALLY across
// all sinks from ONE goroutine (flusher.go:46-51). A blocking TCP sink would
// starve every sibling sink. Submit must return even with no writer draining.
func TestTCPStatsdSink_SubmitNeverBlocks(t *testing.T) {
	block := make(chan struct{})
	s := NewTCPStatsdSink(func(ctx context.Context) (net.Conn, error) {
		<-block // the writer is parked in dial forever
		return nil, errors.New("unreachable")
	}, "p")
	defer func() { close(block); _ = s.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Far more than defaultChannelCapacity (8): the excess must be DROPPED,
		// not block.
		for i := 0; i < 100; i++ {
			s.Submit([]*dto.MetricFamily{counterFam("a.b", 1)})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit blocked; it must drop on a full channel")
	}
}

// TestTCPStatsdSink_EnqueueDropDoesNotLatchDelta pins ADR-0263: delta.apply runs
// in the WRITER, not in Submit, so a batch dropped at the channel never latches
// deltaState — the dropped increments ride the NEXT enqueued flush.
func TestTCPStatsdSink_EnqueueDropDoesNotLatchDelta(t *testing.T) {
	release := make(chan struct{})
	conn := &fakeConn{}
	first := true
	var mu sync.Mutex
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		f := first
		first = false
		mu.Unlock()
		if f {
			<-release // park the writer inside its first dial
		}
		return conn, nil
	}, "p")

	// Fill the channel far past capacity while the writer is parked; all but the
	// buffered few are dropped.
	for i := 0; i < 50; i++ {
		s.Submit([]*dto.MetricFamily{counterFam("a.b", float64(i+1))})
	}
	// One final Submit carrying the CUMULATIVE absolute value.
	close(release)
	waitWrites(t, conn, 1)
	_ = s.Close()

	// The FIRST line the writer ever emits must carry the absolute value of the
	// first batch it actually dequeued — never a delta against a batch that was
	// dropped before reaching the writer.
	w := conn.written()
	if len(w) == 0 {
		t.Fatal("no writes")
	}
	// A dropped batch must not have advanced deltaState: the sum of all emitted
	// a.b deltas equals the LAST absolute value the writer saw.
	// (Asserted precisely in Task 6; here we only require a non-empty write.)
	if len(w[0]) == 0 {
		t.Fatal("first write was empty")
	}
}
```

> `counterFam(name, val)` and `gaugeFam(name, val)` already exist in `delta_test.go:10-24` (same package). **Read them and reuse — do not redefine.** If their signatures differ from `(string, float64) *dto.MetricFamily`, adapt these tests, not the helpers.

- [ ] **Step 2: Run to verify they fail**

```sh
go test ./internal/statssink/ -run 'TestTCPStatsdSink' -count=1
```
Expected: FAIL — `undefined: NewTCPStatsdSink`, `undefined: TCPStatsdSink`.

- [ ] **Step 3: Create `internal/statssink/statsd_tcp.go`**

```go
// Package statssink — the plain-statsd TCP transport (ADR-0272).
package statssink

import (
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	dto "github.com/prometheus/client_model/go"
)

// writeTimeout bounds ONE conn.Write so a wedged-but-not-dead peer cannot stall
// the writer goroutine indefinitely. Belt-and-braces with the hard conn.Close()
// that Close performs (D-TCP-CLOSE): the deadline handles a silently stalled
// peer, the Close handles shutdown.
const writeTimeout = 5 * time.Second

// DialFunc obtains one upstream connection for the sink. cmd/envoy-go/main.go
// closes over the cluster manager and re-looks-up the cluster per dial (the
// grpcclient.go:128-141 idiom), so internal/statssink never imports
// internal/cluster. A func rather than a one-method interface: the seam has one
// method and no shared state.
type DialFunc func(ctx context.Context) (net.Conn, error)

// TCPStatsdSink emits the statsd line protocol (identical to StatsdSink's) over
// one long-lived, newline-delimited TCP connection obtained from a named cluster
// (StatsdSink.statsd_specifier.tcp_cluster_name; ADR-0272).
//
// SHAPE (ADR-0262, the MetricsServiceSink precedent): Submit non-blocking-sends
// onto a bounded channel and NEVER blocks — the Flusher calls Submit serially
// across every sink from one goroutine (flusher.go:46-51), so a blocking TCP
// write here would starve every sibling sink. Phase 48's synchronous Submit was
// licensed by a property TCP lacks ("a UDP datagram never blocks on a peer").
//
// A single writer goroutine owns dial, `pending`, and the write. delta.apply runs
// in the WRITER (ADR-0263), so a channel-full enqueue drop never latches
// deltaState: the dropped increments ride the next enqueued flush.
//
// This is the statsd sinks' FIRST background mutator.
type TCPStatsdSink struct {
	ch     chan []*dto.MetricFamily
	dial   DialFunc
	prefix string
	delta  *deltaState // always non-nil — statsd |c is intrinsically a per-flush delta (no knob)

	done        chan struct{}
	closeOnce   sync.Once
	lastDropLog atomic.Int64

	// ctx/cancel bound an in-flight DialContext. They CANNOT interrupt a blocked
	// conn.Write (a raw net.Conn is not context-bound — the one place the
	// MetricsServiceSink precedent stops transferring); connMu + conn.Close() is
	// the unwedge (D-TCP-CLOSE).
	ctx    context.Context
	cancel context.CancelFunc

	// connMu guards the conn FIELD and the closed flag — never the I/O. The
	// writer copies conn out under the lock and does the blocking Write unlocked;
	// Close closes it under the lock, which unwedges that Write with an error.
	connMu sync.Mutex
	conn   net.Conn
	closed bool

	// pending is WRITER-OWNED (no lock): the unwritten bytes, accumulated across
	// flushes, always ending on a line boundary.
	pending []byte
}

// NewTCPStatsdSink starts the writer goroutine. The dial is LAZY: no connection
// is opened until the first flush.
func NewTCPStatsdSink(dial DialFunc, prefix string) *TCPStatsdSink {
	s := &TCPStatsdSink{
		ch:     make(chan []*dto.MetricFamily, defaultChannelCapacity),
		dial:   dial,
		prefix: prefix,
		delta:  newDeltaState(),
		done:   make(chan struct{}),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}

// Submit enqueues one full-registry snapshot. Non-blocking: on a full channel the
// batch is DROPPED with a rate-limited diagnostic (the sink.go:123-133 idiom).
func (s *TCPStatsdSink) Submit(batch []*dto.MetricFamily) {
	select {
	case s.ch <- batch:
	default:
		s.logDrop("channel full, dropping flush batch", nil)
	}
}

func (s *TCPStatsdSink) run() {
	defer close(s.done)
	defer s.markClosedAndCloseConn()
	for batch := range s.ch {
		s.flush(batch)
	}
}

// flush applies the delta, serializes into pending, and writes. Task 6 fills in
// the serialization; Task 7 the write; Task 8 the cap.
func (s *TCPStatsdSink) flush(batch []*dto.MetricFamily) {
	_ = s.delta.apply(batch) // Task 6 wires this into emitStatsdLines
}

// markClosedAndCloseConn latches closed and hard-closes the live conn, which
// unwedges a blocked Write. Idempotent; safe from any goroutine.
func (s *TCPStatsdSink) markClosedAndCloseConn() {
	s.connMu.Lock()
	s.closed = true
	c := s.conn
	s.conn = nil
	s.connMu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// Close stops the writer. Task 9 adds the drain/grace/unwedge.
func (s *TCPStatsdSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
		<-s.done
		s.cancel()
	})
	return nil
}

// logDrop rate-limits a diagnostic to at most once per second (the sink.go
// lastDropLog idiom). Drops are LOGGED not counted — zero statsd-sink self-stats
// (SPEC-55 §7 / D-TCP-STATS).
func (s *TCPStatsdSink) logDrop(what string, err error) {
	now := time.Now().UnixNano()
	last := s.lastDropLog.Load()
	if now-last < dropLogIntervalNanos || !s.lastDropLog.CompareAndSwap(last, now) {
		return
	}
	if err != nil {
		log.Printf("statssink: statsd tcp: %s: %v", what, err)
		return
	}
	log.Printf("statssink: statsd tcp: %s", what)
}
```

Add the lazy dial + guarded conn accessors (used from Task 7 on):

```go
// currentConn returns the live conn, or nil. Never call I/O while holding connMu.
func (s *TCPStatsdSink) currentConn() net.Conn {
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return s.conn
}

// dropConn closes and clears the live conn after a write error.
func (s *TCPStatsdSink) dropConn() {
	s.connMu.Lock()
	c := s.conn
	s.conn = nil
	s.connMu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

// redial opens a fresh connection. Returns false on failure (pending is RETAINED
// and retried on the next flush) or if Close has already latched.
func (s *TCPStatsdSink) redial() bool {
	c, err := s.dial(s.ctx)
	if err != nil {
		s.logDrop("dial", err)
		return false
	}
	s.connMu.Lock()
	if s.closed {
		s.connMu.Unlock()
		_ = c.Close() // Close raced us; do not leak the fresh conn
		return false
	}
	s.conn = c
	s.connMu.Unlock()
	return true
}
```

Wire `flush` to dial lazily and write once (the real write lands in Task 7):

```go
func (s *TCPStatsdSink) flush(batch []*dto.MetricFamily) {
	_ = s.delta.apply(batch) // Task 6
	if s.currentConn() == nil && !s.redial() {
		return
	}
	conn := s.currentConn()
	if conn == nil {
		return
	}
	_, _ = conn.Write([]byte("\n")) // placeholder, replaced in Task 6/7
}
```

> The placeholder write exists ONLY so Task 5's "dials lazily, exactly once" tests are meaningful. Task 6 replaces it. Do not commit Task 5 and stop.

- [ ] **Step 4: Run to verify they pass**

```sh
go test ./internal/statssink/ -run 'TestTCPStatsdSink' -count=1 -v
go test ./internal/statssink/ -count=1 -race    # FULL package: the first background mutator
```

- [ ] **Step 5: Enforce the import boundary**

```sh
go list -deps ./internal/statssink/ | grep 'envoy-go/internal/cluster' && echo "VIOLATION" && exit 1
echo "boundary OK: internal/statssink does not import internal/cluster"
```

- [ ] **Step 6: Per-task gate + commit**

```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/statsd_tcp.go internal/statssink/statsd_tcp_test.go
git commit -m "phase 55 Task 5: TCPStatsdSink skeleton — dial seam, bounded channel, writer goroutine

The MetricsServiceSink/ADR-0262 shape: Submit non-blocking-sends onto a cap-8
channel and never blocks the serial Flusher; one writer goroutine owns the dial,
pending, and the write. delta.apply runs in the WRITER (ADR-0263).

connMu guards the conn FIELD, never the I/O — the writer copies the conn out and
Writes unlocked, so Close can hard-close it to unwedge a blocked Write. The dial
seam is a DialFunc, so internal/statssink still does not import internal/cluster
(enforced by a go list -deps grep in the gate)."
```

---

## Task 6: Serialize into `pending` via `emitStatsdLines` — `\n`-TERMINATED

**Files:**
- Modify: `internal/statssink/statsd_tcp.go` (`flush`)
- Modify: `internal/statssink/statsd_tcp_test.go`

**Interfaces:**
- Consumes: `emitStatsdLines`, `deltaState.apply`, `s.pending` from Task 5.
- Produces: `pending` always holds whole, `\n`-terminated lines.

- [ ] **Step 1: Write the failing tests**

```go
// TestTCPStatsdSink_LinesAreNewlineTERMINATED pins D-TCP-LINE, adopted verbatim
// from the reference (reference_wire_format_both_sides_see_same_bytes): EVERY
// line, INCLUDING THE LAST OF A FLUSH, ends with '\n'. No write contains "\n\n".
func TestTCPStatsdSink_LinesAreNewlineTERMINATED(t *testing.T) {
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return conn, nil }, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("a.b", 3), gaugeFam("g.h", 9)})
	waitWrites(t, conn, 1)
	_ = s.Close()

	w := conn.written()[0]
	if len(w) == 0 || w[len(w)-1] != '\n' {
		t.Fatalf("write %q must be '\\n'-TERMINATED (not separated)", w)
	}
	if bytes.Contains(w, []byte("\n\n")) {
		t.Fatalf("write %q contains a blank line", w)
	}
	got := strings.Split(strings.TrimSuffix(string(w), "\n"), "\n")
	want := []string{"sdpfx.a.b:3|c", "sdpfx.g.h:9|g"}
	sameSet(t, got, want)
}

// TestTCPStatsdSink_CounterDeltaGaugeAbsolute pins D-TCP-DELTA.
func TestTCPStatsdSink_CounterDeltaGaugeAbsolute(t *testing.T) {
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return conn, nil }, "sdpfx")

	s.Submit([]*dto.MetricFamily{counterFam("a.b", 2), gaugeFam("g.h", 5)})
	waitWrites(t, conn, 1)
	s.Submit([]*dto.MetricFamily{counterFam("a.b", 7), gaugeFam("g.h", 5)})
	waitWrites(t, conn, 2)
	s.Submit([]*dto.MetricFamily{counterFam("a.b", 7), gaugeFam("g.h", 5)})
	waitWrites(t, conn, 3)
	_ = s.Close()

	w := conn.written()
	// COUNTER: per-flush DELTA. 2 → +5 → +0. A ZERO-delta counter is STILL emitted.
	assertContains(t, w[0], "sdpfx.a.b:2|c")
	assertContains(t, w[1], "sdpfx.a.b:5|c")
	assertContains(t, w[2], "sdpfx.a.b:0|c")
	// GAUGE: ABSOLUTE, constant.
	for i := range w {
		assertContains(t, w[i], "sdpfx.g.h:5|g")
	}
}

func assertContains(t *testing.T, haystack []byte, needle string) {
	t.Helper()
	if !bytes.Contains(haystack, []byte(needle+"\n")) {
		t.Errorf("write %q missing terminated line %q", haystack, needle)
	}
}
```

> `sameSet(t, got, want)` already exists at `statsd_test.go:44-61` (same package). Reuse it. Add `"bytes"` and `"strings"` to the test imports.

- [ ] **Step 2: Run to verify they fail**

```sh
go test ./internal/statssink/ -run 'TestTCPStatsdSink_LinesAre|TestTCPStatsdSink_CounterDelta' -count=1
```
Expected: FAIL — the writes contain only the Task-5 `"\n"` placeholder.

- [ ] **Step 3: Implement the serialization**

Replace `flush`'s body:

```go
func (s *TCPStatsdSink) flush(batch []*dto.MetricFamily) {
	// delta.apply runs HERE, in the writer (ADR-0263) — never in Submit — so a
	// channel-full enqueue drop cannot latch deltaState. It returns a NEW slice;
	// the Flusher's absolute batch, fanned to every sink, is never mutated.
	emitStatsdLines(s.delta.apply(batch), func(fam *dto.MetricFamily) (string, string) {
		return s.prefix + "." + fam.GetName(), ""
	}, func(line string) {
		// '\n'-TERMINATED, including the LAST line of the flush. Adopted verbatim
		// from the reference: every captured write ends 0x0A, no write contains
		// "\n\n" (SPEC-55 §11.1).
		s.pending = append(append(s.pending, line...), '\n')
	})
	if len(s.pending) == 0 {
		return
	}
	if s.currentConn() == nil && !s.redial() {
		return // dial failed: pending RETAINED, retried next flush
	}
	conn := s.currentConn()
	if conn == nil {
		return
	}
	if _, err := conn.Write(s.pending); err != nil {
		s.dropConn()
		s.logDrop("write", err)
		return // Task 7 replaces this with the line-aligned resume
	}
	s.pending = s.pending[:0]
}
```

The `nameAndTags` closure is the `StatsdSink.Submit` one verbatim (`statsd.go:59-64`): `prefix + "." + fam.GetName()`, empty tag suffix.

- [ ] **Step 4: Run to verify they pass**

```sh
go test ./internal/statssink/ -count=1 -v -run TestTCPStatsdSink
go test ./internal/statssink/ -count=1 -race
```

- [ ] **Step 5: Deliberate-break liveness check**

- Change `'\n'` to a `\n`-SEPARATOR (append `'\n'` only when `len(s.pending) > 0` *before* the line) → `TestTCPStatsdSink_LinesAreNewlineTERMINATED` MUST fail on the final-terminator assertion.
- Move `s.delta.apply` into `Submit` → `TestTCPStatsdSink_EnqueueDropDoesNotLatchDelta` MUST fail.

`git restore` after each; re-run with `-count=1`.

- [ ] **Step 6: Per-task gate + commit**

```bash
git add internal/statssink/
git commit -m "phase 55 Task 6: serialize into pending via emitStatsdLines, '\n'-TERMINATED

emitStatsdLines is reused verbatim with StatsdSink's own nameAndTags closure. The
emit callback appends line + '\n' — TERMINATED, including the last line of each
flush, adopted byte-for-byte from the reference (SPEC §11.1). A zero-delta
counter is still emitted, matching deltaState.apply."
```

---

## Task 7: The write path — line-aligned resume on a partial write

**Files:**
- Modify: `internal/statssink/statsd_tcp.go` (`flush`)
- Modify: `internal/statssink/statsd_tcp_test.go`

**Interfaces:** consumes Task 6's `pending`. Produces no new exported surface.

### The invariant, stated exactly

A `Write` returning `0 < n < len(pending)` with a non-nil error means bytes `[0,n)` **may** have reached the peer, ending mid-line. Go's blocking `net.Conn.Write` loops internally, so `err == nil` implies `n == len(pending)`: **a non-nil error is the only partial-write path.**

`bytes.LastIndexByte(pending[:n], '\n') + 1` is precisely the start of the straddling line (and equals `n` when `n` already lands on a boundary, correctly retaining nothing; and equals `0` when `n == 0`, correctly retaining everything).

- Complete lines within `[0,n)` were delivered exactly once and are NOT re-sent → **no duplication**.
- The straddling line was **discarded by the dead connection's stream parser at EOF** (Task 2's `TestTCPDiscardsIncompleteTrailingLineAtEOF`) and is re-sent whole on the fresh connection → **no loss**.

This is what the reference achieves by accumulating and delivering every unwritten snapshot on reconnect (45 snapshots over a 30 s outage, nothing lost or duplicated — §11.5). The fixture's receiver never dies, so this is **UNIT-proven**, not differentially proven.

- [ ] **Step 1: Write the failing tests**

```go
// TestTCPStatsdSink_LineAlignedResume is the AMEND-TCP-RESUME proof. Conn #1
// accepts a byte count that lands MID-LINE and then errors; conn #2 receives the
// remainder. The delivered line MULTISET across both conns must equal the emitted
// multiset EXACTLY — no loss, no duplication.
func TestTCPStatsdSink_LineAlignedResume(t *testing.T) {
	// Three lines, deterministic order via a single-family batch per name is not
	// possible (emitStatsdLines walks the batch slice in order), so we control
	// order by the batch slice order.
	batch := []*dto.MetricFamily{
		counterFam("aaa", 1), // "sdpfx.aaa:1|c\n"  → 14 bytes
		counterFam("bbb", 2), // "sdpfx.bbb:2|c\n"  → 14 bytes
		counterFam("ccc", 3), // "sdpfx.ccc:3|c\n"  → 14 bytes
	}
	const line0 = "sdpfx.aaa:1|c\n"
	const line1 = "sdpfx.bbb:2|c\n"
	const line2 = "sdpfx.ccc:3|c\n"
	// Accept line0 in full plus 5 bytes of line1 ("sdpfx"), then error.
	acceptN := len(line0) + 5

	c1 := &fakeConn{acceptN: acceptN, errAfter: errors.New("peer reset")}
	c2 := &fakeConn{}
	var mu sync.Mutex
	dialN := 0
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		dialN++
		if dialN == 1 {
			return c1, nil
		}
		return c2, nil
	}, "sdpfx")

	s.Submit(batch)
	waitWrites(t, c1, 1)
	// Second flush: the writer redials and sends the retained suffix + the new
	// (zero-delta) lines.
	s.Submit(batch)
	waitWrites(t, c2, 1)
	_ = s.Close()

	// (i) conn #1 received line0 in full and a 5-byte fragment of line1.
	got1 := string(c1.written()[0])
	if got1 != line0+"sdpfx" {
		t.Fatalf("conn#1 got %q, want %q", got1, line0+"sdpfx")
	}
	// (ii) conn #2's FIRST bytes are line1 re-sent WHOLE — not its 9-byte tail.
	got2 := string(c2.written()[0])
	if !strings.HasPrefix(got2, line1) {
		t.Fatalf("conn#2 first bytes = %q; the straddling line %q must be re-sent WHOLE", got2, line1)
	}
	// (iii) line0 is NOT re-sent: a complete line that landed is never duplicated.
	if strings.Contains(got2, line0) {
		t.Fatalf("conn#2 %q re-sent the already-delivered line %q (DUPLICATION)", got2, line0)
	}
	// (iv) the straddling line is present exactly once across BOTH conns' COMPLETE
	//      lines: conn#1's trailing fragment is discarded by the receiver at EOF.
	if strings.Count(completeLines(got1)+completeLines(got2), line1) != 1 {
		t.Fatalf("line1 must appear exactly once among complete lines")
	}
	// (v) line2 (never written to conn#1) survives.
	if !strings.Contains(got2, line2) {
		t.Fatalf("conn#2 %q lost line2 %q", got2, line2)
	}
}

// completeLines drops an unterminated trailing line, exactly as the receiver's
// stream parser does at EOF.
func completeLines(s string) string {
	i := strings.LastIndexByte(s, '\n')
	if i < 0 {
		return ""
	}
	return s[:i+1]
}

// TestTCPStatsdSink_WriteErrorAtBoundaryRetainsNothing: n lands exactly on a '\n'.
func TestTCPStatsdSink_WriteErrorAtBoundaryRetainsNothing(t *testing.T) {
	c1 := &fakeConn{acceptN: len("sdpfx.aaa:1|c\n"), errAfter: errors.New("reset")}
	c2 := &fakeConn{}
	var mu sync.Mutex
	n := 0
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return c1, nil
		}
		return c2, nil
	}, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 1), counterFam("bbb", 2)})
	waitWrites(t, c1, 1)
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 1), counterFam("bbb", 2)})
	waitWrites(t, c2, 1)
	_ = s.Close()

	if got := string(c2.written()[0]); strings.Contains(got, "sdpfx.aaa:1|c") {
		t.Fatalf("conn#2 %q re-sent a line that landed exactly at the boundary", got)
	}
}

// TestTCPStatsdSink_ZeroBytesWrittenRetainsEverything: n == 0 ⇒ nothing landed.
func TestTCPStatsdSink_ZeroBytesWrittenRetainsEverything(t *testing.T) {
	c1 := &fakeConn{acceptN: 0, errAfter: errors.New("reset")}
	c2 := &fakeConn{}
	var mu sync.Mutex
	n := 0
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return c1, nil
		}
		return c2, nil
	}, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 1)})
	waitWrites(t, c1, 1)
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 1)})
	waitWrites(t, c2, 1)
	_ = s.Close()

	if got := string(c2.written()[0]); !strings.Contains(got, "sdpfx.aaa:1|c\n") {
		t.Fatalf("conn#2 %q lost the line that was never written (n==0 must retain everything)", got)
	}
}

// TestTCPStatsdSink_DialFailureRetainsPending: a failed dial must not lose bytes.
func TestTCPStatsdSink_DialFailureRetainsPending(t *testing.T) {
	c := &fakeConn{}
	var mu sync.Mutex
	n := 0
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return nil, errors.New("connection refused")
		}
		return c, nil
	}, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 4)}) // dial fails; pending retained
	s.Submit([]*dto.MetricFamily{counterFam("aaa", 4)}) // dial succeeds; delta 0
	waitWrites(t, c, 1)
	_ = s.Close()

	got := string(c.written()[0])
	if !strings.Contains(got, "sdpfx.aaa:4|c\n") {
		t.Fatalf("the first flush's line was lost across a dial failure: %q", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```sh
go test ./internal/statssink/ -run 'Resume|Boundary|ZeroBytes|DialFailureRetains' -count=1
```
Expected: FAIL — Task 6's error path drops the whole `pending` implicitly (it never re-sends), so conn #2's first bytes are `line2`, not `line1`.

- [ ] **Step 3: Implement the resume**

Replace the write tail of `flush`:

```go
	conn := s.currentConn()
	if conn == nil {
		return
	}
	// Bound ONE write so a wedged-but-alive peer cannot stall the writer.
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	n, err := conn.Write(s.pending)
	if err == nil {
		// Go's blocking net.Conn.Write loops internally: err == nil ⇒ n == len.
		s.pending = s.pending[:0]
		return
	}
	s.dropConn()
	// LINE-ALIGNED RESUME (AMEND-TCP-RESUME). Bytes [0,n) may have reached the
	// peer, ending mid-line. Retain from the start of the STRADDLING line:
	//   - complete lines within [0,n) landed exactly once and are NOT re-sent
	//     ⇒ no duplication;
	//   - the straddling line was DISCARDED by the dead conn's stream parser at
	//     EOF and is re-sent WHOLE on the next conn ⇒ no loss.
	// LastIndexByte(...)+1 == n when n lands on a boundary (retain nothing) and
	// == 0 when n == 0 (retain everything).
	keep := bytes.LastIndexByte(s.pending[:n], '\n') + 1
	// Compact to the front: `copy` handles the overlapping forward move, and the
	// buffer's capacity is reused instead of growing without bound.
	s.pending = append(s.pending[:0], s.pending[keep:]...)
	s.logDrop("write", err)
```

Add `"bytes"` to the imports.

> **Subtlety worth an explicit comment in the code:** `append(s.pending[:0], s.pending[keep:]...)` copies a suffix of the same backing array to its front. `copy`'s forward-overlap semantics make this correct (destination index < source index). Do **not** "optimize" it to `s.pending = s.pending[keep:]` — that reslice keeps the head alive and lets the backing array grow unboundedly across reconnects.

- [ ] **Step 4: Run to verify they pass**

```sh
go test ./internal/statssink/ -count=1 -v -run TestTCPStatsdSink
go test ./internal/statssink/ -count=1 -race
```

- [ ] **Step 5: Deliberate-break liveness check (all with `-count=1`)**

| Break | Test that MUST fail |
|---|---|
| `keep := 0` (retain everything) | `TestTCPStatsdSink_LineAlignedResume` (iii): `line0` is re-sent ⇒ DUPLICATION |
| `keep := n` (retain nothing past `n`) | `TestTCPStatsdSink_LineAlignedResume` (ii)/(iv): the straddling line is lost |
| `s.pending = s.pending[:0]` on error (drop the batch — the BRAINSTORM's original Q3) | `TestTCPStatsdSink_LineAlignedResume` (v) and `TestTCPStatsdSink_ZeroBytesWrittenRetainsEverything` |

`git restore internal/statssink/statsd_tcp.go` after each. **Re-run with `-count=1`** — the test cache serves a stale PASS otherwise (`reference_differential_break_protocol_count1`).

- [ ] **Step 6: Per-task gate + commit**

```bash
git add internal/statssink/
git commit -m "phase 55 Task 7: line-aligned resume on a partial write (AMEND-TCP-RESUME)

conn.Write reports n, so a third option exists beyond the BRAINSTORM's false
dichotomy of replay-double-counts vs drop-loses: retain the unwritten suffix
REALIGNED to the last complete line boundary <= n. Complete lines that landed are
never re-sent; the one straddling line -- which the dead conn's stream parser
discards at EOF (statsdrecv, Task 2) -- is re-sent whole.

Unit-proven, not differentially proven: the 0098 receiver never dies. Three
deliberate breaks (keep=0, keep=n, drop-the-batch) each fail a distinct assertion."
```

---

## Task 8: `maxPendingBytes` + `dropOldestLines` + the rate-limited overflow log

**Files:**
- Modify: `internal/statssink/statsd_tcp.go`
- Modify: `internal/statssink/statsd_tcp_test.go`

**Interfaces:** produces `func dropOldestLines(p []byte, capBytes int) []byte` (package-private, pure).

**The departure, stated plainly:** the reference's accumulate buffer is effectively **unbounded** (§11.5 measured ≥45 snapshots / ~200 KB retained with no cap reached). envoy-go BOUNDS it. On overflow the OLDEST whole lines are dropped, permanently losing their counter increments. This is a **deliberate, documented departure** — an unbounded in-process buffer is a memory-growth hazard the tree refuses elsewhere (`MetricsServiceSink` drops whole batches on a full cap-8 channel), and statsd is lossy by design. It must appear in `BEHAVIOR_CONTRACT.md` (Task 13).

- [ ] **Step 1: Write the failing tests** (pure-function first — no goroutines)

```go
func TestDropOldestLines(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		capBytes int
		want    string
	}{
		{"under cap: untouched", "aa\nbb\n", 100, "aa\nbb\n"},
		{"exactly at cap: untouched", "aa\nbb\n", 6, "aa\nbb\n"},
		{"drops the oldest whole line", "aa\nbb\n", 5, "bb\n"},
		{"rounds UP to the next boundary, never splits a line", "aaaa\nbb\n", 6, "bb\n"},
		{"off lands exactly on a boundary: drop no extra line", "aa\nbb\n", 3, "bb\n"},
		{"drops several lines", "a\nb\nc\nd\n", 3, "d\n"},
		{"no boundary at/after off: drop all", "aaaaaaaa\n", 3, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(dropOldestLines([]byte(tc.in), tc.capBytes))
			if got != tc.want {
				t.Errorf("dropOldestLines(%q, %d) = %q, want %q", tc.in, tc.capBytes, got, tc.want)
			}
			if got != "" && got[len(got)-1] != '\n' {
				t.Errorf("result %q must end on a line boundary", got)
			}
		})
	}
}

// TestTCPStatsdSink_PendingIsBounded: with a dead dial, pending must never exceed
// maxPendingBytes, and it must always end on a line boundary.
func TestTCPStatsdSink_PendingIsBounded(t *testing.T) {
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		return nil, errors.New("always refused")
	}, "sdpfx")
	defer func() { _ = s.Close() }()

	// Drive the writer directly: Submit would drop past the cap-8 channel.
	big := make([]*dto.MetricFamily, 0, 200)
	for i := 0; i < 200; i++ {
		big = append(big, counterFam(fmt.Sprintf("fam.%04d", i), float64(i)))
	}
	for i := 0; i < 2000; i++ {
		s.flush(big) // same goroutine: `pending` is writer-owned, so this is legal in-test
		if len(s.pending) > maxPendingBytes {
			t.Fatalf("pending grew to %d bytes, exceeding maxPendingBytes=%d", len(s.pending), maxPendingBytes)
		}
		if len(s.pending) > 0 && s.pending[len(s.pending)-1] != '\n' {
			t.Fatalf("pending does not end on a line boundary")
		}
	}
	if len(s.pending) == 0 {
		t.Fatal("pending is empty; the test never reached the cap and proves nothing")
	}
}
```

> `TestTCPStatsdSink_PendingIsBounded` calls `s.flush` from the **test** goroutine while the sink's own writer goroutine is parked in a failing dial. That is a data race on `s.pending` under `-race`. **Fix it properly:** construct the sink under test with a `newTCPStatsdSinkNoRun(dial, prefix)` package-private constructor that does everything `NewTCPStatsdSink` does **except** `go s.run()`. Add it in Step 3 and have `NewTCPStatsdSink` call it. Do not add a mutex around `pending` to make a test pass.

- [ ] **Step 2: Run to verify they fail**

```sh
go test ./internal/statssink/ -run 'DropOldestLines|PendingIsBounded' -count=1
```
Expected: FAIL — `undefined: dropOldestLines`, `undefined: maxPendingBytes`.

- [ ] **Step 3: Implement**

```go
// maxPendingBytes bounds the writer's accumulate buffer. The reference is
// effectively UNBOUNDED (SPEC-55 §11.5: >=45 snapshots / ~200 KB retained across
// a 30 s outage, no cap reached), so there is no value to mirror — this is an
// envoy-go choice (D-TCP-PENDING-CAP).
//
// 1 MiB. envoy-go's snapshot() walks the WHOLE frozen registry (~1200 stats x
// ~50 B ~= 60 KB per flush; the reference emits only USED stats, AMEND-TCP-
// USEDONLY), so 1 MiB retains ~16 flushes — the same order as
// MetricsServiceSink's defaultChannelCapacity of 8 in-flight batches — and 1 MiB
// is the tree's existing in-process buffer bound (ADR-0076's retry body buffer).
//
// On overflow the OLDEST WHOLE LINES are dropped and their counter increments are
// lost permanently: a DELIBERATE, documented departure from the reference's
// unbounded buffer. statsd is lossy by design, and an unbounded in-process buffer
// is a memory-growth hazard the tree refuses elsewhere.
const maxPendingBytes = 1 << 20

// dropOldestLines returns the longest suffix of p that (a) is at most capBytes
// long and (b) begins at a LINE boundary. Whole lines, never a partial line:
// `pending` carries no snapshot delimiters, so lines are the only unit its
// structure supports, and writing a partial line to the wire would corrupt a
// receiver's stream parse. Returns a subslice of p; the caller compacts.
func dropOldestLines(p []byte, capBytes int) []byte {
	if len(p) <= capBytes {
		return p
	}
	off := len(p) - capBytes // the first byte index we are allowed to keep
	if p[off-1] == '\n' {
		return p[off:] // off already begins a line; drop no extra line
	}
	idx := bytes.IndexByte(p[off:], '\n')
	if idx < 0 {
		return p[:0] // no boundary at or after off: the tail is one huge partial line
	}
	return p[off+idx+1:]
}
```

> `off > 0` always holds when `len(p) > capBytes >= 0`, so `p[off-1]` is in range. Add `if capBytes < 0 { capBytes = 0 }` only if `golangci-lint` demands it; otherwise leave the precondition documented.

Split the constructor:

```go
// newTCPStatsdSinkNoRun builds the sink WITHOUT starting the writer goroutine, so
// unit tests can drive flush() synchronously on the writer-owned `pending`.
func newTCPStatsdSinkNoRun(dial DialFunc, prefix string) *TCPStatsdSink {
	s := &TCPStatsdSink{
		ch:     make(chan []*dto.MetricFamily, defaultChannelCapacity),
		dial:   dial,
		prefix: prefix,
		delta:  newDeltaState(),
		done:   make(chan struct{}),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	return s
}

func NewTCPStatsdSink(dial DialFunc, prefix string) *TCPStatsdSink {
	s := newTCPStatsdSinkNoRun(dial, prefix)
	go s.run()
	return s
}
```

Rewrite `TestTCPStatsdSink_PendingIsBounded` to use `newTCPStatsdSinkNoRun` and drop its `defer s.Close()` (nothing to close — `Close` would block on `<-s.done`).

Insert the cap into `flush`, immediately after the `emitStatsdLines` block:

```go
	if len(s.pending) > maxPendingBytes {
		kept := dropOldestLines(s.pending, maxPendingBytes)
		s.pending = append(s.pending[:0], kept...)
		s.logDrop("pending buffer over maxPendingBytes, dropped oldest lines", nil)
	}
```

- [ ] **Step 4: Run to verify they pass**

```sh
go test ./internal/statssink/ -count=1 -v
go test ./internal/statssink/ -count=1 -race     # FULL package
```

- [ ] **Step 5: Deliberate-break liveness check**

- Remove the cap block → `TestTCPStatsdSink_PendingIsBounded` MUST fail on the `> maxPendingBytes` assertion.
- Make `dropOldestLines` return `p[off:]` unconditionally (splitting a line) → the table's "rounds UP" case and the boundary assertion MUST fail.

- [ ] **Step 6: Per-task gate + commit**

```bash
git add internal/statssink/
git commit -m "phase 55 Task 8: bound pending at 1 MiB, dropping OLDEST WHOLE LINES

D-TCP-PENDING-CAP resolved. The reference is unbounded, so there is no value to
mirror: 1 MiB retains ~16 whole-registry snapshots (~60 KB each), the same order
as MetricsServiceSink's cap-8 channel, and matches ADR-0076's 1 MiB buffer.

Whole LINES, not whole SNAPSHOTS: pending carries no snapshot delimiters, and a
partial line on the wire would corrupt the receiver's stream parse. The lost
increments are a deliberate, documented departure (BEHAVIOR_CONTRACT, Task 13)."
```

---

## Task 9: `Close` — drain, grace, and the `-race`-PROVEN unwedge (D-TCP-CLOSE)

**Files:**
- Modify: `internal/statssink/statsd_tcp.go` (`Close`)
- Modify: `internal/statssink/statsd_tcp_test.go`

**Interfaces:** no new exported surface. `Close() error` already satisfies `Sink`.

`Close` mirrors `MetricsServiceSink.Close` (`sink.go:140-153`): `close(ch)` → wait `done` up to `closeDrainGrace` → force → `<-done`. **The one substantive difference is the force step.** `cancel()` aborts a gRPC stream; it cannot touch a blocked `net.Conn.Write`. The unwedge is `conn.Close()`, reached through `connMu`.

- [ ] **Step 1: Write the failing tests**

```go
// blockingConn parks in Write until Close is called. This is the D-TCP-CLOSE
// hazard made deterministic: a real socket only blocks once its send buffer
// fills, which is timing-dependent and flaky.
type blockingConn struct {
	entered   chan struct{} // closed on the first Write
	unblock   chan struct{} // closed by Close
	closeOnce sync.Once
	enterOnce sync.Once
}

func newBlockingConn() *blockingConn {
	return &blockingConn{entered: make(chan struct{}), unblock: make(chan struct{})}
}

func (c *blockingConn) Write(b []byte) (int, error) {
	c.enterOnce.Do(func() { close(c.entered) })
	<-c.unblock // parks until Close
	return 0, errors.New("blockingConn: closed while writing")
}

func (c *blockingConn) Close() error {
	c.closeOnce.Do(func() { close(c.unblock) })
	return nil
}

func (c *blockingConn) Read([]byte) (int, error)         { return 0, errors.New("unused") }
func (c *blockingConn) LocalAddr() net.Addr              { return nil }
func (c *blockingConn) RemoteAddr() net.Addr             { return nil }
func (c *blockingConn) SetDeadline(time.Time) error      { return nil }
func (c *blockingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *blockingConn) SetWriteDeadline(time.Time) error { return nil }

var _ net.Conn = (*blockingConn)(nil)

// TestTCPStatsdSink_CloseUnwedgesBlockedWrite is the D-TCP-CLOSE PROOF. It must
// be run under -race: the writer goroutine holds `conn` and is parked inside
// Write; Close reaches that conn from the caller's goroutine and hard-closes it.
func TestTCPStatsdSink_CloseUnwedgesBlockedWrite(t *testing.T) {
	bc := newBlockingConn()
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return bc, nil }, "sdpfx")

	s.Submit([]*dto.MetricFamily{counterFam("a.b", 1)})
	select {
	case <-bc.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("the writer never entered Write")
	}

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(closeDrainGrace + 3*time.Second):
		t.Fatal("Close did not return: the blocked Write was never unwedged " +
			"(cancel() cannot interrupt a raw net.Conn — conn.Close() must)")
	}
	if elapsed := time.Since(start); elapsed < closeDrainGrace {
		t.Fatalf("Close returned in %v, before the %v drain grace; it must WAIT for the drain first", elapsed, closeDrainGrace)
	}
}

// TestTCPStatsdSink_CloseIsIdempotent
func TestTCPStatsdSink_CloseIsIdempotent(t *testing.T) {
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return conn, nil }, "sdpfx")
	s.Submit([]*dto.MetricFamily{counterFam("a.b", 1)})
	waitWrites(t, conn, 1)
	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestTCPStatsdSink_CloseDoesNotLeakARacingRedial: if the writer's dial returns a
// conn AFTER Close latched, that conn must be closed, not stashed.
func TestTCPStatsdSink_CloseDoesNotLeakARacingRedial(t *testing.T) {
	gate := make(chan struct{})
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) {
		<-gate // hold the writer inside dial until Close has latched
		return conn, nil
	}, "sdpfx")

	s.Submit([]*dto.MetricFamily{counterFam("a.b", 1)})
	closed := make(chan struct{})
	go func() { defer close(closed); _ = s.Close() }()
	time.Sleep(50 * time.Millisecond) // let Close enter its grace wait
	close(gate)                       // dial now returns, AFTER closed latched (or racing it)
	<-closed

	conn.mu.Lock()
	defer conn.mu.Unlock()
	if !conn.closed {
		t.Fatal("a conn dialed during/after Close was not closed: leaked")
	}
}

// TestTCPStatsdSink_CloseDrainsQueuedBatches: a clean Close (no blocked write)
// must flush what is already queued before returning.
func TestTCPStatsdSink_CloseDrainsQueuedBatches(t *testing.T) {
	conn := &fakeConn{}
	s := NewTCPStatsdSink(func(context.Context) (net.Conn, error) { return conn, nil }, "sdpfx")
	for i := 0; i < 3; i++ {
		s.Submit([]*dto.MetricFamily{counterFam("a.b", float64(i+1))})
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := len(conn.written()); n < 1 {
		t.Fatalf("Close returned with %d writes; queued batches must drain", n)
	}
}
```

> `TestTCPStatsdSink_CloseDoesNotLeakARacingRedial` uses a `time.Sleep` to order Close's latch against the dial's return. That is a *heuristic*, not a synchronization point — it can flake. **Preferred:** have the dial closure signal a channel *before* it blocks, and have the test wait on that instead of sleeping. If a deterministic ordering cannot be built, the test still asserts a real property (the conn ends up closed) under either interleaving, since `redial` closes the fresh conn when `s.closed` is set and `run`'s deferred `markClosedAndCloseConn` closes it otherwise. **Say so in a comment rather than leaving a naked sleep.**

- [ ] **Step 2: Run to verify they fail**

```sh
go test ./internal/statssink/ -run 'CloseUnwedges|CloseDoesNotLeak' -count=1 -race -timeout 60s
```
Expected: `TestTCPStatsdSink_CloseUnwedgesBlockedWrite` FAILS by **timing out** — Task 5's `Close` does an unconditional `<-s.done` with no grace and no unwedge, so it hangs forever behind the parked `Write`.

- [ ] **Step 3: Implement**

```go
// Close stops the writer goroutine and releases the connection. Idempotent and
// threadsafe via sync.Once.
//
// Mirrors MetricsServiceSink.Close (sink.go:140-153) — close(ch), wait for the
// drain up to closeDrainGrace, force, wait again — with ONE substantive
// difference. cancel() aborts a context-bound gRPC stream; it CANNOT interrupt a
// blocked net.Conn.Write. This is precisely where that precedent stops
// transferring (D-TCP-CLOSE). So the force step is TWO actions:
//
//   - cancel()                 aborts an in-flight DialContext;
//   - markClosedAndCloseConn() hard-closes the live conn, which unwedges a
//     blocked Write with an error, AND latches `closed` so the writer's next
//     redial gives up instead of leaking a fresh conn.
//
// The writer never holds connMu across the write, so this can never deadlock.
// Proven under -race by TestTCPStatsdSink_CloseUnwedgesBlockedWrite.
func (s *TCPStatsdSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
		select {
		case <-s.done:
		case <-time.After(closeDrainGrace):
			s.cancel()
			s.markClosedAndCloseConn()
			<-s.done
		}
		s.cancel()                 // release the context (idempotent; satisfies vet lostcancel)
		s.markClosedAndCloseConn() // idempotent; the writer's own defer usually got here first
	})
	return nil
}
```

`run`'s `defer s.markClosedAndCloseConn()` (Task 5) already handles the clean path.

- [ ] **Step 4: Run to verify they pass — under `-race`, FULL package**

```sh
go test ./internal/statssink/ -count=1 -race -timeout 120s
go test ./internal/statssink/ -count=1 -race -run TestTCPStatsdSink_CloseUnwedgesBlockedWrite -v
```
Both must PASS with **no `WARNING: DATA RACE`** in the output. A `-run`-subset `-race` is NOT sufficient evidence here (`reference_full_suite_race_after_background_mutator`): the full-package run is the gate.

- [ ] **Step 5: Deliberate-break liveness check**

| Break | Expected |
|---|---|
| Drop `markClosedAndCloseConn()` from the force branch (keep `cancel()`) | `TestTCPStatsdSink_CloseUnwedgesBlockedWrite` MUST **time out** — the proof that `cancel()` alone is insufficient |
| Read/write `s.conn` directly from `Close` without `connMu` | `go test -race` MUST report `WARNING: DATA RACE` on `TCPStatsdSink.conn` |
| Drop the `if s.closed` check in `redial` | `TestTCPStatsdSink_CloseDoesNotLeakARacingRedial` MUST fail |

Record the *actual* `-race` output of break 2 in the commit message — it is the evidence that the mutex is load-bearing, not decorative.

- [ ] **Step 6: Per-task gate + commit**

```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/
git commit -m "phase 55 Task 9: Close drains, then hard-closes the conn to unwedge a blocked Write

D-TCP-CLOSE resolved and -race-PROVEN, not argued. cancel() aborts an in-flight
DialContext but cannot interrupt a blocked net.Conn.Write -- exactly where the
MetricsServiceSink precedent stops transferring. connMu guards the conn FIELD
(never the I/O: the writer copies it out and Writes unlocked), so Close can
hard-close it from another goroutine, which returns the parked Write with an
error, and latches `closed` so a racing redial gives up instead of leaking.

Proven by blockingConn under a FULL-package -race run. Deliberate break: removing
markClosedAndCloseConn from the force branch makes the test TIME OUT; removing
connMu makes -race report a DATA RACE on TCPStatsdSink.conn."
```

---

## Task 10: `main.go` — the TCP build arm

**Files:**
- Modify: `cmd/envoy-go/main.go` — the statsd loop inside the sink block (`:206-216`)

**Interfaces:**
- Consumes: `bootstrap.StatsdSinkConfig.TCPClusterName` (Task 3), `statssink.NewTCPStatsdSink` / `statssink.DialFunc` (Task 5), `(*cluster.Cluster).DialSink` (Task 1), `cm` (`main.go:105`).
- Produces: a `statssink.Sink` appended to `statsSinks`, closed by the existing defer at `main.go:243-246`.

The dial closure re-looks-up the cluster **per dial**, mirroring `grpcclient.go:128-141` (so a future xDS-CDS hot-reload observes the latest manager state).

- [ ] **Step 1: Replace the statsd loop**

Current (`main.go:206-216`), for reference — it has no arms:

```go
		for _, cfg := range bs.StatsdSinkConfigs {
			sink, err := statssink.NewStatsdSink(cfg.UDPAddress, cfg.Prefix)
			if err != nil {
				log.Fatalf("statssink: statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
```

Replace with:

```go
		// Phase 48 (ADR-0265): the statsd UDP stats sink — synchronous, no
		// goroutine. Phase 55 (ADR-0272): the statsd TCP transport — a bounded-
		// channel + writer-goroutine sink, the statsd sinks' FIRST background
		// mutator. StatsdSinkConfig is a TAGGED UNION over statsd_specifier:
		// exactly one of TCPClusterName / UDPAddress is set (bootstrap.go).
		for _, cfg := range bs.StatsdSinkConfigs {
			if cfg.TCPClusterName != "" {
				name := cfg.TCPClusterName
				// Defensive: bootstrap already rejected an unknown cluster at parse
				// time against static_resources.clusters (the complete set today —
				// no CDS). When CDS lands, THIS becomes the real check.
				if _, ok := cm.Get(name); !ok {
					log.Fatalf("statssink: statsd tcp sink: unknown cluster %q", name)
				}
				// Re-look-up the cluster per dial so the latest cluster-manager
				// state is observed (the grpcclient.go:128-141 idiom). DialSink is
				// the UNACCOUNTED dial: no max_connections permit, no upstream_cx_*
				// (AMEND-TCP-CXSTATS).
				statsSinks = append(statsSinks, statssink.NewTCPStatsdSink(func(ctx context.Context) (net.Conn, error) {
					cl, ok := cm.Get(name)
					if !ok {
						return nil, fmt.Errorf("statssink: statsd tcp: cluster %q vanished", name)
					}
					return cl.DialSink(ctx)
				}, cfg.Prefix))
				continue
			}
			sink, err := statssink.NewStatsdSink(cfg.UDPAddress, cfg.Prefix)
			if err != nil {
				log.Fatalf("statssink: statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
```

Add `"net"` and `"fmt"` to the import block if absent (check first — `main.go` very likely already imports `fmt`).

> **Loop-variable capture:** `go.mod` declares `go 1.23.0`, so loop variables are per-iteration and `cfg` would be safe to capture directly. `name := cfg.TCPClusterName` is retained anyway: it names the captured value at the closure boundary and survives a future `go` directive downgrade.

- [ ] **Step 2: Verify the sink is closed at shutdown**

Read `main.go:234-246`. The existing defer waits on `<-flusherDone` (so the Flusher has stopped `Submit`ting) and then calls `s.Close()` on every `statsSinks` element. `TCPStatsdSink.Close()` satisfies `Sink`, so it is picked up with **no change**. Confirm by reading, and add nothing.

The doc comment immediately above the statsd loop (`main.go:209-210`) says "*Synchronous (no goroutine), so it adds no background mutator to the shutdown drain*". **That is now false for the TCP arm.** Update it:

> **There is a SECOND, near-identical copy of that sentence at `main.go:221-223`, on the `dog_statsd` loop.** That one stays TRUE — `DogStatsdSink` is still synchronous. Do **not** edit it. Only the statsd copy at `:209-210` changes.

```go
	// ... the statsd UDP and dog_statsd sinks are SYNCHRONOUS (no goroutine). The
	// statsd TCP sink (ADR-0272) is NOT: it runs a writer goroutine, so it joins
	// metrics_service in the shutdown drain — the <-flusherDone wait below is what
	// makes closing its channel safe.
```

- [ ] **Step 3: Build + full-tree vet**

```sh
go build ./... && go vet ./... && gofmt -l cmd/envoy-go/ && golangci-lint run ./cmd/...
```

- [ ] **Step 4: Smoke-test the boot path by hand**

There is no unit test for `main.go`. Prove the wiring with a real boot against a real listener:

```sh
# terminal 1 — a throwaway line-printing TCP receiver
python3 -c "
import socket
s=socket.socket(); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
s.bind(('127.0.0.1',18125)); s.listen(1)
c,_=s.accept(); print('ACCEPTED', flush=True)
f=c.makefile('rb')
for i,line in enumerate(f):
    print(line.decode().rstrip(), flush=True)
    if i>5: break
"
```
Write a minimal bootstrap to the scratchpad with `node: {id: n, cluster: c}`, one `c_statsd` STATIC cluster pointing at `127.0.0.1:18125`, `stats_flush_interval: 0.5s`, and the `tcp_cluster_name: c_statsd` sink. Boot `go run ./cmd/envoy-go -c <that file>`.

Expected on terminal 1: `ACCEPTED` **exactly once**, then `\n`-terminated `envoy.*:N|c` / `|g` lines arriving every 0.5 s. Then, still on the running proxy:

```sh
curl -s localhost:<admin>/stats | grep 'cluster.c_statsd.upstream_cx'
```
Expected: `cluster.c_statsd.upstream_cx_total: 0` and `cluster.c_statsd.upstream_cx_active: 0` — the AMEND-TCP-CXSTATS property, live, end-to-end. **Paste this output into the commit message.** Use a fresh receiver process per arm (`feedback_probe_fresh_container_per_arm` — a reused receiver reports cumulative ACCEPT counts).

Also boot with `tcp_cluster_name: c_nope` and confirm the byte-stable parse reject, and with the `node:` block deleted and confirm the node reject.

- [ ] **Step 5: Commit**

```bash
git add cmd/envoy-go/main.go
git commit -m "phase 55 Task 10: main.go builds the statsd TCP sink over a DialSink closure

StatsdSinkConfig is a tagged union, so the statsd loop gains a two-arm branch.
The dial closure re-looks-up the cluster per dial (the grpcclient idiom) and
calls the UNACCOUNTED DialSink, keeping internal/statssink free of any
internal/cluster import.

Corrects the now-false comment claiming every statsd sink is synchronous: the TCP
arm runs a writer goroutine and joins the shutdown drain behind <-flusherDone.

Hand-verified end-to-end: one ACCEPT, '\n'-terminated lines every 0.5s, and
  cluster.c_statsd.upstream_cx_total: 0
  cluster.c_statsd.upstream_cx_active: 0"
```

---

## Task 11: Fixture `0098-stats-sink-statsd-tcp`

**Files:**
- Create: `test/fixtures/0098-stats-sink-statsd-tcp/{envoy.yaml,envoy-go.yaml,expectations.yaml,README.md}`
- Create: `test/fixtures/0098-stats-sink-statsd-tcp/driver/driver.go`
- Modify: `test/differential/runner_test.go` — ONE blank-import line, after the `0097` line at `:124`

**Interfaces:** consumes `statsdrecv.NewTCPAtAddr` / `ConnCount` / `UnparsedCount` (Task 2). Implements `fixture.Driver`, `fixture.BackendKindAware`, `fixture.StatsAsserter`.

> **Discovery is directory-scan based** (`discoverFixtures`, `runner_test.go:1416-1453`) and there is **no hand-rolled fixture-count slice** to bump (checked; `reference_fixture_workload_constant_desync` does not bite here). The blank import is the only registration edit.

### Design

Clone the `0092-stats-sink-statsd` shape (`driver/driver.go`, 629 lines). **Read it in full first.** The deltas:

| Aspect | 0092 (UDP) | 0098 (TCP) |
|---|---|---|
| receiver | `statsdrecv.NewAtAddr` × 2 | **`statsdrecv.NewTCPAtAddr` × 2** |
| port alloc | `mustAllocateUDPPort` | **`mustAllocateTCPPort`** (`net.Listen("tcp","127.0.0.1:0")` → `Addr().(*net.TCPAddr).Port` → `Close()`) |
| sink config | `address.socket_address` | **`tcp_cluster_name: c_statsd`** + a `c_statsd` STATIC cluster |
| `node` | absent (*"NO node field is needed"*) | **REQUIRED on BOTH sides** — the reference will not boot without `node.id` + `node.cluster` (AMEND-TCP-NODE) |
| assertions | cross-side subset + gauge | same, **plus** two SUBJECT-EXACT ones |

> **Do not copy 0092's `// NO node field is needed — statsd carries no proxy identifier.` comment** (`envoy.yaml:17`, `envoy-go.yaml:19`). It is true for UDP and **false for TCP**. Replace it with a comment naming AMEND-TCP-NODE.

**Reachability** (`reference_host_gateway_ip_docker_desktop`): the reference container's `c_statsd` endpoint is the **host-gateway literal IP**; the subject's is `127.0.0.1`. 0092 **inlines** a private `hostGatewayIP(ctx)` (`driver.go:521-589`) rather than calling `differential.HostGatewayIP`, to avoid an import cycle. Copy that inlined helper verbatim.

**BackendCount** (`reference_differential_backendcount_min_one`): `return 1`. `c_backend` is a real `fixture.HTTPFixedBody` backend, so the constraint is satisfied naturally.

**The admin-interface wire-name collision does NOT bite here — dismissal, on the record.** The BRAINSTORM flagged `reference_admin_interface_wire_name_collision` as a watch-item, so it must be closed explicitly rather than silently. That collision only bites **tag-hoisting** sinks (`dog_statsd`, or `metrics_service` with `emit_tags_as_labels`), where `ExtractTags` strips the `stat_prefix` segment out of the name and into a tag, collapsing the admin listener's HCM and the test listener's HCM onto **one wire name** that differs only by tag value — which is why phase 49 needed the exact-tag-match `DeltaSumTagged` accessor. **Plain statsd hoists no tags:** it emits the raw dotted name, so `sdpfx.http.hcm_local.downstream_rq_total` and `sdpfx.http.admin.downstream_rq_total` are distinct map keys. The readiness probe lands only on `http.admin.*` and cannot contaminate the asserted `http.hcm_local.*` subset. `0092` has run clean with plain `DeltaSum` for exactly this reason. **`0098` uses plain `DeltaSum`, not `DeltaSumTagged`.**

### Assertions

**CROSS-SIDE** (both snapshots, after `pollSubset` converges AND `awaitFurtherFlushes(ctx, srv, subsetNames[0], 2)` — `reference_delta_sink_differential_stability_barrier`):

```go
var subsetNames = []string{
	prefix + ".cluster." + backendName + ".upstream_rq_total",
	prefix + ".http." + statPrefix + ".downstream_rq_total",
	prefix + ".http." + statPrefix + ".downstream_rq_2xx",
}
// each: present, |c delta-SUM == numReq (7), and STILL == 7 after >=2 further flushes
var gaugeName = prefix + ".cluster." + backendName + ".membership_total" // absolute == 1
```

`membership_total`, not `membership_healthy` — envoy-go registers the latter only on health-checked clusters, and `c_backend` has no health check (`reference_membership_total_vs_healthy_gauge`).

**SUBJECT-EXACT** (the reference's values are RECORDED, never asserted):

```go
subj.connCount   == 1   // one long-lived connection; no per-flush redial.
                        // Reference is 2 (main + the |ms-only worker-timer sink);
                        // envoy-go has no histograms (ADR-0060) so it can NEVER open
                        // conn #2 — cross-side equality is infeasible BY THE HISTOGRAM
                        // BOUNDARY (AMEND-TCP-CONNCOUNT), not by uncertainty.
subj.unparsed    == 0   // envoy-go emits only |c and |g. The reference legitimately
                        // emits 35 |ms lines. This is the LIVENESS signal for break (b).
subj.cxTotal     == 0   // sdpfx.cluster.c_statsd.upstream_cx_total: DialSink took the
                        // UNACCOUNTED path (AMEND-TCP-CXSTATS). Subject-only because the
                        // reference never emits this line at all (AMEND-TCP-USEDONLY:
                        // never-incremented counters are omitted from the wire).
```

**UNASSERTED, both sides:** the whole line SET (AMEND-TCP-USEDONLY makes it structurally different — assert NAMED SUBSETS, never whole sets, per `reference_stats_sink_emits_used_only`); `|ms` timers; non-deterministic gauges; flush cadence; write granularity (not observable to a line-parsing stream receiver).

- [ ] **Step 1: Write `envoy.yaml` (reference)**

Start from `0092/envoy.yaml` (100 lines). Changes:

```yaml
# AMEND-TCP-NODE (SPEC-55 §3.7, §11.6): the reference REFUSES TO BOOT a
# tcp_cluster_name statsd sink unless BOTH node.id and node.cluster are set --
# either alone fails with the identical message. The UDP address sink boots with
# no node at all (control probe), so the requirement is TCP-SPECIFIC.
node:
  id: {{.NodeID}}
  cluster: {{.NodeCluster}}
```
```yaml
stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": type.googleapis.com/envoy.config.metrics.v3.StatsdSink
      tcp_cluster_name: c_statsd
      prefix: {{.Prefix}}
stats_flush_interval: 0.5s
```
and a new cluster alongside `c_backend`:
```yaml
  - name: c_statsd
    connect_timeout: 1s
    type: STATIC
    load_assignment:
      cluster_name: c_statsd
      endpoints:
        - lb_endpoints:
            - endpoint:
                address:
                  socket_address: { address: {{.StatsdHost}}, port_value: {{.StatsdPort}} }
```
`StatsdHost` = the host-gateway literal IP for the reference; `127.0.0.1` for the subject.

- [ ] **Step 2: Write `envoy-go.yaml` (subject)** — the same, with `StatsdHost` = `127.0.0.1` and `c_backend` as `STATIC` on `127.0.0.1` (0092's subject shape).

- [ ] **Step 3: Write `driver/driver.go`**

Copy `0092/driver/driver.go` and apply. **`0092` already declares `subsetNames` (`driver.go:122-126`), `gaugeName` (`:134`), `fixtureName`, `refListenerPort`, `numReq`, `prefix`, `statPrefix`, and `backendName`. REPLACE those declarations — do not append, or the package will not compile on a duplicate declaration.** Add `"log"` to the driver's import block (`AssertStats` uses `log.Printf`; `fixture.TB` has no `Logf`).

```go
const fixtureName = "0098-stats-sink-statsd-tcp"
const refListenerPort = 10098
const numReq = 7
const prefix = "sdpfx"
const statPrefix = "hcm_local"
const backendName = "c_backend"
const statsdClusterName = "c_statsd"
const nodeID = "sd-node"
const nodeCluster = "sd-cluster"

var cxTotalName = prefix + ".cluster." + statsdClusterName + ".upstream_cx_total"

func init() { fixture.RegisterFixture(fixtureName, &statsdTCPDriver{}) }

type sideSnapshot struct {
	sums      map[string]float64
	gaugeVal  float64
	gaugeOK   bool
	connCount int
	unparsed  int
	cxTotal   float64
	cxTotalOK bool
}
```

`mustAllocateTCPPort` replaces `mustAllocateUDPPort`:

```go
// mustAllocateTCPPort reserves an ephemeral TCP port and releases it, so the
// receiver can bind it on 0.0.0.0 (reachable from the reference container via
// the host gateway). Racy in principle, matching 0092's UDP precedent.
func mustAllocateTCPPort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("%s: allocate tcp port: %v", fixtureName, err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func mustStartReceiver(port int) *statsdrecv.Server {
	srv, err := statsdrecv.NewTCPAtAddr(fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		panic(fmt.Sprintf("%s: start tcp statsd receiver on %d: %v", fixtureName, port, err))
	}
	return srv
}
```

`driveSide` extends 0092's (`:317-359`) snapshot block:

```go
	if err := pollSubset(ctx, srv); err != nil {
		return sideSnapshot{}, err
	}
	// STABILITY BARRIER (reference_delta_sink_differential_stability_barrier):
	// a burst-all-requests delta sink cannot tell a DELTA from an ABSOLUTE — the
	// first flush's delta EQUALS the absolute value. Require >=2 further flushes
	// and re-read: under correct per-flush deltas the idle counters emit 0 and the
	// SUM stays numReq; under absolute values it grows by numReq each flush.
	if err := awaitFurtherFlushes(ctx, srv, subsetNames[0], 2); err != nil {
		return sideSnapshot{}, err
	}
	snap := sideSnapshot{sums: make(map[string]float64, len(subsetNames))}
	for _, name := range subsetNames {
		if v, ok := srv.DeltaSum(name); ok {
			snap.sums[name] = v
		}
	}
	snap.gaugeVal, snap.gaugeOK = srv.Gauge(gaugeName)
	snap.connCount = srv.ConnCount()
	snap.unparsed = srv.UnparsedCount()
	snap.cxTotal, snap.cxTotalOK = srv.DeltaSum(cxTotalName)
	return snap, nil
```

`AssertStats` gains the subject-exact block:

```go
func (d *statsdTCPDriver) AssertStats(t fixture.TB, _, _ string) {
	t.Helper()
	d.mu.Lock()
	ref, subj := d.ref, d.subj
	d.mu.Unlock()

	assertPayloadParity(t, "reference", ref)
	assertPayloadParity(t, "subject", subj)

	// ---- SUBJECT-EXACT (the reference's values are RECORDED, not asserted) ----
	//
	// fixture.TB (fixture.go:64-68) exposes ONLY Errorf/Fatalf/Helper — it
	// deliberately does not import "testing", so there is NO t.Logf. Record the
	// reference's values with the stdlib logger.
	log.Printf("%s reference (RECORDED, not asserted): connCount=%d unparsed=%d "+
		"(2 conns: main + the |ms-only worker-timer sink; 35 |ms lines are legitimately unparsed)",
		fixtureName, ref.connCount, ref.unparsed)

	// ONE long-lived connection. A per-flush redial yields one conn per flush.
	if subj.connCount != 1 {
		t.Errorf("subject ConnCount = %d, want exactly 1 (one long-lived connection; "+
			"a per-flush redial inflates this)", subj.connCount)
	}
	// envoy-go emits only |c and |g. A non-zero count means the receiver saw a line
	// it could not account for — the signature of \n-SEPARATED rather than
	// \n-TERMINATED framing concatenating two lines across a flush boundary.
	if subj.unparsed != 0 {
		t.Errorf("subject UnparsedCount = %d, want 0 (envoy-go emits only |c and |g; "+
			"a non-zero count means the flush framing is not '\\n'-TERMINATED)", subj.unparsed)
	}
	// DialSink took the UNACCOUNTED path. Subject-only: the reference never emits
	// this line at all (AMEND-TCP-USEDONLY — it omits never-incremented counters).
	if !subj.cxTotalOK {
		t.Errorf("subject: %s absent; envoy-go emits every registered stat, so it must be present", cxTotalName)
	} else if subj.cxTotal != 0 {
		t.Errorf("subject %s = %v, want 0 (DialSink must take NO upstream_cx_* accounting; "+
			"a value of 1 means Cluster.Dial was used instead)", cxTotalName, subj.cxTotal)
	}
}

func assertPayloadParity(t fixture.TB, side string, snap sideSnapshot) {
	t.Helper()
	if len(snap.sums) == 0 {
		t.Fatalf("%s: no statsd lines received at all", side)
	}
	for _, name := range subsetNames {
		v, ok := snap.sums[name]
		if !ok {
			t.Errorf("%s: counter %q absent from the emitted lines", side, name)
			continue
		}
		if v != float64(numReq) {
			t.Errorf("%s: %q |c delta-SUM = %v, want %d (still %d after >=2 further flushes)",
				side, name, v, numReq, numReq)
		}
	}
	if !snap.gaugeOK {
		t.Errorf("%s: gauge %q absent", side, gaugeName)
	} else if snap.gaugeVal != 1 {
		t.Errorf("%s: gauge %q = %v, want 1", side, gaugeName, snap.gaugeVal)
	}
}
```

Keep 0092's `fireProbe`, `pollSubset`, `subsetConverged`, `describeSubset`, `awaitFurtherFlushes`, `ProbeAdmin`, `closeServers`, `hostGatewayIP`, `fixtureDir`, `mustReadFixtureFile`, `mustRender` **verbatim**. `closeServers` calls `Close()`, which for a TCP receiver hard-closes the listener and every accepted conn (Task 2) — **never a graceful stop**, because the sink holds its connection open for the process lifetime (`reference_periodic_sink_differential_two_receivers`).

Compile-time assertions at the foot of the file:
```go
var (
	_ fixture.Driver           = (*statsdTCPDriver)(nil)
	_ fixture.BackendKindAware = (*statsdTCPDriver)(nil)
	_ fixture.StatsAsserter    = (*statsdTCPDriver)(nil)
)
```

> **`StatsAsserter`, NOT `SubjectAsserter`** (`reference_differential_asserter_dispatch`). `SubjectAsserter` runs only on the reference-less path; a cross-side fixture that used it would have DEAD, vacuously-passing assertions. `0092` uses `StatsAsserter`; match it.

- [ ] **Step 4: Register the fixture**

Add one line to `test/differential/runner_test.go`, after `:124`:
```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0098-stats-sink-statsd-tcp/driver"
```

- [ ] **Step 5: Write `README.md`** — what is asserted cross-side, what is subject-exact, what is UNASSERTED and why (the `|ms` histogram boundary; the used-only line-set difference; write granularity).

- [ ] **Step 5b: Write `expectations.yaml`**

`0092/expectations.yaml` (6728 bytes) is **ADR-0019 PROSE**, not a machine-parsed file — its own header says *"the driver is the enforcer via `AssertStats`; this file is documentation."* Nothing in `test/differential/*.go` loads it. 77 of the 99 fixtures carry one; `0098` must too, for consistency.

Mirror `0092`'s structure and record: the three cross-side `|c` delta-SUM subset names and their expected `7` behind the ≥2-further-flush stability barrier; `membership_total == 1`; the three SUBJECT-EXACT assertions (`ConnCount == 1`, `UnparsedCount == 0`, `c_statsd.upstream_cx_total == 0`) and, for each, **why the reference's value is recorded rather than asserted**; and the UNASSERTED surfaces (whole line set, `|ms` timers, flush cadence, write granularity).

- [ ] **Step 6: Run the fixture**

```sh
go test ./test/differential/ -run 'TestDifferential/0098-stats-sink-statsd-tcp' -count=1 -v -timeout 15m
```
> `-run '0098'` matches **ZERO** subtests and reports a vacuous PASS (`reference_differential_run_selector`). Use the full subtest path, exactly as written.

Then the full suite, to catch cross-fixture interference:
```sh
go test ./test/differential/ -count=1 -timeout 60m
```
If an **unrelated** fixture fails with `subject ready: EOF`, that is the known startup race under suite load, not a regression (`reference_differential_fullsuite_startup_flake`): isolate-re-run that fixture, then re-run the full suite.

- [ ] **Step 7: Prove the fixture's counts are not desynced**

`numReq = 7` appears in `fireProbe`'s loop bound and in `assertPayloadParity`'s comparison. There is **no hand-rolled count slice** in this driver — but confirm with `grep -n 'numReq' driver/driver.go` that every use derives from the constant (`reference_fixture_workload_constant_desync`).

- [ ] **Step 8: Commit**

```bash
git add test/fixtures/0098-stats-sink-statsd-tcp/ test/differential/runner_test.go
git commit -m "phase 55 Task 11: fixture 0098-stats-sink-statsd-tcp

Cross-side payload parity (3 |c delta-SUMs == 7 behind a >=2-further-flush
stability barrier, + membership_total == 1) plus THREE subject-exact transport
assertions: ConnCount == 1, UnparsedCount == 0, and c_statsd.upstream_cx_total == 0.

Cross-side ConnCount equality is infeasible BY THE HISTOGRAM BOUNDARY: the
reference opens a second, |ms-only worker-timer connection that envoy-go (no
histograms, ADR-0060) can never open. The reference's values are RECORDED via
log.Printf (fixture.TB has no Logf), never asserted.

Both bootstraps carry node:{id,cluster} -- the reference will not boot a
tcp_cluster_name sink without it (AMEND-TCP-NODE), unlike 0092's UDP sink."
```

---

## Task 12: The four deliberate breaks — controller-reperformed

**Files:** none committed. This task produces **evidence**, recorded in the Task-13 commit body and in `STATE.md`.

Per `feedback_subagent_autocommit_claudemd`, the **CONTROLLER** performs this task itself; it is the one step a subagent must not be trusted with, because a subagent that writes the break and the assertion can make both agree.

Every run uses `-count=1` and the full subtest path.

```sh
BREAK="go test ./test/differential/ -run 'TestDifferential/0098-stats-sink-statsd-tcp' -count=1 -timeout 15m"
```

| # | Break | File | Expected failure |
|---|---|---|---|
| **(a)** | Redial on every flush: in `flush`, replace the `if s.currentConn() == nil` guard with `s.dropConn()` before every write | `internal/statssink/statsd_tcp.go` | `subject ConnCount = N, want exactly 1` (N ≈ flush count) |
| **(b)** | Emit `\n`-SEPARATED: change the `emit` callback to prepend `'\n'` when `len(s.pending) > 0` instead of always appending it | `internal/statssink/statsd_tcp.go` | `subject UnparsedCount = N, want 0` |
| **(c)** | Emit ABSOLUTE counters: replace `s.delta.apply(batch)` with `batch` | `internal/statssink/statsd_tcp.go` | `|c` delta-SUM `= 14` (or more), want `7`, on **both** sides' payload parity |
| **(d)** | Use the accounted dial: in `main.go`, call `cl.Dial(ctx)` (discarding the `Endpoint`) instead of `cl.DialSink(ctx)` | `cmd/envoy-go/main.go` | `subject sdpfx.cluster.c_statsd.upstream_cx_total = 1, want 0` |

**Break (c) carries an extra obligation the SPEC calls out explicitly.** Break (c) **PASSES without the stability barrier**: on the very first flush a delta equals the absolute value, so a poll that stops at "the SUM reached 7" cannot tell them apart. Therefore:

1. Apply break (c). Confirm the fixture FAILS.
2. **Additionally** delete the `awaitFurtherFlushes(ctx, srv, subsetNames[0], 2)` call from `driveSide` while break (c) is still applied. Confirm the fixture now **PASSES**.
3. Restore both. Confirm it FAILS again with the barrier and PASSES clean.

Step 2 is the proof that the barrier — not something else — is what makes break (c) bite (`reference_delta_sink_differential_stability_barrier`).

**Break (b) carries a symmetric obligation.** A naive break-(b) assertion via the counter subset alone would be **vacuous** whenever the two corrupted flush-boundary names fall outside `subsetNames` (see Task 2's rationale). Therefore:

1. Apply break (b). Confirm the failure message is the `UnparsedCount` one.
2. **Additionally** confirm that with break (b) applied, the three `subsetNames` delta-SUMs may still all read `7` — i.e. that the SPEC's original "the counter-subset lookups miss" was not a reliable signal. Record whatever you actually observe; if the subset assertion *does* also fire, say so. Do not assert what you did not see.

**GIT HYGIENE** (`feedback_subagent_worktree_detach`): revert every break with `git restore <file>`. Never `git checkout <sha>`; never `git commit --amend`. After each revert, run `git branch --show-current` and `git status --porcelain` and confirm you are on `phase-55-...` with a clean tree.

- [ ] Break (a) applied → FAILS with the expected message → restored → PASSES
- [ ] Break (b) applied → FAILS with the `UnparsedCount` message → subset-vacuity observation recorded → restored → PASSES
- [ ] Break (c) applied → FAILS → barrier removed → PASSES (the masking proof) → both restored → FAILS again → clean → PASSES
- [ ] Break (d) applied → FAILS with `upstream_cx_total = 1` → restored → PASSES
- [ ] Final: `git status --porcelain` is empty; `$BREAK` PASSES

---

## Task 13: ADR-0272 + `BEHAVIOR_CONTRACT.md`

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` — append `## ADR-0272`
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`

Per ADR-0044, the **§Context** body is the SPEC §13 draft, transcribed; **§Decision** and **§Consequences** are written here, in place, against the as-built code.

- [ ] **Step 1: Confirm the ADR number is still free**

```sh
grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1   # expect ADR-0271
```
If it prints `ADR-0272` or later, another phase landed first — **stop and re-derive the number**, do not overwrite.

- [ ] **Step 2: Write ADR-0272**

§Context: transcribe SPEC §13 verbatim.

§Decision must record, each in one paragraph:
1. The bounded-channel + writer-goroutine shape and why phase 48's synchronous `Submit` was unavailable (`SPEC-48.md:78`'s UDP licence).
2. `Cluster.DialSink` — the unaccounted dial; the `dialAndTLS` extraction that keeps it from drifting from `Dial`; the `DialFunc` seam that keeps `internal/statssink` free of `internal/cluster`.
3. The **line-aligned resume** and the exact reason it is neither lossy nor duplicating — including its dependence on the receiver discarding an incomplete trailing line at EOF.
4. The **bounded `pending`** (1 MiB, drop-oldest whole lines) as a DELIBERATE, documented departure from the reference's unbounded accumulate.
5. `Close`'s two-part force step and why `cancel()` alone cannot unwedge a raw `net.Conn` — the precise point at which the `MetricsServiceSink`/ADR-0262 precedent stops transferring.
6. Both new rejects (node-required, unknown-cluster), that both are reference PARITY, that they live at parse time, and that their **relative precedence when both are invalid was not probed and is not claimed**.
7. The `StatsdSinkConfig` tagged union, and that the `statsd_specifier` oneof **CLOSES** with this row (exactly two arms; `reference_strict_reject_sibling_typeurl_gap` is discharged at the oneof level — the independent TypeURL-level `parseStatsSinks` dispatch still rejects unknown sink TypeURLs).

§Consequences must record:
- Stat surface **1200 → 1200 (+0)**. The stats cluster's `upstream_cx_total`/`upstream_cx_active` stay at **0 on both sides** — stronger than the BRAINSTORM's "a new stat INSTANCE, not a new NAME" reasoning, because there is now **no self-reference at all**.
- A `max_connections`-capped stats cluster does **NOT** throttle the sink (reference parity, probed at `max_connections: 0`).
- `least_request` cannot see the sink's connection (the accepted `PickEndpoint` cost).
- Coverage boundaries, each named as a boundary and not a gap: `|ms` timers (ADR-0060); the used-only line-set difference (assert named subsets only — `reference_stats_sink_emits_used_only`); cross-side `ConnCount` infeasible by the histogram boundary; the line-aligned resume is UNIT-proven, not differentially proven.
- The bounded-`pending` departure and its cost (dropped increments on sustained outage).
- `statsdrecv`'s `MaxLinesInAnyDatagram` / `LinesInDatagram` are unpopulated on the TCP path — documented, not silently divergent.
- **`TCPStatsdSink` is the statsd sinks' first background mutator** ⇒ full-package `-race` is now mandatory for `internal/statssink`.
- When CDS lands, the static-cluster-membership reject must move to the cluster manager.

- [ ] **Step 3: `BEHAVIOR_CONTRACT.md`** — add a `Phase 55 — 1200 → 1200 (+0)` block covering: the TCP transport (framing, delta semantics, one long-lived conn); the node precondition; the unaccounted-dial semantics and the `max_connections` non-throttling consequence; the bounded-`pending` departure; the `|ms` and used-only coverage boundaries.

- [ ] **Step 4: Verify the stat surface really is unchanged**

```sh
grep -c '' docs/envoy-go/BEHAVIOR_CONTRACT.md   # sanity
grep -rn 'stats.NewCounter\|stats.NewGauge' internal/statssink/statsd_tcp.go   # MUST print NOTHING
```

- [ ] **Step 5: Commit**

```bash
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 55 Task 13: ADR-0272 + BEHAVIOR_CONTRACT delta (stat surface 1200 -> 1200, +0)"
```

---

## Task 14: The six-gate completion bundle (ADR-0052 atomic landing)

**Files:** `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/STATE.md`, `next-prompt.txt`.

The controller runs every gate on the **frozen HEAD** after squashing, not on a subagent's intermediate commit.

- [ ] **Gate 1 — build, vet, fmt, lint (whole tree)**
```sh
go build ./... && go vet ./... && gofmt -l . | grep -v '^\.worktrees/' ; golangci-lint run ./...
```
`gofmt -l` must print nothing outside `.worktrees/`.

- [ ] **Gate 2 — FULL-package `-race` on the three named packages**

This is the gate `reference_full_suite_race_after_background_mutator` exists for. A `-run`-subset `-race` does **not** substitute.
```sh
go test ./internal/statssink/ -count=1 -race -timeout 120s
go test ./internal/cluster/  -count=1 -race -timeout 300s
go test ./test/differential/ -count=1 -race -timeout 90m
```

- [ ] **Gate 3 — the whole unit suite**
```sh
go test ./... -count=1 -timeout 30m
```

- [ ] **Gate 4 — the differential suite, all 100 fixtures**
```sh
go test ./test/differential/ -count=1 -timeout 90m
```
On an `subject ready: EOF` failure in an **unrelated** fixture: isolate-re-run it, then re-run the full suite (`reference_differential_fullsuite_startup_flake`).

- [ ] **Gate 5 — fuzz smoke**
```sh
go test ./internal/bootstrap/ -fuzz='FuzzStatsdSinkConfigParse' -fuzztime=60s
go test ./internal/bootstrap/ -fuzz='FuzzStatsSinkConfigParse'  -fuzztime=60s   # confirm the real name first
```

- [ ] **Gate 6 — the counts, re-derived, never copied**
```sh
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                                  # 100
grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l          # 52
grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1                     # ADR-0272
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1            # 38
go mod tidy -diff                                                                  # EMPTY
go list -deps ./internal/statssink/ | grep 'envoy-go/internal/cluster'             # NOTHING
```

Reconcile the documented fuzzer count against the grep before writing it anywhere (`reference_fuzzer_count_docs_drift` — the documented running total has drifted before).

- [ ] **Step 7 — ROADMAP + STATE + router**

- `ROADMAP.md`: flip **row 55** `in-progress` → `done`. Escape any literal `|` inside the summary cell. Update the Observability-family deferred-candidate sentence: **remove** `the statsd tcp_cluster_name transport` from `remaining deferred (not-yet-chartered) candidates:` — it is consumed. The family STAYS OPEN (`dog_statsd`/`graphite`/OTLP-metrics sinks, tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace, the tap filter).
- `STATE.md`: new `active-phase` header recording every amendment, the five resolved D-questions with their chosen answers, the four breaks' observed outcomes (including break (b)'s subset-vacuity observation and break (c)'s barrier-masking proof), and the re-derived counts. Demote the current header to `prior active-phase`.
- `next-prompt.txt`: roll forward to the **next phase's BRAINSTORM**. Re-check the TERMINATION SENTINEL mechanically — it will **not** fire (the Observability and Operational-tooling families stay open; five families — HTTP/3+QUIC, gRPC, xDS, Runtime+hot-restart, WASM — have never been opened).

- [ ] **Step 8 — squash, merge, push**

```bash
git checkout master && git merge --ff-only phase-55-stats-sink-statsd-tcp-impl   # after squashing
git push origin master
git worktree remove .worktrees/phase-55-impl
```
Per `feedback_subagents_no_push`, no subagent has pushed; per `feedback_push_to_origin`, the controller pushes here without asking once the suite is green on the frozen HEAD.

---

## Self-review

**Spec coverage.** SPEC §3.1→T5; §3.2→T1,T10; §3.3→T6; §3.4→T6; §3.5→T7; §3.6→T9; §3.7→T4; §3.8→T3; §3.9→T2. §4 primitives→T1,T2,T5. §5 proto roster→T3,T4. §6 reject roster→T3,T4. §7 zero stat delta→T13 (verified by grep). §8.1 fixture→T11; §8.2 the four breaks→T12; §8.3 no new BackendKind/fuzzer→T11,T14 gate 6. §9 contract→T13. §10's 14 indicative tasks→Tasks 1–14. §12's five D-questions→resolved above, each with a task that proves it (`DialSink`→T1; config shape→T3; pending cap→T8; node placement→T4; close→T9). §13 ADR context→T13.

**Three corrections this PLAN makes to the SPEC**, each verified against master tip `708c0dcc`:
1. **`Dial`'s signature is `(net.Conn, Endpoint, error)`**, not `(net.Conn, error)`. `DialSink` deliberately returns two values; the SPEC's sketch happened to be right about `DialSink` but its `Cluster.Dial` cross-reference would have misled an implementer.
2. **`tcp_cluster_name` appears at SIX Go sites**, not one. The SPEC names only `statsd_fuzz_test.go`; there is a **second fuzz seed** at `statssink_fuzz_test.go:110-115` and a table case at `bootstrap_test.go:2075-2082`. Task 3 updates all of them.
3. **`result.Proto` is the `*Bootstrap` *wrapper*'s field, not a field on `StatsdSinkConfig`.** The SPEC's shorthand invites adding a `Proto` field to the sink config. Do not.

**One assertion this PLAN strengthens.** SPEC §8.2 break **(b)** ("the counter-subset lookups miss") is **not reliably live**: `\n`-separated framing corrupts exactly the two names at each flush boundary, and `snapshot()`'s stable walk order means those are the same two names every flush — vacuous whenever neither is in `subsetNames`. Task 2 adds `statsdrecv.UnparsedCount()` and Task 11 asserts it subject-exact at `0`, making break (b) deterministic and order-independent. Task 12 requires the implementer to *observe and record* whether the subset assertion also fires, rather than assume it.

**§6 reject-roster coverage, row by row.** `tcp_cluster_name` set → ACCEPT (T3 `TestStatsdSink_AcceptTCPClusterName`). `statsd_specifier` unset → REJECT (T3 `TestStatsdSink_StillRejectsMissingSpecifier`). Unknown cluster → REJECT (T4). Node missing → REJECT (T4). **Existing but UNREACHABLE cluster → ACCEPT**: covered by T4's `TestStatsdSink_TCPBothNodeFieldsBoots`, whose `mc` cluster points at a dead `127.0.0.1:9999` — parse time cannot distinguish reachable from unreachable, and the sink's runtime retry is proven separately by T7's `TestTCPStatsdSink_DialFailureRetainsPending`. UDP + no node → ACCEPT (T4 `TestStatsdSink_UDPArmNeedsNoNode`, the mirrored control probe). Unknown sink TypeURL → REJECT, unchanged (`parseStatsSinks`'s `default` arm, untouched).

**Two BLOCKERs caught by the document-review gate, both fixed in place:** `endpointFromAddr` is `func(net.Addr) Endpoint` (`cluster_test.go:110`), so the six Task-1 tests must pass `ln.Addr()`, never `ln.Addr().String()`; and `fixture.TB` (`fixture.go:64-68`) exposes only `Errorf`/`Fatalf`/`Helper` — it deliberately does not import `testing` — so Task 11's driver records the reference's values with `log.Printf`, not `t.Logf`. Four line citations drifted (`listenTCP` `:71`, `listenTLS` `:91`, `endpointFromAddr` `:110`, the `main.go` synchronous-sink comment `:209-210` with a second copy at `:221-223` that must NOT be edited) and are corrected above.

**Type consistency.** `DialFunc` (T5) is exactly what `NewTCPStatsdSink` takes (T5) and what `main.go` supplies (T10), and its return matches `DialSink`'s (T1). `sideSnapshot`'s fields (T11) are set in `driveSide` and read in `AssertStats`. `dropOldestLines(p []byte, capBytes int) []byte` (T8) is called only from `flush`. `ingestLine(line string) (string, bool)` (T2) is called from `ingest` and `streamLoop`. `newTCPStatsdSinkNoRun` (T8) is used by `NewTCPStatsdSink` and by `TestTCPStatsdSink_PendingIsBounded` only.

**Placeholder scan.** Task 5's `flush` contains an explicit, labelled placeholder write replaced in Task 6; the plan says so and forbids committing Task 5 alone. Task 11 says "copy verbatim from `0092/driver/driver.go:<lines>`" for pure boilerplate — an exact instruction with exact line numbers, not a TBD. Three steps (`tlsPairForTest`, `counterFam`/`gaugeFam`, `statsBootstrap`) instruct the implementer to **read the existing helper and reuse it**, naming the file and line range, rather than duplicating code.

---

## Execution handoff

Per `feedback_execution_style` and `feedback_git_worktrees`, the IMPL runs **subagent-driven in a fresh worktree**:

```bash
git worktree add .worktrees/phase-55-impl -b phase-55-stats-sink-statsd-tcp-impl master
```

One fresh subagent per task, controller review between tasks. Subagents commit **locally only**. The controller: verifies each commit, re-runs the per-task gate on the frozen HEAD, cleans any leaked files, performs **Task 12 itself**, squashes at stage-close, runs the six gates, and pushes.

**Path targeting** (`feedback_subagent_worktree_path_targeting`). Subagents have written to the MAIN checkout instead of the worktree before (phase-33 Tasks 4 & 16). Every task dispatch MUST:
- pin the canonical worktree root `/home/esa/git/envoy-go/.worktrees/phase-55-impl` and give all file paths **relative to it**;
- after each task, have the CONTROLLER verify the main checkout is untouched:
  ```sh
  git -C /home/esa/git/envoy-go status --porcelain     # must be EMPTY
  git -C /home/esa/git/envoy-go/.worktrees/phase-55-impl branch --show-current   # phase-55-stats-sink-statsd-tcp-impl
  ```
  A non-empty main-checkout status means the subagent wrote to the wrong tree: revert it there, re-dispatch against the worktree.

**This PLAN passed a two-reviewer document gate** (against the SPEC + the as-built source). It caught two BLOCKERs — `endpointFromAddr` takes a `net.Addr` not a `string`, and `fixture.TB` has no `Logf` — both fixed above, plus four line-citation drifts. Do not assume the remaining citations are perfect: **re-verify any line number before you edit at it.**
