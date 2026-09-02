// web/src/features/chat/__tests__/deliverablesNotice.spec.ts
// P1 会话产物点击查看（2026-09-02）：deliverables 通知载荷解析契约。
import { describe, expect, it } from 'vitest';
import { formatToMimeHint, parseDeliverableRefs } from '../deliverablesNotice';

describe('parseDeliverableRefs', () => {
  it('parses a valid deliverables payload', () => {
    const content = JSON.stringify({
      artifacts: [
        { artifact_id: 'a1', name: '云计算十年.md', format: 'markdown', size_chars: 8234 },
        { artifact_id: 'a2', name: 'report.pdf', mime_type: 'application/pdf' },
      ],
    });
    expect(parseDeliverableRefs('deliverables', content)).toEqual([
      { artifactId: 'a1', name: '云计算十年.md', mimeType: 'text/plain' },
      { artifactId: 'a2', name: 'report.pdf', mimeType: 'application/pdf' },
    ]);
  });

  it('returns null for non-deliverables notice types', () => {
    expect(parseDeliverableRefs('info', '{"artifacts":[{"artifact_id":"a1"}]}')).toBeNull();
    expect(parseDeliverableRefs(undefined, '{"artifacts":[{"artifact_id":"a1"}]}')).toBeNull();
  });

  it('matches notice type case-insensitively', () => {
    const refs = parseDeliverableRefs('Deliverables', '{"artifacts":[{"artifact_id":"a1"}]}');
    expect(refs).toEqual([{ artifactId: 'a1', name: undefined, mimeType: undefined }]);
  });

  it('returns null on malformed JSON or wrong shape', () => {
    expect(parseDeliverableRefs('deliverables', 'not-json')).toBeNull();
    expect(parseDeliverableRefs('deliverables', '{"artifacts":"nope"}')).toBeNull();
    expect(parseDeliverableRefs('deliverables', '')).toBeNull();
  });

  it('skips entries without artifact_id; null when none survive', () => {
    const content = JSON.stringify({ artifacts: [{ name: 'x' }, { artifact_id: '  ' }, { artifact_id: 'a9' }] });
    expect(parseDeliverableRefs('deliverables', content)).toEqual([
      { artifactId: 'a9', name: undefined, mimeType: undefined },
    ]);
    expect(parseDeliverableRefs('deliverables', '{"artifacts":[{"name":"x"}]}')).toBeNull();
    expect(parseDeliverableRefs('deliverables', '{"artifacts":[]}')).toBeNull();
  });

  it('prefers mime_type over format hint; blank name becomes undefined', () => {
    const content = JSON.stringify({
      artifacts: [{ artifact_id: 'a1', name: '  ', format: 'png', mime_type: '' }],
    });
    expect(parseDeliverableRefs('deliverables', content)).toEqual([
      { artifactId: 'a1', name: undefined, mimeType: 'image/png' },
    ]);
  });
});

describe('formatToMimeHint', () => {
  it.each([
    ['pdf', 'application/pdf'],
    ['json', 'application/json'],
    ['png', 'image/png'],
    ['mp4', 'video/mp4'],
    ['mp3', 'audio/mp3'],
    ['markdown', 'text/plain'],
    ['', undefined],
    [undefined, undefined],
  ])('maps %s -> %s', (format, expected) => {
    expect(formatToMimeHint(format)).toBe(expected);
  });
});
