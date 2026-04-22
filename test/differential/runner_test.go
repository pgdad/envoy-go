package differential

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	_ "github.com/esalaine/envoy-go/test/fixtures/0000-tcp-echo/driver"
	"github.com/esalaine/envoy-go/test/helpers"
)

// TestDifferential is the differential suite entry point. It discovers
// fixture directories under test/fixtures/, runs each as a subtest, and fails
// the suite if any fixture's diff verdict is not Equal.
func TestDifferential(t *testing.T) {
	if testing.Short() {
		t.Skip("differential suite; skipped under -short")
	}
	ensureDocker(t)

	root := repoRoot(t)
	fixtures := discoverFixtures(t, filepath.Join(root, "test", "fixtures"))
	pin := loadPinFromRepo(t)

	for _, fx := range fixtures {
		fx := fx
		t.Run(fx, func(t *testing.T) {
			driver, ok := fixture.DriverRegistry[fx]
			if !ok {
				t.Fatalf("no driver registered for fixture %q (did its driver package get imported?)", fx)
			}
			runFixture(t, root, pin, fx, driver)
		})
	}
}

func runFixture(t *testing.T, root string, pin *EnvoyPin, _ string, d FixtureDriver) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. Backend echo on a random port. Bind to 0.0.0.0 so the reference
	// Envoy container can reach it via host.docker.internal (which resolves
	// to the Docker Desktop gateway IP, not 127.0.0.1).
	backend, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go acceptEcho(backend)
	backendPort := backend.Addr().(*net.TCPAddr).Port

	// 2. Reference proxy.
	bootstrap := strings.Replace(d.ReferenceBootstrap(), "port_value: 0", fmt.Sprintf("port_value: %d", backendPort), 1)
	ref, err := StartReferenceProxy(ctx, pin, bootstrap, d.ReferenceListenerPort())
	if err != nil {
		t.Fatalf("ref start: %v", err)
	}
	defer func() { _ = ref.Stop(context.Background()) }()
	refAddr := ref.ListenerAddr(d.ReferenceListenerPort())

	// 3. Subject proxy.
	subjPort := freeTCPPort(t)
	subjAdminPort := freeTCPPort(t)
	subjCfg := d.SubjectConfig(d.ReferenceListenerPort(), subjPort, backendPort, subjAdminPort)
	subj, err := StartSubjectProxy(ctx, root, subjCfg, fmt.Sprintf("127.0.0.1:%d", subjAdminPort))
	if err != nil {
		t.Fatalf("subj start: %v", err)
	}
	defer func() { _ = subj.Stop() }()

	// 4. Drive both, diff, report.
	refBytes, subjBytes, err := d.Drive(ctx, refAddr, subj.ListenerAddr())
	if err != nil {
		t.Fatalf("drive: %v", err)
	}
	v, err := CompareBytes(refBytes, subjBytes)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !v.Equal {
		t.Errorf("differential mismatch:\n%s", v.HexDump)
	}

	// 5. Admin /ready observation (phase 01 addition — SPEC §5.6).
	refAdm, subjAdm, err := d.ProbeAdmin(ctx, ref.AdminAddr(), subj.AdminAddr())
	if err != nil {
		t.Fatalf("admin probe: %v", err)
	}
	vAdm, err := compareAdminResponses(refAdm, subjAdm, d)
	if err != nil {
		t.Fatalf("admin compare: %v", err)
	}
	if !vAdm.Equal {
		t.Errorf("admin differential mismatch:\n%s", vAdm.HexDump)
	}
}

func compareAdminResponses(refRaw, subjRaw []byte, _ fixture.Driver) (Verdict, error) {
	refResp, err := helpers.ParseHTTPResponse(refRaw)
	if err != nil {
		return Verdict{}, fmt.Errorf("ref parse: %w", err)
	}
	subjResp, err := helpers.ParseHTTPResponse(subjRaw)
	if err != nil {
		return Verdict{}, fmt.Errorf("subj parse: %w", err)
	}
	// Status line: exact.
	if refResp.StatusLine != subjResp.StatusLine {
		return Verdict{Equal: false, HexDump: fmt.Sprintf("status: ref=%q subj=%q", refResp.StatusLine, subjResp.StatusLine)}, nil
	}
	// Body: byte-exact.
	bv, err := CompareBytes(refResp.Body, subjResp.Body)
	if err != nil {
		return Verdict{}, err
	}
	if !bv.Equal {
		return bv, nil
	}
	// Headers: set-equal modulo allow-list.
	// Per BEHAVIOR_CONTRACT.md §Admin API — /ready, Task 7 evidence:
	// - Date: value non-deterministic (always present on both, value allow-listed)
	// - Content-Length / Transfer-Encoding: framing deviation (subject: Content-Length:5;
	//   upstream: Transfer-Encoding:chunked). Both are dropped from the set-equal check.
	allowList := map[string]struct{}{
		"Date":              {},
		"Content-Length":    {},
		"Transfer-Encoding": {},
	}
	mismatch := diffHeaders(refResp.Headers, subjResp.Headers, allowList)
	if mismatch != "" {
		return Verdict{Equal: false, HexDump: mismatch}, nil
	}
	return Verdict{Equal: true}, nil
}

func diffHeaders(ref, subj map[string]string, allow map[string]struct{}) string {
	// For each header in ref: if not in allow, require subj has it with equal value.
	var sb strings.Builder
	for k, v := range ref {
		if _, a := allow[k]; a {
			continue
		}
		sv, ok := subj[k]
		if !ok {
			fmt.Fprintf(&sb, "header %q: absent in subj (ref=%q)\n", k, v)
			continue
		}
		if sv != v {
			fmt.Fprintf(&sb, "header %q: ref=%q subj=%q\n", k, v, sv)
		}
	}
	// Reverse: headers in subj but not ref (outside allow-list).
	for k, v := range subj {
		if _, a := allow[k]; a {
			continue
		}
		if _, ok := ref[k]; !ok {
			fmt.Fprintf(&sb, "header %q: absent in ref (subj=%q)\n", k, v)
		}
	}
	return sb.String()
}

func discoverFixtures(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		// No fixtures yet is a valid intermediate state (e.g. between Task 12
		// landing the runner skeleton and Task 13 landing the first fixture).
		return nil
	}
	if err != nil {
		t.Fatalf("read fixtures dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Fixture names start with a 4-digit prefix (NNNN-name).
		if len(e.Name()) >= 5 && isNumeric(e.Name()[:4]) && e.Name()[4] == '-' {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func acceptEcho(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if n > 0 {
					_, _ = c.Write(buf[:n])
				}
				if err != nil {
					return
				}
			}
		}(c)
	}
}
