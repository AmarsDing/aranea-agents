// G-16：searchHighlight 纯函数回归——命中词/CJK bigram 拆分、区间归并、片段窗口坐标换算。
import { describe, expect, it } from 'vitest';
import { buildSearchSnippet, findMarks, queryTerms } from '../searchHighlight';

describe('queryTerms', () => {
  it('keeps latin tokens case-insensitive-ready', () => {
    expect(queryTerms(' Ingest  PDF')).toEqual(['Ingest', 'PDF']);
  });

  it('expands pure CJK token into whole-word + bigrams', () => {
    const terms = queryTerms('机柜巡检');
    expect(terms).toContain('机柜巡检');
    for (const bg of ['机柜', '柜巡', '巡检']) expect(terms).toContain(bg);
  });

  it('draws bigrams from CJK runs inside mixed tokens', () => {
    const terms = queryTerms('A1机柜');
    expect(terms).toContain('A1机柜');
    expect(terms).toContain('机柜');
  });

  it('keeps single CJK char as-is (no bigram)', () => {
    expect(queryTerms('页')).toEqual(['页']);
  });
});

describe('findMarks', () => {
  it('collects case-insensitive latin hits', () => {
    expect(findMarks('Ingest the INGEST doc', ['ingest'])).toEqual([
      { start: 0, end: 6 },
      { start: 11, end: 17 },
    ]);
  });

  it('merges overlapping bigram marks into one run', () => {
    // 「机柜巡检」的 bigram 机柜(0-2) 柜巡(1-3) 巡检(2-4) 应归并为 [0,4)
    expect(findMarks('机柜巡检手册', ['机柜', '柜巡', '巡检'])).toEqual([{ start: 0, end: 4 }]);
  });

  it('keeps disjoint marks separate', () => {
    expect(findMarks('巡检机房 A1 机柜', ['巡检', '机柜'])).toEqual([
      { start: 0, end: 2 },
      { start: 8, end: 10 },
    ]);
  });
});

describe('buildSearchSnippet', () => {
  it('windows around first hit and shifts marks into snippet coordinates', () => {
    const content = `${'前置上下文'.repeat(20)}机柜温度正常${'x'.repeat(200)}`;
    const snip = buildSearchSnippet(content, '机柜');
    expect(snip.text.startsWith('…')).toBe(true);
    expect(snip.text.endsWith('…')).toBe(true);
    expect(snip.marks.length).toBeGreaterThan(0);
    const m = snip.marks[0];
    expect(snip.text.slice(m.start, m.end)).toBe('机柜');
  });

  it('falls back to head slice when no term matches', () => {
    const snip = buildSearchSnippet(`${'y'.repeat(300)}`, '机柜');
    expect(snip.text).toBe(`${'y'.repeat(160)}…`);
    expect(snip.marks).toEqual([]);
  });

  it('no prefix ellipsis when hit is at document head', () => {
    const snip = buildSearchSnippet(`巡检手册：${'z'.repeat(200)}`, '巡检');
    expect(snip.text.startsWith('…')).toBe(false);
    const m = snip.marks[0];
    expect(snip.text.slice(m.start, m.end)).toBe('巡检');
  });

  it('clips marks crossing window tail', () => {
    const content = `${'k'.repeat(155)}高亮尾部跨境词${'m'.repeat(50)}`;
    const snip = buildSearchSnippet(content, '尾部跨境');
    for (const m of snip.marks) {
      expect(m.end).toBeLessThanOrEqual(snip.text.length);
    }
  });

  it('handles empty query (no marks, head window)', () => {
    const snip = buildSearchSnippet('abc def', '   ');
    expect(snip.marks).toEqual([]);
    expect(snip.text).toBe('abc def');
  });
});
