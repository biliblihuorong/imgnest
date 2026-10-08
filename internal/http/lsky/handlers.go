package lsky

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// credentialBodyLimit caps token requests, which carry only two short fields.
const credentialBodyLimit = 64 << 10

// createToken exchanges email and password (form or JSON) for an API token.
func (h *Handler) createToken(c *gin.Context) {
	email, password, ok := readCredentials(c)
	if !ok {
		c.JSON(200, failure("The email address or password is incorrect."))
		return
	}
	account := "v1login:" + strings.ToLower(strings.TrimSpace(email))
	if h.logins.Exceeded(account, loginFailureLimit) {
		c.JSON(429, failure("Too Many Attempts."))
		return
	}
	verified, err := h.users.VerifyCredentials(c.Request.Context(), email, password)
	if isCanceled(err) {
		c.JSON(200, failure("请求超时或已取消"))
		return
	}
	if err != nil || verified.User.ID == 0 {
		h.logins.Allow(account, loginFailureLimit)
		c.JSON(200, failure("The email address or password is incorrect."))
		return
	}
	h.logins.Reset(account)
	issued, err := h.tokens.Issue(c.Request.Context(), verified.Subject, service.TokenInput{Name: "api", Kind: service.TokenKindAPI, Abilities: []string{"*"}})
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			c.JSON(200, failure("The email address or password is incorrect."))
			return
		}
		h.serverError(c)
		return
	}
	c.JSON(200, success("success", gin.H{"token": issued.Token}))
}

// readCredentials accepts form and JSON token requests. Both share one body
// cap: gin would otherwise spool an unbounded multipart form to temporary
// files for an unauthenticated caller.
func readCredentials(c *gin.Context) (string, string, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, credentialBodyLimit)
	if strings.HasPrefix(c.ContentType(), "application/json") {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil || body.Email == "" || body.Password == "" {
			return "", "", false
		}
		return body.Email, body.Password, true
	}
	email, password := c.PostForm("email"), c.PostForm("password")
	if email == "" || password == "" {
		return "", "", false
	}
	return email, password, true
}

// revokeTokens removes every token owned by the caller.
func (h *Handler) revokeTokens(c *gin.Context) {
	id := currentIdentity(c)
	if id == nil {
		c.JSON(401, failure("Unauthenticated."))
		return
	}
	if err := h.tokens.RevokeAll(c.Request.Context(), id.User.ID); err != nil {
		if isCanceled(err) {
			c.JSON(200, failure("请求超时或已取消"))
			return
		}
		h.serverError(c)
		return
	}
	c.JSON(200, success("success", nil))
}

// strategies lists the rules of the caller's group, or the guest group's when
// called without a token and guest uploads are enabled.
func (h *Handler) strategies(c *gin.Context) {
	ctx := c.Request.Context()
	if id := currentIdentity(c); id != nil {
		list, err := h.lsky.Strategies(ctx, id.User.GroupID)
		if err != nil {
			h.serverError(c)
			return
		}
		c.JSON(200, success("success", gin.H{"strategies": list}))
		return
	}
	enabled, group, ok, err := h.lsky.GuestUploadState(ctx)
	if err != nil {
		h.serverError(c)
		return
	}
	empty := make([]service.PolicySummary, 0)
	if !enabled || !ok {
		c.JSON(200, success("success", gin.H{"strategies": empty}))
		return
	}
	list, err := h.lsky.Strategies(ctx, group.ID)
	if err != nil {
		h.serverError(c)
		return
	}
	c.JSON(200, success("success", gin.H{"strategies": list}))
}

// listImages serves the paginated v1 image list with its filters. The Lsky
// quirk stands: a missing or zero album_id selects only unassigned images.
func (h *Handler) listImages(c *gin.Context) {
	id := currentIdentity(c)
	if id == nil {
		c.JSON(401, failure("Unauthenticated."))
		return
	}
	page := queryInt(c, "page", 1)
	albumID := queryUint(c, "album_id")
	items, total, err := h.images.ListV1(c.Request.Context(), id.User.ID, page, 40, c.Query("order"), c.Query("permission"), c.Query("keyword"), albumID)
	if err != nil {
		h.serverError(c)
		return
	}
	var album any
	if albumID > 0 {
		if view, findErr := h.albums.FindOwned(c.Request.Context(), id.User.ID, albumID); findErr == nil {
			album = albumRef{ID: view.ID, Name: view.Name}
		}
	}
	now := h.now()
	data := make([]imageItem, 0, len(items))
	for _, item := range items {
		attached := album
		data = append(data, buildImageItem(item.Image, attached, now))
	}
	c.JSON(200, success("success", buildPaginator(baseURL(c.Request.Host, h.forwardedProto(c), c.Request.TLS != nil)+"/api/v1/images", page, 40, total, data)))
}

