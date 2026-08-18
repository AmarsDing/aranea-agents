package deferred

import (
	"sort"
	"strings"
)

// coreResidentToolsByProfile 定义各 profile 的核心常驻工具集。
// 核心工具直接注册到 LLM tools block（schema 稳定，会话内不变）。
// 非核心但有效的工具进入 deferred catalog，以静态目录 cue + tool_load 按需加载。
//
// 设计依据（29-token §14.4 WP-4）：
//   - 核心集 = 高频使用、编排骨架、低延迟要求的工具
//   - 长尾集 = 低频、特定场景、可容忍一次 tool_load 调用的工具
var coreResidentToolsByProfile = map[string][]string{
	"spirit": {
		// 编排骨架（闲聊也常驻：计划入口 + 收口）。DAG 构图 schema 很大，
		// 走 deferred + tool_load，不进 Request.Tools。
		"plan_and_execute", "synthesize_results", "get_team_deliverable",
		"cancel_orchestration",
		// 基础工具
		"datetime", "memory_search",
	},
	"coding": {
		// 文件操作（最高频）
		"read_file", "save_file", "list_file", "search_file", "search_content",
		"replace_content", "diff_edit", "patch_file", "read_lints", "delete_file",
		"shell_exec",
		// 基础工具
		"todo_write", "datetime",
	},
	"research": {
		// 研究核心
		"web_fetch", "memory_search", "todo_write", "datetime",
		"read_file", "save_file", "list_file", "search_file", "search_content",
	},
	"full": {
		// 文件操作 + shell + web（full profile 的高频子集）
		"read_file", "save_file", "list_file", "search_file", "search_content",
		"replace_content", "diff_edit", "patch_file",
		"shell_exec", "web_fetch",
		"todo_write", "datetime", "memory_search",
	},
	"chat_only": {
		"datetime",
	},
	"read_only": {
		"read_file", "list_file", "search_file", "search_content",
		"todo_write", "datetime",
	},
	"safe": {
		"read_file", "list_file", "search_file", "search_content",
		"todo_write", "datetime",
	},
	"system_admin": {
		"web_fetch", "datetime",
	},
	"minimal": {
		"datetime",
	},
}

// defaultCoreResidentTools 是未知 profile 的回退核心集。
var defaultCoreResidentTools = []string{"datetime"}

// SplitCoreResidentTools 将有效工具键分为核心常驻集和延迟加载集。
//
// 规则：
//   - profile 有定义：核心 = enabledTools ∩ profileCoreSet
//   - profile 无定义：核心 = enabledTools ∩ defaultCoreSet
//   - 延迟 = enabledTools - 核心
//   - 两个输出都按字典序排序（确定性，保证缓存前缀稳定）
func SplitCoreResidentTools(enabledTools []string, profile string) (core []string, deferred []string) {
	if len(enabledTools) == 0 {
		return nil, nil
	}

	profile = strings.TrimSpace(profile)
	coreSet, ok := coreResidentToolsByProfile[profile]
	if !ok {
		coreSet = defaultCoreResidentTools
	}
	coreMap := make(map[string]bool, len(coreSet))
	for _, k := range coreSet {
		coreMap[k] = true
	}

	for _, key := range enabledTools {
		if coreMap[key] {
			core = append(core, key)
		} else {
			deferred = append(deferred, key)
		}
	}

	sort.Strings(core)
	sort.Strings(deferred)
	return core, deferred
}

// CoreResidentToolsForProfile 返回指定 profile 的核心常驻工具键列表。
// 用于外部查询（如测试、文档生成）。
func CoreResidentToolsForProfile(profile string) []string {
	profile = strings.TrimSpace(profile)
	if tools, ok := coreResidentToolsByProfile[profile]; ok {
		out := make([]string, len(tools))
		copy(out, tools)
		return out
	}
	out := make([]string, len(defaultCoreResidentTools))
	copy(out, defaultCoreResidentTools)
	return out
}
