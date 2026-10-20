package agentbridge

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// M4 §B 适配器矩阵钉版本常量（版本钉集中于此）。
const (
	// claudeACPCommandNew P0-5 实测（2026-10-21，registry packument 缓存）：
	// @zed-industries/claude-code-acp@0.16.2 已 deprecated，官方迁移至
	// @agentclientprotocol/claude-agent-acp，全局安装 bin 为 claude-agent-acp。
	claudeACPCommandNew = "claude-agent-acp"
	// claudeACPCommandLegacy 旧 bin 存量安装回退（仍能工作但不再收更新）。
	claudeACPCommandLegacy = "claude-code-acp"
	// codexACPPackage codex adapter npm 包坐标。P3-1 在线复核（2026-10-21，
	// npm registry packument）：@zed-industries/codex-acp 已冻结在 0.16.0，
	// 官方迁移至 @agentclientprotocol/codex-acp@2.2.2（ACP 官方组织，持续维护）。
	codexACPPackage = "@agentclientprotocol/codex-acp"
	// shimDeepseekBin / shimGrokBin 自研 shim 二进制名（构建产物落 bin/）。
	shimDeepseekBin = "acp-shim-deepseek"
	shimGrokBin     = "acp-shim-grok"
)

// DefaultACPLaunch returns the argv Codex/Claude/CodeBuddy adapters use
// when a coding agent is registered without an explicit command (E9 / M2-5).
// This is a launch preset, not a second protocol: all five still speak ACP
// over stdio. Unknown keys leave command empty so the API can require it.
//
// M4 §B：workbuddy 归一 codebuddy；deepseek/grok 走自研 shim（bin/ 绝对路径，
// 随宿主 cwd 漂移故破例——设计 §B 深注）。
func DefaultACPLaunch(agentKey string) (command string, args []string, ok bool) {
	switch normalizeCodingAgentKey(agentKey) {
	case "codebuddy":
		return "codebuddy", []string{"--acp"}, true
	case "claude_code":
		return resolveClaudeCommand(), nil, true
	case "codex":
		return "npx", []string{"-y", codexACPPackage}, true
	case "deepseek":
		return shimCommand(shimDeepseekBin), nil, true
	case "grok":
		return shimCommand(shimGrokBin), nil, true
	default:
		return "", nil, false
	}
}

// resolveClaudeCommand 兼容探测（P3-1 终值）：先新 bin，存量回退旧 bin，
// 均缺失时返回新名（让分级探针 L1 给出「未安装」引导而非拼错名字）。
func resolveClaudeCommand() string {
	if _, err := exec.LookPath(claudeACPCommandNew); err == nil {
		return claudeACPCommandNew
	}
	if _, err := exec.LookPath(claudeACPCommandLegacy); err == nil {
		return claudeACPCommandLegacy
	}
	return claudeACPCommandNew
}

// shimCommand 推导 shim 二进制绝对路径：宿主（admin）与 shim 同居 bin/，
// 相对名会随宿主 cwd 漂移（设计 §B）。推导失败回退裸名（交探针诊断）。
func shimCommand(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		return name
	}
	return filepath.Join(filepath.Dir(exe), name)
}

func normalizeCodingAgentKey(agentKey string) string {
	k := strings.ToLower(strings.TrimSpace(agentKey))
	k = strings.ReplaceAll(k, "-", "_")
	switch k {
	case "claude", "claudecode", "claude_code_acp":
		return "claude_code"
	case "openai_codex", "codex_acp":
		return "codex"
	case "code_buddy", "tencent_codebuddy":
		return "codebuddy"
	// M4 §B：workbuddy 键归一复用 codebuddy 适配器（ADR-88 D3，禁逆向反代）。
	case "workbuddy", "work_buddy", "tencent_workbuddy":
		return "codebuddy"
	// M4 自研 shim 两键。
	case "deep_seek", "deepseek_harness":
		return "deepseek"
	case "grok_bot", "xai_grok":
		return "grok"
	default:
		return k
	}
}

// ApplyDefaultLaunch fills Command/Args when the caller omitted them and a
// known adapter preset exists. Existing explicit argv is left untouched.
func ApplyDefaultLaunch(agent *CodingAgent) {
	if agent == nil {
		return
	}
	if strings.TrimSpace(agent.Command) != "" {
		return
	}
	cmd, args, ok := DefaultACPLaunch(agent.AgentKey)
	if !ok {
		return
	}
	agent.Command = cmd
	if len(agent.Args) == 0 {
		agent.Args = append([]string(nil), args...)
	}
}
