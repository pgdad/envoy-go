-- Scenario (l) timestamp wall-clock per 22.2 SPEC §8.2 row (l).
-- REFERENCE-LESS subject-only — wall-clock time is non-deterministic
-- across runs and across sides. The script reads :timestamp('milliseconds')
-- and stamps a presence-marker header (the actual numeric value is
-- discarded cross-side; the driver normalizes to a constant token).
function envoy_on_request(rh)
  local ts = rh:timestamp("milliseconds")
  local present = "no"
  if ts ~= nil and ts > 0 then present = "yes" end
  rh:headers():add("x-ts-present", present)
end
