package fleet

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// 编排状态。
const (
	RunRunning = "running"
	RunDone    = "done"
	RunStopped = "stopped"

	TaskPending    = "pending"
	TaskReady      = "ready"
	TaskDispatched = "dispatched"
	TaskCompleted  = "completed"
	TaskFailed     = "failed"
)

func shortID(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
}

// RunDetail 是一次编排的全貌。
type RunDetail struct {
	Run      Run
	Tasks    []Task
	Messages []Message
}

// RunSummary 是列表里的一行。
type RunSummary struct {
	Run
	TaskCount, DoneCount, Unread int
}

// CreateRunInput 是新建编排的参数。
type CreateRunInput struct {
	ProjectID   string
	WorkspaceID string
	Objective   string
	Agent       string
	FullAccess  bool
}

// CreateRun 起一个协调者 agent，把目标和 fleetctl 用法作为首句提示交给它。
func (u *Usecase) CreateRun(ctx context.Context, in CreateRunInput) (Run, error) {
	if strings.TrimSpace(in.Objective) == "" {
		return Run{}, apierror.BadRequest(domain, "目标不能为空")
	}
	wsID := in.WorkspaceID
	if wsID == "" {
		wsID = primaryID(in.ProjectID)
	}
	w, err := u.workspace(ctx, wsID)
	if err != nil {
		return Run{}, err
	}
	if in.Agent == "" {
		in.Agent = "claude"
	}
	now := nowString()
	run := Run{
		ID: shortID("run_"), Objective: strings.TrimSpace(in.Objective), ProjectID: w.ProjectID, WorkspaceID: w.ID,
		Agent: in.Agent, FullAccess: in.FullAccess, Status: RunRunning, CreatedAt: now, UpdatedAt: now,
	}
	info, err := u.startTerminal(w, sessionSpec{
		WorkspaceID: w.ID, Agent: in.Agent, FullAccess: in.FullAccess, Title: "协调者 · " + truncate(run.Objective, 24),
		Prompt: coordinatorPreamble(run), RunID: run.ID, Role: "coordinator",
	})
	if err != nil {
		return Run{}, err
	}
	run.CoordinatorTerminalID = info.ID
	return run, putDoc(ctx, u.store, kindRun, run.ID, run)
}

func coordinatorPreamble(run Run) string {
	var agents []string
	for _, p := range ListPresets() {
		if p.Available && p.ID != "shell" {
			agents = append(agents, p.ID)
		}
	}
	return fmt.Sprintf(`你是本次编排的协调者（编排 %s）。目标：
%s

你可以用命令行工具 fleetctl 指挥其他编码 agent（worker）。每个 worker 在自己的 git worktree 里工作，互不干扰：
  fleetctl task-create --title "<短标题>" --spec "<完整需求与验收标准>" [--deps <任务id,任务id>]   返回任务 id
  fleetctl worker-start --task <任务id> [--agent <%s>] [--worktree new|current] [--name <worktree 名>]
  fleetctl status                                 查看全部任务与 worker 状态
  fleetctl check                                  读取收件箱：worker 的完成报告与提问
  fleetctl worker-read --task <任务id> [--lines 60]   看 worker 终端最近的输出
  fleetctl send --task <任务id> --body "<补充说明>"     给 worker 追加指示
  fleetctl answer --message <消息id> --body "<答复>"    回答 worker 的提问
  fleetctl worker-stop --task <任务id>
  fleetctl done --summary "<结果总结>"                  全部完成后收尾
工作方式：先拆分任务并启动 worker，然后结束本轮回复等待。worker 汇报或提问时，运行台会在你的终端里输入提示，届时运行 fleetctl check 处理；不要自己循环轮询。全部任务完成后，汇总结果、运行 fleetctl done，并告诉我各 worktree 的分支名以便我审阅合并。`,
		run.ID, run.Objective, strings.Join(agents, "|"))
}

func workerPreamble(t Task, branch string) string {
	return fmt.Sprintf(`你是编排中的 worker，负责任务 %s「%s」。
任务说明：
%s

你在独立的 git worktree 里工作（分支 %s），只改与任务相关的文件，改完自行提交到这个分支。
完成后（无论成功还是失败）必须运行一次：
  fleetctl report --outcome succeeded|failed --body "<做了什么、发现了什么、还剩什么>" [--files "改动的文件,逗号分隔"]
需要协调者决定时运行：fleetctl ask --question "<问题>" [--options "选项A,选项B"]，然后结束本轮回复，答复会输入到你的终端。`,
		t.ID, t.Title, t.Spec, branch)
}

