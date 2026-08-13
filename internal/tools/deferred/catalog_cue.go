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
//	You have additional tools available that are not currently loaded in your tool set.
//	To use any of these tools, call tool_load with the exact tool name.
//	
//	### <category>
//	- <tool_name>: <description>
//	### <category2>
//	- <tool_name2>: <description2>
//	
//	  ...
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
	b.WriteString("You have additional tools available that are not currently loaded in your tool set. ")
	b.WriteString("To use any of these tools, call **tool_load** with the exact tool name. ")
	b.WriteString("The tool will be immediately activated and available for use.\n\n")

	for _, cat := range categories {
		b.WriteString(fmt.Sprintf("### %s\n", cat))
		for _, entry := range byCategory[cat] {
			desc := strings.TrimSpace(entry.Description)
			if desc == "" {
				desc = "(no description)"
			}
			b.WriteString(fmt.Sprintf("- %s: %s\n", entry.Name, desc))
		}
		b.WriteString("\n")
	}

	return strings.TrimSpace(b.String())
}
