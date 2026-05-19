-- Scenario (c) trailers add+remove per 22.2 SPEC §8.2 row (c).
-- Exercises the trailers metatable's 8-method mutation surface. The
-- request-decode side stamps a marker header capturing whether the
-- trailers userdata was returned non-nil (downstream may not send
-- trailers; both sides observe the same nil/non-nil status).
function envoy_on_request(rh)
  local trailers = rh:trailers()
  local present = "nil"
  if trailers ~= nil then
    present = "present"
    trailers:add("x-lua-trailer", "added")
    trailers:remove("x-lua-trailer")
  end
  rh:headers():add("x-trailers-status", present)
end
