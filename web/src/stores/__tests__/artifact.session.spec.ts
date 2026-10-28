import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useArtifactStore } from '../artifact';
import { listArtifacts, listArtifactVersions } from '../../features/artifact/api';
import type { ArtifactMeta, ListArtifactsResult } from '../../features/artifact/types';

vi.mock('../../features/artifact/api', () => ({
  listArtifacts: vi.fn(),
  listArtifactVersions: vi.fn(),
  getArtifact: vi.fn(),
  uploadArtifact: vi.fn(),
  deleteArtifact: vi.fn(),
  deleteArtifactVersion: vi.fn(),
  previewArtifact: vi.fn(),
  signDownloadUrl: vi.fn(),
  artifactDownloadHref: vi.fn(),
  revealArtifact: vi.fn(),
  fetchLocalRevealEnabled: vi.fn(),
}));
function meta(id: string, sessionId = 'A'): ArtifactMeta {
  return {
    id,
    session_id: sessionId,
    name: `${id}.txt`,
    mime_type: 'text/plain',
    size: 1,
    sha256: '',
    storage_kind: 'local',
    storage_uri: '',
    version: 0,
    created_at: '',
  };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
beforeEach(() => {
  setActivePinia(createPinia());
  vi.resetAllMocks();
  vi.mocked(listArtifactVersions).mockResolvedValue([]);
});

describe('session artifact lists', () => {
  it('keeps the total and offset, deduplicates overlapping pages without merging file names', async () => {
    const store = useArtifactStore();
    vi.mocked(listArtifacts).mockResolvedValueOnce({ items: [meta('a'), meta('b')], total: 3 });
    await store.loadSessionArtifacts('A');
    vi.mocked(listArtifacts).mockResolvedValueOnce({ items: [meta('b'), meta('c')], total: 3 });
    await store.loadSessionArtifacts('A', true);
    expect(listArtifacts).toHaveBeenLastCalledWith({ session_id: 'A', limit: 20, offset: 2 });
    expect(store.sessionLists.A?.items.map((item) => item.id)).toEqual(['a', 'b', 'c']);
    expect(store.sessionLists.A?.total).toBe(3);
    expect(store.sessionLists.A?.exhausted).toBe(true);
    expect(store.artifacts).toEqual([]);
  });

  it('does not let an older failure overwrite a successful refresh', async () => {
    const old = deferred<ListArtifactsResult>();
    const store = useArtifactStore();
    vi.mocked(listArtifacts)
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce({ items: [meta('new')], total: 1 });
    const first = store.loadSessionArtifacts('A');
    await store.loadSessionArtifacts('A');
    old.reject(new Error('old failure'));
    await first;
    expect(store.sessionLists.A?.items[0]?.id).toBe('new');
    expect(store.sessionLists.A?.error).toBe('');
    expect(store.sessionLists.A?.loading).toBe(false);
  });

  it('discards a released query even when the same session is opened again', async () => {
    const old = deferred<ListArtifactsResult>();
    const store = useArtifactStore();
    vi.mocked(listArtifacts)
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce({ items: [meta('fresh')], total: 1 });
    const pending = store.loadSessionArtifacts('A');
    store.releaseSessionArtifacts('A');
    await store.loadSessionArtifacts('A');
    old.resolve({ items: [meta('stale')], total: 8 });
    await pending;
    expect(store.sessionLists.A?.items[0]?.id).toBe('fresh');
    expect(store.sessionLists.A?.total).toBe(1);
  });

  it('keeps loaded rows when another page fails', async () => {
    const store = useArtifactStore();
    vi.mocked(listArtifacts)
      .mockResolvedValueOnce({ items: [meta('a')], total: 45 })
      .mockRejectedValueOnce(new Error('offline'));
    await store.loadSessionArtifacts('A');
    await store.loadSessionArtifacts('A', true);
    expect(store.sessionLists.A?.items).toHaveLength(1);
    expect(store.sessionLists.A?.total).toBe(45);
    expect(store.sessionLists.A?.error).toBe('offline');
  });
});

describe('preview ownership', () => {
  it('uses one panel in the task page and restores dialog behavior when leaving', () => {
    const store = useArtifactStore();
    const release = store.claimPreviewHost('A');
    store.openArtifactPreview('a', 0);
    expect(store.previewTarget).toEqual({ id: 'a', version: 0, surface: 'panel', sessionId: 'A' });
    expect(store.dialogPreviewTarget).toBeNull();
    release();
    expect(store.previewTarget).toBeNull();
    store.openArtifactPreview('b', 2);
    expect(store.dialogPreviewTarget).toEqual({ id: 'b', version: 2 });
  });

  it('old host cleanup and version responses cannot replace a newer session', async () => {
    const old = deferred<ArtifactMeta[]>();
    vi.mocked(listArtifactVersions)
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce([meta('new', 'B')]);
    const store = useArtifactStore();
    const releaseOld = store.claimPreviewHost('A');
    store.openArtifactPreview('old');
    store.claimPreviewHost('B');
    store.openArtifactPreview('new');
    releaseOld();
    old.resolve([meta('old')]);
    await Promise.resolve();
    await Promise.resolve();
    expect(store.previewTarget?.id).toBe('new');
    expect(store.previewVersions.map((item) => item.id)).toEqual(['new']);
  });
});
