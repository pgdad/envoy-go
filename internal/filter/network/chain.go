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
// §3.5 construction step). It CLASSIFIES the filters into a read-filter prefix
// and an optional trailing TerminalFilter: the read prefix drives the existing
// OnData iteration; the terminal (if any) takes over the conn once the prefix
// has Continued past it (TerminalReady → HandleTerminal). It injects the
// per-connection callbacks into each read filter once.
func NewChainRuntime(filters []NetworkFilter, conn net.Conn, facts ConnFacts) *ChainRuntime {
	var (
		read     []ReadFilter
		terminal TerminalFilter
	)
	for _, f := range filters {
		switch nf := f.(type) {
		case TerminalFilter:
			// Keep the LAST terminal if more than one is present (boot
			// validation in Task 7 prevents that shape from reaching here).
			terminal = nf
		case ReadFilter:
			read = append(read, nf)
		default:
			// Defensively ignore any NetworkFilter that is neither (the sealed
			// marker should make this unreachable).
		}
	}
	rt := newChainRuntime(read, conn, connFacts{
		serverName: facts.ServerName,
		principals: facts.Principals,
		local:      facts.Local,
		remote:     facts.Remote,
	})
	rt.terminal = terminal
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

// chainRuntime is the per-connection runtime context: it owns the single
// drainable read Buffer (connection-level buffering per SPEC §3.3), the REUSED
// per-connection *dynamicmetadata.Bucket (SPEC §3.4 / AMEND-A5), the
// response-code-details sink string (D-P26.1-5b), and drives the sequential
// read-filter iteration (SPEC §3.3). Single-goroutine-per-connection
// (ADR-0213): no locks beyond the registry's RWMutex.
type chainRuntime struct {
	filters  []ReadFilter
	terminal TerminalFilter // optional trailing connection-takeover filter (26.2)
	conn     net.Conn
	facts    connFacts

	buf    *Buffer
	bucket *dynamicmetadata.Bucket
	rcd    string // response_code_details sink (D-P26.1-5b)

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
func (rt *chainRuntime) onDestroy() {
	for _, f := range rt.filters {
		f.OnDestroy()
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

// Close records the close request + semantics (D-P26.1-5a). The read loop
// checks closeRequested() to exit and performs the actual socket close (with
// FlushWrite/NoFlush handling) so the chain stays single-goroutine.
//
// The CloseType (FlushWrite vs NoFlush) is intentionally ignored at 26.1: the
// only closing filter shipped is direct_response, which writes its body
// SYNCHRONOUSLY (via Connection().Write, already flushed on the blocking
// socket) BEFORE calling Close, so FlushWrite ≡ a plain close and there is no
// pending write to flush. NoFlush (drop-buffered-writes-then-close) only
// becomes observable with the 26.3 enforced-deny path and MUST be honored
// (distinguished from FlushWrite) when 26.3 lands.
func (c *connection) Close(_ CloseType) {
	c.rt.closeReq = true
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
