package biz

import (
	"regexp"
	"strings"
)

// HasTaskActionSignal reports whether the user turn carries task/deliverable
// action signals (M79 R4 / ADR-79-V V2, 2026-08-26).
//
// V2 原则：分类器输出只可用于增加义务，不可用于免除义务。事实查询/直接回复
// 快路径（LooksLikeFactQuery / intent.SkipForDirectReply）的本质是"免除
// intent pass / 规划义务"，因此凡含任务动作词的轮次一律不得命中快路径——
// 子串匹配无法区分「明天天气怎么样」与「核对昨天的天气数据并生成巡检报告」，
// 后者含任务动作词（核对/生成），必须走完整 intent pass + 规划门。
//
// 代价是非对称的（RouteLLM 非对称代价）：任务型误判为闲聊 = 组织路由失效
// （P-INTENT-SKIP 事故）；闲聊误跑 intent pass 仅多一次廉价 LLM 调用。
// 宁重勿轻——动作词表宁宽勿窄。
//
// 包C Q2 结构正则化（session-eval-20260827 方向一根修）：平铺词表无法覆盖
// 能产结构——「让市场部出一版 Q3 推广文案框架」（S06）与「出一条 30 秒创意
// 脚本框架」（S08）88 字明确交付诉求被判 confident_simple（0.1 分），因为
// 「出一版/出一条/文案/脚本/框架」不在任何词表。修复 = 词表之外叠加三个
// 模式族：① 产出动词+数量词（出/写/做/搭 + 一/两/几/个…），② 产出动词…
// 交付物名词窗口共现（规划…方案 / 整理…报告），③ 需求动词…交付物（给我
// 一个方案）。外加④⑤派发句式（见 HasDispatchSignal）。单一数据源同时供
// shouldSkipIntentPass 与 QuickAssess 消费，根除双词表漂移。
func HasTaskActionSignal(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	// 平铺词表优先：词表词是强任务证据，咨询句式不豁免——「为什么我的
	// 服务器一直告警，请帮我排查」虽以「为什么」起首，但含「排查」，
	// 仍是工作命令。
	for _, p := range taskActionPatterns {
		if strings.Contains(t, p) {
			return true
		}
	}
	// 纯咨询句式豁免（仅模式族层）：「如何/怎么/为什么」起首且未命中
	// 词表的消息是方法咨询而非工作命令——「如何用Python写一个 hello
	// world」要的是讲解不是交付物（QuickAssess 既有契约钉住该例判
	// simple）。句首锚定：句中/句尾的「再告诉我怎么处理」不构成咨询
	// 框架。
	if consultationFramePattern.MatchString(t) {
		return false
	}
	for _, fam := range taskSignalFamilies {
		if fam.MatchString(t) {
			return true
		}
	}
	return HasDispatchSignal(userText)
}

// HasDispatchSignal 报告消息是否含「组织实体派发」结构证据（包C Q2-C2
// 组队证据闸 + 任务信号族④⑤）。判定分两路：
//   - 派发动词（让/请/安排/组织/协调/委派）+ 12 字窗口内含组织实体
//     （让市场部出一版文案 / 请技术部排查）；
//   - ≥2 个组织实体并列（技术侧给发布计划、内容侧给宣传文案、运营侧给
//     checklist —— S07 重型编排的合法组队证据）。
//
// 与任务信号的区别：HasDispatchSignal 专指「把工作派给组织实体」的结构，
// 是组队模式（parallel/dag）的正当性证据；自我规划（「我们来规划…」）与
// 内容板块枚举（「渠道、节奏、预算三大块」）不命中——这是 S09-t1（空组队
// 烧 153K）与 S07（合法组队）的鉴别边界。
func HasDispatchSignal(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	if dispatchVerbEntityPattern.MatchString(t) {
		return true
	}
	return countOrgEntities(t) >= 2
}

// taskActionPatterns 任务/交付物动作词。维护纪律：
//   - 只收"产生交付物或变更状态"的动作词；纯闲聊/身份/时钟词不进表。
//   - 「介绍/说明/解释」刻意不收——directReplyPatterns 的「介绍你自己」
//     是 chit-chat 快路径既有契约（有测试锁定），收了会破坏。
//   - 宁宽勿窄：误收（闲聊多跑一次 intent pass）代价 ≪ 漏收（任务被快
//     路径短路、组织路由失效）。
//
// 包C Q2 合并（2026-08-27）：原 agent 侧 taskRequestWords（产出/方案/组织/
// 安排/梳理/汇报/对比/总结/检查/排查）并入本表——两表曾是同一语义的漂移
// 双源（skip 门用本表、QuickAssess 升级用词表），S06/S08 误判即漂移产物。
var taskActionPatterns = []string{
	// 运维/变更类
	"排查", "巡检", "安装", "部署", "修复", "检查", "更新", "配置", "同步",
	"监控", "告警", "升级", "回滚", "重启", "切换", "扩容", "备份", "恢复",
	"演练", "调度", "编排", "开通", "关闭", "启用", "禁用", "迁移", "发布",
	"上线", "下发", "推送", "执行", "处理", "解决", "调查", "定位", "核对",
	// 交付物生产类
	"生成", "导出", "对比", "汇总", "翻译", "总结", "制定", "评估", "审计",
	"编写", "撰写", "起草", "修改", "优化", "重构", "实现", "开发", "设计",
	"测试", "验证", "整理", "完善", "分析", "创建", "删除", "审批", "组队",
	"写成", "出一份", "出个", "做个", "做成", "改成", "设置为", "调整为",
	"记录到", "写入", "通知", "发送",
	// 原 taskRequestWords 并入（包B B2 管理层路由失效语料高频任务词）
	"产出", "方案", "组织", "安排", "梳理", "汇报",
	// 任务链连接词（"然后/接着再做事"几乎必然是复合任务）
	"然后帮", "然后再", "接着帮", "接着再", "顺便帮", "顺便再",
	// English
	"install", "deploy", "troubleshoot", "investigate", "fix ", "analyze",
	"generate", "export", "configure", "summarize", "translate", "audit",
	"backup", "restore", "migrate", "implement", "refactor", "write a",
	"write me", "draft", "create", "delete", "update", "upgrade", "rollback",
	"restart", "verify", "schedule",
}

