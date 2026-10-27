package config

import (
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port              int
	LogLevel          slog.Level
	DatabaseURL       string
	JWTSecret         string
	JWTExpiration     time.Duration
	BlobDir           string
	PublicDir         string
	BackgroundWorkers bool

	QWeatherAPIKey, QWeatherAPIHost string
	AIAPIKey, AIBaseURL, AIModel    string
	TTSAPIKey, TTSBaseURL, TTSModel string
}

func Load() (Config, error) {
	get := func(key, def string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return def
	}
	c := Config{
		DatabaseURL:     get("DATABASE_URL", ""),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		BlobDir:         get("BLOB_DIR", "./blobs"),
		PublicDir:       get("PUBLIC_DIR", "public"),
		QWeatherAPIKey:  get("QWEATHER_API_KEY", ""),
		QWeatherAPIHost: strings.TrimRight(get("QWEATHER_API_HOST", ""), "/"),
		AIAPIKey:        get("AI_API_KEY", ""),
		AIBaseURL:       strings.TrimRight(get("AI_BASE_URL", ""), "/"),
		AIModel:         get("AI_MODEL", "gpt-4o-mini"),
		TTSAPIKey:       get("TTS_API_KEY", ""),
		TTSBaseURL:      strings.TrimRight(get("TTS_BASE_URL", ""), "/"),
		TTSModel:        get("TTS_MODEL", "mimo-v2.5-tts"),
	}
	var err error
	if c.Port, err = strconv.Atoi(get("PORT", "9494")); err != nil || c.Port <= 0 || c.Port > 65535 {
		return c, fmt.Errorf("invalid PORT")
	}
	if c.LogLevel, err = LogLevel(); err != nil {
		return c, err
	}
	if u, err := url.Parse(c.DatabaseURL); err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return c, fmt.Errorf("DATABASE_URL must be a postgresql:// connection string")
	}
	if !strongSecret(c.JWTSecret) {
		return c, fmt.Errorf("JWT_SECRET must contain at least 32 characters with 16 distinct bytes and 128 bits of estimated entropy")
	}
	if c.JWTExpiration, err = ParseDuration(get("JWT_EXPIRATION", "7d")); err != nil {
		return c, fmt.Errorf("invalid JWT_EXPIRATION: %w", err)
	}
	if c.BackgroundWorkers, err = parseBool(get("BACKGROUND_WORKERS", "true")); err != nil {
		return c, fmt.Errorf("invalid BACKGROUND_WORKERS")
	}
	if c.QWeatherAPIHost != "" && !strings.HasPrefix(c.QWeatherAPIHost, "https://") && !strings.HasPrefix(c.QWeatherAPIHost, "http://") {
		return c, fmt.Errorf("QWEATHER_API_HOST must start with https://")
	}
	return c, nil
}

// LogLevel reads LOG_LEVEL: error, warn, info (default) or debug.
func LogLevel() (slog.Level, error) {
	switch os.Getenv("LOG_LEVEL") {
	case "error":
		return slog.LevelError, nil
	case "warn":
		return slog.LevelWarn, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid LOG_LEVEL")
	}
}

// ParseDuration accepts plain seconds or a number followed by s, m, h, d or w.
func ParseDuration(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second, nil
	}
	units := map[byte]time.Duration{'s': time.Second, 'm': time.Minute, 'h': time.Hour, 'd': 24 * time.Hour, 'w': 7 * 24 * time.Hour}
	if len(v) < 2 {
		return 0, fmt.Errorf("expected seconds or a duration like 15m, 1h, 7d")
	}
	unit, ok := units[v[len(v)-1]]
	n, err := strconv.ParseFloat(v[:len(v)-1], 64)
	if !ok || err != nil || n <= 0 {
		return 0, fmt.Errorf("expected seconds or a duration like 15m, 1h, 7d")
	}
	return time.Duration(n * float64(unit)), nil
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean")
}

func strongSecret(v string) bool {
	if len(v) < 32 {
		return false
	}
	counts := map[byte]int{}
	for i := 0; i < len(v); i++ {
		counts[v[i]]++
	}
	if len(counts) < 16 {
		return false
	}
	bits := 0.0
	for _, n := range counts {
		p := float64(n) / float64(len(v))
		bits -= p * math.Log2(p)
	}
	return bits*float64(len(v)) >= 128
}
