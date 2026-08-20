// Package appctx provides a global application-lifecycle context.
// Init() is called once at server startup; Cancel() is called on shutdown.
// Background goroutines should derive their context from Ctx() so they
// are cancelled cleanly when the server stops.
package appctx

import (
	"context"
	"sync"
	"time"
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

// DetachTimeout bounds detached finalization writes (10s 与既有
// recordRunnerCompletion 内联模式一致).
const DetachTimeout = 10 * time.Second

// Detach 返回保留 ctx 全部 values（trace、user scope、turnID、预算台账）
// 但剥离取消信号与截止时间的派生 ctx，并附加有界超时兜底。
//
// 用于 turn 收尾落库（指标、run 状态迁移、usage 记录）：客户端断连或用户
// 取消 turn 时这些写入仍须完成，否则计费与慢查询归因出现盲区（2026-08-20
// 实测：HTTP 客户端 300s 超时 → "session turn record failed: context
// canceled"，该轮指标整行丢失）。与 evaluation AfterTurn 的 EVAL-01 内联
// 模式同理，此处收敛为统一入口。
func Detach(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), DetachTimeout)
}
