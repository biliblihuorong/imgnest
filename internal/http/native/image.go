package native

import (
	"context"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
	"math"
	"strconv"
	"time"
)

// ImageOptions bounds multipart request memory, parallelism and execution time.
type ImageOptions struct {
	MaxRequestBytes int64
	MaxConcurrent   int
	Timeout         time.Duration
}

// ImageService is the shared image business API consumed by native routes.
type ImageService interface {
	Preflight(context.Context, service.TokenSubject, uint64) (service.UploadLimits, error)
	Upload(context.Context, service.TokenSubject, service.UploadInput) (service.ImageView, error)
	Get(context.Context, service.TokenSubject, uint64) (service.ImageView, error)
	List(context.Context, service.TokenSubject, service.ImageQuery) (service.ImagePage, error)
	Exif(context.Context, service.TokenSubject, uint64) (model.ImageExif, error)
	ListPolicies(context.Context, service.TokenSubject) ([]service.PolicySummary, error)
	SetPublic(context.Context, service.TokenSubject, uint64, bool) (service.ImageView, error)
	SetAlbum(context.Context, service.TokenSubject, uint64, uint64) (service.ImageView, error)
	Gallery(context.Context, int, int) (service.GalleryPage, error)
	Trash(context.Context, service.TokenSubject, string) error
	Restore(context.Context, service.TokenSubject, string) error
	Purge(context.Context, service.TokenSubject, string) error
	OpenPublic(context.Context, uint64, string) (service.PublicObject, error)
	Thumbnail(context.Context, service.TokenSubject, string) (service.PublicObject, error)
}

// RegisterImageRoutes binds the M2 native image API when configured.
func (h *Handler) RegisterImageRoutes(ctx context.Context, router gin.IRouter, images ImageService, opts ImageOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if images == nil || opts.MaxRequestBytes < 0 || opts.MaxConcurrent < 0 || opts.Timeout < 0 {
		return service.ErrInvalidInput
	}
	if opts.MaxRequestBytes == 0 {
		opts.MaxRequestBytes = 64 << 20
	}
	if opts.MaxConcurrent == 0 {
		opts.MaxConcurrent = 2
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
	image := &imageHandler{auth: h, images: images, options: opts, slots: make(chan struct{}, opts.MaxConcurrent)}
	protected := router.Group("/api", h.authenticate)
	protected.POST("/upload", image.upload)
	protected.GET("/policies", image.listPolicies)
	protected.GET("/images", image.list)
	protected.GET("/images/:id", image.get)
	protected.GET("/images/:id/exif", image.exif)
	protected.PATCH("/images/:id", image.permission)
	protected.DELETE("/images/:id", image.remove)
	protected.POST("/images/batch", image.batch)
	protected.GET("/trash", image.listTrash)
	protected.POST("/trash/restore", image.restore)
	protected.POST("/trash/purge", image.purge)
	// The gallery is a public read-only endpoint and deliberately sits
	// outside the authenticated group, like /api/site.
	router.GET("/api/gallery", image.gallery)
	router.GET("/i/:storageID/*key", image.publicObject)
	router.HEAD("/i/:storageID/*key", image.publicObject)
	router.GET("/t/:key", image.thumbnail)
	router.HEAD("/t/:key", image.thumbnail)
	return nil
}

type imageHandler struct {
	auth    *Handler
	images  ImageService
	options ImageOptions
	slots   chan struct{}
}
type imageResult struct {
	ID       uint64 `json:"id,omitempty"`
	Filename string `json:"filename,omitempty"`
	Status   int    `json:"status"`
	Response
}

func imageID(c *gin.Context) (uint64, bool) {
	id, err := parseImageID(c.Param("id"))
	if err != nil {
		fail(c, err)
		return 0, false
	}
	return id, true
}
func parseImageID(value string) (uint64, error) {
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 || id > math.MaxInt64 {
		return 0, service.ErrInvalidInput
	}
	return id, nil
}
func (h *imageHandler) get(c *gin.Context) {
	id, ok := imageID(c)
	if !ok {
		return
	}
	image, err := h.images.Get(c.Request.Context(), identity(c).Subject, id)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, image)
}
func (h *imageHandler) exif(c *gin.Context) {
	id, ok := imageID(c)
	if !ok {
		return
	}
	metadata, err := h.images.Exif(c.Request.Context(), identity(c).Subject, id)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, metadata)
}
func (h *imageHandler) list(c *gin.Context)      { h.listImages(c, false) }
func (h *imageHandler) listTrash(c *gin.Context) { h.listImages(c, true) }
func (h *imageHandler) listImages(c *gin.Context, trash bool) {
	page, size, ok := pageParams(c)
	if !ok {
		return
	}
	// album_id semantics: absent means no filter, explicit zero selects
	// unassigned images, a positive value selects one owner-verified album.
	// This differs from the v1 quirk where a missing album_id already means
	// unassigned.
	var albumFilter *uint64
	if value, exists := c.GetQuery("album_id"); exists {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed > math.MaxInt64 {
			fail(c, service.ErrInvalidInput)
			return
		}
		albumFilter = &parsed
	}
	query := service.ImageQuery{Page: page, Size: size, Trash: trash, AlbumID: albumFilter, Keyword: c.Query("keyword"), Q: c.Query("q"), Order: c.Query("order"), Exif: c.Query("exif")}
	if len(query.Keyword) > 200 || len(query.Exif) > 200 || len(query.Q) > 200 {
		fail(c, service.ErrInvalidInput)
		return
	}
	var err error
	if query.MinSize, query.MaxSize, err = sizeRangeParams(c); err != nil {
		fail(c, err)
		return
	}
	if query.From, query.To, err = timeRangeParams(c); err != nil {
		fail(c, err)
		return
	}
	result, err := h.images.List(c.Request.Context(), identity(c).Subject, query)
	if err != nil {
		fail(c, err)
		return
	}
	if result.Items == nil {
		result.Items = []service.ImageView{}
	}
	respond(c, 200, result)
}

