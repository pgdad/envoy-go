// Package sdsfile unit tests. Covers SPEC §14.1 Group 7 (12-15 tests) +
// SPEC §12 item B7 (fsnotify event-debounce window precise behavior) +
// SPEC §14.2 race-test surface `TestWatcher_DebounceRace_*` per planner-time
// D4 (3-5 race tests).
//
// Per ADR-0178 §Decision the Watcher's contract is:
//   - New(path) reads the file once + initializes the atomic.Pointer + creates
//     the fsnotify backend; does NOT auto-start the event goroutine
//   - Start() spawns the goroutine reading fsnotify Events + Errors + done
//   - Current() returns the latest bytes (atomic load; concurrent-read-safe)
//   - Close() is idempotent (sync.Once) + waits for the goroutine
//   - ~100ms debounce window collapses rapid writes (latest-bytes-wins)
//   - fsWatcher watches the PARENT DIRECTORY to survive atomic-rename inode
//     swaps; events filter by basename match against w.path
//
// Test groups:
//
//  1. Construction + initial-load + nonexistent-path (3 tests)
//  2. Event observation: in-place rewrite + atomic-rename (2 tests)
//  3. Debounce window + collapse semantics (2 tests)
//  4. Close: idempotent + during-reload + no-Start (3 tests)
//  5. Race-group `TestWatcher_DebounceRace_*` per D4 (3 tests)
//
// All race-group tests are also race-clean under `go test -race`.
package sdsfile

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeFile writes data atomically by writing to a sibling temp file + rename
// (Linux: atomic if same filesystem). Mirrors the atomic-rename-via-mv pattern
// SDS rotators commonly use.
func writeAtomicRename(t *testing.T, path string, data []byte) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename: %v", err)
	}
}

// writeInPlace writes data via os.WriteFile in-place (truncate + rewrite).
// fsnotify reports this as fsnotify.Write events on the file path.
func writeInPlace(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write in-place: %v", err)
	}
}

