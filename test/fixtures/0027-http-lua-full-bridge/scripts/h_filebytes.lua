-- Scenario (h) fileBytes read per 22.2 SPEC §8.2 row (h) + D8 closure.
-- REFERENCE-LESS subject-only per D8 reclassification: reference Envoy
-- does NOT expose :fileBytes on the request_handle, so the script-load
-- on the ref side would raise nil-callable at runtime. The driver
-- probes ONLY the subject side; the byte stream emits a constant marker
-- on both sides. The script body guards on the method's existence so
-- the lua VM does not crash on the ref-side script-load if the runner
-- accidentally drives both sides.
function envoy_on_request(rh)
  local body = nil
  if rh.fileBytes ~= nil then
    body = rh:fileBytes("/etc/hostname")
  end
  local s = "absent"
  if body ~= nil then s = "present" end
  rh:headers():add("x-file-bytes", s)
end
