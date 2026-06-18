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
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	agentFactorySource     = "system"
	agentFactoryAuthor     = "agent-factory"
	agentFactoryLLMTimeout = 60 * time.Second
)

// GeneratedAgentDefinition is the LLM-generated agent definition parsed from
// the model response. Field names match the JSON schema in the prompt.
type GeneratedAgentDefinition struct {
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
}

// AgentFactoryImpl implements biz.AgentFactory by dynamically generating
// Agents via LLM when 4-layer matching fails.
type AgentFactoryImpl struct {
	llm          trpcmodel.Model
	agentWriter  biz.AgentWriter
	agentReader  biz.AgentReader
	templateRepo biz.AgentTemplateRepo
	bus          contract.Bus
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
	bus contract.Bus,
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
		bus:          bus,
		lg:           lg.With(loggateway.Domain("agent_factory")),
	}
}

// EnsureAgent returns an agent_key suitable for the given TaskProfile.
// If a matching Agent already exists (deterministic AgentKey), it is reused.
// Otherwise a new Agent is generated via LLM, persisted with Source="system",
// and an EnvelopeTypeAgentCreated event is published.
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

	if f.llm == nil {
		return "", apierror.Internal(apierror.DomainAgent, "AgentFactory LLM not configured")
	}

	def, err := f.generateAgentDefinition(ctx, profile)
	if err != nil {
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
	}

	created, err := f.agentWriter.CreateAgent(ctx, agent)
	if err != nil {
		// Race condition: another goroutine created the same agent. Reuse it.
		if existing, getErr := f.agentReader.GetAgentByAgentKey(ctx, agentKey); getErr == nil {
			return existing.AgentKey, nil
		}
		return "", apierror.Internal(apierror.DomainAgent, "persist generated agent").WithCause(err)
	}

	f.publishAgentCreated(ctx, created, profile.TaskDescription)
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
// Same profile inputs always produce the same key, ensuring idempotency.
func (f *AgentFactoryImpl) buildDynamicAgentKey(profile biz.TaskProfile) string {
	h := sha1.New()
	fmt.Fprint(h, profile.Domain, "|", profile.TaskDescription, "|")
	fmt.Fprint(h, strings.Join(profile.RequiredCapabilities, ","), "|")
	fmt.Fprint(h, strings.Join(profile.PreferredTools, ","), "|", profile.PreferredModel)
	sum := hex.EncodeToString(h.Sum(nil))[:12]
	return "factory-" + sum
}

// publishAgentCreated publishes the agent_created event (P1-4).
func (f *AgentFactoryImpl) publishAgentCreated(ctx context.Context, agent biz.Agent, trigger string) {
	if f.bus == nil {
		return
	}
	env := contract.NewEnvelope(contract.EnvelopeTypeAgentCreated, agentFactoryAuthor, "")
	env.Metadata = map[string]any{
		"agent_key":    agent.AgentKey,
		"display_name": agent.DisplayName,
		"source":       agent.Source,
		"trigger":      trigger,
	}
	f.bus.Publish(ctx, env)
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
	sb.WriteString("\nOutput ONLY a JSON object with fields: display_name, description, provider, model, system_prompt\n")
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
