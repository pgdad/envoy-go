package h2

import (
	"context"
	"net"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

func TestFramer_SettingsRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	f1 := newFramer(c1, 16384)
	f2 := newFramer(c2, 16384)

	go func() {
		_ = f1.WriteSettings(
			http2.Setting{ID: http2.SettingMaxConcurrentStreams, Val: 100},
			http2.Setting{ID: http2.SettingInitialWindowSize, Val: 65535},
		)
	}()

	frame, err := f2.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame = %v, want nil", err)
	}
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		t.Fatalf("got %T, want *SettingsFrame", frame)
	}
	if sf.NumSettings() != 2 {
		t.Errorf("NumSettings = %d, want 2", sf.NumSettings())
	}
	v, ok := sf.Value(http2.SettingMaxConcurrentStreams)
	if !ok || v != 100 {
		t.Errorf("MaxConcurrentStreams = (%d, %v), want (100, true)", v, ok)
	}
}

func TestFramer_PingRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	f1 := newFramer(c1, 16384)
	f2 := newFramer(c2, 16384)

	pingData := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}

	// f1 sends PING (no ACK); f2 reads it.
	go func() {
		_ = f1.WritePing(false, pingData)
	}()
	frame, err := f2.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame (ping) = %v, want nil", err)
	}
	pf, ok := frame.(*http2.PingFrame)
	if !ok {
		t.Fatalf("got %T, want *PingFrame", frame)
	}
	if pf.IsAck() {
		t.Errorf("PING should not have ACK flag set")
	}
	if pf.Data != pingData {
		t.Errorf("PING data = %v, want %v", pf.Data, pingData)
	}

	// f2 sends PING ACK; f1 reads it.
	go func() {
		_ = f2.WritePing(true, pingData)
	}()
	frame, err = f1.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame (ping ack) = %v, want nil", err)
	}
	pf, ok = frame.(*http2.PingFrame)
	if !ok {
		t.Fatalf("got %T, want *PingFrame", frame)
	}
	if !pf.IsAck() {
		t.Errorf("PING ACK should have ACK flag set")
	}
}

func TestFramer_HeadersRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	f1 := newFramer(c1, 16384)
	f2 := newFramer(c2, 16384)

	// 0x88 is a valid HPACK-encoded :status: 200 indexed header field.
	block := []byte{0x88}

	go func() {
		_ = f1.WriteHeaders(http2.HeadersFrameParam{
			StreamID:      1,
			BlockFragment: block,
			EndStream:     true,
			EndHeaders:    true,
		})
	}()

	frame, err := f2.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame (headers) = %v, want nil", err)
	}
	hf, ok := frame.(*http2.HeadersFrame)
	if !ok {
		t.Fatalf("got %T, want *HeadersFrame", frame)
	}
	if hf.StreamID != 1 {
		t.Errorf("StreamID = %d, want 1", hf.StreamID)
	}
	if string(hf.HeaderBlockFragment()) != string(block) {
		t.Errorf("BlockFragment = %v, want %v", hf.HeaderBlockFragment(), block)
	}
	if !hf.StreamEnded() {
		t.Errorf("EndStream flag not set")
	}
	if !hf.HeadersEnded() {
		t.Errorf("EndHeaders flag not set")
	}
}

func TestFramer_DataRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	f1 := newFramer(c1, 16384)
	f2 := newFramer(c2, 16384)

	// First DATA frame: endStream=false.
	// Write in goroutine (net.Pipe is unbuffered; writer must run concurrently
	// with the reader). Use a done channel so the next write cannot start until
	// this goroutine finishes, preventing data-races on the shared framer.
	done1 := make(chan struct{})
	go func() {
		_ = f1.WriteData(1, false, []byte("hello"))
		close(done1)
	}()
	frame, err := f2.ReadFrame()
	<-done1 // wait for the writer goroutine to complete before next write
	if err != nil {
		t.Fatalf("ReadFrame (data 1) = %v, want nil", err)
	}
	df, ok := frame.(*http2.DataFrame)
	if !ok {
		t.Fatalf("got %T, want *DataFrame", frame)
	}
	if string(df.Data()) != "hello" {
		t.Errorf("data = %q, want %q", df.Data(), "hello")
	}
	if df.StreamEnded() {
		t.Errorf("EndStream should be false on first frame")
	}

	// Second DATA frame: endStream=true.
	done2 := make(chan struct{})
	go func() {
		_ = f1.WriteData(1, true, []byte("world"))
		close(done2)
	}()
	frame, err = f2.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame (data 2) = %v, want nil", err)
	}
	df, ok = frame.(*http2.DataFrame)
	if !ok {
		t.Fatalf("got %T, want *DataFrame", frame)
	}
	if string(df.Data()) != "world" {
		t.Errorf("data = %q, want %q", df.Data(), "world")
	}
	<-done2 // wait for writer goroutine before test ends
	if !df.StreamEnded() {
		t.Errorf("EndStream should be true on second frame")
	}
}

