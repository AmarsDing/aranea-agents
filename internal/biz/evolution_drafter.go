package biz

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/pkg/loggateway"
)

// EVO-20：L3 通知类建议的 LLM 草稿生成器。
//
// AgentConfigTrigger 等生产者只产出指标通知文本（Content），不带 apply_payload，
// 建议不可应用（Applicable()=false，前端隐藏应用按钮）。EvolutionDrafter 作为
// EvolutionOrchestratorWorker 的 post-pass，为 pending 且无 payload 的
// persona/prompt 建议生成具体修改草稿并写回 metadata：
//
//   - persona：读 IDENTITY.md ## Persona 段 → LLM 输出修订段正文（段级替换）
//   - prompt：读 AGENTS_CORE.md/首个 AGENTS* → LLM 输出完整修订文件（全文件
//     替换，与 ApplySuggestion 现有语义一致），长度校验兜底
//
// 写回 apply_payload + diff_preview 后 Applicable() 自动为 true。LLM 不可用 /
// 失败 / 校验不通过时静默降级为通知态（Warn 日志），每次 LLM 尝试（无论成败）
// 记录 draft_attempt_at，1h 内不重复尝试。
//
// 日志策略：仅进程日志（K6/K7）。进化流水线现有 trigger/orchestrator 均不发
// 流程日志，drafter 保持一致；草稿结果经建议列表直接对用户可见。

const (
	// draftAttemptThrottle 是同一建议两次 LLM 草稿尝试的最小间隔。
	draftAttemptThrottle = time.Hour
	// draftLLMTimeout 上限对齐 refineCallTimeout（PGO-3-BIZ-05）。
	draftLLMTimeout = 60 * time.Second
	// defaultPersonaMaxChars 是 EvoPersonaMaxChars 未配置（<=0）时的默认上限。
	defaultPersonaMaxChars = 2000
	// promptPayloadMaxChars 是 prompt 全文件草稿的硬上限。
	promptPayloadMaxChars = 20000
	// diffPreviewMaxChars 是 diff_preview 的展示上限（超出截断）。
	diffPreviewMaxChars = 4000
	// draftListLimit 是每 agent 每周期拉取的 pending 建议上限。
	draftListLimit = 20
)

// EvolutionDraftStore 是 drafter 对统一进化存储的窄依赖。
// Stability:evolving
type EvolutionDraftStore interface {
	UnifiedEvolutionQueryReader
	UnifiedEvolutionMetadataWriter
}

// EvolutionDraftAgentReader 是 drafter 对 Agent 仓储的窄依赖。
// Stability:evolving
type EvolutionDraftAgentReader interface {
	GetAgentByID(ctx context.Context, id string) (Agent, error)
	ListAgentPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error)
}

// EvolutionDrafter 为通知类 L3 建议生成可应用草稿。nil LLMCaller = 禁用（no-op）。
type EvolutionDrafter struct {
	store    EvolutionDraftStore
	agents   EvolutionDraftAgentReader
	settings AgentEvolutionSettingsReader // nil = 默认值
	llm      LLMCaller                    // nil = 禁用
	refine   *SystemSettingUsecase        // nil = 无 RefineLLM 回退
	lg       loggateway.Logger
}

// NewEvolutionDrafter wires the drafter. llm == nil 时 DraftPending 为 no-op。
func NewEvolutionDrafter(
	store EvolutionDraftStore,
	agents EvolutionDraftAgentReader,
	settings AgentEvolutionSettingsReader,
	llm LLMCaller,
	refine *SystemSettingUsecase,
	lg loggateway.Logger,
) *EvolutionDrafter {
	return &EvolutionDrafter{store: store, agents: agents, settings: settings, llm: llm, refine: refine, lg: lg}
}

