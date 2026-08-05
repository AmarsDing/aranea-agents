// Package memory — ProfileCardDistiller (FR-12.7 / report §6.4).
//
// The distiller maintains the resident profile card: one compact card per
// (agent, user) distilled from active profile/preference/goal/constraint
// facts during Sleep-time. The card is injected into the prompt
// unconditionally (100% inject rate, no recall scoring) — it is the shortest
// path for the user to feel that memory exists (Letta core memory pattern).
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const (
	// profileCardFactLimit caps the source facts read per distillation pass.
	profileCardFactLimit = 50
	// profileCardLLMTimeout caps the distillation LLM call duration.
	profileCardLLMTimeout = 45 * time.Second
	// profileCardMaxRunes is the card content budget handed to the LLM. The
	// inject hook applies the same hard cap (profileCardMaxRunes there).
	profileCardMaxRunes = 1000
	// profileCardFactMaxRunes caps one source fact line in the prompt.
	profileCardFactMaxRunes = 200
)

// profileCardFactKinds are the fact kinds distilled into the resident card.
var profileCardFactKinds = []string{"profile", "preference", "goal", "constraint"}

// ProfileCardDistiller distills active profile-class facts into the resident
// profile card (Sleep-time Phase 3). All operations are best-effort with
// graceful degradation: LLM failure → keep the old card; zero source facts →
// delete the stale card so it never outlives its facts.
type ProfileCardDistiller struct {
	factLister  biz.MemoryPreferenceLister
	cardWriter  biz.MemoryProfileCardWriter
	llm         trpcmodel.Model
	llmResolver LLMResolver // per-target ModelCatalog resolution (takes precedence over llm)
	lg          loggateway.Logger
}

// NewProfileCardDistiller creates a ProfileCardDistiller. Returns nil when
// the required fact lister or card writer is missing.
func NewProfileCardDistiller(
	factLister biz.MemoryPreferenceLister,
	cardWriter biz.MemoryProfileCardWriter,
	llm trpcmodel.Model,
	lg loggateway.Logger,
) *ProfileCardDistiller {
	if factLister == nil || cardWriter == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ProfileCardDistiller{
		factLister: factLister,
		cardWriter: cardWriter,
		llm:        llm,
		lg:         lg.With(loggateway.Domain("profile_card_distiller")),
	}
}

// SetLLMResolver wires a per-target LLM resolver (same contract as
// SleepTimeService.SetLLMResolver). Must be called before use; not safe to
// call concurrently with active distillation.
func (d *ProfileCardDistiller) SetLLMResolver(r LLMResolver) {
	if d == nil {
		return
	}
	d.llmResolver = r
}

// Distill runs one distillation pass for the (agent, user) target. Never
// returns an error: read/LLM/write failures are logged and swallowed so the
// Sleep-time JobRunner retry decision is driven only by Phase 1 (retrying
// the whole job for a Phase 3 failure could duplicate Phase 1/2 mutations).
func (d *ProfileCardDistiller) Distill(ctx context.Context, uk trpcmemory.UserKey) {
	if d == nil {
		return
	}
	agentID := strings.TrimSpace(uk.AppName)
	userID := strings.TrimSpace(uk.UserID)
	if agentID == "" {
		return
	}

	// 1. Read active profile-class facts (user ∪ agent scope, importance-ordered).
	rows, err := d.factLister.ListActivePreferenceFacts(ctx, agentID, userID, profileCardFactKinds, profileCardFactLimit)
	if err != nil {
		d.lg.Warn("profile card: source fact read failed, keeping old card",
			loggateway.Str("agent", agentID),
			loggateway.Str("user", userID),
			loggateway.Err(err))
		return
	}

	// 2. Zero source facts → delete the stale card (a card must never
	//    outlive its facts, e.g. after governance deletes all preferences).
	facts := parseProfileCardFacts(rows)
	if len(facts) == 0 {
		if err := d.cardWriter.DeleteProfileCard(ctx, agentID, userID); err != nil {
			d.lg.Warn("profile card: stale card delete failed",
				loggateway.Str("agent", agentID),
				loggateway.Str("user", userID),
				loggateway.Err(err))
		}
		return
	}

	// 3. LLM distillation. Nil LLM → keep the old card (graceful).
	llm := resolveLLM(ctx, d.llmResolver, d.llm, uk)
	if llm == nil {
		return
	}
	content, err := d.llmDistill(ctx, llm, facts)
	if err != nil {
		d.lg.Warn("profile card: LLM distillation failed, keeping old card",
			loggateway.Str("agent", agentID),
			loggateway.Str("user", userID),
			loggateway.Err(err))
		return
	}
	if content == "" {
		return
	}

	// 4. Upsert (version bump on conflict).
	if err := d.cardWriter.UpsertProfileCard(ctx, biz.ProfileCard{
		AgentID:   agentID,
		UserID:    userID,
		Content:   content,
		FactCount: len(facts),
	}); err != nil {
		d.lg.Warn("profile card: upsert failed",
			loggateway.Str("agent", agentID),
			loggateway.Str("user", userID),
			loggateway.Err(err))
		return
	}
	d.lg.Info("profile card distilled",
		loggateway.Str("agent", agentID),
		loggateway.Str("user", userID),
		loggateway.Int("facts", len(facts)),
		loggateway.Int("card_runes", len([]rune(content))))
}

