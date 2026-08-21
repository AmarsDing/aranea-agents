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
	"aranea-agents/internal/tools/alias"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
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
	// If AfterTool never runs (panic / skipped callback), inflight would
	// otherwise block the same signature for the rest of the invocation.
	// Must exceed the default tool execution timeout (10 min) so a still-
	// running call is not treated as leaked.
	loopGuardInflightStale = 15 * time.Minute
)

// loopGuardMarker 是拦截消息的可识别前缀；AfterTool 见到携带该标记的结果
// 时跳过状态更新（被拦调用的"结果"是纠偏文本，不代表真实工具输出）。
const loopGuardMarker = "⚠ 系统拦截（无效重复调用）"

type loopGuardEntry struct {
	lastSig          string // 上一次调用的签名（tool + 规范化 args 哈希）
	lastTool         string // 上一次调用的原始工具名（跨名重复时供拦截消息解释）
	lastResultKey    string // 上一次成功调用的结果哈希
	lastResultDigest string // 上一次成功调用的结果摘要（供拦截消息回放，防模型「证据丢失感」）
	sameCount        int    // 连续「签名+结果均相同」的成功调用次数
	lastFailed       bool   // 上一次调用是否失败（失败重试不累计、不拦截）
	recentSigs       []string
	recentTools      []string                // 与 recentSigs 对齐的工具名，仅供轮换拦截消息可读性
	emptyStreak      map[string]int          // 检索类工具名 → 连续空结果次数（无视参数差异；非空即清零）
	blockedCount     int                     // 本节点隔离键下累计被拦截次数（B4：满阈值升级 StopError）
	inflight         map[string]inflightSlot // 签名 → 当前正在执行（已放行、尚未 AfterTool）
	lastTouch        time.Time
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
)

// loopGuardEmptyResultTools 登记纳入「空结果熔断」的检索类工具及其空判定谓词。
// 空结果是这些工具的合法终态（库中确无资料），模型换词重试不会产生新信息；
// 判定谓词按各工具的结果结构识别「空」，非空结果即清零该工具的连续空计数。
var loopGuardEmptyResultTools = map[string]func(result any) bool{
	"knowledge_search": loopGuardIsEmptySearchResult,
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
	lastDigest       string
	lastTool         string // 上一次调用的原始工具名（跨名重复拦截时解释用）
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
}

func newToolLoopGuard(lg loggateway.Logger) *toolLoopGuard {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &toolLoopGuard{entries: map[string]*loopGuardEntry{}, lg: lg}
}

// loopGuardInvocationKey 取守卫条目的隔离键：invocation + agent（=图谱节点身份）。
// 取不到时返回空串（守卫 fail-open，不影响工具执行）。
//
// 为什么带 AgentName（2026-08-16 复验实证「连坐」）：图谱执行全链路共享同一
// InvocationID，diagnose 第3步的 gns3_exec 取证与 remediate 调用1 同参同结果，
// 计数跨节点累计 → remediate 首次调用即撞阈值被拦，真实取证额度被压缩，
// 模型「确认」反射未满足陷入重发死循环。按 AgentName 隔离后，各节点独立享有
// 完整计数额度；同节点重试轮次间仍共享（InvocationID 不变），跨轮循环仍受控。
func loopGuardInvocationKey(ctx context.Context) string {
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
		e = &loopGuardEntry{}
		g.entries[key] = e
	}
	e.lastTouch = now
	return e
}

