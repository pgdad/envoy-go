package zookeeperproxy

import (
	"bytes"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	zookeeper_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/stats"
)

// --- test frame builders (big-endian; 4-byte length prefix EXCLUDES itself) ---

func be32(v int32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return b
}

func be64(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func zkFrame(parts ...[]byte) []byte {
	payload := bytes.Join(parts, nil)
	return append(be32(int32(len(payload))), payload...)
}

// connectFrame: protocol_version(4=0) + last_zxid(8=0) + timeout(4) + session(8=0)
// + password(4-byte len + 16 bytes) [+ optional readonly bool(1)].
// The leading protocol_version=0 doubles as the sniffed ConnectXid (AMEND-A5).
func connectFrame(readonly *bool) []byte {
	parts := [][]byte{be32(0), be64(0), be32(30000), be64(0), be32(16), make([]byte, 16)}
	if readonly != nil {
		b := byte(0)
		if *readonly {
			b = 1
		}
		parts = append(parts, []byte{b})
	}
	return zkFrame(parts...)
}

// dataFrame: xid(4) + opcode(4) + payload.
func dataFrame(xid, opcode int32, payload []byte) []byte {
	return zkFrame(be32(xid), be32(opcode), payload)
}

func newTestDecoder(t *testing.T) (*decoder, *rosterStats, *compiledConfig) {
	t.Helper()
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk"})
	if err != nil {
		t.Fatal(err)
	}
	rs := newRosterStats(reg, "zk")
	return newDecoder(cfg, rs), rs, cfg
}

func counterValue(t *testing.T, rs *rosterStats, suffix string) uint64 {
	t.Helper()
	return rs.counters[suffix].Load()
}

// --- special-xid constants (response-side xid, present for completeness) ---

// TestSpecialXidConstants verifies the special xid constants and ensures the
// watchXid constant is exercised (it is declared for completeness as per AMEND-A5,
// but is response-side only; used at 28.2). This prevents a lint "unused constant"
// on watchXid.
func TestSpecialXidConstants(t *testing.T) {
	if connectXid != 0 {
		t.Fatalf("connectXid = %d, want 0", connectXid)
	}
	if watchXid != -1 {
		t.Fatalf("watchXid = %d, want -1", watchXid)
	}
	if pingXid != -2 {
		t.Fatalf("pingXid = %d, want -2", pingXid)
	}
	if authXid != -4 {
		t.Fatalf("authXid = %d, want -4", authXid)
	}
	if setWatchesXid != -8 {
		t.Fatalf("setWatchesXid = %d, want -8", setWatchesXid)
	}
}

// --- special-xid dispatch (AMEND-A5) ---

func TestDecodeConnect(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(connectFrame(nil), int64(len(connectFrame(nil))))
	if got := counterValue(t, rs, "connect_rq"); got != 1 {
		t.Fatalf("connect_rq = %d, want 1", got)
	}
	if got := counterValue(t, rs, "connect_readonly_rq"); got != 0 {
		t.Fatalf("connect_readonly_rq = %d, want 0", got)
	}
}

func TestDecodeConnectReadonly(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	ro := true
	d.decodeOnData(connectFrame(&ro), int64(len(connectFrame(&ro))))
	if got := counterValue(t, rs, "connect_readonly_rq"); got != 1 {
		t.Fatalf("connect_readonly_rq = %d, want 1", got)
	}
	if got := counterValue(t, rs, "connect_rq"); got != 0 {
		t.Fatalf("connect_rq = %d, want 0 (readonly connect counts ONLY connect_readonly_rq)", got)
	}
}

func TestDecodePing(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(zkFrame(be32(pingXid), be32(opPing)), int64(len(zkFrame(be32(pingXid), be32(opPing)))))
	if got := counterValue(t, rs, "ping_rq"); got != 1 {
		t.Fatalf("ping_rq = %d, want 1", got)
	}
}

// authFrame builds a real ZooKeeper auth request frame (length-prefix stripped
// inside zkFrame): xid(-4) | opcode(100) | type(0) | schemeLen | scheme | credLen.
// This mirrors upstream parseAuthRequest's wire layout (decoder.cc:396-413): the
// "Skip opcode + type" step means schemeLen sits at frame offset 12 — a frame
// WITHOUT the type field fails upstream's parse (peekString read-beyond-buffer).
func authFrame(scheme string) []byte {
	s := []byte(scheme)
	return zkFrame(be32(authXid), be32(opSetAuth) /* opcode 100 */, be32(0) /* type */, be32(int32(len(s))), s, be32(0) /* cred len */)
}

// auth: xid −4 → skip opcode(100) + type int(4) → scheme string (4-byte len +
// bytes) → dynamic auth.<scheme>_rq counter (AMEND-A3). A builtin scheme
// ("digest") gets its own counter.
func TestDecodeAuthScheme(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(authFrame("digest"), int64(len(authFrame("digest"))))
	got := rs.reg.NewCounterIfAbsent("zk.zookeeper.auth.digest_rq").Load()
	if got != 1 {
		t.Fatalf("auth.digest_rq = %d, want 1", got)
	}
	// NO static auth_rq counter exists (AMEND-A3) — nothing else incremented.
}

// A non-builtin scheme ("foobar", valid charset but NOT in the upstream builtin
// set) takes the unknown_scheme fallback: auth.unknown_scheme_rq increments and
// NO auth.foobar_rq counter is ever created (upstream getBuiltin parity;
// live-verified scheme "foobar" → zkauth.zookeeper.auth.unknown_scheme_rq).
func TestDecodeAuthSchemeNonBuiltin(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(authFrame("foobar"), int64(len(authFrame("foobar"))))
	// The unknown_scheme fallback counter incremented (this is the live-checkable
	// assertion: we deliberately do NOT probe auth.foobar_rq via NewCounterIfAbsent
	// because that would CREATE it at 0 and mask a divergence — instead we scan the
	// registry to prove no foobar counter exists at all).
	if got := rs.reg.NewCounterIfAbsent("zk.zookeeper.auth.unknown_scheme_rq").Load(); got != 1 {
		t.Fatalf("auth.unknown_scheme_rq = %d, want 1 (non-builtin scheme fallback)", got)
	}
	rs.reg.Walk(func(m stats.Metric) {
		if m.Name() == "zk.zookeeper.auth.foobar_rq" {
			t.Fatalf("auth.foobar_rq must NEVER be created (non-builtin → unknown_scheme)")
		}
	})
}

func TestDecodeSetWatches(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(zkFrame(be32(setWatchesXid), be32(opSetWatches)), int64(len(zkFrame(be32(setWatchesXid), be32(opSetWatches)))))
	if got := counterValue(t, rs, "setwatches_rq"); got != 1 {
		t.Fatalf("setwatches_rq = %d, want 1", got)
	}
}

// --- reassembly + the high-water mark (D-S28.1-3; AMEND-A8) ---

// Partial frames: a frame split across two decodeOnData calls (cumulative chain
// buffer — the chain Buffer is never drained, so call 2 sees call 1's bytes too).
func TestDecodePartialFrameReassembly(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := dataFrame(1, opGetData, []byte("/path"))
	cut := len(frame) / 2
	// First read: chain buffer holds the first half.
	d.decodeOnData(frame[:cut], int64(len(frame[:cut])))
	if got := counterValue(t, rs, "getdata_rq"); got != 0 {
		t.Fatalf("getdata_rq = %d after partial frame, want 0", got)
	}
	// Second read: chain buffer now holds the WHOLE accumulating buffer
	// (the chain Buffer accumulates: zookeeperproxy never drains it).
	d.decodeOnData(frame, int64(len(frame)))
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d after reassembly, want 1", got)
	}
}

