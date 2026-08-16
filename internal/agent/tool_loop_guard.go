package agent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/agent/callbacks"
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
//   - 同工具不同参数（递进式调研/干活）→ 放行；
//   - 同参数但上次失败（重试）→ 放行（失败治理归熔断器）；
//   - 同参数但结果有变化（轮询拿到新数据）→ 放行并重置计数。
//
// 轮换循环的实证（2026-08-16 复验）：verify 节点以 health_check→alarm_get→exec
// 三工具轮换约 10 轮 ≈30 次调用——交替调用使 lastSig 比较恒失效、结果内嵌时间戳
// 使结果哈希恒变，p=1 判定无从触发；且节点指令明文「禁止第 2 轮取证」被无视，
// 最终靠迭代上限强制收敛。周期检测作为 p=1 判定的补充网：只命中含 ≥2 种签名的
// 异质循环，同质连调（p=1）仍归结果感知判定，不误伤「结果在变化」的合法轮询。
//
// 拦截不是静默拒绝：BeforeTool 返回 CustomResult 纠偏消息，模型可见、可转向。
// 被拦调用不占工具真实执行，且后续相同调用会持续被拦（计数不清零），模型只能
// 通过换参数/换工具/收尾输出推进——这正是期望行为。
//
// 已知边界：同一轮响应内并行发射的多个相同调用（parallel tool calls）在
// BeforeTool 阶段计数尚未累积，可能同时放行；串行循环（观测到的唯一形态）
// 不受影响。
const (
	loopGuardBlockThreshold  = 2 // 连续相同（签名+结果）成功调用达到此次数后，下一次起拦截
	loopGuardMaxEntries      = 512
	loopGuardEntryTTL        = 30 * time.Minute
	loopGuardResultCap       = 4096 // 结果哈希只取前 N 字节序列化文本，防超大结果拖慢
	loopGuardDigestCap       = 1500 // 拦截消息回放的上次真实结果摘要上限：必须覆盖取证关键证据
	// （2026-08-16 复验实证：200 截断使 ip link show 的 eth1 state DOWN 落在摘要之外，
	// 模型看不到证据判定「取证未完成」而顽固重发同参调用直至烧光预算）。
	loopGuardWindowCap       = 12   // 签名序列窗口长度上限（= 最大周期 4 × 最少重复 3）
	loopGuardCycleMaxPeriod  = 4    // 轮换循环检测的最大周期（每次轮换的工具数）
	loopGuardCycleMinRepeats = 3    // 轮换满 N 个完整周期才判定为循环
)

// loopGuardMarker 是拦截消息的可识别前缀；AfterTool 见到携带该标记的结果
// 时跳过状态更新（被拦调用的"结果"是纠偏文本，不代表真实工具输出）。
const loopGuardMarker = "⚠ 系统拦截（无效重复调用）"

type loopGuardEntry struct {
	lastSig          string // 上一次调用的签名（tool + 规范化 args 哈希）
	lastResultKey    string // 上一次成功调用的结果哈希
	lastResultDigest string // 上一次成功调用的结果摘要（供拦截消息回放，防模型「证据丢失感」）
	sameCount        int    // 连续「签名+结果均相同」的成功调用次数
	lastFailed       bool   // 上一次调用是否失败（失败重试不累计、不拦截）
	recentSigs       []string
	recentTools      []string // 与 recentSigs 对齐的工具名，仅供轮换拦截消息可读性
	lastTouch        time.Time
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
func loopGuardSignature(toolName string, args []byte) string {
	canonical := strings.TrimSpace(string(args))
	var payload any
	if err := json.Unmarshal(args, &payload); err == nil {
		if b, mErr := json.Marshal(payload); mErr == nil {
			canonical = string(b)
		}
	}
	sum := sha1.Sum([]byte(toolName + "\x00" + canonical))
	return hex.EncodeToString(sum[:])
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
		g.mu.Lock()
		e := g.entryLocked(key, time.Now())
		loop := e.lastSig == sig && !e.lastFailed && e.sameCount >= loopGuardBlockThreshold
		var cycleDesc string
		if !loop && len(e.recentSigs)+1 >= 2*loopGuardCycleMinRepeats {
			// 试追加当前签名，检测末尾是否构成固定轮换循环（如 A→B→C 满 3 轮）。
			trialSigs := append(append([]string(nil), e.recentSigs...), sig)
			if p := loopGuardCyclePeriod(trialSigs, loopGuardCycleMinRepeats); p >= 2 {
				trialTools := append(append([]string(nil), e.recentTools...), args.ToolName)
				cycleDesc = strings.Join(trialTools[len(trialTools)-p:], " → ")
			}
		}
		g.mu.Unlock()
		if !loop && cycleDesc == "" {
			return &trpctool.BeforeToolResult{Context: ctx}, nil
		}
		if cycleDesc != "" {
			g.lg.Warn("tool loop guard blocked rotation cycle call",
				loggateway.StepID("agent.tool_loop_guard"),
				loggateway.Str("tool", args.ToolName),
				loggateway.Str("cycle", cycleDesc))
			msg := fmt.Sprintf("%s：检测到固定调用循环（%s），已重复满 %d 轮——节点内轮询不会产生新信息。"+
				"若在等待外部状态变化，状态复验由图谱重试机制承担，禁止继续该循环；请立即基于现有证据输出结论/裁决。",
				loopGuardMarker, cycleDesc, loopGuardCycleMinRepeats)
			return &trpctool.BeforeToolResult{Context: ctx, CustomResult: msg}, nil
		}
		g.lg.Warn("tool loop guard blocked identical repeat call",
			loggateway.StepID("agent.tool_loop_guard"),
			loggateway.Str("tool", args.ToolName),
			loggateway.Int("consecutive", e.sameCount))
		digest := e.lastResultDigest
		if digest == "" {
			digest = "（结果为空）"
		}
		msg := fmt.Sprintf("%s：禁止重发本调用——%s 已成功返回过，取证已完成。"+
			"立即按任务指令推进到下一动作（发起下一步指定的工具调用；全部步骤完成则直接输出最终结论）。"+
			"重发只消耗你的调用预算，不会产生任何新信息。完整取证结果回放：「%s」（你已连续 %d 次以相同参数调用）。",
			loopGuardMarker, args.ToolName, digest, e.sameCount)
		return &trpctool.BeforeToolResult{Context: ctx, CustomResult: msg}, nil
	})
}

func (g *toolLoopGuard) afterHook() callbacks.AfterToolHook {
	return callbacks.NewAfterToolHook(4, func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
		if args == nil {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		// 被拦调用的结果是纠偏文本，不是真实工具输出，跳过状态更新——
		// 计数不清零，后续相同调用继续被拦。
		if s, ok := args.Result.(string); ok && strings.HasPrefix(s, loopGuardMarker) {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		key := loopGuardInvocationKey(ctx)
		if key == "" {
			return &trpctool.AfterToolResult{Context: ctx}, nil
		}
		sig := loopGuardSignature(args.ToolName, args.Arguments)
		g.mu.Lock()
		defer g.mu.Unlock()
		e := g.entryLocked(key, time.Now())
		if args.Error != nil {
			// 失败重试归熔断器治理：不累计重复计数，也不触发拦截。
			// 但调用本身仍进签名窗口——失败不打破轮换循环的模式判定。
			e.lastSig = sig
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
		e.lastResultKey = rk
		e.lastResultDigest = loopGuardResultDigest(args.Result)
		e.lastFailed = false
		e.appendCallLocked(sig, args.ToolName)
		return &trpctool.AfterToolResult{Context: ctx}, nil
	})
}
