package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

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
	// Per-unit run statistics, enriched via SpiritTeamRunStatsReader.
	// Omitted when the reader is not wired.
	DurationMs   int64  `json:"duration_ms,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
}

type SynthesisOutput struct {
	Content       string                `json:"content"`
	Strategy      SynthesisStrategy     `json:"strategy"`
	TeamResults   []TeamSynthesisResult `json:"team_results"`
	SynthesizedAt string                `json:"synthesized_at"`
}

// ---------------------------------------------------------------------------
// 2026-07-25 Fix 3 收尾报告诚实化
// ---------------------------------------------------------------------------

// TeamFailureBrief is the honest-failure summary fed to the Spirit summary
// report: team name, its task, the recorded failure reason, and its last
// reply (which carries the team's unresolved questions — the 19:29 case was
// an upstream team that only asked clarifying questions yet the final report
// claimed full success).
type TeamFailureBrief struct {
	TeamName  string
	TaskName  string
	Reason    string
	LastReply string
}

// TeamDeliverableDigest is one terminal team's deliverable summary inlined
// into the synthesis trigger (F7, Phase 11): the Spirit LLM composes the
// final report from real structured outputs instead of excavating session
// history with read_session_history. DeliverableSummary is empty when the
// team left no deliverable (failed teams) — rendered as「无交付物」.
type TeamDeliverableDigest struct {
	TeamName           string
	TaskName           string
	Status             string
	DeliverableSummary string
	// Artifacts lists the team's payload entries as compact one-line
	// descriptors (e.g. article《云计算十年》（markdown，8234字）), so the
	// Spirit composing the final report knows long-form full texts exist and
	// can fetch them via read_upstream_deliverable when needed.
	Artifacts []string
}

// synthesisDigestMaxRunes caps each team's digest in the trigger so a
// pathological summary cannot blow up the system-push message budget.
const synthesisDigestMaxRunes = 500

// synthesisDigestMaxArtifacts caps the artifact listing per team in the
// trigger — a pointer-level line per payload, bounded like the summary.
const synthesisDigestMaxArtifacts = 5

// renderSynthesisDigests renders the「各团队交付物摘要」trigger section.
// Returns "" when no digests exist (legacy behavior preserved).
func renderSynthesisDigests(digests []TeamDeliverableDigest) string {
	if len(digests) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n各团队交付物摘要（真实执行产出摘要与载荷清单；长文交付物全文可经 read_upstream_deliverable 按需获取）：\n")
	for _, d := range digests {
		summary := strings.TrimSpace(d.DeliverableSummary)
		if summary == "" {
			summary = "（无交付物）"
		} else if utf8.RuneCountInString(summary) > synthesisDigestMaxRunes {
			summary = TruncateRunes(summary, synthesisDigestMaxRunes) + "…"
		}
		sb.WriteString(fmt.Sprintf("- 团队「%s」（任务：%s，状态：%s）：%s\n", d.TeamName, d.TaskName, d.Status, summary))
		for i, line := range d.Artifacts {
			if i >= synthesisDigestMaxArtifacts {
				sb.WriteString(fmt.Sprintf("  - 载荷：……另有 %d 项\n", len(d.Artifacts)-synthesisDigestMaxArtifacts))
				break
			}
			sb.WriteString(fmt.Sprintf("  - 载荷：%s\n", TruncateRunes(line, 80)))
		}
	}
	return sb.String()
}

// synthesisSummarySuccessTrigger is the summary-report structure used when
// every team completed. The LLM reply (a normal reply step, persisted and
// refresh-safe) IS the summary report — there is no dedicated report UI.
const synthesisSummarySuccessTrigger = "所有团队已完成。请基于%s会话中各团队的执行结果，输出最终任务总结报告（Markdown 格式），严格按以下结构：\n" +
	"## 任务总结\n" +
	"（一段话概述用户目标与整体完成情况）\n" +
	"## 各团队结果\n" +
	"（逐团队列出：团队名称、承担任务、完成状态、核心结论；失败团队需说明失败原因）\n" +
	"## 综合结论\n" +
	"（跨团队的核心发现、结论对比与最终答案）\n" +
	"## 建议与后续行动\n" +
	"（如无建议可省略本节）"

// BuildSynthesisSummaryTrigger renders the system-push message injected into
// the Spirit session when all teams reach a terminal state. When failures
// exist the trigger must be honest: state the real completed/failed counts,
// list each failed team with its reason and last reply (its unresolved
// questions), demand an「未解决问题」section, and forbid fabricating
// conclusions for failed teams. Claiming "所有团队已完成" while teams failed
// is what made the 19:29 final report a lie (2026-07-25 Fix 3).
//
// F7 (Phase 11): digests inline each terminal team's deliverable summary so
// the LLM builds the report from real outputs — no history archaeology.
func BuildSynthesisSummaryTrigger(total, completed, failed int, failures []TeamFailureBrief, digests []TeamDeliverableDigest) string {
	digestSection := renderSynthesisDigests(digests)
	if failed <= 0 {
		basis := ""
		if digestSection != "" {
			basis = "下列各团队交付物摘要与"
		}
		head := fmt.Sprintf(synthesisSummarySuccessTrigger, basis)
		if digestSection == "" {
			return head
		}
		return head + "\n" + digestSection
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("所有团队已结束：共 %d 个团队，%d 个完成，%d 个失败。请基于会话中各团队的执行结果与下列失败事实，输出最终任务总结报告（Markdown 格式）。\n",
		total, completed, failed))
	if digestSection != "" {
		sb.WriteString(digestSection)
	}
	sb.WriteString("\n失败事实（必须如实呈现，禁止隐瞒）：\n")
	for _, f := range failures {
		sb.WriteString(fmt.Sprintf("- 团队「%s」（任务：%s）失败，原因：%s\n", f.TeamName, f.TaskName, f.Reason))
		if strings.TrimSpace(f.LastReply) != "" {
			sb.WriteString(fmt.Sprintf("  该团队最后回复（含其遗留疑问）：%s\n", f.LastReply))
		}
	}
	sb.WriteString("\n严格按以下结构输出：\n")
	sb.WriteString("## 任务总结\n")
	sb.WriteString("（一段话概述用户目标与整体完成情况；存在失败团队时必须如实说明整体目标未完全达成，禁止声称全部成功）\n")
	sb.WriteString("## 各团队结果\n")
	sb.WriteString("（逐团队列出：团队名称、承担任务、完成状态、核心结论；失败团队需说明失败原因）\n")
	sb.WriteString("## 未解决问题\n")
	sb.WriteString("（汇总失败团队遗留的疑问与阻塞事项，逐条列出需要用户澄清或决策的内容）\n")
	sb.WriteString("## 综合结论\n")
	sb.WriteString("（仅基于已完成团队的真实产出给出结论；禁止为失败团队虚构结论，禁止把失败回复当作产出）\n")
	sb.WriteString("## 建议与后续行动\n")
	sb.WriteString("（如无建议可省略本节）")
	return sb.String()
}

// SynthesisUsecase orchestrates the synthesis workflow: active team check,
// completed/failed team collection, cascade blocking, input assembly, and
// engine execution. The output is returned inline to callers (RPC / tool);
// the user-facing summary report in chat is produced by the Spirit summary
// turn triggered by TeamStarter, not by this usecase.
//
// Stability:evolving
type SynthesisUsecase struct {
	spiritUC *SpiritTeamUsecase
	engine   *SynthesisEngine
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
	lg loggateway.Logger,
) *SynthesisUsecase {
	return &SynthesisUsecase{
		spiritUC: spiritUC,
		engine:   engine,
		lg:       lg,
	}
}

// SynthesizeResults executes the synthesis workflow for a spirit session.
// It checks for active teams, collects completed/failed results, assembles
// the synthesis input, and runs the engine. The output is returned inline.
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

	// Step 5: Enrich per-unit duration and error reason (consumed by the
	// synthesize_results tool output given to the calling agent).
	u.attachTeamRunStats(ctx, output.TeamResults)

	return output, nil
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
	systemPrompt := "你是一名资深的多团队协作结果综合分析师。请基于多个团队的执行结果，为用户产出结构化、可执行的综合分析。必须如实反映失败团队与未解决问题，禁止虚构成功。"
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
		tpl = e.defaultTemplate(hasFailedResults(input.TeamResults))
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

// hasFailedResults reports whether any team result is failed or blocked —
// the default template closing must not claim full success in that case
// (2026-07-25 Fix 3).
func hasFailedResults(results []TeamSynthesisResult) bool {
	for _, r := range results {
		if r.Status == "failed" || r.Status == "blocked" {
			return true
		}
	}
	return false
}

func (e *SynthesisEngine) synthesizePrompt(input SynthesisInput) string {
	resultsJSON, _ := json.Marshal(input.TeamResults)
	return fmt.Sprintf(
		"请综合以下 %d 个团队的执行结果，回答用户的原始问题。\n\n"+
			"用户问题: %s\n\n"+
			"团队结果:\n```json\n%s\n```\n\n"+
			"请提供结构化的综合分析，包括：1) 核心发现汇总 2) 各团队结论对比 3) 最终建议。\n"+
			"诚实性要求：如实反映每个团队的完成状态；存在失败或受阻团队时必须明确说明，"+
			"禁止虚构其结论或把提问澄清当作产出；如有未解决问题请汇总列出，供用户澄清。",
		len(input.TeamResults),
		input.SpiritQuery,
		string(resultsJSON),
	)
}

// defaultTemplate returns the built-in report template. The closing line is
// derived from team statuses: with failures it states the partial outcome
// honestly instead of claiming "所有团队已完成任务" (2026-07-25 Fix 3).
func (e *SynthesisEngine) defaultTemplate(hasFailed bool) string {
	closing := "基于以上 {{query}} 的多团队并行分析，所有团队已完成任务。"
	if hasFailed {
		closing = "基于以上 {{query}} 的多团队并行分析，部分团队执行失败（详见各团队状态）；结论仅基于成功团队的产出，失败团队的遗留问题需用户关注。"
	}
	return "# 团队执行结果综合报告\n\n{{results}}\n\n---\n\n" + closing
}
