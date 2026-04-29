package runctx

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/genai"
)

// RuntimeContext 为写入系统提示词的结构化运行时载荷，使模型明确知晓当前会话、
// 所属团队（若有）及本回合可用工具面。注入该层可系统性修复模型滥用文件系统工具
// 回答本已在后端隐含的问题（例如团队成员数量）的情况。
type RuntimeContext struct {
	Session  SessionContext
	Team     *TeamContext
	SelfRole string
	Tools    []ToolHint
}

type SessionContext struct {
	SessionID  string
	DialogMode string
	StartedAt  string
}

type TeamContext struct {
	TeamID      string
	DisplayName string
	Mode        string
	Members     []TeamMemberContext
}

type TeamMemberContext struct {
	AgentID string
	Role    string
	Name    string
}

type ToolHint struct {
	Name        string
	Description string
}

// CloneWithRole 返回替换 SelfRole 后的浅拷贝。team_runtime 在成员间共享同一上下文但各自 SelfRole 不同时使用。
func (c *RuntimeContext) CloneWithRole(role string) *RuntimeContext {
	if c == nil {
		return &RuntimeContext{SelfRole: role}
	}
	clone := *c
	clone.SelfRole = role
	return &clone
}

// RenderBlock 返回追加到系统提示词的确定性固定格式块。空上下文返回空字符串，
// 调用方可安全省略该节。该函数是 runctx 包对外的渲染入口，原 internal/runtime
// 包内同名 unexported 版本（renderRuntimeContextBlock）已随 row #1 迁移至此并
// 升级为 exported。
func RenderBlock(rc *RuntimeContext) string {
	if rc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Runtime Context\n")
	if rc.Session.SessionID != "" {
		b.WriteString("- session_id: ")
		b.WriteString(rc.Session.SessionID)
		b.WriteString("\n")
	}
	if rc.Session.DialogMode != "" {
		b.WriteString("- dialog_mode: ")
		b.WriteString(rc.Session.DialogMode)
		b.WriteString("\n")
	}
	if rc.Session.StartedAt != "" {
		b.WriteString("- started_at: ")
		b.WriteString(rc.Session.StartedAt)
		b.WriteString("\n")
	}
	if rc.SelfRole != "" {
		b.WriteString("- self_role: ")
		b.WriteString(rc.SelfRole)
		b.WriteString("\n")
	}
	if rc.Team != nil {
		b.WriteString("\n### Team\n")
		if rc.Team.DisplayName != "" {
			b.WriteString("- name: ")
			b.WriteString(rc.Team.DisplayName)
			b.WriteString("\n")
		}
		if rc.Team.TeamID != "" {
			b.WriteString("- team_id: ")
			b.WriteString(rc.Team.TeamID)
			b.WriteString("\n")
		}
		if rc.Team.Mode != "" {
			b.WriteString("- orchestration_mode: ")
			b.WriteString(rc.Team.Mode)
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf("- member_count: %d\n", len(rc.Team.Members)))
		if len(rc.Team.Members) > 0 {
			b.WriteString("- members:\n")
			for i, member := range rc.Team.Members {
				label := firstNonEmpty(member.Name, member.Role, member.AgentID)
				role := firstNonEmpty(member.Role, "member")
				b.WriteString(fmt.Sprintf("  %d. %s (role=%s, agent_id=%s)\n", i+1, label, role, member.AgentID))
			}
		}
	}
	if len(rc.Tools) > 0 {
		b.WriteString("\n### Available Tools\n")
		hints := append([]ToolHint(nil), rc.Tools...)
		sort.SliceStable(hints, func(i, j int) bool { return hints[i].Name < hints[j].Name })
		for _, hint := range hints {
			b.WriteString("- `")
			b.WriteString(hint.Name)
			b.WriteString("`")
			if desc := strings.TrimSpace(hint.Description); desc != "" {
				b.WriteString(": ")
				b.WriteString(escapeInstructionPlaceholders(desc))
			}
			b.WriteString("\n")
		}
	} else if rc.Session.SessionID != "" || rc.Team != nil {
		b.WriteString("\n### Available Tools\n- (none — this turn must be answered from context)\n")
	}
	b.WriteString(renderToolUsagePolicy(rc))
	return b.String()
}

func renderToolUsagePolicy(rc *RuntimeContext) string {
	if rc == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Tool Usage Policy\n")
	b.WriteString("1. Treat the Runtime Context block as authoritative. If the user's question is satisfied by it (e.g. team membership, session metadata, current self role), answer directly without calling any tool.\n")
	b.WriteString("2. Use a tool only when (a) the answer requires data the Runtime Context does not contain and (b) the chosen tool's description matches the data class needed.\n")
	b.WriteString("3. Never call file system tools (read_file, list_files, write_file, edit_file) to ask about agents, teams, members, sessions, providers, models or any in-app metadata. That information lives in the Runtime Context only.\n")
	b.WriteString("4. If the same tool fails twice with similar arguments, stop calling it and explain the limitation to the user instead of retrying further.\n")
	b.WriteString("5. Prefer answering with reasoning. A turn that returns a clear answer without any tool call is preferred over a turn with redundant or speculative tool calls.\n")
	return b.String()
}

// escapeInstructionPlaceholders 消除 "{name}" 模式，否则 ADK 指令处理器会将其当作会话状态占位符。
// 若无此防护，工具描述中的 "{ path }" 进入系统模板后会触发运行时的「state key does not exist」错误。
func escapeInstructionPlaceholders(text string) string {
	if !strings.ContainsAny(text, "{}") {
		return text
	}
	replacer := strings.NewReplacer("{", "[", "}", "]")
	return replacer.Replace(text)
}

// ToolHintsFromDeclarations 将发给模型的函数声明转为适合运行时上下文渲染的结构化提示。
// 用于已按 Agent 运行时设置过滤工具的调用点（见 adkRuntimeTools）。
func ToolHintsFromDeclarations(declarations []*genai.FunctionDeclaration) []ToolHint {
	if len(declarations) == 0 {
		return nil
	}
	hints := make([]ToolHint, 0, len(declarations))
	for _, declaration := range declarations {
		if declaration == nil || strings.TrimSpace(declaration.Name) == "" {
			continue
		}
		hints = append(hints, ToolHint{Name: declaration.Name, Description: declaration.Description})
	}
	return hints
}

// firstNonEmpty 是 runctx 内部的字符串选择器；与 internal/runtime 等其它包中
// 同名 helper 是独立副本，保持 runctx 包零外部依赖（除 strings/genai）。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
