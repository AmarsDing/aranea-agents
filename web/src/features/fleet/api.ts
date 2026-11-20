// Harness 多任务运行台的接口封装：生成客户端要求请求字段齐全、返回字段可能缺省，
// 这里统一补齐并把返回值规整为不含 undefined 的类型，组件里不再逐处判空。
import { createFleetService } from '../../services';

const api = createFleetService();

const s = (v: unknown): string => (typeof v === 'string' ? v : v == null ? '' : String(v));
const n = (v: unknown): number => (v == null || v === '' ? 0 : Number(v));
const b = (v: unknown): boolean => v === true;
const arr = <T>(v: T[] | undefined | null): T[] => (Array.isArray(v) ? v : []);

export interface AgentPreset {
  id: string;
  label: string;
  command: string;
  available: boolean;
  supportsFullAccess: boolean;
}

export interface Project {
  id: string;
  name: string;
  path: string;
  defaultBranch: string;
  isGit: boolean;
}

export interface Workspace {
  id: string;
  projectId: string;
  name: string;
  branch: string;
  baseRef: string;
  path: string;
  kind: 'primary' | 'worktree' | string;
  status: 'creating' | 'ready' | 'failed' | 'deleting' | string;
  error: string;
  pinned: boolean;
  archived: boolean;
  parentId: string;
  origin: string;
  linkedIssue: string;
  createdAt: string;
  lastActiveAt: string;
}

export type TerminalStatus = 'starting' | 'working' | 'needs_input' | 'done' | 'idle' | 'exited' | 'failed' | 'shell';

export interface Terminal {
  id: string;
  workspaceId: string;
  agent: string;
  title: string;
  status: TerminalStatus;
  fullAccess: boolean;
  pid: number;
  startedAt: string;
  exitCode: number;
  lastActivityAt: string;
  statusChangedAt: string;
  preview: string;
  runId: string;
  taskId: string;
  role: string;
}

export interface DiffLine {
  kind: 'ctx' | 'add' | 'del';
  oldNo: number;
  newNo: number;
  text: string;
}
export interface DiffHunk {
  header: string;
  lines: DiffLine[];
}
export interface DiffFile {
  path: string;
  oldPath: string;
  status: string;
  additions: number;
  deletions: number;
  binary: boolean;
  truncated: boolean;
  hunks: DiffHunk[];
}
export interface DiffResult {
  baseRef: string;
  mergeBase: string;
  files: DiffFile[];
  additions: number;
  deletions: number;
}

export interface Note {
  id: string;
  workspaceId: string;
  path: string;
  line: number;
  side: 'new' | 'old' | string;
  body: string;
  resolved: boolean;
  sentAt: string;
  createdAt: string;
  lineText: string;
}

export interface GitFile {
  path: string;
  index: string;
  worktree: string;
  staged: boolean;
  untracked: boolean;
}
export interface GitStatus {
  branch: string;
  upstream: string;
  ahead: number;
  behind: number;
  head: string;
  baseRef: string;
  additions: number;
  deletions: number;
  files: GitFile[];
}

export interface FleetEvent {
  id: string;
  kind: string;
  title: string;
  body: string;
  workspaceId: string;
  terminalId: string;
  runId: string;
  read: boolean;
  createdAt: string;
}

export interface UsageWindow {
  label: string;
  tokens: number;
  percent: number;
  resetsAt: string;
}
export interface AgentUsage {
  agent: string;
  windows: UsageWindow[];
  source: string;
  sessions: number;
}

export interface Run {
  id: string;
  objective: string;
  projectId: string;
  workspaceId: string;
  coordinatorTerminalId: string;
  agent: string;
  status: string;
  createdAt: string;
  taskCount: number;
  doneCount: number;
  unread: number;
}
export interface Task {
  id: string;
  title: string;
  spec: string;
  status: string;
  deps: string[];
  workerTerminalId: string;
  workerWorkspaceId: string;
  workerAgent: string;
  outcome: string;
  summary: string;
  filesModified: string[];
  updatedAt: string;
}
export interface Message {
  id: string;
  from: string;
  to: string;
  type: string;
  subject: string;
  body: string;
  taskId: string;
  read: boolean;
  answer: string;
  options: string[];
  createdAt: string;
}
export interface RunDetail {
  run: Run;
  tasks: Task[];
  messages: Message[];
}

