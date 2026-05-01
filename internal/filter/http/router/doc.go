// Package router provides envoy-go's terminal HTTP filter
// (envoy.filters.http.router). The filter dispatches the resolved route
// action — either dialing an upstream cluster and proxying the response, or
// synthesizing a direct_response when the route action is direct_response.
//
// Migrated from internal/filter/hcm/actions.go routerAction + routerActionH2
// at phase 07.1 (per ADR-0071's router-as-terminal-filter discipline). Tests
// are byte-preserved per BRAINSTORM §6.8 — imports + package names update;
// test bodies unchanged.
//
// Per the PLAN's Task 11 + Task 12 split: Task 11 lands the new package with
// passing byte-preserved tests while internal/filter/hcm/actions.go STILL
// contains the routerAction + routerActionH2 originals (the duplication is
// intentional). Task 12 deletes the old code in a clean separate commit.
package router
