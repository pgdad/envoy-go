// Source-port-bind reuse helpers for the 0008 driver. Linux-only build
// constraint mirrors syscall.Setsockopt usage in similar test scaffolding;
// the differential suite is gated on Linux test environments via the
// runner's ensureDocker check, so non-Linux builds are not exercised here.
//
//go:build linux

package driver

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// setReuseSockopts is a net.Dialer.Control hook that enables SO_REUSEADDR
// + SO_REUSEPORT on the source-bind socket. This lets the driver re-bind
// 127.0.0.1:knownDriverPort across the 4 connections (2 of which use
// the same source port) even if a prior socket sits in TIME_WAIT.
//
// Without SO_REUSEADDR, the second source-bind to the same port fails
// with EADDRINUSE while the kernel's TIME_WAIT clears (typically 60s
// per Linux's sysctl tcp_fin_timeout default; the test runs both source
// -bound connections within milliseconds of each other, so the
// TIME_WAIT window is overwhelmingly likely to be hit).
func setReuseSockopts(_ /*network*/ string, _ /*address*/ string, c syscall.RawConn) error {
	var sockErr error
	err := c.Control(func(fd uintptr) {
		// SO_REUSEADDR: allow bind to a port in TIME_WAIT.
		if e := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); e != nil {
			sockErr = e
			return
		}
		// SO_REUSEPORT: allow multiple sockets to share the port. Strictly
		// the test only needs SO_REUSEADDR for back-to-back source-binds;
		// SO_REUSEPORT is set as well as a defensive measure (cheap, and
		// makes the source-bind robust to any future test parallelism).
		if e := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); e != nil {
			sockErr = e
			return
		}
	})
	if err != nil {
		return err
	}
	return sockErr
}
