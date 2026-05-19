-- Scenario (f) connection-SSL fingerprint per 22.2 SPEC §8.2 row (f) +
-- §11.5 D5 closure option (f-B) cert-fingerprint-only cross-side. The
-- script extracts :sha256PeerCertificateDigest() — the SINGLE ssl method
-- that yields a byte-stable cross-side value when both sides present the
-- SAME cert PEM (verified at Task 17). Stamps the digest into the
-- response header for cross-side CompareBytes byte-exact assertion.
function envoy_on_request(rh)
  local ssl = rh:connection():ssl()
  local fp = "no-ssl"
  if ssl ~= nil then
    fp = ssl:sha256PeerCertificateDigest()
  end
  rh:headers():add("x-ssl-fp", fp)
end
