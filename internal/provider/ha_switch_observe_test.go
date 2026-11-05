package provider

import (
	"context"
	"errors"
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// haObserveModel 切换观测的行为钉板（2026-11-03 trpc 出库，fork
// failover/hedge WithSwitchCallback 的应用层替代）。
//
// 观测语义约定：
//   - failover：候选在未产出有效内容前失败 → 回调（精确语义，与 fork 一致；
//     框架随即尝试下一候选）。
//   - hedge：备份候选 i>0 的流完整收官（Done）→ 回调 hedge_winner。这是
//     近似语义，仅作观测，不作为任何正确性依据（详见
//     TestHAObserve_HedgeLoserDonePinsApproximation）。

// scriptModel 按脚本回放响应的测试模型。
type scriptModel struct {
	name  string
	resps []*trpcmodel.Response
	err   error
}

func (m *scriptModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: m.name} }

func (m *scriptModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan *trpcmodel.Response, len(m.resps)+1)
	for _, r := range m.resps {
		ch <- r
	}
	close(ch)
	return ch, nil
}

// blockModel 阻塞直到 ctx 取消后关闭 channel（模拟被 cancelLosers 提前取消
// 的落败候选）。
type blockModel struct {
	name string
}

func (m *blockModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: m.name} }

func (m *blockModel) GenerateContent(ctx context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	ch := make(chan *trpcmodel.Response, 1)
	ch <- chunkResp("partial")
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func chunkResp(delta string) *trpcmodel.Response {
	return &trpcmodel.Response{
		Object:    trpcmodel.ObjectTypeChatCompletionChunk,
		IsPartial: true,
		Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: delta}}},
	}
}

func doneResp() *trpcmodel.Response {
	return &trpcmodel.Response{
		Object:  trpcmodel.ObjectTypeChatCompletion,
		Done:    true,
		Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: "ok"}}},
	}
}

func errRespStream(msg string) *trpcmodel.Response {
	return &trpcmodel.Response{Error: &trpcmodel.ResponseError{Message: msg}}
}

func drainModel(t *testing.T, ctx context.Context, m trpcmodel.Model) {
	t.Helper()
	ch, err := m.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		return
	}
	for range ch {
	}
}

func TestHAObserve_FailoverExactSwitch(t *testing.T) {
	// 候选 0 未产出有效内容即失败 → 回调 0→1；候选 1（末位）失败 → 无回调。
	var calls []string
	cb := func(from, to int, fromName, toName, reason string) {
		calls = append(calls, fromName+"->"+toName+":"+reason)
	}
	cands := wrapFailoverCandidatesForObserve([]trpcmodel.Model{
		&scriptModel{name: "primary", resps: []*trpcmodel.Response{errRespStream("boom")}},
		&scriptModel{name: "backup", resps: []*trpcmodel.Response{errRespStream("backup-boom")}},
	}, cb)

	drainModel(t, context.Background(), cands[0])
	if len(calls) != 1 || calls[0] != "primary->backup:boom" {
		t.Fatalf("primary failure calls = %v, want [primary->backup:boom]", calls)
	}
	drainModel(t, context.Background(), cands[1])
	if len(calls) != 1 {
		t.Fatalf("last-candidate failure must not fire, calls = %v", calls)
	}
}

func TestHAObserve_FailoverNoSwitchAfterContent(t *testing.T) {
	// 候选已产出有效内容后的中段错误不回调（框架不会再切换，与 fork 一致）。
	var calls int
	cb := func(_, _ int, _, _, _ string) { calls++ }
	cands := wrapFailoverCandidatesForObserve([]trpcmodel.Model{
		&scriptModel{name: "primary", resps: []*trpcmodel.Response{chunkResp("x"), errRespStream("mid")}},
		&scriptModel{name: "backup"},
	}, cb)
	drainModel(t, context.Background(), cands[0])
	if calls != 0 {
		t.Fatalf("post-content error must not fire switch callback, got %d", calls)
	}
}

