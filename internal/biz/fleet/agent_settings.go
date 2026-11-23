package fleet

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/pkg/apierror"
)

// AgentSetting 是每个 CLI 的默认启动设置：权限档位 + 附加参数（如 Codex 的 -m <模型>）。
// 运行台里该工具的所有终端（新建、编排 worker、自动化、编码工具派活）都按它启动；
// 单个终端勾「完全放权」时以完全放权覆盖。
type AgentSetting struct {
	ID   string   `json:"id"`
	Mode string   `json:"mode"`
	Args []string `json:"args,omitempty"`
	// AutoUpdate 打开后运行台每天检查一次，满 24 小时按「重装」更新到最新版
	AutoUpdate bool `json:"auto_update,omitempty"`
	// Version / UpdatedAt 是最近一次安装成功时记下的版本与时间（安装流程写，用户设置不改它们）
	Version   string `json:"version,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	// KeyCredID / KeyWorkspace 指向「连接 › 凭据」里加密保存的 API Key（明文不落这里）；
	// KeyHint 是末 4 位，只用于显示。Account 是最近一次登录查到的账号。
	KeyCredID    int64  `json:"key_cred_id,omitempty"`
	KeyWorkspace string `json:"key_workspace,omitempty"`
	KeyHint      string `json:"key_hint,omitempty"`
	Account      string `json:"account,omitempty"`
	// Host 是随 Key 一起保存的企业自定义 API 域名（TRAE 的 TRAECLI_HOST）；空 = 官方
	Host string `json:"host,omitempty"`
	// Model 是选定的模型；空 = 用工具自己的默认
	Model string `json:"model,omitempty"`
	// Effort 是思考强度（low / medium / high / xhigh / max …，以工具支持的为准）；空 = 工具默认
	Effort string `json:"effort,omitempty"`
	// LatestVersion / LatestCheckedAt 是最近一次检查更新查到的远端最新版本与时间
	LatestVersion   string `json:"latest_version,omitempty"`
	LatestCheckedAt string `json:"latest_checked_at,omitempty"`
}

const kindAgentSetting = "agent_setting"

// GetAgentSetting 读某工具的设置；没设过返回逐项审批、无附加参数。
func (u *Usecase) GetAgentSetting(ctx context.Context, id string) AgentSetting {
	s, ok, _ := getDoc[AgentSetting](ctx, u.store, kindAgentSetting, id)
	if !ok || s.Mode == "" {
		s.Mode = PermManual
	}
	s.ID = id
	return s
}

// SetAgentSetting 保存某工具的设置。参数经 cmd.exe 传给 .cmd 垫片，带 shell 元字符的一律拒绝。
func (u *Usecase) SetAgentSetting(ctx context.Context, s AgentSetting) (AgentSetting, error) {
	p, ok := findPreset(s.ID)
	if !ok || p.ID == "shell" {
		return AgentSetting{}, apierror.BadRequest(domain, "unknown agent: "+s.ID)
	}
	switch s.Mode {
	case "", PermManual:
		s.Mode = PermManual
	case PermAutoEdit:
		if p.AutoEditFlag == "" {
			return AgentSetting{}, apierror.BadRequest(domain, p.Label+" 没有「自动改文件」档的原生参数")
		}
	case PermFull:
		if p.FullAccessFlag == "" {
			return AgentSetting{}, apierror.BadRequest(domain, p.Label+" 没有「完全放权」的原生参数")
		}
	default:
		return AgentSetting{}, apierror.BadRequest(domain, "unknown mode: "+s.Mode)
	}
	if err := validateArgs(s.Args); err != nil {
		return AgentSetting{}, err
	}
	if s.Model = strings.TrimSpace(s.Model); s.Model != "" && (p.ModelFlag == "" || !modelRe.MatchString(s.Model)) {
		return AgentSetting{}, apierror.BadRequest(domain, p.Label+" 的模型名不对："+s.Model)
	}
	if s.Effort = strings.TrimSpace(s.Effort); s.Effort != "" && (len(p.EffortArgs) == 0 || !effortRe.MatchString(s.Effort)) {
		return AgentSetting{}, apierror.BadRequest(domain, p.Label+" 不支持这个思考强度："+s.Effort)
	}
	prev := u.GetAgentSetting(ctx, s.ID)
	s.Version, s.UpdatedAt = prev.Version, prev.UpdatedAt
	s.KeyCredID, s.KeyWorkspace, s.KeyHint, s.Account, s.Host = prev.KeyCredID, prev.KeyWorkspace, prev.KeyHint, prev.Account, prev.Host
	s.LatestVersion, s.LatestCheckedAt = prev.LatestVersion, prev.LatestCheckedAt
	return s, putDoc(ctx, u.store, kindAgentSetting, s.ID, s)
}

func validateArgs(args []string) error {
	for _, a := range args {
		if a == "" || strings.ContainsAny(a, "&|<>^%\"'`\r\n\x00") || strings.Contains(a, "!") {
			return apierror.BadRequest(domain, "附加参数不能含空值或 shell 元字符："+a)
		}
	}
	return nil
}

// launchMode 决定一个终端用哪个档位：单终端完全放权 > spec 指定 > 工具默认。
func launchMode(spec sessionSpec, setting AgentSetting) string {
	switch {
	case spec.FullAccess:
		return PermFull
	case spec.Mode != "":
		return spec.Mode
	}
	return setting.Mode
}

// ---------- 上手状态：按运行台实际看到的画面判定 ----------

// 就绪状态。
const (
	ReadyUnknown   = ""
	ReadyNeedLogin = "need_login"
	ReadyNeedTrust = "need_trust"
	ReadyOK        = "ready"
)

var (
	readinessMu sync.Mutex
	readiness   = map[string]string{}
)

// AgentReadiness 返回运行台最近一次看到的该工具状态（重启后清空）。
func AgentReadiness(id string) string {
	readinessMu.Lock()
	defer readinessMu.Unlock()
	return readiness[id]
}

func setReadiness(id, state string) {
	readinessMu.Lock()
	readiness[id] = state
	readinessMu.Unlock()
}

var (
	loginPatterns = []string{"log in", "login", "sign in", "signing in", "authenticat", "扫码", "二维码", "api key", "not logged in"}
	trustPatterns = []string{"trust this", "trust the", "trust folder", "trust and continue", "workspace trust"}
)

// classifyWaiting 判断「等你」的画面是登录、目录信任，还是普通确认框（返回空串）。
func classifyWaiting(screen string) string {
	s := strings.ToLower(strings.Join(strings.Fields(screen), " "))
	for _, p := range loginPatterns {
		if strings.Contains(s, p) {
			return ReadyNeedLogin
		}
	}
	for _, p := range trustPatterns {
		if strings.Contains(s, p) {
			return ReadyNeedTrust
		}
	}
	return ""
}

// 上手状态落库：后端重启后「能对话 / 要登录」不丢。
const kindAgentReady = "agent_ready"

type readyDoc struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

func (u *Usecase) recordReadiness(id, state string) {
	if AgentReadiness(id) == state {
		return
	}
	setReadiness(id, state)
	_ = putDoc(u.ctx, u.store, kindAgentReady, id, readyDoc{ID: id, State: state})
}

func (u *Usecase) loadReadiness() {
	docs, _ := listDocs[readyDoc](u.ctx, u.store, kindAgentReady)
	for _, d := range docs {
		setReadiness(d.ID, d.State)
	}
}
