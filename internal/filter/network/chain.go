// internal/filter/network/chain.go — per-connection chain runner + runtime
// context + the concrete Connection / ReadFilterCallbacks impls.

package network

import (
	"context"
	"net"
	"sync"
	"sync/atomic"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
)

// connFacts carries the L4 connection facts the listener manager already
// extracted before the read-filter chain runs: the SNI (from tls_inspector's
// ChainMatchInputs.ServerName) and the mTLS peer principals (from the TLS
// handshake state the listener captures). local/remote override the net.Conn
// addresses when the manager has more accurate values (e.g. PROXY-protocol);
// when nil the runtime falls back to conn.LocalAddr/RemoteAddr.
type connFacts struct {
	serverName string
	principals []string
	local      net.Addr
	remote     net.Addr
}

// ConnFacts is the EXPORTED form of connFacts (the L4 connection facts the
// listener manager extracted before the read-filter chain runs). The listener
// manager constructs one per accepted connection and hands it to
// NewChainRuntime; ServerName is the SNI (tls_inspector / *tls.Conn
// ServerName), Principals the mTLS peer principals, and Local/Remote override
// the net.Conn addresses when the manager has more accurate values (nil falls
// back to conn.LocalAddr/RemoteAddr).
type ConnFacts struct {
	ServerName string
	Principals []string
	Local      net.Addr
	Remote     net.Addr
}

// ChainRuntime is the EXPORTED per-connection chain driver the listener manager
// uses to run a read-filter chain over a downstream net.Conn (SPEC §3.5). It
// wraps the unexported chainRuntime so manager.go never reaches into framework
// internals: NewChainRuntime builds it, OnNewConnection runs the eager pass,
// OnData feeds socket reads, CloseRequested drives the read loop's exit
// (D-P26.1-5a), and OnDestroy releases per-connection resources.
type ChainRuntime struct {
	rt *chainRuntime
}

// NewChainRuntime constructs the per-connection chain driver over a
// []NetworkFilter + the downstream conn + the manager-extracted facts (SPEC
// §3.5 construction step). It CLASSIFIES the filters into a read-filter prefix,
// a write-filter set, and an optional trailing TerminalFilter: the read prefix
// drives the OnData iteration; the write set intercepts terminal writes before
// they reach the downstream socket (AMEND-A11); the terminal (if any) takes
// over the conn once the prefix has Continued past it (TerminalReady →
// HandleTerminal). It injects the per-connection callbacks into each read filter
// and each write filter once (D-P2: a both-directions filter receives BOTH
// injections — the SAME instance appears in both sets).
func NewChainRuntime(filters []NetworkFilter, conn net.Conn, facts ConnFacts) *ChainRuntime {
	var (
		read     []ReadFilter
		write    []WriteFilter // CHAIN order; dispatch reverses (handleTerminal — AMEND-A11)
		terminal TerminalFilter
	)
	for _, f := range filters {
		// Independent type-asserts (NOT a type-switch): a filter implementing
		// BOTH ReadFilter and WriteFilter must land in BOTH sets — the SAME
		// instance (upstream addFilter parity; D-P2). zookeeperproxy (28.1) is
		// the first such filter. A type-switch's first-match-wins cannot express this.
		if t, ok := f.(TerminalFilter); ok {
			// Keep the LAST terminal if more than one is present (boot
			// validation in Task 7 prevents that shape from reaching here).
			terminal = t
			continue
		}
		if rf, ok := f.(ReadFilter); ok {
			read = append(read, rf)
		}
		if wf, ok := f.(WriteFilter); ok {
			write = append(write, wf)
		}
		// A NetworkFilter that is neither (sealed-marker-only) is defensively
		// ignored, exactly as today.
	}
	rt := newChainRuntime(read, conn, connFacts{
		serverName: facts.ServerName,
		principals: facts.Principals,
		local:      facts.Local,
		remote:     facts.Remote,
	})
	rt.terminal = terminal
	rt.setWriteFilters(write)
	// Write-callbacks injection (mirrors the read-callbacks loop in
	// newChainRuntime): every WriteFilter receives
	// SetWriteFilterCallbacks exactly once at construction. A both-directions
	// filter therefore receives BOTH injections (D-P2).
	for _, wf := range write {
		wf.SetWriteFilterCallbacks(&writeCallbacks{rt: rt})
	}
	return &ChainRuntime{rt: rt}
}

// TerminalReady reports whether the chain's trailing terminal filter is ready
// to take over the downstream conn (the read-filter prefix, if any, has
// Continued past its last filter without halting).
func (c *ChainRuntime) TerminalReady() bool { return c.rt.terminalReady() }

