-- Scenario (d) metadata empty-userdata at binding-gap per 22.2 SPEC §8.2
-- row (d) + §11.6 D1 closure. Bridge surface callable but returns empty
-- userdata until v1.37.x binding bump activates the source-of-data —
-- :get("k") returns nil; pairs() yields 0 iterations.
function envoy_on_request(rh)
  local md = rh:metadata()
  local v = md:get("k")
  local got = "nil"
  if v ~= nil then got = tostring(v) end
  local n = 0
  for k, val in pairs(md) do n = n + 1 end
  rh:headers():add("x-md-get", got)
  rh:headers():add("x-md-pairs", tostring(n))
end
