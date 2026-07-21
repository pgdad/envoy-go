package hcm

import (
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/internal/accesslog"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

type emitCaptureSink struct{ recs []*accesslog.Record }

func (s *emitCaptureSink) Submit(r any) { s.recs = append(s.recs, r.(*accesslog.Record)) }
func (s *emitCaptureSink) Close() error { return nil }

func TestEmitAccessLog_H1_DirectResponseShape(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	req, _ := http.NewRequest("GET", "/health", nil)
	req.Host = "127.0.0.1:10000"
	req.Header.Set("User-Agent", "Go-http-client/1.1")
	req.Proto = "HTTP/1.1"
	start := time.Now().Add(-5 * time.Millisecond)
	f.emitAccessLog(req, 200, 3, cluster.Endpoint{}, start, nil, nil, nil)
	if len(cs.recs) != 1 {
		t.Fatalf("captured %d records, want 1", len(cs.recs))
	}
	r := cs.recs[0]
	if r.Method != "GET" || r.Path != "/health" || r.Protocol != "HTTP/1.1" {
		t.Errorf("Record fields wrong: %+v", r)
	}
	if r.UpstreamHost != "" {
		t.Errorf("UpstreamHost should be empty for direct_response, got %q", r.UpstreamHost)
	}
	if r.ResponseCode != 200 || r.BytesSent != 3 {
		t.Errorf("status/bytes wrong: %d/%d", r.ResponseCode, r.BytesSent)
	}
	if r.Authority != "127.0.0.1:10000" {
		t.Errorf("Authority = %q", r.Authority)
	}
	if r.UserAgent != "Go-http-client/1.1" {
		t.Errorf("UserAgent = %q", r.UserAgent)
	}
}

func TestEmitAccessLog_H1_RoutedShape(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	req, _ := http.NewRequest("GET", "/api/v1/foo", nil)
	req.Proto = "HTTP/1.1"
	picked := cluster.Endpoint{Host: "10.0.0.1", Port: 8080}
	f.emitAccessLog(req, 200, 17, picked, time.Now(), nil, nil, nil)
	if cs.recs[0].UpstreamHost != "10.0.0.1:8080" {
		t.Errorf("UpstreamHost = %q, want 10.0.0.1:8080", cs.recs[0].UpstreamHost)
	}
}

func TestEmitAccessLog_MultipleSinks_AllReceiveRecord(t *testing.T) {
	cs1, cs2 := &emitCaptureSink{}, &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs1, cs2}}
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil)
	if len(cs1.recs) != 1 || len(cs2.recs) != 1 {
		t.Errorf("sink record counts: cs1=%d cs2=%d, want 1/1", len(cs1.recs), len(cs2.recs))
	}
}

func TestEmitAccessLog_H2_PseudoHeadersFromH2Request(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	req := h2.H2Request{Method: "GET", Path: "/api/v1/foo", Authority: "host:1234"}
	f.emitAccessLogH2(req, 200, 17, cluster.Endpoint{Host: "10.0.0.1", Port: 8080}, time.Now(), nil, nil, nil)
	if len(cs.recs) != 1 {
		t.Fatal("expected 1 record")
	}
	if cs.recs[0].Protocol != "HTTP/2.0" {
		t.Errorf("Protocol = %q, want HTTP/2.0", cs.recs[0].Protocol)
	}
	if cs.recs[0].Method != "GET" || cs.recs[0].Path != "/api/v1/foo" {
		t.Errorf("H2 fields wrong: %+v", cs.recs[0])
	}
}

func TestEmitAccessLog_H2_StatusZeroSkipsEmission(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	req := h2.H2Request{}
	f.emitAccessLogH2(req, 0, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil)
	if len(cs.recs) != 0 {
		t.Errorf("expected 0 records on status=0 ctx-cancel, got %d", len(cs.recs))
	}
}

func TestEmitAccessLog_NoSinks_IsNoOp(t *testing.T) {
	f := &Filter{accessLog: nil}
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil)
}

// --- Task 4: emit-hook header capture (H1+H2) -------------------------------

func eqMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestEmitAccessLog_H1_RequestHeaderCapture(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{
		accessLog:         []accesslog.Sink{cs},
		alsReqHeaderNames: []string{"x-req-foo", "x-req-missing", "x-req-multi"},
	}
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	req.Header.Set("X-Req-Foo", "bar")
	req.Header.Add("X-Req-Multi", "m1")
	req.Header.Add("X-Req-Multi", "m2")
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil)
	got := cs.recs[0].RequestHeaders
	want := map[string]string{"x-req-foo": "bar", "x-req-multi": "m1,m2"}
	if !eqMap(got, want) {
		t.Errorf("RequestHeaders = %v, want %v", got, want)
	}
}

func TestEmitAccessLog_H1_ResponseHeaderCapture(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{
		accessLog:          []accesslog.Sink{cs},
		alsRespHeaderNames: []string{"content-type"},
	}
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	resp := filter_http.OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}}
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), resp, nil, nil)
	got := cs.recs[0].ResponseHeaders
	want := map[string]string{"content-type": "text/plain"}
	if !eqMap(got, want) {
		t.Errorf("ResponseHeaders = %v, want %v", got, want)
	}
}

func TestEmitAccessLog_H1_NilResponseHeaders_OmitsAll(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{
		accessLog:          []accesslog.Sink{cs},
		alsRespHeaderNames: []string{"content-type"},
	}
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	// nil response carrier at an error site must not panic and must yield a
	// nil ResponseHeaders map (no entries to capture).
	f.emitAccessLog(req, 404, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil)
	if cs.recs[0].ResponseHeaders != nil {
		t.Errorf("ResponseHeaders = %v, want nil for nil response carrier", cs.recs[0].ResponseHeaders)
	}
}

func TestEmitAccessLog_H1_PresentEmptyValue(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{
		accessLog:         []accesslog.Sink{cs},
		alsReqHeaderNames: []string{"x-empty"},
	}
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	req.Header.Set("X-Empty", "")
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil)
	got := cs.recs[0].RequestHeaders
	if v, ok := got["x-empty"]; !ok || v != "" {
		t.Errorf("RequestHeaders[x-empty] = %q,%v, want present empty", v, ok)
	}
}

func TestEmitAccessLog_H2_RequestHeaderCapture(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{
		accessLog:         []accesslog.Sink{cs},
		alsReqHeaderNames: []string{"x-req-foo", "x-req-missing", "x-req-multi"},
	}
	req := h2.H2Request{
		Method: "GET", Path: "/", Authority: "host",
		Headers: []hpack.HeaderField{
			{Name: "x-req-foo", Value: "bar"},
			{Name: "x-req-multi", Value: "m1"},
			{Name: "x-req-multi", Value: "m2"},
		},
	}
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), nil, nil, nil)
	got := cs.recs[0].RequestHeaders
	want := map[string]string{"x-req-foo": "bar", "x-req-multi": "m1,m2"}
	if !eqMap(got, want) {
		t.Errorf("RequestHeaders = %v, want %v", got, want)
	}
}

func TestEmitAccessLog_H2_ResponseHeaderCapture(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{
		accessLog:          []accesslog.Sink{cs},
		alsRespHeaderNames: []string{"content-type"},
	}
	req := h2.H2Request{Method: "GET", Path: "/", Authority: "host"}
	resp := filter_http.OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}}
	f.emitAccessLogH2(req, 200, 0, cluster.Endpoint{}, time.Now(), resp, nil, nil)
	got := cs.recs[0].ResponseHeaders
	want := map[string]string{"content-type": "text/plain"}
	if !eqMap(got, want) {
		t.Errorf("ResponseHeaders = %v, want %v", got, want)
	}
}

func TestEmitAccessLog_NoCapture_ByteStableNilMaps(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}} // empty union lists
	req, _ := http.NewRequest("GET", "/", nil)
	req.Proto = "HTTP/1.1"
	resp := filter_http.OrderedHeaders{{Name: "Content-Type", Value: "text/plain"}}
	f.emitAccessLog(req, 200, 0, cluster.Endpoint{}, time.Now(), resp, nil, nil)
	if cs.recs[0].RequestHeaders != nil || cs.recs[0].ResponseHeaders != nil {
		t.Errorf("no-capture path must keep nil maps, got req=%v resp=%v",
			cs.recs[0].RequestHeaders, cs.recs[0].ResponseHeaders)
	}
}