// HandleTerminal hands the downstream conn to the trailing terminal filter,
// replaying any undrained buffered prefix before the live conn (R-M). Caller
// must check TerminalReady first.
func (c *ChainRuntime) HandleTerminal(ctx context.Context) { c.rt.handleTerminal(ctx) }

// OnNewConnection runs the eager OnNewConnection pass in chain order before any
// downstream data (SPEC §3.3).
func (c *ChainRuntime) OnNewConnection() { c.rt.onNewConnection() }

// OnData appends newly-read bytes to the connection read buffer and runs the
// data iteration from the current resume index (SPEC §3.3). endStream signals
// downstream end-of-stream (EOF).
func (c *ChainRuntime) OnData(p []byte, endStream bool) { c.rt.onData(p, endStream) }

// OnDestroy calls every filter's OnDestroy in chain order and resets the
// dynamic-metadata bucket (SPEC §3.3 / §3.4). Called when the connection
// dispatch ends, however iteration exited.
func (c *ChainRuntime) OnDestroy() { c.rt.onDestroy() }

// CloseRequested reports whether a filter called Connection().Close — the read
// loop checks it to exit (D-P26.1-5a).
func (c *ChainRuntime) CloseRequested() bool { return c.rt.closeRequested() }

// CloseType reports the close semantics the closing filter requested via
// Connection().Close (F3, D-26.3-2). It is FlushWrite (the zero value) until a
// filter closes the connection; serveNetworkChain reads it at the pure-read
// close site to honor NoFlush (RST via SO_LINGER 0) distinctly from
// FlushWrite (rbac_network enforced-deny uses NoFlush). Only meaningful once
// CloseRequested is true.
func (c *ChainRuntime) CloseType() CloseType { return c.rt.closeType }

// SetDrainState wires the listener's drain decider into the per-connection chain
// (29.3, §3.4). Called by serveNetworkChain right after NewChainRuntime; the
// caller MUST guard against a typed-nil concrete (`if rt.dm != nil`) so a true-nil
// interface is stored when drain is unwired (the typed-nil-in-interface trap —
// otherwise Draining()'s `c.rt.drain != nil` would pass and IsDraining() would
// panic on a nil receiver; risk register line 1846).
func (c *ChainRuntime) SetDrainState(d DrainState) { c.rt.drain = d }

