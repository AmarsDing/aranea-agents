// web/src/stores/__tests__/artifact.store.spec.ts
// P0/P1 会话产物点击查看（2026-09-01）：全局预览目标状态。
import { describe, expect, it, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useArtifactStore } from '../artifact';

describe('useArtifactStore preview target', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('初始无预览目标', () => {
    const store = useArtifactStore();
    expect(store.previewTarget).toBeNull();
  });

  it('openArtifactPreview 设置目标；version>0 时携带版本', () => {
    const store = useArtifactStore();
    store.openArtifactPreview('art-1');
    expect(store.previewTarget).toEqual({ id: 'art-1' });
    store.openArtifactPreview('art-2', 3);
    expect(store.previewTarget).toEqual({ id: 'art-2', version: 3 });
  });

  it('version<=0 视为未指定版本', () => {
    const store = useArtifactStore();
    store.openArtifactPreview('art-1', 0);
    expect(store.previewTarget).toEqual({ id: 'art-1' });
  });

  it('空 id 不改变目标', () => {
    const store = useArtifactStore();
    store.openArtifactPreview('');
    expect(store.previewTarget).toBeNull();
  });

  it('closeArtifactPreview 清空目标', () => {
    const store = useArtifactStore();
    store.openArtifactPreview('art-1');
    store.closeArtifactPreview();
    expect(store.previewTarget).toBeNull();
  });
});
