package native

import (
	"context"
	"math"
	"net/url"
	"strconv"

	"github.com/biliblihuorong/imgnest/internal/searchquery"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

func failSearch(c *gin.Context, err error) {
	if problem, ok := service.IsSearchError(err); ok {
		c.JSON(problem.Status(), Response{Code: 10001, Message: "invalid search query", Data: problem})
		return
	}
	fail(c, err)
}
func parameterError(code string) error {
	return service.SearchDiagnostic(code, searchquery.Span{}, nil)
}
func strictPositive(value string, max uint64) (uint64, bool) {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	v, e := strconv.ParseUint(value, 10, 64)
	return v, e == nil && v <= max
}
func queryPagination(values url.Values, maxSize uint64) (int, int, error) {
	page, size := uint64(1), uint64(20)
	for name, target := range map[string]*uint64{"page": &page, "size": &size} {
		if vals, ok := values[name]; ok {
			if len(vals) != 1 {
				return 0, 0, parameterError("INVALID_PARAMETER")
			}
			max := uint64(10000)
			if name == "size" {
				max = maxSize
			}
			v, ok := strictPositive(vals[0], max)
			if !ok {
				return 0, 0, parameterError("INVALID_PAGINATION")
			}
			*target = v
		}
	}
	return int(page), int(size), nil
}
func searchParams(values url.Values) (service.ImageQuery, error) {
	if len(values["qv"]) != 1 || values.Get("qv") != "1" {
		return service.ImageQuery{}, parameterError("UNSUPPORTED_QUERY_VERSION")
	}
	for key, vals := range values {
		switch key {
		case "qv", "q", "tz", "page", "size":
		default:
			return service.ImageQuery{}, parameterError("MIXED_QUERY_PROTOCOL")
		}
		if len(vals) != 1 {
			return service.ImageQuery{}, parameterError("INVALID_PARAMETER")
		}
	}
	if _, exists := values["q"]; !exists {
		return service.ImageQuery{}, parameterError("MISSING_VALUE")
	}
	if _, exists := values["tz"]; !exists || values.Get("tz") == "" {
		return service.ImageQuery{}, parameterError("INVALID_TIMEZONE")
	}
	page, size, err := queryPagination(values, 100)
	if err != nil {
		return service.ImageQuery{}, err
	}
	if size != 20 && size != 50 && size != 100 {
		return service.ImageQuery{}, parameterError("INVALID_PAGINATION")
	}
	return service.ImageQuery{QueryVersion: 1, Q: values.Get("q"), Timezone: values.Get("tz"), Page: page, Size: size}, nil
}
func (h *imageHandler) listAlbumSearch(c *gin.Context) {
	id, ok := strictPositive(c.Param("id"), math.MaxInt64)
	if !ok {
		failSearch(c, parameterError("INVALID_PARAMETER"))
		return
	}
	h.listSearch(c, &id)
}
func (h *imageHandler) listSearch(c *gin.Context, lockedAlbum *uint64) {
	query, err := searchParams(c.Request.URL.Query())
	if err != nil {
		failSearch(c, err)
		return
	}
	query.LockedAlbumID = lockedAlbum
	select {
	case h.searchSlots <- struct{}{}:
		defer func() { <-h.searchSlots }()
	default:
		c.JSON(429, Response{Code: 30003, Message: "too many search requests", Data: nil})
		return
	}
	result, err := h.images.List(c.Request.Context(), identity(c).Subject, query)
	if err != nil {
		failSearch(c, err)
		return
	}
	if result.Items == nil {
		result.Items = []service.ImageView{}
	}
	respond(c, 200, result)
}

type albumSuggestions interface {
	Suggestions(context.Context, uint64, string, int, int, *uint64) (service.AlbumSuggestions, error)
}

func (h *albumHandler) suggestions(c *gin.Context) {
	values := c.Request.URL.Query()
	for key, v := range values {
		if len(v) != 1 {
			failSearch(c, parameterError("INVALID_PARAMETER"))
			return
		}
		switch key {
		case "keyword", "page", "size", "scope_album_id":
		default:
			failSearch(c, parameterError("INVALID_PARAMETER"))
			return
		}
	}
	page, size, err := queryPagination(values, 20)
	if err != nil {
		failSearch(c, err)
		return
	}
	var scope *uint64
	if v, exists := values["scope_album_id"]; exists {
		n, ok := strictPositive(v[0], math.MaxInt64)
		if !ok {
			failSearch(c, parameterError("INVALID_PARAMETER"))
			return
		}
		scope = &n
	}
	serviceWithSuggestions, ok := h.albums.(albumSuggestions)
	if !ok {
		fail(c, service.ErrStorage)
		return
	}
	result, err := serviceWithSuggestions.Suggestions(c.Request.Context(), identity(c).User.ID, values.Get("keyword"), page, size, scope)
	if err != nil {
		failSearch(c, err)
		return
	}
	respond(c, 200, result)
}
