// Package extprocgrpc implements a minimal in-process scriptable
// envoy.service.ext_proc.v3.ExternalProcessor.Process bidi-stream gRPC server
// for differential fixtures whose driver needs to wire an ext_proc
// grpc_service endpoint into both envoy.yaml and envoy-go.yaml. Used by
// phase 19.1 fixture 0022-http-ext-proc-grpc. THE FIRST in-tree bidi-stream
// gRPC server in envoy-go's test tree (the phase-18.2
// test/helpers/extauthzgrpc is unary — extprocgrpc extends to bidi).
//
// Lifecycle: spawn-per-fixture; the runner allocates a free TCP port, starts
// the server via New(t), wires the EnvoyGrpc.cluster_name to a cluster
// pointing at that port in both yaml configs, runs the scenarios, then stops
// via Stop(). Plaintext h2c (no TLS); per-:path-discriminator scriptable
// per-stage ProcessingResponse sequence per planner-time decision D1.
//
// API surface:
//   - New(t testing.TB) *Server — bind a TCP listener on 127.0.0.1:0
//     (ephemeral port) and start the gRPC ExternalProcessorServer.
//     Registers t.Cleanup(Stop).
//   - NewAtAddr(addr) (*Server, error) — caller-chosen-port arm used by
//     differential drivers that need a stable bound address pre-baked into
//     bootstrap YAMLs.
//   - (*Server).Addr() string — the listener's bound address; load-bearing
//     because t supplies the ephemeral port at New time.
//   - (*Server).Script(discriminator string, responses ...*extprocv3.ProcessingResponse)
//     — register an ORDERED SEQUENCE of *ProcessingResponse values for the
//     discriminator (per planner-time D1: the discriminator is the `:path`
//     extracted from the FIRST ProcessingRequest received on the stream —
//     typically the request_headers stage with a specific path; the `:path`
//     is stable for the lifetime of the bidi-stream since one stream serves
//     one HTTP transaction).
//   - (*Server).Process(stream) error — implements
//     extprocv3.ExternalProcessorServer: Recv-loops the per-stage
//     ProcessingRequest from client; on first req extracts `:path`
//     discriminator from req.GetRequestHeaders().GetHeaders(); advances the
//     per-discriminator script counter; Sends next ProcessingResponse in the
//     sequence; if ImmediateResponse arm in script → Send + return (closes
//     stream); if script exhausted → status.Errorf(codes.Internal, ...);
//     if CloseSend from client → return nil (clean close).
//   - (*Server).Received(discriminator string) []*extprocv3.ProcessingRequest —
//     returns a COPY of all ProcessingRequest values received for the given
//     discriminator across all stream lifetimes (used by drivers for
//     post-run content-assertion against the SPEC §6.6 hypothesis-mapping
//     table — e.g. attribute envelope content).
//   - (*Server).Stop() — GracefulStop the *grpc.Server; idempotent via
//     sync.Once.
//
// Introduced by phase 19.1 Task 13 per planner-time decision D1 — mirrors
// test/helpers/extauthzgrpc/ structure with the bidi-stream Process method
// extension. Plaintext h2c only — no TLS — per SPEC §7.2 + parent SPEC §8
// item 17 (TLS-fronted processor-cluster fixture coverage DEFERRED).
package extprocgrpc
