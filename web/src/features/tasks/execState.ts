// 派给流程 / 编码工具 / 运行台的任务没有「承载会话的任务态势」可读：它们的进度在各自的记录里。
// 这里按任务 ↔ 执行关联表，把任务最近一次这类执行的状态折成任务页用的三态。
// 设计：docs/aranea-agents/plan/2026-11-21-任务入口与执行统筹方案.md §6
import type { CodingTask } from '../../services/kratos/agentbridge/v1';
import { fromCodingTask, fromFlow, fromTerminal, type ExecStatus } from '../conversations/model';
import type { Run, Terminal } from '../fleet/api';
import type { GraphExecution } from '../graph/types';
import type { OfficeTaskExecutionRow } from '../office/api';
import type { DelegatedState } from '../today/logic';

export interface ExecFacts {
  codingTasks: readonly CodingTask[];
  flows: readonly GraphExecution[];
  terminals: readonly Terminal[];
  runs: readonly Run[];
}

export interface ExecOverride {
  state: DelegatedState;
  /** 停下来等人处理（区别于失败） */
  waiting: boolean;
  /** 失败原因、走到哪一步等原文 */
  detail: string;
  at: string;
}

type Seen = { status: ExecStatus; detail: string; at: string };

function look(row: OfficeTaskExecutionRow, facts: ExecFacts): Seen | null {
  if (row.kind === 'coding_task') {
    const t = facts.codingTasks.find((x) => x.id === row.ref_id);
    if (!t) return null;
    const e = fromCodingTask(t);
    return { status: e.status, detail: e.detail, at: e.last };
  }
  if (row.kind === 'graph_execution') {
    const x = facts.flows.find((f) => f.executionId === row.ref_id);
    if (!x) return null;
    const e = fromFlow(x, { hand: '', name: row.executor_name });
    return { status: e.status, detail: e.detail || e.what, at: e.last };
  }
  if (row.kind === 'fleet_terminal') {
    const t = facts.terminals.find((x) => x.id === row.ref_id);
    if (!t) return null;
    const e = fromTerminal(t, { presets: [], workspaces: [] });
    return { status: e.status, detail: '', at: e.last };
  }
  if (row.kind === 'fleet_run') {
    const r = facts.runs.find((x) => x.id === row.ref_id);
    if (!r) return null;
    return {
      status: r.status === 'running' ? 'running' : 'done',
      detail: `${r.doneCount}/${r.taskCount}`,
      at: r.createdAt,
    };
  }
  return null;
}

/** 会话类的执行不在这里判（它们有任务态势）；读不到状态的执行不产生结论，任务按原口径显示 */
export function executionOverrides(
  rows: readonly OfficeTaskExecutionRow[],
  facts: ExecFacts,
): Map<string, ExecOverride> {
  const out = new Map<string, ExecOverride>();
  // rows 按先后排：同一任务后面的覆盖前面的，留下最近一次
  for (const row of rows) {
    const seen = look(row, facts);
    if (!seen) continue;
    const state: DelegatedState =
      seen.status === 'running'
        ? 'running'
        : seen.status === 'waiting' || seen.status === 'failed'
          ? 'stuck'
          : 'review';
    out.set(row.task_id, { state, waiting: seen.status === 'waiting', detail: seen.detail, at: seen.at });
  }
  return out;
}

/** 需要去读状态的执行：只看还开着的派出任务里、非会话类的 */
export function pendingRefs(rows: readonly OfficeTaskExecutionRow[], openTaskIds: ReadonlySet<string>) {
  const mine = rows.filter((r) => openTaskIds.has(r.task_id) && r.kind !== 'session');
  return {
    rows: mine,
    flowIds: [...new Set(mine.filter((r) => r.kind === 'graph_execution').map((r) => r.ref_id))],
    needsFleet: mine.some((r) => r.kind === 'fleet_terminal' || r.kind === 'fleet_run'),
  };
}
