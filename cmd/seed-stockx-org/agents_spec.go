package main

import (
	"encoding/json"
	"os"
	"strings"

	"aranea-agents/internal/biz"
)

type agentSpec struct {
	agentKey          string
	displayName       string
	description       string
	positionKey       string
	provider          string
	model             string
	roleTitle         string
	roleKey           string
	teamRole          string
	taskBrief         string
	outputContract    string
	toolsAllow        []string
	toolsDeny         []string
	skillsAllow       []string
	toolsProfile      string
	systemPromptMode  string
	contextWindow     int
	subagentsEnabled  bool
	toolsParallel     bool
	codeExecutor      string
	maxOutputTokens   int
	modelTier         string // fast | medium | strong
}

type catNode struct {
	key, name, description, level, parentKey string
	sortOrder                                int
	roleConfig                               map[string]any
}

type seedPlan struct {
	categories []catNode
	agents     []agentSpec
}

func buildPlan(fastProvider, fastModel, strongProvider, strongModel string) seedPlan {
	const (
		companyKey   = "stockx-company"
		deptCoord    = "stockx-dept-coordination"
		deptData     = "stockx-dept-data"
		deptResearch = "stockx-dept-research"
		deptOutput   = "stockx-dept-output"
	)

	commonDeny := []string{"workspace_exec", "filesystem", "shell", "bash"}

	categories := []catNode{
		{
			key: "stockx-company", name: "Stockx AI 投研",
			description: "Daily Stock Analysis 场景 — AI 股票分析投研团队",
			level: "industry", sortOrder: 10,
			roleConfig: map[string]any{
				"scenario": "daily_stock_analysis", "namespace": "stockx",
			},
		},
		{
			key: deptCoord, name: "调度管理部",
			description: "任务拆分、团队调度与报告评审",
			level: "department", parentKey: companyKey, sortOrder: 10,
			roleConfig: map[string]any{"scenario": "stockx", "department": "coordination"},
		},
		{
			key: deptData, name: "数据采集部",
			description: "行情、财务、资金、新闻等数据统一采集与归一化",
			level: "department", parentKey: companyKey, sortOrder: 20,
			roleConfig: map[string]any{"scenario": "stockx", "department": "data"},
		},
		{
			key: deptResearch, name: "多维分析部",
			description: "技术、基本面、资金、消息、情绪、行业、风险与因子分析",
			level: "department", parentKey: companyKey, sortOrder: 30,
			roleConfig: map[string]any{"scenario": "stockx", "department": "research"},
		},
		{
			key: deptOutput, name: "报告输出部",
			description: "图表构建、报告撰写与多渠道推送",
			level: "department", parentKey: companyKey, sortOrder: 40,
			roleConfig: map[string]any{"scenario": "stockx", "department": "output"},
		},
	}

	type posDef struct {
		key, name, dept, desc, roleKey, agentKey, teamRole string
		order                                              int
		tools                                              []string
		skills                                             []string
	}

	positions := []posDef{
		{"stockx-pos-coordinator", "主控调度员", deptCoord, "任务拆分、调度成员 Agent、决定是否需要追问", "coordinator", "agent-coordinator", "coordinator", 10,
			nil, []string{"stock-task-planning", "stock-report-template"}},
		{"stockx-pos-critic", "评审员", deptCoord, "对报告草稿打分、提出修改意见", "critic", "agent-critic", "critic", 20,
			nil, []string{"stock-report-critic"}},
		{"stockx-pos-data-collector", "数据采集员", deptData, "统一拉取行情/财务/资金/新闻/公告数据，归一化输出", "data_collector", "agent-data-collector", "worker", 10,
			[]string{"stock_quote_realtime", "stock_quote_history", "stock_fundamental_overview", "stock_news_individual", "stock_money_flow_individual", "stock_announcement"},
			[]string{"stock-data-normalize"}},
		{"stockx-pos-technical-analyst", "技术分析师", deptResearch, "K 线形态、均线、MACD/KDJ/RSI、量价关系、趋势/支撑/压力", "technical_analyst", "agent-technical-analyst", "worker", 10,
			[]string{"stock_quote_history", "indicator_compute", "chart_render_kline"},
			[]string{"stock-technical-patterns", "stock-chart-style"}},
		{"stockx-pos-fundamental-analyst", "基本面分析师", deptResearch, "财报、ROE/PE/PB/PEG、盈利质量、行业地位", "fundamental_analyst", "agent-fundamental-analyst", "worker", 20,
			[]string{"stock_fundamental_overview", "finance_statement_income", "finance_statement_balance", "finance_statement_cashflow", "finance_indicator"},
			[]string{"stock-financial-ratio"}},
		{"stockx-pos-money-flow-analyst", "资金面分析师", deptResearch, "北向资金、龙虎榜、主力净流入、融资融券", "money_flow_analyst", "agent-money-flow-analyst", "worker", 30,
			[]string{"hsgt_flow", "dragon_tiger", "stock_money_flow_individual", "margin_balance"},
			[]string{"stock-fund-signal"}},
		{"stockx-pos-news-analyst", "消息面分析师", deptResearch, "公司公告、研报、政策、行业新闻、突发事件", "news_analyst", "agent-news-analyst", "worker", 40,
			[]string{"stock_announcement", "stock_news_individual", "research_report_search", "policy_search", "web_search"},
			[]string{"stock-news-classify"}},
		{"stockx-pos-sentiment-analyst", "情绪面分析师", deptResearch, "雪球/股吧舆情、热度趋势、关键词共现", "sentiment_analyst", "agent-sentiment-analyst", "worker", 50,
			[]string{"stock_sentiment_xueqiu", "stock_sentiment_guba", "web_search"},
			[]string{"stock-sentiment-score"}},
		{"stockx-pos-industry-analyst", "行业分析师", deptResearch, "行业景气、产业链上下游、板块轮动、政策驱动", "industry_analyst", "agent-industry-analyst", "worker", 60,
			[]string{"industry_classification", "industry_money_flow", "stock_concept_list"},
			[]string{"stock-industry-chain"}},
		{"stockx-pos-risk-assessor", "风险评估师", deptResearch, "波动率、最大回撤、Beta、集中度风险、ST/退市预警", "risk_assessor", "agent-risk-assessor", "worker", 70,
			[]string{"risk_metric_compute", "backtest_simple", "stock_quote_history"},
			[]string{"stock-risk-framework"}},
		{"stockx-pos-quant-factor", "因子计算员", deptResearch, "多因子计算（动量/反转/质量/价值/盈利预期）", "quant_factor", "agent-quant-factor", "worker", 80,
			[]string{"indicator_compute", "factor_compute", "codeexecutor"},
			[]string{"stock-factor-lib"}},
		{"stockx-pos-chart-builder", "图表构建员", deptOutput, "生成 K 线图、财务图表、组合热力图", "chart_builder", "agent-chart-builder", "worker", 10,
			[]string{"chart_render_kline", "chart_render_pie", "chart_render_heatmap", "codeexecutor"},
			[]string{"stock-chart-style"}},
		{"stockx-pos-report-writer", "报告撰写员", deptOutput, "把多 Agent 输出汇总为结构化 Markdown / 飞书卡片", "report_writer", "agent-report-writer", "synthesizer", 20,
			[]string{"artifact_save", "channel_push"},
			[]string{"stock-report-template"}},
	}

	posByKey := map[string]posDef{}
	for _, p := range positions {
		posByKey[p.key] = p
		categories = append(categories, catNode{
			key: p.key, name: p.name, description: p.desc,
			level: "position", parentKey: p.dept, sortOrder: p.order,
			roleConfig: map[string]any{
				"scenario":           "stockx",
				"role_key":           p.roleKey,
				"expected_agent_key": p.agentKey,
				"team_role":          p.teamRole,
				"tools_allow":        p.tools,
				"skills_allow":       p.skills,
			},
		})
	}

	specs := []agentSpec{
		coordinatorSpec(fastProvider, fastModel, strongProvider, strongModel, commonDeny),
		criticSpec(fastProvider, fastModel, commonDeny),
		dataCollectorSpec(fastProvider, fastModel, commonDeny),
		technicalAnalystSpec(fastProvider, fastModel, commonDeny),
		fundamentalAnalystSpec(fastProvider, fastModel, strongProvider, strongModel, commonDeny),
		moneyFlowAnalystSpec(fastProvider, fastModel, commonDeny),
		newsAnalystSpec(fastProvider, fastModel, commonDeny),
		sentimentAnalystSpec(fastProvider, fastModel, commonDeny),
		industryAnalystSpec(fastProvider, fastModel, commonDeny),
		riskAssessorSpec(fastProvider, fastModel, commonDeny),
		quantFactorSpec(fastProvider, fastModel, strongProvider, strongModel, commonDeny),
		chartBuilderSpec(fastProvider, fastModel, commonDeny),
		reportWriterSpec(fastProvider, fastModel, strongProvider, strongModel, commonDeny),
	}

	return seedPlan{categories: categories, agents: specs}
}

