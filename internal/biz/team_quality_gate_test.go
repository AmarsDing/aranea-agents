package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// G3（ADR-G，2026-08-14）：交付物质量门 verdict —— rule-based v1
//
// 二元门（HasRealDeliverable）只判「有无」，质量门判「内容是否达标」：
//   - J2 充分性：自有交付物文本总长 < 80 runes → revise
//   - J3 占位/拒答标记 → revise
//   - J4 成员中途异常（interrupted session / failed step）→ revise 并点名
//   - 全过 → pass；infra 读错 → error（调用方 fail-open）
// ---------------------------------------------------------------------------

// seedQualityGateTeam 种下一个 enable_state_deliverable 的 DAG 团队及其团队
// session，返回团队记录。成员 session/异常证据由调用方按需补种。
func seedQualityGateTeam(teams *deliverableTeamRepo, sessions *deliverableSessionAccessor) Team {
	return seedStateDeliverableTeam(teams, sessions, nil,
		`{"version":1,"mode":"parallel","enable_state_deliverable":true,"intent_anchor_agent_id":"agent-anchor","members":[{"agent_id":"agent-anchor"},{"agent_id":"agent-m1"}]}`,
		"reply 摘要")
}

func TestEvaluateDeliverableQuality_NonDag_Pass(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	u := newDeliverableUsecase(teams, sessions)

	res, err := u.EvaluateDeliverableQuality(context.Background(), Team{ID: "t-x"})
	if err != nil {
		t.Fatalf("non-DAG team must not error: %v", err)
	}
	if res.Verdict != TeamQualityPass {
		t.Fatalf("non-DAG team verdict=%q want pass", res.Verdict)
	}
}

func TestEvaluateDeliverableQuality_Sufficient_Pass(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"report": strings.Repeat("分析结论与数据支撑。", 12), // 远超 80 runes
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, nil, reader)
	team := seedQualityGateTeam(teams, sessions)

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityPass {
		t.Fatalf("verdict=%q want pass (hits=%v)", res.Verdict, res.RuleHits)
	}
}

func TestEvaluateDeliverableQuality_TooShort_Revise(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"report": "完成了。", // 远不足 80 runes
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, nil, reader)
	team := seedQualityGateTeam(teams, sessions)

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityRevise {
		t.Fatalf("verdict=%q want revise", res.Verdict)
	}
	if !strings.Contains(res.Feedback, "简略") {
		t.Fatalf("feedback should name the sufficiency gap, got %q", res.Feedback)
	}
	if len(res.RuleHits) == 0 {
		t.Fatal("RuleHits must record the triggered rule for MAST annotation")
	}
}

func TestEvaluateDeliverableQuality_Placeholder_Revise(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		// 长度足够但含拒答/占位标记 → J3。
		"report": "初步结论：该需求涉及的数据源当前无法完成接入，详细方案待定，后续补充完整分析。" + strings.Repeat("补充说明。", 10),
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, nil, reader)
	team := seedQualityGateTeam(teams, sessions)

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityRevise {
		t.Fatalf("verdict=%q want revise", res.Verdict)
	}
	if !strings.Contains(res.Feedback, "占位") && !strings.Contains(res.Feedback, "拒答") {
		t.Fatalf("feedback should name the placeholder/refusal marker, got %q", res.Feedback)
	}
}

func TestEvaluateDeliverableQuality_MemberEvidence_Revise(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"report": strings.Repeat("内容充分。", 20),
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, nil, reader)
	team := seedQualityGateTeam(teams, sessions)

	// 成员中途异常：child session interrupted。Get 按 ID 全表扫描，假 key 不入 Search 结果。
	member := Session{ID: "msess-1", MemberAgentKey: "agent-m1", Status: "interrupted", StatusReason: "tool timeout"}
	sessions.sessionsByTeam["member-slot"] = member
	sessions.children = []Session{member}

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityRevise {
		t.Fatalf("verdict=%q want revise (member anomaly must reach the review)", res.Verdict)
	}
	if !strings.Contains(res.Feedback, "agent-m1") {
		t.Fatalf("feedback should name the failed member, got %q", res.Feedback)
	}
}