// ListRuns 返回全部编排（新的在前）。
func (u *Usecase) ListRuns(ctx context.Context) ([]RunSummary, error) {
	runs, err := listDocs[Run](ctx, u.store, kindRun)
	if err != nil {
		return nil, err
	}
	tasks, _ := listDocs[Task](ctx, u.store, kindTask)
	msgs, _ := listDocs[Message](ctx, u.store, kindMessage)
	out := make([]RunSummary, 0, len(runs))
	for _, r := range runs {
		s := RunSummary{Run: r}
		for _, t := range tasks {
			if t.RunID == r.ID {
				s.TaskCount++
				if t.Status == TaskCompleted || t.Status == TaskFailed {
					s.DoneCount++
				}
			}
		}
		for _, m := range msgs {
			if m.RunID == r.ID && m.Type == "question" && m.Answer == "" {
				s.Unread++
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// GetRun 返回编排详情。
func (u *Usecase) GetRun(ctx context.Context, id string) (RunDetail, error) {
	run, ok, err := getDoc[Run](ctx, u.store, kindRun, id)
	if err != nil {
		return RunDetail{}, err
	}
	if !ok {
		return RunDetail{}, apierror.NotFound(domain, "run not found")
	}
	d := RunDetail{Run: run}
	tasks, _ := listDocs[Task](ctx, u.store, kindTask)
	for _, t := range tasks {
		if t.RunID == id {
			d.Tasks = append(d.Tasks, t)
		}
	}
	msgs, _ := listDocs[Message](ctx, u.store, kindMessage)
	for _, m := range msgs {
		if m.RunID == id {
			d.Messages = append(d.Messages, m)
		}
	}
	sortByCreated(d.Tasks, func(t Task) string { return t.CreatedAt })
	sortByCreated(d.Messages, func(m Message) string { return m.CreatedAt })
	return d, nil
}

// StopRun 结束协调者与仍在跑的 worker 进程（标签保留，可查看输出）。
func (u *Usecase) StopRun(ctx context.Context, id string) (Run, error) {
	d, err := u.GetRun(ctx, id)
	if err != nil {
		return Run{}, err
	}
	for _, t := range d.Tasks {
		if s, ok := u.Session(t.WorkerTerminalID); ok {
			s.Kill()
		}
	}
	if s, ok := u.Session(d.Run.CoordinatorTerminalID); ok {
		s.Kill()
	}
	d.Run.Status, d.Run.UpdatedAt = RunStopped, nowString()
	return d.Run, putDoc(ctx, u.store, kindRun, d.Run.ID, d.Run)
}

// AnswerQuestion 由人在界面上回答 worker 的提问，答复输入到 worker 终端。
func (u *Usecase) AnswerQuestion(ctx context.Context, messageID, answer string) (Message, error) {
	m, ok, err := getDoc[Message](ctx, u.store, kindMessage, messageID)
	if err != nil {
		return Message{}, err
	}
	if !ok {
		return Message{}, apierror.NotFound(domain, "message not found")
	}
	if strings.TrimSpace(answer) == "" {
		return Message{}, apierror.BadRequest(domain, "答复不能为空")
	}
	return u.answer(ctx, m, answer, "我")
}

func (u *Usecase) answer(ctx context.Context, m Message, answer, by string) (Message, error) {
	m.Answer, m.Read = strings.TrimSpace(answer), true
	if err := putDoc(ctx, u.store, kindMessage, m.ID, m); err != nil {
		return Message{}, err
	}
	if t, ok, _ := getDoc[Task](ctx, u.store, kindTask, m.TaskID); ok {
		if s, ok := u.Session(t.WorkerTerminalID); ok {
			_ = s.Paste(fmt.Sprintf("[%s对你的提问「%s」的答复] %s", by, truncate(m.Subject, 40), m.Answer), true)
		}
	}
	return m, nil
}

func (u *Usecase) addMessage(ctx context.Context, m Message) Message {
	m.ID, m.CreatedAt = shortID("msg_"), nowString()
	_ = putDoc(ctx, u.store, kindMessage, m.ID, m)
	return m
}

// wakeCoordinator 记下「有新消息」；协调者空闲时立刻提醒，忙时等它停下再提醒。
func (u *Usecase) wakeCoordinator(run Run) {
	if run.CoordinatorTerminalID == "" || run.Status != RunRunning {
		return
	}
	u.mu.Lock()
	u.wake[run.CoordinatorTerminalID] = true
	u.mu.Unlock()
	u.maybeWake(run.CoordinatorTerminalID)
}

func (u *Usecase) maybeWake(terminalID string) {
	u.mu.Lock()
	pending := u.wake[terminalID]
	s, ok := u.sessions[terminalID]
	u.mu.Unlock()
	if !pending || !ok {
		return
	}
	if st := s.Info().Status; st != StatusDone && st != StatusIdle {
		return
	}
	u.mu.Lock()
	delete(u.wake, terminalID)
	u.mu.Unlock()
	_ = s.Paste("[运行台] 有新的 worker 消息，请运行 fleetctl check 查看并处理。", true)
}

// onWorkerExit：worker 没汇报就退出，按失败处理并通知协调者。
// 协调者自己退出（崩溃、被关掉）时编排没人推进了：标为已停止，不再显示「进行中」。
func (u *Usecase) onWorkerExit(info TerminalInfo) {
	ctx := u.ctx
	if info.Role == "coordinator" && info.RunID != "" {
		if run, ok, _ := getDoc[Run](ctx, u.store, kindRun, info.RunID); ok && run.Status == RunRunning && run.CoordinatorTerminalID == info.ID {
			run.Status, run.UpdatedAt = RunStopped, nowString()
			_ = putDoc(ctx, u.store, kindRun, run.ID, run)
		}
		return
	}
	if info.Role != "worker" || info.TaskID == "" {
		return
	}
	t, ok, _ := getDoc[Task](ctx, u.store, kindTask, info.TaskID)
	if !ok || t.Status != TaskDispatched || t.WorkerTerminalID != info.ID {
		return
	}
	t.Status, t.Outcome, t.Summary, t.UpdatedAt = TaskFailed, "failed", "worker 进程退出，未汇报结果", nowString()
	_ = putDoc(ctx, u.store, kindTask, t.ID, t)
	run, ok, _ := getDoc[Run](ctx, u.store, kindRun, t.RunID)
	if !ok {
		return
	}
	u.addMessage(ctx, Message{RunID: run.ID, From: "task:" + t.ID, To: "coordinator", Type: "worker_done", Subject: t.Title, Body: t.Summary, TaskID: t.ID, DispatchID: t.DispatchID})
	u.wakeCoordinator(run)
}

// ---------- fleetctl 命令 ----------

// HandleCLI 执行 agent 终端里 fleetctl 发来的命令，按终端令牌识别身份。
func (u *Usecase) HandleCLI(ctx context.Context, token, cmd string, args map[string]string) (string, error) {
	s, ok := u.sessionByToken(token)
	if !ok {
		return "", apierror.Unauthorized(domain, "invalid fleet token")
	}
	info := s.Info()
	if info.RunID == "" {
		return "", apierror.FailedPrecondition(domain, "这个终端不在编排中（只有协调者与 worker 能用 fleetctl）")
	}
	run, ok, err := getDoc[Run](ctx, u.store, kindRun, info.RunID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", apierror.NotFound(domain, "run not found")
	}
	arg := func(k string) string { return strings.TrimSpace(args[k]) }
	switch info.Role {
	case "coordinator":
		return u.coordinatorCmd(ctx, run, info, cmd, arg)
	case "worker":
		return u.workerCmd(ctx, run, info, cmd, arg)
	}
	return "", apierror.Forbidden(domain, "unknown role")
}

func (u *Usecase) coordinatorCmd(ctx context.Context, run Run, info TerminalInfo, cmd string, arg func(string) string) (string, error) {
	switch cmd {
	case "task-create":
		if arg("spec") == "" {
			return "", apierror.BadRequest(domain, "--spec 必填")
		}
		title := arg("title")
		if title == "" {
			title = truncate(arg("spec"), 30)
		}
		var deps []string
		for _, d := range strings.Split(arg("deps"), ",") {
			if d = strings.TrimSpace(d); d != "" {
				deps = append(deps, d)
			}
		}
		now := nowString()
		t := Task{ID: shortID("task_"), RunID: run.ID, Title: title, Spec: arg("spec"), Status: TaskReady, Deps: deps, CreatedAt: now, UpdatedAt: now}
		if len(deps) > 0 {
			t.Status = TaskPending
		}
		if err := putDoc(ctx, u.store, kindTask, t.ID, t); err != nil {
			return "", err
		}
		return fmt.Sprintf("已创建任务 %s「%s」（%s）。下一步：fleetctl worker-start --task %s", t.ID, t.Title, t.Status, t.ID), nil
	case "worker-start":
		return u.workerStart(ctx, run, info, arg)
	case "status":
		return u.statusText(ctx, run.ID)
	case "check":
		return u.checkInbox(ctx, run.ID)
	case "worker-read":
		t, err := u.runTask(ctx, run.ID, arg("task"))
		if err != nil {
			return "", err
		}
		s, ok := u.Session(t.WorkerTerminalID)
		if !ok {
			return "", apierror.FailedPrecondition(domain, "该任务还没有 worker 终端")
		}
		n, _ := strconv.Atoi(arg("lines"))
		if n <= 0 || n > 400 {
			n = 60
		}
		return s.ReadText(n), nil
	case "send":
		t, err := u.runTask(ctx, run.ID, arg("task"))
		if err != nil {
			return "", err
		}
		s, ok := u.Session(t.WorkerTerminalID)
		if !ok || arg("body") == "" {
			return "", apierror.FailedPrecondition(domain, "worker 终端不存在或 --body 为空")
		}
		return "已发送。", s.Paste("[协调者] "+arg("body"), true)
	case "answer":
		m, ok, err := getDoc[Message](ctx, u.store, kindMessage, arg("message"))
		if err != nil || !ok || m.RunID != run.ID {
			return "", apierror.NotFound(domain, "message not found")
		}
		if _, err := u.answer(ctx, m, arg("body"), "协调者"); err != nil {
			return "", err
		}
		return "已答复并输入到 worker 终端。", nil
	case "worker-stop":
		t, err := u.runTask(ctx, run.ID, arg("task"))
		if err != nil {
			return "", err
		}
		if s, ok := u.Session(t.WorkerTerminalID); ok {
			s.Kill()
		}
		t.Status, t.Outcome, t.Summary, t.UpdatedAt = TaskFailed, "failed", "协调者停止了 worker", nowString()
		return "已停止。", putDoc(ctx, u.store, kindTask, t.ID, t)
	case "done":
		run.Status, run.UpdatedAt = RunDone, nowString()
		if err := putDoc(ctx, u.store, kindRun, run.ID, run); err != nil {
			return "", err
		}
		u.addEvent(Event{Kind: EventWorkerDone, Title: "编排完成：" + truncate(run.Objective, 30), Body: arg("summary"), WorkspaceID: run.WorkspaceID, TerminalID: info.ID, RunID: run.ID})
		return "编排已标记完成。", nil
	}
	return "", apierror.BadRequest(domain, "未知命令："+cmd+"（可用：task-create worker-start status check worker-read send answer worker-stop done）")
}

func (u *Usecase) workerStart(ctx context.Context, run Run, info TerminalInfo, arg func(string) string) (string, error) {
	t, err := u.runTask(ctx, run.ID, arg("task"))
	if err != nil {
		return "", err
	}
	if t.Status == TaskDispatched {
		return "", apierror.Conflict(domain, "任务已派发给 "+t.WorkerTerminalID)
	}
	for _, dep := range t.Deps {
		if d, ok, _ := getDoc[Task](ctx, u.store, kindTask, dep); !ok || d.Status != TaskCompleted {
			return "", apierror.FailedPrecondition(domain, "依赖 "+dep+" 还没完成")
		}
	}
	agent := arg("agent")
	if agent == "" {
		agent = run.Agent
	}
	if p, ok := findPreset(agent); !ok || !p.Available || p.ID == "shell" {
		return "", apierror.BadRequest(domain, "agent 不可用："+agent)
	}
	t.Status, t.WorkerAgent, t.DispatchID, t.UpdatedAt = TaskDispatched, agent, shortID("disp_"), nowString()
	switch arg("worktree") {
	case "current":
		w, err := u.workspace(ctx, info.WorkspaceID)
		if err != nil {
			return "", err
		}
		ti, err := u.startTerminal(w, sessionSpec{WorkspaceID: w.ID, Agent: agent, FullAccess: run.FullAccess, Title: "worker · " + t.Title,
			Prompt: workerPreamble(t, w.Branch), RunID: run.ID, TaskID: t.ID, Role: "worker"})
		if err != nil {
			return "", err
		}
		t.WorkerTerminalID, t.WorkerWorkspaceID = ti.ID, w.ID
		if err := putDoc(ctx, u.store, kindTask, t.ID, t); err != nil {
			return "", err
		}
		return fmt.Sprintf("worker 已在当前 worktree 启动（终端 %s）。现在结束本轮回复，等待汇报。", ti.ID), nil
	default:
		name := arg("name")
		if name == "" {
			name = t.Title
		}
		taskID := t.ID
		w, err := u.CreateWorkspace(ctx, CreateWorkspaceInput{
			ProjectID: run.ProjectID, Name: name, ParentID: run.WorkspaceID, Origin: "orchestration",
			Agent: agent, FullAccess: run.FullAccess, Prompt: workerPreamble(t, slugify(name)),
			RunID: run.ID, TaskID: t.ID, Role: "worker",
			onReady: func(w Workspace, ti TerminalInfo) {
				if cur, ok, _ := getDoc[Task](u.ctx, u.store, kindTask, taskID); ok {
					cur.WorkerTerminalID, cur.UpdatedAt = ti.ID, nowString()
					_ = putDoc(u.ctx, u.store, kindTask, cur.ID, cur)
				}
			},
		})
		if err != nil {
			return "", err
		}
		t.WorkerWorkspaceID = w.ID
		if err := putDoc(ctx, u.store, kindTask, t.ID, t); err != nil {
			return "", err
		}
		return fmt.Sprintf("正在为任务 %s 创建 worktree「%s」（分支 %s）并启动 %s。现在结束本轮回复，worker 汇报时你会收到提示。", t.ID, w.Name, w.Branch, agent), nil
	}
}

func (u *Usecase) workerCmd(ctx context.Context, run Run, info TerminalInfo, cmd string, arg func(string) string) (string, error) {
	t, ok, err := getDoc[Task](ctx, u.store, kindTask, info.TaskID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", apierror.NotFound(domain, "task not found")
	}
	switch cmd {
	case "report", "worker_done":
		outcome := arg("outcome")
		if outcome != "succeeded" && outcome != "failed" {
			return "", apierror.BadRequest(domain, "--outcome 必须是 succeeded 或 failed")
		}
		t.Outcome, t.Summary, t.UpdatedAt = outcome, arg("body"), nowString()
		t.Status = TaskCompleted
		if outcome == "failed" {
			t.Status = TaskFailed
		}
		t.FilesModified = nil
		for _, f := range strings.Split(arg("files"), ",") {
			if f = strings.TrimSpace(f); f != "" {
				t.FilesModified = append(t.FilesModified, f)
			}
		}
		if err := putDoc(ctx, u.store, kindTask, t.ID, t); err != nil {
			return "", err
		}
		u.promoteReady(ctx, run.ID)
		u.addMessage(ctx, Message{RunID: run.ID, From: "task:" + t.ID, To: "coordinator", Type: "worker_done", Subject: t.Title, Body: t.Summary, TaskID: t.ID, DispatchID: t.DispatchID})
		u.addEvent(Event{Kind: EventWorkerDone, Title: "worker 汇报（" + outcome + "）：" + t.Title, Body: t.Summary, WorkspaceID: info.WorkspaceID, TerminalID: info.ID, RunID: run.ID})
		u.wakeCoordinator(run)
		return "已汇报给协调者。你的任务到此结束。", nil
	case "ask":
		q := arg("question")
		if q == "" {
			return "", apierror.BadRequest(domain, "--question 必填")
		}
		var opts []string
		for _, o := range strings.Split(arg("options"), ",") {
			if o = strings.TrimSpace(o); o != "" {
				opts = append(opts, o)
			}
		}
		m := u.addMessage(ctx, Message{RunID: run.ID, From: "task:" + t.ID, To: "coordinator", Type: "question", Subject: q, Body: q, Options: opts, TaskID: t.ID, DispatchID: t.DispatchID})
		u.addEvent(Event{Kind: EventQuestion, Title: "worker 提问：" + truncate(q, 40), Body: t.Title, WorkspaceID: info.WorkspaceID, TerminalID: info.ID, RunID: run.ID})
		u.wakeCoordinator(run)
		return "问题已提交（" + m.ID + "）。请结束本轮回复，答复会输入到你的终端。", nil
	case "status":
		return fmt.Sprintf("任务 %s「%s」：%s", t.ID, t.Title, t.Status), nil
	}
	return "", apierror.BadRequest(domain, "worker 可用命令：report ask status")
}

// promoteReady：依赖全部完成的 pending 任务转 ready。
func (u *Usecase) promoteReady(ctx context.Context, runID string) {
	tasks, _ := listDocs[Task](ctx, u.store, kindTask)
	done := map[string]bool{}
	for _, t := range tasks {
		if t.RunID == runID && t.Status == TaskCompleted {
			done[t.ID] = true
		}
	}
	for _, t := range tasks {
		if t.RunID != runID || t.Status != TaskPending {
			continue
		}
		ready := true
		for _, d := range t.Deps {
			ready = ready && done[d]
		}
		if ready {
			t.Status, t.UpdatedAt = TaskReady, nowString()
			_ = putDoc(ctx, u.store, kindTask, t.ID, t)
		}
	}
}

func (u *Usecase) runTask(ctx context.Context, runID, id string) (Task, error) {
	t, ok, err := getDoc[Task](ctx, u.store, kindTask, id)
	if err != nil {
		return Task{}, err
	}
	if !ok || t.RunID != runID {
		return Task{}, apierror.NotFound(domain, "任务不存在："+id)
	}
	return t, nil
}

func (u *Usecase) statusText(ctx context.Context, runID string) (string, error) {
	d, err := u.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "编排 %s（%s）：%s\n", d.Run.ID, d.Run.Status, d.Run.Objective)
	if len(d.Tasks) == 0 {
		b.WriteString("还没有任务。用 fleetctl task-create 创建。\n")
	}
	for _, t := range d.Tasks {
		worker := "-"
		if t.WorkerTerminalID != "" {
			worker = t.WorkerAgent
			if s, ok := u.Session(t.WorkerTerminalID); ok {
				worker += "/" + s.Info().Status
			}
		}
		fmt.Fprintf(&b, "- %s [%s] %s  worker=%s", t.ID, t.Status, t.Title, worker)
		if len(t.Deps) > 0 {
			fmt.Fprintf(&b, " deps=%s", strings.Join(t.Deps, ","))
		}
		if t.Summary != "" {
			fmt.Fprintf(&b, "\n    结果：%s", truncate(t.Summary, 200))
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func (u *Usecase) checkInbox(ctx context.Context, runID string) (string, error) {
	msgs, err := listDocs[Message](ctx, u.store, kindMessage)
	if err != nil {
		return "", err
	}
	sortByCreated(msgs, func(m Message) string { return m.CreatedAt })
	var b strings.Builder
	n := 0
	for _, m := range msgs {
		if m.RunID != runID || m.To != "coordinator" || m.Read {
			continue
		}
		n++
		fmt.Fprintf(&b, "[%s] %s 来自 %s：%s\n", m.ID, m.Type, m.From, m.Subject)
		if m.Body != "" && m.Body != m.Subject {
			fmt.Fprintf(&b, "  %s\n", strings.ReplaceAll(m.Body, "\n", "\n  "))
		}
		if len(m.Options) > 0 {
			fmt.Fprintf(&b, "  选项：%s（用 fleetctl answer --message %s --body <答复> 回答）\n", strings.Join(m.Options, " / "), m.ID)
		} else if m.Type == "question" {
			fmt.Fprintf(&b, "  （用 fleetctl answer --message %s --body <答复> 回答）\n", m.ID)
		}
		if m.Type != "question" {
			m.Read = true
			_ = putDoc(ctx, u.store, kindMessage, m.ID, m)
		}
	}
	if n == 0 {
		return "没有新消息。", nil
	}
	return b.String(), nil
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