// chainRuntime is the per-connection runtime context: it owns the single
// drainable read Buffer (connection-level buffering per SPEC §3.3), the REUSED
// per-connection *dynamicmetadata.Bucket (SPEC §3.4 / AMEND-A5), the
// response-code-details sink string (D-P26.1-5b), and drives the sequential
// read-filter iteration (SPEC §3.3). Single-goroutine-per-connection
// (ADR-0213): no locks beyond the registry's RWMutex.
type chainRuntime struct {
	filters  []ReadFilter
	terminal TerminalFilter // optional trailing connection-takeover filter (26.2)
	// writeFilters is the write-direction half of the chain in CHAIN order
	// (ADR-0221). handleTerminal hands writeDispatch — the REVERSED copy
	// (dispatch order), built ONCE at setWriteFilters since the set is fixed for
	// the connection's lifetime — to the writeChainConn (AMEND-A11 LIFO parity).
	// A both-directions filter appears here AND in filters — the same instance.
	writeFilters  []WriteFilter
	writeDispatch []WriteFilter
	conn          net.Conn
	facts         connFacts

	buf    *Buffer
	bucket *dynamicmetadata.Bucket
	rcd    string // response_code_details sink (D-P26.1-5b)
	// upstreamClusterOverride is the per-connection upstream-cluster-override a
	// read filter (sni_cluster, 27) publishes; "" = no override. It is the NARROW
	// typed stand-in for Envoy's PerConnectionCluster filter-state entry (key
	// "envoy.tcp_proxy.cluster"; ADR-0219) — NOT a general filter-state primitive
	// (Q2/YAGNI). handleTerminal threads it to the terminal filter via the call ctx.
	upstreamClusterOverride string

	cb   *callbacks
	cxn  *connection
	done []bool // OnNewConnection called per filter (lazy after a ContinueReading jump)

	resumeIdx int // index at which the next data iteration resumes
	// connHalted records that the filter at resumeIdx returned StopIteration from
	// its OnNewConnection (the lazy-OnNewConnection halt, SPEC §3.3): that filter
	// has NOT consented to data flowing past it, so no OnData runs at or beyond
	// resumeIdx until ContinueReading advances the chain. This is the ONLY halt
	// that persists across socket reads. A StopIteration returned from a filter's
	// OnData is deliberately NOT recorded here: per upstream
	// FilterManagerImpl::onRead the chain re-iterates on EVERY socket read, so an
	// OnData StopIteration stops only the current pass's downstream propagation —
	// the next socket read re-delivers the accumulated buffer to that same filter.
	connHalted      bool
	resumeRequested bool // a filter called ContinueReading during the current OnData
	lastEndStream   bool // endStream of the most recent onData (replayed on resume)
	closeReq        bool // Connection.Close was called (D-P26.1-5a)
	// closeType records the CloseType the closing filter requested (F3,
	// D-26.3-2). Zero value is FlushWrite, so an un-closed runtime reports the
	// pre-26.3 default; connection.Close overwrites it with the requested
	// semantics. serveNetworkChain honors it at the pure-read close site
	// (NoFlush closes immediately via SO_LINGER 0 → RST; rbac_network
	// enforced-deny uses NoFlush).
	closeType CloseType

	// 29.3 async halt/resume seam (ADR-0226). haltable is set ONCE at construction
	// (immutable thereafter -> race-free without an atomic; the construction-time
	// fast-path gate of SPEC §3.1): true iff >=1 read filter is a HaltingReadFilter
	// with MayHalt()==true. When false, runData/replayRead/readChainConn.Read take
	// the byte-identical pre-29.3 path (R1). haltMu/haltCond + held/resumeReady are
	// the hold<->resume coordination (block-the-dispatcher; D-S29.3-1). The
	// closeDir/drain (§3.5/§3.4) fields land below (Task 9 adds both; the closeDir
	// SETTER + tcp_proxy recording is Task 10).
	haltable    bool
	haltMu      sync.Mutex
	haltCond    *sync.Cond
	held        bool // a delay-hold is in effect (guarded by haltMu)
	resumeReady bool // ContinueReading fired before the holder blocked (guarded by haltMu)
	// 29.3 Task 4 post-handoff replay seam (SPEC §3.1 extension 3). replayIdx is
	// the parked position in rt.filters when a haltable replayRead pass holds at a
	// filter; replayHeld is the PINNED park-state ContinueReading consumes to
	// finish the post-handoff pass (do NOT infer park-state from replayIdx==0 —
	// ambiguous for the single-filter mongo chain). Both are touched only on the
	// haltable replay path (replayRead sets, ContinueReading consumes, both under
	// the read-loop/pump goroutine + haltMu where cross-goroutine).
	replayIdx  int
	replayHeld bool

	// 29.3 drain + close-direction (Task 9). drain is the narrow structural
	// drain-decider (DrainState) the listener wires via SetDrainState; nil when
	// drain is unwired (the typed-nil-in-interface trap is dodged at the call
	// site's `if rt.dm != nil` guard, so this stays a true-nil interface →
	// Draining() returns false). closeDir is the post-handoff close direction
	// (CloseDirection): the READ-side accessor (CloseDirection()) lands here so
	// callbacks compiles; the SETTER + tcp_proxy pump-EOF-first recording is Task
	// 10. atomic.Int32 because the setter (pump goroutine) and the OnDestroy
	// reader (Task 10) are cross-goroutine.
	drain    DrainState
	closeDir atomic.Int32
}

// newChainRuntime constructs the per-connection runtime over filters + conn +
// the manager-extracted facts. It allocates the read Buffer + dynamic-metadata
// Bucket, builds the concrete connection / callbacks impls, and injects the
// callbacks into each filter once (SPEC §3.3 construction step).
func newChainRuntime(filters []ReadFilter, conn net.Conn, facts connFacts) *chainRuntime {
	rt := &chainRuntime{
		filters: filters,
		conn:    conn,
		facts:   facts,
		buf:     &Buffer{},
		bucket:  dynamicmetadata.NewBucket(),
		done:    make([]bool, len(filters)),
	}
	rt.cxn = &connection{rt: rt}
	rt.cb = &callbacks{rt: rt}
	rt.haltCond = sync.NewCond(&rt.haltMu)
	for _, f := range rt.filters {
		f.SetReadFilterCallbacks(rt.cb)
	}
	// 29.3: a read filter that opts into the halt seam (HaltingReadFilter with
	// MayHalt()==true) makes the whole chain "haltable" — set ONCE here at
	// construction (immutable thereafter; D-S29.3-1). NewChainRuntime flows
	// through here, so this is the single classification site for both the
	// production []NetworkFilter path and the internal-test []ReadFilter path.
	for _, f := range rt.filters {
		if hf, ok := f.(HaltingReadFilter); ok && hf.MayHalt() {
			rt.haltable = true
			break
		}
	}
	return rt
}

