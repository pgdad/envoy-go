package differential

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// EnvoyPin captures the upstream image identity from ENVOY_TARGET.md.
type EnvoyPin struct {
	Tag    string // e.g. envoyproxy/envoy:v1.34.0
	SHA256 string // e.g. sha256:<hex>
}

var (
	tagLineRE    = regexp.MustCompile(`(?m)^\*\*Tag:\*\*\s+` + "`" + `([^` + "`" + `]+)` + "`")
	sha256LineRE = regexp.MustCompile(`(?m)^\*\*SHA256:\*\*\s+` + "`" + `([^` + "`" + `]+)` + "`")
)

func parseEnvoyTarget(r io.Reader) (*EnvoyPin, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	tagM := tagLineRE.FindSubmatch(src)
	if tagM == nil {
		return nil, fmt.Errorf("ENVOY_TARGET.md: missing **Tag:** line")
	}
	shaM := sha256LineRE.FindSubmatch(src)
	if shaM == nil {
		return nil, fmt.Errorf("ENVOY_TARGET.md: missing **SHA256:** line")
	}
	return &EnvoyPin{Tag: string(tagM[1]), SHA256: string(shaM[1])}, nil
}

// (More to come in Tasks 10–11.)

// readyTimeout is the wall-clock budget the harness allows each proxy to
// declare itself ready (admin /ready 200 for the reference, ready sentinel on
// stdout for the subject). Generous on purpose; SPEC §11 mitigates flakiness
// by surfacing failures, not retrying.
const readyTimeout = 30 * time.Second

// scanForLine reads lines from r until one of `needle` substrings appears or
// ctx is done. Returns the matching full line.
func scanForLine(ctx context.Context, r io.Reader, needle string) (string, error) {
	br := bufio.NewReader(r)
	out := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		for {
			line, err := br.ReadString('\n')
			if strings.Contains(line, needle) {
				out <- line
				return
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()
	select {
	case line := <-out:
		return line, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