func pickModel(tier, fastP, fastM, strongP, strongM string) (string, string) {
	if tier == "strong" {
		return strongP, strongM
	}
	return fastP, fastM
}

func coordinatorSpec(fastP, fastM, strongP, strongM string, deny []string) agentSpec {
	p, m := pickModel("strong", fastP, fastM, strongP, strongM)
	return agentSpec{
		agentKey: "agent-coordinator", displayName: "主控调度员", positionKey: "stockx-pos-coordinator",
		description: "Daily Stock Analysis 主控：任务拆分、调度成员 Agent、整合输出",
		provider: p, model: m, roleTitle: "主控调度员", roleKey: "coordinator", teamRole: "coordinator",
		taskBrief:      "理解用户意图，拆分投研任务，按顺序或并行调度数据采集员与各分析师，必要时追问澄清",
		outputContract: "先输出任务拆分 JSON（steps + assigned_agents），再调用成员；最终整合为统一回复",
		toolsAllow:     nil,
		toolsDeny:      deny,
		skillsAllow:    []string{"stock-task-planning", "stock-report-template"},
		toolsProfile:   "coordinator", systemPromptMode: "complete", contextWindow: 128000,
		subagentsEnabled: true, maxOutputTokens: 0,
	}
}

func criticSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-critic", displayName: "评审员", positionKey: "stockx-pos-critic",
		description: "报告质量评审：结构完整性、数据引用、风险提示",
		provider: fastP, model: fastM, roleTitle: "评审员", roleKey: "critic", teamRole: "critic",
		taskBrief:      "对报告草稿打分（1-10）并给出可执行的修改意见",
		outputContract: "## 评分\n## 问题清单\n## 修改建议\n## 是否通过",
		toolsDeny: deny, skillsAllow: []string{"stock-report-critic"},
		toolsProfile: "minimal", systemPromptMode: "task", contextWindow: 64000, maxOutputTokens: 800,
	}
}

func dataCollectorSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-data-collector", displayName: "数据采集员", positionKey: "stockx-pos-data-collector",
		description: "拉取并归一化行情/财务/资金/新闻/公告数据",
		provider: fastP, model: fastM, roleTitle: "数据采集员", roleKey: "data_collector", teamRole: "worker",
		taskBrief: "按 coordinator 指令拉取指定 symbol 的多维原始数据，输出归一化 JSON",
		outputContract: "## 数据摘要\n## 字段清单（symbol/market/as_of）\n## 原始数据块（JSON）\n## 缺失项说明",
		toolsAllow: []string{"stock_quote_realtime", "stock_quote_history", "stock_fundamental_overview",
			"stock_news_individual", "stock_money_flow_individual", "stock_announcement"},
		toolsDeny: deny, skillsAllow: []string{"stock-data-normalize"},
		toolsProfile: "stock_data", systemPromptMode: "task", contextWindow: 64000,
		toolsParallel: true, maxOutputTokens: 1200,
	}
}

func technicalAnalystSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-technical-analyst", displayName: "技术分析师", positionKey: "stockx-pos-technical-analyst",
		description: "技术面：趋势、形态、支撑压力、量价配合",
		provider: fastP, model: fastM, roleTitle: "技术分析师", roleKey: "technical_analyst", teamRole: "worker",
		taskBrief: "基于历史行情与指标计算结果，给出技术面结论",
		outputContract: "## 趋势判断\n## 形态识别\n## 关键支撑/压力\n## 量价配合\n## 短期方向\n每条结论附 data_ref",
		toolsAllow: []string{"stock_quote_history", "indicator_compute", "chart_render_kline"},
		toolsDeny: deny, skillsAllow: []string{"stock-technical-patterns", "stock-chart-style"},
		toolsProfile: "stock_analyst", systemPromptMode: "task", contextWindow: 128000, maxOutputTokens: 800,
	}
}

