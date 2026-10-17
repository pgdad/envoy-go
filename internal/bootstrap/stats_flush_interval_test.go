package bootstrap

import (
	"math"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// The three reference rejection classes for stats_flush_interval (phase 102).
const (
	flushNegPhrase   = "Invalid duration: Expected positive duration"
	flushOORPhrase   = "Invalid duration: Duration out-of-range"
	flushRangePhrase = "value must be inside range [1ms, 5m0s)"
)

var flushPhrases = []string{flushNegPhrase, flushOORPhrase, flushRangePhrase}

// flushStatsdSink is one statsd stats_sinks[] entry (bootstrap B1).
const flushStatsdSink = `stats_sinks:
  - name: envoy.stat_sinks.statsd
    typed_config:
      "@type": ` + statsdSinkType + `
      address:
        socket_address: {address: 127.0.0.1, port_value: 8125}
`

// flushBootstraps are B0 (no sink) and B1 (one statsd sink): the reference
// refuses an out-of-range value with or without a sink.
var flushBootstraps = []struct{ name, sinks string }{
	{"B0_no_sink", ""},
	{"B1_statsd_sink", flushStatsdSink},
}

// assertFlushReject asserts err carries exactly one class phrase (want) and
// the "bootstrap: " prefix FuzzBootstrapLoad requires.
func assertFlushReject(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want one containing %q", want)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "bootstrap: stats_flush_interval: ") {
		t.Errorf("error %q lacks the %q prefix", msg, "bootstrap: stats_flush_interval: ")
	}
	if !strings.Contains(msg, want) {
		t.Errorf("error %q does not contain %q", msg, want)
	}
	for _, other := range flushPhrases {
		if other != want && strings.Contains(msg, other) {
			t.Errorf("error %q also contains %q (want a single cause)", msg, other)
		}
	}
}

func TestStatsFlushInterval_YAMLReject(t *testing.T) {
	cases := []struct {
		value, want string
	}{
		{"0s", flushRangePhrase},
		{"0.0005s", flushRangePhrase},
		{"0.000999999s", flushRangePhrase},
		{"300s", flushRangePhrase},
		{"1000s", flushRangePhrase},
		{"-1s", flushNegPhrase},
		{"-0.5s", flushNegPhrase},
		{"9223372036s", flushOORPhrase},
		{"315576000000s", flushOORPhrase},
	}
	for _, b := range flushBootstraps {
		for _, c := range cases {
			t.Run(c.value+"/"+b.name, func(t *testing.T) {
				src := statsBootstrap("stats_flush_interval: " + c.value + "\n" + b.sinks)
				_, err := Load(strings.NewReader(src))
				assertFlushReject(t, err, c.want)
			})
		}
	}
}

func TestStatsFlushInterval_YAMLAccept(t *testing.T) {
	cases := []struct {
		name, line string
		want       time.Duration
	}{
		{"absent", "", 5 * time.Second},
		{"0.001s", "stats_flush_interval: 0.001s\n", time.Millisecond},
		{"0.0015s", "stats_flush_interval: 0.0015s\n", 1500 * time.Microsecond},
		{"0.5s", "stats_flush_interval: 0.5s\n", 500 * time.Millisecond},
		{"299.999999999s", "stats_flush_interval: 299.999999999s\n", 299*time.Second + 999999999},
	}
	for _, b := range flushBootstraps {
		for _, c := range cases {
			t.Run(c.name+"/"+b.name, func(t *testing.T) {
				bs, err := Load(strings.NewReader(statsBootstrap(c.line + b.sinks)))
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if bs.FlushInterval != c.want {
					t.Errorf("FlushInterval: got %v, want %v", bs.FlushInterval, c.want)
				}
			})
		}
	}
}

func TestStatsFlushInterval_GoBuilt(t *testing.T) {
	cases := []struct {
		name   string
		secs   int64
		nanos  int32
		reject string        // class phrase; "" means accept
		want   time.Duration // FlushInterval on accept
	}{
		{"0s_999999ns", 0, 999999, flushRangePhrase, 0},
		{"0s_1000000ns", 0, 1000000, "", time.Millisecond},
		{"299s_999999999ns", 299, 999999999, "", 299*time.Second + 999999999},
		{"300s_0ns", 300, 0, flushRangePhrase, 0},
		{"1s_-1ns", 1, -1, flushNegPhrase, 0},
		{"-1s_0ns", -1, 0, flushNegPhrase, 0},
		{"0s_1000000000ns", 0, 1000000000, flushOORPhrase, 0},
		{"1s_2147483647ns", 1, 2147483647, flushOORPhrase, 0},
		{"MaxInt64s_0ns", math.MaxInt64, 0, flushOORPhrase, 0},
		{"9223372035s_0ns", 9223372035, 0, flushRangePhrase, 0},
		{"9223372036s_0ns", 9223372036, 0, flushOORPhrase, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bs := &bootstrapv3.Bootstrap{StatsFlushInterval: &durationpb.Duration{Seconds: c.secs, Nanos: c.nanos}}
			result := &Bootstrap{}
			err := parseStatsSinks(bs, result)
			if c.reject != "" {
				assertFlushReject(t, err, c.reject)
				return
			}
			if err != nil {
				t.Fatalf("parseStatsSinks: %v", err)
			}
			if result.FlushInterval != c.want {
				t.Errorf("FlushInterval: got %v, want %v", result.FlushInterval, c.want)
			}
		})
	}
}
