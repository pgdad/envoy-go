package hcm

import (
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"time"

	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

// serverHeader returns the canonical Server header value for HCM-locally-
// generated responses. ADR-0014 (admin /ready precedent) reaffirmed for HCM
// per ADR-0044 / SPEC §10 #12 settled.
func serverHeader() string { return "envoy" }

// dateHeader returns the current Date header value formatted as RFC 7231
// IMF-fixdate (e.g. "Sun, 06 Nov 2024 08:49:37 GMT"). Per-response computation
// (no caching) per SPEC §10 #8 settled. Phase-06+ may add caching if profiling
// reveals hot allocation.
func dateHeader() string { return time.Now().UTC().Format(http.TimeFormat) }

// writeStatusReply writes a complete HTTP/1.1 local-reply response to w. The
// status line uses http.StatusText for the reason phrase; if the status is
// unknown to stdlib, the reason phrase is empty. Headers in fixed order:
// Content-Type, Content-Length, Server, Date. A CRLF blank line then the body.
//
// This is the ONLY path in package hcm that locally generates a response
// body. The router action goes through stdlib's Response.Write for proxied
// responses.
func writeStatusReply(w io.Writer, status int, body string) error {
	reason := http.StatusText(status)
	if _, err := fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", status, reason); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"Content-Type: text/plain\r\nContent-Length: %d\r\nServer: %s\r\nDate: %s\r\n\r\n",
		len(body), serverHeader(), dateHeader()); err != nil {
		return err
	}
	if body != "" {
		if _, err := io.WriteString(w, body); err != nil {
			return err
		}
	}
	return nil
}

// writeH1Reply writes a complete HTTP/1.1 response to w from a pre-built
// ordered header set + body. Phase 07.1 Task 18 prereq P2: the chain-mediated
// H1 dispatch path serializes the action's (post-encode-chain-mutated)
// response here. Phase 07.1 Task 19 (I-3 prereq): unified path — the
// SendLocalReply path and action-driven path BOTH use this single helper
// (collapsed from writeH1Reply{,Ordered} duplication). Iterates headers as a
// slice so caller-supplied insertion order (SPEC §11.2 verbatim 6-header
// order from cors.go on the SendLocalReply path; the four-header order from
// localReplyHeaders / clusterRouteAction's alphabetical projection on the
// action-driven path) lands on the wire byte-for-byte.
//
// Header emission discipline:
//  1. Status line: HTTP/1.1 <status> <reason>.
//  2. Iterate the headers slice in order. For each HeaderField, emit
//     "Name: Value\r\n" with canonical-cased name (textproto canonicalization).
//     Content-Length is overridden inline to match len(body) — the encode
//     chain may have mutated the body.
//  3. After the iteration, append Server and Date defaults if not already
//     present in the carrier. (chain.beginLocalReply does NOT inject these;
//     per ADR-0075 they are HCM-wire-write's responsibility.)
//  4. Blank line (CRLF) then body bytes.
//
// Empty Name HeaderFields are skipped defensively.
func writeH1Reply(w io.Writer, status int, headers filter_http.OrderedHeaders, body []byte) error {
	reason := http.StatusText(status)
	if _, err := fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", status, reason); err != nil {
		return err
	}
	hasServer := false
	hasDate := false
	for _, hf := range headers {
		if hf.Name == "" {
			continue
		}
		canon := textproto.CanonicalMIMEHeaderKey(hf.Name)
		val := hf.Value
		if canon == "Content-Length" {
			val = strconv.Itoa(len(body))
		}
		if canon == "Server" {
			hasServer = true
		}
		if canon == "Date" {
			hasDate = true
		}
		if _, err := fmt.Fprintf(w, "%s: %s\r\n", canon, val); err != nil {
			return err
		}
	}
	if !hasServer {
		if _, err := fmt.Fprintf(w, "Server: %s\r\n", serverHeader()); err != nil {
			return err
		}
	}
	if !hasDate {
		if _, err := fmt.Fprintf(w, "Date: %s\r\n", dateHeader()); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\r\n"); err != nil {
		return err
	}
	if len(body) > 0 {
		if _, err := w.Write(body); err != nil {
			return err
		}
	}
	return nil
}
