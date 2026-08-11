// useTeamsPage — UI-2：runEventsConnected 应实时派生自 WS 流 connected ref，
// 而非等到首个匹配事件到达（修复空闲 Team 恒显示「未连接」）。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { defineComponent, ref, nextTick } from 'vue';
import { useTeamsPage } from '../useTeamsPage';
import { useTeamsStore } from '../../../stores/teams';
import type { Team } from '../types';

const subscribeTeamRunEventsWs = vi.hoisted(() => vi.fn());
vi.mock('../api', () => ({
  listTeams: vi.fn(async () => []),
  createTeam: vi.fn(),
  updateTeam: vi.fn(),
  duplicateTeam: vi.fn(),
  deleteTeam: vi.fn(),
  retryTeam: vi.fn(),
  listTeamRuns: vi.fn(async () => []),
  listTeamRunSteps: vi.fn(async () => []),
  getTeamRunSummary: vi.fn(),
  runTeamTest: vi.fn(),
  subscribeTeamRunEventsWs,
  findActiveTeamRun: vi.fn(async () => null),
  listTaskDeadLetters: vi.fn(async () => []),
  resolveTaskDeadLetter: vi.fn(),
}));
vi.mock('../../agents/api', () => ({ listAgents: vi.fn(async () => []) }));

vi.mock('quasar', () => ({
  useQuasar: () => ({ dark: { isActive: false }, notify: vi.fn(), dialog: vi.fn() }),
}));
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock('../../../stores/platform', () => ({
  usePlatformStore: () => ({ loadTaxonomyTree: vi.fn(async () => {}), taxonomyTree: [] }),
}));
vi.mock('../../../stores/graph', () => ({
  useGraphStore: () => ({ graphs: [], loadGraphs: vi.fn(async () => {}) }),
}));
vi.mock('../../orchestration/compileApi', () => ({ compileTeamGraph: vi.fn() }));

function setupPage() {
  let page!: ReturnType<typeof useTeamsPage>;
  mount(
    defineComponent({
      setup() {
        page = useTeamsPage();
        return () => null;
      },
    }),
  );
  return page;
}

const team = { id: 't1', team_key: 'demo', display_name: 'Demo' } as unknown as Team;

describe('useTeamsPage UI-2 实时连接状态', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    subscribeTeamRunEventsWs.mockReset();
  });

  it('runEventsConnected 跟随 WS 流 connected 翻转，无需等待首个事件', async () => {
    const connected = ref(false);
    subscribeTeamRunEventsWs.mockReturnValue({ close: vi.fn(), connected });
    const page = setupPage();
    vi.spyOn(useTeamsStore(), 'loadRuns').mockResolvedValue();

    await page.openRuns(team);
    expect(subscribeTeamRunEventsWs).toHaveBeenCalledTimes(1);
    expect(page.runEventsConnected.value).toBe(false);

    connected.value = true;
    await nextTick();
    expect(page.runEventsConnected.value).toBe(true);

    connected.value = false;
    await nextTick();
    expect(page.runEventsConnected.value).toBe(false);
  });

  it('关闭运行轨迹面板时关闭流并回落为未连接', async () => {
    const connected = ref(true);
    const close = vi.fn();
    subscribeTeamRunEventsWs.mockReturnValue({ close, connected });
    const page = setupPage();
    vi.spyOn(useTeamsStore(), 'loadRuns').mockResolvedValue();

    await page.openRuns(team);
    expect(page.runEventsConnected.value).toBe(true);

    page.runsOpen.value = false;
    await nextTick();
    expect(close).toHaveBeenCalledTimes(1);
    expect(page.runEventsConnected.value).toBe(false);
  });
});