// The high-water mark: re-delivered (undrained) chain-buffer bytes are NOT
// double-counted (D-S28.1-3 — the multi-read no-double-count proof).
// padTo is defined in the Task 10 test block below; these opcodes have min-lengths
// in the table so we need valid (padded) frames to pass min-length validation.
func TestDecodeHighWaterMarkNoDoubleCount(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	f1 := dataFrame(1, opGetData, make([]byte, 5)) // 8+5=13 bytes >= getdata min(13)
	f2 := dataFrame(2, opCreate, make([]byte, 16)) // 8+16=24 bytes >= create min(24)
	// Read 1: chain buffer = f1.
	d.decodeOnData(f1, int64(len(f1)))
	// Read 2: chain buffer = f1 + f2 (accumulated — f1 is RE-DELIVERED).
	d.decodeOnData(append(append([]byte{}, f1...), f2...), int64(len(append(append([]byte{}, f1...), f2...))))
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (re-delivered bytes must not double-count)", got)
	}
	if got := counterValue(t, rs, "create_rq"); got != 1 {
		t.Fatalf("create_rq = %d, want 1", got)
	}
}

// Two complete frames in one read decode in sequence.
func TestDecodeTwoFramesOneRead(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	// 8+5=13 bytes >= getdata min(13); 8+5=13 bytes >= exists min(13)
	buf := append(dataFrame(1, opGetData, make([]byte, 5)), dataFrame(2, opExists, make([]byte, 5))...)
	d.decodeOnData(buf, int64(len(buf)))
	if counterValue(t, rs, "getdata_rq") != 1 || counterValue(t, rs, "exists_rq") != 1 {
		t.Fatal("both frames in a single read must decode")
	}
}

// The decoder NEVER mutates the chain bytes it is fed (R3 precondition; the
// fuzzer re-asserts this). Using opClose (universal 8-byte min) avoids padding.
func TestDecodeDoesNotMutateInput(t *testing.T) {
	d, _, _ := newTestDecoder(t)
	frame := dataFrame(1, opClose, nil) // opClose has no entry in dataRequestMinLength → universal 8
	orig := append([]byte(nil), frame...)
	d.decodeOnData(frame, int64(len(frame)))
	if !bytes.Equal(frame, orig) {
		t.Fatal("decodeOnData mutated its input slice")
	}
}

// --- data-request dispatch (Task 10) ---

// padTo returns filler payload bytes meeting dataRequestMinLength for the given
// opcode so that min-length validation passes. The dataFrame builder adds xid(4)
// + opcode(4) = 8 bytes; padTo supplies the additional bytes to reach the opcode's
// minimum. For opcodes whose minimum is the universal 8 (not in the table), padTo
// returns nil.
func padTo(opcode int32) []byte {
	minLen, ok := dataRequestMinLength[opcode]
	if !ok {
		return nil // universal 8-byte minimum; no extra bytes needed
	}
	extra := minLen - 8 // xid(4)+opcode(4) already in the frame header
	if extra <= 0 {
		return nil
	}
	return make([]byte, extra)
}

// Every wire opcode dispatches to its <opname>_rq counter — including the
// digit-suffixed ones (reference_proto_roster_extraction_digits guard).
func TestDecodeDataRequestAllOpcodes(t *testing.T) {
	cases := []struct {
		opcode int32
		suffix string
	}{
		{opGetData, "getdata_rq"},
		{opCreate, "create_rq"},
		{opCreate2, "create2_rq"},
		{opGetChildren2, "getchildren2_rq"},
		{opSetWatches2, "setwatches2_rq"},
		{opGetAllChildrenNumber, "getallchildrennumber_rq"},
		{opClose, "close_rq"},
		{opMulti, "multi_rq"},
		{opDelete, "delete_rq"},
		{opCheckWatches, "checkwatches_rq"},
	}
	for _, tc := range cases {
		t.Run(tc.suffix, func(t *testing.T) {
			d, rs, _ := newTestDecoder(t)
			d.decodeOnData(dataFrame(1, tc.opcode, padTo(tc.opcode)), int64(len(dataFrame(1, tc.opcode, padTo(tc.opcode)))))
			if got := counterValue(t, rs, tc.suffix); got != 1 {
				t.Fatalf("%s = %d, want 1", tc.suffix, got)
			}
		})
	}
}

// SetAuth as a DATA request (xid > 0, opcode 100): upstream's data-request
// switch has NO SetAuth case (decoder.cc:134-244 v1.37.2) — it decode-errors
// (onDecodeError(nullopt): plain decoder_error, no per-opcode counter, no
// correlation). Mirror exactly (cross-side stat parity is load-bearing).
func TestDecodeSetAuthDataRequest(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	scheme := []byte("digest")
	payload := append(append(append(be32(0), be32(int32(len(scheme)))...), scheme...), be32(0)...)
	d.decodeOnData(dataFrame(5, opSetAuth, payload), int64(len(dataFrame(5, opSetAuth, payload))))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (upstream: SetAuth data request is a decode error)", got)
	}
	// The dynamic auth counter must NOT fire (that's the AuthXid -4 path only):
	if got := rs.reg.NewCounterIfAbsent("zk.zookeeper.auth.digest_rq").Load(); got != 0 {
		t.Fatalf("auth.digest_rq = %d, want 0 (data-xid SetAuth is not an auth request)", got)
	}
	// setauth_rq stays dead:
	if got := counterValue(t, rs, "setauth_rq"); got != 0 {
		t.Fatalf("setauth_rq = %d, want 0", got)
	}
	// NO correlation write (upstream returns before requests_by_xid_ write):
	if _, ok := d.requestsByXid[5]; ok {
		t.Fatal("requestsByXid[5] must NOT be written for a decode-errored SetAuth data request")
	}
}

