// Package eval 是压缩金闸门（A8 / P0-3 设计 §4.7）：对冻结会话导出跑与线上
// 剪枝 hook 完全同源的确定性剪枝（agent.RunToolResultPruneForEval），断言
// 硬指标——
//
//   - 保护项零丢失：protected_indices 指向的消息字节级不变（检索豁免默认
//     ON 的验收载体：检索族 K 轮外超尺寸仍不剪，§4.4③）；
//   - 回读 100%：每条折叠指针携带可取回定位符，readback(locator) 逐字节
//     等于原文（P0-2 契约——toolspill 存在于工作区，折叠必须保留定位符，
//     指针 blob= 段即定位符）；
//   - 长度守恒：折叠不增删消息（append-only 语义，同 hook 契约相容性条款）；
//   - 非保护未折叠项零改动。
//
// 纯函数库，门禁判定（CheckCompressionGoldGate 返回 []Violation）与测试驱动
// 分离，仿 internal/knowledge/eval 的 CheckGoldGate 先例。fixture 三枚
// （retrieval-heavy / tool-loop-heavy / chit-chat）冻结在 testdata/，来源与
// 构造口径见各文件的 provenance 字段。AS-EVT-01：零新增事件（离线评测通路，
// 无日志无事件，违规经返回值上浮由测试断言）。
package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/agent"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ---------------------------------------------------------------------------
// fixture 装载
// ---------------------------------------------------------------------------

// TranscriptMessage 是冻结会话导出里的一条消息（trpcmodel.Message 的 JSON
// 投影，字段裁剪到剪枝语义所需子集）。
type TranscriptMessage struct {
	Role      string           `json:"role"` // system / user / assistant / tool
	Content   string           `json:"content"`
	ToolName  string           `json:"tool_name,omitempty"`
	ToolID    string           `json:"tool_id,omitempty"`
	ToolCalls []TranscriptCall `json:"tool_calls,omitempty"`
}

