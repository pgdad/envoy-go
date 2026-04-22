// Package bootstrap loads an Envoy v3 Bootstrap proto from YAML (the same YAML
// shape upstream Envoy accepts) and exposes skeleton-depth extractors the
// cmd/envoy-go subject uses to wire its listener, upstream endpoint, and admin
// surface. See docs/envoy-go/phases/01-static-bootstrap-config/SPEC.md §5.1.
package bootstrap
