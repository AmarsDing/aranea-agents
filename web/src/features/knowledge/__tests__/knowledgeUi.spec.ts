import { describe, expect, it } from 'vitest';
import {
  knowledgeMediaEditable,
  knowledgeMediaKind,
  knowledgeMediaNeedsAsset,
  promoteTargetOptions,
} from '../knowledgeUi';
import type { KnowledgeCollection } from '../types';

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
