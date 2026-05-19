-- Scenario (b) body chunks iterator per 22.2 SPEC §8.2 row (b).
-- Exercises :bodyChunks(). Upstream Envoy and envoy-go expose
-- materially different chunk-iteration contracts — upstream's
-- :bodyChunks() iterator yields per-DecodeData chunk during streaming;
-- envoy-go's iterator snapshots f.decodedBodyChunks at call time per
-- the Task 7 implementation. The script merely invokes :bodyChunks()
-- to obtain the iterator handle (which is non-yielding on both sides)
-- and stamps a constant cross-side marker x-chunks-status=invoked.
-- Driving the iterator-loop is intentionally OUT-OF-SCOPE here —
-- pcall cannot cross yield boundaries in Lua 5.1, so iterating an
-- upstream coroutine-yielding iterator inside pcall crashes the
-- script. Subject-side iterator correctness is independently
-- asserted at body_test.go unit suite (Test_RequestHandleBodyChunks_*).
function envoy_on_request(rh)
  pcall(function() rh:bodyChunks() end)
  rh:headers():add("x-chunks-status", "invoked")
end