export interface Automation {
  id: string;
  name: string;
  projectId: string;
  agent: string;
  prompt: string;
  schedule: string;
  worktreeMode: 'new' | 'primary' | string;
  fullAccess: boolean;
  enabled: boolean;
  lastRunAt: string;
  nextRunAt: string;
  lastStatus: string;
  lastWorkspaceId: string;
}

// ---------- 规整 ----------

type Raw = Record<string, unknown>;

function toWorkspace(r: Raw): Workspace {
  return {
    id: s(r.id),
    projectId: s(r.projectId),
    name: s(r.name),
    branch: s(r.branch),
    baseRef: s(r.baseRef),
    path: s(r.path),
    kind: s(r.kind),
    status: s(r.status),
    error: s(r.error),
    pinned: b(r.pinned),
    archived: b(r.archived),
    parentId: s(r.parentId),
    origin: s(r.origin),
    linkedIssue: s(r.linkedIssue),
    createdAt: s(r.createdAt),
    lastActiveAt: s(r.lastActiveAt),
  };
}

function toTerminal(r: Raw): Terminal {
  return {
    id: s(r.id),
    workspaceId: s(r.workspaceId),
    agent: s(r.agent),
    title: s(r.title),
    status: (s(r.status) || 'starting') as TerminalStatus,
    fullAccess: b(r.fullAccess),
    pid: n(r.pid),
    startedAt: s(r.startedAt),
    exitCode: n(r.exitCode),
    lastActivityAt: s(r.lastActivityAt),
    statusChangedAt: s(r.statusChangedAt),
    preview: s(r.preview),
    runId: s(r.runId),
    taskId: s(r.taskId),
    role: s(r.role),
  };
}

function toNote(r: Raw): Note {
  return {
    id: s(r.id),
    workspaceId: s(r.workspaceId),
    path: s(r.path),
    line: n(r.line),
    side: s(r.side) || 'new',
    body: s(r.body),
    resolved: b(r.resolved),
    sentAt: s(r.sentAt),
    createdAt: s(r.createdAt),
    lineText: s(r.lineText),
  };
}

function toRun(r: Raw): Run {
  return {
    id: s(r.id),
    objective: s(r.objective),
    projectId: s(r.projectId),
    workspaceId: s(r.workspaceId),
    coordinatorTerminalId: s(r.coordinatorTerminalId),
    agent: s(r.agent),
    status: s(r.status),
    createdAt: s(r.createdAt),
    taskCount: n(r.taskCount),
    doneCount: n(r.doneCount),
    unread: n(r.unread),
  };
}

function toAutomation(r: Raw): Automation {
  return {
    id: s(r.id),
    name: s(r.name),
    projectId: s(r.projectId),
    agent: s(r.agent),
    prompt: s(r.prompt),
    schedule: s(r.schedule),
    worktreeMode: s(r.worktreeMode) || 'new',
    fullAccess: b(r.fullAccess),
    enabled: b(r.enabled),
    lastRunAt: s(r.lastRunAt),
    nextRunAt: s(r.nextRunAt),
    lastStatus: s(r.lastStatus),
    lastWorkspaceId: s(r.lastWorkspaceId),
  };
}

const raw = <T>(v: T) => v as unknown as Raw;
const raws = <T>(v: T[] | undefined) => arr(v) as unknown as Raw[];

// ---------- 调用 ----------

export async function listPresets(): Promise<AgentPreset[]> {
  const res = await api.ListAgentPresets({});
  return raws(res.items).map((r) => ({
    id: s(r.id),
    label: s(r.label),
    command: s(r.command),
    available: b(r.available),
    supportsFullAccess: b(r.supportsFullAccess),
  }));
}

export async function listProjects(): Promise<Project[]> {
  const res = await api.ListProjects({});
  return raws(res.items).map((r) => ({
    id: s(r.id),
    name: s(r.name),
    path: s(r.path),
    defaultBranch: s(r.defaultBranch),
    isGit: b(r.isGit),
  }));
}

