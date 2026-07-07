// Package sdsfile is the envoy-go framework-internal filesystem-path SDS
// Secret reader per ADR-0178. It exposes a *Watcher type that reads a file's
// bytes once at construction time + watches the file via fsnotify for
// changes + atomically swaps a *[]byte snapshot under a ~100ms debounce
// window so concurrent readers observe consistent bytes across reloads.
//
// The package is the SECOND envoy-go top-level filesystem-watching primitive
// authored at phase 20 (alongside `internal/httpclient/` per ADR-0177).
// ADR-0178 records the cross-phase-reuse intent at introduction time: MVP
// consumer is oauth2 (hmac_secret + client_secret + AES-key-derivation-source
// per phase-20 Task 11); cross-phase-reusable for any future filesystem-SDS
// consumer (anticipated future jwt_authn TLS-trust-store reload, future
// ext_authz mTLS, future ratelimit gRPC TLS).
//
// # Design points
//
//  1. The Watcher does NOT interpret the file bytes. The JSON/YAML/proto
//     interpretation of the outer Secret-proto envelope is the CONSUMER's
//     responsibility (e.g., the oauth2 compiledConfig parser unmarshals the
//     bytes returned by Current() into the SDS Secret proto + extracts the
//     generic_secret.inline_string field). The sdsfile primitive is wire-
//     agnostic. See SPEC §3.2.
//
//  2. The fsnotify backend watches the PARENT DIRECTORY of the file, not the
//     file path itself. SDS rotators commonly use the atomic-rename-via-mv
//     pattern (write sibling tmpfile + os.Rename onto the watched path) — the
//     rename replaces the inode; a watch on the file path becomes stale
//     because the underlying inode is gone. Watching the parent directory
//     observes both the in-place-truncate-and-rewrite event sequence
//     (fsnotify.Write on the file path) AND the atomic-rename event sequence
//     (fsnotify.Create + fsnotify.Rename); the dispatcher filters events to
//     the watched basename and dispatches reloads accordingly.
//
//  3. ~100ms debounce window collapses rapid writes into a single reload per
//     SPEC §12 item B7 RATIFIED-PENDING-IMPL-TIME (closed at phase-20 IMPL
//     Task 3). The debounce timer is reset on every observed event; the
//     reload fires only after ~100ms of event quiescence. Latest-bytes-wins
//     semantics: a burst of 5 writes within the debounce window collapses to
//     1 reload returning the LAST written bytes.
//
//  4. atomic.Pointer[[]byte] swap discipline guarantees concurrent readers
//     observe consistent bytes across the reload boundary. The pointer is
//     stored + loaded atomically; the underlying slice is never mutated
//     (each reload allocates a fresh []byte via os.ReadFile). Race-clean
//     under `go test -race` per planner-time D4 + SPEC §14.2.
//
//  5. PARSE-REJECT discipline lives at the CONSUMER. The inner
//     generic_secret.secret_file indirect-arm + the non-filesystem
//     core.ConfigSource oneof arms (ApiConfigSource + Ads) + the deprecated
//     core.ConfigSource.path field 1 all PARSE-REJECT at the CONSUMER's
//     compiledConfig parser (per phase-20 SPEC §2.11 + §8 item 14 +
//     planner-time D2 byte-stable error messages). The sdsfile primitive
//     itself takes a filesystem path string and reads it verbatim — it has
//     no proto awareness.
//
//  6. Lifecycle ownership per planner-time D13: each *Watcher is owned by
//     the consumer's compiledConfig (typically two per filter — one per SDS
//     Secret field — for the oauth2 MVP). Close() runs at filter teardown;
//     MVP leaks-on-exit discipline applies (no os.Exit cleanup hook).
//
//  7. Reload error policy: when os.ReadFile fails (file removed, permission
//     change, transient I/O error), we log via the stdlib log package and
//     PRESERVE the last-good bytes in atomic.Pointer (the reader continues
//     to observe the previous snapshot until the next successful reload).
//     This matches operator expectations for "rotation hiccup" — never
//     observe garbage; the rotator's next valid write triggers a reload that
//     supersedes the stale snapshot.
package sdsfile

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounceWindow is the ~100ms quiescence window per SPEC §12 item B7. Set as
// a package variable (not const) so the race-test surface can override it for
// deterministic debounce-collapse assertions if needed in the future; the
// production value is fixed at 100ms.
var debounceWindow = 100 * time.Millisecond