// setWriteFilters records the write-direction half of the chain (CHAIN order)
// and builds the REVERSED dispatch copy once (AMEND-A11 LIFO parity: config
// [A, B, C] ⇒ write dispatch C → B → A). The write set is fixed for the
// connection's lifetime, so handleTerminal reuses writeDispatch instead of
// rebuilding the reversal per handoff. The chain-order slice is never mutated.
func (rt *chainRuntime) setWriteFilters(write []WriteFilter) {
	rt.writeFilters = write
	rt.writeDispatch = make([]WriteFilter, len(write))
	for i, wf := range write {
		rt.writeDispatch[len(write)-1-i] = wf
	}
}

// setCloseDirection records the post-handoff close direction first-wins (29.3,
// §3.5). Called by tcp_proxy via the readChainConn.SetCloseDirection forwarding on
// the pump goroutine; read by callbacks.CloseDirection() at OnDestroy (after the
// pumps join — a happens-after edge; atomic for race-cleanliness).
func (rt *chainRuntime) setCloseDirection(d CloseDirection) {
	rt.closeDir.CompareAndSwap(int32(CloseDirectionUnset), int32(d))
}

// callbacks returns the per-connection ReadFilterCallbacks. The RCD sink lives
// on the concrete *callbacks (SetResponseCodeDetails), reachable only by
// type-asserting the returned interface — it is NOT part of ReadFilterCallbacks.
func (rt *chainRuntime) callbacks() ReadFilterCallbacks { return rt.cb }

// closeRequested reports whether a filter called Connection().Close (the read
// loop checks it to exit; D-P26.1-5a).
func (rt *chainRuntime) closeRequested() bool { return rt.closeReq }

// terminalReady reports whether the trailing terminal filter may take over the
// downstream conn: the chain has a terminal, the read-filter prefix has not
// halted (connHalted), and iteration has advanced past the last read filter
// (resumeIdx >= len(filters)). Pure-terminal (0 read filters, resumeIdx 0) is
// ready immediately; mixed becomes ready once every read filter Continued;
// pure-read (terminal == nil) is never ready.
func (rt *chainRuntime) terminalReady() bool {
	return rt.terminal != nil && !rt.connHalted && rt.resumeIdx >= len(rt.filters)
}

// handleTerminal hands the downstream conn to the terminal filter. If the
// read-filter prefix left undrained bytes in the connection buffer, those are
// replayed to the terminal BEFORE the live conn via a prefixConn (R-M). For a
// pure-terminal chain the buffer is empty → conn == rt.conn → byte-identical to
// a direct Handle of the raw conn.
//
// The read-side seam (28.1b, ADR-0221 §AMEND) wraps the RAW conn in a
// readChainConn — INNERMOST — under the SAME ≥1-write-filter predicate as the
// writeChainConn, giving the §3.1 composition writeChainConn(prefixConn(
// readChainConn(conn))): the prefixConn serves its buffered prefix WITHOUT
// delegating inward, so prefix bytes (already seen pre-handoff) bypass the
// replay while only LIVE post-handoff socket reads pass through readChainConn.
func (rt *chainRuntime) handleTerminal(ctx context.Context) {
	conn := rt.conn
	// Read-side seam (ADR-0221 §AMEND): wrap the RAW conn in a readChainConn —
	// INNERMOST — under the SAME predicate as the writeChainConn below (R1: the
	// two seams wrap together; zero-write-filter chains get NEITHER wrap).
	// Innermost is load-bearing: the prefixConn's buffered prefix (bytes the
	// read filters ALREADY saw pre-handoff) is served by prefixConn.Read WITHOUT
	// delegating inward, so prefix bytes bypass the replay; only LIVE post-handoff
	// socket reads pass through readChainConn.Read (28.1b SPEC §3.1).
	if len(rt.writeFilters) > 0 {
		conn = newReadChainConn(conn, rt)
	}
	if rt.buf.Len() > 0 {
		prefix := make([]byte, rt.buf.Len())
		copy(prefix, rt.buf.Bytes())
		rt.buf.Drain(rt.buf.Len())
		conn = newPrefixConn(conn, prefix)
	}
	// WriteFilter seam (ADR-0221): wrap the conn handed to the terminal in a
	// writeChainConn IFF the chain has ≥1 write filter, so terminal-originated
	// downstream writes run the write chain BEFORE reaching the socket.
	// Composition: writeChainConn OUTER, prefixConn MIDDLE, readChainConn INNER
	// (reads promote: write → prefix replay → live read + replay → raw conn;
	// writes promote: write chain → embedded conns → raw conn). Zero-write-filter
	// chains get NO wrap → byte-identical to the pre-28.1 path (R1 back-compat
	// over all 47 existing fixtures).
	// The dispatch slice is the REVERSED COPY of the chain-order writeFilters
	// (AMEND-A11 LIFO parity: config [A, B, C] ⇒ write dispatch C → B → A),
	// built once at setWriteFilters.
	if len(rt.writeFilters) > 0 {
		conn = newWriteChainConn(conn, rt.writeDispatch)
	}
	if rt.upstreamClusterOverride != "" {
		ctx = withUpstreamClusterOverride(ctx, rt.upstreamClusterOverride)
	}
	rt.terminal.Handle(ctx, conn)
}

