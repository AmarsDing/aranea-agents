import type { SessionTimelineItem } from '../../features/session/types';
import { computed } from 'vue';
import { i18n } from '../../i18n';
export type { TimelineStat } from '../../features/session/timelineHelpers';
export { buildTimelineStats } from '../../features/session/timelineHelpers';

const t = (key: string) => i18n.global.t(key);

export type TimelineAccent = 'user' | 'agent' | 'tool' | 'skill' | 'mcp' | 'error';

export function timelineEntryAccent(item: SessionTimelineItem): TimelineAccent {
  if (/fail|error/i.test(item.status)) return 'error';
  if (item.tags.includes('User')) return 'user';
  if (item.kind === 'message') return 'agent';
  if (item.kind === 'skill') return 'skill';
  if (item.kind === 'mcp') return 'mcp';
  if (item.kind === 'tool') return 'tool';
  return 'agent';
}

export function timelineEntryIcon(item: SessionTimelineItem): string {
  switch (timelineEntryAccent(item)) {
    case 'user':
      return 'person';
    case 'tool':
      return 'build';
    case 'skill':
      return 'auto_awesome';
    case 'mcp':
      return 'hub';
    case 'error':
      return 'error_outline';
    default:
      return 'smart_toy';
  }
}

export function isTimelineMessage(item: SessionTimelineItem): boolean {
  return item.kind === 'message';
}

export function timelineHasDetail(item: SessionTimelineItem): boolean {
  return Boolean(item.content_markdown || item.detail_json || item.actor_name || item.subtitle);
}

export function formatTimelineTime(value: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat([], {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  }).format(date);
}

export function formatTimelineDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

export const timelineKindFilterOptions = computed(() => [
  { label: t('sessionDetail.timelineKinds.all'), value: '' },
  { label: t('sessionDetail.timelineKinds.message'), value: 'message' },
  { label: t('sessionDetail.timelineKinds.tool'), value: 'tool' },
  { label: t('sessionDetail.timelineKinds.skill'), value: 'skill' },
  { label: t('sessionDetail.timelineKinds.mcp'), value: 'mcp' },
]);

export const timelineSortOptions = computed(() => [
  { label: t('sessionDetail.timelineSort.desc'), value: 'desc' },
  { label: t('sessionDetail.timelineSort.asc'), value: 'asc' },
]);

export function filterTimelineItems(
  items: SessionTimelineItem[],
  kindFilter: string | null | undefined,
  sortOrder: string,
): SessionTimelineItem[] {
  let list = items;
  if (kindFilter) {
    list = list.filter((item) => item.kind === kindFilter);
  }
  const sorted = [...list];
  sorted.sort((a, b) => {
    const ta = new Date(a.occurred_at).getTime() || 0;
    const tb = new Date(b.occurred_at).getTime() || 0;
    return sortOrder === 'asc' ? ta - tb : tb - ta;
  });
  return sorted;
}