// taskSignalFamilies 任务信号模式族（包C Q2 结构正则化）。正则包级预编译，
// 匹配前调用方已做 ToLower。三个族互补覆盖能产结构：
var taskSignalFamilies = []*regexp.Regexp{
	// 族① 产出动词+数量词（能产量产结构）：出一版 / 出一条 / 写个 / 做一套 /
	// 搭一个 / 产 3 份。动词+数量即强任务信号，不要求交付物名词跟随
	// （「出一条 30 秒创意脚本框架」在「条」后即命中）。反向：「出现一个
	// 问题」「想出一个办法」属 borderline 误收，代价仅一次 intent pass。
	regexp.MustCompile(`(?:出|写|撰|做|作|搭|产|制|创|编|译|绘|拍|剪|录)\s*(?:一|两|几|数|\d+|个)\s*(?:个|版|份|条|篇|套|批|部|张|期|段|则|本|封)?`),
	// 族② 产出动词…交付物名词（窗口 ≤12 字共现）：规划内容运营方案 /
	// 整理巡检报告 / 制定年度预算。交付物名词兜底「方案/报告」类名词
	// 未进平铺词表的形态（如「内容运营方案」中「方案」前的动词非词表词）。
	regexp.MustCompile(`(?:规划|策划|制定|拟定|起草|撰写|编写|编制|整理|汇总|梳理|总结|对比|分析|设计|开发|翻译|优化|重构|排版|审校|输出|沉淀).{0,12}(?:方案|报告|文案|脚本|框架|计划|清单|日历|预算|材料|文档|表格|报表|台账|手册|流程|制度|视频|海报|邮件|代码|接口|规划|策划案|sop|runbook)`),
	// 族③ 需求动词…交付物：给我一个容灾方案 / 帮我写封邮件 / 我要一份报告。
	regexp.MustCompile(`(?:给我|我要|需要|想要|帮我|为我|替我|帮忙).{0,12}(?:方案|报告|文案|脚本|框架|清单|材料|文档|表格|邮件|代码|计划|预算|sop)`),
}

// consultationFramePattern 纯咨询句式（句首锚定）：「如何/怎么/为什么」
// 起首的消息是方法咨询而非工作命令——「如何用Python写一个hello world」
// 要的是讲解不是交付物（QuickAssess 既有契约钉住该例判 simple）。仅豁免
// 模式族层：词表词优先于本豁免（「为什么我的服务器一直告警，请帮我排查」
// 含「排查」仍是工作命令）。句中/句尾的「再告诉我怎么处理」不构成咨询
// 框架——句首锚定防「陈述+顺带一问」被整体豁免。
var consultationFramePattern = regexp.MustCompile(`^(如何|怎么|为什么|为啥|怎样|何以|为何)`)

// dispatchVerbEntityPattern 派发句式（族④）：派发动词 + ≤12 字窗口内含
// 组织实体。「让数字内容媒体公司市场部出一版推广文案」（S06，让→市场部
// 窗口 10 字）命中；「让子弹飞一会儿」无实体不命中。
var dispatchVerbEntityPattern = regexp.MustCompile(`(?:让|请|叫|安排|组织|协调|委派|指派|派遣|交给|交由).{0,12}(?:团队|部门|公司|小组|分队|班子|` + orgEntityAlternation + `)`)

// orgEntityAlternation 组织实体词表（族④⑤共用）：职能侧（技术侧/内容侧…）
// 与部门（市场部/运营部…）。维护纪律：只收组织承载体，内容板块名（渠道/
// 节奏/预算）与交付物名词（方案/框架）不进表——这是 S09-t1 与 S07 的
// 鉴别边界。
const orgEntityAlternation = `技术侧|内容侧|运营侧|市场侧|产品侧|研发侧|销售侧|业务侧|客服侧|品牌侧|设计侧|测试侧|市场部|运营部|技术部|产品部|研发部|销售部|内容部|创作部|人事部|财务部|客服部|品牌部|设计部|测试部|运维部|数据部`

var orgEntityPattern = regexp.MustCompile(orgEntityAlternation)

// countOrgEntities 统计消息中不同组织实体的命中种数（≥2 即多实体并列
// 派发，族⑤）。去重按词面——同一实体重复提及不累计。
func countOrgEntities(t string) int {
	hits := orgEntityPattern.FindAllString(t, -1)
	if len(hits) < 2 {
		return len(hits)
	}
	seen := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		seen[h] = struct{}{}
	}
	return len(seen)
}
