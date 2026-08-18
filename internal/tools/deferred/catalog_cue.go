package deferred

import (
	"fmt"
	"sort"
	"strings"
)

// RenderCatalogCue 为 LLM 生成静态工具目录 cue。
//
// 输出格式（按类别分组，类别和工具均按字典序排序，保证确定性）：
//
//	## Available Tools Catalog
//
//	Call **tool_load** with the exact tool name to activate a listed tool.
//
//	### <category>
//	- <tool_name>: <first sentence, ≤80 runes>
//
// 设计约束（29-token §14.4 WP-4）：
//   - 只包含工具名 + 一句话描述，不含 schema/参数/示例
//   - 内容按字典序确定，会话内不变 → 缓存前缀稳定
//   - cue 注入到消息流尾部（非系统 prompt），与现有静态 runtime cue 隔离
//
// 当 catalog 为空时返回空字符串，调用者跳过注入。
func RenderCatalogCue(catalog []DeferredToolEntry) string {
	if len(catalog) == 0 {
		return ""
	}

	// 按类别分组，并按字典序排序
	byCategory := make(map[string][]DeferredToolEntry)
	var categories []string
	for _, entry := range catalog {
		cat := entry.Category
		if cat == "" {
			cat = "other"
		}
		if len(byCategory[cat]) == 0 {
			categories = append(categories, cat)
		}
		byCategory[cat] = append(byCategory[cat], entry)
	}
	sort.Strings(categories)

	// 每个类别内按工具名排序
	for cat := range byCategory {
		sort.Slice(byCategory[cat], func(i, j int) bool {
			return byCategory[cat][i].Name < byCategory[cat][j].Name
		})
	}

	var b strings.Builder
	b.WriteString("## Available Tools Catalog\n\n")
	b.WriteString("Call **tool_load** with the exact tool name to activate a listed tool.\n\n")

	for _, cat := range categories {
		b.WriteString(fmt.Sprintf("### %s\n", cat))
		for _, entry := range byCategory[cat] {
			desc := compactCatalogDesc(entry.Description)
			if desc == "" {
				b.WriteString(fmt.Sprintf("- %s\n", entry.Name))
			} else {
				b.WriteString(fmt.Sprintf("- %s: %s\n", entry.Name, desc))
			}
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}

const catalogDescMaxRunes = 80

// compactCatalogDesc keeps the catalog cue to a short first sentence so
// deferring more tools does not grow tail tokens as fast as the schema saved.
func compactCatalogDesc(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	cut := len(desc)
	for _, sep := range []string{"。", "；", ". ", "; "} {
		if i := strings.Index(desc, sep); i > 0 && i < cut {
			cut = i
		}
	}
	desc = strings.TrimSpace(desc[:cut])
	runes := []rune(desc)
	if len(runes) > catalogDescMaxRunes {
		return string(runes[:catalogDescMaxRunes-1]) + "…"
	}
	return desc
}
