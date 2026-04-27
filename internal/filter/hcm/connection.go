package hcm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
)

// runConnection drives one downstream HTTP/1.1 connection from acceptance to
// close. The loop reads requests via http.ReadRequest off a bufio.Reader
// over downstream, applies phase-04 out-of-scope guards (Expect:→417,
// Upgrade:→501), dispatches each request through the route table, and exits
// on:
//   - clean EOF from the downstream
//   - any non-EOF parse error (a 400 is sent before close)
//   - phase-04-out-of-scope guard trip (417 or 501 before close)
//   - Connection: close on the request OR the response signaling
//     errCloseAfterAction
//
// On TLS listeners with ALPN, this driver is reached via filter.Handle's
// codec dispatch when the negotiated ALPN is "http/1.1" (or the codec
// is set to HTTP1 explicitly); see ADR-0050 and SPEC §5.4 for the
// dispatch contract.
//
// Per SPEC §5.3 / §5.6 / SPEC §10 #3 settled.
//
// Phase 06.1 Task 11: f's per-instance HCM counters are incremented on
// dispatch entry (downstream_rq_total) and on response status finalization
// (downstream_rq_<Nxx>) per SPEC §5.5 + §12 #1 site (a). The dispatch-entry
// hook fires once per successfully-parsed request; the status-class hook
// fires once per response (including the 400/404/417/501/503/etc. local-
// reply paths so the integer-divide-by-100 discipline observes every
// served response).
func runConnection(ctx context.Context, downstream net.Conn, f *Filter) {
	defer func() { _ = downstream.Close() }()

	br := bufio.NewReader(downstream)
	bw := bufio.NewWriter(downstream)

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		req, err := http.ReadRequest(br)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = writeStatusReply(bw, 400, "")
				_ = bw.Flush()
				// Parse-error is a synthesized 400 response — a real served
				// response, so Inc downstream_rq_total + the 4xx class
				// counter to keep observability honest (matches Envoy's
				// "request observed" semantics on the parse-error path).
				f.downstreamRqTotal.Inc()
				if c := f.downstreamStatusClassCounter(400); c != nil {
					c.Inc()
				}
			}
			return
		}

		// Per SPEC §12 #1 site (a): on first byte of request line/headers in
		// connection.go's read loop. http.ReadRequest succeeded → this is the
		// once-per-request dispatch-entry hook.
		f.downstreamRqTotal.Inc()

		if req.Header.Get("Expect") != "" {
			_ = writeStatusReply(bw, 417, "")
			_ = bw.Flush()
			if c := f.downstreamStatusClassCounter(417); c != nil {
				c.Inc()
			}
			drainAndClose(req)
			return
		}
		if req.Header.Get("Upgrade") != "" || strings.EqualFold(req.Header.Get("Connection"), "Upgrade") {
			_ = writeStatusReply(bw, 501, "")
			_ = bw.Flush()
			if c := f.downstreamStatusClassCounter(501); c != nil {
				c.Inc()
			}
			drainAndClose(req)
			return
		}

		closeAfterRequest := strings.EqualFold(req.Header.Get("Connection"), "close")
		closeAfterAction := false

		entry, ok := f.table.match(req)
		var status int
		if !ok {
			_ = writeStatusReply(bw, 404, "")
			status = 404
		} else {
			s, actErr := entry.action.do(ctx, req, bw)
			status = s
			if errors.Is(actErr, errCloseAfterAction) {
				closeAfterAction = true
			} else if actErr != nil {
				log.Printf("hcm: action: %v", actErr)
				// Inc the response-class counter for whatever the action
				// finalized before the writer error: status was already set
				// by do() (e.g. 200/502/503), so the integer-divide bucket
				// reflects what HCM produced even when the downstream
				// flush failed.
				if c := f.downstreamStatusClassCounter(status); c != nil {
					c.Inc()
				}
				_ = bw.Flush()
				drainAndClose(req)
				return
			}
		}

		// Response status finalized (status is set on every code path above:
		// 404 catch-all, action.do return). Inc the bucket per SPEC §5.5
		// "switch on response status class → downstream_rq_<Nxx>.Inc() once
		// per response". Lives BEFORE bw.Flush per SPEC's "before bytes
		// hit the wire" — Inc'ing post-Flush would skew on flush-error.
		if c := f.downstreamStatusClassCounter(status); c != nil {
			c.Inc()
		}

		if err := bw.Flush(); err != nil {
			drainAndClose(req)
			return
		}

		drainAndClose(req)

		if closeAfterRequest || closeAfterAction {
			return
		}
	}
}

// drainAndClose discards any unread request body bytes and closes the body.
// Without this, the next iteration's http.ReadRequest would read stale body
// bytes as the request line.
func drainAndClose(req *http.Request) {
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
}
