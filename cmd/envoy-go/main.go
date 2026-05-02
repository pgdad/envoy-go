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
	"path/filepath"
	"syscall"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/admin"
	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/listener"
)

func main() {
	cfgPath := flag.String("c", "", "path to envoy-go.yaml (Envoy v3 Bootstrap)")
	allowH2C := flag.Bool("allow-h2c", false,
		"test-only; not for production — permits HCM codec_type=HTTP2 on plaintext listeners for h2spec conformance only")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml> [--allow-h2c]")
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

	adminHost, adminPort, err := bootstrap.AdminSocket(bs.Proto)
	if err != nil {
		log.Fatalf("extract admin: %v", err)
	}
	adminAddr := fmt.Sprintf("%s:%d", adminHost, adminPort)

	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, filepath.Dir(*cfgPath), bs.Stats)
	if err != nil {
		log.Fatalf("cluster manager: %v", err)
	}

	// Phase 06.2 (Task 14): open one AsyncFileSink per access_log[] file entry
	// parsed by bootstrap.Load. A single dropped counter is shared across all
	// sinks per ADR-0069. The defer fires after lm.Stop() (defers are LIFO)
	// so in-flight log records have been flushed before the files are closed.
	droppedCounter := accesslog.RegisterDroppedCounter(bs.Stats)
	sinks := make([]accesslog.Sink, 0, len(bs.AccessLogConfigs))
	for _, cfg := range bs.AccessLogConfigs {
		sink, err := accesslog.NewAsyncFileSink(cfg.Path, droppedCounter)
		if err != nil {
			log.Fatalf("accesslog: open %q: %v", cfg.Path, err)
		}
		sinks = append(sinks, sink)
	}
	defer func() {
		for _, s := range sinks {
			_ = s.Close()
		}
	}()

	admSrv := admin.New(adminAddr, bs.Stats)
	if _, err := admSrv.Start(); err != nil {
		log.Fatalf("admin start %s: %v", adminAddr, err)
	}
	defer func() { _ = admSrv.Close() }()

	// Phase 07.1 Task 15 minimal boot wiring: build the *filter_http.HTTPRegistry
	// and register the router terminal filter so HCM bootstraps that include
	// the router in http_filters[] (i.e., every well-formed HCM config) parse
	// cleanly. Per ADR-0072 the registry is freeze-after-boot. cors and
	// envoygotest filter registrations land at Task 20 (when their factories
	// land at Tasks 18+19); for Task 15 the router-only registry is sufficient
	// to drive the H1 differential gate (fixtures 0003-http11-routing,
	// 0006-access-log) which exercise only the router terminal filter.
	httpReg := filter_http.NewHTTPRegistry()
	httpReg.Register(router.TypeURL, router.New)
	httpReg.Freeze()

	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats, sinks, httpReg)
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

	// LBP-1 (SPEC §5.3 + §5.4): all NewCounter / NewGauge calls have completed
	// — admin server.live (admin.New), cluster 8×N (cluster.NewManager…),
	// listener 2×M (listener.NewManager… + Listener.Start post-bind), HCM 5×K
	// (filter-chain build, eagerly executed inside listener.NewManager…).
	// Post-Freeze any further NewCounter/NewGauge call panics; this is what
	// makes the Walk-under-RLock-plus-atomic-Load read path lock-free against
	// hot-path increments (SPEC §5.2).
	bs.Stats.Freeze()

	// Per-listener ready sentinels + terminal sentinel (ADR-0026).
	for _, info := range lm.Listeners() {
		_, _ = fmt.Fprintf(os.Stdout, "envoy-go listener %s ready on %s\n", info.Name, info.Addr)
	}
	_, _ = fmt.Fprintln(os.Stdout, "envoy-go ready")

	<-ctx.Done()
}
