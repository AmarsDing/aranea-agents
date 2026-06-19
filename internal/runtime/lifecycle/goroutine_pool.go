package lifecycle

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"

	"aranea-agents/pkg/appctx"
)

// GoroutinePool 提供多模式 goroutine 启动，强制 ctx 传播与 panic recovery。
//
// 设计目标（A2）：替代裸 safego.Go 调用，区分"请求级"与"进程级"语义：
//   - Go(ctx, name, fn)：请求级，ctx 来自请求（如 WS connCtx）。ctx 取消时
//     fn 应通过 select ctx.Done() 退出。适用于 WS handleUserMessage 等场景。
//   - GoBackground(name, fn)：进程级，内部用 appctx.Ctx()，跨请求存活，
//     仅在应用 shutdown 时取消。适用于 processPendingQueue、graph.consumeEvents
//     等跨请求场景。
//
// 与 safego.Go 的关系：GoroutinePool 复用 safego 的 panic recovery 语义，
// 但增加了多模式 ctx 选择与可选的 PanicHook 注入。
//
// 并发安全：所有方法均可并发调用。
type GoroutinePool struct {
	panicHook PanicHook
	hookMu    sync.RWMutex
}

// PanicHook 在 goroutine recover panic 后调用。
type PanicHook func(name string, r interface{}, stack []byte)

// NewGoroutinePool 创建一个 GoroutinePool。
func NewGoroutinePool() *GoroutinePool {
	return &GoroutinePool{}
}

// SetPanicHook 设置 panic hook。线程安全。
func (p *GoroutinePool) SetPanicHook(h PanicHook) {
	p.hookMu.Lock()
	defer p.hookMu.Unlock()
	p.panicHook = h
}

func (p *GoroutinePool) getPanicHook() PanicHook {
	p.hookMu.RLock()
	defer p.hookMu.RUnlock()
	return p.panicHook
}

// Go 启动一个请求级 goroutine。ctx 应来自请求（如 WS connCtx、HTTP ctx）。
//
// fn 内部应通过 select ctx.Done() 监听取消信号以避免泄漏。
// panic 会被 recover，不会导致进程崩溃。
func (p *GoroutinePool) Go(ctx context.Context, name string, fn func(ctx context.Context)) {
	if ctx == nil {
		ctx = context.Background()
	}
	go p.runWithRecovery(ctx, name, fn)
}

// GoBackground 启动一个进程级 goroutine，使用 appctx.Ctx() 作为 ctx。
//
// 适用于跨请求场景（processPendingQueue、graph.consumeEvents），
// 仅在应用 shutdown（appctx.Cancel()）时取消。
// fn 内部应通过 select ctx.Done() 监听取消信号以避免泄漏。
func (p *GoroutinePool) GoBackground(name string, fn func(ctx context.Context)) {
	ctx := appctx.Ctx()
	go p.runWithRecovery(ctx, name, fn)
}

// runWithRecovery 执行 fn 并 recover panic。
func (p *GoroutinePool) runWithRecovery(ctx context.Context, name string, fn func(ctx context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			fmt.Fprintf(os.Stderr, "[lifecycle] goroutine panic recovered name=%s err=%v\n%s\n",
				name, r, stack)
			_ = os.Stderr.Sync()
			if hook := p.getPanicHook(); hook != nil {
				hook(name, r, stack)
			}
		}
	}()
	fn(ctx)
}
