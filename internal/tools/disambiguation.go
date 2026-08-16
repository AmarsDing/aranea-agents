package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/tools/deferred"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func buildToolGroups() map[string][]string {
	groups := make(map[string][]string)
	for _, reg := range Registry() {
		if reg.Group != "" {
			groups[reg.Group] = append(groups[reg.Group], reg.Name)
		}
	}
	return groups
}

var toolGroupsCache map[string][]string
var toolGroupsOnce sync.Once

func getToolGroups() map[string][]string {
	toolGroupsOnce.Do(func() {
		toolGroupsCache = buildToolGroups()
	})
	return toolGroupsCache
}

func ApplyDisambiguationHints(tools []trpctool.Tool) {
	for i, t := range tools {
		decl := t.Declaration()
		if decl == nil {
			continue
		}
		for _, reg := range Registry() {
			if reg.Name != decl.Name {
				continue
			}
			// Clone Declaration to avoid mutating shared objects.
			// TECH-DEBT: If Declaration gains additional reference-type fields
			// beyond InputSchema, update this deep-copy logic accordingly.
			clone := *decl
			if decl.InputSchema != nil {
				schemaCopy := *decl.InputSchema
				clone.InputSchema = &schemaCopy
			}
			if len(reg.Examples) > 0 {
				var sb strings.Builder
				sb.WriteString(clone.Description)
				sb.WriteString("\n\nExamples of when to use this tool:")
				for i, ex := range reg.Examples {
					if i >= 3 {
						break
					}
					sb.WriteString("\n- When user asks: \"")
					sb.WriteString(ex.UserQuery)
					sb.WriteString("\"")
					if ex.Explanation != "" {
						sb.WriteString(" (")
						sb.WriteString(ex.Explanation)
						sb.WriteString(")")
					}
				}
				clone.Description = sb.String()
			}
			if reg.Group != "" {
				if peers, ok := getToolGroups()[reg.Group]; ok && len(peers) > 1 {
					clone.Description += "\n\nNote: This tool is in the \"" + reg.Group + "\" group. Alternatives: " + strings.Join(filterNames(peers, clone.Name), ", ") + "."
				}
			}
			// Wrap the tool with a decorator that returns the cloned Declaration
			// instead of mutating the shared Declaration object.
			base := &disambiguatedTool{inner: t, decl: &clone}
			// Only preserve the StreamableTool interface when the inner tool is
			// actually streamable — otherwise the framework misclassifies the
			// wrapper and routes calls to StreamableCall (2026-07-18 fix).
			if _, streamable := t.(trpctool.StreamableTool); streamable {
				tools[i] = &streamableDisambiguatedTool{disambiguatedTool: base}
			} else {
				tools[i] = base
			}
			break
		}
	}
}

// disambiguatedTool wraps a Tool to return a modified Declaration without
// mutating the original shared Declaration object.
type disambiguatedTool struct {
	inner trpctool.Tool
	decl  *trpctool.Declaration
}

func (d *disambiguatedTool) Declaration() *trpctool.Declaration {
	return d.decl
}

func (d *disambiguatedTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if ct, ok := d.inner.(trpctool.CallableTool); ok {
		return ct.Call(ctx, jsonArgs)
	}
	return nil, fmt.Errorf("tool %q is not callable", d.decl.Name)
}

// streamableDisambiguatedTool adds StreamableCall to disambiguatedTool. It is
// a separate type so that only disambiguated tools whose inner is streamable
// satisfy trpctool.StreamableTool (mirrors streamableToolDecorator).
type streamableDisambiguatedTool struct {
	*disambiguatedTool
}

func (d *streamableDisambiguatedTool) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	if st, ok := d.inner.(trpctool.StreamableTool); ok {
		return st.StreamableCall(ctx, jsonArgs)
	}
	return nil, fmt.Errorf("tool %q is not streamable", d.decl.Name)
}

// ShouldDefer forwards the DeferredTool interface so that deferred tools
// wrapped by ApplyDisambiguationHints retain their deferred semantics.
// Without this, the framework's trpctool.ShouldDefer type assertion fails on
// the wrapper and deferred tools are eagerly loaded (B2 fix).
func (d *disambiguatedTool) ShouldDefer(ctx context.Context) bool {
	type deferred interface {
		ShouldDefer(context.Context) bool
	}
	if dt, ok := d.inner.(deferred); ok {
		return dt.ShouldDefer(ctx)
	}
	return false
}

func filterNames(names []string, exclude string) []string {
	var result []string
	for _, n := range names {
		if n != exclude {
			result = append(result, n)
		}
	}
	return result
}

// ApplyMergeDisambiguationHints 是 P0-2 阶段A 分片合并期的消歧重放，语义对齐
// 装配期 phase 11 的消歧分支，但对共享分片产物安全：
//   - flat 工具：合并期并集切片每次构建新建，就地替换元素不触及产物，与
//     装配期 ApplyDisambiguationHints(out.Tools) 等价。
//   - ToolSet 成员：装配期对 ts.Tools(ctx) 返回切片就地替换元素——对返回
//     内部切片的静态 toolset（file/email/google 等）生效，对每次返回新切片
//     的 toolset（MCP/claudecode/DeferredToolSet 等）本不生效。分片产物被
//     多代际/多 agent 共享，原地改写会跨构建叠加包装（示例文本重复），因此
//     改为 copy-on-write 包装：Tools(ctx) 返回提示后的副本，产物不可变。
//   - DeferredToolSet 跳过：装配期其 Tools(ctx) 恒返回新切片，消歧对其成员
//     从不生效，合并期保持同等语义（不在延迟工具集成员上叠加提示）。
func ApplyMergeDisambiguationHints(out *AssembledToolsets) {
	if out == nil {
		return
	}
	ApplyDisambiguationHints(out.Tools)
	for i, ts := range out.ToolSets {
		if ts == nil {
			continue
		}
		if _, ok := ts.(*deferred.DeferredToolSet); ok {
			continue
		}
		out.ToolSets[i] = &disambiguatedToolSet{inner: ts}
	}
}

// disambiguatedToolSet 是 ToolSet 的消歧 copy-on-write 包装：成员工具的
// 消歧提示作用于每次 Tools(ctx) 返回的副本，内部共享产物不被改写。
type disambiguatedToolSet struct {
	inner trpctool.ToolSet
}

var _ trpctool.ToolSet = (*disambiguatedToolSet)(nil)

func (s *disambiguatedToolSet) Name() string { return s.inner.Name() }

func (s *disambiguatedToolSet) Close() error { return s.inner.Close() }

func (s *disambiguatedToolSet) Tools(ctx context.Context) []trpctool.Tool {
	raw := s.inner.Tools(ctx)
	if len(raw) == 0 {
		return raw
	}
	cp := make([]trpctool.Tool, len(raw))
	copy(cp, raw)
	ApplyDisambiguationHints(cp)
	return cp
}
