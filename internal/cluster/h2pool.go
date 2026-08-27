package cluster

import (
	"context"
	"errors"

	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
)

// errH2GrantRaced is returned by AcquireH2Stream when a stream-grant arrives for
// a pooled conn that was evicted between the grant send and the post-grant
// lookup (the defensive grant/eviction race). It is DISTINCT from
// errConnPoolOverflow (which means "the pending queue is full") so Task 7's
// classifier does not mistake a lost-race for a real overflow (this path does
// NOT Inc upstream_rq_pending_overflow). The intended Task-7 disposition is a
// transparent retry / fresh acquire — not the fail-fast 503 + overflow-stat that
// errConnPoolOverflow drives. No exported predicate yet (added in Task 7 only if
// the router needs to distinguish it via errors.Is). (phase 43.2a, ADR-0253)
var errH2GrantRaced = errors.New("cluster: h2 pool: granted conn evicted before use (retryable)")

// IsH2GrantRaced reports whether err is the retryable grant/eviction-race signal
// returned by AcquireH2Stream when a granted conn was evicted before use. It is
// the predicate sibling of IsConnPoolOverflow; the router (Task 7) uses it to
// distinguish a transparent-retry case (re-acquire) from a real overflow
// (fail-fast 503 + the overflow stat). (phase 43.2a, ADR-0253)
func IsH2GrantRaced(err error) bool { return errors.Is(err, errH2GrantRaced) }

// pooledH2Conn is one multiplexed upstream H2 connection plus its in-flight
// stream count. inFlight is guarded SOLELY by Cluster.h2PoolMu (D-H2-MUTEX);
// there is no conn-level stream counter. (phase 43.2a, ADR-0253)
type pooledH2Conn struct {
	cc       *h2.ClientConn
	inFlight int64
}

// connectingH2Conn is one IN-FLIGHT dial — the "connecting" tier between a
// stream-HIT and a fresh dial (Task 9.5, connect-time coalescing). The
// reference H2 pool assumes a connecting conn will provide its full C stream
// slots, so concurrent demand ATTACHES to it (reserving a promised slot) and
// waits for it to establish rather than each acquirer opening its own conn —
// so K concurrent streams at cap C open exactly ceil(K/C) conns, not K.
//
// Lifecycle (all field mutation under Cluster.h2PoolMu; the only out-of-lock
// ops are the dial itself, the ready-chan close, and the waits on ready):
//   - The INITIATOR reserves the connection-creation permit (tryAcquireConnSlot
//     / h2PromoteLocked), registers this entry with promised=1, unlocks, and
//     dials OUT OF LOCK.
//   - ATTACHERS find this entry with promised < C under the lock, do promised++
//     (+ streams_active++ for their slot), unlock, and WAIT on ready.
//   - On dial SUCCESS the initiator re-locks, converts this entry into a
//     pooledH2Conn{cc, inFlight: promised} (the promised count = initiator +
//     everyone who attached while connecting), moves it into h2Pool[addr], sets
//     cc, and closes ready so attachers ride the now-established conn.
//   - On dial FAILURE the initiator re-locks, sets err, closes ready, and frees
//     ITS OWN slot (permit already released by dialPooledH2To; streams_active--
//     for its own slot; lbRelease already fired by dialPooledH2To). Each
//     attacher wakes on ready, sees err, frees ITS OWN slot (streams_active-- +
//     its own lbRelease) and RETRIES the whole acquire.
//   - A waiting attacher whose ctx fires before ready releases its promised slot
//     (promised-- + streams_active-- + its own lbRelease), with the drain-give-
//     back discipline if ready raced the cancel.
//
// promised never exceeds C (the attach scan only attaches when promised < C).
// A connecting entry holds EXACTLY ONE permit (the initiator's), freed via the
// converted conn's connWithGauge dec on Close (success) or already released by
// dialPooledH2To (failure) — attachers consume NO permit. (phase 43.2a, ADR-0253)
type connectingH2Conn struct {
	ready    chan struct{}
	cc       *h2.ClientConn
	err      error
	promised int64
}

// h2Grant is the wake token handed to a pending H2 waiter. cc != nil ⇒ ride
// this existing conn (a stream slot was reserved on it; no permit, no dial).
// cc == nil ⇒ a permit was reserved; the waiter dials a fresh conn. (ADR-0253)
type h2Grant struct {
	cc *h2.ClientConn
}

// h2Waiter is a pending request in the stream-aware queue. ch is buffered cap-1
// so the grant send under h2PoolMu never blocks. (ADR-0253)
type h2Waiter struct {
	ch chan h2Grant
}

