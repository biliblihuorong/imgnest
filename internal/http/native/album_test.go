package native_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type albumsStub struct {
	native.Albums
	listResult  service.AlbumPage
	listQuery   service.AlbumQuery
	createInput service.AlbumInput
	createView  service.AlbumView
	createErr   error
	updateArgs  []any
	updateView  service.AlbumView
	updateErr   error
	deleteArgs  []any
	deleteErr   error
}

func (s *albumsStub) List(_ context.Context, owner uint64, query service.AlbumQuery) (service.AlbumPage, error) {
	s.listQuery = query
	page := s.listResult
	if page.Items == nil {
		page.Items = []service.AlbumView{}
	}
	return service.AlbumPage{Items: page.Items, Total: page.Total, Page: query.Page, Size: query.Size}, nil
}

func (s *albumsStub) Create(_ context.Context, owner uint64, input service.AlbumInput) (service.AlbumView, error) {
	if s.createErr != nil {
		return service.AlbumView{}, s.createErr
	}
	s.createInput = input
	view := s.createView
	view.Name = input.Name
	return view, nil
}

func (s *albumsStub) Update(_ context.Context, owner, albumID uint64, patch service.AlbumPatch) (service.AlbumView, error) {
	if s.updateErr != nil {
		return service.AlbumView{}, s.updateErr
	}
	s.updateArgs = []any{owner, albumID, patch}
	return s.updateView, nil
}

func (s *albumsStub) Delete(_ context.Context, owner, albumID uint64) error {
	s.deleteArgs = []any{owner, albumID}
	return s.deleteErr
}

type galleryImagesStub struct {
	native.ImageService
	gallery      service.GalleryPage
	galleryArgs  []int
	galleryErr   error
	listQuery    service.ImageQuery
	setAlbumArgs []any
	setAlbumView service.ImageView
	setAlbumErr  error
}

func (s *galleryImagesStub) Get(_ context.Context, _ service.TokenSubject, id uint64) (service.ImageView, error) {
	if id == 9 {
		return service.ImageView{}, service.ErrForbidden
	}
	return service.ImageView{ID: id, Key: "image-key"}, nil
}

func (s *galleryImagesStub) List(_ context.Context, _ service.TokenSubject, query service.ImageQuery) (service.ImagePage, error) {
	s.listQuery = query
	return service.ImagePage{Items: []service.ImageView{}, Page: query.Page, Size: query.Size}, nil
}

func (s *galleryImagesStub) Gallery(_ context.Context, page, size int) (service.GalleryPage, error) {
	s.galleryArgs = []int{page, size}
	if s.galleryErr != nil {
		return service.GalleryPage{}, s.galleryErr
	}
	page_ := s.gallery
	if page_.Items == nil {
		page_.Items = []service.GalleryItem{}
	}
	page_.Page, page_.Size = page, size
	return page_, nil
}

func (s *galleryImagesStub) SetAlbum(_ context.Context, _ service.TokenSubject, id, albumID uint64) (service.ImageView, error) {
	if s.setAlbumErr != nil {
		return service.ImageView{}, s.setAlbumErr
	}
	s.setAlbumArgs = []any{id, albumID}
	return s.setAlbumView, nil
}

type albumTokensStub struct{ native.TokenService }

func (albumTokensStub) Authenticate(_ context.Context, raw string) (service.Identity, error) {
	if raw != testCredential {
		return service.Identity{}, service.ErrUnauthenticated
	}
	return service.Identity{User: service.UserView{ID: 1}, TokenID: 7, Kind: "web"}, nil
}

