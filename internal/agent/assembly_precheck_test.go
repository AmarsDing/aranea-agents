package agent

// D4-b seat②（assembly.compress.precheck ratio 镜像对照）行为级钉板
// （D4-f 评审 wf_0699f3d1 #10 确认项——双坐席零行为级钉回本批证据通道）：
// off 全静默 / observe·on 恰一步带完整字段面 / verdict ratio 镜像三带判级
// / on 在 seat② 同样只观测不触发（触发义务恒为 seat①）。

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/llmcontext"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// captureFlowBus 捕获 flowlog MonitorEvent（codeexecutor fallback 测试同款
// 包级镜像——event.captureMonitorBus 未导出）。
type captureFlowBus struct {
	mu  sync.Mutex
	evs []contract.MonitorEvent
}

func (b *captureFlowBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evs = append(b.evs, ev)
}

func (b *captureFlowBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return nil, func() {}
}

func (b *captureFlowBus) DropCount() uint64 { return 0 }

func (b *captureFlowBus) flowMetadata(stepID string) []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for _, ev := range b.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			continue
		}
		if step, _ := ev.Metadata["step_id"].(string); step == stepID {
			out = append(out, ev.Metadata)
		}
	}
	return out
}

func newPrecheckBusCtx(runID string, bus *captureFlowBus) context.Context {
	em := event.NewTraceEmitter(&event.Infra{MonitorEventBus: bus}, event.TraceContext{
		TraceID: "trace-" + runID,
		RunID:   runID,
		Domain:  event.TraceDomainChat,
	}, nil)
	return event.WithTraceEmitter(context.Background(), em)
}

// TestAssemblyPrecheckSeat_OffSilent 钉 off 档全静默：装配闸照常跑、预检
// 对照步零发射（缺省已有 emitter 的 chat 路径同样静默，翻牌前零行为变化）。
func TestAssemblyPrecheckSeat_OffSilent(t *testing.T) {
	SetDurablePrecheckModeProvider(func() string { return "off" })
	t.Cleanup(func() { SetDurablePrecheckModeProvider(nil) })

	ag := biz.Agent{AgentKey: "a1"}
	cb := newAssemblyBudgetBeforeHook(ag, TRPCBuilderDeps{})
	if cb == nil {
		t.Fatal("hook = nil for named agent, want budget gate enabled")
	}
	bm := cb.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})

	bus := &captureFlowBus{}
	ctx := llmcontext.ContextWithWindow(newPrecheckBusCtx("r-off", bus), 1000)
	_, err := bm.HandleBeforeModel(ctx, &trpcmodel.BeforeModelArgs{
		Request: &trpcmodel.Request{Messages: []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "短输入"}}},
	})
	if err != nil {
		t.Fatalf("HandleBeforeModel err = %v", err)
	}
	if got := bus.flowMetadata("assembly.compress.precheck"); len(got) != 0 {
		t.Fatalf("off mode must emit zero assembly.compress.precheck steps, got %d: %+v", len(got), got)
	}
}

