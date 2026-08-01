package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── Meta Team LLM stages (73-self-iteration-v3, design D5, W6) ──────────────
//
// LLM-backed Analyst / Patcher stages, mirroring SICriticAgent (T3.3):
// prompt contract from biz.SI*SystemPrompt, structured output parsed by
// biz.Parse*JSON. provider/model come from SystemSetting.DefaultRefineLLM
// (wire provider, same pattern as provideSkillAutoCreator).
//
// 工具降级说明：design D5 中 Analyst/Patcher 配源码只读/worktree 读写工具，
// 本实现为单轮调用适配器——Patcher 将 Diagnosis.AffectedFiles 的 worktree
// 文件内容内联进 user prompt（路径限定 worktree 内，总量 48KB 封顶），
// 不跑工具回路；工具化升级属后续迭代。
//
// 成本控制（D10）：Patcher 日配额 20 次（24h 窗口，与 Critic 同模式）；
// 配额耗尽返回 ErrSIPatcherQuotaExceeded，流水线将 run 置 failed（保守制动，
// Outcome 记 neutral 反哺触发器降频）。

// ErrSIPatcherQuotaExceeded is returned when the Patcher daily LLM quota is
// exhausted (design D10: 20 次/日).
var ErrSIPatcherQuotaExceeded = errors.New("si patcher daily quota exceeded")

const (
	// siAnalystEvidenceBudget bounds the suggestion evidence JSON inlined into
	// the Analyst user prompt.
	siAnalystEvidenceBudget = 8 * 1024
	// siPatcherFileBudget bounds the total worktree file bytes inlined into
	// the Patcher user prompt.
	siPatcherFileBudget = 48 * 1024
	// siPatcherSingleFileBudget bounds one inlined file.
	siPatcherSingleFileBudget = 16 * 1024
)

// SIAnalystAgent is the LLM-backed Analyst stage of the self-improvement
// Meta Team. It implements biz.SIAnalystStage.
type SIAnalystAgent struct {
	caller   biz.LLMCaller
	provider string
	model    string
	lg       loggateway.Logger
}

// NewSIAnalystAgent wires the Analyst stage.
func NewSIAnalystAgent(caller biz.LLMCaller, provider, model string, lg loggateway.Logger) *SIAnalystAgent {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SIAnalystAgent{
		caller: caller, provider: provider, model: model,
		lg: lg.With(loggateway.Domain("si_analyst")),
	}
}

// Analyze runs one Analyst diagnosis of the run's originating suggestion.
func (a *SIAnalystAgent) Analyze(ctx context.Context, run *biz.SelfImprovementRun, sug *biz.UnifiedEvolutionSuggestion) (*biz.Diagnosis, error) {
	if a == nil || a.caller == nil {
		return nil, apierror.Internal("SELF_IMPROVEMENT", "analyst agent not initialized")
	}
	text, _, err := a.caller.Call(ctx, biz.LLMCallRequest{
		Provider: a.provider,
		Model:    a.model,
		System:   biz.SIAnalystSystemPrompt,
		User:     siAnalystUserMessage(run, sug),
	})
	if err != nil {
		return nil, fmt.Errorf("analyst llm: %w", err)
	}
	return biz.ParseDiagnosisJSON(text)
}

// siAnalystUserMessage packs suggestion + evidence snapshot into the prompt.
func siAnalystUserMessage(run *biz.SelfImprovementRun, sug *biz.UnifiedEvolutionSuggestion) string {
	var b strings.Builder
	if run != nil {
		fmt.Fprintf(&b, "run_id: %s\ntrigger_source: %s\nbase_ref: %s\n", run.ID, run.TriggerSource, run.BaseRef)
	}
	if sug != nil {
		fmt.Fprintf(&b, "建议标题: %s\n触发原因: %s\n", sug.DraftName, sug.TriggerReason)
		if body := strings.TrimSpace(sug.DraftBody); body != "" {
			if len(body) > siAnalystEvidenceBudget {
				body = body[:siAnalystEvidenceBudget] + "\n…[truncated]"
			}
			fmt.Fprintf(&b, "建议内容:\n%s\n", body)
		}
		if meta := strings.TrimSpace(string(sug.Metadata)); meta != "" && meta != "null" {
			if len(meta) > siAnalystEvidenceBudget {
				meta = meta[:siAnalystEvidenceBudget] + "…[truncated]"
			}
			fmt.Fprintf(&b, "证据快照(JSON):\n%s\n", meta)
		}
	}
	return b.String()
}

// SIPatcherAgent is the LLM-backed Patcher stage of the self-improvement
// Meta Team. It implements biz.SIPatcherStage with a D10 daily quota.
type SIPatcherAgent struct {
	caller   biz.LLMCaller
	provider string
	model    string
	dailyMax int32
	lg       loggateway.Logger

	mu          sync.Mutex
	dailyCount  int32
	windowStart time.Time
}

