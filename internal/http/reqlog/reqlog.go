// Package reqlog lets handlers add what the request log and the audit trail
// record about a request: the signed-in account and the cause of a failure.
package reqlog

import (
	"github.com/biliblihuorong/imgnest/internal/logx"
	"github.com/gin-gonic/gin"
)

const userKey = "reqlog_user"

// User is the account a request acted as.
type User struct {
	ID   uint64
	Name string
}

// SetUser records the account a request is authenticated as.
func SetUser(c *gin.Context, id uint64, name string) {
	if id != 0 {
		c.Set(userKey, User{ID: id, Name: name})
	}
}

// CurrentUser returns the account recorded by SetUser.
func CurrentUser(c *gin.Context) (User, bool) {
	value, ok := c.Get(userKey)
	if !ok {
		return User{}, false
	}
	user, ok := value.(User)
	return user, ok
}

// Stack is the gin error metadata holding where a failure was handled.
type Stack string

// Fail records err as the cause of a failed request together with the stack
// of the handler that met it, so the request log shows both. Handlers call it
// for failures answered as internal, storage or processing errors.
func Fail(c *gin.Context, err error) {
	FailWithStack(c, err, logx.Stack())
}

// FailWithStack records err with a stack taken elsewhere, such as in the
// goroutine that recovered a panic.
func FailWithStack(c *gin.Context, err error, stack string) {
	if err == nil {
		return
	}
	_ = c.Error(err).SetMeta(Stack(stack))
}
