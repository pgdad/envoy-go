// env_vars.go — VmConfig.environment_variables assembly + WASI environ
// encoding per phase-25.3 Task 4 AMEND-C4.
//
// Upstream parity: cpp-host plugin.cc:25-51 assembles the guest env from two
// fields in the VmConfig:
//
//	host_env_keys  — names whose VALUES are read from the HOST process env
//	key_values     — explicit key→value pairs supplied by the operator
//
// Cross-field key collisions are REJECTED (NOT overridden) per plugin.cc:30-42.
// The assembled map feeds the WASI environ_get / environ_sizes_get shims as
// KEY=VALUE\0 entries (one NUL-terminated byte slice per entry).
//
// envoy-go-strict adds caps per AMEND-C4:
//   - max 64 env entries total (envVarsMaxEntries)
//   - max 4096 bytes per value  (envVarsMaxValueBytes)
//
// Exceeding either cap is REJECTED (ErrEnvVarsCapExceeded).

package wasm

import (
	"errors"
	"fmt"
	"os"
	"sort"
)

// envoy-go-strict caps per AMEND-C4.
const (
	envVarsMaxEntries    = 64
	envVarsMaxValueBytes = 4096
)

// ErrEnvVarsKeyCollision is returned by AssembleEnvVars when a key appears in
// BOTH host_env_keys and key_values (upstream parity: all keys must be unique;
// the collision is REJECTED, not overridden — cpp-host plugin.cc:30-42).
var ErrEnvVarsKeyCollision = errors.New("wasm: environment_variables: key duplicated across host_env_keys and key_values")

// ErrEnvVarsCapExceeded is returned by AssembleEnvVars when the assembled env
// exceeds the envoy-go-strict cap (max 64 entries OR any value > 4096 bytes).
var ErrEnvVarsCapExceeded = errors.New("wasm: environment_variables: exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)")

// AssembleEnvVars builds the guest-visible environment variable map from the
// two VmConfig sources, mirroring cpp-host plugin.cc:25-51 semantics with
// envoy-go-strict caps applied AFTER assembly.
//
// Assembly rules:
//  1. Cross-field collision: a key present in BOTH keyValues AND hostEnvKeys
//     causes a wrapped ErrEnvVarsKeyCollision return (the key name is embedded
//     in the error so errors.Is + a human-readable message both work).
//  2. All keyValues pairs are inserted verbatim into the result.
//  3. For each key in hostEnvKeys: the value is read from the HOST process
//     environment via os.LookupEnv; if absent the key is silently skipped
//     (LookupEnv distinguishes unset from set-to-empty; an absent var is NOT
//     inserted as "").
//  4. Cap enforcement (after full assembly):
//     - len(result) > envVarsMaxEntries → ErrEnvVarsCapExceeded
//     - any value whose len > envVarsMaxValueBytes → ErrEnvVarsCapExceeded
//
// Duplicates WITHIN hostEnvKeys (the same key listed twice) are harmless — the
// second lookup overwrites the first with an identical value.
func AssembleEnvVars(hostEnvKeys []string, keyValues map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(keyValues)+len(hostEnvKeys))

	// Step 1: collision check — iterate hostEnvKeys, test membership in keyValues.
	for _, key := range hostEnvKeys {
		if _, inKV := keyValues[key]; inKV {
			return nil, fmt.Errorf("key %q: %w", key, ErrEnvVarsKeyCollision)
		}
	}

	// Step 2: insert explicit key_values pairs.
	for k, v := range keyValues {
		result[k] = v
	}

	// Step 3: insert host-env values for present keys.
	for _, key := range hostEnvKeys {
		if val, ok := os.LookupEnv(key); ok {
			result[key] = val
		}
		// Absent host key: silently skipped per upstream parity.
	}

	// Step 4: cap enforcement.
	if len(result) > envVarsMaxEntries {
		return nil, fmt.Errorf("wasm: environment_variables: %d entries > max %d: %w",
			len(result), envVarsMaxEntries, ErrEnvVarsCapExceeded)
	}
	for k, v := range result {
		if len(v) > envVarsMaxValueBytes {
			return nil, fmt.Errorf("wasm: environment_variables: key %q value len %d > max %d bytes: %w",
				k, len(v), envVarsMaxValueBytes, ErrEnvVarsCapExceeded)
		}
	}

	return result, nil
}

// encodeWASIEnviron converts the assembled environment map into the byte-slice
// format expected by the WASI environ_get ABI: one []byte per entry of the
// form `KEY=VALUE\0` (KEY, '=', VALUE, single NUL terminator).
//
// Keys are sorted ascending before encoding — an envoy-go-strict determinism
// choice (upstream cpp-host iteration order is unordered/implementation-
// defined; sorted output is stable across Go map-iteration randomness and
// reproducible in tests + golden fixtures).
//
// Empty or nil env → nil slice (no entries to encode).
func encodeWASIEnviron(env map[string]string) [][]byte {
	if len(env) == 0 {
		return nil
	}

	// Collect and sort keys for deterministic output.
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([][]byte, len(keys))
	for i, k := range keys {
		v := env[k]
		// Build KEY=VALUE\0 as a single allocation.
		// len("KEY") + 1 ('=') + len("VALUE") + 1 ('\0')
		buf := make([]byte, len(k)+1+len(v)+1)
		copy(buf, k)
		buf[len(k)] = '='
		copy(buf[len(k)+1:], v)
		buf[len(k)+1+len(v)] = 0x00 // NUL terminator
		entries[i] = buf
	}
	return entries
}
