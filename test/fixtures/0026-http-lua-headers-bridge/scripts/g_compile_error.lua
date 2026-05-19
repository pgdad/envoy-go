-- Scenario (g) script-compile-error per phase 22.1 SPEC §9.1 row (g) +
-- AMEND-10 option 2 (substring-match `"script load error"`). The trailing
-- tokens after `end` are NOT valid Lua-5.1 syntax — gopher-lua's parser
-- raises a parse error at config-load + envoy-go's boot-reject path wraps
-- it with the `"script load error: "` prefix per §13-W + Task 15 wording-
-- pinning at cmd/envoy-go/main.go:60-66 (matching upstream Envoy v1.37.2
-- per parent §11.7.5). Reference Envoy (LuaJIT) raises the same parse
-- error wrapped with the same `"script load error: "` prefix at
-- source/extensions/filters/common/lua/lua.cc.
function envoy_on_request(rh) end this-is-not-valid-lua-syntax
