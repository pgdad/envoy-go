package mongoproxy

import (
	"fmt"
	"time"

	mongo_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/stats"
)

const (
	// mongoMetadataNamespace is the dynamic-metadata namespace (parent §11.9 /
	// AMEND-B11). mongoMetadataKey is the single-Set wrapper key under which the
	// whole collection→ops StructValue is written (D-S29.2-3 — a unit-test-asserted
	// internal detail; differential-invisible, no cross-side surface).
	mongoMetadataNamespace = "envoy.filters.network.mongo_proxy"
	mongoMetadataKey       = "operations"
)

// NewFactory returns the mongoproxy NetworkFilterFactory with the stats Registry
// closure-captured (the zookeeperproxy.go:26 precedent — network.FactoryCtx
// carries no stats registry). The factory parses + validates ONCE at boot
// (ADR-0079) and EAGERLY creates the 23-stat roster per distinct stat_prefix at
// parse (D-P1). The returned FilterInstanceFactory allocates a fresh *filter per
// connection, all sharing the boot-parsed *compiledConfig (incl. the roster).
func NewFactory(reg *stats.Registry) network.NetworkFilterFactory {
	return func(tc *anypb.Any, _ network.FactoryCtx) (network.FilterInstanceFactory, error) {
		var msg mongo_proxyv3.MongoProxy
		if tc != nil && len(tc.GetValue()) > 0 {
			if err := tc.UnmarshalTo(&msg); err != nil {
				return nil, fmt.Errorf("mongo_proxy: invalid typed_config: %w", err)
			}
		}
		cfg, err := parseConfig(&msg)
		if err != nil {
			return nil, err
		}
		cfg.stats = newMongoStats(reg, cfg.statPrefix)
		// 29.3 access log (AMEND-B10): construct the sink ONCE per configured path
		// (freeze-after-boot; the HCM/main.go precedent) and SHARE it across the
		// listener's per-connection filter instances (AsyncFileSink.Submit is a
		// goroutine-safe non-blocking channel send). nil when access_log is unset →
		// the per-connection emit is a no-op. The dropped counter rides the shared
		// server.accesslog_dropped stat (ADR-0069). Boot errors fail-fast (ADR-0072).
		var alSink *accesslog.AsyncFileSink
		if cfg.accessLog != "" {
			s, err := accesslog.NewAsyncFileSinkWithFormatter(cfg.accessLog, accessLogDroppedCounter(reg), accesslog.MongoFormat)
			if err != nil {
				return nil, fmt.Errorf("mongo_proxy: access_log %q: %w", cfg.accessLog, err)
			}
			alSink = s
		}
		return func() network.NetworkFilter {
			return &filter{cfg: cfg, dec: newDecoder(cfg, cfg.stats), alSink: alSink}
		}, nil
	}
}

// accessLogDroppedCounter returns the shared server.accesslog_dropped counter
// (ADR-0069). It uses NewCounterIfAbsent (NOT accesslog.RegisterDroppedCounter,
// which calls NewCounter and PANICS on a duplicate): main.go registers this same
// counter at boot for the HCM access-log sinks, so the mongo factory must reuse
// the existing counter idempotently. This adds NO new stat — the 360-stat surface
// is unchanged (the name already exists in the boot roster).
func accessLogDroppedCounter(reg *stats.Registry) *stats.Counter {
	return reg.NewCounterIfAbsent("server.accesslog_dropped")
}

// filter is the per-connection mongo_proxy filter. It implements BOTH
// network.ReadFilter and network.WriteFilter (one instance, both directions —
// consumer #2 of the ADR-0221 seam; the zookeeperproxy both-directions shape).
type filter struct {
	network.Marker
	cfg        *compiledConfig // shared, boot-parsed (incl. the roster)
	dec        *decoder        // per-connection (private readBuf + sniffing + chainConsumed + active-query list)
	cb         network.ReadFilterCallbacks
	delayTimer *time.Timer              // 29.3 fault-delay async-resume timer (SPEC §3.2); OnDestroy cancels.
	alSink     *accesslog.AsyncFileSink // 29.3 access log; nil when access_log is unset (no-op emit). Shared across connections.
}

