package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ErrSICriticQuotaExceeded is returned when the Critic daily LLM quota is
// exhausted (design D10: 10 次/日). The pipeline treats any Critic error as a
// degradation (continues without G4), so quota exhaustion never blocks a run.
var ErrSICriticQuotaExceeded = errors.New("si critic daily quota exceeded")

// siCriticDiffBudget bounds the diff bytes sent to the Critic LLM.
const siCriticDiffBudget = 32 * 1024

// SICriticAgent is the LLM-backed Critic stage (G4) of the self-improvement
// Meta Team. It implements biz.SICriticStage: prompt contract from
// biz.SICriticSystemPrompt, output parsed by biz.ParseCriticReportJSON.
//
// 日配额计数模式复用 V2 skill_curator_worker（24h 窗口 + 计数重置）。
type SICriticAgent struct {
	caller   biz.LLMCaller
	provider string
	model    string
	dailyMax int32
	lg       loggateway.Logger

	mu          sync.Mutex
	dailyCount  int32
	windowStart time.Time

	// nowOffsetSec shifts the quota clock (test hook for window reset).
	nowOffsetSec atomic.Int64
}

// NewSICriticAgent wires the Critic stage. dailyMax <= 0 falls back to 10
// (design D10).
func NewSICriticAgent(caller biz.LLMCaller, provider, model string, dailyMax int32, lg loggateway.Logger) *SICriticAgent {
	if dailyMax <= 0 {
		dailyMax = 10
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SICriticAgent{
		caller:      caller,
		provider:    provider,
		model:       model,
		dailyMax:    dailyMax,
		windowStart: time.Now(),
		lg:          lg.With(loggateway.Domain("si_critic")),
	}
}

// Review runs one Critic G4 semantic review of the patch diff.
func (a *SICriticAgent) Review(ctx context.Context, run *biz.SelfImprovementRun, patch *biz.PatcherOutput) (*biz.CriticReport, error) {
	if a == nil || a.caller == nil {
		return nil, apierror.Internal("SELF_IMPROVEMENT", "critic agent not initialized")
	}
	if !a.consumeQuota() {
		a.lg.Warn("si critic daily quota exhausted, G4 degraded",
			loggateway.StepID("si_critic.quota"),
			loggateway.Str("run_id", criticRunID(run)))
		return nil, ErrSICriticQuotaExceeded
	}

	user := siCriticUserMessage(run, patch)
	text, _, err := a.caller.Call(ctx, biz.LLMCallRequest{
		Provider: a.provider,
		Model:    a.model,
		System:   biz.SICriticSystemPrompt,
		User:     user,
	})
	if err != nil {
		return nil, fmt.Errorf("critic llm: %w", err)
	}
	return biz.ParseCriticReportJSON(text)
}

// consumeQuota enforces the 24h-window daily budget. One Review consumes one
// unit regardless of LLM outcome (the call itself is the cost).
func (a *SICriticAgent) consumeQuota() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().Add(time.Duration(a.nowOffsetSec.Load()) * time.Second)
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

// siCriticUserMessage packs run context + (bounded) diff into the user prompt.
func siCriticUserMessage(run *biz.SelfImprovementRun, patch *biz.PatcherOutput) string {
	var b strings.Builder
	if run != nil {
		fmt.Fprintf(&b, "触发源: %s\n", run.TriggerSource)
		if run.Diagnosis != nil {
			fmt.Fprintf(&b, "诊断: %s（影响面 %s，策略 %s）\n",
				run.Diagnosis.RootCause, run.Diagnosis.ImpactScope, run.Diagnosis.FixStrategy)
		}
	}
	diff := ""
	kind := ""
	if patch != nil {
		diff = patch.Diff
		kind = string(patch.Kind)
	}
	if len(diff) > siCriticDiffBudget {
		diff = diff[:siCriticDiffBudget] + "\n…[diff truncated]"
	}
	fmt.Fprintf(&b, "补丁类型: %s\n待审查 diff:\n%s", kind, diff)
	return b.String()
}

func criticRunID(run *biz.SelfImprovementRun) string {
	if run == nil {
		return ""
	}
	return run.ID
}
