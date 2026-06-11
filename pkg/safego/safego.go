package safego

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
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
