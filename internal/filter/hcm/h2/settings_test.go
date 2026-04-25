package h2

import (
	"net"
	"testing"

	"golang.org/x/net/http2"
)

func TestServerSettings_DefaultsMatchADR0047(t *testing.T) {
	s := DefaultServerSettings
	if s.MaxConcurrentStreams != 100 {
		t.Errorf("MaxConcurrentStreams = %d, want 100", s.MaxConcurrentStreams)
	}
	if s.InitialWindowSize != 65535 {
		t.Errorf("InitialWindowSize = %d, want 65535", s.InitialWindowSize)
	}
	if s.MaxFrameSize != 16384 {
		t.Errorf("MaxFrameSize = %d, want 16384", s.MaxFrameSize)
	}
	if s.EnablePush != 0 {
		t.Errorf("EnablePush = %d, want 0", s.EnablePush)
	}
	if s.HeaderTableSize != 4096 {
		t.Errorf("HeaderTableSize = %d, want 4096", s.HeaderTableSize)
	}
}

func TestSettings_RoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	srvF := newFramer(c1)
	cliF := newFramer(c2)

	go func() {
		_ = writeServerInitialSettings(srvF, DefaultServerSettings)
	}()
	frame, err := cliF.ReadFrame()
	if err != nil {
		t.Fatalf("ReadFrame = %v, want nil", err)
	}
	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		t.Fatalf("got %T, want *SettingsFrame", frame)
	}
	v, _ := sf.Value(http2.SettingMaxConcurrentStreams)
	if v != 100 {
		t.Errorf("peer-side MaxConcurrentStreams = %d, want 100", v)
	}
}

func TestReadClientSettings_AckOnFirstReadIsProtocolError(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	srvF := newFramer(c1)
	cliF := newFramer(c2)
	go func() {
		_ = cliF.WriteSettingsAck()
	}()
	var cs clientSettings
	err := readClientSettings(srvF, &cs)
	if err == nil {
		t.Fatal("readClientSettings(ACK first) returned nil; want PROTOCOL_ERROR")
	}
}
