package native

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"testing"

	"github.com/knadh/koanf/parsers/yaml"
)

// loadOpenAPI reads docs/openapi.yaml as plain maps.
func loadOpenAPI(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec, err := yaml.Parser().Unmarshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func dig(t *testing.T, node any, path ...string) any {
	t.Helper()
	for _, key := range path {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("openapi: %v is not an object at %q", path, key)
		}
		if node, ok = m[key]; !ok {
			t.Fatalf("openapi: missing %v", path)
		}
	}
	return node
}

func stringSet(t *testing.T, node any) []string {
	t.Helper()
	var out []string
	switch v := node.(type) {
	case []any:
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
	case map[string]any:
		for key := range v {
			out = append(out, key)
		}
	default:
		t.Fatalf("openapi: %T is not a list or object", node)
	}
	sort.Strings(out)
	return out
}

// Every code errorResponse can send must be allowed by ErrorEnvelope.code,
// or responses that narrow it with allOf (GroupHasMembers, StillReferenced)
// can never validate.
func TestOpenAPIErrorEnvelopeListsEveryNativeCode(t *testing.T) {
	source, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatal(err)
	}
	enum := stringSet(t, dig(t, loadOpenAPI(t), "components", "schemas", "ErrorEnvelope", "properties", "code", "enum"))
	codes := regexp.MustCompile(`(?:status, code, message = \d{3}, |Code: )(\d{5})`).FindAllStringSubmatch(string(source), -1)
	if len(codes) < 30 {
		t.Fatalf("found only %d codes in errorResponse; the pattern no longer matches", len(codes))
	}
	for _, match := range codes {
		if !slices.Contains(enum, match[1]) {
			t.Errorf("code %s is sent but missing from ErrorEnvelope.code", match[1])
		}
	}
}

func TestOpenAPIMatchesGuestAndLskyResponses(t *testing.T) {
	spec := loadOpenAPI(t)
	schemas := dig(t, spec, "components", "schemas")

	// Guest uploads belong to user 0.
	if minimum := fmt.Sprint(dig(t, schemas, "ImageView", "properties", "user_id", "minimum")); minimum != "0" {
		t.Fatalf("ImageView.user_id minimum = %s, want 0 for guest images", minimum)
	}

	// The Lsky upload links are one closed object holding every golden key.
	raw, err := os.ReadFile("../lsky/testdata/upload_success.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Data struct {
			Links map[string]any `json:"links"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for key := range golden.Data.Links {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	links := dig(t, schemas, "LskyUploadLinks").(map[string]any)
	if _, ok := links["allOf"]; ok {
		t.Fatal("LskyUploadLinks extends a closed object with allOf")
	}
	if props, required := stringSet(t, links["properties"]), stringSet(t, links["required"]); !slices.Equal(props, keys) || !slices.Equal(required, keys) {
		t.Fatalf("LskyUploadLinks properties=%v required=%v, golden links=%v", props, required, keys)
	}

	// The partial purge failure carries data.purged, which the closed
	// ErrorData cannot; it must be declared on its own.
	purge := dig(t, spec, "paths", "/api/admin/trash/purge-all", "post", "responses", "502", "content", "application/json", "schema").(map[string]any)
	if _, ok := purge["allOf"]; ok {
		t.Fatal("partial purge response extends the closed ErrorEnvelope with allOf")
	}
	if props := stringSet(t, dig(t, purge, "properties", "data", "properties")); !slices.Equal(props, []string{"purged"}) {
		t.Fatalf("partial purge data properties = %v", props)
	}
}
