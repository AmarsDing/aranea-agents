package biz

import "strings"

// LooksLikeKnowledgeQuery reports whether the user turn explicitly references
// the knowledge base / document repository (P2-④, session-eval-20260829-r2
// R4-Q7 根修）。
//
// 背景：Spirit profile 的核心常驻工具集不含 knowledge_search（延迟目录），
// 且知识 cue 对无工具面明确告知「不要 tool_search 猎取」——S03「结合知识库
// 回答」「知识库里有对应的处置 SOP 吗」全程零检索、参数化作答。本判定是
// 知识意图的词法路由信号：命中即在本轮激活 knowledge_search/knowledge_reflect
// 并在 cue 中打知识意图标签。
//
// 与 LooksLikeFactQuery/LooksLikeDirectAnswer 的区别：那两个是「免除规划
// 义务」的快路径（须任务信号 veto）；本判定是「增加检索能力」——挂载工具
// 只增义务/能力、不免除任何义务（ADR-79-V V2 单向约束不触发），故**不做**
// 任务动作 veto：「核对知识库数据并生成报告」同样是正当的知识检索诉求。
// 误判代价 = 本会话工具面多挂一个检索工具 schema（数百 token），远小于
// 漏判代价（用户点名知识库却参数化作答）。
//
// 维护纪律：只收「显式指向知识库/文档库/规程手册」的词；泛知识问法
// （「什么是/为什么/怎么做」）不进表——那是世界知识问题，预检索与记忆
// 注入已覆盖，不应为此常驻检索工具。
var knowledgeQueryPatterns = []string{
	// 显式知识库引用（S03 语料：「结合知识库回答」「知识库里有对应的处置 SOP」）
	"知识库",
	"knowledge base", "knowledge_base", "knowledgebase",
	// 文档/资料库
	"文档库", "资料库", "档案库",
	// 规程/手册/预案类载体（与「处置/运维/操作/值班」限定共现，避免误收
	// 「帮我写一份手册」类交付物诉求——写手册不需要先检索手册）
	"处置规程", "处置预案", "运维手册", "操作手册", "值班手册",
	"应急预案", "巡检规程", "操作规程",
}

func LooksLikeKnowledgeQuery(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	for _, p := range knowledgeQueryPatterns {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}
