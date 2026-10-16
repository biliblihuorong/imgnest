package storage

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestNormalizePrefix(t *testing.T) {
	for input, want := range map[string]string{"": "", " /": "", "/ShellBlogImg/": "ShellBlogImg", "a/b": "a/b", "图床/2026": "图床/2026"} {
		if got, err := NormalizePrefix(input); err != nil || got != want {
			t.Fatalf("NormalizePrefix(%q) = %q, %v", input, got, err)
		}
	}
	for _, bad := range []string{"a//b", "../a", "a/./b", "_trash", "a/_checks", `a\b`, "a:b", "a/ b", "x\x00", strings.Repeat("a", MaxPathPrefix+1)} {
		if _, err := NormalizePrefix(bad); !errors.Is(err, ErrInvalidPrefix) {
			t.Fatalf("NormalizePrefix(%q) err = %v", bad, err)
		}
	}
}

func TestWithPrefixKeepsKeysInsideFolder(t *testing.T) {
	ctx := t.Context()
	inner, err := NewLocal(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if same, _ := WithPrefix(inner, ""); same != Driver(inner) {
		t.Fatal("empty prefix should return the driver unchanged")
	}
	if _, err := WithPrefix(inner, "/a/"); err == nil {
		t.Fatal("unnormalized prefix accepted")
	}
	driver, err := WithPrefix(inner, "blog/img")
	if err != nil {
		t.Fatal(err)
	}
	opts := PutOptions{OwnerID: "owner", MIME: "image/png"}
	receipt, err := driver.PutNew(ctx, "2026/a.png", strings.NewReader("png"), opts)
	if err != nil || receipt.Key != "2026/a.png" {
		t.Fatalf("put = %+v, %v", receipt, err)
	}
	body, info, err := inner.Open(ctx, "blog/img/2026/a.png")
	if err != nil || info.OwnerID != "owner" {
		t.Fatalf("inner open = %+v, %v", info, err)
	}
	data, _ := io.ReadAll(body)
	_ = body.Close()
	if string(data) != "png" {
		t.Fatalf("body = %q", data)
	}
	if _, err := inner.Stat(ctx, "2026/a.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("object escaped the folder: %v", err)
	}
	if receipt, err = driver.Copy(ctx, "2026/a.png", "_trash/2026/a.png", opts); err != nil || receipt.Key != "_trash/2026/a.png" {
		t.Fatalf("copy = %+v, %v", receipt, err)
	}
	if _, err := inner.Stat(ctx, "blog/img/_trash/2026/a.png"); err != nil {
		t.Fatalf("copy target: %v", err)
	}
	if err := driver.(ImagePurger).PurgeImage(ctx, "2026/a.png", "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Stat(ctx, "2026/a.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after purge: %v", err)
	}
	if _, ok := driver.(LegacyObjects); ok {
		t.Fatal("local driver gained legacy support")
	}
}