func albumRouter(t *testing.T, albums *albumsStub, images *galleryImagesStub) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, albumTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if albums != nil {
		if err := handler.RegisterAlbumRoutes(t.Context(), router, albums); err != nil {
			t.Fatal(err)
		}
	}
	if images != nil {
		if err := handler.RegisterImageRoutes(t.Context(), router, images, native.ImageOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	return router
}

func callAlbum(t *testing.T, router http.Handler, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func decodeEnvelope(t *testing.T, response *httptest.ResponseRecorder) (int, json.RawMessage) {
	t.Helper()
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("envelope %s", response.Body.String())
	}
	return envelope.Code, envelope.Data
}

func TestAlbumRoutesRequireAuthentication(t *testing.T) {
	router := albumRouter(t, &albumsStub{}, nil)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/albums"},
		{http.MethodPost, "/api/albums"},
		{http.MethodPatch, "/api/albums/5"},
		{http.MethodDelete, "/api/albums/5"},
	} {
		response := callAlbum(t, router, tc.method, tc.path, "", "")
		if code, _ := decodeEnvelope(t, response); response.Code != http.StatusUnauthorized || code != 20001 {
			t.Fatalf("%s %s status=%d code=%d", tc.method, tc.path, response.Code, code)
		}
	}
}

func TestAlbumListParsesPagination(t *testing.T) {
	albums := &albumsStub{}
	router := albumRouter(t, albums, nil)
	response := callAlbum(t, router, http.MethodGet, "/api/albums?page=2&size=10&keyword=博客", "", testCredential)
	if response.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	if albums.listQuery.Page != 2 || albums.listQuery.Size != 10 || albums.listQuery.Keyword != "博客" {
		t.Fatalf("query = %+v", albums.listQuery)
	}
	response = callAlbum(t, router, http.MethodGet, "/api/albums?page=0", "", testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
		t.Fatalf("page 0 status=%d code=%d", response.Code, code)
	}
	response = callAlbum(t, router, http.MethodGet, "/api/albums?size=101", "", testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
		t.Fatalf("size 101 status=%d code=%d", response.Code, code)
	}
}

func TestAlbumCreateReturnsViewWithContractFields(t *testing.T) {
	albums := &albumsStub{createView: service.AlbumView{ID: 12, Name: "博客", IsPublic: true, CoverImageID: 4, ImageNum: 0, CoverThumbURL: "/t/coverkey.webp"}}
	router := albumRouter(t, albums, nil)
	response := callAlbum(t, router, http.MethodPost, "/api/albums", `{"name":"博客","intro":"配图","is_public":true,"cover_image_id":4}`, testCredential)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	if albums.createInput.Name != "博客" || albums.createInput.Intro != "配图" || !albums.createInput.IsPublic || albums.createInput.CoverImageID != 4 {
		t.Fatalf("create input = %+v", albums.createInput)
	}
	_, data := decodeEnvelope(t, response)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "name", "intro", "is_public", "cover_image_id", "image_count", "cover_thumb_url", "created_at", "updated_at"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("album view lacks %s: %s", field, data)
		}
	}
	if len(fields) != 9 {
		t.Fatalf("album view exposes %d fields: %s", len(fields), data)
	}

	// Value rules (blank name, oversized intro, foreign cover) live in the
	// album service; the handler only rejects malformed envelopes.
	for _, body := range []string{
		`{"name":"ok","unknown":1}`,
		`not-json`,
		`{"name":"ok"} trailing`,
	} {
		response := callAlbum(t, router, http.MethodPost, "/api/albums", body, testCredential)
		if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
			t.Fatalf("invalid body %q status=%d code=%d", body, response.Code, code)
		}
	}
	albums.createErr = service.ErrInvalidInput
	response = callAlbum(t, router, http.MethodPost, "/api/albums", `{"name":"博客","cover_image_id":9}`, testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
		t.Fatalf("service invalid status=%d code=%d", response.Code, code)
	}
	albums.createErr = service.ErrStorage
	response = callAlbum(t, router, http.MethodPost, "/api/albums", `{"name":"博客"}`, testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadGateway || code != 50002 {
		t.Fatalf("storage status=%d code=%d", response.Code, code)
	}
}

