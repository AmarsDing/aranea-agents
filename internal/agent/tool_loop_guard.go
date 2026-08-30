package agent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/tools/alias"
	"aranea-agents/internal/tools/inverse"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// 工具循环守卫（2026-08-15 SW1 事故根修）。
//
// 背景：max_tool_iterations 由 vendored 框架一刀切硬停，不区分调用内容；
// 熔断器只感知「连续失败」。二者都管不住「同工具+同参数+同结果」的成功空转
// ——SW1 事故中 remediate agent 以相同参数连调 20 次 gns3_exec（全部成功），
// 耗尽迭代配额，始终未推进到 fault_clear。
//
// 判定语义（区分「死循环」与「持续干活」）：
//   - 同工具 + 相同参数签名 + 上次成功 + 结果内容一致，连续 >=3 次 → 无效空转，拦截；
//   - 多工具固定轮换（如 A→B→C 满 3 轮，周期 p∈[2,4]）→ 节点内轮询空转，拦截；
//   - 检索类工具连续空结果（换词重试 ≥2 次仍空）→ 库中确无资料，本节点内禁调该工具；
//   - 同工具不同参数（递进式调研/干活）→ 放行；
//   - 同参数但上次失败（重试）→ 放行（失败治理归熔断器）；
//   - 同参数但结果有变化（轮询拿到新数据）→ 放行并重置计数。
//
// 空结果熔断的实证（2026-08-18 域 B 评测 sh-04/ab-04 超时事故）：team KB 纯词法库
// 无语义层，knowledge_search 降级 BM25 返回空；模型将空结果误读为「查询方式不对」
// 而无限换词重试——每次新参数生成新签名绕过 p=1 判定，且不重复固定模式使轮换检测
// 同样失效，单 turn 烧到 157+ 次模型调用 / 260K+ tokens 直至客户端超时。空结果对
// 检索类工具是合法终态（库中确实无资料），连续空即构成「换词亦无新信息」的确定性
// 证据，故按工具维度（无视参数差异）熔断，拦截消息引导模型直接作答/声明未收录。
//
// 轮换循环的实证（2026-08-16 复验）：verify 节点以 health_check→alarm_get→exec
// 三工具轮换约 10 轮 ≈30 次调用——交替调用使 lastSig 比较恒失效、结果内嵌时间戳
// 使结果哈希恒变，p=1 判定无从触发；且节点指令明文「禁止第 2 轮取证」被无视，
// 最终靠迭代上限强制收敛。周期检测作为 p=1 判定的补充网：只命中含 ≥2 种签名的
// 异质循环，同质连调（p=1）仍归结果感知判定，不误伤「结果在变化」的合法轮询。
//
// 拦截不是静默拒绝：BeforeTool 返回普通 error，框架将其转为 RoleTool 的
// error 消息回灌模型，agent 循环继续、节点不中断（2026-08-16 B1 根修：此前用
// CustomResult 以「成功结果」形态返回纠偏文本，模型将其误读为取证失败/证据不足
// 而顽固重发同参调用，16 次预算烧光仍未推进 fault_clear；error 形态与正常结果
// 走不同处理路径，显著性更高）。被拦调用不进 AfterTool（框架提前 return），
// 守卫计数不清零，后续相同调用持续被拦，模型只能通过换参数/换工具/收尾输出推进。
//
// 饱和升级（2026-08-16 B4 止损）：复验实证存在模型连 error 形态纠偏也无视、
// 反复重发直至烧光预算的顽固样本。拦截按节点隔离键累计 blockedCount，达到
// loopGuardSaturatedStopThreshold 次后升级为 StopError——框架（functioncall.go
// executeToolCall）对 StopError 走 shouldIgnoreError=false 路径直接上抛，
// 强制终止当前节点运行止损：节点以失败快速收尾（图谱重试/critic 收敛机制接管），
// 优于让模型继续空转耗尽全部迭代预算后仍以失败告终。
//
// 并行窗口：同一轮响应内相同签名的多次 BeforeTool 在 AfterTool 记账之前
// 会同时到达。entry.inflight 只放行第一次，其余立即拦截（普通 error，
// 不计入 blockedCount / 饱和止损），避免首轮扇出直接 StopError。
//
// Q1 行为模式闸（2026-08-27 二轮，S02「合法失控」根修）：旧四重守卫
// （同参/轮换/空结果/空转轮）全部针对「调用内容形态」，管不住 S02 实证的
// 「合法失控」——spirit 节点 24 次异质 tool_load 每次签名不同、结果成功、
// 记有产出，四重守卫均不命中，装载行为本身无闸。新增三闸：
//   - 装载闸（BeforeTool 拦截）：重复装载已激活工具第二次起拦截（激活幂等、
//     无运行时卸载，重复装载恒零新信息）；异质装载配额（默认 8，DB 列
//     loop_guard_tool_load_max 可覆盖）耗尽后拦截新装载。被拦计入
//     blockedCount，共享 B4 饱和 StopError。
//   - wall-time 软/硬闸：节点存续超软闸（默认 240s，loop_guard_wall_soft_sec）
//     由 BeforeModel 注入收尾引导 cue（对齐 LangGraph TimeoutPolicy 双超时与
//     A2'b 先引导后封锁两段式）；超硬闸（默认 600s，loop_guard_wall_hard_sec）
//     由 BeforeTool 返回 StopError 强制终止节点。计时基准 entry.firstSeen，
//     跨节点重试累计（反复重跑持续烧预算正是硬闸要拦的形态）。
//   - plan-execute 漂移拦截：plan_and_execute 成功声明编排后节点仍持续本地
//     装载达 3 次 → 第 4 次 tool_load 拦截（测试用 setPlanDrift(-1) 关闭）。
//   - 轮数×单轮积闸：BeforeModel 累计 modelRounds×lastEst；软阈注入收尾 cue，
//     硬阈封锁新工具（todo_declare_blocker 豁免）。HITL 等待不加轮数。
//
// 三阈值经 PolicyResolver 每调用读取（0=内置默认），策略变更零重建生效。
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

type toolLoopGuard struct {
	mu      sync.Mutex
	entries map[string]*loopGuardEntry
	lg      loggateway.Logger
	// decisions 是 M80 系统闸决策双写口（设计 §3.2 row 3 loop_guard_blocked）。
	// 可选：nil 时拦截仅回灌错误消息，不产决策记录。
	decisions decision.Collector
	// Q1 行为模式闸阈值的每调用解析入参：policyAgentID 是 resolver 查询键，
	// build* 是构建期快照（resolver miss 兜底；≤0 归一内置默认）。装配点
	// callback_chain.go 经 setGateThresholds 注入（与 setDecisionCollector 同型，
	// 保持 newToolLoopGuard(lg) 签名不变，存量测试零改动）。
	policyAgentID string
	buildLoadMax  int
	buildWallSoft int
	buildWallHard int
	// buildBootstrapRatio：<0 测试关闭占比闸；0 用默认 0.5；>0 覆盖。
	buildBootstrapRatio float64
	// buildPlanDrift：<0 测试关闭漂移拦截；0 用默认观察阈值并拦截；>0 覆盖阈值。
	buildPlanDrift int
	// buildRoundProduct：<0 关闭轮数积闸；0 用默认 350K/500K；>0 覆盖硬阈（软=70%）。
	buildRoundProduct int
}

func newToolLoopGuard(lg loggateway.Logger) *toolLoopGuard {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &toolLoopGuard{entries: map[string]*loopGuardEntry{}, lg: lg}
}

// setDecisionCollector wires the M80 decision collector (callback_chain.go
// 装配点）。保持 newToolLoopGuard(lg) 签名不变，存量测试零改动。
func (g *toolLoopGuard) setDecisionCollector(c decision.Collector) {
	if g == nil {
		return
	}
	g.decisions = c
}

// setGateThresholds 注入 Q1 行为模式闸的每调用解析入参（callback_chain.go
// 装配点）：agentID 作 resolver 查询键，build* 为构建期快照兜底。阈值本体
// 不固化进守卫——每调用经 loopGuardToolLoadMaxFor / loopGuardWallSecFor 查询
// （resolver 命中用 resolver 值），策略变更零重建生效。
func (g *toolLoopGuard) setGateThresholds(agentID string, loadMax, wallSoftSec, wallHardSec int) {
	if g == nil {
		return
	}
	g.policyAgentID = agentID
	g.buildLoadMax = loadMax
	g.buildWallSoft = wallSoftSec
	g.buildWallHard = wallHardSec
}