// Watcher tracks an outer filesystem-path SDS Secret file via fsnotify with
// ~100ms debounced reload + atomic.Pointer[[]byte] snapshot swap. Construct
// via New(path); call Start() to spawn the event-loop goroutine; call
// Current() for the latest bytes; call Close() for clean teardown.
//
// Goroutine-safe per the atomic.Pointer + sync.Once + sync.Mutex discipline.
// Concurrent Current() readers + concurrent Close() callers are race-clean
// under `go test -race`.
type Watcher struct {
	// path is the watched file's absolute-or-relative filesystem path. Stored
	// verbatim from New(); used for os.ReadFile + basename matching against
	// fsnotify events delivered on the parent directory.
	path string

	// dir is the parent directory of path, derived once at New() via
	// filepath.Dir. fsnotify watches the directory; events filter by basename
	// match against w.path's basename.
	dir string

	// base is the basename of path, derived once at New() via filepath.Base.
	// Cached to avoid per-event basename derivation in the hot event loop.
	base string

	// bytes is the latest in-memory snapshot of the file contents. Loaded via
	// atomic.Pointer.Load by Current(); stored via atomic.Pointer.Store by
	// reload(). The underlying slice is never mutated; each reload allocates
	// a fresh []byte via os.ReadFile.
	bytes atomic.Pointer[[]byte]

	// fsWatcher is the fsnotify backend. Created at New(); add-watched on the
	// parent directory at New(); closed at Close().
	fsWatcher *fsnotify.Watcher

	// mu protects the timer field. The fsnotify event handler stops + resets
	// the timer under the mutex; the reload callback runs without holding mu
	// (the timer's AfterFunc-scheduled goroutine).
	mu sync.Mutex

	// timer is the debounce timer. nil until the first event arrives; reset
	// on every subsequent event within the debounce window. The AfterFunc
	// callback invokes reload() asynchronously.
	timer *time.Timer

	// done is the close-signal channel. Closed once by Close() via closeOnce;
	// the event-loop goroutine selects on done to terminate cleanly.
	done chan struct{}

	// closeOnce guards the done close + fsWatcher.Close + final teardown so
	// that Close() is idempotent (safe to call multiple times).
	closeOnce sync.Once

	// wg waits for the event-loop goroutine to exit so Close() returns only
	// after teardown completes. Add(1) at Start; Done() at the goroutine exit.
	wg sync.WaitGroup

	// started records whether Start() was called. Close() must short-circuit
	// the wg.Wait when Start was never called (no goroutine was spawned).
	// atomic.Bool because the documented filter-lifecycle ownership permits
	// Start and Close to run on different goroutines (race-clean per D4).
	started atomic.Bool

	// reloadCount is incremented by reload() on every successful or attempted
	// reload. Used by the debounce-collapse unit test to assert the
	// "5 rapid writes -> 1 reload" invariant. Read via atomic.LoadInt64 in
	// tests; written via atomic.AddInt64 inside reload().
	reloadCount int64
}

