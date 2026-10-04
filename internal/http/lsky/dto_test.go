package lsky

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
)

func TestV1EscapeKeepsQuotesLiteral(t *testing.T) {
	got := v1Escape("<img src=\"https://img.test/a.webp\" alt='姓名 & co' title=\"n\" />")
	want := "&lt;img src=\"https://img.test/a.webp\" alt='姓名 &amp; co' title=\"n\" /&gt;"
	if got != want {
		t.Fatalf("v1Escape = %q, want the Lsky format %q", got, want)
	}
	if strings.Contains(got, "&#34;") || strings.Contains(got, "&#39;") {
		t.Fatal("quotes must stay literal; Lsky only escapes &, < and >")
	}
}

func TestKilobytesRendersTheFloatUnit(t *testing.T) {
	cases := map[int64]float64{
		0:        0,
		1024:     1,
		251392:   245.5,
		1:        0.0009765625,
		1 << 20:  1024,
		1 << 30:  1024 * 1024,
		20971520: 20480,
	}
	for bytes, want := range cases {
		if got := kilobytes(bytes); got != want {
			t.Fatalf("kilobytes(%d) = %v, want %v", bytes, got, want)
		}
	}
	if math.IsInf(kilobytes(math.MaxInt64), 0) || kilobytes(math.MaxInt64) <= 0 {
		t.Fatal("kilobytes must stay a finite positive float for huge inputs")
	}
}

func TestHumanDateMatchesTheContractText(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		age  time.Duration
		want string
	}{
		{0, "刚刚"},
		{30 * time.Second, "刚刚"},
		{3 * time.Minute, "3 分钟前"},
		{59 * time.Minute, "59 分钟前"},
		{3 * time.Hour, "3 小时前"},
		{2 * 24 * time.Hour, "2 天前"},
		{45 * 24 * time.Hour, "2026-08-20"},
	}
	for _, testCase := range cases {
		if got := humanDate(now, now.Add(-testCase.age)); got != testCase.want {
			t.Fatalf("humanDate(age=%v) = %q, want %q", testCase.age, got, testCase.want)
		}
	}
}

func TestBuildPaginatorMirrorsLaravelFields(t *testing.T) {
	items := []imageItem{{Key: "a"}, {Key: "b"}}
	page := buildPaginator("https://img.example.com/api/v1/images", 1, 40, 41, items)
	checks := map[string]any{
		"current_page": float64(1), "last_page": float64(2), "per_page": float64(40), "total": float64(41),
		"first_page_url": "https://img.example.com/api/v1/images?page=1",
		"last_page_url":  "https://img.example.com/api/v1/images?page=2",
		"next_page_url":  "https://img.example.com/api/v1/images?page=2",
		"prev_page_url":  nil, "path": "https://img.example.com/api/v1/images",
		"from": float64(1), "to": float64(2),
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for key, want := range checks {
		got, ok := decoded[key]
		if !ok {
			t.Fatalf("paginator lost %q", key)
		}
		switch want.(type) {
		case nil:
			if got != nil {
				t.Fatalf("%s = %v, want null", key, got)
			}
		default:
			if got != want {
				t.Fatalf("%s = %v, want %v", key, got, want)
			}
		}
	}
	if _, ok := decoded["data"]; !ok {
		t.Fatal("paginator lost data")
	}
	links, ok := decoded["links"].([]any)
	if !ok || len(links) != 0 {
		t.Fatal("links must serialize as an empty array, never null")
	}

	empty := buildPaginator("https://img.example.com/api/v1/albums", 3, 40, 0, []albumItem{})
	encoded, err = json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	var emptyDecoded map[string]any
	if err := json.Unmarshal(encoded, &emptyDecoded); err != nil {
		t.Fatal(err)
	}
	if emptyDecoded["from"] != nil || emptyDecoded["to"] != nil {
		t.Fatalf("empty page from/to = %v/%v, want null", emptyDecoded["from"], emptyDecoded["to"])
	}
	if emptyDecoded["last_page"] != float64(1) {
		t.Fatalf("last_page = %v, want the minimum of 1", emptyDecoded["last_page"])
	}
	if emptyDecoded["prev_page_url"] != "https://img.example.com/api/v1/albums?page=2" {
		t.Fatalf("prev_page_url = %v", emptyDecoded["prev_page_url"])
	}
}

func TestBuildImageItemFieldMapping(t *testing.T) {
	created := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	view := service.ImageView{
		Key: "GDmF2b", Name: "screenshot.png", Path: "2026/10/66ff1b2a3c4d", Ext: "png",
		MIME: "image/png", Size: 251392, Width: 1920, Height: 1080,
		MD5: "md5-value", SHA1: "sha1-value",
		Links:     service.ImageLinks{URL: "https://img.test/u.webp", Original: "https://img.test/u.png", WebP: "https://img.test/u.webp", Thumbnail: "https://img.test/u_thumbs.webp"},
		CreatedAt: created,
	}
	item := buildImageItem(view, albumRef{ID: 3, Name: "博客"}, created)
	if item.Album.(albumRef) != (albumRef{ID: 3, Name: "博客"}) {
		t.Fatalf("album = %v", item.Album)
	}
	if item.Name != "66ff1b2a3c4d.png" || item.Pathname != "2026/10/66ff1b2a3c4d.png" {
		t.Fatalf("name/pathname = %s/%s, want the stored file name on the path", item.Name, item.Pathname)
	}
	if item.OriginName != "screenshot.png" {
		t.Fatalf("origin_name = %s", item.OriginName)
	}
	if item.Size != 245.5 {
		t.Fatalf("size = %v, want 245.5 KB", item.Size)
	}
	if item.Links.HTML != "&lt;img src=\"https://img.test/u.webp\" alt=\"screenshot.png\" title=\"screenshot.png\" /&gt;" {
		t.Fatalf("html = %q", item.Links.HTML)
	}
	if item.Links.ThumbnailURL != "https://img.test/u_thumbs.webp" {
		t.Fatalf("thumbnail_url = %s", item.Links.ThumbnailURL)
	}
	if item.Date != "2026-10-04 12:00:00" || item.HumanDate != "刚刚" {
		t.Fatalf("date/human_date = %s/%s", item.Date, item.HumanDate)
	}
	if item.MD5 != "md5-value" || item.SHA1 != "sha1-value" || item.Mimetype != "image/png" || item.Extension != "png" || item.Width != 1920 || item.Height != 1080 {
		t.Fatal("image item lost stored metadata fields")
	}

	// Without a cloud thumbnail the thumbnail URL falls back to the image URL.
	noThumb := view
	noThumb.Links.Thumbnail = ""
	if got := buildImageItem(noThumb, nil, created).Links.ThumbnailURL; got != "https://img.test/u.webp" {
		t.Fatalf("thumbnail fallback = %s, want the image URL", got)
	}
	if got := buildImageItem(noThumb, nil, created).Album; got != nil {
		t.Fatalf("album = %v, want null", got)
	}
}

func TestBaseURLHonoursProxyScheme(t *testing.T) {
	if got := baseURL("img.example.com", "", false); got != "http://img.example.com" {
		t.Fatalf("base = %s", got)
	}
	if got := baseURL("img.example.com", "https", false); got != "https://img.example.com" {
		t.Fatalf("forwarded base = %s", got)
	}
	if got := baseURL("img.example.com", "", true); got != "https://img.example.com" {
		t.Fatalf("tls base = %s", got)
	}
}
