import { createAgentBridgeService } from '../../services';
import type {
  CodingAgent,
  CodingProject,
  CodingTask,
  CodingTaskEvent,
  DispatchTaskResponse,
  ProjectCandidate,
  UpsertAgentRequest,
  UpsertProjectRequest,
} from '../../services/kratos/agentbridge/v1/index';

export type {
  CodingAgent,
  CodingProject,
  CodingTask,
  CodingTaskEvent,
  DispatchTaskResponse,
  ProjectCandidate,
  UpsertAgentRequest,
  UpsertProjectRequest,
};

const bridge = createAgentBridgeService();

export async function listCodingAgents(): Promise<CodingAgent[]> {
  const res = await bridge.ListAgents({});
  return res.items ?? [];
}

export async function upsertCodingAgent(req: UpsertAgentRequest): Promise<CodingAgent> {
  return bridge.UpsertAgent(req);
}

export async function probeCodingAgent(agentKey: string) {
  return bridge.ProbeAgent({ agentKey });
}

export async function listCodingProjects(): Promise<CodingProject[]> {
  const res = await bridge.ListProjects({});
  return res.items ?? [];
}

export async function upsertCodingProject(req: UpsertProjectRequest): Promise<CodingProject> {
  return bridge.UpsertProject(req);
}

export async function deleteCodingProject(id: string): Promise<void> {
  await bridge.DeleteProject({ id });
}

export async function listCodingTasks(limit = 100, agentKey = ''): Promise<CodingTask[]> {
  const res = await bridge.ListTasks({
    sessionId: undefined,
    limit,
    agentKey: agentKey || undefined,
  });
  return res.items ?? [];
}

export async function getCodingTask(id: string): Promise<CodingTask> {
  return bridge.GetTask({ id });
}

export async function cancelCodingTask(id: string): Promise<CodingTask> {
  return bridge.CancelTask({ id });
}

export async function dispatchCodingTask(req: {
  agentKey?: string;
  projectQuery: string;
  prompt: string;
  origin?: string;
}): Promise<DispatchTaskResponse> {
  return bridge.DispatchTask({
    sessionId: undefined,
    agentKey: req.agentKey || undefined,
    projectQuery: req.projectQuery,
    prompt: req.prompt,
    origin: req.origin || 'workbench',
  });
}

export async function listCodingTaskEvents(
  taskId: string,
  afterSeq = 0,
  limit = 200,
): Promise<CodingTaskEvent[]> {
  const res = await bridge.ListTaskEvents({ taskId, afterSeq, limit });
  return res.items ?? [];
}

export async function reviewCodingTask(id: string, action: string, comment = ''): Promise<CodingTask> {
  return bridge.ReviewTask({ id, action, comment });
}

// —— 展示辅助（纯函数，页面/卡片共用） ——

export const TERMINAL_STATUSES = ['done', 'failed', 'cancelled'];

export function isTerminalStatus(status: string | undefined): boolean {
  return TERMINAL_STATUSES.includes(status ?? '');
}

export function isActiveStatus(status: string | undefined): boolean {
  return ['dispatched', 'running', 'awaiting_approval', 'cancelling'].includes(status ?? '');
}

export function statusLabel(status: string | undefined): string {
  switch (status) {
    case 'dispatched':
      return '排队中';
    case 'running':
      return '运行中';
    case 'awaiting_approval':
      return '待审批';
    case 'done':
      return '已完成';
    case 'failed':
      return '失败';
    case 'cancelling':
    case 'cancelled':
      return '已中止';
    default:
      return status || '未知';
  }
}

export function statusColor(status: string | undefined): string {
  switch (status) {
    case 'dispatched':
      return 'grey-7';
    case 'running':
      return 'primary';
    case 'awaiting_approval':
      return 'warning';
    case 'done':
      return 'positive';
    case 'failed':
      return 'negative';
    case 'cancelling':
    case 'cancelled':
      return 'orange-8';
    default:
      return 'grey-6';
  }
}

export function eventKindColor(kind: string | undefined): string {
  switch (kind) {
    case 'tool_call':
      return 'primary';
    case 'plan':
      return 'purple';
    case 'permission':
      return 'warning';
    case 'error':
      return 'negative';
    case 'agent_message_chunk':
    default:
      return 'grey-8';
  }
}

export function eventKindLabel(kind: string | undefined): string {
  switch (kind) {
    case 'agent_message_chunk':
      return '消息';
    case 'tool_call':
      return '工具调用';
    case 'plan':
      return '计划';
    case 'permission':
      return '权限';
    case 'error':
      return '错误';
    default:
      return kind || '事件';
  }
}

export function parseTime(value: string | undefined): number {
  if (!value) return 0;
  const t = new Date(value).getTime();
  return Number.isNaN(t) ? 0 : t;
}

export function formatDateTime(value: string | undefined): string {
  const t = parseTime(value);
  if (!t) return '-';
  return new Date(t).toLocaleString();
}

/** 已耗时：优先 createdAt→completedAt，运行中则 createdAt→now。 */
export function formatDuration(task: CodingTask, now = Date.now()): string {
  const start = parseTime(task.createdAt);
  if (!start) return '-';
  const end = parseTime(task.completedAt) || now;
  const sec = Math.max(0, Math.floor((end - start) / 1000));
  if (sec < 60) return `${sec} 秒`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min} 分 ${sec % 60} 秒`;
  const hr = Math.floor(min / 60);
  return `${hr} 小时 ${min % 60} 分`;
}

/** 运行中任务最后输出距今的分钟数（0 表示 1 分钟内）。 */
export function silentMinutes(task: CodingTask, now = Date.now()): number {
  const last = parseTime(task.lastActiveAt);
  if (!last) return 0;
  return Math.max(0, Math.floor((now - last) / 60000));
}

export function agentInitial(agent: CodingAgent | undefined | null): string {
  const name = agent?.displayName || agent?.agentKey || '?';
  return name.trim().charAt(0).toUpperCase() || '?';
}

export const CAPABILITY_OPTIONS = [
  { value: 'read_workspace', label: '读取工作区' },
  { value: 'write_workspace', label: '写入工作区' },
  { value: 'exec_command', label: '执行命令' },
];

export const AUTONOMY_OPTIONS = [
  { value: 'readonly', label: '只读（readonly）— 仅浏览与读取，不修改文件' },
  { value: 'workspace', label: '工作区（workspace）— 可修改项目文件' },
  { value: 'full', label: '完全（full）— 可执行任意命令' },
];

export function autonomyLabel(value: string | undefined): string {
  const hit = AUTONOMY_OPTIONS.find((o) => o.value === value);
  return hit ? hit.label : value || '未设置';
}