// Unknown opcode → decoder_error (no per-opcode counter — opcode unknown).
func TestDecodeUnknownOpcode(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnData(dataFrame(1, 9999, nil), int64(len(dataFrame(1, 9999, nil))))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// Oversized frame (len > max_packet_bytes) → decoder_error + buffer abandoned;
// later reads decode normally (the 0046 arm-4 unit mirror).
func TestDecodeOversizedThenRecovers(t *testing.T) {
	reg := stats.NewRegistry()
	cfg, _ := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
		MaxPacketBytes: wrapperspb.UInt32(64)})
	rs := newRosterStats(reg, "zk")
	d := newDecoder(cfg, rs)
	// Oversized: length prefix says 1000 > 64.
	oversized := append(be32(1000), make([]byte, 10)...)
	d.decodeOnData(oversized, int64(len(oversized)))
	if got := rs.counters["decoder_error"].Load(); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	// Later read (fresh bytes appended after the abandoned buffer): decodes fine.
	prior := d.chainConsumed
	good := dataFrame(1, opGetData, padTo(opGetData))
	cumulative := append(make([]byte, prior), good...) // chain buffer grew by `good`
	d.decodeOnData(cumulative, prior+int64(len(good))) // totalAppended is cumulative
	if got := rs.counters["getdata_rq"].Load(); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (decoder must recover after abandon)", got)
	}
}

// Min-length validation (D-S28.1-1): a known opcode with a too-short frame →
// decoder_error + (flag-gated) <opname>_decoder_error.
// opGetData minimum is 13 (XID+OPCODE+INT+BOOL); sending only xid+opcode (8 bytes)
// triggers the error.
func TestDecodeMinLengthViolation(t *testing.T) {
	reg := stats.NewRegistry()
	cfg, _ := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
		EnablePerOpcodeDecoderErrorMetrics: true})
	rs := newRosterStats(reg, "zk")
	d := newDecoder(cfg, rs)
	// Only xid+opcode (8 bytes) — getdata minimum is 13.
	d.decodeOnData(dataFrame(1, opGetData, nil), int64(len(dataFrame(1, opGetData, nil))))
	if got := rs.counters["decoder_error"].Load(); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := rs.counters["getdata_decoder_error"].Load(); got != 1 {
		t.Fatalf("getdata_decoder_error = %d, want 1 (flag enabled + opcode known)", got)
	}
}

// Flag gating (AMEND-A2): _rq_bytes increments ONLY when the flag is true;
// request_bytes increments ALWAYS; the wire footprint includes the 4-byte prefix.
func TestDecodeFlagGatedRequestBytes(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		reg := stats.NewRegistry()
		msg := &zookeeper_proxyv3.ZooKeeperProxy{StatPrefix: "zk",
			EnablePerOpcodeRequestBytesMetrics: enabled}
		cfg, _ := parseConfig(msg)
		rs := newRosterStats(reg, "zk")
		d := newDecoder(cfg, rs)
		frame := dataFrame(1, opGetData, padTo(opGetData))
		d.decodeOnData(frame, int64(len(frame)))
		wantWire := uint64(len(frame)) // 4-byte prefix + payload
		if got := rs.counters["request_bytes"].Load(); got != wantWire {
			t.Fatalf("request_bytes = %d, want %d (always counted)", got, wantWire)
		}
		gotGated := rs.counters["getdata_rq_bytes"].Load()
		if enabled && gotGated != wantWire {
			t.Fatalf("getdata_rq_bytes = %d, want %d (flag on)", gotGated, wantWire)
		}
		if !enabled && gotGated != 0 {
			t.Fatalf("getdata_rq_bytes = %d, want 0 (flag off — gates increments not creation)", gotGated)
		}
	}
}

// --- control-frame decoder_error paths (closing the Task-10 review gap) ---

// TestDecodeControlFrameErrors verifies the flag-gated connect_decoder_error
// and auth_decoder_error counters are live, and that the universal sub-8-byte
// path fires plain decoder_error only.
func TestDecodeControlFrameErrors(t *testing.T) {
	mkDecoder := func(flagOn bool) (*decoder, *rosterStats) {
		reg := stats.NewRegistry()
		cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
			StatPrefix:                         "zk",
			EnablePerOpcodeDecoderErrorMetrics: flagOn,
		})
		if err != nil {
			t.Fatal(err)
		}
		rs := newRosterStats(reg, "zk")
		return newDecoder(cfg, rs), rs
	}

	t.Run("short connect frame", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// xid=0 (connectXid) but frame only 12 bytes < 28-byte fixed header:
		// decodeFrame universal 8-byte check passes; onConnect fires decoderError("connect").
		d.decodeOnData(zkFrame(be32(connectXid), be32(0), be32(0)), int64(len(zkFrame(be32(connectXid), be32(0), be32(0)))))
		if rs.counters["decoder_error"].Load() != 1 || rs.counters["connect_decoder_error"].Load() != 1 {
			t.Fatalf("connect short frame: decoder_error=%d connect_decoder_error=%d, want 1/1",
				rs.counters["decoder_error"].Load(), rs.counters["connect_decoder_error"].Load())
		}
	})

	t.Run("negative connect pwLen", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// Exactly 28-byte frame (fixedLen): passes len<fixedLen check; pwLen=-1 fires decoderError.
		// be32(0)=protocol_version, be64(0)=last_zxid, be32(30000)=timeout,
		// be64(0)=session_id, be32(-1)=password_length (negative).
		d.decodeOnData(zkFrame(be32(0), be64(0), be32(30000), be64(0), be32(-1)), int64(len(zkFrame(be32(0), be64(0), be32(30000), be64(0), be32(-1)))))
		if rs.counters["connect_decoder_error"].Load() != 1 {
			t.Fatal("negative pwLen must take the connect decoder_error path")
		}
	})

	t.Run("short auth frame", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// xid=-4 (authXid): 16-byte frame passes decodeFrame universal 8-byte check
		// but is < the 20-byte auth floor (XID+OPCODE+INT+INT+INT, decoder.cc:397-398)
		// → onAuth fires decoderError("auth").
		// Layout: xid | opcode | type | schemeLen (4×4 = 16 bytes < 20-byte floor).
		d.decodeOnData(zkFrame(be32(authXid), be32(opSetAuth), be32(0), be32(0)), int64(len(zkFrame(be32(authXid), be32(opSetAuth), be32(0), be32(0)))))
		if rs.counters["auth_decoder_error"].Load() != 1 {
			t.Fatal("short auth frame must take the auth decoder_error path")
		}
	})

	t.Run("negative auth schemeLen", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// 20-byte frame passes the len<20 floor; schemeLen at offset 12 = -1
		// fires decoderError("auth"). Layout: xid | opcode | type | schemeLen(-1) | pad(4).
		// The 4-byte pad is needed so the frame meets the 20-byte floor and the
		// negative-schemeLen branch is independently exercised (not shadowed by floor).
		d.decodeOnData(zkFrame(be32(authXid), be32(opSetAuth), be32(0), be32(-1), be32(0)), int64(len(zkFrame(be32(authXid), be32(opSetAuth), be32(0), be32(-1), be32(0)))))
		if rs.counters["auth_decoder_error"].Load() != 1 {
			t.Fatal("negative schemeLen must take the auth decoder_error path")
		}
	})

	t.Run("sub-8-byte frame", func(t *testing.T) {
		d, rs := mkDecoder(true)
		// 4-byte frame: passes nextFrame (frameLen=0 valid), but len(frame)=0 < 8
		// → decodeFrame fires decoderError("") — plain counter only, no per-opcode.
		d.decodeOnData(zkFrame(be32(1)), int64(len(zkFrame(be32(1))))) // 4-byte payload = 4-byte frame < universal 8 min
		if rs.counters["decoder_error"].Load() != 1 {
			t.Fatal("sub-8-byte frame must take the plain decoder_error path")
		}
	})

	t.Run("flag off gates per-opcode counters", func(t *testing.T) {
		d, rs := mkDecoder(false)
		d.decodeOnData(zkFrame(be32(connectXid), be32(0), be32(0)), int64(len(zkFrame(be32(connectXid), be32(0), be32(0))))) // 12-byte short connect
		if rs.counters["decoder_error"].Load() != 1 {
			t.Fatal("plain decoder_error must fire regardless of flag")
		}
		if rs.counters["connect_decoder_error"].Load() != 0 {
			t.Fatal("per-opcode counter must NOT fire when flag is off")
		}
	})
}