func fundamentalAnalystSpec(fastP, fastM, strongP, strongM string, deny []string) agentSpec {
	p, m := pickModel("strong", fastP, fastM, strongP, strongM)
	return agentSpec{
		agentKey: "agent-fundamental-analyst", displayName: "基本面分析师", positionKey: "stockx-pos-fundamental-analyst",
		description: "基本面：财报、估值、盈利质量、行业地位",
		provider: p, model: m, roleTitle: "基本面分析师", roleKey: "fundamental_analyst", teamRole: "worker",
		taskBrief: "解读三大报表与关键财务指标，评估估值与盈利质量",
		outputContract: "## 公司概览\n## 财务健康\n## 估值水平（PE/PB/PEG）\n## 盈利质量\n## 行业地位",
		toolsAllow: []string{"stock_fundamental_overview", "finance_statement_income", "finance_statement_balance",
			"finance_statement_cashflow", "finance_indicator"},
		toolsDeny: deny, skillsAllow: []string{"stock-financial-ratio"},
		toolsProfile: "stock_analyst", systemPromptMode: "task", contextWindow: 128000, modelTier: "strong", maxOutputTokens: 800,
	}
}

func moneyFlowAnalystSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-money-flow-analyst", displayName: "资金面分析师", positionKey: "stockx-pos-money-flow-analyst",
		description: "资金面：北向、龙虎榜、主力流向、两融",
		provider: fastP, model: fastM, roleTitle: "资金面分析师", roleKey: "money_flow_analyst", teamRole: "worker",
		taskBrief: "分析资金流入流出、北向与龙虎榜信号",
		outputContract: "## 北向/南向\n## 主力净流入\n## 龙虎榜\n## 两融\n## 资金面结论",
		toolsAllow: []string{"hsgt_flow", "dragon_tiger", "stock_money_flow_individual", "margin_balance"},
		toolsDeny: deny, skillsAllow: []string{"stock-fund-signal"},
		toolsProfile: "stock_analyst", systemPromptMode: "task", contextWindow: 64000, maxOutputTokens: 800,
	}
}

func newsAnalystSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-news-analyst", displayName: "消息面分析师", positionKey: "stockx-pos-news-analyst",
		description: "消息面：公告、研报、政策、行业新闻",
		provider: fastP, model: fastM, roleTitle: "消息面分析师", roleKey: "news_analyst", teamRole: "worker",
		taskBrief: "收集并分类公告、新闻、研报与政策，评估事件影响",
		outputContract: "## 重要公告\n## 新闻与政策\n## 研报摘要\n## 事件影响评估",
		toolsAllow: []string{"stock_announcement", "stock_news_individual", "research_report_search", "policy_search", "web_search"},
		toolsDeny: deny, skillsAllow: []string{"stock-news-classify"},
		toolsProfile: "stock_analyst", systemPromptMode: "task", contextWindow: 64000, maxOutputTokens: 800,
	}
}

func sentimentAnalystSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-sentiment-analyst", displayName: "情绪面分析师", positionKey: "stockx-pos-sentiment-analyst",
		description: "情绪面：雪球/股吧舆情与热度",
		provider: fastP, model: fastM, roleTitle: "情绪面分析师", roleKey: "sentiment_analyst", teamRole: "worker",
		taskBrief: "量化舆情热度与情感倾向，识别异常波动",
		outputContract: "## 热度趋势\n## 情感分布\n## 关键词\n## 情绪结论",
		toolsAllow: []string{"stock_sentiment_xueqiu", "stock_sentiment_guba", "web_search"},
		toolsDeny: deny, skillsAllow: []string{"stock-sentiment-score"},
		toolsProfile: "stock_analyst", systemPromptMode: "task", contextWindow: 64000, maxOutputTokens: 800,
	}
}

func industryAnalystSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-industry-analyst", displayName: "行业分析师", positionKey: "stockx-pos-industry-analyst",
		description: "行业：景气、产业链、板块轮动",
		provider: fastP, model: fastM, roleTitle: "行业分析师", roleKey: "industry_analyst", teamRole: "worker",
		taskBrief: "分析行业景气度、产业链位置与板块资金轮动",
		outputContract: "## 行业景气\n## 产业链\n## 板块轮动\n## 政策驱动",
		toolsAllow: []string{"industry_classification", "industry_money_flow", "stock_concept_list"},
		toolsDeny: deny, skillsAllow: []string{"stock-industry-chain"},
		toolsProfile: "stock_analyst", systemPromptMode: "task", contextWindow: 64000, maxOutputTokens: 800,
	}
}

func riskAssessorSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-risk-assessor", displayName: "风险评估师", positionKey: "stockx-pos-risk-assessor",
		description: "风险：波动、回撤、Beta、集中度、ST 预警",
		provider: fastP, model: fastM, roleTitle: "风险评估师", roleKey: "risk_assessor", teamRole: "worker",
		taskBrief: "量化风险指标并给出风险等级与预警项",
		outputContract: "## 波动与回撤\n## Beta/相关性\n## 集中度\n## ST/退市预警\n## 风险等级",
		toolsAllow: []string{"risk_metric_compute", "backtest_simple", "stock_quote_history"},
		toolsDeny: deny, skillsAllow: []string{"stock-risk-framework"},
		toolsProfile: "stock_analyst", systemPromptMode: "task", contextWindow: 64000, maxOutputTokens: 800,
	}
}

func quantFactorSpec(fastP, fastM, strongP, strongM string, deny []string) agentSpec {
	p, m := pickModel("strong", fastP, fastM, strongP, strongM)
	return agentSpec{
		agentKey: "agent-quant-factor", displayName: "因子计算员", positionKey: "stockx-pos-quant-factor",
		description: "量化因子：动量/价值/质量/盈利预期",
		provider: p, model: m, roleTitle: "因子计算员", roleKey: "quant_factor", teamRole: "worker",
		taskBrief: "计算多因子得分并解释因子暴露",
		outputContract: "## 因子得分\n## 因子暴露\n## 计算方法\n## 局限性",
		toolsAllow: []string{"indicator_compute", "factor_compute", "codeexecutor"},
		toolsDeny: deny, skillsAllow: []string{"stock-factor-lib"},
		toolsProfile: "stock_quant", systemPromptMode: "task", contextWindow: 128000,
		codeExecutor: "local", modelTier: "strong", maxOutputTokens: 1000,
	}
}

