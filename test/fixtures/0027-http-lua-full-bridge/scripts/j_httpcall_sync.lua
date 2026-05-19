-- Scenario (j) httpCall sync per 22.2 SPEC §8.2 row (j). REFERENCE-LESS
-- subject-only per non-deterministic transport (timing-dependent). The
-- driver probes only the subject; the script attempts a sync httpCall
-- but tolerates failure (asynchronous=nil → yield+resume; envoy-go's
-- response timing differs from reference Envoy's). Stamps a marker
-- header that the driver normalizes to a constant token cross-side.
function envoy_on_request(rh)
  local ok = pcall(function()
    rh:httpCall("c_httpcall", {[":method"]="GET", [":path"]="/", [":authority"]="hc"}, "", 1000)
  end)
  rh:headers():add("x-httpcall-sync", tostring(ok))
end
