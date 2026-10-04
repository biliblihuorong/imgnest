package lsky_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// assertGolden compares the response body against the pattern document in
// testdata/<name>. The comparison is strict for everything the Lsky v1
// clients depend on:
//
//   - every key in the golden document must exist in the actual document;
//     renamed or missing fields fail (extra ImgNest fields are allowed);
//   - JSON types must match (a number never equals a string, arrays must
//     have the same length);
//   - literal values must be equal.
//
// Pattern values inside the golden file:
//
//	"<string>"     any non-empty string
//	"<number>"     any JSON number
//	"<positive>"   any JSON number > 0
//	"<any>"        any value, including null
//	"re:<expr>"    full regexp match against a string
//
// Everything else is compared literally.
func assertGolden(t *testing.T, name string, body []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name)) // #nosec G304 -- fixed in-repo golden directory.
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	var pattern any
	patternDecoder := json.NewDecoder(bytes.NewReader(raw))
	patternDecoder.UseNumber()
	if err := patternDecoder.Decode(&pattern); err != nil {
		t.Fatalf("golden %s is not JSON: %v", name, err)
	}
	var actual any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&actual); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	match(t, name, "$", pattern, actual)
}

func match(t *testing.T, golden, path string, pattern, actual any) {
	t.Helper()
	// String directives describe the expected JSON type, not a literal value.
	if directive, ok := pattern.(string); ok && (directive == "<string>" || directive == "<number>" || directive == "<positive>" || len(directive) > 3 && directive[:3] == "re:") {
		matchDirective(t, golden, path, directive, actual)
		return
	}
	switch want := pattern.(type) {
	case map[string]any:
		object, ok := actual.(map[string]any)
		if !ok {
			t.Fatalf("%s: %s: want object, got %s", golden, path, jsonKind(actual))
		}
		for key, sub := range want {
			value, exists := object[key]
			if !exists {
				t.Fatalf("%s: %s: missing field %q (Lsky clients depend on it)", golden, path, key)
			}
			match(t, golden, path+"."+key, sub, value)
		}
	case []any:
		list, ok := actual.([]any)
		if !ok {
			t.Fatalf("%s: %s: want array, got %s", golden, path, jsonKind(actual))
		}
		if len(list) != len(want) {
			t.Fatalf("%s: %s: array length %d, want %d", golden, path, len(list), len(want))
		}
		for i := range want {
			match(t, golden, fmt.Sprintf("%s[%d]", path, i), want[i], list[i])
		}
	case string:
		text, ok := actual.(string)
		if !ok {
			t.Fatalf("%s: %s: want string, got %s (%v)", golden, path, jsonKind(actual), actual)
		}
		if text != want {
			t.Fatalf("%s: %s: %q, want %q", golden, path, text, want)
		}
	case bool, nil:
		if actual != want {
			t.Fatalf("%s: %s: %v, want %v", golden, path, actual, want)
		}
	case json.Number:
		number, ok := actual.(json.Number)
		if !ok {
			t.Fatalf("%s: %s: want number, got %s (%v)", golden, path, jsonKind(actual), actual)
		}
		expected, expectErr := want.Float64()
		got, gotErr := number.Float64()
		if expectErr != nil || gotErr != nil || expected != got {
			t.Fatalf("%s: %s: %s, want %s", golden, path, number.String(), want.String())
		}
	default:
		t.Fatalf("%s: unsupported golden pattern at %s: %v (%T)", golden, path, pattern, pattern)
	}
}

func matchDirective(t *testing.T, golden, path, directive string, actual any) {
	t.Helper()
	switch directive {
	case "<string>":
		text, ok := actual.(string)
		if !ok || text == "" {
			t.Fatalf("%s: %s: want non-empty string, got %s (%v)", golden, path, jsonKind(actual), actual)
		}
	case "<number>":
		if _, ok := actual.(json.Number); !ok {
			t.Fatalf("%s: %s: want number, got %s (%v)", golden, path, jsonKind(actual), actual)
		}
	case "<positive>":
		number, ok := actual.(json.Number)
		if !ok {
			t.Fatalf("%s: %s: want positive number, got %s (%v)", golden, path, jsonKind(actual), actual)
		}
		value, err := number.Float64()
		if err != nil || value <= 0 {
			t.Fatalf("%s: %s: want positive number, got %s", golden, path, number.String())
		}
	default: // "re:<expr>"
		expression := regexp.MustCompile(directive[3:])
		text, ok := actual.(string)
		if !ok {
			t.Fatalf("%s: %s: want string matching %s, got %s (%v)", golden, path, directive, jsonKind(actual), actual)
		}
		if !expression.MatchString(text) {
			t.Fatalf("%s: %s: %q does not match %s", golden, path, text, directive)
		}
	}
}

func jsonKind(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "bool"
	case json.Number:
		return "number"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}