// New constructs a Watcher pointing at path. The file MUST exist + be
// readable; on error the function returns nil + the underlying error.
//
// New reads the file once + initializes the atomic.Pointer with the initial
// bytes so Current() returns the seeded snapshot before Start() is called.
//
// New also creates the *fsnotify.Watcher backend + add-watches the file's
// parent directory. The event-loop goroutine is NOT spawned until Start() is
// called; this two-phase construction lets a consumer construct multiple
// Watchers atomically (e.g., during compiledConfig parse) and Start() them
// only after parse succeeds, simplifying error-cleanup paths.
//
// Per ADR-0178 §Decision the Watcher watches the PARENT DIRECTORY of path,
// NOT path itself, to survive atomic-rename-via-mv inode swaps (the common
// SDS rotation pattern). Events are filtered by basename match against
// filepath.Base(path) in the event-loop.
func New(path string) (*Watcher, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if dir == "" {
		// filepath.Dir returns "." for paths with no directory part — we
		// pass that through to fsnotify, which interprets "." as cwd.
		dir = "."
	}
	if err := fsw.Add(dir); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	w := &Watcher{
		path:      path,
		dir:       dir,
		base:      filepath.Base(path),
		fsWatcher: fsw,
		done:      make(chan struct{}),
	}
	w.bytes.Store(&data)
	return w, nil
}

// Start spawns the event-loop goroutine that reads from the fsnotify event
// stream + dispatches debounced reloads. Returns nil unconditionally for the
// current implementation; reserved as error-returning to match the surface
// settled at SPEC §3.2 (a future implementation may pre-flight validate the
// watch state, e.g., re-Add a previously-removed directory).
//
// Start MUST be called at most once. Calling Start twice on the same Watcher
// will produce undefined behavior (the second goroutine racing the first on
// the same fsnotify event stream). The consumer is expected to call Start
// exactly once per Watcher lifecycle.
func (w *Watcher) Start() error {
	// wg.Add BEFORE the atomic started flip: Close only calls wg.Wait after
	// observing started==true, and the atomic Store/Load pair then orders the
	// first Add happens-before the Wait (the reverse order let a Close racing
	// Start enter Wait before the first Add — the WaitGroup-misuse data race
	// the detector deliberately reports).
	w.wg.Add(1)
	w.started.Store(true)
	go w.run()
	return nil
}

// Current returns the latest in-memory snapshot of the file bytes via an
// atomic pointer load. The returned slice MUST be treated as read-only by
// the caller; the Watcher reuses the slice across all readers between
// reloads (atomic.Pointer swap replaces the pointer wholesale on each
// reload; the prior slice remains valid for any reader that loaded it
// before the swap).
//
// Goroutine-safe per the atomic.Pointer.Load discipline; readers see either
// the pre-reload slice or the post-reload slice in full (never a torn read).
//
// If the file was never successfully read (e.g., New returned an error so
// no Watcher was constructed), Current is not callable; this function
// assumes New succeeded and so the atomic.Pointer was seeded.
func (w *Watcher) Current() []byte {
	p := w.bytes.Load()
	if p == nil {
		// Defensive: should be unreachable since New always seeds. Return
		// nil rather than panic so callers can handle the corner case.
		return nil
	}
	return *p
}

// Close stops the event-loop goroutine + closes the fsnotify backend + waits
// for the goroutine to exit. Idempotent (safe to call multiple times via
// sync.Once). Returns the first non-nil fsnotify.Close error if any, nil
// otherwise.
//
// Per ADR-0178 §Decision the Close discipline guarantees: (a) no goroutine
// leak after Close returns; (b) no panic if Close races a debounce-triggered
// reload (the done channel + the goroutine's select-on-done preempt the
// reload path); (c) idempotent multi-call safety via sync.Once.
//
// MVP leaks-on-exit lifecycle per planner-time D13: typical consumer pattern
// is to call Close at filter teardown (or never call it for the
// boot-singleton case — process exit reclaims resources).
func (w *Watcher) Close() error {
	var closeErr error
	w.closeOnce.Do(func() {
		close(w.done)
		// Cancel any pending debounce timer to prevent a late reload firing
		// after the fsWatcher is closed (which would log a spurious
		// missing-file error if the rotator deleted the file just before
		// teardown).
		w.mu.Lock()
		if w.timer != nil {
			w.timer.Stop()
			w.timer = nil
		}
		w.mu.Unlock()
		closeErr = w.fsWatcher.Close()
		if w.started.Load() {
			w.wg.Wait()
		}
	})
	return closeErr
}

