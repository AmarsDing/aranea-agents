import { describe, expect, it } from 'vitest';
import {
  KNOWLEDGE_DOC_TABLE_COLUMNS,
  knowledgeDocColumns,
  knowledgeMediaEditable,
  knowledgeMediaKind,
  knowledgeMediaNeedsAsset,
  knowledgeStatusLabelKey,
} from '../knowledgeUi';

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
