package http

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/gin-gonic/gin"
)

// spaHandler serves the embedded single-page app with history-mode fallback.
// Reserved native prefixes (/api, /i, /t, /healthz, the crawler files) and
// non-GET/HEAD verbs never fall back to HTML, so unknown API endpoints keep
// their JSON 404.
type spaHandler struct {
	files fs.FS
	cache sync.Map // map[string][]byte — embedded FS is immutable, so cache forever
}

func (s *spaHandler) reserved(requestPath string) bool {
	switch requestPath {
	case "/healthz", "/robots.txt", "/sitemap.xml":
		return true
	}
	for _, prefix := range []string{"/api", "/i", "/t"} {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}

func (s *spaHandler) handle(c *gin.Context) {
	if s.reserved(c.Request.URL.Path) || (c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead) {
		c.JSON(http.StatusNotFound, native.Response{Code: 10001, Message: "not found", Data: nil})
		return
	}
	if name := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/"); name != "" && name != "." {
		// io/fs rejects invalid and escaping names, so Open cannot leave the
		// embedded root; missing and directory names fall back to the index.
		if data, ok := s.readFile(name); ok {
			s.serveFile(c, name, data)
			return
		}
	}
	s.serveIndex(c)
}

func (s *spaHandler) readFile(name string) ([]byte, bool) {
	if cached, ok := s.cache.Load(name); ok {
		return cached.([]byte), true
	}
	f, err := s.files.Open(name)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return nil, false
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, false
	}
	s.cache.Store(name, data)
	return data, true
}

func (s *spaHandler) serveFile(c *gin.Context, name string, data []byte) {
	cache := "no-cache"
	if strings.HasPrefix(name, "assets/") {
		// Vite content-hashed filenames are safe to cache forever.
		cache = "public, max-age=31536000, immutable"
	}
	c.Writer.Header().Set("Cache-Control", cache)
	http.ServeContent(c.Writer, c.Request, path.Base(name), time.Time{}, bytes.NewReader(data))
}

func (s *spaHandler) serveIndex(c *gin.Context) {
	data, ok := s.readFile("index.html")
	if !ok {
		c.JSON(http.StatusInternalServerError, native.Response{Code: 50001, Message: "internal error", Data: nil})
		return
	}
	// The SPA entry must always revalidate: a stale index would reference
	// hashed assets that no longer exist after a redeploy.
	c.Writer.Header().Set("Cache-Control", "no-store")
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Only the public home page and gallery may appear in search results;
	// the console, login and every other app route stay out of indexes even
	// when robots.txt lets crawlers in.
	if !indexablePage(c.Request.URL.Path) {
		c.Writer.Header().Set("X-Robots-Tag", "noindex, nofollow")
	}
	// gin presets the writer status to 404 before NoRoute handlers run;
	// unlike http.ServeContent (used in serveFile), a manual write would
	// otherwise keep that status.
	c.Status(http.StatusOK)
	_, _ = c.Writer.Write(data)
}

// indexablePage reports whether an app route is one of the public pages a
// search engine may index.
func indexablePage(requestPath string) bool {
	switch strings.TrimSuffix(requestPath, "/") {
	case "", "/home", "/gallery":
		return true
	}
	return false
}
