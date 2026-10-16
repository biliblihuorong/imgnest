package config

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestImageDeploymentSettings(t *testing.T) {
	cfg, err := Load(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.MaxUploadMB != 20 || cfg.Server.MaxRequestMB != 64 || cfg.Server.UploadConcurrency != 2 || cfg.Server.ProcessingTimeout != 5*time.Minute || cfg.Server.MaxPixels != 100000000 || cfg.Images.ThumbCache != "data/thumbs" {
		t.Fatal("image defaults differ")
	}
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32))
	cfg, err = Load(t.Context(), "", []string{"IMGNEST_SERVER_MAX_UPLOAD_MB=10", "IMGNEST_SERVER_MAX_REQUEST_MB=12", "IMGNEST_SERVER_UPLOAD_CONCURRENCY=1", "IMGNEST_SERVER_MAX_PIXELS=2000", "IMGNEST_SERVER_PROCESSING_TIMEOUT=30s", "IMGNEST_IMAGES_THUMB_CACHE=cache/thumbs", "IMGNEST_SECURITY_MASTER_KEY=" + key})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.MaxUploadMB != 10 || cfg.Server.MaxRequestMB != 12 || cfg.Server.MaxPixels != 2000 || cfg.Security.MasterKey != key || cfg.Images.ThumbCache != "cache/thumbs" {
		t.Fatal("image env ignored")
	}
	for _, assignment := range []string{"IMGNEST_SERVER_MAX_UPLOAD_MB=0", "IMGNEST_SERVER_MAX_UPLOAD_MB=21", "IMGNEST_SERVER_MAX_REQUEST_MB=0", "IMGNEST_SERVER_UPLOAD_CONCURRENCY=0", "IMGNEST_SERVER_MAX_PIXELS=0", "IMGNEST_SERVER_PROCESSING_TIMEOUT=0s", "IMGNEST_SECURITY_MASTER_KEY=secret-marker"} {
		if _, err = Load(t.Context(), "", []string{assignment}); err == nil || strings.Contains(err.Error(), "secret-marker") {
			t.Fatalf("bad config accepted or leaked: %s", assignment)
		}
	}
}

func TestVariantLookupSettings(t *testing.T) {
	cfg, err := Load(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Images.VariantCORSOrigins == nil || len(cfg.Images.VariantCORSOrigins) != 0 || cfg.Images.VariantLookupPerMinute != 60 {
		t.Fatalf("variant defaults = %+v", cfg.Images)
	}
	cfg, err = Load(t.Context(), "", []string{"IMGNEST_IMAGES_VARIANT_CORS_ORIGINS=https://blog.example, http://localhost:8090", "IMGNEST_IMAGES_VARIANT_LOOKUP_PER_MINUTE=10"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.Images.VariantCORSOrigins, ",") != "https://blog.example,http://localhost:8090" || cfg.Images.VariantLookupPerMinute != 10 {
		t.Fatalf("variant env ignored: %+v", cfg.Images)
	}
	for _, assignment := range []string{"IMGNEST_IMAGES_VARIANT_CORS_ORIGINS=https://blog.example/path", "IMGNEST_IMAGES_VARIANT_CORS_ORIGINS=blog.example", "IMGNEST_IMAGES_VARIANT_LOOKUP_PER_MINUTE=0"} {
		if _, err = Load(t.Context(), "", []string{assignment}); err == nil {
			t.Fatalf("bad variant config accepted: %s", assignment)
		}
	}
}
