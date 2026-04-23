// Package cluster materializes one cluster per static_resources.clusters[]
// entry of an Envoy v3 Bootstrap proto, exposes them by name, and gives each
// cluster a round-robin load balancer over its endpoints. Phase 02 supports
// only STATIC clusters with ROUND_ROBIN policy; see SPEC §5.4.
package cluster
