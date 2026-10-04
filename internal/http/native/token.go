package native

import (
	"strconv"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *Handler) listTokens(c *gin.Context) {
	items, err := h.tokens.List(c.Request.Context(), identity(c).User.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if items == nil {
		items = []service.TokenView{}
	}
	respond(c, 200, items)
}
func (h *Handler) createToken(c *gin.Context) {
	var in struct {
		Name      string     `json:"name"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if !decode(c, &in) {
		return
	}
	issued, err := h.tokens.Issue(c.Request.Context(), identity(c).Subject, service.TokenInput{Name: in.Name, Kind: "api", ExpiresAt: in.ExpiresAt, Abilities: []string{"*"}})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 201, issued)
}
func (h *Handler) revokeToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		fail(c, service.ErrInvalidInput)
		return
	}
	if err := h.tokens.Revoke(c.Request.Context(), identity(c).User.ID, id); err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, nil)
}