// DraftPending 为该 agent 的 pending 通知类 persona/prompt 建议生成草稿。
// 每次调用最多处理 1 条（per-agent 每周期 1 条的自然节流）。所有失败均降级
// 为通知态，不向调用方返回错误。
func (d *EvolutionDrafter) DraftPending(ctx context.Context, agentID string) error {
	if d == nil || d.llm == nil || d.store == nil || d.agents == nil {
		return nil
	}
	rows, err := d.store.ListByTargetAndAction(ctx, string(EvolutionTargetAgent), agentID, string(EvolutionActionEvolve), "", "pending", draftListLimit, 0)
	if err != nil {
		d.lg.Warn("evolution drafter: list pending failed", loggateway.StepID("evolution.draft"), loggateway.Err(err))
		return nil
	}
	for i := range rows {
		row := &rows[i]
		typ := row.MetaString(EvoMetaLegacyType)
		if typ != "persona" && typ != "prompt" {
			continue
		}
		if strings.TrimSpace(row.MetaString(EvoMetaApplyPayload)) != "" {
			continue // 已有草稿
		}
		if throttled(row.MetaString(EvoMetaDraftAttemptAt)) {
			continue
		}
		d.draftOne(ctx, agentID, row, typ)
		return nil // 每 agent 每周期 1 条
	}
	return nil
}

// throttled 报告 attemptAt（RFC3339）是否在节流窗口内。
func throttled(attemptAt string) bool {
	if attemptAt == "" {
		return false
	}
	ts, err := time.Parse(time.RFC3339, attemptAt)
	return err == nil && time.Since(ts) < draftAttemptThrottle
}

func (d *EvolutionDrafter) draftOne(ctx context.Context, agentID string, row *UnifiedEvolutionSuggestion, typ string) {
	start := time.Now()
	// 无论成败先记尝试时间，保证节流生效。
	defer func() {
		if err := d.store.UpdateMetadataKey(ctx, row.ID, EvoMetaDraftAttemptAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
			d.lg.Warn("evolution drafter: record attempt failed", loggateway.StepID("evolution.draft"), loggateway.Err(err))
		}
	}()

	provider, model := d.resolveModel(ctx, agentID)
	if provider == "" || model == "" {
		return
	}
	files, err := d.agents.ListAgentPromptFiles(ctx, agentID)
	if err != nil {
		d.lg.Warn("evolution drafter: list prompt files failed", loggateway.StepID("evolution.draft"), loggateway.Err(err))
		return
	}

	var sysPrompt, userPrompt, oldContent string
	var maxChars int
	if typ == "persona" {
		oldContent = extractPersonaSection(findFile(files, "IDENTITY.md"))
		maxChars = d.personaMaxChars(ctx, agentID)
		sysPrompt, userPrompt = buildPersonaDraftPrompts(oldContent, row.MetaString(EvoMetaTitle), row.DraftBody, maxChars)
	} else {
		name, body := findAgentsFile(files)
		if name == "" {
			return // 无 AGENTS* 文件可改
		}
		oldContent = body
		maxChars = promptPayloadMaxChars
		sysPrompt, userPrompt = buildPromptDraftPrompts(name, body, row.MetaString(EvoMetaTitle), row.DraftBody)
	}

	callCtx, cancel := context.WithTimeout(ctx, draftLLMTimeout)
	defer cancel()
	resp, _, err := d.llm.Call(callCtx, LLMCallRequest{Provider: provider, Model: model, System: sysPrompt, User: userPrompt})
	if err != nil {
		d.lg.Warn("evolution drafter: llm call failed", loggateway.StepID("evolution.draft"), loggateway.Err(err))
		return
	}
	payload := strings.TrimSpace(stripCodeFence(resp))
	if typ == "persona" {
		payload = strings.TrimSpace(strings.TrimPrefix(payload, "## Persona"))
	}
	if !validDraftPayload(payload, oldContent, maxChars) {
		d.lg.Warn("evolution drafter: draft rejected by validation",
			loggateway.StepID("evolution.draft"),
			loggateway.Str("suggestion_id", row.ID),
			loggateway.Str("type", typ))
		return
	}
	payload = truncateAtLineBoundary(payload, maxChars)

	if err := d.store.UpdateMetadataKey(ctx, row.ID, EvoMetaApplyPayload, payload); err != nil {
		d.lg.Warn("evolution drafter: write payload failed", loggateway.StepID("evolution.draft"), loggateway.Err(err))
		return
	}
	diff := UnifiedDiffSimple(oldContent, payload)
	if utf8.RuneCountInString(diff) > diffPreviewMaxChars {
		diff = truncateAtLineBoundary(diff, diffPreviewMaxChars)
	}
	if err := d.store.UpdateMetadataKey(ctx, row.ID, EvoMetaDiffPreview, diff); err != nil {
		d.lg.Warn("evolution drafter: write diff failed", loggateway.StepID("evolution.draft"), loggateway.Err(err))
	}
	d.lg.Info("evolution drafter: draft generated",
		loggateway.StepID("evolution.draft"),
		loggateway.Str("suggestion_id", row.ID),
		loggateway.Str("type", typ),
		loggateway.Str("provider", provider),
		loggateway.Str("model", model),
		loggateway.Duration(time.Since(start).Milliseconds()))
}

