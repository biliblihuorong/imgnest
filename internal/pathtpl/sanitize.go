package pathtpl

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Sanitize removes dot segments and unsafe filename characters without truncation.
func Sanitize(ctx context.Context, value string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("invalid path encoding")
	}
	parts := make([]string, 0)
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			continue
		}
		part = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			if unicode.IsSpace(r) || strings.ContainsRune("?#%&\\:*\"<>|", r) {
				return '-'
			}
			return r
		}, part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		if strings.EqualFold(part, "_trash") || strings.EqualFold(part, ".trash") {
			return "", fmt.Errorf("reserved path namespace")
		}
		part = strings.TrimRight(part, ".") + strings.Repeat("-", len(part)-len(strings.TrimRight(part, ".")))
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9') {
			part = part[:len(stem)] + "-1" + part[len(stem):]
		}
		if utf8.RuneCountInString(part) > 100 {
			return "", fmt.Errorf("path segment exceeds 100 characters")
		}
		parts = append(parts, part)
	}
	result := strings.Join(parts, "/")
	if result == "" || len(result) > 255 {
		return "", fmt.Errorf("path is empty or exceeds 255 bytes")
	}
	return result, nil
}