func chartBuilderSpec(fastP, fastM string, deny []string) agentSpec {
	return agentSpec{
		agentKey: "agent-chart-builder", displayName: "图表构建员", positionKey: "stockx-pos-chart-builder",
		description: "K 线、财务与组合图表生成",
		provider: fastP, model: fastM, roleTitle: "图表构建员", roleKey: "chart_builder", teamRole: "worker",
		taskBrief: "根据数据生成 K 线/饼图/热力图，返回 artifact 引用",
		outputContract: "## 图表清单\n## artifact_id 列表\n## 图表说明",
		toolsAllow: []string{"chart_render_kline", "chart_render_pie", "chart_render_heatmap", "codeexecutor"},
		toolsDeny: deny, skillsAllow: []string{"stock-chart-style"},
		toolsProfile: "stock_chart", systemPromptMode: "task", contextWindow: 64000,
		codeExecutor: "local", toolsParallel: true, maxOutputTokens: 600,
	}
}

func reportWriterSpec(fastP, fastM, strongP, strongM string, deny []string) agentSpec {
	p, m := pickModel("strong", fastP, fastM, strongP, strongM)
	return agentSpec{
		agentKey: "agent-report-writer", displayName: "报告撰写员", positionKey: "stockx-pos-report-writer",
		description: "汇总多维分析并输出 Markdown/飞书卡片",
		provider: p, model: m, roleTitle: "报告撰写员", roleKey: "report_writer", teamRole: "synthesizer",
		taskBrief: "按报告类型选择模板，整合各分析师输出，注入数据引用与免责声明",
		outputContract: "## TL;DR\n## 分维度分析\n## 数据引用\n## 风险提示\n## 附录（图表）",
		toolsAllow: []string{"artifact_save", "channel_push"},
		toolsDeny: deny, skillsAllow: []string{"stock-report-template"},
		toolsProfile: "stock_report", systemPromptMode: "complete", contextWindow: 128000,
		modelTier: "strong", maxOutputTokens: 0,
	}
}

func (spec agentSpec) toBizAgent(posID, createdBy string) biz.Agent {
	settings := biz.DefaultAgentRuntimeSettings()
	settings.ToolsEnabled = len(spec.toolsAllow) > 0 || spec.subagentsEnabled
	settings.ToolsProfile = spec.toolsProfile
	if spec.toolsProfile == "" {
		settings.ToolsProfile = "full"
	}
	settings.ToolsAllowJSON = jsonStringList(spec.toolsAllow)
	settings.ToolsDenyJSON = jsonStringList(spec.toolsDeny)
	settings.ToolsParallelEnabled = spec.toolsParallel
	settings.SubagentsEnabled = spec.subagentsEnabled
	settings.SkillLoadMode = "manual"
	settings.SkillRuntimeJSON = skillRuntimeJSON(spec.skillsAllow...)
	settings.IntentPassEnabled = true
	settings.CodeExecutorType = spec.codeExecutor
	if settings.CodeExecutorType == "" {
		settings.CodeExecutorType = "local"
	}
	settings.ContextCompactionEnabled = true
	settings.SessionSummaryEnabled = true

	mode := spec.systemPromptMode
	if mode == "" {
		mode = "task"
	}
	cw := spec.contextWindow
	if cw == 0 {
		cw = 64000
	}

	return biz.Agent{
		AgentKey:           spec.agentKey,
		DisplayName:        spec.displayName,
		Provider:           spec.provider,
		Model:              spec.model,
		AgentDescription:   spec.description,
		CategoryPositionID: posID,
		Status:             "active",
		SystemPromptMode:   mode,
		ContextWindow:      cw,
		Roles:              []string{spec.roleKey, spec.teamRole, "stockx"},
		Settings:           &settings,
		Files:              buildPromptFiles(spec),
		CreatedBy:          createdBy,
	}
}

func jsonStringList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(items)
	return string(b)
}

func skillRuntimeJSON(allow ...string) string {
	if len(allow) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(map[string]any{"allowed_slugs": allow})
	return string(b)
}

func roleConfigJSON(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
