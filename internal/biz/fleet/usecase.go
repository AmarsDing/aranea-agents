package fleet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

const domain = "FLEET"

// Options 是运行台的部署参数。
type Options struct {
	// WorktreeRoot 是新 worktree 的落盘根目录；空 = ~/.aranea/worktrees。
	WorktreeRoot string
	// ServerURL 是 agent 终端里 fleetctl 回连的地址，如 http://127.0.0.1:8810。
	ServerURL string
	// CLIDir 是 fleetctl 可执行文件所在目录，会前置进终端 PATH；空 = 本进程所在目录。
	CLIDir string
}

// Usecase 是运行台的全部业务。
type Usecase struct {
	store    Store
	projects ProjectSource
	opts     Options
	lg       loggateway.Logger

	mu       sync.Mutex
	sessions map[string]*Session
	order    []string
	byToken  map[string]*Session
	wake     map[string]bool // 等待唤醒的协调者终端
	evMu     sync.Mutex

	usageMu    sync.Mutex
	usageCache []AgentUsage
	usageAt    time.Time

	ctx    context.Context
	cancel context.CancelFunc
}

// NewUsecase 装配运行台，并启动状态判定与自动化两个后台循环。
func NewUsecase(store Store, projects ProjectSource, opts Options, lg loggateway.Logger) *Usecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if opts.WorktreeRoot == "" {
		if home, err := os.UserHomeDir(); err == nil {
			opts.WorktreeRoot = filepath.Join(home, ".aranea", "worktrees")
		}
	}
	if opts.CLIDir == "" {
		if exe, err := os.Executable(); err == nil {
			opts.CLIDir = filepath.Dir(exe)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	u := &Usecase{
		store: store, projects: projects, opts: opts, lg: lg.With(loggateway.Str("module", "fleet")),
		sessions: map[string]*Session{}, byToken: map[string]*Session{}, wake: map[string]bool{},
		ctx: ctx, cancel: cancel,
	}
	safego.Go(ctx, "fleet-status-loop", u.statusLoop)
	safego.Go(ctx, "fleet-automation-loop", u.automationLoop)
	return u
}

// Close 停后台循环并结束全部终端。
func (u *Usecase) Close() {
	u.cancel()
	u.mu.Lock()
	all := make([]*Session, 0, len(u.sessions))
	for _, s := range u.sessions {
		all = append(all, s)
	}
	u.mu.Unlock()
	for _, s := range all {
		s.Kill()
	}
}

func (u *Usecase) statusLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case now := <-t.C:
			u.mu.Lock()
			all := make([]*Session, 0, len(u.sessions))
			for _, s := range u.sessions {
				all = append(all, s)
			}
			u.mu.Unlock()
			for _, s := range all {
				s.tick(now)
			}
		}
	}
}

// ---------- agent 与项目 ----------

// ListPresets 返回 agent 清单。
func (u *Usecase) ListPresets() []AgentPreset { return ListPresets() }

// ListProjects 返回登记的项目（附默认分支）。
func (u *Usecase) ListProjects(ctx context.Context) ([]Project, error) {
	if u.projects == nil {
		return nil, nil
	}
	items, err := u.projects.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].IsGit = isGitRepo(ctx, items[i].Path)
		if items[i].IsGit {
			items[i].DefaultBranch = defaultBranch(ctx, items[i].Path)
		}
	}
	return items, nil
}

// AddProject 登记一个本机仓库。
func (u *Usecase) AddProject(ctx context.Context, path, name string) (Project, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Project{}, apierror.BadRequest(domain, "path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Project{}, apierror.BadRequest(domain, "invalid path")
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return Project{}, apierror.BadRequest(domain, "目录不存在："+abs)
	}
	if !isGitRepo(ctx, abs) {
		return Project{}, apierror.BadRequest(domain, "不是 git 仓库："+abs)
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(abs)
	}
	if u.projects == nil {
		return Project{}, apierror.Internal(domain, "project registry not wired")
	}
	p, err := u.projects.AddProject(ctx, name, abs)
	if err != nil {
		return Project{}, err
	}
	p.IsGit, p.DefaultBranch = true, defaultBranch(ctx, abs)
	return p, nil
}

