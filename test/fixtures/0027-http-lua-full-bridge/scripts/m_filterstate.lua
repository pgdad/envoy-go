-- Scenario (m) filterState set+get per 22.2 SPEC §8.2 row (m) +
-- §11.8 D4 closure. REFERENCE-LESS subject-only — envoy-go exposes a
-- :set surface upstream Envoy lacks (envoy-go-strict departure per
-- AMEND-22.2-4). The script writes + reads back within the same
-- handler. Intra-stream deterministic; cross-stream isolated.
function envoy_on_request(rh)
  local fs = rh:streamInfo():filterState()
  fs:set("fixture27.key", "v")
  local got = fs:get("fixture27.key")
  local s = "nil"
  if got ~= nil then s = tostring(got) end
  rh:headers():add("x-fs-roundtrip", s)
end