export async function addProject(path: string, name = ''): Promise<void> {
  await api.AddProject({ path, name });
}

export async function listRefs(projectId: string): Promise<{ local: string[]; remote: string[]; defaultRef: string }> {
  const res = await api.ListRefs({ projectId });
  return { local: arr(res.local), remote: arr(res.remote), defaultRef: s(res.defaultRef) };
}

export async function listWorkspaces(): Promise<Workspace[]> {
  const res = await api.ListWorkspaces({ projectId: '', includeArchived: true });
  return raws(res.items).map(toWorkspace);
}

export interface CreateWorkspaceInput {
  projectId: string;
  name: string;
  branch?: string;
  startFrom?: string;
  agent?: string;
  prompt?: string;
  fullAccess?: boolean;
  parentId?: string;
  fetch?: boolean;
}

export async function createWorkspace(i: CreateWorkspaceInput): Promise<Workspace> {
  const res = await api.CreateWorkspace({
    projectId: i.projectId,
    name: i.name,
    branch: i.branch ?? '',
    startFrom: i.startFrom ?? '',
    agent: i.agent ?? '',
    prompt: i.prompt ?? '',
    fullAccess: i.fullAccess ?? false,
    parentId: i.parentId ?? '',
    fetch: i.fetch ?? false,
  });
  return toWorkspace(raw(res));
}

export async function updateWorkspace(
  id: string,
  patch: { name?: string; pinned?: boolean; archived?: boolean; linkedIssue?: string },
): Promise<Workspace> {
  return toWorkspace(raw(await api.UpdateWorkspace({ id, ...patch })));
}

export async function deleteWorkspace(
  id: string,
  forceBranch = false,
): Promise<{ branchKept: boolean; message: string }> {
  const res = await api.DeleteWorkspace({ id, forceBranch });
  return { branchKept: b(res.branchKept), message: s(res.message) };
}

export async function listTerminals(): Promise<Terminal[]> {
  const res = await api.ListTerminals({ workspaceId: '' });
  return raws(res.items).map(toTerminal);
}

export async function createTerminal(i: {
  workspaceId: string;
  agent: string;
  prompt?: string;
  fullAccess?: boolean;
  title?: string;
}): Promise<Terminal> {
  const res = await api.CreateTerminal({
    workspaceId: i.workspaceId,
    agent: i.agent,
    prompt: i.prompt ?? '',
    fullAccess: i.fullAccess ?? false,
    title: i.title ?? '',
  });
  return toTerminal(raw(res));
}

export async function killTerminal(id: string): Promise<void> {
  await api.KillTerminal({ id });
}
export async function restartTerminal(id: string): Promise<void> {
  await api.RestartTerminal({ id });
}
export async function closeTerminal(id: string): Promise<void> {
  await api.CloseTerminal({ id });
}
export async function sendInput(id: string, text: string, submit = true): Promise<void> {
  await api.SendInput({ id, text, submit });
}

export async function getDiff(workspaceId: string, baseRef = ''): Promise<DiffResult> {
  const res = await api.GetDiff({ workspaceId, baseRef });
  return {
    baseRef: s(res.baseRef),
    mergeBase: s(res.mergeBase),
    additions: n(res.additions),
    deletions: n(res.deletions),
    files: raws(res.files).map((f) => ({
      path: s(f.path),
      oldPath: s(f.oldPath),
      status: s(f.status),
      additions: n(f.additions),
      deletions: n(f.deletions),
      binary: b(f.binary),
      truncated: b(f.truncated),
      hunks: arr(f.hunks as Raw[] | undefined).map((h) => ({
        header: s(h.header),
        lines: arr(h.lines as Raw[] | undefined).map((l) => ({
          kind: (s(l.kind) || 'ctx') as DiffLine['kind'],
          oldNo: n(l.oldNo),
          newNo: n(l.newNo),
          text: s(l.text),
        })),
      })),
    })),
  };
}

export async function listNotes(workspaceId: string): Promise<Note[]> {
  const res = await api.ListNotes({ workspaceId });
  return raws(res.items).map(toNote);
}

