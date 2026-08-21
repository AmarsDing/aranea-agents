import { describe, expect, it, vi } from 'vitest';
import { computed, ref } from 'vue';
import type { Agent } from '../types';
import { useAgentSettingsPersistence, type UseAgentSettingsPersistenceDeps } from '../useAgentSettingsPersistence';

function makeDeps(overrides: Partial<UseAgentSettingsPersistenceDeps> = {}) {
  const notify = vi.fn();
  const patch = vi.fn().mockResolvedValue({ id: 'a1' });
  const form = {
    id: 'a1',
    agent_key: 'demo',
    display_name: 'Demo',
    readonly: false,
  } as unknown as Agent;
  const deps: UseAgentSettingsPersistenceDeps = {
    form,
    $q: { notify },
    t: (key: string) => key,
    agentId: computed(() => 'a1'),
    detailStore: {
      fetchById: vi.fn().mockResolvedValue(null),
      patch,
    },
    appStore: { upsertAgent: vi.fn() },
    channelsStore: { loadChannels: vi.fn().mockResolvedValue(undefined) },
    selectedProviderModelID: ref('p/m'),
    orphanProviderModel: ref(false),
    loadProviderModels: vi.fn().mockResolvedValue(undefined),
    runAgentModelValidate: vi.fn().mockResolvedValue({ ok: true, message: '' }),
    validatePlannerFormState: () => null,
    serializePlannerFormState: () => ({}),
    hydratePlannerForm: vi.fn(),
    validateRalphLoopFormState: () => null,
    serializeRalphLoopFormState: () => ({}),
    hydrateRalphLoopForm: vi.fn(),
    hydrateSettings: vi.fn(),
    buildSettingsPayload: () => ({}),
    buildConfigJson: () => '{}',
    onAdvancedSave: vi.fn().mockResolvedValue(undefined),
    advancedState: {} as UseAgentSettingsPersistenceDeps['advancedState'],
    filesForSave: () => [],
    snapshotFiles: vi.fn(),
    hydrateFiles: vi.fn(),
    refreshFileTokenEstimates: vi.fn().mockResolvedValue(undefined),
    loadPromptPreview: vi.fn().mockResolvedValue(undefined),
    syncPreviewModeFromAgent: vi.fn(),
    primeThumbnailCache: vi.fn().mockResolvedValue(undefined),
    loadCatalogTools: vi.fn().mockResolvedValue(undefined),
    loadSkillSlugOptions: vi.fn().mockResolvedValue(undefined),
    loadSkillTagOptions: vi.fn().mockResolvedValue(undefined),
    loadCodeExecutorCapabilities: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  };
  return { deps, notify, patch, form };
}

describe('useAgentSettingsPersistence readonly guard', () => {
  it('allows saveAgent for readonly (built-in) agents', async () => {
    const { deps, notify, patch, form } = makeDeps();
    (form as { readonly: boolean }).readonly = true;
    const { saveAgent } = useAgentSettingsPersistence(deps);

    await saveAgent();

    expect(patch).toHaveBeenCalledTimes(1);
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'positive' }));
  });

  it('allows saveAgent for normal agents', async () => {
    const { deps, notify, patch } = makeDeps();
    const { saveAgent } = useAgentSettingsPersistence(deps);

    await saveAgent();

    expect(patch).toHaveBeenCalledTimes(1);
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'positive' }));
  });
});