// deleteImage moves one owned image into the recycle bin.
func (h *Handler) deleteImage(c *gin.Context) {
	id := currentIdentity(c)
	if id == nil {
		c.JSON(401, failure("Unauthenticated."))
		return
	}
	err := h.images.Trash(c.Request.Context(), id.Subject, c.Param("key"))
	if err == nil {
		c.JSON(200, success("success", nil))
		return
	}
	if isCanceled(err) {
		c.JSON(200, failure("请求超时或已取消"))
		return
	}
	message := "删除失败，请稍后再试"
	switch {
	case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrInvalidInput):
		message = "图片不存在"
	case errors.Is(err, service.ErrForbidden):
		message = "没有权限操作该图片"
	case errors.Is(err, service.ErrImageBusy):
		message = "图片操作进行中，请稍后再试"
	case errors.Is(err, service.ErrStorage):
		message = "存储服务异常，请稍后再试"
	}
	c.JSON(200, failure(message))
}

// listAlbums serves the paginated v1 album list ordered by newest, earliest,
// most or least images.
func (h *Handler) listAlbums(c *gin.Context) {
	id := currentIdentity(c)
	if id == nil {
		c.JSON(401, failure("Unauthenticated."))
		return
	}
	page := queryInt(c, "page", 1)
	result, err := h.albums.List(c.Request.Context(), id.User.ID, service.AlbumQuery{Page: page, Size: 40, Order: c.Query("order"), Keyword: c.Query("keyword")})
	if err != nil {
		h.serverError(c)
		return
	}
	data := make([]albumItem, 0, len(result.Items))
	for _, album := range result.Items {
		data = append(data, albumItem{ID: album.ID, Name: album.Name, Intro: album.Intro, ImageNum: album.ImageNum})
	}
	c.JSON(200, success("success", buildPaginator(baseURL(c.Request.Host, h.forwardedProto(c), c.Request.TLS != nil)+"/api/v1/albums", page, 40, result.Total, data)))
}

// deleteAlbum removes one owned album; its images stay and become unassigned.
func (h *Handler) deleteAlbum(c *gin.Context) {
	id := currentIdentity(c)
	if id == nil {
		c.JSON(401, failure("Unauthenticated."))
		return
	}
	albumID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || albumID == 0 {
		c.JSON(200, failure("相册不存在"))
		return
	}
	if err := h.albums.Delete(c.Request.Context(), id.User.ID, albumID); err != nil {
		switch {
		case errors.Is(err, service.ErrNotFound), errors.Is(err, service.ErrForbidden), errors.Is(err, service.ErrInvalidInput):
			c.JSON(200, failure("相册不存在"))
		case isCanceled(err):
			c.JSON(200, failure("请求超时或已取消"))
		default:
			h.serverError(c)
		}
		return
	}
	c.JSON(200, success("删除成功", nil))
}

// profile serves the authenticated account's v1 profile.
func (h *Handler) profile(c *gin.Context) {
	id := currentIdentity(c)
	if id == nil {
		c.JSON(401, failure("Unauthenticated."))
		return
	}
	view, err := h.lsky.Profile(c.Request.Context(), id.User)
	if err != nil {
		h.serverError(c)
		return
	}
	c.JSON(200, success("success", gin.H{
		"name":          view.Name,
		"avatar":        "",
		"email":         view.Email,
		"capacity":      kilobytes(view.Capacity),
		"used_capacity": kilobytes(view.UsedCapacity),
		"url":           "",
		"image_num":     view.ImageNum,
		"album_num":     view.AlbumNum,
		"registered_ip": view.RegisteredIP,
	}))
}

func queryInt(c *gin.Context, name string, fallback int) int {
	value := c.Query(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func queryUint(c *gin.Context, name string) uint64 {
	value := c.Query(name)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
