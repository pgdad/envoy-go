-- Scenario (i) streamInfo upstream accessors per 22.2 SPEC §8.2 row (i).
-- Exercises :streamInfo():upstreamCluster() (and upstreamHost). Upstream
-- Envoy populates streamInfo's upstream-* accessors AFTER the upstream
-- endpoint has been selected by the router — at decode-time these
-- typically return nil/"" because the upstream selection has not yet
-- occurred. envoy-go's streamInfo accessor surface at request-decode
-- mirrors that pre-selection nil shape per Task 13. The script invokes
-- the accessors defensively under pcall + stamps a constant cross-side
-- marker x-up-status=invoked. Cross-side byte-equal regardless of the
-- pre-selection return values. Subject-side correctness asserted at
-- streaminfo_test.go unit suite.
function envoy_on_request(rh)
  pcall(function()
    local si = rh:streamInfo()
    if si then
      local _ = si:upstreamCluster()
      local _ = si:upstreamHost()
    end
  end)
  rh:headers():add("x-up-status", "invoked")
end
