package fleet

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// AgentPreset 是一种可在终端里启动的编码 agent 命令行。
// FullAccessFlag / AutoEditFlag 可含多个参数（空格分隔）；为空表示该 agent 没有对应档位的原生参数。
type AgentPreset struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Command        string `json:"command"`
	FullAccessFlag string `json:"full_access_flag"`
	// AutoEditFlag 是「自动改文件」档（改文件不问、执行命令仍问）的原生参数。
	AutoEditFlag string `json:"auto_edit_flag"`
	// Install 是一键安装方式：npm:<包名>，或内置下载器名（见 install.go 的 archiveInstallers）；空 = 不支持。
	Install string `json:"install"`
	// ApproveKey / RejectKey 是确认框里「批准」「拒绝」的按键；空 = 回车（多数 CLI 默认高亮「是」）/ Esc。
	ApproveKey string `json:"approve_key"`
	RejectKey  string `json:"reject_key"`
	// TrustKey 是目录信任框里「信任」的按键；运行台自己建的 worktree 遇到信任框时自动按。空 = 不自动按。
	TrustKey string `json:"trust_key"`
	// LoginHint 是首次登录怎么做（上手说明）。
	LoginHint    string `json:"login_hint"`
	Available    bool   `json:"-"`
	ResolvedPath string `json:"-"`
}

// 权限档位：逐项审批（默认）/ 自动改文件 / 完全放权。
const (
	PermManual   = "manual"
	PermAutoEdit = "auto_edit"
	PermFull     = "full"
)

// 内置清单：只收命令名公开、稳定的 agent；安装来源只用官方包。
// 档位参数只在用户选了对应档位时追加（Orca 默认完全放权，这里默认逐项审批）。
// 按键只登记实测过的（2026-11-20：Cursor 的 y / a、Codex 信任框回车；拒绝都用默认 Esc）。
// 新工具不改代码也能加：在 CLI 根目录放 agents.json（见 extraPresets）。
var builtinPresets = []AgentPreset{
	{ID: "claude", Label: "Claude Code", Command: "claude", FullAccessFlag: "--dangerously-skip-permissions", AutoEditFlag: "--permission-mode acceptEdits",
		// 连同 ACP 适配器一起装：编码工具页里 claude_code 走 ACP，命令 claude-agent-acp
		Install: "npm:@anthropic-ai/claude-code @agentclientprotocol/claude-agent-acp", LoginHint: "首次启动按向导登录；用第三方模型网关的在 ~/.claude/settings.json 里配置"},
	{ID: "codex", Label: "Codex（ChatGPT）", Command: "codex", FullAccessFlag: "--dangerously-bypass-approvals-and-sandbox", AutoEditFlag: "--full-auto",
		Install: "npm:@openai/codex", ApproveKey: "y", TrustKey: "\r", LoginHint: "首次启动选「Sign in with ChatGPT」或在终端运行 codex login"},
	{ID: "gemini", Label: "Gemini CLI", Command: "gemini", FullAccessFlag: "--yolo", AutoEditFlag: "--approval-mode auto_edit",
		Install: "npm:@google/gemini-cli", LoginHint: "首次启动选「Sign in with Google」，或设置 GEMINI_API_KEY"},
	{ID: "grok", Label: "Grok CLI", Command: "grok", Install: "npm:@xai-official/grok", LoginHint: "首次启动会打开浏览器登录 x.ai，或设置 XAI_API_KEY"},
	{ID: "cursor", Label: "Cursor CLI", Command: "cursor-agent", FullAccessFlag: "--force", Install: "cursor",
		ApproveKey: "y", TrustKey: "a", LoginHint: "首次启动按任意键，在浏览器里登录 Cursor 账号"}, // 拒绝用默认 Esc：确认框和随后的「改做什么」输入框都认 Esc，按 n 会被当成输入
	{ID: "trae", Label: "TRAE CLI", Command: "traecli", Install: "trae", LoginHint: "启动后输入 /login 登录 TRAE 账号"},
	{ID: "codebuddy", Label: "CodeBuddy / WorkBuddy", Command: "codebuddy", Install: "npm:@tencent-ai/codebuddy-code", LoginHint: "首次启动选登录站点（国内站 / 国际站），在浏览器里登录"},
	{ID: "doubao", Label: "豆包 CLI", Command: "doubao", Install: "npm:@bytedance/doubao-cli", LoginHint: "首次启动用豆包 App 扫终端里的二维码，或运行 doubao login"},
	{ID: "qoder", Label: "Qoder CLI", Command: "qodercli", Install: "npm:@qoder-ai/qodercli", LoginHint: "首次启动选「Sign in」，在浏览器里登录"},
	{ID: "qwen", Label: "Qwen Code", Command: "qwen", FullAccessFlag: "--yolo", Install: "npm:@qwen-code/qwen-code"},
	{ID: "copilot", Label: "GitHub Copilot", Command: "copilot", FullAccessFlag: "--allow-all-tools", Install: "npm:@github/copilot"},
	{ID: "opencode", Label: "OpenCode", Command: "opencode", Install: "npm:opencode-ai"},
	{ID: "kimi", Label: "Kimi Code", Command: "kimi"},
	{ID: "amp", Label: "Amp", Command: "amp", Install: "npm:@sourcegraph/amp"},
	{ID: "droid", Label: "Droid", Command: "droid"},
	{ID: "shell", Label: "终端", Command: ""},
}

