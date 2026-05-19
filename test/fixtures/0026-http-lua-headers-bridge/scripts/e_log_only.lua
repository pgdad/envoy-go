-- Scenario (e) log-only-passthrough per phase 22.1 SPEC §9.1 row (e) +
-- D3 closure (option (a) at parent §11.7.7 RECOMMENDED): stat-counter
-- `lua.<prefix>.executions` delta IS the "Lua ran" assertion; literal
-- log line is supplementary, NOT cross-side asserted. Driver scrapes
-- /stats?format=text pre + post per probe + emits the delta.
function envoy_on_request(rh)
  rh:logInfo("lua hit")
end
