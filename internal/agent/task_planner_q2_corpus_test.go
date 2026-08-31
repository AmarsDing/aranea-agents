package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 包C Q2 语料回归（session-eval-20260827）：QuickAssess 档位 + 组队证据闸
// 的战役话术判别边界。与 biz/task_signal_corpus_test.go 互补——那边钉
// 信号函数，这边钉门控消费结果。

// TestQuickAssess_Q2CampaignCorpus 方向一失准话术（S06/S08）必须升级
// Moderate+；闲聊/直答话术必须保持 Simple（防方向二退化）。
func TestQuickAssess_Q2CampaignCorpus(t *testing.T) {
	impl := &taskPlannerImpl{lg: loggateway.NewNoop()}
	ctx := context.Background()

	forced := map[string]string{
		"S06-t1": "让数字内容媒体公司市场部出一版 Q3 推广文案框架，含三个渠道。",
		"S07-t1": "组织一次新产品上线方案：技术侧给发布计划，内容侧给宣传文案，运营侧给上线 checklist，最后汇总成一份方案。",
	}
	simple := map[string]string{
		"S11-t5荐书": "推荐三本关于分布式系统的书",
		"S01-问候":   "你好",
		"S01-天气":   "今天天气怎么样",
		"S11-漂移":   "对了，你平时喜欢什么音乐",
		// S08 单交付物直求：有任务信号但不强制组队（组织路由靠管理层
		// intent_skip=false，不靠 Spirit ForcePlanning）。
		"S08": "出一条 30 秒产品宣传短视频的创意脚本框架，下周要用。",
		// S09-t1 自我规划：任务信号走 intent pass；组队证据闸与本门对齐，
		// 不得 ForcePlanning（153K 失控根因）。
		"S09-t1": "我们来规划 Q3 的内容运营方案。先搭一个整体框架，包含渠道、节奏、预算三大块，每块简要说明。",
	}
	for tag, msg := range forced {
		level, score, err := impl.QuickAssess(ctx, biz.PlanInput{UserMessage: msg})
		if err != nil {
			t.Fatalf("[%s] QuickAssess error: %v", tag, err)
		}
		if level == biz.ComplexitySimple {
			t.Errorf("[%s] 应升级 Moderate+（方向一失准回归）: level=%s score=%.4f msg=%q", tag, level, score, msg)
		}
	}
	for tag, msg := range simple {
		level, score, err := impl.QuickAssess(ctx, biz.PlanInput{UserMessage: msg})
		if err != nil {
			t.Fatalf("[%s] QuickAssess error: %v", tag, err)
		}
		if level != biz.ComplexitySimple {
			t.Errorf("[%s] 应保持 Simple（方向二退化）: level=%s score=%.4f msg=%q", tag, level, score, msg)
		}
	}
}

// TestHasTeamModeEvidence_Corpus 组队证据闸的证据判别：S07（多实体派发）
// 与显式组队诉求放行；S09-t1（自我规划+内容板块）与 S08（单交付物直求）
// 无证据。
func TestHasTeamModeEvidence_Corpus(t *testing.T) {
	positive := map[string]string{
		"S07-t1": "组织一次新产品上线方案：技术侧给发布计划，内容侧给宣传文案，运营侧给上线 checklist，最后汇总成一份方案。",
		"S06-t1": "让数字内容媒体公司市场部出一版 Q3 推广文案框架，含三个渠道。",
		"显式组队":   "组建两个团队分别调研 A 和 B",
		"显式并行":   "并行分析 A 和 B",
		"组织链":    "这次请按组织链走编制汇报，完成整条软件交付",
		"模式字面量":  "用 dag 模式跑这个复盘任务",
	}
	negative := map[string]string{
		"S09-t1": "我们来规划 Q3 的内容运营方案。先搭一个整体框架，包含渠道、节奏、预算三大块，每块简要说明。",
		"S08":    "出一条 30 秒产品宣传短视频的创意脚本框架，下周要用。",
		"S11-t5": "推荐三本关于分布式系统的书",
	}
	for tag, msg := range positive {
		if !hasTeamModeEvidence(msg) {
			t.Errorf("[%s] 应有组队证据: %q", tag, msg)
		}
	}
	for tag, msg := range negative {
		if hasTeamModeEvidence(msg) {
			t.Errorf("[%s] 不应有组队证据: %q", tag, msg)
		}
	}
}

