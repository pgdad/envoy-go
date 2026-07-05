package statsdrecv_test

import (
	"maps"
	"net"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/test/helpers/statsdrecv"
)

func TestStatsdRecvBasic(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Write 3 datagrams
	for _, msg := range []string{
		"p.cluster.x.rq_total:7|c",
		"p.cluster.x.rq_total:0|c",
		"p.cluster.x.healthy:1|g",
	} {
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatalf("Write %q: %v", msg, err)
		}
	}

	// Poll until SeenCount("p.cluster.x.rq_total") == 2 (≤2s)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount("p.cluster.x.rq_total") == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Assert DeltaSum
	sum, ok := srv.DeltaSum("p.cluster.x.rq_total")
	if !ok {
		t.Error("DeltaSum: expected ok=true for p.cluster.x.rq_total")
	}
	if sum != 7 {
		t.Errorf("DeltaSum: got %v, want 7", sum)
	}

	// Assert Gauge
	gval, gok := srv.Gauge("p.cluster.x.healthy")
	if !gok {
		t.Error("Gauge: expected ok=true for p.cluster.x.healthy")
	}
	if gval != 1 {
		t.Errorf("Gauge: got %v, want 1", gval)
	}

	// Assert SeenCount
	if c := srv.SeenCount("p.cluster.x.rq_total"); c != 2 {
		t.Errorf("SeenCount: got %v, want 2", c)
	}

	// Assert absent name => ok=false
	_, absent := srv.DeltaSum("no.such.metric")
	if absent {
		t.Error("DeltaSum: expected ok=false for absent name")
	}
	_, gabsent := srv.Gauge("no.such.metric")
	if gabsent {
		t.Error("Gauge: expected ok=false for absent name")
	}

	// Assert Reset clears all
	srv.Reset()
	_, afterReset := srv.DeltaSum("p.cluster.x.rq_total")
	if afterReset {
		t.Error("After Reset: DeltaSum expected ok=false")
	}
	_, gAfterReset := srv.Gauge("p.cluster.x.healthy")
	if gAfterReset {
		t.Error("After Reset: Gauge expected ok=false")
	}
	if c := srv.SeenCount("p.cluster.x.rq_total"); c != 0 {
		t.Errorf("After Reset: SeenCount expected 0, got %v", c)
	}

	// regression (phase 49 Task 6): a tagless line never populates a tag set.
	// (Server was Reset above, so re-seed a tagless datagram first.)
	if _, err := conn.Write([]byte("p.cluster.x.rq_total:7|c")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount("p.cluster.x.rq_total") == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if tags, ok := srv.Tags("p.cluster.x.rq_total"); ok || tags != nil {
		t.Errorf("Tags(tagless name): got (%v, %v), want (nil, false)", tags, ok)
	}
}