// resolveModel 两层解析：agent 自有 provider/model → 平台 DefaultRefineLLM。
func (d *EvolutionDrafter) resolveModel(ctx context.Context, agentID string) (string, string) {
	ag, err := d.agents.GetAgentByID(ctx, agentID)
	if err == nil && strings.TrimSpace(ag.Provider) != "" && strings.TrimSpace(ag.Model) != "" {
		return ag.Provider, ag.Model
	}
	if d.refine != nil {
		if rl, rerr := d.refine.GetRefineLLM(ctx); rerr == nil {
			return strings.TrimSpace(rl.Provider), strings.TrimSpace(rl.Model)
		}
	}
	return "", ""
}

func (d *EvolutionDrafter) personaMaxChars(ctx context.Context, agentID string) int {
	if d.settings != nil {
		if s, err := d.settings.GetAgentRuntimeSettings(ctx, agentID); err == nil && s.EvoPersonaMaxChars > 0 {
			return s.EvoPersonaMaxChars
		}
	}
	return defaultPersonaMaxChars
}

// validDraftPayload 校验草稿：非空；prompt 全文件草稿不得短于原件一半（防
// LLM 截断导致配置丢失）。
func validDraftPayload(payload, oldContent string, maxChars int) bool {
	n := utf8.RuneCountInString(payload)
	if n == 0 || n > maxChars {
		return false
	}
	if oldLen := utf8.RuneCountInString(oldContent); oldLen > 0 && n < oldLen/2 {
		return false
	}
	return true
}

// extractPersonaSection 返回 ## Persona 段正文（不含标题行）；无该段返回 ""。
// 锚点逻辑与 replaceOrAppendPersona 保持一致。
func extractPersonaSection(identityBody string) string {
	const anchor = "## Persona"
	idx := strings.Index(identityBody, anchor)
	if idx < 0 {
		return ""
	}
	rest := identityBody[idx+len(anchor):]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[nl+1:]
	} else {
		return ""
	}
	if next := strings.Index(rest, "\n## "); next >= 0 {
		rest = rest[:next]
	}
	return strings.TrimSpace(rest)
}

// findFile 按精确文件名查找 prompt 文件。
func findFile(files []AgentPromptFile, name string) string {
	for _, f := range files {
		if f.Name == name {
			return f.Body
		}
	}
	return ""
}

// findAgentsFile 返回 AGENTS_CORE.md 或首个 AGENTS* 文件的 (name, body)，与
// ApplySuggestion prompt 分支的选择逻辑一致。
func findAgentsFile(files []AgentPromptFile) (string, string) {
	for _, f := range files {
		name := strings.TrimSpace(f.Name)
		if name == "AGENTS_CORE.md" || name == "AGENTS_TASK.md" || strings.HasPrefix(name, "AGENTS") {
			return name, f.Body
		}
	}
	return "", ""
}

func buildPersonaDraftPrompts(current, title, reason string, maxChars int) (string, string) {
	sys := "你是 Agent 人设优化专家。根据运行指标反馈，改写 Agent 的 Persona 段落。" +
		"只输出修订后的 Persona 正文（markdown），不要输出 ## 标题行，不要解释。"
	user := "当前 Persona 内容：\n---\n" + current + "\n---\n\n" +
		"运行反馈：" + title + "——" + reason + "\n\n" +
		"请输出修订后的 Persona 正文，针对反馈做出具体改进，保留原有合理内容。" +
		"长度不超过 " + strconv.Itoa(maxChars) + " 字符。"
	return sys, user
}

func buildPromptDraftPrompts(name, body, title, reason string) (string, string) {
	sys := "你是系统提示词优化专家。根据运行指标反馈，修订 Agent 的系统提示文件。" +
		"输出完整的修订后文件内容，不要解释，不要 markdown 代码块包装。"
	user := "当前文件 " + name + " 内容：\n---\n" + body + "\n---\n\n" +
		"运行反馈：" + title + "——" + reason + "\n\n" +
		"请输出修订后的完整文件，针对反馈改进相关策略，其余内容保持不变。"
	return sys, user
}