// responseCodeDetails returns the response-code-details string set via the sink
// (D-P26.1-5b). direct_response sets "DirectResponse"; "" otherwise.
func (rt *chainRuntime) responseCodeDetails() string { return rt.rcd }

// onNewConnection runs the eager OnNewConnection pass in chain order before any
// downstream data (SPEC §3.3). On StopIteration it halts at the current filter;
// the data iteration lazily resumes OnNewConnection for not-yet-called filters.
func (rt *chainRuntime) onNewConnection() {
	for i := 0; i < len(rt.filters); i++ {
		if rt.done[i] {
			continue
		}
		rt.done[i] = true
		if rt.filters[i].OnNewConnection() == StopIteration {
			rt.connHalted = true
			rt.resumeIdx = i
			return
		}
		rt.resumeIdx = i + 1
	}
	// All OnNewConnection calls advanced: data iteration starts at the first
	// filter (resumeIdx reset to 0 below by onData's fresh pass).
	rt.resumeIdx = 0
}

// onData appends the newly-read bytes to the connection read buffer and
// RE-RUNS the data iteration from the current resume index on EVERY socket read
// (SPEC §3.3, mirroring upstream FilterManagerImpl::onRead, which re-iterates
// the read-filter chain on each read). A StopIteration returned from a prior
// read's OnData stops only that pass — it must never permanently swallow a
// later socket read. The sole exception is connHalted: a filter that returned
// StopIteration from its OnNewConnection has not consented to data, so the
// accumulated bytes wait in the connection-level buffer until ContinueReading
// advances the chain past it.
func (rt *chainRuntime) onData(p []byte, endStream bool) {
	rt.buf.Append(p)
	rt.lastEndStream = endStream
	if rt.connHalted {
		return
	}
	rt.runData()
}

// runData iterates filters from resumeIdx: lazily calling OnNewConnection for
// any filter not yet initialized (StopIteration → connHalted, sticky across
// reads), then OnData with the shared read buffer (when there are bytes or
// end-of-stream). On an OnData StopIteration the iteration STOPS this pass at
// the current filter (resumeIdx unchanged), so the next socket read re-delivers
// the accumulated buffer to that same filter — UNLESS that filter called
// ContinueReading (resumeRequested), in which case the chain advances to the
// next filter with the currently-buffered bytes in the SAME pass (SPEC §3.3).
// An OnData StopIteration is NOT a permanent halt (that is the bug this fix
// corrects); only the OnNewConnection halt persists. Undrained bytes remain in
// the buffer for the next socket read.
func (rt *chainRuntime) runData() {
	for rt.resumeIdx < len(rt.filters) {
		i := rt.resumeIdx

		if !rt.done[i] {
			rt.done[i] = true
			if rt.filters[i].OnNewConnection() == StopIteration {
				rt.connHalted = true
				return
			}
		}

		if rt.buf.Len() == 0 && !rt.lastEndStream {
			// No bytes to deliver and not end-of-stream: pause here until the
			// next socket read appends bytes.
			return
		}

		rt.resumeRequested = false
		status := rt.filters[i].OnData(rt.buf, rt.lastEndStream)
		if status == StopIteration {
			if rt.resumeRequested {
				rt.resumeRequested = false
				rt.resumeIdx = i + 1
				continue
			}
			if rt.haltable {
				// 29.3 delay-hold (SPEC §3.1): block this dispatcher goroutine
				// until the async ContinueReading releases, then advance PAST the
				// halting filter (upstream std::next(filter->entry()) parity) and
				// continue. Non-haltable chains never reach here.
				rt.holdUntilResume()
				rt.resumeIdx = i + 1
				continue
			}
			// Per-pass stop: leave resumeIdx at i so the next socket read
			// re-delivers the accumulated buffer to this same filter. Do NOT
			// set connHalted — that would permanently swallow later reads.
			return
		}
		rt.resumeIdx = i + 1
	}
	// A completed pass over a PURE-READ chain (no terminal) resets the resume
	// index so the next socket read re-delivers the accumulated buffer to every
	// filter (upstream FilterManagerImpl::onRead re-iterates on EVERY read).
	// Without the reset, resumeIdx would stick at len(filters) after an
	// all-Continue pass and every later read would be buffered but never
	// delivered. Terminal chains keep resumeIdx at len(filters): terminalReady
	// keys on it, and the terminal takes over the conn at handoff.
	if rt.terminal == nil {
		rt.resumeIdx = 0
	}
}

