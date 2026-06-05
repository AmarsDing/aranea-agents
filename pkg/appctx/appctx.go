// Package appctx provides a global application-lifecycle context.
// Init() is called once at server startup; Cancel() is called on shutdown.
// Background goroutines should derive their context from Ctx() so they
// are cancelled cleanly when the server stops.
package appctx

import (
	"context"
	"sync"
)

var (
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
)

// Init creates the application-lifecycle context. Must be called once at startup.
func Init() {
	mu.Lock()
	defer mu.Unlock()
	if ctx != nil {
		return
	}
	ctx, cancel = context.WithCancel(context.Background())
}

// Ctx returns the application-lifecycle context. If Init has not been called,
// it returns context.Background() as a safe fallback.
func Ctx() context.Context {
	mu.Lock()
	defer mu.Unlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// Cancel cancels the application-lifecycle context, signalling all derived
// goroutines to stop. Must be called once on shutdown.
func Cancel() {
	mu.Lock()
	defer mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
