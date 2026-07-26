package biz_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent/skillinvocation"
	"aranea-agents/internal/data/ent/skillversion"
	skillpkg "aranea-agents/internal/skill"
)

// ── T1 自我进化：CuratorFlow → LLM draft → 审批 → Reload 新版本 ──────────────
//
// 链路：10 条失败调用 → 健康度触发（30d 失败率>30%）→ RunCuratorFlow →
// LLMSkillEvolver 生成改进草稿（fake LLM）→ gate 通过 → lifecycle=ready →
// ApproveSuggestion → ApplyApprovedSuggestion → SkillVersionReloader 注册新版本。
func TestSkillSelfEvolution_EndToEnd(t *testing.T) {
	env := newSkillFuncEnv(t)
	ctx := context.Background()

	const skillID = "skill_t1"
	oldBody := "---\nname: code_review\ndescription: 代码评审\n---\n# Code Review\n\nOLD_BODY_MARKER 直接评审，无重试无校验\n"
	_, seedVersionID := seedSkill(t, env, skillID, "code_review", "Code Review", "代码评审技能", oldBody, []string{"review"})
	seedFailingInvocations(t, env, skillID, 10) // ≥10 且 100% 失败 → 触发 fix_failure

	// 可编程 LLM：返回带规则块改进的草稿，并记录请求证据。
	fakeLLM := &fakeLLMCaller{handler: func(req biz.LLMCallRequest) (string, error) {
		return "---\nname: code_review\ndescription: 代码评审（进化版）\n---\n# Code Review Evolved\n\n## 失败处理\n- 增加超时重试与参数校验（LLM_IMPROVEMENT_MARKER）\n", nil
	}}

	aggregator := data.NewSkillIntelligenceRepo(env.d, env.lg)
	scorer := biz.NewSkillScoringUsecase(aggregator, env.lg)
	unified := data.NewUnifiedEvolutionRepo(env.d, env.lg)
	skillRepo := data.NewSkillRepo(env.d)
	evolver := biz.NewLLMSkillEvolver(fakeLLM, skillRepo, "fake-provider", "fake-model", env.lg)
	reloader := biz.NewSkillVersionReloader(skillRepo, skillRepo, env.lg)
	uc := biz.NewSkillIntelligenceUsecase(scorer, nil, unified, aggregator, env.lg,
		biz.SkillIntelligenceConfig{Gate: passGate{}, Evolver: evolver, Reloader: reloader})

	// 1) CuratorFlow：触发 + LLM 草稿 + sandbox + lifecycle。
	suggestion, err := uc.RunCuratorFlow(ctx, skillID)
	if err != nil {
		t.Fatalf("RunCuratorFlow: %v", err)
	}
	if suggestion == nil {
		t.Fatal("RunCuratorFlow returned nil suggestion, want health-triggered evolution")
	}
	if suggestion.Type != biz.EvoSuggestionFixFailure {
		t.Errorf("type = %q, want %q", suggestion.Type, biz.EvoSuggestionFixFailure)
	}
	if suggestion.LifecycleStatus != biz.EvoLifecycleReady {
		t.Errorf("lifecycle = %q, want %q (LLM draft + gate pass)", suggestion.LifecycleStatus, biz.EvoLifecycleReady)
	}
	if !suggestion.SandboxPassed {
		t.Error("sandbox_passed = false, want true")
	}
	if !strings.Contains(suggestion.DraftSkillBody, "LLM_IMPROVEMENT_MARKER") {
		t.Errorf("draft missing LLM improvement marker, got:\n%s", suggestion.DraftSkillBody)
	}

	// 2) LLM 证据：确实调用了 Curator LLM，且 prompt 携带当前 skill 正文。
	if fakeLLM.callCount() != 1 {
		t.Fatalf("LLM call count = %d, want 1", fakeLLM.callCount())
	}
	llmReq := fakeLLM.requests[0]
	if !strings.Contains(llmReq.System, "Curator") && !strings.Contains(llmReq.System, "进化策展人") {
		t.Errorf("LLM system prompt missing Curator role: %q", llmReq.System)
	}
	if !strings.Contains(llmReq.User, "OLD_BODY_MARKER") {
		t.Error("LLM user prompt missing current skill body")
	}
	if !strings.Contains(llmReq.User, "failure rate") {
		t.Error("LLM user prompt missing trigger reason")
	}

	// 3) 审批。
	if err := uc.ApproveSuggestion(ctx, suggestion.ID, "tester"); err != nil {
		t.Fatalf("ApproveSuggestion: %v", err)
	}

	// 4) Reload：注册为新版本。
	if err := uc.ApplyApprovedSuggestion(ctx, suggestion.ID); err != nil {
		t.Fatalf("ApplyApprovedSuggestion: %v", err)
	}

	// 5) DB 证据：新版本已生效，parent_version_id 指向旧版本。
	latest, err := env.client.SkillVersion.Query().
		Where(skillversion.SkillIDEQ(skillID)).
		Order(skillversion.ByCreatedAt()).
		All(ctx)
	if err != nil {
		t.Fatalf("query skill versions: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("skill version count = %d, want 2 (old + evolved)", len(latest))
	}
	newVer := latest[1]
	if newVer.ContentMarkdown != suggestion.DraftSkillBody {
		t.Errorf("new version body mismatch:\n got %q\nwant %q", newVer.ContentMarkdown, suggestion.DraftSkillBody)
	}
	if newVer.ParentVersionID != seedVersionID {
		t.Errorf("parent_version_id = %q, want %q", newVer.ParentVersionID, seedVersionID)
	}
	if newVer.EvolutionReason == "" {
		t.Error("evolution_reason is empty on new version")
	}

	// 6) 建议状态：applied + lifecycle applied。
	row, err := unified.GetByID(ctx, suggestion.ID)
	if err != nil || row == nil {
		t.Fatalf("unified GetByID: row=%v err=%v", row, err)
	}
	if row.Status != string(biz.UnifiedEvolutionStateApplied) {
		t.Errorf("unified status = %q, want applied", row.Status)
	}
	if row.LifecycleStatus != string(biz.EvoLifecycleApplied) {
		t.Errorf("unified lifecycle = %q, want applied", row.LifecycleStatus)
	}

	// 7) 生效内容可读：GetLatestSkillMarkdown 返回进化后正文。
	md, err := skillRepo.GetLatestSkillMarkdown(ctx, skillID)
	if err != nil {
		t.Fatalf("GetLatestSkillMarkdown: %v", err)
	}
	if !strings.Contains(md, "LLM_IMPROVEMENT_MARKER") {
		t.Error("latest skill markdown is not the evolved draft")
	}
	t.Logf("T1 OK: skill evolved via LLM draft, new version %s (parent %s)", newVer.ID, newVer.ParentVersionID)
}

// ── T2 对话内容创建：observations → patterns → proposal → 注册 SKILL.md ──────
//
// 链路：对话中沉淀的工具调用观测 → LearningLoop.DetectPatterns 检测高频模式 →
// SkillEvolutionUsecase.DetectAndPropose 用真实 SkillAutoCreator(fake LLM) 生成
// SKILL.md → ApproveProposal → RegisterApproved → DB 中可解析的 SKILL.md。
func TestSkillAutoCreate_FromConversation(t *testing.T) {
	env := newSkillFuncEnv(t)
	ctx := context.Background()
	const agentID = "agent_t2"
	seedAgent(t, env, agentID)

	// 真实 repos + usecase 组装。
	obsRepo := data.NewObservationRepo(env.d)
	patRepo := data.NewPatternRepo(env.d)
	propRepo := data.NewProposalRepo(env.d)
	agentRepo := data.NewAgentRepo(env.d)
	learningLoop := biz.NewLearningLoopUsecase(obsRepo, patRepo, propRepo, agentRepo, env.lg)

	fakeLLM := &fakeLLMCaller{handler: func(req biz.LLMCallRequest) (string, error) {
		return "NAME: web_research\n" +
			"---\nname: web_research\ndescription: 对话中高频 web_search 调用沉淀出的技能\n---\n" +
			"# Web Research\n\n## triggers\n- 用户要求搜索资料时触发\n\n## steps\n1. 调用 web_search\n2. 汇总结果\n", nil
	}}
	creator := skillpkg.NewSkillAutoCreator(skillpkg.NewLLMCallerAdapter(fakeLLM, "fake", "fake-model"), env.lg)
	registrar := &dbSkillRegistrar{client: env.client}
	unified := data.NewUnifiedEvolutionRepo(env.d, env.lg)
	evoUC := biz.NewSkillEvolutionUsecase(unified, unified, patRepo, agentRepo, creator, registrar, env.lg)

	// 1) 对话观测：3 次 web_search + 1 次 fetch_page（≥3 次同 kind → 形成模式）。
	obs := []biz.Observation{
		{AgentID: agentID, SessionID: "sess_1", Kind: biz.ObservationKindToolCall, Content: "搜索量子计算最新进展", Metadata: `{"tool_name":"web_search"}`, ObservedAt: time.Now().UTC()},
		{AgentID: agentID, SessionID: "sess_1", Kind: biz.ObservationKindToolCall, Content: "搜索 aranea 框架文档", Metadata: `{"tool_name":"web_search"}`, ObservedAt: time.Now().UTC()},
		{AgentID: agentID, SessionID: "sess_2", Kind: biz.ObservationKindToolCall, Content: "搜索竞品动态", Metadata: `{"tool_name":"web_search"}`, ObservedAt: time.Now().UTC()},
		{AgentID: agentID, SessionID: "sess_2", Kind: biz.ObservationKindToolCall, Content: "抓取页面正文", Metadata: `{"tool_name":"fetch_page"}`, ObservedAt: time.Now().UTC()},
	}
	if err := learningLoop.RecordObservations(ctx, obs); err != nil {
		t.Fatalf("RecordObservations: %v", err)
	}

	// 2) 模式检测。
	patterns, err := learningLoop.DetectPatterns(ctx, agentID)
	if err != nil {
		t.Fatalf("DetectPatterns: %v", err)
	}
	if len(patterns) != 1 {
		t.Fatalf("patterns = %d, want 1", len(patterns))
	}
	if patterns[0].Kind != string(biz.ObservationKindToolCall) {
		t.Errorf("pattern kind = %q, want tool_call", patterns[0].Kind)
	}
	if !strings.Contains(patterns[0].Description, "web_search(3)") {
		t.Errorf("pattern description missing tool frequency: %q", patterns[0].Description)
	}
	if patterns[0].Confidence < 0.15 {
		t.Errorf("pattern confidence = %f, want >= 0.15", patterns[0].Confidence)
	}

	// 3) 生成提案（真实 SkillAutoCreator + fake LLM）。
	proposals, err := evoUC.DetectAndPropose(ctx, agentID)
	if err != nil {
		t.Fatalf("DetectAndPropose: %v", err)
	}
	if len(proposals) != 1 {
		t.Fatalf("proposals = %d, want 1", len(proposals))
	}
	prop := proposals[0]
	if prop.SkillName != "web_research" {
		t.Errorf("skill name = %q, want web_research", prop.SkillName)
	}
	if !strings.HasPrefix(strings.TrimSpace(prop.SkillMD), "---") {
		t.Errorf("SKILL.md must start with YAML front matter, got:\n%s", prop.SkillMD)
	}
	if prop.Status != biz.SkillProposalStatusPending {
		t.Errorf("proposal status = %q, want pending", prop.Status)
	}
	// LLM 证据：prompt 携带模式描述与工具历史。
	if fakeLLM.callCount() != 1 {
		t.Fatalf("LLM call count = %d, want 1", fakeLLM.callCount())
	}
	if !strings.Contains(fakeLLM.requests[0].User, "web_search") {
		t.Error("creator prompt missing detected tool name")
	}

	// 4) 审批 → 注册。
	if _, err := evoUC.ApproveProposal(ctx, prop.ID, "tester"); err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	registered, err := evoUC.RegisterApproved(ctx, prop.ID)
	if err != nil {
		t.Fatalf("RegisterApproved: %v", err)
	}
	if registered.Status != biz.SkillProposalStatusRegistered {
		t.Errorf("registered status = %q, want registered", registered.Status)
	}

	// 5) DB 证据：skill 已注册，SKILL.md 可解析（frontmatter + name 字段）。
	exists, err := registrar.SkillExists(ctx, agentID, "web_research")
	if err != nil || !exists {
		t.Fatalf("registered skill not found in DB: exists=%v err=%v", exists, err)
	}
	skillRepo := data.NewSkillRepo(env.d)
	skillEnt, err := skillRepo.GetSkillBySkillKey(ctx, "web_research")
	if err != nil {
		t.Fatalf("GetSkillBySkillKey: %v", err)
	}
	md, err := skillRepo.GetLatestSkillMarkdown(ctx, skillEnt.ID)
	if err != nil {
		t.Fatalf("GetLatestSkillMarkdown: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(md), "---") || !strings.Contains(md, "name: web_research") {
		t.Errorf("registered SKILL.md not parseable frontmatter:\n%s", md)
	}
	if !strings.Contains(md, "## triggers") || !strings.Contains(md, "## steps") {
		t.Error("registered SKILL.md missing required sections (triggers/steps)")
	}
	t.Logf("T2 OK: skill %q auto-created from conversation pattern %q", prop.SkillName, patterns[0].Description)
}

// ── T3 去重 ──────────────────────────────────────────────────────────────────

// T3a patternHash 去重：同一模式重复 DetectAndPropose 不重复生成提案。
// T3b SkillExists 预检：推断名已注册时跳过 LLM 生成。
func TestSkillDedup_ProposalLevel(t *testing.T) {
	ctx := context.Background()

	newEvoUC := func(t *testing.T, env *skillFuncEnv, fakeLLM *fakeLLMCaller) *biz.SkillEvolutionUsecase {
		patRepo := data.NewPatternRepo(env.d)
		agentRepo := data.NewAgentRepo(env.d)
		creator := skillpkg.NewSkillAutoCreator(skillpkg.NewLLMCallerAdapter(fakeLLM, "fake", "fake-model"), env.lg)
		unified := data.NewUnifiedEvolutionRepo(env.d, env.lg)
		return biz.NewSkillEvolutionUsecase(unified, unified, patRepo, agentRepo, creator, &dbSkillRegistrar{client: env.client}, env.lg)
	}
	seedDetectedPattern := func(t *testing.T, env *skillFuncEnv, agentID, desc string) {
		t.Helper()
		_, err := data.NewPatternRepo(env.d).Create(ctx, biz.Pattern{
			ID:          "pat_" + agentID,
			AgentID:     agentID,
			Kind:        string(biz.ObservationKindToolCall),
			Description: desc,
			Frequency:   5,
			Confidence:  0.8,
			Evidence:    "[]",
			Status:      biz.PatternStatusDetected,
			DetectedAt:  time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("seed pattern: %v", err)
		}
	}

	t.Run("patternHash 去重", func(t *testing.T) {
		env := newSkillFuncEnv(t)
		const agentID = "agent_t3a"
		seedAgent(t, env, agentID)
		seedDetectedPattern(t, env, agentID, "web_search(5), fetch_page(3)")
		fakeLLM := &fakeLLMCaller{handler: func(req biz.LLMCallRequest) (string, error) {
			return "NAME: web_search\n---\nname: web_search\ndescription: x\n---\n# Web Search\n", nil
		}}
		evoUC := newEvoUC(t, env, fakeLLM)

		first, err := evoUC.DetectAndPropose(ctx, agentID)
		if err != nil || len(first) != 1 {
			t.Fatalf("first DetectAndPropose: n=%d err=%v, want 1 proposal", len(first), err)
		}
		second, err := evoUC.DetectAndPropose(ctx, agentID)
		if err != nil {
			t.Fatalf("second DetectAndPropose: %v", err)
		}
		if len(second) != 0 {
			t.Errorf("second DetectAndPropose = %d proposals, want 0 (patternHash dedup)", len(second))
		}
		if fakeLLM.callCount() != 1 {
			t.Errorf("LLM calls = %d, want 1 (second run must not regenerate)", fakeLLM.callCount())
		}
	})

	t.Run("SkillExists 预检", func(t *testing.T) {
		env := newSkillFuncEnv(t)
		const agentID = "agent_t3b"
		seedAgent(t, env, agentID)
		// 模式描述推断名为 web_search（第一个 "(" 前的部分）。
		seedDetectedPattern(t, env, agentID, "web_search(5), fetch_page(3)")
		// 预注册同名 skill → 应跳过 LLM 生成。
		registrar := &dbSkillRegistrar{client: env.client}
		if err := registrar.RegisterSkill(ctx, agentID, "web_search", "---\nname: web_search\n---\n# existing\n"); err != nil {
			t.Fatalf("pre-register skill: %v", err)
		}
		fakeLLM := &fakeLLMCaller{handler: func(req biz.LLMCallRequest) (string, error) {
			return "NAME: web_search\n---\nname: web_search\n---\n# dup\n", nil
		}}
		evoUC := newEvoUC(t, env, fakeLLM)

		proposals, err := evoUC.DetectAndPropose(ctx, agentID)
		if err != nil {
			t.Fatalf("DetectAndPropose: %v", err)
		}
		if len(proposals) != 0 {
			t.Errorf("proposals = %d, want 0 (skill already exists)", len(proposals))
		}
		if fakeLLM.callCount() != 0 {
			t.Errorf("LLM calls = %d, want 0 (SkillExists pre-check must short-circuit)", fakeLLM.callCount())
		}
	})
}

// T3c DetectDuplicateGroups：真实 repo + Jaccard 相似度引擎检测重复 skill 分组。
func TestSkillDedup_DetectDuplicateGroups(t *testing.T) {
	env := newSkillFuncEnv(t)
	ctx := context.Background()

	bodyA := "---\nname: web-search-assistant\n---\n# 网页搜索助手\n\n## 步骤\n1. 接收查询\n2. 调用 web_search\n3. 总结结果\n"
	bodyB := "---\nname: web-search-assistant-v2\n---\n# 网页搜索助手\n\n## 步骤\n1. 接收查询\n2. 调用 web_search\n3. 总结结果并输出\n"
	seedSkill(t, env, "skill_dup_a", "web-search-a", "网页搜索助手", "用于搜索网页并总结结果的助手技能", bodyA, []string{"搜索", "网页"})
	seedSkill(t, env, "skill_dup_b", "web-search-b", "网页搜索助手", "用于搜索网页并总结结果的助手技能", bodyB, []string{"搜索", "网页"})
	seedSkill(t, env, "skill_distinct", "image-gen", "图像生成专家", "根据文本描述生成图像", "# 图像生成\n\n## 步骤\n1. 解析提示词\n2. 调用图像模型\n", []string{"图像", "生成"})

	dedupUC := biz.NewSkillDedupUsecase(
		data.NewSkillDedupRepo(env.d, env.lg),
		biz.NewSkillSimilarityEngine(nil, biz.DefaultDedupWeights(), env.lg),
		env.lg,
	)
	groups, err := dedupUC.DetectDuplicateGroups(ctx)
	if err != nil {
		t.Fatalf("DetectDuplicateGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("duplicate groups = %d, want 1", len(groups))
	}
	g := groups[0]
	if len(g.Skills) != 2 {
		t.Fatalf("group size = %d, want 2", len(g.Skills))
	}
	ids := map[string]bool{g.Skills[0].ID: true, g.Skills[1].ID: true}
	if !ids["skill_dup_a"] || !ids["skill_dup_b"] {
		t.Errorf("group members = %v, want skill_dup_a + skill_dup_b", ids)
	}
	if g.OverlapScore < 0.5 {
		t.Errorf("overlap score = %f, want >= 0.5 (dedup threshold)", g.OverlapScore)
	}
	if ids["skill_distinct"] {
		t.Error("distinct skill must not be grouped as duplicate")
	}
	t.Logf("T3c OK: duplicate group %s score=%.3f risk=%s rec=%s", g.GroupID, g.OverlapScore, g.ConflictRisk, g.Recommendation)
}

// ── T4 融合提升：rule_fuse 融合草稿 + 真实 merge 两 skill 合一 ────────────────
//
// 链路：RuleBasedContentFuser 生成融合草稿（source 独有段落并入 target）→
// gate 通过 → ApplyMerge 事务：target 新版本 + source 废弃 + 调用记录转移 +
// 标签并集。
func TestSkillMerge_FuseAndApply(t *testing.T) {
	env := newSkillFuncEnv(t)
	ctx := context.Background()

	targetBody := "---\nname: data_analysis\n---\n# 数据分析技能\n\n## 触发条件\n用户要求分析数据时触发\n\n## 步骤\n1. 读取数据文件\n2. 统计分析\n"
	sourceBody := "---\nname: data_analysis_pro\n---\n# 数据分析技能增强版\n\n## 触发条件\n用户要求分析数据时触发\n\n## 可视化\n1. 自动生成图表\n2. 支持 matplotlib 输出\n"
	const targetID, sourceID = "skill_merge_t", "skill_merge_s"
	seedSkill(t, env, targetID, "data-analysis", "数据分析", "基础数据分析", targetBody, []string{"数据分析"})
	seedSkill(t, env, sourceID, "data-analysis-pro", "数据分析Pro", "增强数据分析", sourceBody, []string{"数据分析", "可视化"})
	for i := 0; i < 3; i++ {
		seedInvocation(t, env, "inv_merge_"+string(rune('a'+i)), sourceID, "success", "completed", 80, time.Now().UTC())
	}

	mergeRepo := data.NewSkillMergeRepo(env.d, env.lg)
	fuser := biz.NewRuleBasedContentFuser()

	// 1) 融合草稿证据：source 独有的「## 可视化」段落并入，标签并集。
	targetSrc, err := mergeRepo.GetFullSkillForMerge(ctx, targetID)
	if err != nil {
		t.Fatalf("GetFullSkillForMerge(target): %v", err)
	}
	sourceSrc, err := mergeRepo.GetFullSkillForMerge(ctx, sourceID)
	if err != nil {
		t.Fatalf("GetFullSkillForMerge(source): %v", err)
	}
	draft, err := fuser.Fuse(ctx, *targetSrc, *sourceSrc)
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	if !strings.Contains(draft.Body, "## 可视化") {
		t.Errorf("fused draft missing source-unique section:\n%s", draft.Body)
	}
	if !strings.Contains(draft.Body, "Merged from") {
		t.Error("fused draft missing merge provenance marker")
	}
	if len(draft.Tags) != 2 {
		t.Errorf("fused tags = %v, want union of 2", draft.Tags)
	}

	// 2) 真实 merge。
	mergeUC := biz.NewSkillMergeUsecase(mergeRepo, mergeRepo, fuser, passGate{}, env.lg)
	result, err := mergeUC.Merge(ctx, biz.SkillMergeRequest{
		SourceID: sourceID,
		TargetID: targetID,
		Strategy: biz.MergeStrategyRuleFuse,
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if result.NewVersionID == "" {
		t.Error("NewVersionID is empty")
	}
	if result.TransferredCount != 3 {
		t.Errorf("TransferredCount = %d, want 3", result.TransferredCount)
	}
	if result.FusedBody != draft.Body {
		t.Error("merge result body differs from fuser draft")
	}

	// 3) DB 证据：target 新 published 版本内容 == 融合草稿。
	newVer, err := env.client.SkillVersion.Get(ctx, result.NewVersionID)
	if err != nil {
		t.Fatalf("get new version: %v", err)
	}
	if newVer.SkillID != targetID || newVer.Status != "published" {
		t.Errorf("new version skill=%s status=%s, want target/published", newVer.SkillID, newVer.Status)
	}
	if newVer.ContentMarkdown != draft.Body {
		t.Error("persisted version body != fused draft")
	}

	// 4) DB 证据：source 已废弃（disabled + deprecated + deleted_at）。
	sourceEnt, err := env.client.PlatformSkill.Get(ctx, sourceID)
	if err != nil {
		t.Fatalf("get source skill: %v", err)
	}
	if sourceEnt.Enabled || sourceEnt.Status != "deprecated" || sourceEnt.DeletedAt == "" {
		t.Errorf("source not deprecated: enabled=%v status=%s deleted_at=%q", sourceEnt.Enabled, sourceEnt.Status, sourceEnt.DeletedAt)
	}

	// 5) DB 证据：调用记录已转移到 target。
	transferred, err := env.client.SkillInvocation.Query().
		Where(skillinvocation.SkillIDEQ(targetID)).
		Count(ctx)
	if err != nil {
		t.Fatalf("count target invocations: %v", err)
	}
	if transferred != 3 {
		t.Errorf("target invocations = %d, want 3 (transferred)", transferred)
	}

	// 6) DB 证据：target 标签并入 source 的「可视化」。
	targetEnt, err := env.client.PlatformSkill.Get(ctx, targetID)
	if err != nil {
		t.Fatalf("get target skill: %v", err)
	}
	if !strings.Contains(targetEnt.MetadataJSON, "可视化") {
		t.Errorf("target metadata missing merged tag 可视化: %s", targetEnt.MetadataJSON)
	}
	t.Logf("T4 OK: merged %s → %s, new version %s, %d invocations transferred", sourceID, targetID, result.NewVersionID, result.TransferredCount)
}