func (u *Usecase) project(ctx context.Context, id string) (Project, error) {
	items, err := u.ListProjects(ctx)
	if err != nil {
		return Project{}, err
	}
	for _, p := range items {
		if p.ID == id {
			return p, nil
		}
	}
	return Project{}, apierror.NotFound(domain, "project not found: "+id)
}

// ListRefs 返回项目的本地/远端分支。
func (u *Usecase) ListRefs(ctx context.Context, projectID string) (local, remote []string, def string, err error) {
	p, err := u.project(ctx, projectID)
	if err != nil {
		return nil, nil, "", err
	}
	local, remote, err = listRefs(ctx, p.Path)
	return local, remote, p.DefaultBranch, err
}

// ---------- worktree ----------

func primaryID(projectID string) string { return "primary-" + projectID }

// ListWorkspaces 返回 worktree（含各项目主检出）。
func (u *Usecase) ListWorkspaces(ctx context.Context, projectID string, includeArchived bool) ([]Workspace, error) {
	projects, err := u.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	stored, err := listDocs[Workspace](ctx, u.store, kindWorkspace)
	if err != nil {
		return nil, err
	}
	byID := map[string]Workspace{}
	for _, w := range stored {
		byID[w.ID] = w
	}
	known := map[string]bool{}
	var out []Workspace
	for _, p := range projects {
		known[p.ID] = true
		if projectID != "" && p.ID != projectID {
			continue
		}
		pw, ok := byID[primaryID(p.ID)]
		if !ok {
			pw = Workspace{ID: primaryID(p.ID), ProjectID: p.ID, Name: p.Name, Kind: KindPrimary, CreatedAt: "0"}
		}
		pw.Path, pw.Status, pw.BaseRef = p.Path, WorkspaceReady, p.DefaultBranch
		pw.Branch = currentBranch(ctx, p.Path)
		out = append(out, pw)
	}
	for _, w := range stored {
		if w.Kind == KindPrimary || !known[w.ProjectID] || (projectID != "" && w.ProjectID != projectID) {
			continue
		}
		if w.Archived && !includeArchived {
			continue
		}
		out = append(out, w)
	}
	u.mu.Lock()
	for i := range out {
		for _, s := range u.sessions {
			if s.spec.WorkspaceID == out[i].ID {
				if last := s.Info().LastActivityAt; !last.IsZero() {
					if ts := last.UTC().Format(time.RFC3339); ts > out[i].LastActiveAt {
						out[i].LastActiveAt = ts
					}
				}
			}
		}
	}
	u.mu.Unlock()
	sortByCreated(out, func(w Workspace) string { return w.CreatedAt })
	return out, nil
}