// --- correlation structures ---

// Correlation structures (R5): data requests land in requestsByXid (insert
// overwrites); control requests append to the per-xid FIFO queue.
func TestDecodeCorrelationStructuresPopulated(t *testing.T) {
	d, _, _ := newTestDecoder(t)
	d.decodeOnData(dataFrame(7, opGetData, padTo(opGetData)), int64(len(dataFrame(7, opGetData, padTo(opGetData)))))
	pr, ok := d.requestsByXid[7]
	if !ok || pr.opname != "getdata" || pr.wireOpcode != opGetData {
		t.Fatalf("requestsByXid[7] = (%+v, %v), want getdata entry", pr, ok)
	}
	// Insert overwrites (AMEND-A7): cumulative chain buffer = first-frame + second-frame.
	// chainConsumed after first call = len(dataFrame(7, opGetData, padTo(opGetData))).
	first := dataFrame(7, opGetData, padTo(opGetData))
	second := dataFrame(7, opExists, padTo(opExists))
	d.decodeOnData(append(first, second...), int64(len(append(first, second...))))
	if d.requestsByXid[7].opname != "exists" {
		t.Fatalf("requestsByXid[7].opname = %q, want exists (insert overwrites)", d.requestsByXid[7].opname)
	}
	// Control FIFO: two pings queue in order.
	d2, _, _ := newTestDecoder(t)
	ping := zkFrame(be32(pingXid), be32(opPing))
	d2.decodeOnData(append(append([]byte{}, ping...), ping...), int64(len(append(append([]byte{}, ping...), ping...))))
	if got := len(d2.controlRequestsByXid[pingXid]); got != 2 {
		t.Fatalf("control queue len = %d, want 2 (FIFO per control xid)", got)
	}
}

// --- §3.3 drain-regime re-base (28.1b) ---

// TestDecodeFeedAfterRuntimeDrain proves the §3.3 re-base: after the runtime
// drains the chain buffer (terminal handoff / post-handoff replay), the
// physical chainBytes slice RESTARTS while totalAppended keeps growing — the
// decoder must keep decoding the new tail with no drop and no double-count.
func TestDecodeFeedAfterRuntimeDrain(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	ping := zkFrame(be32(pingXid), be32(opPing))
	getdata := dataFrame(1, opGetData, padTo(opGetData))

	// Pre-drain feed: cumulative regime (chainBytes == all bytes ever appended).
	d.decodeOnData(ping, int64(len(ping)))
	// The runtime now drains the chain buffer (handoff or replay-pass drain).
	// Post-drain feed: chainBytes holds ONLY the new bytes; totalAppended is
	// cumulative across the drain.
	d.decodeOnData(getdata, int64(len(ping)+len(getdata)))

	if got := counterValue(t, rs, "ping_rq"); got != 1 {
		t.Fatalf("ping_rq = %d, want 1 (no double-count across the drain)", got)
	}
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (no drop across the drain)", got)
	}
}

// TestDecodeHandoffBoundarySequence proves the exact handoff regime: cumulative
// feeds pre-handoff, then a drain, then per-replay delta feeds — every frame
// decoded exactly once.
func TestDecodeHandoffBoundarySequence(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	f1 := zkFrame(be32(pingXid), be32(opPing))
	f2 := dataFrame(1, opGetData, padTo(opGetData))
	f3 := dataFrame(2, opGetData, padTo(opGetData))

	// Pre-handoff: two cumulative feeds (the chain buffer accumulates).
	d.decodeOnData(f1, int64(len(f1)))
	cum := append(append([]byte{}, f1...), f2...)
	d.decodeOnData(cum, int64(len(cum)))
	// Handoff: the runtime drains the buffer. Post-handoff replay feeds are
	// per-pass deltas (the replay drains after each pass).
	d.decodeOnData(f3, int64(len(cum)+len(f3)))

	if got := counterValue(t, rs, "ping_rq"); got != 1 {
		t.Fatalf("ping_rq = %d, want 1", got)
	}
	if got := counterValue(t, rs, "getdata_rq"); got != 2 {
		t.Fatalf("getdata_rq = %d, want 2 (f2 pre-handoff + f3 post-handoff, each exactly once)", got)
	}
}