// CLIRoot 是一键安装的根目录：npm 包装到 <根>\npm，下载器装到 <根>\<id>。
// 取 ARANEA_FLEET_CLI_ROOT，未设时为 ~/.aranea/cli。
func CLIRoot() string {
	if r := strings.TrimSpace(os.Getenv("ARANEA_FLEET_CLI_ROOT")); r != "" {
		return r
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".aranea", "cli")
	}
	return ""
}

// cliDirs 是一键安装落下可执行文件的目录；查找命令和终端 PATH 都把它们排在系统 PATH 前面。
func cliDirs() []string {
	root := CLIRoot()
	if root == "" {
		return nil
	}
	dirs := []string{filepath.Join(root, "npm")}
	for id := range archiveInstallers {
		dirs = append(dirs, filepath.Join(root, id))
	}
	return dirs
}

// lookCommand 先在一键安装目录里找，再找 PATH。
func lookCommand(cmd string) (string, error) {
	for _, d := range cliDirs() {
		if p, err := exec.LookPath(filepath.Join(d, cmd)); err == nil {
			return p, nil
		}
	}
	return exec.LookPath(cmd)
}

// LookInCLIRoot 只在一键安装目录里找命令（不查 PATH），给 agentbridge 解析 ACP 适配器用：
// 装在 CLI 根目录里的适配器不依赖 PATH 也能被找到。
func LookInCLIRoot(cmd string) (string, bool) {
	for _, d := range cliDirs() {
		if p, err := exec.LookPath(filepath.Join(d, cmd)); err == nil {
			return p, true
		}
	}
	return "", false
}

// extraPresets 读 <CLI 根>\agents.json：id 与内置相同则覆盖，否则追加。读不到或格式错就当没有。
func extraPresets() []AgentPreset {
	b, err := os.ReadFile(filepath.Join(CLIRoot(), "agents.json"))
	if err != nil {
		return nil
	}
	var out []AgentPreset
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}

// ListPresets 返回 agent 清单及本机可用性（每次现查，装了新 CLI 不用重启）。
func ListPresets() []AgentPreset {
	list := append([]AgentPreset(nil), builtinPresets[:len(builtinPresets)-1]...) // 末项 shell 最后补
next:
	for _, x := range extraPresets() {
		if x.ID == "" || x.ID == "shell" || x.Command == "" {
			continue
		}
		if x.Label == "" {
			x.Label = x.ID
		}
		for i := range list {
			if list[i].ID == x.ID {
				list[i] = x
				continue next
			}
		}
		list = append(list, x)
	}
	list = append(list, builtinPresets[len(builtinPresets)-1])
	for i := range list {
		p := &list[i]
		if p.ID == "shell" {
			p.Command, p.ResolvedPath = shellCommand()
			p.Available = true
		} else if path, err := lookCommand(p.Command); err == nil {
			p.Available, p.ResolvedPath = true, path
		}
	}
	return list
}

func findPreset(id string) (AgentPreset, bool) {
	if id == "" {
		id = "shell"
	}
	for _, p := range ListPresets() {
		if p.ID == id {
			return p, true
		}
	}
	return AgentPreset{}, false
}

func shellCommand() (string, string) {
	if runtime.GOOS == "windows" {
		for _, c := range []string{"pwsh.exe", "powershell.exe"} {
			if p, err := exec.LookPath(c); err == nil {
				return c, p
			}
		}
		return "cmd.exe", comspec()
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh, sh
	}
	return "/bin/bash", "/bin/bash"
}

// launchArgv 组装要执行的命令行。Windows 上 npm 装的 CLI 是 .cmd 垫片，
// ConPTY 不能直接起，要经 cmd.exe /c。
func launchArgv(p AgentPreset, mode string, extra []string) []string {
	if p.ID == "shell" {
		if runtime.GOOS != "windows" {
			return []string{p.ResolvedPath, "-l"}
		}
		if c := strings.ToLower(p.Command); strings.Contains(c, "powershell") || strings.Contains(c, "pwsh") {
			return []string{p.ResolvedPath, "-NoLogo"}
		}
		return []string{p.ResolvedPath}
	}
	argv := []string{p.ResolvedPath}
	switch mode {
	case PermFull:
		argv = append(argv, strings.Fields(p.FullAccessFlag)...)
	case PermAutoEdit:
		argv = append(argv, strings.Fields(p.AutoEditFlag)...)
	}
	argv = append(argv, extra...)
	lower := strings.ToLower(p.ResolvedPath)
	if runtime.GOOS == "windows" && (strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat")) {
		return append([]string{comspec(), "/c"}, argv...)
	}
	return argv
}

// comspec 返回 cmd.exe 的绝对路径。必须是绝对路径：伪终端库把裸文件名当成
// 工作目录下的相对路径，传 "cmd.exe" 会启动失败。
func comspec() string {
	if p := os.Getenv("ComSpec"); p != "" {
		return p
	}
	if p, err := exec.LookPath("cmd.exe"); err == nil {
		return p
	}
	return `C:\Windows\System32\cmd.exe`
}

// FindPreset 按 id 找工具（含本机可用性）。
func FindPreset(id string) (AgentPreset, bool) { return findPreset(id) }
