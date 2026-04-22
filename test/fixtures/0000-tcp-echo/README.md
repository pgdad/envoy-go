# 0000-tcp-echo

The trivial fixture that proves the differential harness works end-to-end.

## What it tests

- Both proxies (upstream Envoy reference, envoy-go subject) terminate a TCP
  connection and pump bytes bidirectionally to the same backend.
- For a deterministic echo backend, both proxies' response byte streams are
  byte-exact.

## Configs

- `envoy.yaml` — real Envoy bootstrap with one listener (port `15000` in-container),
  one cluster (the in-host echo backend, address resolved at runtime by the
  reference container's gateway), and a `tcp_proxy` network filter routing
  listener → cluster.
- `envoy-go.yaml` — sample envoy-go-minimal config; the runner generates the
  effective config at test time (host-port substitutions).

## Driver

`driver/driver.go` opens a TCP connection to each proxy's listener, sends
ten payloads `ping-N-<uuid>\n` for N ∈ {0..9}, half-closes write, reads to
EOF or 1s idle, returns the concatenated response stream.

## Expectations

`expectations.yaml` enumerates every §7.2 dimension. Only `response-body` is
applicable; the rest are not-applicable with one-line reasons (no HTTP, no
filter chain, no stats subsystem in the subject yet).