// enqueueWaiterLocked appends w to the per-endpoint stream-aware FIFO and Inc's
// the pending-queue gauges (active + total). It REUSES the 43.1 connPool pending
// handles (the H2 queue shares the 43.1 pending budget + stats per SPEC §3.1) —
// it does NOT register new gauges. Caller holds c.h2PoolMu. The bound check
// (maxPendingRequests → errConnPoolOverflow) lives in the lifecycle (Task 5);
// this primitive only appends + accounts. (phase 43.2a, ADR-0253)
func (c *Cluster) enqueueWaiterLocked(addr string, w *h2Waiter) {
	if c.h2Waiters == nil {
		c.h2Waiters = make(map[string][]*h2Waiter)
	}
	c.h2Waiters[addr] = append(c.h2Waiters[addr], w)
	c.h2PendingInc()
}

// removeH2WaiterLocked drops w from addr's FIFO if present, Dec'ing the pending-
// active gauge on a successful clean removal, and reports whether it was found.
// A false return means w was already dequeued (a grant is en route on its
// buffered channel) — the caller must drain-and-give-back. Caller holds
// c.h2PoolMu. (phase 43.2a, ADR-0253)
func (c *Cluster) removeH2WaiterLocked(addr string, w *h2Waiter) bool {
	q := c.h2Waiters[addr]
	for i, x := range q {
		if x == w {
			c.h2Waiters[addr] = append(q[:i], q[i+1:]...)
			c.h2PendingDec()
			return true
		}
	}
	return false
}

// h2PromoteLocked wakes at most one head waiter for addr after a capacity-
// freeing event (stream release or conn eviction). Caller holds c.h2PoolMu.
// Ordering (D-H2-EVICTORDER): (1) hand a stream slot on a LIVE conn (never a
// Closed one); else (2) reserve a permit and hand a dial-grant; else (3) leave
// the waiter queued. The buffered send happens after the state mutation, still
// under the lock (cap-1 → never blocks). (phase 43.2a, ADR-0253)
func (c *Cluster) h2PromoteLocked(addr string) {
	q := c.h2Waiters[addr]
	if len(q) == 0 {
		return
	}
	// (1) live conn with a free stream slot. A draining conn (GoneAway() — its
	// codec observed a peer GOAWAY) takes NO new streams (Task 3 admission-skip).
	for _, pc := range c.h2Pool[addr] {
		if !pc.cc.Closed() && !pc.cc.GoneAway() && pc.inFlight < c.h2MaxConcurrentStreams {
			pc.inFlight++
			c.h2StreamsActiveInc()
			w := q[0]
			c.h2Waiters[addr] = q[1:]
			c.h2PendingDec()
			w.ch <- h2Grant{cc: pc.cc} // buffered cap-1 → never blocks
			return
		}
	}
	// (2) a free permit ⇒ dial-grant.
	if c.tryAcquireConnSlot() {
		w := q[0]
		c.h2Waiters[addr] = q[1:]
		c.h2PendingDec()
		w.ch <- h2Grant{cc: nil} // buffered cap-1 → never blocks
		return
	}
	// (3) no capacity → leave the waiter queued; a later free event re-promotes.
}

// ---------------------------------------------------------------------------
// Nil-guarded stat helpers (the 43.1 setGauge pattern). The streams_active
// helpers drive the NEW http2.streams_active gauge (Task 6 binds the handle;
// nil until then). The pending helpers REUSE the 43.1 connPool pending handles
// (the H2 queue shares the 43.1 pending budget + stats per SPEC §3.1) and
// nil-guard the circuitBreaker/pool so a no-CB H2 cluster (which never pends)
// is safe. (phase 43.2a, ADR-0253)
// ---------------------------------------------------------------------------

// h2StreamsActiveInc Inc's the cluster's http2.streams_active gauge.
func (c *Cluster) h2StreamsActiveInc() { incGauge(c.http2StreamsActive) }

// h2StreamsActiveDec Dec's the cluster's http2.streams_active gauge.
func (c *Cluster) h2StreamsActiveDec() { decGauge(c.http2StreamsActive) }

// h2PendingInc Inc's upstream_rq_pending_active + upstream_rq_pending_total on
// enqueue (the shared 43.1 connPool handles).
func (c *Cluster) h2PendingInc() {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return
	}
	incGauge(c.circuitBreaker.pool.upstreamRqPendingActive)
	incCounter(c.circuitBreaker.pool.upstreamRqPendingTotal)
}

// h2PendingDec Dec's upstream_rq_pending_active on dequeue / clean removal.
func (c *Cluster) h2PendingDec() {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return
	}
	decGauge(c.circuitBreaker.pool.upstreamRqPendingActive)
}

// h2PendingOverflowInc Inc's upstream_rq_pending_overflow when an H2 request
// hits the max_pending_requests bound (the shared 43.1 connPool handle).
func (c *Cluster) h2PendingOverflowInc() {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return
	}
	incCounter(c.circuitBreaker.pool.upstreamRqPendingOverflow)
}

// h2CxHTTP2TotalInc Inc's the cluster's upstream_cx_http2_total counter (one per
// new pooled H2 conn). nil-guarded; Task 6 binds the handle (useH2-gated).
func (c *Cluster) h2CxHTTP2TotalInc() { incCounter(c.upstreamCxHTTP2Total) }

