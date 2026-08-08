import { describe, expect, it } from 'vitest';
import {
  KNOWLEDGE_DOC_TABLE_COLUMNS,
  knowledgeDocColumns,
  knowledgeMediaEditable,
  knowledgeMediaKind,
  knowledgeMediaNeedsAsset,
  knowledgeStatusLabelKey,
  promoteTargetOptions,
  splitDanglingPreview,
} from '../knowledgeUi';
import type { KnowledgeCollection } from '../types';

describe('knowledgeUi doc table columns', () => {
  it('exposes updated_at column so users see last update time', () => {
    const names = KNOWLEDGE_DOC_TABLE_COLUMNS.map((c) => c.name);
    expect(names).toContain('updated_at');
  });

  it('keeps created_at before updated_at', () => {
    const names = KNOWLEDGE_DOC_TABLE_COLUMNS.map((c) => c.name);
    expect(names.indexOf('created_at')).toBeLessThan(names.indexOf('updated_at'));
  });

  it('deprecated alias stays in sync', () => {
    expect(knowledgeDocColumns).toBe(KNOWLEDGE_DOC_TABLE_COLUMNS);
  });
});

describe('knowledgeMediaKind（G2-F 媒体分类）', () => {
  it('classifies media by extension case-insensitively', () => {
    expect(knowledgeMediaKind('a.PNG')).toBe('image');
    expect(knowledgeMediaKind('clip.Mp4')).toBe('video');
    expect(knowledgeMediaKind('song.m4a')).toBe('audio');
    expect(knowledgeMediaKind('report.docx')).toBe('word');
    expect(knowledgeMediaKind('notes/readme.md')).toBe('markdown');
    expect(knowledgeMediaKind('log.txt')).toBe('text');
  });

  it('falls back to other for unknown or missing extension', () => {
    expect(knowledgeMediaKind('archive.zip')).toBe('other');
    expect(knowledgeMediaKind('README')).toBe('other');
    expect(knowledgeMediaKind('dir/file')).toBe('other');
  });

  it('image/audio/video need B6 asset stream; word/markdown/text do not', () => {
    expect(knowledgeMediaNeedsAsset('image')).toBe(true);
    expect(knowledgeMediaNeedsAsset('audio')).toBe(true);
    expect(knowledgeMediaNeedsAsset('video')).toBe(true);
    expect(knowledgeMediaNeedsAsset('word')).toBe(false);
    expect(knowledgeMediaNeedsAsset('markdown')).toBe(false);
    expect(knowledgeMediaNeedsAsset('other')).toBe(false);
  });

  it('only markdown/text are editable (V12.4)', () => {
    expect(knowledgeMediaEditable('markdown')).toBe(true);
    expect(knowledgeMediaEditable('text')).toBe(true);
    expect(knowledgeMediaEditable('word')).toBe(false);
    expect(knowledgeMediaEditable('image')).toBe(false);
  });
});

describe('knowledgeStatusLabelKey（文档状态本地化）', () => {
  it('maps known statuses to i18n keys', () => {
    expect(knowledgeStatusLabelKey('indexed')).toBe('knowledgePage.statusIndexed');
    expect(knowledgeStatusLabelKey('active')).toBe('knowledgePage.statusIndexed');
    expect(knowledgeStatusLabelKey('indexing')).toBe('knowledgePage.statusIndexing');
    expect(knowledgeStatusLabelKey('pending')).toBe('knowledgePage.statusPending');
    expect(knowledgeStatusLabelKey('error')).toBe('knowledgePage.statusError');
  });

  it('returns empty string for unknown status (caller falls back to raw)', () => {
    expect(knowledgeStatusLabelKey('')).toBe('');
    expect(knowledgeStatusLabelKey('migrating')).toBe('');
  });
});

describe('splitDanglingPreview（SP1-I/I-2 dangling 灰显分段）', () => {
  const targets = new Set(['未创建笔记', 'dir/页面#标题']);

  it('returns null when no dangling wikilink is hit', () => {
    expect(splitDanglingPreview('正文 [[已存在]] 链接', targets)).toBeNull();
    expect(splitDanglingPreview('', targets)).toBeNull();
    expect(splitDanglingPreview('纯文本无链接', targets)).toBeNull();
  });

  it('splits content around dangling wikilinks', () => {
    const segs = splitDanglingPreview('见 [[未创建笔记]] 与 [[已存在]]', targets);
    expect(segs).toEqual([
      { text: '见 ', dangling: false },
      { text: '[[未创建笔记]]', dangling: true },
      { text: ' 与 [[已存在]]', dangling: false },
    ]);
  });

  it('matches alias form by target before | and keeps heading suffix', () => {
    const segs = splitDanglingPreview('[[未创建笔记|别名]] [[dir/页面#标题]]', targets);
    expect(segs).toEqual([
      { text: '[[未创建笔记|别名]]', dangling: true },
      { text: ' ', dangling: false },
      { text: '[[dir/页面#标题]]', dangling: true },
    ]);
  });

  it('marks embed form ![[...]] as dangling too', () => {
    const segs = splitDanglingPreview('嵌入 ![[未创建笔记]] 完毕', targets);
    expect(segs).toEqual([
      { text: '嵌入 ', dangling: false },
      { text: '![[未创建笔记]]', dangling: true },
      { text: ' 完毕', dangling: false },
    ]);
  });
});

describe('promoteTargetOptions（SP1-I/I-3 晋升目标库选项）', () => {
  function col(id: string, backend: string, name = id): KnowledgeCollection {
    return { vault_backend: backend, id, name } as KnowledgeCollection;
  }

  it('keeps only team vaults and excludes the source collection', () => {
    const options = promoteTargetOptions([col('local1', 'local'), col('team1', 'team'), col('team2', 'team')], 'team1');
    expect(options.map((o) => o.value)).toEqual(['team2']);
  });

  it('excludes the local source vault from targets', () => {
    const options = promoteTargetOptions([col('local1', 'local'), col('team1', 'team')], 'local1');
    expect(options.map((o) => o.value)).toEqual(['team1']);
  });

  it('returns empty when no team vault exists', () => {
    expect(promoteTargetOptions([col('local1', 'local'), col('local2', '')], 'local1')).toEqual([]);
  });

  it('uses name as label with id fallback', () => {
    const options = promoteTargetOptions([col('team1', 'team', '团队库')], '');
    expect(options[0]).toEqual({ label: '团队库', value: 'team1' });
  });
});
