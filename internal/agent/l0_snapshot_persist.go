package agent

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/llmcontext"
	"aranea-agents/pkg/loggateway"

	"go.opentelemetry.io/otel/trace"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"

	"github.com/google/uuid"
)

const l0SnapshotByCallStateKey = "aranea:l0_snapshot_by_call"

const l0SnapshotThrottleStateKey = "aranea:l0_snapshot_throttle"

const (
	// TODO(debt): make throttle params configurable via agent settings.
	l0SnapshotMinIntervalSec = 300
	l0SnapshotRatioDelta     = 0.10
	l0SnapshotThresholdCross = 0.80
	l0SnapshotLowRatioSkip   = 0.60
)

type l0SnapshotPending struct {
	ID        string
	SessionID string
	Window    int
}

type l0SnapshotThrottleState struct {
	LastWriteAt time.Time
	LastRatio   float64
}

func l0SnapshotForceDebug() bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_L0_SNAPSHOT"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "always") || strings.EqualFold(v, "force")
}

func l0SnapshotHookActive(ag biz.Agent) bool {
	if l0SnapshotForceDebug() {
		return true
	}
	if ag.Settings == nil || !ag.Settings.L0SnapshotEnabled {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(ag.Settings.L0SnapshotMode))
	return mode != "off"
}

func newL0SnapshotAfterModelHook(deps TRPCBuilderDeps) callbacks.Callback {
	if deps.L0Admin == nil {
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
		if err := deps.L0Admin.UpdateL0SnapshotActual(ctx, pending.ID, pending.SessionID, actual, pending.Window); err != nil {
			deps.Logger().Warn("L0 快照 actual 更新失败",
				loggateway.StepID("agent.l0.snapshot"),
				loggateway.Str("snapshot_id", pending.ID),
				loggateway.Int("model_call_index", callIndex),
				loggateway.Err(err),
			)
		}
		return &trpcmodel.AfterModelResult{Context: ctx}, nil
	})
}