func TestHAObserve_HedgeWinnerCallback(t *testing.T) {
	// 备份候选 i>0 流完整收官（内容 + Done）→ 回调 0→i hedge_winner；
	// 主候选夺冠不回调（与 fork 一致）。
	var calls []string
	cb := func(from, to int, fromName, toName, reason string) {
		calls = append(calls, fromName+"->"+toName+":"+reason)
	}
	cands := wrapHedgeCandidatesForObserve([]trpcmodel.Model{
		&scriptModel{name: "primary", resps: []*trpcmodel.Response{chunkResp("p"), doneResp()}},
		&scriptModel{name: "backup1", resps: []*trpcmodel.Response{chunkResp("b"), doneResp()}},
	}, cb)

	drainModel(t, context.Background(), cands[1])
	if len(calls) != 1 || calls[0] != "primary->backup1:hedge_winner" {
		t.Fatalf("backup Done calls = %v, want [primary->backup1:hedge_winner]", calls)
	}
	drainModel(t, context.Background(), cands[0])
	if len(calls) != 1 {
		t.Fatalf("primary Done must not fire, calls = %v", calls)
	}
}

// TestHAObserve_HedgeLoserDonePinsApproximation 钉住 hedge 观测的近似语义：
// 观测器只凭「备份候选流完整收官（Done）」判定 hedge_winner，无法区分
// 真赢家与「上游 cancelLosers 生效前恰好走完 Done 的落败者」——落败者
// Done 先到同样会判 hedge_winner。该事件仅作观测（进程日志 / monitor
// 流程事件），不作为任何正确性依据；真正决定内容归属的是上游 hedge 的
// winner 选择逻辑。
func TestHAObserve_HedgeLoserDonePinsApproximation(t *testing.T) {
	var calls int
	cb := func(_, _ int, _, _, _ string) { calls++ }
	cands := wrapHedgeCandidatesForObserve([]trpcmodel.Model{
		&scriptModel{name: "primary", resps: []*trpcmodel.Response{chunkResp("p"), doneResp()}},
		// 「落败者」：流在被取消前恰好完整走完 Done —— 观测器照样判赢。
		&scriptModel{name: "backup1", resps: []*trpcmodel.Response{chunkResp("b"), doneResp()}},
	}, cb)
	drainModel(t, context.Background(), cands[0]) // 主候选先夺冠
	drainModel(t, context.Background(), cands[1]) // 落败者 Done 仍被判 hedge_winner
	if calls != 1 {
		t.Fatalf("loser-done-first must still report hedge_winner (observational approximation), got %d", calls)
	}
}

func TestHAObserve_HedgeCancelledLoserNoCallback(t *testing.T) {
	// 落败候选的正常形态：被 cancelLosers 提前取消，流走不到 Done → 无回调。
	var calls int
	cb := func(_, _ int, _, _, _ string) { calls++ }
	cands := wrapHedgeCandidatesForObserve([]trpcmodel.Model{
		&scriptModel{name: "primary", resps: []*trpcmodel.Response{chunkResp("p"), doneResp()}},
		&blockModel{name: "backup1"},
	}, cb)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		drainModel(t, ctx, cands[1])
		close(done)
	}()
	cancel() // 模拟 cancelLosers
	<-done
	if calls != 0 {
		t.Fatalf("cancelled loser must not fire hedge_winner, got %d", calls)
	}
}

func TestHAObserve_NilCallbackPassthrough(t *testing.T) {
	// cb=nil 时原样返回候选列表（零包装，零行为差异）。
	orig := []trpcmodel.Model{&scriptModel{name: "a"}, &scriptModel{name: "b"}}
	if got := wrapFailoverCandidatesForObserve(orig, nil); len(got) != 2 {
		t.Fatalf("nil cb failover wrap len = %d", len(got))
	}
	if got := wrapHedgeCandidatesForObserve(orig, nil); len(got) != 2 {
		t.Fatalf("nil cb hedge wrap len = %d", len(got))
	}
}

func TestHAObserve_GenerateContentError(t *testing.T) {
	// GenerateContent 直接返回错误（未产出任何内容）→ failover 回调。
	var calls int
	cb := func(_, _ int, _, _, _ string) { calls++ }
	cands := wrapFailoverCandidatesForObserve([]trpcmodel.Model{
		&scriptModel{name: "primary", err: errors.New("dial fail")},
		&scriptModel{name: "backup"},
	}, cb)
	drainModel(t, context.Background(), cands[0])
	if calls != 1 {
		t.Fatalf("GenerateContent error must fire switch callback, got %d", calls)
	}
}
