package main

import (
	"context"
	"testing"

	"aranea-agents/internal/cronrunner/jobs"
)

// TestBackgroundWorkersConfigFromOutputToolSpillGCTypedNil 钉 D2-f-7（对抗评审
// 确认）：wireOut.ToolSpillGC 为具体指针 nil（杀闸 TOOLSPILL_GC_DISABLED 路径
// provider 返回 nil）时，FromOutput 装瓶成的 cfg.ToolSpillGC 接口必须保持
// nil——若退化为 typed-nil 接口装瓶，启动块 `!= nil` 判真、goroutine 照起、
// 日志谎报 "scheduled"，与「杀闸=零注册」契约（93 三件套 FR-2）相悖。
func TestBackgroundWorkersConfigFromOutputToolSpillGCTypedNil(t *testing.T) {
	cfg := backgroundWorkersConfigFromOutput(context.Background(), &wireOut{})
	if cfg.ToolSpillGC != nil {
		t.Fatalf("期望杀闸路径 ToolSpillGC 接口为 nil，谷得 typed-nil 装箱（%T）", cfg.ToolSpillGC)
	}
}

// TestBackgroundWorkersConfigFromOutputToolSpillGCCarried 反向钉：provider
// 产出非 nil 实例时 FromOutput 正常携带同一实例。
func TestBackgroundWorkersConfigFromOutputToolSpillGCCarried(t *testing.T) {
	w := &jobs.ToolSpillGC{}
	cfg := backgroundWorkersConfigFromOutput(context.Background(), &wireOut{ToolSpillGC: w})
	if cfg.ToolSpillGC == nil {
		t.Fatal("期望非 nil ToolSpillGC 被携带，谷得 nil")
	}
	got, ok := cfg.ToolSpillGC.(*jobs.ToolSpillGC)
	if !ok || got != w {
		t.Fatalf("期望携带同一具体实例，谷得 %T %v", cfg.ToolSpillGC, cfg.ToolSpillGC)
	}
}
