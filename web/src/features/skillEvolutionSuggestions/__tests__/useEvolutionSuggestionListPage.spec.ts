// useEvolutionSuggestionListPage — P4：审批成功后按 500ms/3s/8s 轮询刷新，
// 使异步 applier（approved → applied）的最终状态无需手动刷新即可落表；
// 卸载后清理未触发定时器；状态筛选含 expired 项。
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import {
  useEvolutionSuggestionListPage,
  statusOptions,
} from '../useEvolutionSuggestionListPage';
import type { SkillEvolutionView } from '../../skills/types';

const listUnifiedEvolutionSuggestions = vi.hoisted(() =>
  vi.fn(async () => ({ items: [], total: 0, skillTotal: 0, agentTotal: 0 })),
);
const approveUnifiedEvolutionSuggestion = vi.hoisted(() => vi.fn(async () => {}));

vi.mock('../../skills/api', () => ({
  listUnifiedEvolutionSuggestions,
  approveUnifiedEvolutionSuggestion,
  rejectUnifiedEvolutionSuggestion: vi.fn(async () => {}),
  triggerCuratorFlow: vi.fn(async () => {}),
}));

vi.mock('../../../stores/auth', () => ({
  useAuthStore: () => ({ displayLabel: 'tester' }),
}));

function setupPage() {
  let page!: ReturnType<typeof useEvolutionSuggestionListPage>;
  const wrapper = mount(
    defineComponent({
      setup() {
        page = useEvolutionSuggestionListPage();
        return () => null;
      },
    }),
  );
  return { page, wrapper };
}

const pendingItem = { id: 'sug-1', status: 'pending' } as SkillEvolutionView;

describe('useEvolutionSuggestionListPage P4', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    setActivePinia(createPinia());
    listUnifiedEvolutionSuggestions.mockClear();
    approveUnifiedEvolutionSuggestion.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('statusOptions 包含 expired 筛选项', () => {
    const values = statusOptions.map((o) => o.value);
    expect(values).toContain('expired');
    const expired = statusOptions.find((o) => o.value === 'expired');
    expect(expired?.label).toBe('已过期');
  });

  it('审批成功后按 500ms/3s/8s 轮询刷新', async () => {
    const { page } = setupPage();
    expect(listUnifiedEvolutionSuggestions).toHaveBeenCalledTimes(1); // onMounted

    await page.approveSuggestion(pendingItem);
    expect(approveUnifiedEvolutionSuggestion).toHaveBeenCalledWith('sug-1', 'tester');

    await vi.advanceTimersByTimeAsync(500);
    expect(listUnifiedEvolutionSuggestions).toHaveBeenCalledTimes(2);

    await vi.advanceTimersByTimeAsync(2500);
    expect(listUnifiedEvolutionSuggestions).toHaveBeenCalledTimes(3);

    await vi.advanceTimersByTimeAsync(5000);
    expect(listUnifiedEvolutionSuggestions).toHaveBeenCalledTimes(4);

    // 之后不再有额外刷新
    await vi.advanceTimersByTimeAsync(30000);
    expect(listUnifiedEvolutionSuggestions).toHaveBeenCalledTimes(4);
  });

  it('组件卸载后未触发的轮询定时器被清理', async () => {
    const { page, wrapper } = setupPage();
    await page.approveSuggestion(pendingItem);

    await vi.advanceTimersByTimeAsync(500);
    expect(listUnifiedEvolutionSuggestions).toHaveBeenCalledTimes(2);

    wrapper.unmount();
    await vi.advanceTimersByTimeAsync(30000);
    expect(listUnifiedEvolutionSuggestions).toHaveBeenCalledTimes(2);
  });
});
