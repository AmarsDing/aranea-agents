import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { OfficeTaskRow } from '../../office/types';
import { COMMAND_DONE_EVENT } from '../../today/useCommandRunner';
import { acceptFiles, quickAdd, quickAddPlan } from '../quickAdd';

const api = vi.hoisted(() => ({ createOfficeTask: vi.fn(), uploadTaskFile: vi.fn() }));
vi.mock('../../office/api', () => ({ ...api, TASK_FILE_MAX_BYTES: 50 * 1024 * 1024, TASK_FILES_MAX: 20 }));

const writer = { kind: 'agent' as const, id: 'a1', key: 'writer', name: '写手' };
const task = (extra: Partial<OfficeTaskRow> = {}): OfficeTaskRow => ({
  id: 't1',
  title: 't',
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
  created_at: '',
  updated_at: '',
  ...extra,
});

describe('指令窗直接记任务', () => {
  beforeEach(() => vi.clearAllMocks());

  it('一句话落成什么：@某 Agent → 派出；记：… → 随手记录（带附件时只能是任务）；其余 → 任务', () => {
    expect(quickAddPlan('@写手 整理周报', [writer], false)).toMatchObject({ kind: 'delegate', title: '整理周报' });
    expect(quickAddPlan('记：王总说月底签', [writer], false)).toMatchObject({ kind: 'note', title: '王总说月底签' });
    expect(quickAddPlan('记：王总说月底签', [writer], true)).toMatchObject({ kind: 'task', title: '记：王总说月底签' });
    expect(quickAddPlan('  写周报 ', [writer], false)).toMatchObject({ kind: 'task', title: '写周报' });
  });

  it('附件筛选：超过 50MB 的、第 21 个起的不收', () => {
    const big = new File(['x'], 'big.mp4');
    Object.defineProperty(big, 'size', { value: 60 * 1024 * 1024 });
    const small = new File(['x'], 'a.txt');
    expect(acceptFiles(0, [big, small])).toEqual({
      accepted: [small],
      rejected: [{ name: 'big.mp4', reason: 'tooLarge' }],
    });
    expect(acceptFiles(20, [small]).rejected).toEqual([{ name: 'a.txt', reason: 'tooMany' }]);
  });

  it('建任务 → 传附件 → 派出，并广播给开着的页面', async () => {
    api.createOfficeTask.mockResolvedValue(task({ title: '整理周报' }));
    api.uploadTaskFile.mockResolvedValueOnce({}).mockRejectedValueOnce(new Error('boom'));
    const assigned = task({ title: '整理周报', assignee_kind: 'agent', session_id: 's1' });
    const dispatch = vi.fn(async () => ({ ok: true as const, link: '/chat?session=s1', task: assigned }));
    const heard: unknown[] = [];
    const listener = (e: Event) => heard.push((e as CustomEvent).detail);
    window.addEventListener(COMMAND_DONE_EVENT, listener);
    const a = new File(['1'], 'a.csv');
    const b = new File(['2'], 'b.png');

    const outcome = await quickAdd('@写手 整理周报', [a, b], { recipients: [writer], dispatch });
    window.removeEventListener(COMMAND_DONE_EVENT, listener);

    expect(api.createOfficeTask).toHaveBeenCalledWith('整理周报', '', 'manual');
    expect(api.uploadTaskFile).toHaveBeenCalledTimes(2);
    expect(dispatch).toHaveBeenCalledWith(expect.objectContaining({ file_count: 1 }), writer);
    expect(outcome).toMatchObject({
      ok: true,
      kind: 'delegate',
      link: '/chat?session=s1',
      failedFiles: ['b.png（boom）'],
    });
    expect(heard).toEqual([{ intent: 'delegate', task: assigned }]);
  });

  it('派不出去时任务留在我的待办里（照样广播成任务）', async () => {
    api.createOfficeTask.mockResolvedValue(task({ title: '整理周报' }));
    const dispatch = vi.fn(async () => ({ ok: false as const, message: 'no model', link: '' }));
    const heard: unknown[] = [];
    const listener = (e: Event) => heard.push((e as CustomEvent).detail);
    window.addEventListener(COMMAND_DONE_EVENT, listener);
    const outcome = await quickAdd('@写手 整理周报', [], { recipients: [writer], dispatch });
    window.removeEventListener(COMMAND_DONE_EVENT, listener);
    expect(outcome).toEqual({ ok: false, message: 'no model', link: '' });
    expect(heard).toEqual([{ intent: 'task', task: expect.objectContaining({ title: '整理周报' }) }]);
  });
});
