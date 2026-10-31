package http

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

// clientRequestID is the shape of an X-Request-ID kept from a proxy or
// client; anything else is replaced so the log never echoes arbitrary input.
var clientRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,64}$`)

// requestID keeps a well-formed incoming ID, so one request can be followed
// through a reverse proxy's log and this one, and otherwise makes a new one.
func requestID(incoming string) string {
	if clientRequestID.MatchString(incoming) {
		return incoming
	}
	var buf [12]byte
	_, _ = rand.Read(buf[:])
	return hex.EncodeToString(buf[:])
}

// unaudited are state-changing methods on routes that change nothing: lookups
// sent as POST and dry runs.
var unaudited = []string{
	"POST /api/variants",
	"POST /api/admin/policies/preview",
}

// audited reports whether a request belongs in the operation audit trail:
// every POST, PUT, PATCH and DELETE that reached an API route.
func audited(method, route string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return false
	}
	if !strings.HasPrefix(route, "/api/") {
		return false
	}
	return !slices.Contains(unaudited, method+" "+route)
}

// auditParams are the route parameters an audit entry keeps. Others (such as
// a random link's token) can be secrets and are left out.
var auditParams = []string{"id", "name", "key"}

// auditTarget renders the kept route parameters as "id=12".
func auditTarget(params gin.Params) string {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		if slices.Contains(auditParams, param.Key) {
			parts = append(parts, param.Key+"="+param.Value)
		}
	}
	return strings.Join(parts, " ")
}
