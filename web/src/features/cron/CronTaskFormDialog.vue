<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="cron-form-card">
      <q-card-section class="row items-start justify-between q-gutter-md">
        <div>
          <div class="text-h6">{{ row ? "编辑定时任务" : "创建定时任务" }}</div>
          <div class="text-caption text-grey-7">安排定期 Agent 任务，计划字段会保存到 config_json。</div>
        </div>
        <q-btn flat dense round icon="close" aria-label="关闭" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />

      <q-card-section>
        <q-form class="row q-col-gutter-md" @submit.prevent="save">
          <q-input v-model="form.name" class="col-12 col-md-6" dense outlined label="名称 *" placeholder="my-daily-task" :rules="[slugRule]" />
          <q-input v-model="form.display_name" class="col-12 col-md-6" dense outlined label="展示名称" />
          <q-input v-model="form.description" class="col-12" dense outlined autogrow type="textarea" label="描述" />

          <div class="col-12">
            <div class="section-label q-mb-sm">目标类型</div>
            <q-btn-toggle
              v-model="form.target_type"
              spread
              no-caps
              unelevated
              toggle-color="primary"
              color="grey-2"
              text-color="grey-9"
              :options="targetOptions"
            />
          </div>

          <q-select
            v-if="form.target_type === 'agent'"
            v-model="form.agent_id"
            class="col-12"
            dense
            outlined
            clearable
            emit-value
            map-options
            label="Agent"
            hint="留空时调度器使用默认 Agent"
            :options="agentOptions"
          />
          <q-select
            v-else
            v-model="form.team_id"
            class="col-12"
            dense
            outlined
            emit-value
            map-options
            label="Team *"
            :options="teamOptions"
            :rules="[teamRule]"
          />

          <div class="col-12">
            <div class="section-label q-mb-sm">计划类型</div>
            <q-btn-toggle
              v-model="form.schedule_type"
              spread
              no-caps
              unelevated
              toggle-color="orange"
              color="grey-2"
              text-color="grey-9"
              :options="scheduleOptions"
            />
          </div>

          <q-input
            v-if="form.schedule_type === 'interval'"
            v-model.number="form.interval_minutes"
            class="col-12 col-md-6"
            dense
            outlined
            type="number"
            min="1"
            suffix="分钟"
            label="每隔 *"
            :rules="[positiveNumberRule]"
          />
          <q-input
            v-if="form.schedule_type === 'cron'"
            v-model="form.cron_expression"
            class="col-12"
            dense
            outlined
            label="Cron 表达式 *"
            placeholder="0 * * * *"
            hint="标准 5 字段 cron: 分 时 日 月 周"
            :rules="[cronRule]"
          />
          <template v-if="form.schedule_type === 'once'">
            <q-input v-model="form.run_at_date" class="col-12 col-md-6" dense outlined mask="####-##-##" label="执行日期 *" placeholder="2026-04-22">
              <template #append>
                <q-icon name="event" class="cursor-pointer">
                  <q-popup-proxy cover transition-show="scale" transition-hide="scale">
                    <q-date v-model="form.run_at_date" mask="YYYY-MM-DD" />
                  </q-popup-proxy>
                </q-icon>
              </template>
            </q-input>
            <q-input v-model="form.run_at_time" class="col-12 col-md-6" dense outlined mask="##:##" label="执行时间 *" placeholder="09:00">
              <template #append>
                <q-icon name="access_time" class="cursor-pointer">
                  <q-popup-proxy cover transition-show="scale" transition-hide="scale">
                    <q-time v-model="form.run_at_time" format24h />
                  </q-popup-proxy>
                </q-icon>
              </template>
            </q-input>
          </template>

          <q-input v-model="form.timezone" class="col-12 col-md-6" dense outlined label="时区" placeholder="Asia/Shanghai" />
          <q-toggle v-model="form.enabled" class="col-12 col-md-6" color="primary" label="启用任务" />
          <q-input
            v-model="form.message"
            class="col-12"
            dense
            outlined
            autogrow
            type="textarea"
            label="消息 *"
            placeholder="Agent 应该做什么?"
            :rules="[messageRule]"
          />

          <div v-if="serverError" class="col-12 text-negative">{{ serverError }}</div>
        </q-form>
      </q-card-section>

      <q-separator />
      <q-card-actions align="right">
        <q-btn flat rounded label="取消" @click="$emit('update:modelValue', false)" />
        <q-btn color="orange" text-color="white" rounded unelevated :label="row ? '保存' : '创建'" :loading="saving" :disable="!canSave" @click="save" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import type { Agent } from "../../features/agents/api";
import type { Team } from "../../api/client";
import type { PlatformResourceInput } from "../platform/api";
import { createCronTask, updateCronTask } from "./api";
import type { CronTaskConfig, CronTaskFormValue, CronTaskMetadata, CronTaskRow } from "./types";