// holdUntilResume blocks the calling (dispatcher) goroutine until an async
// ContinueReading releases the hold (block-the-dispatcher; D-S29.3-1). Only
// called on a haltable chain after a filter returned a hold StopIteration. The
// resumeReady flag closes the missed-wakeup race: if ContinueReading fired
// between OnData's return and this acquiring haltMu, resumeReady is already set
// and we do not block.
//
// Contract: each hold is matched by exactly one resume (1:1). Halting filters
// (mongo fault-delay today; redis/kafka reusing this seam later) MUST arm a
// single ContinueReading per hold StopIteration — no extra resumes, no missed
// resumes.
func (rt *chainRuntime) holdUntilResume() {
	rt.haltMu.Lock()
	if rt.resumeReady {
		rt.resumeReady = false
	} else {
		rt.held = true
		for rt.held {
			rt.haltCond.Wait()
		}
	}
	rt.haltMu.Unlock()
}

// releaseResume is the resume side: if a holder is blocked, wake it; otherwise
// record that a resume arrived (resumeReady) so the next holdUntilResume does
// not block. Called by ContinueReading's async path (the timer goroutine).
//
// Contract: paired 1:1 with holdUntilResume — exactly one resume per hold (see
// holdUntilResume). A spurious extra resume would leave resumeReady set and let
// a future unrelated hold fall through without blocking.
func (rt *chainRuntime) releaseResume() {
	rt.haltMu.Lock()
	if rt.held {
		rt.held = false
		rt.haltCond.Broadcast()
	} else {
		rt.resumeReady = true
	}
	rt.haltMu.Unlock()
}

// replayRead re-iterates the read-filter chain over post-handoff downstream
// bytes (the read-side seam, ADR-0221 §AMEND). For a NON-haltable chain it is
// the pre-29.3 OBSERVATIONAL pass: it restores upstream
// FilterManagerImpl::onRead parity for wrapped chains (every read filter's
// OnData runs, in chain order, on every socket read for the connection's
// lifetime), Status is IGNORED (the bytes are already committed to the terminal
// via readChainConn.Read's return), the buffer is fully drained after the pass
// (bounded memory), and it returns false — byte-identical to pre-29.3 (R1).
//
// For a HALTABLE chain (29.3, SPEC §3.1 extension 3) it honors a hold
// StopIteration: it stops at the halting filter, leaves the buffer UNDRAINED,
// parks replayIdx, sets rt.replayHeld (the pinned park-state ContinueReading
// consumes), and returns parked=true so readChainConn.Read withholds the bytes
// from the upstream pump until the async ContinueReading releases.
//
// Called from readChainConn.Read on the terminal's downstream-pump goroutine
// (§3.6 concurrency pin) — never concurrently with the pre-handoff read loop
// (handoff is a happens-before edge: the loop returns before Handle spawns the
// pumps).
func (rt *chainRuntime) replayRead(p []byte, endStream bool) (parked bool) {
	rt.buf.Append(p)
	if !rt.haltable {
		for _, f := range rt.filters {
			_ = f.OnData(rt.buf, endStream) // Status ignored — observational (§3.5)
		}
		rt.buf.Drain(rt.buf.Len())
		return false
	}
	for rt.replayIdx < len(rt.filters) {
		i := rt.replayIdx
		if rt.filters[i].OnData(rt.buf, endStream) == StopIteration {
			// Hold: do NOT advance replayIdx, do NOT drain. The pump
			// (readChainConn.Read) holdUntilResume()s; ContinueReading advances +
			// drains + releases.
			//
			// Publish replayHeld UNDER haltMu: the halting filter's OnData (above)
			// arms the async ContinueReading, which reads replayHeld under haltMu in
			// finishParkedReplayLocked. If that resume goroutine fires before this
			// write lands, an unlocked write here would race the guarded read — so
			// the publication of the park-state must hold the same lock as its
			// reader (the replay-state publication fence, D-S29.3-1).
			rt.haltMu.Lock()
			rt.replayHeld = true // the pinned park-state ContinueReading consumes
			rt.haltMu.Unlock()
			return true
		}
		rt.replayIdx++
	}
	rt.replayIdx = 0
	rt.buf.Drain(rt.buf.Len())
	return false
}

