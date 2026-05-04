// Package main implements the fixture 0012-http-header-mutation echo backend.
// Bound to a runner-allocated port via --port flag. The / endpoint reflects
// received request headers into the response body (one header per line,
// sorted for determinism) and emits a single-value X-Resp-Test header and a
// multi-value X-Multi header for OVERWRITE / APPEND multi-value testing per
// phase 10 SPEC §7.5 + §11.4.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func main() {
	port := flag.Int("port", 18012, "listen port")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Reflect received request headers into the response body, sorted
		// for determinism (Go map iteration is non-deterministic; sort is
		// required so reference vs envoy-go body bytes compare equal).
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
		body := b.String()
		// Single-value response header for OVERWRITE_IF_EXISTS variants.
		w.Header().Set("X-Resp-Test", "backend-original")
		// Multi-value response header for APPEND/OVERWRITE multi-value testing per §11.4.
		w.Header().Add("X-Multi", "alpha")
		w.Header().Add("X-Multi", "beta")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	addr := fmt.Sprintf(":%d", *port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		panic(err)
	}
}
