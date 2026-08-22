package agent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	stderrors "errors"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/metrics"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	agentFactorySource     = "system"
	agentFactoryAuthor     = "agent-factory"
	agentFactoryLLMTimeout = 60 * time.Second

	// sameDomainReuseMinSim is the cosine threshold for reusing a same-domain
	// agent by mission similarity before creating a new one（B.10.21.6）。
	sameDomainReuseMinSim = 0.85
	// sameDomainReuseScanLimit bounds the active-agent scan for reuse checks.
	sameDomainReuseScanLimit = 200
)

// GeneratedAgentDefinition is the LLM-generated agent definition parsed from
// the model response. Field names match the JSON schema in the prompt.
type GeneratedAgentDefinition struct {
	DisplayName      string `json:"display_name"`
	Description      string `json:"description"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	SystemPrompt     string `json:"system_prompt"`
	MissionStatement string `json:"mission_statement"`
	DomainPath       string `json:"domain_path"`
}

// AgentFactoryImpl implements biz.AgentFactory by dynamically generating
// Agents via LLM when 4-layer matching fails.
type AgentFactoryImpl struct {
	llm          trpcmodel.Model
	agentWriter  biz.AgentWriter
	agentReader  biz.AgentReader
	templateRepo biz.AgentTemplateRepo
	eventBus     biz.EventBus
	embedder     knowledge.Embedder // nil → 同域复用检查跳过（仅 key 命中，不变量 3）
	org          biz.OrganizationReader
	lg           loggateway.Logger
}

var _ biz.AgentFactory = (*AgentFactoryImpl)(nil)

// NewAgentFactoryImpl creates a new AgentFactory implementation.
// The llm parameter may be nil when no planner model is configured; in that
// case EnsureAgent returns an Internal error so callers can fall back.
func NewAgentFactoryImpl(
	llm trpcmodel.Model,
	agentWriter biz.AgentWriter,
	agentReader biz.AgentReader,
	templateRepo biz.AgentTemplateRepo,
	eventBus biz.EventBus,
	embedder knowledge.Embedder,
	lg loggateway.Logger,
) biz.AgentFactory {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &AgentFactoryImpl{
		llm:          llm,
		agentWriter:  agentWriter,
		agentReader:  agentReader,
		templateRepo: templateRepo,
		eventBus:     eventBus,
		embedder:     embedder,
		lg:           lg.With(loggateway.Domain("agent_factory")),
	}
}

// SetOrganizationReader attaches org lookup so newly created agents occupy a
// position under TaskProfile.DepartmentID (M78 ORGFAST-12). Nil is allowed.
func (f *AgentFactoryImpl) SetOrganizationReader(org biz.OrganizationReader) {
	if f == nil {
		return
	}
	f.org = org
}

// EnsureAgent returns an agent_key suitable for the given TaskProfile.
// If a matching Agent already exists (deterministic AgentKey), it is reused.
// Otherwise a new Agent is generated via LLM, persisted with Source="system",
// and an EnvelopeTypeAgentCreated event is published.
//
// NOTE: CreateAgent + publishAgentCreated 不在事务中。顺序为先持久化后发布（符合
// AS-EVT-01）。若进程在 publish 前崩溃，事件丢失但 Agent 已落库。这是可接受的降级：
// agent_created 事件为 Informational 级别，丢失不影响业务正确性，前端通过 Agent
// 列表轮询/刷新可发现新 Agent。
func (f *AgentFactoryImpl) EnsureAgent(ctx context.Context, profile biz.TaskProfile) (string, error) {
	if strings.TrimSpace(profile.TaskDescription) == "" {
		return "", apierror.BadRequest(apierror.DomainAgent, "task description is required")
	}

	agentKey := f.buildDynamicAgentKey(profile)

	// Idempotency: reuse existing agent with the same deterministic key.
	if existing, err := f.agentReader.GetAgentByAgentKey(ctx, agentKey); err == nil {
		f.lg.Debug("AgentFactory 命中已有 Agent",
			loggateway.StepID("agent_factory.reuse"),
			loggateway.AgentKey(agentKey),
		)
		return existing.AgentKey, nil
	} else if !stderrors.Is(err, shared.ErrNotFound) {
		return "", apierror.Internal(apierror.DomainAgent, "check existing agent").WithCause(err)
	}

	// 同域使命相似复用（key 未命中时的二级检查，需 embedder）。
	if reused, ok := f.findSameDomainAgent(ctx, profile); ok {
		f.lg.Info("AgentFactory 同域使命相似，复用已有 Agent",
			loggateway.StepID("agent_factory.domain_reuse"),
			loggateway.AgentKey(reused),
			loggateway.Str("domain_path", profile.DomainPath),
		)
		return reused, nil
	}

	if f.llm == nil {
		return "", apierror.Internal(apierror.DomainAgent, "AgentFactory LLM not configured")
	}

	// P-ORCH: publish creating_agent progress before the LLM call so the user
	// sees fine-grained feedback during the generation window. The display
	// name is not yet known, so the task description stands in as the label.
	f.publishOrchestrationProgress(ctx, profile, "creating_agent", map[string]any{
		"agent_name": truncateText(profile.TaskDescription, 30),
	})

	def, err := f.generateAgentDefinition(ctx, profile)
	if err != nil {
		return "", err
	}

	// P-ORCH: user confirmation gate — reuses the tool_confirmation context
	// pattern (ReplyFunc + ActivityEmitter from ctx). A nil ReplyFunc means
	// the caller cannot confirm (CLI/tests/non-chat contexts) → legacy
	// direct creation is preserved.
	if err := f.confirmAgentCreation(ctx, profile, def); err != nil {
		return "", err
	}

	agent := biz.Agent{
		AgentKey:         agentKey,
		DisplayName:      def.DisplayName,
		AgentDescription: def.Description,
		Provider:         def.Provider,
		Model:            def.Model,
		Source:           agentFactorySource,
		CreatedBy:        agentFactoryAuthor,
		Status:           string(biz.AgentStatusActive),
		Roles:            profile.RequiredCapabilities,
		MissionStatement: def.MissionStatement,
		DomainPath:       def.DomainPath,
	}
	agent.PositionID, agent.PositionKey = f.resolveBirthPosition(ctx, profile)

	created, err := f.agentWriter.CreateAgent(ctx, agent)
	if err != nil {
		// Race condition: another goroutine created the same agent. Reuse it.
		if existing, getErr := f.agentReader.GetAgentByAgentKey(ctx, agentKey); getErr == nil {
			return existing.AgentKey, nil
		}
		return "", apierror.Internal(apierror.DomainAgent, "persist generated agent").WithCause(err)
	}

	f.publishAgentCreated(ctx, created, profile.TaskDescription)
	f.publishOrchestrationProgress(ctx, profile, "agent_created", map[string]any{
		"agent_name": created.DisplayName,
		"agent_key":  created.AgentKey,
	})
	metrics.AgentFactoryCreated.Inc()

	f.lg.Info("AgentFactory 创建新 Agent",
		loggateway.StepID("agent_factory.created"),
		loggateway.AgentKey(agentKey),
		loggateway.Str("display_name", created.DisplayName),
	)

	return created.AgentKey, nil
}

// generateAgentDefinition calls the LLM to generate an agent definition.
func (f *AgentFactoryImpl) generateAgentDefinition(ctx context.Context, profile biz.TaskProfile) (GeneratedAgentDefinition, error) {
	template, tmplErr := f.selectClosestTemplate(ctx, profile)
	if tmplErr != nil {
		f.lg.Warn("AgentFactory 模板查询失败，使用空模板",
			loggateway.StepID("agent_factory.template_query"),
			loggateway.Err(tmplErr),
		)
	}
	prompt := f.buildAgentFactoryPrompt(profile, template)

	callCtx, cancel := context.WithTimeout(ctx, agentFactoryLLMTimeout)
	defer cancel()

	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: agentFactorySystemPrompt()},
		{Role: trpcmodel.RoleUser, Content: prompt},
	})

	respChan, err := f.llm.GenerateContent(callCtx, req)
	if err != nil {
		return GeneratedAgentDefinition{}, apierror.Internal(apierror.DomainAgent, "LLM generate content").WithCause(err)
	}

	text, err := f.consumeModelResponse(respChan)
	if err != nil {
		return GeneratedAgentDefinition{}, err
	}

	return f.parseAgentDefinition(text, profile, template)
}

// consumeModelResponse drains the response channel and concatenates content.
func (f *AgentFactoryImpl) consumeModelResponse(respChan <-chan *trpcmodel.Response) (string, error) {
	var sb strings.Builder
	for resp := range respChan {
		if resp == nil {
			continue
		}
		if resp.Error != nil {
			return "", apierror.Internal(apierror.DomainAgent, "LLM response error").WithCause(resp.Error)
		}
		for _, choice := range resp.Choices {
			if choice.Delta.Content != "" {
				sb.WriteString(choice.Delta.Content)
			}
			if choice.Message.Content != "" {
				sb.WriteString(choice.Message.Content)
			}
		}
	}
	return sb.String(), nil
}

// parseAgentDefinition parses the LLM response as JSON, with fallbacks.
func (f *AgentFactoryImpl) parseAgentDefinition(text string, profile biz.TaskProfile, template biz.AgentTemplate) (GeneratedAgentDefinition, error) {
	text = stripDecompositionFences(text)
	var def GeneratedAgentDefinition
	if err := json.Unmarshal([]byte(text), &def); err != nil {
		f.lg.Warn("AgentFactory LLM 返回非法 JSON，使用降级定义",
			loggateway.StepID("agent_factory.parse_fallback"),
			loggateway.Err(err),
		)
		return f.defaultDefinition(profile, template), nil
	}
	if def.DisplayName == "" {
		def.DisplayName = defaultDisplayName(profile)
	}
	if def.Description == "" {
		def.Description = defaultDescription(profile)
	}
	if def.Provider == "" {
		def.Provider = template.Provider
	}
	if def.Model == "" {
		def.Model = template.Model
	}
	if def.MissionStatement == "" {
		def.MissionStatement = def.Description
	}
	def.DomainPath = NormalizeDomainPath(def.DomainPath)
	if def.DomainPath == "" {
		// LLM 未输出 domain_path 时回退 profile 域的一级域（出生登记兜底）。
		def.DomainPath = TopLevelDomain(profile.DomainPath)
	}
	return def, nil
}

// selectClosestTemplate returns the template with the highest keyword overlap.
func (f *AgentFactoryImpl) selectClosestTemplate(ctx context.Context, profile biz.TaskProfile) (biz.AgentTemplate, error) {
	if f.templateRepo == nil {
		return biz.AgentTemplate{}, nil
	}
	templates, err := f.templateRepo.ListAgentTemplates(ctx)
	if err != nil || len(templates) == 0 {
		return biz.AgentTemplate{}, err
	}

	taskText := strings.ToLower(profile.Domain + " " + profile.TaskDescription + " " + strings.Join(profile.RequiredCapabilities, " "))
	best := templates[0]
	bestScore := -1.0
	for _, t := range templates {
		templateText := strings.ToLower(t.Label + " " + t.DisplayName + " " + t.Description)
		score := keywordOverlapScore(taskText, templateText)
		if score > bestScore {
			bestScore = score
			best = t
		}
	}
	return best, nil
}

// buildDynamicAgentKey returns a deterministic AgentKey for the profile.
// DomainPath 非空时按 domain+model 派生（同域同模型复用同一 Agent，B.10.21.6）；
// 为空时保留旧行为（任务文本参与哈希，兼容兜底路径）。
func (f *AgentFactoryImpl) buildDynamicAgentKey(profile biz.TaskProfile) string {
	h := sha1.New()
	if dp := NormalizeDomainPath(profile.DomainPath); dp != "" {
		fmt.Fprint(h, dp, "|", profile.PreferredModel)
	} else {
		fmt.Fprint(h, profile.Domain, "|", profile.TaskDescription, "|")
		fmt.Fprint(h, strings.Join(profile.RequiredCapabilities, ","), "|")
		fmt.Fprint(h, strings.Join(profile.PreferredTools, ","), "|", profile.PreferredModel)
	}
	sum := hex.EncodeToString(h.Sum(nil))[:12]
	return "factory-" + sum
}

// publishAgentCreated publishes the agent_created event as a v2 SystemNoticeEvent
// (replaces the legacy system-domain ActivityEvent; NOT persisted, WS-only broadcast).
func (f *AgentFactoryImpl) publishAgentCreated(ctx context.Context, agent biz.Agent, trigger string) {
	if f.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type":   "agent_created",
		"agent_key":    agent.AgentKey,
		"display_name": agent.DisplayName,
		"source":       agent.Source,
		"trigger":      trigger,
	}
	f.eventBus.Publish(ctx, biz.NewSystemNoticeEvent("", "agent_created", "Agent dynamically created by AgentFactory", meta))
}

// confirmAgentCreation asks the user to approve the LLM-generated agent
// proposal before persistence (P-ORCH). It reuses the tool_confirmation
// context pattern: serviceawaitreply.ReplyFunc blocks until the user responds
// via the ConfirmActivity RPC, and biz.ActivityEmitter renders the confirm
// card in the activity timeline. A nil ReplyFunc (CLI/tests/non-chat ctx)
// skips the gate and preserves the legacy direct-creation behavior.
func (f *AgentFactoryImpl) confirmAgentCreation(ctx context.Context, profile biz.TaskProfile, def GeneratedAgentDefinition) error {
	fn := serviceawaitreply.ReplyFuncFromContext(ctx)
	if fn == nil {
		return nil
	}

	proposal := map[string]any{
		"display_name":      def.DisplayName,
		"description":       def.Description,
		"provider":          def.Provider,
		"model":             def.Model,
		"capabilities":      profile.RequiredCapabilities,
		"domain":            profile.Domain,
		"task":              profile.TaskDescription,
		"mission_statement": def.MissionStatement,
		"domain_path":       def.DomainPath,
	}
	proposalJSON, err := json.Marshal(proposal)
	if err != nil {
		return apierror.Internal(apierror.DomainAgent, "marshal agent proposal").WithCause(err)
	}

	emitter := biz.ActivityEmitterFromContext(ctx)
	var confirmActivityID string
	if emitter != nil {
		content := fmt.Sprintf("未匹配到合适的 Agent，需要创建新 Agent「%s」（%s / %s）", def.DisplayName, def.Provider, def.Model)
		id, emitErr := emitter.EmitConfirmRequest(ctx, biz.ActivityConfirmParams{
			ToolName:      "agent_factory",
			ToolArguments: string(proposalJSON),
			Content:       content,
		})
		if emitErr != nil {
			f.lg.Warn("AgentFactory EmitConfirmRequest failed",
				loggateway.StepID("agent_factory.confirm"),
				loggateway.Err(emitErr))
		} else {
			confirmActivityID = id
		}
	}

	confirmCtx, cancel := context.WithTimeout(ctx, defaultToolConfirmationTimeout)
	defer cancel()
	reply, err := fn(confirmCtx)
	approved := err == nil && toolConfirmApproved(reply)

	if emitter != nil && confirmActivityID != "" {
		// A deadline expiry is NOT a user rejection — emit the timeout variant
		// so the UI renders "已超时" instead of "已拒绝".
		if err != nil && confirmCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			if emitErr := emitter.EmitConfirmTimeout(ctx, confirmActivityID); emitErr != nil {
				f.lg.Warn("AgentFactory EmitConfirmTimeout failed",
					loggateway.StepID("agent_factory.confirm"),
					loggateway.Err(emitErr))
			}
		} else if emitErr := emitter.EmitConfirmResult(ctx, confirmActivityID, approved); emitErr != nil {
			f.lg.Warn("AgentFactory EmitConfirmResult failed",
				loggateway.StepID("agent_factory.confirm"),
				loggateway.Err(emitErr))
		}
	}

	if err != nil {
		return apierror.Internal(apierror.DomainAgent, "await agent creation confirmation").WithCause(err)
	}
	if !approved {
		f.lg.Info("AgentFactory 用户拒绝创建 Agent",
			loggateway.StepID("agent_factory.confirm"),
			loggateway.Str("display_name", def.DisplayName),
		)
		return apierror.Forbidden(apierror.DomainAgent, "agent creation rejected by user")
	}
	return nil
}

// publishOrchestrationProgress emits an orchestration_progress SystemNoticeEvent
// (WS-only, not persisted) so the frontend can render fine-grained loading
// feedback during agent creation. Nil bus or empty session → skipped.
func (f *AgentFactoryImpl) publishOrchestrationProgress(ctx context.Context, profile biz.TaskProfile, phase string, extra map[string]any) {
	if f.eventBus == nil || profile.SpiritSessionID == "" {
		return
	}
	meta := map[string]any{"phase": phase}
	for k, v := range extra {
		meta[k] = v
	}
	f.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(profile.SpiritSessionID, "orchestration_progress", "orchestration progress: "+phase, meta))
}

// buildAgentFactoryPrompt builds the user prompt for LLM generation.
func (f *AgentFactoryImpl) buildAgentFactoryPrompt(profile biz.TaskProfile, template biz.AgentTemplate) string {
	var sb strings.Builder
	sb.WriteString("Generate an agent definition for the following task:\n\n")
	sb.WriteString("Domain: " + profile.Domain + "\n")
	sb.WriteString("Task: " + profile.TaskDescription + "\n")
	sb.WriteString("Required Capabilities: " + strings.Join(profile.RequiredCapabilities, ", ") + "\n")
	if len(profile.PreferredTools) > 0 {
		sb.WriteString("Preferred Tools: " + strings.Join(profile.PreferredTools, ", ") + "\n")
	}
	if profile.PreferredModel != "" {
		sb.WriteString("Preferred Model: " + profile.PreferredModel + "\n")
	}
	if template.Key != "" {
		sb.WriteString("\nClosest template for reference:\n")
		sb.WriteString("  Label: " + template.Label + "\n")
		sb.WriteString("  Description: " + template.Description + "\n")
		sb.WriteString("  Provider: " + template.Provider + "\n")
		sb.WriteString("  Model: " + template.Model + "\n")
	}
	sb.WriteString("\nOutput ONLY a JSON object with fields: display_name, description, provider, model, system_prompt, mission_statement, domain_path\n")
	sb.WriteString("domain_path must classify the agent into this domain lexicon (most specific entry; fallback top-level or \"其他\"): " + DomainLexiconPromptList() + "\n")
	return sb.String()
}

// defaultDefinition returns a fallback definition when LLM output is unparseable.
func (f *AgentFactoryImpl) defaultDefinition(profile biz.TaskProfile, template biz.AgentTemplate) GeneratedAgentDefinition {
	prov := template.Provider
	mod := template.Model
	if prov == "" {
		prov = "openrouter"
	}
	if mod == "" {
		mod = "gpt-4.1-mini"
	}
	return GeneratedAgentDefinition{
		DisplayName:  defaultDisplayName(profile),
		Description:  defaultDescription(profile),
		Provider:     prov,
		Model:        mod,
		SystemPrompt: "",
	}
}

// agentFactorySystemPrompt returns the system prompt for LLM generation.
func agentFactorySystemPrompt() string {
	return `You are an agent definition generator. Given a task profile, generate a concise agent definition as JSON.

Rules:
- Output ONLY a JSON object, no markdown fences, no explanation
- display_name: short, human-readable name (max 30 chars)
- description: one-sentence description of what the agent does
- provider: LLM provider (e.g. "openrouter")
- model: model identifier (e.g. "gpt-4.1-mini")
- system_prompt: the agent's system prompt (max 500 chars)
- mission_statement: one-sentence long-term mission of the agent (its enduring identity across tasks, e.g. "擅长中文诗歌与散文创作的文学写手")
- domain_path: domain classification from the provided lexicon

The agent should be specialized for the given task domain and capabilities.`
}

// defaultDisplayName returns a fallback display name from the profile.
func defaultDisplayName(profile biz.TaskProfile) string {
	if profile.Domain != "" {
		return profile.Domain + " 助手"
	}
	return "动态 Agent"
}

// defaultDescription returns a fallback description from the profile.
func defaultDescription(profile biz.TaskProfile) string {
	if profile.TaskDescription != "" {
		return "为任务动态创建的 Agent: " + profile.TaskDescription
	}
	return "AgentFactory 动态创建的 Agent"
}

// findSameDomainAgent 在同域 Agent 中查找使命高相似（cosine ≥ 0.85）者复用。
// DomainPath 为空或 embedder 为 nil 时跳过（仅 key 命中复用，不变量 3）。
func (f *AgentFactoryImpl) findSameDomainAgent(ctx context.Context, profile biz.TaskProfile) (string, bool) {
	dp := NormalizeDomainPath(profile.DomainPath)
	if dp == "" || f.embedder == nil {
		return "", false
	}
	res, err := f.agentReader.SearchAgents(ctx, biz.AgentListQuery{Status: "active", Limit: sameDomainReuseScanLimit})
	if err != nil || len(res.Items) == 0 {
		return "", false
	}
	cands := make([]biz.Agent, 0, len(res.Items))
	for _, ag := range res.Items {
		if biz.IsSystemAgentKey(ag.AgentKey) {
			continue
		}
		if DomainPathRelated(ag.DomainPath, dp) {
			cands = append(cands, ag)
		}
	}
	if len(cands) == 0 {
		return "", false
	}
	query := profile.Mission
	if query == "" {
		query = profile.TaskDescription
	}
	texts := make([]string, 0, len(cands)+1)
	texts = append(texts, query)
	for _, ag := range cands {
		m := ag.MissionStatement
		if m == "" {
			m = ag.AgentDescription
		}
		texts = append(texts, m)
	}
	vectors, err := f.embedder.Embed(ctx, texts)
	if err != nil || len(vectors) != len(texts) || len(vectors[0]) == 0 {
		if err != nil {
			f.lg.Warn("AgentFactory 同域复用 embedding 失败，跳过",
				loggateway.StepID("agent_factory.domain_reuse"),
				loggateway.Err(err),
			)
		}
		return "", false
	}
	bestIdx, bestSim := -1, 0.0
	for i := range cands {
		if sim := cosineSimilarity32(vectors[0], vectors[i+1]); sim > bestSim {
			bestSim, bestIdx = sim, i
		}
	}
	if bestIdx < 0 || bestSim < sameDomainReuseMinSim {
		return "", false
	}
	return cands[bestIdx].AgentKey, true
}

// keywordOverlapScore computes the fraction of tokens in `a` that also appear in `b`.
func keywordOverlapScore(a, b string) float64 {
	aTokens := tokenizeForSemantic(a)
	bTokens := tokenizeForSemantic(b)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0
	}
	bSet := make(map[string]bool, len(bTokens))
	for _, t := range bTokens {
		bSet[t] = true
	}
	overlap := 0
	for _, t := range aTokens {
		if bSet[t] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(aTokens))
}

// resolveBirthPosition picks an existing position under DepartmentID.
// Factory never creates company/department/position nodes (ORGFAST-05/12).
func (f *AgentFactoryImpl) resolveBirthPosition(ctx context.Context, profile biz.TaskProfile) (positionID, positionKey string) {
	if id := strings.TrimSpace(profile.PositionID); id != "" {
		return id, ""
	}
	deptID := strings.TrimSpace(profile.DepartmentID)
	if deptID == "" || f.org == nil {
		return "", ""
	}
	children, err := f.org.ListOrgNodesByParentID(ctx, deptID)
	if err != nil {
		f.lg.Warn("AgentFactory 查询部门岗位失败，新 Agent 暂不占岗",
			loggateway.StepID("agent_factory.position"),
			loggateway.Str("department_id", deptID),
			loggateway.Err(err),
		)
		return "", ""
	}
	var fallback biz.OrganizationNode
	for _, n := range children {
		if n.Level != "position" || strings.TrimSpace(n.DeletedAt) != "" {
			continue
		}
		if fallback.ID == "" {
			fallback = n
		}
		label := strings.ToLower(n.Name + " " + n.Key)
		if strings.Contains(label, "其他") || strings.Contains(label, "other") || strings.Contains(label, "general") {
			return n.ID, n.Key
		}
	}
	return fallback.ID, fallback.Key
}
