// Package admin serves the envoy-go admin API on HTTP/1.1. The server is
// allocated by Server.Start() (per phase 01 contract; reused unchanged in
// 06.1 and 08.1) and registers six endpoints on a single *http.ServeMux:
//
//   - GET /ready              — phase 01: pre-init/ready/draining state
//     (LIVE/PRE_INITIALIZING; DRAINING is 08.2 future work).
//   - GET /stats/prometheus   — phase 06.1: Prometheus text-format
//     exposition built from the in-tree internal/stats Registry threaded
//     in by main.go (per ADR-0059); the same task wires the
//     server.live gauge that the /ready handler Set(1)s under sync.Once
//     on the first 200/LIVE response.
//   - GET /config_dump        — phase 08.1 (per ADR-0086):
//     application/json via protojson over *adminv3.ConfigDump with three
//     sub-envelopes in order — BootstrapConfigDump, ListenersConfigDump,
//     ClustersConfigDump.
//   - GET /clusters           — phase 08.1 (per ADR-0087):
//     text/plain; charset=UTF-8; 10 cluster-level lines + 18 per-endpoint
//     lines per cluster, alphabetical-by-cluster-name.
//   - GET /listeners          — phase 08.1 (per ADR-0087):
//     text/plain; charset=UTF-8; one line per listener `<name>::<addr>`,
//     alphabetical-by-listener-name.
//   - GET /server_info        — phase 08.1 (per ADR-0088):
//     application/json via protojson over *adminv3.ServerInfo; covers
//     LIVE / PRE_INITIALIZING enum values (DRAINING is 08.2; INITIALIZING
//     is unreachable in the static-bootstrap-only model).
//
// See docs/envoy-go/BEHAVIOR_CONTRACT.md `## Admin API` for the per-
// endpoint equivalence claims and the umbrella framing/header/method
// posture. Phase 08.2 will register POST /drain_listeners on the same
// mux and extend /ready + /server_info for the DRAINING state.
package admin