// 每调用阈值解析（Q1）：resolver 优先、构建期快照兜底、≤0 归一内置默认。
func (g *toolLoopGuard) loadMaxFor() int {
	return loopGuardToolLoadMaxFor(g.policyAgentID, g.buildLoadMax)
}

func (g *toolLoopGuard) wallSecFor(hard bool) int {
	build := g.buildWallSoft
	if hard {
		build = g.buildWallHard
	}
	return loopGuardWallSecFor(g.policyAgentID, build, hard)
}

// wallElapsedNet is node wall time minus HITL confirmation wait. S05 turns
// spent 300s of 314s waiting on confirm; counting that toward the 240/600s
// gates false-kills paused nodes (campaign lesson: wall-time must exclude wait).
func wallElapsedNet(ctx context.Context, firstSeen, now time.Time) time.Duration {
	elapsed := now.Sub(firstSeen)
	if elapsed < 0 {
		return 0
	}
	wait := time.Duration(ConfirmWaitMS(ctx)) * time.Millisecond
	if wait <= 0 {
		return elapsed
	}
	if wait >= elapsed {
		return 0
	}
	return elapsed - wait
}

func (g *toolLoopGuard) setBootstrapRatio(r float64) {
	if g == nil {
		return
	}
	g.buildBootstrapRatio = r
}

func (g *toolLoopGuard) setPlanDrift(n int) {
	if g == nil {
		return
	}
	g.buildPlanDrift = n
}

func (g *toolLoopGuard) planDriftAt() int {
	if g == nil {
		return loopGuardPlanDriftObserveAt
	}
	if g.buildPlanDrift < 0 {
		return 0
	}
	if g.buildPlanDrift > 0 {
		return g.buildPlanDrift
	}
	return loopGuardPlanDriftObserveAt
}

func (g *toolLoopGuard) setRoundProduct(hard int) {
	if g == nil {
		return
	}
	g.buildRoundProduct = hard
}

func (g *toolLoopGuard) roundProductHard() int {
	if g == nil {
		return loopGuardRoundProductHardDefault
	}
	if g.buildRoundProduct < 0 {
		return 0
	}
	if g.buildRoundProduct > 0 {
		return g.buildRoundProduct
	}
	return loopGuardRoundProductHardDefault
}

func (g *toolLoopGuard) roundProductSoft() int {
	hard := g.roundProductHard()
	if hard <= 0 {
		return 0
	}
	return int(float64(hard) * 0.7)
}

func (g *toolLoopGuard) bootstrapRatioFor() float64 {
	if g == nil {
		return loopGuardBootstrapRatioDefault
	}
	if g.buildBootstrapRatio < 0 {
		return 0
	}
	if g.buildBootstrapRatio > 0 {
		return g.buildBootstrapRatio
	}
	return loopGuardBootstrapRatioDefault
}

func loopGuardIsBootstrapTool(name string) bool {
	return name == toolLoadToolName || name == toolSearchToolName
}

func loopGuardResolveAlias(name string) string {
	name = strings.TrimSpace(name)
	for i := 0; i < 8 && name != ""; i++ {
		next, ok := alias.RuntimeToolNameAliases[name]
		if !ok || next == "" || next == name {
			return name
		}
		name = next
	}
	return name
}

