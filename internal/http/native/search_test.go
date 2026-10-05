package native_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSearchProtocolRejectsMixedAndUnsupported(t *testing.T) {
	images := &galleryImagesStub{}
	router := albumRouter(t, &albumsStub{}, images)
	for _, v := range []struct{ query, code string }{{"qv=2&q=&tz=UTC", "UNSUPPORTED_QUERY_VERSION"}, {"qv=1&q=&tz=UTC&album_id=0", "MIXED_QUERY_PROTOCOL"}, {"qv=1&q=&tz=UTC&sort=newest", "MIXED_QUERY_PROTOCOL"}, {"qv=1&tz=UTC", "MISSING_VALUE"}, {"qv=1&q=&tz=UTC&page=100001", "INVALID_PAGINATION"}, {"qv=1&q=&tz=UTC&size=40", "INVALID_PAGINATION"}, {"qv=1&q=a&q=b&tz=UTC", "INVALID_PARAMETER"}} {
		req := httptest.NewRequest(http.MethodGet, "/api/images?"+v.query, nil)
		req.Header.Set("Authorization", "Bearer "+testCredential)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var body struct {
			Data struct {
				Diagnostics []struct {
					Code string `json:"code"`
				} `json:"diagnostics"`
			} `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if rec.Code != 400 || len(body.Data.Diagnostics) != 1 || body.Data.Diagnostics[0].Code != v.code {
			t.Errorf("%s: status=%d body=%s", v.query, rec.Code, rec.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/albums/42/images?qv=1&q="+url.QueryEscape("album:#57")+"&tz=UTC&page=1&size=20", nil)
	req.Header.Set("Authorization", "Bearer "+testCredential)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 || images.listQuery.QueryVersion != 1 || images.listQuery.LockedAlbumID == nil || *images.listQuery.LockedAlbumID != 42 || images.listQuery.AlbumID != nil {
		t.Fatalf("locked route: %d %s query=%+v", rec.Code, rec.Body.String(), images.listQuery)
	}
}

func TestAlbumSuggestionsParameterWhitelist(t *testing.T) {
	router := albumRouter(t, &albumsStub{}, &galleryImagesStub{})
	for _, q := range []string{"keyword%7Cpage=ignored", "owner_id=2", "keyword=a&keyword=b", "scope_album_id=01"} {
		response := callAlbum(t, router, http.MethodGet, "/api/albums/suggestions?"+q, "", testCredential)
		if response.Code != 400 {
			t.Errorf("invalid parameter %q returned %d: %s", q, response.Code, response.Body.String())
		}
	}
}
