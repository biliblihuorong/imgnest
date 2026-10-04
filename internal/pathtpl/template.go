// Package pathtpl renders safe image object paths.
package pathtpl

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// Variables supplies rendering data and an optional random source.
type Variables struct {
	Time                time.Time
	UserID              uint64
	Filename, MD5, SHA1 string
	Random              io.Reader
}

// Result is the canonical extensionless path and its collision strategy hint.
type Result struct {
	Path      string
	HasRandom bool
}

// Build renders a canonical path.
func Build(ctx context.Context, pathTpl, nameTpl string, vars Variables) (Result, error) {
	if err := Validate(ctx, pathTpl, nameTpl); err != nil {
		return Result{}, err
	}
	if vars.Random == nil {
		vars.Random = rand.Reader
	}
	for range 5 {
		dir, randomDir, err := render(ctx, pathTpl, vars)
		if err != nil {
			return Result{}, err
		}
		name, randomName, err := render(ctx, nameTpl, vars)
		if err != nil {
			return Result{}, err
		}
		name, err = Sanitize(ctx, name)
		if err != nil {
			return Result{}, err
		}
		if strings.Contains(name, "/") {
			return Result{}, fmt.Errorf("filename template contains a directory")
		}
		if strings.HasSuffix(strings.ToLower(name), "_thumbs") {
			if randomName {
				continue
			}
			name += "-1"
		}
		value, err := Sanitize(ctx, dir+"/"+name)
		if err != nil {
			return Result{}, err
		}
		return Result{Path: value, HasRandom: randomDir || randomName}, nil
	}
	return Result{}, fmt.Errorf("reserved filename after five attempts")
}

// Validate checks all template placeholders.
func Validate(ctx context.Context, templates ...string) error {
	for _, template := range templates {
		if _, _, err := walk(ctx, template, func(key string) (string, bool, error) { _, _, err := variableKind(key); return "", false, err }); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// Sanitize makes a relative image path safe.
func render(ctx context.Context, template string, v Variables) (string, bool, error) {
	return walk(ctx, template, func(key string) (string, bool, error) {
		kind, n, err := variableKind(key)
		if err != nil {
			return "", false, err
		}
		if format, ok := map[string]string{"Y": "2006", "y": "06", "m": "01", "d": "02", "H": "15", "i": "04", "s": "05"}[key]; ok {
			return v.Time.Format(format), false, nil
		}
		switch kind {
		case "timestamp":
			return strconv.FormatInt(v.Time.Unix(), 10), false, nil
		case "uid":
			return strconv.FormatUint(v.UserID, 10), false, nil
		case "filename":
			name := path.Base(strings.ReplaceAll(v.Filename, "\\", "/"))
			return strings.TrimSuffix(name, path.Ext(name)), false, nil
		case "md5", "md5-16", "hash":
			if !validDigest(v.MD5, 32) {
				return "", false, fmt.Errorf("invalid MD5 render input")
			}
			if kind == "md5-16" {
				return strings.ToLower(v.MD5[8:24]), false, nil
			}
			if kind == "hash" {
				return strings.ToLower(v.MD5[:n]), false, nil
			}
			return strings.ToLower(v.MD5), false, nil
		case "sha1":
			if !validDigest(v.SHA1, 40) {
				return "", false, fmt.Errorf("invalid SHA1 render input")
			}
			return strings.ToLower(v.SHA1), false, nil
		case "uniqid":
			var b [3]byte
			if _, err := io.ReadFull(v.Random, b[:]); err != nil {
				return "", true, fmt.Errorf("generate identifier: %w", err)
			}
			if v.Time.Unix() < 0 || v.Time.Unix() > 0xffffffff {
				return "", true, fmt.Errorf("identifier time outside supported range")
			}
			return fmt.Sprintf("%08x%05x", v.Time.Unix(), (uint32(b[0])<<16|uint32(b[1])<<8|uint32(b[2]))&0xfffff), true, nil
		case "uuid":
			var b [16]byte
			if _, err := io.ReadFull(v.Random, b[:]); err != nil {
				return "", true, fmt.Errorf("generate uuid: %w", err)
			}
			b[6] = (b[6] & 15) | 64
			b[8] = (b[8] & 63) | 128
			return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), true, nil
		case "rand":
			const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
			out := make([]byte, n)
			for i := range out {
				for {
					if err := ctx.Err(); err != nil {
						return "", true, err
					}
					var b [1]byte
					if _, err := io.ReadFull(v.Random, b[:]); err != nil {
						return "", true, fmt.Errorf("generate random filename: %w", err)
					}
					if b[0] < 252 {
						out[i] = alphabet[int(b[0])%len(alphabet)]
						break
					}
				}
			}
			return string(out), true, nil
		}
		return "", false, fmt.Errorf("unsupported template variable")
	})
}

func variableKind(key string) (string, int, error) {
	switch key {
	case "Y", "y", "m", "d", "H", "i", "s", "timestamp", "uid", "filename", "md5", "md5-16", "sha1", "uuid", "uniqid":
		return key, 0, nil
	case "str-random-16":
		return "rand", 16, nil
	case "str-random-10":
		return "rand", 10, nil
	}
	kind, raw, ok := strings.Cut(key, ":")
	n, err := strconv.Atoi(raw)
	max := 64
	if kind == "hash" {
		max = 32
	}
	if ok && (kind == "rand" || kind == "hash") && err == nil && n >= 1 && n <= max && strconv.Itoa(n) == raw {
		return kind, n, nil
	}
	return "", 0, fmt.Errorf("invalid template variable")
}

func walk(ctx context.Context, template string, expand func(string) (string, bool, error)) (string, bool, error) {
	var out strings.Builder
	random := false
	for len(template) > 0 {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		i := strings.IndexAny(template, "{}")
		if i < 0 {
			out.WriteString(template)
			break
		}
		out.WriteString(template[:i])
		if template[i] != '{' {
			return "", false, fmt.Errorf("unmatched template brace")
		}
		end := strings.IndexByte(template[i+1:], '}')
		if end < 0 {
			return "", false, fmt.Errorf("unmatched template brace")
		}
		end += i + 1
		value, r, err := expand(template[i+1 : end])
		if err != nil {
			return "", false, err
		}
		out.WriteString(value)
		random = random || r
		template = template[end+1:]
	}
	return out.String(), random, nil
}
func validDigest(value string, length int) bool {
	if len(value) != length {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
