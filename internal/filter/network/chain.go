// internal/filter/network/chain.go — per-connection chain runner + runtime
// context + the concrete Connection / ReadFilterCallbacks impls.

package network

import (
	"context"
	"net"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
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
	rt.writeFilters = write
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
	// (ADR-0221). handleTerminal hands a REVERSED copy (dispatch order) to the
	// writeChainConn (AMEND-A11 LIFO parity). A both-directions filter appears
	// here AND in filters — the same instance.
	writeFilters []WriteFilter
	conn         net.Conn
	facts        connFacts

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
	for _, f := range rt.filters {
		f.SetReadFilterCallbacks(rt.cb)
	}
	return rt
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
func (rt *chainRuntime) handleTerminal(ctx context.Context) {
	conn := rt.conn
	if rt.buf.Len() > 0 {
		prefix := make([]byte, rt.buf.Len())
		copy(prefix, rt.buf.Bytes())
		rt.buf.Drain(rt.buf.Len())
		conn = newPrefixConn(rt.conn, prefix)
	}
	// WriteFilter seam (ADR-0221): wrap the conn handed to the terminal in a
	// writeChainConn IFF the chain has ≥1 write filter, so terminal-originated
	// downstream writes run the write chain BEFORE reaching the socket.
	// Composition: writeChainConn OUTER, prefixConn INNER (reads promote through
	// to the buffered-prefix replay; writes run the chain then hit the inner
	// conn). Zero-write-filter chains get NO wrap → byte-identical to the
	// pre-28.1 path (R1 back-compat over all 47 existing fixtures).
	// The dispatch slice is a REVERSED COPY of the chain-order writeFilters
	// (AMEND-A11 LIFO parity: config [A, B, C] ⇒ write dispatch C → B → A).
	if len(rt.writeFilters) > 0 {
		dispatch := make([]WriteFilter, len(rt.writeFilters))
		for i, wf := range rt.writeFilters {
			dispatch[len(rt.writeFilters)-1-i] = wf
		}
		conn = newWriteChainConn(conn, dispatch)
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
			// Per-pass stop: leave resumeIdx at i so the next socket read
			// re-delivers the accumulated buffer to this same filter. Do NOT
			// set connHalted — that would permanently swallow later reads.
			return
		}
		rt.resumeIdx = i + 1
	}
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
