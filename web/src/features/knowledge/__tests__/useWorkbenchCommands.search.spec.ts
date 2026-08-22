import { describe, it, expect, vi, beforeEach } from 'vitest';
import { nextTick, ref } from 'vue';

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn(), dialog: vi.fn() }),
}));
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}));

const search = vi.fn().mockResolvedValue([]);
vi.mock('../../../stores/knowledge', () => ({
  useKnowledgeStore: () => ({ search }),
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
import type { KnowledgeWorkbench } from '../useKnowledgeWorkbench';

function deps(): WorkbenchCommandsDeps {
  return {
    workbench: { activeTab: ref(null), toggleMode: vi.fn() } as unknown as KnowledgeWorkbench,
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
