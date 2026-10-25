import { describe, it, expect, vi, beforeEach } from 'vitest';
import { nextTick, ref } from 'vue';

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn(), dialog: vi.fn() }),
}));
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}));

const search = vi.fn().mockResolvedValue([]);
const loadDocument = vi.fn();
const addVaultDocument = vi.fn();
vi.mock('../../../stores/knowledge', () => ({
  useKnowledgeStore: () => ({ search, loadDocument, addVaultDocument, documentsByCollection: {} }),
}));
vi.mock('../api', () => ({
  applyOutgoingAutolink: vi.fn(),
  applyWriteBackPending: vi.fn(),
  backfillAutolinkIndex: vi.fn(),
  getCollectionHealth: vi.fn(),
  getWriteBackHome: vi.fn(),
  listCollectionExperts: vi.fn(),
  listGovernanceProposals: vi.fn(),
  listRecentLinkUses: vi.fn().mockResolvedValue([]),
  listWriteBackPending: vi.fn(),
  previewOutgoingAutolink: vi.fn(),
  recordLinkUse: vi.fn(),
  rebuildKnowledgeIndex: vi.fn(),
  resolveGovernanceProposal: vi.fn(),
}));

import { useWorkbenchCommands, type WorkbenchCommandsDeps } from '../useWorkbenchCommands';
import type { KnowledgeWorkbench, WorkbenchTab } from '../useKnowledgeWorkbench';

function deps(active: WorkbenchTab | null = null): WorkbenchCommandsDeps {
  return {
    workbench: {
      activeTab: ref(active),
      toggleMode: vi.fn(),
      openDoc: vi.fn().mockResolvedValue(undefined),
    } as unknown as KnowledgeWorkbench,
    currentVaultId: ref('vault-1'),
    currentPrefix: ref(''),
    documents: ref([]),
    collections: ref([]),
    events: {
      refreshTree: vi.fn(),
      switchVault: vi.fn(),
      openGraph: vi.fn(),
      promoteActive: vi.fn(),
      ingestText: vi.fn(),
      saveDoc: vi.fn().mockResolvedValue(true),
    },
  };
}

describe('useWorkbenchCommands search US-14', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    search.mockResolvedValue([]);
  });

  it('全库搜索不绑定当前 vault', async () => {
    vi.useFakeTimers();
    const cmds = useWorkbenchCommands(deps());
    cmds.searchQuery.value = 'alpha';
    await vi.advanceTimersByTimeAsync(300);
    await nextTick();
    expect(search).toHaveBeenCalledWith({ query: 'alpha', top_k: 12 });
    vi.useRealTimers();
  });
});

describe('workbench cross-vault regressions', () => {
  it('opens a cross-vault hit using freshly authorized metadata and switches to the owning vault', async () => {
    const input = deps();
    const doc = { id: 'remote-doc', collection_id: 'vault-2', source: 'Report', rel_path: 'docs/report.md' };
    loadDocument.mockResolvedValue(doc);
    const commands = useWorkbenchCommands(input);
    await commands.onSearchPick({
      docId: 'remote-doc',
      chunk: { collection_id: 'vault-2' },
      name: 'Report',
      path: '',
      snippet: '',
      score: 1,
    } as Parameters<typeof commands.onSearchPick>[0]);
    expect(loadDocument).toHaveBeenCalledWith('remote-doc');
    expect(input.events.switchVault).toHaveBeenCalledWith('vault-2');
    expect(input.workbench.openDoc).toHaveBeenCalledWith(doc);
  });

  it('creates a dangling link in the active note vault and directory after the sidebar changes vault', async () => {
    const input = deps({
      docId: 'a',
      collectionId: 'vault-A',
      relPath: 'guides/source.md',
    } as WorkbenchTab);
    input.currentVaultId.value = 'vault-B';
    input.currentPrefix.value = 'unrelated/';
    addVaultDocument.mockResolvedValue({ id: 'created', collection_id: 'vault-A', rel_path: 'guides/target.md' });
    const commands = useWorkbenchCommands(input);
    await commands.createDocByName('target');
    expect(addVaultDocument).toHaveBeenCalledWith('vault-A', 'guides/target.md');
  });
});
