// 在 Harness 里直接发起的执行（运行台开一次编排、流程页运行一个流程）也登记成一条工作台任务，
// 这样「任务」里是全的：任务页、今日页、看板、会话中心都能找到它。
// 设计：docs/aranea-agents/plan/2026-11-21-任务入口与执行统筹方案.md §3（任何执行都挂在一个任务下）
import { addOfficeTaskExecution, createOfficeTask, type OfficeTaskExecutionKind } from '../office/api';

export interface HarnessRun {
  /** 任务标题：这次执行要办成什么（取第一行） */
  title: string;
  body?: string;
  /** run = 运行台几个工具一起做；flow = 流程；tool = 运行台的一个编码工具 */
  assigneeKind: 'tool' | 'run' | 'flow';
  /** 工具 id / 流程 id */
  assigneeId: string;
  assigneeName: string;
  exec: { kind: OfficeTaskExecutionKind; ref_id: string; executor: string; hand?: string };
}

/**
 * 尽力而为：执行已经发起了，登记不上只是任务页里少一条，不打断发起它的页面。
 * 返回任务 id；没登记成返回空串。
 */
export async function recordHarnessRun(run: HarnessRun): Promise<string> {
  const [first = '', ...rest] = run.title.trim().split('\n');
  const title = first.trim();
  if (!title || !run.assigneeId || !run.exec.ref_id) return '';
  try {
    const task = await createOfficeTask(title, run.body ?? rest.join('\n').trim(), {
      sessionId: '',
      assigneeKind: run.assigneeKind,
      assigneeId: run.assigneeId,
      assigneeName: run.assigneeName,
    });
    await addOfficeTaskExecution(task.id, {
      kind: run.exec.kind,
      ref_id: run.exec.ref_id,
      executor: run.exec.executor,
      executor_name: run.assigneeName,
      hand: run.exec.hand,
    });
    return task.id;
  } catch (err) {
    console.warn('[tasks] harness run not recorded as a task', err);
    return '';
  }
}
