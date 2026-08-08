// Command probe-84.1 is the phase-84 (grpc-unary-response-trailers) PLAN-84.1
// Task 1 RED anchor probe. It is NOT a differential fixture (0119 is 84.2) —
// it is a standalone runnable program, in its OWN Go module so `go build
// ./...` and `go test ./...` from the repo root never pick it up.
//
// Shape (PLAN-84.1 §1.2): an in-process grpc-go health server (h2c, the
// verbatim serveGRPCHealth shape from test/differential/runner_test.go:3105)
// on 127.0.0.1:45810; an envoy-go subject with a TLS + alpn_protocols
// ["h2","http/1.1"] + codec_type AUTO listener on 127.0.0.1:45801 (config.yaml,
// the 0079-h2-multiplex-pool PKI reused from pki/); an upstream cluster
// c_grpc with explicit_http_config.http2_protocol_options{} pointing at the
// health server; a grpc-go client dialing the subject with
// credentials.NewTLS(RootCAs=ca.pem, ServerName="localhost").
//
// Three arms, each printed verbatim so the RED can be recorded exactly:
//
//  1. success unary (grpc_health_v1.Health/Check) x3 — at the unpatched tip
//     this is expected to fail because writeH2Reply never forwards the
//     upstream trailing HEADERS block, so grpc-go's client sees the stream
//     end without a grpc-status trailer.
//  2. error unary to /a1.NoSuchService/NoSuchMethod x3 — the reference/tip
//     behavior is a silently-degraded codes.Unknown with an EMPTY message
//     (grpc-go infers Unknown from a 200 status with no grpc-status),
//     WORSE than an explicit Unimplemented.
//  3. plain-H2 GET over the same TLS listener — the invariance control; must
//     be byte-stable regardless of what arms 1/2 show.
//
// Usage:
//
//	go run . -bin /path/to/envoy-go -config ./config.yaml
package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

var (
	subjectBin  = flag.String("bin", "", "path to the compiled envoy-go subject binary (required)")
	configPath  = flag.String("config", "config.yaml", "path to the subject bootstrap config")
	caPath      = flag.String("ca", "pki/ca.pem", "path to the CA PEM trusted by the probe's TLS clients")
	healthAddr  = flag.String("health-addr", "127.0.0.1:45810", "grpc-go health server listen addr (h2c)")
	subjectAddr = flag.String("subject-addr", "127.0.0.1:45801", "envoy-go subject TLS listener addr")
	readyTO     = flag.Duration("ready-timeout", 10*time.Second, "how long to wait for the subject's ready sentinel")
)

func main() {
	flag.Parse()
	if *subjectBin == "" {
		log.Fatal("probe-84.1: -bin is required (path to the compiled envoy-go subject binary)")
	}

	ln, err := net.Listen("tcp", *healthAddr)
	if err != nil {
		log.Fatalf("probe-84.1: listen health server on %s: %v", *healthAddr, err)
	}
	go serveGRPCHealth(ln, 0)
	fmt.Printf("probe-84.1: grpc-go health server listening on %s (h2c)\n", *healthAddr)

	// run() owns the subject child process for its entire lifetime and its
	// `defer killSubject(cmd)` runs on EVERY return path, including error
	// returns — unlike a bare log.Fatalf after cmd.Start(), which calls
	// os.Exit and skips all pending defers, orphaning the child holding
	// 45801/45802. main() logs and exits with a single log.Fatalf AFTER
	// run() has already returned (so its defers have already fired).
	if err := run(); err != nil {
		log.Fatalf("probe-84.1: %v", err)
	}
}

// run execs the subject binary, waits for its ready sentinel, drives the
// three RED-anchor arms, and unconditionally kills the subject child before
// returning — success or failure.
func run() error {
	cmd := exec.Command(*subjectBin, "-c", *configPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("subject stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start subject %s: %w", *subjectBin, err)
	}
	defer killSubject(cmd)

	if err := waitForReady(stdout, *readyTO); err != nil {
		return fmt.Errorf("subject did not become ready: %w", err)
	}
	fmt.Printf("probe-84.1: subject ready on %s\n", *subjectAddr)

	pool, err := loadCAPool(*caPath)
	if err != nil {
		return err
	}

	fmt.Println("=== ARM 1: success unary health check (grpc_health_v1.Health/Check), 3x ===")
	for i := 1; i <= 3; i++ {
		fmt.Printf("ARM1[%d]: %s\n", i, runHealthCheck(*subjectAddr, pool))
	}

	fmt.Println("=== ARM 2: error unary to /a1.NoSuchService/NoSuchMethod, 3x ===")
	for i := 1; i <= 3; i++ {
		fmt.Printf("ARM2[%d]: %s\n", i, runUnknownMethod(*subjectAddr, pool))
	}

	fmt.Println("=== ARM 3: plain-H2 GET over TLS (invariance control) ===")
	status, body, gerr := runPlainH2Get(*subjectAddr, pool)
	if gerr != nil {
		fmt.Printf("ARM3: transport error: %v\n", gerr)
	} else {
		fmt.Printf("ARM3: status=%d body=%q\n", status, body)
	}
	return nil
}

