/**
 * 任务生命周期的纯函数：四栏归类、拖动规则、到期口径、派给 AI 时发出的那条消息。
 * 设计：docs/aranea-agents/design/2026-11-20-工作台任务生命周期.md §4。
 *
 *   待办 todo    我的、未开始（open、没派给 AI）
 *   进行中 doing 我在做的（doing）+ AI 在做 / 卡住的
 *   待验收 review AI 做完等我看的
 *   已完成 done  最近完成的
 */
import type { OfficeTaskRow } from '../office/types';
import type { DelegatedRow } from '../today/logic';

export type BoardColumn = 'todo' | 'doing' | 'review' | 'done';

export const BOARD_COLUMNS: readonly BoardColumn[] = ['todo', 'doing', 'review', 'done'];

/** 看板上的一张卡：task = 工作台任务（我的 / 派给 AI 的 / 已完成）；run = AI 执行进度（编码任务只有 run）。 */
export type BoardItem = {
  key: string;
  column: BoardColumn;
  task: OfficeTaskRow | null;
  run: DelegatedRow | null;
};

/** 把一张卡拖到某栏意味着什么；null = 不允许。 */
export type MoveAction = 'open' | 'doing' | 'done' | 'accept' | 'rework';

export type DueState = 'overdue' | 'today' | 'later' | '';

function parseMs(iso: string): number {
  return iso ? new Date(iso).getTime() : NaN;
}

/** 到期口径：已过 = overdue；今天之内 = today；以后 = later；没设 = ''。 */
export function dueState(dueAt: string, now: Date): DueState {
  const ms = parseMs(dueAt);
  if (!Number.isFinite(ms)) return '';
  if (ms < now.getTime()) return 'overdue';
  const end = new Date(now);
  end.setHours(24, 0, 0, 0);
  return ms < end.getTime() ? 'today' : 'later';
}

/** 我的任务排序：在做的在前；再按到期（早的在前，没设的最后）；再按新建时间新到旧。 */
export function sortMyTasks(tasks: readonly OfficeTaskRow[]): OfficeTaskRow[] {
  const due = (t: OfficeTaskRow) => {
    const ms = parseMs(t.due_at);
    return Number.isFinite(ms) ? ms : Number.POSITIVE_INFINITY;
  };
  return [...tasks].sort(
    (a, b) =>
      Number(b.status === 'doing') - Number(a.status === 'doing') ||
      due(a) - due(b) ||
      b.created_at.localeCompare(a.created_at),
  );
}

/**
 * 四栏归类。mine = 我的待办 + 在做（status active）；runs = 进行中的 AI 执行（含编码任务）；
 * aiTasks = 派给 AI 的工作台任务（按 id 配上 run）；done = 最近完成。
 */
export function buildBoard(input: {
  mine: readonly OfficeTaskRow[];
  runs: readonly DelegatedRow[];
  aiTasks: readonly OfficeTaskRow[];
  done: readonly OfficeTaskRow[];
}): Record<BoardColumn, BoardItem[]> {
  const out: Record<BoardColumn, BoardItem[]> = { todo: [], doing: [], review: [], done: [] };
  for (const task of sortMyTasks(input.mine)) {
    out[task.status === 'doing' ? 'doing' : 'todo'].push({ key: `task:${task.id}`, column: 'todo', task, run: null });
  }
  const byId = new Map(input.aiTasks.map((t) => [t.id, t]));
  // 卡住的、等我回复的排在进行中最前（runs 已按 卡住 → 待验收 → 运行中 排好）
  const aiRows: BoardItem[] = input.runs.map((run) => ({
    key: run.id,
    column: run.state === 'review' ? 'review' : 'doing',
    task: (run.officeTaskId && byId.get(run.officeTaskId)) || null,
    run,
  }));
  out.doing = [...aiRows.filter((r) => r.column === 'doing'), ...out.doing];
  out.review = aiRows.filter((r) => r.column === 'review');
  out.done = input.done.map((task) => ({ key: `task:${task.id}`, column: 'done', task, run: null }));
  for (const col of BOARD_COLUMNS) for (const item of out[col]) item.column = col;
  return out;
}

/** 卡片能不能拖：编码任务（只有 run，没有工作台任务）不能。 */
export function isDraggable(item: BoardItem): boolean {
  return item.task !== null;
}

