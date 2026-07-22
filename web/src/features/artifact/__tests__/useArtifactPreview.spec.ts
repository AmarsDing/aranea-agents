import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
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

    const { inlineMediaSrc, loadPreview } = useArtifactPreview(() => 'a1');
    await loadPreview();

    expect(signDownloadUrl).toHaveBeenCalledWith('a1', undefined);
    expect(inlineMediaSrc.value).toBe('http://api/v1/artifacts/download?id=a1&token=t&inline=1');
  });

  it('signs an inline media URL for video previews', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('video'));
    vi.mocked(signDownloadUrl).mockResolvedValue({ url: '/v1/artifacts/download?id=a1&token=t', expires_at: '' });

    const { inlineMediaSrc, loadPreview } = useArtifactPreview(() => 'a1');
    await loadPreview();

    expect(inlineMediaSrc.value).toContain('&inline=1');
  });

  it('does not sign media URL for text previews', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('text'));

    const { inlineMediaSrc, loadPreview } = useArtifactPreview(() => 'a1');
    await loadPreview();

    expect(signDownloadUrl).not.toHaveBeenCalled();
    expect(inlineMediaSrc.value).toBe('');
  });

  it('keeps inlineMediaSrc empty when signing fails', async () => {
    vi.mocked(previewArtifact).mockResolvedValue(previewOf('audio'));
    vi.mocked(signDownloadUrl).mockRejectedValue(new Error('nope'));

    const { inlineMediaSrc, loadPreview, error } = useArtifactPreview(() => 'a1');
    await loadPreview();

    expect(inlineMediaSrc.value).toBe('');
    expect(error.value).toBe('');
  });
});