func TestAlbumUpdateAndDeletePassOwnerAndID(t *testing.T) {
	albums := &albumsStub{updateView: service.AlbumView{ID: 5, Name: "新名"}}
	router := albumRouter(t, albums, nil)
	response := callAlbum(t, router, http.MethodPatch, "/api/albums/5", `{"name":"新名","is_public":true}`, testCredential)
	if response.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", response.Code, response.Body.String())
	}
	if albums.updateArgs[0] != uint64(1) || albums.updateArgs[1] != uint64(5) {
		t.Fatalf("update args = %v", albums.updateArgs)
	}
	patch := albums.updateArgs[2].(service.AlbumPatch)
	if patch.Name == nil || *patch.Name != "新名" || patch.IsPublic == nil || !*patch.IsPublic {
		t.Fatalf("patch = %+v", patch)
	}

	response = callAlbum(t, router, http.MethodPatch, "/api/albums/5", `{}`, testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
		t.Fatalf("empty patch status=%d code=%d", response.Code, code)
	}
	albums.updateErr = service.ErrForbidden
	response = callAlbum(t, router, http.MethodPatch, "/api/albums/5", `{"name":"新名"}`, testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusForbidden || code != 20003 {
		t.Fatalf("foreign update status=%d code=%d", response.Code, code)
	}
	albums.updateErr = service.ErrNotFound
	response = callAlbum(t, router, http.MethodPatch, "/api/albums/5", `{"name":"新名"}`, testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusNotFound || code != 10001 {
		t.Fatalf("missing update status=%d code=%d", response.Code, code)
	}

	response = callAlbum(t, router, http.MethodDelete, "/api/albums/5", "", testCredential)
	if code, data := decodeEnvelope(t, response); response.Code != http.StatusOK || code != 0 || string(data) != "null" {
		t.Fatalf("delete status=%d data=%s", response.Code, data)
	}
	if albums.deleteArgs[0] != uint64(1) || albums.deleteArgs[1] != uint64(5) {
		t.Fatalf("delete args = %v", albums.deleteArgs)
	}
	albums.deleteErr = service.ErrForbidden
	response = callAlbum(t, router, http.MethodDelete, "/api/albums/5", "", testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusForbidden || code != 20003 {
		t.Fatalf("foreign delete status=%d code=%d", response.Code, code)
	}
	for _, path := range []string{"/api/albums/0", "/api/albums/x"} {
		response := callAlbum(t, router, http.MethodPatch, path, `{"name":"新名"}`, testCredential)
		if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
			t.Fatalf("bad id %s status=%d code=%d", path, response.Code, code)
		}
	}
}

func TestGalleryIsPublicAndEmptyWhenClosed(t *testing.T) {
	images := &galleryImagesStub{}
	router := albumRouter(t, nil, images)

	response := callAlbum(t, router, http.MethodGet, "/api/gallery?page=3&size=12", "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("gallery status=%d body=%s", response.Code, response.Body.String())
	}
	if images.galleryArgs[0] != 3 || images.galleryArgs[1] != 12 {
		t.Fatalf("gallery args = %v", images.galleryArgs)
	}
	_, data := decodeEnvelope(t, response)
	var page struct {
		Items []json.RawMessage `json:"items"`
		Total int64             `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("closed gallery page = %s", data)
	}

	images.gallery = service.GalleryPage{Items: []service.GalleryItem{{ID: 4, Uploader: "alice"}}, Total: 1}
	response = callAlbum(t, router, http.MethodGet, "/api/gallery", "", "")
	_, data = decodeEnvelope(t, response)
	if !strings.Contains(string(data), `"uploader":"alice"`) {
		t.Fatalf("gallery item lost uploader: %s", data)
	}
	if images.galleryArgs[0] != 1 || images.galleryArgs[1] != 20 {
		t.Fatalf("gallery defaults = %v", images.galleryArgs)
	}

	response = callAlbum(t, router, http.MethodGet, "/api/gallery?page=-1", "", "")
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
		t.Fatalf("bad page status=%d code=%d", response.Code, code)
	}
	images.galleryErr = service.ErrStorage
	response = callAlbum(t, router, http.MethodGet, "/api/gallery", "", "")
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadGateway || code != 50002 {
		t.Fatalf("gallery error status=%d code=%d", response.Code, code)
	}
}

func TestImageListParsesAlbumFilter(t *testing.T) {
	images := &galleryImagesStub{}
	router := albumRouter(t, nil, images)

	response := callAlbum(t, router, http.MethodGet, "/api/images", "", testCredential)
	if response.Code != http.StatusOK {
		t.Fatalf("list status=%d", response.Code)
	}
	if images.listQuery.AlbumID != nil {
		t.Fatalf("absent album_id filtered: %v", *images.listQuery.AlbumID)
	}

	response = callAlbum(t, router, http.MethodGet, "/api/images?album_id=0", "", testCredential)
	if response.Code != http.StatusOK || images.listQuery.AlbumID == nil || *images.listQuery.AlbumID != 0 {
		t.Fatalf("explicit zero album_id = %v status=%d", images.listQuery.AlbumID, response.Code)
	}

	seven := uint64(7)
	response = callAlbum(t, router, http.MethodGet, "/api/images?album_id=7", "", testCredential)
	if response.Code != http.StatusOK || images.listQuery.AlbumID == nil || *images.listQuery.AlbumID != seven {
		t.Fatalf("album_id 7 = %v", images.listQuery.AlbumID)
	}

	response = callAlbum(t, router, http.MethodGet, "/api/images?album_id=-1", "", testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
		t.Fatalf("negative album_id status=%d code=%d", response.Code, code)
	}
	response = callAlbum(t, router, http.MethodGet, "/api/images?album_id=abc", "", testCredential)
	if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
		t.Fatalf("text album_id status=%d code=%d", response.Code, code)
	}
}

func TestImageBatchAlbumAction(t *testing.T) {
	images := &galleryImagesStub{setAlbumView: service.ImageView{ID: 4, AlbumID: 3}}
	router := albumRouter(t, nil, images)

	response := callAlbum(t, router, http.MethodPost, "/api/images/batch", `{"action":"album","ids":[4,9],"album_id":3}`, testCredential)
	if response.Code != http.StatusMultiStatus {
		t.Fatalf("batch status=%d body=%s", response.Code, response.Body.String())
	}
	if images.setAlbumArgs[0] != uint64(4) || images.setAlbumArgs[1] != uint64(3) {
		t.Fatalf("set album args = %v", images.setAlbumArgs)
	}
	var results []struct {
		ID     uint64          `json:"id"`
		Status int             `json:"status"`
		Code   int             `json:"code"`
		Data   json.RawMessage `json:"data"`
	}
	_, data := decodeEnvelope(t, response)
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Status != 200 || results[0].Code != 0 || !strings.Contains(string(results[0].Data), `"album_id":3`) {
		t.Fatalf("album batch results = %s", data)
	}

	for _, body := range []string{
		`{"action":"album","ids":[4],"is_public":true}`,
		`{"action":"delete","ids":[4],"album_id":3}`,
		`{"action":"permission","ids":[4],"album_id":3}`,
		`{"action":"album","ids":[]}`,
		`{"action":"unknown","ids":[4]}`,
	} {
		response := callAlbum(t, router, http.MethodPost, "/api/images/batch", body, testCredential)
		if code, _ := decodeEnvelope(t, response); response.Code != http.StatusBadRequest || code != 10001 {
			t.Fatalf("invalid batch %s status=%d code=%d", body, response.Code, code)
		}
	}

	images.setAlbumErr = service.ErrForbidden
	response = callAlbum(t, router, http.MethodPost, "/api/images/batch", `{"action":"album","ids":[4],"album_id":3}`, testCredential)
	_, data = decodeEnvelope(t, response)
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	if results[0].Status != http.StatusForbidden || results[0].Code != 20003 {
		t.Fatalf("foreign album item = %s", data)
	}

	images.setAlbumErr = service.ErrNotFound
	response = callAlbum(t, router, http.MethodPost, "/api/images/batch", `{"action":"album","ids":[4],"album_id":3}`, testCredential)
	_, data = decodeEnvelope(t, response)
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	if results[0].Status != http.StatusNotFound || results[0].Code != 10001 {
		t.Fatalf("missing album item = %s", data)
	}
}
