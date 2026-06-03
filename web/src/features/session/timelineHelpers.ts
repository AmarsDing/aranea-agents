import type { SessionTimelineSummary } from './types';

export type TimelineStat = {
  key: string;
  label: string;
  value: number;
  icon: string;
};

export function buildTimelineStats(summary?: SessionTimelineSummary | null): TimelineStat[] {
  return [
    { key: 'message', label: 'Messages', value: summary?.message_count ?? 0, icon: 'chat' },
    { key: 'tool', label: 'Tools', value: summary?.tool_count ?? 0, icon: 'build' },
    { key: 'skill', label: 'Skills', value: summary?.skill_count ?? 0, icon: 'auto_awesome' },
    { key: 'mcp', label: 'MCP', value: summary?.mcp_count ?? 0, icon: 'hub' },
  ];
}
