import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { defineComponent, ref } from 'vue';
import { flushPromises } from '@vue/test-utils';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import type { ArtifactPreview } from '../types';

vi.mock('../api', () => ({
  previewArtifact: vi.fn(),
  signDownloadUrl: vi.fn(),
  artifactDownloadHref: (p: string) => `http://api${p}`,
  listArtifacts: vi.fn(),
  getArtifact: vi.fn(),
  uploadArtifact: vi.fn(),
  deleteArtifact: vi.fn(),
  deleteArtifactVersion: vi.fn(),
  listArtifactVersions: vi.fn(),
}));

import { previewArtifact, signDownloadUrl } from '../api';
import { useArtifactPreview } from '../useArtifactPreview';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': {} } });

/** Run a composable inside a host component so useI18n() has app context. */
function withSetup<T>(composable: () => T): T {
  let result!: T;
  mount(
    defineComponent({
      setup() {
        result = composable();
        return () => null;
      },
    }),
    { global: { plugins: [i18n] } },
  );
  return result;
}

function previewOf(kind: string): ArtifactPreview {
  return {
    meta: {
      id: 'a1',
      session_id: 's1',
      name: `f.${kind}`,
      mime_type: kind === 'audio' ? 'audio/mpeg' : kind === 'video' ? 'video/mp4' : 'text/plain',
      size: 10,
      sha256: '',
      storage_kind: 'local',
      storage_uri: '',
      version: 1,
      created_at: '',
    },
    preview_kind: kind,
    text_content: kind === 'text' ? 'hello' : '',
    data_base64: '',
  };
}

describe('useArtifactPreview audio/video', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('signs an inline media URL for audio previews', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('audio'));
    vi.mocked(signDownloadUrl).mockResolvedValue({ url: '/v1/artifacts/download?id=a1&token=t', expires_at: '' });

    const { inlineMediaSrc, loadPreview } = withSetup(() => useArtifactPreview(() => 'a1'));
    await loadPreview();

    expect(signDownloadUrl).toHaveBeenCalledWith('a1', undefined);
    expect(inlineMediaSrc.value).toBe('http://api/v1/artifacts/download?id=a1&token=t&inline=1');
  });

  it('signs an inline media URL for video previews', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('video'));
    vi.mocked(signDownloadUrl).mockResolvedValue({ url: '/v1/artifacts/download?id=a1&token=t', expires_at: '' });

    const { inlineMediaSrc, loadPreview } = withSetup(() => useArtifactPreview(() => 'a1'));
    await loadPreview();

    expect(inlineMediaSrc.value).toContain('&inline=1');
  });

  it('does not sign media URL for text previews', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('text'));

    const { inlineMediaSrc, loadPreview } = withSetup(() => useArtifactPreview(() => 'a1'));
    await loadPreview();

    expect(signDownloadUrl).not.toHaveBeenCalled();
    expect(inlineMediaSrc.value).toBe('');
  });

  it('keeps inlineMediaSrc empty when signing fails', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('audio'));
    vi.mocked(signDownloadUrl).mockRejectedValue(new Error('nope'));

    const { inlineMediaSrc, loadPreview, error } = withSetup(() => useArtifactPreview(() => 'a1'));
    await loadPreview();

    expect(inlineMediaSrc.value).toBe('');
    expect(error.value).toBe('');
  });

  it('ignores a stale preview failure after changing the selected file', async () => {
    let rejectOld!: (error: Error) => void;
    vi.mocked(previewArtifact).mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectOld = reject;
      }),
    );
    vi.mocked(previewArtifact).mockResolvedValueOnce(previewOf('text'));
    const id = ref('old');
    const result = withSetup(() => useArtifactPreview(() => id.value));
    id.value = 'a1';
    await flushPromises();
    rejectOld(new Error('old request'));
    await flushPromises();
    expect(result.preview.value?.meta.id).toBe('a1');
    expect(result.error.value).toBe('');
    expect(result.loading.value).toBe(false);
  });

  it('does not apply an old media signature to a new text selection', async () => {
    let resolveSign!: (value: { url: string; expires_at: string }) => void;
    vi.mocked(previewArtifact).mockResolvedValueOnce(previewOf('audio')).mockResolvedValueOnce(previewOf('text'));
    vi.mocked(signDownloadUrl).mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSign = resolve;
      }),
    );
    const id = ref('audio');
    const result = withSetup(() => useArtifactPreview(() => id.value));
    await flushPromises();
    id.value = 'text';
    await flushPromises();
    resolveSign({ url: '/old-audio', expires_at: '' });
    await flushPromises();
    expect(result.inlineMediaSrc.value).toBe('');
    expect(result.preview.value?.preview_kind).toBe('text');
  });

  it('rejects preview data from another session before exposing its contents', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('text'));
    const result = withSetup(() =>
      useArtifactPreview(
        () => 'a1',
        undefined,
        () => 'other-session',
      ),
    );
    await flushPromises();
    expect(result.preview.value).toBeNull();
    expect(result.error.value).toBeTruthy();
  });

  it('does not silently show latest when a selected first version cannot be resolved', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('text'));
    const result = withSetup(() =>
      useArtifactPreview(
        () => 'a1',
        () => 0,
        () => 's1',
      ),
    );
    await flushPromises();
    expect(result.preview.value).toBeNull();
    expect(result.error.value).toBeTruthy();
  });
});
