import { describe, expect, it } from 'vitest';
import type { KnowledgeChunk, KnowledgeDocument, VaultTreeNode } from '../../knowledge/types';
import {
  crumbsOf,
  dirOf,
  fileKind,
  localPathOf,
  mdHeadings,
  mergeSearchHits,
  nameMatches,
  previewMode,
  pushRecent,
  sortNodes,
  spaceOf,
  toggleFav,
  validFileName,
} from '../model';

const doc = (id: string, rel: string, source = rel): KnowledgeDocument =>
  ({ id, collection_id: 'c1', rel_path: rel, source }) as KnowledgeDocument;
const chunk = (doc_id: string, score: number, content: string): KnowledgeChunk =>
  ({ doc_id, score, content }) as KnowledgeChunk;

describe('library model', () => {
  it('spaceOf：登记了就用登记的，没登记按后端归', () => {
    expect(spaceOf({ space: 'company', vault_backend: 'local' })).toBe('company');
    expect(spaceOf({ space: '', vault_backend: 'team' })).toBe('company');
    expect(spaceOf({ space: undefined, vault_backend: 'local' })).toBe('mine');
  });

  it('fileKind / previewMode', () => {
    expect(fileKind('合同.DOCX')).toBe('doc');
    expect(fileKind('报价.xlsx')).toBe('xls');
    expect(fileKind('a.md')).toBe('md');
    expect(fileKind('scan', 'image/png')).toBe('img');
    expect(fileKind('x.bin')).toBe('file');
    expect(previewMode('pdf')).toBe('pdf');
    expect(previewMode('doc')).toBe('text');
    expect(previewMode('md')).toBe('markdown');
  });

  it('sortNodes：文件夹在前，数字按大小', () => {
    const nodes = [
      { name: 'b.md', kind: 'file' },
      { name: '10月', kind: 'dir' },
      { name: '9月', kind: 'dir' },
      { name: 'a.md', kind: 'file' },
    ] as VaultTreeNode[];
    expect(sortNodes(nodes).map((n) => n.name)).toEqual(['9月', '10月', 'a.md', 'b.md']);
  });

  it('crumbsOf / dirOf / localPathOf', () => {
    expect(crumbsOf('合同/2026/')).toEqual([
      { name: '合同', path: '合同/' },
      { name: '2026', path: '合同/2026/' },
    ]);
    expect(crumbsOf('')).toEqual([]);
    expect(dirOf('合同/2026/a.docx')).toBe('合同/2026/');
    expect(dirOf('a.docx')).toBe('');
    expect(localPathOf('D:\\公司资料', '合同/a.docx')).toBe('D:\\公司资料\\合同\\a.docx');
    expect(localPathOf('/home/me/docs', 'a.md')).toBe('/home/me/docs/a.md');
    expect(localPathOf('', 'a.md')).toBe('');
  });

  it('validFileName', () => {
    expect(validFileName('周报 11-20')).toBe(true);
    expect(validFileName(' ')).toBe(false);
    expect(validFileName('a/b')).toBe(false);
    expect(validFileName('a:b')).toBe(false);
  });

  it('mergeSearchHits：按文件聚合，文件名命中在前，同文件留最高分片段', () => {
    const docs = new Map([
      ['d1', doc('d1', '合同/技术服务合同.docx')],
      ['d2', doc('d2', '周报.md')],
    ]);
    const hits = mergeSearchHits(
      [docs.get('d1')!],
      [
        chunk('d2', 0.4, '本周  推进合同\n修订'),
        chunk('d2', 0.9, '最高分片段'),
        chunk('d1', 0.5, '合同正文'),
        chunk('gone', 1, 'x'),
      ],
      docs,
    );
    expect(hits.map((h) => h.docId)).toEqual(['d1', 'd2']);
    expect(hits[0]).toMatchObject({ nameMatch: true, snippet: '合同正文', name: '技术服务合同.docx' });
    expect(hits[1]).toMatchObject({ nameMatch: false, snippet: '最高分片段', score: 0.9 });
    expect(nameMatches('服务', docs.get('d1')!)).toBe(true);
    expect(nameMatches('', docs.get('d1')!)).toBe(false);
  });
});

describe('快速访问与目录', () => {
  const it1 = { doc: 'a', lib: 'l', path: 'x/a.md', name: 'a.md', at: 1 };
  const it2 = { doc: 'b', lib: 'l', path: 'b.md', name: 'b.md', at: 2 };
  it('最近打开去重、最新在前、有上限', () => {
    expect(pushRecent([it1, it2], { ...it1, at: 3 }).map((x) => x.doc)).toEqual(['a', 'b']);
    expect(pushRecent([it1], it2, 1).map((x) => x.doc)).toEqual(['b']);
  });
  it('收藏切换', () => {
    expect(toggleFav([], it1)).toHaveLength(1);
    expect(toggleFav([it1, it2], it1).map((x) => x.doc)).toEqual(['b']);
  });
  it('Markdown 标题，跳过代码块', () => {
    expect(mdHeadings('# 一\ntext\n```\n# 不是\n```\n### 三 ##\n')).toEqual([
      { level: 1, text: '一' },
      { level: 3, text: '三' },
    ]);
  });
});
