package pathtpl

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func variables() Variables {
	return Variables{Time: time.Date(2026, 10, 4, 15, 6, 7, 123456000, time.UTC), UserID: 42, Filename: "旅行.png", MD5: "0123456789abcdef0123456789abcdef", SHA1: "0123456789abcdef0123456789abcdef01234567", Random: bytes.NewReader(make([]byte, 4096))}
}

func TestBuildVariables(t *testing.T) {
	for _, tc := range []struct{ template, want string }{
		{"{Y}/{y}/{m}/{d}/{H}/{i}/{s}", "2026/26/10/04/15/06/07"},
		{"{md5}", "0123456789abcdef0123456789abcdef"}, {"{md5-16}", "89abcdef01234567"},
		{"{sha1}", "0123456789abcdef0123456789abcdef01234567"}, {"{hash:2}", "01"}, {"{uid}", "42"}, {"{filename}", "旅行"},
	} {
		t.Run(tc.template, func(t *testing.T) {
			got, err := Build(t.Context(), tc.template, "image", variables())
			if err != nil || got.Path != tc.want+"/image" || got.HasRandom {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
	got, err := Build(t.Context(), "{Y}/{m}/{d}", "{filename}", variables())
	if err != nil || got.Path != "2026/10/04/旅行" {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestRandomAndReservedNames(t *testing.T) {
	for _, name := range []string{"{uniqid}", "{uuid}", "{rand:1}", "{rand:64}", "{str-random-16}", "{str-random-10}"} {
		t.Run(name, func(t *testing.T) {
			got, err := Build(t.Context(), "", name, variables())
			if err != nil || !got.HasRandom || got.Path == "" {
				t.Fatalf("got %+v %v", got, err)
			}
			switch name {
			case "{uniqid}":
				if len(got.Path) != 13 {
					t.Fatal(got.Path)
				}
			case "{uuid}":
				if got.Path != "00000000-0000-4000-8000-000000000000" {
					t.Fatal(got.Path)
				}
			case "{rand:64}":
				if len(got.Path) != 64 {
					t.Fatal(got.Path)
				}
			}
		})
	}
	got, err := Build(t.Context(), "", "photo_thumbs", variables())
	if err != nil || got.Path != "photo_thumbs-1" {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := Build(t.Context(), "", "{rand:1}_thumbs", variables()); err == nil {
		t.Fatal("permanently reserved random name accepted")
	}
	for _, bad := range []string{"_trash/image", ".trash/image", "safe/_trash/image"} {
		if _, err := Build(t.Context(), bad, "photo", variables()); err == nil {
			t.Fatal("reserved namespace accepted")
		}
	}
}

func TestTemplateValidation(t *testing.T) {
	for _, bad := range []string{"{unknown}", "{rand:0}", "{rand:65}", "{hash:0}", "{hash:33}", "{rand:x}", "{filename", "filename}", "{{uid}}"} {
		if err := Validate(t.Context(), bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if err := Validate(t.Context(), "{timestamp}", "{hash:32}", "{filename}"); err != nil {
		t.Fatal(err)
	}
	v := variables()
	v.MD5 = "bad"
	if _, err := Build(t.Context(), "", "{hash:10}", v); err == nil {
		t.Fatal("short digest accepted")
	}
	v = variables()
	v.Random = bytes.NewReader(nil)
	if _, err := Build(t.Context(), "", "{rand:10}", v); err == nil {
		t.Fatal("failed random source accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Build(ctx, "", "x", variables()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSanitize(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"/.././相册//照片 ?#%&\\:*\"<>|", "相册/照片------------"}, {"a/../../b", "a/b"}, {"\x00a\nb", "ab"}, {"CON", "CON-1"}, {"a.", "a-"}} {
		got, err := Sanitize(t.Context(), tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%q => %q %v want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"/../", strings.Repeat("a", 101), strings.Repeat("图", 86), "_trash/a", "a/.trash/b"} {
		if _, err := Sanitize(t.Context(), bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func FuzzSanitize(f *testing.F) {
	for _, s := range []string{"../a", "照片 ?", "a\\b", "", "\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got, err := Sanitize(t.Context(), s)
		if err != nil {
			return
		}
		if got == "" || strings.HasPrefix(got, "/") || len(got) > 255 {
			t.Fatalf("unsafe result %q", got)
		}
		for _, segment := range strings.Split(got, "/") {
			if segment == "." || segment == ".." || segment == "" {
				t.Fatalf("unsafe segment %q", got)
			}
		}
	})
}

func TestWindowsDeviceStemSanitizedBeforeExtension(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"CON.png", "CON-1.png"}, {"lpt1.photo", "lpt1-1.photo"}} {
		got, err := Sanitize(t.Context(), tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("device path %q => %q %v", tc.in, got, err)
		}
	}
}
