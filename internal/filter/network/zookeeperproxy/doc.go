// Package zookeeperproxy implements envoy.filters.network.zookeeper_proxy —
// a passive both-direction ZooKeeper-protocol observability sniffer (ADR-0222).
//
// Phase 28.1 lands the REQUEST side: the 9-field config parse, the 201-counter
// EAGER roster under <stat_prefix>.zookeeper. (creation parity — response-side
// counters exist at zero), the shallow request decoder (framing + xid sniffing
// + opcode dispatch + min-length validation + decoder-internal reassembly),
// the two xid correlation structures (written here, consumed at 28.2), and the
// dynamic per-scheme auth.<scheme>_rq counters. The filter implements BOTH
// network.ReadFilter and network.WriteFilter (one instance, both directions —
// the first consumer of the ADR-0221 WriteFilter seam); its 28.1 OnWrite is a
// PURE no-op Continue stub. Phase 28.2 lands the response decoder + xid
// correlation consumption + latency-threshold counters (ADR-0223).
package zookeeperproxy
