package tls

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

func TestLoadDataSource(t *testing.T) {
	t.Run("inline_bytes happy", func(t *testing.T) {
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte("hello")}}
		got, err := loadDataSource(ds, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("inline_string happy", func(t *testing.T) {
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "hello"}}
		got, err := loadDataSource(ds, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("filename absolute happy", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "data.txt")
		if err := os.WriteFile(path, []byte("abs"), 0o600); err != nil {
			t.Fatal(err)
		}
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: path}}
		got, err := loadDataSource(ds, "/unused/base")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "abs" {
			t.Errorf("got %q, want %q", got, "abs")
		}
	})

	t.Run("filename relative resolved against baseDir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "rel.txt"), []byte("rel"), 0o600); err != nil {
			t.Fatal(err)
		}
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: "rel.txt"}}
		got, err := loadDataSource(ds, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "rel" {
			t.Errorf("got %q, want %q", got, "rel")
		}
	})

	t.Run("filename nonexistent", func(t *testing.T) {
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: "/nonexistent/path"}}
		_, err := loadDataSource(ds, "")
		if err == nil || !strings.HasPrefix(err.Error(), "tls: data source: read ") {
			t.Errorf("want tls-prefixed read error, got: %v", err)
		}
	})

	t.Run("environment_variable errors", func(t *testing.T) {
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_EnvironmentVariable{EnvironmentVariable: "FOO"}}
		_, err := loadDataSource(ds, "")
		if err == nil || !strings.HasPrefix(err.Error(), "tls: data source: environment_variable is not supported") {
			t.Errorf("want not-supported error, got: %v", err)
		}
	})

	t.Run("zero value errors", func(t *testing.T) {
		ds := &corev3.DataSource{}
		_, err := loadDataSource(ds, "")
		if err == nil || !strings.HasPrefix(err.Error(), "tls: data source: none of inline_bytes") {
			t.Errorf("want zero-value error, got: %v", err)
		}
	})

	t.Run("large file read no truncation", func(t *testing.T) {
		dir := t.TempDir()
		big := make([]byte, 10*1024*1024)
		for i := range big {
			big[i] = byte(i % 251)
		}
		path := filepath.Join(dir, "big.bin")
		if err := os.WriteFile(path, big, 0o600); err != nil {
			t.Fatal(err)
		}
		ds := &corev3.DataSource{Specifier: &corev3.DataSource_Filename{Filename: path}}
		got, err := loadDataSource(ds, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(big) {
			t.Errorf("got len %d, want %d", len(got), len(big))
		}
	})
}
