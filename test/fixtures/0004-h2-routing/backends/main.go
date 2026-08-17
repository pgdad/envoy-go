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