func (g *toolLoopGuard) verdictBeforeLocked(e *loopGuardEntry, sig, toolName string, now time.Time) loopGuardVerdict {
	if e.inflightCount(sig, now) > 0 {
		return loopGuardVerdict{kind: loopGuardBlockParallel}
	}
	// 空结果熔断优先于签名/轮换判定：库中确无资料时，任何参数的再调用都无意义，
	// 拦截消息也比通用重复文案更具行动指引（直接作答/声明未收录）。
	emptyBlocked := e.emptyStreak[toolName] >= loopGuardEmptyStreakThreshold
	loop := e.lastSig == sig && !e.lastFailed && e.sameCount >= loopGuardBlockThreshold
	var cycleDesc string
	if !loop && len(e.recentSigs)+1 >= 2*loopGuardCycleMinRepeats {
		// 试追加当前签名，检测末尾是否构成固定轮换循环（如 A→B→C 满 3 轮）。
		trialSigs := append(append([]string(nil), e.recentSigs...), sig)
		if p := loopGuardCyclePeriod(trialSigs, loopGuardCycleMinRepeats); p >= 2 {
			trialTools := append(append([]string(nil), e.recentTools...), toolName)
			cycleDesc = strings.Join(trialTools[len(trialTools)-p:], " → ")
		}
	}
	if !emptyBlocked && !loop && cycleDesc == "" {
		e.beginInflight(sig, now)
		return loopGuardVerdict{kind: loopGuardBlockNone}
	}
	// B4 饱和止损：被拦调用不进 AfterTool，计数只能在此累计。
	e.blockedCount++
	v := loopGuardVerdict{
		blockedCount:     e.blockedCount,
		consecutiveCount: e.sameCount,
		emptyStreak:      e.emptyStreak[toolName],
		lastDigest:       e.lastResultDigest,
		lastTool:         e.lastTool,
		saturated:        e.blockedCount >= loopGuardSaturatedStopThreshold,
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
	v.kind = loopGuardBlockLoop
	return v
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
		v := g.verdictBeforeLocked(e, sig, args.ToolName, now)
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
			return nil, trpcagent.NewStopError(fmt.Sprintf("%s：本节点已连续 %d 次触发系统拦截仍重发被拦调用，"+
				"节点被强制终止以防止调用预算耗尽。已取得的取证结论与本次终止原因将随节点结果上报。", loopGuardMarker, loopGuardSaturatedStopThreshold))
		}
		if v.kind == loopGuardBlockEmpty {
			g.lg.Warn("tool loop guard blocked empty-result retry",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Int("empty_streak", v.emptyStreak))
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
			msg := fmt.Sprintf("%s：检测到固定调用循环（%s），已重复满 %d 轮——本调用被系统拦截，工具未执行、也非执行失败，"+
				"节点内轮询不会产生新信息。若在等待外部状态变化，状态复验由图谱重试机制承担，禁止继续该循环；请立即基于现有证据输出结论/裁决。"+
				"（本节点累计被拦 %d 次，满 %d 次将被强制终止）",
				loopGuardMarker, v.cycleDesc, loopGuardCycleMinRepeats, v.blockedCount, loopGuardSaturatedStopThreshold)
			return nil, errors.New(msg)
		}
		g.lg.Warn("tool loop guard blocked identical repeat call",
			loggateway.StepID("agent.tool_loop_guard"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Int("consecutive", v.consecutiveCount))
		digest := v.lastDigest
		if digest == "" {
			digest = "（结果为空）"
		}
		crossName := ""
		if v.lastTool != "" && v.lastTool != args.ToolName {
			crossName = fmt.Sprintf("（%s 与 %s 是同一底层工具的不同名字，同参数调用属重复）", args.ToolName, v.lastTool)
		}
		msg := fmt.Sprintf("%s：本调用被系统拦截，工具未执行、也非执行失败——%s 此前已成功返回，取证已完成。%s"+
			"禁止重发本调用，立即按任务指令推进到下一动作（发起下一步指定的工具调用；全部步骤完成则直接输出最终结论）。"+
			"重发只会反复触发本拦截并消耗你的调用预算，不会产生任何新信息。完整取证结果回放：「%s」（你已连续 %d 次以相同参数调用；本节点累计被拦 %d 次，满 %d 次将被强制终止）。",
			loopGuardMarker, args.ToolName, crossName, digest, v.consecutiveCount, v.blockedCount, loopGuardSaturatedStopThreshold)
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
		g.mu.Lock()
		defer g.mu.Unlock()
		e := g.entryLocked(key, time.Now())
		e.endInflight(sig)
		if args.Error != nil {
			// 失败重试归熔断器治理：不累计重复计数，也不触发拦截。
			// 但调用本身仍进签名窗口——失败不打破轮换循环的模式判定。
			e.lastSig = sig
			e.lastTool = args.ToolName
			e.lastResultKey = ""
			e.sameCount = 0
			e.lastFailed = true
			e.appendCallLocked(sig, args.ToolName)
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		rk := loopGuardResultKey(args.Result)
		if e.lastSig == sig && !e.lastFailed && e.lastResultKey == rk {
			e.sameCount++
		} else {
			e.sameCount = 1
		}
		e.lastSig = sig
		e.lastTool = args.ToolName
		e.lastResultKey = rk
		e.lastResultDigest = loopGuardResultDigest(args.Result)
		e.lastFailed = false
		// 空结果熔断记账（无视参数差异）：检索类工具连续空则累计，一旦拿到
		// 非空结果立即清零——熔断针对的是「库中确无资料仍换词重试」的空转。
		if isEmpty, ok := loopGuardEmptyResultTools[args.ToolName]; ok {
			if isEmpty(args.Result) {
				if e.emptyStreak == nil {
					e.emptyStreak = map[string]int{}
				}
				e.emptyStreak[args.ToolName]++
			} else {
				delete(e.emptyStreak, args.ToolName)
			}
		}
		e.appendCallLocked(sig, args.ToolName)
		return &trpctool.AfterToolResult{Context: ctx}, nil
	})
}