// profileCardFact is one parsed source fact for the distillation prompt.
type profileCardFact struct {
	Kind      string
	Statement string
}

// parseProfileCardFacts decodes raw fact rows into prompt items, skipping
// malformed/empty rows.
func parseProfileCardFacts(rows [][]byte) []profileCardFact {
	out := make([]profileCardFact, 0, len(rows))
	for _, raw := range rows {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		stmt, _ := m["statement"].(string)
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if runes := []rune(stmt); len(runes) > profileCardFactMaxRunes {
			stmt = string(runes[:profileCardFactMaxRunes]) + "…"
		}
		kind, _ := m["fact_kind"].(string)
		out = append(out, profileCardFact{Kind: strings.TrimSpace(kind), Statement: stmt})
	}
	return out
}

// llmDistill calls the LLM to compress the source facts into one compact
// card. Returns the sanitized card content ("" when the LLM produced
// nothing usable).
func (d *ProfileCardDistiller) llmDistill(ctx context.Context, llm trpcmodel.Model, facts []profileCardFact) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, profileCardLLMTimeout)
	defer cancel()

	var b strings.Builder
	for i, f := range facts {
		kind := f.Kind
		if kind == "" {
			kind = "profile"
		}
		fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, kind, f.Statement)
	}
	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: profileCardSystemPrompt},
		{Role: trpcmodel.RoleUser, Content: fmt.Sprintf("源事实（共 %d 条）：\n%s", len(facts), b.String())},
	})

	respCh, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return "", err
	}
	var content string
	for resp := range respCh {
		if resp == nil {
			continue
		}
		if resp.Error != nil {
			return "", fmt.Errorf("LLM API error: %s", resp.Error.Message)
		}
		for _, choice := range resp.Choices {
			content += choice.Message.Content
		}
	}
	return sanitizeProfileCardContent(content), nil
}

// sanitizeProfileCardContent normalizes the LLM output: strips markdown
// fences/headers (the inject hook adds its own block header), trims blanks,
// and enforces the rune budget.
func sanitizeProfileCardContent(raw string) string {
	content := strings.TrimSpace(raw)
	// Strip a wrapping code fence if the LLM emitted one.
	if strings.HasPrefix(content, "```") {
		if idx := strings.Index(content, "\n"); idx >= 0 {
			content = content[idx+1:]
		}
		content = strings.TrimSuffix(strings.TrimSpace(content), "```")
		content = strings.TrimSpace(content)
	}
	// Drop leading markdown headers — the hook renders its own block title.
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "#") {
			continue
		}
		kept = append(kept, ln)
	}
	content = strings.TrimSpace(strings.Join(kept, "\n"))
	if runes := []rune(content); len(runes) > profileCardMaxRunes {
		content = string(runes[:profileCardMaxRunes]) + "…"
	}
	return content
}

const profileCardSystemPrompt = `你是用户档案蒸馏器。把给定的源事实压缩成一张紧凑的「用户档案卡」，供对话系统在每次对话开头常驻注入。

要求：
1. 直接输出档案卡正文（纯文本/Markdown 列表），不要输出标题、代码围栏或解释。
2. 按语义分组合并同类事实（如：基本画像 / 偏好 / 目标 / 约束），重复或冲突的事实以更新、更具体的为准。
3. 每条一行，以 "- " 开头；总长度控制在 800 字以内。
4. 只保留对长期对话有价值的信息；丢弃一次性事件、过时状态。
5. 不要编造源事实之外的信息。`
