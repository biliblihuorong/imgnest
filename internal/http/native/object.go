package native

import (
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (h *imageHandler) publicObject(c *gin.Context) {
	id, err := parseImageID(c.Param("storageID"))
	if err != nil {
		fail(c, err)
		return
	}
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		fail(c, service.ErrNotFound)
		return
	}
	object, err := h.images.OpenPublic(c.Request.Context(), id, key)
	if err != nil {
		fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	streamObject(c, object)
}
func (h *imageHandler) thumbnail(c *gin.Context) {
	subject := service.TokenSubject{}
	if c.GetHeader("Authorization") != "" {
		h.auth.authenticate(c)
		if c.IsAborted() {
			return
		}
		subject = identity(c).Subject
	}
	name := c.Param("key")
	if !strings.HasSuffix(name, ".webp") || len(name) <= len(".webp") {
		fail(c, service.ErrNotFound)
		return
	}
	object, err := h.images.Thumbnail(c.Request.Context(), subject, strings.TrimSuffix(name, ".webp"))
	if err != nil {
		fail(c, err)
		return
	}
	streamObject(c, object)
}
func streamObject(c *gin.Context, object service.PublicObject) {
	if object.Body != nil {
		defer func() { _ = object.Body.Close() }()
	}
	if object.Redirect != "" {
		c.Redirect(http.StatusFound, object.Redirect)
		return
	}
	if object.Body == nil || object.Size < 0 {
		c.Header("Cache-Control", "no-store")
		fail(c, service.ErrStorage)
		return
	}
	c.Header("Content-Type", object.MIME)
	c.Header("Content-Length", strconv.FormatInt(object.Size, 10))
	c.Status(http.StatusOK)
	if c.Request.Method == http.MethodHead {
		return
	}
	// Once streaming begins no JSON error can safely be appended to image bytes.
	if _, err := io.CopyN(c.Writer, object.Body, object.Size); err != nil {
		c.Abort()
	}
}