/**
 * 拖到 to 栏时要做的事；同栏或不允许返回 null。
 * - 待验收不接受拖入（由 Agent 做完触发）；
 * - AI 在做的不能直接拖回待办（先在详情里收回自己做）；
 * - 待验收拖到进行中 = 退回返工，拖到已完成 = 验收通过。
 */
export function moveAction(item: BoardItem, to: BoardColumn): MoveAction | null {
  const task = item.task;
  if (!task || item.column === to || to === 'review') return null;
  const ai = Boolean(task.assignee_kind);
  if (item.column === 'done') {
    if (to === 'done') return null;
    // 派给 AI 的重开后回到它的会话里（进行中 / 待验收由会话状态决定）
    return ai ? 'open' : to === 'todo' ? 'open' : 'doing';
  }
  if (item.column === 'review') return to === 'done' ? 'accept' : to === 'doing' ? 'rework' : null;
  if (ai) return to === 'done' ? 'accept' : null;
  return to === 'todo' ? 'open' : to === 'doing' ? 'doing' : 'done';
}

/** 派给 AI 时发出的消息正文（labels 由调用方按语言给）。 */
export function composeDispatchMessage(
  task: Pick<OfficeTaskRow, 'title' | 'body' | 'due_at'>,
  opts: {
    note?: string;
    attached: readonly string[];
    skipped: readonly string[];
    labels: { due: string; attached: string; skipped: string; note: string };
    formatDue: (iso: string) => string;
  },
): string {
  const parts = [task.title.trim()];
  if (task.body.trim()) parts.push(task.body.trim());
  if (opts.note?.trim()) parts.push(`${opts.labels.note}${opts.note.trim()}`);
  if (task.due_at) parts.push(`${opts.labels.due}${opts.formatDue(task.due_at)}`);
  if (opts.attached.length) parts.push(`${opts.labels.attached}${opts.attached.join('、')}`);
  if (opts.skipped.length) parts.push(`${opts.labels.skipped}${opts.skipped.join('、')}`);
  return parts.join('\n\n');
}

/** 文件大小的人话写法。 */
export function fileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(bytes < 10 * 1024 ? 1 : 0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

/** 按 MIME / 扩展名给一个图标名（Material Icons）。 */
export function fileIcon(name: string, contentType: string): string {
  const ext = name.split('.').pop()?.toLowerCase() ?? '';
  if (contentType.startsWith('image/')) return 'image';
  if (contentType.startsWith('video/')) return 'movie';
  if (contentType.startsWith('audio/')) return 'audiotrack';
  if (contentType === 'application/pdf' || ext === 'pdf') return 'picture_as_pdf';
  if (['xls', 'xlsx', 'csv'].includes(ext)) return 'table_chart';
  if (['ppt', 'pptx', 'key'].includes(ext)) return 'slideshow';
  if (['doc', 'docx', 'txt', 'md', 'rtf'].includes(ext)) return 'description';
  if (['zip', 'rar', '7z', 'tar', 'gz'].includes(ext)) return 'folder_zip';
  return 'insert_drive_file';
}

/** 执行者图标：团队、流程、运行台的工具 / 编排、其余是同事（Agent） */
export function assigneeIcon(kind: string | undefined): string {
  if (kind === 'team') return 'groups';
  if (kind === 'flow') return 'schema';
  if (kind === 'run') return 'account_tree';
  if (kind === 'tool') return 'terminal';
  return 'smart_toy';
}

/**
 * 不是从工作台下达、而是在 Harness 里直接发起后登记进来的任务：从哪发起的。
 * 交办面板派给流程 / 编码工具的任务也是这几种执行对象，但它们带着资料袋会话，不算。
 */
export function harnessOrigin(
  task: { assignee_kind?: string; session_id?: string } | null | undefined,
): 'fleet' | 'flow' | '' {
  if (!task || task.session_id) return '';
  if (task.assignee_kind === 'flow') return 'flow';
  if (task.assignee_kind === 'run' || task.assignee_kind === 'tool') return 'fleet';
  return '';
}

/** 执行对象是一段可以直接对话的会话（同事 / 团队）；流程、编码工具、编排的过程不在会话里 */
export function talksInSession(kind: string | undefined): boolean {
  return kind === 'agent' || kind === 'team';
}
