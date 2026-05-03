package admin

import (
	"log"
	"net/http"
	"sync/atomic"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/drain"
)

// handleServerInfo implements /server_info per SPEC §5.5 + §11.4 + ADR-0088.
// Body is application/json via the same MarshalOptions as /config_dump
// (reused from configdump.go's configDumpMarshalOptions). Field set:
// version (BuildVersionString), state (LIVE/PRE_INITIALIZING per
// planner-time decision 4), uptime_current_epoch + uptime_all_epochs,
// hot_restart_version: "disabled", partial command_line_options{config_path},
// node (from bootstrap proto). DRAINING state is 08.2's deliverable.
func (s *Server) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	info := buildServerInfo(s)
	body, err := configDumpMarshalOptions.Marshal(info)
	if err != nil {
		log.Printf("admin: /server_info: marshal: %v", err)
		writeAdminHeaders(w, "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("{}"))
		return
	}
	writeAdminHeaders(w, "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// buildServerInfo assembles the *adminv3.ServerInfo proto from the
// Server's threaded fields. Callable from tests.
func buildServerInfo(s *Server) *adminv3.ServerInfo {
	configPath := ""
	var node *corev3.Node
	if s.bs != nil {
		configPath = s.bs.ConfigPath
		if s.bs.Proto != nil {
			node = s.bs.Proto.GetNode()
		}
	}
	uptime := durationpb.New(time.Since(s.bootTime))
	return &adminv3.ServerInfo{
		Version:            BuildVersionString(),
		State:              deriveState(&s.ready, s.dm),
		UptimeCurrentEpoch: uptime,
		UptimeAllEpochs:    uptime,
		HotRestartVersion:  "disabled",
		CommandLineOptions: &adminv3.CommandLineOptions{
			ConfigPath: configPath,
		},
		Node: node,
	}
}

// deriveState returns the ServerInfo state enum value using the precedence
// rule DRAINING > LIVE > PRE_INITIALIZING (ADR-0097 + ADR-0098 + SPEC §11.7).
//
// If dm is non-nil and dm.State() == StateDraining, ServerInfo_DRAINING is
// returned regardless of the ready flag (DRAINING takes highest precedence
// per ADR-0098 + ADR-0097 — the additively-amended extension of ADR-0088).
//
// Otherwise: ServerInfo_LIVE when ready is set, ServerInfo_PRE_INITIALIZING
// when it is not (the ADR-0088 two-state coverage, preserved verbatim).
//
// INITIALIZING is unreachable in the MVP (no xDS init phase that survives
// admin-server bind — ADR-0088 §f).
func deriveState(ready *atomic.Bool, dm *drain.Manager) adminv3.ServerInfo_State {
	// 08.2 (Task 8) DRAINING-first check per SPEC §6.5 + §11.2 + ADR-0098
	// (amends ADR-0088 additively). Precedence DRAINING > LIVE >
	// PRE_INITIALIZING per ADR-0097 + ADR-0098 cross-reference.
	if dm != nil && dm.State() == drain.StateDraining {
		return adminv3.ServerInfo_DRAINING
	}
	if ready.Load() {
		return adminv3.ServerInfo_LIVE
	}
	return adminv3.ServerInfo_PRE_INITIALIZING
}
