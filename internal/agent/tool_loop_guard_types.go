package agent

import (
	"encoding/json"
	"time"
)

const (
	loopGuardBlockThreshold = 2 // 连续相同（签名+结果）成功调用达到此次数后，下一次起拦截
	loopGuardMaxEntries     = 512
	loopGuardEntryTTL       = 30 * time.Minute
	loopGuardResultCap      = 4096 // 结果哈希只取前 N 字节序列化文本，防超大结果拖慢
	loopGuardDigestCap      = 1500 // 拦截消息回放的上次真实结果摘要上限：必须覆盖取证关键证据
	// （2026-08-16 复验实证：200 截断使 ip link show 的 eth1 state DOWN 落在摘要之外，
	// 模型看不到证据判定「取证未完成」而顽固重发同参调用直至烧光预算）。
	loopGuardWindowCap       = 12 // 签名序列窗口长度上限（= 最大周期 4 × 最少重复 3）
	loopGuardCycleMaxPeriod  = 4  // 轮换循环检测的最大周期（每次轮换的工具数）
	loopGuardCycleMinRepeats = 3  // 轮换满 N 个完整周期才判定为循环
	// 同一节点隔离键下拦截满此次数后升级 StopError 强制终止节点（B4 止损）。
	// 取值权衡：2~3 次不足以区分「模型读不懂纠偏」与「模型顽固性重发」，
	// 5 次给足纠偏机会又远小于 max_tool_iterations，止损收益明确。
	loopGuardSaturatedStopThreshold = 5
	// 检索类工具连续空结果达到此次数后，本节点内禁止再调该工具（换词亦无新信息）。
	// 取值与 blockThreshold 对齐：首次空属正常未命中，第二次换词仍空即构成
	// 「库中确无资料」的足够证据，第三次起拦截。
	loopGuardEmptyStreakThreshold = 2
	// 同类业务错误（empty command / HTTP 4xx/5xx / ok:false）连续达到此次数后
	// 禁止再调该工具。与 emptyStreak 对齐：第 1、2 次放行记证据，第 3 次起拦。
	// 签名无关：cmd vs command 空命令不会因参数哈希不同而绕开。
	loopGuardErrorClassStreakThreshold = 2
	// Q1 行为模式闸（2026-08-27 二轮，S02「合法失控」根修）内置默认阈值。
	// S02 实证：spirit 节点 24 次异质 tool_load 全部"合法"（签名不同/结果成功/
	// 记有产出），同参/轮换/空结果/空转轮四重守卫均不命中——装载行为本身无闸。
	// 取值依据（Bansal《Agent Budgets and Runaway Prevention》「limits from
	// observed runs」）：合法渐进装载 1~4 次/节点，8 给足余量又远低于失控观测 24；
	// wall 软/硬闸对齐 LangGraph TimeoutPolicy 双超时（idle 软顶 + run 硬顶）与
	// 既有 A2'b 两段式（先引导后封锁）。三值均可经 agent_runtime_settings DB 列
	// 覆盖（PolicyResolver 每调用读取，0=跟随本默认）。
	loopGuardToolLoadMaxDefault = 8
	loopGuardWallSoftSecDefault = 240
	loopGuardWallHardSecDefault = 600
	// C1 装载循环占比闸（2026-08-28，S02 对症）：窗口内 tool_load/tool_search
	// 占比 ≥50% 即封锁新的自举调用。次数闸（配额 8）拦不住「每次签名不同
	// 的合法自举」；占比闸在异质装载耗尽配额前收敛。窗口 10、最少 6 次才
	// 判定，避免开场 2 次 tool_load 误伤。
	loopGuardBootstrapWindow       = 10
	loopGuardBootstrapMinCalls     = 6
	loopGuardBootstrapRatioDefault = 0.5
	// plan-execute 漂移（2026-08-28 由 record-only 升级为拦截）：plan_and_execute
	// 声明编排后节点仍持续本地 tool_load 达此次数 → 第 N+1 次装载拦截。观测
	// 阈值同时是执行阈值；测试用 setPlanDrift(-1) 关闭。
	loopGuardPlanDriftObserveAt = 3
	// 轮数×单轮积（2026-08-28）：计费 input ≈ llm_rounds × per-call assembly。
	// 50 轮闸与 1.5M run 闸都拦不住 32×17K≈544K 的合法失控。软闸注收尾 cue，
	// 硬闸封锁新工具（todo_declare_blocker 豁免）。HITL 等待不加轮数。
	loopGuardRoundProductSoftDefault = 350_000
	loopGuardRoundProductHardDefault = 500_000
	// 空转轮次早停（2026-08-25 包A-A2'b，session-eval-20260825 取证）：连续 N 轮
	// 工具调用零有效产出（失败/被拦/检索空结果）→ BeforeModel 注入降级引导；
	// 满 M 轮 → BeforeTool 拦截一切新调用（todo_declare_blocker 豁免，保留投降
	// 通道），顽固重发计入 blockedCount 共享饱和 StopError。
	// 实证：dept lead 会话 10 轮异质空转（memory_search 空 → list_inbox ×2 失败
	// → tool_load → declare_blocker）烧 80K in——同参/轮换/空结果三重守卫都管
	// 不到「跨工具异质空转」，按轮维度收口。
	unproductiveRoundGuideThreshold = 3
	unproductiveRoundBlockThreshold = 5
	// If AfterTool never runs (panic / skipped callback), inflight would
	// otherwise block the same signature for the rest of the invocation.
	// Must exceed the default tool execution timeout (10 min) so a still-
	// running call is not treated as leaked.
	loopGuardInflightStale = 15 * time.Minute
)

