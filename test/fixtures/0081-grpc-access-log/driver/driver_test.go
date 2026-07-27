package driver

import (
	"fmt"
	"net"
	"testing"
)

// TestAllocateALSPortIsBackedByALiveListener pins the ALS receiver's port
// allocation against the reserve-then-release TOCTOU that aborted a full
// differential run:
//
//	panic: driver: start ALS receiver on 0.0.0.0:36523:
//	       listen tcp 0.0.0.0:36523: bind: address already in use
//
// The port must be held by a LIVE listener from the moment the driver hands it
// out. Reserving it with a Listen+Close probe and rebinding later leaves a
// window in which any other listener in the same test binary — the runner
// allocates backend/admin/listener ports for ~100 fixtures in one process, and
// the kernel hands out ephemeral ports from the same range — can take it. The
// receiver's bind then fails, and because the fixture.Driver interface has no
// error return the failure surfaces as a panic that kills the whole run rather
// than one fixture.
//
// Binding on the wildcard address (not loopback) is also load-bearing: the
// receiver binds 0.0.0.0 so the reference container can reach it via
// host.docker.internal, and a port free on 127.0.0.1 is not necessarily free
// on 0.0.0.0.
func TestAllocateALSPortIsBackedByALiveListener(t *testing.T) {
	d := &alsDriver{}
	t.Cleanup(func() {
		if d.srv != nil {
			d.srv.Close()
		}
	})

	port := d.allocateALSPort()
	if port == 0 {
		t.Fatal("allocateALSPort returned port 0")
	}

	// If the port is genuinely held, re-binding it must fail. Probe the same
	// wildcard address the receiver itself binds so the check matches the
	// receiver's bind semantics exactly.
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		_ = ln.Close()
		t.Fatalf("port %d handed out by allocateALSPort is still free: an unrelated "+
			"listener can take it before the ALS receiver binds it", port)
	}
}

// TestAllocateALSPortIsStableAcrossCalls pins the invariant the two bootstrap
// renderers depend on: ReferenceBootstrap and SubjectConfig each call
// allocateALSPort and must template the SAME port into both sides' configs.
func TestAllocateALSPortIsStableAcrossCalls(t *testing.T) {
	d := &alsDriver{}
	t.Cleanup(func() {
		if d.srv != nil {
			d.srv.Close()
		}
	})

	first := d.allocateALSPort()
	second := d.allocateALSPort()
	if first != second {
		t.Fatalf("allocateALSPort is not idempotent: got %d then %d", first, second)
	}
}