func TestFramer_RSTStreamWindowUpdateGoAway(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	f1 := newFramer(c1, 16384)
	f2 := newFramer(c2, 16384)

	// Each write runs in its own goroutine (net.Pipe is unbuffered — writer must
	// run concurrently with reader). A done channel ensures the previous writer
	// goroutine finishes before the next one starts, preventing data-races on the
	// shared f1 framer state.

	// RST_STREAM
	doneRST := make(chan struct{})
	go func() {
		_ = f1.WriteRSTStream(1, http2.ErrCodeProtocol)
		close(doneRST)
	}()
	frame, err := f2.ReadFrame()
	<-doneRST
	if err != nil {
		t.Fatalf("ReadFrame (rst) = %v, want nil", err)
	}
	rf, ok := frame.(*http2.RSTStreamFrame)
	if !ok {
		t.Fatalf("got %T, want *RSTStreamFrame", frame)
	}
	if rf.StreamID != 1 {
		t.Errorf("RST StreamID = %d, want 1", rf.StreamID)
	}
	if rf.ErrCode != http2.ErrCodeProtocol {
		t.Errorf("RST ErrCode = %v, want ErrCodeProtocol", rf.ErrCode)
	}

	// WINDOW_UPDATE (connection-scope, streamID=0)
	doneWU := make(chan struct{})
	go func() {
		_ = f1.WriteWindowUpdate(0, 65535)
		close(doneWU)
	}()
	frame, err = f2.ReadFrame()
	<-doneWU
	if err != nil {
		t.Fatalf("ReadFrame (window_update) = %v, want nil", err)
	}
	wf, ok := frame.(*http2.WindowUpdateFrame)
	if !ok {
		t.Fatalf("got %T, want *WindowUpdateFrame", frame)
	}
	if wf.StreamID != 0 {
		t.Errorf("WINDOW_UPDATE StreamID = %d, want 0", wf.StreamID)
	}
	if wf.Increment != 65535 {
		t.Errorf("WINDOW_UPDATE Increment = %d, want 65535", wf.Increment)
	}

	// GOAWAY
	doneGA := make(chan struct{})
	go func() {
		_ = f1.WriteGoAway(0, http2.ErrCodeNo, nil)
		close(doneGA)
	}()
	frame, err = f2.ReadFrame()
	<-doneGA
	if err != nil {
		t.Fatalf("ReadFrame (goaway) = %v, want nil", err)
	}
	gf, ok := frame.(*http2.GoAwayFrame)
	if !ok {
		t.Fatalf("got %T, want *GoAwayFrame", frame)
	}
	if gf.LastStreamID != 0 {
		t.Errorf("GOAWAY LastStreamID = %d, want 0", gf.LastStreamID)
	}
	if gf.ErrCode != http2.ErrCodeNo {
		t.Errorf("GOAWAY ErrCode = %v, want ErrCodeNo", gf.ErrCode)
	}
}

func TestFramer_ReadFrameCtxCancel(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	f := newFramer(c1, 16384)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := f.readFrameCtx(ctx)
	if err == nil {
		t.Fatal("readFrameCtx returned nil; want ctx.Err()")
	}
	if ctx.Err() == nil {
		t.Errorf("ctx.Err() = nil after cancel; want non-nil")
	}
}