// TestDecodePartialFrameAcrossDrainBoundary: a frame whose bytes arrive split
// across the drain boundary (first half pre-drain cumulative, second half
// post-drain delta) must still reassemble in the decoder-internal readBuf.
func TestDecodePartialFrameAcrossDrainBoundary(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := dataFrame(1, opGetData, padTo(opGetData))
	cut := len(frame) / 2

	d.decodeOnData(frame[:cut], int64(cut))        // pre-drain: first half
	d.decodeOnData(frame[cut:], int64(len(frame))) // post-drain: second half only
	if got := counterValue(t, rs, "getdata_rq"); got != 1 {
		t.Fatalf("getdata_rq = %d, want 1 (reassembled across the drain boundary)", got)
	}
}

// --- response frame builders (28.2; big-endian; 4-byte length prefix EXCLUDES itself) ---

// stdRespFrame builds a standard response frame: xid(4) + zxid(8) + error(4)
// (SPEC §3.3 rows 3/4 framing).
func stdRespFrame(xid int32, zxid int64, errCode int32) []byte {
	return zkFrame(be32(xid), be64(zxid), be32(errCode))
}

// connectRespFrame builds a connect response: proto_version(4=0) + timeout(4) +
// session_id(8) + password(4-byte len + bytes) — NO zxid, NO error (SPEC §3.3
// row 1). The leading proto_version=0 doubles as the sniffed connectXid.
// Used at Task 4.
func connectRespFrame(pwLen int) []byte {
	return zkFrame(be32(0), be32(30000), be64(0x1234), be32(int32(pwLen)), make([]byte, pwLen))
}

// watchEventFrame builds a server-initiated watch event with the full ReplyHeader
// (upstream decoder.cc decodeOnWrite + parseWatchEvent — every non-connect response
// carries xid+zxid+error): xid(-1) + zxid(8) + error(4) + event_type(4) +
// client_state(4) + path(4-byte len + bytes).
// (SPEC §3.3 row 2; min length 28 per D-S28.2-1 upstream verification — the SPEC's
// 16-byte pin omitted zxid+error and was corrected to upstream's value at IMPL.)
func watchEventFrame(path string) []byte {
	return zkFrame(be32(watchXid), be64(0), be32(0), be32(1), be32(3), be32(int32(len(path))), []byte(path))
}

// feedRequest feeds one request frame through decodeOnData using the decoder's
// own high-water mark for the totalAppended bookkeeping (a per-call delta feed).
// Used at Task 4.
func feedRequest(d *decoder, frame []byte) {
	d.decodeOnData(frame, d.chainConsumed+int64(len(frame)))
}

// --- Task 3 (28.2): write-side reassembly + framing + uncorrelated dispatch ---

// A watch event (xid −1) increments watch_event + response_bytes and NOTHING
// else: never correlated, no per-opcode counter, no latency (SPEC §3.3 row 2).
func TestDecodeWatchEvent(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := watchEventFrame("/zk-test")
	d.decodeOnWrite(frame)
	if got := counterValue(t, rs, "watch_event"); got != 1 {
		t.Fatalf("watch_event = %d, want 1", got)
	}
	// wireFootprint = 4-byte prefix + payload; watchEventFrame returns the
	// PREFIXED frame, so the footprint equals len(frame).
	if got := counterValue(t, rs, "response_bytes"); got != uint64(len(frame)) {
		t.Fatalf("response_bytes = %d, want %d", got, len(frame))
	}
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0", got)
	}
}

// A short watch event (< 28 bytes payload) → decoder_error + abandon.
func TestDecodeWatchEventTooShort(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(zkFrame(be32(watchXid), be32(1))) // 8-byte payload < 28
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := counterValue(t, rs, "watch_event"); got != 0 {
		t.Fatalf("watch_event = %d, want 0", got)
	}

	// A frame that satisfies the SPEC's original (incorrect) 16-byte minimum but
	// not upstream's 28-byte ReplyHeader minimum → decoder_error (D-S28.2-1:
	// upstream's value is load-bearing). This frame is 24 bytes: xid(4) +
	// event_type(4) + client_state(4) + path-len(4) + path(8) = 24 — it would
	// have passed the old 16-byte minimum but must fail the corrected 28-byte one.
	d2, rs2, _ := newTestDecoder(t)
	d2.decodeOnWrite(zkFrame(be32(watchXid), be32(1), be32(3), be32(8), []byte("/zk-test")))
	if got := counterValue(t, rs2, "decoder_error"); got != 1 {
		t.Fatalf("24-byte watch frame: decoder_error = %d, want 1 (28-byte upstream minimum)", got)
	}
	if got := counterValue(t, rs2, "watch_event"); got != 0 {
		t.Fatalf("24-byte watch frame: watch_event = %d, want 0", got)
	}
}