// emitAccessLog submits one MongoRecord per pending log line for THIS pass (the
// decoder accumulates per-message strings during decode; the filter drains them).
// A no-op when no sink is configured or no message decoded this pass. Timing-
// bearing → differential-INVISIBLE (AMEND-B10). The upstream host: at envoy-go's
// L4 seam the mongo filter sees NO upstream address (tcp_proxy owns the upstream
// dial), so upstream_host is recorded as the available downstream remote address
// (or "-" when unavailable). Per the differential-invisibility this affects no
// gate (no fixture). Connection() may be nil (a pre-injection / test edge) — guarded.
func (f *filter) emitAccessLog(lines []string) {
	if f.alSink == nil || len(lines) == 0 {
		return
	}
	host := "-"
	if f.cb != nil {
		if conn := f.cb.Connection(); conn != nil {
			if ra := conn.RemoteAddr(); ra != nil {
				host = ra.String()
			}
		}
	}
	now := time.Now()
	for _, msgStr := range lines {
		f.alSink.Submit(&accesslog.MongoRecord{Time: now, Message: msgStr, UpstreamHost: host})
	}
}

// MayHalt declares the filter haltable (the chain's async halt/resume seam, 29.3)
// iff a fault delay is configured. A no-delay mongo filter is non-haltable → the
// chain takes the byte-identical pre-29.3 path (R1).
func (f *filter) MayHalt() bool { return f.cfg.delayConfigured }

// OnNewConnection is a no-op Continue: an OnNewConnection StopIteration would set
// the chain's sticky connHalted flag and block all OnData
// (reference_network_read_filter_onnewconnection_halts).
func (f *filter) OnNewConnection() network.Status { return network.Continue }

// OnData feeds the decoder the chain-buffer's NEW bytes (the chainConsumed
// high-water mark against TotalAppended — D-S29.1-4) and ALWAYS returns Continue.
// It NEVER drains the chain buffer, never closes, never halts (R3; at 29.1
// mongoproxy has no halt path — fault delay is 29.3).
func (f *filter) OnData(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnData(buf.Bytes(), buf.TotalAppended())
	f.emitDynamicMetadata()
	f.emitAccessLog(f.dec.takeLogLines()) // 29.3 access log; drained BEFORE the delay-arm return so messages decoded this pass are logged.
	if dur, ok := f.dec.takePendingDelay(); ok {
		return f.armDelay(dur)
	}
	return network.Continue
}

// armDelay increments delays_injected AT ARM (upstream stats_.delays_injected_.inc()),
// sets the cross-goroutine re-entrancy guard, schedules the resume timer, and returns
// StopIteration so the chain HOLDS (the haltable seam — runData/replayRead block the
// dispatcher until onDelayTimer's ContinueReading; SPEC §3.2 / §3.1).
func (f *filter) armDelay(dur time.Duration) network.Status {
	f.cfg.stats.inc("delays_injected")
	f.dec.delayPending.Store(true)
	f.delayTimer = time.AfterFunc(dur, f.onDelayTimer)
	return network.StopIteration
}

// onDelayTimer runs on the time.AfterFunc goroutine: clear the re-entrancy guard,
// then resume the chain (the §3.1 async-active ContinueReading — the load-bearing
// cross-goroutine resume). cb is set once at construction; safe to read here.
func (f *filter) onDelayTimer() {
	f.dec.delayPending.Store(false)
	if f.cb != nil {
		f.cb.ContinueReading()
	}
}

