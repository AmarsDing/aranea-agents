/**
 * 定时任务页的统一列表：平台定时任务（cron_task）+ 运行台自动化（fleet automation）。
 * 纯函数，无网络；页面按 state / kind / 关键字筛。
 */
import type { Agent } from '../agents/types';
import type { Team } from '../teams/types';
import type { CronTaskConfig, CronTaskMetadata, CronTaskRow } from './types';
import type { FleetAutomation, FleetProject } from './types';
import {
  CRON_SYSTEM_TARGETS,
  cronTargetKind,
  describeCronSchedule,
  describeFleetSchedule,
  parseCronJSON,
} from '../../components/cron/cronTaskUtils';

export type ScheduleState = 'active' | 'paused' | 'dead';
/** 上次结果的统一口径：success / failure / skipped / running / ''（没跑过）。 */
export type ScheduleOutcome = 'success' | 'failure' | 'skipped' | 'running' | '';

export interface ScheduleItem {
  key: string; // cron:<id> | fleet:<id>
  kind: 'cron' | 'fleet';
  id: string;
  name: string;
  sub: string; // 任务标识 / 项目名
  schedule: string; // 人话计划
  target: string; // Agent: xx / Team: xx / 系统 · xx / claude · 项目
  state: ScheduleState;
  enabled: boolean;
  nextRunAt: string;
  lastRunAt: string;
  lastOutcome: ScheduleOutcome;
  lastError: string;
  /** 连续失败次数（定时任务进死信前的计数）；自动化没有这个概念，恒为 0。 */
  failureCount: number;
  searchText: string;
}

export const STATE_LABEL: Record<ScheduleState, string> = {
  active: '运行中',
  paused: '已暂停',
  dead: '失败停用',
};

function outcomeOf(status: string | undefined): ScheduleOutcome {
  if (!status) return '';
  if (status === 'success') return 'success';
  if (status === 'failure' || status.startsWith('failed')) return 'failure';
  if (status === 'pending' || status === 'started') return 'running';
  if (status.startsWith('skipped')) return 'skipped';
  return '';
}

export function cronTargetLabel(cfg: CronTaskConfig, agentId: string, agents: Agent[], teams: Team[]): string {
  const kind = cronTargetKind(cfg);
  if (CRON_SYSTEM_TARGETS[kind]) return `系统 · ${CRON_SYSTEM_TARGETS[kind]}`;
  if (kind === 'team') {
    const team = teams.find((x) => x.id === cfg.team_id);
    return `Team · ${team?.display_name || team?.team_key || cfg.team_id || '未选'}`;
  }
  if (!agentId) return 'Agent · 默认';
  const agent = agents.find((x) => x.id === agentId);
  return `Agent · ${agent?.display_name || agent?.agent_key || agentId}`;
}

export function cronItem(row: CronTaskRow, agents: Agent[], teams: Team[]): ScheduleItem {
  const cfg = parseCronJSON<CronTaskConfig>(row.config_json, {});
  const meta = parseCronJSON<CronTaskMetadata>(row.metadata_json, {});
  const state: ScheduleState = row.status === 'dead' ? 'dead' : row.enabled ? 'active' : 'paused';
  const target = cronTargetLabel(cfg, row.agent_id, agents, teams);
  const schedule = describeCronSchedule(cfg);
  return {
    key: `cron:${row.id}`,
    kind: 'cron',
    id: row.id,
    name: row.name || row.key,
    sub: row.key,
    schedule,
    target,
    state,
    enabled: row.enabled,
    nextRunAt: state === 'active' ? meta.next_run_at || '' : '',
    lastRunAt: meta.last_run_at || '',
    lastOutcome: outcomeOf(meta.last_run_status),
    lastError: meta.last_error || '',
    failureCount: Number(meta.failure_count || 0),
    searchText: [row.key, row.name, row.description, cfg.message, cfg.cron_expression, schedule, target]
      .join(' ')
      .toLowerCase(),
  };
}

export function fleetItem(a: FleetAutomation, projects: FleetProject[]): ScheduleItem {
  const project = projects.find((p) => p.id === a.projectId)?.name || a.projectId;
  const schedule = describeFleetSchedule(a.schedule);
  const failed = a.lastStatus.startsWith('failed');
  return {
    key: `fleet:${a.id}`,
    kind: 'fleet',
    id: a.id,
    name: a.name,
    sub: project,
    schedule,
    target: `${a.agent} · ${a.worktreeMode === 'primary' ? '主检出' : '新 worktree'}`,
    state: a.enabled ? 'active' : 'paused',
    enabled: a.enabled,
    nextRunAt: a.enabled ? a.nextRunAt : '',
    lastRunAt: a.lastRunAt,
    // 自动化只记录「已启动 / 启动失败」，启动后 agent 干得怎样在运行台里看
    lastOutcome: failed ? 'failure' : a.lastStatus ? 'success' : '',
    lastError: failed ? a.lastStatus.replace(/^failed:\s*/, '') : '',
    failureCount: 0,
    searchText: [a.name, a.prompt, a.schedule, schedule, project, a.agent].join(' ').toLowerCase(),
  };
}

/** 列表排序：失败停用最前（要人处理），其次运行中按下次时间，最后已暂停。 */
export function sortScheduleItems(items: ScheduleItem[]): ScheduleItem[] {
  const rank: Record<ScheduleState, number> = { dead: 0, active: 1, paused: 2 };
  return items.slice().sort((a, b) => {
    if (rank[a.state] !== rank[b.state]) return rank[a.state] - rank[b.state];
    const an = a.nextRunAt || '9999';
    const bn = b.nextRunAt || '9999';
    return an === bn ? a.name.localeCompare(b.name) : an < bn ? -1 : 1;
  });
}
