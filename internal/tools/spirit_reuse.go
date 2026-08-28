package tools

import (
	"strings"
	"unicode"

	"aranea-agents/internal/biz"
)

const reuseStrategy = "reuse"

// ExistingTeamSummary is a compact team snapshot returned when plan_and_execute
// reuses an already-running or completed orchestration instead of decomposing again.
type ExistingTeamSummary struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Task    string `json:"task,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// genericOverlapTokens are 4-char CJK grams that appear in almost every
// research/orchestration team name. Matching only these would reuse a 金鹏
// DAG when the user later asks about a different 上市公司.
var genericOverlapTokens = map[string]bool{
	"上市公司": true, "证券代码": true, "基本面分": true, "本面分析": true,
	"技术分析": true, "技术面分": true, "综合评审": true, "投资建议": true,
	"行情分析": true, "分析报告": true, "研究报告": true, "执行结果": true,
	"团队协作": true, "任务分解": true, "对应主体": true, "核实对应": true,
}

// wantsForceNewOrchestration reports whether the caller asked to start a brand
// new DAG even if overlapping teams already exist in the spirit session.
func wantsForceNewOrchestration(forceNew bool, prompt string) bool {
	return forceNew || biz.WantsNewOrchestration(prompt)
}

// currentOrchestrationCohort returns the live (active/completed) teams of the
// latest DAG in the session. Used when the user follow-up has no entity overlap
// (e.g. "结果怎么样") but the session phase already has an orchestration.
func currentOrchestrationCohort(teams []biz.Team) []biz.Team {
	live := make([]biz.Team, 0, len(teams))
	for _, t := range teams {
		if strings.TrimSpace(t.DeletedAt) != "" {
			continue
		}
		switch t.Status {
		case biz.TeamStatusFailed, biz.TeamStatusCancelled, biz.TeamStatusArchived:
			continue
		case biz.TeamStatusPending, biz.TeamStatusRunning, biz.TeamStatusInterrupted, biz.TeamStatusCompleted, biz.TeamStatusPartialFailure:
			// partial_failure 交付物已产出，与 completed 同属可跟进/可复用队列。
			live = append(live, t)
		}
	}
	if len(live) == 0 {
		return nil
	}
	latest := live[0]
	for _, t := range live[1:] {
		if t.UpdatedAt > latest.UpdatedAt || (t.UpdatedAt == latest.UpdatedAt && t.CreatedAt > latest.CreatedAt) {
			latest = t
		}
	}
	if g := strings.TrimSpace(latest.LinkedGraphID); g != "" {
		return expandToOrchestrationCohort([]biz.Team{latest}, teams)
	}
	return live
}

// reuseFallbackAllowed reports whether a follow-up with no distinctive entity
// overlap may attach to the current orchestration cohort. Active teams may
// always be followed ("还在跑吗"). Completed-only cohorts require a status
// follow-up — a fresh analysis/team-formation ask without entity overlap
// must open a new DAG instead of replaying the last completed run.
func reuseFallbackAllowed(prompt string, fallback []biz.Team) bool {
	if len(fallback) == 0 {
		return false
	}
	if !cohortAllCompleted(fallback) {
		return true
	}
	return !biz.LooksLikeFreshOrchestrationAsk(prompt)
}

func cohortAllCompleted(teams []biz.Team) bool {
	if len(teams) == 0 {
		return false
	}
	for _, t := range teams {
		if biz.IsTeamStatusActive(t.Status) {
			return false
		}
	}
	return true
}

// reusableOverlappingTeams returns session teams that overlap the new prompt
// and are still usable (active or completed). Failed/cancelled/archived teams
// do not block a fresh plan.
func reusableOverlappingTeams(prompt string, teams []biz.Team) []biz.Team {
	if strings.TrimSpace(prompt) == "" || len(teams) == 0 {
		return nil
	}
	out := make([]biz.Team, 0, len(teams))
	for _, t := range teams {
		if strings.TrimSpace(t.DeletedAt) != "" {
			continue
		}
		switch t.Status {
		case biz.TeamStatusFailed, biz.TeamStatusCancelled, biz.TeamStatusArchived:
			continue
		}
		if teamOverlapsPrompt(t, prompt) {
			out = append(out, t)
		}
	}
	return out
}

// expandToOrchestrationCohort returns the overlapping teams plus siblings from
// the same DAG (LinkedGraphID). A 4-team 金鹏 run often only has the entity in
// one DisplayName ("核实金鹏科技…"); the 基本面/技术面/综合评审 siblings would
// otherwise be dropped. When no graph id is present, keep the overlap set
// (do not dump the whole session — a later unrelated topic in the same chat
// must still be able to start a new DAG).
func expandToOrchestrationCohort(overlap, all []biz.Team) []biz.Team {
	if len(overlap) == 0 {
		return nil
	}
	graphs := make(map[string]bool)
	seen := make(map[string]bool)
	out := make([]biz.Team, 0, len(all))
	add := func(t biz.Team) {
		if t.ID == "" || seen[t.ID] {
			return
		}
		if strings.TrimSpace(t.DeletedAt) != "" {
			return
		}
		switch t.Status {
		case biz.TeamStatusFailed, biz.TeamStatusCancelled, biz.TeamStatusArchived:
			return
		}
		seen[t.ID] = true
		out = append(out, t)
	}
	for _, t := range overlap {
		add(t)
		if g := strings.TrimSpace(t.LinkedGraphID); g != "" {
			graphs[g] = true
		}
	}
	if len(graphs) == 0 {
		return out
	}
	for _, t := range all {
		if graphs[strings.TrimSpace(t.LinkedGraphID)] {
			add(t)
		}
	}
	return out
}

func reuseNextAction(teams []biz.Team) string {
	active, completed := 0, 0
	for _, t := range teams {
		if biz.IsTeamStatusActive(t.Status) {
			active++
		}
		if t.Status == biz.TeamStatusCompleted || t.Status == biz.TeamStatusPartialFailure {
			completed++
		}
	}
	if active > 0 {
		return "Teams are still running. Wait for the system completion notice. Do NOT call get_team_deliverable, synthesize_results, or plan_and_execute again."
	}
	if completed > 0 {
		return "All listed teams completed. Summaries are in existing_teams[].summary. get_team_deliverable is already in your tool list this turn if you need full text; otherwise synthesize_results or answer the user. Do not call plan_and_execute again unless the user asked for a NEW independent analysis (force_new=true)."
	}
	return "Reuse the listed teams. Do not start a new DAG."
}

func teamOverlapsPrompt(t biz.Team, prompt string) bool {
	src := strings.TrimSpace(t.DisplayName) + " " + strings.TrimSpace(t.TaskDescription)
	if strings.TrimSpace(src) == "" {
		return false
	}
	for _, tok := range distinctiveTokens(src) {
		if genericOverlapTokens[tok] {
			continue
		}
		if tok != "" && strings.Contains(prompt, tok) {
			return true
		}
	}
	return false
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

func summarizeExistingTeams(teams []biz.Team) []ExistingTeamSummary {
	out := make([]ExistingTeamSummary, 0, len(teams))
	for _, t := range teams {
		out = append(out, ExistingTeamSummary{
			ID:      t.ID,
			Name:    t.DisplayName,
			Status:  t.Status,
			Task:    t.TaskDescription,
			Summary: teamDeliverablePreview(t),
		})
	}
	return out
}

func teamDeliverablePreview(t biz.Team) string {
	refs := biz.ParseDeliverableRefs(t.DeliverablesOutput)
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
	text := strings.Join(parts, "\n")
	rs := []rune(text)
	const maxRunes = 800
	if len(rs) > maxRunes {
		return string(rs[:maxRunes]) + "…"
	}
	return text
}
