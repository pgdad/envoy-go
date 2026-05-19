-- Scenario (d) respond-shortcircuit per phase 22.1 SPEC §9.1 row (d) +
-- parent §11.6.7 + AMEND-7 wire-pin: status 403; body "denied" (6 bytes);
-- content-length: 6 auto-set; content-type: text/plain default; no
-- upstream request initiated.
function envoy_on_request(rh)
  rh:respond({[":status"]="403"}, "denied")
end
