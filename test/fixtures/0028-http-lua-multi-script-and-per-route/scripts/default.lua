-- fixture-0028 default script (listener default_source_code).
-- Deterministic header mutation only — NO :timestamp() / :httpCall() /
-- non-deterministic API per phase 22.3 IMPL Task 5. The echobackend
-- reflects the request headers as JSON; the driver asserts x-lua-script
-- arrived with the value stamped by THIS default script. Used by:
--   (a) listener-default: route with NO per-route config → this runs.
--   (d) per-route disabled: this default does NOT run (hooks skipped).
function envoy_on_request(handle)
  handle:headers():add("x-lua-script", "default")
end
