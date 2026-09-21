# 0124-listener-sni-longest-suffix

Phase 99 (`chain-match-sni-longest-suffix`), SPEC §7. This fixture checks
which filter chain serves when **two or more chains' `server_names` patterns
match one SNI**.

## The proposition

Reference Envoy serves the chain whose **matching** pattern is the most
specific one. An exact name ranks first, then the **longest** matching `*.`
suffix. Declaration order does not matter. Any pattern the chain lists that
does *not* match the SNI does not count either. At the un-fixed tip, envoy-go
ranked each chain's **whole** pattern set (exact > suffix > universal):

- two wildcard chains that match one SNI tied, and envoy-go **closed** the
  connection (the TLS handshake got EOF);
- when a mixed set listed an exact name that does not match, envoy-go served
  that chain anyway.

## Topology

Four TLS listeners. Every listener has `listener_filters: [tls_inspector]`.
Each chain is an HCM with its own `stat_prefix` and a `direct_response` 200
whose body is `<listener>/<CHAIN>\n`. All chains share one leaf certificate
(`pki/`, SANs `*.foo.test`, `*.b.foo.test`, `*.c.b.foo.test`, `x.test`,
`nomatch.example`). Both sides deliver it through `inline_string:`.

| listener | ref port | chains (declared order) | SNI -> chain |
|---|---|---|---|
| `l_long_first` | 15124 | LONG `*.b.foo.test`, SHORT `*.foo.test` | a.b.foo.test -> LONG; x.foo.test -> SHORT |
| `l_short_first` | 15225 | SHORT, LONG | a.b.foo.test -> LONG; x.foo.test -> SHORT |
| `l_mixed` | 15226 | X `[x.test, *.foo.test]`, Y `[*.b.foo.test]` | a.b.foo.test -> Y; q.foo.test -> X; x.test -> X |
| `l_default` | 15227 | LONG, SHORT + `default_filter_chain` DEFAULT | a.b.foo.test -> LONG; nomatch.example -> DEFAULT |

The subject listener ports are `subjListenerPort + i` (i = 0..3), inside the
16-port block that `freeTCPPortBlock` probes. This is the 0123 derivation.

## Load-bearing design points

- **The reversal pair.** `l_long_first` and `l_short_first` differ only in
  declaration order. A subject that chooses by order is red on exactly one of
  them.
- **The P1-vs-P2 discriminator is `l_mixed` a.b.foo.test.** X's exact
  `x.test` does not match, so it must not help X. P1 ranks the whole set and
  only then compares matched length, so it serves X and is red **on this row
  only**. Delete `l_mixed` and the fixture cannot see P1.
- **The single-candidate rows are structurally green.** These are x.foo.test,
  q.foo.test, x.test and nomatch.example. Only one chain is eligible, so no
  tie-break runs, and they are green at the un-fixed tip. They prove that each
  losing chain is live. They are **not** evidence of precedence.
- **No row is a no-match close.** nomatch.example lands on `l_default`'s
  default chain. The reference books a TCP no-match close in
  `no_filter_chain_match`, and envoy-go does not emit that name, so a close
  row could only be pinned vacuously. `downstream_cx_total` is not pinned
  across sides.
- **`tls_inspector` is required.** Without it neither side reads the SNI
  before chain selection.
- The driver sets `ServerName` explicitly on every request and verifies the
  leaf against `pki/ca.pem`. All SNIs are lowercase.
- All nine `stat_prefix`es are distinct. A shared prefix panics envoy-go at
  boot with a duplicate metric registration.

## Assertions (driver `AssertStats`, per side, Errorf per property)

1. Per (listener, SNI): the connection was not closed, the status is 200, and
   the body is `<listener>/<want>\n`.
2. Per chain: the **value** of `http.<prefix>.downstream_rq_total` equals the
   number of rows that target that chain (0 for a chain no row targets). The
   value is scraped from `/stats/prometheus` as
   `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<prefix>"}`
   and re-projected. Both sides emit every name at boot (value 0), so the
   check pins values. Checking only that a name is present would be vacuous.
   It is still guarded by a presence check.
3. The runner's cross-side `CompareBytes` compares the per-row
   `listener/sni/status/body` stream. A close is recorded as the fixed token
   `<CLOSED>`, never as the error text.

## Measured (prototype, phase-99 PLAN)

| arm | red rows (subject) | fixture |
|---|---|---|
| un-fixed tip | a.b on l_long_first, l_short_first, l_default = CLOSED; l_mixed a.b = X | RED |
| P2 (the fix) | none | GREEN (3 runs) |
| P1 | l_mixed a.b = X | RED |
| inverted P2 | a.b on l_long_first, l_short_first, l_default = SHORT; l_mixed a.b = X | RED |

The reference side was green in every run.
