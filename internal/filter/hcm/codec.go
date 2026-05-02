package hcm

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
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
// header set + body. Phase 07.1 Task 18 prereq P2: the chain-mediated H1
// dispatch path serializes the action's (post-encode-chain-mutated) response
// here. This replaces the previous "Action writes via bw directly" pattern.
//
// Header emission order:
//  1. Status line: HTTP/1.1 <status> <reason>
//  2. Content-Length is recomputed from len(body) (overrides any value in
//     headers — the encode chain may have mutated body).
//  3. Server + Date are stamped if absent (filters that mutate Server/Date
//     are honored).
//  4. All other headers from the headers map are emitted in arbitrary order
//     (Go map iteration; deterministic byte-equivalence with phase-04 is
//     preserved by the writeStatusReply path for the local-reply HCM-internal
//     synthesis call sites — only chain-mediated dispatch goes through here).
//  5. Blank line (CRLF) then body bytes.
func writeH1Reply(w io.Writer, status int, headers http.Header, body []byte) error {
	reason := http.StatusText(status)
	if _, err := fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", status, reason); err != nil {
		return err
	}
	// Ensure Content-Length matches the body bytes regardless of upstream value.
	headers = headers.Clone()
	headers.Set("Content-Length", strconv.Itoa(len(body)))
	if headers.Get("Server") == "" {
		headers.Set("Server", serverHeader())
	}
	if headers.Get("Date") == "" {
		headers.Set("Date", dateHeader())
	}
	if err := headers.Write(w); err != nil {
		return err
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
