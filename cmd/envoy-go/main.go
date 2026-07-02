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
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/admin"
	"github.com/esalaine/envoy-go/internal/bootstrap"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/adaptive_concurrency"
	"github.com/esalaine/envoy-go/internal/filter/http/admission_control"
	"github.com/esalaine/envoy-go/internal/filter/http/bandwidthlimit"
	"github.com/esalaine/envoy-go/internal/filter/http/buffer"
	"github.com/esalaine/envoy-go/internal/filter/http/compressor"
	"github.com/esalaine/envoy-go/internal/filter/http/cors"
	"github.com/esalaine/envoy-go/internal/filter/http/csrf"
	"github.com/esalaine/envoy-go/internal/filter/http/envoygotest"
	"github.com/esalaine/envoy-go/internal/filter/http/extauthz"
	"github.com/esalaine/envoy-go/internal/filter/http/extproc"
	"github.com/esalaine/envoy-go/internal/filter/http/fault"
	"github.com/esalaine/envoy-go/internal/filter/http/header_mutation"
	"github.com/esalaine/envoy-go/internal/filter/http/jwtauthn"
	"github.com/esalaine/envoy-go/internal/filter/http/localratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/lua"
	"github.com/esalaine/envoy-go/internal/filter/http/oauth2"
	"github.com/esalaine/envoy-go/internal/filter/http/ratelimit"
	"github.com/esalaine/envoy-go/internal/filter/http/rbac"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/filter/http/wasm"
	network "github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/builtins"
	"github.com/esalaine/envoy-go/internal/grpcclient"
	"github.com/esalaine/envoy-go/internal/httpclient"
	"github.com/esalaine/envoy-go/internal/listener"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector"
	"github.com/esalaine/envoy-go/internal/statssink"
	"github.com/esalaine/envoy-go/internal/tracing"
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
	// Phase 44.1 (ADR-0255) + phase 45.1 (ADR-0258) + phase 46.1b (ADR-0260):
	// the shared grpcclient.Dialer is built UNCONDITIONALLY (it is cheap and
	// lazy-dialing — no connection is opened until a named cluster is first
	// requested). The access-log sinks (ALS, OTLP-log) are still built only when
	// their config slices are non-empty, reusing this hoisted dialer. The tracing
	// ExporterProvider is also built unconditionally (it is INERT until ExporterFor
	// is called during HCM filter-parse for a tracing-enabled listener; the
	// lazy-sync.Once counter register guarantees a no-tracing boot has zero
	// tracing.opentelemetry.* stat surface). Buffer defaults (16384 bytes /
	// 1 s flush) mirror the ALS + OTLP-log defaults from bootstrap parsing
	// (alsDefaultBufferSizeBytes=16384, bufferFlushInterval default=1s).
	dialer := grpcclient.New(cm)
	// Phase-20 Task 2b (ADR-0177 + ADR-0150 §Decision AMENDMENT): construct
	// the shared *httpclient.Client singleton AT BOOT for cross-phase reuse
	// by jwks Fetcher (this Task 2b) + extauthz httpAuthClient (Task 2c) +
	// oauth2 token_endpoint POST (Task 10). Per planner-time D8 the
	// singleton uses Options{Timeout: 30s} — matches the phase-17-pinned
	// per-request timeout discipline preserved by ADR-0150 §Decision
	// AMENDMENT + the phase-18.1 zero-retry posture preserved by ADR-0159
	// §Decision AMENDMENT. Threaded into the listener manager + via
	// hcm.ListenerCtx{HTTPClient: ...} into per-filter FactoryCtx.HTTPClient.
	// Declared before netReg so it is in scope for builtins.Deps.HTTPClient.
	// Phase 46.2 (D-TRACE-ZIPKIN-TRANSPORT-WIRING): HOISTED above the tracing
	// ExporterProvider so the zipkinTransportAdapter can carry it (+ cm) into
	// NewExporterProvider for the Zipkin arm's v2-JSON ClusterDispatch POSTs.
	httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	tracingProvider := tracing.NewExporterProvider(tracesDialerAdapter{dialer}, zipkinTransportAdapter{httpClient, cm}, bs.Stats, 16384, time.Second)
	if len(bs.ALSConfigs) > 0 || len(bs.OTLPConfigs) > 0 {
		if len(bs.ALSConfigs) > 0 {
			written, dropped := accesslog.RegisterGrpcSinkCounters(bs.Stats)
			node := &corev3.Node{Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster()}
			for _, cfg := range bs.ALSConfigs {
				client, err := grpcclient.NewALSClient(dialer, cfg.ClusterName)
				if err != nil {
					log.Fatalf("accesslog: gRPC ALS client for cluster %q: %v", cfg.ClusterName, err)
				}
				sinks = append(sinks, accesslog.NewGrpcAccessLogSink(client, cfg.LogName, node, written, dropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval, cfg.AdditionalRequestHeaders, cfg.AdditionalResponseHeaders))
			}
		}
		if len(bs.OTLPConfigs) > 0 {
			otlpWritten, otlpDropped := accesslog.RegisterOTLPSinkCounters(bs.Stats)
			// The OTLP Resource labels need node.locality.zone, so source the FULL
			// bootstrap node (Id/Cluster/Locality) — NOT the ALS minimal node (D-OTLP-NODE).
			otlpNode := bs.Proto.GetNode()
			for _, cfg := range bs.OTLPConfigs {
				client, err := grpcclient.NewOTLPLogsClient(dialer, cfg.ClusterName)
				if err != nil {
					log.Fatalf("accesslog: OTLP logs client for cluster %q: %v", cfg.ClusterName, err)
				}
				sinks = append(sinks, accesslog.NewOTLPAccessLogSink(client, cfg.LogName, otlpNode, cfg.DisableBuiltinLabels, cfg.Body, cfg.Attributes, cfg.ResourceAttributes, otlpWritten, otlpDropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval))
			}
		}
	}
	defer func() {
		for _, s := range sinks {
			_ = s.Close()
		}
	}()

	// Phase 47.1 (ADR-0262) + Phase 48 (ADR-0265) + Phase 49 (ADR-0266): the stats
	// sinks (metrics_service, statsd, and dog_statsd) feed the SAME statsSinks
	// slice + Flusher. Built when ANY of the three sink kinds is present, reusing
	// the hoisted dialer for metrics_service.
	// The *statssink.MetricsServiceSink does NOT satisfy accesslog.Sink
	// (Submit(batch []*dto.MetricFamily) vs Submit(r any)), so the sinks are
	// collected in their OWN statsSinks slice + closed via a dedicated defer.
	// The Flusher is BUILT here (pre-Freeze) but Start()ed only AFTER
	// bs.Stats.Freeze() so the Walk snapshot is over the frozen registry
	// (D-MS-FLUSH-INERT: when both config slices are empty, statsFlusher stays nil
	// and NO flush goroutine starts — byte-stability).
	var statsFlusher *statssink.Flusher
	var statsSinks []statssink.Sink
	flusherDone := make(chan struct{})
	if len(bs.StatsSinkConfigs) > 0 || len(bs.StatsdSinkConfigs) > 0 || len(bs.DogStatsdSinkConfigs) > 0 {
		if len(bs.StatsSinkConfigs) > 0 {
			node := &corev3.Node{Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster()}
			for _, cfg := range bs.StatsSinkConfigs {
				client, err := grpcclient.NewMetricsServiceClient(dialer, cfg.ClusterName)
				if err != nil {
					log.Fatalf("statssink: metrics_service client for cluster %q: %v", cfg.ClusterName, err)
				}
				statsSinks = append(statsSinks, statssink.NewMetricsServiceSink(client, node, cfg.ReportCountersAsDeltas, cfg.EmitTagsAsLabels))
			}
		}
		// Phase 48 (ADR-0265): the statsd UDP stats sink. NewStatsdSink dials a
		// connected UDP socket; a resolve/dial error is a fatal boot failure (the
		// metrics_service-client precedent). Synchronous (no goroutine), so it adds
		// no background mutator to the shutdown drain.
		for _, cfg := range bs.StatsdSinkConfigs {
			sink, err := statssink.NewStatsdSink(cfg.UDPAddress, cfg.Prefix)
			if err != nil {
				log.Fatalf("statssink: statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
		// Phase 49 (ADR-0266): the dog_statsd UDP stats sink with tags.
		// NewDogStatsdSink dials a SECOND, independent connected UDP socket; a
		// resolve/dial error is a fatal boot failure (the StatsdSink precedent).
		// Synchronous (no goroutine), so it adds no background mutator to the
		// shutdown drain.
		for _, cfg := range bs.DogStatsdSinkConfigs {
			sink, err := statssink.NewDogStatsdSink(cfg.UDPAddress, cfg.Prefix)
			if err != nil {
				log.Fatalf("statssink: dog_statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
		statsFlusher = statssink.NewFlusher(bs.Stats, bs.FlushInterval, statsSinks)
	}
	// LIFO: this runs in the shutdown drain AFTER the server ctx is canceled (the
	// cancel() defer is registered later, so it runs first). We WAIT on flusherDone
	// before closing the sink channels: cancel() fires ctx.Done(), this defer blocks
	// on <-flusherDone, the Flusher's Start loop finishes any in-progress flushOnce
	// (its last Submit goes to the still-OPEN channel) then returns and closes
	// flusherDone, and only THEN do we Close() the sinks (close their channels). This
	// enforces the sink contract (no Submit after Close) and prevents a flush-tick /
	// Close race from sending on a closed channel. Each Close() drains the in-flight
	// stream (CloseAndRecv) + closes the gRPC conn.
	defer func() {
		<-flusherDone // wait for the Flusher goroutine to stop Submitting before closing the sink channels
		for _, s := range statsSinks {
			_ = s.Close()
		}
	}()
	// Phase 46.1b (ADR-0260): flush-and-stop the OTLP trace exporter goroutines on
	// shutdown. In LIFO order this runs AFTER lm.Stop() (so listeners are stopped
	// and no new spans are generated) but BEFORE the access-log sinks close. The
	// provider is INERT for no-tracing boots (byCluster stays empty, CloseAll is
	// a trivial no-op returning nil).
	defer func() { _ = tracingProvider.CloseAll() }()

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
	httpReg.Register(adaptive_concurrency.TypeURL, adaptive_concurrency.New)
	httpReg.Register(admission_control.TypeURL, admission_control.New)
	httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)
	httpReg.Register(buffer.TypeURL, buffer.New)
	httpReg.Register(compressor.TypeURL, compressor.New)
	httpReg.Register(cors.TypeURL, cors.New)
	httpReg.Register(csrf.TypeURL, csrf.New)
	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
	httpReg.Register(extauthz.TypeURL, extauthz.New)
	httpReg.Register(extproc.TypeURL, extproc.New)
	httpReg.Register(fault.TypeURL, fault.New)
	httpReg.Register(header_mutation.TypeURL, header_mutation.New)
	httpReg.Register(jwtauthn.TypeURL, jwtauthn.New)
	httpReg.Register(localratelimit.TypeURL, localratelimit.New)
	httpReg.Register(lua.TypeURL, lua.New)
	httpReg.Register(oauth2.TypeURL, oauth2.New)
	httpReg.Register(ratelimit.TypeURL, ratelimit.New) // phase-24.1 Task 7 (ADR-0197 core); 18 → 19 HTTP filters
	httpReg.Register(rbac.TypeURL, rbac.New)
	httpReg.Register(wasm.TypeURL, wasm.New) // phase-25.1 Task 13 (ADR-0202/0203/0204); 19 → 20 HTTP filters
	// Register header_mutation per-route validator before Freeze (the registry
	// rejects registrations after Freeze; New is called post-Freeze during
	// listener construction, so it cannot call RegisterPerRouteValidator itself).
	header_mutation.RegisterPerRouteValidator(httpReg)
	// Phase-20 Task 11: register the oauth2 per-route validator BEFORE Freeze
	// per SPEC §5.2 + planner-time D2 (HCM-parse-time PARSE-REJECT for any
	// TPFC placement at route or virtualHost level). The v1.37.x oauth2 proto
	// has NO OAuth2PerRoute message at all per §20.P7 RATIFIED — the validator
	// rejects UNCONDITIONALLY with the byte-stable D2 wording.
	oauth2.RegisterPerRouteValidator(httpReg)
	// Phase-22.1 Task 10: register the lua per-route validator BEFORE Freeze
	// per parent §6.2 arm 18 + 22.1 PLAN D-P6 + ADR-0110 single-chokepoint.
	// The validator one-liner returns "lua: per-route configuration is not
	// yet supported (lands in phase 22.3)"; the 9th canonical per-route shape
	// validator replaces the body at 22.3 IMPL.
	lua.RegisterPerRouteValidator(httpReg)
	// Phase-24.2 Task 3: register the ratelimit per-route validator BEFORE Freeze
	// per parent §5.3 + ADR-0199 (the 10th canonical per-route shape) + ADR-0110
	// single-chokepoint. The validator enforces the embedded rate_limits[]
	// §5.2 PARSE-REJECT arms (REUSES ValidateRouteRateLimits from 24.1 Task 3 —
	// disable_key / extension / dynamic_metadata + per-policy stage > 10) and
	// the vh_rate_limits enum-bounds check; override_option + domain are
	// PARSE-ACCEPTED (override_option is INERT per AMEND-4; empty domain
	// defers to the filter-config domain).
	ratelimit.RegisterPerRouteValidator(httpReg)
	// Phase-25.1 Task 13: register the wasm per-route validator BEFORE Freeze
	// per parent §6.2 arm 18 + AMEND-A3 REUSE-by-absence (5th-canonical
	// PARSE-REJECT-by-presence) + ADR-0110 single-chokepoint. The validator
	// rejects UNCONDITIONALLY at 25.1+25.2 with the byte-stable wording
	// "wasm: per-route configuration is not yet supported (lands in phase 25.3)";
	// the 5th canonical per-route shape (`WasmPerRoute` wholesale-override)
	// replaces the body at 25.3 IMPL.
	wasm.RegisterPerRouteValidator(httpReg)
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

	// Phase-26.2 (§3.4): register all four built-in network filters (echo +
	// direct_response read filters; tcp_proxy + HCM terminal filters) via the
	// shared seam, capturing the boot singletons in the terminal adapters. Freeze
	// BEFORE the listener manager is constructed (the per-listener parser resolves
	// filter_chains[].filters[].type_urls against the frozen registry).
	netReg := network.NewRegistry()
	builtins.RegisterBuiltins(netReg, builtins.Deps{
		ClusterManager:   cm,
		StatsRegistry:    bs.Stats,
		AccessLogSinks:   sinks,
		HTTPRegistry:     httpReg,
		DrainManager:     drainMgr,
		HTTPClient:       httpClient,
		TracingExporters: tracingProvider,
	})
	netReg.Freeze()

	lm, err := listener.NewManagerWithBaseDirAndAllowH2C(bs.Proto, cm, filepath.Dir(*cfgPath), *allowH2C, bs.Stats, sinks, httpReg, lfReg, drainMgr, httpClient, netReg)
	if err != nil {
		log.Fatalf("listener manager: %v", maybeWrapLuaScriptLoadError(err))
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

	cm.StartHealthChecks(ctx)     // active health checks (phase 39.1); checkers stop on shutdown ctx + cm.Drain()
	cm.StartOutlierDetection(ctx) // passive outlier-detection sweeps (phase 40.3); stop on shutdown ctx + cm.Drain()

	// Phase 47.1 (ADR-0262): start the metrics_service flush loop on the server
	// lifetime ctx, AFTER Freeze so each Walk snapshot reads the frozen registry
	// (ADR-0059). nil when no stats_sinks[] entry is configured (no goroutine).
	if statsFlusher != nil {
		// close(flusherDone) when Start returns so the shutdown sink-close defer
		// (which waits on <-flusherDone) closes the sink channels only AFTER the
		// flush loop has stopped Submitting (no send-on-closed-channel race).
		go func() { defer close(flusherDone); statsFlusher.Start(ctx) }() // ticker loop; stops on ctx.Done() at shutdown
	} else {
		close(flusherDone) // no flusher: unblock the sink-close defer immediately
	}

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

// luaCompileErrorSubstring is the byte-stable arm-16 wrap prefix emitted by
// internal/filter/http/lua/compiled_config.go::wrapParseRejectScriptCompileFailed
// (`"lua: default_source_code: compile: %w"`). Detecting this substring lets
// the boot-reject sink at main.go identify a Lua script-compile failure that
// surfaced through the HCM filter-factory error chain
// (`listener: %q: filter_chains[%d]: hcm: http_filters[%d]: factory: <inner>`).
// Phase 22.1 Task 15 + parent §13-W + 22.1 SPEC §6 Task 15.
const luaCompileErrorSubstring = "lua: default_source_code: compile:"

// scriptLoadErrorWrapPrefix is the literal wording prefix the upstream
// Envoy v1.37.2 lua filter prints to stderr on script-compile failure per
// `source/extensions/filters/common/lua/lua.cc` (parent §11.7.5). The
// envoy-go-side boot-reject path wraps the surfaced error with this prefix
// so the cross-side substring assertion at fixture-0026 scenario (g) (per
// AMEND-10 option 2 + parent §13-R1) lands on both proxies.
//
// The trailing colon + space match upstream's wording byte-exactly; the
// substring assertion in test/differential/runner_test.go::runBootRejectFixture
// only checks for `"script load error"` (no colon) per AMEND-10 wording
// discipline — the colon is preserved here for upstream-parity readability
// of operator-facing stderr.
const scriptLoadErrorWrapPrefix = "script load error: "

// tracesDialerAdapter bridges *grpcclient.Dialer to the unexported
// tracing.tracesClientDialer interface (single method NewTracesClient). It
// allows main.go to pass the shared grpcclient.Dialer into
// tracing.NewExporterProvider without creating an import cycle (tracing
// never imports grpcclient; the structural match is verified by the compiler
// at the NewExporterProvider call site). Phase 46.1b (ADR-0260).
type tracesDialerAdapter struct{ d *grpcclient.Dialer }

func (a tracesDialerAdapter) NewTracesClient(clusterName string) (tracing.TracesClient, error) {
	return grpcclient.NewOTLPTracesClient(a.d, clusterName)
}

// zipkinTransportAdapter bridges the shared *httpclient.Client + *cluster.Manager
// to the tracing.ZipkinTransport seam (HasCluster/Dispatch). It lets main.go pass
// the HTTP cluster-dispatch transport into tracing.NewExporterProvider without
// internal/tracing importing internal/httpclient or internal/cluster (no import
// cycle): HasCluster gates the boot-time collector-cluster existence check and
// Dispatch binds the cluster manager into (*httpclient.Client).ClusterDispatch
// for the Zipkin arm's v2-JSON POSTs. Phase 46.2 (D-TRACE-ZIPKIN-TRANSPORT-WIRING).
type zipkinTransportAdapter struct {
	c  *httpclient.Client
	cm *cluster.Manager
}

func (a zipkinTransportAdapter) HasCluster(name string) bool { _, ok := a.cm.Get(name); return ok }

func (a zipkinTransportAdapter) Dispatch(ctx context.Context, clusterName string, req *http.Request) (*http.Response, error) {
	return a.c.ClusterDispatch(ctx, clusterName, req, a.cm)
}

var _ tracing.ZipkinTransport = zipkinTransportAdapter{}

// maybeWrapLuaScriptLoadError inspects the supplied error for the arm-16
// Lua compile-failure substring (the byte-stable wrap emitted by the lua
// filter's `buildCompiledConfig` per `compiled_config.go::wrapParseReject
// ScriptCompileFailed`). When matched, returns a new error wrapping the
// original with the upstream-parity prefix `"script load error: "` per
// parent §11.7.5 + §13-W + 22.1 SPEC §6 Task 15.
//
// When the substring is NOT present (i.e., the listener-manager error is
// from a NON-lua filter / a non-compile lua failure / a NON-filter source
// such as bind / cluster construction), the original error is returned
// unchanged — the wrap is filter-scoped, not generic.
//
// The match is a `strings.Contains` (case-sensitive) against the error's
// `Error()` string rather than `errors.As` against a typed sentinel
// because the arm-16 wrap is a string-format chain — the wrapped inner is
// a `*lua.ApiError` value but the prefix lives in the wrap layer, not in
// a typed wrapper. The substring is the contract per parent §6.1
// byte-stable PARSE-REJECT wording discipline.
func maybeWrapLuaScriptLoadError(err error) error {
	if err == nil {
		return nil
	}
	if !strings.Contains(err.Error(), luaCompileErrorSubstring) {
		return err
	}
	return fmt.Errorf("%s%w", scriptLoadErrorWrapPrefix, err)
}
