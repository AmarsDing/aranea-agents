package biz

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// SpiritSessionPhase is the orchestration lifecycle of a Spirit chat session.
// The system resolves it from persisted teams before the Spirit LLM runs, so
// turn routing (ForcePlanning, closeout tools, reuse) follows facts rather
// than prompt heuristics.
type SpiritSessionPhase string

const (
	SpiritPhaseIdle          SpiritSessionPhase = "idle"
	SpiritPhaseOrchestrating SpiritSessionPhase = "orchestrating"
	SpiritPhaseReady         SpiritSessionPhase = "ready"
	SpiritPhaseInterrupted   SpiritSessionPhase = "interrupted"
)

const orchestrationBriefMaxRunes = 1000

type spiritTurnOrchKey struct{}

// SpiritTurnOrchestration is the per-turn snapshot injected on the turn ctx
// so the pre-planning gate and runtime hooks share one resolution.
type SpiritTurnOrchestration struct {
	Phase SpiritSessionPhase
	Brief string
}

// WithSpiritTurnOrchestration stores the session-phase snapshot on ctx.
func WithSpiritTurnOrchestration(ctx context.Context, orch SpiritTurnOrchestration) context.Context {
	return context.WithValue(ctx, spiritTurnOrchKey{}, orch)
}

// SpiritTurnOrchestrationFrom returns the snapshot stored by
// WithSpiritTurnOrchestration. ok is false when the value was never set.
func SpiritTurnOrchestrationFrom(ctx context.Context) (SpiritTurnOrchestration, bool) {
	if ctx == nil {
		return SpiritTurnOrchestration{}, false
	}
	v, ok := ctx.Value(spiritTurnOrchKey{}).(SpiritTurnOrchestration)
	return v, ok
}

// DeferredSummaryGuardCue forbids closing a Spirit turn with a deferred
// "I'll summarize when the background job finishes" promise unless an
// aggregation step is actually in this turn's tool list.
const DeferredSummaryGuardCue = `## Closeout rule
Do not end this turn by promising a later summary (「后台跑完再汇总」「稍后告诉你」「等做完再回复」「等团队完成后我再告诉你」). Either answer now from what you already know, or call synthesize_results / get_team_deliverable this turn if those tools are in your list. If teams are still running, wait for the system completion notice — do not invent a deferred-summary promise.`

var deferredSummaryPromiseMarkers = []string{
	"后台跑完再汇总",
	"稍后告诉你",
	"等做完再回复",
	"等团队完成后",
	"跑完再告诉",
	"I'll summarize later",
	"i'll get back to you once",
}

// LooksLikeDeferredSummaryPromise reports whether model text is the
// forbidden closeout (promise a summary after background work, with no
// aggregation call this turn).
func LooksLikeDeferredSummaryPromise(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return false
	}
	for _, m := range deferredSummaryPromiseMarkers {
		if strings.Contains(t, strings.ToLower(m)) {
			return true
		}
	}
	return false
}

// ResolveSpiritSessionPhase classifies a Spirit session from its teams.
// Deleted rows are ignored. Running/pending wins over interrupted; interrupted
// with no running work is Interrupted; all remaining live teams terminal → Ready.
func ResolveSpiritSessionPhase(teams []Team) SpiritSessionPhase {
	var hasLive, hasRunningOrPending, hasInterrupted bool
	for _, t := range teams {
		if teamRowIgnored(t) {
			continue
		}
		hasLive = true
		switch t.Status {
		case TeamStatusPending, TeamStatusRunning:
			hasRunningOrPending = true
		case TeamStatusInterrupted:
			hasInterrupted = true
		}
	}
	if !hasLive {
		return SpiritPhaseIdle
	}
	if hasRunningOrPending {
		return SpiritPhaseOrchestrating
	}
	if hasInterrupted {
		return SpiritPhaseInterrupted
	}
	return SpiritPhaseReady
}

