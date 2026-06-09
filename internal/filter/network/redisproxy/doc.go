// Package redisproxy implements the envoy.filters.network.redis_proxy network
// filter (ADR-0229) — the project's FIRST terminal routing proxy. UNLIKE the
// prior network-filter rows (echo/sni_cluster/zookeeper_proxy/mongo_proxy/
// kafka_broker, all passive sniffers on a [filter, tcp_proxy] chain), redis_proxy
// TERMINATES the downstream connection: it implements network.TerminalFilter
// (Handle owns the raw net.Conn), parses RESP request frames, answers PING/AUTH
// locally, and round-trips data commands to an upstream cluster member via the
// upstream connection-pool / cluster-routing seam (ADR-0230,
// internal/filter/network/upstreampool.go).
//
// At phase 32.1 it lands the seam + the RESP codec + the round-trip foundation:
// the config parse (stat_prefix + settings.op_timeout + catch_all_route.cluster,
// with unknown clusters tolerated at validate and resolved lazily at Handle), the
// in-house RESP codec (the +/-/:/$/* value types + null sentinels + inline
// commands; the streaming-reader partial-frame model — the terminal owns the conn
// and block-reads a bufio.Reader, NO private high-water buffer), the PING/AUTH
// local-reply set (zero upstream), the 10 fixed downstream cx/rq counters + the 4
// created-not-yet-incremented gauges under redis.<stat_prefix>., and the
// TerminalFilter.Handle command->reply pump (one upstream conn per downstream
// conn; synchronous single-flight FIFO/positional reply correlation).
//
// 32.2 completes the full command set + the per-command/splitter/REDIS_CLUSTER
// stat roster + the gauges' inc/dec + the redis. Prometheus tag-extractor arm +
// the differential command matrix + the 41st fuzzer FuzzRESPDecode + the
// parent-row-32 rollup.
package redisproxy