// TestAssemblyPrecheckSeat_ObserveEmitsOneStep 钉 observe 档恰一条
// assembly.compress.precheck 步，字段面完整（est/window/ratio 三阈/verdict/
// mode/run_id join 键），低水位判 below_soft；on 档同形（seat② 永不触发）。
func TestAssemblyPrecheckSeat_ObserveEmitsOneStep(t *testing.T) {
	t.Cleanup(func() { SetDurablePrecheckModeProvider(nil) })

	for _, mode := range []string{"observe", "on"} {
		t.Run(mode, func(t *testing.T) {
			SetDurablePrecheckModeProvider(func() string { return mode })

			ag := biz.Agent{AgentKey: "a1"}
			cb := newAssemblyBudgetBeforeHook(ag, TRPCBuilderDeps{})
			if cb == nil {
				t.Fatal("hook = nil for named agent")
			}
			bm := cb.(interface {
				HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
			})

			bus := &captureFlowBus{}
			ctx := llmcontext.ContextWithWindow(newPrecheckBusCtx("r-"+mode, bus), 1000)
			_, err := bm.HandleBeforeModel(ctx, &trpcmodel.BeforeModelArgs{
				Request: &trpcmodel.Request{Messages: []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: "短输入"}}},
			})
			if err != nil {
				t.Fatalf("HandleBeforeModel err = %v", err)
			}

			entries := bus.flowMetadata("assembly.compress.precheck")
			if len(entries) != 1 {
				t.Fatalf("mode=%s must emit exactly one precheck step, got %d", mode, len(entries))
			}
			md := entries[0]
			for _, k := range []string{"est_tokens", "context_window", "used_ratio", "soft_ratio", "hard_ratio", "verdict", "mode", "run_id"} {
				if _, ok := md[k]; !ok {
					t.Fatalf("mode=%s missing field %q in %+v", mode, k, md)
				}
			}
			if md["verdict"] != "below_soft" {
				t.Fatalf("mode=%s low est → verdict=%v, want below_soft", mode, md["verdict"])
			}
			if md["mode"] != mode {
				t.Fatalf("mode=%s metadata mode=%v", mode, md["mode"])
			}
			if md["run_id"] != "r-"+mode {
				t.Fatalf("mode=%s run_id join key = %v, want r-%s", mode, md["run_id"], mode)
			}
			// seat② 只观测不触发：流程步 phase 恒为 done（无 error/trigger 语义）。
			if fmt.Sprint(md["flow_phase"]) != string(event.FlowPhaseDone) {
				t.Fatalf("mode=%s flow_phase=%v, want done (observe-only)", mode, md["flow_phase"])
			}
		})
	}
}

// TestEmitAssemblyCompressPrecheck_VerdictBands 钉 ratio 镜像三带判级：
// below_soft / soft_band / hard_band 边界与 hard 优先顺序，刻意略过
// reserved/buffer 项（authoritative 恒 seat①——记档 87 design）。
func TestEmitAssemblyCompressPrecheck_VerdictBands(t *testing.T) {
	ag := biz.Agent{} // 默认 ratio：soft 0.70 / hard 0.90
	const win = 1000
	cases := []struct {
		name string
		est  int
		want string
	}{
		{"below", int(0.69 * win), "below_soft"},
		{"soft_edge", int(0.70 * win), "soft_band"},
		{"mid", int(0.80 * win), "soft_band"},
		{"hard_edge", int(0.90 * win), "hard_band"},
		{"above_hard", int(0.95 * win), "hard_band"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bus := &captureFlowBus{}
			ctx := llmcontext.ContextWithWindow(newPrecheckBusCtx("r-band", bus), win)
			emitAssemblyCompressPrecheck(ctx, "a1", ag, TRPCBuilderDeps{}, c.est, "observe")
			entries := bus.flowMetadata("assembly.compress.precheck")
			if len(entries) != 1 {
				t.Fatalf("est=%d: want 1 step, got %d", c.est, len(entries))
			}
			if got := entries[0]["verdict"]; got != c.want {
				t.Fatalf("est=%d (win=%d, ratio=%.3f): verdict=%v, want %s",
					c.est, win, float64(c.est)/win, got, c.want)
			}
			if got := entries[0]["context_window"]; got != win {
				t.Fatalf("context_window=%v, want %d", got, win)
			}
		})
	}

	// 无 emitter 的 ctx（后台/测试路径）：静默跳过、不 panic。
	emitAssemblyCompressPrecheck(context.Background(), "a1", ag, TRPCBuilderDeps{}, 100, "observe")

	// 设置覆盖档 Settings 镜像到软阈 ratio。
	overridden := biz.Agent{Settings: &biz.AgentRuntimeSettings{SoftTriggerRatio: 0.30}}
	bus := &captureFlowBus{}
	ctx := llmcontext.ContextWithWindow(newPrecheckBusCtx("r-override", bus), win)
	emitAssemblyCompressPrecheck(ctx, "a1", overridden, TRPCBuilderDeps{}, int(0.50*win), "on")
	entries := bus.flowMetadata("assembly.compress.precheck")
	if len(entries) != 1 || entries[0]["verdict"] != "soft_band" {
		t.Fatalf("overridden soft ratio=0.30: est=0.50win expect soft_band, got %+v", entries)
	}
	if got := entries[0]["soft_ratio"]; fmt.Sprintf("%.2f", got) != "0.30" {
		t.Fatalf("soft_ratio field = %v, want 0.30", got)
	}
}