// run is the event-loop goroutine spawned by Start. It selects on the
// fsnotify Events + Errors channels + the done close-signal channel,
// dispatching debounced reloads on file-change events that match the watched
// basename.
//
// Exit conditions:
//   - done channel closed via Close: clean exit; sets wg.Done.
//   - fsnotify Events channel closed (which fsnotify.Close triggers): clean
//     exit; sets wg.Done.
func (w *Watcher) run() {
	defer w.wg.Done()
	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			// Filter to events on our basename. fsnotify delivers events for
			// every file in the watched directory; we only care about path.
			if filepath.Base(ev.Name) != w.base {
				continue
			}
			// Subset of fsnotify ops that warrant a reload:
			//   - Write: in-place truncate-and-rewrite (os.WriteFile pattern)
			//   - Create: atomic rename target appears (mv tmpfile path)
			//   - Rename: rotator's mv-old-out path (a sibling rename onto path)
			// Chmod / Remove are NOT reload triggers (permission change without
			// content change; removal triggers the "next valid write" reload).
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			w.scheduleReload()
		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			// fsnotify Errors are typically transient (event-buffer overflow,
			// platform-specific notifier errors). Log + continue; the next
			// valid event triggers a reload.
			log.Printf("sdsfile: fsnotify error on %s: %v", w.path, err)
		}
	}
}

// scheduleReload arms (or resets) the debounce timer under the mutex. When
// the timer fires it invokes reload via time.AfterFunc's goroutine; the timer
// is set to nil INSIDE reload via the mutex to permit a subsequent event to
// re-arm the timer.
//
// Per ADR-0178 §Decision the debounce window is ~100ms (debounceWindow
// package var). Calling scheduleReload while a timer is already pending
// resets the deadline — the reload fires only after ~100ms of event
// quiescence. This is the "latest-bytes-wins" + "collapse rapid writes"
// invariant per SPEC §12 item B7.
func (w *Watcher) scheduleReload() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer == nil {
		w.timer = time.AfterFunc(debounceWindow, w.reload)
		return
	}
	// Reset returns false if the timer had already fired or been stopped.
	// Either way, we want the next reload to fire ~debounceWindow from now,
	// so we Stop + AfterFunc to guarantee the schedule (Reset on an
	// already-fired timer is racy per the time.Timer doc).
	w.timer.Stop()
	w.timer = time.AfterFunc(debounceWindow, w.reload)
}

// reload is the debounce-timer callback. Re-reads the file via os.ReadFile +
// atomically swaps the bytes pointer. On read error preserves the last-good
// bytes + logs the error (the operator observes the stale-bytes warning;
// readers continue to see the prior snapshot until the next successful
// reload).
//
// Runs on the time.AfterFunc-scheduled goroutine (NOT the event-loop
// goroutine). Clears w.timer under the mutex so a subsequent event re-arms
// the debounce window. Honors w.done close: if Close races the reload, we
// short-circuit before the os.ReadFile to avoid a spurious missing-file
// error (the rotator may have removed the file just before teardown).
func (w *Watcher) reload() {
	// Clear the timer slot so the next event re-arms the debounce window.
	w.mu.Lock()
	w.timer = nil
	w.mu.Unlock()

	// Honor close-in-flight: bail before the os.ReadFile to avoid spurious
	// error log lines during teardown.
	select {
	case <-w.done:
		return
	default:
	}

	atomic.AddInt64(&w.reloadCount, 1)
	data, err := os.ReadFile(w.path)
	if err != nil {
		// Distinguish a "file does not exist (yet)" transient (rotator may
		// be mid-rename; the next event will re-trigger a reload) from a
		// permanent error. Both paths preserve the last-good bytes.
		if errors.Is(err, os.ErrNotExist) {
			log.Printf("sdsfile: %s missing during reload (rotator mid-rename?); preserving last-good bytes", w.path)
		} else {
			log.Printf("sdsfile: reload of %s failed: %v; preserving last-good bytes", w.path, err)
		}
		return
	}
	w.bytes.Store(&data)
}
