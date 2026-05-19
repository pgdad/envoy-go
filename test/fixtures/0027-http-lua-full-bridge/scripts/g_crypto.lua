-- Scenario (g) crypto :base64Escape per 22.2 SPEC §8.2 row (g) +
-- AMEND-22.2-1 (upstream-parity :base64Escape). Cross-side BYTE-EXACT
-- on the upstream-parity base64Escape return:
--   base64Escape("fixture-0027") = "Zml4dHVyZS0wMDI3"
-- envoy-go's additional :sha256 surface is envoy-go-strict (NOT in
-- upstream Envoy per D8 §13-R8) — exercised at the unit suite
-- (crypto_test.go) rather than cross-side here so the reference side's
-- script-load does not fail on the unknown method.
function envoy_on_request(rh)
  local b = rh:base64Escape("fixture-0027")
  rh:headers():add("x-base64", b)
end