// h2RxResetInc Inc's the cluster's http2.rx_reset counter (a received RST_STREAM
// observed by the codec). nil-guarded; useH2-gated handle. (phase 43.2b)
func (c *Cluster) h2RxResetInc() { incCounter(c.http2RxReset) }

// h2TxResetInc Inc's the cluster's http2.tx_reset counter (a sent RST_STREAM —
// the codec's downstream-cancel CANCEL). nil-guarded. (phase 43.2b)
func (c *Cluster) h2TxResetInc() { incCounter(c.http2TxReset) }

// h2RxMessagingErrorInc Inc's the cluster's http2.rx_messaging_error counter (an
// inbound message the codec rejected as malformed). nil-guarded VIA incCounter —
// the handle is useH2-gated and therefore nil on every non-H2 cluster, and
// stats.Counter.Inc has no nil guard of its own. (phase 92, ADR-0313)
func (c *Cluster) h2RxMessagingErrorInc() { incCounter(c.http2RxMessagingError) }

// ---------------------------------------------------------------------------
// The acquire / release lifecycle (phase 43.2a, Task 5, ADR-0253)
//
// AcquireH2Stream returns a multiplexed H2 stream slot on a pooled upstream conn
// (dialing a fresh conn or pending in the stream-aware queue as capacity
// demands), plus a release closure the caller MUST invoke EXACTLY ONCE when the
// RoundTrip completes. The router (Task 7) holds the *h2.ClientConn for one
// RoundTrip and calls release() afterward — the call-once contract, mirroring
// the H1 PutIdleH1 discipline (there is no double-release safety; the router
// owns the single call).
//
// TASK 9.5 (connect-time coalescing) added a CONNECTING tier between stream-HIT
// and dial: an in-flight dial (connectingH2Conn) holds EXACTLY ONE permit (the
// initiator's). Concurrent demand ATTACHES to it (promised++) and rides the
// establishing conn — attachers consume NO permit and NO new dial, so K
// concurrent streams at cap C open exactly ceil(K/C) conns. The permit + LB
// traces below cover the initiator, the attacher, and the connecting-dial
// success/failure + attacher-cancel paths.
//
// PERMIT CONSERVATION (the #1 correctness property — every tryAcquireConnSlot
// success pairs with EXACTLY one release on EVERY path):
//   - stream-HIT: rides an existing conn's slot — NO permit reserved, NO release.
//   - connecting-HIT (ATTACH): rides the initiator's establishing conn — NO permit
//     reserved (the initiator holds the conn's single permit), NO release.
//   - MISS + dial success (INITIATOR): the permit reserved by tryAcquireConnSlot
//     is carried by the connecting entry; on conversion it transfers to the new
//     conn's connWithGauge dec closure (freed on conn Close via connDec →
//     releaseConn). evictH2ConnLocked (release of a Closed conn's last stream)
//     Closes the conn → frees that permit.
//   - MISS + dial FAILURE (INITIATOR): the permit is ALREADY released
//     (dialPooledH2To's contract); dialAndPool only removes the connecting entry,
//     decrements streams_active for all promised slots, wakes attachers, and
//     promotes — NEVER releaseConnSlot's.
//   - ATTACHER on connecting-dial FAILURE: holds NO permit → nothing to release;
//     surfaces the SAME dial error (the router owns retry). Its streams_active
//     slot was already decremented by the initiator's failing conversion.
//   - ATTACHER ctx-cancel while connecting (ready not yet closed): holds NO permit
//     → only promised-- + streams_active-- for its own slot.
//   - ATTACHER ctx-cancel racing ready: drains the outcome via attachAfterReady —
//     success ⇒ ride-then-release its real slot (inFlight-- + streams_active-- +
//     promote); failure ⇒ slot already freed by conversion. NO permit either way.
//   - PEND → stream-grant (g.cc != nil): the slot was reserved on g.cc under the
//     lock by h2PromoteLocked — NO permit, NO release; just ride it.
//   - PEND → dial-grant (g.cc == nil): a permit was reserved by h2PromoteLocked;
//     the grant becomes a connecting entry (promised=1) → same initiator
//     dial-success/dial-failure permit handling as the MISS path.
//   - ctx-cancel give-back: a raced grant (removeH2WaiterLocked == false) is
//     drained and given back — stream-grant ⇒ inFlight-- + streams_active-- +
//     promote (hand the slot to the next waiter); dial-grant ⇒ releaseConnSlot +
//     promote (hand the permit to the next waiter). Mirrors the 43.1 drain-give-
//     back.
//
// LB-RELEASE CONSERVATION (Task 7.5, ADR-0253 — the SECOND conservation property,
// traced exactly like the permit): AcquireH2Stream picks the endpoint EXACTLY
// ONCE, ctx-aware (hash key + subset match), obtaining the LB pick's release
// (lbRelease — it accounts load, e.g. least_request active count). lbRelease MUST
// fire EXACTLY ONCE per acquire on EVERY path — either IMMEDIATELY (we created no
// new conn) or TRANSFERRED-to-conn (fired at conn-Close via connDec):
//   - stream-HIT: ride an existing conn (it holds its OWN lbRelease from when IT
//     was dialed) → fire lbRelease() now.
//   - connecting-HIT (ATTACH): ride the initiator's establishing conn (the conn
//     will hold the INITIATOR's lbRelease) → fire OUR lbRelease() now, before the
//     wait on ready. An attacher creates no conn — like a stream-HIT. This fires
//     on EVERY attacher exit (dial-success ride, dial-failure error, cancel).
//   - MISS-dial (INITIATOR): dial the picked ep holding lbRelease → it transfers
//     to the new conn's connWithGauge dec (or fires on dial failure, per
//     dialPooledH2To). dialAndPool never re-fires it.
//   - overflow (queue full): fire lbRelease() before returning errConnPoolOverflow.
//   - PEND → stream-grant: ride an existing conn → fire lbRelease() now.
//   - PEND → dial-grant: become a connecting initiator; dial the held ep with
//     lbRelease (transfer on success / dialPooledH2To fires it on dial failure).
//   - PEND → clean ctx-cancel (still queued): fire lbRelease() then return ctx.Err().
//   - PEND → raced give-back: the give-back handles the GRANT's resources; ALSO
//     fire lbRelease() (our pick) before returning ctx.Err().
//   - errH2GrantRaced (defensive vanished-conn branch): fire lbRelease() before
//     returning. (The attacher fires its lbRelease at attach time, so its
//     defensive vanished-conn branch — attachAfterReady pc==nil — needs no
//     further LB action.)
// A double-release or leak of the LB slot corrupts least_request accounting; the
// trace above keeps it conserved alongside the permit.
//
// The pre-Task-7.5 design picked the endpoint TWICE (a ctx-BLIND PickEndpoint for
// keying + a ctx-aware re-pick inside the dial), mis-keying conns into the wrong
// bucket on multi-endpoint clusters and double-advancing the LB cursor. Task 7.5
// collapsed that to the single ctx-aware pick threaded through dialPooledH2To.
// ---------------------------------------------------------------------------