// loopGuardMarker 是拦截消息的可识别前缀；AfterTool 见到携带该标记的结果
// 时跳过状态更新（被拦调用的"结果"是纠偏文本，不代表真实工具输出）。
const loopGuardMarker = "⚠ 系统拦截（无效重复调用）"

// unproductiveRoundCueMarker 标识空转早停引导 cue（A2'b）。用于续轮去重——
// 每轮注入前先摘除历史中的同名 cue，保证只有一条最新轮次文案。无段分类
// （classifyAssemblyCue default=protected，引导指令宁保勿丢）。
const unproductiveRoundCueMarker = "<!-- aranea:unproductive_rounds -->\n"

// roundProductCueMarker 标识轮数×单轮积软闸 cue。
const roundProductCueMarker = "<!-- aranea:round_product -->\n"

// todoDeclareBlockerToolName 是空转封锁的唯一豁免工具（A2'b）：满 M 轮封锁
// 一切新调用时仍保留投降通道，模型可声明阻塞原因后收尾，而非被堵死重发。
const todoDeclareBlockerToolName = "todo_declare_blocker"

// toolLoadToolName / planAndExecuteToolName 是 Q1 行为模式闸观测的两个元工具：
// 装载闸统计 tool_load 配额与重复装载；plan-execute 漂移观测以 plan_and_execute
// 的成功调用为「计划已声明」锚点。
const (
	toolLoadToolName       = "tool_load"
	toolSearchToolName     = "tool_search"
	planAndExecuteToolName = "plan_and_execute"
)

// wallTimeCueMarker 标识 wall-time 软闸引导 cue（Q1）：与 unproductive cue 同型，
// 续轮注入前先摘除历史同名 cue，保证只有一条最新文案。
const wallTimeCueMarker = "<!-- aranea:wall_time -->\n"

