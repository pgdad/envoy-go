package main

import (
	"strings"
	"testing"
)

func TestLoadConfig_Valid(t *testing.T) {
	src := `
listener:
  address: 0.0.0.0
  port: 10000
upstream:
  address: 127.0.0.1
  port: 19000
`
	cfg, err := loadConfig(strings.NewReader(src))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Listener.Address != "0.0.0.0" || cfg.Listener.Port != 10000 {
		t.Errorf("listener: got %+v", cfg.Listener)
	}
	if cfg.Upstream.Address != "127.0.0.1" || cfg.Upstream.Port != 19000 {
		t.Errorf("upstream: got %+v", cfg.Upstream)
	}
}

func TestLoadConfig_RejectsMissingFields(t *testing.T) {
	cases := map[string]string{
		"missing listener": `upstream: { address: 127.0.0.1, port: 19000 }`,
		"missing upstream": `listener: { address: 0.0.0.0, port: 10000 }`,
		"missing listener address": `listener: { port: 10000 }
upstream: { address: 127.0.0.1, port: 19000 }`,
		"port zero": `listener: { address: 0.0.0.0, port: 0 }
upstream: { address: 127.0.0.1, port: 19000 }`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadConfig(strings.NewReader(src)); err == nil {
				t.Fatalf("loadConfig succeeded; want error")
			}
		})
	}
}

func TestLoadConfig_RejectsUnknownFields(t *testing.T) {
	src := `
listener: { address: 0.0.0.0, port: 10000 }
upstream: { address: 127.0.0.1, port: 19000 }
extra: nope
`
	if _, err := loadConfig(strings.NewReader(src)); err == nil {
		t.Fatalf("loadConfig accepted unknown field; want error")
	}
}
