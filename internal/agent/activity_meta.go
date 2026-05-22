package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	araneatools "aranea-agents/internal/tools"
)

const maxChatResultJSONBytes = 256 * 1024

type ActivityMetaInput struct {
	ToolName      string
	ArgumentsJSON string
	ResultJSON    string
	Status        string
	Author        string
	StartedAt     time.Time
	FinishedAt    *time.Time
	DurationMS    int64
	ErrorCode     string
}

type ActivityMeta struct {
	ActivityKind  string
	DisplayLabel  string
	IconKey       string
	Summary       string
	ArgumentsJSON string
	ResultJSON    string
	StartedAt     string
	FinishedAt    string
	DurationMS    int64
	ErrorCode     string
	AgentKey      string
	AgentID       string
	AgentName     string
}

func ClassifyActivityKind(toolName string) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if name == "" {
		return "tool"
	}
	switch name {
	case "skill_load", "skill_run", "skill_search", "use_skill":
		return "skill"
	case "mcp_call", "mcp_list_tools", "mcp_list_servers", "mcp_inspect_tools":
		return "mcp"
	case "transfer_to_agent", "spawn_subagent", "call_agent":
		return "subagent"
	case "load_memory", "preload_memory":
		return "memory"
	case "knowledge_search":
		return "knowledge"
	case "await_user_reply":
		return "session"
	}
	if strings.HasPrefix(name, "skill_") {
		return "skill"
	}
	if strings.HasPrefix(name, "mcp:") || strings.HasPrefix(name, "mcp_") {
		return "mcp"
	}
	if strings.HasPrefix(name, "memory_") || strings.HasPrefix(name, "working_memory.") {
		return "memory"
	}
	return "tool"
}

var builtinDisplayLabels = map[string]string{
	"read_file":        "读取文件",
	"save_file":        "保存文件",
	"replace_content":  "替换内容",
	"diff_edit":        "片段编辑",
	"patch_file":       "应用补丁",
	"exec_command":     "执行命令",
	"workspace_exec":   "执行命令",
	"skill_load":       "加载 Skill",
	"skill_run":        "运行 Skill",
	"skill_search":     "搜索 Skill",
	"mcp_call":         "MCP 调用",
	"mcp_list_tools":   "列出 MCP 工具",
	"knowledge_search": "知识库检索",
	"await_user_reply": "等待用户回复",
	"call_agent":       "调用 Agent",
}

func ResolveDisplayLabel(toolName string) string {
	name := strings.TrimSpace(toolName)
	if name == "" {
		return "tool"
	}
	if label, ok := builtinDisplayLabels[name]; ok {
		return label
	}
	if label, ok := builtinDisplayLabels[strings.ToLower(name)]; ok {
		return label
	}
	return name
}

func ResolveIconKey(kind, toolName string) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch name {
	case "read_file", "save_file", "write_file", "replace_content", "diff_edit", "patch_file":
		return "description"
	case "exec_command", "workspace_exec", "shell_exec":
		return "terminal"
	case "skill_run":
		return "play_circle"
	case "skill_load":
		return "download"
	}
	switch kind {
	case "skill":
		return "auto_awesome"
	case "mcp":
		return "hub"
	case "subagent":
		return "group"
	case "memory":
		return "psychology"
	case "knowledge":
		return "menu_book"
	case "session":
		return "forum"
	default:
		return "build"
	}
}

func BuildSummary(kind, toolName string, argsJSON []byte) string {
	args := parseJSONObject(argsJSON)
	if len(args) == 0 {
		return ""
	}
	switch kind {
	case "skill":
		if v := firstStringArg(args, "skill", "skill_name", "name"); v != "" {
			return v
		}
	case "mcp":
		server := firstStringArg(args, "server_key", "server", "mcp_server")
		tool := firstStringArg(args, "tool_name", "tool", "name")
		if server != "" && tool != "" {
			return server + "/" + tool
		}
		if server != "" {
			return server
		}
	case "knowledge":
		collection := firstStringArg(args, "collection_id", "collection")
		query := firstStringArg(args, "query", "q")
		if collection != "" && query != "" {
			return collection + " · " + truncateRunes(query, 40)
		}
		if query != "" {
			return truncateRunes(query, 40)
		}
	}
	if path := firstStringArg(args, "file_name", "path", "file_path"); path != "" {
		switch strings.ToLower(strings.TrimSpace(toolName)) {
		case "diff_edit":
			if n := diffEditCountFromArgs(args); n > 0 {
				return "`" + path + "` · " + fmt.Sprintf("%d edit(s)", n)
			}
		case "patch_file":
			if n := patchHunkCountFromArgs(args); n > 0 {
				return "`" + path + "` · " + fmt.Sprintf("%d hunk(s)", n)
			}
		}
		return "`" + path + "`"
	}
	if cmd := firstStringArg(args, "command", "cmd"); cmd != "" {
		return truncateRunes(cmd, 80)
	}
	return ""
}

