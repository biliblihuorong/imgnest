//go:build integration

package http_test

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

var randomPathPattern = regexp.MustCompile(`^/random/([0-9A-Za-z]{10})/([0-9A-Za-z]{24})$`)

func (f *albumHTTPFixture) randomLink(t *testing.T, album uint64, enabled bool) service.RandomLinkView {
	t.Helper()
	body := fmt.Sprintf(`{"enabled":%t}`, enabled)
	view := albumHTTPData[service.RandomLinkView](t, request(t, f.router, "PUT", fmt.Sprintf("/api/albums/%d/random-link", album), body, f.tokens[0]), 200)
	if !randomPathPattern.MatchString(view.Path) || view.Enabled != enabled {
		t.Fatalf("random link view = %+v", view)
	}
	return view
}

// draw requests the anonymous link n times and returns each Location once.
func (f *albumHTTPFixture) draw(t *testing.T, path string, n int) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	for range n {
		w := request(t, f.router, "GET", path, "", "")
		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("GET %s = %d %s, want 307", path, w.Code, w.Body.String())
		}
		if cache := w.Header().Get("Cache-Control"); cache != "no-store" {
			t.Fatalf("Cache-Control=%q, want no-store", cache)
		}
		seen[w.Header().Get("Location")] = true
	}
	return seen
}

func (f *albumHTTPFixture) expectRandomGone(t *testing.T, path string) {
	t.Helper()
	w := request(t, f.router, "GET", path, "", "")
	expectCode(t, w, http.StatusNotFound, 10001)
	if w.Header().Get("Location") != "" {
		t.Fatalf("404 carried Location %q", w.Header().Get("Location"))
	}
}

func TestRandomLinkEndToEnd(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	album := f.album(t, 0, "壁纸", 0)
	// A private image in a private album is still served: the owner opted in.
	first := f.upload(t, 0, "one.png", album.ID, false)
	second := f.upload(t, 0, "two.png", album.ID, true)
	outside := f.upload(t, 0, "outside.png", 0, true)

	none := request(t, f.router, "GET", fmt.Sprintf("/api/albums/%d/random-link", album.ID), "", f.tokens[0])
	expectCode(t, none, 200, 0)
	if string(envelope(t, none)["data"]) != "null" {
		t.Fatalf("album without link returned %s", envelope(t, none)["data"])
	}

	link := f.randomLink(t, album.ID, true)
	seen := f.draw(t, link.Path, 20)
	if len(seen) != 2 || !seen[first.Links.WebP] || !seen[second.Links.WebP] {
		t.Fatalf("20 draws = %v, want both album WebP links %q and %q", seen, first.Links.WebP, second.Links.WebP)
	}
	if seen[outside.Links.WebP] {
		t.Fatal("an image outside the album was served")
	}
	originals := f.draw(t, link.Path+"?format=original", 20)
	if len(originals) != 2 || !originals[first.Links.Original] || !originals[second.Links.Original] {
		t.Fatalf("original draws = %v", originals)
	}
	head := request(t, f.router, "HEAD", link.Path, "", "")
	if head.Code != http.StatusTemporaryRedirect || !seen[head.Header().Get("Location")] {
		t.Fatalf("HEAD = %d %q", head.Code, head.Header().Get("Location"))
	}
	expectCode(t, request(t, f.router, "GET", link.Path+"?format=png", "", ""), 400, 10001)

	// The link is the owner's alone to read or change.
	for _, tc := range []struct{ method, suffix, body string }{
		{"GET", "", ""}, {"PUT", "", `{"enabled":false}`}, {"POST", "/reset", ""}, {"DELETE", "", ""},
	} {
		w := request(t, f.router, tc.method, fmt.Sprintf("/api/albums/%d/random-link%s", album.ID, tc.suffix), tc.body, f.tokens[1])
		expectCode(t, w, 403, 20003)
	}
	if len(f.draw(t, link.Path, 1)) != 1 {
		t.Fatal("a foreign caller changed the link")
	}

	// A redirect response never carries image metadata or EXIF.
	w := request(t, f.router, "GET", link.Path, "", "")
	for _, leaked := range []string{"exif", "gps", "latitude", "\"md5\"", "origin_name"} {
		if strings.Contains(strings.ToLower(w.Body.String()), leaked) {
			t.Fatalf("redirect body leaked %q: %s", leaked, w.Body.String())
		}
	}
}

