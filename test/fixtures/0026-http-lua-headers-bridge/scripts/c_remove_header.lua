-- Scenario (c) remove-header per phase 22.1 SPEC §9.1 row (c).
-- Verbatim from SPEC: rh:headers():remove("x-blocked").
function envoy_on_request(rh)
  rh:headers():remove("x-blocked")
end
