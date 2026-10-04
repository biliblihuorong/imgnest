package storage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func testS3(t *testing.T, handler http.HandlerFunc) *S3 {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	d, err := NewS3(t.Context(), S3Config{Endpoint: server.URL, Region: "us-east-1", Bucket: "images", AccessKeyID: "test-access", SecretAccessKey: "test-secret", UsePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestS3PutWireContract(t *testing.T) {
	var seen bool
	d := testS3(t, func(w http.ResponseWriter, r *http.Request) {
		seen = true
		if r.Method != "PUT" || r.URL.EscapedPath() != "/images/album/%E6%97%85%E8%A1%8C.png" || r.ContentLength != 5 || r.Header.Get("If-None-Match") != "*" || r.Header.Get("X-Amz-Meta-ImgNest-Owner") != "image-key" {
			t.Errorf("wrong put contract %s %s length=%d", r.Method, r.URL.EscapedPath(), r.ContentLength)
		}
		for name := range r.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-amz-checksum-") {
				t.Error("unexpected optional checksum")
			}
		}
		data, err := io.ReadAll(r.Body)
		if err != nil || string(data) != "bytes" {
			t.Error("wrong body")
		}
		w.Header().Set("X-Amz-Version-Id", "version-one")
		w.WriteHeader(200)
	})
	receipt, err := d.putNew(t.Context(), "album/旅行.png", strings.NewReader("bytes"), PutOptions{OwnerID: "image-key", MIME: "image/png"})
	if err != nil || !seen || receipt.VersionID != "version-one" || receipt.Size != 5 {
		t.Fatalf("receipt %+v err=%v", receipt, err)
	}
}
func TestS3ErrorsRedacted(t *testing.T) {
	d := testS3(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(501)
		_, _ = io.WriteString(w, `<Error><Code>NotImplemented</Code><Message>private-secret-url</Message></Error>`)
	})
	_, err := d.putNew(t.Context(), "a.jpg", strings.NewReader("bytes"), PutOptions{OwnerID: "one"})
	if !errors.Is(err, ErrUnsupported) || strings.Contains(err.Error(), "private-secret-url") {
		t.Fatalf("unsafe error %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.Stat(ctx, "a.jpg"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestS3PurgeExactVersionsAndMarkersAcrossPages(t *testing.T) {
	var mu sync.Mutex
	var deleted []string
	d := testS3(t, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method == "DELETE" {
			mu.Lock()
			deleted = append(deleted, r.URL.Path+":"+q.Get("versionId"))
			mu.Unlock()
			w.WriteHeader(204)
			return
		}
		if r.Method != "GET" || !q.Has("versions") || q.Get("prefix") != "a.jpg" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		if q.Get("key-marker") == "" {
			_, _ = io.WriteString(w, `<ListVersionsResult><IsTruncated>true</IsTruncated><NextKeyMarker>a.jpg</NextKeyMarker><NextVersionIdMarker>v1</NextVersionIdMarker><Version><Key>a.jpg</Key><VersionId>v1</VersionId></Version><Version><Key>a.jpg-neighbor</Key><VersionId>foreign</VersionId></Version></ListVersionsResult>`)
		} else {
			_, _ = io.WriteString(w, `<ListVersionsResult><IsTruncated>false</IsTruncated><DeleteMarker><Key>a.jpg</Key><VersionId>marker</VersionId></DeleteMarker><Version><Key>a.jpg</Key><VersionId>v2</VersionId></Version></ListVersionsResult>`)
		}
	})
	if err := d.PurgeAllVersions(t.Context(), "a.jpg"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(deleted, ",") != "/images/a.jpg:v1,/images/a.jpg:v2,/images/a.jpg:marker" {
		t.Fatalf("deleted %v", deleted)
	}
}
func TestS3CopyUsesConditionalCompletion(t *testing.T) {
	var escaped string
	var completed bool
	d := testS3(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.Method == "POST" && r.URL.Query().Has("uploads"):
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><UploadId>upload-one</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == "PUT":
			escaped = r.Header.Get("X-Amz-Copy-Source")
			_, _ = io.WriteString(w, `<CopyPartResult><ETag>"copied"</ETag></CopyPartResult>`)
		case r.Method == "POST":
			completed = true
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("copy completion can overwrite")
			}
			w.Header().Set("X-Amz-Version-Id", "copy-version")
			_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><ETag>"done"</ETag></CompleteMultipartUploadResult>`)
		default:
			t.Errorf("unexpected request %s", r.Method)
			w.WriteHeader(400)
		}
	})
	receipt, err := d.copyNew(t.Context(), "album/旅行 #.png", "_trash/a.png", ObjectInfo{Size: 5, OwnerID: "owner", VersionID: "v+1"}, CopyOptions{OwnerID: "owner", MIME: "image/png"})
	if err != nil || !completed || receipt.VersionID != "copy-version" {
		t.Fatalf("copy %+v %v", receipt, err)
	}
	if escaped != "images/album/%E6%97%85%E8%A1%8C%20%23.png?versionId=v%2B1" {
		t.Fatalf("copy source %s", escaped)
	}
}

func TestS3PurgeOwnedRetainsForeignVersionsAndMarkers(t *testing.T) {
	var removed []string
	d := testS3(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			_, _ = io.WriteString(w, `<ListVersionsResult><IsTruncated>false</IsTruncated><Version><Key>a.jpg</Key><VersionId>owned</VersionId></Version><Version><Key>a.jpg</Key><VersionId>foreign</VersionId></Version><DeleteMarker><Key>a.jpg</Key><VersionId>marker</VersionId></DeleteMarker></ListVersionsResult>`)
		case "HEAD":
			owner := "other"
			if r.URL.Query().Get("versionId") == "owned" {
				owner = "one"
			}
			w.Header().Set("X-Amz-Meta-ImgNest-Owner", owner)
		case "DELETE":
			removed = append(removed, r.URL.Query().Get("versionId"))
			w.WriteHeader(204)
		}
	})
	if err := d.PurgeOwned(t.Context(), "a.jpg", "one"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(removed, ",") != "owned" {
		t.Fatalf("deleted non-owned version %v", removed)
	}
}
func TestS3RejectsIgnoredPutPrecondition(t *testing.T) {
	d := testS3(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = io.WriteString(w, `<ListVersionsResult><IsTruncated>false</IsTruncated></ListVersionsResult>`)
			return
		}
		w.WriteHeader(200)
	})
	if err := d.Check(t.Context()); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ignored precondition accepted: %v", err)
	}
}

func TestS3AmbiguousPutReturnsOwnershipReceipt(t *testing.T) {
	var committed atomic.Bool
	d := testS3(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Error(err)
			}
			committed.Store(true)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			if err := conn.Close(); err != nil {
				t.Error(err)
			}
			return
		}
		if !committed.Load() {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Length", "5")
		w.Header().Set("X-Amz-Meta-ImgNest-Owner", "image-key")
		w.Header().Set("X-Amz-Version-Id", "committed-version")
	})
	receipt, err := d.putNew(t.Context(), "a.jpg", strings.NewReader("bytes"), PutOptions{OwnerID: "image-key"})
	if err == nil || receipt.Key != "a.jpg" || receipt.OwnerID != "image-key" || receipt.Size != 5 {
		t.Fatalf("ambiguous receipt %+v %v", receipt, err)
	}
	info, err := d.Stat(t.Context(), receipt.Key)
	if err != nil || info.OwnerID != receipt.OwnerID || info.VersionID != "committed-version" {
		t.Fatalf("cannot recover committed write %+v %v", info, err)
	}
}
