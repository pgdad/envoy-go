-- Scenario (a) add-fixed-header per phase 22.1 SPEC §9.1 row (a).
-- Verbatim from SPEC: rh:headers():add("x-lua-injected", "hello").
function envoy_on_request(rh)
  rh:headers():add("x-lua-injected", "hello")
end
