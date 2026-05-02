// Command backends is the listener-address-echo TCP backend for fixture
// 0008-listener-chain-match. It binds a TCP listener on a configurable port;
// for each accepted connection it reads until EOF and writes back the
// listener's local address (formatted as `127.0.0.1:NNNN\n`), then closes.
//
// The fixture spawns one instance of this backend per chain-distinguishing
// cluster (c_dstport, c_srcprefix, c_other, c_default → 4 instances; the
// runner may allocate a 5th for symmetry with the 5-connection workload).
// Each instance's response body uniquely identifies which chain handled the
// connection, which is the differential dimension fixture-0008 asserts
// (per SPEC §9.4).
//
// CLI convention: `--port <port>` flag matches the existing fixture-0001 /
// 0005 / 0006 / 0007a / 0007b backend shape; the runner's `start*Backend`
// helpers all spawn via this flag. (The PLAN brief mentioned BACKEND_PORT
// env var but the runner-spawn pattern uniformly uses --port; this backend
// follows the established pattern.)
//
// Usage:
//
//	backends --port <port>
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
)

func main() {
	port := flag.Int("port", 0, "TCP port to listen on (0 = OS-chosen)")
	flag.Parse()

	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", *port))
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()
	log.Printf("backend listening on %s", addr)

	// Echo body: the listener's local address (verbatim Addr().String()).
	// Newline-terminated so the differential gate's byte comparison is
	// line-stable. Per-connection bytes are identical (the response is the
	// listener's own address, not connection-specific data).
	body := []byte(addr + "\n")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatalf("accept: %v", err)
		}
		go handle(conn, body)
	}
}

// handle reads the inbound bytes until EOF (draining the request payload),
// writes the fixed echo body, and closes. The drain ensures the proxy's
// upstream-to-downstream pipe sees the full backend response before the
// proxy itself sees connection close.
func handle(conn net.Conn, body []byte) {
	defer func() { _ = conn.Close() }()
	_, _ = io.Copy(io.Discard, conn)
	_, _ = conn.Write(body)
}