// AcquireH2Stream reserves a multiplexed H2 stream slot for one RoundTrip. The
// returned release MUST be called exactly once (Task 7's router owns the call).
// Returns errConnPoolOverflow when the pending queue is full, or ctx.Err() on a
// cancel/timeout while pending. (phase 43.2a, ADR-0253)
func (c *Cluster) AcquireH2Stream(ctx context.Context) (cc *h2.ClientConn, release func(), ep Endpoint, err error) {
	// Task 7.5 (ADR-0253): pick the endpoint EXACTLY ONCE, ctx-aware (hash key +
	// subset match), mirroring Dial — and hold its LB release (lbRelease) so it is
	// threaded through EVERY exit path (fired immediately on a non-dialing path, or
	// transferred to the dialed conn on a dialing path). NO second, ctx-blind pick.
	//
	// Task 9.5 (connect-time coalescing): the MISS path is now three-tiered —
	// stream-HIT (ride a live conn) → connecting-HIT (ATTACH to an in-flight dial,
	// no new permit/dial) → dial (only when neither live nor connecting spare
	// exists). A connecting-dial FAILURE wakes its attachers with the SAME error;
	// like the initiator (and the pre-9.5 MISS-dial path) the attacher SURFACES
	// the error to the router (which owns retry uniformly) rather than re-looping
	// here — symmetric semantics, no in-pool retry storm.
	hk, ok := hashKeyFrom(ctx)
	match, hasMatch := subsetMatchFrom(ctx)
	ep, lbRelease, err := c.lb.Pick(hk, ok, match, hasMatch)
	if err != nil {
		return nil, nil, Endpoint{}, err
	}
	addr := ep.Addr()

	c.h2PoolMu.Lock()
	if c.h2Pool == nil {
		c.h2Pool = make(map[string][]*pooledH2Conn)
	}
	if c.h2Waiters == nil {
		c.h2Waiters = make(map[string][]*h2Waiter)
	}
	if c.h2Connecting == nil {
		c.h2Connecting = make(map[string][]*connectingH2Conn)
	}

	// (A) Stream-HIT: ride a live conn with a free stream slot. The conn
	// already holds its OWN lbRelease (from when IT was dialed), so fire ours.
	if pc := c.findStreamHitLocked(addr); pc != nil {
		pc.inFlight++
		c.h2StreamsActiveInc()
		c.h2PoolMu.Unlock()
		lbRelease()
		return pc.cc, c.makeRelease(addr, pc), ep, nil
	}

	// (B) Connecting-HIT (Task 9.5): ATTACH to an in-flight dial with a spare
	// promised slot. promised++ + streams_active++ (commit our slot now); fire
	// our OWN lbRelease immediately (we created no conn — like a stream-HIT, we
	// ride the initiator's establishing conn). Then wait on ready.
	if cn := c.findConnectingHitLocked(addr); cn != nil {
		cn.promised++
		c.h2StreamsActiveInc()
		c.h2PoolMu.Unlock()
		lbRelease() // attacher: rides the initiator's conn → release our pick.
		return c.awaitAttach(ctx, addr, cn, ep)
	}

	// (C) MISS: try to reserve a permit and dial a fresh conn to the PICKED
	// ep as the INITIATOR of a connecting entry (promised=1 + streams_active++
	// for our own slot), transferring lbRelease to the new conn (no re-pick).
	if c.tryAcquireConnSlot() {
		cn := &connectingH2Conn{ready: make(chan struct{}), promised: 1}
		c.h2Connecting[addr] = append(c.h2Connecting[addr], cn)
		c.h2StreamsActiveInc() // the initiator's own slot.
		c.h2PoolMu.Unlock()
		return c.dialAndPool(ctx, addr, ep, lbRelease, cn)
	}

	// (D) No permit (at max_connections) → PEND in the stream-aware queue,
	// bounded by max_pending_requests. The nil-pool case never reaches here
	// (tryAcquireConnSlot returns true with no pool), so c.circuitBreaker.pool
	// is non-nil; read maxPendingRequests directly.
	maxPending := c.circuitBreaker.pool.maxPendingRequests
	if int64(len(c.h2Waiters[addr])) >= maxPending {
		c.h2PendingOverflowInc()
		c.h2PoolMu.Unlock()
		lbRelease() // overflow: we created no conn → release our pick now.
		return nil, nil, ep, errConnPoolOverflow
	}
	w := &h2Waiter{ch: make(chan h2Grant, 1)}
	c.enqueueWaiterLocked(addr, w)
	c.h2PoolMu.Unlock()

	select {
	case g := <-w.ch:
		// A grant arrived (sent under the lock by h2PromoteLocked).
		if g.cc != nil {
			// Stream-grant: the slot was already reserved on g.cc (inFlight++ +
			// streams_active++ done under the lock by h2PromoteLocked). Ride it.
			c.h2PoolMu.Lock()
			pc := c.findPooledLocked(addr, g.cc)
			c.h2PoolMu.Unlock()
			if pc == nil {
				// Defensive: the conn vanished between grant and lookup (eviction
				// race). Treat the reserved slot as freed: undo + promote the next.
				// errH2GrantRaced (NOT errConnPoolOverflow) so Task 7 retries this
				// rather than failing fast — and so it is not miscounted as an
				// overflow (no pending_overflow Inc here).
				// Defensive: not directly unit-tested — requires EvictH2ConnOnError to
				// race the buffered grant send; accounting (streams_active--, no
				// permit, lbRelease fired) is correct by inspection.
				c.h2PoolMu.Lock()
				c.h2StreamsActiveDec()
				c.h2PromoteLocked(addr)
				c.h2PoolMu.Unlock()
				lbRelease() // created no conn → release our pick.
				return nil, nil, ep, errH2GrantRaced
			}
			lbRelease() // ride an existing conn (it holds its own release).
			return g.cc, c.makeRelease(addr, pc), ep, nil
		}
		// Dial-grant: a permit was reserved for us; dial + pool a fresh conn as
		// the INITIATOR of a connecting entry (promised=1 + streams_active++),
		// transferring lbRelease to it (no re-pick).
		c.h2PoolMu.Lock()
		cn := &connectingH2Conn{ready: make(chan struct{}), promised: 1}
		c.h2Connecting[addr] = append(c.h2Connecting[addr], cn)
		c.h2StreamsActiveInc()
		c.h2PoolMu.Unlock()
		return c.dialAndPool(ctx, addr, ep, lbRelease, cn)

	case <-ctx.Done():
		c.h2PoolMu.Lock()
		if c.removeH2WaiterLocked(addr, w) {
			// Still queued → clean cancel; no grant was sent.
			c.h2PoolMu.Unlock()
			lbRelease() // clean cancel: created no conn → release our pick.
			return nil, nil, ep, ctx.Err()
		}
		c.h2PoolMu.Unlock()
		// Already dequeued → a grant is en route on the buffered channel. Drain
		// it and give it back (the 43.1 drain-give-back discipline).
		g := <-w.ch
		c.h2PoolMu.Lock()
		if g.cc != nil {
			// Stream-grant: the reserved slot is ours to hand back. Decrement the
			// conn's inFlight + streams_active, then promote the next waiter onto
			// the freed slot.
			if pc := c.findPooledLocked(addr, g.cc); pc != nil {
				pc.inFlight--
			}
			c.h2StreamsActiveDec()
			c.h2PromoteLocked(addr)
		} else {
			// Dial-grant: a permit was reserved for us. Give it back + promote.
			c.releaseConnSlot()
			c.h2PromoteLocked(addr)
		}
		c.h2PoolMu.Unlock()
		// The give-back above handled the GRANT's resources; ALSO release our own
		// LB pick (it is independent of the grant's stream slot / permit).
		lbRelease()
		return nil, nil, ep, ctx.Err()
	}
}