// P7（2026-08-21 修复）：交付物已提交后的 reply/cancelled 不视为失败证据。
// 场景：成员先 set_deliverable 成功，后续总结 reply 被 cancelled（doom-loop/
// 协调器取消等）——任务实际已完成，J4 不应误判为成员失败。
func TestEvaluateDeliverableQuality_DeliverableSubmitted_ReplyCancelled_Pass(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"report": strings.Repeat("内容充分。", 20),
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)
	team := seedQualityGateTeam(teams, sessions)

	// 团队会话中：成员先有 set_deliverable completed，后有 reply cancelled。
	teamSessID := sessions.sessionsByTeam[team.ID].ID
	memberKey := "agent-m1"
	steps.stepsBySession[teamSessID] = []Step{
		{Kind: StepKindAction, Status: StepStatusCompleted, AuthorAgentKey: memberKey, ToolName: "set_deliverable"},
		{Kind: StepKindReply, Status: StepStatusCancelled, AuthorAgentKey: memberKey, Content: "诗歌创作已完成并提交为结构化交付物"},
	}
	// 成员子 session（coordinator 模式下成员会话 0 steps，证据走团队会话回扫）。
	member := Session{ID: "msess-1", MemberAgentKey: memberKey, ParentSessionID: teamSessID}
	sessions.sessionsByTeam["member-slot"] = member
	sessions.children = []Session{member}

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityPass {
		t.Fatalf("verdict=%q want pass (deliverable submitted, reply cancelled must not flag member failure), hits=%v", res.Verdict, res.RuleHits)
	}
}

// P7 对照组：交付物未提交时，reply/cancelled 仍视为失败证据（保持 J4 原有语义）。
func TestEvaluateDeliverableQuality_NoDeliverable_ReplyCancelled_Revise(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"report": strings.Repeat("内容充分。", 20),
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)
	team := seedQualityGateTeam(teams, sessions)

	teamSessID := sessions.sessionsByTeam[team.ID].ID
	memberKey := "agent-m1"
	// 仅 reply cancelled，无 set_deliverable。
	steps.stepsBySession[teamSessID] = []Step{
		{Kind: StepKindReply, Status: StepStatusCancelled, AuthorAgentKey: memberKey, Content: "任务执行中被取消"},
	}
	member := Session{ID: "msess-1", MemberAgentKey: memberKey, ParentSessionID: teamSessID}
	sessions.sessionsByTeam["member-slot"] = member
	sessions.children = []Session{member}

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityRevise {
		t.Fatalf("verdict=%q want revise (no deliverable + reply cancelled must still flag member failure)", res.Verdict)
	}
	if !strings.Contains(res.Feedback, memberKey) {
		t.Fatalf("feedback should name the failed member, got %q", res.Feedback)
	}
}

// P7 缺口 G3（P2a 自愈场景）：成员先遇 tool-not-found（kind=error step 落在
// 团队会话），自愈后成功 set_deliverable —— 交付事实成立，过程性 error step
// 不得再判成员失败。
func TestEvaluateDeliverableQuality_DeliverableSubmitted_ErrorStep_Pass(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"report": strings.Repeat("内容充分。", 20),
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)
	team := seedQualityGateTeam(teams, sessions)

	teamSessID := sessions.sessionsByTeam[team.ID].ID
	memberKey := "agent-m1"
	steps.stepsBySession[teamSessID] = []Step{
		{Kind: StepKindError, Status: StepStatusFailed, AuthorAgentKey: memberKey, Content: "tool not found: get_team_deliverable"},
		{Kind: StepKindAction, Status: StepStatusCompleted, AuthorAgentKey: memberKey, ToolName: "set_deliverable"},
	}
	member := Session{ID: "msess-1", MemberAgentKey: memberKey, ParentSessionID: teamSessID}
	sessions.sessionsByTeam["member-slot"] = member
	sessions.children = []Session{member}

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityPass {
		t.Fatalf("verdict=%q want pass (deliverable submitted after self-heal; error step must not flag member), hits=%v", res.Verdict, res.RuleHits)
	}
}

// P7 缺口 G2：成员已提交交付物后 session 被中断（超时/清理）——交付事实
// 成立，interrupted 不得再判成员失败。
func TestEvaluateDeliverableQuality_DeliverableSubmitted_Interrupted_Pass(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"report": strings.Repeat("内容充分。", 20),
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)
	team := seedQualityGateTeam(teams, sessions)

	teamSessID := sessions.sessionsByTeam[team.ID].ID
	memberKey := "agent-m1"
	steps.stepsBySession[teamSessID] = []Step{
		{Kind: StepKindAction, Status: StepStatusCompleted, AuthorAgentKey: memberKey, ToolName: "set_deliverable"},
	}
	member := Session{
		ID: "msess-1", MemberAgentKey: memberKey, ParentSessionID: teamSessID,
		Status: "interrupted", StatusReason: "turn timeout after deliverable submit",
	}
	sessions.sessionsByTeam["member-slot"] = member
	sessions.children = []Session{member}

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityPass {
		t.Fatalf("verdict=%q want pass (deliverable submitted; later session interrupt must not flag member), hits=%v", res.Verdict, res.RuleHits)
	}
}

