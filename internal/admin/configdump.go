package admin

import (
	"log"
	"net/http"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/pgdad/envoy-go/internal/bootstrap"
)

// configDumpMarshalOptions is the protojson MarshalOptions tuple pinned by
// SPEC §11.1 empirical-pin findings: 1-space indent (NOT 2 or 4), snake_case
// field names (NOT camelCase), zero-valued fields emitted (the
// EmitUnpopulated: true setting is what gives Envoy's body its
// "show me everything" character — the differential equivalence comparator
// relies on both sides emitting the same set of fields). Settled by
// ADR-0086 (planner consolidates the four-value tuple).
//
// The same MarshalOptions is reused by /server_info (Task 9) for cross-endpoint
// JSON-body shape consistency.
var configDumpMarshalOptions = protojson.MarshalOptions{
	Multiline:       true,
	Indent:          " ",
	UseProtoNames:   true,
	EmitUnpopulated: true,
}

// handleConfigDump implements the /config_dump contract per SPEC §5.2 +
// §11.1 + ADR-0086. Body is application/json; envelope is
// *adminv3.ConfigDump with three sub-envelopes (Bootstrap, Listeners,
// Clusters) packed as *anypb.Any in this exact order. No dynamic_* arrays
// (no xDS); no RoutesConfigDump / SecretsConfigDump / ScopedRoutesConfigDump
// / EndpointsConfigDump (deferred per ADR-0089). The handler is stateless
// and walks immutable-post-boot state at request time.
//
// Errors from protojson.Marshal are recovered + logged + synthesized as
// 500 Internal Server Error with empty body {} per SPEC §5.2 (defensive;
// Envoy's empirical behavior on /config_dump failure is undocumented but
// 500-with-empty-body is the conservative shape).
func (s *Server) handleConfigDump(w http.ResponseWriter, r *http.Request) {
	cd, err := buildConfigDump(s.bs, s.bootTime)
	if err != nil {
		log.Printf("admin: /config_dump: build: %v", err)
		writeAdminHeaders(w, "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{}"))
		return
	}
	body, err := configDumpMarshalOptions.Marshal(cd)
	if err != nil {
		log.Printf("admin: /config_dump: marshal: %v", err)
		writeAdminHeaders(w, "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{}"))
		return
	}
	writeAdminHeaders(w, "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// buildConfigDump assembles the *adminv3.ConfigDump envelope from the
// bootstrap proto and the boot timestamp. Walks the bootstrap proto
// directly per planner-time decision 7; does not consult cluster/listener
// managers' runtime state. Returns an error from anypb.New (which can fail
// if the inner proto's type isn't registered with protoregistry.GlobalTypes
// — but the ConfigDump sub-envelope types are all from go-control-plane
// admin/v3 and are registered transitively).
func buildConfigDump(bs *bootstrap.Bootstrap, bootTime time.Time) (*adminv3.ConfigDump, error) {
	if bs == nil || bs.Proto == nil {
		// Defensive: in tests with bs == nil, return an empty ConfigDump
		// so the response is a valid (if empty) JSON body.
		return &adminv3.ConfigDump{}, nil
	}
	bootDump := &adminv3.BootstrapConfigDump{
		Bootstrap:   bs.Proto,
		LastUpdated: timestamppb.New(bootTime),
	}
	lisDump := &adminv3.ListenersConfigDump{
		VersionInfo:     "static",
		StaticListeners: enumerateStaticListeners(bs.Proto, bootTime),
	}
	cluDump := &adminv3.ClustersConfigDump{
		VersionInfo:    "static",
		StaticClusters: enumerateStaticClusters(bs.Proto, bootTime),
	}
	bootAny, err := anypb.New(bootDump)
	if err != nil {
		return nil, err
	}
	lisAny, err := anypb.New(lisDump)
	if err != nil {
		return nil, err
	}
	cluAny, err := anypb.New(cluDump)
	if err != nil {
		return nil, err
	}
	return &adminv3.ConfigDump{Configs: []*anypb.Any{bootAny, lisAny, cluAny}}, nil
}

// enumerateStaticListeners walks bs.GetStaticResources().GetListeners() and
// packs each one into a *adminv3.ListenersConfigDump_StaticListener. Per
// planner-time decision 7, this walks the bootstrap proto directly (NOT
// through the listener manager's runtime state).
func enumerateStaticListeners(bs *bootstrapv3.Bootstrap, bootTime time.Time) []*adminv3.ListenersConfigDump_StaticListener {
	ls := bs.GetStaticResources().GetListeners()
	out := make([]*adminv3.ListenersConfigDump_StaticListener, 0, len(ls))
	ts := timestamppb.New(bootTime)
	for _, l := range ls {
		any, err := anypb.New(l)
		if err != nil {
			log.Printf("admin: /config_dump: anypb.New listener %q: %v", l.GetName(), err)
			continue
		}
		out = append(out, &adminv3.ListenersConfigDump_StaticListener{
			Listener:    any,
			LastUpdated: ts,
		})
	}
	return out
}

// enumerateStaticClusters mirrors enumerateStaticListeners for clusters.
func enumerateStaticClusters(bs *bootstrapv3.Bootstrap, bootTime time.Time) []*adminv3.ClustersConfigDump_StaticCluster {
	cs := bs.GetStaticResources().GetClusters()
	out := make([]*adminv3.ClustersConfigDump_StaticCluster, 0, len(cs))
	ts := timestamppb.New(bootTime)
	for _, c := range cs {
		any, err := anypb.New(c)
		if err != nil {
			log.Printf("admin: /config_dump: anypb.New cluster %q: %v", c.GetName(), err)
			continue
		}
		out = append(out, &adminv3.ClustersConfigDump_StaticCluster{
			Cluster:     any,
			LastUpdated: ts,
		})
	}
	return out
}
