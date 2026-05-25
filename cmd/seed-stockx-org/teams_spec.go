package main

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

type teamSpec struct {
	teamKey     string
	displayName string
	description string
	spec        biz.OrchestrationSpec
}

func buildTeamSpecs(ids agentIDMap) ([]teamSpec, error) {
	if err := ids.require(
		"agent-coordinator",
		"agent-critic",
		"agent-data-collector",
		"agent-technical-analyst",
		"agent-fundamental-analyst",
		"agent-money-flow-analyst",
		"agent-news-analyst",
		"agent-sentiment-analyst",
		"agent-industry-analyst",
		"agent-risk-assessor",
		"agent-quant-factor",
		"agent-chart-builder",
		"agent-report-writer",
	); err != nil {
		return nil, err
	}

	coord := ids.id("agent-coordinator")
	critic := ids.id("agent-critic")
	collector := ids.id("agent-data-collector")
	technical := ids.id("agent-technical-analyst")
	fundamental := ids.id("agent-fundamental-analyst")
	moneyFlow := ids.id("agent-money-flow-analyst")
	news := ids.id("agent-news-analyst")
	sentiment := ids.id("agent-sentiment-analyst")
	industry := ids.id("agent-industry-analyst")
	risk := ids.id("agent-risk-assessor")
	quant := ids.id("agent-quant-factor")
	chart := ids.id("agent-chart-builder")
	report := ids.id("agent-report-writer")

	scenarioDesc := "Daily Stock Analysis (stockx) — "

	teams := []teamSpec{
		{
			teamKey: "team-premarket-brief", displayName: "盘前简报团队",
			description: "盘前 30 分钟自选股简报（Coordinator）",
			spec: biz.OrchestrationSpec{
				Version: biz.OrchestrationSpecVersion, Mode: "coordinator",
				RuntimeEngine: "graph", TeamGraphRuntime: true,
				Description: scenarioDesc + "coordinator 调度采集 + 技术/资金/消息分析 → 报告撰写",
				MaxConcurrency: 3, TimeoutSeconds: 300, LoopMaxIterations: 1,
				SynthesizerAgentID: report,
				IntentAnchorAgentID: coord,
				Members: []biz.OrchestrationMember{
					member(coord, "coordinator", "主控调度", 0),
					member(collector, "worker", "数据采集", 10, "拉取自选股盘前行情/公告/资金快照"),
					member(news, "worker", "消息面", 20, "检索昨晚至今晨关键新闻与公告"),
					member(moneyFlow, "worker", "资金面", 30, "北向资金与龙虎榜摘要"),
					member(technical, "worker", "技术面", 40, "关键个股技术形态与支撑压力"),
					member(report, "synthesizer", "报告撰写", 90, "生成盘前简报 Markdown/飞书卡片"),
				},
			},
		},
		{
			teamKey: "team-stock-deep-dive", displayName: "个股深度分析团队",
			description: "多维并行深度分析（Coordinator 版，Graph 编排待 Phase 6）",
			spec: biz.OrchestrationSpec{
				Version: biz.OrchestrationSpecVersion, Mode: "coordinator",
				RuntimeEngine: "graph", TeamGraphRuntime: true,
				Description: scenarioDesc + "全维度分析师协作 + 图表 + 综合报告",
				MaxConcurrency: 4, TimeoutSeconds: 900, LoopMaxIterations: 2,
				SynthesizerAgentID: report,
				IntentAnchorAgentID: coord,
				Members: []biz.OrchestrationMember{
					member(coord, "coordinator", "主控调度", 0),
					member(collector, "worker", "数据采集", 10),
					member(technical, "worker", "技术面", 20),
					member(fundamental, "worker", "基本面", 30),
					member(moneyFlow, "worker", "资金面", 40),
					member(news, "worker", "消息面", 50),
					member(sentiment, "worker", "情绪面", 60),
					member(industry, "worker", "行业面", 70),
					member(risk, "worker", "风险评估", 80),
					member(chart, "worker", "图表构建", 85),
					member(report, "synthesizer", "报告撰写", 90),
				},
			},
		},
		{
			teamKey: "team-sector-rotation", displayName: "板块扫描团队",
			description: "周板块轮动扫描（Sequential）",
			spec: biz.OrchestrationSpec{
				Version: biz.OrchestrationSpecVersion, Mode: "sequential",
				RuntimeEngine: "graph", TeamGraphRuntime: true,
				Description: scenarioDesc + "行业 → 资金 → 消息 → 报告，逐步累积上下文",
				MaxConcurrency: 1, TimeoutSeconds: 600,
				SynthesizerAgentID: report,
				Members: []biz.OrchestrationMember{
					member(industry, "worker", "行业分析", 10, "板块景气与产业链"),
					member(moneyFlow, "worker", "板块资金", 20, "行业/概念资金流向"),
					member(news, "worker", "政策与新闻", 30, "政策驱动与板块新闻"),
					member(report, "synthesizer", "扫描报告", 90, "输出板块扫描 Markdown"),
				},
				Graph: linearGraph("sector_rotation", []graphStep{
					{"start", "start", "开始", "", ""},
					{"industry", "agent", "行业分析", industry, "worker"},
					{"flow", "agent", "板块资金", moneyFlow, "worker"},
					{"news", "agent", "政策新闻", news, "worker"},
					{"report", "agent", "扫描报告", report, "synthesizer"},
					{"end", "end", "结束", "", ""},
				}),
			},
		},
		{
			teamKey: "team-portfolio-doctor", displayName: "持仓诊断团队",
			description: "组合并行诊断（Parallel + Synthesizer）",
			spec: biz.OrchestrationSpec{
				Version: biz.OrchestrationSpecVersion, Mode: "parallel",
				RuntimeEngine: "graph", TeamGraphRuntime: true,
				Description: scenarioDesc + "风险/基本面/因子并行 → 组合诊断报告",
				MaxConcurrency: 3, TimeoutSeconds: 900,
				SynthesizerAgentID: report,
				Members: []biz.OrchestrationMember{
					member(risk, "worker", "风险评估", 10, "波动、回撤、集中度"),
					member(fundamental, "worker", "基本面", 20, "持仓标的财务与估值"),
					member(quant, "worker", "因子分析", 30, "多因子暴露与风格"),
					member(report, "synthesizer", "诊断报告", 90, "汇总为组合诊断报告"),
				},
				Graph: parallelGraph("portfolio_doctor", []string{risk, fundamental, quant}, report),
			},
		},
		{
			teamKey: "team-market-recap", displayName: "盘后复盘团队",
			description: "收盘后复盘流水线（Sequential）",
			spec: biz.OrchestrationSpec{
				Version: biz.OrchestrationSpecVersion, Mode: "sequential",
				RuntimeEngine: "graph", TeamGraphRuntime: true,
				Description: scenarioDesc + "行情/资金/板块/消息 → 图表 → 复盘长报",
				MaxConcurrency: 1, TimeoutSeconds: 900,
				SynthesizerAgentID: report,
				Members: []biz.OrchestrationMember{
					member(technical, "worker", "行情快照", 10, "大盘与重点标的收盘技术概况"),
					member(moneyFlow, "worker", "资金复盘", 20, "全天资金流向与北向"),
					member(industry, "worker", "板块复盘", 30, "行业涨跌与轮动"),
					member(news, "worker", "异动与关注", 40, "异动股与明日关注事件"),
					member(chart, "worker", "复盘图表", 50, "K 线与板块热力图"),
					member(report, "synthesizer", "复盘报告", 90, "盘后复盘 Markdown 长报"),
				},
				Graph: linearGraph("market_recap", []graphStep{
					{"start", "start", "开始", "", ""},
					{"tech", "agent", "行情快照", technical, "worker"},
					{"flow", "agent", "资金复盘", moneyFlow, "worker"},
					{"sector", "agent", "板块复盘", industry, "worker"},
					{"news", "agent", "异动关注", news, "worker"},
					{"chart", "agent", "复盘图表", chart, "worker"},
					{"report", "agent", "复盘报告", report, "synthesizer"},
					{"end", "end", "结束", "", ""},
				}),
			},
		},
		{
			teamKey: "team-research-pipeline", displayName: "自定义研究流水线",
			description: "可编辑研究流水线模板（Sequential 入门版）",
			spec: biz.OrchestrationSpec{
				Version: biz.OrchestrationSpecVersion, Mode: "sequential",
				RuntimeEngine: "graph", TeamGraphRuntime: true,
				Description: scenarioDesc + "采集 → 行业/基本面 → 风险 → 报告（可在 UI 扩展为 Graph）",
				MaxConcurrency: 2, TimeoutSeconds: 1200, EnableCheckpoint: true,
				SynthesizerAgentID: report,
				Members: []biz.OrchestrationMember{
					member(collector, "worker", "数据采集", 10),
					member(industry, "worker", "行业分析", 20),
					member(fundamental, "worker", "基本面", 30),
					member(risk, "worker", "风险评估", 40),
					member(report, "synthesizer", "研究报告", 90),
				},
				Graph: linearGraph("research_pipeline", []graphStep{
					{"start", "start", "开始", "", ""},
					{"collect", "agent", "数据采集", collector, "worker"},
					{"industry", "agent", "行业分析", industry, "worker"},
					{"fund", "agent", "基本面", fundamental, "worker"},
					{"risk", "agent", "风险评估", risk, "worker"},
					{"report", "agent", "研究报告", report, "synthesizer"},
					{"end", "end", "结束", "", ""},
				}),
			},
		},
		{
			teamKey: "team-deep-dive-critic", displayName: "深度分析·评审精修",
			description: "深度报告 + critic_loop 二次精修（可选）",
			spec: biz.OrchestrationSpec{
				Version: biz.OrchestrationSpecVersion, Mode: "critic_loop",
				RuntimeEngine: "graph", TeamGraphRuntime: true,
				Description: scenarioDesc + "report_writer 生成初稿 → critic 评审迭代",
				MaxConcurrency: 1, TimeoutSeconds: 900,
				CriticLoop: &biz.CriticLoopSpec{MaxIterations: 2, ScoreThreshold: 0.8},
				Members: []biz.OrchestrationMember{
					member(report, "generator", "报告生成", 10, "基于上游多维分析输出初稿"),
					member(critic, "critic", "报告评审", 20, "结构/数据引用/风险提示评审"),
				},
			},
		},
	}

	return teams, nil
}

