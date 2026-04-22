// Package main is the phase-00 subject binary for the envoy-go differential test harness.
package main

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Config is the phase-00 minimal subject-binary configuration. Per ADR-0007,
// this schema is replaced by phase 01's real Envoy bootstrap loader.
type Config struct {
	Listener Endpoint `yaml:"listener"`
	Upstream Endpoint `yaml:"upstream"`
}

// Endpoint is a network address + port pair.
type Endpoint struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

func loadConfig(r io.Reader) (*Config, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true) // reject unknown keys per phase-00 strictness
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Listener.Address == "" {
		return fmt.Errorf("listener.address is required")
	}
	if c.Listener.Port <= 0 || c.Listener.Port > 65535 {
		return fmt.Errorf("listener.port must be 1..65535")
	}
	if c.Upstream.Address == "" {
		return fmt.Errorf("upstream.address is required")
	}
	if c.Upstream.Port <= 0 || c.Upstream.Port > 65535 {
		return fmt.Errorf("upstream.port must be 1..65535")
	}
	return nil
}
