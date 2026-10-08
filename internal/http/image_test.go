package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/http/ratelimit"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

type imagesStub struct {
	native.ImageService
	preflightErr error
	maxFile      int64
	perMinute    int
	inputs       []service.UploadInput
	policies     []uint64
	actions      []string
	closed       atomic.Bool
}

func (s *imagesStub) Preflight(_ context.Context, _ service.TokenSubject, policy uint64) (service.UploadLimits, error) {
	s.policies = append(s.policies, policy)
	limit := s.maxFile
	if limit == 0 {
		limit = 1024
	}
	return service.UploadLimits{MaxFileBytes: limit, PerMinute: s.perMinute}, s.preflightErr
}
func (s *imagesStub) Upload(_ context.Context, _ service.TokenSubject, input service.UploadInput) (service.ImageView, error) {
	s.inputs = append(s.inputs, input)
	if input.Filename == "bad.bin" {
		return service.ImageView{}, service.ErrUnsupportedFormat
	}
	return service.ImageView{ID: 1, Key: "image-key", Name: input.Filename, Size: int64(len(input.Data)), Links: service.ImageLinks{URL: "http://images.test/image.webp"}}, nil
}
func (s *imagesStub) Get(_ context.Context, _ service.TokenSubject, id uint64) (service.ImageView, error) {
	if id == 9 {
		return service.ImageView{}, service.ErrForbidden
	}
	return service.ImageView{ID: id, Key: "image-key"}, nil
}
func (s *imagesStub) List(_ context.Context, _ service.TokenSubject, q service.ImageQuery) (service.ImagePage, error) {
	return service.ImagePage{Items: []service.ImageView{}, Page: q.Page, Size: q.Size}, nil
}
func (s *imagesStub) Exif(_ context.Context, _ service.TokenSubject, id uint64) (model.ImageExif, error) {
	if id == 9 {
		return model.ImageExif{}, service.ErrForbidden
	}
	gps := 1.25
	return model.ImageExif{GPSLat: &gps, Raw: json.RawMessage(`{"private":"metadata"}`)}, nil
}
func (s *imagesStub) Trash(context.Context, service.TokenSubject, string) error {
	s.actions = append(s.actions, "delete")
	return nil
}
func (s *imagesStub) Restore(context.Context, service.TokenSubject, string) error {
	s.actions = append(s.actions, "restore")
	return nil
}
func (s *imagesStub) Purge(context.Context, service.TokenSubject, string) error {
	s.actions = append(s.actions, "purge")
	return nil
}
func (s *imagesStub) SetPublic(_ context.Context, _ service.TokenSubject, id uint64, value bool) (service.ImageView, error) {
	s.actions = append(s.actions, "permission")
	return service.ImageView{ID: id, IsPublic: value}, nil
}

type observedCloser struct {
	io.Reader
	closed *atomic.Bool
}

func (r observedCloser) Close() error { r.closed.Store(true); return nil }
func (s *imagesStub) OpenPublic(_ context.Context, _ uint64, key string) (service.PublicObject, error) {
	if key == "missing.png" {
		return service.PublicObject{}, service.ErrNotFound
	}
	if key == "skip.webp" {
		return service.PublicObject{Redirect: "/i/1/skip.jpg"}, nil
	}
	return service.PublicObject{Body: observedCloser{strings.NewReader("image"), &s.closed}, Size: 5, MIME: "image/png"}, nil
}
func (s *imagesStub) Thumbnail(ctx context.Context, _ service.TokenSubject, key string) (service.PublicObject, error) {
	if key == "private" {
		return service.PublicObject{}, service.ErrForbidden
	}
	return s.OpenPublic(ctx, 1, key)
}

