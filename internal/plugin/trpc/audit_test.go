package plugintrpc

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// countingLogger counts Info calls (the only level onEvent uses).
type countingLogger struct {
	mu   sync.Mutex
	info int
}

func (l *countingLogger) Debug(string, ...loggateway.Field) {}
func (l *countingLogger) Info(string, ...loggateway.Field) {
	l.mu.Lock()
	l.info++
	l.mu.Unlock()
}
func (l *countingLogger) Warn(string, ...loggateway.Field)  {}
func (l *countingLogger) Error(string, ...loggateway.Field) {}
func (l *countingLogger) With(...loggateway.Field) loggateway.Logger {
	return l
}

func (l *countingLogger) infoCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.info
}

// 00:52 会话取证：单 turn 产生 13501 条 audit on_event 日志（6725 条
// chat.completion.chunk + 6739 条空 object 事件），每条触发一次 pipeline
// 写入 + 一个 MonitorEvent 发布 goroutine，造成日志洪泛。高频流式事件必须
// 按种类节流：首条 + 每 200 条采样一次；低频事件保持逐条审计。
func TestAuditLogPlugin_OnEventThrottlesHighFrequencyKinds(t *testing.T) {
	cl := &countingLogger{}
	p := NewAuditLogPlugin(biz.Plugin{Key: "audit_log"}, &noopStatsRecorder{}, nil, cl)

	chunk := &trpcevent.Event{Response: &trpcmodel.Response{Object: trpcmodel.ObjectTypeChatCompletionChunk}, Author: "assistant"}
	for i := 0; i < 250; i++ {
		if _, err := p.onEvent(context.Background(), nil, chunk); err != nil {
			t.Fatalf("onEvent returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 2 {
		t.Fatalf("chunk events must be sampled (1st + every 200th): expected 2 logs for 250 events, got %d", got)
	}

	// 空 object 事件同样高频，按相同规则节流。
	empty := &trpcevent.Event{Response: &trpcmodel.Response{Object: ""}, Author: "assistant"}
	for i := 0; i < 250; i++ {
		if _, err := p.onEvent(context.Background(), nil, empty); err != nil {
			t.Fatalf("onEvent returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 4 {
		t.Fatalf("empty-object events must be sampled too: expected 4 total logs, got %d", got)
	}

	// 低频事件（完整响应等）保持逐条审计，不得被节流。
	full := &trpcevent.Event{Response: &trpcmodel.Response{Object: trpcmodel.ObjectTypeChatCompletion}, Author: "assistant"}
	for i := 0; i < 3; i++ {
		if _, err := p.onEvent(context.Background(), nil, full); err != nil {
			t.Fatalf("onEvent returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 7 {
		t.Fatalf("low-frequency events must be logged every time: expected 7 total logs, got %d", got)
	}
}

// 00:52 会话补充取证：框架对流式响应的每个 chunk 触发一次 afterModel，
// audit 每 chunk 写一条 Info（实测 8287 条/4min），与 on_event 同构的高频
// 洪泛。chunk 响应必须采样；错误响应与完整响应保持逐条。
func TestAuditLogPlugin_AfterModelThrottlesChunks(t *testing.T) {
	cl := &countingLogger{}
	p := NewAuditLogPlugin(biz.Plugin{Key: "audit_log"}, &noopStatsRecorder{}, nil, cl)

	chunkArgs := &trpcmodel.AfterModelArgs{Response: &trpcmodel.Response{Object: trpcmodel.ObjectTypeChatCompletionChunk}}
	for i := 0; i < 250; i++ {
		if _, err := p.afterModel(context.Background(), chunkArgs); err != nil {
			t.Fatalf("afterModel returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 2 {
		t.Fatalf("chunk after_model must be sampled (1st + every 200th): expected 2 logs for 250 calls, got %d", got)
	}

	// 错误响应永远逐条（错误路径是审计的高价值信号）。
	errArgs := &trpcmodel.AfterModelArgs{
		Response: &trpcmodel.Response{Object: trpcmodel.ObjectTypeChatCompletionChunk},
		Error:    &trpcmodel.ResponseError{Message: "boom"},
	}
	for i := 0; i < 3; i++ {
		if _, err := p.afterModel(context.Background(), errArgs); err != nil {
			t.Fatalf("afterModel returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 5 {
		t.Fatalf("error responses must be logged every time: expected 5 total logs, got %d", got)
	}

	// 非 chunk 的完整响应保持逐条审计。
	fullArgs := &trpcmodel.AfterModelArgs{Response: &trpcmodel.Response{Object: trpcmodel.ObjectTypeChatCompletion}}
	for i := 0; i < 3; i++ {
		if _, err := p.afterModel(context.Background(), fullArgs); err != nil {
			t.Fatalf("afterModel returned error: %v", err)
		}
	}
	if got := cl.infoCount(); got != 8 {
		t.Fatalf("full responses must be logged every time: expected 8 total logs, got %d", got)
	}
}
