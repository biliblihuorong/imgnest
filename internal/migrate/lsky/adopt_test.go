package lsky

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/storage"
)

func bucket(keys ...string) func(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error) {
	return func(_ context.Context, key string) (io.ReadCloser, storage.ObjectInfo, error) {
		for _, k := range keys {
			if k == key {
				return io.NopCloser(bytes.NewReader(nil)), storage.ObjectInfo{VersionID: "v1"}, nil
			}
		}
		return nil, storage.ObjectInfo{}, storage.ErrNotFound
	}
}

func TestCheckAdoption(t *testing.T) {
	snap := Snapshot{
		Strategies: []Strategy{{ID: 9, URL: "https://img.example.com/ShellBlogImg"}},
		Images:     []Image{{ID: 1, StrategyID: 9, Pathname: "1/2025/a.webp"}, {ID: 2, StrategyID: 9, Pathname: "1/2025/b.webp"}},
	}
	cases := []struct {
		name, base, folder string
		keys               []string
		err                string
	}{
		{name: "found, same link", base: "https://img.example.com/ShellBlogImg/", keys: []string{"1/2025/b.webp"}},
		{name: "found, new domain", base: "https://cdn.example.net/ShellBlogImg", keys: []string{"1/2025/a.webp"}},
		{name: "found, link path differs", base: "https://img.example.com", keys: []string{"1/2025/a.webp"}, err: "Base URL 改成 https://img.example.com/ShellBlogImg 后"},
		{name: "files under a folder", base: "https://img.example.com/ShellBlogImg", keys: []string{"ShellBlogImg/1/2025/a.webp"}, err: "「路径前缀」设成 ShellBlogImg，Base URL 保持 https://img.example.com/ShellBlogImg"},
		{name: "folder under an existing folder", base: "https://img.example.com/ShellBlogImg", folder: "site", keys: []string{"ShellBlogImg/1/2025/a.webp"}, err: "设成 site/ShellBlogImg"},
		{name: "nothing found", base: "https://img.example.com/ShellBlogImg", err: "在桶的根目录下找不到 1/2025/a.webp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckAdoption(t.Context(), snap, 9, tc.base, tc.folder, bucket(tc.keys...))
			if tc.err == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("err = %v, want %q", err, tc.err)
			}
		})
	}
}

func TestCheckAdoptionReportsReadFailure(t *testing.T) {
	snap := Snapshot{Strategies: []Strategy{{ID: 1, URL: "https://x.example/b"}}, Images: []Image{{StrategyID: 1, Pathname: "a.png"}}}
	broken := func(context.Context, string) (io.ReadCloser, storage.ObjectInfo, error) {
		return nil, storage.ObjectInfo{}, errors.New("AccessDenied")
	}
	if err := CheckAdoption(t.Context(), snap, 0, "https://x.example/b", "", broken); err == nil || !strings.Contains(err.Error(), "测试这个存储的连接") {
		t.Fatalf("err = %v", err)
	}
}