// waitForCurrent polls w.Current() every 10ms until it equals want, fatally
// failing after a generous deadline. Condition-based waiting replaces the
// former fixed `time.Sleep(debounceSettle)`-then-assert pattern for the
// event-observation tests: under CPU contention (2-core CI runners running
// the full -short suite) fsnotify event delivery + the ~100ms debounce timer
// + the reload goroutine can collectively take well over 250ms, which made
// the fixed-sleep assertions flaky. The deadline is intentionally generous —
// the test normally completes in ~100-150ms; the deadline only bounds the
// failure case.
func waitForCurrent(t *testing.T, w *Watcher, want []byte) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if bytes.Equal(w.Current(), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("Current() = %q, want %q (reload not observed within 10s)", w.Current(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// newWatcherStarted constructs + Start()s a Watcher pointing at path. Caller
// is responsible for t.Cleanup(w.Close).
func newWatcherStarted(t *testing.T, path string) *Watcher {
	t.Helper()
	w, err := New(path)
	if err != nil {
		t.Fatalf("New(%q): %v", path, err)
	}
	if err := w.Start(); err != nil {
		_ = w.Close()
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// 1. Construction + initial-load + nonexistent-path

// TestWatcher_New_LoadsInitialBytes verifies that New(path) reads the file
// once at construction time and Current() returns those bytes immediately
// without requiring Start().
func TestWatcher_New_LoadsInitialBytes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	want := []byte("hello-world-bytes-v0")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	w, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	got := w.Current()
	if !bytes.Equal(got, want) {
		t.Fatalf("Current() = %q, want %q", got, want)
	}
}

// TestWatcher_New_NonexistentPath_Errors verifies that New(path) returns an
// error when the file does not exist. The error propagates the os.ReadFile
// failure; the Watcher is NOT constructed.
func TestWatcher_New_NonexistentPath_Errors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.bin")
	w, err := New(path)
	if err == nil {
		_ = w.Close()
		t.Fatalf("New(%q) succeeded; want error", path)
	}
	if w != nil {
		t.Fatalf("New returned non-nil Watcher on error: %v", w)
	}
}

// TestWatcher_New_RelativePath_OK verifies that relative paths work — the
// Watcher resolves the path via os.ReadFile + filepath.Dir for the parent
// directory watch. We swap cwd manually + restore on cleanup (rather than
// t.Chdir which requires Go 1.24; the module is on go 1.23 per go.mod).
func TestWatcher_New_RelativePath_OK(t *testing.T) {
	// Not parallel: process-wide cwd mutation.
	dir := t.TempDir()
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(origCwd) })

	path := "relative.bin"
	want := []byte("relative-bytes")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	w, err := New(path)
	if err != nil {
		t.Fatalf("New(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if got := w.Current(); !bytes.Equal(got, want) {
		t.Fatalf("Current() = %q, want %q", got, want)
	}
}

// 2. Event observation: in-place rewrite + atomic-rename

// TestWatcher_Start_ObservesInPlaceTruncate verifies that an in-place
// os.WriteFile (truncate + rewrite) triggers a reload after the debounce
// window. The watcher watches the parent directory; fsnotify reports the
// rewrite as a Write event on the file path.
func TestWatcher_Start_ObservesInPlaceTruncate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("initial"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := newWatcherStarted(t, path)
	if got := w.Current(); !bytes.Equal(got, []byte("initial")) {
		t.Fatalf("pre-write Current() = %q, want %q", got, "initial")
	}

	writeInPlace(t, path, []byte("rewritten"))
	waitForCurrent(t, w, []byte("rewritten"))
}

// TestWatcher_Start_ObservesAtomicRename verifies that an atomic-rename-via-mv
// (write-sibling-then-rename — the common SDS-rotator pattern) triggers a
// reload after the debounce window. The atomic rename replaces the file's
// inode; watching the parent directory (rather than the file path) survives
// this — fsnotify reports a Create + Rename event sequence; the basename
// match against w.path filters both into a reload.
func TestWatcher_Start_ObservesAtomicRename(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := newWatcherStarted(t, path)
	if got := w.Current(); !bytes.Equal(got, []byte("v0")) {
		t.Fatalf("pre-rename Current() = %q, want %q", got, "v0")
	}

	writeAtomicRename(t, path, []byte("v1"))
	waitForCurrent(t, w, []byte("v1"))
}

// 3. Debounce window + collapse semantics

// TestWatcher_Debounce_CollapsesRapidWrites verifies the ~100ms debounce
// window per SPEC §12 item B7: 5 back-to-back in-place writes within a window
// shorter than the debounce timer collapse to a SINGLE reload; Current()
// returns the LAST written bytes (latest-bytes-wins).
func TestWatcher_Debounce_CollapsesRapidWrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := newWatcherStarted(t, path)

	// Reset the reload counter so we measure only the writes we trigger here
	// (the seed write happens BEFORE Start so it does not fire an event).
	atomic.StoreInt64(&w.reloadCount, 0)

	// Write 5 versions back-to-back. Each interval is well under the ~100ms
	// debounce so all 5 events should collapse into a single reload firing
	// after the last write's debounce expiry.
	for i := 1; i <= 5; i++ {
		writeInPlace(t, path, []byte{byte('v'), byte('0' + i)})
		time.Sleep(10 * time.Millisecond)
	}
	// Latest-bytes-wins: Current() returns the last write (v5).
	// Condition-based wait (not a fixed debounceSettle sleep): under CPU
	// contention the debounce timer + reload goroutine can take well over
	// 250ms after the last write. reloadCount is incremented before the
	// bytes swap, so once v5 is visible the collapsed reload is counted.
	waitForCurrent(t, w, []byte("v5"))
	// Single reload: 5 events collapse to 1 reload via the debounce window.
	// We allow up to 2 in case a stray pre-batch event slipped through (the
	// debounce window is ~100ms; a write that lands precisely at the edge
	// can fire a separate reload). The substantive invariant is "MANY fewer
	// reloads than writes", which is what the debounce promises.
	if reloads := atomic.LoadInt64(&w.reloadCount); reloads < 1 || reloads > 2 {
		t.Fatalf("reloads = %d, want 1 (or up to 2 for boundary races); writes=5", reloads)
	}
}

// TestWatcher_Debounce_SequentialWritesEachReload verifies the inverse: when
// writes are SEPARATED by more than the debounce window, each fires its own
// reload. This pins the "fires-after-quiescence" behavior of the debounce.
func TestWatcher_Debounce_SequentialWritesEachReload(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := newWatcherStarted(t, path)
	atomic.StoreInt64(&w.reloadCount, 0)

	// 3 writes, each separated by a full debounce quiescence interval.
	// Condition-based separation: wait until the PREVIOUS write's reload has
	// landed (Current() observes it) before issuing the next write. This
	// guarantees each write's fsnotify event arrives after the prior debounce
	// timer fired (a distinct quiescence interval ⇒ a distinct reload), and
	// replaces the former fixed debounceSettle sleeps, which under CPU
	// contention both under-waited the final assertion (reload still in
	// flight ⇒ "Current() = v2, want v3" flake) and could coalesce delayed
	// events into one window.
	writeInPlace(t, path, []byte("v1"))
	waitForCurrent(t, w, []byte("v1"))
	writeInPlace(t, path, []byte("v2"))
	waitForCurrent(t, w, []byte("v2"))
	writeInPlace(t, path, []byte("v3"))
	waitForCurrent(t, w, []byte("v3"))
	// 3 separated writes => 3 reloads (allow up to 4 for boundary fsnotify
	// dedup that some platforms perform; the substantive invariant is "one
	// reload per quiescence interval").
	if reloads := atomic.LoadInt64(&w.reloadCount); reloads < 3 || reloads > 4 {
		t.Fatalf("reloads = %d, want 3 (sequential writes each fire their own reload)", reloads)
	}
}

// 4. Close: idempotent + during-reload + no-Start

// TestWatcher_Close_Idempotent verifies that Close() is safe to call multiple
// times (sync.Once guards the channel close + the underlying fsnotify.Close).
func TestWatcher_Close_Idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// First Close: returns nil + tears down the goroutine.
	if err := w.Close(); err != nil {
		t.Fatalf("Close#1: %v", err)
	}
	// Second Close: must not panic; must return nil (sync.Once short-circuit).
	if err := w.Close(); err != nil {
		t.Fatalf("Close#2: %v", err)
	}
	// Third Close for paranoia: same.
	if err := w.Close(); err != nil {
		t.Fatalf("Close#3: %v", err)
	}
}

