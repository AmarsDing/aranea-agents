package biz

import "testing"

// 包C Q2 语料回归（session-eval-20260827）：以战役真实话术 + 历史契约话术
// 钉住任务信号 / 派发信号 / 直答判定三函数的判别边界。每条语料标注来源
// 场景——新增失准话术先进本表再修词表/模式族，防「修一条破一条」。

// TestHasTaskActionSignal_Corpus 任务信号语料：方向一（该重判轻）失准话术
// 必须全部命中；闲聊/直答话术必须全部豁免（防方向二退化）。
func TestHasTaskActionSignal_Corpus(t *testing.T) {
	positive := map[string]string{
		// S06 中型组队：88 字明确组队诉求被判 confident_simple（0.1）——
		// 派发句式（让+市场部）+ 产出族（出+一+版）双命中。
		"S06-t1": "让数字内容媒体公司市场部出一版 Q3 推广文案框架，含三个渠道。",
		// S08 组织路由：产出族（出+一+条）命中。
		"S08": "出一条 30 秒产品宣传短视频的创意脚本框架，下周要用。",
		// S09-t1 长上下文首轮：规划…方案族命中（任务判定正确——S09 的
		// 失控在下游组队升级，由组队证据闸拦截，不在本门抑制）。
		"S09-t1": "我们来规划 Q3 的内容运营方案。先搭一个整体框架，包含渠道、节奏、预算三大块，每块简要说明。",
		// S07 重型编排：组织+汇总+方案，多命中（历史已通，防退化）。
		"S07-t1": "组织一次新产品上线方案：技术侧给发布计划，内容侧给宣传文案，运营侧给上线 checklist，最后汇总成一份方案。",
		// 历史契约（包B B2 管理层路由失效语料）。
		"B2-汇报": "把本周告警数据整理成汇报材料",
		"B2-排查": "请排查杭州滨江机房核心交换机最近一次告警的根因",
		"B2-方案": "给我一个容灾方案",
		"B2-组织": "组织一次季度复盘会",
		"B2-安排": "安排下周的值班表",
		// 复合任务 veto 契约（事实查询不得豁免任务）。
		"V2-复合": "核对昨天的天气数据并生成巡检报告",
		// 运维动作词（安装）是 V2 正确语义下的任务信号——原「连字符技能名
		// 不升级」期望是双词表漂移产物（agent 侧契约已改钉 true）。
		"运维-安装": "安装 slack-gif-creator 技能",
		// 词表词优先于咨询句式豁免：句首「为什么」+「排查」仍是工作命令。
		"咨询框+任务词": "为什么我的服务器一直告警，请帮我排查",
	}
	negative := map[string]string{
		"S01-问候":   "你好",
		"S01-闲聊":   "嗯嗯，我明白了，那就先这样吧",
		"S01-天气":   "今天天气怎么样",
		"S11-漂移":   "对了，你平时喜欢什么音乐",
		"S11-t5荐书": "推荐三本关于分布式系统的书",
		"介绍自己":    "介绍一下你自己",
		"时钟":      "现在几点",
		"唱首歌":     "唱一首晚安曲",
		// 纯咨询句式豁免（仅模式族层）：「写一个」命中最能产族①，但句首
		// 「如何」锚定为方法咨询——agent 侧既有契约钉住该例判 simple。
		"咨询-如何": "如何用Python写一个hello world",
		"咨询-怎么": "怎么理解 Raft 的选主流程",
	}
	for tag, msg := range positive {
		if !HasTaskActionSignal(msg) {
			t.Errorf("[%s] 应命中任务信号: %q", tag, msg)
		}
	}
	for tag, msg := range negative {
		if HasTaskActionSignal(msg) {
			t.Errorf("[%s] 不应命中任务信号: %q", tag, msg)
		}
	}
}

// TestHasDispatchSignal_Corpus 派发信号语料：组队证据闸的核心判别边界——
// S07（多实体派发，合法组队）与 S09-t1（自我规划+内容板块枚举，空组队）
// 必须判开。
func TestHasDispatchSignal_Corpus(t *testing.T) {
	positive := map[string]string{
		"S06-t1":  "让数字内容媒体公司市场部出一版 Q3 推广文案框架，含三个渠道。",
		"S07-t1":  "组织一次新产品上线方案：技术侧给发布计划，内容侧给宣传文案，运营侧给上线 checklist，最后汇总成一份方案。",
		"联合评估":   "请技术部和市场部联合评估这个方案",
		"交给部门":   "交给运维部处理这批告警",
		"派遣团队":   "派遣两个团队分别驻守两个机房",
	}
	negative := map[string]string{
		// S09-t1：渠道/节奏/预算是内容板块不是组织实体；「我们来规划」是
		// 自我规划不是派发。这是证据闸放行/降级的核心鉴别边界。
		"S09-t1": "我们来规划 Q3 的内容运营方案。先搭一个整体框架，包含渠道、节奏、预算三大块，每块简要说明。",
		// S08：单交付物直求，无组织实体（组织路由由角色驱动，非消息证据）。
		"S08":   "出一条 30 秒产品宣传短视频的创意脚本框架，下周要用。",
		"S11-t5": "推荐三本关于分布式系统的书",
		"让子弹飞":  "让子弹飞一会儿",
		"产品宣传":  "这个产品宣传片很精彩",
	}
	for tag, msg := range positive {
		if !HasDispatchSignal(msg) {
			t.Errorf("[%s] 应命中派发信号: %q", tag, msg)
		}
	}
	for tag, msg := range negative {
		if HasDispatchSignal(msg) {
			t.Errorf("[%s] 不应命中派发信号: %q", tag, msg)
		}
	}
}

// TestLooksLikeDirectAnswer_Corpus 直答判定语料（工具边界 Q2-C1）：明显直答
// 必须命中；复合任务必须被任务信号 veto。
func TestLooksLikeDirectAnswer_Corpus(t *testing.T) {
	positive := map[string]string{
		"S11-t5": "推荐三本关于分布式系统的书",
		"概念解释":   "什么是 Kubernetes",
		"观点":     "你觉得这本书怎么样",
		"科普":     "讲讲 Raft 共识算法",
	}
	negative := map[string]string{
		// 任务信号 veto：推荐+整理对比表格是复合任务，不得直答拦截。
		"veto-复合": "推荐三本微服务架构的书并整理成对比表格",
		"veto-写":  "帮我写一份推荐信",
		"天气":     "明天天气怎么样",
		"任务":     "推荐一个部署方案并落地执行",
	}
	for tag, msg := range positive {
		if !LooksLikeDirectAnswer(msg) {
			t.Errorf("[%s] 应判明显直答: %q", tag, msg)
		}
	}
	for tag, msg := range negative {
		if LooksLikeDirectAnswer(msg) {
			t.Errorf("[%s] 不应判明显直答: %q", tag, msg)
		}
	}
}

// TestLooksLikeFactQuery_TaskVetoPersists 既有契约防退化：模式族扩展后，
// 事实查询的任务 veto 与纯查询命中保持原样。
func TestLooksLikeFactQuery_TaskVetoPersists(t *testing.T) {
	if LooksLikeFactQuery("核对昨天的天气数据并生成巡检报告") {
		t.Error("复合任务不得命中事实查询快路径")
	}
	if !LooksLikeFactQuery("明天天气怎么样") {
		t.Error("纯天气查询应命中事实查询")
	}
	if LooksLikeFactQuery("推荐三本书") {
		t.Error("荐书不是事实查询")
	}
}
