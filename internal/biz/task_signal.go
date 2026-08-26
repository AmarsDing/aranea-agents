package biz

import "strings"

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
func HasTaskActionSignal(userText string) bool {
	t := strings.ToLower(strings.TrimSpace(userText))
	if t == "" {
		return false
	}
	for _, p := range taskActionPatterns {
		if strings.Contains(t, p) {
			return true
		}
	}
	return false
}

// taskActionPatterns 任务/交付物动作词。维护纪律：
//   - 只收"产生交付物或变更状态"的动作词；纯闲聊/身份/时钟词不进表。
//   - 「介绍/说明/解释」刻意不收——directReplyPatterns 的「介绍你自己」
//     是 chit-chat 快路径既有契约（有测试锁定），收了会破坏。
//   - 宁宽勿窄：误收（闲聊多跑一次 intent pass）代价 ≪ 漏收（任务被快
//     路径短路、组织路由失效）。
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
	// 任务链连接词（"然后/接着再做事"几乎必然是复合任务）
	"然后帮", "然后再", "接着帮", "接着再", "顺便帮", "顺便再",
	// English
	"install", "deploy", "troubleshoot", "investigate", "fix ", "analyze",
	"generate", "export", "configure", "summarize", "translate", "audit",
	"backup", "restore", "migrate", "implement", "refactor", "write a",
	"write me", "draft", "create", "delete", "update", "upgrade", "rollback",
	"restart", "verify", "schedule",
}
