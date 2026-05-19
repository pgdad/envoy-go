-- Scenario (b) replace-header per phase 22.1 SPEC §9.1 row (b).
-- Verbatim from SPEC: rh:headers():replace("user-agent", "envoy-go-lua/1.0").
function envoy_on_request(rh)
  rh:headers():replace("user-agent", "envoy-go-lua/1.0")
end
