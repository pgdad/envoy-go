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
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/admin"
	"github.com/pgdad/envoy-go/internal/boot"
	"github.com/pgdad/envoy-go/internal/bootstrap"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	"github.com/pgdad/envoy-go/internal/grpcclient"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/statssink"
	"github.com/pgdad/envoy-go/validate"
)

func main() {
	cfgPath := flag.String("c", "", "path to envoy-go.yaml (Envoy v3 Bootstrap)")
	allowH2C := flag.Bool("allow-h2c", false,
		"test-only; not for production — permits HCM codec_type=HTTP2 on plaintext listeners for h2spec conformance only")
	mode := flag.String("mode", "", `operation mode: empty (default) boots normally; "validate" validates the config named by -c and exits without booting, mirroring upstream Envoy's --mode validate`)
	flag.Parse()
	if *mode != "" && *mode != "validate" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml> [--mode validate] [--allow-h2c]")
		os.Exit(2)
	}
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: envoy-go -c <config.yaml> [--mode validate] [--allow-h2c]")
		os.Exit(2)
	}
	// Phase 51 Task 5 (ADR-0268): --mode validate calls validate.Bootstrap
	// DIRECTLY (not validate.BootstrapFile) so *allowH2C composes — BootstrapFile
	// hardcodes allowH2C=false, which would silently drop -allow-h2c when both
	// flags are passed together.
	if *mode == "validate" {
		f, err := os.Open(*cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		err = validate.Bootstrap(f, filepath.Dir(*cfgPath), *allowH2C)
		_ = f.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("configuration OK")
		os.Exit(0)
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
	// net.JoinHostPort (not fmt.Sprintf) so an IPv6 admin address renders
	// bracketed ("[::1]:9901") and stays bindable; IPv4/hostname output is
	// byte-identical to the previous Sprintf form.
	adminAddr := net.JoinHostPort(adminHost, strconv.FormatUint(uint64(adminPort), 10))

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
	// Declared before boot.Construct so it is in scope for builtins.Deps.HTTPClient.
	// Phase 46.2 (D-TRACE-ZIPKIN-TRANSPORT-WIRING): HOISTED above the tracing
	// ExporterProvider so internal/boot's zipkinTransportAdapter can carry it
	// (+ cm) into NewExporterProvider (via boot.NewTracingProvider, phase 51
	// Task 3) for the Zipkin arm's v2-JSON ClusterDispatch POSTs.
	httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	tracingProvider := boot.NewTracingProvider(dialer, httpClient, cm, bs.Stats)
	// Phase 60.2 Task 5 (ADR-0280): pre-scan bs for a downstream SDS-bound TLS
	// context and, when present, build the blocking xds.SecretProvider — nil
	// when no listener carries tls_certificate_sds_secret_configs (the tls
	// lift only engages then). Built here (not inside boot.Construct) because
	// it needs the shared dialer, mirroring tracingProvider immediately above.
	sdsProvider, err := boot.NewSDSProvider(dialer, bs, filepath.Dir(*cfgPath), bs.Stats)
	if err != nil {
		log.Fatalf("sds provider: %v", err)
	}
	// minNode is the minimal Node (Id/Cluster only) shared by the gRPC ALS
	// access-log sink and the metrics_service stats sink below. The OTLP
	// access-log sink deliberately does NOT use it — its Resource labels need
	// node.locality.zone, so it sources the FULL bootstrap node (D-OTLP-NODE).
	minNode := &corev3.Node{Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster()}
	if len(bs.ALSConfigs) > 0 {
		written, dropped := accesslog.RegisterGrpcSinkCounters(bs.Stats)
		for _, cfg := range bs.ALSConfigs {
			client, err := grpcclient.NewALSClient(dialer, cfg.ClusterName)
			if err != nil {
				log.Fatalf("accesslog: gRPC ALS client for cluster %q: %v", cfg.ClusterName, err)
			}
			sinks = append(sinks, accesslog.NewGrpcAccessLogSink(client, cfg.LogName, minNode, written, dropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval, cfg.AdditionalRequestHeaders, cfg.AdditionalResponseHeaders))
		}
	}
	if len(bs.OTLPConfigs) > 0 {
		otlpWritten, otlpDropped := accesslog.RegisterOTLPSinkCounters(bs.Stats)
		otlpNode := bs.Proto.GetNode()
		for _, cfg := range bs.OTLPConfigs {
			client, err := grpcclient.NewOTLPLogsClient(dialer, cfg.ClusterName)
			if err != nil {
				log.Fatalf("accesslog: OTLP logs client for cluster %q: %v", cfg.ClusterName, err)
			}
			sinks = append(sinks, accesslog.NewOTLPAccessLogSink(client, cfg.LogName, otlpNode, cfg.DisableBuiltinLabels, cfg.Body, cfg.Attributes, cfg.ResourceAttributes, otlpWritten, otlpDropped, int(cfg.BufferSizeBytes), cfg.BufferFlushInterval))
		}
	}
	defer func() {
		for _, s := range sinks {
			_ = s.Close()
		}
	}()

	// Phase 47.1 (ADR-0262) + Phase 48 (ADR-0265) + Phase 49 (ADR-0266)
	// + Phase 57 (ADR-0275): the stats sinks (metrics_service, statsd, dog_statsd,
	// and graphite_statsd) feed the SAME statsSinks slice + Flusher. Built when ANY
	// of the four sink kinds is present, reusing the hoisted dialer for metrics_service.
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
	if len(bs.StatsSinkConfigs) > 0 || len(bs.StatsdSinkConfigs) > 0 || len(bs.DogStatsdSinkConfigs) > 0 || len(bs.GraphiteStatsdSinkConfigs) > 0 {
		if len(bs.StatsSinkConfigs) > 0 {
			for _, cfg := range bs.StatsSinkConfigs {
				client, err := grpcclient.NewMetricsServiceClient(dialer, cfg.ClusterName)
				if err != nil {
					log.Fatalf("statssink: metrics_service client for cluster %q: %v", cfg.ClusterName, err)
				}
				statsSinks = append(statsSinks, statssink.NewMetricsServiceSink(client, minNode, cfg.ReportCountersAsDeltas, cfg.EmitTagsAsLabels))
			}
		}
		// Phase 48 (ADR-0265): the statsd UDP stats sink — synchronous, no
		// goroutine. Phase 55 (ADR-0272): the statsd TCP transport — a bounded-
		// channel + writer-goroutine sink, the statsd sinks' FIRST background
		// mutator. StatsdSinkConfig is a TAGGED UNION over statsd_specifier:
		// exactly one of TCPClusterName / UDPAddress is set (bootstrap.go).
		for _, cfg := range bs.StatsdSinkConfigs {
			if cfg.TCPClusterName != "" {
				name := cfg.TCPClusterName
				// Defensive: bootstrap already rejected an unknown cluster at parse
				// time against static_resources.clusters (the complete set today —
				// no CDS). When CDS lands, THIS becomes the real check.
				if _, ok := cm.Get(name); !ok {
					log.Fatalf("statssink: statsd tcp sink: unknown cluster %q", name)
				}
				// Re-look-up the cluster per dial so the latest cluster-manager
				// state is observed (the grpcclient.go:128-141 idiom). DialSink is
				// the UNACCOUNTED dial: no max_connections permit, no upstream_cx_*
				// (AMEND-TCP-CXSTATS).
				statsSinks = append(statsSinks, statssink.NewTCPStatsdSink(func(ctx context.Context) (net.Conn, error) {
					cl, ok := cm.Get(name)
					if !ok {
						return nil, fmt.Errorf("statssink: statsd tcp: cluster %q vanished", name)
					}
					return cl.DialSink(ctx)
				}, cfg.Prefix))
				continue
			}
			sink, err := statssink.NewStatsdSink(cfg.UDPAddress, cfg.Prefix)
			if err != nil {
				log.Fatalf("statssink: statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
		// Phase 49 (ADR-0266): the dog_statsd UDP stats sink with tags.
		// Phase 50 (ADR-0267): the batching cap is threaded through MaxBytesPerDatagram.
		// NewDogStatsdSink dials a SECOND, independent connected UDP socket; a
		// resolve/dial error is a fatal boot failure (the StatsdSink precedent).
		// Synchronous (no goroutine), so it adds no background mutator to the
		// shutdown drain.
		for _, cfg := range bs.DogStatsdSinkConfigs {
			sink, err := statssink.NewDogStatsdSink(cfg.UDPAddress, cfg.Prefix, cfg.MaxBytesPerDatagram)
			if err != nil {
				log.Fatalf("statssink: dog_statsd sink for %q: %v", cfg.UDPAddress, err)
			}
			statsSinks = append(statsSinks, sink)
		}
		// Phase 57 (ADR-0275): the graphite_statsd UDP stats sink — tags folded
		// into the metric NAME as ;k=v pairs; batching per max_bytes_per_datagram
		// (the shared phase-50 machinery). NewGraphiteStatsdSink dials a THIRD
		// independent connected UDP socket; a resolve/dial error is a fatal boot
		// failure (the statsd/dog_statsd precedent). Synchronous (no goroutine),
		// so it adds no background mutator to the shutdown drain.
		for _, cfg := range bs.GraphiteStatsdSinkConfigs {
			sink, err := statssink.NewGraphiteStatsdSink(cfg.UDPAddress, cfg.Prefix, cfg.MaxBytesPerDatagram)
			if err != nil {
				log.Fatalf("statssink: graphite_statsd sink for %q: %v", cfg.UDPAddress, err)
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

	// Phase 51 Task 3 (ADR-0268): the registry-and-listener-manager tail of
	// the boot sequence (HTTP/listener/network filter registries + the
	// listener manager itself) is built by the shared internal/boot.Construct
	// seam, so main.go's normal boot path and the public validate package
	// (Task 4) can never silently diverge on what "valid" means.
	lm, err := boot.Construct(bs, cm, filepath.Dir(*cfgPath), *allowH2C, sinks, drainMgr, httpClient, tracingProvider, sdsProvider)
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
