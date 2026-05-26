package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	"go.opentelemetry.io/otel/trace"

	"github.com/google/uuid"
)

const l0SnapshotByCallStateKey = "aranea:l0_snapshot_by_call"

type l0SnapshotPending struct {
	ID     string
	Window int
}

func l0SnapshotForceDebug() bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_L0_SNAPSHOT"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "always") || strings.EqualFold(v, "force")
}

func l0SnapshotHookActive(ag biz.Agent) bool {
	if l0SnapshotForceDebug() {
		return true
	}
	if ag.Settings == nil || !ag.Settings.EvolutionMetricsEnabled {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(ag.Settings.L0SnapshotMode))
	return mode != "off"
}

func newL0SnapshotAfterModelHook(deps TRPCBuilderDeps) callbacks.Callback {
	if deps.MemoryAdmin == nil {
		return nil
	}
	return callbacks.NewAfterModelHook(10, func(ctx context.Context, args *trpcmodel.AfterModelArgs) (*trpcmodel.AfterModelResult, error) {
		inv, ok := trpcagent.InvocationFromContext(ctx)
		if !ok || inv == nil {
			return &trpcmodel.AfterModelResult{Context: ctx}, nil
		}
		callIndex := promptSnapshotCurrentCallIndex(ctx)
		pending, ok := l0SnapshotPendingForCall(inv, callIndex)
		if !ok {
			return &trpcmodel.AfterModelResult{Context: ctx}, nil
		}
		actual := 0
		if args != nil && args.Response != nil && args.Response.Usage != nil {
			actual = args.Response.Usage.PromptTokens
		}
		if err := deps.MemoryAdmin.UpdateL0SnapshotActual(ctx, pending.ID, actual, pending.Window); err != nil {
			event.CtxFlowLogWarn(ctx, "memory.l0.snapshot", "L0 快照 actual 更新失败",
				event.P("snapshot_id", pending.ID),
				event.P("model_call_index", callIndex),
				event.P("error", err.Error()),
			)
		}
		return &trpcmodel.AfterModelResult{Context: ctx}, nil
	})
}

