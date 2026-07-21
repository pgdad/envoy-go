-- Phase 70 fixture 0114 dynamic-metadata WRITER (cloned from
-- 0027-http-lua-full-bridge/scripts/e_dynamic_metadata.lua). Runs on BOTH
-- sides (reference Envoy contrib-v1.37.2 + subject envoy-go) BEFORE the router,
-- writing a FIXED, cross-side-identical value into the request dynamic-metadata
-- bucket under namespace "envoy.test", key "meta_k". The HCM tracing block's
-- REQUEST `metadata` custom_tag (meta_hit) resolves that value out of the same
-- bucket → the same serialized string on both sides. A FIXED value (not a
-- per-side env/header) is what makes the cross-side VALUE-equality assertion
-- possible (STRONGER than 0106's key-only environment tag).
function envoy_on_request(handle)
  handle:streamInfo():dynamicMetadata():set("envoy.test", "meta_k", "v-meta-0114")
end
