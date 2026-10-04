// Package config loads and validates deployment configuration.
package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

var errInvalid = errors.New("invalid configuration")

var environmentKeys = map[string]string{
	"IMGNEST_SERVER_ADDR":                "server.addr",
	"IMGNEST_SERVER_TRUSTED_PROXIES":     "server.trusted_proxies",
	"IMGNEST_SERVER_READ_HEADER_TIMEOUT": "server.read_header_timeout",
	"IMGNEST_SERVER_SHUTDOWN_TIMEOUT":    "server.shutdown_timeout",
	"IMGNEST_SERVER_MAX_UPLOAD_MB":       "server.max_upload_mb",
	"IMGNEST_SERVER_MAX_REQUEST_MB":      "server.max_request_mb",
	"IMGNEST_SERVER_UPLOAD_CONCURRENCY":  "server.upload_concurrency",
	"IMGNEST_SERVER_PROCESSING_TIMEOUT":  "server.processing_timeout",
	"IMGNEST_SERVER_MAX_PIXELS":          "server.max_pixels",
	"IMGNEST_IMAGES_THUMB_CACHE":         "images.thumb_cache",
	"IMGNEST_SECURITY_MASTER_KEY":        "security.master_key",
	"IMGNEST_DATABASE_DRIVER":            "database.driver",
	"IMGNEST_DATABASE_DSN":               "database.dsn",
	"IMGNEST_DATABASE_MAX_OPEN":          "database.max_open",
	"IMGNEST_DATABASE_MAX_IDLE":          "database.max_idle",
	"IMGNEST_DATABASE_MAX_LIFETIME":      "database.max_lifetime",
	"IMGNEST_DATABASE_BUSY_TIMEOUT":      "database.busy_timeout",
}

// Config contains the HTTP server and database deployment settings.
type Config struct {
	Server   Server
	Database Database
	Images   Images
	Security Security
}

// Server configures HTTP serving and graceful shutdown.
type Server struct {
	Addr              string
	TrustedProxies    []string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
	MaxUploadMB       int
	MaxRequestMB      int
	UploadConcurrency int
	ProcessingTimeout time.Duration
	MaxPixels         int
}

// Images configures the replaceable local preview cache.
type Images struct{ ThumbCache string }

// Security contains the deployment key used for encrypted cloud credentials.
type Security struct{ MasterKey string }

// Database configures the selected database and its connection pool.
type Database struct {
	Driver      string
	DSN         string
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
	BusyTimeout time.Duration
}