func persistL0AssemblySnapshot(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, messages []trpcmodel.Message, callIndex int) {
	if deps.MemoryAdmin == nil || !l0SnapshotHookActive(ag) {
		return
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return
	}
	sessionID := strings.TrimSpace(inv.Session.ID)
	if sessionID == "" {
		return
	}

	report := analyzePromptRequest(messages)
	prov := strutil.FirstNonEmpty(deps.Provider, ag.Provider)
	mod := strutil.FirstNonEmpty(deps.Model, ag.Model)
	force := l0SnapshotForceDebug()

	gateWin := l0GateContextWindow(ag)
	gateRatio := l0UsedRatio(report.EstTokens, gateWin)
	if !biz.ShouldWriteL0AssemblySnapshot(ag.Settings, gateRatio, force) && !l0NeedsWindowRecheck(ag.Settings, gateRatio, force, gateWin) {
		return
	}

	win := resolveL0ContextWindow(ctx, deps, ag, prov, mod)
	usedRatio := l0UsedRatio(report.EstTokens, win)
	if !biz.ShouldWriteL0AssemblySnapshot(ag.Settings, usedRatio, force) {
		return
	}

	l1Fields, l1Tok := memorySectionStats(messages, "## L1 working memory")
	l3Chunks, l3Tok := memorySectionStats(messages, "## L3 semantic memory", "## L2+L3 memory")
	l4Paths, l4Tok := memorySectionStats(messages, "## L4 knowledge graph")
	summaryTok := report.Sections["session_summary"]

	settings := ag.Settings
	recentTurns := 0
	recentTok := 0
	truncateStrategy := ""
	if settings != nil {
		recentTurns = settings.L0RecentWindowTurns
		recentTok = settings.L0RecentWindowTokens
		truncateStrategy = settings.L0TruncateStrategy
	}
	budget := win
	if win > 0 {
		budget = int(float64(win) * 0.85)
	}

	rt := memoryRuntimeContext(inv, ag)
	metaJSON := l0SnapshotMetadataJSON(callIndex, inv.InvocationID)
	segsJSON := buildL0SegmentsJSON(messages)

	in := biz.L0AssemblySnapshotInsert{
		ID:                   uuid.NewString(),
		SessionID:            sessionID,
		RunID:                l0CorrelationRunID(ctx),
		TurnID:               l0CorrelationTurnID(ctx),
		SpanID:               l0SpanID(ctx),
		AgentID:              strings.TrimSpace(ag.ID),
		TeamID:               rt.TeamID,
		Provider:             prov,
		Model:                mod,
		ContextWindowTokens:  win,
		BudgetTokens:         budget,
		RecentWindowTurns:    recentTurns,
		RecentWindowTokens:   recentTok,
		SummaryTokenEstimate: summaryTok,
		L1FieldCount:         l1Fields,
		L1TokenEstimate:      l1Tok,
		L3ChunkCount:         l3Chunks,
		L3TokenEstimate:      l3Tok,
		L4PathCount:          l4Paths,
		L4TokenEstimate:      l4Tok,
		PromptTokenEstimate:  report.EstTokens,
		UsedRatio:            usedRatio,
		TruncateStrategy:     truncateStrategy,
		SegmentsJSON:         segsJSON,
		WarningCodesJSON:     biz.L0WarningCodesJSON(biz.L0WarningCodesFromRatio(usedRatio)),
		MetadataJSON:         metaJSON,
		CreatedAt:            time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := deps.MemoryAdmin.InsertL0AssemblySnapshot(ctx, in); err != nil {
		event.CtxFlowLogWarn(ctx, "memory.l0.snapshot", "L0 快照落库失败",
			event.P("session_id", sessionID),
			event.P("error", err.Error()),
		)
		return
	}
	setL0SnapshotPendingForCall(inv, callIndex, in.ID, win)
	event.CtxFlowLogDone(ctx, "memory.l0.snapshot", "L0 快照已落库",
		event.P("snapshot_id", in.ID),
		event.P("session_id", sessionID),
		event.P("model_call_index", callIndex),
		event.P("est_tokens", report.EstTokens),
		event.P("used_ratio", usedRatio),
		event.P("context_window", win),
	)
}

func l0GateContextWindow(ag biz.Agent) int {
	if ag.ContextWindow > 0 {
		return ag.ContextWindow
	}
	return llmcontext.DefaultWindowTokens
}

func l0UsedRatio(estTokens, window int) float64 {
	if window <= 0 || estTokens <= 0 {
		return 0
	}
	return float64(estTokens) / float64(window)
}

// l0NeedsWindowRecheck re-resolves window when the coarse agent window may hide a near-limit session.
func l0NeedsWindowRecheck(settings *biz.AgentRuntimeSettings, gateRatio float64, force bool, gateWin int) bool {
	if force {
		return false
	}
	if settings == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(settings.L0SnapshotMode))
	if mode == "always" || mode == "off" {
		return false
	}
	if gateWin <= 0 {
		return true
	}
	margin := llmcontext.ContextStatusWarningThreshold - 0.15
	return gateRatio >= margin
}

func setL0SnapshotPendingForCall(inv *trpcagent.Invocation, callIndex int, id string, win int) {
	if inv == nil || callIndex <= 0 || strings.TrimSpace(id) == "" {
		return
	}
	m := l0SnapshotPendingMap(inv)
	m[callIndex] = l0SnapshotPending{ID: id, Window: win}
	inv.SetState(l0SnapshotByCallStateKey, m)
}

func l0SnapshotPendingForCall(inv *trpcagent.Invocation, callIndex int) (l0SnapshotPending, bool) {
	if inv == nil || callIndex <= 0 {
		return l0SnapshotPending{}, false
	}
	m := l0SnapshotPendingMap(inv)
	p, ok := m[callIndex]
	return p, ok && strings.TrimSpace(p.ID) != ""
}

func l0SnapshotPendingMap(inv *trpcagent.Invocation) map[int]l0SnapshotPending {
	if inv == nil {
		return map[int]l0SnapshotPending{}
	}
	if v, ok := inv.GetState(l0SnapshotByCallStateKey); ok {
		if m, ok := v.(map[int]l0SnapshotPending); ok && m != nil {
			return m
		}
	}
	return map[int]l0SnapshotPending{}
}

func resolveL0ContextWindow(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, prov, mod string) int {
	cfgJSON := ""
	if deps.Catalog != nil {
		if row, err := deps.Catalog.GetByProviderAndModel(ctx, prov, mod); err == nil {
			cfgJSON = row.ConfigJSON
		}
	}
	return llmcontext.ResolveWindow(llmcontext.ResolveInput{
		ProviderModelConfigJSON: cfgJSON,
		SessionDefaultWindow:    event.SessionDefaultContextWindowFromContext(ctx),
		AgentWindow:             ag.ContextWindow,
	})
}

func l0CorrelationRunID(ctx context.Context) string {
	if id := event.SessionRunIDFromContext(ctx); id != "" {
		return id
	}
	if tc, ok := event.TraceContextFromContext(ctx); ok {
		return strings.TrimSpace(tc.RunID)
	}
	return ""
}

