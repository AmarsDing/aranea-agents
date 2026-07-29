// GOAI 赛事（零人工运维方向）环境一键搭建脚本。
// 幂等可重入：已存在的岗位/Agent/Skill 自动跳过。
// 功能：
//  1. 在「智能运维部」下创建 12 个运维岗位（含岗位职责说明）
//  2. 创建部门主管 Agent 并回写 organizations.dept_lead_agent_id
//     （绕过平台缺陷：data 层 agent_repo.CreateAgent 强制要求 provider/model，
//     而 DeptLeadManager.CreateDeptLead 不设置二者导致自动创建失败）
//  3. 创建 12 个预设运维 Agent（完整定义：角色/职责/工具白名单/文档格式/报表格式）
//  4. 创建并启用运维域 Skill（RCA/告警诊断/系统巡检/运维报告）
//go:build ignore

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"

	_ "github.com/lib/pq"
)

const (
	baseURL = "http://localhost:8000"
	deptID  = "fd1e7d9e-bf9c-4916-b53a-f9062424645e" // 智能运维部
	deptKey = "aiops"
	dsn     = "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
)

// ---------- HTTP helper ----------

type client struct {
	hc *http.Client
}

func newClient() *client {
	jar, _ := cookiejar.New(nil)
	return &client{hc: &http.Client{Jar: jar}}
}

