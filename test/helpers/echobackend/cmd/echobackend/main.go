// Command echobackend is a cmdline wrapper around the shared
// test/helpers/echobackend package. The differential runner spawns it as a
// subprocess on a pre-allocated port via go run; the binary serves the JSON
// echo handler described by the package doc.
//
// Introduced by phase 14 Task 10 for fixture 0016 scenario 6's per-route
// remove_accept_encoding_header assertion.
package main

import (
	"flag"
	"log"

	"github.com/pgdad/envoy-go/test/helpers/echobackend"
)

func main() {
	port := flag.Int("port", 0, "TCP port to bind")
	flag.Parse()
	if *port == 0 {
		log.Fatal("--port is required")
	}
	listener, err := echobackend.Listen(*port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := echobackend.New()
	log.Printf("echobackend listening on %d", *port)
	if err := srv.Serve(listener); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
