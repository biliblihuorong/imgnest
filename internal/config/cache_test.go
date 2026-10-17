package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCacheAndCDNSettings(t *testing.T) {
	cfg, err := Load(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cache.RedisURL != "" || cfg.Images.VariantCDNMaxAge != time.Hour {
		t.Fatalf("defaults = %+v %v", cfg.Cache, cfg.Images.VariantCDNMaxAge)
	}
	cfg, err = Load(t.Context(), "", []string{"IMGNEST_CACHE_REDIS_URL=redis://:pw@redis:6379/2", "IMGNEST_IMAGES_VARIANT_CDN_MAX_AGE=0s"})
	if err != nil || cfg.Cache.RedisURL != "redis://:pw@redis:6379/2" || cfg.Images.VariantCDNMaxAge != 0 {
		t.Fatalf("env = %+v %v %v", cfg.Cache, cfg.Images.VariantCDNMaxAge, err)
	}
	path := filepath.Join(t.TempDir(), "imgnest.yaml")
	if err := os.WriteFile(path, []byte("cache:\n  redis_url: rediss://cache.internal:6380/0\nimages:\n  variant_cdn_max_age: 24h\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(t.Context(), path, nil)
	if err != nil || cfg.Cache.RedisURL != "rediss://cache.internal:6380/0" || cfg.Images.VariantCDNMaxAge != 24*time.Hour {
		t.Fatalf("yaml = %+v %v %v", cfg.Cache, cfg.Images.VariantCDNMaxAge, err)
	}
	for _, assignment := range []string{"IMGNEST_CACHE_REDIS_URL=http://redis:6379", "IMGNEST_CACHE_REDIS_URL=redis://", "IMGNEST_IMAGES_VARIANT_CDN_MAX_AGE=-1s", "IMGNEST_IMAGES_VARIANT_CDN_MAX_AGE=200h"} {
		_, err := Load(t.Context(), "", []string{assignment})
		if err == nil {
			t.Fatalf("bad setting accepted: %s", assignment)
		}
		if strings.Contains(err.Error(), "redis:6379") {
			t.Fatalf("error repeats the URL: %v", err)
		}
	}
}
