// Backend for fixture 0004-h2-routing.
//
// Listens on a TLS port advertising NextProtos=["h2"] via http2.ConfigureServer
// (driver-side; D-3.2 governs envoy-go runtime, not test backends — the test
// tree may import http2.Server freely).
//
// Routes:
//   - /health        -> 200 "OK\n"
//   - /api/v1/<tail> -> 200 "backend-<idx>:v1/<tail>"
//   - /api/v1/reflect -> 200 "reflect:probe=<v>,padlen=<n>" (phase-88 request arm)
//   - /api/v1/emit    -> 200 "emit-ok" + a split response header block (phase-88)
//   - /api/v1/reflect-headers/<arm> -> 200, a SORTED `name: value` block of the
//     request headers actually received (phase-89 decode-mutation arms)
//   - /api/v1/p92-<shape> -> 200 "p92-ok" + ONE illegal connection-specific
//     RESPONSE header (phase-92 response-header-validation arms)
//   - any other path -> 404 "not found\n"
//
// BACKEND_IDX env var supplies the per-instance numeric idx that flows into the
// /api/v1 response body (used by the driver's distribution + body-equivalence
// assertions).
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"golang.org/x/net/http2"
)

// contPadLen is the pad-header length used by the phase-88 CONTINUATION arms.
//
// ⚠️ The control variable is the HPACK-ENCODED block size, NOT the raw byte
// count. MEASURED at this fixture's entropy with contPad's alphabet:
//
//	1024 B  -> 774 B encoded   -> one HEADERS frame
//	16000 B -> 11902 B encoded -> one HEADERS frame
//	32000 B -> 23792 B encoded -> HEADERS + CONTINUATION (exceeds the 16384 B
//	                              RFC 9113 §6.5.2 default SETTINGS_MAX_FRAME_SIZE)
//
// The flip is at the FRAME-SPLIT boundary, not at a header-size threshold.
// Changing contPad's alphabet changes bits-per-character and therefore moves
// the split point — RE-MEASURE the encoded size if you touch it.
const contPadLen = 32000

// contPad returns an n-byte pad drawn from a rotating lowercase+digit alphabet
// (~5.95 HPACK-Huffman bits per character; see contPadLen for the measurement).
func contPad(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[i%len(alphabet)]
	}
	return string(b)
}

