package lsky_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/lsky"
)

// A multipart token request is bounded like the JSON one: an anonymous caller
// cannot make the server spool a large form to temporary files.
func TestV1TokenMultipartBodyIsBounded(t *testing.T) {
	fixture := newV1Fixture(t, "sqlite")
	post := func(padding int) bool {
		t.Helper()
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		for _, field := range [][2]string{{"email", "alice@example.com"}, {"password", alicePassword}} {
			if err := writer.WriteField(field[0], field[1]); err != nil {
				t.Fatal(err)
			}
		}
		if padding > 0 {
			part, err := writer.CreateFormFile("padding", "padding.bin")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(bytes.Repeat([]byte{'x'}, padding)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		response := fixture.request(t, "POST", "/api/v1/tokens", body.String(), writer.FormDataContentType(), "")
		mustStatus(t, response, 200)
		var envelope struct {
			Status bool `json:"status"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Status
	}
	if !post(0) {
		t.Fatal("small multipart login refused")
	}
	if post(1 << 20) {
		t.Fatal("1 MiB multipart token request was parsed")
	}
}

// A stalled upload body must end at the handler timeout instead of holding an
// upload slot; guests reach this path without any credentials.
func TestV1UploadReadTimeout(t *testing.T) {
	fixture := newV1Fixture(t, "sqlite")
	fixture.enableGuestUpload(t)
	handler, err := lsky.NewHandler(t.Context(), lsky.Dependencies{
		Users: fixture.users, Tokens: fixture.tokens, Images: fixture.images, Albums: fixture.albums, Lsky: fixture.lsky,
		Now: func() time.Time { return fixedNow }, Options: lsky.Options{Timeout: 50 * time.Millisecond, MaxConcurrent: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	router := ginRouter(t, handler)
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	r := httptest.NewRequest("POST", "/api/v1/upload", reader)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("Content-Type", "multipart/form-data; boundary=waiting")
	w := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() { router.ServeHTTP(w, r); close(finished) }()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("stalled v1 upload body ignored the timeout")
	}
	if !strings.Contains(w.Body.String(), "请求超时或已取消") {
		t.Fatalf("stalled upload response = %s", w.Body.String())
	}
}
