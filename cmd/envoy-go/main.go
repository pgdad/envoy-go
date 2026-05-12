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
	"time"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/admin"
	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit"
	"github.com/esalaine/envoy-go/internal/filter/http/buffer"
	"github.com/esalaine/envoy-go/internal/filter/http/compressor"
	"github.com/esalaine/envoy-go/internal/filter/http/cors"
	"github.com/esalaine/envoy-go/internal/filter/http/csrf"
	"github.com/esalaine/envoy-go/internal/filter/http/envoygotest"
	"github.com/esalaine/envoy-go/internal/filter/http/fault"
	"github.com/esalaine/envoy-go/internal/filter/http/header_mutation"
	"github.com/esalaine/envoy-go/internal/filter/http/localratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/rbac"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/listener"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector"
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
	// Phase 08.1 (Task 2 + planner-time decision 9): record the config-file
	// path on the bootstrap so /server_info can emit it under
	// command_line_options.config_path. The field is plumbed-from-flag rather
	// than parsed-from-bootstrap because Envoy v3 Bootstrap has no
	// config_path field — it's a CLI argument the operator passed.
	bs.ConfigPath = *cfgPath

	// Phase 08.2 (Task 11) drain manager allocation per SPEC §5.1 boot-order
	// + planner-time decision 7: after bootstrap.Load (no dependencies on the
	// bootstrap proto) and before cluster.NewManagerWithBaseDir (the drain
	// manager is consumed by all subsequent constructors). The 30s timeout is
	// the hardcoded envoy-go MVP default per ADR-0095 (Envoy v1.37.2 default
	// is 600s per §11.7 + 08.1 SPEC §11.4 — deliberate divergence to keep test-
	// suite cost tractable; operator-knob deferred per ADR-0095).
	drainMgr := drain.New(30 * time.Second)

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

	// Phase 07.1 Task 20 boot wiring: build the *filter_http.HTTPRegistry and
	// register the three filter factories envoy-go ships at 07.1 — router
	// (terminal; ADR-0071 supersedes ADR-0040 routerAction), cors (real
	// Envoy filter; ADR-0074), envoygotest (test-only probe; ADR-0074). Per
	// ADR-0072 the registry is freeze-after-boot: Freeze MUST be invoked
	// after all Register calls and before the first listener is constructed
	// (the chain build inside listener.NewManagerWithBaseDirAndAllowH2C
	// resolves typed_config TypeURLs against the frozen registry). Task 15
	// landed the minimal router-only variant; Task 20 is the full boot
	// wiring per PLAN.
	httpReg := filter_http.NewHTTPRegistry()
	httpReg.Register(router.TypeURL, router.New)
	httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)
	httpReg.Register(buffer.TypeURL, buffer.New)
	httpReg.Register(compressor.TypeURL, compressor.New)
	httpReg.Register(cors.TypeURL, cors.New)
	httpReg.Register(csrf.TypeURL, csrf.New)
	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
	httpReg.Register(fault.TypeURL, fault.New)
	httpReg.Register(header_mutation.TypeURL, header_mutation.New)
	httpReg.Register(localratelimit.TypeURL, localratelimit.New)
	httpReg.Register(rbac.TypeURL, rbac.New)
	// Register header_mutation per-route validator before Freeze (the registry
	// rejects registrations after Freeze; New is called post-Freeze during
	// listener construction, so it cannot call RegisterPerRouteValidator itself).
	header_mutation.RegisterPerRouteValidator(httpReg)
	httpReg.Freeze()

	// Phase 07.2 Task 11 boot wiring: build the
	// *listenerfilter.ListenerFilterRegistry and register the one listener
	// filter envoy-go ships at 07.2 — tls_inspector (extracts SNI / ALPN /
	// transport_protocol from the ClientHello so the unified pre-handshake
	// dispatch path can do 8-dimension chain selection per ADR-0079 +
	// ADR-0081). Per ADR-0072 / ADR-0079 the registry is freeze-after-boot:
	// Freeze MUST be invoked after all Register calls and before the listener
	// manager is constructed (the per-listener parser inside
	// NewManagerWithBaseDirAndAllowH2C resolves listener_filters[] type_urls
	// against the frozen registry). Task 10's accept-loop refactor deleted the
	// crypto/tls.GetConfigForClient SNI shortcut, so a bootstrap with
	// SNI-indexed filter chains now requires explicit
	// `listener_filters: [tls_inspector]` to extract SNI.
	lfReg := listenerfilter.NewListenerFilterRegistry()
	lfReg.Register(tls_inspector.TypeURL, tls_inspector.New)
	lfReg.Freeze()

	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats, sinks, httpReg, lfReg, drainMgr)
	if err != nil {
		log.Fatalf("listener manager: %v", err)
	}

	// Phase 08.1: admin.New is constructed AFTER cm + lm so the four new
	// read-only operator-introspection endpoints (/config_dump, /clusters,
	// /listeners, /server_info per ADR-0085) can read live cluster +
	// listener state. The constructor must be called before bs.Stats.Freeze()
	// because admin allocates the server.live gauge at New time (SPEC §5.4 +
	// §12 #3). Defers are LIFO; the order here is:
	//   1. defer sinks-close (above) — flushes access logs last
	//   2. defer admSrv.Close() — closes admin first, before sinks
	//   3. defer lm.Stop()        — shuts listeners after admin
	// 08.1 SPEC does not mandate a strict ordering across these resources;
	// the move from pre-lm to post-lm is the LBP-1 cost (cluster + listener
	// must exist before admin can introspect them).
	admSrv := admin.New(adminAddr, bs.Stats, bs, cm, lm, drainMgr)
	if _, err := admSrv.Start(); err != nil {
		log.Fatalf("admin start %s: %v", adminAddr, err)
	}
	defer func() { _ = admSrv.Close() }()

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
	log.Print("signal received; initiating graceful drain")
	// Phase 08.2 (Task 11) drain rendezvous per SPEC §5.2 + §6.8 + ADR-0092:
	// drain-then-exit on SIGTERM/SIGINT (deliberate divergence from Envoy
	// v1.37.2's SIGTERM=immediate-exit per §11.7 — operator-ergonomic choice).
	// Bound by drainMgr.Timeout() (30s default per ADR-0095).
	drainMgr.Drain()
	select {
	case <-drainMgr.Done():
		log.Print("drain rendezvous: in-flight reached 0")
	case <-time.After(drainMgr.Timeout()):
		log.Print("drain rendezvous: timeout fired (best-effort)")
	}
	// Per planner-time decision 9: explicit cm.Drain() call after rendezvous,
	// before deferred-stop chain runs (LIFO: lm.Stop, admSrv.Close, sinks-close).
	// Best-effort upstream-pool close per ADR-0096.
	cm.Drain()
	// Existing deferred-stop chain runs as the function unwinds.
}