// killSubject terminates the COMPILED subject child process (not a `go run`
// parent — the binary itself) and waits for it to exit so no listener is left
// bound when the probe returns.
func killSubject(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
}

// waitForReady scans the subject's stdout for the "envoy-go ready" terminal
// sentinel (cmd/envoy-go/main.go:396), echoing each line it sees. Reading
// continues in the background afterward so the child's stdout pipe never
// fills and blocks it.
func waitForReady(r io.Reader, timeout time.Duration) error {
	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				return fmt.Errorf("subject stdout closed before the ready sentinel appeared")
			}
			fmt.Println("subject: " + line)
			if strings.Contains(line, "envoy-go ready") {
				go func() {
					for range lines {
					}
				}()
				return nil
			}
		case <-deadline:
			return fmt.Errorf("timed out after %s waiting for \"envoy-go ready\"", timeout)
		}
	}
}

// loadCAPool returns an error rather than calling log.Fatalf directly: it is
// invoked from run(), AFTER the subject child is already started, and a
// log.Fatalf here would os.Exit before run()'s deferred killSubject ran,
// orphaning the child.
func loadCAPool(path string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CA %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("parse CA PEM %s", path)
	}
	return pool, nil
}

func tlsCreds(pool *x509.CertPool) credentials.TransportCredentials {
	return credentials.NewTLS(&tls.Config{
		RootCAs:    pool,
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	})
}

// runHealthCheck dials the subject with grpc-go and issues one
// grpc_health_v1.Health/Check RPC, returning "SERVING" on success or the
// error's exact String() on failure (the RED anchor's success-unary arm).
func runHealthCheck(addr string, pool *x509.CertPool) string {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(tlsCreds(pool)))
	if err != nil {
		return fmt.Sprintf("dial error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		return err.Error()
	}
	return resp.GetStatus().String()
}

// runUnknownMethod dials the subject with grpc-go and invokes a method on a
// service the health server never registered, returning the error's exact
// String() (the RED anchor's error-unary arm).
func runUnknownMethod(addr string, pool *x509.CertPool) string {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(tlsCreds(pool)))
	if err != nil {
		return fmt.Sprintf("dial error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := &healthpb.HealthCheckRequest{}
	resp := &healthpb.HealthCheckResponse{}
	err = conn.Invoke(ctx, "/a1.NoSuchService/NoSuchMethod", req, resp)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("unexpected success: %v", resp)
}

// runPlainH2Get issues one plain HTTP/2-over-TLS GET against the SAME subject
// listener (the invariance control — must be byte-stable regardless of what
// the gRPC arms show).
func runPlainH2Get(addr string, pool *x509.CertPool) (int, string, error) {
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			ServerName: "localhost",
			NextProtos: []string{"h2"},
			MinVersion: tls.VersionTLS12,
		},
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	resp, err := client.Get("https://" + addr + "/plain")
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "", err
	}
	return resp.StatusCode, string(body), nil
}

// serveGRPCHealth is the verbatim shape of test/differential/runner_test.go:3105
// serveGRPCHealth — a grpc-go health server multiplexed with a plain-HTTP
// fallback (application/grpc requests go to the gRPC server; everything else
// gets a "backend-<idx>:<path>" 200 body), served h2c (prior-knowledge, no
// TLS) so envoy-go's H2 upstream cluster dials it in cleartext.
func serveGRPCHealth(ln net.Listener, idx int) {
	gs := grpc.NewServer()
	hs := health.NewServer()
	hs.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(gs, hs)
	mux := func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			gs.ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "backend-%d:%s", idx, r.URL.Path)
	}
	srv := &http.Server{Handler: h2c.NewHandler(http.HandlerFunc(mux), &http2.Server{})}
	_ = srv.Serve(ln)
}