// sizeRangeParams parses the optional byte-size bounds; zero means unbounded.
func sizeRangeParams(c *gin.Context) (int64, int64, error) {
	var minSize, maxSize int64
	if value, exists := c.GetQuery("min_size"); exists {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			return 0, 0, service.ErrInvalidInput
		}
		minSize = parsed
	}
	if value, exists := c.GetQuery("max_size"); exists {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed < 0 {
			return 0, 0, service.ErrInvalidInput
		}
		maxSize = parsed
	}
	return minSize, maxSize, nil
}

// timeRangeParams parses the optional RFC3339 upload-time window.
func timeRangeParams(c *gin.Context) (*time.Time, *time.Time, error) {
	var from, to *time.Time
	if value, exists := c.GetQuery("from"); exists {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil, nil, service.ErrInvalidInput
		}
		from = &parsed
	}
	if value, exists := c.GetQuery("to"); exists {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil, nil, service.ErrInvalidInput
		}
		to = &parsed
	}
	return from, to, nil
}
func (h *imageHandler) gallery(c *gin.Context) {
	page, size, ok := pageParams(c)
	if !ok {
		return
	}
	result, err := h.images.Gallery(c.Request.Context(), page, size)
	if err != nil {
		fail(c, err)
		return
	}
	if result.Items == nil {
		result.Items = []service.GalleryItem{}
	}
	respond(c, 200, result)
}
func (h *imageHandler) permission(c *gin.Context) {
	id, ok := imageID(c)
	if !ok {
		return
	}
	var input struct {
		Public *bool `json:"is_public"`
	}
	if !decode(c, &input) {
		return
	}
	if input.Public == nil {
		fail(c, service.ErrInvalidInput)
		return
	}
	image, err := h.images.SetPublic(c.Request.Context(), identity(c).Subject, id, *input.Public)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, image)
}
func (h *imageHandler) remove(c *gin.Context) {
	id, ok := imageID(c)
	if !ok {
		return
	}
	if _, err := h.perform(c.Request.Context(), identity(c).Subject, id, "delete", false, 0); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}
func (h *imageHandler) batch(c *gin.Context) {
	var input struct {
		Action  string   `json:"action"`
		IDs     []uint64 `json:"ids"`
		Public  *bool    `json:"is_public"`
		AlbumID uint64   `json:"album_id"`
	}
	if !decode(c, &input) {
		return
	}
	// Each action owns a disjoint body: album takes album_id (0 = move out),
	// permission takes is_public, delete takes neither.
	switch input.Action {
	case "delete":
		if input.Public != nil || input.AlbumID != 0 {
			fail(c, service.ErrInvalidInput)
			return
		}
	case "permission":
		if input.Public == nil || input.AlbumID != 0 {
			fail(c, service.ErrInvalidInput)
			return
		}
	case "album":
		if input.Public != nil {
			fail(c, service.ErrInvalidInput)
			return
		}
	default:
		fail(c, service.ErrInvalidInput)
		return
	}
	value := input.Public != nil && *input.Public
	h.batchActions(c, input.IDs, input.Action, value, input.AlbumID)
}
func (h *imageHandler) restore(c *gin.Context) { h.trashBatch(c, "restore") }
func (h *imageHandler) purge(c *gin.Context)   { h.trashBatch(c, "purge") }
func (h *imageHandler) trashBatch(c *gin.Context, action string) {
	var input struct {
		IDs []uint64 `json:"ids"`
	}
	if !decode(c, &input) {
		return
	}
	h.batchActions(c, input.IDs, action, false, 0)
}
func (h *imageHandler) batchActions(c *gin.Context, ids []uint64, action string, public bool, albumID uint64) {
	if len(ids) == 0 || len(ids) > 100 {
		fail(c, service.ErrInvalidInput)
		return
	}
	seen := map[uint64]bool{}
	for _, id := range ids {
		if id == 0 || id > math.MaxInt64 || seen[id] {
			fail(c, service.ErrInvalidInput)
			return
		}
		seen[id] = true
	}
	results := make([]imageResult, 0, len(ids))
	for _, id := range ids {
		data, err := h.perform(c.Request.Context(), identity(c).Subject, id, action, public, albumID)
		item := imageResult{ID: id, Status: 200, Response: Response{Code: 0, Message: "ok", Data: data}}
		if err != nil {
			item.Status, item.Response = errorResponse(err)
		}
		results = append(results, item)
	}
	respond(c, 207, results)
}
func (h *imageHandler) perform(ctx context.Context, subject service.TokenSubject, id uint64, action string, public bool, albumID uint64) (any, error) {
	image, err := h.images.Get(ctx, subject, id)
	if err != nil {
		return nil, err
	}
	switch action {
	case "delete":
		return nil, h.images.Trash(ctx, subject, image.Key)
	case "restore":
		return nil, h.images.Restore(ctx, subject, image.Key)
	case "purge":
		return nil, h.images.Purge(ctx, subject, image.Key)
	case "permission":
		return h.images.SetPublic(ctx, subject, id, public)
	case "album":
		return h.images.SetAlbum(ctx, subject, id, albumID)
	}
	return nil, service.ErrInvalidInput
}
