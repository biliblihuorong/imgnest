// Package config loads and validates deployment configuration.
package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
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
	"IMGNEST_SERVER_ADDR":                      "server.addr",
	"IMGNEST_SERVER_TRUSTED_PROXIES":           "server.trusted_proxies",
	"IMGNEST_SERVER_READ_HEADER_TIMEOUT":       "server.read_header_timeout",
	"IMGNEST_SERVER_SHUTDOWN_TIMEOUT":          "server.shutdown_timeout",
	"IMGNEST_SERVER_MAX_UPLOAD_MB":             "server.max_upload_mb",
	"IMGNEST_SERVER_MAX_REQUEST_MB":            "server.max_request_mb",
	"IMGNEST_SERVER_UPLOAD_CONCURRENCY":        "server.upload_concurrency",
	"IMGNEST_SERVER_PROCESSING_TIMEOUT":        "server.processing_timeout",
	"IMGNEST_SERVER_MAX_PIXELS":                "server.max_pixels",
	"IMGNEST_IMAGES_THUMB_CACHE":               "images.thumb_cache",
	"IMGNEST_IMAGES_VARIANT_CORS_ORIGINS":      "images.variant_cors_origins",
	"IMGNEST_IMAGES_VARIANT_LOOKUP_PER_MINUTE": "images.variant_lookup_per_minute",
	"IMGNEST_IMAGES_VARIANT_CDN_MAX_AGE":       "images.variant_cdn_max_age",
	"IMGNEST_CACHE_REDIS_URL":                  "cache.redis_url",
	"IMGNEST_SECURITY_MASTER_KEY":              "security.master_key",
	"IMGNEST_DATABASE_DRIVER":                  "database.driver",
	"IMGNEST_DATABASE_DSN":                     "database.dsn",
	"IMGNEST_DATABASE_MAX_OPEN":                "database.max_open",
	"IMGNEST_DATABASE_MAX_IDLE":                "database.max_idle",
	"IMGNEST_DATABASE_MAX_LIFETIME":            "database.max_lifetime",
	"IMGNEST_DATABASE_BUSY_TIMEOUT":            "database.busy_timeout",
	"IMGNEST_BACKUP_DIR":                       "backup.dir",
	"IMGNEST_UPDATES_CHECK":                    "updates.check",
	"IMGNEST_LOG_LEVEL":                        "log.level",
	"IMGNEST_LOG_SLOW_QUERY":                   "log.slow_query",
	"IMGNEST_LOG_AUDIT_DAYS":                   "log.audit_days",
}

// Config contains the HTTP server and database deployment settings.
type Config struct {
	Server   Server
	Database Database
	Images   Images
	Security Security
	Backup   Backup
	Updates  Updates
	Cache    Cache
	Log      Log
}

// Log configures the process log on stdout and the operation audit trail.
type Log struct {
	// Level is the lowest level written: debug, info, warn or error.
	Level string
	// SlowQuery logs database statements slower than this at warn level;
	// zero turns slow-query logging off.
	SlowQuery time.Duration
	// AuditDays is how long operation audit entries are kept.
	AuditDays int
}

// Cache selects where random-link candidates and variant lookups are cached.
type Cache struct {
	// RedisURL (redis:// or rediss://) shares the cache between instances;
	// empty keeps it in this process's memory.
	RedisURL string
}

// Updates configures the console's check for newer releases.
type Updates struct {
	// Check lets administrators see whether a newer release is published on
	// GitHub; turn it off on servers that must not call out.
	Check bool
}

// Backup configures where database and configuration backups are written.
type Backup struct{ Dir string }

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

