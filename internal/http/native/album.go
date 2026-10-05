package native

import (
	"context"
	"math"
	"strconv"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Albums is the album business API consumed by native routes.
type Albums interface {
	List(context.Context, uint64, service.AlbumQuery) (service.AlbumPage, error)
	Create(context.Context, uint64, service.AlbumInput) (service.AlbumView, error)
	Update(context.Context, uint64, uint64, service.AlbumPatch) (service.AlbumView, error)
	Delete(context.Context, uint64, uint64) error
}

// RegisterAlbumRoutes binds the M5 native album API to authenticated routes.
func (h *Handler) RegisterAlbumRoutes(ctx context.Context, router gin.IRouter, albums Albums) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if albums == nil {
		return service.ErrInvalidInput
	}
	album := &albumHandler{auth: h, albums: albums}
	protected := router.Group("/api", h.authenticate)
	protected.GET("/albums", album.list)
	protected.GET("/albums/suggestions", album.suggestions)
	protected.POST("/albums", album.create)
	protected.PATCH("/albums/:id", album.update)
	protected.DELETE("/albums/:id", album.remove)
	return nil
}

type albumHandler struct {
	auth   *Handler
	albums Albums
}

func albumID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 || id > math.MaxInt64 {
		fail(c, service.ErrInvalidInput)
		return 0, false
	}
	return id, true
}

func (h *albumHandler) list(c *gin.Context) {
	page, size, ok := pageParams(c)
	if !ok {
		return
	}
	page1, err := h.albums.List(c.Request.Context(), identity(c).User.ID, service.AlbumQuery{Page: page, Size: size, Keyword: c.Query("keyword")})
	if err != nil {
		fail(c, err)
		return
	}
	if page1.Items == nil {
		page1.Items = []service.AlbumView{}
	}
	respond(c, 200, page1)
}

func (h *albumHandler) create(c *gin.Context) {
	var input struct {
		Name         string `json:"name"`
		Intro        string `json:"intro"`
		IsPublic     bool   `json:"is_public"`
		CoverImageID uint64 `json:"cover_image_id"`
	}
	if !decode(c, &input) {
		return
	}
	view, err := h.albums.Create(c.Request.Context(), identity(c).User.ID, service.AlbumInput{
		Name: input.Name, Intro: input.Intro, IsPublic: input.IsPublic, CoverImageID: input.CoverImageID,
	})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, view)
}

func (h *albumHandler) update(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	var input struct {
		Name         *string `json:"name"`
		Intro        *string `json:"intro"`
		IsPublic     *bool   `json:"is_public"`
		CoverImageID *uint64 `json:"cover_image_id"`
	}
	if !decode(c, &input) {
		return
	}
	patch := service.AlbumPatch{Name: input.Name, Intro: input.Intro, IsPublic: input.IsPublic, CoverImageID: input.CoverImageID}
	if patch.Name == nil && patch.Intro == nil && patch.IsPublic == nil && patch.CoverImageID == nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	view, err := h.albums.Update(c.Request.Context(), identity(c).User.ID, id, patch)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}

func (h *albumHandler) remove(c *gin.Context) {
	id, ok := albumID(c)
	if !ok {
		return
	}
	if err := h.albums.Delete(c.Request.Context(), identity(c).User.ID, id); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}

// pageParams bounds the shared native pagination pair; nonpositive or
// oversized values reject the request.
func pageParams(c *gin.Context) (page, size int, ok bool) {
	page, size = 1, 20
	for name, target := range map[string]*int{"page": &page, "size": &size} {
		if value, exists := c.GetQuery(name); exists {
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				fail(c, service.ErrInvalidInput)
				return 0, 0, false
			}
			*target = parsed
		}
	}
	if size > 100 || page > (math.MaxInt/size) {
		fail(c, service.ErrInvalidInput)
		return 0, 0, false
	}
	return page, size, true
}
