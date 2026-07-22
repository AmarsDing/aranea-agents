// web/src/features/chat/__tests__/useMediaUrl.spec.ts
import { describe, it, expect, vi } from 'vitest';
import { useMediaUrl, isArtifactUrl } from '../useMediaUrl';
import type { MediaArtifact } from '../mediaTypes';

function art(partial: Partial<MediaArtifact>): MediaArtifact {
  return {
    artifact_id: 'a1',
    url: 'https://cdn.example.com/x.png',
    mime_type: 'image/png',
    ...partial,
  };
}

describe('isArtifactUrl', () => {
  it('detects artifact:// scheme', () => {
    expect(isArtifactUrl('artifact://abc')).toBe(true);
    expect(isArtifactUrl('https://x.com/a.png')).toBe(false);
    expect(isArtifactUrl('')).toBe(false);
  });
});

describe('useMediaUrl', () => {
  it('passes http(s) URLs through without signing', () => {
    const sign = vi.fn();
    const { mediaSrc } = useMediaUrl(sign);
    const a = art({ url: 'https://cdn.example.com/x.png' });
    expect(mediaSrc(a)).toBe('https://cdn.example.com/x.png');
    expect(sign).not.toHaveBeenCalled();
  });

  it('resolves artifact:// URLs via sign and returns placeholder first', async () => {
    const sign = vi.fn().mockResolvedValue('http://api/v1/artifacts/download?token=t');
    const { mediaSrc, resolve } = useMediaUrl(sign);
    const a = art({ artifact_id: 'art-1', url: 'artifact://art-1' });

    const first = mediaSrc(a);
    expect(first).not.toBe('artifact://art-1');
    expect(first.startsWith('data:')).toBe(true);

    await resolve(a);
    expect(sign).toHaveBeenCalledWith('art-1');
    expect(mediaSrc(a)).toBe('http://api/v1/artifacts/download?token=t');
  });

  it('caches the signed URL per artifact id (no duplicate signing)', async () => {
    const sign = vi.fn().mockResolvedValue('http://api/signed');
    const { mediaSrc, resolve } = useMediaUrl(sign);
    const a = art({ artifact_id: 'art-2', url: 'artifact://art-2' });

    await resolve(a);
    await resolve(a);
    expect(mediaSrc(a)).toBe('http://api/signed');
    expect(sign).toHaveBeenCalledTimes(1);
  });

  it('falls back to the original URL when signing fails', async () => {
    const sign = vi.fn().mockRejectedValue(new Error('gone'));
    const { mediaSrc, resolve } = useMediaUrl(sign);
    const a = art({ artifact_id: 'art-3', url: 'artifact://art-3' });

    await resolve(a);
    expect(mediaSrc(a)).toBe('artifact://art-3');
  });

  it('derives the artifact id from the URL when artifact_id is empty', async () => {
    const sign = vi.fn().mockResolvedValue('http://api/signed-4');
    const { mediaSrc, resolve } = useMediaUrl(sign);
    const a = art({ artifact_id: '', url: 'artifact://id-from-url' });

    await resolve(a);
    expect(sign).toHaveBeenCalledWith('id-from-url');
    expect(mediaSrc(a)).toBe('http://api/signed-4');
  });
});
