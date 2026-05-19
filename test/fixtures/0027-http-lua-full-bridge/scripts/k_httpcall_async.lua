-- Scenario (k) httpCall async fire-and-forget per 22.2 SPEC §8.2 row (k)
-- + AMEND-22.2-3 D6 closure. REFERENCE-LESS subject-only — the async
-- path returns 0 values, no yield, response discarded. The script body
-- runs synchronously despite the in-flight outbound call.
function envoy_on_request(rh)
  local ok = pcall(function()
    rh:httpCall("c_httpcall", {[":method"]="GET", [":path"]="/", [":authority"]="hc"}, "", 1000, true)
  end)
  rh:headers():add("x-httpcall-async", tostring(ok))
end
