package usage

import (
	"context"
	"strings"
)

// run_peak.go — 79-runtime-governance R7（G-2）：run 级单轮 input tokens 峰值。
// 513 万事故的关键度量此前只能从事故窗口日志估算；成员 step 记账点
// （model_token_usage_events team_member 行）已持久化 input_tokens，
// 读侧补 MAX 聚合即可回测「峰值下降」类回归。

// RunTurnPeak 是一个 run 的成员级 input tokens 峰值。
type RunTurnPeak struct {
	// Found 报告该 run 是否存在成员用量行。False ⇒ 调用方按「无数据」
	// 处理，而非峰值 0。
	Found bool
	// MaxInputTokens 是 run 内成员行 MAX(input_tokens)。口径（2026-08-27
	// 最终裁定）：**纳入** attribution 标记行——graph runtime 下带 token 的
	// member 行全带 member_level_stream/stream_anchor_remainder/
	// run_level_anchor_fallback 标记（每行=成员 run 总量或 anchor 兜底
	// 总量，互斥无双计），过滤会使峰值恒 0；team_turn 总账行按
	// usage_kind 排除。峰值语义即「单成员 run 总量峰值」（513 万事故中
	// 单成员单轮即总量，口径等价）。注意与 RunCacheHitRatio 回退分支的
	// 旧口径（attribution='' 过滤）不同——该分支仅服务无 team_turn 行的
	// 失败/熔断 run，维持 P2-1 语义不翻案。
	MaxInputTokens int64
}

// RunTurnPeakRepo 从用量事件面读取 run 级单轮峰值。
//
// Stability:evolving
type RunTurnPeakRepo interface {
	// RunTurnPeak 聚合一个 run 的 team_member 行 MAX(input_tokens)
	// （含 attribution 标记行，口径见 RunTurnPeak.MaxInputTokens）。
	// runID 为空或无命中返回零值, nil。
	RunTurnPeak(ctx context.Context, runID string) (RunTurnPeak, error)
}

// RunTurnPeak 服务 R7 stats API 读路径（G-2）。窄能力 type-assertion 解析，
// repo 无该能力时返回零值（Found=false）。
func (u *Usecase) RunTurnPeak(ctx context.Context, runID string) (RunTurnPeak, error) {
	if u == nil || strings.TrimSpace(runID) == "" {
		return RunTurnPeak{}, nil
	}
	repo, ok := u.repo.(RunTurnPeakRepo)
	if !ok {
		return RunTurnPeak{}, nil
	}
	return repo.RunTurnPeak(ctx, runID)
}
