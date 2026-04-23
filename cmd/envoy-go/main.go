// envoy-go is the phase-02 subject binary. It loads an Envoy v3 Bootstrap
// proto from YAML (internal/bootstrap), builds the cluster manager
// (internal/cluster) and the listener manager (internal/listener) which wires
// each listener to its terminal TCP proxy filter (internal/filter/tcpproxy),
// starts the admin /ready server (internal/admin), binds every listener, marks
// admin ready, prints per-listener + terminal ready sentinels, and blocks on
// SIGINT. The phase-00 ad-hoc TCP pump is gone — its byte-level logic now
// lives in internal/filter/tcpproxy/filter.go (ADR-0023).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/esalaine/envoy-go/internal/admin"
	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/listener"
)

func main() {
	cfgPath := flag.String("c", "", "path to envoy-go.yaml (Envoy v3 Bootstrap)")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml>")
		os.Exit(2)
	}
	f, err := os.Open(*cfgPath)
	if err != nil {
		log.Fatalf("open config: %v", err)
	}
	bs, err := bootstrap.Load(f)
	_ = f.Close()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	adminHost, adminPort, err := bootstrap.AdminSocket(bs)
	if err != nil {
		log.Fatalf("extract admin: %v", err)
	}
	adminAddr := fmt.Sprintf("%s:%d", adminHost, adminPort)

	cm, err := cluster.NewManager(bs)
	if err != nil {
		log.Fatalf("cluster manager: %v", err)
	}

	admSrv := admin.New(adminAddr)
	if _, err := admSrv.Start(); err != nil {
		log.Fatalf("admin start %s: %v", adminAddr, err)
	}
	defer func() { _ = admSrv.Close() }()

	lm, err := listener.NewManager(bs, cm)
	if err != nil {
		log.Fatalf("listener manager: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := lm.Start(ctx); err != nil {
		log.Fatalf("listener start: %v", err)
	}
	defer lm.Stop()

	admSrv.MarkReady()

	// Per-listener ready sentinels + terminal sentinel (ADR-0026).
	for _, info := range lm.Listeners() {
		_, _ = fmt.Fprintf(os.Stdout, "envoy-go listener %s ready on %s\n", info.Name, info.Addr)
	}
	_, _ = fmt.Fprintln(os.Stdout, "envoy-go ready")

	<-ctx.Done()
}
