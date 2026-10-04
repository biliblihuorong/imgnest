package native

import (
	"github.com/gin-gonic/gin"
)

// site serves the public site descriptor and never exposes other settings.
func (h *Handler) site(c *gin.Context) {
	view, err := h.users.Site(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, view)
}
