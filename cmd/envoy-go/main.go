// envoy-go is the phase-00 subject binary. It is intentionally minimal: parse a
// minimal YAML config (ADR-0007), bind a TCP listener, and bidirectionally
// io.Copy bytes between each accepted connection and a single fixed upstream.
// Phase 02 retires this binary and replaces it with a real listener manager +
// TCP proxy filter + cluster manager.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
)

func main() {
	cfgPath := flag.String("c", "", "path to envoy-go.yaml")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml>")
		os.Exit(2)
	}
	f, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatalf("open config: %v", err)
	}
	cfg, err := loadConfig(f)
	_ = f.Close()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	listenAddr := fmt.Sprintf("%s:%d", cfg.Listener.Address, cfg.Listener.Port)
	upstreamAddr := fmt.Sprintf("%s:%d", cfg.Upstream.Address, cfg.Upstream.Port)

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", listenAddr, err)
	}
	defer func() { _ = ln.Close() }()

	// Ready sentinel: harness consumes this line from stdout to know the
	// listener is bound. Format is part of the harness contract; do not
	// change without updating test/differential/harness.go.
	_, _ = fmt.Fprintf(os.Stdout, "envoy-go ready on %s\n", listenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go pump(conn, upstreamAddr)
	}
}

// netConn wraps net.Conn and hides the *net.TCPConn type, preventing
// io.Copy from using the Linux splice(2) syscall optimisation. splice can
// return 0 bytes when the source socket has data+FIN already queued,
// causing silent data loss on loopback. Using a plain Read/Write loop via a
// 32 KiB heap buffer is fast enough for the phase-00 test workload.
type netConn struct{ net.Conn }

func pump(client net.Conn, upstreamAddr string) {
	defer func() { _ = client.Close() }()
	upstream, err := net.Dial("tcp", upstreamAddr)
	if err != nil {
		log.Printf("dial upstream %s: %v", upstreamAddr, err)
		return
	}
	defer func() { _ = upstream.Close() }()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{upstream}, netConn{client}); halfClose(upstream) }()
	go func() { defer wg.Done(); _, _ = io.Copy(netConn{client}, netConn{upstream}); halfClose(client) }()
	wg.Wait()
}

func halfClose(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}
