package config

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"3600": time.Hour, "15m": 15 * time.Minute, "1.5h": 90 * time.Minute, "7d": 7 * 24 * time.Hour, "2w": 14 * 24 * time.Hour,
	} {
		if got, err := ParseDuration(in); err != nil || got != want {
			t.Errorf("%q: got %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "d", "0", "-1h", "NaNd", "Infh", "+Inf", "9223372036854775807", "1e300w", "1e-12s", "7x"} {
		if got, err := ParseDuration(in); err == nil {
			t.Errorf("%q: got %v, want error", in, got)
		}
	}
}
