/**
 * Cron 表单纯逻辑（无网络）。与 `components/cron/*.vue` 共址，见 aranea-frontend-guide SKILL §3.3。
 */
import type { Agent } from '../../features/agents/types';
import type { PlatformResourceInput } from '../../features/platform/types';
import type { CronTaskConfig, CronTaskFormValue, CronTaskRow } from '../../features/cron/types';
import type { Team } from '../../features/teams/types';

export const cronScheduleToggleOptions = [
  { label: '每隔', value: 'interval' as const },
  { label: 'Cron', value: 'cron' as const },
  { label: '一次', value: 'once' as const },
];

export const cronTargetToggleOptions = [
  { label: 'Agent', value: 'agent' as const },
  { label: 'Team', value: 'team' as const },
];

export const cronSlugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export function emptyCronTaskForm(): CronTaskFormValue {
  return {
    name: '',
    display_name: '',
    description: '',
    target_type: 'agent',
    agent_id: '',
    team_id: '',
    schedule_type: 'interval',
    interval_minutes: 15,
    cron_expression: '0 * * * *',
    run_at_date: '',
    run_at_time: '',
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai',
    message: '',
    retry_max_attempts: 3,
    enabled: true,
  };
}

export function splitCronRunAt(value?: string): { date: string; time: string } {
  if (!value) return { date: '', time: '' };
  const normalized = value.replace(' ', 'T');
  const [date = '', rawTime = ''] = normalized.split('T');
  return { date, time: rawTime.slice(0, 5) };
}

export function parseCronJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

/** 将 `row` 同步进 `form`（就地修改）。 */
export function applyCronRowToForm(row: CronTaskRow | null, form: CronTaskFormValue): void {
  const config = parseCronJSON<CronTaskConfig>(row?.config_json, {});
  const runAt = splitCronRunAt(config.run_at);
  Object.assign(form, {
    name: row?.key || '',
    display_name: row?.name || '',
    description: row?.description || '',
    target_type: cronTargetKind(config),
    agent_id: row?.agent_id || '',
    team_id: config.team_id || '',
    schedule_type: config.schedule_type || 'interval',
    interval_minutes: Math.max(1, Math.round((config.interval_seconds || 900) / 60)),
    cron_expression: config.cron_expression || '0 * * * *',
    run_at_date: runAt.date,
    run_at_time: runAt.time,
    // 已有任务没存时区时调度器按 UTC 算；回填浏览器时区会让「只改了个名字」也悄悄改掉执行时间。
    timezone: config.timezone || (row ? 'UTC' : Intl.DateTimeFormat().resolvedOptions().timeZone || 'Asia/Shanghai'),
    message: config.message || '',
    retry_max_attempts: config.retry_max_attempts ?? 3,
    enabled: row?.enabled ?? true,
  });
}

export function buildCronTaskConfig(form: CronTaskFormValue, prior?: CronTaskConfig): CronTaskConfig {
  return {
    // 先铺已有配置再覆写表单字段：无表单映射的键（如 missed_run_grace_seconds——
    // 经 API/种子配置后）不得在 UI 保存时被静默丢弃（C6 修正）。
    ...prior,
    target_type: form.target_type,
    team_id: form.target_type === 'team' ? form.team_id : '',
    schedule_type: form.schedule_type,
    cron_expression: form.schedule_type === 'cron' ? form.cron_expression.trim() : '',
    interval_seconds: form.schedule_type === 'interval' ? Number(form.interval_minutes) * 60 : 0,
    run_at: form.schedule_type === 'once' ? `${form.run_at_date}T${form.run_at_time}:00` : '',
    timezone: form.timezone.trim() || 'Asia/Shanghai',
    message: form.message.trim(),
    retry_max_attempts: Math.max(0, form.retry_max_attempts ?? 3),
  };
}

