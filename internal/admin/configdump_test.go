package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	adminv3 "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
)

func TestBuildConfigDump_ThreeSubEnvelopesInOrder(t *testing.T) {
	bs := mustMinimalBs(t)
	cd, err := buildConfigDump(bs, time.Now())
	if err != nil {
		t.Fatalf("buildConfigDump: %v", err)
	}
	if len(cd.Configs) != 3 {
		t.Fatalf("Configs len: got %d, want 3", len(cd.Configs))
	}
	wantTypes := []string{
		"type.googleapis.com/envoy.admin.v3.BootstrapConfigDump",
		"type.googleapis.com/envoy.admin.v3.ListenersConfigDump",
		"type.googleapis.com/envoy.admin.v3.ClustersConfigDump",
	}
	for i, want := range wantTypes {
		if got := cd.Configs[i].GetTypeUrl(); got != want {
			t.Errorf("Configs[%d].@type: got %q, want %q", i, got, want)
		}
	}
}

func TestBuildConfigDump_BootstrapEnvelopeContainsParsedProto(t *testing.T) {
	bs := mustMinimalBs(t)
	cd, _ := buildConfigDump(bs, time.Now())
	bootAny := cd.Configs[0]
	bootDump := &adminv3.BootstrapConfigDump{}
	if err := bootAny.UnmarshalTo(bootDump); err != nil {
		t.Fatalf("UnmarshalTo BootstrapConfigDump: %v", err)
	}
	if bootDump.GetBootstrap() == nil {
		t.Errorf("BootstrapConfigDump.Bootstrap is nil")
	}
	if bootDump.GetLastUpdated() == nil {
		t.Errorf("BootstrapConfigDump.LastUpdated is nil")
	}
}

func TestBuildConfigDump_ListenersEnvelopeContainsOneStaticListener(t *testing.T) {
	bs := mustMinimalBs(t)
	cd, _ := buildConfigDump(bs, time.Now())
	lisDump := &adminv3.ListenersConfigDump{}
	if err := cd.Configs[1].UnmarshalTo(lisDump); err != nil {
		t.Fatalf("UnmarshalTo ListenersConfigDump: %v", err)
	}
	if got := lisDump.GetVersionInfo(); got != "static" {
		t.Errorf("ListenersConfigDump.VersionInfo: got %q, want %q", got, "static")
	}
	if got := len(lisDump.GetStaticListeners()); got != 1 {
		t.Errorf("StaticListeners len: got %d, want 1", got)
	}
}

func TestBuildConfigDump_ClustersEnvelopeContainsOneStaticCluster(t *testing.T) {
	bs := mustMinimalBs(t)
	cd, _ := buildConfigDump(bs, time.Now())
	cluDump := &adminv3.ClustersConfigDump{}
	if err := cd.Configs[2].UnmarshalTo(cluDump); err != nil {
		t.Fatalf("UnmarshalTo ClustersConfigDump: %v", err)
	}
	if got := cluDump.GetVersionInfo(); got != "static" {
		t.Errorf("ClustersConfigDump.VersionInfo: got %q, want %q", got, "static")
	}
	if got := len(cluDump.GetStaticClusters()); got != 1 {
		t.Errorf("StaticClusters len: got %d, want 1", got)
	}
}

func TestHandleConfigDump_HTTPSmoke200JSON(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/config_dump")
	if err != nil {
		t.Fatalf("GET /config_dump: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
	if srv := resp.Header.Get("Server"); srv != "envoy" {
		t.Errorf("Server: got %q, want %q", srv, "envoy")
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("body is empty")
	}
	// Body must be valid JSON
	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err != nil {
		t.Errorf("body is not valid JSON: %v\nbody: %s", err, body)
	}
	if _, ok := generic["configs"]; !ok {
		t.Errorf("body has no 'configs' field; body: %s", body)
	}
}

func TestHandleConfigDump_ProtoJSONUsesSnakeCaseAndOneSpaceIndent(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm, nil)
	addr, _ := s.Start()
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, _ := http.Get("http://" + addr + "/config_dump")
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Snake-case field names per ADR-0086 (UseProtoNames: true)
	if !strings.Contains(bodyStr, `"static_listeners"`) {
		t.Errorf("body lacks snake_case 'static_listeners'; got camelCase? body excerpt: %s", bodyStr[:min(300, len(bodyStr))])
	}
	// Indent: " " (1-space) per ADR-0086. The depth-1 JSON field "configs"
	// appears at exactly 1 space of indent under Indent: " "; under any
	// wider indent (e.g., "  ") it would be N+ spaces. This is the
	// load-bearing distinguisher between the empirical pin and the nearest
	// wrong shape — `\n ` alone is too loose (also matches 2-space, etc).
	if !strings.Contains(bodyStr, "\n \"configs\"") {
		t.Errorf("body lacks 1-space-indent depth-1 marker '\\n \"configs\"'; indent appears wider than 1 space; body excerpt: %s", bodyStr[:min(300, len(bodyStr))])
	}
	// EmitUnpopulated: true per ADR-0086. The bootstrap's `node` field is
	// unset on the §7.3 fixture; under EmitUnpopulated: true it serializes
	// as `"node": null` — under EmitUnpopulated: false it would be elided
	// entirely. protojson deliberately randomizes the post-colon spacing
	// (1 or 2 spaces) per Marshal call to discourage consumers from
	// depending on its output format, so we tolerate either via regex.
	if !regexp.MustCompile(`"node":\s+null`).MatchString(bodyStr) {
		t.Errorf("body lacks EmitUnpopulated marker '\"node\": null'; flag may be false; body excerpt: %s", bodyStr[:min(300, len(bodyStr))])
	}
}
