package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Synthesis usecase tests — per-unit run stats enrichment, engine error
// propagation, and CheckAllTeamsCompleted cancelled counting.
// ---------------------------------------------------------------------------

// stubTeamRunStats implements SpiritTeamRunStatsReader.
type stubTeamRunStats struct {
	stats map[string]SpiritTeamRunStats
	err   error
}

func (s *stubTeamRunStats) ListLatestRunStatsByTeams(_ context.Context, _ []string) (map[string]SpiritTeamRunStats, error) {
	return s.stats, s.err
}

func newReportUsecase(t *testing.T, teams *deliverableTeamRepo, sessions *deliverableSessionAccessor, engine *SynthesisEngine, opts ...SpiritTeamUsecaseOption) *SynthesisUsecase {
	t.Helper()
	t.Setenv("ARANEA_ENV", "dev")
	spiritUC := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop(), opts...)
	return NewSynthesisUsecase(spiritUC, engine, loggateway.NewNoop())
}

func TestSynthesizeResults_EnrichesTeamRunStats(t *testing.T) {
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", DisplayName: "团队A", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	teams.items["t2"] = Team{ID: "t2", DisplayName: "团队B", Status: TeamStatusFailed, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	stats := &stubTeamRunStats{stats: map[string]SpiritTeamRunStats{
		"t1": {TeamID: "t1", DurationMs: 8000},
		"t2": {TeamID: "t2", DurationMs: 5000, ErrorMessage: "timeout"},
	}}
	uc := newReportUsecase(t, teams, sessions, NewSynthesisEngine(nil, loggateway.NewNoop()), WithSpiritTeamRunStatsReader(stats))

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyTemplate))
	if err != nil {
		t.Fatalf("SynthesizeResults returned error: %v", err)
	}
	byID := map[string]TeamSynthesisResult{}
	for _, r := range out.TeamResults {
		byID[r.TeamID] = r
	}
	if r := byID["t1"]; r.DurationMs != 8000 || r.ErrorMessage != "" {
		t.Fatalf("t1 stats = %d/%q, want 8000/\"\"", r.DurationMs, r.ErrorMessage)
	}
	if r := byID["t2"]; r.DurationMs != 5000 || r.ErrorMessage != "timeout" {
		t.Fatalf("t2 stats = %d/%q, want 5000/timeout", r.DurationMs, r.ErrorMessage)
	}
}

func TestSynthesizeResults_NilRunStatsReader_OmitsStats(t *testing.T) {
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", DisplayName: "团队A", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	uc := newReportUsecase(t, teams, sessions, NewSynthesisEngine(nil, loggateway.NewNoop()))

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyTemplate))
	if err != nil {
		t.Fatalf("SynthesizeResults returned error: %v", err)
	}
	if len(out.TeamResults) != 1 || out.TeamResults[0].DurationMs != 0 || out.TeamResults[0].ErrorMessage != "" {
		t.Fatalf("stats should be omitted when reader is nil: %+v", out.TeamResults)
	}
}

// Engine failure propagates as (nil, err) — the usecase no longer builds a
// degraded report (the dedicated report UI was removed; the user-facing
// summary is produced by the Spirit summary turn).
func TestSynthesizeResults_EngineErrorReturnsNilOutput(t *testing.T) {
	t.Setenv("ARANEA_ENV", "production")
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", DisplayName: "团队A", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	stub := &stubSynthesisModel{err: errors.New("LLM unavailable")}
	spiritUC := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop())
	uc := NewSynthesisUsecase(spiritUC, NewSynthesisEngine(stub, loggateway.NewNoop()), loggateway.NewNoop())

	out, err := uc.SynthesizeResults(context.Background(), "spirit-1", string(SynthesisStrategyPrompt))
	if err == nil {
		t.Fatal("expected error when model fails in production")
	}
	if !errors.Is(err, ErrSynthesisModelFailed) {
		t.Fatalf("want ErrSynthesisModelFailed, got %v", err)
	}
	if out != nil {
		t.Fatalf("output should be nil on engine error, got %+v", out)
	}
}