// TestStatsdRecvTaggedSingleTag covers phase 49 Task 6: a DogStatsd line with a
// single |#key:val tag suffix must be parsed via the first-pipe-then-colon split
// (the old last-colon split would mis-take the tag's colon for the name/value
// boundary).
func TestStatsdRecvTaggedSingleTag(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	name := "dsdpfx.cluster.upstream_rq_total"
	for _, msg := range []string{
		name + ":6|c|#envoy.cluster_name:backend",
		name + ":1|c|#envoy.cluster_name:backend",
	} {
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatalf("Write %q: %v", msg, err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount(name) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if c := srv.SeenCount(name); c != 2 {
		t.Fatalf("SeenCount: got %v, want 2", c)
	}
	sum, ok := srv.DeltaSum(name)
	if !ok {
		t.Fatal("DeltaSum: expected ok=true")
	}
	if sum != 7 {
		t.Errorf("DeltaSum: got %v, want 7", sum)
	}

	wantTags := map[string]string{"envoy.cluster_name": "backend"}
	gotTags, tagsOK := srv.Tags(name)
	if !tagsOK {
		t.Fatal("Tags: expected ok=true")
	}
	if !maps.Equal(gotTags, wantTags) {
		t.Errorf("Tags: got %v, want %v", gotTags, wantTags)
	}
}

// TestStatsdRecvTaggedTwoTags covers a comma-joined multi-tag suffix, asserted
// via order-independent map equality.
func TestStatsdRecvTaggedTwoTags(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	name := "dsdpfx.http.downstream_rq_xx"
	msg := name + ":5|c|#envoy.response_code_class:2,envoy.http_conn_manager_prefix:hcm_local"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount(name) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	wantTags := map[string]string{
		"envoy.response_code_class":      "2",
		"envoy.http_conn_manager_prefix": "hcm_local",
	}
	gotTags, ok := srv.Tags(name)
	if !ok {
		t.Fatal("Tags: expected ok=true")
	}
	if !maps.Equal(gotTags, wantTags) {
		t.Errorf("Tags: got %v, want %v", gotTags, wantTags)
	}
}

// TestStatsdRecvTaggedGauge covers a tagged |g gauge line.
func TestStatsdRecvTaggedGauge(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	name := "dsdpfx.cluster.membership_total"
	msg := name + ":1|g|#envoy.cluster_name:backend"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount(name) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	gval, gok := srv.Gauge(name)
	if !gok {
		t.Fatal("Gauge: expected ok=true")
	}
	if gval != 1 {
		t.Errorf("Gauge: got %v, want 1", gval)
	}

	wantTags := map[string]string{"envoy.cluster_name": "backend"}
	gotTags, ok := srv.Tags(name)
	if !ok {
		t.Fatal("Tags: expected ok=true")
	}
	if !maps.Equal(gotTags, wantTags) {
		t.Errorf("Tags: got %v, want %v", gotTags, wantTags)
	}
}

// TestStatsdRecvTagsAbsentName covers Tags on a name never seen at all.
func TestStatsdRecvTagsAbsentName(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()

	if tags, ok := srv.Tags("no.such.metric"); ok || tags != nil {
		t.Errorf("Tags(absent name): got (%v, %v), want (nil, false)", tags, ok)
	}
}

// TestStatsdRecvResetClearsTags covers phase 49 Task 6: Reset() clears tags too.
func TestStatsdRecvResetClearsTags(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	name := "dsdpfx.cluster.upstream_rq_total"
	msg := name + ":6|c|#envoy.cluster_name:backend"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount(name) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := srv.Tags(name); !ok {
		t.Fatal("Tags: expected ok=true before Reset")
	}

	srv.Reset()

	if tags, ok := srv.Tags(name); ok || tags != nil {
		t.Errorf("Tags after Reset: got (%v, %v), want (nil, false)", tags, ok)
	}
}

// TestStatsdRecvMalformedTagSegment covers a malformed tag segment (no colon
// inside): must not panic, and the counter's DeltaSum/SeenCount still update
// normally.
func TestStatsdRecvMalformedTagSegment(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	name := "dsdpfx.cluster.malformed_tag"
	msg := name + ":1|c|#malformed"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount(name) == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if c := srv.SeenCount(name); c != 1 {
		t.Fatalf("SeenCount: got %v, want 1 (malformed tag segment must not be fatal)", c)
	}
	sum, ok := srv.DeltaSum(name)
	if !ok {
		t.Fatal("DeltaSum: expected ok=true")
	}
	if sum != 1 {
		t.Errorf("DeltaSum: got %v, want 1", sum)
	}
	// Either an empty map or (nil, false) is acceptable per the plan.
	if tags, tagsOK := srv.Tags(name); tagsOK && len(tags) != 0 {
		t.Errorf("Tags(malformed): got %v, want empty map if ok=true", tags)
	}
}

// TestMaxLinesInAnyDatagram covers phase 50 Task 5: MaxLinesInAnyDatagram and
// LinesInDatagram accessors for batching verification.
func TestMaxLinesInAnyDatagram(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Test 1: MaxLinesInAnyDatagram starts at 0 on a fresh Server
	if m := srv.MaxLinesInAnyDatagram(); m != 0 {
		t.Errorf("MaxLinesInAnyDatagram on fresh Server: got %v, want 0", m)
	}

	// Test 2: A single-line datagram leaves MaxLinesInAnyDatagram() == 1
	if _, err := conn.Write([]byte("p.a:1|c")); err != nil {
		t.Fatalf("Write single-line datagram: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount("p.a") == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if m := srv.MaxLinesInAnyDatagram(); m != 1 {
		t.Errorf("MaxLinesInAnyDatagram after single-line datagram: got %v, want 1", m)
	}

	// Test 3: A multi-line datagram updates MaxLinesInAnyDatagram() == 2
	if _, err := conn.Write([]byte("p.a:1|c\np.b:2|c")); err != nil {
		t.Fatalf("Write multi-line datagram: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount("p.b") == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if m := srv.MaxLinesInAnyDatagram(); m != 2 {
		t.Errorf("MaxLinesInAnyDatagram after multi-line datagram: got %v, want 2", m)
	}

	// Test 3b: MaxLinesInAnyDatagram does NOT regress if a subsequent single-line datagram arrives
	if _, err := conn.Write([]byte("p.c:3|c")); err != nil {
		t.Fatalf("Write single-line datagram: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount("p.c") == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if m := srv.MaxLinesInAnyDatagram(); m != 2 {
		t.Errorf("MaxLinesInAnyDatagram after single-line (should not regress): got %v, want 2", m)
	}

	// Test 4: LinesInDatagram(name) reflects the datagram that most recently carried name
	lines, ok := srv.LinesInDatagram("p.a")
	if !ok || lines != 2 {
		t.Errorf("LinesInDatagram(p.a): got (%v, %v), want (2, true)", lines, ok)
	}
	lines, ok = srv.LinesInDatagram("p.b")
	if !ok || lines != 2 {
		t.Errorf("LinesInDatagram(p.b): got (%v, %v), want (2, true)", lines, ok)
	}
	lines, ok = srv.LinesInDatagram("p.c")
	if !ok || lines != 1 {
		t.Errorf("LinesInDatagram(p.c): got (%v, %v), want (1, true)", lines, ok)
	}

	// Test 4b: LinesInDatagram(name) tracks LAST-SEEN not MAX — send p.a alone in a new
	// datagram and assert it drops back to 1, proving a buggy max-per-name would fail here.
	if _, err := conn.Write([]byte("p.a:1|c")); err != nil {
		t.Fatalf("Write single-line datagram with p.a: %v", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.SeenCount("p.a") == 3 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	lines, ok = srv.LinesInDatagram("p.a")
	if !ok || lines != 1 {
		t.Errorf("LinesInDatagram(p.a) after decrease: got (%v, %v), want (1, true) — must track last-seen not max", lines, ok)
	}
	// MaxLinesInAnyDatagram should still be 2 (it is a running max, not affected by the decrease)
	if m := srv.MaxLinesInAnyDatagram(); m != 2 {
		t.Errorf("MaxLinesInAnyDatagram after decrease: got %v, want 2 (running max unaffected)", m)
	}

	// Test 5: An absent name returns (0, false)
	lines, ok = srv.LinesInDatagram("no.such.metric")
	if ok || lines != 0 {
		t.Errorf("LinesInDatagram(absent name): got (%v, %v), want (0, false)", lines, ok)
	}

	// Test 6: Reset() clears both MaxLinesInAnyDatagram and LinesInDatagram
	srv.Reset()
	if m := srv.MaxLinesInAnyDatagram(); m != 0 {
		t.Errorf("MaxLinesInAnyDatagram after Reset: got %v, want 0", m)
	}
	for _, name := range []string{"p.a", "p.b", "p.c"} {
		lines, ok := srv.LinesInDatagram(name)
		if ok || lines != 0 {
			t.Errorf("LinesInDatagram(%s) after Reset: got (%v, %v), want (0, false)", name, lines, ok)
		}
	}
}
