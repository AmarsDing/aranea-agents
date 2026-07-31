// useGraphsPage — M53 Phase 11 F5：Team badge / 归属过滤 / 打开 Team 编排跳转。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { useGraphsPage } from '../useGraphsPage';
import { useGraphStore } from '../../../stores/graph';
import { useTeamsStore } from '../../../stores/teams';
import type { GraphDefinition } from '../types';

// ── 外部依赖 mock ──
const routerPush = vi.hoisted(() => vi.fn());
const spies = vi.hoisted(() => ({
  loadGraphs: vi.fn(async () => {}),
  loadTeams: vi.fn(async () => {}),
}));

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: routerPush }),
}));
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));
vi.mock('quasar', () => ({
  useQuasar: () => ({
    dark: { isActive: false },
    notify: vi.fn(),
    dialog: () => ({ onOk: () => ({}) }),
  }),
}));

// storeToRefs 依赖真实 pinia setup store 的 ref 属性，用 defineStore 构造最小替身。
vi.mock('../../../stores/graph', async () => {
  const { defineStore } = await import('pinia');
  const { ref } = await import('vue');
  return {
    useGraphStore: defineStore('graph-f5-test', () => {
      const graphs = ref<GraphDefinition[]>([]);
      const loading = ref(false);
      const graphsNextPageToken = ref('');
      const templates = ref<unknown[]>([]);
      return {
        graphs,
        loading,
        graphsNextPageToken,
        templates,
        loadGraphs: spies.loadGraphs,
        addGraph: vi.fn(),
        removeGraph: vi.fn(),
        reorderGraphList: vi.fn(async () => {}),
        exportGraphDefinition: vi.fn(),
        loadTemplates: vi.fn(async () => {}),
        instantiateTemplate: vi.fn(),
      };
    }),
  };
});

vi.mock('../../../stores/teams', async () => {
  const { defineStore } = await import('pinia');
  const { ref } = await import('vue');
  return {
    useTeamsStore: defineStore('teams-f5-test', () => {
      const teams = ref([{ id: 'team-1', display_name: '阿尔法小队' }]);
      return { teams, loadTeams: spies.loadTeams };
    }),
  };
});

vi.mock('../useGraphExecute', () => ({
  useGraphExecute: () => ({
    runDialogOpen: { value: false },
    runSessionId: { value: '' },
    runInitialState: { value: '' },
    runLoading: { value: false },
    openRunDialog: vi.fn(),
    executeRun: vi.fn(),
  }),
}));
vi.mock('../api', () => ({ listGraphExecutions: vi.fn(async () => ({ items: [] })) }));

function makeGraph(partial: Partial<GraphDefinition>): GraphDefinition {
  return {
    id: 'g-x',
    name: 'graph',
    description: '',
    stateFields: [],
    nodes: [],
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: '',
    finishPoint: '',
    enableCheckpoint: true,
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 1,
    sortOrder: 0,
    createdAt: '2026-07-01T00:00:00Z',
    updatedAt: '2026-07-01T00:00:00Z',
    ...partial,
  };
}

function harness() {
  let api: ReturnType<typeof useGraphsPage> | undefined;
  const Comp = defineComponent({
    setup() {
      api = useGraphsPage();
      return () => null;
    },
  });
  mount(Comp);
  return api!;
}

const ownedGraph = makeGraph({ id: 'g-owned', name: 'owned', teamId: 'team-1', metadata: { team_owned: true } });
const linkedGraph = makeGraph({ id: 'g-linked', name: 'linked', teamId: 'team-1', metadata: {} });
const independentGraph = makeGraph({ id: 'g-indie', name: 'indie', teamId: '' });

describe('useGraphsPage F5 Team 归属 (M53 Phase 11)', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    routerPush.mockReset();
    spies.loadTeams.mockClear();
    const graphStore = useGraphStore();
    graphStore.graphs = [ownedGraph, linkedGraph, independentGraph];
  });

  it('onMounted best-effort 加载 teams 列表用于属主名映射', () => {
    harness();
    expect(spies.loadTeams).toHaveBeenCalled();
  });

  it('teamFilter：team 只保留 teamId 非空图；independent 只保留独立图；空值全部', () => {
    const api = harness();
    expect(api.filteredRows.value.map((g) => g.id).sort()).toEqual(['g-indie', 'g-linked', 'g-owned']);

    api.teamFilter.value = 'team';
    expect(api.filteredRows.value.map((g) => g.id).sort()).toEqual(['g-linked', 'g-owned']);

    api.teamFilter.value = 'independent';
    expect(api.filteredRows.value.map((g) => g.id)).toEqual(['g-indie']);
  });

  it('isTeamOwned 仅以 metadata.team_owned 判定，teamId 回填的 external 图不算 owned', () => {
    const api = harness();
    expect(api.isTeamOwned(ownedGraph)).toBe(true);
    expect(api.isTeamOwned(linkedGraph)).toBe(false);
    expect(api.isTeamOwned(independentGraph)).toBe(false);
  });

  it('teamDisplayName：命中 teams 列表返回 display_name，未命中回退 id，空 id 返回空串', () => {
    const teamsStore = useTeamsStore();
    const api = harness();
    expect(api.teamDisplayName('team-1')).toBe('阿尔法小队');
    expect(api.teamDisplayName('team-gone')).toBe('team-gone');
    expect(api.teamDisplayName('')).toBe('');
    teamsStore.teams = [];
    expect(api.teamDisplayName('team-1')).toBe('team-1');
  });

  it('Team 关联图上下文菜单含 open-team 项并跳转编排页；独立图无该项', () => {
    const api = harness();
    const evt = { preventDefault: vi.fn(), clientX: 0, clientY: 0 } as unknown as MouseEvent;

    api.onCardContextMenu(evt, ownedGraph);
    expect(api.ctxMenuItems.value.some((item) => item.action === 'open-team')).toBe(true);
    api.onCtxMenuAction('open-team');
    expect(routerPush).toHaveBeenCalledWith({ name: 'team-orchestrate', params: { teamId: 'team-1' } });

    routerPush.mockReset();
    api.onCardContextMenu(evt, independentGraph);
    expect(api.ctxMenuItems.value.some((item) => item.action === 'open-team')).toBe(false);
    api.onCtxMenuAction('open-team');
    expect(routerPush).not.toHaveBeenCalled();
  });
});
