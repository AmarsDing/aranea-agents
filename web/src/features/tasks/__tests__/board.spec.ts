import { describe, expect, it } from 'vitest';
import type { OfficeTaskRow } from '../../office/types';
import type { DelegatedRow } from '../../today/logic';
import { buildBoard, composeDispatchMessage, dueState, fileIcon, fileSize, moveAction, sortMyTasks } from '../board';

const task = (id: string, extra: Partial<OfficeTaskRow> = {}): OfficeTaskRow => ({
  id,
  title: id,
  body: '',
  source: 'manual',
  source_ref: '',
  status: 'open',
  assignee_kind: '',
  assignee_id: '',
  assignee_name: '',
  session_id: '',
  due_at: '',
  file_count: 0,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
  ...extra,
});
const run = (id: string, state: DelegatedRow['state'], officeTaskId = ''): DelegatedRow => ({
  id,
  title: id,
  agentKey: '',
  state,
  at: '',
  to: '',
  officeTaskId,
  detail: '',
});

describe('任务生命周期：归栏、拖动、到期', () => {
  const now = new Date(2026, 10, 20, 10, 0);

  it('到期口径：已过 / 今天 / 以后 / 没设', () => {
    expect(dueState(new Date(2026, 10, 20, 9, 0).toISOString(), now)).toBe('overdue');
    expect(dueState(new Date(2026, 10, 20, 23, 0).toISOString(), now)).toBe('today');
    expect(dueState(new Date(2026, 10, 21, 0, 0).toISOString(), now)).toBe('later');
    expect(dueState('', now)).toBe('');
  });

  it('我的任务排序：在做的在前，再按到期早的在前（没设的最后），再按新建时间新到旧', () => {
    const rows = sortMyTasks([
      task('无到期-旧', { created_at: '2026-11-01T00:00:00Z' }),
      task('无到期-新', { created_at: '2026-11-05T00:00:00Z' }),
      task('后天到期', { due_at: '2026-11-22T00:00:00Z' }),
      task('明天到期', { due_at: '2026-11-21T00:00:00Z' }),
      task('在做', { status: 'doing' }),
    ]);
    expect(rows.map((r) => r.id)).toEqual(['在做', '明天到期', '后天到期', '无到期-新', '无到期-旧']);
  });

  it('四栏：AI 在做 / 卡住的排进行中最前；做完的进待验收；编码任务没有工作台任务也能进栏', () => {
    const board = buildBoard({
      mine: [task('待办'), task('我在做', { status: 'doing' })],
      runs: [run('编码卡住', 'stuck'), run('AI 做完', 'review', 'a2'), run('AI 在做', 'running', 'a1')],
      aiTasks: [task('a1', { assignee_kind: 'agent' }), task('a2', { assignee_kind: 'agent' })],
      done: [task('完成的', { status: 'done' })],
    });
    expect(board.todo.map((i) => i.task?.id)).toEqual(['待办']);
    expect(board.doing.map((i) => i.key)).toEqual(['编码卡住', 'AI 在做', 'task:我在做']);
    expect(board.doing[0]!.task).toBeNull();
    expect(board.doing[1]!.task?.id).toBe('a1');
    expect(board.review.map((i) => i.task?.id)).toEqual(['a2']);
    expect(board.done.map((i) => i.column)).toEqual(['done']);
  });

  it('拖动规则：待验收不接受拖入；AI 在做的不能拖回待办；待验收拖到进行中 = 退回、到已完成 = 验收；重开回原处', () => {
    const mine = { key: '', column: 'todo' as const, task: task('m'), run: null };
    expect(moveAction(mine, 'doing')).toBe('doing');
    expect(moveAction(mine, 'done')).toBe('done');
    expect(moveAction(mine, 'review')).toBeNull();
    expect(moveAction(mine, 'todo')).toBeNull();

    const ai = {
      key: '',
      column: 'doing' as const,
      task: task('a', { assignee_kind: 'agent' }),
      run: run('a', 'running'),
    };
    expect(moveAction(ai, 'todo')).toBeNull();
    expect(moveAction(ai, 'done')).toBe('accept');

    const review = { ...ai, column: 'review' as const };
    expect(moveAction(review, 'doing')).toBe('rework');
    expect(moveAction(review, 'done')).toBe('accept');
    expect(moveAction(review, 'todo')).toBeNull();

    const coding = { key: '', column: 'doing' as const, task: null, run: run('c', 'running') };
    expect(moveAction(coding, 'done')).toBeNull();

    const doneMine = { key: '', column: 'done' as const, task: task('d', { status: 'done' }), run: null };
    expect(moveAction(doneMine, 'todo')).toBe('open');
    expect(moveAction(doneMine, 'doing')).toBe('doing');
    const doneAi = { ...doneMine, task: task('d', { status: 'done', assignee_kind: 'team' }) };
    expect(moveAction(doneAi, 'doing')).toBe('open');
  });

  it('派给 AI 的消息：标题、说明、补充、到期、附件（附上的 / 太大留下的）', () => {
    const text = composeDispatchMessage(
      { title: '整理周报', body: '附上上周数据', due_at: '2026-11-21T10:00:00Z' },
      {
        note: '重点看转化率',
        attached: ['数据.xlsx', '截图.png'],
        skipped: ['录屏.mp4'],
        labels: { due: '到期：', attached: '附件：', skipped: '太大：', note: '补充：' },
        formatDue: () => '11-21 18:00',
      },
    );
    expect(text).toBe(
      [
        '整理周报',
        '附上上周数据',
        '补充：重点看转化率',
        '到期：11-21 18:00',
        '附件：数据.xlsx、截图.png',
        '太大：录屏.mp4',
      ].join('\n\n'),
    );
    expect(
      composeDispatchMessage(
        { title: '只有标题', body: ' ', due_at: '' },
        { attached: [], skipped: [], labels: { due: '', attached: '', skipped: '', note: '' }, formatDue: String },
      ),
    ).toBe('只有标题');
  });

  it('文件大小与图标', () => {
    expect(fileSize(512)).toBe('512 B');
    expect(fileSize(2048)).toBe('2.0 KB');
    expect(fileSize(5 * 1024 * 1024)).toBe('5.0 MB');
    expect(fileIcon('a.png', 'image/png')).toBe('image');
    expect(fileIcon('a.mp4', 'video/mp4')).toBe('movie');
    expect(fileIcon('报价.xlsx', '')).toBe('table_chart');
    expect(fileIcon('方案.docx', '')).toBe('description');
    expect(fileIcon('x.bin', '')).toBe('insert_drive_file');
  });
});