// findStreamHitLocked returns a live pooled conn for addr with a free stream
// slot (inFlight < C), or nil. Caller holds c.h2PoolMu. (phase 43.2a, Task 9.5)
func (c *Cluster) findStreamHitLocked(addr string) *pooledH2Conn {
	for _, pc := range c.h2Pool[addr] {
		// Skip a draining conn (GoneAway() — its codec observed a peer GOAWAY): it
		// takes NO new streams, only drains its in-flight ones (Task 3).
		if !pc.cc.Closed() && !pc.cc.GoneAway() && pc.inFlight < c.h2MaxConcurrentStreams {
			return pc
		}
	}
	return nil
}

// findConnectingHitLocked returns an in-flight dial for addr with a spare
// promised slot (promised < C), or nil. Caller holds c.h2PoolMu. The attach
// scan only attaches when promised < C, so promised never exceeds C. (Task 9.5)
func (c *Cluster) findConnectingHitLocked(addr string) *connectingH2Conn {
	for _, cn := range c.h2Connecting[addr] {
		if cn.promised < c.h2MaxConcurrentStreams {
			return cn
		}
	}
	return nil
}

// removeConnectingLocked drops cn from addr's connecting set if present. Caller
// holds c.h2PoolMu. (phase 43.2a, Task 9.5)
func (c *Cluster) removeConnectingLocked(addr string, cn *connectingH2Conn) {
	q := c.h2Connecting[addr]
	for i, x := range q {
		if x == cn {
			c.h2Connecting[addr] = append(q[:i], q[i+1:]...)
			return
		}
	}
}

