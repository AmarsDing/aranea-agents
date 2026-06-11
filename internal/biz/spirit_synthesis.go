package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type SynthesisStrategy string

const (
	SynthesisStrategyTemplate SynthesisStrategy = "template"
	SynthesisStrategyPrompt   SynthesisStrategy = "prompt"
	SynthesisStrategyHybrid   SynthesisStrategy = "hybrid"
)

type SynthesisInput struct {
	TeamResults []TeamSynthesisResult `json:"team_results"`
	Strategy    SynthesisStrategy     `json:"strategy"`
	Template    string                `json:"template,omitempty"`
	SpiritQuery string                `json:"spirit_query,omitempty"`
}

type TeamSynthesisResult struct {
	TeamID      string `json:"team_id"`
	TeamName    string `json:"team_name"`
	TaskName    string `json:"task_name"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	KeyFindings string `json:"key_findings,omitempty"`
}

type SynthesisOutput struct {
	Content       string                `json:"content"`
	Strategy      SynthesisStrategy     `json:"strategy"`
	TeamResults   []TeamSynthesisResult `json:"team_results"`
	SynthesizedAt string                `json:"synthesized_at"`
}

// SynthesisEventPublisher is a biz-level port for publishing synthesis completion
// events. Implemented by the service layer to bridge to event.Bus without
// importing the event package in biz.
//
// Stability:evolving
type SynthesisEventPublisher interface {
	PublishSynthesisCompleted(ctx context.Context, spiritSessionID string, output *SynthesisOutput)
}

// SynthesisUsecase orchestrates the full synthesis workflow: active team check,
// completed/failed team collection, cascade blocking, input assembly, engine
// execution, and event publishing. This consolidates business logic that was
// previously scattered across the service layer.
//
// Stability:evolving
type SynthesisUsecase struct {
	spiritUC *SpiritTeamUsecase
	engine   *SynthesisEngine
	pub      SynthesisEventPublisher
	lg       loggateway.Logger
}

var (
	ErrActiveTeamsExist = apierror.Conflict(apierror.DomainSpirit, "cannot synthesize: active teams still running")
	ErrNoCompletedTeams = apierror.BadRequest(apierror.DomainSpirit, "no completed or failed teams to synthesize")
	ErrNoTeamResults    = apierror.BadRequest(apierror.DomainSpirit, "no team results to synthesize")
	ErrUnknownStrategy  = apierror.BadRequest(apierror.DomainSpirit, "unknown synthesis strategy")
)

func NewSynthesisUsecase(
	spiritUC *SpiritTeamUsecase,
	engine *SynthesisEngine,
	pub SynthesisEventPublisher,
	lg loggateway.Logger,
) *SynthesisUsecase {
	return &SynthesisUsecase{
		spiritUC: spiritUC,
		engine:   engine,
		pub:      pub,
		lg:       lg,
	}
}

// SynthesizeResults executes the full synthesis workflow for a spirit session.
// It checks for active teams, collects completed/failed results, assembles the
// synthesis input, runs the engine, and publishes the completion event.
func (u *SynthesisUsecase) SynthesizeResults(ctx context.Context, spiritSessionID string, strategy string) (*SynthesisOutput, error) {
	// Step 1: Check for active teams — synthesis is only allowed when all teams are done.
	activeTeams, activeErr := u.spiritUC.ListActiveTeams(ctx, spiritSessionID)
	if activeErr != nil {
		u.lg.Warn("查询活跃团队失败，跳过活跃检查",
			loggateway.StepID("spirit.synthesis.active_check_err"),
			loggateway.Err(activeErr),
		)
	} else if len(activeTeams) > 0 {
		return nil, ErrActiveTeamsExist
	}

	// Step 2: Collect completed and failed teams.
	teams, err := u.spiritUC.ListCompletedAndFailedTeams(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	teamResults := u.spiritUC.BuildCascadeBlockedResults(ctx, teams)
	if len(teamResults) == 0 {
		return nil, ErrNoCompletedTeams
	}

	// Step 3: Assemble synthesis input.
	input := SynthesisInput{
		TeamResults: teamResults,
		Strategy:    SynthesisStrategy(strategy),
		SpiritQuery: u.spiritUC.GetSpiritQuery(ctx, spiritSessionID),
	}

	// Step 4: Execute synthesis engine.
	output, err := u.engine.Synthesize(ctx, input)
	if err != nil {
		return nil, err
	}

	// Step 5: Publish completion event via port.
	if u.pub != nil {
		u.pub.PublishSynthesisCompleted(ctx, spiritSessionID, output)
	}

	return output, nil
}

// ---------------------------------------------------------------------------
// SynthesisEngine — pure synthesis logic (no I/O, no external deps)
// ---------------------------------------------------------------------------

type SynthesisEngine struct{}

func NewSynthesisEngine() *SynthesisEngine {
	return &SynthesisEngine{}
}

func (e *SynthesisEngine) Synthesize(ctx context.Context, input SynthesisInput) (*SynthesisOutput, error) {
	if len(input.TeamResults) == 0 {
		return nil, ErrNoTeamResults
	}
	strategy := input.Strategy
	if strategy == "" {
		strategy = e.inferStrategy(input)
	}
	var content string
	var err error
	switch strategy {
	case SynthesisStrategyTemplate:
		content, err = e.synthesizeTemplate(input)
	case SynthesisStrategyPrompt:
		content = e.synthesizePrompt(input)
	case SynthesisStrategyHybrid:
		content, err = e.synthesizeTemplate(input)
		if err == nil {
			content += "\n\n---\n\n" + e.synthesizePrompt(input)
		}
	default:
		return nil, ErrUnknownStrategy
	}
	if err != nil {
		return nil, err
	}
	return &SynthesisOutput{
		Content:       content,
		Strategy:      strategy,
		TeamResults:   input.TeamResults,
		SynthesizedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (e *SynthesisEngine) inferStrategy(input SynthesisInput) SynthesisStrategy {
	if input.Template != "" {
		return SynthesisStrategyTemplate
	}
	hasFailed := false
	completedCount := 0
	for _, r := range input.TeamResults {
		if r.Status == "completed" {
			completedCount++
		}
		if r.Status == "failed" || r.Status == "blocked" {
			hasFailed = true
		}
	}
	if hasFailed {
		return SynthesisStrategyHybrid
	}
	if completedCount == len(input.TeamResults) && len(input.TeamResults) <= 3 {
		return SynthesisStrategyTemplate
	}
	return SynthesisStrategyHybrid
}

func (e *SynthesisEngine) synthesizeTemplate(input SynthesisInput) (string, error) {
	tpl := input.Template
	if tpl == "" {
		tpl = e.defaultTemplate()
	}
	var sb strings.Builder
	for i, r := range input.TeamResults {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("### %s\n\n", r.TeamName))
		sb.WriteString(fmt.Sprintf("- **任务**: %s\n", r.TaskName))
		sb.WriteString(fmt.Sprintf("- **状态**: %s\n", r.Status))
		if r.Summary != "" {
			sb.WriteString(fmt.Sprintf("- **结果**: %s\n", r.Summary))
		}
		if r.KeyFindings != "" {
			sb.WriteString(fmt.Sprintf("- **关键发现**: %s\n", r.KeyFindings))
		}
	}
	result := strings.ReplaceAll(tpl, "{{results}}", sb.String())
	result = strings.ReplaceAll(result, "{{query}}", input.SpiritQuery)
	return result, nil
}

func (e *SynthesisEngine) synthesizePrompt(input SynthesisInput) string {
	resultsJSON, _ := json.Marshal(input.TeamResults)
	return fmt.Sprintf(
		"请综合以下 %d 个团队的执行结果，回答用户的原始问题。\n\n"+
			"用户问题: %s\n\n"+
			"团队结果:\n```json\n%s\n```\n\n"+
			"请提供结构化的综合分析，包括：1) 核心发现汇总 2) 各团队结论对比 3) 最终建议",
		len(input.TeamResults),
		input.SpiritQuery,
		string(resultsJSON),
	)
}

func (e *SynthesisEngine) defaultTemplate() string {
	return "# 团队执行结果综合报告\n\n{{results}}\n\n---\n\n基于以上 {{query}} 的多团队并行分析，所有团队已完成任务。"
}
