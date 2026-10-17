package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

// guestImagesStub adds the anonymous upload half to the account stub.
type guestImagesStub struct {
	*imagesStub
	guestInputs []service.UploadInput
}

func (s *guestImagesStub) GuestPreflight(context.Context, uint64) (service.UploadLimits, error) {
	return service.UploadLimits{MaxFileBytes: 1024, PerMinute: s.perMinute}, s.preflightErr
}
func (s *guestImagesStub) GuestPreflightLimits(context.Context) (service.UploadLimits, error) {
	return service.UploadLimits{MaxFileBytes: 1024, PerMinute: s.perMinute}, s.preflightErr
}
func (s *guestImagesStub) GuestUpload(_ context.Context, input service.UploadInput) (service.ImageView, error) {
	s.guestInputs = append(s.guestInputs, input)
	return service.ImageView{ID: 2, Key: "guest-key", Name: input.Filename, Links: service.ImageLinks{URL: "http://images.test/guest.webp"}}, nil
}

type guestStateStub struct{ open bool }

func (s *guestStateStub) GuestUploadState(context.Context) (bool, model.Group, bool, error) {
	return s.open, model.Group{ID: 3}, s.open, nil
}

func guestRouter(t *testing.T, images *guestImagesStub, state native.GuestState) http.Handler {
	t.Helper()
	router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{Users: &usersStub{}, Tokens: &tokensStub{}, Images: images, Guest: state, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Now: time.Now, Health: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func guestUpload(router http.Handler, body []byte, contentType, ip string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/guest/upload", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.RemoteAddr = ip + ":1234"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestGuestUploadFollowsTheSwitch(t *testing.T) {
	images := &guestImagesStub{imagesStub: &imagesStub{}}
	state := &guestStateStub{}
	router := guestRouter(t, images, state)
	body, contentType := multipartBody(t, multipartEntry{"file", "a.png", "png"}, multipartEntry{"is_public", "", "true"})

	closed := guestUpload(router, body, contentType, "192.0.2.1")
	expectCode(t, closed, 401, 20001)
	limits := request(t, router, "GET", "/api/guest/upload/limits", "", "")
	expectCode(t, limits, 401, 20001)

	state.open = true
	uploaded := guestUpload(router, body, contentType, "192.0.2.1")
	expectCode(t, uploaded, 201, 0)
	if len(images.guestInputs) != 1 || len(images.inputs) != 0 {
		t.Fatalf("guest upload went to the wrong path: guest=%d account=%d", len(images.guestInputs), len(images.inputs))
	}
	// Anonymous uploads never land in the public gallery.
	if images.guestInputs[0].IsPublic || images.guestInputs[0].IP != "192.0.2.1" {
		t.Fatalf("guest input = %+v", images.guestInputs[0])
	}
	limits = request(t, router, "GET", "/api/guest/upload/limits", "", "")
	expectCode(t, limits, 200, 0)
	var view struct {
		MaxFileBytes int64 `json:"max_file_bytes"`
		MaxFiles     int   `json:"max_files"`
	}
	if err := json.Unmarshal(envelope(t, limits)["data"], &view); err != nil || view.MaxFileBytes != 1024 || view.MaxFiles != 20 {
		t.Fatalf("limits = %+v %v", view, err)
	}

	album, albumType := multipartBody(t, multipartEntry{"file", "a.png", "png"}, multipartEntry{"album_id", "", "4"})
	expectCode(t, guestUpload(router, album, albumType, "192.0.2.1"), 403, 20003)
}

func TestGuestUploadSpendsAPerAddressAllowance(t *testing.T) {
	images := &guestImagesStub{imagesStub: &imagesStub{perMinute: 1}}
	router := guestRouter(t, images, &guestStateStub{open: true})
	body, contentType := multipartBody(t, multipartEntry{"file", "a.png", "png"})
	expectCode(t, guestUpload(router, body, contentType, "192.0.2.1"), 201, 0)
	expectCode(t, guestUpload(router, body, contentType, "192.0.2.1"), 429, 30003)
	// Another visitor has an allowance of their own.
	expectCode(t, guestUpload(router, body, contentType, "192.0.2.2"), 201, 0)
}

func TestGuestUploadRoutesNeedTheState(t *testing.T) {
	images := &guestImagesStub{imagesStub: &imagesStub{}}
	router := guestRouter(t, images, nil)
	body, contentType := multipartBody(t, multipartEntry{"file", "a.png", "png"})
	if w := guestUpload(router, body, contentType, "192.0.2.1"); w.Code != 404 {
		t.Fatalf("guest route mounted without a guest state: %d", w.Code)
	}
}