// awaitAttach is the ATTACHER's wait on a connecting conn's ready chan (Task
// 9.5). It has ALREADY committed its slot (promised++ + streams_active++) and
// fired its own lbRelease before calling this. It returns:
//   - (cc, release, ep, nil): dial succeeded → ride the now-established conn (our
//     reserved promised slot was converted into inFlight by the initiator).
//   - (nil, nil, ep, cn.err): the dial FAILED — surface the SAME error to the
//     router (symmetric with the initiator + the pre-9.5 MISS-dial path; the
//     router owns retry). Our slot was already freed (streams_active--) by the
//     failing conversion.
//   - (nil, nil, ep, ctx.Err()): our ctx fired before ready → we released our
//     promised slot (promised-- + streams_active--) OR, if ready raced us,
//     drained the outcome and gave it back. lbRelease already fired by the caller.
func (c *Cluster) awaitAttach(ctx context.Context, addr string, cn *connectingH2Conn, ep Endpoint) (*h2.ClientConn, func(), Endpoint, error) {
	select {
	case <-cn.ready:
		return c.attachAfterReady(addr, cn, ep)
	case <-ctx.Done():
		c.h2PoolMu.Lock()
		select {
		case <-cn.ready:
			// ready raced our cancel: the dial outcome is committed. Honor the
			// cancel but give our slot back through the post-ready path (which
			// rides-then-releases on success, or treats the failure as already-freed).
			c.h2PoolMu.Unlock()
			_, release, _, aerr := c.attachAfterReady(addr, cn, ep)
			if aerr != nil {
				// Dial failed (or vanished): attachAfterReady already freed our slot.
				// Honor the cancel.
				return nil, nil, ep, ctx.Err()
			}
			// Dial succeeded and we hold a real stream slot — but the caller
			// canceled, so hand the freshly-acquired slot straight back + honor
			// the cancel.
			release()
			return nil, nil, ep, ctx.Err()
		default:
			// Still connecting: release our reserved promised slot. If the dial
			// later succeeds it pools a conn with inFlight==the SURVIVING promised
			// count; on failure the converter decrements only the surviving slots.
			// Decrement streams_active for our slot.
			cn.promised--
			c.h2StreamsActiveDec()
			c.h2PoolMu.Unlock()
			return nil, nil, ep, ctx.Err()
		}
	}
}