// An unknown negative xid (not 0/−1/−2/−4/−8) → decoder_error + abandon
// (SPEC §3.3 row 5 — upstream unknown-xid onDecodeError parity).
func TestDecodeResponseUnknownNegativeXid(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(stdRespFrame(-3, 1, 0))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// A response frame shorter than the universal 4-byte minimum → decoder_error.
func TestDecodeResponseTooShortForXid(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(zkFrame([]byte{0x00, 0x01})) // 2-byte payload < 4
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// An oversized response frame (length prefix > max_packet_bytes) → decoder_error
// + abandon ("packet is too big" — parent §11.5 symmetry).
func TestDecodeResponseOversized(t *testing.T) {
	d, rs, cfg := newTestDecoder(t)
	huge := append(be32(int32(cfg.maxPacketBytes)+1), make([]byte, 16)...)
	d.decodeOnWrite(huge)
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if d.writeBuf != nil {
		t.Fatal("oversized frame must ABANDON writeBuf (no resync)")
	}
}

// Partial-frame reassembly: a watch event split across three decodeOnWrite calls
// decodes exactly once when complete (the writeBuf reassembly — SPEC §3.2 item 2).
func TestDecodeResponsePartialFrameReassembly(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	frame := watchEventFrame("/zk-test")
	d.decodeOnWrite(frame[:3])
	d.decodeOnWrite(frame[3:10])
	if got := counterValue(t, rs, "watch_event"); got != 0 {
		t.Fatalf("watch_event = %d before the frame is complete, want 0", got)
	}
	d.decodeOnWrite(frame[10:])
	if got := counterValue(t, rs, "watch_event"); got != 1 {
		t.Fatalf("watch_event = %d, want 1 (reassembled across 3 OnWrite calls)", got)
	}
}

// Abandon-no-resync recovery: after a decode failure abandons writeBuf, a LATER
// decodeOnWrite (a fresh socket write) decodes normally (AMEND-A8 symmetry —
// the 0046 arm-4 request-side analog).
func TestDecodeResponseAbandonThenRecover(t *testing.T) {
	d, rs, cfg := newTestDecoder(t)
	huge := append(be32(int32(cfg.maxPacketBytes)+1), make([]byte, 16)...)
	d.decodeOnWrite(huge) // decoder_error + abandon
	d.decodeOnWrite(watchEventFrame("/zk-test"))
	if got := counterValue(t, rs, "watch_event"); got != 1 {
		t.Fatalf("watch_event = %d, want 1 (the connection survives the abandon)", got)
	}
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (only the oversized frame)", got)
	}
}

// Multiple complete frames in ONE decodeOnWrite call all decode (the frames loop).
func TestDecodeResponseMultipleFramesOneWrite(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	two := append(watchEventFrame("/a"), watchEventFrame("/b")...)
	d.decodeOnWrite(two)
	if got := counterValue(t, rs, "watch_event"); got != 2 {
		t.Fatalf("watch_event = %d, want 2", got)
	}
}

// --- Task 4 (28.2): correlated dispatch + correlation consumption (§3.4) ---

// A data response correlates against requestsByXid, increments <opname>_resp +
// response_bytes, and ERASES the entry (erase-on-lookup — upstream parity).
func TestDecodeDataResponseCorrelates(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	resp := stdRespFrame(1, 100, 0)
	d.decodeOnWrite(resp)
	if got := counterValue(t, rs, "getdata_resp"); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1", got)
	}
	if got := counterValue(t, rs, "response_bytes"); got != uint64(len(resp)) {
		t.Fatalf("response_bytes = %d, want %d (wireFootprint)", got, len(resp))
	}
	if len(d.requestsByXid) != 0 {
		t.Fatal("erase-on-lookup: the entry must be ERASED by the correlation hit")
	}
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0", got)
	}
}

// A second response with the same data xid finds nothing → decoder_error
// (the erase-on-lookup consequence — SPEC §3.4 item 1).
func TestDecodeDataResponseDoubleResponse(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	d.decodeOnWrite(stdRespFrame(1, 100, 0))
	d.decodeOnWrite(stdRespFrame(1, 101, 0)) // same xid again
	if got := counterValue(t, rs, "getdata_resp"); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1", got)
	}
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (double response = missing xid)", got)
	}
}

// A data response whose xid has no pending request → decoder_error (upstream
// InvalidArgumentError parity — SPEC §3.3 row 4).
func TestDecodeDataResponseMissingXid(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(stdRespFrame(42, 100, 0))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
}

// Control responses FIFO-pop the per-xid queue: two pings answered in order;
// a third ping response with an empty queue → decoder_error (SPEC §3.4 item 2).
func TestDecodeControlResponseFIFOAndUnderflow(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
	feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
	d.decodeOnWrite(stdRespFrame(pingXid, 1, 0))
	d.decodeOnWrite(stdRespFrame(pingXid, 2, 0))
	if got := counterValue(t, rs, "ping_resp"); got != 2 {
		t.Fatalf("ping_resp = %d, want 2", got)
	}
	if len(d.controlRequestsByXid[pingXid]) != 0 {
		t.Fatal("FIFO pop must drain the control queue")
	}
	d.decodeOnWrite(stdRespFrame(pingXid, 3, 0)) // empty queue
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1 (empty control queue)", got)
	}
}

// A connect response (leading int32 == 0) uses the special framing and pops the
// connect control queue → connect_resp (SPEC §3.3 row 1).
func TestDecodeConnectResponse(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, connectFrame(nil))
	d.decodeOnWrite(connectRespFrame(16))
	if got := counterValue(t, rs, "connect_resp"); got != 1 {
		t.Fatalf("connect_resp = %d, want 1", got)
	}
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0", got)
	}
}

// THE §3.4-ITEM-4 PANIC TRAP: a READONLY connect request's queue entry carries
// opname "connect_readonly", and respOpNames has NO connect_readonly_resp — a
// naive inc(entry.opname + "_resp") PANICS on the closed roster. The response
// decoder must count connect_resp (upstream onConnectResponse parity).
func TestDecodeConnectReadonlyResponseMapsToConnect(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	ro := true
	feedRequest(d, connectFrame(&ro)) // entry opname = "connect_readonly"
	// Must NOT panic; must count connect_resp.
	d.decodeOnWrite(connectRespFrame(16))
	if got := counterValue(t, rs, "connect_resp"); got != 1 {
		t.Fatalf("connect_resp = %d, want 1 (the connect_readonly→connect mapping)", got)
	}
	if got := counterValue(t, rs, "decoder_error"); got != 0 {
		t.Fatalf("decoder_error = %d, want 0", got)
	}
}

// A connect response with NO pending connect request → decoder_error.
func TestDecodeConnectResponseEmptyQueue(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	d.decodeOnWrite(connectRespFrame(0))
	if got := counterValue(t, rs, "decoder_error"); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := counterValue(t, rs, "connect_resp"); got != 0 {
		t.Fatalf("connect_resp = %d, want 0", got)
	}
}

// Correlate-then-validate (PLAN refinement 2 — upstream parity): a TRUNCATED
// data response with a valid, correlatable xid consumes the entry AND fires the
// flag-gated per-opcode decoder error (the opname IS known from the correlation
// hit — SPEC §3.3 decode-failure clause).
func TestDecodeDataResponseTruncatedAfterCorrelation(t *testing.T) {
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
		StatPrefix:                         "zk",
		EnablePerOpcodeDecoderErrorMetrics: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rs := newRosterStats(reg, "zk")
	d := newDecoder(cfg, rs)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	// 12-byte payload: xid(4) + 8 more — short of the 16-byte xid+zxid+error minimum.
	d.decodeOnWrite(zkFrame(be32(1), be64(100)))
	if got := rs.counters["decoder_error"].Load(); got != 1 {
		t.Fatalf("decoder_error = %d, want 1", got)
	}
	if got := rs.counters["getdata_decoder_error"].Load(); got != 1 {
		t.Fatalf("getdata_decoder_error = %d, want 1 (flag-gated, opname from the correlation hit)", got)
	}
	if len(d.requestsByXid) != 0 {
		t.Fatal("correlate-then-validate: the entry is consumed even on a truncated frame")
	}
	if got := rs.counters["getdata_resp"].Load(); got != 0 {
		t.Fatalf("getdata_resp = %d, want 0", got)
	}
}