export async function upsertNote(
  note: Partial<Note> & { workspaceId: string; path: string; line: number; body: string },
): Promise<Note> {
  const res = await api.UpsertNote({
    note: {
      id: note.id ?? '',
      workspaceId: note.workspaceId,
      path: note.path,
      line: note.line,
      side: note.side ?? 'new',
      body: note.body,
      resolved: note.resolved ?? false,
      sentAt: '',
      createdAt: '',
      lineText: note.lineText ?? '',
    },
  });
  return toNote(raw(res));
}

export async function deleteNote(id: string): Promise<void> {
  await api.DeleteNote({ id });
}

export async function sendNotes(
  workspaceId: string,
  terminalId: string,
  agent = '',
  fullAccess = false,
): Promise<{ terminalId: string; sent: number }> {
  const res = await api.SendNotes({ workspaceId, terminalId, agent, fullAccess });
  return { terminalId: s(res.terminalId), sent: n(res.sent) };
}

export async function getGitStatus(workspaceId: string): Promise<GitStatus> {
  const res = raw(await api.GetGitStatus({ workspaceId }));
  return {
    branch: s(res.branch),
    upstream: s(res.upstream),
    ahead: n(res.ahead),
    behind: n(res.behind),
    head: s(res.head),
    baseRef: s(res.baseRef),
    additions: n(res.additions),
    deletions: n(res.deletions),
    files: arr(res.files as Raw[] | undefined).map((f) => ({
      path: s(f.path),
      index: s(f.index),
      worktree: s(f.worktree),
      staged: b(f.staged),
      untracked: b(f.untracked),
    })),
  };
}

export interface GitOp {
  ok: boolean;
  output: string;
}
const op = (r: { ok?: boolean; output?: string }): GitOp => ({ ok: b(r.ok), output: s(r.output) });

export async function stageFiles(workspaceId: string, paths: string[]): Promise<GitOp> {
  return op(await api.StageFiles({ workspaceId, paths }));
}
export async function unstageFiles(workspaceId: string, paths: string[]): Promise<GitOp> {
  return op(await api.UnstageFiles({ workspaceId, paths }));
}
export async function commit(workspaceId: string, message: string, amend = false): Promise<GitOp> {
  return op(await api.Commit({ workspaceId, message, amend }));
}
export async function push(workspaceId: string): Promise<GitOp> {
  return op(await api.Push({ workspaceId }));
}

export async function listEvents(limit = 100): Promise<{ items: FleetEvent[]; unread: number }> {
  const res = await api.ListEvents({ limit, unreadOnly: false });
  return {
    unread: n(res.unread),
    items: raws(res.items).map((r) => ({
      id: s(r.id),
      kind: s(r.kind),
      title: s(r.title),
      body: s(r.body),
      workspaceId: s(r.workspaceId),
      terminalId: s(r.terminalId),
      runId: s(r.runId),
      read: b(r.read),
      createdAt: s(r.createdAt),
    })),
  };
}

export async function markEventsRead(ids: string[], all = false): Promise<void> {
  await api.MarkEventsRead({ ids, all });
}

export async function getUsage(): Promise<AgentUsage[]> {
  const res = await api.GetUsage({});
  return raws(res.items).map((r) => ({
    agent: s(r.agent),
    source: s(r.source),
    sessions: n(r.sessions),
    windows: arr(r.windows as Raw[] | undefined).map((w) => ({
      label: s(w.label),
      tokens: n(w.tokens),
      percent: w.percent == null ? -1 : Number(w.percent),
      resetsAt: s(w.resetsAt),
    })),
  }));
}

/** 平台积分。total / remaining 为 -1 表示未知；updatedAt 为空表示还没有数据（手填平台没登记过）。 */
export interface PlatformCredit {
  platform: string;
  source: 'auto' | 'manual';
  plan: string;
  total: number;
  remaining: number;
  unit: string;
  unlimited: boolean;
  updatedAt: string;
  note: string;
  /** 近 30 天每个 UTC 日最后一次的剩余，日期升序。 */
  history: Array<{ date: string; remaining: number }>;
}