func main() {
	port := flag.String("port", "0", "listen port (numeric)")
	cert := flag.String("cert", "", "server cert PEM path")
	key := flag.String("key", "", "server key PEM path")
	flag.Parse()
	idx := os.Getenv("BACKEND_IDX")
	if idx == "" {
		idx = "?"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "OK\n")
	})
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/")
		_, _ = fmt.Fprintf(w, "backend-%s:v1/%s", idx, suffix)
	})
	// Phase-88 request-direction reporter. Exact-match patterns beat the
	// "/api/v1/" subtree handler above, so no existing behavior moves.
	//
	// Reports the LENGTH of the large request header the backend actually
	// received. The length is the load-bearing datum: the CONTINUATION-discard
	// defect is PARTIAL (fields encoded before the split survive), so reporting
	// only the small probe header's presence would read GREEN on a broken tip.
	mux.HandleFunc("/api/v1/reflect", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "reflect:probe=%s,padlen=%d",
			r.Header.Get("X-Cont-Probe"), len(r.Header.Get("X-Cont-Pad")))
	})
	// Phase-88 response-direction emitter: a response header block that
	// HPACK-encodes past the peer's SETTINGS_MAX_FRAME_SIZE, so x/net's server
	// codec must split it into HEADERS + CONTINUATION on the upstream wire.
	mux.HandleFunc("/api/v1/emit", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Cont-Marker", "emitted")
		w.Header().Set("X-Cont-Pad", contPad(contPadLen))
		_, _ = fmt.Fprint(w, "emit-ok")
	})
	// Phase-92 illegal-response-header emitters (ONE ILLEGAL SHAPE PER PATH).
	//
	// x/net's H/2 server deletes ONLY `Connection` from a handler's response
	// header map -- server.go carries a live
	// `TODO: remove more Connection-specific header fields here` right beside
	// that delete -- so every other connection-specific field a handler sets
	// travels verbatim onto the upstream wire. These handlers are the
	// fixture's source of the illegal RESPONSE shapes phase 92 must make the
	// proxy reject.
	//
	// ⚠️ ONE ILLEGAL FIELD PER PATH, deliberately. A single path emitting all
	// three at once would be BLIND to a fix that catches one shape and
	// launders another: one arm would keep showing a non-empty illegal set and
	// no per-shape verdict could be read out of it.
	//
	// ⚠️ Registered as EXACT patterns, so Go's ServeMux prefers them over the
	// `/api/v1/` subtree handler above and no existing response body moves.
	// They sit UNDER /api so the fixture route table needs NO edit: both
	// sides' `- match: { prefix: "/api" }` -> c_h2_backend already covers
	// them. A TOP-LEVEL `/p92-*` path would NOT match that prefix -- it falls
	// through to `- match: { prefix: "/" }` and is answered by a 404
	// direct_response that never reaches a backend at all.
	//
	// ⚠️ FOUR ILLEGAL SHAPES ARE STRUCTURALLY UNREACHABLE from a net/http
	// backend and are deliberately absent here: `connection` (the H/2 server
	// deletes it), `transfer-encoding` (the H/2 server frames the body itself
	// and never emits it), an UPPERCASE wire name (the HPACK encoder
	// lowercases every field name), and a DUPLICATE `content-length` (the
	// server synthesizes exactly one). Those are pinned at the unit layer.
	mux.HandleFunc("/api/v1/p92-keepalive", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Keep-Alive", "timeout=5, max=100")
		_, _ = fmt.Fprint(w, "p92-ok")
	})
	mux.HandleFunc("/api/v1/p92-upgrade", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Upgrade", "websocket")
		_, _ = fmt.Fprint(w, "p92-ok")
	})
	mux.HandleFunc("/api/v1/p92-proxyconn", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Proxy-Connection", "keep-alive")
		_, _ = fmt.Fprint(w, "p92-ok")
	})
	// PROBE PATHS (phase-92 T2 measurement obligation): whether a `te` field
	// set by a net/http handler survives onto the upstream H/2 wire at all was
	// UNMEASURED. Driven and read on the wire; kept only if they leak.
	mux.HandleFunc("/api/v1/p92-te-gzip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Te", "gzip")
		_, _ = fmt.Fprint(w, "p92-ok")
	})
	mux.HandleFunc("/api/v1/p92-te-empty", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Te", "")
		_, _ = fmt.Fprint(w, "p92-ok")
	})
	// Phase-89 decode-side filter-mutation reporter. Reflects the request
	// headers this backend ACTUALLY received, one `name: value` line per
	// (name, value) pair, SORTED.
	//
	// The sort is load-bearing, not cosmetic: Go map iteration is
	// nondeterministic, so an unsorted block would vary run to run and no
	// stable assertion could be written against it. (Pattern copied from
	// test/fixtures/0012-http-header-mutation/backends/backend.go.)
	//
	// ⚠️ Registered as a SUBTREE pattern (trailing `/`), NOT an exact one:
	// every driver arm needs its OWN request path so it can carry its OWN
	// route-level `typed_per_filter_config`, so the arms live at
	// `/api/v1/reflect-headers/a1` … `/a8`. Go's ServeMux prefers the longest
	// matching pattern, so this beats the `/api/v1/` subtree handler above and
	// no existing driver path moves.
	//
	// ⚠️ The reflected block is NOT byte-comparable across sides — reference
	// Envoy adds `x-request-id` (random UUID), `x-forwarded-proto` and
	// `x-envoy-expected-rq-timeout-ms` that envoy-go does not. The driver
	// parses this body and asserts named headers IN-BAND; it never writes the
	// body into the differential transcript.
	mux.HandleFunc("/api/v1/reflect-headers/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body) // drain the POST arm's body
		names := make([]string, 0, len(r.Header))
		for n := range r.Header {
			names = append(names, n)
		}
		sort.Strings(names)
		var b strings.Builder
		for _, n := range names {
			for _, v := range r.Header[n] {
				fmt.Fprintf(&b, "%s: %s\n", n, v)
			}
		}
		// Phase 90: the AUTHORITY the proxy forwarded. x/net's H/2 server puts
		// :authority into r.Host (server.go:2373) and leaves a regular `host`
		// field in r.Header as "Host" (only "Trailer" is deleted, :2341), so the
		// sorted block above already shows arm A while r.Host shows arm B.
		// Emitted AFTER the sort, deliberately: folding it into `names` would let
		// a lexical sort move it and re-baseline every existing arm.
		fmt.Fprintf(&b, "x-observed-authority: %s\n", r.Host)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(b.String()))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found\n"))
	})

	tlsCert, err := tls.LoadX509KeyPair(*cert, *key)
	if err != nil {
		log.Fatalf("load cert: %v", err)
	}
	srv := &http.Server{
		Addr:    ":" + *port,
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			NextProtos:   []string{"h2"},
		},
	}
	if err := http2.ConfigureServer(srv, &http2.Server{}); err != nil {
		log.Fatalf("ConfigureServer: %v", err)
	}
	log.Printf("backend %s listening on %s", idx, srv.Addr)
	if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ListenAndServeTLS: %v", err)
	}
}
