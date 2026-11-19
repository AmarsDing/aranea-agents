package fleet

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// AgentPreset 是一种可在终端里启动的编码 agent 命令行。
// FullAccessFlag 为空表示该 agent 没有公认的「免逐项批准」参数。
type AgentPreset struct {
	ID             string
	Label          string
	Command        string
	FullAccessFlag string
	Available      bool
	ResolvedPath   string
}

// 内置清单：只收命令名与免批准参数都公开、稳定的 agent。
// 免批准参数只在用户选择「完全放权」时追加（Orca 默认追加，这里默认不加）。
var builtinPresets = []AgentPreset{
	{ID: "claude", Label: "Claude Code", Command: "claude", FullAccessFlag: "--dangerously-skip-permissions"},
	{ID: "codex", Label: "Codex", Command: "codex", FullAccessFlag: "--dangerously-bypass-approvals-and-sandbox"},
	{ID: "gemini", Label: "Gemini CLI", Command: "gemini", FullAccessFlag: "--yolo"},
	{ID: "qwen", Label: "Qwen Code", Command: "qwen", FullAccessFlag: "--yolo"},
	{ID: "copilot", Label: "GitHub Copilot", Command: "copilot", FullAccessFlag: "--allow-all-tools"},
	{ID: "opencode", Label: "OpenCode", Command: "opencode"},
	{ID: "cursor", Label: "Cursor CLI", Command: "cursor-agent"},
	{ID: "codebuddy", Label: "CodeBuddy", Command: "codebuddy"},
	{ID: "kimi", Label: "Kimi Code", Command: "kimi"},
	{ID: "amp", Label: "Amp", Command: "amp"},
	{ID: "droid", Label: "Droid", Command: "droid"},
	{ID: "shell", Label: "终端", Command: ""},
}

// ListPresets 返回内置 agent 清单及本机可用性（每次现查 PATH，装了新 CLI 不用重启）。
func ListPresets() []AgentPreset {
	out := make([]AgentPreset, 0, len(builtinPresets))
	for _, p := range builtinPresets {
		if p.ID == "shell" {
			p.Command, p.ResolvedPath = shellCommand()
			p.Available = true
		} else if path, err := exec.LookPath(p.Command); err == nil {
			p.Available, p.ResolvedPath = true, path
		}
		out = append(out, p)
	}
	return out
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
func launchArgv(p AgentPreset, fullAccess bool) []string {
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
	if fullAccess && p.FullAccessFlag != "" {
		argv = append(argv, p.FullAccessFlag)
	}
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
