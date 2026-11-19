import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent } from 'vue';
import { mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { decideApproval, listApprovals } from '../api';
import {
  isApprovalTypingTarget,
  resolveApprovalShortcut,
  useApprovalsPage,
} from '../useApprovalsPage';
import type { ApprovalItemRow } from '../types';

vi.mock('quasar', () => ({ useQuasar: () => ({ notify: vi.fn() }) }));
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

function item(id: string): ApprovalItemRow {
  return {
    id,
    kind: 'task_review',
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
  mount(defineComponent({
    setup() {
      page = useApprovalsPage();
      return () => null;
    },
  }));
  return page;
}

async function flush() {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
}

describe('approvals continuous keyboard flow', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('moves selection without leaving the queue and selects the next row after a decision', async () => {
    let rows = [item('a'), item('b'), item('c')];
    vi.mocked(listApprovals).mockImplementation(async () => ({ items: rows, total: rows.length }));
    const page = setupPage();
    await flush();

    page.select(rows[1]);
    page.selectNext();
    expect(page.selected.value?.id).toBe('c');
    page.selectPrevious();
    expect(page.selected.value?.id).toBe('b');

    rows = [item('a'), item('c')];
    await page.decide('approve');
    expect(decideApproval).toHaveBeenCalledOnce();
    expect(page.selected.value?.id).toBe('c');
  });

  it('does not reject without a reason', async () => {
    const rows = [item('a')];
    vi.mocked(listApprovals).mockImplementation(async () => ({ items: rows, total: rows.length }));
    const page = setupPage();
    await flush();
    page.select(rows[0]);

    await page.decide('reject');
    expect(decideApproval).not.toHaveBeenCalled();
    expect(page.selected.value?.id).toBe('a');
  });
});

describe('approval shortcut guards', () => {
  it('maps queue keys and leaves ordinary typing untouched', () => {
    const textarea = document.createElement('textarea');
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'j' }))).toBe('next');
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'n' }))).toBe('next');
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'k' }))).toBe('previous');
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'a' }))).toBe('approve');
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'r' }))).toBe('reject');
    expect(isApprovalTypingTarget(textarea)).toBe(true);
    expect(resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'a' }), textarea)).toBeNull();
    expect(
      resolveApprovalShortcut(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true }), textarea),
    ).toBe('reject');
  });
});