// Images configures the replaceable local preview cache and the public
// responsive-variant lookup.
type Images struct {
	ThumbCache string
	// VariantCORSOrigins are the browser origins (such as a blog) allowed to
	// read variant lookups; "*" allows any origin, empty allows none.
	VariantCORSOrigins []string
	// VariantLookupPerMinute bounds lookups per client address.
	VariantLookupPerMinute int
	// VariantCDNMaxAge is how long a CDN may keep a lookup answer
	// (s-maxage); zero leaves CDNs on the 5-minute browser lifetime.
	VariantCDNMaxAge time.Duration
}

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
		for _, section := range []string{"server", "database", "images", "security", "backup", "updates", "cache", "log"} {
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
			if key == "server.trusted_proxies" || key == "images.variant_cors_origins" {
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
		"server.addr":                      ":8080",
		"server.trusted_proxies":           []string{},
		"server.read_header_timeout":       "5s",
		"server.shutdown_timeout":          "10s",
		"server.max_upload_mb":             20,
		"server.max_request_mb":            64,
		"server.upload_concurrency":        2,
		"server.processing_timeout":        "5m",
		"server.max_pixels":                100000000,
		"images.thumb_cache":               "data/thumbs",
		"images.variant_cors_origins":      []string{},
		"images.variant_lookup_per_minute": 60,
		"images.variant_cdn_max_age":       "1h",
		"cache.redis_url":                  "",
		"security.master_key":              "",
		"backup.dir":                       "data/backup",
		"updates.check":                    true,
		"log.level":                        "info",
		"log.slow_query":                   "500ms",
		"log.audit_days":                   90,
		"database.driver":                  driver,
		"database.dsn":                     "data/imgnest.db",
		"database.max_open":                1,
		"database.max_idle":                1,
		"database.max_lifetime":            "0s",
		"database.busy_timeout":            "5000ms",
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
		{key: "backup.dir", value: &cfg.Backup.Dir},
		{key: "cache.redis_url", value: &cfg.Cache.RedisURL},
		{key: "log.level", value: &cfg.Log.Level},
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
		{key: "images.variant_lookup_per_minute", value: &cfg.Images.VariantLookupPerMinute},
		{key: "log.audit_days", value: &cfg.Log.AuditDays},
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
		{key: "images.variant_cdn_max_age", value: &cfg.Images.VariantCDNMaxAge},
		{key: "log.slow_query", value: &cfg.Log.SlowQuery},
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
	switch value := k.Get("updates.check").(type) {
	case bool:
		cfg.Updates.Check = value
	case string:
		check, err := strconv.ParseBool(value)
		if err != nil {
			return Config{}, invalid("updates.check", "must be true or false")
		}
		cfg.Updates.Check = check
	default:
		return Config{}, invalid("updates.check", "must be true or false")
	}
	var err error
	if cfg.Server.TrustedProxies, err = stringList(k, "server.trusted_proxies", "must be a list of IP addresses or CIDRs"); err != nil {
		return Config{}, err
	}
	if cfg.Images.VariantCORSOrigins, err = stringList(k, "images.variant_cors_origins", "must be a list of origins"); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func stringList(k *koanf.Koanf, key, reason string) ([]string, error) {
	list := []string{}
	switch values := k.Get(key).(type) {
	case []string:
		list = append(list, values...)
	case []any:
		for _, value := range values {
			item, ok := value.(string)
			if !ok {
				return nil, invalid(key, reason)
			}
			list = append(list, item)
		}
	default:
		return nil, invalid(key, reason)
	}
	return list, nil
}

// maxSizeMB (1 TiB) bounds the size settings given in MiB.
const maxSizeMB = 1 << 20

func validate(cfg Config) error {
	// max_upload_mb only applies to groups without their own per-file
	// limit, and a request always stretches to fit one file of the group's
	// limit, so neither has a product ceiling; the bound only keeps the byte
	// arithmetic far from overflow.
	if cfg.Server.MaxUploadMB < 1 || cfg.Server.MaxUploadMB > maxSizeMB {
		return invalid("server.max_upload_mb", "must be between 1 and 1048576")
	}
	if cfg.Server.MaxRequestMB < 1 || cfg.Server.MaxRequestMB > maxSizeMB {
		return invalid("server.max_request_mb", "must be between 1 and 1048576")
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
	if strings.TrimSpace(cfg.Backup.Dir) == "" {
		return invalid("backup.dir", "must not be empty")
	}
	if cfg.Images.VariantLookupPerMinute < 1 || cfg.Images.VariantLookupPerMinute > 10000 {
		return invalid("images.variant_lookup_per_minute", "must be between 1 and 10000")
	}
	if cfg.Images.VariantCDNMaxAge < 0 || cfg.Images.VariantCDNMaxAge > 7*24*time.Hour {
		return invalid("images.variant_cdn_max_age", "must be between 0s and 168h")
	}
	if _, ok := LogLevel(cfg.Log.Level); !ok {
		return invalid("log.level", "must be debug, info, warn or error")
	}
	if cfg.Log.SlowQuery < 0 || cfg.Log.SlowQuery > time.Hour {
		return invalid("log.slow_query", "must be between 0s and 1h")
	}
	if cfg.Log.AuditDays < 1 || cfg.Log.AuditDays > 3650 {
		return invalid("log.audit_days", "must be between 1 and 3650")
	}
	if cfg.Cache.RedisURL != "" && !validRedisURL(cfg.Cache.RedisURL) {
		return invalid("cache.redis_url", "must be a redis:// or rediss:// URL")
	}
	for _, origin := range cfg.Images.VariantCORSOrigins {
		if !validOrigin(origin) {
			return invalid("images.variant_cors_origins", "must contain only * or scheme://host[:port] origins")
		}
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

// validOrigin accepts "*" or an exact browser origin without a path.
func validOrigin(origin string) bool {
	if origin == "*" {
		return true
	}
	scheme, host, ok := strings.Cut(origin, "://")
	if !ok || (scheme != "http" && scheme != "https") || host == "" {
		return false
	}
	return !strings.ContainsAny(host, "/?#@ *")
}

// LogLevel parses a log.level value, ignoring case.
func LogLevel(name string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return 0, false
}

func invalid(field, reason string) error {
	return fmt.Errorf("configuration %s %s: %w", field, reason, errInvalid)
}

// validRedisURL checks the shape only; the connection is tried at startup.
func validRedisURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && (parsed.Scheme == "redis" || parsed.Scheme == "rediss") && parsed.Host != ""
}
