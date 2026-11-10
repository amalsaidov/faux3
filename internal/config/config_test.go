package config

import (
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		AccessKeyID:     "faux3",
		SecretAccessKey: "faux3",
		Region:          "us-east-1",
		DataDir:         "/data",
		Port:            9000,
		Fsync:           true,
		MaxObjectSize:   5368709120,
		ClockSkew:       15 * time.Minute,
		LogLevel:        slog.LevelInfo,
	}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v, want %+v", c, want)
	}
	if c.CORSEnabled() {
		t.Fatal("CORS should be off by default")
	}
}

func TestInvalid(t *testing.T) {
	cases := []struct {
		name, key, val string
	}{
		{"port not int", "FAUX3_PORT", "abc"},
		{"port zero", "FAUX3_PORT", "0"},
		{"port too big", "FAUX3_PORT", "70000"},
		{"bool", "FAUX3_FSYNC", "maybe"},
		{"duration", "FAUX3_CLOCK_SKEW", "15"},
		{"duration negative", "FAUX3_CLOCK_SKEW", "-1m"},
		{"size", "FAUX3_MAX_OBJECT_SIZE", "5GB"},
		{"size negative", "FAUX3_MAX_OBJECT_SIZE", "-1"},
		{"log level", "FAUX3_LOG_LEVEL", "trace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(env(map[string]string{tc.key: tc.val}))
			if err == nil {
				t.Fatalf("expected error for %s=%q", tc.key, tc.val)
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("error %q does not name %s", err, tc.key)
			}
		})
	}
}

func TestSecretNotInErrors(t *testing.T) {
	_, err := Load(env(map[string]string{"FAUX3_SECRET_ACCESS_KEY": "s3cr3t-value", "FAUX3_PORT": "x"}))
	if err == nil || strings.Contains(err.Error(), "s3cr3t-value") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestOverrides(t *testing.T) {
	c, err := Load(env(map[string]string{
		"FAUX3_PORT":            "9100",
		"FAUX3_FSYNC":           "false",
		"FAUX3_MAX_OBJECT_SIZE": "1024",
		"FAUX3_CLOCK_SKEW":      "90s",
		"FAUX3_LOG_LEVEL":       "DEBUG",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != 9100 || c.Fsync || c.MaxObjectSize != 1024 || c.ClockSkew != 90*time.Second || c.LogLevel != slog.LevelDebug {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestCORSOrigins(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"*", []string{"*"}},
		{"http://a.test", []string{"http://a.test"}},
		{" http://a.test , http://b.test,,", []string{"http://a.test", "http://b.test"}},
	}
	for _, tc := range cases {
		c, err := Load(env(map[string]string{"FAUX3_CORS_ORIGINS": tc.raw}))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(c.CORSOrigins, tc.want) {
			t.Fatalf("%q: got %v, want %v", tc.raw, c.CORSOrigins, tc.want)
		}
		if c.CORSEnabled() != (tc.want != nil) {
			t.Fatalf("%q: CORSEnabled mismatch", tc.raw)
		}
	}
}