func TestCheckAllTeamsCompleted_CountsCancelled(t *testing.T) {
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", Status: TeamStatusCompleted, SpiritSessionID: "spirit-1"}
	teams.items["t2"] = Team{ID: "t2", Status: TeamStatusCancelled, SpiritSessionID: "spirit-1"}
	sessions := newDeliverableSessionAccessor()
	uc := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop())

	res := uc.CheckAllTeamsCompleted(context.Background(), "spirit-1")
	if !res.AllDone {
		t.Fatal("AllDone should be true when all teams are terminal")
	}
	if res.CancelledTeams != 1 {
		t.Fatalf("CancelledTeams = %d, want 1", res.CancelledTeams)
	}
	if res.FailedTeams != 1 {
		t.Fatalf("FailedTeams = %d, want 1 (cancelled still counted as failed)", res.FailedTeams)
	}
	if res.CompletedTeams != 1 {
		t.Fatalf("CompletedTeams = %d, want 1", res.CompletedTeams)
	}
}

// ---------------------------------------------------------------------------
// 2026-07-25 Fix 3 收尾报告诚实化
//
// 19:29 根因链的最后一环：上游团队只提问未产出（Fix 1+2 已翻转为 failed），
// 但收尾报告仍声称「所有团队已完成」并幻觉出成功结论。诚实化要求：
//   - 存在失败团队时，触发文本/报告模板必须如实说明完成与失败数量，禁止
//     「所有团队已完成」式谎报；
//   - 失败团队的失败原因与其遗留疑问（最后一次回复）必须提供给报告生成方；
//   - 报告结构必须包含「未解决问题」小节，且禁止为失败团队虚构结论。
// ---------------------------------------------------------------------------

// 全部完成 → 保留既有成功结构（不出现「未解决问题」要求）。
func TestBuildSynthesisSummaryTrigger_AllCompleted_KeepsSuccessStructure(t *testing.T) {
	out := BuildSynthesisSummaryTrigger(2, 2, 0, nil, nil)

	if !strings.Contains(out, "所有团队已完成") {
		t.Fatalf("all-completed trigger should keep the success opening, got:\n%s", out)
	}
	for _, section := range []string{"## 任务总结", "## 各团队结果", "## 综合结论"} {
		if !strings.Contains(out, section) {
			t.Fatalf("all-completed trigger missing section %q, got:\n%s", section, out)
		}
	}
	if strings.Contains(out, "## 未解决问题") {
		t.Fatalf("success path must not demand an unresolved-questions section, got:\n%s", out)
	}
}

