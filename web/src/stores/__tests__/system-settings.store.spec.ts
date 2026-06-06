import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSystemSettingsStore } from '../system-settings';

vi.mock('../../features/system-settings/api', () => ({
  getSystemSettings: vi.fn().mockResolvedValue(null),
  updateSystemSettings: vi.fn(),
  testWebResearch: vi.fn(),
}));

describe('useSystemSettingsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('initialises with null settings and loading false', () => {
    const store = useSystemSettingsStore();
    expect(store.settings).toBeNull();
    expect(store.loading).toBe(false);
  });

  it('loadSettings populates settings and sets loading to false', async () => {
    const { getSystemSettings } = await import('../../features/system-settings/api');
    const mockSettings = {
      rootDirectory: '/data',
      workDirectory: '/data/work',
      globalMonthlyMicroUsd: 0,
    };
    (getSystemSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce(mockSettings);

    const store = useSystemSettingsStore();
    expect(store.loading).toBe(false);

    await store.loadSettings();

    expect(store.settings).toEqual(mockSettings);
    expect(store.loading).toBe(false);
  });

  it('loadSettings sets loading to false even on error', async () => {
    const { getSystemSettings } = await import('../../features/system-settings/api');
    (getSystemSettings as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network'));

    const store = useSystemSettingsStore();
    await expect(store.loadSettings()).rejects.toThrow('network');
    expect(store.loading).toBe(false);
  });

  it('saveAll updates settings via API and returns result', async () => {
    const { updateSystemSettings } = await import('../../features/system-settings/api');
    const updatedSettings = {
      rootDirectory: '/new-data',
      workDirectory: '/new-data/work',
      globalMonthlyMicroUsd: 100,
    };
    (updateSystemSettings as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updatedSettings);

    const store = useSystemSettingsStore();
    const result = await store.saveAll({
      rootDirectory: '/new-data',
      workDirectory: '/new-data/work',
      globalMonthlyMicroUsd: 100,
    });

    expect(updateSystemSettings).toHaveBeenCalledWith({
      rootDirectory: '/new-data',
      workDirectory: '/new-data/work',
      globalMonthlyMicroUsd: 100,
    });
    expect(store.settings).toEqual(updatedSettings);
    expect(result).toEqual(updatedSettings);
  });

  it('testWebResearchConnection delegates to testWebResearch', async () => {
    const { testWebResearch } = await import('../../features/system-settings/api');
    const testResult = { ok: true, message: 'OK', provider: 'tavily', resultCount: 5, latencyMs: 200 };
    (testWebResearch as ReturnType<typeof vi.fn>).mockResolvedValueOnce(testResult);

    const store = useSystemSettingsStore();
    const result = await store.testWebResearchConnection({
      provider: 'tavily',
      apiKey: 'test-key',
    });

    expect(testWebResearch).toHaveBeenCalledWith({
      provider: 'tavily',
      apiKey: 'test-key',
    });
    expect(result.ok).toBe(true);
    expect(result.resultCount).toBe(5);
  });

  it('loadSettings sets loading to true during fetch', async () => {
    const { getSystemSettings } = await import('../../features/system-settings/api');
    let resolvePromise: (v: any) => void;
    const pending = new Promise((resolve) => {
      resolvePromise = resolve;
    });
    (getSystemSettings as ReturnType<typeof vi.fn>).mockReturnValueOnce(pending);

    const store = useSystemSettingsStore();
    const loadPromise = store.loadSettings();

    expect(store.loading).toBe(true);

    resolvePromise!({});
    await loadPromise;

    expect(store.loading).toBe(false);
  });
});