func l0CorrelationTurnID(ctx context.Context) string {
	if id := event.TurnIDFromContext(ctx); id != "" {
		return id
	}
	if spec, ok := event.DurableResumeFromContext(ctx); ok && spec.TurnID != "" {
		return spec.TurnID
	}
	return ""
}

func l0SpanID(ctx context.Context) string {
	if sc := trace.SpanFromContext(ctx); sc != nil {
		if scCtx := sc.SpanContext(); scCtx.IsValid() {
			return scCtx.SpanID().String()
		}
	}
	return ""
}

func l0SnapshotMetadataJSON(callIndex int, invocationID string) string {
	meta := map[string]any{
		"phase":            biz.L0SnapshotObservePhase,
		"model_call_index": callIndex,
	}
	if id := strings.TrimSpace(invocationID); id != "" {
		meta["invocation_id"] = id
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return `{}`
	}
	return string(b)
}

func memorySectionStats(messages []trpcmodel.Message, markers ...string) (bulletCount, tokenEst int) {
	for _, m := range messages {
		if m.Role != trpcmodel.RoleSystem {
			continue
		}
		text := strings.TrimSpace(m.Content)
		for _, marker := range markers {
			if !strings.Contains(text, marker) {
				continue
			}
			bulletCount += countBulletLines(text, marker)
			tokenEst += sectionTokens(text, marker, l0SectionEndMarker(marker))
		}
	}
	return bulletCount, tokenEst
}

func l0SectionEndMarker(start string) string {
	switch start {
	case "## L1 working memory":
		return "## L2 episodic"
	case "## L3 semantic memory":
		return "## L4 knowledge"
	case "## L2+L3 memory":
		return "## L4 knowledge"
	case "## L4 knowledge graph":
		return ""
	default:
		return ""
	}
}

func countBulletLines(text, marker string) int {
	i := strings.Index(text, marker)
	if i < 0 {
		return 0
	}
	seg := text[i+len(marker):]
	if end := l0SectionEndMarker(marker); end != "" {
		if j := strings.Index(seg, end); j >= 0 {
			seg = seg[:j]
		}
	}
	n := 0
	for _, line := range strings.Split(seg, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			n++
		}
	}
	return n
}

func buildL0SegmentsJSON(messages []trpcmodel.Message) string {
	segs := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		role := string(m.Role)
		segs = append(segs, map[string]any{
			"section": l0SegmentSection(content, role),
			"role":    role,
			"source":  l0SegmentSource(content, role),
			"tokens":  estTokensFromChars(messageCharLen(m)),
			"preview": l0Preview(content, 120),
		})
	}
	b, err := json.Marshal(segs)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func l0SegmentSection(content, role string) string {
	switch role {
	case string(trpcmodel.RoleUser):
		return "user.input"
	case string(trpcmodel.RoleAssistant):
		return "history"
	case string(trpcmodel.RoleTool):
		return "tool.result"
	default:
	}
	switch {
	case strings.HasPrefix(content, "You are "):
		return "system.prompt"
	case strings.Contains(content, "## L1 working memory"):
		return "memory.l1"
	case strings.Contains(content, "## L2+L3 memory"):
		return "memory.l2_l3"
	case strings.Contains(content, "## L2 episodic memory"):
		return "memory.l2"
	case strings.Contains(content, "## L3 semantic memory"):
		return "memory.l3"
	case strings.Contains(content, "## L4 knowledge graph"):
		return "memory.l4"
	case strings.Contains(content, "Session summary"), strings.Contains(content, "session summary"):
		return "summary"
	case strings.Contains(content, "Available skills:"):
		return "system.skill"
	case strings.Contains(content, "## Runtime capability policy"):
		return "system.runtime"
	default:
		return "system.other"
	}
}

func l0SegmentSource(content, role string) string {
	switch role {
	case string(trpcmodel.RoleUser):
		return "messages[current]"
	case string(trpcmodel.RoleAssistant):
		return "messages[assistant]"
	case string(trpcmodel.RoleTool):
		return "messages[tool]"
	}
	if sec := l0SegmentSection(content, role); strings.HasPrefix(sec, "memory.") {
		return sec
	}
	return "agent.prompt"
}

func l0Preview(content string, maxRunes int) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes]) + "…"
}

func promptSnapshotCurrentCallIndex(ctx context.Context) int {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return 1
	}
	if v, ok := inv.GetState(promptSnapshotStateKey); ok {
		if i, ok := v.(int); ok && i > 0 {
			return i
		}
	}
	return 1
}