type agentIDMap map[string]string

func (m agentIDMap) id(key string) string { return m[key] }

func (m agentIDMap) require(keys ...string) error {
	for _, k := range keys {
		if strings.TrimSpace(m[k]) == "" {
			return fmt.Errorf("missing agent %q — run agent seed first", k)
		}
	}
	return nil
}

func member(agentID, role, name string, order int, taskPrompt ...string) biz.OrchestrationMember {
	m := biz.OrchestrationMember{
		AgentID: agentID, Role: role, Name: name,
		Enabled: true, SortOrder: order,
	}
	if len(taskPrompt) > 0 {
		m.TaskPrompt = taskPrompt[0]
	}
	return m
}

type graphStep struct {
	id, typ, label, agentID, role string
}

func linearGraph(layout string, steps []graphStep) *biz.EmbeddedGraphSpec {
	nodes := make([]biz.EmbeddedGraphNodeSpec, 0, len(steps))
	edges := make([]biz.EmbeddedGraphEdgeSpec, 0, len(steps)-1)
	for _, s := range steps {
		n := biz.EmbeddedGraphNodeSpec{ID: s.id, Type: s.typ, Label: s.label}
		if s.agentID != "" {
			n.AgentID = s.agentID
			n.Role = s.role
		}
		nodes = append(nodes, n)
	}
	for i := 0; i+1 < len(steps); i++ {
		edges = append(edges, biz.EmbeddedGraphEdgeSpec{
			ID: fmt.Sprintf("e-%s-%s", steps[i].id, steps[i+1].id),
			Source: steps[i].id, Target: steps[i+1].id,
		})
	}
	return &biz.EmbeddedGraphSpec{Version: 1, Layout: layout, Nodes: nodes, Edges: edges}
}

