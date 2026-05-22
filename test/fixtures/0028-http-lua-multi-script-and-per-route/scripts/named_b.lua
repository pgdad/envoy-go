-- fixture-0028 named_b script (source_codes registry entry "named_b").
-- Selected by a route with typed_per_filter_config LuaPerRoute{name:
-- "named_b"}. Distinct value from named_a proves multi-script selection
-- picks DISTINCT registry entries (scenario (e)).
function envoy_on_request(handle)
  handle:headers():add("x-lua-script", "named_b")
end
