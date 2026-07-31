// useTeamOrchestratePage — M53 Phase 11 F3：「在 Graph 编辑器中打开」目标图资产解析。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import { useTeamOrchestratePage } from '../useTeamOrchestratePage';

// ── 外部依赖 mock：router / quasar / stores / stream ──
const routerPush = vi.hoisted(() => vi.fn());
const notify = vi.hoisted(() => vi.fn());
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { teamId: 'team-1' } }),
  useRouter: () => ({ push: routerPush }),
}));
vi.mock('quasar', () => ({
  useQuasar: () => ({ dark: { isActive: false }, notify }),
}));

const graphStoreMock = vi.hoisted(() => ({
  graphs: [] as Array<{ id: string; teamId: string; metadata?: Record<string, unknown> }>,
  loadGraphs: vi.fn(async () => {}),
}));
vi.mock('../../../stores/graph', () => ({ useGraphStore: () => graphStoreMock }));

const teamsStoreMock = vi.hoisted(() => ({ fetchTeam: vi.fn() }));
vi.mock('../../../stores/teams', () => ({ useTeamsStore: () => teamsStoreMock }));

const orchestrationStoreMock = vi.hoisted(() => ({
  compileTeam: vi.fn(async () => ({
    template_id: 't',
    mode: 'sequential',
    entry_point: '',
    finish_point: '',
    nodes: [],
    edges: [],
    conditional_edges: [],
    graph_json: '',
    issues: [],
    valid: true,
    definition_graph_json: '',
  })),
  fetchRunObservatory: vi.fn(),
}));
vi.mock('../../../stores/orchestration', () => ({ useOrchestrationStore: () => orchestrationStoreMock }));

vi.mock('../api', () => ({ findActiveTeamRun: vi.fn(async () => null) }));
vi.mock('../../orchestration/useOrchestrationStream', () => ({ useOrchestrationStream: vi.fn() }));

function mkTeam(definitionJson: string) {
  return {
    id: 'team-1',
    team_key: 'demo',
    display_name: 'Demo',
    status: 'active',
    is_default: false,
    taxonomy_industry_id: '',
    definition_json: definitionJson,
    app_name: 'demo',
    linked_graph_id: '',
    has_active_run: false,
    created_at: '',
    updated_at: '',
    deleted_at: '',
  };
}

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': {} } });

function harness() {
  let api: ReturnType<typeof useTeamOrchestratePage> | undefined;
  const Comp = defineComponent({
    setup() {
      api = useTeamOrchestratePage();
      return () => null;
    },
  });
  mount(Comp, { global: { plugins: [i18n] } });
  return api!;
}

describe('useTeamOrchestratePage.openInGraphEditor (M53 Phase 11 F3)', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    routerPush.mockReset();
    notify.mockReset();
    graphStoreMock.graphs = [];
    // mockReset 同时清掉上一用例的 mockImplementation，恢复默认空加载
    graphStoreMock.loadGraphs.mockReset().mockImplementation(async () => {});
  });

  it('linked_external team navigates to the linked graph id', async () => {
    teamsStoreMock.fetchTeam.mockResolvedValue(
      mkTeam(
        JSON.stringify({
          version: 1,
          source: 'linked_external',
          linked_graph_id: 'g-ext',
          mode: 'sequential',
          members: [],
        }),
      ),
    );
    const api = harness();
    await vi.waitFor(() => expect(teamsStoreMock.fetchTeam).toHaveBeenCalled());
    await api.openInGraphEditor();
    expect(routerPush).toHaveBeenCalledWith({ name: 'graph-editor', params: { id: 'g-ext' } });
  });

  it('preset/custom team resolves team-owned graph by teamId (loading list on demand)', async () => {
    teamsStoreMock.fetchTeam.mockResolvedValue(mkTeam(JSON.stringify({ version: 1, mode: 'sequential', members: [] })));
    graphStoreMock.loadGraphs.mockImplementation(async () => {
      graphStoreMock.graphs = [
        { id: 'g-other', teamId: 'team-9', metadata: { team_owned: true } },
        { id: 'g-owned', teamId: 'team-1', metadata: { team_owned: true } },
        { id: 'g-independent', teamId: '', metadata: {} },
      ];
    });
    const api = harness();
    await vi.waitFor(() => expect(teamsStoreMock.fetchTeam).toHaveBeenCalled());
    await api.openInGraphEditor();
    expect(graphStoreMock.loadGraphs).toHaveBeenCalled();
    expect(routerPush).toHaveBeenCalledWith({ name: 'graph-editor', params: { id: 'g-owned' } });
  });

  it('notifies and does not navigate when no graph asset exists yet', async () => {
    teamsStoreMock.fetchTeam.mockResolvedValue(mkTeam(JSON.stringify({ version: 1, mode: 'sequential', members: [] })));
    const api = harness();
    await vi.waitFor(() => expect(teamsStoreMock.fetchTeam).toHaveBeenCalled());
    await api.openInGraphEditor();
    expect(routerPush).not.toHaveBeenCalled();
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'warning' }));
  });
});
