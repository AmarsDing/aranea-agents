import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useAvatarCatalogStore } from '../avatar';

vi.mock('../../features/avatar/api', () => ({
  listAvatarAssets: vi.fn().mockResolvedValue([]),
  getAvatarThumbnailDataUrl: vi.fn().mockResolvedValue('data:image/png;base64,abc'),
  uploadAvatarAsset: vi.fn(),
  refreshChannelPlatformIcons: vi.fn().mockResolvedValue({ updated: 0, failed: 0 }),
}));

vi.mock('../../features/avatar/iconModel', () => ({
  isAvatarAssetRef: vi.fn().mockReturnValue(true),
}));

describe('useAvatarCatalogStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('ensureAgentsCatalog fills agentsCatalog on first call', async () => {
    const { listAvatarAssets } = await import('../../features/avatar/api');
    const mockAssets = [
      { id: 'av1', key: 'icon1', name: 'Icon 1', description: '', mime_type: 'image/png' },
      { id: 'av2', key: 'icon2', name: 'Icon 2', description: '', mime_type: 'image/png' },
    ];
    (listAvatarAssets as ReturnType<typeof vi.fn>).mockResolvedValueOnce(mockAssets);

    const store = useAvatarCatalogStore();
    await store.ensureAgentsCatalog();

    expect(store.agentsCatalog).toHaveLength(2);
    expect(store.agentsCatalogLoaded).toBe(true);
  });

  it('ensureAgentsCatalog skips on subsequent calls', async () => {
    const { listAvatarAssets } = await import('../../features/avatar/api');
    (listAvatarAssets as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);

    const store = useAvatarCatalogStore();
    await store.ensureAgentsCatalog();
    await store.ensureAgentsCatalog();

    expect(listAvatarAssets).toHaveBeenCalledTimes(1);
  });

  it('ensurePickerAssets fills pickerSystem and pickerMine', async () => {
    const { listAvatarAssets } = await import('../../features/avatar/api');
    const systemAssets = [{ id: 's1', key: 'sys1', name: 'System 1', description: '', mime_type: 'image/png' }];
    const mineAssets = [{ id: 'm1', key: 'mine1', name: 'Mine 1', description: '', mime_type: 'image/png' }];
    (listAvatarAssets as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(systemAssets)
      .mockResolvedValueOnce(mineAssets);

    const store = useAvatarCatalogStore();
    await store.ensurePickerAssets();

    expect(store.pickerSystem).toHaveLength(1);
    expect(store.pickerSystem[0].id).toBe('s1');
    expect(store.pickerMine).toHaveLength(1);
    expect(store.pickerMine[0].id).toBe('m1');
    expect(store.pickerLoaded).toBe(true);
  });

  it('ensurePickerAssets skips when already loaded', async () => {
    const { listAvatarAssets } = await import('../../features/avatar/api');
    (listAvatarAssets as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([]);

    const store = useAvatarCatalogStore();
    await store.ensurePickerAssets();
    await store.ensurePickerAssets();

    // first call: 2 invocations (system + mine), second call: 0 (skipped)
    expect(listAvatarAssets).toHaveBeenCalledTimes(2);
  });

  it('ensureThumbnail writes cache on first call', async () => {
    const store = useAvatarCatalogStore();
    await store.ensureThumbnail('asset-1');

    expect(store.thumbnailById['asset-1']).toBe('data:image/png;base64,abc');
  });

  it('ensureThumbnail skips when already cached', async () => {
    const { getAvatarThumbnailDataUrl } = await import('../../features/avatar/api');

    const store = useAvatarCatalogStore();
    await store.ensureThumbnail('asset-1');
    await store.ensureThumbnail('asset-1');

    expect(getAvatarThumbnailDataUrl).toHaveBeenCalledTimes(1);
  });

  it('ensureThumbnail skips empty or URL-like ids', async () => {
    const { getAvatarThumbnailDataUrl } = await import('../../features/avatar/api');

    const store = useAvatarCatalogStore();
    await store.ensureThumbnail('');
    await store.ensureThumbnail('https://example.com/icon.png');
    await store.ensureThumbnail('data:image/png;base64,xxx');

    expect(getAvatarThumbnailDataUrl).not.toHaveBeenCalled();
  });

  it('forgetThumbnail removes from cache', async () => {
    const store = useAvatarCatalogStore();
    await store.ensureThumbnail('asset-1');
    expect(store.thumbnailById['asset-1']).toBeDefined();

    store.forgetThumbnail('asset-1');
    expect(store.thumbnailById['asset-1']).toBeUndefined();
  });

  it('forgetThumbnail skips empty id', () => {
    const store = useAvatarCatalogStore();
    store.thumbnailById = { 'asset-1': 'data:image/png;base64,abc' };

    store.forgetThumbnail('');
    store.forgetThumbnail('  ');

    expect(Object.keys(store.thumbnailById)).toHaveLength(1);
  });

  it('mergeUploaded adds asset to pickerMine and agentsCatalog', () => {
    const store = useAvatarCatalogStore();
    store.pickerMine = [];
    store.agentsCatalog = [];

    const asset = { id: 'new1', key: 'new-icon', name: 'New', description: '', mime_type: 'image/png' };
    store.mergeUploaded(asset as any);

    expect(store.pickerMine).toHaveLength(1);
    expect(store.pickerMine[0].id).toBe('new1');
    expect(store.agentsCatalog).toHaveLength(1);
    expect(store.agentsCatalog[0].id).toBe('new1');
  });

  it('mergeUploaded does not duplicate in agentsCatalog if already present', () => {
    const store = useAvatarCatalogStore();
    const existing = { id: 'existing1', key: 'icon', name: 'Existing', description: '', mime_type: 'image/png' };
    store.pickerMine = [];
    store.agentsCatalog = [existing] as any;

    store.mergeUploaded(existing as any);

    expect(store.agentsCatalog).toHaveLength(1);
    expect(store.pickerMine).toHaveLength(1);
  });

  it('invalidateAll resets all state', async () => {
    const { listAvatarAssets } = await import('../../features/avatar/api');
    (listAvatarAssets as ReturnType<typeof vi.fn>).mockResolvedValue([
      { id: 'av1', key: 'icon1', name: 'Icon 1', description: '', mime_type: 'image/png' },
    ]);

    const store = useAvatarCatalogStore();
    await store.ensureAgentsCatalog();
    await store.ensurePickerAssets();
    await store.ensureThumbnail('asset-1');

    expect(store.agentsCatalogLoaded).toBe(true);
    expect(store.pickerLoaded).toBe(true);
    expect(Object.keys(store.thumbnailById)).toHaveLength(1);

    store.invalidateAll();

    expect(store.agentsCatalogLoaded).toBe(false);
    expect(store.pickerLoaded).toBe(false);
    expect(store.agentsCatalog).toEqual([]);
    expect(store.pickerSystem).toEqual([]);
    expect(store.pickerMine).toEqual([]);
    expect(store.thumbnailById).toEqual({});
  });
});
