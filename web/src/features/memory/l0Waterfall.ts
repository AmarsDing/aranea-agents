import type { L0AssemblySegmentStats, L0AssemblySegmentsMap } from './types';

/** Prompt assembly sections, low → high (system first, user last). */
const SECTION_RANK = [
  'system',
  'identity',
  'strategy',
  'history',
  'summary',
  'l0',
  'l1',
  'l2',
  'l3',
  'l4',
  'tools',
  'tool',
  'user',
  'message',
];

export type L0WaterfallBar = {
  section: string;
  tokens: number;
  percent: number;
};

function sectionRank(section: string): number {
  const key = section.trim().toLowerCase();
  const idx = SECTION_RANK.findIndex((name) => key === name || key.includes(name));
  return idx < 0 ? SECTION_RANK.length : idx;
}

/** Turns snapshot `segments_json` into an ordered token waterfall. */
export function buildL0Waterfall(segments: L0AssemblySegmentsMap | null | undefined): L0WaterfallBar[] {
  const entries = Object.entries(segments ?? {}).map(([section, stats]) => ({
    section,
    tokens: tokenEstimate(stats),
  }));
  const total = entries.reduce((sum, row) => sum + row.tokens, 0);
  entries.sort((a, b) => {
    const rank = sectionRank(a.section) - sectionRank(b.section);
    if (rank !== 0) return rank;
    return a.section.localeCompare(b.section);
  });
  return entries.map((row) => ({
    ...row,
    percent: total > 0 ? (row.tokens / total) * 100 : 0,
  }));
}

function tokenEstimate(stats: L0AssemblySegmentStats | undefined): number {
  const n = Number(stats?.token_estimate);
  return Number.isFinite(n) && n > 0 ? n : 0;
}