export function buildCronTaskPayload(form: CronTaskFormValue, row: CronTaskRow | null): PlatformResourceInput {
  const existingConfig = parseCronJSON<CronTaskConfig>(row?.config_json, {});
  const payload: PlatformResourceInput = {
    key: form.name.trim(),
    name: form.display_name.trim() || form.name.trim(),
    description: form.description.trim(),
    agent_id: form.target_type === 'agent' ? form.agent_id || '' : '',
    enabled: form.enabled,
    status: form.enabled ? 'active' : 'paused',
    sort_order: row?.sort_order || 0,
    config_json: JSON.stringify(buildCronTaskConfig(form, existingConfig)),
  };
  // 编辑时不回传 metadata：那是调度器在写的计数 / 下次时间，回传打开表单时的旧快照会把
  // 期间新增的执行统计覆盖掉；后端对未传的 metadata 保持原样。
  if (!row) payload.metadata_json = '{}';
  return payload;
}

export function isCronTargetValid(form: CronTaskFormValue): boolean {
  return form.target_type === 'agent' || isCronSystemTarget(form.target_type) || Boolean(form.team_id);
}

export function isCronScheduleValid(form: CronTaskFormValue): boolean {
  if (form.schedule_type === 'interval') return Number(form.interval_minutes) > 0;
  if (form.schedule_type === 'cron') return form.cron_expression.trim().split(/\s+/).length === 5;
  return Boolean(form.run_at_date && form.run_at_time);
}

export function canSaveCronForm(form: CronTaskFormValue, isEdit = false): boolean {
  return (
    // 编辑时标识只读不参与 slug 校验：内置系统任务（如 dream_cycle）key 含下划线，
    // 创建期 slug 规则不应回填拦截已有任务的保存。
    (isEdit || cronSlugPattern.test(form.name)) &&
    Boolean(form.message.trim()) &&
    isCronTargetValid(form) &&
    isCronScheduleValid(form)
  );
}

export function cronSlugRule(value: string): true | string {
  return cronSlugPattern.test(value) || '仅支持小写字母、数字、连字符，且不能以连字符开头或结尾';
}

export function cronPositiveMinutesRule(value: number): true | string {
  return Number(value) > 0 || '请输入大于 0 的分钟数';
}

export function cronExpressionRule(value: string): true | string {
  return value.trim().split(/\s+/).length === 5 || '请输入标准 5 字段 cron 表达式';
}

export function cronMessageRule(value: string): true | string {
  return Boolean(value.trim()) || '请填写 Agent 要执行的消息';
}

export function cronTeamRule(value: string): true | string {
  return Boolean(value) || '请选择要调动的 Team';
}

export function cronAgentSelectOptions(agents: Agent[]): Array<{ label: string; value: string }> {
  return [
    { label: '默认', value: '' },
    ...agents.map((agent) => ({
      label: agent.display_name || agent.agent_key || agent.id,
      value: agent.id,
    })),
  ];
}

export function cronTeamSelectOptions(teams: Team[]): Array<{ label: string; value: string }> {
  return teams.map((team) => ({
    label: team.display_name || team.team_key || team.id,
    value: team.id,
  }));
}

