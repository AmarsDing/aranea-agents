import { describe, expect, it } from 'vitest';
import { parseMessageAttachments, attachmentMimeIcon } from '../messageAttachments';

describe('parseMessageAttachments', () => {
  it('returns empty for missing or invalid json', () => {
    expect(parseMessageAttachments(undefined)).toEqual([]);
    expect(parseMessageAttachments('')).toEqual([]);
    expect(parseMessageAttachments('{')).toEqual([]);
  });

  it('parses attachment refs from options_json', () => {
    const json = JSON.stringify({
      dialog_mode: 'chat',
      attachments: [
        { id: 'abc', name: 'a.png', mime_type: 'image/png', size: 2048 },
        { id: '', name: 'skip' },
      ],
    });
    expect(parseMessageAttachments(json)).toEqual([{ id: 'abc', name: 'a.png', mime_type: 'image/png', size: 2048 }]);
  });
});

describe('attachmentMimeIcon', () => {
  it('maps common mime types', () => {
    expect(attachmentMimeIcon('image/png')).toBe('image');
    expect(attachmentMimeIcon('application/pdf')).toBe('picture_as_pdf');
    expect(attachmentMimeIcon('text/plain')).toBe('code');
  });
});
