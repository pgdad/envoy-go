package differential

import (
	"strings"
	"testing"
)

func TestParseEnvoyTarget_PullsTagAndDigest(t *testing.T) {
	src := `# envoy-go Reference Envoy Pin

**Tag:** ` + "`envoyproxy/envoy:v1.34.0`" + `
**SHA256:** ` + "`sha256:abc123def456`" + `
`
	pin, err := parseEnvoyTarget(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parseEnvoyTarget: %v", err)
	}
	if pin.Tag != "envoyproxy/envoy:v1.34.0" {
		t.Errorf("Tag: got %q", pin.Tag)
	}
	if pin.SHA256 != "sha256:abc123def456" {
		t.Errorf("SHA256: got %q", pin.SHA256)
	}
}

func TestParseEnvoyTarget_RejectsMissingTag(t *testing.T) {
	src := "no tag here\n**SHA256:** `sha256:abc`\n"
	if _, err := parseEnvoyTarget(strings.NewReader(src)); err == nil {
		t.Fatalf("parseEnvoyTarget accepted input without Tag")
	}
}