func (c *client) do(method, path string, body any) (int, []byte) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, rdr)
	if err != nil {
		fatal("build request", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		fatal(method+" "+path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func fatal(step string, err error) {
	fmt.Fprintf(os.Stderr, "FATAL %s: %v\n", step, err)
	os.Exit(1)
}

// ---------- 岗位定义 ----------

type positionDef struct {
	Key  string
	Name string
	Desc string
}

var positions = []positionDef{
	{"ops_alarm_handler", "告警处理工程师", "负责平台告警的接入响应、严重度评估（P1-P4）与处理建议输出；产出《告警分析单》（摘要/评估/原因/建议/后续跟进）；联动告警确认、通知与工单创建。"},
	{"ops_fault_diagnosis", "故障诊断工程师", "负责故障症状分析、根因定位（含置信度）、排查步骤与解决方案输出；产出《故障诊断报告》；为 RCA 根因分析场景提供诊断结论。"},
	{"ops_log_analysis", "日志分析工程师", "负责系统/应用日志的错误模式识别、异常事件检测与性能问题挖掘；产出《日志分析报告》（错误模式/异常时间线/性能问题/改进建议）。"},
	{"ops_system_inspection", "系统巡检工程师", "负责服务器资源使用与服务状态周期性评估，识别风险项并输出优化建议；产出《系统巡检报告》（资源阈值表/服务状态/风险项/优化建议）。"},
	{"ops_change_execution", "变更执行工程师", "负责变更方案的步骤化执行、执行结果验证与失败回滚；产出《变更执行记录》（执行步骤/验证结果/回滚方案/结论）；高危操作必经人工审批。"},
	{"ops_doc_generation", "运维文档工程师", "负责将运维过程与结果沉淀为标准 Markdown 运维报告与知识文档；维护统一报告模板与写作规范；联动知识库沉淀（knowledge.create）。"},
	{"ops_compliance_check", "合规检查工程师", "负责安全基线与合规项验证，输出不符合项清单（严重度/证据/影响）与修复建议；产出《合规检查报告》与复查计划。"},
	{"ops_server_command", "服务器命令执行工程师", "负责在目标服务器执行命令并分析执行结果（一切命令经 TwinMonitor-MCP-Server → 16 运维工具，本人不持有设备凭据）；产出《命令执行记录》。"},
	{"ops_command_expert", "命令生成专家", "按自然语言意图与目标 OS（Linux/Windows/网络设备）生成安全命令；仅返回 JSON {command, explanation}；拒绝危险命令并说明理由。"},
	{"ops_auto_inspection", "自动巡检工程师", "负责多服务器批量巡检的编排、执行与结果汇总；产出《批量巡检汇总报告》（概览/各机明细/共性问题/个别风险/建议）。"},
	{"ops_network_inspection", "网络巡检专家", "负责华为/H3C/Cisco/锐捷/中兴网络设备巡检（标准/自定义/全面三模式）；阈值评估（CPU<70 正常/70-85 关注/>85 告警；内存<75/75-90/>90）；产出《网络巡检报告》。"},
	{"ops_database", "数据库运维工程师", "负责数据库健康检查、慢查询诊断、安全审计、死锁分析与 SQL 审核；产出《数据库运维报告》（健康/慢查询/安全/死锁/审核结论/优化建议）。"},
}

// ---------- Agent 定义 ----------

type agentDef struct {
	Key      string
	Name     string
	Icon     string
	Desc     string
	PosKey   string
	Category string
	Risk     string
	Temp     float64
	Profile  string
	Allow    []string
	MCPTools []string
	Prompt   string
}

const promptPreamble = `你是「IT 运维行业 → 智能运维部」的预设运维 Agent，服务于 TwinMonitor 集中监控数字孪生平台。

## 平台与边界（必须遵守）

- 平台全部设备/数据能力经 **TwinMonitor-MCP-Server** 以 MCP 工具形式开放，风险五级（readonly/low/medium/high/destructive），默认只读。
- **一切设备命令的唯一出口是 16 运维工具**（经 MCP ` + "`server.exec_command`" + ` 等转发）；你不持有任何设备凭据，禁止尝试直连 SSH/数据库。
- high/destructive 级工具调用必须经人工审批（HITL）后才可执行；未获审批时输出待审批说明。
- 分析结论必须基于证据（告警详情/指标/日志/知识库条目），禁止编造数据；证据不足时明确标注「证据缺口」。
- 知识沉淀：有价值的结论经 ` + "`knowledge.create`" + ` 写入知识库（10 模块），供后续检索复用。

## 协作关系

- 你受智能运维部部门主管协调，与部门内其他 11 个预设运维 Agent 协作完成「告警 → 诊断 → 建议 → 审批 → 执行 → 验证 → 沉淀」闭环。
- 需要其他专业能力时，在输出末尾给出「建议转交」说明（目标岗位 + 原因），不越权执行他人职责。

`

func buildPrompt(role, duty, outputSpec, extra string) string {
	return promptPreamble + "## 角色定位\n\n" + role + "\n\n## 职责\n\n" + duty + "\n\n## 输出格式（严格执行）\n\n" + outputSpec + "\n" + extra
}

var agents = []agentDef{
	{
		Key: "ops_alarm_handler", Name: "告警处理 Agent", Icon: "🚨", PosKey: "ops_alarm_handler",
		Desc: "分析告警、评估严重程度、给出处理建议（摘要/评估/原因/建议/后续）",
		Category: "alarm", Risk: "low", Temp: 0.7, Profile: "read_only",
		Allow:    []string{"memory_search", "skill_search", "web_fetch"},
		MCPTools: []string{"alarm.query", "alarm.get", "alarm.severity_assess", "alarm.acknowledge", "knowledge.search", "notify.send", "ticket.create", "ticket.get_oncall"},
		Prompt: buildPrompt(
			"智能运维部的告警处理专家，第一时间响应平台告警，完成严重度评估与处置建议输出。",
			"1. 经 `alarm.query`/`alarm.get` 获取告警详情与关联告警；\n2. 结合资产信息（`asset.get`）与历史处置经验（`knowledge.search`）评估严重度；\n3. 输出《告警分析单》；\n4. 需要时确认告警（`alarm.acknowledge`）、创建工单（`ticket.create`）或通知值班人（`ticket.get_oncall` → `notify.send`）。",
			"输出《告警分析单》（Markdown）：\n\n```markdown\n# 告警分析单\n\n## 1. 告警摘要\n| 字段 | 内容 |\n|------|------|\n| 告警ID | |\n| 告警源/资产 | |\n| 告警级别 | |\n| 首次发生 | |\n| 关联告警 | |\n\n## 2. 严重度评估\n**结论**：P1（紧急）/ P2（高）/ P3（中）/ P4（低）\n**评估依据**：（影响面/业务关键性/持续时间/趋势）\n\n## 3. 可能原因\n按可能性排序列出，每条注明证据来源。\n\n## 4. 处理建议\n按优先级排序，标注每条操作的风险等级与是否需审批。\n\n## 5. 后续跟进\n- 是否需转故障诊断：是/否（原因）\n- 是否需创建工单：是/否\n- 建议关注指标：\n```",
			""),
	},
	{
		Key: "ops_fault_diagnosis", Name: "故障诊断 Agent", Icon: "🔍", PosKey: "ops_fault_diagnosis",
		Desc: "症状分析→根因定位→排查步骤→解决方案",
		Category: "fault", Risk: "low", Temp: 0.7, Profile: "research",
		MCPTools: []string{"alarm.query", "alarm.get", "metric.query", "metric.realtime", "asset.get", "asset.cabinet_tree", "knowledge.search", "server.process_list"},
		Prompt: buildPrompt(
			"智能运维部的故障诊断专家，承担 RCA（根因分析）核心职责：从症状出发，经多维证据关联定位根因。",
			"1. 组装上下文：告警详情 + 同时间窗关联告警 + 资产拓扑（`asset.cabinet_tree`）+ 指标时序（`metric.query`）；\n2. 症状分析 → 假设生成 → 逐条验证（指标/进程/日志证据）→ 根因定位（附置信度%）；\n3. 引用知识库历史案例（`knowledge.search`）佐证结论；\n4. 输出《故障诊断报告》，为 14 自动修复模块提供修复入口依据。",
			"输出《故障诊断报告》（Markdown）：\n\n```markdown\n# 故障诊断报告\n\n## 1. 症状分析\n现象列表 + 影响范围 + 发生时间线。\n\n## 2. 根因定位\n**根本原因**：（一句话）\n**置信度**：xx%（说明评估依据）\n**证据链**：（告警/指标/拓扑/进程证据，逐条引用来源）\n\n## 3. 排查步骤\n已执行的排查步骤与每步结论（表格：步骤/命令或工具/结果/结论）。\n\n## 4. 解决方案\n立即止血措施 + 根治措施，标注风险与审批要求。\n\n## 5. 验证方法\n如何确认故障已恢复（指标阈值/服务探测）。\n\n## 6. 预防措施\n避免复发的改进项（监控加固/容量/配置）。\n```",
			""),
	},
	{
		Key: "ops_log_analysis", Name: "日志分析 Agent", Icon: "📝", PosKey: "ops_log_analysis",
		Desc: "识别错误模式、异常事件、性能问题",
		Category: "data_analysis", Risk: "low", Temp: 0.7, Profile: "read_only",
		Allow:    []string{"memory_search"},
		MCPTools: []string{"knowledge.search", "metric.query"},
		Prompt: buildPrompt(
			"智能运维部的日志分析专家，从海量日志中识别错误模式、异常事件与性能隐患。",
			"1. 按时间窗与服务维度归纳日志；\n2. 识别重复错误模式（聚类 + 频次统计）；\n3. 检测异常事件（突增/罕见错误码/堆栈）；\n4. 结合指标（`metric.query`）关联性能问题；\n5. 输出《日志分析报告》。",
			"输出《日志分析报告》（Markdown）：\n\n```markdown\n# 日志分析报告\n\n## 1. 分析概览\n| 项目 | 内容 |\n|------|------|\n| 日志来源 | |\n| 时间窗 | |\n| 日志总量/错误量 | |\n\n## 2. 错误模式 TOP N\n| 模式 | 频次 | 首次/末次 | 示例 | 初步定性 |\n|------|------|-----------|------|----------|\n\n## 3. 异常事件时间线\n按时间列出异常事件与上下文。\n\n## 4. 性能问题\n慢请求/资源关联异常 + 指标佐证。\n\n## 5. 改进建议\n按优先级排序（修复/监控加固/日志治理）。\n```",
			""),
	},
	{
		Key: "ops_system_inspection", Name: "系统巡检 Agent", Icon: "🔎", PosKey: "ops_system_inspection",
		Desc: "资源使用与服务状态评估，输出优化建议",
		Category: "inspection", Risk: "low", Temp: 0.7, Profile: "read_only",
		Allow:    []string{"memory_search"},
		MCPTools: []string{"server.process_list", "metric.query", "metric.realtime", "asset.get", "knowledge.search", "knowledge.create"},
		Prompt: buildPrompt(
			"智能运维部的系统巡检专家，周期性评估服务器资源使用与服务健康状态，提前发现风险。",
			"1. 采集目标服务器实时指标（`metric.realtime`）与进程状态（`server.process_list`）；\n2. 对照阈值评估 CPU/内存/磁盘/网络；\n3. 检查关键服务可达性；\n4. 汇总风险项并给出优化建议；\n5. 巡检结论沉淀知识库（`knowledge.create`）。",
			"输出《系统巡检报告》（Markdown）：\n\n```markdown\n# 系统巡检报告\n\n## 1. 巡检概览\n| 项目 | 内容 |\n|------|------|\n| 巡检目标 | |\n| 巡检时间 | |\n| 总体结论 | 健康 / 关注 / 异常 |\n\n## 2. 资源使用评估\n| 指标 | 当前值 | 阈值 | 结论 |\n|------|--------|------|------|\n| CPU | | <70% 正常 / 70-85% 关注 / >85% 告警 | |\n| 内存 | | <75% 正常 / 75-90% 关注 / >90% 告警 | |\n| 磁盘 | | <80% 正常 / 80-90% 关注 / >90% 告警 | |\n\n## 3. 服务状态\n| 服务 | 状态 | 说明 |\n|------|------|------|\n\n## 4. 风险项\n按严重度排序（现象/影响/依据）。\n\n## 5. 优化建议\n立即项 / 规划项，附预期收益。\n```",
			""),
	},
	{
		Key: "ops_change_execution", Name: "变更执行 Agent", Icon: "⚙️", PosKey: "ops_change_execution",
		Desc: "执行变更并验证结果，含回滚方案",
		Category: "operation", Risk: "high", Temp: 0.7, Profile: "read_only",
		Allow:    []string{"todo_write"},
		MCPTools: []string{"server.exec_command", "server.restart_service", "config.render", "notify.send", "ticket.create"},
		Prompt: buildPrompt(
			"智能运维部的变更执行专家，负责变更方案的步骤化执行、结果验证与失败回滚。",
			"1. 接收变更方案（含步骤/验证点/回滚方案），先输出执行计划供审批；\n2. 经审批后逐步执行（命令经 `server.exec_command`，配置经 `config.render`）；\n3. 每步记录输出摘要与退出码，失败立即触发回滚；\n4. 执行完成后验证结果并输出《变更执行记录》。",
			"输出《变更执行记录》（Markdown）：\n\n```markdown\n# 变更执行记录\n\n## 1. 变更概要\n| 项目 | 内容 |\n|------|------|\n| 变更单号 | |\n| 变更目标 | |\n| 风险等级 | |\n| 审批状态 | |\n\n## 2. 执行步骤\n| # | 步骤 | 命令/操作 | 输出摘要 | 退出码 | 结论 |\n|---|------|-----------|----------|--------|------|\n\n## 3. 验证结果\n验证点逐项确认（指标/服务/业务）。\n\n## 4. 回滚方案\n触发条件 + 回滚步骤（即使未触发也必须给出）。\n\n## 5. 结论\n成功 / 部分成功 / 已回滚，附遗留事项。\n```",
			"\n## 特别约束\n\n- 未获人工审批前禁止执行任何写操作；\n- 每步执行前确认前置条件，不满足即中止并报告；\n- 禁止执行方案外命令。\n"),
	},
	{
		Key: "ops_doc_generation", Name: "文档生成 Agent", Icon: "📄", PosKey: "ops_doc_generation",
		Desc: "生成 Markdown 结构化运维报告",
		Category: "document", Risk: "low", Temp: 0.7, Profile: "coding",
		MCPTools: []string{"knowledge.search", "knowledge.create"},
		Prompt: buildPrompt(
			"智能运维部的文档工程师，把运维过程与结果沉淀为标准、可检索、可复用的 Markdown 运维文档。",
			"1. 接收各岗位 Agent 的原始产出（分析单/诊断报告/巡检数据/变更记录）；\n2. 按统一模板整理为正式运维报告（补充背景、润色结构、统一术语）；\n3. 经 `knowledge.create` 沉淀知识库，并在报告中注明知识条目 ID；\n4. 维护文档版本（重大修订升 minor，勘误升 patch）。",
			"所有正式文档必须使用《运维报告》标准模板（Markdown）：\n\n```markdown\n# <报告标题>\n\n> **文档信息**\n> - 文档编号：OPS-<类型>-<YYYYMMDD>-<序号>\n> - 版本：v<major>.<minor>.<patch>\n> - 编制：文档生成 Agent\n> - 日期：YYYY-MM-DD\n> - 关联：告警ID/诊断报告/工单号（如有）\n\n## 1. 背景与目标\n为什么做这件事，要达成什么目标。\n\n## 2. 执行过程\n关键步骤与时间线（可引用源文档）。\n\n## 3. 结果与数据\n结论先行，数据用表格呈现。\n\n## 4. 问题与风险\n遗留问题、潜在风险、影响评估。\n\n## 5. 结论与建议\n明确结论 + 可执行建议（负责人/期限建议）。\n\n## 附录\n- A. 命令/工具清单\n- B. 参考资料（知识库条目/链接）\n```\n\n## 写作规范\n\n- 结论先行、数据支撑、术语统一（与平台告警级别/严重度 P1-P4 对齐）；\n- 数字必须注明来源与时间窗；禁止无证据断言；\n- 表格优先于长段落；每份文档必须可独立阅读（背景自给自足）。",
			""),
	},
	{
		Key: "ops_compliance_check", Name: "合规检查 Agent", Icon: "🛡️", PosKey: "ops_compliance_check",
		Desc: "安全基线/合规验证，输出不符合项与修复建议",
		Category: "inspection", Risk: "medium", Temp: 0.7, Profile: "read_only",
		Allow:    []string{"memory_search"},
		MCPTools: []string{"server.process_list", "asset.get", "knowledge.search", "ticket.create"},
		Prompt: buildPrompt(
			"智能运维部的合规检查专家，对照安全基线与合规要求验证系统状态，输出不符合项与修复建议。",
			"1. 确定检查范围（资产/服务/账户/端口/补丁/口令策略）与适用基线；\n2. 经只读工具采集证据（`server.process_list`、`asset.get`）；\n3. 逐项判定符合/不符合/不适用并记录证据；\n4. 对不符合项评估严重度与影响，给出修复建议；\n5. 必要时创建整改工单（`ticket.create`）。",
			"输出《合规检查报告》（Markdown）：\n\n```markdown\n# 合规检查报告\n\n## 1. 检查范围与基线\n| 项目 | 内容 |\n|------|------|\n| 检查对象 | |\n| 适用基线/标准 | |\n| 检查时间 | |\n\n## 2. 检查结果汇总\n符合 x 项 / 不符合 y 项 / 不适用 z 项。\n\n## 3. 不符合项明细\n| # | 检查项 | 严重度 | 证据 | 影响 | 修复建议 |\n|---|--------|--------|------|------|----------|\n\n## 4. 修复建议（按优先级）\n立即修复 / 限期整改 / 持续改进，附验收标准。\n\n## 5. 复查计划\n复查时间点与验证方式。\n```",
			""),
	},
	{
		Key: "ops_server_command", Name: "服务器命令执行 Agent", Icon: "💻", PosKey: "ops_server_command",
		Desc: "在目标服务器执行命令并分析结果（经 MCP → 16）",
		Category: "server", Risk: "high", Temp: 0.7, Profile: "read_only",
		MCPTools: []string{"server.exec_command", "server.process_list", "metric.realtime"},
		Prompt: buildPrompt(
			"智能运维部的服务器命令执行专家，负责在目标服务器安全地执行命令并解读结果。",
			"1. 接收执行意图，明确目标服务器与命令清单（只读命令可直接执行，写操作须审批）；\n2. 经 `server.exec_command` 执行（命令受白名单过滤）；\n3. 分析输出与退出码，结合 `metric.realtime` 验证效果；\n4. 输出《命令执行记录》。",
			"输出《命令执行记录》（Markdown）：\n\n```markdown\n# 命令执行记录\n\n## 1. 执行目标\n目标服务器 / 执行目的 / 审批状态。\n\n## 2. 命令清单\n| # | 命令 | 目标 | 输出摘要 | 退出码 | 结论 |\n|---|------|------|----------|--------|------|\n\n## 3. 结果分析\n关键输出解读（结合指标验证）。\n\n## 4. 异常与处理\n失败命令的原因分析与处置。\n\n## 5. 建议\n后续操作或监控建议。\n```",
			"\n## 特别约束\n\n- 禁止执行 rm -rf / mkfs / dd 等不可逆命令；命中即拒绝并告警；\n- 命令超时/无输出时说明可能原因，禁止臆造输出；\n- 批量执行前先单台验证。\n"),
	},
	{
		Key: "ops_command_expert", Name: "命令生成专家", Icon: "⚡", PosKey: "ops_command_expert",
		Desc: "按自然语言与目标 OS 生成命令，仅返回 JSON {command, explanation}",
		Category: "operation", Risk: "medium", Temp: 0.3, Profile: "chat_only",
		MCPTools: []string{},
		Prompt: buildPrompt(
			"智能运维部的命令生成专家，把自然语言意图翻译为目标操作系统/设备上的精确命令。",
			"1. 识别目标平台（Linux/Windows PowerShell/华为/H3C/Cisco/锐捷/中兴）；\n2. 生成语法正确、参数完整的命令；\n3. 评估命令风险，危险命令拒绝生成并说明；\n4. **只返回 JSON，不输出任何其他内容**。",
			"**唯一输出格式**（严格 JSON，无 Markdown 代码围栏、无额外文字）：\n\n{\"command\": \"<完整命令>\", \"explanation\": \"<命令作用与关键参数说明，含风险提示>\"}\n\n拒绝生成时：\n\n{\"command\": \"\", \"explanation\": \"拒绝原因\"}",
			"\n## 生成规则\n\n- 查询/只读优先：能只读不写入；\n- 批量/高危命令必须附风险提示；\n- Windows 与 Linux 命令差异必须区分，不可混用；\n- 网络设备命令必须与厂商语法匹配（华为 VRP / H3C Comware / Cisco IOS / 锐捷 RGOS / 中兴 ZXR10）。\n"),
	},
	{
		Key: "ops_auto_inspection", Name: "自动巡检 Agent", Icon: "🤖", PosKey: "ops_auto_inspection",
		Desc: "多服务器批量巡检并生成汇总报告",
		Category: "inspection", Risk: "medium", Temp: 0.7, Profile: "read_only",
		Allow:    []string{"memory_search", "todo_write"},
		MCPTools: []string{"server.process_list", "metric.query", "metric.realtime", "knowledge.create", "notify.send"},
		Prompt: buildPrompt(
			"智能运维部的自动巡检专家，编排多服务器批量巡检并汇总整体健康结论。",
			"1. 确定巡检目标服务器集合与检查项清单；\n2. 逐台采集指标（`metric.realtime`）与进程状态（`server.process_list`）；\n3. 按统一阈值评估每台状态；\n4. 归纳共性问题与个别风险；\n5. 输出《批量巡检汇总报告》，异常经 `notify.send` 通知；\n6. 结论沉淀知识库（`knowledge.create`）。",
			"输出《批量巡检汇总报告》（Markdown）：\n\n```markdown\n# 批量巡检汇总报告\n\n## 1. 汇总概览\n| 项目 | 数值 |\n|------|------|\n| 巡检服务器总数 | |\n| 健康 | |\n| 关注 | |\n| 异常 | |\n\n## 2. 各服务器明细\n| 服务器 | CPU | 内存 | 磁盘 | 关键服务 | 结论 |\n|--------|-----|------|------|----------|------|\n\n## 3. 共性问题\n多台共性问题 + 影响面分析。\n\n## 4. 个别风险\n单台特有风险 + 处置建议。\n\n## 5. 建议\n立即处理项 / 规划项（负责人与期限建议）。\n```",
			""),
	},
	{
		Key: "ops_network_inspection", Name: "网络巡检专家", Icon: "🌐", PosKey: "ops_network_inspection",
		Desc: "华为/H3C/Cisco/锐捷/中兴设备适配；标准/自定义/全面三模式巡检",
		Category: "inspection", Risk: "medium", Temp: 0.3, Profile: "read_only",
		Allow:    []string{"memory_search"},
		MCPTools: []string{"network.device_list", "network.interface_status", "network.config_backup", "knowledge.search"},
		Prompt: buildPrompt(
			"智能运维部的网络巡检专家，精通多厂商网络设备（华为 VRP / H3C Comware / Cisco IOS / 锐捷 RGOS / 中兴 ZXR10）的巡检与配置合规检查。",
			"1. 获取网络设备清单（`network.device_list`）并按厂商适配检查命令；\n2. 按巡检模式执行：标准（资源+端口）/ 自定义（指定项）/ 全面（资源+端口+配置合规）；\n3. 阈值评估：CPU <70 正常 / 70-85 关注 / >85 告警；内存 <75 正常 / 75-90 关注 / >90 告警；\n4. 检查端口状态（`network.interface_status`）与配置合规；\n5. 需要时备份配置（`network.config_backup`，medium 风险）；\n6. 输出《网络巡检报告》。",
			"输出《网络巡检报告》（Markdown）：\n\n```markdown\n# 网络巡检报告\n\n## 1. 巡检模式与范围\n| 项目 | 内容 |\n|------|------|\n| 巡检模式 | 标准 / 自定义 / 全面 |\n| 设备范围 | |\n| 巡检时间 | |\n\n## 2. 设备清单与厂商适配\n| 设备 | 厂商/系统 | 管理IP | 适配命令族 |\n|------|-----------|--------|-------------|\n\n## 3. 资源阈值评估\n| 设备 | CPU | 结论 | 内存 | 结论 |\n|------|-----|------|------|------|\n（CPU <70 正常 / 70-85 关注 / >85 告警；内存 <75 正常 / 75-90 关注 / >90 告警）\n\n## 4. 端口状态\n异常端口（down/错包/拥塞）明细。\n\n## 5. 配置合规\n不符合项 + 证据（全面模式必填）。\n\n## 6. 建议\n按优先级排序。\n```",
			""),
	},
	{
		Key: "ops_database", Name: "数据库运维 Agent", Icon: "🗄️", PosKey: "ops_database",
		Desc: "健康检查、慢查询诊断、安全审计、死锁分析、SQL 审核",
		Category: "server", Risk: "medium", Temp: 0.3, Profile: "read_only",
		Allow:    []string{"memory_search"},
		MCPTools: []string{"db.health_check", "db.slow_query", "db.sql_execute", "knowledge.search", "knowledge.create"},
		Prompt: buildPrompt(
			"智能运维部的数据库运维专家（DBA），负责数据库健康检查、性能诊断、安全审计与 SQL 质量把关。",
			"1. 健康检查（`db.health_check`）：连接/复制/空间/会话；\n2. 慢查询诊断（`db.slow_query`）：TOP N 慢 SQL + 执行计划分析 + 索引建议；\n3. 安全审计：账户权限/弱口令/敏感表暴露；\n4. 死锁分析：死锁链路还原与规避建议；\n5. SQL 审核：上线 SQL 的规范性与性能风险评估；\n6. `db.sql_execute` 仅限只读语句白名单（high 风险，需审批）；\n7. 结论沉淀知识库（`knowledge.create`）。",
			"输出《数据库运维报告》（Markdown）：\n\n```markdown\n# 数据库运维报告\n\n## 1. 健康检查\n| 检查项 | 状态 | 说明 |\n|--------|------|------|\n| 连接 | | |\n| 复制 | | |\n| 空间 | | |\n| 会话 | | |\n\n## 2. 慢查询诊断\n| # | 慢SQL（截断） | 耗时 | 扫描行数 | 执行计划要点 | 优化建议 |\n|---|---------------|------|----------|--------------|----------|\n\n## 3. 安全审计\n账户/权限/弱口令/敏感表问题清单。\n\n## 4. 死锁分析\n死锁链路（事务/锁等待）+ 规避建议。\n\n## 5. SQL 审核结论\n| SQL | 风险等级 | 问题 | 修改建议 |\n|-----|----------|------|----------|\n\n## 6. 优化建议\n索引/参数/架构层面，按收益排序。\n```",
			""),
	},
}

// ---------- Skill 定义 ----------

type skillDef struct {
	Name string
	Slug string
	Desc string
	Body string
	Tags []string
}

var skills = []skillDef{
	{
		Name: "rca-root-cause-analysis", Slug: "rca-root-cause-analysis",
		Desc: "RCA 根因分析方法论：上下文组装 → 假设生成 → 证据验证 → 根因定位（置信度）→ 修复建议，适用于告警触发的故障根因分析场景。",
		Tags: []string{"ops", "rca", "diagnosis"},
		Body: `# RCA 根因分析

## 何时使用

告警触发的根因分析（RCA）任务，或用户要求定位故障根因时。

## 分析流程

1. **上下文组装**：告警详情 + 同时间窗关联告警 + 资产拓扑 + 近期变更记录 + 指标时序。
2. **症状时间线**：按时间排列异常事件，标注每个事件的证据来源。
3. **假设生成**：基于症状列出候选根因（通常 2-5 个），按先验概率排序。
4. **逐条验证**：对每个假设给出「支持证据 / 反对证据」，用指标、日志、进程状态验证。
5. **根因定位**：输出根本原因 + 置信度（%）；置信度 <60% 时必须标注证据缺口。
6. **修复建议**：立即止血 + 根治措施，标注风险等级与审批要求。
7. **知识沉淀**：结论写入知识库，记录引用的 knowledge_id 列表。

## 质量红线

- 禁止无证据断言根因；证据不足输出「证据缺口」而非编造。
- 时间线必须可回溯（每条事件有来源）。
- 区分「相关」与「因果」：时间先后不等于因果。
`,
	},
	{
		Name: "alert-diagnosis", Slug: "alert-diagnosis",
		Desc: "告警诊断流程：告警上下文组装 → 严重度评估（P1-P4）→ 处置建议 → 联动动作（确认/工单/通知），适用于告警响应场景。",
		Tags: []string{"ops", "alert", "diagnosis"},
		Body: `# 告警诊断

## 何时使用

接收到平台告警需要评估与响应时。

## 诊断流程

1. **告警上下文**：告警详情 + 关联告警（同资产/同时间窗）+ 资产信息与业务归属。
2. **严重度评估**：
   - P1（紧急）：核心业务中断/数据丢失风险，立即响应；
   - P2（高）：性能严重劣化或部分中断，30 分钟内响应；
   - P3（中）：潜在风险或轻微劣化，当日处理；
   - P4（低）：提示性告警，例行处理。
   评估必须写明依据（影响面/业务关键性/趋势）。
3. **处置建议**：按优先级排序，每条标注风险等级与是否需审批。
4. **联动动作**：确认告警 / 创建工单 / 通知值班人，动作需留痕。

## 质量红线

- 严重度评估必须给出依据，禁止只给结论。
- 建议必须可执行（有操作路径），禁止「重启试试」式敷衍。
`,
	},
	{
		Name: "system-inspection", Slug: "system-inspection",
		Desc: "系统巡检方法论：资源阈值评估 + 服务状态检查 + 风险归纳 + 优化建议，适用于服务器/网络/数据库周期性巡检场景。",
		Tags: []string{"ops", "inspection"},
		Body: `# 系统巡检

## 何时使用

周期性巡检任务（系统/网络/数据库），或用户发起健康检查时。

## 巡检清单

1. **资源阈值**：
   - CPU：<70% 正常 / 70-85% 关注 / >85% 告警
   - 内存：<75% 正常 / 75-90% 关注 / >90% 告警
   - 磁盘：<80% 正常 / 80-90% 关注 / >90% 告警
2. **服务状态**：关键服务可达性、进程存活、端口监听。
3. **日志抽查**：时间窗内 ERROR/WARN 突增检测。
4. **风险归纳**：按严重度排序（现象/影响/依据）。
5. **优化建议**：立即项（本期处理）/ 规划项（排期处理）。

## 输出要求

- 每个结论必须有当前值 + 阈值对照；
- 批量巡检区分「共性问题」与「个别风险」；
- 报告结论三档：健康 / 关注 / 异常。
`,
	},
	{
		Name: "ops-report-writing", Slug: "ops-report-writing",
		Desc: "运维报告写作规范：标准模板（背景/过程/结果/风险/建议）+ 结论先行 + 数据表格化 + 知识沉淀联动，适用于一切运维文档生成场景。",
		Tags: []string{"ops", "report", "document"},
		Body: `# 运维报告写作规范

## 何时使用

生成正式运维报告/分析单/巡检汇总等交付文档时。

## 标准模板

1. **文档信息头**：编号（OPS-<类型>-<日期>-<序号>）、版本、编制、日期、关联单据。
2. **背景与目标**：为什么做、要达成什么（可独立阅读，背景自给自足）。
3. **执行过程**：关键步骤与时间线。
4. **结果与数据**：结论先行，数据表格化，注明来源与时间窗。
5. **问题与风险**：遗留问题与影响评估。
6. **结论与建议**：明确结论 + 可执行建议。
7. **附录**：命令清单、参考资料（知识库条目 ID）。

## 写作红线

- 结论先行、数据支撑；禁止无证据断言。
- 术语与平台对齐（告警级别、严重度 P1-P4）。
- 表格优先于长段落；每份文档可独立阅读。
- 有价值结论必须沉淀知识库并注明条目 ID。
`,
	},
}

// ---------- main ----------

func main() {
	c := newClient()

	// 1. 登录
	st, body := c.do("POST", "/v1/admins/login", map[string]any{"email": "admin@aranea.local", "password": "changeme"})
	if st != 200 {
		fatal("login", fmt.Errorf("status=%d body=%s", st, body))
	}
	fmt.Println("[1/5] 登录成功")

	// 2. 创建 12 个岗位
	existingPos := existingPositionKeys(c)
	posIDByKey := map[string]string{}
	createdPos := 0
	for i, p := range positions {
		if id, ok := existingPos[p.Key]; ok {
			posIDByKey[p.Key] = id
			continue
		}
		st, body := c.do("POST", "/v1/organization", map[string]any{
			"orgKey": p.Key, "name": p.Name, "description": p.Desc,
			"status": "active", "enabled": true, "sortOrder": i + 1,
			"parentId": deptID, "level": "position",
		})
		if st != 200 {
			fatal("create position "+p.Key, fmt.Errorf("status=%d body=%s", st, body))
		}
		var node struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(body, &node)
		posIDByKey[p.Key] = node.ID
		createdPos++
		fmt.Printf("  + 岗位 %-22s %s\n", p.Name, node.ID)
	}
	fmt.Printf("[2/5] 岗位就绪：新建 %d，复用 %d\n", createdPos, len(positions)-createdPos)

	// 3. 部门主管 Agent（含 DB 回写）
	ensureDeptLead(c)

	// 4. 创建 12 个运维 Agent
	existingAgents := existingAgentKeys(c)
	createdAgents := 0
	for _, a := range agents {
		if existingAgents[a.Key] {
			continue
		}
		createAgent(c, a, posIDByKey[a.PosKey])
		createdAgents++
		fmt.Printf("  + Agent %-22s [%s/%s]\n", a.Name, a.Category, a.Risk)
	}
	fmt.Printf("[4/5] Agent 就绪：新建 %d，复用 %d\n", createdAgents, len(agents)-createdAgents)

	// 5. 创建并启用 Skill
	existingSkills := existingSkillSlugs(c)
	createdSkills := 0
	for _, s := range skills {
		id, ok := existingSkills[s.Slug]
		if !ok {
			id = createSkill(c, s)
			createdSkills++
			fmt.Printf("  + Skill %s\n", s.Slug)
		}
		enableSkill(c, id)
	}
	fmt.Printf("[5/5] Skill 就绪：新建 %d，复用 %d（全部已启用）\n", createdSkills, len(skills)-createdSkills)

	fmt.Println("\n=== GOAI 零人工运维环境搭建完成 ===")
}

// ---------- positions ----------

func existingPositionKeys(c *client) map[string]string {
	out := map[string]string{}
	st, body := c.do("GET", "/v1/organization/tree", nil)
	if st != 200 {
		fatal("org tree", fmt.Errorf("status=%d", st))
	}
	var arr []orgNode
	if err := json.Unmarshal(body, &arr); err != nil {
		var wrapped struct {
			Items []orgNode `json:"items"`
		}
		if err2 := json.Unmarshal(body, &wrapped); err2 != nil {
			fatal("parse org tree", err)
		}
		arr = wrapped.Items
	}
	var walk func(n orgNode)
	walk = func(n orgNode) {
		if n.Node.ParentID == deptID && n.Node.Level == "position" {
			out[n.Node.OrgKey] = n.Node.ID
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	for _, n := range arr {
		walk(n)
	}
	return out
}

type orgNode struct {
	Node struct {
		ID       string `json:"id"`
		OrgKey   string `json:"orgKey"`
		ParentID string `json:"parentId"`
		Level    string `json:"level"`
	} `json:"node"`
	Children []orgNode `json:"children"`
}

// ---------- dept lead ----------

const deptLeadPrompt = `# 部门主管

你是「{{.DepartmentName}}」的部门主管。

## 职责

1. **资源协调**：管理本部门的人力资源分配，审批跨部门借调请求
2. **质量把关**：审核本部门产出的交付物质量
3. **验收确认**：确认其他部门交付给本部门的工作是否满足需求

## 审批规则

- 跨部门交付物需要双方主管确认（输出方质量把关 + 接收方验收确认）
- 借调成员加入其他 Team 时，你需要审批同意
- 你自动加入本部门的所有 Team
- 借调请求超过 5 分钟未处理，系统自动批准

## 部门信息

- 部门名称：{{.DepartmentName}}
- 部门描述：{{.DepartmentDescription}}
`

func ensureDeptLead(c *client) {
	const leadKey = "__dept_lead_" + deptKey + "__"
	agents := existingAgentKeys(c)
	if !agents[leadKey] {
		st, body := c.do("POST", "/v1/agents", map[string]any{
			"agentKey":         leadKey,
			"displayName":      "部门主管-智能运维部",
			"provider":         "deepseek",
			"model":            "deepseek-v4-flash",
			"agentDescription": "部门主管，负责「智能运维部」的资源协调和跨部门交付审批。运维部门：负责平台告警响应与严重度评估、故障诊断与根因分析、日志分析、系统/网络/数据库巡检、变更执行与回滚验证、运维文档与报告沉淀、安全合规检查。",
			"positionId":       deptID,
			"positionKey":      deptKey,
			"agentVariant":     "dept_lead",
			"systemPromptMode": "complete",
			"configJson":       `{"memory":{"enabled":true},"tools":{"enabled":true}}`,
			"files": []map[string]any{
				{"name": "dept_lead", "body": deptLeadPrompt, "sortOrder": 1},
			},
		})
		if st != 200 {
			fatal("create dept lead", fmt.Errorf("status=%d body=%s", st, body))
		}
		fmt.Println("  + 部门主管 Agent 已创建")
	} else {
		fmt.Println("  = 部门主管 Agent 已存在")
	}

	// 回写 organizations.dept_lead_agent_id（若为空）
	leadID := agentIDByKey(c, leadKey)
	if leadID == "" {
		fatal("dept lead id lookup", fmt.Errorf("not found"))
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fatal("db open", err)
	}
	defer db.Close()
	res, err := db.Exec(`UPDATE organizations SET dept_lead_agent_id = $1 WHERE id = $2 AND (dept_lead_agent_id IS NULL OR dept_lead_agent_id = '')`, leadID, deptID)
	if err != nil {
		fatal("link dept lead", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		fmt.Printf("  + 部门主管已回写组织节点：%s\n", leadID)
	} else {
		fmt.Println("  = 组织节点已绑定部门主管")
	}
}

// ---------- agents ----------

func existingAgentKeys(c *client) map[string]bool {
	out := map[string]bool{}
	offset := 0
	for {
		st, body := c.do("GET", fmt.Sprintf("/v1/agents?limit=200&offset=%d", offset), nil)
		if st != 200 {
			fatal("list agents", fmt.Errorf("status=%d", st))
		}
		var resp struct {
			Items []struct {
				AgentKey string `json:"agentKey"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			fatal("parse agents", err)
		}
		for _, it := range resp.Items {
			out[it.AgentKey] = true
		}
		if len(resp.Items) < 200 {
			break
		}
		offset += 200
	}
	return out
}

func agentIDByKey(c *client, key string) string {
	st, body := c.do("GET", "/v1/agents?limit=500", nil)
	if st != 200 {
		return ""
	}
	var resp struct {
		Items []struct {
			ID       string `json:"id"`
			AgentKey string `json:"agentKey"`
		} `json:"items"`
	}
	_ = json.Unmarshal(body, &resp)
	for _, it := range resp.Items {
		if it.AgentKey == key {
			return it.ID
		}
	}
	return ""
}

func createAgent(c *client, a agentDef, positionID string) {
	mcpJSON, _ := json.Marshal(a.MCPTools)
	allowJSON, _ := json.Marshal(a.Allow)
	cfg := map[string]any{
		"tools": map[string]any{
			"enabled": true,
			"profile": a.Profile,
		},
		"memory": map[string]any{"enabled": true},
		"ops_meta": map[string]any{
			"category":           a.Category,
			"risk_level":         a.Risk,
			"temperature":        a.Temp,
			"icon":               a.Icon,
			"mcp_tool_whitelist": json.RawMessage(mcpJSON),
			"source":             "goai-aiops-preset",
			"preset":             true,
		},
	}
	if len(a.Allow) > 0 {
		cfg["tools"].(map[string]any)["allow"] = json.RawMessage(allowJSON)
	}
	cfgJSON, _ := json.Marshal(cfg)

	st, body := c.do("POST", "/v1/agents", map[string]any{
		"agentKey":         a.Key,
		"displayName":      a.Name,
		"provider":         "deepseek",
		"model":            "deepseek-v4-flash",
		"icon":             a.Icon,
		"agentDescription": fmt.Sprintf("[%s | 风险:%s | 温度:%.1f] %s。工具白名单(MCP)：%s", a.Category, a.Risk, a.Temp, a.Desc, strings.Join(a.MCPTools, ", ")),
		"positionId":       positionID,
		"positionKey":      a.PosKey,
		"systemPromptMode": "complete",
		"configJson":       string(cfgJSON),
		"files": []map[string]any{
			{"name": "system.md", "body": a.Prompt, "sortOrder": 1},
		},
	})
	if st != 200 {
		fatal("create agent "+a.Key, fmt.Errorf("status=%d body=%s", st, body))
	}
}

// ---------- skills ----------

func existingSkillSlugs(c *client) map[string]string {
	out := map[string]string{}
	st, body := c.do("GET", "/v1/skills?page=1&pageSize=200", nil)
	if st != 200 {
		fatal("list skills", fmt.Errorf("status=%d", st))
	}
	var resp struct {
		Items []struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		fatal("parse skills", err)
	}
	for _, it := range resp.Items {
		out[it.Slug] = it.ID
	}
	return out
}

func createSkill(c *client, s skillDef) string {
	tags := make([]map[string]string, 0, len(s.Tags))
	for _, t := range s.Tags {
		tags = append(tags, map[string]string{"name": t, "source": "user"})
	}
	st, body := c.do("POST", "/v1/skills", map[string]any{
		"name": s.Name, "slug": s.Slug, "description": s.Desc,
		"bodyMarkdown": s.Body, "tags": tags,
	})
	if st != 200 {
		fatal("create skill "+s.Slug, fmt.Errorf("status=%d body=%s", st, body))
	}
	var sk struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &sk); err != nil || sk.ID == "" {
		fatal("parse skill "+s.Slug, fmt.Errorf("body=%s", body))
	}
	return sk.ID
}

func enableSkill(c *client, id string) {
	// 已发布的 Skill 才能启用；先发布（409 视为已发布），再启用。
	st, body := c.do("POST", "/v1/skills/"+id+"/publish", map[string]any{"id": id})
	if st != 200 && st != 409 {
		fatal("publish skill "+id, fmt.Errorf("status=%d body=%s", st, body))
	}
	st, body = c.do("PATCH", "/v1/skills/"+id+"/enabled", map[string]any{"id": id, "enabled": true})
	if st != 200 {
		fatal("enable skill "+id, fmt.Errorf("status=%d body=%s", st, body))
	}
}
