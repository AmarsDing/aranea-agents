import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent } from 'vue';
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { decideApproval, isConflictError, listApprovals } from '../api';
import { resolveApprovalShortcut, useApprovalsPage } from '../useApprovalsPage';
import { canDecideApproval, type ApprovalItemRow, type ApprovalKindKey } from '../types';

const notify = vi.hoisted(() => vi.fn());

vi.mock('quasar', () => ({ useQuasar: () => ({ notify }) }));
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock('../../../stores/auth', () => ({ useAuthStore: () => ({ user: { id: 7 } }) }));
vi.mock('../../../stores/nav', () => ({
  useNavStore: () => ({ permissions: [], ensureMenus: vi.fn() }),
}));
vi.mock('../api', () => ({
  decideApproval: vi.fn(async () => undefined),
  getApprovalSummary: vi.fn(async () => ({ counts: {}, total: 0 })),
  getRiskMatrix: vi.fn(async () => []),
  isConflictError: vi.fn(() => false),
  listApprovals: vi.fn(),
  putRiskMatrix: vi.fn(async () => []),
}));

function item(id: string, kind: ApprovalKindKey = 'task_review'): ApprovalItemRow {
  return {
    id,
    kind,
    title: `审批 ${id}`,
    summary: '',
    requester: 'tester',
    risk_level: 'low',
    context: null,
    context_raw: '',
    agent_key: '',
    created_at: '2026-11-10T00:00:00Z',
  };
}

function setupPage() {
  let page!: ReturnType<typeof useApprovalsPage>;
  mount(
    defineComponent({
      setup() {
        page = useApprovalsPage();
        return () => null;
      },
    }),
  );
  return page;
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

function mockRows(rows: ApprovalItemRow[]) {
  vi.mocked(listApprovals).mockImplementation(async () => ({ items: rows, total: rows.length }));
}

describe('decision dock bulk selection', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    vi.mocked(isConflictError).mockReturnValue(false);
  });

  it('only selects kinds that have a decision endpoint', async () => {
    const rows = [item('a'), item('b', 'mask_proposal'), item('c', 'skill_approval')];
    mockRows(rows);
    const page = setupPage();
    await flush();

    expect(canDecideApproval('mask_proposal')).toBe(false);
    expect(canDecideApproval('skill_approval', 'archive')).toBe(false);

    page.selectAllVisible();
    expect(page.selectedKeys.value).toEqual(['task_review:a', 'skill_approval:c']);
    expect(page.allVisibleChecked.value).toBe(true);

    page.clearSelection();
    expect(page.selectedKeys.value).toEqual([]);
  });

  it('toggles a single row and ignores unsupported kinds', async () => {
    const rows = [item('a'), item('b', 'memory_fact')];
    mockRows(rows);
    const page = setupPage();
    await flush();

    page.toggleSelect(rows[0]);
    expect(page.isChecked(rows[0])).toBe(true);
    page.toggleSelect(rows[0]);
    expect(page.isChecked(rows[0])).toBe(false);

    page.toggleSelect(rows[1]);
    expect(page.selectedKeys.value).toEqual([]);
  });

  it('prunes checked keys that leave the visible page', async () => {
    let rows = [item('a'), item('b')];
    mockRows(rows);
    const page = setupPage();
    await flush();
    page.selectAllVisible();
    expect(page.selectedKeys.value).toHaveLength(2);

    rows = [item('b')];
    mockRows(rows);
    await page.loadList();
    expect(page.selectedKeys.value).toEqual(['task_review:b']);
  });
});

describe('decision dock batch decisions', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    vi.mocked(isConflictError).mockReturnValue(false);
  });

  it('records done / conflict / failed per item and keeps failed items selected for retry', async () => {
    const rows = [item('a'), item('b'), item('c')];
    mockRows(rows);
    vi.mocked(decideApproval)
      .mockResolvedValueOnce(undefined)
      .mockRejectedValueOnce(new Error('409 conflict'))
      .mockRejectedValueOnce(new Error('boom'));
    vi.mocked(isConflictError).mockImplementation((error) => error instanceof Error && error.message.includes('409'));

    const page = setupPage();
    await flush();
    page.selectAllVisible();
    await page.batchDecide('approve');

    expect(page.outcomes.value.map((o) => [o.key, o.status])).toEqual([
      ['task_review:a', 'done'],
      ['task_review:b', 'conflict'],
      ['task_review:c', 'failed'],
    ]);
    expect(page.failedOutcomes.value.map((o) => o.key)).toEqual(['task_review:c']);
    // 成功与冲突移出勾选，失败保留可直接重试。
    expect(page.selectedKeys.value).toEqual(['task_review:c']);
    expect(page.batchDone.value).toBe(3);
    expect(page.batchTotal.value).toBe(3);
  });

  it('refuses batch reject without a shared reason', async () => {
    const rows = [item('a')];
    mockRows(rows);
    const page = setupPage();
    await flush();
    page.selectAllVisible();

    await page.batchDecide('reject');
    expect(decideApproval).not.toHaveBeenCalled();
    expect(page.selectedKeys.value).toEqual(['task_review:a']);
  });

  it('retries only the failed items without resubmitting successful ones', async () => {
    const rows = [item('a'), item('b')];
    mockRows(rows);
    vi.mocked(decideApproval).mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('boom'));

    const page = setupPage();
    await flush();
    page.selectAllVisible();
    await page.batchDecide('approve');
    expect(decideApproval).toHaveBeenCalledTimes(2);

    vi.mocked(decideApproval).mockResolvedValueOnce(undefined);
    await page.retryFailed();

    expect(decideApproval).toHaveBeenCalledTimes(3);
    expect(vi.mocked(decideApproval).mock.calls[2][0].item.id).toBe('b');
    expect(page.outcomes.value).toHaveLength(1);
    expect(page.outcomes.value[0].status).toBe('done');
    expect(page.selectedKeys.value).toEqual([]);
  });

  it('keeps the selected item and reason when a single decision fails', async () => {
    const rows = [item('a')];
    mockRows(rows);
    vi.mocked(decideApproval).mockRejectedValueOnce(new Error('boom'));

    const page = setupPage();
    await flush();
    page.select(rows[0]);
    page.note.value = '材料待补';

    await page.decide('reject');
    expect(page.selected.value?.id).toBe('a');
    expect(page.note.value).toBe('材料待补');
    expect(page.deciding.value).toBe(false);
  });
});

describe('decision dock keyboard contract', () => {
  it('adds selection keys without breaking existing queue shortcuts', () => {
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'x' }))).toBe('toggleSelect');
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'Escape' }))).toBe('clearSelection');
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'a', ctrlKey: true }))).toBe('selectAll');
    // 输入框内 Ctrl+A 保留原生全选
    const input = document.createElement('input');
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'a', ctrlKey: true }), input)).toBeNull();
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'a' }))).toBe('approve');
  });
});