// TestWatcher_Close_WithoutStart verifies that Close() works on a Watcher
// that was New()'d but never Start()ed. The underlying fsnotify.Watcher must
// still close cleanly + the sync.WaitGroup must not block (no goroutine
// was ever spawned).
func TestWatcher_Close_WithoutStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestWatcher_Close_DuringInFlightReload_NoPanic verifies that Close() called
// while a debounce-triggered reload is in flight (or pending) terminates
// cleanly without panic. The done channel + sync.Once + sync.WaitGroup
// coordinate the teardown.
func TestWatcher_Close_DuringInFlightReload_NoPanic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Trigger an event; immediately Close before the debounce timer fires.
	writeInPlace(t, path, []byte("rewritten"))
	// Very short wait: enough for fsnotify to deliver the event + arm the
	// timer, but NOT enough for the debounce timer to fire.
	time.Sleep(20 * time.Millisecond)
	if err := w.Close(); err != nil {
		t.Fatalf("Close mid-debounce: %v", err)
	}
}

// TestWatcher_Close_RacesStart_RaceClean verifies that Start() and Close()
// invoked from different goroutines with no external synchronization are
// race-clean: the started flag is an atomic.Bool (a plain bool here was
// flagged by -race — the documented filter-lifecycle ownership permits
// construction/Start and teardown to happen on different goroutines). If
// Close wins the race, Start's run() goroutine observes the closed done
// channel (or the closed fsnotify Events channel) and exits promptly.
func TestWatcher_Close_RacesStart_RaceClean(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = w.Start()
	}()
	go func() {
		defer wg.Done()
		_ = w.Close()
	}()
	wg.Wait()
	// Idempotent teardown regardless of which goroutine won.
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// 5. Race-group `TestWatcher_DebounceRace_*` per planner-time D4

