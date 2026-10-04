package native

import (
	"github.com/gin-gonic/gin"
)

// listPolicies serves the enabled group-bound rules of the authenticated
// caller for the upload page; an empty rule set is serialized as [].
func (h *imageHandler) listPolicies(c *gin.Context) {
	rules, err := h.images.ListPolicies(c.Request.Context(), identity(c).Subject)
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, rules)
}
