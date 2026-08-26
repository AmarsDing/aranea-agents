package usage

import (
	"context"
	"strings"
)

// run_peak.go — 79-runtime-governance R7（G-2）：run 级单轮 input tokens 峰值。
// 513 万事故的关键度量此前只能从事故窗口日志估算；成员 step 记账点
// （model_token_usage_events genuine team_member 行，每行=一次模型调用）
// 已持久化 per-call input_tokens，读侧补 MAX 聚合即可回测「峰值下降」类回归。

// RunTurnPeak 是一个 run 的单次模型调用 input tokens 峰值。
type RunTurnPeak struct {
	// Found 报告该 run 是否存在 genuine 成员用量行。False ⇒ 调用方按
	// 「无数据」处理，而非峰值 0。
	Found bool
	// MaxInputTokens 是 run 内单次调用最大 input tokens（genuine 成员行
	// MAX；镜像/对账行排除——attribution 非空行与 team_turn 行均非单次
	// 调用口径，见 RunCacheHitRatio 同语义注释）。
	MaxInputTokens int64
}

// RunTurnPeakRepo 从用量事件面读取 run 级单轮峰值。
//
// Stability:evolving
type RunTurnPeakRepo interface {
	// RunTurnPeak 聚合一个 run 的 genuine team_member 行 MAX(input_tokens)。
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