func parallelGraph(layout string, workers []string, synthesizer string) *biz.EmbeddedGraphSpec {
	nodes := []biz.EmbeddedGraphNodeSpec{
		{ID: "start", Type: "start", Label: "开始"},
		{ID: "join", Type: "join", Label: "汇总"},
		{ID: "report", Type: "agent", Label: "诊断报告", AgentID: synthesizer, Role: "synthesizer"},
		{ID: "end", Type: "end", Label: "结束"},
	}
	edges := []biz.EmbeddedGraphEdgeSpec{
		{ID: "e-start-join", Source: "start", Target: "join"},
		{ID: "e-join-report", Source: "join", Target: "report"},
		{ID: "e-report-end", Source: "report", Target: "end"},
	}
	for i, wid := range workers {
		nid := fmt.Sprintf("w%d", i+1)
		nodes = append(nodes, biz.EmbeddedGraphNodeSpec{
			ID: nid, Type: "agent", Label: fmt.Sprintf("Worker %d", i+1),
			AgentID: wid, Role: "worker",
		})
		edges = append(edges,
			biz.EmbeddedGraphEdgeSpec{ID: fmt.Sprintf("e-start-%s", nid), Source: "start", Target: nid},
			biz.EmbeddedGraphEdgeSpec{ID: fmt.Sprintf("e-%s-join", nid), Source: nid, Target: "join"},
		)
	}
	return &biz.EmbeddedGraphSpec{Version: 1, Layout: layout, Nodes: nodes, Edges: edges}
}
