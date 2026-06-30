package statsdrecv_test

import (
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
}
