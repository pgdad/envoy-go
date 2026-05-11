package compressor

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// Accept-Encoding q-value parser per RFC 7231 §5.3.4 + classification dispatch
// per SPEC §6.4 + §11.4 + §11.5 verbatim probeA evidence. Split out of
// compressor.go per planner-time decision 1 (the q-value parser is the most-
// self-contained primitive in the package and benefits from isolation).
//
// The parser is consumed by (f *filter).DecodeHeaders (Task 5 wiring); Task 3
// lands the parser + Group 4 unit tests stand-alone. The 6 classification
// tokens map verbatim to the 6 `header_*` counter names registered in
// filterStats per ADR-0132 §Decision (i) + SPEC §11.5.

// recognizedUnconfiguredCodings is the closed set of content-coding tokens
// the parser recognizes as "known but not selectable on the gzip-only MVP".
// Per SPEC §6.4: known codings unconfigured (e.g., br with gzip-only MVP)
// map to selected="" + classification="overshadowed" — codings present in
// the AE list but not selectable. The presence of any of these tokens (with
// q>0) in the Accept-Encoding list, when no gzip/wildcard/identity is
// selectable, drives the `overshadowed` classification rather than the
// `no_accept_header` fallback.
//
// `gzip` is the only configured codec in the MVP; if/when additional codecs
// are added, they move out of this set and into the codec-selection branch.
var recognizedUnconfiguredCodings = map[string]struct{}{
	"br":       {}, // brotli — RFC 7932
	"deflate":  {}, // RFC 1951
	"compress": {}, // legacy LZW
	"zstd":     {}, // RFC 8478
}

// encodingEntry captures one (coding, q-value) pair parsed from an
// Accept-Encoding header. `coding` is lowercased; `*` is preserved as the
// wildcard token. `qValue` defaults to 1.0 when no `q=` parameter is present
// per RFC 7231 §5.3.4. Stable tie-breaking on equal q-values is provided by
// `sort.SliceStable` at parseAcceptEncoding time (declared-list-position
// preserved implicitly).
type encodingEntry struct {
	coding string
	qValue float64
}

// parseAcceptEncoding parses an Accept-Encoding header per RFC 7231 §5.3.4
// and returns the negotiated encoding + the classification token used by the
// 6 `header_*` counter increments at EncodeHeaders time (per SPEC §6.4 +
// §11.5).
//
// Returns:
//   - selected ∈ {"", "gzip"} — "gzip" when the gzip codec was selected
//     (explicit gzip with q>0 OR wildcard `*` with q>0 absent explicit gzip);
//     "" otherwise (identity selected; overshadowed by non-configured codec;
//     malformed header; absent header).
//   - classification ∈ {"compressor_used", "overshadowed", "identity",
//     "wildcard", "no_accept_header", "not_valid"} — the 6 classifications
//     map verbatim to the 6 `header_*` counter names per SPEC §11.5 probeA.
//
// Algorithm:
//  1. Empty/whitespace-only header → ("", "no_accept_header") per §6.4.
//  2. Split on `,` to produce comma-separated coding entries; each entry is
//     parsed via parseEncoding (split on `;`; coding token + q-value param).
//     Malformed q-value or empty token aborts the parse → ("", "not_valid").
//  3. Sort entries by q-value desc with declared-order stable tie-break.
//  4. Walk the sorted list via selectByQValue to apply the dispatch rules.
//
// The parser is package-local (no exported names) per the file-structure
// table row; consumers must live in the compressor package.
func parseAcceptEncoding(header string) (selected string, classification string) {
	if strings.TrimSpace(header) == "" {
		return "", "no_accept_header"
	}
	entries := make([]encodingEntry, 0, 4)
	for _, raw := range strings.Split(header, ",") {
		entry, ok := parseEncoding(raw)
		if !ok {
			return "", "not_valid"
		}
		entries = append(entries, entry)
	}
	// Stable sort by q-value desc; declared order breaks ties.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].qValue > entries[j].qValue
	})
	return selectByQValue(entries)
}