// Byte accounting flag-gating (SPEC §3.3): response_bytes is ALWAYS counted
// (ungated); <opname>_resp_bytes is counted ONLY when
// enable_per_opcode_response_bytes_metrics is true.
func TestDecodeResponseBytesFlagGating(t *testing.T) {
	// Flag OFF (the newTestDecoder default):
	d, rs, _ := newTestDecoder(t)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	resp := stdRespFrame(1, 100, 0)
	d.decodeOnWrite(resp)
	if got := counterValue(t, rs, "getdata_resp_bytes"); got != 0 {
		t.Fatalf("flag OFF: getdata_resp_bytes = %d, want 0", got)
	}
	if got := counterValue(t, rs, "response_bytes"); got != uint64(len(resp)) {
		t.Fatalf("flag OFF: response_bytes = %d, want %d (ungated)", got, len(resp))
	}

	// Flag ON:
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
		StatPrefix:                          "zk2",
		EnablePerOpcodeResponseBytesMetrics: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rs2 := newRosterStats(reg, "zk2")
	d2 := newDecoder(cfg, rs2)
	feedRequest(d2, dataFrame(1, opGetData, padTo(opGetData)))
	d2.decodeOnWrite(resp)
	if got := rs2.counters["getdata_resp_bytes"].Load(); got != uint64(len(resp)) {
		t.Fatalf("flag ON: getdata_resp_bytes = %d, want %d", got, len(resp))
	}
}

// Control responses for auth and setwatches use their roster opnames
// (auth_resp / setwatches_resp both exist in respOpNames).
func TestDecodeControlResponseAuthAndSetwatches(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	// auth request (xid -4, scheme "digest"):
	authReqFrame := zkFrame(be32(authXid), be32(opSetAuth), be32(0), be32(6), []byte("digest"), be32(0))
	feedRequest(d, authReqFrame)
	// setwatches request (xid -8):
	feedRequest(d, zkFrame(be32(setWatchesXid), be32(opSetWatches)))
	d.decodeOnWrite(stdRespFrame(authXid, 1, 0))
	d.decodeOnWrite(stdRespFrame(setWatchesXid, 2, 0))
	if got := counterValue(t, rs, "auth_resp"); got != 1 {
		t.Fatalf("auth_resp = %d, want 1", got)
	}
	if got := counterValue(t, rs, "setwatches_resp"); got != 1 {
		t.Fatalf("setwatches_resp = %d, want 1", got)
	}
}

// The 28.1 "correlation maps grow unbounded" boundary CLOSES: responses drain
// both structures (SPEC §3.4 item 5).
func TestDecodeResponsesDrainCorrelationStructures(t *testing.T) {
	d, _, _ := newTestDecoder(t)
	feedRequest(d, connectFrame(nil))
	feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	feedRequest(d, dataFrame(2, opSetData, padTo(opSetData)))
	d.decodeOnWrite(connectRespFrame(0))
	d.decodeOnWrite(stdRespFrame(pingXid, 1, 0))
	d.decodeOnWrite(stdRespFrame(1, 2, 0))
	d.decodeOnWrite(stdRespFrame(2, 3, 0))
	if len(d.requestsByXid) != 0 {
		t.Fatalf("requestsByXid has %d entries after all responses, want 0", len(d.requestsByXid))
	}
	for xid, q := range d.controlRequestsByXid {
		if len(q) != 0 {
			t.Fatalf("controlRequestsByXid[%d] has %d entries, want 0", xid, len(q))
		}
	}
}

// --- Task 5 (28.2): latency-threshold counters (§4) ---

// latencyTestDecoder builds a decoder with enable_latency_threshold_metrics +
// optional overrides. defaultThreshold uses the proto Duration field.
func latencyTestDecoder(t *testing.T, defaultThreshold time.Duration,
	overrides []*zookeeper_proxyv3.LatencyThresholdOverride) (*decoder, *rosterStats) {
	t.Helper()
	reg := stats.NewRegistry()
	cfg, err := parseConfig(&zookeeper_proxyv3.ZooKeeperProxy{
		StatPrefix:                    "zk",
		EnableLatencyThresholdMetrics: true,
		DefaultLatencyThreshold:       durationpb.New(defaultThreshold),
		LatencyThresholdOverrides:     overrides,
	})
	if err != nil {
		t.Fatal(err)
	}
	rs := newRosterStats(reg, "zk")
	return newDecoder(cfg, rs), rs
}

// The inclusive edge (AMEND-A10 / parent §11.7): latency == threshold → FAST.
// recordLatency takes the measured latency as a parameter (PLAN refinement 3),
// so the boundary is tested with exact injected durations.
func TestRecordLatencyInclusiveEdge(t *testing.T) {
	d, rs := latencyTestDecoder(t, 100*time.Millisecond, nil)
	d.recordLatency("getdata", opGetData, 100*time.Millisecond) // == threshold
	if got := counterValue(t, rs, "getdata_resp_fast"); got != 1 {
		t.Fatalf("getdata_resp_fast = %d, want 1 (latency == threshold is FAST — inclusive)", got)
	}
	if got := counterValue(t, rs, "getdata_resp_slow"); got != 0 {
		t.Fatalf("getdata_resp_slow = %d, want 0", got)
	}
	d.recordLatency("getdata", opGetData, 100*time.Millisecond+time.Nanosecond) // > threshold
	if got := counterValue(t, rs, "getdata_resp_slow"); got != 1 {
		t.Fatalf("getdata_resp_slow = %d, want 1 (latency > threshold is SLOW)", got)
	}
}

// A wire-opcode-keyed override beats the default (§4.1 item 3).
func TestRecordLatencyOverrideBeatsDefault(t *testing.T) {
	d, rs := latencyTestDecoder(t, time.Millisecond, []*zookeeper_proxyv3.LatencyThresholdOverride{
		{Opcode: zookeeper_proxyv3.LatencyThresholdOverride_GetData, Threshold: durationpb.New(time.Hour)},
	})
	// 10 ms latency: getdata (override 1 h) → fast; setdata (default 1 ms) → slow.
	d.recordLatency("getdata", opGetData, 10*time.Millisecond)
	d.recordLatency("setdata", opSetData, 10*time.Millisecond)
	if got := counterValue(t, rs, "getdata_resp_fast"); got != 1 {
		t.Fatalf("getdata_resp_fast = %d, want 1 (the override wins)", got)
	}
	if got := counterValue(t, rs, "setdata_resp_slow"); got != 1 {
		t.Fatalf("setdata_resp_slow = %d, want 1 (no override → default)", got)
	}
}

