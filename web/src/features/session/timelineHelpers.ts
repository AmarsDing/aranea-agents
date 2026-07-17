import type { SessionTimelineSummary } from './types';
import { i18n } from '../../i18n';

const t = (key: string) => i18n.global.t(key);

export type TimelineStat = {
  key: string;
  label: string;
  value: number;
  icon: string;
};

export function buildTimelineStats(summary?: SessionTimelineSummary | null): TimelineStat[] {
  return [
    {
      key: 'message',
      label: t('sessionDetail.timelineStats.message'),
      value: summary?.message_count ?? 0,
      icon: 'chat',
    },
    { key: 'tool', label: t('sessionDetail.timelineStats.tool'), value: summary?.tool_count ?? 0, icon: 'build' },
    {
      key: 'skill',
      label: t('sessionDetail.timelineStats.skill'),
      value: summary?.skill_count ?? 0,
      icon: 'auto_awesome',
    },
    { key: 'mcp', label: t('sessionDetail.timelineStats.mcp'), value: summary?.mcp_count ?? 0, icon: 'hub' },
  ];
}
