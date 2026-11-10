// Package config loads faux3 settings from FAUX3_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Config holds the validated server settings.
type Config struct {
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	DataDir         string
	Port            int
	CORSOrigins     []string
	Fsync           bool
	MaxObjectSize   int64
	ClockSkew       time.Duration
	LogLevel        slog.Level
}

// CORSEnabled reports whether any CORS origin is configured.
func (c Config) CORSEnabled() bool { return len(c.CORSOrigins) > 0 }

// Load builds a Config from getenv, applying defaults for unset or empty
// variables. Errors name the offending variable and never echo secrets.
func Load(getenv func(string) string) (Config, error) {
	get := func(name, def string) string {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		AccessKeyID:     get("FAUX3_ACCESS_KEY_ID", "faux3"),
		SecretAccessKey: get("FAUX3_SECRET_ACCESS_KEY", "faux3"),
		Region:          get("FAUX3_REGION", "us-east-1"),
		DataDir:         get("FAUX3_DATA_DIR", "/data"),
	}

	port, err := strconv.Atoi(get("FAUX3_PORT", "9000"))
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("FAUX3_PORT: must be an integer between 1 and 65535")
	}
	c.Port = port

	c.CORSOrigins = parseOrigins(getenv("FAUX3_CORS_ORIGINS"))

	fsync, err := strconv.ParseBool(get("FAUX3_FSYNC", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("FAUX3_FSYNC: must be a boolean (true or false)")
	}
	c.Fsync = fsync

	size, err := strconv.ParseInt(get("FAUX3_MAX_OBJECT_SIZE", "5368709120"), 10, 64)
	if err != nil || size < 0 {
		return Config{}, fmt.Errorf("FAUX3_MAX_OBJECT_SIZE: must be a non-negative integer number of bytes")
	}
	c.MaxObjectSize = size

	skew, err := time.ParseDuration(get("FAUX3_CLOCK_SKEW", "15m"))
	if err != nil || skew <= 0 {
		return Config{}, fmt.Errorf("FAUX3_CLOCK_SKEW: must be a positive Go duration such as 15m")
	}
	c.ClockSkew = skew

	switch strings.ToLower(get("FAUX3_LOG_LEVEL", "info")) {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		return Config{}, fmt.Errorf("FAUX3_LOG_LEVEL: must be one of debug, info, warn, error")
	}
	return c, nil
}

func parseOrigins(raw string) []string {
	var out []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}