// NewSIPatcherAgent wires the Patcher stage. dailyMax <= 0 falls back to 20
// (design D10).
func NewSIPatcherAgent(caller biz.LLMCaller, provider, model string, dailyMax int32, lg loggateway.Logger) *SIPatcherAgent {
	if dailyMax <= 0 {
		dailyMax = 20
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SIPatcherAgent{
		caller: caller, provider: provider, model: model, dailyMax: dailyMax,
		windowStart: time.Now(),
		lg:          lg.With(loggateway.Domain("si_patcher")),
	}
}

// Patch runs one Patcher invocation: diagnosis + (bounded) affected worktree
// file contents in, unified diff out.
func (a *SIPatcherAgent) Patch(ctx context.Context, req biz.SIPatchRequest) (*biz.PatcherOutput, error) {
	if a == nil || a.caller == nil {
		return nil, apierror.Internal("SELF_IMPROVEMENT", "patcher agent not initialized")
	}
	if !a.consumeQuota() {
		a.lg.Warn("si patcher daily quota exhausted",
			loggateway.StepID("si_patcher.quota"),
			loggateway.Str("run_id", siPatcherRunID(req.Run)))
		return nil, ErrSIPatcherQuotaExceeded
	}
	text, _, err := a.caller.Call(ctx, biz.LLMCallRequest{
		Provider: a.provider,
		Model:    a.model,
		System:   biz.SIPatcherSystemPrompt,
		User:     siPatcherUserMessage(req, a.lg),
	})
	if err != nil {
		return nil, fmt.Errorf("patcher llm: %w", err)
	}
	return biz.ParsePatcherOutputJSON(text)
}

// consumeQuota enforces the 24h-window daily budget (same pattern as Critic).
func (a *SIPatcherAgent) consumeQuota() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if now.Sub(a.windowStart) >= 24*time.Hour {
		a.dailyCount = 0
		a.windowStart = now
	}
	if a.dailyCount >= a.dailyMax {
		return false
	}
	a.dailyCount++
	return true
}

// siPatcherUserMessage packs diagnosis + retry hint + affected worktree file
// contents into the user prompt.
func siPatcherUserMessage(req biz.SIPatchRequest, lg loggateway.Logger) string {
	var b strings.Builder
	if req.Run != nil {
		fmt.Fprintf(&b, "run_id: %s\ntrigger_source: %s\n", req.Run.ID, req.Run.TriggerSource)
	}
	if req.Diagnosis != nil {
		fmt.Fprintf(&b, "根因: %s\n修复策略: %s\n影响面: %s\n影响文件: %s\n",
			req.Diagnosis.RootCause, req.Diagnosis.FixStrategy, req.Diagnosis.ImpactScope,
			strings.Join(req.Diagnosis.AffectedFiles, ", "))
	}
	if hint := strings.TrimSpace(req.RetryHint); hint != "" {
		fmt.Fprintf(&b, "上一次验证失败输出（第 %d 次重试）:\n%s\n", req.Attempt-1, hint)
	}
	if req.Diagnosis != nil && req.WorktreePath != "" {
		b.WriteString("\n相关文件当前内容（worktree）:\n")
		budget := siPatcherFileBudget
		for _, rel := range req.Diagnosis.AffectedFiles {
			if budget <= 0 {
				break
			}
			content, ok := siReadWorktreeFile(req.WorktreePath, rel, min(budget, siPatcherSingleFileBudget))
			if !ok {
				continue
			}
			n := fmt.Sprintf("--- %s ---\n%s\n", rel, content)
			b.WriteString(n)
			budget -= len(n)
		}
	} else if lg != nil && req.WorktreePath == "" {
		lg.Debug("si patcher: no worktree path, prompting without file contents",
			loggateway.StepID("si_patcher.prompt"))
	}
	return b.String()
}

// siReadWorktreeFile reads one file inside the worktree with path-traversal
// safety (the resolved path must stay within root). budget caps returned bytes.
func siReadWorktreeFile(root, rel string, budget int) (string, bool) {
	rel = strings.TrimSpace(rel)
	if rel == "" || budget <= 0 {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	full, err := filepath.Abs(filepath.Join(absRoot, rel))
	if err != nil || !strings.HasPrefix(full, absRoot+string(filepath.Separator)) {
		return "", false
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", false
	}
	if len(data) > budget {
		data = append(data[:budget], []byte("\n…[file truncated]")...)
	}
	return string(data), true
}

func siPatcherRunID(run *biz.SelfImprovementRun) string {
	if run == nil {
		return ""
	}
	return run.ID
}