// TestWatcher_DebounceRace_ConcurrentCurrent verifies that N goroutines
// concurrently calling Current() across the boundary of a reload observe
// only consistent byte slices (no torn read; atomic.Pointer guarantees the
// reader sees either the pre-reload slice or the post-reload slice in full).
//
// Race-clean under -race per planner-time D4 + SPEC §14.2.
func TestWatcher_DebounceRace_ConcurrentCurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	preBytes := bytes.Repeat([]byte("A"), 256)
	postBytes := bytes.Repeat([]byte("B"), 256)
	if err := os.WriteFile(path, preBytes, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := newWatcherStarted(t, path)

	const readers = 16
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(readers)
	var inconsistentReads atomic.Int64
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				cur := w.Current()
				// Consistency check: all bytes must be either all 'A' or all
				// 'B' (NEVER a mix). A mix would indicate a torn read.
				if len(cur) == 0 {
					continue
				}
				first := cur[0]
				if first != 'A' && first != 'B' {
					inconsistentReads.Add(1)
					continue
				}
				for _, b := range cur {
					if b != first {
						inconsistentReads.Add(1)
						break
					}
				}
			}
		}()
	}

	// Trigger reload while readers race. Condition-based wait (non-fatal so
	// the reader goroutines are always stopped + drained before any Fatalf):
	// poll until the reload lands instead of sleeping a fixed debounceSettle,
	// which under-waits when the 16 spinning readers starve the fsnotify +
	// debounce-timer goroutines on a contended 2-core runner. The post-reload
	// Current() assertion below reports the failure if the deadline expires.
	writeInPlace(t, path, postBytes)
	deadline := time.Now().Add(10 * time.Second)
	for !bytes.Equal(w.Current(), postBytes) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	wg.Wait()

	if inconsistentReads.Load() != 0 {
		t.Fatalf("torn reads observed: %d", inconsistentReads.Load())
	}
	if got := w.Current(); !bytes.Equal(got, postBytes) {
		t.Fatalf("post-reload Current() = %q, want all-B", got[:8])
	}
}

// TestWatcher_DebounceRace_ConcurrentWrites verifies that multiple concurrent
// writers + concurrent Current() readers run race-clean under -race. The
// atomic.Pointer swap + the debounce-mu protect the timer state; no data
// races should fire.
func TestWatcher_DebounceRace_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.bin")
	if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	w := newWatcherStarted(t, path)

	const writers = 4
	const readers = 8
	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(writers)
	for i := 0; i < writers; i++ {
		id := byte('0' + i)
		go func() {
			defer wg.Done()
			payload := bytes.Repeat([]byte{id}, 64)
			for {
				select {
				case <-stop:
					return
				default:
				}
				// Best-effort writes; fsnotify may coalesce.
				_ = os.WriteFile(path, payload, 0o600)
				time.Sleep(5 * time.Millisecond)
			}
		}()
	}
	wg.Add(readers)
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = w.Current()
			}
		}()
	}

	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()
	// Pure race-detector test: no assertions beyond "no panic + no race
	// detector firing". The -race build is the substantive gate.
}

// TestWatcher_DebounceRace_CloseRacesReload verifies that Close() racing a
// reload-in-flight terminates cleanly. The done channel + the goroutine's
// select-on-done preempt the debounce-fire path.
func TestWatcher_DebounceRace_CloseRacesReload(t *testing.T) {
	t.Parallel()
	// Run several iterations to bias the race window.
	for iter := 0; iter < 5; iter++ {
		dir := t.TempDir()
		path := filepath.Join(dir, "secret.bin")
		if err := os.WriteFile(path, []byte("v0"), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
		w, err := New(path)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if err := w.Start(); err != nil {
			_ = w.Close()
			t.Fatalf("Start: %v", err)
		}
		// Trigger a reload; immediately Close to race against the debounce
		// timer + the reload goroutine.
		writeInPlace(t, path, []byte("racy"))
		// Vary the close timing across iterations to hit different windows.
		time.Sleep(time.Duration(iter*10) * time.Millisecond)
		if err := w.Close(); err != nil {
			t.Fatalf("iter=%d Close: %v", iter, err)
		}
	}
}
