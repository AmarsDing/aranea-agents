package agent

import (
	"context"
	"time"
)

const chatPersistTimeout = 90 * time.Second

// ChatPersistCtx returns a context for Ent/SQLite chat persistence after a long provider round-trip.
// Kratos http.Timeout (and similar) attach a deadline to the request context; once the LLM call
// exceeds it, entClient.Tx(requestCtx) fails with "context deadline exceeded" even though the model
// already returned. Detach from cancellation but keep a bounded timeout for the write itself.
func ChatPersistCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), chatPersistTimeout)
}