/** Format an ISO date string for display; returns '—' for empty/invalid values. */
export function formatCronDate(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

const pad2 = (n: number) => String(n).padStart(2, '0');

/** 列表里的短时间：同年「11-20 03:00」，跨年带年份；空值 / 非法值返回 '—'。 */
export function formatCronShort(value?: string, now = new Date()): string {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '—';
  const md = `${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
  return d.getFullYear() === now.getFullYear() ? md : `${d.getFullYear()}-${md}`;
}

/** 后端 cronTargetType 的同口径：空 target_type 有 team_id 视为 team，否则 agent。 */
export function cronTargetKind(cfg: CronTaskConfig): string {
  const raw = String(cfg.target_type || '')
    .trim()
    .toLowerCase();
  if (raw) return raw;
  return cfg.team_id ? 'team' : 'agent';
}

/** 系统内置目标（没有 Agent / Team 可选，表单里只读）。 */
export const CRON_SYSTEM_TARGETS: Record<string, string> = {
  model_registry_sync: '模型目录同步',
  mvp_rss_ingest: 'RSS 内容采集',
  gm_patrol: '总经理巡视',
  gm_report: '总经理汇报',
};

export function isCronSystemTarget(kind: string): boolean {
  return kind in CRON_SYSTEM_TARGETS;
}

const WEEKDAY_CN = ['日', '一', '二', '三', '四', '五', '六', '日'];

function describeWeekdays(field: string): string {
  if (field === '1-5') return '工作日';
  if (field === '0,6' || field === '6,0' || field === '6,7') return '周末';
  if (/^[0-7]$/.test(field)) return `每周${WEEKDAY_CN[Number(field)]}`;
  if (/^[0-7](,[0-7])+$/.test(field))
    return `每周${field
      .split(',')
      .map((d) => WEEKDAY_CN[Number(d)])
      .join('、')}`;
  return '';
}

/** 把 5 字段 cron 表达式翻成一句话；认不出的返回空串（调用方回退显示原表达式）。 */
export function describeCronExpression(expr: string): string {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) return '';
  const [min = '', hour = '', dom = '', mon = '', dow = ''] = parts;
  const num = (s: string) => /^\d+$/.test(s);
  if (mon !== '*') return '';
  if (num(min) && num(hour)) {
    const at = `${pad2(Number(hour))}:${pad2(Number(min))}`;
    if (dom === '*' && dow === '*') return `每天 ${at}`;
    if (dom === '*') {
      const days = describeWeekdays(dow);
      return days ? `${days} ${at}` : '';
    }
    if (num(dom) && dow === '*') return `每月 ${Number(dom)} 日 ${at}`;
    return '';
  }
  if (dom !== '*' || dow !== '*') return '';
  if (num(min) && hour === '*') return Number(min) === 0 ? '每小时整点' : `每小时第 ${Number(min)} 分`;
  const step = /^\*\/(\d+)$/.exec(min);
  if (step && hour === '*') return `每 ${Number(step[1])} 分钟`;
  if (min === '0' && /^\*\/(\d+)$/.test(hour)) return `每 ${Number(hour.slice(2))} 小时整点`;
  return '';
}

/** 间隔秒数的人话：每 15 分钟 / 每 2 小时 / 每天 / 每 3 天。 */
export function describeInterval(seconds?: number): string {
  const s = Number(seconds) > 0 ? Number(seconds) : 900; // 与后端缺省一致
  if (s % 86400 === 0) return s === 86400 ? '每天' : `每 ${s / 86400} 天`;
  if (s % 3600 === 0) return s === 3600 ? '每小时' : `每 ${s / 3600} 小时`;
  return `每 ${Math.max(1, Math.round(s / 60))} 分钟`;
}

/** 定时任务计划的一句话描述。 */
export function describeCronSchedule(cfg: CronTaskConfig): string {
  const type = cfg.schedule_type || 'interval';
  if (type === 'cron') {
    const expr = cfg.cron_expression || '';
    return describeCronExpression(expr) || `cron ${expr || '—'}`;
  }
  if (type === 'once') return `一次 · ${formatCronShort(cfg.run_at?.replace(' ', 'T'))}`;
  return describeInterval(cfg.interval_seconds);
}

const FLEET_WEEKDAY: Record<string, string> = {
  sun: '日',
  mon: '一',
  tue: '二',
  wed: '三',
  thu: '四',
  fri: '五',
  sat: '六',
};

/** 运行台自动化的计划（every 6h / daily 02:00 / weekdays 09:30 / weekly mon 09:00）翻成一句话。 */
export function describeFleetSchedule(raw: string): string {
  const f = raw.trim().toLowerCase().split(/\s+/);
  if (f[0] === 'hourly') return '每小时';
  if (f[0] === 'every' && f[1]) {
    const m = /^(\d+)(m|h)$/.exec(f[1]);
    if (m) return m[2] === 'h' ? `每 ${m[1]} 小时` : `每 ${m[1]} 分钟`;
  }
  if (f[0] === 'daily' && f[1]) return `每天 ${f[1]}`;
  if (f[0] === 'weekdays' && f[1]) return `工作日 ${f[1]}`;
  if (f[0] === 'weekly' && f[1] && f[2]) return `每周${FLEET_WEEKDAY[f[1]] ?? f[1]} ${f[2]}`;
  return raw;
}