// attachAfterReady completes an attacher once the connecting conn's ready chan
// is closed (Task 9.5). On dial success it rides the established conn (our
// promised slot was converted into the pooled conn's inFlight by the initiator).
// On dial failure our slot was already freed (streams_active--) by the failing
// conversion, so it surfaces cn.err (the router owns retry). Caller must NOT hold
// c.h2PoolMu. Returns (cc, release, ep, nil) on success or (nil, nil, ep, err).
func (c *Cluster) attachAfterReady(addr string, cn *connectingH2Conn, ep Endpoint) (*h2.ClientConn, func(), Endpoint, error) {
	c.h2PoolMu.Lock()
	if cn.err != nil {
		// Dial failed: the conversion already decremented streams_active for ALL
		// promised slots (initiator + attachers) and the permit/lbRelease were
		// released by dialPooledH2To. Our slot is gone; surface the dial error.
		c.h2PoolMu.Unlock()
		return nil, nil, ep, cn.err
	}
	pc := c.findPooledLocked(addr, cn.cc)
	if pc == nil {
		// Defensive: the established conn was evicted (EvictH2ConnOnError) between
		// conversion and our wake. Our promised slot was converted into the conn's
		// inFlight at conversion but the eviction did not account it; free our slot
		// (streams_active--) and surface the retryable grant-raced signal (the
		// router re-acquires rather than failing fast — mirrors errH2GrantRaced).
		// Defensive: not directly unit-tested — requires EvictH2ConnOnError to race
		// close(ready); accounting (streams_active--, no permit) is correct by
		// inspection.
		c.h2StreamsActiveDec()
		c.h2PoolMu.Unlock()
		return nil, nil, ep, errH2GrantRaced
	}
	c.h2PoolMu.Unlock()
	return cn.cc, c.makeRelease(addr, pc), ep, nil
}

// dialAndPool is the INITIATOR's dial of a connecting entry cn (Task 9.5). The
// permit is ALREADY reserved by the caller (tryAcquireConnSlot / h2PromoteLocked)
// and cn is ALREADY registered in c.h2Connecting[addr] with promised>=1 and the
// initiator's streams_active slot already counted. lbRelease is the caller's
// single ctx-aware LB pick release. It dials the ALREADY-PICKED ep OUTSIDE the
// lock (so concurrent acquirers can ATTACH to cn while it establishes), then:
//
//   - On SUCCESS: under the lock it CONVERTS cn → a pooledH2Conn{cc, inFlight:
//     cn.promised} (promised = the initiator + everyone who attached while
//     connecting), removes cn from the connecting set, appends the pooled conn,
//     Inc's upstream_cx_http2_total, sets cn.cc, closes cn.ready (so attachers
//     ride the established conn), and promotes any permit-pending waiter. NO
//     streams_active Inc here — every promised slot was already counted at its
//     commit (initiator at registration, each attacher at attach). lbRelease
//     already transferred to the conn's connWithGauge dec (fired at conn Close).
//   - On FAILURE: the permit + lbRelease were already released by dialPooledH2To
//     (its contract). Under the lock it removes cn, sets cn.err, decrements
//     streams_active by cn.promised (the initiator's slot + every attacher's —
//     they all abandon and re-acquire), closes cn.ready (waking attachers to
//     retry), and promotes the next waiter onto whatever capacity freed.
//
// Returns the INITIATOR's own stream + release on success, or the dial error.
// (phase 43.2a, ADR-0253; Task 7.5; Task 9.5)
func (c *Cluster) dialAndPool(ctx context.Context, addr string, ep Endpoint, lbRelease func(), cn *connectingH2Conn) (*h2.ClientConn, func(), Endpoint, error) {
	cc, err := c.dialPooledH2To(ctx, ep, lbRelease)
	if err != nil {
		// The permit + lbRelease are already released (dialPooledH2To contract).
		// Fail the connecting entry: free ALL its promised slots' streams_active
		// (initiator + attachers re-acquire), wake attachers to retry, promote.
		c.h2PoolMu.Lock()
		c.removeConnectingLocked(addr, cn)
		cn.err = err
		for i := int64(0); i < cn.promised; i++ {
			c.h2StreamsActiveDec()
		}
		close(cn.ready)
		c.h2PromoteLocked(addr)
		c.h2PoolMu.Unlock()
		return nil, nil, ep, err
	}
	c.h2PoolMu.Lock()
	c.removeConnectingLocked(addr, cn)
	pc := &pooledH2Conn{cc: cc, inFlight: cn.promised}
	c.h2Pool[addr] = append(c.h2Pool[addr], pc)
	go c.watchDrain(addr, cc)
	c.h2CxHTTP2TotalInc()
	cn.cc = cc
	close(cn.ready) // attachers ride the now-established conn.
	// A new conn with spare promised capacity (inFlight < C) may satisfy a
	// permit-pending waiter via a stream-grant; promote.
	c.h2PromoteLocked(addr)
	c.h2PoolMu.Unlock()
	return cc, c.makeRelease(addr, pc), ep, nil
}