// emitDynamicMetadata writes THIS request pass's collection→ops map to the
// per-connection dynamic-metadata Bucket as ONE StructValue (the §3.7 single-Set
// model — the next emitting pass overwrites it, giving per-pass clear for free).
// Gated by emit_dynamic_metadata; a no-op when the flag is off or the pass
// produced no insert/query (SPEC §3.7 "skip the Set if empty"). The Bucket is
// nil-receiver tolerant (ADR-0085).
func (f *filter) emitDynamicMetadata() {
	if !f.cfg.emitDynamicMetadata || len(f.dec.dynMeta) == 0 {
		return
	}
	fields := make(map[string]*structpb.Value, len(f.dec.dynMeta))
	for coll, ops := range f.dec.dynMeta {
		vals := make([]*structpb.Value, len(ops))
		for i, op := range ops {
			vals[i] = structpb.NewStringValue(op)
		}
		fields[coll] = structpb.NewListValue(&structpb.ListValue{Values: vals})
	}
	sv := structpb.NewStructValue(&structpb.Struct{Fields: fields})
	f.cb.DynamicMetadata().Set(mongoMetadataNamespace, mongoMetadataKey, sv)
}

// OnWrite feeds the response decoder the write-direction (upstream→downstream)
// bytes and ALWAYS returns Continue (R3 extended to the write side; upstream
// onWrite parity — never halts). Replaces the 29.1 no-op stub.
func (f *filter) OnWrite(buf *network.Buffer, _ bool) network.Status {
	f.dec.decodeOnWrite(buf.Bytes())
	f.emitAccessLog(f.dec.takeWriteLogLines()) // 29.3 access log, response direction.
	if f.dec.takeReplyEmptied() && f.cb != nil && f.cb.Draining() {
		// cx_drain_close (SPEC §3.4): the active-query list emptied on a correlated
		// reply while draining → increment + close FlushWrite (the reply is flushed
		// first; the deferred close subsumes upstream's zero-ms timer — D-S29.3-7).
		// Increment + close OUTSIDE the decoder lock (ADR-0223).
		f.cfg.stats.inc("cx_drain_close")
		f.cb.Connection().Close(network.FlushWrite)
	}
	return network.Continue
}

// SetReadFilterCallbacks stores the read callbacks (used by the delay resume,
// dynamic metadata, and the drain-close). SetWriteFilterCallbacks satisfies the
// WriteFilter interface (the both-directions dual injection — chain.go injects
// each exactly once); the write callbacks are never used, so they are
// deliberately not retained (dead state otherwise; the zookeeperproxy shape).
func (f *filter) SetReadFilterCallbacks(cb network.ReadFilterCallbacks) { f.cb = cb }
func (f *filter) SetWriteFilterCallbacks(network.WriteFilterCallbacks)  {}

// OnDestroy drains any residual active-query entries (Dec the gauge per entry so
// it returns to 0 at connection end — §3.4) then drops the per-connection
// decoder. Called exactly once per filter instance (the 28.1a dedupe); it runs
// strictly after both pumps join (the ADR-0221 happens-after edge), so the
// onDestroy lock is uncontended.
func (f *filter) OnDestroy() {
	if f.delayTimer != nil {
		f.delayTimer.Stop() // best-effort; the timer always fires on a blocked pump, so this
		// races only a torn-down-mid-delay edge — no double-count (delays_injected Inc'd at arm).
	}
	if f.dec != nil {
		n := f.dec.onDestroy() // drains residual + Decs the gauge; returns the residual count
		if n > 0 && f.cb != nil {
			// D-P4 CLOSED (SPEC §3.5): a non-empty active-query list at close keys the
			// direction-specific counter. The count is snapshotted under mu inside
			// onDestroy; the increment is OUTSIDE the lock (ADR-0223).
			switch f.cb.CloseDirection() {
			case network.CloseDirectionLocal:
				f.cfg.stats.inc("cx_destroy_local_with_active_rq")
			case network.CloseDirectionRemote:
				f.cfg.stats.inc("cx_destroy_remote_with_active_rq")
			}
		}
	}
	f.dec = nil
}
