// Package directresponse implements the
// envoy.filters.network.direct_response network read-filter. At connection
// accept (OnNewConnection) it writes a fixed response body — resolved once at
// boot from the configured DataSource (inline_string / inline_bytes / filename
// baseDir-relative / environment_variable; D-P26.1-2) — back to the downstream
// connection with end_stream set, sets the "DirectResponse" response-code-detail
// on the per-connection callbacks (forward-consumer readiness; no operator
// surface at 26.1 per SPEC §2.10 / D-P26.1-5b), then closes the connection with
// FlushWrite semantics and returns StopIteration (SPEC §4.2). An absent or
// specifier-less response.DataSource is a boot-time PARSE-REJECT (§6.1;
// D-P26.1-3).
package directresponse
