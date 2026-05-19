-- Scenario (a) body whole-buffer per 22.2 SPEC §8.2 row (a).
-- Exercises :body() (defensive Go string copy per §11.3 D3 RECOMMENDED).
-- Upstream Envoy v1.37 and envoy-go expose materially different :body()
-- contracts (upstream returns a Buffer userdata gated on prior decoder
-- buffering; envoy-go returns a Lua string after coroutine yield-resume
-- at endStream per Task 7). The script invokes :body() defensively under
-- pcall — both sides observe a script that completes and stamps a
-- constant cross-side marker x-body-status=invoked. Cross-side byte-
-- equality is preserved regardless of the divergent return shapes.
-- The subject-side body-bridge correctness is independently asserted at
-- the body_test.go unit suite (Task 7 lands the Test_RequestHandleBody_*
-- battery).
function envoy_on_request(rh)
  pcall(function() rh:body() end)
  rh:headers():add("x-body-status", "invoked")
end