func currentBranch(ctx context.Context, dir string) string {
	out, err := runGit(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (u *Usecase) workspace(ctx context.Context, id string) (Workspace, error) {
	if strings.HasPrefix(id, "primary-") {
		items, err := u.ListWorkspaces(ctx, strings.TrimPrefix(id, "primary-"), true)
		if err != nil {
			return Workspace{}, err
		}
		for _, w := range items {
			if w.ID == id {
				return w, nil
			}
		}
		return Workspace{}, apierror.NotFound(domain, "workspace not found: "+id)
	}
	w, ok, err := getDoc[Workspace](ctx, u.store, kindWorkspace, id)
	if err != nil {
		return Workspace{}, err
	}
	if !ok {
		return Workspace{}, apierror.NotFound(domain, "workspace not found: "+id)
	}
	return w, nil
}

// CreateWorkspaceInput 是建 worktree 的参数。
type CreateWorkspaceInput struct {
	ProjectID  string
	Name       string
	Branch     string
	StartFrom  string
	Agent      string
	Prompt     string
	FullAccess bool
	ParentID   string
	Fetch      bool
	Origin     string
	// 编排用：worker 终端的身份。
	RunID, TaskID, Role string
	// onReady 在 worktree 就绪且终端已起后回调（编排记 worker 终端 id）。
	onReady func(Workspace, TerminalInfo)
}

// CreateWorkspace 立即返回 creating 状态的记录，git 操作在后台完成，
// 就绪后（若指定了 agent）自动起终端并发首句提示。
func (u *Usecase) CreateWorkspace(ctx context.Context, in CreateWorkspaceInput) (Workspace, error) {
	p, err := u.project(ctx, in.ProjectID)
	if err != nil {
		return Workspace{}, err
	}
	if !p.IsGit {
		return Workspace{}, apierror.BadRequest(domain, "项目不是 git 仓库")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "task-" + time.Now().Format("0102-150405")
	}
	slug := slugify(name)
	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		branch = slug
	}
	branch = u.uniqueBranch(ctx, p.Path, branch, in.Branch != "")
	start := strings.TrimSpace(in.StartFrom)
	if start == "" {
		start = p.DefaultBranch
	}
	path := u.uniquePath(filepath.Join(u.opts.WorktreeRoot, slugify(p.Name), strings.ReplaceAll(slugify(branch), "/", "-")))
	now := nowString()
	origin := in.Origin
	if origin == "" {
		origin = "ui"
	}
	w := Workspace{
		ID: uuid.NewString(), ProjectID: p.ID, Name: name, Branch: branch, BaseRef: start, Path: path,
		Kind: KindWorktree, Status: WorkspaceCreating, ParentID: in.ParentID, Origin: origin,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := putDoc(ctx, u.store, kindWorkspace, w.ID, w); err != nil {
		return Workspace{}, err
	}
	safego.Go(u.ctx, "fleet-create-worktree", func() { u.finishCreate(p, w, in) })
	return w, nil
}

func (u *Usecase) uniqueBranch(ctx context.Context, repo, branch string, explicit bool) string {
	if explicit || !branchExists(ctx, repo, branch) {
		return branch
	}
	for i := 2; i < 100; i++ {
		cand := fmt.Sprintf("%s-%d", branch, i)
		if !branchExists(ctx, repo, cand) {
			return cand
		}
	}
	return branch + "-" + time.Now().Format("150405")
}

func (u *Usecase) uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	for i := 2; i < 100; i++ {
		cand := fmt.Sprintf("%s-%d", path, i)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return path + "-" + time.Now().Format("150405")
}

func (u *Usecase) finishCreate(p Project, w Workspace, in CreateWorkspaceInput) {
	ctx := u.ctx
	if in.Fetch {
		_, _ = runGit(ctx, p.Path, "fetch", "--prune")
	}
	err := addWorktree(ctx, p.Path, w.Path, w.Branch, w.BaseRef)
	w.UpdatedAt = nowString()
	if err != nil {
		w.Status, w.Error = WorkspaceFailed, err.Error()
		_ = putDoc(ctx, u.store, kindWorkspace, w.ID, w)
		u.addEvent(Event{Kind: EventWorktreeFailed, Title: "worktree 创建失败：" + w.Name, Body: err.Error(), WorkspaceID: w.ID})
		u.lg.Warn("fleet worktree create failed", loggateway.Str("workspace", w.ID), loggateway.Err(err))
		return
	}
	copied := copyWorktreeIncludes(ctx, p.Path, w.Path)
	w.Status = WorkspaceReady
	_ = putDoc(ctx, u.store, kindWorkspace, w.ID, w)
	body := w.Branch + " ← " + w.BaseRef
	if len(copied) > 0 {
		body += "；已复制 " + strings.Join(copied, ", ")
	}
	u.addEvent(Event{Kind: EventWorktreeReady, Title: "worktree 已就绪：" + w.Name, Body: body, WorkspaceID: w.ID})
	if in.Agent == "" {
		return
	}
	info, err := u.startTerminal(w, sessionSpec{
		WorkspaceID: w.ID, Agent: in.Agent, Prompt: in.Prompt, FullAccess: in.FullAccess,
		RunID: in.RunID, TaskID: in.TaskID, Role: in.Role,
	})
	if err != nil {
		u.addEvent(Event{Kind: EventExited, Title: "agent 启动失败：" + w.Name, Body: err.Error(), WorkspaceID: w.ID})
		return
	}
	if in.onReady != nil {
		in.onReady(w, info)
	}
}

// WorkspacePatch 是可改的 worktree 字段。
type WorkspacePatch struct {
	Name        *string
	Pinned      *bool
	Archived    *bool
	LinkedIssue *string
}

// UpdateWorkspace 改名、置顶、归档、关联 issue。
func (u *Usecase) UpdateWorkspace(ctx context.Context, id string, patch WorkspacePatch) (Workspace, error) {
	w, err := u.workspace(ctx, id)
	if err != nil {
		return Workspace{}, err
	}
	if patch.Name != nil && strings.TrimSpace(*patch.Name) != "" {
		w.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Pinned != nil {
		w.Pinned = *patch.Pinned
	}
	if patch.Archived != nil && w.Kind != KindPrimary {
		w.Archived = *patch.Archived
	}
	if patch.LinkedIssue != nil {
		w.LinkedIssue = strings.TrimSpace(*patch.LinkedIssue)
	}
	w.UpdatedAt = nowString()
	return w, putDoc(ctx, u.store, kindWorkspace, w.ID, w)
}

// DeleteWorkspace 关掉其中的终端、删 worktree 目录和分支。分支有未合并提交时
// （git branch -d 拒绝）保留分支并如实告知，除非 forceBranch。
func (u *Usecase) DeleteWorkspace(ctx context.Context, id string, forceBranch bool) (bool, string, error) {
	w, err := u.workspace(ctx, id)
	if err != nil {
		return false, "", err
	}
	if w.Kind == KindPrimary {
		return false, "", apierror.BadRequest(domain, "项目主检出不能删除")
	}
	for _, t := range u.ListTerminals(w.ID) {
		_ = u.CloseTerminal(t.ID)
	}
	p, perr := u.project(ctx, w.ProjectID)
	kept, msg := false, "已删除 worktree 与分支 "+w.Branch
	if perr == nil && w.Status != WorkspaceFailed {
		if err := removeWorktree(ctx, p.Path, w.Path); err != nil {
			return false, "", apierror.FailedPrecondition(domain, "删除 worktree 失败："+err.Error())
		}
		if err := deleteBranch(ctx, p.Path, w.Branch, forceBranch); err != nil {
			kept = true
			msg = "worktree 已删除；分支 " + w.Branch + " 有未合并的提交，已保留（" + err.Error() + "）"
		}
	} else if perr != nil {
		msg = "项目已不存在，只清理了运行台记录"
	}
	if err := u.store.Delete(ctx, kindWorkspace, w.ID); err != nil {
		return kept, msg, err
	}
	return kept, msg, nil
}

// ---------- 终端 ----------

// CreateTerminalInput 是新开终端的参数。
type CreateTerminalInput struct {
	WorkspaceID string
	Agent       string
	Prompt      string
	FullAccess  bool
	Title       string
}

// CreateTerminal 在 worktree 里起一个 agent 或 shell。
func (u *Usecase) CreateTerminal(ctx context.Context, in CreateTerminalInput) (TerminalInfo, error) {
	w, err := u.workspace(ctx, in.WorkspaceID)
	if err != nil {
		return TerminalInfo{}, err
	}
	if w.Status != WorkspaceReady {
		return TerminalInfo{}, apierror.FailedPrecondition(domain, "worktree 还没就绪")
	}
	return u.startTerminal(w, sessionSpec{WorkspaceID: w.ID, Agent: in.Agent, Prompt: in.Prompt, FullAccess: in.FullAccess, Title: in.Title})
}

func (u *Usecase) startTerminal(w Workspace, spec sessionSpec) (TerminalInfo, error) {
	if spec.Agent == "" {
		spec.Agent = "shell"
	}
	preset, ok := findPreset(spec.Agent)
	if !ok {
		return TerminalInfo{}, apierror.BadRequest(domain, "unknown agent: "+spec.Agent)
	}
	if !preset.Available {
		return TerminalInfo{}, apierror.FailedPrecondition(domain, preset.Label+" 未安装（PATH 上找不到 "+preset.Command+"）")
	}
	spec.Dir = w.Path
	id := uuid.NewString()
	token := randomToken()
	spec.Env = u.terminalEnv(id, token)
	s := newSession(id, token, spec, u.onStatus)
	if err := s.start(launchArgv(preset, spec.FullAccess)); err != nil {
		return TerminalInfo{}, apierror.Internal(domain, "启动失败："+err.Error())
	}
	u.mu.Lock()
	u.sessions[id] = s
	u.byToken[token] = s
	u.order = append(u.order, id)
	u.mu.Unlock()
	return s.Info(), nil
}

// backendEnvPrefixes 是后端自己读的环境变量命名空间，整段不传给运行台终端：
// agent 命令行由模型驱动，不该拿到平台自己的凭据（会话签名密钥、对外门面令牌、
// 数据库口令、对象存储 / 语音 / 向量服务的密钥）。
//
// 按「本仓库自创的前缀」剔除，而不是按 *_KEY / *_TOKEN 后缀：后缀会误伤用户自己的
// ANTHROPIC_API_KEY、OPENAI_API_KEY、GITHUB_TOKEN（agent 命令行靠它们干活）；前缀则把
// 以后新加的 ARANEA_XXX_SECRET 也一并盖住。新增后端密钥变量请用这些前缀，否则补到这里。
//
// shortcut: 厂商标准名（OPENAI / GOOGLE / DEEPSEEK / XAI / TAVILY / SERPAPI / E2B / COHERE 的
// *_API_KEY）后端也读，但从名字分不出是给后端配的还是用户自己的，照常保留；
// 要隔离就得让后端只从配置文件取这些 key。
var backendEnvPrefixes = []string{
	"KRATOS_",    // KRATOS_AUTH_SECRET、KRATOS_ARTIFACT_SIGN_KEY、KRATOS_KNOWLEDGE_EMBED_API_KEY
	"ARANEA_",    // ARANEA_TWINOPENAPI_TOKEN / _CALLERS、ARANEA_PG_DSN / _PASSWORD、ARANEA_TOKEN；ARANEA_FLEET_* 由 terminalEnv 重新注入
	"TWIN_",      // TWIN_API_KEY
	"SPEECH_",    // SPEECH_ASR_* / SPEECH_TTS_* 的 API_KEY、ACCESS_KEY、APP_KEY
	"ARTIFACT_",  // ARTIFACT_S3_* / ARTIFACT_COS_* 的密钥对
	"EMBEDDING_", // EMBEDDING_API_KEY
	"INFINITY_",  // INFINITY_RERANK_API_KEY
	"EVAL_",      // EVAL_MEMORY_TOKEN、EVAL_MEDIA_VLM_API_KEY
}

// isBackendEnv 判断变量名（已转大写）是否属于后端自用。
func isBackendEnv(k string) bool {
	// PGPASSWORD 没有专属前缀，但 launcher 把它当平台库口令读，并原样带进后端环境。
	if k == "PGPASSWORD" {
		return true
	}
	for _, p := range backendEnvPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

func (u *Usecase) terminalEnv(id, token string) []string {
	env := make([]string, 0, len(os.Environ())+6)
	sep := string(os.PathListSeparator)
	for _, kv := range os.Environ() {
		k := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		if k == "TERM" || k == "COLORTERM" || isBackendEnv(k) {
			continue
		}
		if k == "PATH" && u.opts.CLIDir != "" {
			kv = kv[:5] + u.opts.CLIDir + sep + kv[5:]
		}
		env = append(env, kv)
	}
	return append(env,
		"TERM=xterm-256color", "COLORTERM=truecolor",
		"ARANEA_FLEET_URL="+strings.TrimRight(u.opts.ServerURL, "/")+"/v1/fleet/cli",
		"ARANEA_FLEET_TOKEN="+token, "ARANEA_FLEET_TERMINAL="+id,
	)
}

func randomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Session 返回运行中的终端（WebSocket 接入用）。
func (u *Usecase) Session(id string) (*Session, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	s, ok := u.sessions[id]
	return s, ok
}

func (u *Usecase) sessionByToken(token string) (*Session, bool) {
	if token == "" {
		return nil, false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	s, ok := u.byToken[token]
	return s, ok
}

// ListTerminals 返回终端快照；workspaceID 为空返回全部。
func (u *Usecase) ListTerminals(workspaceID string) []TerminalInfo {
	u.mu.Lock()
	ids := append([]string(nil), u.order...)
	sessions := make([]*Session, 0, len(ids))
	for _, id := range ids {
		if s, ok := u.sessions[id]; ok {
			sessions = append(sessions, s)
		}
	}
	u.mu.Unlock()
	out := make([]TerminalInfo, 0, len(sessions))
	for _, s := range sessions {
		info := s.Info()
		if workspaceID == "" || info.WorkspaceID == workspaceID {
			out = append(out, info)
		}
	}
	return out
}

// KillTerminal 结束进程，保留标签（可重启）。
func (u *Usecase) KillTerminal(id string) (TerminalInfo, error) {
	s, ok := u.Session(id)
	if !ok {
		return TerminalInfo{}, apierror.NotFound(domain, "terminal not found")
	}
	s.Kill()
	time.Sleep(200 * time.Millisecond)
	return s.Info(), nil
}

// RestartTerminal 用同样的 agent、目录重起（不重发首句提示）。
func (u *Usecase) RestartTerminal(id string) (TerminalInfo, error) {
	s, ok := u.Session(id)
	if !ok {
		return TerminalInfo{}, apierror.NotFound(domain, "terminal not found")
	}
	s.Kill()
	preset, ok := findPreset(s.spec.Agent)
	if !ok || !preset.Available {
		return TerminalInfo{}, apierror.FailedPrecondition(domain, "agent 不可用")
	}
	s.spec.Prompt = ""
	s.mu.Lock()
	s.buf = append(s.buf, []byte("\r\n\x1b[2m── 已重启 ──\x1b[0m\r\n")...)
	s.mu.Unlock()
	if err := s.start(launchArgv(preset, s.spec.FullAccess)); err != nil {
		return TerminalInfo{}, apierror.Internal(domain, "重启失败："+err.Error())
	}
	return s.Info(), nil
}

// CloseTerminal 结束进程并移除标签。
func (u *Usecase) CloseTerminal(id string) error {
	u.mu.Lock()
	s, ok := u.sessions[id]
	if ok {
		delete(u.sessions, id)
		delete(u.byToken, s.token)
		delete(u.wake, id)
		for i, oid := range u.order {
			if oid == id {
				u.order = append(u.order[:i], u.order[i+1:]...)
				break
			}
		}
	}
	u.mu.Unlock()
	if !ok {
		return apierror.NotFound(domain, "terminal not found")
	}
	s.Kill()
	return nil
}

// SendInput 往终端里送一段文本；submit 时末尾补回车。
func (u *Usecase) SendInput(id, text string, submit bool) error {
	s, ok := u.Session(id)
	if !ok {
		return apierror.NotFound(domain, "terminal not found")
	}
	return s.Paste(text, submit)
}

// onStatus 是所有终端的状态变化回调：发通知、唤醒协调者。
func (u *Usecase) onStatus(s *Session, old, next string) {
	info := s.Info()
	label := agentLabel(info.Agent)
	ws := info.WorkspaceID
	wsName := u.workspaceName(ws)
	switch next {
	case StatusDone:
		if old == StatusWorking {
			u.addEvent(Event{Kind: EventAgentDone, Title: label + " 完成了一轮：" + wsName, Body: info.Preview, WorkspaceID: ws, TerminalID: info.ID, RunID: info.RunID})
		}
	case StatusNeedsInput:
		u.addEvent(Event{Kind: EventNeedsInput, Title: label + " 在等你：" + wsName, Body: info.Preview, WorkspaceID: ws, TerminalID: info.ID, RunID: info.RunID})
	case StatusExited, StatusFailed:
		s.mu.Lock()
		killed := s.killed
		s.mu.Unlock()
		if !killed && info.Agent != "shell" { // 自己关的、普通终端退出都不算新闻
			title := label + " 已退出：" + wsName
			if next == StatusFailed {
				title = fmt.Sprintf("%s 异常退出（%d）：%s", label, info.ExitCode, wsName)
			}
			u.addEvent(Event{Kind: EventExited, Title: title, Body: info.Preview, WorkspaceID: ws, TerminalID: info.ID, RunID: info.RunID})
		}
		u.onWorkerExit(info)
	}
	if next == StatusDone || next == StatusIdle {
		u.maybeWake(info.ID)
	}
}

func (u *Usecase) workspaceName(id string) string {
	if w, err := u.workspace(u.ctx, id); err == nil {
		return w.Name
	}
	return id
}

func agentLabel(id string) string {
	for _, p := range builtinPresets {
		if p.ID == id {
			return p.Label
		}
	}
	return id
}

// ---------- diff 与批注 ----------

// Diff 返回 worktree 相对起点（或指定 base）的改动。
func (u *Usecase) Diff(ctx context.Context, workspaceID, base string) (DiffResult, error) {
	w, err := u.workspace(ctx, workspaceID)
	if err != nil {
		return DiffResult{}, err
	}
	if base == "" {
		base = w.BaseRef
	}
	if w.Kind == KindPrimary && base == w.Branch {
		base = "HEAD"
	}
	return gitDiff(ctx, w.Path, base)
}

// ListNotes 返回 worktree 上的批注。
func (u *Usecase) ListNotes(ctx context.Context, workspaceID string) ([]Note, error) {
	all, err := listDocs[Note](ctx, u.store, kindNote)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, n := range all {
		if n.WorkspaceID == workspaceID {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

// UpsertNote 新建或修改批注。
func (u *Usecase) UpsertNote(ctx context.Context, n Note) (Note, error) {
	if strings.TrimSpace(n.WorkspaceID) == "" || strings.TrimSpace(n.Path) == "" || n.Line <= 0 {
		return Note{}, apierror.BadRequest(domain, "workspace_id, path and line are required")
	}
	if n.ID == "" {
		n.ID, n.CreatedAt = uuid.NewString(), nowString()
	} else if old, ok, _ := getDoc[Note](ctx, u.store, kindNote, n.ID); ok {
		n.CreatedAt = old.CreatedAt
		if n.Body != old.Body {
			n.SentAt = ""
		}
	}
	if n.Side == "" {
		n.Side = "new"
	}
	return n, putDoc(ctx, u.store, kindNote, n.ID, n)
}

// DeleteNote 删除批注。
func (u *Usecase) DeleteNote(ctx context.Context, id string) error {
	return u.store.Delete(ctx, kindNote, id)
}

// SendNotes 把未解决的批注拼成一段提示，整批发给指定 agent 终端（或新起一个）。
func (u *Usecase) SendNotes(ctx context.Context, workspaceID, terminalID, agent string, fullAccess bool) (string, int, error) {
	notes, err := u.ListNotes(ctx, workspaceID)
	if err != nil {
		return "", 0, err
	}
	var pending []Note
	for _, n := range notes {
		if !n.Resolved {
			pending = append(pending, n)
		}
	}
	if len(pending) == 0 {
		return "", 0, apierror.BadRequest(domain, "没有待发送的批注")
	}
	prompt := composeReviewPrompt(pending)
	if terminalID == "" {
		w, err := u.workspace(ctx, workspaceID)
		if err != nil {
			return "", 0, err
		}
		if agent == "" {
			agent = "claude"
		}
		info, err := u.startTerminal(w, sessionSpec{WorkspaceID: w.ID, Agent: agent, Prompt: prompt, FullAccess: fullAccess})
		if err != nil {
			return "", 0, err
		}
		terminalID = info.ID
	} else if err := u.SendInput(terminalID, prompt, true); err != nil {
		return "", 0, err
	}
	now := nowString()
	for _, n := range pending {
		n.SentAt = now
		_ = putDoc(ctx, u.store, kindNote, n.ID, n)
	}
	return terminalID, len(pending), nil
}

func composeReviewPrompt(notes []Note) string {
	var b strings.Builder
	b.WriteString("我审阅了这个 worktree 的改动，下面是逐行意见。请按意见一次性修改，改完简要说明每条怎么处理的：\n")
	for i, n := range notes {
		fmt.Fprintf(&b, "\n%d. %s 第 %d 行", i+1, n.Path, n.Line)
		if n.Side == "old" {
			b.WriteString("（删除侧）")
		}
		if t := strings.TrimSpace(n.LineText); t != "" {
			if r := []rune(t); len(r) > 120 {
				t = string(r[:120]) + "…"
			}
			fmt.Fprintf(&b, "：`%s`", t)
		}
		b.WriteString("\n   " + strings.ReplaceAll(strings.TrimSpace(n.Body), "\n", "\n   "))
	}
	return b.String()
}

// ---------- 源码管理 ----------

// GitStatus 返回 worktree 的分支与改动文件。
func (u *Usecase) GitStatus(ctx context.Context, workspaceID string) (GitStatus, error) {
	w, err := u.workspace(ctx, workspaceID)
	if err != nil {
		return GitStatus{}, err
	}
	st, err := gitStatus(ctx, w.Path)
	if err != nil {
		return GitStatus{}, err
	}
	st.BaseRef = w.BaseRef
	if w.Kind == KindWorktree {
		st.Additions, st.Deletions = diffTotals(ctx, w.Path, mergeBase(ctx, w.Path, w.BaseRef))
	}
	return st, nil
}

func cleanPaths(paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) || strings.Contains(filepath.ToSlash(p), "../") || strings.HasPrefix(p, "-") {
			return nil, apierror.BadRequest(domain, "invalid path: "+p)
		}
		out = append(out, p)
	}
	return out, nil
}

// Stage 暂存文件；paths 为空 = 全部。
func (u *Usecase) Stage(ctx context.Context, workspaceID string, paths []string) (string, error) {
	w, err := u.workspace(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	ps, err := cleanPaths(paths)
	if err != nil {
		return "", err
	}
	args := []string{"add", "-A", "--"}
	if len(ps) > 0 {
		args = append(args, ps...)
	}
	return runGitCombined(ctx, w.Path, args...)
}

// Unstage 取消暂存；paths 为空 = 全部。
func (u *Usecase) Unstage(ctx context.Context, workspaceID string, paths []string) (string, error) {
	w, err := u.workspace(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	ps, err := cleanPaths(paths)
	if err != nil {
		return "", err
	}
	args := append([]string{"reset", "-q", "HEAD", "--"}, ps...)
	return runGitCombined(ctx, w.Path, args...)
}

// Commit 提交已暂存的改动；钩子照常执行，失败时原样返回输出。
func (u *Usecase) Commit(ctx context.Context, workspaceID, message string, amend bool) (string, error) {
	w, err := u.workspace(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" && !amend {
		return "", apierror.BadRequest(domain, "提交说明不能为空")
	}
	args := []string{"commit"}
	if amend {
		args = append(args, "--amend")
		if strings.TrimSpace(message) == "" {
			args = append(args, "--no-edit")
		}
	}
	if strings.TrimSpace(message) != "" {
		args = append(args, "-m", message)
	}
	return runGitCombined(ctx, w.Path, args...)
}

// Push 推送当前分支到 origin（首次设上游）；从不强推。
func (u *Usecase) Push(ctx context.Context, workspaceID string) (string, error) {
	w, err := u.workspace(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	return runGitCombined(ctx, w.Path, "push", "-u", "origin", "HEAD")
}

// ---------- 动态与通知 ----------

const maxEvents = 400

func (u *Usecase) addEvent(e Event) {
	u.evMu.Lock()
	defer u.evMu.Unlock()
	e.ID = fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.NewString()[:8])
	e.CreatedAt = nowString()
	ctx := u.ctx
	if err := putDoc(ctx, u.store, kindEvent, e.ID, e); err != nil {
		u.lg.Warn("fleet event persist failed", loggateway.Err(err))
		return
	}
	all, err := listDocs[Event](ctx, u.store, kindEvent)
	if err != nil || len(all) <= maxEvents {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	for _, old := range all[:len(all)-maxEvents] {
		_ = u.store.Delete(ctx, kindEvent, old.ID)
	}
}

// ListEvents 返回最近的动态（新的在前）与未读数。
func (u *Usecase) ListEvents(ctx context.Context, limit int, unreadOnly bool) ([]Event, int, error) {
	all, err := listDocs[Event](ctx, u.store, kindEvent)
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	unread := 0
	var out []Event
	for _, e := range all {
		if !e.Read {
			unread++
		}
		if unreadOnly && e.Read {
			continue
		}
		if limit <= 0 || len(out) < limit {
			out = append(out, e)
		}
	}
	return out, unread, nil
}

// MarkEventsRead 标已读。
func (u *Usecase) MarkEventsRead(ctx context.Context, ids []string, all bool) error {
	u.evMu.Lock()
	defer u.evMu.Unlock()
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	items, err := listDocs[Event](ctx, u.store, kindEvent)
	if err != nil {
		return err
	}
	for _, e := range items {
		if !e.Read && (all || want[e.ID]) {
			e.Read = true
			_ = putDoc(ctx, u.store, kindEvent, e.ID, e)
		}
	}
	return nil
}

// fleetctlName 是 agent 终端里的命令名。
func fleetctlName() string {
	if runtime.GOOS == "windows" {
		return "fleetctl.exe"
	}
	return "fleetctl"
}
