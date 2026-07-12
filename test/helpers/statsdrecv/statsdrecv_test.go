package statsdrecv_test

import (
	"maps"
	"net"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/test/helpers/statsdrecv"
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

// dialAndWrite opens a TCP conn to srv and writes b in one Write.
func dialAndWrite(t *testing.T, srv *statsdrecv.Server, b []byte) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := c.Write(b); err != nil {
		t.Fatalf("write: %v", err)
	}
	return c
}

// waitSeen polls until SeenCount(name) >= want, or fails after 2s.
func waitSeen(t *testing.T, srv *statsdrecv.Server, name string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for srv.SeenCount(name) < want {
		if time.Now().After(deadline) {
			t.Fatalf("timed out: SeenCount(%q) = %d, want >= %d", name, srv.SeenCount(name), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTCPBasicCounterAndGauge(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	c := dialAndWrite(t, srv, []byte("a.b:3|c\na.b:4|c\ng.h:9|g\n"))
	defer func() { _ = c.Close() }()

	waitSeen(t, srv, "a.b", 2)
	waitSeen(t, srv, "g.h", 1)

	if sum, ok := srv.DeltaSum("a.b"); !ok || sum != 7 {
		t.Errorf("DeltaSum(a.b) = %v,%v; want 7,true", sum, ok)
	}
	if v, ok := srv.Gauge("g.h"); !ok || v != 9 {
		t.Errorf("Gauge(g.h) = %v,%v; want 9,true", v, ok)
	}
	if n := srv.UnparsedCount(); n != 0 {
		t.Errorf("UnparsedCount = %d, want 0", n)
	}
}

// TestTCPConnCount: one long-lived conn ⇒ ConnCount()==1; a second dial ⇒ 2.
func TestTCPConnCount(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	c1 := dialAndWrite(t, srv, []byte("x.y:1|c\n"))
	defer func() { _ = c1.Close() }()
	waitSeen(t, srv, "x.y", 1)
	if n := srv.ConnCount(); n != 1 {
		t.Fatalf("ConnCount after 1 dial = %d, want 1", n)
	}

	c2 := dialAndWrite(t, srv, []byte("x.y:1|c\n"))
	defer func() { _ = c2.Close() }()
	waitSeen(t, srv, "x.y", 2)
	if n := srv.ConnCount(); n != 2 {
		t.Fatalf("ConnCount after 2 dials = %d, want 2", n)
	}
}

// TestTCPSplitReadMidToken is THE trap-1 test. The probe measured a ~200 KB
// post-reconnect write arriving in <=65536-byte recv() chunks that split lines
// MID-TOKEN. Here the split is forced and traced byte-for-byte.
//
// The full logical stream is:
//
//	"sdpfx.cluster.c_backend.upstream_rq_total:7|c\nsdpfx.server.live:1|g\n"
//
// Chunk 1 is the first 50 bytes:
//
//	"sdpfx.cluster.c_backend.upstream_rq_total:7|c\nsdpfx"
//
// i.e. a COMPLETE first line (44 bytes + '\n' at index 44) followed by the
// 5-byte fragment "sdpfx" — the second line split mid-NAME, before its ':'.
// A datagram parser would see "sdpfx" as a whole line, fail to find ':', and
// drop it. A stream parser MUST carry "sdpfx" as a remainder and only emit the
// second line once chunk 2 supplies ".server.live:1|g\n".
func TestTCPSplitReadMidToken(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	const line1 = "sdpfx.cluster.c_backend.upstream_rq_total:7|c\n"
	const line2 = "sdpfx.server.live:1|g\n"
	full := line1 + line2
	const split = 50 // len(line1) == 45; 50 lands 5 bytes into line2's name

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte(full[:split])); err != nil {
		t.Fatalf("write chunk 1: %v", err)
	}
	waitSeen(t, srv, "sdpfx.cluster.c_backend.upstream_rq_total", 1)
	// The remainder "sdpfx" must NOT have been ingested as a line.
	if n := srv.UnparsedCount(); n != 0 {
		t.Fatalf("UnparsedCount after the mid-token chunk = %d, want 0 "+
			"(the %q remainder must be carried, not parsed)", n, full[len(line1):split])
	}
	if _, ok := srv.Gauge("sdpfx.server.live"); ok {
		t.Fatal("sdpfx.server.live must not be visible before its line completes")
	}

	if _, err := conn.Write([]byte(full[split:])); err != nil {
		t.Fatalf("write chunk 2: %v", err)
	}
	waitSeen(t, srv, "sdpfx.server.live", 1)
	if v, ok := srv.Gauge("sdpfx.server.live"); !ok || v != 1 {
		t.Errorf("Gauge(sdpfx.server.live) = %v,%v; want 1,true", v, ok)
	}
	if n := srv.UnparsedCount(); n != 0 {
		t.Errorf("UnparsedCount = %d, want 0", n)
	}
}

// TestTCPDiscardsIncompleteTrailingLineAtEOF pins the property that SPEC §3.5's
// no-DUPLICATION argument depends on: a connection that dies mid-line must
// DISCARD the straddling line, so the sink can safely re-send it whole.
//
// The trailing partial MUST be a truncation of a WELL-FORMED, COMPLETE line
// ("partial.line:9|c" with the trailing '\n' withheld) rather than something
// structurally incomplete ("partial.li", no '|'). If the fragment could never
// parse as a valid line, DeltaSum("partial.li") would report absent whether or
// not the discard actually happened — a broken implementation that wrongly
// ingested the remainder would still bail out at ingestLine's `pipe1 < 0`
// check and bump unparsed instead of populating deltaSums, so the assertion
// could not distinguish correct from broken. By truncating a line that WOULD
// parse (has a '|', a ':', a numeric value, and a known type) if wrongly
// ingested, a bufio.Scanner-style regression that emits the final unterminated
// token as a line populates deltaSums["partial.line"]=9 and trips the
// DeltaSum assertion below — making it genuinely load-bearing.
func TestTCPDiscardsIncompleteTrailingLineAtEOF(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	// NOTE: no trailing '\n' after "partial.line:9|c" — it is buffered
	// unterminated when the connection closes.
	conn := dialAndWrite(t, srv, []byte("done.line:5|c\npartial.line:9|c"))
	waitSeen(t, srv, "done.line", 1)
	_ = conn.Close() // EOF with "partial.line:9|c" buffered and unterminated

	time.Sleep(100 * time.Millisecond) // let the stream goroutine observe EOF
	if sum, ok := srv.DeltaSum("partial.line"); ok {
		t.Fatalf("the incomplete trailing line was ingested (DeltaSum=%v); it MUST be discarded at EOF", sum)
	}
	if n := srv.UnparsedCount(); n != 0 {
		t.Errorf("UnparsedCount = %d, want 0 (a discarded partial line is not an unparsed line)", n)
	}
	if sum, ok := srv.DeltaSum("done.line"); !ok || sum != 5 {
		t.Errorf("DeltaSum(done.line) = %v,%v; want 5,true", sum, ok)
	}
}

// TestTCPResetPreservesConnCount is the regression guard for the plan's
// invariant (mirrored in Server.Reset's doc comment): Reset() zeroes the value
// accumulators and unparsed, but connCount is a lifetime transport fact and
// must survive a Reset.
func TestTCPResetPreservesConnCount(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	conn := dialAndWrite(t, srv, []byte("r.s:4|c\n"))
	defer func() { _ = conn.Close() }()
	waitSeen(t, srv, "r.s", 1)

	if n := srv.ConnCount(); n != 1 {
		t.Fatalf("ConnCount before Reset = %d, want 1", n)
	}

	srv.Reset()

	if n := srv.ConnCount(); n != 1 {
		t.Errorf("ConnCount after Reset = %d, want 1 (must NOT be zeroed)", n)
	}
	if n := srv.UnparsedCount(); n != 0 {
		t.Errorf("UnparsedCount after Reset = %d, want 0", n)
	}
	if _, ok := srv.DeltaSum("r.s"); ok {
		t.Error("DeltaSum(r.s) after Reset: expected ok=false (value accumulators must be cleared)")
	}
}

// TestTCPUnparsedCountCatchesConcatenatedLines is the LIVENESS proof for the
// differential's break (b): \n-SEPARATED framing concatenates the last line of
// one flush with the first of the next, producing an unknown metric type.
func TestTCPUnparsedCountCatchesConcatenatedLines(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()

	// "a.b:7|c" with NO terminator, immediately followed by "c.d:1|g\n".
	conn := dialAndWrite(t, srv, []byte("a.b:7|cc.d:1|g\n"))
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(2 * time.Second)
	for srv.UnparsedCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("UnparsedCount stayed 0; the concatenated line was not detected")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, ok := srv.DeltaSum("a.b"); ok {
		t.Error("a.b must NOT be accounted: its type token is the corrupted \"cc.d:1\"")
	}
}

// TestUDPPathUnchanged: the datagram accessors keep working; the TCP path leaves
// them unpopulated (documented in SPEC §3.9, not silently divergent).
func TestUDPPathUnchanged(t *testing.T) {
	srv, err := statsdrecv.NewAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewAtAddr: %v", err)
	}
	defer srv.Close()
	c, err := net.Dial("udp", srv.Addr())
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Write([]byte("u.v:2|c\nw.x:3|c\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	waitSeen(t, srv, "u.v", 1)
	if got := srv.MaxLinesInAnyDatagram(); got != 2 {
		t.Errorf("MaxLinesInAnyDatagram = %d, want 2", got)
	}
	if n, ok := srv.LinesInDatagram("u.v"); !ok || n != 2 {
		t.Errorf("LinesInDatagram(u.v) = %d,%v; want 2,true", n, ok)
	}
	if got := srv.ConnCount(); got != 0 {
		t.Errorf("ConnCount on a UDP receiver = %d, want 0 (connectionless)", got)
	}
}

func TestTCPLeavesDatagramAccessorsUnpopulated(t *testing.T) {
	srv, err := statsdrecv.NewTCPAtAddr("127.0.0.1:0")
	if err != nil {
		t.Fatalf("NewTCPAtAddr: %v", err)
	}
	defer srv.Close()
	c := dialAndWrite(t, srv, []byte("p.q:1|c\nr.s:2|c\n"))
	defer func() { _ = c.Close() }()
	waitSeen(t, srv, "r.s", 1)
	if got := srv.MaxLinesInAnyDatagram(); got != 0 {
		t.Errorf("MaxLinesInAnyDatagram on a TCP receiver = %d, want 0 (a stream has no datagrams)", got)
	}
	if _, ok := srv.LinesInDatagram("p.q"); ok {
		t.Error("LinesInDatagram must be unpopulated on the TCP path")
	}
}

// TestGraphiteTwoTagCounter covers phase 57 Task 7: a graphite line folds tags
// INTO the name as ";k=v" pairs. The receiver must strip them off the name,
// key DeltaSumTagged/Tags by the STRIPPED name, and treat the full ';'-bearing
// wire text as NOT a key in its own right (DeltaSum on it must miss).
func TestGraphiteTwoTagCounter(t *testing.T) {
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

	strippedName := "grpfx.http.downstream_rq_xx"
	fullLine := strippedName + ";envoy.response_code_class=2;envoy.http_conn_manager_prefix=hcm"
	msg := fullLine + ":5|c"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	waitSeen(t, srv, strippedName, 1)

	wantTags := map[string]string{
		"envoy.response_code_class":      "2",
		"envoy.http_conn_manager_prefix": "hcm",
	}
	sum, ok := srv.DeltaSumTagged(strippedName, wantTags)
	if !ok {
		t.Error("DeltaSumTagged: expected ok=true on the stripped name")
	} else if sum != 5 {
		t.Errorf("DeltaSumTagged: got %v, want 5", sum)
	}

	gotTags, tagsOK := srv.Tags(strippedName)
	if !tagsOK {
		t.Error("Tags: expected ok=true on the stripped name")
	} else if !maps.Equal(gotTags, wantTags) {
		t.Errorf("Tags: got %v, want %v", gotTags, wantTags)
	}

	// The unstripped, ';'-bearing name must NOT be a key in its own right.
	if _, fullOK := srv.DeltaSum(fullLine); fullOK {
		t.Error("DeltaSum(fullLine-name): expected ok=false; the ';'-bearing name must not be a key")
	}
}

// TestGraphiteOneTagGauge covers a single-tag graphite |g line.
func TestGraphiteOneTagGauge(t *testing.T) {
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

	strippedName := "grpfx.cluster.membership_total"
	msg := strippedName + ";envoy.cluster_name=b:1|g"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	waitSeen(t, srv, strippedName, 1)

	gval, gok := srv.Gauge(strippedName)
	if !gok {
		t.Error("Gauge: expected ok=true on the stripped name")
	} else if gval != 1 {
		t.Errorf("Gauge: got %v, want 1", gval)
	}

	wantTags := map[string]string{"envoy.cluster_name": "b"}
	gotTags, ok := srv.Tags(strippedName)
	if !ok {
		t.Fatal("Tags: expected ok=true")
	}
	if !maps.Equal(gotTags, wantTags) {
		t.Errorf("Tags: got %v, want %v", gotTags, wantTags)
	}
}

// TestGraphiteTagFreeLineUnchanged is the regression proof: a tag-free
// graphite-style line parses exactly as before the phase 57 extension.
func TestGraphiteTagFreeLineUnchanged(t *testing.T) {
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

	name := "grpfx.server.uptime"
	msg := name + ":3|g"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	waitSeen(t, srv, name, 1)

	gval, gok := srv.Gauge(name)
	if !gok {
		t.Fatal("Gauge: expected ok=true")
	}
	if gval != 3 {
		t.Errorf("Gauge: got %v, want 3", gval)
	}
	if tags, ok := srv.Tags(name); ok || tags != nil {
		t.Errorf("Tags(tag-free graphite name): got (%v, %v), want (nil, false)", tags, ok)
	}
	if n := srv.UnparsedCount(); n != 0 {
		t.Errorf("UnparsedCount = %d, want 0", n)
	}
}

// TestGraphiteMissingEqualsInPair covers a ';'-bearing name whose tag segment
// lacks '=': the line must be unaccountable (UnparsedCount==1) with no
// deltaSums/tags entry under any name.
func TestGraphiteMissingEqualsInPair(t *testing.T) {
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

	msg := "grpfx.x;badpair:1|c"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.UnparsedCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if n := srv.UnparsedCount(); n != 1 {
		t.Errorf("UnparsedCount: got %d, want 1", n)
	}
	if _, ok := srv.DeltaSum("grpfx.x"); ok {
		t.Error("DeltaSum(grpfx.x): expected ok=false; the malformed pair must not be accounted")
	}
	if _, ok := srv.Tags("grpfx.x"); ok {
		t.Error("Tags(grpfx.x): expected ok=false; the malformed pair must not be accounted")
	}
	if _, ok := srv.DeltaSum("grpfx.x;badpair"); ok {
		t.Error("DeltaSum(grpfx.x;badpair): expected ok=false; the malformed pair must not be accounted")
	}
}

// TestGraphiteDeltaAccumulation covers delta accumulation across two graphite
// lines carrying the SAME tag set on the same stripped name: 3+2 == 5.
func TestGraphiteDeltaAccumulation(t *testing.T) {
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

	strippedName := "grpfx.cluster.upstream_rq_total"
	for _, msg := range []string{
		strippedName + ";envoy.cluster_name=b:3|c",
		strippedName + ";envoy.cluster_name=b:2|c",
	} {
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatalf("Write %q: %v", msg, err)
		}
	}

	waitSeen(t, srv, strippedName, 2)

	wantTags := map[string]string{"envoy.cluster_name": "b"}
	sum, ok := srv.DeltaSumTagged(strippedName, wantTags)
	if !ok {
		t.Fatal("DeltaSumTagged: expected ok=true")
	}
	if sum != 5 {
		t.Errorf("DeltaSumTagged: got %v, want 5", sum)
	}
}

// TestGraphiteAndDogStatsdCoexist covers a dog_statsd '|#'-tagged line and a
// graphite ';'-tagged line for DIFFERENT names sharing one receiver: both
// grammars must bucket correctly through the same tag machinery.
func TestGraphiteAndDogStatsdCoexist(t *testing.T) {
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

	dsdName := "dsdpfx.cluster.upstream_rq_total"
	grName := "grpfx.cluster.upstream_rq_total"
	for _, msg := range []string{
		dsdName + ":4|c|#envoy.cluster_name:backend",
		grName + ";envoy.cluster_name=backend:9|c",
	} {
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatalf("Write %q: %v", msg, err)
		}
	}

	waitSeen(t, srv, dsdName, 1)
	waitSeen(t, srv, grName, 1)

	wantTags := map[string]string{"envoy.cluster_name": "backend"}

	dsdSum, dsdOK := srv.DeltaSumTagged(dsdName, wantTags)
	if !dsdOK || dsdSum != 4 {
		t.Errorf("DeltaSumTagged(dsdName): got (%v, %v), want (4, true)", dsdSum, dsdOK)
	}
	grSum, grOK := srv.DeltaSumTagged(grName, wantTags)
	if !grOK || grSum != 9 {
		t.Errorf("DeltaSumTagged(grName): got (%v, %v), want (9, true)", grSum, grOK)
	}
}

// TestGraphiteMultiLineDatagram covers the batching signal: a two-line
// datagram where the second line carries graphite tags must key
// LinesInDatagram by the STRIPPED name.
func TestGraphiteMultiLineDatagram(t *testing.T) {
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

	strippedName := "grpfx.cluster.upstream_cx_total"
	line1 := "grpfx.other.metric:1|c"
	line2 := strippedName + ";envoy.cluster_name=b:2|c"
	if _, err := conn.Write([]byte(line1 + "\n" + line2)); err != nil {
		t.Fatalf("Write: %v", err)
	}

	waitSeen(t, srv, strippedName, 1)

	lines, ok := srv.LinesInDatagram(strippedName)
	if !ok || lines != 2 {
		t.Errorf("LinesInDatagram(%s): got (%v, %v), want (2, true)", strippedName, lines, ok)
	}
}

// TestGraphiteResetClears is the confirm-test for Reset(): the graphite tag
// state lives in the existing maps, so Reset() clears it the same way it
// clears dog_statsd tag state.
func TestGraphiteResetClears(t *testing.T) {
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

	strippedName := "grpfx.cluster.reset_check"
	msg := strippedName + ";envoy.cluster_name=b:6|c"
	if _, err := conn.Write([]byte(msg)); err != nil {
		t.Fatalf("Write %q: %v", msg, err)
	}

	waitSeen(t, srv, strippedName, 1)
	if _, ok := srv.Tags(strippedName); !ok {
		t.Fatal("Tags: expected ok=true before Reset")
	}

	srv.Reset()

	if _, ok := srv.DeltaSum(strippedName); ok {
		t.Error("DeltaSum after Reset: expected ok=false")
	}
	if _, ok := srv.DeltaSumTagged(strippedName, map[string]string{"envoy.cluster_name": "b"}); ok {
		t.Error("DeltaSumTagged after Reset: expected ok=false")
	}
	if tags, ok := srv.Tags(strippedName); ok || tags != nil {
		t.Errorf("Tags after Reset: got (%v, %v), want (nil, false)", tags, ok)
	}
	if n := srv.SeenCount(strippedName); n != 0 {
		t.Errorf("SeenCount after Reset: got %d, want 0", n)
	}
}
