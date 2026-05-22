-- fixture-0028 named_a script (source_codes registry entry "named_a").
-- Selected by a route with typed_per_filter_config LuaPerRoute{name:
-- "named_a"}. Deterministic header mutation; distinct value proves the
-- "named_a" registry entry was picked (vs default / named_b).
function envoy_on_request(handle)
  handle:headers():add("x-lua-script", "named_a")
end
