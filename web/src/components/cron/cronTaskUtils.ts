/**
 * Cron 表单纯逻辑（无网络）。与 `components/cron/*.vue` 共址，见 aranea-frontend-guide SKILL §3.3。
 */
import type { Agent } from "../../features/agents/types";
import type { PlatformResourceInput } from "../../features/platform/types";
import type { CronTaskConfig, CronTaskFormValue, CronTaskMetadata, CronTaskRow } from "../../features/cron/types";
import type { Team } from "../../features/teams/types";

export const cronScheduleToggleOptions = [
  { label: "每隔", value: "interval" as const },
  { label: "Cron", value: "cron" as const },
  { label: "一次", value: "once" as const }
];

export const cronTargetToggleOptions = [
  { label: "Agent", value: "agent" as const },
  { label: "Team", value: "team" as const }
];

export const cronSlugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export function emptyCronTaskForm(): CronTaskFormValue {
  return {
    name: "",
    display_name: "",
    description: "",
    target_type: "agent",
    agent_id: "",
    team_id: "",
    schedule_type: "interval",
    interval_minutes: 15,
    cron_expression: "0 * * * *",
    run_at_date: "",
    run_at_time: "",
    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Shanghai",
    message: "",
    retry_max_attempts: 3,
    enabled: true
  };
}

export function splitCronRunAt(value?: string): { date: string; time: string } {
  if (!value) return { date: "", time: "" };
  const normalized = value.replace(" ", "T");
  const [date = "", rawTime = ""] = normalized.split("T");
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
    name: row?.key || "",
    display_name: row?.name || "",
    description: row?.description || "",
    target_type: config.target_type || (config.team_id ? "team" : "agent"),
    agent_id: row?.agent_id || "",
    team_id: config.team_id || "",
    schedule_type: config.schedule_type || "interval",
    interval_minutes: Math.max(1, Math.round((config.interval_seconds || 900) / 60)),
    cron_expression: config.cron_expression || "0 * * * *",
    run_at_date: runAt.date,
    run_at_time: runAt.time,
    timezone: config.timezone || Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Shanghai",
    message: config.message || "",
    retry_max_attempts: config.retry_max_attempts ?? 3,
    enabled: row?.enabled ?? true
  });
}

export function buildCronTaskConfig(form: CronTaskFormValue): CronTaskConfig {
  return {
    target_type: form.target_type,
    team_id: form.target_type === "team" ? form.team_id : "",
    schedule_type: form.schedule_type,
    cron_expression: form.schedule_type === "cron" ? form.cron_expression.trim() : "",
    interval_seconds: form.schedule_type === "interval" ? Number(form.interval_minutes) * 60 : 0,
    run_at: form.schedule_type === "once" ? `${form.run_at_date}T${form.run_at_time}:00` : "",
    timezone: form.timezone.trim() || "Asia/Shanghai",
    message: form.message.trim(),
    retry_max_attempts: Math.max(0, form.retry_max_attempts ?? 3)
  };
}

export function buildCronTaskPayload(form: CronTaskFormValue, row: CronTaskRow | null): PlatformResourceInput {
  const existingMetadata = parseCronJSON<CronTaskMetadata>(row?.metadata_json, {});
  return {
    key: form.name.trim(),
    name: form.display_name.trim() || form.name.trim(),
    description: form.description.trim(),
    agent_id: form.target_type === "agent" ? form.agent_id || "" : "",
    enabled: form.enabled,
    status: form.enabled ? "active" : "paused",
    sort_order: row?.sort_order || 0,
    config_json: JSON.stringify(buildCronTaskConfig(form)),
    metadata_json: JSON.stringify(existingMetadata)
  };
}

export function isCronTargetValid(form: CronTaskFormValue): boolean {
  return form.target_type === "agent" || Boolean(form.team_id);
}

export function isCronScheduleValid(form: CronTaskFormValue): boolean {
  if (form.schedule_type === "interval") return Number(form.interval_minutes) > 0;
  if (form.schedule_type === "cron") return form.cron_expression.trim().split(/\s+/).length === 5;
  return Boolean(form.run_at_date && form.run_at_time);
}

export function canSaveCronForm(form: CronTaskFormValue): boolean {
  return (
    cronSlugPattern.test(form.name) &&
    Boolean(form.message.trim()) &&
    isCronTargetValid(form) &&
    isCronScheduleValid(form)
  );
}

export function cronSlugRule(value: string): true | string {
  return cronSlugPattern.test(value) || "仅支持小写字母、数字、连字符，且不能以连字符开头或结尾";
}

export function cronPositiveMinutesRule(value: number): true | string {
  return Number(value) > 0 || "请输入大于 0 的分钟数";
}

export function cronExpressionRule(value: string): true | string {
  return value.trim().split(/\s+/).length === 5 || "请输入标准 5 字段 cron 表达式";
}

export function cronMessageRule(value: string): true | string {
  return Boolean(value.trim()) || "请填写 Agent 要执行的消息";
}

export function cronTeamRule(value: string): true | string {
  return Boolean(value) || "请选择要调动的 Team";
}

export function cronAgentSelectOptions(agents: Agent[]): Array<{ label: string; value: string }> {
  return [
    { label: "默认", value: "" },
    ...agents.map((agent) => ({
      label: agent.display_name || agent.agent_key || agent.id,
      value: agent.id
    }))
  ];
}

export function cronTeamSelectOptions(teams: Team[]): Array<{ label: string; value: string }> {
  return teams.map((team) => ({
    label: team.display_name || team.team_key || team.id,
    value: team.id
  }));
}