func BuildActivityMeta(ctx context.Context, in ActivityMetaInput, resolver ActivityMetaResolver) ActivityMeta {
	kind := ClassifyActivityKind(in.ToolName)
	argsSanitized := SanitizeJSONString(in.ArgumentsJSON)
	resultSanitized := truncateJSONString(SanitizeJSONString(in.ResultJSON), maxChatResultJSONBytes)

	summary := BuildSummary(kind, in.ToolName, []byte(argsSanitized))
	if extra := fileEditResultSummary(in.ToolName, resultSanitized); extra != "" {
		if summary != "" {
			summary = summary + " · " + extra
		} else {
			summary = extra
		}
	}

	displayLabel := ResolveDisplayLabel(in.ToolName)
	if resolver != nil {
		if v := strings.TrimSpace(resolver.ResolveDisplayLabel(ctx, in.ToolName)); v != "" {
			displayLabel = v
		}
	}
	agentKey := strings.TrimSpace(in.Author)
	agentName := agentKey
	agentID := ""
	if resolver != nil && agentKey != "" {
		if v := strings.TrimSpace(resolver.ResolveAgentDisplayName(ctx, agentKey)); v != "" {
			agentName = v
		}
		if v := strings.TrimSpace(resolver.ResolveAgentID(ctx, agentKey)); v != "" {
			agentID = v
		}
	}

	meta := ActivityMeta{
		ActivityKind:  kind,
		DisplayLabel:  displayLabel,
		IconKey:       ResolveIconKey(kind, in.ToolName),
		Summary:       summary,
		ArgumentsJSON: argsSanitized,
		ResultJSON:    resultSanitized,
		DurationMS:    in.DurationMS,
		ErrorCode:     in.ErrorCode,
		AgentKey:      agentKey,
		AgentID:       agentID,
		AgentName:     agentName,
	}
	if !in.StartedAt.IsZero() {
		meta.StartedAt = in.StartedAt.UTC().Format(time.RFC3339Nano)
	}
	if in.FinishedAt != nil && !in.FinishedAt.IsZero() {
		meta.FinishedAt = in.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	return meta
}

func SanitizeJSONString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}
	out, err := json.Marshal(sanitizeJSONValue(parsed))
	if err != nil {
		return raw
	}
	return string(out)
}

func sanitizeJSONValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if isSensitiveKey(key) {
				out[key] = "***"
				continue
			}
			out[key] = sanitizeJSONValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, child := range v {
			out = append(out, sanitizeJSONValue(child))
		}
		return out
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, token := range []string{"api_key", "apikey", "token", "secret", "password", "authorization", "cookie"} {
		if strings.Contains(key, token) {
			return true
		}
	}
	return false
}

func truncateJSONString(raw string, maxBytes int) string {
	if maxBytes <= 0 || len(raw) <= maxBytes {
		return raw
	}
	return raw[:maxBytes] + "…"
}

func parseJSONObject(raw []byte) map[string]any {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func firstStringArg(args map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := args[key]
		if !ok || v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

func diffEditCountFromArgs(args map[string]any) int {
	raw, ok := args["edits"]
	if !ok || raw == nil {
		return 0
	}
	items, ok := raw.([]any)
	if !ok {
		return 0
	}
	return len(items)
}

func patchHunkCountFromArgs(args map[string]any) int {
	if raw, ok := args["hunks"]; ok {
		if items, ok := raw.([]any); ok && len(items) > 0 {
			return len(items)
		}
	}
	if patch := firstStringArg(args, "patch"); patch != "" {
		return 1
	}
	return 0
}

func fileEditResultSummary(toolName, resultJSON string) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if name != "diff_edit" && name != "patch_file" && name != "edit_file" {
		return ""
	}
	obj := parseJSONObject([]byte(resultJSON))
	if len(obj) == 0 {
		return ""
	}
	if applied, ok := obj["applied_edits"].(float64); ok && applied > 0 {
		if total, ok := obj["total_replacements"].(float64); ok && total > 0 {
			return fmt.Sprintf("%d applied · %d repl", int(applied), int(total))
		}
		return fmt.Sprintf("%d edit(s) applied", int(applied))
	}
	if applied, ok := obj["applied_hunks"].(float64); ok && applied > 0 {
		return fmt.Sprintf("%d hunk(s) applied", int(applied))
	}
	if hunks, ok := obj["structured_patch"].([]any); ok && len(hunks) > 0 {
		return fmt.Sprintf("%d hunk(s)", len(hunks))
	}
	return ""
}

// CatalogLookupKeysForRuntimeName returns catalog keys to query for a runtime tool name.
func CatalogLookupKeysForRuntimeName(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	seen := map[string]struct{}{name: {}}
	keys := []string{name}
	if canon, ok := araneatools.RuntimeToolNameAliases[name]; ok && canon != "" {
		if _, ok := seen[canon]; !ok {
			keys = append(keys, canon)
			seen[canon] = struct{}{}
		}
	}
	for alias, canon := range araneatools.RuntimeToolNameAliases {
		if canon == name && alias != name {
			if _, ok := seen[alias]; !ok {
				keys = append(keys, alias)
				seen[alias] = struct{}{}
			}
		}
	}
	return keys
}
