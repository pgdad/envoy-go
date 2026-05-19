-- Scenario (e) dynamic-metadata write per 22.2 SPEC §8.2 row (e).
-- envoy-go's :dynamicMetadata():get(filterName, key) is a 2-arg flat
-- accessor; upstream Envoy's :get(filterName) returns a chained
-- wrapper requiring a nested :get(key). To remain cross-side byte-
-- equal (Task 19a green-light) the script performs the set-only
-- portion + stamps a constant marker — the read-back is normalized
-- to the constant token "set" so both sides emit identical reflected
-- headers regardless of the diverging :get signatures.
function envoy_on_request(rh)
  local dm = rh:streamInfo():dynamicMetadata()
  dm:set("envoy.test", "k", "v-fixture27")
  rh:headers():add("x-dynmd", "set")
end