// Load applies defaults, an optional explicit YAML file, and the supplied environment.
// A blank path skips the file; nil environ ignores the process environment.
// Errors identify fields without exposing configuration values or file paths.
func Load(ctx context.Context, path string, environ []string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, fmt.Errorf("load configuration: %w", err)
	}

	k := koanf.New(".")
	if path != "" {
		err := k.Load(file.Provider(path), yaml.Parser())
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Config{}, fmt.Errorf("load configuration: %w", ctxErr)
		}
		if err != nil {
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				return Config{}, fmt.Errorf("read configuration file: %w", pathErr.Err)
			}
			// YAML diagnostics may quote DSNs, so do not retain their raw error.
			return Config{}, fmt.Errorf("parse configuration YAML: %w", errInvalid)
		}
		for _, section := range []string{"server", "database", "images", "security"} {
			if k.Exists(section) {
				if _, ok := k.Get(section).(map[string]any); !ok {
					return Config{}, invalid(section, "must be a mapping")
				}
			}
		}
	}

	assignments := make([]string, 0, len(environ))
	for _, assignment := range environ {
		name, _, hasValue := strings.Cut(assignment, "=")
		if _, known := environmentKeys[name]; !known {
			continue
		}
		if !hasValue {
			return Config{}, invalid(environmentKeys[name], "environment assignment requires an equals sign")
		}
		assignments = append(assignments, assignment)
	}
	if err := k.Load(env.Provider(".", env.Opt{
		Prefix:      "IMGNEST_",
		EnvironFunc: func() []string { return assignments },
		TransformFunc: func(name, value string) (string, any) {
			key := environmentKeys[name]
			if key == "server.trusted_proxies" {
				proxies := []string{}
				if value != "" {
					for _, proxy := range strings.Split(value, ",") {
						proxies = append(proxies, strings.TrimSpace(proxy))
					}
				}
				return key, proxies
			}
			return key, value
		},
	}), nil); err != nil {
		return Config{}, fmt.Errorf("load configuration environment: %w", errInvalid)
	}

	driver := "sqlite"
	if k.Exists("database.driver") {
		value, ok := k.Get("database.driver").(string)
		if !ok {
			return Config{}, invalid("database.driver", "must be a string")
		}
		driver = value
	}
	if driver != "sqlite" && driver != "postgres" {
		return Config{}, invalid("database.driver", "must be sqlite or postgres")
	}

	defaults := map[string]any{
		"server.addr":                ":8080",
		"server.trusted_proxies":     []string{},
		"server.read_header_timeout": "5s",
		"server.shutdown_timeout":    "10s",
		"server.max_upload_mb":       20,
		"server.max_request_mb":      64,
		"server.upload_concurrency":  2,
		"server.processing_timeout":  "5m",
		"server.max_pixels":          100000000,
		"images.thumb_cache":         "data/thumbs",
		"security.master_key":        "",
		"database.driver":            driver,
		"database.dsn":               "data/imgnest.db",
		"database.max_open":          1,
		"database.max_idle":          1,
		"database.max_lifetime":      "0s",
		"database.busy_timeout":      "5000ms",
	}
	if driver == "postgres" {
		defaults["database.dsn"] = ""
		defaults["database.max_open"] = 25
		defaults["database.max_idle"] = 10
		defaults["database.max_lifetime"] = "5m"
	}
	// Fill only missing fields to preserve YAML scalar types and explicit zero values.
	for key := range defaults {
		if k.Exists(key) {
			delete(defaults, key)
		}
	}
	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		return Config{}, fmt.Errorf("load configuration defaults: %w", errInvalid)
	}

	cfg, err := decode(k)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Config{}, fmt.Errorf("load configuration: %w", ctxErr)
	}
	if err != nil {
		return Config{}, err
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func decode(k *koanf.Koanf) (Config, error) {
	cfg := Config{}
	for _, field := range []struct {
		key   string
		value *string
	}{
		{key: "server.addr", value: &cfg.Server.Addr},
		{key: "database.driver", value: &cfg.Database.Driver},
		{key: "database.dsn", value: &cfg.Database.DSN},
		{key: "images.thumb_cache", value: &cfg.Images.ThumbCache},
		{key: "security.master_key", value: &cfg.Security.MasterKey},
	} {
		value, ok := k.Get(field.key).(string)
		if !ok {
			return Config{}, invalid(field.key, "must be a string")
		}
		*field.value = value
	}
	for _, field := range []struct {
		key   string
		value *int
	}{
		{key: "database.max_open", value: &cfg.Database.MaxOpen},
		{key: "database.max_idle", value: &cfg.Database.MaxIdle},
		{key: "server.max_upload_mb", value: &cfg.Server.MaxUploadMB},
		{key: "server.max_request_mb", value: &cfg.Server.MaxRequestMB},
		{key: "server.upload_concurrency", value: &cfg.Server.UploadConcurrency},
		{key: "server.max_pixels", value: &cfg.Server.MaxPixels},
	} {
		switch value := k.Get(field.key).(type) {
		case int:
			*field.value = value
		case string:
			number, err := strconv.Atoi(value)
			if err != nil {
				return Config{}, invalid(field.key, "must be an integer")
			}
			*field.value = number
		default:
			return Config{}, invalid(field.key, "must be an integer")
		}
	}
	for _, field := range []struct {
		key   string
		value *time.Duration
	}{
		{key: "server.read_header_timeout", value: &cfg.Server.ReadHeaderTimeout},
		{key: "server.shutdown_timeout", value: &cfg.Server.ShutdownTimeout},
		{key: "database.max_lifetime", value: &cfg.Database.MaxLifetime},
		{key: "database.busy_timeout", value: &cfg.Database.BusyTimeout},
		{key: "server.processing_timeout", value: &cfg.Server.ProcessingTimeout},
	} {
		value, ok := k.Get(field.key).(string)
		if !ok {
			return Config{}, invalid(field.key, "must be a duration string")
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, invalid(field.key, "must be a duration string")
		}
		*field.value = duration
	}
	cfg.Server.TrustedProxies = []string{}
	switch values := k.Get("server.trusted_proxies").(type) {
	case []string:
		cfg.Server.TrustedProxies = append(cfg.Server.TrustedProxies, values...)
	case []any:
		for _, value := range values {
			proxy, ok := value.(string)
			if !ok {
				return Config{}, invalid("server.trusted_proxies", "must be a list of IP addresses or CIDRs")
			}
			cfg.Server.TrustedProxies = append(cfg.Server.TrustedProxies, proxy)
		}
	default:
		return Config{}, invalid("server.trusted_proxies", "must be a list of IP addresses or CIDRs")
	}
	return cfg, nil
}

func validate(cfg Config) error {
	if cfg.Server.MaxUploadMB < 1 || cfg.Server.MaxUploadMB > 20 {
		return invalid("server.max_upload_mb", "must be between 1 and 20")
	}
	if cfg.Server.MaxRequestMB < cfg.Server.MaxUploadMB || cfg.Server.MaxRequestMB > 256 {
		return invalid("server.max_request_mb", "must cover one file and be at most 256")
	}
	if cfg.Server.UploadConcurrency < 1 || cfg.Server.UploadConcurrency > 64 {
		return invalid("server.upload_concurrency", "must be between 1 and 64")
	}
	if cfg.Server.MaxPixels < 1 || cfg.Server.MaxPixels > 100000000 {
		return invalid("server.max_pixels", "must be between 1 and 100000000")
	}
	if cfg.Server.ProcessingTimeout <= 0 || cfg.Server.ProcessingTimeout > 30*time.Minute {
		return invalid("server.processing_timeout", "must be positive and at most 30m")
	}
	if strings.TrimSpace(cfg.Images.ThumbCache) == "" {
		return invalid("images.thumb_cache", "must not be empty")
	}
	if cfg.Security.MasterKey != "" {
		key, err := base64.StdEncoding.DecodeString(cfg.Security.MasterKey)
		if err != nil || len(key) != 32 {
			return invalid("security.master_key", "must encode exactly 32 bytes")
		}
	}
	_, port, err := net.SplitHostPort(cfg.Server.Addr)
	if err != nil {
		return invalid("server.addr", "must be a host and port")
	}
	number, err := strconv.Atoi(port)
	invalidPort := number < 0 || number > 65535
	if err != nil || invalidPort {
		return invalid("server.addr", "must contain a valid port")
	}
	for _, proxy := range cfg.Server.TrustedProxies {
		if _, err := netip.ParseAddr(proxy); err == nil {
			continue
		}
		if _, err := netip.ParsePrefix(proxy); err != nil {
			return invalid("server.trusted_proxies", "must contain only IP addresses or CIDRs")
		}
	}
	if cfg.Server.ReadHeaderTimeout <= 0 {
		return invalid("server.read_header_timeout", "must be positive")
	}
	if cfg.Server.ShutdownTimeout <= 0 {
		return invalid("server.shutdown_timeout", "must be positive")
	}
	if strings.TrimSpace(cfg.Database.DSN) == "" {
		return invalid("database.dsn", "is required")
	}
	if cfg.Database.MaxOpen <= 0 {
		return invalid("database.max_open", "must be positive")
	}
	if cfg.Database.MaxIdle < 0 || cfg.Database.MaxIdle > cfg.Database.MaxOpen {
		return invalid("database.max_idle", "must be between zero and max_open")
	}
	if cfg.Database.MaxLifetime < 0 {
		return invalid("database.max_lifetime", "must not be negative")
	}
	if cfg.Database.BusyTimeout < 0 {
		return invalid("database.busy_timeout", "must not be negative")
	}
	return nil
}

func invalid(field, reason string) error {
	return fmt.Errorf("configuration %s %s: %w", field, reason, errInvalid)
}