// makeRelease builds the call-once release closure bound to (addr, pc). On
// release: inFlight-- + streams_active-- + promote the next waiter; then if the
// conn is Closed() with no in-flight streams, evict + Close it (freeing its
// permit) and promote again (the freed permit may now satisfy a dial-grant).
// (phase 43.2a, ADR-0253)
func (c *Cluster) makeRelease(addr string, pc *pooledH2Conn) func() {
	return func() {
		c.h2PoolMu.Lock()
		pc.inFlight--
		c.h2StreamsActiveDec()
		c.h2PromoteLocked(addr)
		// Eager-close on the last in-flight stream of a conn that is Closed OR
		// draining (GoneAway() — its codec observed a peer GOAWAY): a draining conn
		// took no new streams (admission-skip), so once it drains to 0 it closes +
		// frees its permit (Task 3 eviction generalization).
		if (pc.cc.Closed() || pc.cc.GoneAway()) && pc.inFlight == 0 {
			c.evictH2ConnLocked(addr, pc)
			// The eviction freed a permit (via cc.Close → connDec → releaseConn);
			// promote again so a queued waiter can take a dial-grant.
			c.h2PromoteLocked(addr)
		}
		c.h2PoolMu.Unlock()
	}
}

// evictH2ConnLocked removes pc from the per-endpoint pool slice and Closes its
// conn (which runs connDec → releaseConn, freeing the pool's permit + Dec'ing
// upstream_cx_active). Caller holds c.h2PoolMu. Used by makeRelease (the
// Closed-conn last-stream case) and by Task 7's RoundTrip-error path.
// (phase 43.2a, ADR-0253)
func (c *Cluster) evictH2ConnLocked(addr string, pc *pooledH2Conn) {
	q := c.h2Pool[addr]
	for i, x := range q {
		if x == pc {
			c.h2Pool[addr] = append(q[:i], q[i+1:]...)
			break
		}
	}
	_ = pc.cc.Close()
}

// EvictH2ConnOnError removes the pooled conn wrapping cc (for ep's endpoint)
// from the pool after a RoundTrip transport error, so the poisoned conn is not
// reused by a later acquire. It is the exported, router-facing wrapper over
// evictH2ConnLocked: it takes c.h2PoolMu, locates the pooledH2Conn for cc, and
// (if still pooled) evicts + Closes it, then promotes the next waiter (the
// eviction frees a permit that a queued dial-grant can take).
//
// Idempotent + safe under concurrency: if cc was already evicted (e.g. by
// makeRelease's Closed-conn last-stream case, or a concurrent error path),
// findPooledLocked returns nil and this is a no-op. The conn's OWN slot is still
// released separately by the router's deferred release() — eviction removes the
// conn from the reuse set; release() accounts the stream slot. evictH2ConnLocked
// Closes the conn, which (once inFlight drains to 0 via release) runs connDec →
// releaseConn to free the permit; calling Close here is safe even with the
// caller's stream still in-flight because makeRelease re-checks Closed()+inFlight
// and the gauge dec is permit-conserving. (phase 43.2a, ADR-0253; Task 7)
func (c *Cluster) EvictH2ConnOnError(cc *h2.ClientConn, ep Endpoint) {
	addr := ep.Addr()
	c.h2PoolMu.Lock()
	defer c.h2PoolMu.Unlock()
	pc := c.findPooledLocked(addr, cc)
	if pc == nil {
		return // already evicted (idempotent)
	}
	c.evictH2ConnLocked(addr, pc)
	c.h2PromoteLocked(addr)
}

// findPooledLocked returns the pooledH2Conn wrapping cc for addr, or nil if it
// is no longer pooled (evicted). Caller holds c.h2PoolMu. (phase 43.2a)
func (c *Cluster) findPooledLocked(addr string, cc *h2.ClientConn) *pooledH2Conn {
	for _, pc := range c.h2Pool[addr] {
		if pc.cc == cc {
			return pc
		}
	}
	return nil
}

// watchDrain is the per-conn drain-watcher (phase 43.2b, ADR-0254). One
// goroutine per pooled conn, spawned at pool insertion (dialAndPool). It closes
// an IDLE draining conn promptly: such a conn has no in-flight stream whose
// release() would evict it, and its ctx is never canceled, so without this
// watcher it would be admission-skipped forever but never closed (a conn leak).
//
// Single-shot: it acts once then exits.
//   - GoneAwayCh fires (peer GOAWAY): re-lock h2PoolMu, re-check the conn is
//     still pooled AND idle (the findPooledLocked != nil guard dodges the
//     double-evict race with EvictH2ConnOnError / makeRelease); if so evict+Close
//     it and promote a waiter onto the freed permit. If inFlight > 0 the last
//     release()'s generalized eager-close (Task 3) does the eviction instead, so
//     this is a no-op and the watcher just exits.
//   - Done fires (conn evicted/closed by another path): exit, no action.
func (c *Cluster) watchDrain(addr string, cc *h2.ClientConn) {
	select {
	case <-cc.GoneAwayCh():
		c.h2PoolMu.Lock()
		if pc := c.findPooledLocked(addr, cc); pc != nil && pc.inFlight == 0 {
			c.evictH2ConnLocked(addr, pc)
			c.h2PromoteLocked(addr)
		}
		c.h2PoolMu.Unlock()
	case <-cc.Done():
		// evicted/closed by another path (EvictH2ConnOnError / makeRelease) → nothing to do.
	}
}