// TranscriptCall 记录一条 tool_call 的来源信息（剪枝只吃 ID，Name/Args 为
// fixture 可读性/取证保留，转换时不注入——与 hook 测试 fixture 同规则）。
type TranscriptCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"args,omitempty"`
}

// PruneFixtureConfig 对应 agent.ToolResultPruneConfig 的评测子集；豁免表
// 解析（含检索豁免默认 ON）由 RunToolResultPruneForEval 走 hook 同路径。
type PruneFixtureConfig struct {
	Enabled         bool  `json:"enabled"`
	AfterTurns      int   `json:"after_turns"`
	SizeBytes       int64 `json:"size_bytes"`
	RetrievalExempt *bool `json:"retrieval_exempt,omitempty"`
}

// Transcript 是一份冻结会话导出（金闸门评测用例）。
type Transcript struct {
	// Name 是用例名（文件名去扩展名）。
	Name string `json:"name"`
	// Provenance 记录冻结来源与构造口径（变更必须同步，防 fixture 漂移无据可查）。
	Provenance string `json:"provenance"`
	// Config 是本次折叠的剪枝配置。
	Config PruneFixtureConfig `json:"config"`
	// ProtectedIndices 是保护项下标（期望字节级不变的 tool 结果：检索豁免 /
	// 错误结果 / K 轮内）。
	ProtectedIndices []int `json:"protected_indices"`
	// Locators 是期望被折叠项的定位符表（spill:// 定位符 → 原文）：折叠后
	// 每条定位符必须出现在对应指针的 blob= 段且可回读到声明原文。
	Locators map[string]string   `json:"locators"`
	Messages []TranscriptMessage `json:"messages"`
}

// LoadTranscript 从 JSON 文件装载冻结 fixture。
func LoadTranscript(path string) (Transcript, error) {
	var tc Transcript
	raw, err := os.ReadFile(path)
	if err != nil {
		return tc, fmt.Errorf("eval: read transcript %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return tc, fmt.Errorf("eval: parse transcript %s: %w", path, err)
	}
	if len(tc.Messages) == 0 {
		return tc, fmt.Errorf("eval: transcript %s has no messages", path)
	}
	return tc, nil
}

// ---------------------------------------------------------------------------
// 折叠执行（driver 调 agent 同源入口）
// ---------------------------------------------------------------------------

// FoldedItem 记录一条被折叠的 tool 结果。
type FoldedItem struct {
	Index    int // 消息下标（长度守恒下与 fixture 对齐）
	ToolName string
	ToolID   string
	Locator  string // archive 返回的定位符（指针 blob= 段）
	Pointer  string // 折叠后的指针文本
	Original string // 折叠前原文
}

// CollapseOutcome 是一次金闸门折叠的结果。
type CollapseOutcome struct {
	Messages []trpcmodel.Message
	Folded   []FoldedItem
	Meta     agent.PruneMeta
}

// toModelMessages 把 fixture 消息投影为 trpcmodel（剪枝只吃 Role/Content/
// ToolName/ToolID；ToolCall 只带 ID，与 hook 测试 fixture 同规则）。
func toModelMessages(tc Transcript) []trpcmodel.Message {
	msgs := make([]trpcmodel.Message, len(tc.Messages))
	for i, m := range tc.Messages {
		msg := trpcmodel.Message{
			Role:     trpcmodel.Role(m.Role),
			Content:  m.Content,
			ToolName: m.ToolName,
			ToolID:   m.ToolID,
		}
		for _, c := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, trpcmodel.ToolCall{Type: "function", ID: c.ID})
		}
		msgs[i] = msg
	}
	return msgs
}

// RunPruneCollapse 用 agent.RunToolResultPruneForEval（与线上 hook 同源的
// 剪枝体）折叠一份 fixture 副本，返回结果与内存归档（locator → 原文，模拟
// toolspill 取回设施；fold 期望集由 fixture Locators 字段声明比对）。
// 定位符形态：spill://fs/eval-<toolID>，覆盖 P0-2 契约的 spill:// 条目。
func RunPruneCollapse(tc Transcript) (CollapseOutcome, map[string]string) {
	msgs := toModelMessages(tc)
	archive := map[string]string{}
	var folded []FoldedItem
	meta := agent.RunToolResultPruneForEval(msgs, agent.ToolResultPruneConfig{
		Enabled:         tc.Config.Enabled,
		AfterTurns:      tc.Config.AfterTurns,
		SizeBytes:       tc.Config.SizeBytes,
		RetrievalExempt: tc.Config.RetrievalExempt,
	}, func(toolID, toolName, content string) (string, bool) {
		locator := "spill://fs/eval-" + toolID
		archive[locator] = content
		return locator, true
	})
	for i := range msgs {
		if msgs[i].Role == trpcmodel.RoleTool && msgs[i].Content != tc.Messages[i].Content {
			// 指针 blob= 段即定位符（biz.ToolResultPrunePointer 契约）。
			locator := ""
			for loc := range archive {
				if strings.Contains(msgs[i].Content, "blob="+loc) {
					locator = loc
					break
				}
			}
			folded = append(folded, FoldedItem{
				Index:    i,
				ToolName: msgs[i].ToolName,
				ToolID:   msgs[i].ToolID,
				Locator:  locator,
				Pointer:  msgs[i].Content,
				Original: tc.Messages[i].Content,
			})
		}
	}
	return CollapseOutcome{Messages: msgs, Folded: folded, Meta: meta}, archive
}

// ---------------------------------------------------------------------------
// 金闸门（纯函数判定）
// ---------------------------------------------------------------------------

// ViolationKind 标识一类金闸门违规。
type ViolationKind string

const (
	// ViolationLengthMismatch：折叠后消息条数变化（append-only 语义被破坏），
	// 后续按下标的比对全部失效，单独短路返回。
	ViolationLengthMismatch ViolationKind = "length-mismatch"
	// ViolationProtectedMutated：保护项（检索豁免 / 错误 / K 轮内）字节变化——
	// 硬指标「保护项零丢失」的直接违例。
	ViolationProtectedMutated ViolationKind = "protected-mutated"
	// ViolationUnexpectedFold：被折叠项未在 fixture Locators 表中声明
	// （fixture 与行为漂移，或豁免表被意外收窄）。
	ViolationUnexpectedFold ViolationKind = "unexpected-fold"
	// ViolationUnprotectedMutated：既非保护项也未折叠的消息内容变化。
	ViolationUnprotectedMutated ViolationKind = "unprotected-mutated"
	// ViolationLocatorMissing：fixture 声明的定位符未在任何指针 blob= 段出现
	// （期望的折叠没发生，或定位符丢失——P0-2 契约违例）。
	ViolationLocatorMissing ViolationKind = "locator-missing"
	// ViolationPointerNoLocator：折叠指针未携带定位符（无法回读）。
	ViolationPointerNoLocator ViolationKind = "pointer-no-locator"
	// ViolationReadbackMismatch：readback(locator) 缺失或不等于原文——
	// 硬指标「回读 100%」的直接违例。
	ViolationReadbackMismatch ViolationKind = "readback-mismatch"
)

// Violation 是一条金闸门违规；Detail 面向人读（测试失败输出）。
type Violation struct {
	Kind   ViolationKind
	Index  int // -1 表示与具体下标无关
	Detail string
}

func (v Violation) String() string {
	if v.Index >= 0 {
		return fmt.Sprintf("[%s] msg %d: %s", v.Kind, v.Index, v.Detail)
	}
	return fmt.Sprintf("[%s] %s", v.Kind, v.Detail)
}

// CheckCompressionGoldGate 是压缩金闸门的单一判定源。返回空 = 门禁通过；
// 违规按下标确定性顺序枚举。readback 由调用方提供（通常直接查
// RunPruneCollapse 返回的归档 map），模拟 read_tool_result 经 toolspill
// 取回。
func CheckCompressionGoldGate(tc Transcript, out CollapseOutcome, readback func(locator string) (string, bool)) []Violation {
	if len(out.Messages) != len(tc.Messages) {
		return []Violation{{
			Kind:  ViolationLengthMismatch,
			Index: -1,
			Detail: fmt.Sprintf("message count changed: %d -> %d (append-only contract broken)",
				len(tc.Messages), len(out.Messages)),
		}}
	}

	protected := map[int]bool{}
	for _, idx := range tc.ProtectedIndices {
		protected[idx] = true
	}
	foldedByIndex := map[int]FoldedItem{}
	for _, f := range out.Folded {
		foldedByIndex[f.Index] = f
	}

	var out2 []Violation
	// 硬指标 1：保护项零丢失。
	for _, idx := range tc.ProtectedIndices {
		if idx < 0 || idx >= len(out.Messages) {
			out2 = append(out2, Violation{Kind: ViolationProtectedMutated, Index: idx,
				Detail: "protected index out of range"})
			continue
		}
		got := out.Messages[idx]
		want := tc.Messages[idx]
		if got.Content != want.Content || got.ToolID != want.ToolID || got.ToolName != want.ToolName {
			out2 = append(out2, Violation{Kind: ViolationProtectedMutated, Index: idx,
				Detail: fmt.Sprintf("protected tool result mutated (tool=%s, %d bytes)", want.ToolName, len(want.Content))})
		}
	}
	// 每条折叠：指针携带定位符 + 回读 100% 等于原文（硬指标 2）。
	for _, f := range out.Folded {
		if f.Locator == "" {
			out2 = append(out2, Violation{Kind: ViolationPointerNoLocator, Index: f.Index,
				Detail: fmt.Sprintf("folded pointer carries no locator: %q", f.Pointer)})
			continue
		}
		if _, declared := tc.Locators[f.Locator]; !declared {
			out2 = append(out2, Violation{Kind: ViolationUnexpectedFold, Index: f.Index,
				Detail: fmt.Sprintf("folded locator %q not declared in fixture locators map", f.Locator)})
		}
		back, ok := readback(f.Locator)
		if !ok {
			out2 = append(out2, Violation{Kind: ViolationReadbackMismatch, Index: f.Index,
				Detail: fmt.Sprintf("readback(%q) missing", f.Locator)})
		} else if back != f.Original {
			out2 = append(out2, Violation{Kind: ViolationReadbackMismatch, Index: f.Index,
				Detail: fmt.Sprintf("readback(%q) = %d bytes, want original %d bytes", f.Locator, len(back), len(f.Original))})
		}
	}
	// fixture 声明的每条定位符都必须落到一条折叠（期望折叠发生且定位符保留）。
	for loc := range tc.Locators {
		found := false
		for _, f := range out.Folded {
			if f.Locator == loc {
				found = true
				break
			}
		}
		if !found {
			out2 = append(out2, Violation{Kind: ViolationLocatorMissing, Index: -1,
				Detail: fmt.Sprintf("declared locator %q absent from folded pointers", loc)})
		}
	}
	// 非保护未折叠项零改动。
	for i := range out.Messages {
		if protected[i] {
			continue
		}
		if _, folded := foldedByIndex[i]; folded {
			continue
		}
		if out.Messages[i].Content != tc.Messages[i].Content {
			out2 = append(out2, Violation{Kind: ViolationUnprotectedMutated, Index: i,
				Detail: fmt.Sprintf("role=%s content changed without fold", tc.Messages[i].Role)})
		}
	}
	return out2
}
