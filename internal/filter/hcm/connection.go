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
func runConnection(ctx context.Context, downstream net.Conn, table *routeTable) {
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
			}
			return
		}

		if req.Header.Get("Expect") != "" {
			_ = writeStatusReply(bw, 417, "")
			_ = bw.Flush()
			drainAndClose(req)
			return
		}
		if req.Header.Get("Upgrade") != "" || strings.EqualFold(req.Header.Get("Connection"), "Upgrade") {
			_ = writeStatusReply(bw, 501, "")
			_ = bw.Flush()
			drainAndClose(req)
			return
		}

		closeAfterRequest := strings.EqualFold(req.Header.Get("Connection"), "close")
		closeAfterAction := false

		entry, ok := table.match(req)
		if !ok {
			_ = writeStatusReply(bw, 404, "")
		} else {
			actErr := entry.action.do(ctx, req, bw)
			if errors.Is(actErr, errCloseAfterAction) {
				closeAfterAction = true
			} else if actErr != nil {
				log.Printf("hcm: action: %v", actErr)
				_ = bw.Flush()
				drainAndClose(req)
				return
			}
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