// PhasePromotedToolNames returns deferred tools that should be activated for
// this phase so they appear in Request.Tools without a tool_load round-trip.
// Idle keeps the WP-4 four-tool resident set (prefix cache).
func PhasePromotedToolNames(phase SpiritSessionPhase) []string {
	switch phase {
	case SpiritPhaseOrchestrating, SpiritPhaseInterrupted:
		return []string{"cancel_orchestration"}
	case SpiritPhaseReady:
		return []string{"get_team_deliverable", "synthesize_results"}
	default:
		return nil
	}
}

// ShouldForcePlanning reports whether the pre-planning gate may inject the
// forced plan_and_execute path. Complexity still drives Idle sessions;
// once teams exist, ForcePlanning is allowed only for an explicit new task.
func ShouldForcePlanning(phase SpiritSessionPhase, forceFromComplexity, looksLikeNewTask bool) bool {
	if !forceFromComplexity {
		return false
	}
	switch phase {
	case SpiritPhaseOrchestrating, SpiritPhaseReady, SpiritPhaseInterrupted:
		return looksLikeNewTask
	default:
		return true
	}
}

// WantsNewOrchestration reports explicit user language asking to start a brand
// new DAG even when the session already has teams.
func WantsNewOrchestration(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, kw := range []string{
		"重新组建", "再组建", "重新规划", "另组", "另起",
		"从头再来", "换标的", "换成", "换一家", "另外一家", "新标的",
		"force_new", "start over", "re-plan", "replan",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// LooksLikeFreshOrchestrationAsk reports whether the message looks like a new
// analysis/team-formation request (as opposed to a follow-up such as "结果怎么样").
func LooksLikeFreshOrchestrationAsk(prompt string) bool {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	for _, kw := range []string{
		"组建", "创建团队", "成立团队", "编排", "分派",
		"分析", "调研", "研究行情", "帮我看看",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// OrchestrationLooksLikeNewTask reports whether this user turn should open a
// new DAG despite an existing session phase. Keywords win; otherwise a fresh
// ask whose distinctive tokens diverge from the last plan (or refined goal)
// counts as a new task.
func OrchestrationLooksLikeNewTask(userMessage, lastPlanMessage, refinedGoal string) bool {
	if WantsNewOrchestration(userMessage) {
		return true
	}
	compare := strings.TrimSpace(userMessage)
	if g := strings.TrimSpace(refinedGoal); g != "" {
		compare = g
	}
	if !LooksLikeFreshOrchestrationAsk(compare) && !LooksLikeFreshOrchestrationAsk(userMessage) {
		return false
	}
	baseline := strings.TrimSpace(lastPlanMessage)
	if baseline == "" {
		return false
	}
	return distinctiveShift(compare, baseline)
}

// OrchestrationGoalShifted reports that the prompt looks like a fresh
// analysis/team-formation ask whose distinctive entity tokens do not appear
// in any live team name/task. Follow-ups with no such ask ("结果怎么样")
// return false so the current DAG is kept.
func OrchestrationGoalShifted(prompt string, teams []Team) bool {
	if !LooksLikeFreshOrchestrationAsk(prompt) || len(teams) == 0 {
		return false
	}
	var corpus strings.Builder
	for _, t := range teams {
		if teamRowIgnored(t) {
			continue
		}
		corpus.WriteString(t.DisplayName)
		corpus.WriteByte(' ')
		corpus.WriteString(t.TaskDescription)
		corpus.WriteByte(' ')
	}
	return distinctiveShift(prompt, corpus.String())
}

// FormatOrchestrationBrief builds a ≤1k-rune runtime cue: phase, team
// id/status/one-liner, and the turn rule for that phase. Empty for Idle.
func FormatOrchestrationBrief(phase SpiritSessionPhase, teams []Team) string {
	if phase == SpiritPhaseIdle || phase == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Session orchestration\n")
	fmt.Fprintf(&b, "phase: %s\n", phase)
	live := make([]Team, 0, len(teams))
	for _, t := range teams {
		if teamRowIgnored(t) {
			continue
		}
		live = append(live, t)
	}
	fmt.Fprintf(&b, "teams: %d\n", len(live))
	for _, t := range live {
		line := fmt.Sprintf("- %s [%s] %s", t.ID, t.Status, strings.TrimSpace(t.DisplayName))
		if preview := teamBriefPreview(t); preview != "" {
			line += " | " + preview
		}
		b.WriteString(truncateRunes(line, 160))
		b.WriteByte('\n')
	}
	switch phase {
	case SpiritPhaseOrchestrating:
		b.WriteString("Teams are still running. Wait for the system completion notice. cancel_orchestration is in your tool list this turn. Do NOT call plan_and_execute again unless the user explicitly asked to start a NEW independent task. Do NOT call get_team_deliverable yet.\n")
	case SpiritPhaseInterrupted:
		b.WriteString("Some teams were interrupted. Do NOT start a new DAG. Wait for recovery or ask the user whether to resume. cancel_orchestration is in your tool list this turn.\n")
	case SpiritPhaseReady:
		b.WriteString("All listed teams are terminal. Answer from existing results (a system synthesis may already be in this chat). get_team_deliverable and synthesize_results are already in your tool list this turn — do not tool_load them. Do NOT call plan_and_execute unless the user explicitly asked to start a NEW independent analysis.\n")
	}
	return truncateRunes(strings.TrimSpace(b.String()), orchestrationBriefMaxRunes)
}

func teamRowIgnored(t Team) bool {
	if strings.TrimSpace(t.DeletedAt) != "" {
		return true
	}
	return t.Status == TeamStatusDeleted
}

func teamBriefPreview(t Team) string {
	refs := ParseDeliverableRefs(t.DeliverablesOutput)
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		s := strings.TrimSpace(ref.Summary)
		if s == "" {
			s = strings.TrimSpace(ref.KeyFindings)
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	return truncateRunes(strings.Join(parts, " / "), 80)
}

func distinctiveShift(probe, baseline string) bool {
	probeToks := distinctiveTokens(probe)
	if len(probeToks) == 0 {
		return false
	}
	matched := 0
	distinct := 0
	for _, tok := range probeToks {
		if genericOverlapTokens[tok] {
			continue
		}
		distinct++
		if tok != "" && strings.Contains(baseline, tok) {
			matched++
		}
	}
	if distinct == 0 {
		return false
	}
	return matched == 0
}

// genericOverlapTokens are 4-char CJK grams that appear in almost every
// research/orchestration team name. Matching only these would treat a later
// "另一家上市公司" ask as the same goal.
var genericOverlapTokens = map[string]bool{
	"上市公司": true, "证券代码": true, "基本面分": true, "本面分析": true,
	"技术分析": true, "技术面分": true, "综合评审": true, "投资建议": true,
	"行情分析": true, "分析报告": true, "研究报告": true, "执行结果": true,
	"团队协作": true, "任务分解": true, "对应主体": true, "核实对应": true,
}

func distinctiveTokens(src string) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(tok string) {
		if tok == "" || seen[tok] {
			return
		}
		seen[tok] = true
		out = append(out, tok)
	}
	for _, run := range cjkRuns(src, 4) {
		rs := []rune(run)
		if len(rs) == 4 {
			add(run)
			continue
		}
		for i := 0; i+4 <= len(rs); i++ {
			add(string(rs[i : i+4]))
		}
	}
	var word []rune
	flushWord := func() {
		if len(word) >= 6 {
			add(strings.ToLower(string(word)))
		}
		word = word[:0]
	}
	for _, r := range src {
		if unicode.IsLetter(r) && r < 0x2E80 {
			word = append(word, unicode.ToLower(r))
			continue
		}
		flushWord()
	}
	flushWord()
	return out
}

func cjkRuns(s string, min int) []string {
	var run []rune
	var out []string
	flush := func() {
		if len(run) >= min {
			out = append(out, string(run))
		}
		run = run[:0]
	}
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			run = append(run, r)
			continue
		}
		flush()
	}
	flush()
	return out
}