type loopGuardEntry struct {
	lastSig          string // 上一次调用的签名（tool + 规范化 args 哈希）
	lastTool         string // 上一次调用的原始工具名（跨名重复时供拦截消息解释）
	lastResultKey    string // 上一次成功调用的结果哈希
	lastResultDigest string // 上一次成功调用的结果摘要（供拦截消息回放，防模型「证据丢失感」）
	sameCount        int    // 连续「签名+结果均相同」的成功调用次数
	lastFailed       bool   // 上一次调用是否失败（失败重试不累计、不拦截；FORBIDDEN 除外）
	forbiddenStreak  int    // 同签名连续 FORBIDDEN/not found 次数（≥2 后拦下一跳）
	recentSigs       []string
	recentTools      []string                // 与 recentSigs 对齐的工具名，仅供轮换拦截消息可读性
	emptyStreak      map[string]int          // 检索类工具名 → 连续空结果次数（无视参数差异；非空即清零）
	errorClassStreak map[string]int          // 工具名 → 连续同类业务错误次数（无视参数差异）
	lastErrorClass   map[string]string       // 工具名 → 上一跳业务错误类
	blockedCount     int                     // 本节点隔离键下累计被拦截次数（B4：满阈值升级 StopError）
	inflight         map[string]inflightSlot // 签名 → 当前正在执行（已放行、尚未 AfterTool）
	// deniedSigs 记录本节点生命周期内被 HITL 确认门禁否决的签名（P2-③，
	// R4-Q6 根修）：用户拒绝/无回复通道的工具调用不再执行、AfterTool 不运行，
	// 确认门禁经 noteConfirmationOutcome 归还 inflight 并登记于此；同一节点
	// 内同参重发即拦（阈值收紧为 1）。超时不登记（确认卡仍有效，用户授意的
	// 重发属合法路径）。条目随 invocation/team-run 键隔离，新对话轮次自然清零。
	deniedSigs map[string]int
	// 空转轮次早停状态（A2'b）：roundSawTool/roundProductive 累积当前 LLM 轮的
	// 工具结果，BeforeModel 续轮时结算进 unprodRounds（任一有产出的调用即清零）。
	roundSawTool    bool
	roundProductive bool
	unprodRounds    int
	// Q1 行为模式闸状态。firstSeen 是条目创建时间（wall-time 闸基准；条目因
	// 容量清理重建时闸重新计时，与计数类守卫的「重新计数」语义一致）。
	// loadedTools 记成功装载的目标（归一化名）：激活幂等且无运行时卸载
	// （deferred 包无 evict/unload 路径），重复装载恒零新信息，第二次起拦截；
	// loadCount 只计首次装载（被拦/失败/重复均不计），配额只约束异质装载面。
	firstSeen         time.Time
	loadCount         int
	loadedTools       map[string]bool
	planDeclared      bool // plan_and_execute 已成功（漂移观测锚点）
	postPlanLoads     int  // 计划声明后的 tool_load 成功次数
	planDriftRecorded bool // 漂移决策已写（once-per-entry 防刷屏）
	wallSoftRecorded  bool // 软闸决策已写（once-per-entry；cue 本身按轮刷新）
	roundProdRecorded bool // 轮数积软闸决策已写
	modelRounds       int  // BeforeModel 进入次数（含续轮）
	lastEst           int  // 最近一次 messages+schema 估 token
	// inflightLoads：本批并行尚未 AfterTool 的 tool_load 目标（含别名键）。
	// justLoaded：本 LLM 步内已成功激活、须等下一 model step 才能直调。
	inflightLoads map[string]int
	justLoaded    map[string]bool
	lastTouch     time.Time
}

type inflightSlot struct {
	count int
	since time.Time
}

type loopGuardBlockKind int

const (
	loopGuardBlockNone loopGuardBlockKind = iota
	loopGuardBlockParallel
	loopGuardBlockLoop
	loopGuardBlockCycle
	loopGuardBlockEmpty
	// Q1 行为模式闸（S02「合法失控」根修）的两个装载闸拦截形态：
	// 重复装载已激活工具（零新信息）与异质装载配额耗尽（装载面失控）。
	loopGuardBlockLoadRepeat
	loopGuardBlockLoadQuota
	loopGuardBlockLoadRatio
	loopGuardBlockLoadThenCall
	loopGuardBlockPlanDrift
	loopGuardBlockForbidden
	// loopGuardBlockDenied：同参重发刚被 HITL 否决的调用（P2-③）。
	loopGuardBlockDenied
	// loopGuardBlockErrorClass：同工具连续同类业务错误（ok:false / empty
	// command / HTTP 4xx）达到阈值后拦截下一跳。
	loopGuardBlockErrorClass
)

// loopGuardEmptyResultTools 登记纳入「空结果熔断」的检索类工具及其空判定谓词。
// 空结果是这些工具的合法终态（库中确无资料），模型换词重试不会产生新信息；
// 判定谓词按各工具的结果结构识别「空」，非空结果即清零该工具的连续空计数。
//
// 覆盖范围（2026-08-24 扩展，sh-04 同类风险收口）：knowledge_search 用专用
// 谓词（chunks 字段）；其余检索类工具统一用通用集合字段探测，与各工具真实
// 序列化形态对齐——memory_search=SearchMemoryResponse{results}、
// duckduckgo_search/web_research/google_search/arxiv_search/wikipedia_search
// 均为 {results}、skill_search 为 {skills}/{results} 形态。
var loopGuardEmptyResultTools = map[string]func(result any) bool{
	"knowledge_search":  loopGuardIsEmptySearchResult,
	"memory_search":     loopGuardGenericSearchEmpty,
	"skill_search":      loopGuardGenericSearchEmpty,
	"duckduckgo_search": loopGuardGenericSearchEmpty,
	"web_research":      loopGuardGenericSearchEmpty,
	"google_search":     loopGuardGenericSearchEmpty,
	"arxiv_search":      loopGuardGenericSearchEmpty,
	"wikipedia_search":  loopGuardGenericSearchEmpty,
}

