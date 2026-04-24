package tls

import (
	"fmt"
	"os"
	"path/filepath"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// loadDataSource resolves a DataSource envelope into raw bytes. See ADR-0029
// for the phase-03 support matrix: inline_bytes, inline_string, filename are
// honoured; environment_variable errors; zero value errors; SDS-bound secret
// configs are handled at the caller layer (not reachable via this function).
//
// If filename is not absolute, it is resolved relative to baseDir. An empty
// baseDir combined with a relative filename resolves against the process's
// current working directory — callers should pass a stable baseDir (typically
// the directory of the bootstrap file) for reproducibility.
func loadDataSource(ds *corev3.DataSource, baseDir string) ([]byte, error) {
	switch s := ds.GetSpecifier().(type) {
	case *corev3.DataSource_InlineBytes:
		return s.InlineBytes, nil
	case *corev3.DataSource_InlineString:
		return []byte(s.InlineString), nil
	case *corev3.DataSource_Filename:
		p := s.Filename
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("tls: data source: read %s: %w", p, err)
		}
		return b, nil
	case *corev3.DataSource_EnvironmentVariable:
		return nil, fmt.Errorf("tls: data source: environment_variable is not supported in phase 03")
	default:
		return nil, fmt.Errorf("tls: data source: none of inline_bytes, inline_string, filename set")
	}
}
