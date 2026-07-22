package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
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
	// B.10.17: per-unit run statistics, enriched via SpiritTeamRunStatsReader.
	// Omitted when the reader is not wired.
	DurationMs   int64  `json:"duration_ms,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// ExecutionOverview is the aggregate section of the execution report (B.10.17).
type ExecutionOverview struct {
	Query          string `json:"query"`
	FinalStatus    string `json:"final_status"` // completed | partial_failure | failed
	DurationMs     int64  `json:"duration_ms"`
	TotalUnits     int    `json:"total_units"`
	CompletedUnits int    `json:"completed_units"`
	FailedUnits    int    `json:"failed_units"`
	TokenIn        int    `json:"token_in"`
	TokenOut       int    `json:"token_out"`
}

// DeliverableItem is one deliverable entry in the execution report (B.10.17),
// mapped from the team's DeliverableRef envelopes keyed by dag node id.
type DeliverableItem struct {
	NodeID    string `json:"node_id"`
	UnitName  string `json:"unit_name"` // 产出团队显示名
	Summary   string `json:"summary"`
	Type      string `json:"type,omitempty"`
	Format    string `json:"format,omitempty"`
	SizeChars int    `json:"size_chars"`
}

type SynthesisOutput struct {
	Content       string                `json:"content"`
	Strategy      SynthesisStrategy     `json:"strategy"`
	TeamResults   []TeamSynthesisResult `json:"team_results"`
	SynthesizedAt string                `json:"synthesized_at"`
	// B.10.17 execution report sections. Overview is always attached by
	// SynthesizeResults; Deliverables is empty when no team produced output;
	// Degraded marks an LLM-conclusion-missing report (structured sections
	// preserved).
	Overview     *ExecutionOverview `json:"overview,omitempty"`
	Deliverables []DeliverableItem  `json:"deliverables,omitempty"`
	Degraded     bool               `json:"degraded,omitempty"`
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
	ErrActiveTeamsExist       = apierror.Conflict(apierror.DomainSpirit, "cannot synthesize: active teams still running")
	ErrNoCompletedTeams       = apierror.BadRequest(apierror.DomainSpirit, "no completed or failed teams to synthesize")
	ErrNoTeamResults          = apierror.BadRequest(apierror.DomainSpirit, "no team results to synthesize")
	ErrUnknownStrategy        = apierror.BadRequest(apierror.DomainSpirit, "unknown synthesis strategy")
	ErrSynthesisModelRequired = apierror.Unavailable(apierror.DomainSpirit, "synthesis model required in production")
	ErrSynthesisModelFailed   = apierror.Unavailable(apierror.DomainSpirit, "synthesis model generate failed")
)

// isProductionEnv reports whether ARANEA_ENV is production or prod (C-24).
func isProductionEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ARANEA_ENV"))) {
	case "production", "prod":
		return true
	default:
		return false
	}
}

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
		// B.10.17 FR-6 degraded path: the LLM conclusion failed (C-24 semantics
		// — the engine error is still returned to the RPC caller), but the
		// structured report sections are preserved and published so the user
		// still gets a reviewable execution summary.
		degraded := &SynthesisOutput{
			Strategy:      input.Strategy,
			TeamResults:   teamResults,
			SynthesizedAt: time.Now().UTC().Format(time.RFC3339),
			Degraded:      true,
		}
		u.attachReportData(ctx, spiritSessionID, teams, degraded)
		if u.pub != nil {
			u.pub.PublishSynthesisCompleted(ctx, spiritSessionID, degraded)
		}
		return degraded, err
	}

	// Step 5: Attach execution report sections (B.10.17).
	u.attachReportData(ctx, spiritSessionID, teams, output)

	// Step 6: Publish completion event via port.
	if u.pub != nil {
		u.pub.PublishSynthesisCompleted(ctx, spiritSessionID, output)
	}

	return output, nil
}

// attachReportData populates the execution report sections (overview,
// deliverables, per-unit run stats) on the synthesis output.
func (u *SynthesisUsecase) attachReportData(ctx context.Context, spiritSessionID string, teams []Team, output *SynthesisOutput) {
	output.Overview = u.assembleOverview(ctx, spiritSessionID, teams, output.TeamResults)
	output.Deliverables = assembleDeliverableItems(teams)
	u.attachTeamRunStats(ctx, output.TeamResults)
}

// assembleOverview builds the aggregate overview: status derivation from unit
// results, duration from team CreatedAt→UpdatedAt span, and token aggregation
// reused from CheckAllTeamsCompleted.
func (u *SynthesisUsecase) assembleOverview(ctx context.Context, spiritSessionID string, teams []Team, results []TeamSynthesisResult) *ExecutionOverview {
	ov := &ExecutionOverview{
		Query:      u.spiritUC.GetSpiritQuery(ctx, spiritSessionID),
		TotalUnits: len(results),
	}
	for _, r := range results {
		if r.Status == TeamStatusCompleted {
			ov.CompletedUnits++
		} else {
			ov.FailedUnits++
		}
	}
	switch {
	case ov.FailedUnits == 0:
		ov.FinalStatus = TeamStatusCompleted
	case ov.CompletedUnits == 0:
		ov.FinalStatus = TeamStatusFailed
	default:
		ov.FinalStatus = "partial_failure"
	}
	ov.DurationMs = aggregateTeamsDurationMs(teams)
	// Token aggregation reuses the CheckAllTeamsCompleted read path (terminal
	// state is consistent by the time synthesis runs).
	agg := u.spiritUC.CheckAllTeamsCompleted(ctx, spiritSessionID)
	ov.TokenIn = agg.TotalTokenIn
	ov.TokenOut = agg.TotalTokenOut
	return ov
}

// aggregateTeamsDurationMs computes max(UpdatedAt) - min(CreatedAt) across
// teams. Team covers the full orchestration span (no StartedAt field).
// Returns 0 when timestamps are missing/unparseable or inverted.
func aggregateTeamsDurationMs(teams []Team) int64 {
	var minCreated, maxUpdated time.Time
	for _, t := range teams {
		if c, err := parseTimeFlexible(t.CreatedAt); err == nil {
			if minCreated.IsZero() || c.Before(minCreated) {
				minCreated = c
			}
		}
		if u, err := parseTimeFlexible(t.UpdatedAt); err == nil {
			if u.After(maxUpdated) {
				maxUpdated = u
			}
		}
	}
	if minCreated.IsZero() || maxUpdated.IsZero() || !maxUpdated.After(minCreated) {
		return 0
	}
	return maxUpdated.Sub(minCreated).Milliseconds()
}

// assembleDeliverableItems maps each team's DeliverableRef envelopes to
// report deliverable items. Node IDs are sorted for deterministic output.
// Legacy summary-only refs get SizeChars derived from the summary rune count
// (the legacy string IS the full stored summary).
func assembleDeliverableItems(teams []Team) []DeliverableItem {
	var items []DeliverableItem
	for _, t := range teams {
		refs := ParseDeliverableRefs(t.DeliverablesOutput)
		if len(refs) == 0 {
			continue
		}
		nodeIDs := make([]string, 0, len(refs))
		for id := range refs {
			nodeIDs = append(nodeIDs, id)
		}
		sort.Strings(nodeIDs)
		for _, nodeID := range nodeIDs {
			ref := refs[nodeID]
			size := ref.SizeChars
			if size == 0 {
				size = len([]rune(ref.Summary))
			}
			items = append(items, DeliverableItem{
				NodeID:    nodeID,
				UnitName:  t.DisplayName,
				Summary:   ref.Summary,
				SizeChars: size,
			})
		}
	}
	return items
}

// attachTeamRunStats enriches per-unit duration and error reason via the
// SpiritTeamRunStatsReader port. No-op when the reader is not wired.
func (u *SynthesisUsecase) attachTeamRunStats(ctx context.Context, results []TeamSynthesisResult) {
	if len(results) == 0 {
		return
	}
	teamIDs := make([]string, 0, len(results))
	for _, r := range results {
		teamIDs = append(teamIDs, r.TeamID)
	}
	stats := u.spiritUC.ListTeamRunStats(ctx, teamIDs)
	if len(stats) == 0 {
		return
	}
	for i := range results {
		if s, ok := stats[results[i].TeamID]; ok {
			results[i].DurationMs = s.DurationMs
			results[i].ErrorMessage = s.ErrorMessage
		}
	}
}

// ---------------------------------------------------------------------------
// SynthesisEngine — pure synthesis logic (no I/O, no external deps)
// ---------------------------------------------------------------------------

// SynthesisModelPort is a biz-level port for invoking an LLM to synthesize
// team results into a final answer. Implemented by the service layer adapter
// (which resolves provider/model via system settings + catalog and delegates
// to biz.LLMCaller). nil-safe: when not wired, the engine falls back to
// returning the raw synthesis prompt so callers still get usable content.
//
// C-24 fix: previously SynthesisEngine.synthesizePrompt only built a prompt
// string and never called any LLM, so Prompt/Hybrid strategies produced no
// real synthesis. The port lets the engine invoke a model while keeping biz
// free of LLM infrastructure details.
//
// Stability:evolving
type SynthesisModelPort interface {
	// SynthesizeWithModel calls the resolved LLM with the given system and
	// user prompts and returns the model's text output. Implementations MUST
	// be nil-safe at the call site (the engine checks for nil before calling).
	SynthesizeWithModel(ctx context.Context, system, user string) (string, error)
}

type SynthesisEngine struct {
	// model is optional; nil falls back to raw prompt output (used in tests
	// and when no LLM is configured).
	model SynthesisModelPort
	lg    loggateway.Logger
}

// NewSynthesisEngine constructs a SynthesisEngine. model and lg may be nil;
// when model is nil, Prompt/Hybrid strategies return the raw prompt string.
func NewSynthesisEngine(model SynthesisModelPort, lg loggateway.Logger) *SynthesisEngine {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SynthesisEngine{model: model, lg: lg}
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
		content, err = e.synthesizePromptOrModel(ctx, input)
	case SynthesisStrategyHybrid:
		content, err = e.synthesizeTemplate(input)
		if err == nil {
			var modelPart string
			modelPart, err = e.synthesizePromptOrModel(ctx, input)
			if err == nil {
				content += "\n\n---\n\n" + modelPart
			}
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

// synthesizePromptOrModel invokes the LLM via the model port when wired.
// C-24: in production (ARANEA_ENV=production|prod), nil model or Generate
// failure returns an error — never the raw prompt template. Non-prod keeps
// the template fallback with a warn log.
func (e *SynthesisEngine) synthesizePromptOrModel(ctx context.Context, input SynthesisInput) (string, error) {
	rawPrompt := e.synthesizePrompt(input)
	prod := isProductionEnv()
	if e.model == nil {
		if prod {
			return "", ErrSynthesisModelRequired
		}
		e.lg.Warn("synthesis model port nil, falling back to raw prompt (non-prod)",
			loggateway.StepID("spirit.synthesis.model_nil_fallback"))
		return rawPrompt, nil
	}
	systemPrompt := "你是一名资深的多团队协作结果综合分析师。请基于多个团队的执行结果，为用户产出结构化、可执行的综合分析。"
	text, err := e.model.SynthesizeWithModel(ctx, systemPrompt, rawPrompt)
	if err != nil {
		if prod {
			e.lg.Error("LLM 综合失败（生产环境不回退）",
				loggateway.StepID("spirit.synthesis.model_fail_prod"),
				loggateway.Err(err))
			return "", fmt.Errorf("%w: %v", ErrSynthesisModelFailed, err)
		}
		e.lg.Warn("LLM 综合失败，回退到原始 prompt",
			loggateway.StepID("spirit.synthesis.model_fallback"),
			loggateway.Err(err))
		return rawPrompt, nil
	}
	if text = strings.TrimSpace(text); text == "" {
		if prod {
			return "", ErrSynthesisModelFailed
		}
		return rawPrompt, nil
	}
	return text, nil
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