const props = defineProps<{
  modelValue: boolean;
  row: CronTaskRow | null;
  agents: Agent[];
  teams: Team[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  saved: [row: CronTaskRow];
}>();

const $q = useQuasar();
const saving = ref(false);
const serverError = ref("");
const form = reactive<CronTaskFormValue>(emptyForm());

const scheduleOptions = [
  { label: "每隔", value: "interval" },
  { label: "Cron", value: "cron" },
  { label: "一次", value: "once" }
];
const targetOptions = [
  { label: "Agent", value: "agent" },
  { label: "Team", value: "team" }
];
const agentOptions = computed(() => [
  { label: "默认", value: "" },
  ...props.agents.map((agent) => ({
    label: agent.display_name || agent.agent_key || agent.id,
    value: agent.id
  }))
]);
const teamOptions = computed(() =>
  props.teams.map((team) => ({
    label: team.display_name || team.team_key || team.id,
    value: team.id
  }))
);
const canSave = computed(() => slugPattern.test(form.name) && Boolean(form.message.trim()) && isTargetValid() && isScheduleValid());

watch(
  () => props.modelValue,
  (open) => {
    if (open) resetForm();
  }
);

function emptyForm(): CronTaskFormValue {
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
    enabled: true
  };
}

function resetForm() {
  serverError.value = "";
  const row = props.row;
  const config = parseJSON<CronTaskConfig>(row?.config_json, {});
  const runAt = splitRunAt(config.run_at);
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
    enabled: row?.enabled ?? true
  });
}

async function save() {
  serverError.value = "";
  saving.value = true;
  try {
    const payload = buildPayload();
    const saved = props.row ? await updateCronTask(props.row.id, payload) : await createCronTask(payload);
    emit("saved", saved);
    emit("update:modelValue", false);
    $q.notify({ type: "positive", message: "定时任务已保存" });
  } catch (err) {
    serverError.value = err instanceof Error ? err.message : "保存失败";
    $q.notify({ type: "negative", message: serverError.value });
  } finally {
    saving.value = false;
  }
}

function buildPayload(): PlatformResourceInput {
  const existingMetadata = parseJSON<CronTaskMetadata>(props.row?.metadata_json, {});
  return {
    key: form.name.trim(),
    name: form.display_name.trim() || form.name.trim(),
    description: form.description.trim(),
    agent_id: form.target_type === "agent" ? form.agent_id || "" : "",
    enabled: form.enabled,
    status: form.enabled ? "active" : "paused",
    sort_order: props.row?.sort_order || 0,
    config_json: JSON.stringify(buildConfig()),
    metadata_json: JSON.stringify(existingMetadata)
  };
}

function buildConfig(): CronTaskConfig {
  return {
    target_type: form.target_type,
    team_id: form.target_type === "team" ? form.team_id : "",
    schedule_type: form.schedule_type,
    cron_expression: form.schedule_type === "cron" ? form.cron_expression.trim() : "",
    interval_seconds: form.schedule_type === "interval" ? Number(form.interval_minutes) * 60 : 0,
    run_at: form.schedule_type === "once" ? `${form.run_at_date}T${form.run_at_time}:00` : "",
    timezone: form.timezone.trim() || "Asia/Shanghai",
    message: form.message.trim()
  };
}

function isScheduleValid() {
  if (form.schedule_type === "interval") return Number(form.interval_minutes) > 0;
  if (form.schedule_type === "cron") return form.cron_expression.trim().split(/\s+/).length === 5;
  return Boolean(form.run_at_date && form.run_at_time);
}

function isTargetValid() {
  return form.target_type === "agent" || Boolean(form.team_id);
}

function splitRunAt(value?: string) {
  if (!value) return { date: "", time: "" };
  const normalized = value.replace(" ", "T");
  const [date = "", rawTime = ""] = normalized.split("T");
  return { date, time: rawTime.slice(0, 5) };
}

function parseJSON<T>(value: string | undefined, fallback: T): T {
  if (!value) return fallback;
  try {
    return JSON.parse(value) as T;
  } catch {
    return fallback;
  }
}

const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function slugRule(value: string) {
  return slugPattern.test(value) || "仅支持小写字母、数字、连字符，且不能以连字符开头或结尾";
}

function positiveNumberRule(value: number) {
  return Number(value) > 0 || "请输入大于 0 的分钟数";
}

function cronRule(value: string) {
  return value.trim().split(/\s+/).length === 5 || "请输入标准 5 字段 cron 表达式";
}

function messageRule(value: string) {
  return Boolean(value.trim()) || "请填写 Agent 要执行的消息";
}

function teamRule(value: string) {
  return Boolean(value) || "请选择要调动的 Team";
}
</script>

<style scoped>
.cron-form-card {
  max-width: 96vw;
  width: 760px;
}

.section-label {
  color: #5d4037;
  font-weight: 800;
}

body.body--dark .section-label {
  color: inherit;
}
</style>