// The flag gates INCREMENTS (AMEND-A2): flag off → neither fast nor slow moves.
func TestRecordLatencyFlagOff(t *testing.T) {
	d, rs, _ := newTestDecoder(t) // enable_latency_threshold_metrics defaults false
	d.recordLatency("getdata", opGetData, time.Nanosecond)
	if counterValue(t, rs, "getdata_resp_fast") != 0 || counterValue(t, rs, "getdata_resp_slow") != 0 {
		t.Fatal("flag off: neither fast nor slow may increment")
	}
}

// End-to-end injected-timestamp test: a pending request whose start is in the
// deep past → response decode → SLOW (the time.Since plumbing — §4.1).
func TestLatencyEndToEndInjectedStart(t *testing.T) {
	d, rs := latencyTestDecoder(t, 100*time.Millisecond, nil)
	feedRequest(d, dataFrame(1, opGetData, padTo(opGetData)))
	// Inject: back-date the pending entry far past any threshold.
	d.mu.Lock()
	e := d.requestsByXid[1]
	e.start = time.Now().Add(-time.Hour)
	d.requestsByXid[1] = e
	d.mu.Unlock()
	d.decodeOnWrite(stdRespFrame(1, 100, 0))
	if got := counterValue(t, rs, "getdata_resp_slow"); got != 1 {
		t.Fatalf("getdata_resp_slow = %d, want 1 (1 h latency >> 100 ms threshold)", got)
	}
	if got := counterValue(t, rs, "getdata_resp"); got != 1 {
		t.Fatalf("getdata_resp = %d, want 1 (the _resp counter increments alongside fast/slow)", got)
	}
}

// Connect responses participate in latency with opname "connect" + wire opcode
// opConnect (here via the default threshold; override-vs-default precedence is
// pinned by TestRecordLatencyOverrideBeatsDefault).
func TestLatencyConnectResponse(t *testing.T) {
	d, rs := latencyTestDecoder(t, time.Hour, nil)
	feedRequest(d, connectFrame(nil))
	d.decodeOnWrite(connectRespFrame(0))
	if got := counterValue(t, rs, "connect_resp_fast"); got != 1 {
		t.Fatalf("connect_resp_fast = %d, want 1 (1 h threshold → fast)", got)
	}
}

// Control responses (ping) participate in latency end-to-end — pins the
// onControlResponse → recordLatency wiring (a deleted call site would not be
// caught by the _resp-only assertions in TestDecodeControlResponseFIFOAndUnderflow).
func TestLatencyControlResponse(t *testing.T) {
	d, rs := latencyTestDecoder(t, time.Hour, nil)
	feedRequest(d, zkFrame(be32(pingXid), be32(opPing)))
	d.decodeOnWrite(stdRespFrame(pingXid, 1, 0))
	if got := counterValue(t, rs, "ping_resp_fast"); got != 1 {
		t.Fatalf("ping_resp_fast = %d, want 1 (1 h threshold → fast)", got)
	}
}

// Watch events NEVER get fast/slow (uncorrelated — no request timestamp; §4.1
// item 4); decoder_error responses never get fast/slow (§4.1 item 5).
func TestLatencyNeverForWatchEventsOrErrors(t *testing.T) {
	d, rs := latencyTestDecoder(t, time.Hour, nil)
	d.decodeOnWrite(watchEventFrame("/zk-test"))
	d.decodeOnWrite(stdRespFrame(42, 1, 0)) // missing xid → decoder_error
	suffixes := []string{"_resp_fast", "_resp_slow"}
	for _, op := range []string{"getdata", "connect", "exists"} {
		for _, s := range suffixes {
			if got := counterValue(t, rs, op+s); got != 0 {
				t.Fatalf("%s%s = %d, want 0", op, s, got)
			}
		}
	}
}

// --- Task 6 (28.2): the §3.6 concurrent request/response race test (R9) ---

// TestDecoderConcurrentRequestResponseRace drives goroutine A (request decode —
// the replayRead → OnData path) and goroutine B (response decode — the
// writeChainConn.Write → OnWrite path) CONCURRENTLY over one decoder. This is
// the production goroutine topology post-handoff (tcpproxy filter.go:134-138).
// The assertion is `go test -race` itself (the §3.6 mutex makes the correlation
// maps race-free) plus a conservation check: every response either correlated
// or counted decoder_error. Run with -race -count=5 (the zookeeperproxy-package
// analog of the 28.1b framework-level concurrent-pumps test).
//
// Conservation soundness: each decodeOnWrite call here delivers exactly ONE
// complete frame to a writeBuf that is empty on entry (each call is
// self-contained). If the response arrives before its request is recorded,
// takeData returns ok=false → responseError → decoder_error +1 AND writeBuf is
// set to nil. But the NEXT call starts by appending to nil (effectively an empty
// slice — append(nil, p...) is valid Go), so the abandon does NOT lose the
// subsequent frame. Thus: getdata_resp + decoder_error == n exactly.
func TestDecoderConcurrentRequestResponseRace(t *testing.T) {
	d, rs, _ := newTestDecoder(t)
	const n = 200
	var wg sync.WaitGroup
	wg.Add(2)
	// Goroutine A: request decode (WRITES the correlation maps).
	// Uses the feedRequest delta-feed pattern (d.chainConsumed is owned by
	// goroutine A — not shared — so no lock is needed here; mu only guards the
	// map writes inside onDataRequest).
	go func() {
		defer wg.Done()
		for i := 1; i <= n; i++ {
			frame := dataFrame(int32(i), opGetData, padTo(opGetData))
			feedRequest(d, frame)
		}
	}()
	// Goroutine B: response decode (READS + ERASES the correlation maps via
	// takeData under mu). Each call is self-contained: writeBuf is nil between
	// calls, so a prior abandon does not affect this call's frame.
	go func() {
		defer wg.Done()
		for i := 1; i <= n; i++ {
			d.decodeOnWrite(stdRespFrame(int32(i), int64(i), 0))
		}
	}()
	wg.Wait()

	// Conservation: every response was either correlated (getdata_resp) or
	// arrived before its request was recorded (decoder_error). No response
	// is lost, none double-counted.
	resp := counterValue(t, rs, "getdata_resp")
	errs := counterValue(t, rs, "decoder_error")
	if resp+errs != n {
		t.Fatalf("getdata_resp(%d) + decoder_error(%d) = %d, want %d (conservation)", resp, errs, resp+errs, n)
	}
}
