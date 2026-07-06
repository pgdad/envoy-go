package admin

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/pgdad/envoy-go/internal/cluster"
)

// handleClusters implements /clusters per SPEC §5.3 + §11.2 + ADR-0087.
// Body is text/plain; charset=UTF-8. For each cluster (alphabetical-by-name,
// per cluster.Manager.Clusters() ordering), emits 10 cluster-level lines +
// 18 per-endpoint lines (in bootstrap-declared order). Per planner-time
// decision 8 + ADR-0063 per-endpoint-stats deferral, all 8 per-endpoint
// cx_*/rq_* counter lines emit literal `0` (envoy-go has no per-endpoint
// stats; cluster-level counters are surfaced via /stats/prometheus and would
// not partition meaningfully across endpoints). Cluster-level constants
// (1024, 3, false) and per-endpoint constants (healthy, 1, 0, empty, false,
// -1) are emitted unconditionally per §11.2(b)+(c).
//
// nil-cm path: when admin.New is called with cm=nil (test convention per
// ADR-0085's nil-tolerated constructor), the handler emits an empty body
// and 200 OK rather than panicking. Production call sites always thread a
// non-nil cluster.Manager (see cmd/envoy-go/main.go, Task 10).
func (s *Server) handleClusters(w http.ResponseWriter, _ *http.Request) {
	var buf bytes.Buffer
	if s.cm != nil {
		for _, c := range s.cm.Clusters() {
			writeClusterBlock(&buf, c)
		}
	}
	writeAdminHeaders(w, "text/plain; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// writeClusterBlock emits the 10 cluster-level lines + 18 per-endpoint
// lines per endpoint for one cluster, in the exact order pinned by SPEC
// §11.2. Cluster-level lines have a `<cluster_name>::` prefix; per-endpoint
// lines have a `<cluster_name>::<address>:<port>::` prefix. Pulled out for
// unit-test readability and to localize the 28-line constant block.
func writeClusterBlock(buf *bytes.Buffer, c cluster.ClusterInfo) {
	writeClusterLevelLines(buf, c.Name)
	for _, ep := range c.Endpoints {
		addr := fmt.Sprintf("%s:%d", ep.Address, ep.Port)
		writeEndpointLines(buf, c.Name, addr)
	}
}

// writeClusterLevelLines emits the 10 cluster-level lines per SPEC §11.2(b).
// The constants `1024` (max_connections, max_pending_requests, max_requests)
// and `3` (max_retries) are envoy.config.cluster.v3.CircuitBreakers.Thresholds
// proto defaults (Envoy emits them unconditionally per §11.2 verbatim
// scrape; envoy-go has no circuit-breaker machinery but emits the same
// constants for byte-shape parity per ADR-0087).
func writeClusterLevelLines(buf *bytes.Buffer, name string) {
	fmt.Fprintf(buf, "%s::observability_name::%s\n", name, name)
	fmt.Fprintf(buf, "%s::default_priority::max_connections::1024\n", name)
	fmt.Fprintf(buf, "%s::default_priority::max_pending_requests::1024\n", name)
	fmt.Fprintf(buf, "%s::default_priority::max_requests::1024\n", name)
	fmt.Fprintf(buf, "%s::default_priority::max_retries::3\n", name)
	fmt.Fprintf(buf, "%s::high_priority::max_connections::1024\n", name)
	fmt.Fprintf(buf, "%s::high_priority::max_pending_requests::1024\n", name)
	fmt.Fprintf(buf, "%s::high_priority::max_requests::1024\n", name)
	fmt.Fprintf(buf, "%s::high_priority::max_retries::3\n", name)
	fmt.Fprintf(buf, "%s::added_via_api::false\n", name)
}

// writeEndpointLines emits the 18 per-endpoint lines per SPEC §11.2(c).
// First 8 lines: cx_*/rq_* counters, all `0` per planner-time decision 8
// (ADR-0063 per-endpoint stats deferral). Last 10 lines: constants
// (hostname empty; health_flags::healthy; weight::1; region/zone/sub_zone
// empty; canary::false; priority::0; success_rate::-1;
// local_origin_success_rate::-1) — Envoy's default-when-not-configured
// values, emitted by envoy-go for byte-shape parity per ADR-0087.
func writeEndpointLines(buf *bytes.Buffer, clusterName, addr string) {
	for _, key := range []string{
		"cx_active",
		"cx_connect_fail",
		"cx_total",
		"rq_active",
		"rq_error",
		"rq_success",
		"rq_timeout",
		"rq_total",
	} {
		fmt.Fprintf(buf, "%s::%s::%s::0\n", clusterName, addr, key)
	}
	fmt.Fprintf(buf, "%s::%s::hostname::\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::health_flags::healthy\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::weight::1\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::region::\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::zone::\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::sub_zone::\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::canary::false\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::priority::0\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::success_rate::-1\n", clusterName, addr)
	fmt.Fprintf(buf, "%s::%s::local_origin_success_rate::-1\n", clusterName, addr)
}