// F7 (Phase 11)：交付物摘要内嵌 trigger —— 成功路径也必须带摘要段，
// LLM 无需 read_session_history 考古。
func TestBuildSynthesisSummaryTrigger_AllCompleted_WithDigests(t *testing.T) {
	digests := []TeamDeliverableDigest{{
		TeamName:          "安装 xlsx 团队",
		TaskName:          "安装 xlsx skill",
		Status:            "completed",
		DeliverableSummary: `{"status":"success","detail":"xlsx 1.2.3 installed"}`,
	}}
	out := BuildSynthesisSummaryTrigger(1, 1, 0, nil, digests)

	if !strings.Contains(out, "所有团队已完成") {
		t.Fatalf("success opening must remain, got:\n%s", out)
	}
	for _, want := range []string{"交付物摘要", "安装 xlsx 团队", `"status":"success"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("success trigger missing digest content %q, got:\n%s", want, out)
		}
	}
}

// 存在失败团队 → 触发文本必须诚实：不得出现「所有团队已完成」，必须给出
// 真实完成/失败数量、失败团队名称/原因/遗留疑问，并要求「未解决问题」小节
// 与反虚构约束。
func TestBuildSynthesisSummaryTrigger_WithFailures_Honest(t *testing.T) {
	failures := []TeamFailureBrief{{
		TeamName:  "数据采集团队",
		TaskName:  "采集竞品价格",
		Reason:    "团队未通过 set_deliverable 提交真实交付物",
		LastReply: "需要您澄清：目标竞品名单与时间范围？",
	}}
	out := BuildSynthesisSummaryTrigger(2, 1, 1, failures, nil)

	if strings.Contains(out, "所有团队已完成") {
		t.Fatalf("trigger must not claim all teams completed when failures exist, got:\n%s", out)
	}
	if !strings.Contains(out, "1 个完成") || !strings.Contains(out, "1 个失败") {
		t.Fatalf("trigger must state the truthful completed/failed counts, got:\n%s", out)
	}
	for _, want := range []string{
		"数据采集团队", // 失败团队名称
		"团队未通过 set_deliverable 提交真实交付物", // 失败原因
		"需要您澄清：目标竞品名单与时间范围？",            // 遗留疑问（最后一次回复）
		"## 未解决问题", // 必备小节
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("honest trigger missing %q, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "虚构") {
		t.Fatalf("trigger must forbid fabricating conclusions for failed teams, got:\n%s", out)
	}
}

// F7：失败路径摘要段位于失败事实段之前（含成功团队摘要 + 失败团队「无交付物」）。
func TestBuildSynthesisSummaryTrigger_WithFailures_DigestsBeforeFailureFacts(t *testing.T) {
	failures := []TeamFailureBrief{{TeamName: "数据采集团队", TaskName: "采集", Reason: "无交付物"}}
	digests := []TeamDeliverableDigest{
		{TeamName: "成功团队", TaskName: "调研", Status: "completed", DeliverableSummary: "报告已完成"},
		{TeamName: "数据采集团队", TaskName: "采集", Status: "failed"},
	}
	out := BuildSynthesisSummaryTrigger(2, 1, 1, failures, digests)

	digestIdx := strings.Index(out, "交付物摘要")
	failureIdx := strings.Index(out, "失败事实（必须如实呈现")
	if digestIdx < 0 || failureIdx < 0 || digestIdx > failureIdx {
		t.Fatalf("digest section must precede failure facts (digest=%d failure=%d), got:\n%s", digestIdx, failureIdx, out)
	}
	if !strings.Contains(out, "成功团队") || !strings.Contains(out, "报告已完成") {
		t.Fatalf("completed team digest missing, got:\n%s", out)
	}
	if !strings.Contains(out, "无交付物") {
		t.Fatalf("failed team without deliverable should render 无交付物, got:\n%s", out)
	}
}

// 引擎模板策略：存在失败团队时默认模板的收尾句不得声称「所有团队已完成任务」。
func TestSynthesizeTemplate_FailedTeam_HonestClosing(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	e := NewSynthesisEngine(nil, loggateway.NewNoop())
	out, err := e.Synthesize(context.Background(), SynthesisInput{
		Strategy: SynthesisStrategyTemplate,
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", TaskName: "任务A", Status: "completed", Summary: "完成"},
			{TeamID: "t2", TeamName: "团队B", TaskName: "任务B", Status: "failed", ErrorMessage: "no deliverable"},
		},
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if strings.Contains(out.Content, "所有团队已完成任务") {
		t.Fatalf("template closing lies when a team failed, got:\n%s", out.Content)
	}
	if !strings.Contains(out.Content, "失败") {
		t.Fatalf("template closing must mention the failure, got:\n%s", out.Content)
	}
}

// 引擎模板策略：全部完成时保留「所有团队已完成任务」收尾句。
func TestSynthesizeTemplate_AllCompleted_KeepsSuccessClosing(t *testing.T) {
	t.Setenv("ARANEA_ENV", "dev")
	e := NewSynthesisEngine(nil, loggateway.NewNoop())
	out, err := e.Synthesize(context.Background(), SynthesisInput{
		Strategy: SynthesisStrategyTemplate,
		TeamResults: []TeamSynthesisResult{
			{TeamID: "t1", TeamName: "团队A", TaskName: "任务A", Status: "completed", Summary: "完成"},
		},
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if !strings.Contains(out.Content, "所有团队已完成任务") {
		t.Fatalf("all-completed template should keep the success closing, got:\n%s", out.Content)
	}
}

// 引擎 prompt 策略：必须要求模型如实反映失败、禁止虚构、汇总未解决问题。
func TestSynthesizePrompt_DemandsHonesty(t *testing.T) {
	e := NewSynthesisEngine(nil, loggateway.NewNoop())
	p := e.synthesizePrompt(SynthesisInput{
		TeamResults: []TeamSynthesisResult{{TeamID: "t1", TeamName: "团队A", Status: "failed"}},
	})
	for _, kw := range []string{"如实", "虚构", "未解决问题"} {
		if !strings.Contains(p, kw) {
			t.Fatalf("synthesis prompt missing honesty requirement %q, got:\n%s", kw, p)
		}
	}
}

// ListFailedTeamBriefs：仅收集 failed 团队（排除 completed/cancelled/其他会话），
// 失败原因取自运行统计，遗留疑问取自团队最后一次回复。
func TestListFailedTeamBriefs_CollectsReasonAndReply(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()

	teams.items["t-fail"] = Team{
		ID: "t-fail", DisplayName: "数据采集团队", TaskDescription: "采集竞品价格",
		Status: TeamStatusFailed, SpiritSessionID: "sp1",
	}
	sessions.sessionsByTeam["t-fail"] = Session{
		ID: "sess-t-fail", TeamID: "t-fail", SessionType: string(SessionTypeTeam),
	}
	steps.stepsBySession["sess-t-fail"] = []Step{
		{ID: "r1", SessionID: "sess-t-fail", Kind: StepKindReply, Status: StepStatusCompleted,
			Content: "需要您澄清：目标竞品名单与时间范围？"},
	}
	teams.items["t-ok"] = Team{ID: "t-ok", DisplayName: "分析团队", Status: TeamStatusCompleted, SpiritSessionID: "sp1"}
	teams.items["t-cancel"] = Team{ID: "t-cancel", DisplayName: "取消团队", Status: TeamStatusCancelled, SpiritSessionID: "sp1"}
	teams.items["t-other"] = Team{ID: "t-other", DisplayName: "其他团队", Status: TeamStatusFailed, SpiritSessionID: "sp2"}

	stats := &stubTeamRunStats{stats: map[string]SpiritTeamRunStats{
		"t-fail": {TeamID: "t-fail", ErrorMessage: "团队未通过 set_deliverable 提交真实交付物"},
	}}
	u := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop(),
		WithSpiritStepReader(steps), WithSpiritTeamRunStatsReader(stats))

	briefs := u.ListFailedTeamBriefs(context.Background(), "sp1")
	if len(briefs) != 1 {
		t.Fatalf("len(briefs) = %d, want 1 (only the failed team of sp1)", len(briefs))
	}
	b := briefs[0]
	if b.TeamName != "数据采集团队" {
		t.Fatalf("TeamName = %q, want 数据采集团队", b.TeamName)
	}
	if b.TaskName != "采集竞品价格" {
		t.Fatalf("TaskName = %q, want 采集竞品价格", b.TaskName)
	}
	if b.Reason != "团队未通过 set_deliverable 提交真实交付物" {
		t.Fatalf("Reason = %q, want the run-stats error message", b.Reason)
	}
	if !strings.Contains(b.LastReply, "需要您澄清") {
		t.Fatalf("LastReply should carry the team's unresolved questions, got %q", b.LastReply)
	}
}

// 无失败团队 → 空切片。
func TestListFailedTeamBriefs_NoFailures_ReturnsEmpty(t *testing.T) {
	teams := newDeliverableTeamRepo()
	teams.items["t1"] = Team{ID: "t1", Status: TeamStatusCompleted, SpiritSessionID: "sp1"}
	sessions := newDeliverableSessionAccessor()
	u := NewSpiritTeamUsecase(teams, sessions, deliverableAgentResolver{}, loggateway.NewNoop())

	if briefs := u.ListFailedTeamBriefs(context.Background(), "sp1"); len(briefs) != 0 {
		t.Fatalf("len(briefs) = %d, want 0", len(briefs))
	}
}
