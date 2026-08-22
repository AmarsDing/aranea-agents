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
	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/internal/tools/patcherfs"
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
// 工具回路：Analyst 对仓库根只读（patcher_fs_read/list）；Patcher 对本次
// worktree 读写 + git diff。官方产物仍是 Diagnosis / PatcherOutput JSON。
// Patcher 写盘后 Restore，pipeline 按 diff ApplyDiff。
//
// 成本控制（D10）：Patcher 日配额 20 次（24h 窗口，按 LLM 调用计——含
// 工具轮次与一次格式纠正）；配额耗尽返回 ErrSIPatcherQuotaExceeded。

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
	readRoot string
	rca      heal.RootCauseAnalyzer
	lg       loggateway.Logger
}

// SIAnalystOption customizes NewSIAnalystAgent.
type SIAnalystOption func(*SIAnalystAgent)

// WithSIAnalystReadRoot binds Analyst read-only tools to the repository root.
func WithSIAnalystReadRoot(root string) SIAnalystOption {
	return func(a *SIAnalystAgent) {
		a.readRoot = strings.TrimSpace(root)
	}
}

// NewSIAnalystAgent wires the Analyst stage.
func NewSIAnalystAgent(caller biz.LLMCaller, provider, model string, lg loggateway.Logger, opts ...SIAnalystOption) *SIAnalystAgent {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	a := &SIAnalystAgent{
		caller: caller, provider: provider, model: model,
		lg: lg.With(loggateway.Domain("si_analyst")),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	return a
}

// Analyze runs one Analyst diagnosis of the run's originating suggestion.
//
// S5 格式纠正重试：LLM 输出解析失败（非合法 JSON / 契约字段缺失）时，把
// 解析错误与原始输出反馈给模型重问一次；仍失败则返回解析错误。LLM 调用
// 本身失败（网络/限流）不重试。
func (a *SIAnalystAgent) Analyze(ctx context.Context, run *biz.SelfImprovementRun, sug *biz.UnifiedEvolutionSuggestion) (*biz.Diagnosis, error) {
	if a == nil || a.caller == nil {
		return nil, apierror.Internal("SELF_IMPROVEMENT", "analyst agent not initialized")
	}
	report, rcaHint := a.rcaPrior(ctx, run, sug)
	user := siAnalystUserMessage(run, sug) + rcaHint + siAnalystToolsHint(a.readRoot)
	var ws *patcherfs.Workspace
	if a.readRoot != "" {
		bound, werr := patcherfs.New(a.readRoot, patcherfs.ModeRead)
		if werr != nil {
			a.lg.Warn("si analyst: read root unavailable, diagnosing without tools",
				loggateway.StepID("si_analyst.workspace"), loggateway.Err(werr))
		} else {
			ws = bound
		}
	}
	diag, err := siRunToolLoop(ctx, func(ctx context.Context, user string) (string, error) {
		text, _, err := a.caller.Call(ctx, biz.LLMCallRequest{
			Provider: a.provider,
			Model:    a.model,
			System:   biz.SIAnalystSystemPrompt,
			User:     user,
		})
		if err != nil {
			return "", fmt.Errorf("analyst llm: %w", err)
		}
		return text, nil
	}, ws, user, biz.ParseDiagnosisJSON, siAnalystMaxToolRounds, a.lg, "si_analyst")
	if err != nil {
		return nil, err
	}
	return siEnrichDiagnosis(diag, report), nil
}

// siFormatFeedbackLimit caps the bad-output bytes echoed back to the LLM.
const siFormatFeedbackLimit = 1024

// siFormatCorrection builds the format-correction retry prompt: original user
// message + parse error + (bounded) bad output, asking for strict re-output.
func siFormatCorrection(original, badOutput string, parseErr error) string {
	bad := strings.TrimSpace(badOutput)
	if len(bad) > siFormatFeedbackLimit {
		bad = bad[:siFormatFeedbackLimit] + "\n…[truncated]"
	}
	return fmt.Sprintf("%s\n\n[系统]上一次输出无法解析：%v\n上一次输出：\n%s\n请严格按要求重新输出一个 JSON 对象（不要输出任何额外文字）。",
		original, parseErr, bad)
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
//
// S5 格式纠正重试：LLM 输出解析失败时反馈解析错误重问一次。配额按 LLM 调用
// 计（一次 Patch 最多消耗 2 单位：首次 + 格式重试）；重试时配额耗尽则放弃
// 重试、返回原始解析错误。
func (a *SIPatcherAgent) Patch(ctx context.Context, req biz.SIPatchRequest) (*biz.PatcherOutput, error) {
	if a == nil || a.caller == nil {
		return nil, apierror.Internal("SELF_IMPROVEMENT", "patcher agent not initialized")
	}
	runID := siPatcherRunID(req.Run)
	user := siPatcherUserMessage(req, a.lg) + siPatcherToolsHint(req.WorktreePath)
	var ws *patcherfs.Workspace
	if req.WorktreePath != "" {
		bound, werr := patcherfs.New(req.WorktreePath, patcherfs.ModeReadWrite)
		if werr != nil {
			a.lg.Warn("si patcher: worktree unavailable, patching without tools",
				loggateway.StepID("si_patcher.workspace"),
				loggateway.Str("run_id", runID), loggateway.Err(werr))
		} else {
			ws = bound
			defer func() {
				if rerr := ws.Restore(); rerr != nil {
					a.lg.Warn("si patcher: restore worktree after tools failed",
						loggateway.StepID("si_patcher.restore"),
						loggateway.Str("run_id", runID), loggateway.Err(rerr))
				}
			}()
		}
	}
	out, err := siRunToolLoop(ctx, func(ctx context.Context, user string) (string, error) {
		return a.callLLM(ctx, runID, user)
	}, ws, user, biz.ParsePatcherOutputJSON, siPatcherMaxToolRounds, a.lg, "si_patcher")
	if err != nil {
		if filled, ferr := siMaybeFillDiffFromWorktree(ws, "", nil, err); ferr == nil && filled != nil {
			return filled, nil
		}
		return nil, err
	}
	if filled, ferr := siMaybeFillDiffFromWorktree(ws, "", out, nil); ferr == nil && filled != nil {
		return filled, nil
	}
	return out, nil
}

// callLLM consumes one quota unit then performs the Patcher LLM call.
func (a *SIPatcherAgent) callLLM(ctx context.Context, runID, user string) (string, error) {
	if !a.consumeQuota() {
		a.lg.Warn("si patcher daily quota exhausted",
			loggateway.StepID("si_patcher.quota"),
			loggateway.Str("run_id", runID))
		return "", ErrSIPatcherQuotaExceeded
	}
	text, _, err := a.caller.Call(ctx, biz.LLMCallRequest{
		Provider: a.provider,
		Model:    a.model,
		System:   biz.SIPatcherSystemPrompt,
		User:     user,
	})
	if err != nil {
		return "", fmt.Errorf("patcher llm: %w", err)
	}
	return text, nil
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