// P7 缺口 G1（非 coordinator 模式）：成员 steps 落在成员会话自身——
// set_deliverable completed 之后又有 action failed，交付事实成立，不得误判。
func TestEvaluateDeliverableQuality_DeliverableInMemberSession_FailedAction_Pass(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	steps := newDeliverableStepReader()
	reader := &graphDeliverableReaderStub{data: map[string]any{
		"report": strings.Repeat("内容充分。", 20),
	}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, steps, reader)
	team := seedQualityGateTeam(teams, sessions)

	teamSessID := sessions.sessionsByTeam[team.ID].ID
	memberKey := "agent-m1"
	// 成员会话自带 steps：先提交交付物，后一个无关工具 action 失败。
	steps.stepsBySession["msess-1"] = []Step{
		{Kind: StepKindAction, Status: StepStatusCompleted, ToolName: "set_deliverable"},
		{Kind: StepKindAction, Status: StepStatusFailed, ToolName: "web_search", Content: "rate limited"},
	}
	member := Session{ID: "msess-1", MemberAgentKey: memberKey, ParentSessionID: teamSessID}
	sessions.sessionsByTeam["member-slot"] = member
	sessions.children = []Session{member}

	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityPass {
		t.Fatalf("verdict=%q want pass (member-session deliverable fact must exempt later failed action), hits=%v", res.Verdict, res.RuleHits)
	}
}

func TestEvaluateDeliverableQuality_ReaderError_ReturnsError(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	reader := &graphDeliverableReaderStub{err: errors.New("state unreadable")}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, nil, reader)
	team := seedQualityGateTeam(teams, sessions)

	if _, err := u.EvaluateDeliverableQuality(context.Background(), team); err == nil {
		t.Fatal("reader infra error must propagate (caller fail-open), got nil")
	}
}

func TestEvaluateDeliverableQuality_EmptyOwn_Pass(t *testing.T) {
	teams := newDeliverableTeamRepo()
	sessions := newDeliverableSessionAccessor()
	reader := &graphDeliverableReaderStub{data: map[string]any{}}
	u := newDeliverableUsecaseWithGraphReader(teams, sessions, nil, reader)
	team := seedQualityGateTeam(teams, sessions)

	// 空交付物由二元门（HasRealDeliverable）先行拦截；质量门不重复否决。
	res, err := u.EvaluateDeliverableQuality(context.Background(), team)
	if err != nil {
		t.Fatalf("EvaluateDeliverableQuality: %v", err)
	}
	if res.Verdict != TeamQualityPass {
		t.Fatalf("empty own deliverable verdict=%q want pass (binary gate owns the veto)", res.Verdict)
	}
}

// ─── 纯函数：文本拍平与规则命中 ─────────────────────────────────────────────

func TestFlattenDeliverableText_ExcludesReservedKeys(t *testing.T) {
	text := flattenDeliverableText(map[string]any{
		"summary":            "结构化摘要",
		"cognition":          "推理过程元数据",
		"ack/member-1":       "确认回执",
		"report":             "正文",
		"nested":             map[string]any{"k": "v"},
	})
	for _, excluded := range []string{"推理过程元数据", "确认回执"} {
		if strings.Contains(text, excluded) {
			t.Fatalf("reserved key content must be excluded, text contains %q", excluded)
		}
	}
	for _, included := range []string{"结构化摘要", "正文"} {
		if !strings.Contains(text, included) {
			t.Fatalf("real content missing from flattened text: %q", included)
		}
	}
}

func TestDeliverableQualityRuleHits_Markers(t *testing.T) {
	cases := map[string]bool{ // text → expect J3 hit
		"详细方案待定":            true,
		"TODO: 补充数据源":       true,
		"此处为占位内容":           true,
		"该部分无法完成":           true,
		"作为AI我无法访问实时数据":    true,
		"正常且充分的分析结论内容描述": false,
		// R1-b（2026-09-01 eval0831-s06-fix1 实证）：「占位符」是描述占位语法的
		// 合法术语（视觉交付物：占位符统一【】包裹待产品信息回填），子串匹配
		// 「占位」不得命中——六团队曾被全量误判 revise。
		"占位符统一【】包裹待产品信息回填":      false,
		"A/B 双方案共用令牌可互换，占位符待回填": false,
	}
	for text, wantHit := range cases {
		hits := deliverableQualityRuleHits(text)
		gotHit := false
		for _, h := range hits {
			if strings.Contains(h, "占位") || strings.Contains(h, "拒答") {
				gotHit = true
			}
		}
		if gotHit != wantHit {
			t.Fatalf("deliverableQualityRuleHits(%q) J3 hit=%v want %v (hits=%v)", text, gotHit, wantHit, hits)
		}
	}
}
