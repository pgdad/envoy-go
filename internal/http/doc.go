// Package http is a phase-00 placeholder. Phase 04 landed the HTTP connection
// manager + route match + router action + direct_response under
// internal/filter/hcm/. This package is retained as a stable import target if
// future code needs HTTP utilities that are not HCM-specific (e.g., a shared
// HTTP/2 or HTTP/3 helper layer when those phases land). It contains no
// symbols at phase 04. See SPEC §10 #10 settled choice.
package http