// parseEncoding parses one comma-separated chunk like "gzip;q=0.5" or
// " *; q=1.0 " or "identity" or "gzip;" (trailing semicolon tolerated) into
// an encodingEntry. Returns (entry, false) on any parse error: empty coding
// token, malformed q-value, or out-of-range q-value.
//
// Non-q parameters (e.g., the unusual `gzip;foo=bar`) are silently ignored
// per RFC 7231 — only the `q=` parameter is standardized on Accept-Encoding.
// The token is lowercased for case-insensitive comparison per RFC 7231 §3.1.2.1
// ("All content-coding values are case-insensitive").
func parseEncoding(raw string) (encodingEntry, bool) {
	parts := strings.Split(raw, ";")
	token := strings.ToLower(strings.TrimSpace(parts[0]))
	if token == "" {
		return encodingEntry{}, false
	}
	q := 1.0
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if p == "" {
			// Trailing or duplicate semicolons (e.g., "gzip;" or "gzip;;q=0.5")
			// are tolerated per RFC 7230 §3.2.6 OWS rules.
			continue
		}
		lp := strings.ToLower(p)
		if !strings.HasPrefix(lp, "q=") {
			// Non-q parameter; silently ignored.
			continue
		}
		val, ok := parseQValue(p[2:])
		if !ok {
			return encodingEntry{}, false
		}
		q = val
	}
	return encodingEntry{coding: token, qValue: q}, true
}

// parseQValue parses the q= parameter value as float64 in the RFC 7231 §5.3.1
// range [0.0, 1.0]. Returns (q, true) on success; (0, false) on parse error
// (non-numeric input, e.g., "blah"), on NaN/Inf (which slip past
// `strconv.ParseFloat` but are outside the §5.3.1 decimal grammar), or on
// out-of-range value (e.g., "2.0" or "-0.1"). All three error modes are
// treated as malformed per SPEC §14.1 Group 4.
func parseQValue(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
		return 0, false
	}
	return v, true
}

// selectByQValue walks the sorted-by-q-value-desc entry list and returns the
// negotiated encoding + classification per the SPEC §6.4 + §11.4 + §11.5
// dispatch rules.
//
// The selection walks the q-sorted list and returns on the first selectable
// entry (gzip / identity / wildcard, q>0). The classification token reflects
// which kind of entry won:
//
//   - explicit `gzip` (q>0) selected → ("gzip", "compressor_used") per §11.5
//     probeA: header_compressor_used increments whenever gzip was the negotiated
//     selection regardless of subsequent skip-decisions.
//   - explicit `identity` (q>0) selected → ("", "identity") per §11.4 probeA
//     "AE: identity → [no compression — identity selected over gzip]". Identity
//     at higher q outranks gzip at lower q.
//   - wildcard `*` (q>0) selected → ("gzip", "wildcard") per §11.4 probeA
//     "AE: * → wildcard matched gzip". Configured codec gzip selected via
//     the wildcard's blanket acceptance.
//
// Fallback (no entry matched the gzip/identity/wildcard selectable set with
// q>0):
//
//   - recognized non-configured coding (br/deflate/compress/zstd) with q>0
//     present → ("", "overshadowed") per §6.4 prose: known codings
//     unconfigured (e.g., `br` with gzip-only MVP) map to "overshadowed" —
//     codings present in the list but not selectable.
//   - else (all q=0 OR list of unknown tokens) → ("", "identity") per
//     §11.4(b) verbatim probeA "gzip;q=0 → no gzip selected; identity
//     selected (default fallback)" + RFC 7231 §5.3.4 default-fallback
//     semantics.
//
// Note §11.4(a) load-bearing case: `gzip;q=0.5, br;q=1.0` — br has the
// higher q-value and sorts first; the walk encounters (br, 1.0) which is
// neither gzip/identity/wildcard, so it's not selectable; the walk continues
// to (gzip, 0.5) which IS selectable and wins. Classification is
// `compressor_used` because gzip was the negotiated selection, not
// `overshadowed`. (Empirically confirmed at probeA: `content-encoding: gzip`
// + `transfer-encoding: chunked`; SPEC §11.4 line 1356.)
func selectByQValue(entries []encodingEntry) (selected string, classification string) {
	// Pass 1: walk q-sorted list; first selectable entry (gzip/identity/
	// wildcard with q>0) wins. Non-selectable (recognized-unconfigured or
	// unknown) entries are skipped — their presence is recorded by the
	// fallback passes below.
	for _, e := range entries {
		if e.qValue <= 0 {
			continue
		}
		switch e.coding {
		case "gzip":
			return "gzip", "compressor_used"
		case "identity":
			return "", "identity"
		case "*":
			return "gzip", "wildcard"
		}
	}
	// Pass 2: no selectable entry — check for recognized non-configured
	// codings with q>0 (§6.4 overshadowed prose; e.g., br on the gzip-only
	// MVP).
	for _, e := range entries {
		if e.qValue <= 0 {
			continue
		}
		if _, ok := recognizedUnconfiguredCodings[e.coding]; ok {
			return "", "overshadowed"
		}
	}
	// Pass 3: default fallback (§11.4(b)). All entries are q=0 OR carry only
	// unknown tokens; identity is implicitly acceptable per RFC 7231 §5.3.4.
	return "", "identity"
}
