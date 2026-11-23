package fleet

import (
	"context"
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 工具的登录：账号登录跑工具自己的登录子命令（浏览器授权 / 设备码 / 扫码），aranea 只把登录页链接、
// 设备码、二维码从输出里取出来摆到页面上，不经手账号密码；API Key 先真实验证，再加密存进
// 「连接 › 凭据」，启动该工具时注入环境变量。

// authSpec 是各家登录方式，2026-11-21 对照本机装的版本 --help 与包内代码核实。
type authSpec struct {
	login, status []string
	emailFlag     string
	keyEnv        string
	keyURL        string
	// keyOnly：厂商已停用账号登录，只能用 API Key
	keyOnly bool
	// keyName 这家对 Key 的叫法；hostEnv 企业自定义 API 域名的环境变量；keyLogin 用 Key 登录的命令
	// （TRAE 的令牌要先 login --with-trae-pat 换成登录态，验证就是跑它再查登录状态）
	keyName, hostEnv string
	keyLogin         []string
	// 用 Key 时给工具一个 aranea 自己的配置目录（keyHomeEnv 指过去，keyHomeFile 写 keySettings），
	// 把认证方式切到 API Key，用户自己的配置不动；keyEnvExtra 一并加上
	keyHomeEnv, keyHomeFile, keySettings string
	keyEnvExtra                          []string
	// modelFlag 选模型的参数；models 列模型的命令；staticModels 没有列表命令时内置的在售型号
	modelFlag    string
	models       []string
	staticModels []ToolModel
	// effortArgs 思考强度参数模板；efforts 模型没写时可选的档位（2026-11-21 对照各家 --help）
	effortArgs []string
	efforts    []string
	// keyModels 存了 API Key 时直接问厂商的模型列表接口（Claude Code 账号登录没有列表命令）
	keyModels keyModelsFunc
}

var defaultAuth = map[string]authSpec{
	"claude": {login: []string{"auth", "login"}, status: []string{"auth", "status"}, emailFlag: "--email",
		keyEnv: "ANTHROPIC_API_KEY", keyURL: "https://console.anthropic.com/settings/keys",
		modelFlag: "--model", staticModels: claudeLineup, keyModels: anthropicModels,
		effortArgs: []string{"--effort", "{effort}"}, efforts: []string{"low", "medium", "high", "xhigh", "max"}},
	"codex": {login: []string{"login"}, status: []string{"login", "status"},
		keyEnv: "CODEX_API_KEY", keyURL: "https://platform.openai.com/api-keys",
		modelFlag: "-m", models: []string{"debug", "models"}, effortArgs: []string{"-c", "model_reasoning_effort={effort}"}},
	// 2026-11 起 Google 不再让个人账号用 Gemini CLI（报 no longer supported，让迁到 Antigravity），只剩 API Key。
	// 用户设置里选过 Google 登录时非交互模式会先用它、无视 GEMINI_API_KEY；系统级设置要管理员目录，
	// 所以用 GEMINI_CLI_HOME 给它一个只选 API Key 的配置目录（没有用户的信任目录表，按官方变量放行工作区）
	"gemini": {keyOnly: true, keyEnv: "GEMINI_API_KEY", keyURL: "https://aistudio.google.com/apikey",
		keyHomeEnv: "GEMINI_CLI_HOME", keyHomeFile: ".gemini/settings.json",
		keySettings: `{"security":{"auth":{"selectedType":"gemini-api-key"}}}`, keyEnvExtra: []string{"GEMINI_CLI_TRUST_WORKSPACE=true"},
		modelFlag: "-m", models: []string{"--list-models"}, keyModels: geminiModels},
	"cursor": {login: []string{"login"}, status: []string{"status"},
		keyEnv: "CURSOR_API_KEY", keyURL: "https://cursor.com/dashboard?tab=integrations",
		modelFlag: "--model", models: []string{"--list-models"}},
	// --oauth：浏览器回环登录，授权完自动回来；默认的设备码要手抄代码。models 顺带报登录状态
	"grok": {login: []string{"login", "--oauth"}, status: []string{"models"}, keyEnv: "XAI_API_KEY", keyURL: "https://console.x.ai",
		modelFlag: "-m", models: []string{"models"},
		effortArgs: []string{"--reasoning-effort", "{effort}"}, efforts: []string{"low", "medium", "high"}},
	"doubao": {login: []string{"login", "--qr=png"}, status: []string{"auth", "status", "--verify"},
		modelFlag: "--model", models: []string{"model", "list", "--json"}, effortArgs: []string{"--effort", "{effort}"}},
	"qoder": {login: []string{"login"}, status: []string{"status"}, keyEnv: "QODER_PERSONAL_ACCESS_TOKEN", keyName: "访问令牌",
		modelFlag: "-m", models: []string{"--list-models"},
		effortArgs: []string{"--reasoning-effort", "{effort}"}, efforts: []string{"low", "medium", "high"}},
	// TRAE CLI 2.0（0.216.0 的 login --help 与 docs.trae.cn「快速开始」，2026-11-21 实测）：只认 TRAE 企业版账号——
	// login 与 login --enterprise 打开的是同一个「企业版登录」页（企业邮箱 / 豆包飞书 / 企业 SSO）；
	// 令牌（企业版 PAT）用 login --with-trae-pat，企业自定义 API 域名加 TRAECLI_HOST
	"trae": {login: []string{"login"}, status: []string{"login", "status"},
		keyEnv: "TRAECLI_PERSONAL_ACCESS_TOKEN", keyName: "访问令牌", hostEnv: "TRAECLI_HOST", keyLogin: []string{"login", "--with-trae-pat"},
		modelFlag: "-m", models: []string{"models"}},
	"deepseek": {keyEnv: "DEEPSEEK_API_KEY", keyURL: "https://platform.deepseek.com/api_keys", keyModels: deepseekModels},
	// CodeBuddy 没有列表命令，--help 里写着「Currently supported: (…)」
	"codebuddy": {keyEnv: "CODEBUDDY_API_KEY", modelFlag: "--model", models: []string{"--help"},
		effortArgs: []string{"--effort", "{effort}"}, efforts: []string{"minimal", "low", "medium", "high", "xhigh", "max"}},
}

// claudeLineup 是 Claude Code 账号登录时可选的在售型号（与 Claude 客户端的模型菜单一致，2026-11-21）。
// 账号登录没有列表命令；存了 API Key 时改用 Anthropic 模型列表接口，拿到实时的完整列表与各型号支持的强度。
var claudeLineup = []ToolModel{
	{ID: "claude-opus-5-5", Name: "Opus 5.5", Desc: "默认：大多数编码与智能体任务", DefaultEffort: "medium", Default: true},
	{ID: "claude-fable-5-2", Name: "Fable 5.2", Desc: "最强：最难的推理与长任务，更慢更贵", DefaultEffort: "high"},
	{ID: "claude-sonnet-5-5", Name: "Sonnet 5.5", Desc: "速度与能力兼顾的日常模型", DefaultEffort: "high"},
	{ID: "claude-haiku-5-5", Name: "Haiku 5.5", Desc: "最快、最省", DefaultEffort: "medium"},
}

// applyDefaultAuth 给清单项补上内置的登录方式（agents.json 里写了的以它为准）。
func applyDefaultAuth(p *AgentPreset) {
	d, ok := defaultAuth[p.ID]
	if !ok {
		return
	}
	if len(p.LoginArgs) == 0 {
		p.LoginArgs = d.login
	}
	if len(p.StatusArgs) == 0 {
		p.StatusArgs = d.status
	}
	if p.EmailFlag == "" {
		p.EmailFlag = d.emailFlag
	}
	if p.KeyEnv == "" {
		p.KeyEnv, p.KeyURL, p.KeyOnly, p.KeyName, p.HostEnv = d.keyEnv, d.keyURL, d.keyOnly, d.keyName, d.hostEnv
	}
	if p.ModelFlag == "" {
		p.ModelFlag, p.ModelsArgs, p.Models = d.modelFlag, d.models, d.staticModels
	}
	if len(p.EffortArgs) == 0 {
		p.EffortArgs, p.Efforts = d.effortArgs, d.efforts
	}
}

// keyEnvFor 是用某个 Key 启动工具时要加的环境变量（含切换认证方式的设置文件）。
func keyEnvFor(p AgentPreset, key string) []string {
	env := []string{p.KeyEnv + "=" + key}
	d := defaultAuth[p.ID]
	if d.keyHomeEnv == "" {
		return env
	}
	base := CLIRoot()
	if base == "" {
		base = os.TempDir()
	}
	home := filepath.Join(base, ".keyhome", p.ID)
	f := filepath.Join(home, filepath.FromSlash(d.keyHomeFile))
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err == nil && os.WriteFile(f, []byte(d.keySettings), 0o600) == nil {
		env = append(append(env, d.keyHomeEnv+"="+home), d.keyEnvExtra...)
	}
	return env
}

// ---------- API Key ----------

// KeyVault 是 API Key 的加密保管处（由「连接 › 凭据」实现）。fleet 只记凭据 id 与所属工作区。
type KeyVault interface {
	// Save 新建一条凭据；prevID>0 时轮换那一条。ctx 带调用者的工作区，返回凭据 id 与工作区。
	Save(ctx context.Context, prevID int64, name, site, secret string) (id int64, ws string, err error)
	Reveal(ctx context.Context, ws string, id int64) (string, error)
	Delete(ctx context.Context, ws string, id int64) error
}

// keyVault 进程内只有一个运行台用例，凭据保管处在装配时登记（与 SetFleetBridge 同例）。
var keyVault atomic.Pointer[KeyVault]

// SetKeyVault 登记 API Key 保管处；nil 忽略。
func SetKeyVault(v KeyVault) {
	if v != nil {
		keyVault.Store(&v)
	}
}

func getKeyVault() KeyVault {
	if p := keyVault.Load(); p != nil {
		return *p
	}
	return nil
}

// SaveToolKey 先带着新 Key 真实发一句话验证，通过（或只是额度受限）才加密保存；过程与结论走连通测试的状态。
func (u *Usecase) SaveToolKey(ctx context.Context, id, key, host string) error {
	p, ok := findPreset(id)
	if !ok || p.ID == "shell" {
		return apierror.BadRequest(domain, "unknown agent: "+id)
	}
	if p.KeyEnv == "" {
		return apierror.FailedPrecondition(domain, p.Label+" 不支持用 API Key 登录")
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 512 || strings.ContainsAny(key, " \t\r\n\x00") {
		return apierror.BadRequest(domain, keyName(p)+" 格式不对（不能为空、不能含空格或换行）")
	}
	if host = strings.TrimRight(strings.TrimSpace(host), "/"); host != "" && (p.HostEnv == "" || !hostRe.MatchString(host)) {
		return apierror.BadRequest(domain, "企业 API 域名要写成 https://api.example.com 这样的地址")
	}
	env := append(keyEnvFor(p, key), hostEnvFor(p, host)...)
	v := getKeyVault()
	if v == nil {
		return apierror.FailedPrecondition(domain, "凭据保管没有启用，不能保存 API Key")
	}
	vctx := context.WithoutCancel(ctx) // 保留调用者的工作区，验证在后台跑完
	// 厂商有模型列表接口的（Claude、Gemini）直接拿 Key 调接口验证：快，不耗对话额度；其余真实对话一次
	verify := func(p AgentPreset, set AgentSetting, dir string, j *installJob) TestResult {
		if f := defaultAuth[p.ID].keyModels; f != nil {
			return verifyKeyByModels(u.ctx, key, f, j)
		}
		if len(defaultAuth[p.ID].keyLogin) > 0 {
			return u.runKeyLogin(p, env, j)
		}
		return u.runSmokeTest(p, set, dir, j, env)
	}
	return u.startTest(id, true, verify, func(res TestResult) TestResult {
		limited := strings.HasPrefix(res.Verdict, "模型服务拒绝")
		if !res.OK && !limited {
			if strings.HasPrefix(res.Verdict, "要登录") {
				res.Verdict = keyName(p) + " 无效或没有权限，没有保存"
			} else {
				res.Verdict = keyName(p) + " 没通过验证，没有保存：" + res.Verdict
			}
			return res
		}
		set := u.GetAgentSetting(vctx, id)
		cid, ws, err := v.Save(vctx, set.KeyCredID, p.Label+" "+keyName(p), keySite(p), key)
		if err != nil {
			res.OK, res.Verdict = false, "验证通过，但保存失败："+err.Error()
			return res
		}
		set.KeyCredID, set.KeyWorkspace, set.KeyHint, set.Host = cid, ws, keyHint(key), host
		if err := putDoc(vctx, u.store, kindAgentSetting, id, set); err != nil {
			res.OK, res.Verdict = false, "保存失败："+err.Error()
			return res
		}
		u.recordReadiness(id, ReadyOK)
		res.OK, res.Verdict = true, keyName(p)+" 已验证并保存"
		if limited {
			res.Verdict += "（Key 有效，但目前额度用完或被限流）"
		}
		return res
	})
}

// DeleteToolKey 删掉保存的 API Key（凭据页里已删的也照样清掉记录）。
func (u *Usecase) DeleteToolKey(ctx context.Context, id string) error {
	set := u.GetAgentSetting(ctx, id)
	if set.KeyCredID == 0 {
		return nil
	}
	if v := getKeyVault(); v != nil {
		if err := v.Delete(ctx, set.KeyWorkspace, set.KeyCredID); err != nil && !apierror.IsCode(err, apierror.CodeNotFound) {
			return err
		}
	}
	set.KeyCredID, set.KeyWorkspace, set.KeyHint, set.Host = 0, "", "", ""
	return putDoc(ctx, u.store, kindAgentSetting, id, set)
}

var hostRe = regexp.MustCompile(`^https://[A-Za-z0-9.-]+(:\d{1,5})?(/[A-Za-z0-9._~/-]*)?$`)

func keyName(p AgentPreset) string {
	if p.KeyName != "" {
		return p.KeyName
	}
	return "API Key"
}

// hostEnvFor 是企业自定义 API 域名的环境变量；没填或这家不支持返回空。
func hostEnvFor(p AgentPreset, host string) []string {
	if host == "" || p.HostEnv == "" {
		return nil
	}
	return []string{p.HostEnv + "=" + host}
}

// runKeyLogin 带着令牌跑工具自己的登录命令（TRAE 的 login --with-trae-pat），成功后再查一次登录状态。
func (u *Usecase) runKeyLogin(p AgentPreset, env []string, j *installJob) TestResult {
	args := defaultAuth[p.ID].keyLogin
	j.logf("$ %s %s", p.Command, strings.Join(args, " "))
	start := time.Now()
	ctx, cancel := context.WithTimeout(u.ctx, 90*time.Second)
	defer cancel()
	var buf strings.Builder
	err := runTool(ctx, p, args, os.TempDir(), append(u.terminalEnv("toolstatus", ""), env...), &buf)
	out := StripANSI(buf.String())
	if t := strings.TrimSpace(out); t != "" {
		j.logf("%s", lastLines(t, 8))
	}
	res := TestResult{DurationMS: time.Since(start).Milliseconds(), Reply: lastLines(out, 3)}
	low := strings.ToLower(out)
	switch {
	case ctx.Err() != nil:
		res.Verdict = "登录命令 90 秒没有结束"
	case err != nil && containsAny(low, "dial tcp", "no such host", "connection refused", "timed out", "network", "proxy", "tls"):
		res.Verdict = "连不上登录服务：检查网络或企业 API 域名"
	case err != nil:
		res.Verdict = "要登录：" + lastLines(out, 1)
	default:
		ok, account, msg := u.checkLogin(p)
		if !ok {
			res.Verdict = "要登录：" + msg
			break
		}
		res.OK, res.Verdict = true, "能用"
		if account != "" {
			res.Verdict += "（已登录 " + account + "）"
			set := u.GetAgentSetting(u.ctx, p.ID)
			set.Account = account
			_ = putDoc(u.ctx, u.store, kindAgentSetting, p.ID, set)
		}
	}
	j.logf("— %s", res.Verdict)
	return res
}

// toolKeyEnv 是启动该工具时注入的 API Key 环境变量；没存或取不出来返回空。
func (u *Usecase) toolKeyEnv(p AgentPreset) []string {
	if key := u.toolKey(p); key != "" {
		return append(keyEnvFor(p, key), hostEnvFor(p, u.GetAgentSetting(u.ctx, p.ID).Host)...)
	}
	return nil
}

// toolKey 取出该工具已存的 API Key 明文；没存或取不出来返回空。
func (u *Usecase) toolKey(p AgentPreset) string {
	if p.KeyEnv == "" {
		return ""
	}
	set := u.GetAgentSetting(u.ctx, p.ID)
	v := getKeyVault()
	if set.KeyCredID == 0 || v == nil {
		return ""
	}
	key, err := v.Reveal(u.ctx, set.KeyWorkspace, set.KeyCredID)
	if err != nil {
		u.lg.Warn("fleet tool key reveal failed", loggateway.Str("agent", p.ID), loggateway.Err(err))
	}
	return key
}

// keySite 是凭据记录的站点：申请 Key 的网站域名；没有就用 <工具 id>.cli。
func keySite(p AgentPreset) string {
	if u, err := url.Parse(p.KeyURL); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return p.ID + ".cli"
}

func keyHint(key string) string {
	if len(key) <= 8 {
		return "…"
	}
	return "…" + key[len(key)-4:]
}

// ---------- 账号登录 ----------

// LoginState 是一次账号登录的进展：登录页链接 / 设备码 / 二维码，以及结束后的结论。
type LoginState struct {
	TerminalID string `json:"terminal_id"`
	Running    bool   `json:"running"`
	Done       bool   `json:"done"`
	OK         bool   `json:"ok"`
	URL        string `json:"url"`
	Code       string `json:"code"`
	QR         string `json:"qr"` // data:image/png;base64,…
	Account    string `json:"account"`
	Message    string `json:"message"`
}

type loginJob struct {
	sid string
	mu  sync.Mutex
	end *LoginState // 登录命令结束后定下的结论
}

var (
	loginMu   sync.Mutex
	loginJobs = map[string]*loginJob{}
	emailRe   = regexp.MustCompile(`^[A-Za-z0-9._+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
)

// StartToolLogin 在工具工作区开一个登录终端：有登录子命令的跑子命令（可预填邮箱），
// 没有的直接启动工具，在它的首次启动界面里登录。上一次没收尾的登录终端先关掉。
func (u *Usecase) StartToolLogin(id, email string) (TerminalInfo, error) {
	p, ok := findPreset(id)
	if !ok || p.ID == "shell" {
		return TerminalInfo{}, apierror.BadRequest(domain, "unknown agent: "+id)
	}
	w, err := toolWorkspace()
	if err != nil {
		return TerminalInfo{}, err
	}
	spec := sessionSpec{WorkspaceID: w.ID, Agent: p.ID, Title: "登录 · " + p.Label, Cols: 400}
	if len(p.LoginArgs) > 0 {
		args := append([]string(nil), p.LoginArgs...)
		if email = strings.TrimSpace(email); email != "" && p.EmailFlag != "" {
			if !emailRe.MatchString(email) {
				return TerminalInfo{}, apierror.BadRequest(domain, "邮箱格式不对")
			}
			args = append(args, p.EmailFlag, email)
		}
		spec.Argv = rawArgv(p, args)
	}
	u.closeLogin(id)
	info, err := u.startTerminal(w, spec)
	if err != nil {
		return TerminalInfo{}, err
	}
	loginMu.Lock()
	loginJobs[id] = &loginJob{sid: info.ID}
	loginMu.Unlock()
	return info, nil
}

func (u *Usecase) closeLogin(id string) {
	loginMu.Lock()
	j := loginJobs[id]
	delete(loginJobs, id)
	loginMu.Unlock()
	if j != nil {
		_ = u.CloseTerminal(j.sid)
	}
}

// ToolLoginState 读登录终端的输出，取出链接 / 设备码 / 二维码；登录命令结束后查一次登录状态定结论。
func (u *Usecase) ToolLoginState(id string) LoginState {
	loginMu.Lock()
	j := loginJobs[id]
	loginMu.Unlock()
	if j == nil {
		return LoginState{}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.end != nil {
		return *j.end
	}
	u.mu.Lock()
	s := u.sessions[j.sid]
	u.mu.Unlock()
	if s == nil {
		return LoginState{TerminalID: j.sid, Done: true, Message: "登录终端已关闭"}
	}
	// 先看退出没有、再读输出：反过来的话，命令在两步之间打印完就退出，链接会丢
	s.mu.Lock()
	exited, code := s.exited, s.exitCode
	s.mu.Unlock()
	text := s.ReadText(400)
	p, _ := findPreset(id)
	// 没有登录命令的工具（DeepSeek Harness 的 Web 界面）要登录的正是本机地址
	st := LoginState{TerminalID: j.sid, URL: pickLoginURL(text, len(p.LoginArgs) == 0), Code: pickDeviceCode(text), QR: readQR(text)}
	if !exited {
		st.Running = true
		return st
	}
	st.Done = true
	if code != 0 {
		st.Message = lastLines(text, 3)
		j.end = &st
		return st
	}
	st.OK, st.Account, st.Message = u.checkLogin(p)
	if st.OK {
		u.recordReadiness(id, ReadyOK)
		set := u.GetAgentSetting(u.ctx, id)
		set.Account = st.Account
		_ = putDoc(u.ctx, u.store, kindAgentSetting, id, set)
	}
	j.end = &st
	return st
}

// refreshLoginStatus 用各家的登录状态命令刷新「已登录 / 未登录」（启动时与每小时一次）。
func (u *Usecase) refreshLoginStatus() {
	for _, p := range ListPresets() {
		if p.ID == "shell" || !p.Available || len(p.StatusArgs) == 0 || u.ctx.Err() != nil {
			continue
		}
		ok, account, _ := u.checkLogin(p)
		if !ok {
			u.recordReadiness(p.ID, ReadyNeedLogin)
			continue
		}
		u.recordReadiness(p.ID, ReadyOK)
		if set := u.GetAgentSetting(u.ctx, p.ID); account != "" && set.Account != account {
			set.Account = account
			_ = putDoc(u.ctx, u.store, kindAgentSetting, p.ID, set)
		}
	}
}

// checkLogin 用登录状态命令确认（没有状态命令就以登录命令正常结束为准），顺带取账号邮箱。
func (u *Usecase) checkLogin(p AgentPreset) (ok bool, account, msg string) {
	if len(p.StatusArgs) == 0 {
		return true, "", "登录完成"
	}
	ctx, cancel := context.WithTimeout(u.ctx, 30*time.Second)
	defer cancel()
	var buf strings.Builder
	err := runTool(ctx, p, p.StatusArgs, os.TempDir(), append(u.terminalEnv("toolstatus", ""), u.toolKeyEnv(p)...), &buf)
	out := StripANSI(buf.String())
	low := strings.ToLower(out)
	if err != nil || containsAny(low, "not logged in", "not authenticated", "logged out", "未登录", "\"loggedin\": false", "please log in", "please login", "login required") {
		return false, "", "登录没有生效：" + lastLines(out, 2)
	}
	return true, accountRe.FindString(out), "登录完成"
}

var (
	accountRe = regexp.MustCompile(`[A-Za-z0-9._+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+`)
	urlRe     = regexp.MustCompile(`https?://[^\s"'<>\x60]+`)
	codeRe    = regexp.MustCompile(`\b[A-Z0-9]{4,5}-[A-Z0-9]{4,5}\b`)
	qrPathRe  = regexp.MustCompile(`(?i)[a-z]:\\[^\r\n"<>|?*]+?\.png|/[^\s"'<>]+\.png`)
)

// pickLoginURL 在输出里挑登录页链接：带 oauth / login / device 的优先，本机回调地址（localhost）不要。
func pickLoginURL(text string, allowLocal bool) string {
	type cand struct {
		u     string
		score int
		i     int
	}
	var cs []cand
	for i, raw := range urlRe.FindAllString(text, -1) {
		u := strings.TrimRight(raw, ".,;:)]}")
		low := strings.ToLower(u)
		c := cand{u: u, i: i}
		if containsAny(low, "oauth", "authorize", "login", "device", "activate", "verify", "auth", "signin", "sign-in") {
			c.score += 2
		}
		if containsAny(low, "://localhost", "://127.0.0.1", "://[::1]") {
			c.score -= 5
		}
		cs = append(cs, c)
	}
	if len(cs) == 0 {
		return ""
	}
	sort.SliceStable(cs, func(a, b int) bool { return cs[a].score > cs[b].score })
	if cs[0].score < 0 && !allowLocal {
		return ""
	}
	return cs[0].u
}

// ToolEnvForACP 给走 ACP 接入的工具（DeepSeek Harness 的 dsh --profile acp）补上在这里存的 API Key；
// 编码工具登记里自己写了同名变量的以登记为准。
func (u *Usecase) ToolEnvForACP(command string, env map[string]string) map[string]string {
	for _, p := range ListPresets() {
		if p.Command != command || len(p.ACPArgs) == 0 {
			continue
		}
		out := map[string]string{}
		for _, kv := range u.toolKeyEnv(p) {
			if k, v, ok := strings.Cut(kv, "="); ok {
				out[k] = v
			}
		}
		for k, v := range env {
			out[k] = v
		}
		return out
	}
	return env
}

// pickDeviceCode 取设备码（形如 ABCD-EFGH）；输出里没提到 code / 代码 时不认，免得把别的编号当成设备码。
func pickDeviceCode(text string) string {
	if !containsAny(strings.ToLower(text), "code", "代码", "验证码") {
		return ""
	}
	return codeRe.FindString(text)
}

// readQR 读工具输出里提到的二维码图片（豆包 login --qr=png）。只认系统临时目录下、1MB 以内的 png。
func readQR(text string) string {
	tmp, err := filepath.Abs(os.TempDir())
	if err != nil {
		return ""
	}
	for _, path := range qrPathRe.FindAllString(text, -1) {
		abs, err := filepath.Abs(strings.TrimSpace(path))
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(tmp, abs); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		fi, err := os.Stat(abs)
		if err != nil || fi.Size() > 1<<20 {
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	}
	return ""
}

// rawArgv 原样拼启动参数（不加档位参数）；Windows 上 .cmd 垫片经 cmd.exe。
func rawArgv(p AgentPreset, args []string) []string {
	argv := append([]string{p.ResolvedPath}, args...)
	lower := strings.ToLower(p.ResolvedPath)
	if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
		return append([]string{comspec(), "/c"}, argv...)
	}
	return argv
}
