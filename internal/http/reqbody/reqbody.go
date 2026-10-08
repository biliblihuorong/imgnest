// Package reqbody bounds request bodies shared by the native and Lsky handlers.
package reqbody

import (
	"context"
	"net/http"
	"time"
)

// Bound caps the request body at limit bytes and ties reading it to ctx: when
// ctx ends the underlying body is closed and the connection read deadline is
// set to ctx's deadline, so a slow client cannot hold the handler past its
// timeout. The returned release must be called before the handler returns.
func Bound(ctx context.Context, w http.ResponseWriter, r *http.Request, limit int64) (release func()) {
	original := r.Body
	stop := context.AfterFunc(ctx, func() { _ = original.Close() })
	controller := http.NewResponseController(w)
	reset := false
	if deadline, ok := ctx.Deadline(); ok {
		reset = controller.SetReadDeadline(deadline) == nil
	}
	r.Body = http.MaxBytesReader(w, original, limit)
	return func() {
		stop()
		if reset {
			_ = controller.SetReadDeadline(time.Time{})
		}
	}
}
