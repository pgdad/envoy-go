-- Scenario (f) headers-iteration per phase 22.1 SPEC §9.1 row (f) +
-- §11.2 D7 closure: bridge `__pairs` snapshots http.Header into an
-- alphabetically-sorted Go slice + walks by integer index. Count-only
-- assertion is order-independent + deterministic across runs on both
-- sides (cross-side byte-exact for `x-headers-count: N` HOLDS because
-- both sides count the same N regardless of iteration order).
function envoy_on_request(rh)
  local n = 0
  for k, v in pairs(rh:headers()) do
    n = n + 1
  end
  rh:headers():add("x-headers-count", tostring(n))
end