function creditFrom(r: Raw): PlatformCredit {
  return {
    platform: s(r.platform),
    source: s(r.source) === 'manual' ? 'manual' : 'auto',
    plan: s(r.plan),
    // 0 在 JSON 里会被省略，未知固定是 -1，所以缺省按 0 读
    total: n(r.total),
    remaining: n(r.remaining),
    unit: s(r.unit),
    unlimited: b(r.unlimited),
    updatedAt: s(r.updatedAt),
    note: s(r.note),
    history: raws(r.history as Raw[] | undefined).map((p) => ({ date: s(p.date), remaining: n(p.remaining) })),
  };
}

export async function listCredits(): Promise<PlatformCredit[]> {
  const res = await api.ListCredits({});
  return raws(res.items).map(creditFrom);
}

export async function setCredit(i: {
  platform: string;
  plan: string;
  total: number;
  remaining: number;
  unit: string;
  note: string;
}): Promise<PlatformCredit> {
  return creditFrom((await api.SetCredit(i)) as unknown as Raw);
}

export async function createRun(i: {
  projectId: string;
  workspaceId?: string;
  objective: string;
  agent: string;
  fullAccess: boolean;
}): Promise<Run> {
  return toRun(
    raw(
      await api.CreateRun({
        projectId: i.projectId,
        workspaceId: i.workspaceId ?? '',
        objective: i.objective,
        agent: i.agent,
        fullAccess: i.fullAccess,
      }),
    ),
  );
}
export async function listRuns(): Promise<Run[]> {
  return raws((await api.ListRuns({})).items).map(toRun);
}
export async function getRun(id: string): Promise<RunDetail> {
  const res = await api.GetRun({ id });
  return {
    run: toRun(raw(res.run ?? {})),
    tasks: raws(res.tasks).map((t) => ({
      id: s(t.id),
      title: s(t.title),
      spec: s(t.spec),
      status: s(t.status),
      deps: arr(t.deps as string[] | undefined),
      workerTerminalId: s(t.workerTerminalId),
      workerWorkspaceId: s(t.workerWorkspaceId),
      workerAgent: s(t.workerAgent),
      outcome: s(t.outcome),
      summary: s(t.summary),
      filesModified: arr(t.filesModified as string[] | undefined),
      updatedAt: s(t.updatedAt),
    })),
    messages: raws(res.messages).map((m) => ({
      id: s(m.id),
      from: s(m.from),
      to: s(m.to),
      type: s(m.type),
      subject: s(m.subject),
      body: s(m.body),
      taskId: s(m.taskId),
      read: b(m.read),
      answer: s(m.answer),
      options: arr(m.options as string[] | undefined),
      createdAt: s(m.createdAt),
    })),
  };
}
export async function stopRun(id: string): Promise<void> {
  await api.StopRun({ id });
}
export async function answerQuestion(messageId: string, answer: string): Promise<void> {
  await api.AnswerQuestion({ messageId, answer });
}

export async function listAutomations(): Promise<Automation[]> {
  return raws((await api.ListAutomations({})).items).map(toAutomation);
}
export async function upsertAutomation(
  a: Partial<Automation> & { projectId: string; prompt: string; schedule: string },
): Promise<Automation> {
  const res = await api.UpsertAutomation({
    automation: {
      id: a.id ?? '',
      name: a.name ?? '',
      projectId: a.projectId,
      agent: a.agent ?? 'claude',
      prompt: a.prompt,
      schedule: a.schedule,
      worktreeMode: a.worktreeMode ?? 'new',
      fullAccess: a.fullAccess ?? false,
      enabled: a.enabled ?? true,
      lastRunAt: '',
      nextRunAt: '',
      lastStatus: '',
      lastWorkspaceId: '',
      createdAt: '',
    },
  });
  return toAutomation(raw(res));
}
export async function deleteAutomation(id: string): Promise<void> {
  await api.DeleteAutomation({ id });
}
export async function runAutomationNow(id: string): Promise<Automation> {
  return toAutomation(raw(await api.RunAutomationNow({ id })));
}

/** 从后端错误里取一句给人看的话。 */
export function errText(e: unknown): string {
  const anyE = e as { response?: { data?: { message?: string; reason?: string } }; message?: string };
  return anyE?.response?.data?.message || anyE?.message || String(e);
}
