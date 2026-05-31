// Package rbac is the shared RBAC principal/permission evaluation engine
// extracted from the HTTP rbac filter (phase-16) at the arrival of its second
// consumer (rbac_network, phase-26.3).
//
// Consumer #1: internal/filter/http/rbac — HTTP-layer RBAC enforcement.
// Consumer #2: internal/filter/network/rbac — network-layer RBAC enforcement.
//
// The package evaluates access-control rules against the abstract [EvalContext]
// interface. The Permission Large 11 and Principal Large 11 evaluator trees,
// the adapter helpers (matchString / matchHeader / matchPath / matchCidr), and
// the builder entry-points (buildOnePermission / buildOnePrincipal) are
// package-internal; both consumers reach them through the compiler/Evaluate
// surface (BuildRulesEngine / BuildMatcherEngine / Evaluate).
//
// The Profile input-capability parameter (distinguishing HTTP-only vs
// network-compatible permissions/principals) gates HTTP-only arms
// (header + url_path) at compile time: ProfileL4 PARSE-REJECTs them;
// ProfileHTTP permits all arms.
//
// Per ADR-0216.
package rbac
