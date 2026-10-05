//go:build integration

package http_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/service"
)

func TestSearchHTTPRealOwnerScopeAndSuggestions(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	owned := f.album(t, 0, "Cafe\u0301 相册", 0)
	another := f.album(t, 0, "工作", 0)
	foreign := f.album(t, 1, "Private Album", 0)
	first := f.upload(t, 0, "暑假 100%_完成.JPG", owned.ID, false)
	second := f.upload(t, 0, "暑假别处.jpg", another.ID, true)
	_ = f.upload(t, 1, "暑假 100%_完成.JPG", foreign.ID, false)
	path := func(q string) string { return "/api/images?qv=1&tz=UTC&page=1&size=20&q=" + url.QueryEscape(q) }
	page := albumHTTPData[service.ImagePage](t, request(t, f.router, "GET", path(`暑假 100%_ format:jpg album:"Café 相册"`), "", f.tokens[0]), 200)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != first.ID || page.Search == nil || page.Search.AppliedVersion != 1 || !strings.Contains(page.Search.CanonicalQ, fmt.Sprintf("album:#%d", owned.ID)) {
		t.Fatalf("search=%+v", page)
	}
	// The administrator remains owner scoped at the ordinary endpoint too.
	f.imageIDs(t, path("暑假"), first.ID, second.ID)
	locked := fmt.Sprintf("/api/albums/%d/images?qv=1&tz=UTC&page=1&size=20&q=", owned.ID)
	f.imageIDs(t, locked, first.ID)
	rScoped := request(t, f.router, "GET", locked+url.QueryEscape(fmt.Sprintf("album:#%d", another.ID)), "", f.tokens[0])
	if rScoped.Code != 422 {
		t.Fatalf("out-of-scope owned album accepted: %s", rScoped.Body.String())
	}
	f.imageIDs(t, locked+url.QueryEscape(fmt.Sprintf("album:#%d,unfiled", owned.ID)), first.ID)
	f.imageIDs(t, locked+url.QueryEscape("album:unfiled"))
	for _, q := range []string{fmt.Sprintf("album:#%d", foreign.ID), "album:#999999", `album:"Private Album"`} {
		r := request(t, f.router, "GET", path(q), "", f.tokens[0])
		if r.Code != 422 || !strings.Contains(r.Body.String(), "ALBUM_NOT_AVAILABLE") || strings.Contains(r.Body.String(), "Private Album") {
			t.Fatalf("unavailable leak: %s", r.Body.String())
		}
	}
	// Names are exact NFC, ambiguity candidates are only owned names and IDs.
	_ = f.album(t, 0, "同名", 0)
	_ = f.album(t, 0, "同名", 0)
	r := request(t, f.router, "GET", path(`album:"同名"`), "", f.tokens[0])
	if r.Code != 422 || !strings.Contains(r.Body.String(), "ALBUM_AMBIGUOUS") {
		t.Fatalf("ambiguous=%s", r.Body.String())
	}
	for i := 0; i < 23; i++ {
		_ = f.album(t, 0, fmt.Sprintf("Suggest %02d", i), 0)
	}
	suggestions := albumHTTPData[service.AlbumSuggestions](t, request(t, f.router, "GET", "/api/albums/suggestions?keyword=Suggest&page=1&size=20", "", f.tokens[0]), 200)
	if len(suggestions.Items) != 20 || !suggestions.HasMore {
		t.Fatalf("suggestions=%+v", suggestions)
	}
	raw := request(t, f.router, "GET", "/api/albums/suggestions?keyword=&page=1&size=20", "", f.tokens[0])
	var envelope struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	for _, a := range envelope.Data.Items {
		if len(a) != 2 {
			t.Fatalf("extra album metadata: %+v", a)
		}
		if _, ok := a["id"].(string); !ok {
			t.Fatal("album ID is not a string")
		}
	}
	scoped := albumHTTPData[service.AlbumSuggestions](t, request(t, f.router, "GET", fmt.Sprintf("/api/albums/suggestions?keyword=&scope_album_id=%d", owned.ID), "", f.tokens[0]), 200)
	if len(scoped.Items) != 1 || scoped.Items[0].Name != owned.Name {
		t.Fatalf("scope=%+v", scoped)
	}
	for _, id := range []uint64{foreign.ID, 999999} {
		r := request(t, f.router, "GET", fmt.Sprintf("/api/albums/suggestions?scope_album_id=%d", id), "", f.tokens[0])
		if r.Code != 422 || !strings.Contains(r.Body.String(), "ALBUM_NOT_AVAILABLE") {
			t.Fatalf("suggestions scope status=%d body=%s", r.Code, r.Body.String())
		}
	}
	// The old needle protocol remains available, without search metadata.
	old := albumHTTPData[service.ImagePage](t, request(t, f.router, "GET", "/api/images?q="+url.QueryEscape("暑假"), "", f.tokens[0]), 200)
	if old.Total != 2 || old.Search != nil {
		t.Fatalf("legacy regression %+v", old)
	}
}
