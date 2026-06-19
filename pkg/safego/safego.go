package safego

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"aranea-agents/pkg/appctx"
)

// PanicHook is called when a goroutine started by Go recovers from a panic.
// Implementations must be safe for concurrent use.
type PanicHook func(name string, r interface{}, stack []byte)

var (
	panicHook PanicHook
	hookMu    sync.RWMutex
)

// RegisterPanicHook sets the global panic hook called after recovering a panic.
// It replaces any previously registered hook. Pass nil to remove the hook.
// This is typically called once during application startup.
func RegisterPanicHook(h PanicHook) {
	hookMu.Lock()
	defer hookMu.Unlock()
	panicHook = h
}

func getPanicHook() PanicHook {
	hookMu.RLock()
	defer hookMu.RUnlock()
	return panicHook
}

func Go(ctx context.Context, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				fmt.Fprintf(os.Stderr, "[flow][system] safego panic recovered where=%s err=%v\n%s\n",
					name, r, stack)
				_ = os.Stderr.Sync()

				if hook := getPanicHook(); hook != nil {
					hook(name, r, stack)
				}
			}
		}()
		fn()
	}()
}

// GoBackground starts a process-level goroutine using appctx.Ctx() as the
// context. This is intended for cross-request background work that should
// only be cancelled when the application shuts down (appctx.Cancel()).
//
// Use GoBackground for goroutines that outlive any single request, such as
// the pending-queue drainer or graph event consumers. For request-scoped
// goroutines (e.g. WebSocket handlers), use Go with the request context.
//
// The panic recovery semantics are identical to Go: panics are recovered,
// logged to stderr, and forwarded to the registered PanicHook (if any).
func GoBackground(name string, fn func()) {
	Go(appctx.Ctx(), name, fn)
}