func TestRandomLinkStopsAfterTrash(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	album := f.album(t, 0, "壁纸", 0)
	first := f.upload(t, 0, "one.png", album.ID, false)
	second := f.upload(t, 0, "two.png", album.ID, false)
	link := f.randomLink(t, album.ID, true)
	if len(f.draw(t, link.Path, 20)) != 2 {
		t.Fatal("pool was not warmed with both images")
	}

	expectCode(t, request(t, f.router, "DELETE", fmt.Sprintf("/api/images/%d", first.ID), "", f.tokens[0]), 200, 0)
	if seen := f.draw(t, link.Path, 20); len(seen) != 1 || !seen[second.Links.WebP] {
		t.Fatalf("after trashing one image draws = %v, want only %q", seen, second.Links.WebP)
	}

	// A new upload and a move both become visible without waiting for the TTL.
	third := f.upload(t, 0, "three.png", album.ID, false)
	loose := f.upload(t, 0, "loose.png", 0, false)
	f.move(t, loose.ID, album.ID)
	if seen := f.draw(t, link.Path, 40); len(seen) != 3 || !seen[third.Links.WebP] || !seen[loose.Links.WebP] {
		t.Fatalf("after upload and move draws = %v", seen)
	}
	f.move(t, loose.ID, 0)
	if seen := f.draw(t, link.Path, 20); seen[loose.Links.WebP] {
		t.Fatal("an image moved out of the album was still served")
	}

	restore := fmt.Sprintf(`{"ids":[%d]}`, first.ID)
	if w := request(t, f.router, "POST", "/api/trash/restore", restore, f.tokens[0]); w.Code != 207 {
		t.Fatalf("restore = %d %s", w.Code, w.Body.String())
	}
	if seen := f.draw(t, link.Path, 40); !seen[first.Links.WebP] {
		t.Fatalf("restored image did not return to rotation: %v", seen)
	}

	for _, id := range []uint64{first.ID, second.ID, third.ID} {
		expectCode(t, request(t, f.router, "DELETE", fmt.Sprintf("/api/images/%d", id), "", f.tokens[0]), 200, 0)
	}
	f.expectRandomGone(t, link.Path)
}

func TestRandomLinkResetAndDisable(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	album := f.album(t, 0, "壁纸", 0)
	f.upload(t, 0, "one.png", album.ID, false)
	link := f.randomLink(t, album.ID, true)
	f.draw(t, link.Path, 1)

	reset := albumHTTPData[service.RandomLinkView](t, request(t, f.router, "POST", fmt.Sprintf("/api/albums/%d/random-link/reset", album.ID), "", f.tokens[0]), 200)
	if reset.Path == link.Path || !randomPathPattern.MatchString(reset.Path) {
		t.Fatalf("reset path %q (was %q)", reset.Path, link.Path)
	}
	if randomPathPattern.FindStringSubmatch(reset.Path)[1] != randomPathPattern.FindStringSubmatch(link.Path)[1] {
		t.Fatal("reset changed the owner's public id")
	}
	f.expectRandomGone(t, link.Path)
	f.draw(t, reset.Path, 1)

	// A valid token under another account's id shape must not resolve.
	parts := randomPathPattern.FindStringSubmatch(reset.Path)
	f.expectRandomGone(t, "/random/"+strings.Repeat("Z", 10)+"/"+parts[2])

	f.randomLink(t, album.ID, false)
	f.expectRandomGone(t, reset.Path)
	enabled := f.randomLink(t, album.ID, true)
	if enabled.Path != reset.Path {
		t.Fatal("re-enabling the link changed its URL")
	}
	f.draw(t, reset.Path, 1)

	if err := f.db.Model(&model.User{}).Where("id = ?", f.users[0].ID).Update("status", model.UserStatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	f.expectRandomGone(t, reset.Path)
}

func TestRandomLinkAlbumDeleted(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	album := f.album(t, 0, "壁纸", 0)
	f.upload(t, 0, "one.png", album.ID, false)
	link := f.randomLink(t, album.ID, true)
	f.draw(t, link.Path, 1)

	expectCode(t, request(t, f.router, "DELETE", fmt.Sprintf("/api/albums/%d", album.ID), "", f.tokens[0]), 200, 0)
	f.expectRandomGone(t, link.Path)

	second := f.album(t, 0, "新相册", 0)
	deleted := request(t, f.router, "DELETE", fmt.Sprintf("/api/albums/%d/random-link", second.ID), "", f.tokens[0])
	expectCode(t, deleted, 200, 0)
	expectCode(t, request(t, f.router, "POST", fmt.Sprintf("/api/albums/%d/random-link/reset", second.ID), "", f.tokens[0]), 404, 10001)
}

func TestRandomLinkNotLogged(t *testing.T) {
	f := newAlbumHTTPFixture(t)
	album := f.album(t, 0, "壁纸", 0)
	f.upload(t, 0, "one.png", album.ID, false)
	link := f.randomLink(t, album.ID, true)
	parts := randomPathPattern.FindStringSubmatch(link.Path)
	f.logs.Reset()
	f.draw(t, link.Path, 3)
	f.expectRandomGone(t, "/random/"+parts[1]+"/"+strings.Repeat("Z", 24))
	logged := f.logs.String()
	if !strings.Contains(logged, "/random/:uid/:token") {
		t.Fatalf("random requests were not logged by route template: %s", logged)
	}
	if strings.Contains(logged, parts[1]) || strings.Contains(logged, parts[2]) {
		t.Fatalf("request log leaked the link's uid or token: %s", logged)
	}
}