func imageRouter(t *testing.T, images *imagesStub, opts native.ImageOptions) http.Handler {
	t.Helper()
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{Users: &usersStub{}, Tokens: &tokensStub{}, Images: images, ImageOptions: opts, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Now: time.Now, Health: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

type multipartEntry struct{ name, filename, value string }

func multipartBody(t *testing.T, entries ...multipartEntry) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, entry := range entries {
		if entry.filename != "" {
			part, err := writer.CreateFormFile(entry.name, entry.filename)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(part, entry.value); err != nil {
				t.Fatal(err)
			}
		} else if err := writer.WriteField(entry.name, entry.value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}
func uploadRequest(router http.Handler, body []byte, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/upload", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("Authorization", "Bearer "+testBearer)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestImageUploadSingleAndPartialBatch(t *testing.T) {
	images := &imagesStub{}
	router := imageRouter(t, images, native.ImageOptions{})
	body, contentType := multipartBody(t, multipartEntry{"file", "travel.fake", "image"})
	expectCode(t, uploadRequest(router, body, contentType), 201, 0)
	body, contentType = multipartBody(t, multipartEntry{"files[]", "ok.png", "good"}, multipartEntry{"files[]", "bad.bin", "bad"}, multipartEntry{name: "policy_id", value: "2"}, multipartEntry{name: "album_id", value: "3"}, multipartEntry{name: "is_public", value: "true"})
	response := uploadRequest(router, body, contentType)
	expectCode(t, response, 207, 0)
	var results []struct {
		Status, Code int
		Data         json.RawMessage
	}
	if err := json.Unmarshal(envelope(t, response)["data"], &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Status != 201 || results[1].Status != 415 || results[1].Code != 30007 || string(results[1].Data) != "null" {
		t.Fatalf("partial batch %s", response.Body.String())
	}
	if len(images.inputs) != 3 || images.inputs[1].PolicyID != 2 || images.inputs[1].AlbumID != 3 || !images.inputs[1].IsPublic || string(images.inputs[1].Data) != "good" {
		t.Fatal("multipart metadata after files was not applied")
	}
}

type monitoredBody struct{ read atomic.Bool }

func (b *monitoredBody) Read([]byte) (int, error) { b.read.Store(true); return 0, io.EOF }
func (*monitoredBody) Close() error               { return nil }
func TestImageUploadAuthenticatesAndPreflightsBeforeBody(t *testing.T) {
	for _, authorized := range []bool{false, true} {
		images := &imagesStub{preflightErr: service.ErrForbidden}
		router := imageRouter(t, images, native.ImageOptions{})
		body := &monitoredBody{}
		r := httptest.NewRequest("POST", "/api/upload", body)
		r.Header.Set("Content-Type", "multipart/form-data; boundary=x")
		if authorized {
			r.Header.Set("Authorization", "Bearer "+testBearer)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if body.read.Load() {
			t.Fatal("upload read private source before authorization/preflight")
		}
		if authorized {
			expectCode(t, w, 403, 20003)
		} else {
			expectCode(t, w, 401, 20001)
		}
	}
}

func TestImageUploadBoundsAndStrictFields(t *testing.T) {
	images := &imagesStub{maxFile: 4}
	router := imageRouter(t, images, native.ImageOptions{MaxRequestBytes: 4096})
	body, contentType := multipartBody(t, multipartEntry{"file", "large.png", "12345"}, multipartEntry{"file", "small.png", "123"})
	response := uploadRequest(router, body, contentType)
	expectCode(t, response, 207, 0)
	if len(images.inputs) != 1 || images.inputs[0].Filename != "small.png" {
		t.Fatal("oversized file reached service")
	}
	for _, entries := range [][]multipartEntry{{{name: "unknown", value: "x"}, {"file", "a.png", "abc"}}, {{name: "policy_id", value: "1"}, {name: "policy_id", value: "2"}, {"file", "a.png", "abc"}}, {{name: "is_public", value: "invalid"}, {"file", "a.png", "abc"}}} {
		body, contentType = multipartBody(t, entries...)
		expectCode(t, uploadRequest(router, body, contentType), 400, 10001)
	}
	tiny := imageRouter(t, &imagesStub{}, native.ImageOptions{MaxRequestBytes: 20})
	expectCode(t, uploadRequest(tiny, body, contentType), 413, 10002)
}

func TestImageMetadataAndBatchAuthorization(t *testing.T) {
	images := &imagesStub{}
	router := imageRouter(t, images, native.ImageOptions{})
	expectCode(t, request(t, router, "GET", "/api/images/9/exif", "", testBearer), 403, 20003)
	expectCode(t, request(t, router, "GET", "/api/images/1/exif", "", testBearer), 200, 0)
	expectCode(t, request(t, router, "GET", "/api/images?page=0", "", testBearer), 400, 10001)
	expectCode(t, request(t, router, "PATCH", "/api/images/1", `{"is_public":false,"user_id":9}`, testBearer), 400, 10001)
	response := request(t, router, "POST", "/api/images/batch", `{"action":"delete","ids":[1,9]}`, testBearer)
	expectCode(t, response, 207, 0)
	if strings.Join(images.actions, ",") != "delete" {
		t.Fatal("unauthorized batch action ran")
	}
	expectCode(t, request(t, router, "POST", "/api/trash/restore", `{"ids":[1]}`, testBearer), 207, 0)
}

func TestImagePublicObjectsAndOptionalThumbnailAuth(t *testing.T) {
	images := &imagesStub{}
	router := imageRouter(t, images, native.ImageOptions{})
	response := request(t, router, "GET", "/i/1/photo.png", "", "")
	if response.Code != 200 || response.Body.String() != "image" || response.Header().Get("Content-Type") != "image/png" || response.Header().Get("Content-Length") != "5" || !images.closed.Load() {
		t.Fatalf("public object response %d %s", response.Code, response.Body.String())
	}
	expectCode(t, request(t, router, "GET", "/i/1/missing.png", "", ""), 404, 10001)
	redirect := request(t, router, "GET", "/i/1/skip.webp", "", "")
	if redirect.Code != 302 || redirect.Header().Get("Location") != "/i/1/skip.jpg" {
		t.Fatal("skipped webp did not redirect")
	}
	if response := request(t, router, "HEAD", "/i/1/photo.png", "", ""); response.Code != 200 || response.Body.Len() != 0 {
		t.Fatal("HEAD returned body")
	}
	expectCode(t, request(t, router, "GET", "/t/private.webp", "", ""), 403, 20003)
	expectCode(t, request(t, router, "GET", "/t/public.webp", "", "invalid"), 401, 20001)
	if response := request(t, router, "GET", "/t/public.webp", "", ""); response.Code != 200 {
		t.Fatal("anonymous public thumbnail rejected")
	}
}

func TestImageUploadRejectsChunkedOversizedEpilogue(t *testing.T) {
	images := &imagesStub{}
	router := imageRouter(t, images, native.ImageOptions{MaxRequestBytes: 1024})
	body, contentType := multipartBody(t, multipartEntry{"file", "a.png", "abc"})
	body = append(body, bytes.Repeat([]byte("x"), 2048)...)
	r := httptest.NewRequest("POST", "/api/upload", bytes.NewReader(body))
	r.ContentLength = -1
	r.Header.Set("Authorization", "Bearer "+testBearer)
	r.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	expectCode(t, w, 413, 10002)
	if len(images.inputs) != 0 {
		t.Fatal("oversized request committed an upload")
	}
}

func TestImageUploadReadTimeout(t *testing.T) {
	images := &imagesStub{}
	router := imageRouter(t, images, native.ImageOptions{Timeout: 30 * time.Millisecond})
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	r := httptest.NewRequest("POST", "/api/upload", reader)
	r.Header.Set("Authorization", "Bearer "+testBearer)
	r.Header.Set("Content-Type", "multipart/form-data; boundary=waiting")
	w := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() { router.ServeHTTP(w, r); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("body read ignored upload timeout")
	}
	expectCode(t, w, 408, 10004)
}

type gatedImages struct {
	*imagesStub
	started, canceled, release chan struct{}
	calls                      atomic.Int32
}

func (g *gatedImages) Preflight(context.Context, service.TokenSubject, uint64) (service.UploadLimits, error) {
	return service.UploadLimits{MaxFileBytes: 1024}, nil
}
func (g *gatedImages) Upload(ctx context.Context, _ service.TokenSubject, _ service.UploadInput) (service.ImageView, error) {
	g.calls.Add(1)
	close(g.started)
	<-ctx.Done()
	close(g.canceled)
	<-g.release
	return service.ImageView{}, ctx.Err()
}
func TestImageUploadCancellationDoesNotReleaseActiveSlot(t *testing.T) {
	images := &gatedImages{imagesStub: &imagesStub{}, started: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{Users: &usersStub{}, Tokens: &tokensStub{}, Images: images, ImageOptions: native.ImageOptions{MaxConcurrent: 1, Timeout: 30 * time.Millisecond}, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Now: time.Now, Health: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	body, contentType := multipartBody(t, multipartEntry{"file", "image.png", "image"})
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- uploadRequest(router, body, contentType) }()
	select {
	case <-images.canceled:
	case <-time.After(time.Second):
		t.Fatal("first operation did not observe cancellation")
	}
	second := uploadRequest(router, body, contentType)
	expectCode(t, second, 408, 10004)
	if images.calls.Load() != 1 {
		t.Fatal("canceled active processing released concurrency slot")
	}
	close(images.release)
	select {
	case response := <-first:
		expectCode(t, response, 408, 10004)
	case <-time.After(time.Second):
		t.Fatal("completed canceled operation did not return")
	}
}

// The native upload counts against the limiter shared with the v1 API.
func TestImageUploadSharesV1RateLimit(t *testing.T) {
	images := &imagesStub{perMinute: 1}
	shared := ratelimit.New(time.Now, time.Minute, ratelimit.DefaultCapacity)
	// The v1 API already used this minute's only upload for the test user.
	if !shared.Allow(ratelimit.UploadKey(testUser().ID), 1) {
		t.Fatal("fresh limiter refused the first upload")
	}
	router := imageRouter(t, images, native.ImageOptions{Uploads: shared})
	body, contentType := multipartBody(t, multipartEntry{"file", "travel.fake", "image"})
	expectCode(t, uploadRequest(router, body, contentType), 429, 30003)
	if len(images.inputs) != 0 {
		t.Fatal("rate-limited upload reached the service")
	}
}