// loopGuardEmptyCollectionKeys 是检索类工具结果中常见的集合字段名。
var loopGuardEmptyCollectionKeys = []string{
	"results", "items", "memories", "skills", "hits", "matches", "entries", "documents",
}

// loopGuardGenericSearchEmpty 检索类工具的通用空判定：结果序列化为 JSON object
// 后，任一常见集合字段存在且为空数组即判空。含 error 字段时不判空——失败重试
// 归熔断器治理，不与空结果熔断混淆。无集合字段的结果保守判非空（不误伤）。
func loopGuardGenericSearchEmpty(result any) bool {
	if result == nil {
		return false
	}
	b, err := json.Marshal(result)
	if err != nil {
		return false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return false
	}
	if _, hasErr := m["error"]; hasErr {
		return false
	}
	for _, k := range loopGuardEmptyCollectionKeys {
		raw, ok := m[k]
		if !ok {
			continue
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			continue
		}
		return len(arr) == 0
	}
	return false
}

// loopGuardIsEmptySearchResult 判定 knowledge_search 结果为空：
// 序列化形态 {"chunks":[...]}，空 = chunks 缺失或长度 0。
func loopGuardIsEmptySearchResult(result any) bool {
	if result == nil {
		return false
	}
	b, err := json.Marshal(result)
	if err != nil {
		return false
	}
	var probe struct {
		Chunks []json.RawMessage `json:"chunks"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return false
	}
	return len(probe.Chunks) == 0
}

type loopGuardVerdict struct {
	kind             loopGuardBlockKind
	cycleDesc        string
	saturated        bool
	blockedCount     int
	consecutiveCount int
	emptyStreak      int
	// forbiddenStreak 是同签名连续 FORBIDDEN/not found 次数（判定见
	// forbiddenLoop），禁止与 consecutiveCount（同参同结果连击）混用。
	forbiddenStreak int
	errorClass      string
	errorClassN     int
	lastDigest      string
	lastTool        string // 上一次调用的原始工具名（跨名重复拦截时解释用）
	// Q1 装载闸消息字段：loadTarget 是被拦 tool_load 的目标工具名；
	// loadCount/loadMax 是配额拦截时的当前装载数与配额上限。
	loadTarget string
	loadCount  int
	loadMax    int
	// threshold 是本次同参判定实际使用的连击阈值（补偿对工具收紧为 1，
	// 其余为 loopGuardBlockThreshold），仅供拦截消息展示「第 N 次起拦截」。
	threshold int
}

func (e *loopGuardEntry) inflightCount(sig string, now time.Time) int {
	if e.inflight == nil {
		return 0
	}
	slot, ok := e.inflight[sig]
	if !ok {
		return 0
	}
	if now.Sub(slot.since) > loopGuardInflightStale {
		delete(e.inflight, sig)
		return 0
	}
	return slot.count
}

func (e *loopGuardEntry) beginInflight(sig string, now time.Time) {
	if e.inflight == nil {
		e.inflight = map[string]inflightSlot{}
	}
	slot := e.inflight[sig]
	if slot.count == 0 {
		slot.since = now
	}
	slot.count++
	e.inflight[sig] = slot
}

func (e *loopGuardEntry) endInflight(sig string) {
	if e.inflight == nil {
		return
	}
	slot, ok := e.inflight[sig]
	if !ok {
		return
	}
	if slot.count <= 1 {
		delete(e.inflight, sig)
		return
	}
	slot.count--
	e.inflight[sig] = slot
}

// appendCallLocked 将一次真实（未被拦截的）调用追加进签名窗口，超额丢弃最旧。
func (e *loopGuardEntry) appendCallLocked(sig, tool string) {
	e.recentSigs = append(e.recentSigs, sig)
	e.recentTools = append(e.recentTools, tool)
	if len(e.recentSigs) > loopGuardWindowCap {
		e.recentSigs = e.recentSigs[len(e.recentSigs)-loopGuardWindowCap:]
		e.recentTools = e.recentTools[len(e.recentTools)-loopGuardWindowCap:]
	}
}
