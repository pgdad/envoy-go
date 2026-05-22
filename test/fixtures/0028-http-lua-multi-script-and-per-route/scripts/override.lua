-- fixture-0028 per-route source_code override script (scenario (c)).
-- Spliced INLINE into a route's typed_per_filter_config LuaPerRoute{
-- source_code: {inline_string: <this file's content>}}. The inline
-- override runs INSTEAD of the listener default; the distinct value
-- proves the per-route source_code arm took effect.
function envoy_on_request(handle)
  handle:headers():add("x-lua-script", "override")
end
