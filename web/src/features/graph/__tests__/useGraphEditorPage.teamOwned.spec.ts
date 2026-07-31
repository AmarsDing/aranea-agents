// useGraphEditorPage — M53 Phase 11 F4：team-owned 图保存确认。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import { useGraphEditorPage } from '../useGraphEditorPage';

// ── 外部依赖 mock ──
const routerReplace = vi.hoisted(() => vi.fn());
const notify = vi.hoisted(() => vi.fn());
const dialogOnOk = vi.hoisted(() => ({ current: null as null | (() => void) }));
vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'graph-editor', params: { id: 'g-1' } }),
  useRouter: () => ({ push: vi.fn(), replace: routerReplace }),
  onBeforeRouteLeave: vi.fn(),
}));
vi.mock('quasar', () => ({
  useQuasar: () => ({
    dark: { isActive: false },
    notify,
    dialog: () => ({
      onOk: (cb: () => void) => {
        dialogOnOk.current = cb;
      },
      onCancel: () => ({}),
    }),
  }),
}));

const graphStoreMock = vi.hoisted(() => ({
  fetchGraph: vi.fn(),
  editGraph: vi.fn(),
  addGraph: vi.fn(),
  validateGraphDefinition: vi.fn(async () => ({ valid: true, errors: [], warnings: [] })),
  templates: [],
  templatesLoading: false,
}));
vi.mock('../../../stores/graph', () => ({ useGraphStore: () => graphStoreMock }));
const toolsStoreMock = vi.hoisted(() => ({ loadTools: vi.fn(async () => ({ items: [] })) }));
vi.mock('../../../stores/tools', () => ({ useToolsStore: () => toolsStoreMock }));
const teamsStoreMock = vi.hoisted(() => ({ fetchTeam: vi.fn() }));
vi.mock('../../../stores/teams', () => ({ useTeamsStore: () => teamsStoreMock }));

// 子 composable 保持最小化（不相关于保存确认流）
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
vi.mock('../useGraphEditorAssets', () => ({
  useGraphEditorAssets: () => ({
    versionDialogOpen: { value: false },
    versions: { value: [] },
    versionsLoading: { value: false },
    rollingBackVersion: { value: false },
    templateDialogOpen: { value: false },
    templateName: { value: '' },
    templateCategory: { value: '' },
    templateSaving: { value: false },
    importInputRef: { value: null },
    exportCurrentGraph: vi.fn(),
    triggerImport: vi.fn(),
    onImportFile: vi.fn(),
    openTemplateDialog: vi.fn(),
    saveTemplate: vi.fn(),
    openVersionDialog: vi.fn(),
    rollbackVersion: vi.fn(),
  }),
}));
vi.mock('../useGraphUndoRedo', () => ({
  useGraphUndoRedo: () => ({
    canUndo: { value: false },
    canRedo: { value: false },
    undo: vi.fn(),
    redo: vi.fn(),
    clear: vi.fn(),
    pushMoveNodes: vi.fn(),
  }),
}));
vi.mock('../useGraphLocalValidation', () => ({
  useGraphLocalValidation: () => ({ localErrors: { value: [] }, localWarnings: { value: [] } }),
}));
vi.mock('../useGraphValidationDock', () => ({
  useGraphValidationDock: () => ({ dock: { value: null } }),
}));

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': {} } });

function harness() {
  let api: ReturnType<typeof useGraphEditorPage> | undefined;
  const Comp = defineComponent({
    setup() {
      api = useGraphEditorPage();
      return () => null;
    },
  });
  mount(Comp, { global: { plugins: [i18n] } });
  return api!;
}

describe('useGraphEditorPage F4 team-owned 保存确认 (M53 Phase 11)', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    dialogOnOk.current = null;
    graphStoreMock.editGraph.mockReset().mockImplementation(async (_id: string, g: unknown) => g);
    teamsStoreMock.fetchTeam.mockReset();
    // onMounted 的 loadGraphDefinition 用一个普通独立图，避免干扰用例
    graphStoreMock.fetchGraph.mockReset().mockResolvedValue({
      id: 'g-1',
      name: '独立图',
      nodes: [{ id: 'n1' }],
      teamId: '',
      metadata: {},
    });
  });

  it('独立图保存不弹确认，直接调用 editGraph', async () => {
    const api = harness();
    await vi.waitFor(() => expect(graphStoreMock.fetchGraph).toHaveBeenCalled());
    api.graphDef.metadata = {};
    api.markDirty();
    api.onSaveClick();
    await vi.waitFor(() => expect(graphStoreMock.editGraph).toHaveBeenCalled());
    expect(dialogOnOk.current).toBeNull();
  });

  it('team-owned 图保存先弹确认，onOk 后才落库；确认文案含属主 Team 名', async () => {
    teamsStoreMock.fetchTeam.mockResolvedValue({ id: 'team-1', display_name: '阿尔法小队' });
    const api = harness();
    await vi.waitFor(() => expect(graphStoreMock.fetchGraph).toHaveBeenCalled());
    api.graphDef.teamId = 'team-1';
    api.graphDef.metadata = { team_owned: true };
    api.markDirty();

    api.onSaveClick();
    await vi.waitFor(() => expect(dialogOnOk.current).not.toBeNull());
    expect(graphStoreMock.editGraph).not.toHaveBeenCalled();

    dialogOnOk.current!();
    await vi.waitFor(() => expect(graphStoreMock.editGraph).toHaveBeenCalledWith('g-1', expect.anything()));
  });

  it('属主 Team 查询失败时回退 teamId 且流程不中断', async () => {
    teamsStoreMock.fetchTeam.mockRejectedValue(new Error('Team not found'));
    const api = harness();
    await vi.waitFor(() => expect(graphStoreMock.fetchGraph).toHaveBeenCalled());
    api.graphDef.teamId = 'team-gone';
    api.graphDef.metadata = { team_owned: true };
    api.markDirty();

    api.onSaveClick();
    await vi.waitFor(() => expect(dialogOnOk.current).not.toBeNull());
    dialogOnOk.current!();
    await vi.waitFor(() => expect(graphStoreMock.editGraph).toHaveBeenCalled());
  });
});
