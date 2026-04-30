package accesslog

import (
	"bufio"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

func newTestRegistryAndCounter(t *testing.T) (*stats.Registry, *stats.Counter) {
	t.Helper()
	reg := stats.NewRegistry()
	c := reg.NewCounter("test.dropped")
	return reg, c
}

func TestAsyncFileSink_HappyPath_NRecordsLandNLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subject.log")
	_, c := newTestRegistryAndCounter(t)
	s, err := NewAsyncFileSink(path, c)
	if err != nil {
		t.Fatalf("NewAsyncFileSink: %v", err)
	}
	for i := 0; i < 5; i++ {
		s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x",
			Protocol: "HTTP/1.1", ResponseCode: 200, BytesSent: 3})
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	if count != 5 {
		t.Errorf("file has %d lines, want 5", count)
	}
	if c.Load() != 0 {
		t.Errorf("dropped counter = %d, want 0", c.Load())
	}
}

func TestAsyncFileSink_ConcurrentSubmit_RaceClean(t *testing.T) {
	dir := t.TempDir()
	_, c := newTestRegistryAndCounter(t)
	s, err := NewAsyncFileSink(filepath.Join(dir, "subject.log"), c)
	if err != nil {
		t.Fatalf("NewAsyncFileSink: %v", err)
	}
	const G, N = 8, 100
	var wg sync.WaitGroup
	for i := 0; i < G; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < N; j++ {
				s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x",
					Protocol: "HTTP/1.1", ResponseCode: 200})
			}
		}()
	}
	wg.Wait()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestAsyncFileSink_DropNewest_FullChannelIncrementsCounter(t *testing.T) {
	dir := t.TempDir()
	_, c := newTestRegistryAndCounter(t)
	s, err := newAsyncFileSinkWithCapacity(filepath.Join(dir, "subject.log"), c, 1)
	if err != nil {
		t.Fatalf("NewAsyncFileSink: %v", err)
	}
	rec := &Record{StartTime: time.Now(), Method: "GET", Path: "/x", Protocol: "HTTP/1.1", ResponseCode: 200}
	for i := 0; i < 100; i++ {
		s.Submit(rec)
	}
	_ = s.Close()
	if c.Load() == 0 {
		t.Errorf("expected at least one drop; counter = 0")
	}
}

func TestAsyncFileSink_Close_Idempotent(t *testing.T) {
	dir := t.TempDir()
	_, c := newTestRegistryAndCounter(t)
	s, err := NewAsyncFileSink(filepath.Join(dir, "x.log"), c)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestAsyncFileSink_Close_DrainsPending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.log")
	_, c := newTestRegistryAndCounter(t)
	s, err := NewAsyncFileSink(path, c)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x", Protocol: "HTTP/1.1", ResponseCode: 200})
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(path)
	if stat.Size() == 0 {
		t.Errorf("file empty after Close; expected drained records")
	}
}
