package safego

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
)

func Go(ctx context.Context, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "[flow][system] safego panic recovered where=%s err=%v\n%s\n",
					name, r, debug.Stack())
				_ = os.Stderr.Sync()
			}
		}()
		fn()
	}()
}