// TestPlan_TeamModeEvidenceGate 证据闸端到端（Plan 层）：
//   - S09-t1 + llm_mode dag：无证据 → 降级 direct，不触发分解（catalog=nil
//     时若误入分解会落 decompose_failed——用 DecomposeReason 区分两条路径）；
//   - S07 + dag：多实体派发证据 → 放行 → 尝试分解（catalog=nil 必失败，
//     落 decompose_failed 证明闸未拦）；
//   - 显式 agent_keys 路由：豁免证据闸（butler 契约）。
func TestPlan_TeamModeEvidenceGate(t *testing.T) {
	newImpl := func() biz.TaskPlannerPort {
		// catalog=nil + httpClient=nil：分解必失败——分解是否被尝试由
		// DecomposeReason 可观测（decompose_failed=尝试了，空=未尝试）。
		return NewTaskPlanner(&stubTaskPlanRepo{}, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil)
	}

	t.Run("S09-t1 dag 无证据降级 direct", func(t *testing.T) {
		plan, err := newImpl().Plan(context.Background(), biz.PlanInput{
			SpiritSessionID: "sp-q2-gate",
			UserMessage:     "我们来规划 Q3 的内容运营方案。先搭一个整体框架，包含渠道、节奏、预算三大块，每块简要说明。",
			Mode:            "dag",
		})
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if plan.Strategy != biz.StrategyDirect {
			t.Fatalf("无证据 dag 应降级 direct，got strategy=%s", plan.Strategy)
		}
		if plan.DecomposeReason != "" {
			t.Fatalf("降级后不得尝试分解，got decompose_reason=%q", plan.DecomposeReason)
		}
	})

	t.Run("S07 dag 多实体派发证据放行", func(t *testing.T) {
		plan, err := newImpl().Plan(context.Background(), biz.PlanInput{
			SpiritSessionID: "sp-q2-gate",
			UserMessage:     "组织一次新产品上线方案：技术侧给发布计划，内容侧给宣传文案，运营侧给上线 checklist，最后汇总成一份方案。",
			Mode:            "dag",
		})
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		// 证据放行 → 尝试分解 → catalog=nil 失败 → 既有失败降级路径。
		if plan.DecomposeReason != "decompose_failed" {
			t.Fatalf("有证据 dag 应尝试分解（decompose_failed 证明未被闸拦），got %q", plan.DecomposeReason)
		}
	})

	t.Run("显式 agent_keys 豁免证据闸", func(t *testing.T) {
		plan, err := newImpl().Plan(context.Background(), biz.PlanInput{
			SpiritSessionID: "sp-q2-gate",
			UserMessage:     "安装一个新技能",
			Mode:            "parallel", // 工具入口键路由升级后的形态
			AgentKeys:       []string{"__system_admin__"},
		})
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if plan.DecomposeReason != "decompose_failed" {
			t.Fatalf("agent_keys 路由不得被证据闸拦截，got decompose_reason=%q", plan.DecomposeReason)
		}
	})

	t.Run("S06 empty mode + committed PlanTeam 不得 silent direct", func(t *testing.T) {
		msg := "让数字内容媒体公司市场部出一版 Q3 推广文案框架，含三个渠道。"
		committed := CommitRoute(biz.ComplexityModerate, true, 0.4, "gate", msg)
		if committed.Lane != biz.RouteLanePlanTeam {
			t.Fatalf("precondition: S06 must commit plan_team, got %s", committed.Lane)
		}
		ctx := biz.ContextWithRouteDecision(context.Background(), committed)
		plan, err := newImpl().Plan(ctx, biz.PlanInput{
			SpiritSessionID: "sp-s06-commit",
			UserMessage:     msg,
		})
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if err := biz.CheckRouteHonored(committed, plan.Strategy, len(plan.SubTasks), plan.DecomposeReason); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("keyword_fallback 不受闸影响", func(t *testing.T) {
		// mode 为空但消息含组队意图：detectTeamIntent 命中即证据。
		plan, err := newImpl().Plan(context.Background(), biz.PlanInput{
			SpiritSessionID: "sp-q2-gate",
			UserMessage:     "组建两个团队分别调研 A 和 B",
			Mode:            "",
		})
		if err != nil {
			t.Fatalf("Plan: %v", err)
		}
		if plan.DecomposeReason != "decompose_failed" {
			t.Fatalf("keyword_fallback 组队应尝试分解，got decompose_reason=%q", plan.DecomposeReason)
		}
	})
}
