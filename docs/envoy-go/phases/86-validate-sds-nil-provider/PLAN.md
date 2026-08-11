# PLAN 86 — validate-sds-nil-provider

**Stage:** PLAN (lifecycle-state 2 -> 3). **Date:** 2026-08-11.
**Base master:** `23ad12329103ee42b064b17d9d4f323f3bf59e6e` (from `git rev-parse master`), branch `phase-86-plan`.
**Method:** the 86-lineage named departure CONTINUES — no investigation agents; every probe run INLINE by the controller (the "a PLAN whose probes are cheap may CONTINUE the inline departure" clause, exercised and named). Probes this stage: the tip binary built with `-o` into session scratch + SEVEN `--mode validate` CLI executions (arms A/B/C + static control + n1/n3/n7), configs regenerated from the SPEC §1.3 descriptions with template ports **47200-47203 — VALUES ONLY, validate binds nothing and nothing was bound**. No docker, no reference container, zero repo-tree edits by any probe.

---

## 1. WHAT THIS PLAN RE-PROVED AT ITS OWN TIP (the stage's job: stress the predecessor by execution)

### 1.1 The RED anchors, re-executed at `23ad1232` — failure lines read, not cited

| shape | result at this tip (MEASURED) |
|---|---|
| control (static TLS) | **exit 0** — `configuration OK` (the rejects are not vacuous) |
| arm A `tls_certificate_sds_secret_configs` | **exit 1** — `listener: "l_sds_tls": filter_chains[0]: tls: downstream: SDS-delivered certificate requires a live SDS provider (unavailable in this mode)` |
| arm B `validation_context_sds_secret_config` | **exit 1** — `listener: "l_sds_mtls": filter_chains[0]: tls: downstream: SDS-bound validation_context_sds_secret_config is not supported in phase 03` |
| arm C `combined_validation_context` | **exit 1** — `listener: "l_sds_cvc": filter_chains[0]: tls: downstream: combined_validation_context is not supported in phase 03` |
| n1 node-absent | **exit 1** — the arm-A string (**MASKED**, confirming the message-transition claim: post-fix this must become `xds: sds: node.id and node.cluster are required for SDS`) |
| n3 `DELTA_GRPC` | **exit 1** — `… tls: downstream: xds: sds: DELTA_GRPC api_type unsupported (only GRPC / State-of-the-World)` (**already the boot string** — SPEC §10.5's pre-existing-parity nuance CONFIRMED) |
| n7 SDS cluster without `http2_protocol_options` | **exit 1** — the arm-A string (**MASKED**; post-fix: the `xds: sds: dial cluster "sds_cluster": grpcclient: dial "sds_cluster": cluster does not have http2_protocol_options{} set …` boot string) |

n2/n4/n5/n6 were NOT re-run here (cheap-probe scope); the IMPL's Task 0 census re-runs ALL ELEVEN shapes at ITS tip with failure lines read (the phase-85 census discipline).

**No SPEC claim moved.** Everything this stage probed held exactly as SPEC §1.3 recorded it: the three arm rejects fire byte-identically (modulo the `listener: %q: filter_chains[%d]:` wrap, which the SPEC's table elided — the IMPL's substring assertions must use `strings.Contains`, never full-string equality, for exactly this reason), n1/n7 are masked, n3 already has parity. The refutation ledger therefore gains no new entry — recorded as a measurement, not as praise.

### 1.2 Counts re-derived MECHANICALLY at this tip (all match the carried roster)

`DECISIONS.md` **18066** lines · **307** `^## ADR-` headings · tail **ADR-0308** (`^## ADR-0309` = 0 ⇒ next-free **ADR-0309**, TAIL-derived) · strict `^> \*\*STATUS: PROPOSED` guard **1 — ARMED** (the live pointer this phase's IMPL disarms) · STATUS census **21** · `^---$` **216** · `BEHAVIOR_CONTRACT.md` **5955** · `STATE.md` **64** · `STATE_HISTORY.md` **478** · `BOOTSTRAP_PROMPT.md` **522** · fixtures **121** (tail `0119-grpc-unary-trailers`, next-free `0120`) · fuzzers **55 / 48** files · BackendKind tail **38** (`fixture.go:614`) · phase dirs **127** · `REVIEW.md` **37** (standing departure) · `go list -deps ./validate` carries **both** `internal/xds` and `internal/xds/xdsgrpc` · validate surface: `validate/validate.go` **64** · `validate/validate_test.go` **432** (zero SDS) · `cmd/envoy-go/main_test.go` **1502** (zero SDS) · `internal/tls/config.go` **529** · `internal/tls/config_test.go` **2398** · `internal/boot/boot.go` **373**.

Symbol anchors re-verified by read: reject sites `config.go:387-389 / :436-438 / :453-455` · fetch sites `:390` + `NewDownstreamConfig`'s VC-SDS block (`:100-136`) and CVC block (`:137-228`) · the `:518` no-cert check · the pins `config_test.go:921` (arm-A nil), `:1198` (`TestValidationContextSDS_SiblingRejectsStay`), `:1310` (`cvcRetainedReject`) · QUIC nil at `manager.go:567` (comment `:569-573`) · `main.go:156` (the ONLY production `NewSDSProvider` call site — the Task 2 split is signature-safe) · `grpcclient.SDSClient.Close()` exists and is pinned idempotent + nil-safe (`grpcclient_test.go:1911/:1982` — the D-86-CONN plumbing needs no new Close machinery).

⚠️ **One form-dependence finding (recorded, not a defect):** the carried "ARM-A well-formedness flags {119, 131} ONLY" could NOT be reproduced by either of two candidate forms tried here (`NF != 8` flags 17 rows; a status/deps field-parse flags different rows) — the ARM-A command is escape-aware pipe-splitting per BRAINSTORM-86 §6, which neither ad-hoc form implements. **Carry the figure only with its own command; for a PLAN the binding invariance proof is the empty `ROADMAP.md` diff, which this stage's close verifies.**

---

## 2. THE IMPL — ONE LEG, ONE ATOMIC COMMIT (D-86-SEQ), EIGHT ORDERED TASKS

The lift, the guards, the guard-preservation tests, the docs, and the ROADMAP flip land as **ONE commit** on branch `phase-86-impl` off the then-current master tip (`reference_lifted_reject_hidden_enforcement`, engaged prospectively — a lift whose guards land later is the exact shape this row exists to avoid). TDD order is held INSIDE the commit, exactly as phase 85 did: every arm proven RED with its failure line read BEFORE its production edit, gates run LAST as the commit's evidence. Task boundaries below are review boundaries, not commit boundaries.

⚠️ House rules that bite here: `go build ./cmd/envoy-go/` drops an untracked binary — always `-o` into scratch. `gofmt -l` gates on OUTPUT, not exit code. `out=$(…); rc=$?`, never `rc=$?` after a pipe. `git -C <abs-worktree-path>` for every git command (the cwd reset fired again at THIS stage). Assert the SYMBOL after every edit, not the build.

### Task 0 — the RED census (probes only, ZERO edits)

1. Build the tip binary: `go build -o "$SCRATCH/impl86/envoy-go" ./cmd/envoy-go/`.
2. Regenerate ALL ELEVEN probe shapes per §6 (arms A/B/C, control, n1-n7) with template ports from **47200-47299** (values only — validate binds nothing).
3. Run each under `--mode validate`; record exit codes AND failure lines. Expected pre-fix: control exit 0; arms A/B/C exit 1 with the three §1.1 strings; n1/n2/n4/n6/n7 exit 1 with the **masking arm-A string**; n3 exit 1 with the `DELTA_GRPC` boot string (already-parity); n5 exit 1 masked.
4. Any deviation from this table is a FINDING that stops the leg (the tip moved under the PLAN) — re-derive before editing.

### Task 1 — `internal/xds/provider_nofetch.go` (NEW) + `provider_nofetch_test.go` (NEW)

**Files:** Create `internal/xds/provider_nofetch.go` (~37-45 lines with house comments) · Create `internal/xds/provider_nofetch_test.go` (~40-80 lines).
**Produces (later tasks consume):** `xds.NoFetchProvider() SecretProvider` · `xds.IsNoFetch(p SecretProvider) bool`.

Step 1 — failing test first (`provider_nofetch_test.go`):

```go
func TestNoFetchProvider_Discriminator(t *testing.T) {
	if !IsNoFetch(NoFetchProvider()) {
		t.Error("IsNoFetch(NoFetchProvider()) = false, want true")
	}
	if IsNoFetch(nil) {
		t.Error("IsNoFetch(nil) = true, want false — nil must NEVER classify as the sentinel")
	}
	var other SecretProvider = &Provider{} // any non-sentinel implementation
	if IsNoFetch(other) {
		t.Error("IsNoFetch(non-sentinel) = true, want false")
	}
}

func TestNoFetchProvider_FetchMethodsError(t *testing.T) {
	p := NoFetchProvider()
	if _, err := p.FetchInitialCertificate(context.Background(), "s"); err == nil ||
		!strings.Contains(err.Error(), "no-fetch (validate-mode) provider asked to fetch") {
		t.Errorf("FetchInitialCertificate: want the defense-in-depth substring, got %v", err)
	}
	if _, err := p.FetchInitialValidationContext(context.Background(), "s"); err == nil ||
		!strings.Contains(err.Error(), "no-fetch (validate-mode) provider asked to fetch") {
		t.Errorf("FetchInitialValidationContext: want the defense-in-depth substring, got %v", err)
	}
}
```

Step 2 — run: `go test ./internal/xds/ -run 'TestNoFetchProvider' -count=1`. Expected: **compile FAILURE** (`undefined: IsNoFetch` / `NoFetchProvider`) — read the line.
Step 3 — implement (the prototype's measured shape):

```go
// noFetchProvider is the validate-mode sentinel SecretProvider (phase 86,
// ADR-0308). It exists so --mode validate can run the ENTIRE boot pre-scan
// (boot.NewValidateSDSProvider) and then thread a NON-NIL provider whose only
// job is to be recognized by internal/tls's IsNoFetch fetch-site skips. The
// discriminator is this TYPE, never provider == nil — the nil-reject's other
// consumers (QUIC, the exported test-only constructors, main.go's seen==0
// case) keep rejecting byte-identically.
type noFetchProvider struct{}

// NoFetchProvider returns the validate-mode sentinel. Value type: the zero
// value IS the sentinel, and IsNoFetch works on any copy.
func NoFetchProvider() SecretProvider { return noFetchProvider{} }

// IsNoFetch reports whether p is the validate-mode sentinel. A nil p is NOT
// the sentinel (the type assertion on a nil interface is false).
func IsNoFetch(p SecretProvider) bool { _, ok := p.(noFetchProvider); return ok }

func (noFetchProvider) FetchInitialCertificate(context.Context, string) (*stdtls.Certificate, error) {
	// Never called: every fetch site checks IsNoFetch FIRST. Kept as
	// defense-in-depth (ADR-0080-distinct if ever seen).
	return nil, fmt.Errorf("xds: sds: internal: no-fetch (validate-mode) provider asked to fetch a certificate")
}

func (noFetchProvider) FetchInitialValidationContext(context.Context, string) (*x509.CertPool, error) {
	return nil, fmt.Errorf("xds: sds: internal: no-fetch (validate-mode) provider asked to fetch a validation context")
}
```

Step 4 — run the same selector: PASS. Step 5 — `gofmt -l internal/xds/` (gate on empty OUTPUT).

### Task 2 — `internal/boot`: the `newSDSProviderAndClient` split + `NewValidateSDSProvider` + `internal/boot/validate_sds_provider_test.go` (NEW)

**Files:** Modify `internal/boot/boot.go` (`NewSDSProvider` at `:120-210`; +30/-7 measured on the prototype) · Create `internal/boot/validate_sds_provider_test.go` (~120-200 lines).
**Consumes:** `xds.NoFetchProvider()` / `xds.IsNoFetch` (Task 1).
**Produces:** `boot.NewValidateSDSProvider(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, error)` — same signature family as `NewSDSProvider`.

Step 1 — failing tests first. Build `*bootstrap.Bootstrap` values via `bootstrap.Load` on YAML strings (the boot package's existing test idiom — see `boot_sds_e2e_test.go`), one per shape:

```go
// Six build-time negatives: each must return the BOOT string (parity by
// code-path). Table-driven; substrings from §4's frozen roster.
func TestNewValidateSDSProvider_BootParityNegatives(t *testing.T) {
	cases := []struct{ name, yaml, wantSub string }{
		{"n1-node-absent", yamlArmANoNode, "xds: sds: node.id and node.cluster are required for SDS"},
		{"n2-two-positions", yamlTwoSDS, "multiple SDS-bound downstream TLS contexts unsupported"},
		{"n3-delta-grpc", yamlDeltaGRPC, "DELTA_GRPC api_type unsupported"},
		{"n4-unknown-cluster", yamlUnknownCluster, `dial cluster "missing_cluster": grpcclient: dial "missing_cluster": unknown cluster`},
		{"n6-bad-name", yamlBadSecretName, "invalid secret name"},
		{"n7-no-h2", yamlNoH2, "cluster does not have http2_protocol_options{} set"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bs := mustLoadBootstrap(t, tc.yaml)
			dialer := grpcclient.New(mustClusterManager(t, bs))
			_, err := NewValidateSDSProvider(dialer, bs, t.TempDir(), bs.Stats)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want substring %q, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestNewValidateSDSProvider_NoSDSReturnsNilNil(t *testing.T) { /* seen==0 ⇒ (nil, nil), mirroring NewSDSProvider */ }

func TestNewValidateSDSProvider_ArmPositiveReturnsSentinel(t *testing.T) {
	// arm-A shape with a well-formed sds_cluster ⇒ (sentinel, nil); xds.IsNoFetch true.
}
```

⚠️ n4's want-substring carries the misleading-but-real `dial cluster` prefix WITHOUT any dial happening (SPEC §2's sub-finding) — quote it verbatim, do not "fix" the wording.
Step 2 — run: compile FAILURE (`undefined: NewValidateSDSProvider`). Read it.
Step 3 — implement. The split (the prototype's measured shape — the ENTIRE current `NewSDSProvider` body moves, so every pre-scan arm is inherited BY CONSTRUCTION, present and future):

```go
func newSDSProviderAndClient(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, *grpcclient.SDSClient, error) {
	// … the ENTIRE current NewSDSProvider body, with the three returns adjusted:
	//   seen==0            -> return nil, nil, nil
	//   every error return -> return nil, nil, err
	//   success            -> return xds.NewProvider(…), client, nil
}

// NewSDSProvider — doc comment unchanged in substance; the body becomes:
func NewSDSProvider(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, error) {
	p, _, err := newSDSProviderAndClient(dialer, bs, baseDir, registry)
	return p, err // the client stays open — the boot path's provider owns it via the opener
}

// NewValidateSDSProvider runs the ENTIRE boot pre-scan (parity by CODE-PATH:
// every present and future pre-scan arm is inherited by construction), then
// CLOSES the never-dialed lazy client (D-86-CONN: validate is a library
// consumed by long-running Gateway controllers; leaking one lazy
// *grpc.ClientConn per reconcile call is not acceptable hygiene) and returns
// the no-fetch sentinel. (nil, nil) at seen==0 exactly like the boot path.
func NewValidateSDSProvider(dialer *grpcclient.Dialer, bs *bootstrap.Bootstrap, baseDir string, registry *stats.Registry) (xds.SecretProvider, error) {
	p, client, err := newSDSProviderAndClient(dialer, bs, baseDir, registry)
	if err != nil {
		return nil, err // these ARE the boot-parity rejects
	}
	if p == nil {
		return nil, nil // no SDS anywhere: the nil provider threads harmlessly
	}
	_ = client.Close() // idempotent + nil-safe (grpcclient_test.go:1911/:1982)
	return xds.NoFetchProvider(), nil
}
```

Step 4 — run the three new test functions: PASS. Step 5 — run the WHOLE package `go test ./internal/boot/ -count=1` (the existing `NewSDSProvider` tests must stay green — the split is behavior-preserving for the boot path; `RegisterSDSStats` is still called exactly once inside the shared body, so the stat-surface delta stays 0).

### Task 3 — `internal/tls/config.go`: three fetch-site skips + the `:518` interplay + tests

**Files:** Modify `internal/tls/config.go` (+25/-5 measured on the prototype: arm A at `:379-395`, the `:518` check, the VC-SDS block `:100-136`, the CVC block `:137-228`) · Modify `internal/tls/config_test.go` (+~100-160; the three pins at `:921/:1198/:1310` **BYTE-UNTOUCHED** — verify by `git diff` hunk inspection: no hunk may overlap them).
**Consumes:** `xds.IsNoFetch` (Task 1).

Step 1 — failing tests first (all in `config_test.go`, reusing its `sdsDownstreamTS`/`sdsSecretConfig`/`cvcCTC` helpers):

```go
// Post-fix contract: the sentinel provider ACCEPTS all three arms with NO fetch.
func TestCommonTLSContext_ArmA_NoFetchSentinel_Accepted(t *testing.T) {
	cfg, err := commonTLSContextToConfig(sdsOnlyCertCTC(t), "", "downstream", xds.NoFetchProvider())
	if err != nil {
		t.Fatalf("sentinel provider must accept the arm-A shape without fetching: %v", err)
	}
	if len(cfg.Certificates) != 0 {
		t.Errorf("no material may be fabricated: got %d certificates, want 0", len(cfg.Certificates))
	}
}

// D-86-NOFETCH-CERT (the :518 interplay): an SDS-ONLY-cert listener must NOT
// hit "no tls_certificates configured" when the fetch is skipped.
func TestCommonTLSContext_ArmA_SDSOnlyCert_NoEmptyCertReject(t *testing.T) { /* same shape; assert err == nil AND the specific absent substring */ }

func TestNewDownstreamConfig_VCSDS_NoFetchSentinel_Accepted(t *testing.T)  { /* arm-B shape + static cert + sentinel ⇒ nil error, cfg.ClientCAs nil (no pool fabricated) */ }
func TestNewDownstreamConfig_CVC_NoFetchSentinel_Accepted(t *testing.T)    { /* arm-C shape + static cert + sentinel ⇒ nil error; E1/E2 presence checks still enforced (missing default_validation_context STILL rejects under the sentinel) */ }

// Guard preservation beyond the pins — §4's roster, one test per row.
```

Step 2 — run: the accept arms FAIL — arm A errors with the **defense-in-depth fetch string** (the sentinel is non-nil, so `:387` passes and `:390` fetches from the sentinel) — a readable, discriminating RED, not a compile error. Read each line.
Step 3 — implement the four edits:

- **arm A** (`:379-395`): after the `provider == nil` reject, replace the unconditional fetch with:
  ```go
  var sdsCertPromised bool
  // … inside the arm:
  if xds.IsNoFetch(provider) {
  	sdsCertPromised = true // validate mode: structural checks ran; the leaf arrives at runtime
  } else {
  	cert, err := provider.FetchInitialCertificate(context.Background(), secretName)
  	if err != nil { /* unchanged */ }
  	sdsCert = cert
  }
  ```
- **the `:518` check**: `if side == "downstream" && len(cfg.Certificates) == 0 && !sdsCertPromised {` — the reject string itself is BYTE-UNTOUCHED.
- **VC-SDS block** (`NewDownstreamConfig`, after its `ParseSDSConfig` + before `FetchInitialValidationContext`): `if xds.IsNoFetch(provider) { return &DownstreamConfig{TLSConfig: cfg}, nil }` — structural validation preserved, no fetch, no `installPool`, no fabricated pool.
- **CVC block**: same skip at the same position (after its `ParseSDSConfig` call at `:178`). The E1/E2 presence checks in `commonTLSContextToConfig` already ran before this point.

Step 4 — run the new tests: PASS. Step 5 — run the WHOLE package + `-race`; the three pins and `TestNewQUICDownstreamConfig_*` (`:1126-1176`) must be green UNTOUCHED.

### Task 4 — `validate/validate.go` threading + `validate/validate_test.go` end-to-end + `cmd/envoy-go/main_test.go` CLI arms

**Files:** Modify `validate/validate.go` (+10/-2 measured: `:48-49`) · Modify `validate/validate_test.go` (+~120-200) · Modify `cmd/envoy-go/main_test.go` (+~60-120).
**Consumes:** `boot.NewValidateSDSProvider` (Task 2).

Step 1 — failing tests first. `validate_test.go` (package-level, YAML-string configs — its existing idiom): three arm ACCEPTS (`err == nil`), the n5-class accept (structurally valid config, dead SDS endpoint ⇒ `err == nil` — D-86-N5 pinned), and the negatives' error-substring arms (n1/n2/n4/n6/n7 → boot strings — these ARE the message-transition arms: today they'd get the masking arm-A string, so each doubles as a RED anchor). `main_test.go` (subprocess idiom): `--mode validate` on an arm-A shape ⇒ exit 0 + stdout `configuration OK`; on the n1 shape ⇒ exit 1 + stderr containing the node boot string.
Step 2 — run: every accept arm and every message-transition arm FAILS with the masking arm-A string (Task 0's census lines). Read them.
Step 3 — implement:

```go
	dialer := grpcclient.New(cm)
	tracingProvider := boot.NewTracingProvider(dialer, httpClient, cm, bs.Stats)
	defer func() { _ = tracingProvider.CloseAll() }()
	// Phase 86 (ADR-0308): run the ENTIRE boot SDS pre-scan (node requirement,
	// one-secret cap, ParseSDSConfig arms, secret-name charset, cluster
	// existence + http2_protocol_options — parity by code-path), then thread
	// the no-fetch sentinel so internal/tls skips the fetches. NOTHING dials
	// or fetches: the phase-60.2 no-DIAL decision survives; the literal-nil
	// reject was its over-broad implementation.
	sdsProvider, err := boot.NewValidateSDSProvider(dialer, bs, baseDir, bs.Stats)
	if err != nil {
		return err // the boot-parity rejects, byte-identical by code-path reuse
	}
	_, err = boot.Construct(bs, cm, baseDir, allowH2C, nil, dm, httpClient, tracingProvider, sdsProvider)
	return err
```

Step 4 — run `go test ./validate/ ./cmd/envoy-go/ -count=1`: PASS. Step 5 — re-run the Task 0 probe battery against a REBUILT binary: arms A/B/C + control + n5 now `configuration OK` exit 0; n1/n2/n4/n6/n7 exit 1 with the boot strings **byte-identical to normal-mode boot** (diff the strings mechanically against a normal-mode run, per shape — parity by code-path DEMONSTRATED, not asserted).

### Task 5 — production comment rewrites (the §4-sweep edit sites; same commit)

**Files:** Modify `internal/tls/config.go` comments at `:344-346` (「nil for upstream/validate callers, which never fetch SDS」 — validate no longer passes nil; restate: nil for upstream callers; validate passes the no-fetch sentinel), `:407-435` (the VC-SDS consumer enumeration: `validate.Bootstrap` LEAVES the roster — it now threads the sentinel; the remaining nil consumers are QUIC + the two test-only constructors), `:447-452` (the CVC consumer roster: THREE → TWO production consumers + validate-now-sentinel note) · `internal/listener/manager.go:569-573` (the "nil at all pre-60.2 call sites" comment gains the phase-86 note: validate threads the sentinel through `Construct`; the exported test-only constructors still hardcode nil) · `validate/validate.go` package doc (the "without … dialing" sentence STAYS TRUE — extend with the SDS-acceptance sentence).
Estimated +15-30 comment lines (inside the §7 budget). ⚠️ Every rewritten comment must LEAD with what survives (the ADR-0297 ¶7/¶9 discipline).

### Task 6 — docs riding ADR-0308 (same commit): the contract edits, the ADR completion, the ROADMAP flip

**Files:** Modify `docs/envoy-go/BEHAVIOR_CONTRACT.md` (4 sites) · Modify `docs/envoy-go/DECISIONS.md` (ADR-0308 completed IN PLACE) · Modify `docs/envoy-go/ROADMAP.md` (row 86 `in-progress` → `done` at `:148` — the ONLY ROADMAP edit, `numstat 1 1`, `want` STAYS 118).

**6a — `BEHAVIOR_CONTRACT.md:1062` (item 7): the REVERSAL.** Replace the item's text with (drafted here; finalize at IMPL against the landed code):

> 7. **The `validate` path REJOINS — the phase-66 "diverges, and stays that way" decision is REVERSED by phase 86 (ADR-0308).** `validate.Bootstrap` now threads `boot.NewValidateSDSProvider`'s no-fetch sentinel (never a literal nil), so a well-formed CVC — and both sibling SDS shapes — validates `configuration OK` exactly where the boot path accepts, with the ENTIRE boot pre-scan run by code-path reuse and ZERO I/O. The contract: **validate accepts iff boot accepts, modulo exactly the rejects that require I/O to discover** (the dead-SDS-endpoint class — boot rejects at fetch, bounded by `initial_fetch_timeout`; validate accepts). The phase-66 rationale recorded here ("plumbing SDS into a config-validator implies dialing a management server from `validate` mode") was REFUTED BY EXECUTION on both sides at the phase-86 SPEC: the fix validates all three arms with zero dial, and the reference's own `--mode validate` runs SDS structural checks without dialing (it rejects node-absent). The phase-60.2 no-DIAL decision SURVIVES — nothing dials, nothing fetches; the literal-nil reject was its over-broad implementation.

**6b — `:1050` (item 1): the consumer-roster count.** "THREE production consumers" becomes TWO plus the sentinel note: `NewQUICDownstreamConfig` (hardcoded nil) and `cmd/envoy-go/main.go`'s `seen == 0` case remain; `validate.Bootstrap` LEAVES the roster (it now passes the no-fetch sentinel, which is non-nil, so the guard no longer fires for it); the two test-only constructors stay stated separately. Preserve the item's "count is stated with its scope" closing discipline.

**6c — `:1034` (Siblings STAY): drop `validate.Bootstrap` from the still-rejects roster.** The sentence "It still rejects, byte-identically …, for the QUIC downstream arm, `validate.Bootstrap`, the upstream arm, …" loses `validate.Bootstrap` and gains "(`validate.Bootstrap` rejoined the accept path at phase 86 — ADR-0308)". The trailing parenthetical about `NewDownstreamConfig`'s defense-in-depth nil guard STAYS TRUE as written (the sentinel skips BEFORE the fetch, so that guard remains unreachable dead code).

**6d — `:948-958` (the validate section): the extension paragraph.** Append after the "validation depth" paragraph (the phase-51 "No change to WHAT is validated" sentence STAYS — it is that phase's history):

> **SDS-bound TLS under `--mode validate` (phase 86, ADR-0308).** All three downstream SDS shapes the boot path honors (`tls_certificate_sds_secret_configs` — phase 60.2; `validation_context_sds_secret_config` — phase 65; `combined_validation_context` — phase 66) now VALIDATE `configuration OK`. `validate.Bootstrap` calls `boot.NewValidateSDSProvider`, which reuses the ENTIRE boot pre-scan by code-path (`newSDSProviderAndClient`): the node id/cluster requirement, the one-secret MVP cap, the `ParseSDSConfig` arms, the secret-name charset, and the `grpcclient` cluster-existence + `http2_protocol_options` checks all reject BYTE-IDENTICALLY to normal-mode boot. It then closes the never-dialed lazy client and returns the no-fetch sentinel (`xds.NoFetchProvider`), which `internal/tls` recognizes (`xds.IsNoFetch`) and skips all three fetch sites — NOTHING dials or fetches. **Validate accepts iff boot accepts, modulo exactly the rejects that require I/O to discover:** a structurally valid config whose SDS endpoint is dead validates OK (boot rejects it at fetch time). The reference's validate draws the same line — it accepts the dead-endpoint shape and runs its SDS structural checks (node-absent rejects) without dialing.

**6e — ADR-0308 §Decision + §Consequences, APPENDED IN PLACE** after the retained italic footer (`*§Decision and §Consequences follow at the phase-86 IMPL.*` — RETAINED, the ADR-0294-0307 shared block form), **no renumber, no `---`** (`^---$` STAYS 216). §Decision: the seven-decision ledger (D-86-MECH / PARITY / NOFETCH-CERT / CONN / N5 / REF / SEQ) restated as landed decisions with the four file-level shapes. §Consequences: (i) the parity contract as the row's permanent statement; (ii) the `:1062` reversal ON THE RECORD with the refuted rationale quoted; (iii) realized cost vs the §7 FLOOR — overrun RECORDED, not silently absorbed (the ninth-firing lineage); (iv) the reference per-side record (SPEC §3's table — observations, never expectations); (v) the two new never-called defense-in-depth strings (ADR-0080-distinct if ever seen); (vi) row 86 flips `done`, `want` stays 118, check (1) goes silent (stated as a measurement at the close). **The strict `^> \*\*STATUS: PROPOSED` guard goes 1 → 0** (STATUS line becomes COMPLETE) — re-derive headings **307** (unchanged), tail **ADR-0308**, next-free **ADR-0309**, `^---$` **216** at the close.

**6f — `ROADMAP.md:148`:** row 86 status `in-progress` → `done`. `git diff --numstat` on the file MUST read `1 1`; every leak axis re-verified INVARIANT by whole-file count (lines 236 · rows 118 · union 6 · `-family row` 95/67 · `gRPC-family row` 2 · `Operational-tooling-family row` 3). ⚠️ Nothing narrows in any family-window sentence at row-done (the row's provenance is OUTSIDE the windows — BRAINSTORM §2; the Op-tooling "RTDS/SDS validate companion" sentence stays BYTE-UNTOUCHED).

### Task 7 — gates, sentinel, close (the commit's evidence, run LAST)

1. `gofmt -l` empty + `golangci-lint run` clean on the five touched packages (`internal/xds`, `internal/boot`, `internal/tls`, `validate`, `cmd/envoy-go`) — misspell locale US.
2. `go test ./... -count=1` rc=0 (captured via `out=$(…); rc=$?`); `-race` on the four production-touched packages. Flake register consulted BEFORE blaming the row (§8).
3. The FULL differential suite: **121/121 `INNER_EXIT=0`**, zero FAIL/SKIP, zero `no driver registered`, anchored panic gate (`^panic:|DATA RACE|SIGSEGV`) SILENT — expected UNCHANGED (this row has no differential surface; the suite is the byte-stability anchor).
4. Anticipated counts, each with its NC observed: fixtures **121** (+0) · fuzzers **55/48** (+0) · BackendKind tail **38** (+0) · `go mod tidy -diff` EMPTY · `go list -deps ./validate` edge set UNCHANGED · **stat-surface DELTA 0** by same-command census both sides — state the form: `NewCounter(|NewGauge(` over `internal/ cmd/ validate/`, tests excluded (**145/21** at the SPEC tip; re-derive both sides at the IMPL tip; only the DELTA is asserted).
5. Sentinel re-run MECHANICALLY, BEFORE and AFTER the flip, both doctoring NCs + the check-(2) one-arm and check-(3) doctoring NCs each side; record ACTUAL output. Expected (not forecast — verify): pre-flip (1) `NOT DONE: row 86` · post-flip (1) **SILENT** (the third silent reading in project history) · (2) SIX both sides · (3) SILENT both sides ⇒ the conjunction FAILS both sides, `stop` NOT created (verify absent at repo root AND worktree).
6. `STATE.md` rolled IN PLACE (row-86 IMPL done, lifecycle 3 → DONE); the §Recent five-entry cap enforced by DIRECT DATE READ; eviction to `STATE_HISTORY.md` strictly append-only (`numstat N 0` + `cmp` prefix + the ROBUST absence guard with THREE from-the-file controls, each control's count re-read at that close — they DRIFT UP). `PROGRESS.md` updated. `next-prompt.txt` rolled to the post-86 pick (`git add -f`).
7. ONE squashed commit on `phase-86-impl`, merged to master, pushed (`feedback_push_to_origin`).

---

## 3. TDD ORDER — THE EXPLICIT SEQUENCE INSIDE THE ONE COMMIT

**Census (Task 0) → T1 RED → T1 green → T2 RED → T2 green → T3 RED → T3 green → T4 RED → T4 green → probe battery re-run vs rebuilt binary (parity strings diffed per shape) → comments (T5) → docs (T6) → gates (T7).**

The RED census discipline (phase 85): every failing run's FAILURE LINE is READ and recorded, never inferred from the test name. Expected RED census: **Task 1** compile-fail ×2 · **Task 2** compile-fail (then, post-symbol, any wrong-string arm fails with the actual string printed) · **Task 3** the accept arms RED with the sentinel's defense-in-depth fetch string (a DISCRIMINATING red: it proves the sentinel reached the fetch site, i.e. the skip is genuinely missing) · **Task 4** the accept + message-transition arms RED with the masking arm-A string (Task 0's lines). Green-on-arrival arms (n3-class: already-parity) are LABELED as regression pins, not claimed as reds — the phase-85 census discipline again.

---

## 4. THE GUARD-PRESERVATION NC ROSTER (owes item 3)

**The three pins STAY BYTE-UNTOUCHED and green:** `config_test.go:921` (arm-A nil-provider reject) · `:1198` (`TestValidationContextSDS_SiblingRejectsStay`) · `:1310` (`cvcRetainedReject` + its consumers). Verified two ways at the IMPL close: the full-package run is green AND no `git diff` hunk overlaps their line ranges.

**New NCs, landing IN THE SAME COMMIT as the lift** (each a test in the file named; each asserts the BYTE-IDENTICAL substring, `strings.Contains`):

| # | shape | expected post-lift behavior | file |
|---|---|---|---|
| NC-1 | arm A, `provider == nil` | STILL rejects `requires a live SDS provider` (beyond pin `:921` — asserts adjacent to the new skip) | `config_test.go` |
| NC-2 | arm B, nil | STILL rejects the phase-03 VC-SDS string | `config_test.go` |
| NC-3 | arm C, nil | STILL rejects the phase-03 CVC string | `config_test.go` |
| NC-4 | `NewQUICDownstreamConfig` whose inner DTC carries `tls_certificate_sds_secret_configs` | rejects the arm-A string (QUIC hardcodes nil at `manager.go:567` / `config.go:273` — mode-independent) | `config_test.go` |
| NC-5 | `NewQUICDownstreamConfig` whose inner DTC carries a CVC | rejects the phase-03 CVC string | `config_test.go` |
| NC-6 | `listener.NewManager` / `NewManagerWithBaseDir` on an SDS-carrying bootstrap | reject (test-only ctors hardcode nil) | `internal/listener` tests |
| NC-7 | `boot.NewValidateSDSProvider` seen==0 + a pure-inline CVC listener | `(nil, nil)` and the downstream build still emits the retained phase-03 string (main.go's seen==0 shape preserved) | `validate_sds_provider_test.go` |
| NC-8 | `xds.IsNoFetch(nil)` | **false** — nil can NEVER classify as the sentinel (the discriminator's own guard) | `provider_nofetch_test.go` |
| NC-9 | the sentinel's two fetch methods | error with the `no-fetch (validate-mode)` substring (defense-in-depth stays armed) | `provider_nofetch_test.go` |

NC-4/NC-5 are the "QUIC shape included" requirement stated by SPEC §13 — a QUIC-wrapped SDS shape must keep rejecting REGARDLESS of mode, because its nil is hardcoded, not validate-inherited.

## 5. THE ERROR-STRING CONSTRAINT SET (Q6, restated for the IMPL)

**ZERO landed reject strings change.** The frozen roster the IMPL asserts against (all pre-existing, all byte-identical by code-path reuse): the three arm strings (`:387-389/:436-438/:453-455`) · the six boot-parity strings (node · one-secret cap · `ParseSDSConfig` arms incl. `DELTA_GRPC` · `invalid secret name` · `unknown cluster` · `http2_protocol_options`) · `no tls_certificates configured` (the `:518` string — its GATE gains `&& !sdsCertPromised`; its BYTES don't change). The ONLY new strings are the sentinel's two never-called defense-in-depth messages (Task 1 step 3 — ADR-0080-distinct if ever seen). Test assertions use substrings (`strings.Contains`) because the listener build WRAPS every tls error with `listener: %q: filter_chains[%d]:` (measured at §1.1).

## 6. PROBE-SHAPE REGENERATION (owes item 6)

The eleven shapes are DESCRIBED in SPEC §1.3/§2/§3 and were regenerated (7 of 11) at this PLAN from those descriptions alone — the recipe holds: base = fixture `0103`'s arm-A shape / `0108`'s arm-B shape / `0109`'s arm-C shape with inline `filename:` PKI (an openssl self-signed pair in scratch) and a STATIC `c_backend` + `sds_cluster` (h2 options block present except in n7). Derivations: **control** = arm A with the SDS entry swapped for static `tls_certificates` · **n1** = arm A minus `node:` · **n2** = arm A + a second SDS position · **n3** = `api_type: GRPC` → `DELTA_GRPC` · **n4** = `cluster_name:` → `missing_cluster` · **n5** = arm A with `sds_cluster` pointing at a dead loopback port · **n6** = secret name → `bad/name` · **n7** = arm A minus the `typed_extension_protocol_options` block. Template ports: values from **47200-47299**, NEVER BOUND (validate binds nothing; the IMPL's normal-mode parity-string runs for n1-n7 also exit before bind — 5-9 ms measured at the SPEC). All configs + PKI + binaries live in session scratch, never the repo tree.

## 7. BUDGET — CARRIED AS A FLOOR (owes item 5), PER-TASK ENUMERATION

| task | production `.go` | test `.go` |
|---|---|---|
| T1 sentinel | ~37-45 (new file) | ~40-80 |
| T2 boot split | ~30-40 | ~120-200 |
| T3 tls skips + interplay | ~25-35 | ~100-160 |
| T4 validate + CLI | ~10-15 | ~180-320 (validate ~120-200 + main_test ~60-120) |
| T5 comment rewrites | ~15-30 (comment lines) | — |
| **sum** | **~117-165** | **~440-760** |

The SPEC's envelope (~110-160 prod / ~400-680 test) is CARRIED AS A FLOOR; this enumeration's top end already exceeds it on the test side — consistent with nine consecutive `reference_measured_prototype_is_a_lower_bound` firings, cause always under-ENUMERATION. **Overrun beyond ~165/~760 is a RECORDABLE FINDING for ADR-0308 §Consequences, not a silent absorption.** Docs (not `.go`): the four contract paragraphs (§2 Task 6a-6d) + ADR-0308 §Decision/§Consequences (~60-100 lines) + `PROGRESS.md`/`STATE.md`/`next-prompt.txt` rolls.

## 8. FLAKE REGISTER (verbatim carry — the index is the memory directory, this is the working set)

SDS dial-budget (TWO packages) · `internal/cluster` `-race` outlier · `internal/httpclient` zero-value · driver-receiver port race (roster 42; `0081` binds `0.0.0.0:42039`) · the two 84.2-era flakes (cleared, unclassed) · `TestServerConn_TinyWindowDelivery` (85-IMPL, cleared, unclassed) · the REFERENCE h2spec section-8 flip (reference-side only). Full-suite rule: check for sibling sessions before blaming the row; a flake is cleared scoped ×5 + full-package ×3 before recording; recurrence IN-BAND is a FINDING.

## 9. SENTINEL — RE-RUN MECHANICALLY AT THIS PLAN. IT DOES **NOT** FIRE

Input measured **236 lines / 118 data rows** first. **(1)** `NOT DONE: row 86` at `want=118` — the single expected line, denominator silent · **(2)** SIX — `:196 :202 :208 :218 :224 :232` · **(3)** SILENT ⇒ the conjunction FAILS — **no fire**; `stop` NOT created (verified absent at repo root and this worktree). **All four NCs fired at this stage:** row-62 doctoring ⇒ `NOT DONE: row 62` AND `NOT DONE: row 86`, with `NC LANDED? [ in-progress ]` inspected first · `want=117` on the REAL file ⇒ `NOT DONE: row 86` + `GATE FAIL: examined 118 data rows, expected 117` · check-(3) doctoring (residual confirmed **2 → 0** on the doctored copy first) ⇒ `NEVER OPENED: gRPC`, WASM control silent · check-(2) one-arm **5** (long) / **1** (short), union **6**. Leak axes at this tip: `-family row` **95 occurrences / 67 lines** · `gRPC-family row` **2** · `Operational-tooling-family row` **3**. `ROADMAP.md` BYTE-UNTOUCHED by this stage — verified by empty diff at close (§10).

## 10. HYGIENE

This stage lands **ZERO production `.go`, ZERO test `.go`** — docs only (`PLAN.md`, `PROGRESS.md`, `STATE.md`/`STATE_HISTORY.md`, `next-prompt.txt`). `ROADMAP.md`, `DECISIONS.md`, `BEHAVIOR_CONTRACT.md`, `go.mod`/`go.sum` **BYTE-UNTOUCHED** (verified by empty diff at close; a PLAN adds no ADR — the strict `PROPOSED` guard STAYS 1, disarmed only by the IMPL). Probe artifacts (binary, 7 configs, openssl PKI) in session scratch only; ports 47200-47203 template values, nothing bound; no docker. The stage worktree is removed after the squash-merge-push.

## 11. NEXT

**IMPL** — the single leg of §2, Tasks 0-7 in the §3 order, ONE atomic commit. It owes: the full eleven-shape census at ITS tip with failure lines read; the per-shape parity-string diff against normal-mode boot (Task 4 step 5); the §4 NC roster green with the pins byte-untouched; the §6a-6d contract text finalized against the landed code; ADR-0308 completed in place (strict guard 1 → 0); row 86 `done` with `want` 118 and every leak axis invariant; realized cost vs the §7 floor recorded in ADR-0308 §Consequences; the sentinel re-run both sides of the flip with all four NCs.
