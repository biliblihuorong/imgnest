package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
)

func processImageSmoke(t *testing.T, _ string, origin, token string, environ []string, client *http.Client, call func(string, string, string, string, string, int) json.RawMessage) {
	t.Helper()
	ctx := t.Context()
	cfg, err := config.Load(ctx, "", environ)
	if err != nil {
		t.Fatal(err)
	}
	db, err := repo.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	stores, err := repo.NewStorageRepository(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stores.SetBaseURL(ctx, 1, origin+"/i/1"); err != nil {
		t.Fatal(err)
	}
	fixture := image.NewRGBA(image.Rect(0, 0, 128, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 128; x++ {
			fixture.SetRGBA(x, y, color.RGBA{R: uint8((x*7 + y*3) % 256), G: uint8((x + y*11) % 256), B: 99, A: 255})
		}
	}
	var source bytes.Buffer
	if err = jpeg.Encode(&source, fixture, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "pretend.png")
	if err = os.WriteFile(file, source.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	// #nosec G204 -- fixed curl command, test-owned config on stdin; no credential in shell/argv or logs.
	curl := exec.CommandContext(ctx, "curl", "--silent", "--show-error", "--fail-with-body", "--config", "-")
	curl.Stdin = strings.NewReader(fmt.Sprintf("url = %q\nheader = %q\nform = %q\n", origin+"/api/upload", "Authorization: Bearer "+token, "file=@"+file+";filename=pretend.png;type=image/png"))
	var response, stderr bytes.Buffer
	curl.Stdout = &response
	curl.Stderr = &stderr
	if err = curl.Run(); err != nil {
		t.Fatal("actual curl image upload failed")
	}
	var envelope struct {
		Code int               `json:"code"`
		Data service.ImageView `json:"data"`
	}
	if err = json.Unmarshal(response.Bytes(), &envelope); err != nil || envelope.Code != 0 {
		t.Fatal("invalid curl image response")
	}
	imageView := envelope.Data
	if imageView.Ext != "jpg" || !imageView.HasOriginal || !imageView.HasWebP || !imageView.HasThumb || imageView.IsPublic {
		t.Fatal("actual pipeline did not publish private JPEG/WebP/thumb")
	}
	fetch := func(link string, want int) {
		t.Helper()
		parsed, err := url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, "GET", origin+parsed.Path, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil || res.StatusCode != want {
			t.Fatalf("image link status=%d want%d", res.StatusCode, want)
		}
		if want == 200 && len(data) == 0 {
			t.Fatal("empty image")
		}
	}
	fetch(imageView.Links.Original, 200)
	fetch(imageView.Links.WebP, 200)
	fetch(imageView.Links.Thumbnail, 200)
	id := fmt.Sprint(imageView.ID)
	call(origin, "GET", "/api/images/"+id+"/exif", "", token, 200)
	call(origin, "DELETE", "/api/images/"+id, "", token, 200)
	fetch(imageView.Links.Original, 404)
	call(origin, "POST", "/api/trash/restore", `{"ids":[`+id+`]}`, token, 207)
	fetch(imageView.Links.Original, 200)
	call(origin, "DELETE", "/api/images/"+id, "", token, 200)
	call(origin, "POST", "/api/trash/purge", `{"ids":[`+id+`]}`, token, 207)
	fetch(imageView.Links.Original, 404)
}