func persistL0AssemblySnapshot(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, messages []trpcmodel.Message, callIndex int) {
	if deps.L0Admin == nil || !l0SnapshotHookActive(ag) {
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
	prov := strings.TrimSpace(ag.Provider)
	mod := strings.TrimSpace(ag.Model)
	force := l0SnapshotForceDebug()

	// Check if compression set a force-write flag for this session.
	if !force && deps.L0SnapshotForcer != nil && deps.L0SnapshotForcer.ConsumeForceL0Snapshot(sessionID) {
		force = true
	}

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

	// Throttle check: skip if interval/ratio-delta conditions not met (unless forced).
	snapshotMode := ""
	if ag.Settings != nil {
		snapshotMode = strings.ToLower(strings.TrimSpace(ag.Settings.L0SnapshotMode))
	}
	if !l0SnapshotThrottleAllows(inv, usedRatio, force, snapshotMode) {
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
	segsJSON := buildL0SegmentsSummaryJSON(messages)

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
	// Overlay compression metadata when the compression hook fired this call.
	// Deterministic emergency truncation (no LLM summary) since 2026-07-20
	// dual-compression unification.
	if compMeta := LoadCompressionMeta(ctx); compMeta.Occurred {
		in.TruncateStrategy = "emergency_truncation"
		in.TruncatedMessageCount = compMeta.EvictedCount
		if compMeta.SummaryText != "" {
			in.SummaryTokenEstimate = estTokensFromChars(utf8.RuneCountInString(compMeta.SummaryText))
		}
	}
	if err := deps.L0Admin.InsertL0AssemblySnapshot(ctx, in); err != nil {
		deps.Logger().Warn("L0 快照落库失败",
			loggateway.StepID("agent.l0.snapshot"),
			loggateway.Str("session_id", sessionID),
			loggateway.Err(err),
		)
		return
	}
	setL0SnapshotPendingForCall(inv, callIndex, in.ID, in.SessionID, win)
	l0SnapshotThrottleRecord(inv, usedRatio)
	deps.Logger().Info("L0 快照已落库", loggateway.StepID("agent.l0.snapshot"), loggateway.Phase("done"),
		loggateway.Str("snapshot_id", in.ID),
		loggateway.Str("session_id", sessionID),
		loggateway.Int("model_call_index", callIndex),
		loggateway.Int("est_tokens", report.EstTokens),
		loggateway.Any("used_ratio", usedRatio),
		loggateway.Int("context_window", win),
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

// l0SnapshotThrottleAllows checks whether a snapshot write should proceed
// based on throttling rules: minimum interval, ratio delta, and threshold crossing.
// Force mode and "always" mode bypass throttling.
func l0SnapshotThrottleAllows(inv *trpcagent.Invocation, usedRatio float64, force bool, snapshotMode string) bool {
	if force {
		return true
	}
	if inv == nil {
		return true
	}
	if snapshotMode == "always" {
		return true
	}
	// Low ratio: skip entirely
	if usedRatio < l0SnapshotLowRatioSkip {
		return false
	}
	state := l0SnapshotThrottleLoad(inv)
	now := time.Now()
	// Threshold crossing: force write when usedRatio crosses 0.80
	if l0CrossedThreshold(state.LastRatio, usedRatio, l0SnapshotThresholdCross) {
		return true
	}
	// Minimum interval check
	if !state.LastWriteAt.IsZero() && now.Sub(state.LastWriteAt) < time.Duration(l0SnapshotMinIntervalSec)*time.Second {
		return false
	}
	// Ratio delta check
	if state.LastRatio > 0 && math.Abs(usedRatio-state.LastRatio) < l0SnapshotRatioDelta {
		return false
	}
	return true
}

// l0CrossedThreshold returns true if the ratio crossed the threshold between
// the last write and the current value (from below to above).
func l0CrossedThreshold(lastRatio, usedRatio, threshold float64) bool {
	return lastRatio < threshold && usedRatio >= threshold
}

// l0SnapshotThrottleRecord updates the throttle state after a successful write.
func l0SnapshotThrottleRecord(inv *trpcagent.Invocation, usedRatio float64) {
	if inv == nil {
		return
	}
	inv.SetState(l0SnapshotThrottleStateKey, l0SnapshotThrottleState{
		LastWriteAt: time.Now(),
		LastRatio:   usedRatio,
	})
}

// l0SnapshotThrottleLoad reads the current throttle state from the invocation.
func l0SnapshotThrottleLoad(inv *trpcagent.Invocation) l0SnapshotThrottleState {
	if inv == nil {
		return l0SnapshotThrottleState{}
	}
	if v, ok := inv.GetState(l0SnapshotThrottleStateKey); ok {
		if s, ok := v.(l0SnapshotThrottleState); ok {
			return s
		}
	}
	return l0SnapshotThrottleState{}
}

func setL0SnapshotPendingForCall(inv *trpcagent.Invocation, callIndex int, id, sessionID string, win int) {
	if inv == nil || callIndex <= 0 || strings.TrimSpace(id) == "" {
		return
	}
	m := l0SnapshotPendingMap(inv)
	m[callIndex] = l0SnapshotPending{ID: id, SessionID: sessionID, Window: win}
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
	if deps.ModelCatalog != nil {
		if row, err := deps.ModelCatalog.GetByProviderAndModel(ctx, prov, mod); err == nil {
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
		if m.Role != trpcmodel.RoleSystem && !isDynamicCueMessage(m) {
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

// buildL0SegmentsSummaryJSON produces aggregated segment statistics instead of
// per-message details. This reduces data volume by 80%+ while preserving the
// information needed for monitoring and diagnostics.
func buildL0SegmentsSummaryJSON(messages []trpcmodel.Message) string {
	type sectionAgg struct {
		tokenEstimate int
		messageCount  int
		fieldCount    int // for memory sections
		factCount     int // for L3
		entityCount   int // for L4
		fromTurn      int
		toTurn        int
	}
	aggs := make(map[string]*sectionAgg)
	turnIdx := 0
	for _, m := range messages {
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		role := string(m.Role)
		section := l0SegmentSection(content, role)
		tokens := estTokensFromChars(messageCharLen(m))
		agg, ok := aggs[section]
		if !ok {
			agg = &sectionAgg{}
			aggs[section] = agg
		}
		agg.tokenEstimate += tokens
		agg.messageCount++
		// Track turn range for history/user sections
		if role == string(trpcmodel.RoleUser) || role == string(trpcmodel.RoleAssistant) {
			turnIdx++
			if agg.fromTurn == 0 {
				agg.fromTurn = turnIdx
			}
			agg.toTurn = turnIdx
		}
		// Count bullets for memory sections
		switch section {
		case "memory.l1":
			agg.fieldCount += countBulletLines(content, "## L1 working memory")
		case "memory.l3", "memory.l2_l3":
			marker := "## L3 semantic memory"
			if section == "memory.l2_l3" {
				marker = "## L2+L3 memory"
			}
			agg.factCount += countBulletLines(content, marker)
		case "memory.l4":
			agg.entityCount += countBulletLines(content, "## L4 knowledge graph")
		}
	}
	// Build output map
	result := make(map[string]map[string]any, len(aggs))
	for section, agg := range aggs {
		entry := map[string]any{
			"token_estimate": agg.tokenEstimate,
			"message_count":  agg.messageCount,
		}
		if agg.fieldCount > 0 {
			entry["field_count"] = agg.fieldCount
		}
		if agg.factCount > 0 {
			entry["fact_count"] = agg.factCount
		}
		if agg.entityCount > 0 {
			entry["entity_count"] = agg.entityCount
		}
		if agg.fromTurn > 0 {
			entry["from_turn"] = agg.fromTurn
			entry["to_turn"] = agg.toTurn
		}
		result[section] = entry
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func l0SegmentSection(content, role string) string {
	switch role {
	case string(trpcmodel.RoleAssistant):
		return "history"
	case string(trpcmodel.RoleTool):
		return "tool.result"
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
	case strings.Contains(content, "Available skills:"), strings.Contains(content, "## Available Skills"), strings.Contains(content, "## Routed Skills"):
		return "system.skill"
	case strings.Contains(content, "## Runtime capability policy"):
		return "system.runtime"
	case role == string(trpcmodel.RoleUser):
		return "user.input"
	default:
		return "system.other"
	}
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
