package agent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/tools/alias"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

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

func (e *loopGuardEntry) noteErrorClass(tool, class string) {
	if e == nil || class == "" || tool == "" {
		return
	}
	if e.errorClassStreak == nil {
		e.errorClassStreak = map[string]int{}
	}
	if e.lastErrorClass == nil {
		e.lastErrorClass = map[string]string{}
	}
	if e.lastErrorClass[tool] == class {
		e.errorClassStreak[tool]++
		return
	}
	e.lastErrorClass[tool] = class
	e.errorClassStreak[tool] = 1
}

func (e *loopGuardEntry) clearErrorClass(tool string) {
	if e == nil {
		return
	}
	delete(e.errorClassStreak, tool)
	delete(e.lastErrorClass, tool)
}

func loopGuardAsObject(result any) map[string]any {
	if result == nil {
		return nil
	}
	if m, ok := result.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

func loopGuardBusinessPayload(result any) map[string]any {
	m := loopGuardAsObject(result)
	if m == nil {
		return nil
	}
	if inner, ok := m["result"]; ok {
		if im := loopGuardAsObject(inner); im != nil {
			if _, hasOK := im["ok"]; hasOK {
				return im
			}
			if _, hasErr := im["error"]; hasErr {
				return im
			}
		}
	}
	return m
}

func loopGuardErrorClass(toolName string, result any, err error) string {
	if err != nil {
		// Generic Go errors (timeout / "tool execution failed") stay on the
		// circuit-breaker retry path. Only known business classes fuse here.
		return loopGuardKnownErrorClass(err.Error())
	}
	m := loopGuardBusinessPayload(result)
	if m == nil {
		return ""
	}
	okVal, hasOK := m["ok"]
	okTrue := false
	if b, ok := okVal.(bool); ok {
		okTrue = b
	}
	if hasOK && !okTrue {
		if es, _ := m["error"].(string); es != "" {
			if c := loopGuardKnownErrorClass(es); c != "" {
				return c
			}
		}
		if st := loopGuardHTTPStatus(m["httpStatus"]); st >= 400 {
			return fmt.Sprintf("http_%d", st)
		}
		return "ok_false"
	}
	return ""
}

func loopGuardHTTPStatus(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	default:
		return 0
	}
}

func loopGuardKnownErrorClass(s string) string {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	if strings.Contains(low, "empty command") || strings.Contains(s, "cmd 不能为空") {
		return "empty_command"
	}
	if strings.Contains(low, "not implemented") {
		return "http_501"
	}
	return ""
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