func loopGuardNameKeys(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	seen := map[string]struct{}{name: {}, loopGuardResolveAlias(name): {}}
	for aliasName, target := range alias.RuntimeToolNameAliases {
		if target == name || aliasName == name || loopGuardResolveAlias(aliasName) == loopGuardResolveAlias(name) {
			seen[aliasName] = struct{}{}
			seen[target] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

func loopGuardBootstrapRatioTripped(recentTools []string, next string, ratio float64) bool {
	if ratio <= 0 || !loopGuardIsBootstrapTool(next) {
		return false
	}
	w := append(append([]string(nil), recentTools...), next)
	if len(w) > loopGuardBootstrapWindow {
		w = w[len(w)-loopGuardBootstrapWindow:]
	}
	if len(w) < loopGuardBootstrapMinCalls {
		return false
	}
	n := 0
	for _, t := range w {
		if loopGuardIsBootstrapTool(t) {
			n++
		}
	}
	return float64(n) >= float64(len(w))*ratio
}

func (e *loopGuardEntry) beginInflightLoad(target string) {
	if target == "" {
		return
	}
	if e.inflightLoads == nil {
		e.inflightLoads = map[string]int{}
	}
	for _, k := range loopGuardNameKeys(target) {
		e.inflightLoads[k]++
	}
}

func (e *loopGuardEntry) endInflightLoad(target string) {
	if e.inflightLoads == nil || target == "" {
		return
	}
	for _, k := range loopGuardNameKeys(target) {
		if e.inflightLoads[k] <= 1 {
			delete(e.inflightLoads, k)
			continue
		}
		e.inflightLoads[k]--
	}
}

func (e *loopGuardEntry) markJustLoaded(target string) {
	if target == "" {
		return
	}
	if e.justLoaded == nil {
		e.justLoaded = map[string]bool{}
	}
	for _, k := range loopGuardNameKeys(target) {
		e.justLoaded[k] = true
	}
}

func (e *loopGuardEntry) loadThenCallTarget(toolName string) string {
	if toolName == "" || loopGuardIsBootstrapTool(toolName) {
		return ""
	}
	for _, k := range loopGuardNameKeys(toolName) {
		if e.inflightLoads[k] > 0 || e.justLoaded[k] {
			return k
		}
	}
	return ""
}

// emitGateDecision 把一次拦截/强停双写为 system_guard 决策记录。
// observed/threshold/action 由调用点按分支语义给出。
func (g *toolLoopGuard) emitGateDecision(ctx context.Context, toolName, scenario, reasoning string, observed any, threshold any, action string) {
	g.emitGateEvent(ctx, "blocked", toolName, scenario, reasoning, observed, threshold, action)
}

// emitGateEvent 是 emitGateDecision 的 outcome 参数化形态（Q1）：装载/重复
// 拦截与既有守卫同用 outcome=blocked；wall-time / 轮数积软闸用
// outcome=tripped 落决策记录（once-per-entry）但不在该钩子拦截。
// plan-execute 漂移达阈值后由 BeforeTool 拦截（blocked）。
// toolName 为空时省略 tool 实体（软闸无单一触发工具）。
func (g *toolLoopGuard) emitGateEvent(ctx context.Context, outcome, toolName, scenario, reasoning string, observed any, threshold any, action string) {
	runID := gateRunID(ctx)
	var entities []decision.EntityRef
	if toolName != "" {
		entities = []decision.EntityRef{{Type: "tool", Key: toolName}}
	}
	event.EmitGate(ctx, g.decisions, decision.GateDecision{
		TriggerRule:   decision.TriggerLoopGuardBlocked,
		Outcome:       outcome,
		Scenario:      scenario,
		Reasoning:     reasoning,
		GuardName:     "tool_loop_guard",
		RunID:         runID,
		SessionID:     gateSessionID(ctx),
		Entities:      entities,
		ObservedValue: observed,
		Threshold:     threshold,
		Action:        action,
	})
}

// loopGuardInvocationKey 取守卫条目的隔离键：invocation + agent（=图谱节点身份）。
// 取不到时返回空串（守卫 fail-open，不影响工具执行）。
//
// 为什么带 AgentName（2026-08-16 复验实证「连坐」）：图谱执行时各节点共享
// run 级身份（彼时 InvocationID 全链路相同；2026-08-27 框架 Clone 补丁后成员
// 子 invocation 每次节点执行换新 uuid，run 级身份改经 ctx 注入的 team run id
// 表达，见下方优先取值），diagnose 第3步的 gns3_exec 取证与 remediate 调用1
// 同参同结果，计数跨节点累计 → remediate 首次调用即撞阈值被拦，真实取证额度
// 被压缩，模型「确认」反射未满足陷入重发死循环。按 AgentName 隔离后，各节点
// 独立享有完整计数额度；同节点跨执行仍共享（run id 不变），跨轮循环仍受控。
func loopGuardInvocationKey(ctx context.Context) string {
	// 2026-08-27 二轮审查 H5 顺带根修：team 图谱下优先取 ctx 注入的 team
	// run id——Clone 补丁后以 InvocationID 为键会让跨执行（replanner 重跑
	// 节点）的循环计数清零，守卫退化为单执行内有效；run id 稳定则恢复
	// 「按节点隔离、跨执行累计」语义。
	if id := decision.GateRunIDFromContext(ctx); id != "" {
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.AgentName != "" {
			return id + "|" + inv.AgentName
		}
		return id
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return ""
	}
	if inv.AgentName != "" {
		return inv.InvocationID + "|" + inv.AgentName
	}
	return inv.InvocationID
}

// loopGuardSignature 规范化参数 JSON（map 键排序由 encoding/json 保证）后
// 与工具名一起哈希，容忍空白/键序差异造成的伪不同参数。
// 字符串值再做语义级归一（P2c）：LLM 转写参数时的码点/空白微差异
// （" poem-a " / 全角 "ｐｏｅｍ－ａ"）不再产生新签名绕过守卫。
// 工具名先归一到别名家簇定点（P1c）：同一底层能力的名字变体
// （shell / shell_exec / hostexec_exec_command / exec_command）收敛为同一签名——
// 17:03 诗歌会话实证模型以三个工具名重发同一条 curl 命令，签名随工具名变化
// 绕过「签名+结果一致」判定。
func loopGuardSignature(toolName string, args []byte) string {
	canonical := strings.TrimSpace(string(args))
	var payload any
	if err := json.Unmarshal(args, &payload); err == nil {
		if b, mErr := json.Marshal(loopGuardNormalizeArgStrings(payload)); mErr == nil {
			canonical = string(b)
		}
	}
	sum := sha1.Sum([]byte(loopGuardCanonicalToolName(toolName) + "\x00" + canonical))
	return hex.EncodeToString(sum[:])
}

// loopGuardAliasClusterMembers 是 RuntimeToolNameAliases 别名链的源与终点集合
// （包级预计算）。用于识别「ToolSet 前缀_家簇成员」形态的名字（hostexec_exec_command）。
var loopGuardAliasClusterMembers = func() map[string]bool {
	m := make(map[string]bool, len(alias.RuntimeToolNameAliases)*2)
	for src, dst := range alias.RuntimeToolNameAliases {
		m[src] = true
		m[dst] = true
	}
	return m
}()

// loopGuardCanonicalToolName 将同一底层能力的工具名变体归一为家簇定点（2026-08-21 P1c）。
// 规则：
//   - 名字在别名链上（shell / shell_exec）→ 链定点（exec_command）；
//   - ToolSet 前缀形态且后缀是家簇成员（hostexec_exec_command / file_save_file）
//     → 后缀的链定点（exec_command / save_file）——同一 agent 内同一底层工具
//     经别名包装后以不同名字暴露，同参数调用属语义重复；
//   - 其余名字原样返回（不参与跨名去重，避免误伤无关工具）。
func loopGuardCanonicalToolName(name string) string {
	if f := loopGuardAliasFixedPoint(name); loopGuardAliasClusterMembers[name] {
		return f
	}
	if i := strings.Index(name, "_"); i > 0 && i < len(name)-1 {
		suffix := name[i+1:]
		if loopGuardAliasClusterMembers[suffix] {
			return loopGuardAliasFixedPoint(suffix)
		}
	}
	return name
}

// loopGuardAliasFixedPoint 沿 RuntimeToolNameAliases 链走到无出边的定点，带环保护。
func loopGuardAliasFixedPoint(name string) string {
	visited := map[string]bool{}
	for {
		if visited[name] {
			return name
		}
		visited[name] = true
		next, ok := alias.RuntimeToolNameAliases[name]
		if !ok {
			return name
		}
		name = next
	}
}

// loopGuardNormalizeArgStrings 递归归一参数中的字符串值（2026-08-21 P2c）。
// 与 deliverable topic 同规则（trim+NFC+全角折叠，biz.NormalizeDeliverableTopic）：
// 诗歌会话实证模型以 " poem-a " / "ｐｏｅｍ－ａ" 等码点微差异参数重发同一语义
// 调用，每次都生成新签名绕过「签名+结果一致」判定；归一后语义相同调用收敛为
// 同一签名。不做大小写折叠——大小写差异可能是合法的不同参数（对齐 topic 的
// 大小写敏感语义）。只影响签名哈希，不改写下发给工具的真实参数。
func loopGuardNormalizeArgStrings(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = loopGuardNormalizeArgStrings(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = loopGuardNormalizeArgStrings(val)
		}
		return t
	case string:
		return biz.NormalizeDeliverableTopic(t)
	default:
		return v
	}
}

func loopGuardResultKey(result any) string {
	if result == nil {
		return ""
	}
	var text string
	if s, ok := result.(string); ok {
		text = s
	} else if b, err := json.Marshal(result); err == nil {
		text = string(b)
	} else {
		text = fmt.Sprintf("%v", result)
	}
	if len(text) > loopGuardResultCap {
		text = text[:loopGuardResultCap]
	}
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}

// loopGuardToolLoadTarget 从 tool_load 参数中解析目标工具名（Q1 装载闸）。
// 归一规则与签名归一同级（TrimSpace；工具名为 ASCII 标识符，码点微差异
// 不产生语义区别）。解析失败返回空串——守卫 fail-open，参数缺失由工具
// 自身报错，不归循环守卫治理。
func loopGuardToolLoadTarget(args []byte) string {
	var in struct {
		ToolName string `json:"tool_name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return ""
	}
	return strings.TrimSpace(in.ToolName)
}

// loopGuardToolLoadResult 解析 tool_load 成功结果（Q1 装载闸记账）：
// ok=false 表示装载未成功（success:false / 结果不可解析），不计入装载面。
// name 优先取结果中的归一后规范名（manager.ResolveName 定点），缺失时回退
// 请求名——规范名与请求名都会记入 loadedTools，覆盖别名变体重复装载。
func loopGuardToolLoadResult(result any, requested string) (name string, ok bool) {
	if result == nil {
		return "", false
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "", false
	}
	var out struct {
		Success  bool   `json:"success"`
		ToolName string `json:"tool_name"`
	}
	if err := json.Unmarshal(b, &out); err != nil || !out.Success {
		return "", false
	}
	name = strings.TrimSpace(out.ToolName)
	if name == "" {
		name = requested
	}
	if name == "" {
		return "", false
	}
	return name, true
}

// loopGuardCyclePeriod 检测 seq 末尾是否由同一 p 长度块连续重复 minRepeats 次构成
// （如 ABCABCABC）。命中返回周期 p，未命中返回 0。
// 只认 p∈[2,loopGuardCycleMaxPeriod] 且基块含 ≥2 种签名的异质循环——同质连调
// （p=1 域）归「签名+结果一致」判定，避免误伤结果在变化的合法轮询。
func loopGuardCyclePeriod(seq []string, minRepeats int) int {
	n := len(seq)
	for p := 2; p <= loopGuardCycleMaxPeriod; p++ {
		need := p * minRepeats
		if n < need {
			continue
		}
		tail := seq[n-need:]
		distinct := false
		ok := true
		for i := 0; i < need; i++ {
			if tail[i] != tail[i%p] {
				ok = false
				break
			}
			if i < p && tail[i] != tail[0] {
				distinct = true
			}
		}
		if ok && distinct {
			return p
		}
	}
	return 0
}

// loopGuardResultDigest 提取结果的可读摘要：压平空白为单空格、按字节截断。
// 用于拦截消息向模型回放「上次真实结果」，防止模型把拦截误读为取证失败
// （2026-08-16 复验实证：旧文案「未获得任何新信息」被模型解读成证据未拿到，
// 反复重试同参取证，始终不推进下一步）。
func loopGuardResultDigest(result any) string {
	if result == nil {
		return ""
	}
	var text string
	if s, ok := result.(string); ok {
		text = s
	} else if b, err := json.Marshal(result); err == nil {
		text = string(b)
	} else {
		text = fmt.Sprintf("%v", result)
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > loopGuardDigestCap {
		text = text[:loopGuardDigestCap] + "…"
	}
	return text
}

func (g *toolLoopGuard) entryLocked(key string, now time.Time) *loopGuardEntry {
	if len(g.entries) >= loopGuardMaxEntries {
		cutoff := now.Add(-loopGuardEntryTTL)
		for k, e := range g.entries {
			if e.lastTouch.Before(cutoff) {
				delete(g.entries, k)
			}
		}
		// 过期清理仍超容时清空最旧一半之外的兜底过严，直接整体重置更简洁；
		// 丢失历史仅意味着守卫重新计数，不影响正确性。
		if len(g.entries) >= loopGuardMaxEntries {
			g.entries = map[string]*loopGuardEntry{}
		}
	}
	e, ok := g.entries[key]
	if !ok {
		e = &loopGuardEntry{firstSeen: now}
		g.entries[key] = e
	}
	e.lastTouch = now
	return e
}

func (g *toolLoopGuard) verdictBeforeLocked(e *loopGuardEntry, sig, toolName string, args []byte, now time.Time) loopGuardVerdict {
	if e.inflightCount(sig, now) > 0 {
		return loopGuardVerdict{kind: loopGuardBlockParallel}
	}
	// P2-③ HITL 恢复路径（R4-Q6 根修）：本签名刚被确认门禁否决（用户拒绝/
	// 无回复通道，inflight 已由 noteConfirmationOutcome 归还）——同一节点
	// 生命周期内同参重发即拦。被拦调用不进 AfterTool，blockedCount 与本轮
	// 工具活动只能在此记账（与下方通用拦截路径同构）。
	if e.deniedSigs[sig] > 0 {
		e.blockedCount++
		e.roundSawTool = true
		return loopGuardVerdict{
			kind:         loopGuardBlockDenied,
			blockedCount: e.blockedCount,
			saturated:    e.blockedCount >= loopGuardSaturatedStopThreshold,
		}
	}
	// Q1 装载闸（S02「合法失控」根修）：优先于签名/轮换/空结果判定——
	// tool_load 的异质调用序列对三重旧守卫全部"合法"，必须由装载语义自身收口。
	//   - 重复装载：目标已在 loadedTools（激活幂等、无运行时卸载，重复装载恒零
	//     新信息），第二次起拦截，引导模型直接调用已激活工具；
	//   - 配额：异质装载面 loadCount 达阈值（默认 8，DB 可覆盖）后拦截新装载——
	//     装载行为本身即失控向量（S02 观测 24 次异质装载），与装载结果成功与否无关。
	//   - 占比闸：窗口内 tool_load/tool_search ≥50% 封锁新自举（C1，异质装载
	//     在配额耗尽前收敛）。
	//   - 同批直调：刚 tool_load 的目标不能在同一 model step / 并行批次里立刻 call。
	// 参数不可解析时跳过装载闸（fail-open，参数错误归工具自身报错）。
	var loadTarget string
	loadRepeat := false
	loadQuota := false
	loadRatio := false
	loadThenCall := ""
	loadMax := 0
	if toolName == toolLoadToolName {
		loadTarget = loopGuardToolLoadTarget(args)
		if loadTarget != "" {
			loadMax = g.loadMaxFor()
			if e.loadedTools[loadTarget] {
				loadRepeat = true
			} else if e.loadCount >= loadMax {
				loadQuota = true
			}
		}
	}
	if !loadRepeat && !loadQuota && loopGuardBootstrapRatioTripped(e.recentTools, toolName, g.bootstrapRatioFor()) {
		loadRatio = true
		if loadTarget == "" {
			loadTarget = toolName
		}
	}
	planDrift := false
	if !loadRepeat && !loadQuota && !loadRatio {
		if at := g.planDriftAt(); at > 0 && e.planDeclared && e.postPlanLoads >= at && toolName == toolLoadToolName {
			planDrift = true
			if loadTarget == "" {
				loadTarget = loopGuardToolLoadTarget(args)
			}
		}
	}
	if !loadRepeat && !loadQuota && !loadRatio {
		loadThenCall = e.loadThenCallTarget(toolName)
	}
	// 空结果熔断优先于签名/轮换判定：库中确无资料时，任何参数的再调用都无意义，
	// 拦截消息也比通用重复文案更具行动指引（直接作答/声明未收录）。
	emptyBlocked := e.emptyStreak[toolName] >= loopGuardEmptyStreakThreshold
	// P2-③ 补偿对阈值收紧（R4-Q6 根修）：副作用工具的副作用不可重放——
	// 重复 inject 不叠加新效果、重复 clear 幂等空转，首次成功即视为终态，
	// 第 2 次同参同结果调用起拦（含审批通过后的重放形态）；其余工具维持
	// 默认阈值（取证确认属合理模式）。
	loopThreshold := loopGuardBlockThreshold
	if loopGuardCompensationPairTool(toolName) {
		loopThreshold = 1
	}
	loop := e.lastSig == sig && !e.lastFailed && e.sameCount >= loopThreshold
	forbiddenLoop := e.lastSig == sig && e.forbiddenStreak >= 2
	var cycleDesc string
	if !loop && len(e.recentSigs)+1 >= 2*loopGuardCycleMinRepeats {
		// 试追加当前签名，检测末尾是否构成固定轮换循环（如 A→B→C 满 3 轮）。
		trialSigs := append(append([]string(nil), e.recentSigs...), sig)
		if p := loopGuardCyclePeriod(trialSigs, loopGuardCycleMinRepeats); p >= 2 {
			trialTools := append(append([]string(nil), e.recentTools...), toolName)
			cycleDesc = strings.Join(trialTools[len(trialTools)-p:], " → ")
		}
	}
	if !loadRepeat && !loadQuota && !loadRatio && !planDrift && loadThenCall == "" && !emptyBlocked && !loop && !forbiddenLoop && cycleDesc == "" {
		e.beginInflight(sig, now)
		if toolName == toolLoadToolName && loadTarget != "" {
			e.beginInflightLoad(loadTarget)
		}
		return loopGuardVerdict{kind: loopGuardBlockNone}
	}
	// B4 饱和止损：被拦调用不进 AfterTool，计数只能在此累计。
	e.blockedCount++
	// A2'b：被拦调用不进 AfterTool，本轮工具活动只能在此记账（被拦=零产出，
	// roundProductive 保持 false，供 BeforeModel 结算空转轮次）。
	e.roundSawTool = true
	v := loopGuardVerdict{
		blockedCount:     e.blockedCount,
		consecutiveCount: e.sameCount,
		emptyStreak:      e.emptyStreak[toolName],
		forbiddenStreak:  e.forbiddenStreak,
		lastDigest:       e.lastResultDigest,
		lastTool:         e.lastTool,
		loadTarget:       loadTarget,
		loadCount:        e.loadCount,
		loadMax:          loadMax,
		saturated:        e.blockedCount >= loopGuardSaturatedStopThreshold,
		threshold:        loopThreshold,
	}
	if loadThenCall != "" {
		v.kind = loopGuardBlockLoadThenCall
		v.loadTarget = loadThenCall
		return v
	}
	if loadRepeat {
		v.kind = loopGuardBlockLoadRepeat
		return v
	}
	if loadQuota {
		v.kind = loopGuardBlockLoadQuota
		return v
	}
	if loadRatio {
		v.kind = loopGuardBlockLoadRatio
		return v
	}
	if planDrift {
		v.kind = loopGuardBlockPlanDrift
		v.loadCount = e.postPlanLoads
		return v
	}
	if emptyBlocked {
		v.kind = loopGuardBlockEmpty
		return v
	}
	if cycleDesc != "" {
		v.kind = loopGuardBlockCycle
		v.cycleDesc = cycleDesc
		return v
	}
	if forbiddenLoop {
		v.kind = loopGuardBlockForbidden
		return v
	}
	v.kind = loopGuardBlockLoop
	return v
}

// loopGuardCompensationPairTool 报告工具是否属于已登记的补偿对（正向或逆向）。
// 补偿对工具的副作用不可重放（重复 inject 不叠加新效果、重复 clear 幂等空转），
// 同参去重阈值收紧为 1（P2-③，R4-Q6 根修：审批通过后的同参重放首次即拦）。
// 注册表为进程级只读快照（构造路径幂等注册），锁外调用安全。
func loopGuardCompensationPairTool(toolName string) bool {
	if _, ok := inverse.LookupForward(toolName); ok {
		return true
	}
	return inverse.IsInverse(toolName)
}

// noteConfirmationOutcome 由 HITL 确认门禁在非批准出口调用（P2-③，R4-Q6 根修）。
//
// 背景：确认门禁（priority 10）在循环守卫（priority 4）放行后才等待用户裁定，
// 守卫已为该签名 beginInflight；非批准出口下工具不再执行、框架短路返回
// CustomResult、AfterTool 不运行，inflight 槽位必须由本函数显式归还——否则
// 陈旧期（loopGuardInflightStale）内的重发会被误判为「并行重复」。
//
// denied=true（用户明确拒绝 / 无回复通道）额外把签名登记进 deniedSigs：同一
// 节点生命周期内同参重发即拦，拦截消息由 beforeHook 拼接方案 C 补偿引导。
// denied=false（确认超时 / 等待异常）只归还槽位不登记——确认卡仍然有效，
// 用户授意的重发必须能再次走进确认流程。
//
// 批准出口严禁调用本函数：工具继续执行，inflight 由 AfterTool 正常归还，
// 重复归还会吞掉后续真实并行调用的计数。
func (g *toolLoopGuard) noteConfirmationOutcome(ctx context.Context, toolName string, args []byte, denied bool) {
	if g == nil {
		return
	}
	key := loopGuardInvocationKey(ctx)
	if key == "" {
		return
	}
	sig := loopGuardSignature(toolName, args)
	g.mu.Lock()
	defer g.mu.Unlock()
	e := g.entryLocked(key, time.Now())
	e.endInflight(sig)
	if toolName == toolLoadToolName {
		e.endInflightLoad(loopGuardToolLoadTarget(args))
	}
	if denied {
		if e.deniedSigs == nil {
			e.deniedSigs = map[string]int{}
		}
		e.deniedSigs[sig]++
	}
	// 本轮有工具活动但零产出（工具未执行），供 BeforeModel 空转轮结算。
	e.roundSawTool = true
}

func (g *toolLoopGuard) beforeHook() callbacks.BeforeToolHook {
	return callbacks.NewBeforeToolHook(4, func(ctx context.Context, args *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error) {
		if args == nil {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		key := loopGuardInvocationKey(ctx)
		if key == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		sig := loopGuardSignature(args.ToolName, args.Arguments)
		now := time.Now()
		g.mu.Lock()
		e := g.entryLocked(key, now)
		// Q1 wall-time 硬闸：节点存续超过硬时限 → StopError 强制终止（S02
		// 「合法失控」根修；对齐 LangGraph TimeoutPolicy 的 run 级硬超时）。
		// 无豁免工具——硬闸语义是节点生命期终结，不同于空转封锁保留投降通道。
		// 计时基准 firstSeen=条目创建（节点在本 run 内首次活动），跨重试累计：
		// 节点反复重跑仍持续烧预算正是硬闸要拦的形态。
		if hardSec := g.wallSecFor(true); wallElapsedNet(ctx, e.firstSeen, now) > time.Duration(hardSec)*time.Second {
			elapsed := wallElapsedNet(ctx, e.firstSeen, now)
			g.mu.Unlock()
			g.lg.Warn("tool loop guard wall-time hard gate tripped, stopping node",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("elapsed_sec", int(elapsed.Seconds())),
				loggateway.Int("hard_sec", hardSec))
			g.emitGateEvent(ctx, "blocked", args.ToolName, "节点运行超 wall-time 硬闸，强制终止",
				fmt.Sprintf("节点已运行 %d 秒，超过 wall-time 硬闸 %d 秒", int(elapsed.Seconds()), hardSec),
				int(elapsed.Seconds()), hardSec, "stop_node")
			return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已运行约 %d 分钟，超过系统硬时限（%d 秒），"+
				"节点被强制终止以防止预算耗尽。已取得的结论与本次终止原因将随节点结果上报；后续步骤由编排层接手。",
				loopGuardMarker, int(elapsed.Minutes()), hardSec))
		}
		if hard := g.roundProductHard(); hard > 0 {
			prod := e.modelRounds * e.lastEst
			if prod >= hard && args.ToolName != todoDeclareBlockerToolName {
				e.blockedCount++
				e.roundSawTool = true
				blocked := e.blockedCount
				rounds := e.modelRounds
				est := e.lastEst
				saturated := blocked >= loopGuardSaturatedStopThreshold
				g.mu.Unlock()
				g.lg.Warn("tool loop guard blocked call after round×est product hard gate",
					loggateway.StepID("agent.tool_loop_guard"),
					loggateway.Str("tool", args.ToolName),
					loggateway.Int("model_rounds", rounds),
					loggateway.Int("last_est", est),
					loggateway.Int("product", prod),
					loggateway.Int("hard", hard))
				g.emitGateDecision(ctx, args.ToolName, "轮数×单轮积越硬闸，工具面封锁",
					fmt.Sprintf("%d 轮 × %d tok = %d，超过硬阈 %d", rounds, est, prod, hard),
					prod, hard, "block_call")
				if saturated {
					return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已连续 %d 次触发系统拦截仍重发被拦调用，"+
						"节点被强制终止以防止调用预算耗尽。已取得的取证结论与本次终止原因将随节点结果上报。", loopGuardMarker, loopGuardSaturatedStopThreshold))
				}
				msg := fmt.Sprintf("%s：本节点已进行 %d 次模型调用，最近一轮约 %d tokens（积 %d，硬阈 %d）——本调用被系统拦截，工具未执行、也非执行失败。"+
					"继续调用只会按轮数平方放大计费。请立即基于现有信息输出最终结论；若任务无法推进，调用 todo_declare_blocker 说明阻塞原因。"+
					"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
					loopGuardMarker, rounds, est, prod, hard, blocked, loopGuardSaturatedStopThreshold)
				return nil, errors.New(msg)
			}
		}
		// A2'b 空转封锁：连续 M 轮零有效产出后，封锁一切新工具调用
		// （todo_declare_blocker 豁免，保留投降通道）。顽固重发计入
		// blockedCount，共享 B4 饱和 StopError 止损。
		if e.unprodRounds >= unproductiveRoundBlockThreshold && args.ToolName != todoDeclareBlockerToolName {
			e.blockedCount++
			e.roundSawTool = true
			blocked := e.blockedCount
			unprod := e.unprodRounds
			saturated := blocked >= loopGuardSaturatedStopThreshold
			g.mu.Unlock()
			g.lg.Warn("tool loop guard blocked call after unproductive rounds",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("unproductive_rounds", unprod),
				loggateway.Int("blocked", blocked))
			action := "block_call"
			if saturated {
				action = "stop_node"
			}
			g.emitGateDecision(ctx, args.ToolName, "连续零有效产出轮，工具面封锁",
				fmt.Sprintf("连续 %d 轮工具调用零有效产出，封锁新调用（%s）", unprod, action),
				unprod, unproductiveRoundBlockThreshold, action)
			if saturated {
				return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已连续 %d 次触发系统拦截仍重发被拦调用，"+
					"节点被强制终止以防止调用预算耗尽。已取得的取证结论与本次终止原因将随节点结果上报。", loopGuardMarker, loopGuardSaturatedStopThreshold))
			}
			msg := fmt.Sprintf("%s：本节点已连续 %d 轮工具调用零有效产出（失败/被拦/检索空结果），系统已封锁新的工具调用——本调用未执行、也非执行失败。"+
				"继续调用任何工具都不会产生新信息。请立即基于现有信息输出最终结论；若任务确实无法推进，调用 todo_declare_blocker 说明阻塞原因（该工具不受本封锁限制）。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, unproductiveRoundBlockThreshold, blocked, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		v := g.verdictBeforeLocked(e, sig, args.ToolName, args.Arguments, now)
		g.mu.Unlock()
		switch v.kind {
		case loopGuardBlockNone:
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		case loopGuardBlockParallel:
			g.lg.Warn("tool loop guard blocked parallel duplicate call",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName))
			msg := fmt.Sprintf("%s：同一轮并行发出了重复的 %s 调用，仅执行第一次。工具未执行、也非执行失败。"+
				"禁止在同一响应中重复发射相同工具与相同参数；请等待第一次结果后再决定下一步。",
				loopGuardMarker, args.ToolName)
			return nil, errors.New(msg)
		}
		if v.saturated {
			g.lg.Warn("tool loop guard saturated, stopping node",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("blocked", loopGuardSaturatedStopThreshold))
			g.emitGateDecision(ctx, args.ToolName, "拦截饱和，节点强制终止",
				fmt.Sprintf("节点内累计被拦 %d 次仍重发被拦调用，强制终止节点止损", loopGuardSaturatedStopThreshold),
				loopGuardSaturatedStopThreshold, loopGuardSaturatedStopThreshold, "stop_node")
			return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已连续 %d 次触发系统拦截仍重发被拦调用，"+
				"节点被强制终止以防止调用预算耗尽。已取得的取证结论与本次终止原因将随节点结果上报。", loopGuardMarker, loopGuardSaturatedStopThreshold))
		}
		if v.kind == loopGuardBlockLoadRepeat {
			g.lg.Warn("tool loop guard blocked repeated tool_load",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("target", v.loadTarget))
			g.emitGateDecision(ctx, args.ToolName, "重复装载已激活工具，拦截零信息调用",
				fmt.Sprintf("工具 %s 已处于激活状态，重复 tool_load 不产生新信息", v.loadTarget),
				v.loadTarget, 1, "block_call")
			msg := fmt.Sprintf("%s：工具 %s 此前已成功装载并处于激活状态——激活是幂等的，本调用被系统拦截，工具未执行、也非执行失败。"+
				"重复装载不会产生任何新信息（schema 已在历史中，且本会话无卸载机制）。请立即直接调用 %s 本身推进任务，或基于现有信息收尾。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.loadTarget, v.loadTarget, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockLoadQuota {
			g.lg.Warn("tool loop guard blocked tool_load over quota",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("target", v.loadTarget),
				loggateway.Int("load_count", v.loadCount),
				loggateway.Int("load_max", v.loadMax))
			g.emitGateDecision(ctx, args.ToolName, "工具装载配额耗尽，拦截装载面扩张",
				fmt.Sprintf("节点已成功装载 %d 个工具，达配额上限 %d，继续装载属失控模式", v.loadCount, v.loadMax),
				v.loadCount, v.loadMax, "block_call")
			msg := fmt.Sprintf("%s：本节点已成功装载 %d 个工具，达到单节点装载配额上限（%d）——本调用被系统拦截，工具未执行、也非执行失败。"+
				"持续装载新工具而不使用是典型的失控模式：请立即停止 tool_load，从已激活的工具中选择能推进任务的直接调用；若现有工具面确实无法完成，基于已有信息说明缺口后收尾。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.loadCount, v.loadMax, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockLoadRatio {
			g.lg.Warn("tool loop guard blocked bootstrap ratio",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("target", v.loadTarget))
			g.emitGateDecision(ctx, args.ToolName, "装载/搜索占比过高，强制收敛",
				fmt.Sprintf("窗口内 tool_load/tool_search 占比达到 %.0f%%，继续自举属失控模式", loopGuardBootstrapRatioDefault*100),
				loopGuardBootstrapWindow, int(loopGuardBootstrapRatioDefault*100), "block_call")
			msg := fmt.Sprintf("%s：本节点最近工具调用中 tool_load/tool_search 占比已达 50%% 以上——本调用被系统拦截，工具未执行、也非执行失败。"+
				"持续搜索/装载而不使用是 S02 类失控：请立即停止 tool_load 与 tool_search，直接调用已激活工具推进任务；若工具面不足，基于已有信息说明缺口后收尾。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockPlanDrift {
			g.lg.Warn("tool loop guard blocked plan-execute drift tool_load",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("target", v.loadTarget),
				loggateway.Int("post_plan_loads", v.loadCount))
			g.emitGateDecision(ctx, args.ToolName, "计划声明后持续本地装载，拦截漂移",
				fmt.Sprintf("plan_and_execute 已声明编排后仍装载 %d 个工具", v.loadCount),
				v.loadCount, g.planDriftAt(), "block_call")
			msg := fmt.Sprintf("%s：本节点已通过 plan_and_execute 声明编排，随后又本地装载了 %d 个工具——本调用被系统拦截，工具未执行、也非执行失败。"+
				"计划与执行漂移：编排既已声明，应等待成员结果或基于已有信息收尾，不要继续 tool_load。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.loadCount, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockLoadThenCall {
			g.lg.Warn("tool loop guard blocked same-step call after tool_load",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("target", v.loadTarget))
			g.emitGateDecision(ctx, args.ToolName, "刚激活的工具不能在同一步立刻调用",
				fmt.Sprintf("工具 %s 刚由 tool_load 激活，须等下一 model step", v.loadTarget),
				v.loadTarget, 1, "block_call")
			msg := fmt.Sprintf("%s：工具 %s 刚在本轮由 tool_load 激活（或与 tool_load 处于同一批并行调用）——本调用被系统拦截，工具未执行、也非执行失败。"+
				"请等待 tool_load 返回后，在下一轮模型请求中调用 %s，不要与 tool_load 放在同一批并行 tool call 里。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.loadTarget, v.loadTarget, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockEmpty {
			g.lg.Warn("tool loop guard blocked empty-result retry",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("empty_streak", v.emptyStreak))
			g.emitGateDecision(ctx, args.ToolName, "检索连续空结果，拦截换词重试",
				fmt.Sprintf("%s 连续 %d 次返回空结果（含换词），库中确无资料", args.ToolName, v.emptyStreak),
				v.emptyStreak, loopGuardEmptyStreakThreshold, "block_call")
			msg := fmt.Sprintf("%s：%s 已连续 %d 次（含换用不同关键词）返回空结果——知识库中确实没有相关资料。"+
				"本调用被系统拦截，工具未执行、也非执行失败；继续换词重试不会产生任何新信息，只会消耗你的调用预算。"+
				"请立即停止调用本工具。若本轮已注入 ## L2+L3 memory，必须根据其中匹配事实作答，禁止声称记忆中没有。"+
				"若记忆与知识库都没有该事实，在回答中明确说明未收录，禁止编造姓名、报告编号、品牌、电话或个人偏好。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, args.ToolName, v.emptyStreak, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockCycle {
			g.lg.Warn("tool loop guard blocked rotation cycle call",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("cycle", v.cycleDesc))
			g.emitGateDecision(ctx, args.ToolName, "固定调用循环，拦截轮询空转",
				fmt.Sprintf("检测到固定调用循环（%s），已重复满 %d 轮", v.cycleDesc, loopGuardCycleMinRepeats),
				v.cycleDesc, loopGuardCycleMinRepeats, "block_call")
			msg := fmt.Sprintf("%s：检测到固定调用循环（%s），已重复满 %d 轮——本调用被系统拦截，工具未执行、也非执行失败，"+
				"节点内轮询不会产生新信息。若在等待外部状态变化，状态复验由图谱重试机制承担，禁止继续该循环；请立即基于现有证据输出结论/裁决。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.cycleDesc, loopGuardCycleMinRepeats, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockForbidden {
			g.lg.Warn("tool loop guard blocked forbidden retry",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("forbidden_streak", v.forbiddenStreak))
			g.emitGateDecision(ctx, args.ToolName, "目标 agent 不存在，停止重试",
				fmt.Sprintf("%s 连续返回 FORBIDDEN/target not found，继续重试不会接通", args.ToolName),
				2, 2, "block_call")
			msg := fmt.Sprintf("%s：%s 已连续因目标 agent 不存在（FORBIDDEN）失败——本调用被系统拦截。"+
				"不要再猜 agent_key 或重试同一调用。请改走路由建议（部门主管会话/精灵组队）或如实向用户说明岗位未接通；禁止继续 memory_search 打转。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, args.ToolName, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		if v.kind == loopGuardBlockDenied {
			g.lg.Warn("tool loop guard blocked HITL-denied resend",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName))
			g.emitGateDecision(ctx, args.ToolName, "同参重发已被确认门禁否决的调用，拦截",
				fmt.Sprintf("%s 的同参调用刚被确认流程否决（用户拒绝或无确认通道），节点内重发不会改变裁定", args.ToolName),
				1, 1, "block_call")
			cue := planCCompensationCue(ctx, args.ToolName)
			if cue != "" {
				cue = " " + cue
			}
			msg := fmt.Sprintf("%s：本调用与方才确认流程已否决的调用（工具与参数完全一致）重合——该调用已被判定为不可执行（用户拒绝或当前环境无确认通道），本调用被系统拦截，工具未执行、也非执行失败。"+
				"同一节点内以相同参数重发不会改变裁定；若用户改变主意，会在新的对话轮次中明确提出，届时重新发起确认。请直接向用户说明该操作保持未执行状态，并询问接下来如何处理。%s"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, cue, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		threshold := v.threshold
		if threshold <= 0 {
			threshold = loopGuardBlockThreshold
		}
		g.lg.Warn("tool loop guard blocked identical repeat call",
			loggateway.StepID("agent.tool_loop_guard"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Int("consecutive", v.consecutiveCount))
		g.emitGateDecision(ctx, args.ToolName, "同工具同参数同结果连续重发，拦截无效空转",
			fmt.Sprintf("%s 已连续 %d 次以相同参数调用且结果一致（第 %d 次起拦截）", args.ToolName, v.consecutiveCount, threshold+1),
			v.consecutiveCount, threshold+1, "block_call")
		digest := v.lastDigest
		if digest == "" {
			digest = "（结果为空）"
		}
		crossName := ""
		if v.lastTool != "" && v.lastTool != args.ToolName {
			crossName = fmt.Sprintf("（%s 与 %s 是同一底层工具的不同名字，同参数调用属重复）", args.ToolName, v.lastTool)
		}
		// P2-③ 方案 C 引导：补偿对工具（如 fault_inject）存在未核销副作用时，
		// 把模型从「重发原调用」引导到「调用逆工具完成补偿」；无 pending 时为空串。
		cue := planCCompensationCue(ctx, args.ToolName)
		if cue != "" {
			cue = " " + cue
		}
		msg := fmt.Sprintf("%s：本调用被系统拦截，工具未执行、也非执行失败——%s 此前已成功返回，取证已完成。%s"+
			"禁止重发本调用，立即按任务指令推进到下一动作（发起下一步指定的工具调用；全部步骤完成则直接输出最终结论）。"+
			"重发只会反复触发本拦截并消耗你的调用预算，不会产生任何新信息。完整取证结果回放：「%s」（你已连续 %d 次以相同参数调用；本节点累计被拦 %d 次，满 %d 次将被强制终止）。%s",
			loopGuardMarker, args.ToolName, crossName, digest, v.consecutiveCount, v.blockedCount, loopGuardSaturatedStopThreshold, cue)
		return nil, errors.New(msg)
	})
}

func (g *toolLoopGuard) afterHook() callbacks.AfterToolHook {
	return callbacks.NewAfterToolHook(4, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		// 注：被拦调用以 error 形态返回、框架不执行 AfterTool，故这里只见真实执行结果。
		key := loopGuardInvocationKey(ctx)
		if key == "" {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		sig := loopGuardSignature(args.ToolName, args.Arguments)
		// Q1 plan-execute 漂移观测的锁外发射负载（driftAt>0 且未发过才发射）。
		driftAt := 0
		g.mu.Lock()
		e := g.entryLocked(key, time.Now())
		e.endInflight(sig)
		if args.ToolName == toolLoadToolName {
			e.endInflightLoad(loopGuardToolLoadTarget(args.Arguments))
		}
		// A2'b 空转轮记账：本轮发生了真实工具调用。失败/检索空结果均不计
		// 产出（roundProductive 保持 false），BeforeModel 续轮结算空转轮次。
		e.roundSawTool = true
		if args.Error != nil {
			// 失败重试归熔断器治理：不累计重复计数，也不触发拦截。
			// FORBIDDEN/target not found 除外：同类失败 ≥2 次后拦下一跳（E3）。
			if loopGuardForbiddenNotFound(args.Error) && (e.lastSig == "" || e.lastSig == sig) {
				e.forbiddenStreak++
			} else if loopGuardForbiddenNotFound(args.Error) {
				e.forbiddenStreak = 1
			} else {
				e.forbiddenStreak = 0
			}
			e.lastSig = sig
			e.lastTool = args.ToolName
			e.lastResultKey = ""
			e.sameCount = 0
			e.lastFailed = true
			e.appendCallLocked(sig, args.ToolName)
			g.mu.Unlock()
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		e.forbiddenStreak = 0
		rk := loopGuardResultKey(args.Result)
		sameResult := e.lastSig == sig && !e.lastFailed && e.lastResultKey == rk
		if e.lastSig == sig && !e.lastFailed && (sameResult || loopGuardVolatileResultTool(args.ToolName)) {
			e.sameCount++
		} else {
			e.sameCount = 1
		}
		e.lastSig = sig
		e.lastTool = args.ToolName
		e.lastResultKey = rk
		e.lastResultDigest = loopGuardResultDigest(args.Result)
		e.lastFailed = false
		// Q1 行为模式闸记账（成功路径）：
		//   - plan_and_execute 成功 → planDeclared 锚点置位；
		//   - tool_load 成功（result.success=true）→ 记 loadedTools（请求名+规范名，
		//     覆盖别名变体）；首次装载的目标才计 loadCount（配额只约束异质装载面）；
		//     锚点已置位时的成功装载累计 postPlanLoads，达阈值后 BeforeTool
		//     拦截下一次 tool_load；AfterTool 在达阈值当次写一次 tripped 记录。
		if args.ToolName == planAndExecuteToolName {
			e.planDeclared = true
		}
		if args.ToolName == toolLoadToolName {
			requested := loopGuardToolLoadTarget(args.Arguments)
			if name, ok := loopGuardToolLoadResult(args.Result, requested); ok {
				if e.loadedTools == nil {
					e.loadedTools = map[string]bool{}
				}
				if !e.loadedTools[name] {
					e.loadCount++
				}
				e.loadedTools[name] = true
				if requested != "" {
					e.loadedTools[requested] = true
				}
				e.markJustLoaded(name)
				if requested != "" {
					e.markJustLoaded(requested)
				}
				if e.planDeclared {
					e.postPlanLoads++
					if e.postPlanLoads >= loopGuardPlanDriftObserveAt && !e.planDriftRecorded {
						e.planDriftRecorded = true
						driftAt = e.postPlanLoads
					}
				}
			}
		}
		// 空结果熔断记账（无视参数差异）：检索类工具连续空则累计，一旦拿到
		// 非空结果立即清零——熔断针对的是「库中确无资料仍换词重试」的空转。
		productive := true
		if isEmpty, ok := loopGuardEmptyResultTools[args.ToolName]; ok {
			if isEmpty(args.Result) {
				productive = false
				if e.emptyStreak == nil {
					e.emptyStreak = map[string]int{}
				}
				e.emptyStreak[args.ToolName]++
			} else {
				delete(e.emptyStreak, args.ToolName)
			}
		}
		if productive {
			e.roundProductive = true
		}
		e.appendCallLocked(sig, args.ToolName)
		g.mu.Unlock()
		if driftAt > 0 {
			g.lg.Warn("plan-execute drift observed: tool_load continues after plan declared",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Int("post_plan_loads", driftAt))
			g.emitGateEvent(ctx, "tripped", toolLoadToolName, "计划声明后仍持续本地装载工具（plan-execute 漂移）",
				fmt.Sprintf("plan_and_execute 已成功声明编排，节点随后又成功装载 %d 个工具——计划与执行漂移", driftAt),
				driftAt, loopGuardPlanDriftObserveAt, "record_only")
		}
		return &trpctool.AfterToolResult{Context: ctx}, nil
	})
}

// modelHook 是空转轮次早停（A2'b）与 wall-time 软闸（Q1）的 BeforeModel 侧：
// 每次模型调用（含工具循环续轮）先结算上一 LLM 轮的工具产出——
//   - 上一轮有工具调用且任一有产出 → unprodRounds 清零；
//   - 上一轮有工具调用但零有效产出（全部失败/被拦/检索空结果）→ unprodRounds+1；
//   - 上一轮无工具调用（纯文本续轮）→ 不结算，计数保持。
//
// 结算后 unprodRounds 满 unproductiveRoundGuideThreshold 即注入降级引导 cue
// （先摘除历史同名 cue，保证只有一条最新轮次文案）；满
// unproductiveRoundBlockThreshold 的封锁由 beforeHook 执行（拦截一切新调用，
// todo_declare_blocker 豁免）。
//
// Q1 wall-time 软闸：节点存续（entry.firstSeen 起算）超软闸秒数后，每轮注入
// 收尾引导 cue（与空转 cue 并存、各自按标记去重），并在首次越线时写一条
// outcome=tripped 决策记录（once-per-entry；cue 本身按轮刷新）。硬闸的
// StopError 强终止由 beforeHook 执行——模型侧先获引导、工具侧后封锁，与
// A2'b 两段式同构（对齐 LangGraph TimeoutPolicy idle/run 双超时语义）。
func (g *toolLoopGuard) modelHook() callbacks.BeforeModelHook {
	return callbacks.NewBeforeModelHook(4, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		key := loopGuardInvocationKey(ctx)
		if key == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		now := time.Now()
		g.mu.Lock()
		e := g.entryLocked(key, now)
		e.justLoaded = nil
		if e.roundSawTool {
			if e.roundProductive {
				e.unprodRounds = 0
			} else {
				e.unprodRounds++
			}
			e.roundSawTool = false
			e.roundProductive = false
		}
		unprod := e.unprodRounds
		softSec := g.wallSecFor(false)
		elapsed := wallElapsedNet(ctx, e.firstSeen, now)
		wallSoftTripped := elapsed > time.Duration(softSec)*time.Second
		recordWallSoft := wallSoftTripped && !e.wallSoftRecorded
		if recordWallSoft {
			e.wallSoftRecorded = true
		}
		e.modelRounds++
		e.lastEst = analyzePromptRequest(args.Request.Messages).EstTokens + toolsSchemaEstTokens(args.Request)
		prod := e.modelRounds * e.lastEst
		softProd := g.roundProductSoft()
		hardProd := g.roundProductHard()
		prodSoftTripped := hardProd > 0 && prod >= softProd
		recordProdSoft := prodSoftTripped && !e.roundProdRecorded
		if recordProdSoft {
			e.roundProdRecorded = true
		}
		rounds := e.modelRounds
		est := e.lastEst
		g.mu.Unlock()
		msgs := args.Request.Messages
		if unprod >= unproductiveRoundGuideThreshold {
			msgs = stripDynamicCueByMarker(msgs, unproductiveRoundCueMarker)
			msgs = appendDynamicCue(msgs, unproductiveRoundCueMarker+buildUnproductiveRoundCue(unprod))
			g.lg.Warn("unproductive tool rounds, degradation guide injected",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Int("unproductive_rounds", unprod))
		}
		if wallSoftTripped {
			msgs = stripDynamicCueByMarker(msgs, wallTimeCueMarker)
			msgs = appendDynamicCue(msgs, wallTimeCueMarker+buildWallTimeCue(elapsed, g.wallSecFor(true)))
			g.lg.Warn("wall-time soft gate tripped, wrap-up cue injected",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Int("elapsed_sec", int(elapsed.Seconds())),
				loggateway.Int("soft_sec", softSec))
			if recordWallSoft {
				g.emitGateEvent(ctx, "tripped", "", "节点运行超 wall-time 软闸，注入收尾引导",
					fmt.Sprintf("节点已运行 %d 秒，超过 wall-time 软闸 %d 秒，注入收尾引导 cue", int(elapsed.Seconds()), softSec),
					int(elapsed.Seconds()), softSec, "inject_cue")
			}
		}
		if prodSoftTripped {
			msgs = stripDynamicCueByMarker(msgs, roundProductCueMarker)
			msgs = appendDynamicCue(msgs, roundProductCueMarker+buildRoundProductCue(rounds, est, prod, hardProd))
			g.lg.Warn("round×est product soft gate tripped, wrap-up cue injected",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Int("model_rounds", rounds),
				loggateway.Int("last_est", est),
				loggateway.Int("product", prod),
				loggateway.Int("soft", softProd),
				loggateway.Int("hard", hardProd))
			if recordProdSoft {
				g.emitGateEvent(ctx, "tripped", "", "轮数×单轮积越软闸，注入收尾引导",
					fmt.Sprintf("%d 轮 × %d tok = %d，软阈 %d / 硬阈 %d", rounds, est, prod, softProd, hardProd),
					prod, softProd, "inject_cue")
			}
		}
		args.Request.Messages = msgs
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// buildUnproductiveRoundCue 是降级引导 cue 文案（A2'b）：告知模型已连续 N 轮
// 零有效产出，引导其停止盲目重试、基于已有信息收尾；满封锁阈值时改口告知
// 工具面已封锁（与 beforeHook 拦截文案口径一致），仅剩 declare_blocker/收尾。
func buildUnproductiveRoundCue(unprod int) string {
	if unprod >= unproductiveRoundBlockThreshold {
		return fmt.Sprintf(`<tool_round_notice>
你已连续 %d 轮工具调用零有效产出（失败/被系统拦截/检索空结果）。系统现已封锁新的工具调用：除 todo_declare_blocker 外，任何工具调用都会被直接拦截。不要再尝试调用工具——立即基于现有信息输出最终结论；若任务确实无法推进，调用 todo_declare_blocker 说明阻塞原因后收尾。
</tool_round_notice>`, unprod)
	}
	return fmt.Sprintf(`<tool_round_notice>
你已连续 %d 轮工具调用零有效产出（失败/被系统拦截/检索空结果）。继续用相似方式调用工具大概率仍无收获，只会消耗调用预算。请立即调整策略：基于已拿到的信息直接作答；或换用根本不同的方法（不同工具/不同思路）。若确认任务无法推进，调用 todo_declare_blocker 说明阻塞原因。再连续零产出 %d 轮，系统将封锁一切新的工具调用。
</tool_round_notice>`, unprod, unproductiveRoundBlockThreshold-unprod)
}

// buildWallTimeCue 是 wall-time 软闸的收尾引导 cue 文案（Q1）：告知模型节点
// 已运行时长与硬时限，引导其停止扩张性动作（继续装载/广泛取证）、基于已
// 有信息收尾。硬闸强终止由 beforeHook 执行，文案口径与 StopError 消息一致。
func buildWallTimeCue(elapsed time.Duration, hardSec int) string {
	return fmt.Sprintf(`<runtime_notice>
本节点已运行约 %d 分钟，超过系统软时限；运行满 %d 秒将被强制终止（不可豁免）。请立即停止扩张性动作（装载新工具/广泛取证/轮询等待），基于已取得的信息输出当前最优结论收尾；若任务确实无法完成，说明缺口后收尾，不要继续消耗预算。
</runtime_notice>`, int(elapsed.Minutes()), hardSec)
}

func buildRoundProductCue(rounds, est, prod, hard int) string {
	return fmt.Sprintf(`<runtime_notice>
本节点已进行 %d 次模型调用，最近一轮约 %d tokens（轮数×单轮积 %d）。继续调用工具会按轮数平方放大计费；积达到 %d 后系统将封锁新的工具调用。请立即停止扩张性装载/检索，基于已有信息输出结论；若任务无法完成，调用 todo_declare_blocker 说明缺口后收尾。
</runtime_notice>`, rounds, est, prod, hard)
}

// loopGuardVolatileResultTool reports tools whose success payload always
// changes (clock/time) so same-param repeats would never trip the
// result-hash guard. Same signature still counts toward the 3rd-call block.
func loopGuardVolatileResultTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "datetime", "get_datetime", "current_time", "get_current_time", "now", "clock":
		return true
	default:
		return false
	}
}

func loopGuardForbiddenNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "forbidden") && strings.Contains(msg, "target agent not found")
}
