import { describe, expect, it } from 'vitest';
import { bytesToBase64, inferUploadMime, isExtractSupported, utf8ToBase64 } from '../knowledgeUploadCodec';

describe('knowledgeUploadCodec', () => {
  it('treats markdown and text as extractable', () => {
    expect(isExtractSupported('text/markdown')).toBe(true);
    expect(isExtractSupported('text/plain')).toBe(true);
    expect(isExtractSupported('image/gif')).toBe(false);
  });

  it('infers mime from filename when File.type is empty', () => {
    const file = new File(['hi'], 'note.md', { type: '' });
    expect(inferUploadMime(file)).toBe('text/markdown');
  });

  it('encodes utf8 to base64 without corruption', () => {
    expect(utf8ToBase64('知识库')).toBe(bytesToBase64(new TextEncoder().encode('知识库')));
  });
});
