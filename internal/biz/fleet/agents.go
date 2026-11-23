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
	LoginHint string `json:"login_hint"`
	// TestArgs 是非交互「连通测试」的参数，{prompt} 处换成测试提示；空 = 不支持连通测试。
	TestArgs []string `json:"test_args"`
	// LoginArgs 是账号登录子命令（如 claude 的 auth login）；空 = 只能在工具自己的首次启动界面里登录。
	LoginArgs []string `json:"login_args"`
	// StatusArgs 查登录状态（输出里取账号邮箱）；空 = 登录后用连通测试确认。
	StatusArgs []string `json:"status_args"`
	// EmailFlag 是把邮箱预填到登录页的参数（claude 的 --email）；空 = 不支持。
	EmailFlag string `json:"email_flag"`
	// KeyEnv 是 API Key 注入的环境变量名；空 = 不支持 API Key 登录。KeyURL 是去哪里申请 Key。
	KeyEnv string `json:"key_env"`
	KeyURL string `json:"key_url"`
	// KeyOnly：厂商已停用账号登录，只能用 API Key
	KeyOnly bool `json:"key_only"`
	// KeyName 是这家对 Key 的叫法（空 = API Key）；HostEnv 是企业自定义 API 域名的环境变量（空 = 不支持）
	KeyName string `json:"key_name"`
	HostEnv string `json:"host_env"`
	// ModelFlag 是选模型的参数（空 = 不支持选模型）；ModelsArgs 列出可用模型；Models 是没有列表命令时内置的在售型号
	ModelFlag  string      `json:"model_flag"`
	ModelsArgs []string    `json:"models_args"`
	Models     []ToolModel `json:"models"`
	// LaunchArgs 是交互终端固定带的参数（DeepSeek Harness 的 web）；ACPArgs 非空 = 编码工具走 ACP（command + ACPArgs）
	LaunchArgs []string `json:"launch_args"`
	ACPArgs    []string `json:"acp_args"`
	// EffortArgs 是思考强度参数模板（{effort} 换成档位）；Efforts 是模型没写时可选的档位
	EffortArgs   []string `json:"effort_args"`
	Efforts      []string `json:"efforts"`
	Available    bool     `json:"-"`
	ResolvedPath string   `json:"-"`
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
// 档位参数 2026-11-21 逐个对照本机版本的 --help 补齐。中间档各家语义略有不同：Claude / Gemini / Grok /
// CodeBuddy 是「改文件不问」，Cursor 的 auto-review、豆包的 ask_risky、Qoder 的 auto 是「低风险自动、高风险仍问」，
// TRAE 1.0 没有这一档，用 --allowed-tool 放行它的改文件工具（Edit / Replace）。
// 新工具不改代码也能加：在 CLI 根目录放 agents.json（见 extraPresets）。
var builtinPresets = []AgentPreset{
	{ID: "claude", Label: "Claude Code", Command: "claude", FullAccessFlag: "--dangerously-skip-permissions", AutoEditFlag: "--permission-mode acceptEdits",
		// 连同 ACP 适配器一起装：编码工具页里 claude_code 走 ACP，命令 claude-agent-acp
		Install: "npm:@anthropic-ai/claude-code @agentclientprotocol/claude-agent-acp", LoginHint: "首次启动按向导登录；用第三方模型网关的在 ~/.claude/settings.json 里配置"},
	{ID: "codex", Label: "Codex（ChatGPT）", Command: "codex", FullAccessFlag: "--dangerously-bypass-approvals-and-sandbox", AutoEditFlag: "--full-auto",
		Install: "npm:@openai/codex", ApproveKey: "y", TrustKey: "\r", LoginHint: "首次启动选「Sign in with ChatGPT」或在终端运行 codex login"},
	{ID: "gemini", Label: "Gemini", Command: "gemini", FullAccessFlag: "--yolo", AutoEditFlag: "--approval-mode auto_edit",
		Install: "npm:@google/gemini-cli", LoginHint: "首次启动选「Sign in with Google」，或设置 GEMINI_API_KEY"},
	{ID: "grok", Label: "Grok", Command: "grok", FullAccessFlag: "--permission-mode bypassPermissions", AutoEditFlag: "--permission-mode acceptEdits", Install: "npm:@xai-official/grok", LoginHint: "首次启动会打开浏览器登录 x.ai，或设置 XAI_API_KEY"},
	{ID: "cursor", Label: "Cursor", Command: "cursor-agent", FullAccessFlag: "--force", AutoEditFlag: "--auto-review", Install: "cursor",
		ApproveKey: "y", TrustKey: "a", LoginHint: "首次启动按任意键，在浏览器里登录 Cursor 账号"}, // 拒绝用默认 Esc：确认框和随后的「改做什么」输入框都认 Esc，按 n 会被当成输入
	// TRAE CLI 2.0（0.216.0，2026-11-21 对照 --help）：档位用 --permission-mode auto（确认交给自动审查）/ 完全放权
	{ID: "trae", Label: "TRAE", Command: "traecli", FullAccessFlag: "--dangerously-bypass-approvals-and-sandbox", AutoEditFlag: "--permission-mode auto",
		Install: "trae", LoginHint: "TRAE CLI 只认 TRAE 企业版账号：登录页里用企业邮箱、豆包 / 飞书账号或企业 SSO 登录"},
	{ID: "codebuddy", Label: "CodeBuddy / WorkBuddy", Command: "codebuddy", FullAccessFlag: "--permission-mode bypassPermissions", AutoEditFlag: "--permission-mode acceptEdits", Install: "npm:@tencent-ai/codebuddy-code", LoginHint: "首次启动选登录站点（国内站 / 国际站），在浏览器里登录"},
	{ID: "doubao", Label: "豆包", Command: "doubao", FullAccessFlag: "--permission-mode always_allow", AutoEditFlag: "--permission-mode ask_risky", Install: "npm:@bytedance/doubao-cli", LoginHint: "首次启动用豆包 App 扫终端里的二维码，或运行 doubao login"},
	{ID: "qoder", Label: "Qoder", Command: "qodercli", FullAccessFlag: "--dangerously-skip-permissions", AutoEditFlag: "--permission-mode auto", Install: "npm:@qoder-ai/qodercli", LoginHint: "首次启动选「Sign in」，在浏览器里登录"},
	{ID: "qwen", Label: "Qwen Code", Command: "qwen", FullAccessFlag: "--yolo", Install: "npm:@qwen-code/qwen-code"},
	{ID: "copilot", Label: "GitHub Copilot", Command: "copilot", FullAccessFlag: "--allow-all-tools", Install: "npm:@github/copilot"},
	{ID: "opencode", Label: "OpenCode", Command: "opencode", FullAccessFlag: "--auto", Install: "npm:opencode-ai"},
	// DeepSeek 官方 DeepSeek Harness（2026-11-21 核对 0.2.2-rc.5）：没有终端界面，交互走本机 Web 界面（dsh web，
	// 登录 DeepSeek 账号、填 API Key、选模型都在那里）；派活走 ACP（dsh --profile acp），连通测试用 headless。
	{ID: "deepseek", Label: "DeepSeek Harness", Command: "dsh", Install: "npm:@deepseek-ai/dsh",
		LaunchArgs: []string{"web"}, ACPArgs: []string{"--profile", "acp"},
		LoginHint: "在它的 Web 界面里登录 DeepSeek 账号，或在「设置 → 模型」填 API Key"},
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
		if len(p.TestArgs) == 0 {
			p.TestArgs = defaultTestArgs[p.ID]
		}
		applyDefaultAuth(p)
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
	argv := append([]string{p.ResolvedPath}, p.LaunchArgs...)
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