// finishParkedReplayLocked completes a parked post-handoff replay pass: it clears
// the pinned park-state (replayHeld), advances PAST the halting filter, and drains
// the buffer. ContinueReading's haltable branch calls it when replayHeld is set.
//
// Caller MUST hold rt.haltMu (the replay-state publication guard — the parked
// replayRead wrote replayHeld/replayIdx/buf unlocked on the pump goroutine; this
// finish reads/mutates them on the resume goroutine, so the lock fences the two).
//
// Completeness boundary (D-S29.3-2): the general N-filter re-dispatch of any read
// filters AFTER the halting one is NOT performed here — it would have to call
// OnData while holding haltMu, which a re-entrant arm-another-delay filter would
// deadlock on, and no 29.3 chain has a post-mongo read filter. A future
// multi-read-filter haltable chain re-dispatches the tail outside the lock;
// today's single-filter mongo chain does not need it.
func (rt *chainRuntime) finishParkedReplayLocked() {
	rt.replayHeld = false
	// replayIdx = 0 here is the COMBINED "advance-past-the-halting-filter + reset"
	// for the single-filter mongo chain (rt.filters = [mongo]): there is no filter
	// after the halting one, so advancing past it lands exactly at the reset
	// position. A future N-filter extension must NOT read this as a plain reset
	// that skipped the advance — the advance is folded in because the tail is empty.
	rt.replayIdx = 0
	rt.buf.Drain(rt.buf.Len())
}

// onDestroy calls every filter's OnDestroy in chain order (mirroring
// pipeline.go's defer-OnDestroy discipline) and resets the dynamic-metadata
// bucket (SPEC §3.3 / §3.4).
//
// Once-per-instance dedupe (D-P2): a both-directions filter appears in BOTH
// rt.filters and rt.writeFilters as the SAME instance; its OnDestroy must run
// exactly once. Filter instances are pointers (interface identity comparison
// is well-defined), hence usable as map keys.
func (rt *chainRuntime) onDestroy() {
	destroyed := make(map[NetworkFilter]bool, len(rt.filters)+len(rt.writeFilters))
	for _, f := range rt.filters {
		if !destroyed[f] {
			destroyed[f] = true
			f.OnDestroy()
		}
	}
	for _, f := range rt.writeFilters {
		if !destroyed[f] {
			destroyed[f] = true
			f.OnDestroy()
		}
	}
	rt.bucket.Reset()
}

// callbacks is the concrete ReadFilterCallbacks handed to every filter on the
// connection. It threads the per-connection runtime context.
type callbacks struct {
	rt *chainRuntime
}

func (c *callbacks) Connection() Connection { return c.rt.cxn }

// ContinueReading resumes a chain halted by StopIteration, restarting at the
// NEXT filter with the currently-available buffered bytes (SPEC §3.3). Two
// cases: (1) the chain is parked on an OnNewConnection halt (connHalted) — a
// filter that returned StopIteration from OnNewConnection — so it clears the
// halt, advances past that filter, and re-runs the iteration directly. (2) it
// is called re-entrantly from within a filter's OnData (before that filter
// returns StopIteration) — it records the resume intent for runData to honor
// when the filter returns. The OnData StopIteration alone never persists as a
// halt across socket reads, so there is no post-pass OnData-resume case here.
func (c *callbacks) ContinueReading() {
	rt := c.rt
	if rt.connHalted {
		rt.connHalted = false
		rt.resumeIdx++
		rt.runData()
		return
	}
	if rt.haltable {
		// 29.3 async-active third context (SPEC §3.1 extension 1): a ContinueReading
		// arriving off-dispatch (from a time.AfterFunc goroutine) while the dispatcher
		// is blocked at a hold. The post-handoff replay-finish (replayHeld/replayIdx +
		// drain) runs under haltMu to stay -race-clean; the holder release is then
		// delegated to releaseResume (which re-acquires haltMu — do NOT hold it across
		// that call, or it deadlocks). The unlock-then-release split is safe: only
		// this resume side clears held, and the holder cannot wake until the Broadcast
		// inside releaseResume, so the replayHeld mutation happens-before the wake.
		rt.haltMu.Lock()
		if rt.replayHeld {
			rt.finishParkedReplayLocked()
		}
		rt.haltMu.Unlock()
		// Pre-handoff: replayHeld was false; the blocked runData advances resumeIdx
		// itself on wake. Either way, release the holder (wake the blocked dispatcher,
		// or record the pending resume). NOTE: mongo's fault-delay resume is ALWAYS
		// async (never re-entrant), so a haltable chain routes here; the re-entrant
		// resumeRequested path below is reached only by non-haltable filters.
		rt.releaseResume()
		return
	}
	// Called re-entrantly from the current filter's OnData: defer the advance
	// to runData (which sees resumeRequested when the filter returns).
	rt.resumeRequested = true
}

func (c *callbacks) DynamicMetadata() *dynamicmetadata.Bucket { return c.rt.bucket }

// SetResponseCodeDetails writes the response-code-details sink string on the
// per-connection runtime (D-P26.1-5b). direct_response (Task 8) type-asserts
// ReadFilterCallbacks to interface{ SetResponseCodeDetails(string) } and calls
// this to set "DirectResponse"; no operator-visible surface emits it at 26.1.
func (c *callbacks) SetResponseCodeDetails(s string) { c.rt.rcd = s }

// SetUpstreamCluster records the per-connection upstream-cluster-override on the
// runtime (ADR-0219). sni_cluster (27) calls it from OnNewConnection with the
// verbatim SNI; handleTerminal threads it to the terminal filter via the ctx.
func (c *callbacks) SetUpstreamCluster(name string) { c.rt.upstreamClusterOverride = name }

// Draining reports whether the listener is draining (29.3, §3.4). The
// `c.rt.drain != nil` short-circuit keeps it nil-tolerant: when no DrainState is
// wired (rt.drain is a true-nil interface) it returns false WITHOUT dereferencing
// — dodging the typed-nil-in-interface panic (risk register line 1846). mongo_proxy
// gates cx_drain_close on it.
func (c *callbacks) Draining() bool { return c.rt.drain != nil && c.rt.drain.IsDraining() }

// CloseDirection reports the post-handoff close direction (29.3, §3.5). The
// read-side accessor lands at Task 9 (so callbacks compiles); the closeDir SETTER
// + tcp_proxy pump-EOF-first recording is Task 10. Defaults to CloseDirectionUnset.
func (c *callbacks) CloseDirection() CloseDirection { return CloseDirection(c.rt.closeDir.Load()) }

// writeCallbacks is the concrete WriteFilterCallbacks injected into every
// WriteFilter at chain construction (ADR-0221; D-P2 — a both-directions filter
// receives BOTH a *callbacks and a *writeCallbacks injection). Connection()
// returns the SAME per-connection accessor the read callbacks expose.
type writeCallbacks struct {
	rt *chainRuntime
}

func (w *writeCallbacks) Connection() Connection { return w.rt.cxn }

// connection is the concrete Connection accessor over the dispatch net.Conn +
// the manager-extracted L4 facts.
type connection struct {
	rt *chainRuntime
}

// Write writes data to the downstream net.Conn. net.Conn.Write is already
// flushed; endStream is advisory at 26.1 (echo/direct_response pass it for
// forward-consumer parity but the L4 socket has no half-close signal here).
func (c *connection) Write(data []byte, _ bool) {
	// Error/byte-count drop is intentional: write failures surface to the read
	// loop / socket lifecycle (EOF/reset on the next read), not back to the
	// filter, so there is no in-band error to propagate here.
	_, _ = c.rt.conn.Write(data)
}

// Close records the close request + the requested semantics (D-P26.1-5a; F3,
// D-26.3-2). The read loop checks closeRequested() to exit and performs the
// actual socket close, honoring the recorded CloseType (FlushWrite vs NoFlush),
// so the chain stays single-goroutine.
//
// As of 26.3 the CloseType is RECORDED + reaches serveNetworkChain distinctly:
// FlushWrite (the zero value / pre-26.3 default) drains then closes; NoFlush
// closes immediately, discarding any pending downstream write (RST semantics
// via SO_LINGER 0; rbac_network enforced-deny uses NoFlush). For
// direct_response — which writes its body SYNCHRONOUSLY
// (Connection().Write, already flushed on the blocking socket) BEFORE Close —
// FlushWrite remains operationally a plain close with no pending write, so its
// byte behavior is unchanged.
func (c *connection) Close(ct CloseType) {
	c.rt.closeReq = true
	c.rt.closeType = ct
}

func (c *connection) LocalAddr() net.Addr {
	if c.rt.facts.local != nil {
		return c.rt.facts.local
	}
	return c.rt.conn.LocalAddr()
}

func (c *connection) RemoteAddr() net.Addr {
	if c.rt.facts.remote != nil {
		return c.rt.facts.remote
	}
	return c.rt.conn.RemoteAddr()
}

func (c *connection) RequestedServerName() string { return c.rt.facts.serverName }

func (c *connection) DownstreamPrincipals() []string { return c.rt.facts.principals }
