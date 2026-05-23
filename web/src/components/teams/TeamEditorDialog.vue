<!--
  Team 域展示组件：仅 props / emits（vue-design.md §0.2）。
  路径约定：vue-design.md §2 → `web/src/components/teams/`。
-->
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card :class="['team-editor app-dialog-card app-dialog-card--xl app-glass-dialog', { 'is-dark': isDark }]">
      <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">{{ editingId ? "编辑 Team" : "新增 Team" }}</div>
          <div class="app-glass-dialog__subtitle">
            并行模式按「并行批大小」分批同时执行 Worker；coordinator/adaptive 的外圈迭代可在表单中配置。
          </div>
        </div>
        <q-btn flat round dense icon="close" v-close-popup />
      </q-card-section>
      <div class="app-glass-dialog__scroll">
        <div class="team-editor-workspace">
          <div class="team-editor-workspace__left q-gutter-md">
      <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md q-pa-none">
        <div class="app-dialog-section">
          <div class="app-form-field-grid app-form-field-grid--2col items-center">
            <div class="app-grid-span-full">
              <div class="text-subtitle2">Team 模板</div>
              <div class="text-caption text-grey-7">选择模板可快速生成成员角色和编排参数。</div>
            </div>
            <q-select
              class="team-control app-field-md"
              dense
              outlined
              clearable
              emit-value
              map-options
              label="选择模板"
              :model-value="selectedTemplateKey"
              :options="teamTemplateOptions"
              @update:model-value="onTemplatePick"
            />
          </div>
        </div>

        <div class="app-form-field-grid app-form-field-grid--wide">
          <q-input v-model.trim="form.display_name" class="team-control" dense outlined label="Team 名称 *" />
          <q-input v-model.trim="form.team_key" class="team-control" dense outlined label="Team Key *" hint="小写字母、数字、连字符" />
          <q-input v-model.trim="form.app_name" class="team-control" dense outlined label="App Name" hint="应用标识，留空则使用 Team Key" />
          <q-select v-model="definition.mode" class="team-control" dense outlined emit-value map-options label="编排模式" :options="modeOptions" />
          <q-select v-model="form.status" class="team-control" dense outlined emit-value map-options label="状态" :options="statusOptions" />
          <q-input
            v-if="definition.mode === 'parallel'"
            v-model.number="definition.max_concurrency"
            class="team-control"
            dense
            outlined
            type="number"
            min="1"
            label="并行批大小"
            hint="每批同时执行的 Worker 数，批与批之间顺序执行"
          />
          <q-input
            v-else-if="definition.mode === 'coordinator' || definition.mode === 'adaptive'"
            v-model.number="definition.loop_max_iterations"
            class="team-control"
            dense
            outlined
            type="number"
            min="0"
            max="32"
            label="外圈循环迭代"
            hint="0 = 默认 1 轮外圈迭代（每轮整链成员各跑一遍；轮数×成员数会成倍拉长耗时与超时风险）"
          />
          <q-input
            v-else-if="definition.mode === 'critic_loop'"
            v-model.number="criticLoopMaxIterations"
            class="team-control"
            dense
            outlined
            type="number"
            min="1"
            max="32"
            label="评审迭代次数"
            hint="对应 critic_loop.max_iterations"
          />
          <q-input v-model="definition.description" class="team-control app-field-long" dense outlined autogrow type="textarea" label="Team 说明" />
        </div>

        <q-expansion-item icon="settings" label="运行时 / 失败策略（OrchestrationSpec v2）" default-opened caption="runtime_engine · failure_policy · 熔断">
          <q-banner v-if="nativeLocked" dense rounded class="bg-orange-1 text-dark q-mb-sm">
            Native 执行引擎仅平台管理员可选；当前将使用 Graph。
          </q-banner>
          <div class="app-form-field-grid app-form-field-grid--wide q-mt-sm">
            <q-select
              v-model="runtimeEngine"
              class="team-control"
              dense
              outlined
              emit-value
              map-options
              label="执行引擎 runtime_engine"
              hint="默认 Graph；Native 需 admin 权限 + 后端 ARANEA_TEAM_NATIVE=1"
              :options="filteredRuntimeEngineOptions"
            />
            <q-select
              v-model="failureDefault"
              class="team-control"
              dense
              outlined
              emit-value
              map-options
              clearable
              label="默认失败策略"
              :options="failureDefaultOptions"
            />
            <q-select
              v-if="definition.mode === 'parallel'"
              v-model="parallelFail"
              class="team-control"
              dense
              outlined
              emit-value
              map-options
              clearable
              label="并行失败策略 parallel_fail"
              :options="parallelFailOptions"
            />
            <q-input
              v-model.number="failureRetryMax"
              class="team-control"
              dense
              outlined
              type="number"
              min="0"
              max="10"
              label="默认重试次数"
            />
            <q-input
              v-model.number="circuitFailureThreshold"
              class="team-control"
              dense
              outlined
              type="number"
              min="0"
              max="100"
              label="熔断阈值"
              hint="0=禁用"
            />
            <q-select
              v-model="failureOnError"
              class="team-control"
              dense
              outlined
              emit-value
              map-options
              clearable
              label="错误接管 on_error"
              :options="failureOnErrorOptions"
            />
          </div>
        </q-expansion-item>

        <div class="app-form-field-grid app-form-field-grid--wide">
          <q-input
            v-model.number="definition.timeout_seconds"
            class="team-control"
            dense
            outlined
            type="number"
            min="0"
            max="7200"
            label="单次运行超时（秒）"
            hint="0=仅遵循 HTTP/反代超时；否则后端 120～7200s。coordinator/长任务建议 ≥600，并与 Nginx proxy_read_timeout 等对齐"
          />
          <q-select
            v-model="definition.intent_anchor_agent_id"
            class="team-control app-field-md"
            dense
            outlined
            clearable
            emit-value
            map-options
            label="Intent / 选项锚定成员（可选）"
            hint="intent 预处理与 user options 的锚点；留空则用排序后首位启用成员"
            :options="intentAnchorOptions"
          />
        </div>

        <q-expansion-item icon="sync_alt" label="A2A 协议">
          <div class="app-form-field-grid q-mt-sm">
            <q-toggle v-model="a2aEnabled" color="primary" label="启用 A2A 信封" />
            <q-input v-model="a2aEnvelopeVersion" class="team-control" dense outlined label="Envelope Version" />
            <q-select v-model="a2aMessageFormat" class="team-control" dense outlined emit-value map-options label="消息格式" :options="a2aFormatOptions" />
            <q-input v-model.number="a2aMaxPayloadChars" class="team-control" dense outlined type="number" min="500" label="最大载荷字符" />
            <q-toggle v-model="a2aIncludeTrace" class="app-grid-span-full" color="primary" label="包含 trace metadata" />
          </div>
        </q-expansion-item>

        <q-separator />
        <div class="row items-center justify-between">
          <div class="text-subtitle2">成员 Agent</div>
          <q-btn flat rounded color="primary" icon="add" label="添加成员" @click="$emit('addMember')" />
        </div>
        <div class="member-editor-list">
          <q-card v-for="(member, index) in definition.members" :key="index" flat bordered class="member-editor">
            <q-card-section class="app-form-field-grid app-form-field-grid--wide items-center">
              <q-select v-model="member.agent_id" class="team-control" dense outlined emit-value map-options label="Agent" :options="agentOptions" />
              <q-select v-model="member.role" class="team-control" dense outlined emit-value map-options label="角色" :options="roleOptions" />
              <q-input v-model="member.name" class="team-control" dense outlined label="成员名称" />
              <q-input v-model="member.task_prompt" class="team-control team-control--wide" dense outlined autogrow type="textarea" label="职责 / 任务说明" />
              <q-input v-model.number="member.sort_order" class="team-control" dense outlined type="number" label="顺序" />
              <q-toggle v-model="member.enabled" color="primary" />
              <div class="app-actions-bar app-actions-bar--start">
                <q-btn flat dense round color="negative" icon="delete" @click="$emit('removeMember', index)" />
              </div>
            </q-card-section>
          </q-card>
        </div>

        <q-expansion-item icon="code" label="definition_json 预览" dense>
          <pre class="definition-json">{{ definitionJSON }}</pre>
        </q-expansion-item>
      </q-card-section>
          </div>

          <aside class="team-editor-workspace__right">
            <TeamCompilePreview
              :team-id="editingId"
              :definition-json="definitionJSON"
              :is-dark="isDark"
            />
          </aside>
        </div>
      </div>
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn flat rounded no-caps label="取消" v-close-popup />
        <q-btn color="primary" rounded unelevated no-caps label="保存" :loading="saving" :disable="!canSave" @click="$emit('save')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, watch } from "vue";
import { storeToRefs } from "pinia";
import type { TeamDefinition } from "../../features/teams/types";
import { useAuthStore } from "../../stores/auth";
import TeamCompilePreview from "./TeamCompilePreview.vue";
import { failureDefaultOptions, failureOnErrorOptions, modeOptions, parallelFailOptions, roleOptions, runtimeEngineOptions, statusOptions, teamTemplateOptions, type TeamTemplateKey } from "./teamUtils";

const authStore = useAuthStore();
const { isPlatformAdmin } = storeToRefs(authStore);

const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    selectedTemplateKey: TeamTemplateKey | null;
    editingId: string;
    form: {
      team_key: string;
      display_name: string;
      status: string;
      app_name: string;
    };
    definition: TeamDefinition;
    definitionJSON?: string;
    agentOptions: Array<{ label: string; value: string }>;
    saving: boolean;
    canSave: boolean;
    isDark: boolean;
  }>(),
  { definitionJSON: "{}", selectedTemplateKey: null },
);

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  "update:selectedTemplateKey": [value: TeamTemplateKey | null];
  addMember: [];
  removeMember: [index: number];
  applyTemplate: [template: TeamTemplateKey];
  save: [];
}>();

const intentAnchorOptions = computed(() =>
  props.definition.members
    .filter((m) => m.enabled !== false && String(m.agent_id || "").trim() !== "")
    .map((m) => ({
      label: [m.name, m.role].filter(Boolean).join(" · ") || String(m.agent_id).slice(0, 8),
      value: m.agent_id,
    })),
);

function onTemplatePick(key: TeamTemplateKey | null) {
  emit("update:selectedTemplateKey", key);
  if (key) emit("applyTemplate", key);
}

const filteredRuntimeEngineOptions = computed(() =>
  isPlatformAdmin.value ? runtimeEngineOptions : runtimeEngineOptions.filter((o) => o.value !== "native"),
);

const nativeLocked = computed(
  () => !isPlatformAdmin.value && String(props.definition.runtime_engine || "graph").toLowerCase() === "native",
);

watch(nativeLocked, (locked) => {
  if (locked) {
    props.definition.runtime_engine = "graph";
    props.definition.team_graph_runtime = true;
  }
}, { immediate: true });

const a2aFormatOptions = [
  { label: "Markdown + JSON", value: "markdown_json" },
  { label: "Plain", value: "plain" }
];
const a2aEnabled = computed({
  get: () => props.definition.a2a?.enabled ?? true,
  set: (value: boolean) => {
    props.definition.a2a = { ...props.definition.a2a, enabled: value };
  }
});
const a2aEnvelopeVersion = computed({
  get: () => props.definition.a2a?.envelope_version || "a2a.v1",
  set: (value: string) => {
    props.definition.a2a = { ...props.definition.a2a, envelope_version: value };
  }
});
const a2aMessageFormat = computed({
  get: () => props.definition.a2a?.message_format || "markdown_json",
  set: (value: string) => {
    props.definition.a2a = { ...props.definition.a2a, message_format: value };
  }
});
const a2aMaxPayloadChars = computed({
  get: () => props.definition.a2a?.max_payload_chars || 6000,
  set: (value: number) => {
    props.definition.a2a = { ...props.definition.a2a, max_payload_chars: value };
  }
});
const a2aIncludeTrace = computed({
  get: () => props.definition.a2a?.include_trace ?? true,
  set: (value: boolean) => {
    props.definition.a2a = { ...props.definition.a2a, include_trace: value };
  }
});

const criticLoopMaxIterations = computed({
  get: () => props.definition.critic_loop?.max_iterations ?? 2,
  set: (value: number) => {
    const n = Number.isFinite(value) ? Math.min(32, Math.max(1, Math.floor(value))) : 2;
    const prev = props.definition.critic_loop ?? { max_iterations: 2, score_threshold: 0.8 };
    props.definition.critic_loop = { ...prev, max_iterations: n };
  }
});

const runtimeEngine = computed({
  get: () => (String(props.definition.runtime_engine || "graph").toLowerCase() === "native" ? "native" : "graph"),
  set: (value: "native" | "graph") => {
    props.definition.runtime_engine = value;
    props.definition.team_graph_runtime = value === "graph";
  }
});

function ensureFailurePolicy() {
  if (!props.definition.failure_policy) {
    props.definition.failure_policy = { default: "retry_then_block", parallel_fail: "continue" };
  }
  return props.definition.failure_policy;
}

const failureDefault = computed({
  get: () => props.definition.failure_policy?.default ?? "retry_then_block",
  set: (value: string | null) => {
    const policy = ensureFailurePolicy();
    policy.default = value || "retry_then_block";
  }
});

const parallelFail = computed({
  get: () => props.definition.failure_policy?.parallel_fail ?? "continue",
  set: (value: string | null) => {
    const policy = ensureFailurePolicy();
    policy.parallel_fail = value || "continue";
  }
});

const failureRetryMax = computed({
  get: () => props.definition.failure_policy?.retry?.max_attempts ?? 3,
  set: (value: number) => {
    const policy = ensureFailurePolicy();
    policy.retry = { ...(policy.retry ?? {}), max_attempts: Math.max(0, Math.floor(Number(value) || 0)) };
  }
});

const circuitFailureThreshold = computed({
  get: () => props.definition.failure_policy?.circuit_breaker?.failure_threshold ?? 0,
  set: (value: number) => {
    const policy = ensureFailurePolicy();
    const n = Math.max(0, Math.floor(Number(value) || 0));
    if (n <= 0) {
      delete policy.circuit_breaker;
      return;
    }
    policy.circuit_breaker = { ...(policy.circuit_breaker ?? {}), failure_threshold: n, reset_timeout_seconds: policy.circuit_breaker?.reset_timeout_seconds ?? 60 };
  }
});

const failureOnError = computed({
  get: () => props.definition.failure_policy?.on_error ?? "",
  set: (value: string | null) => {
    const policy = ensureFailurePolicy();
    policy.on_error = value || undefined;
  }
});

</script>

<style scoped>
.team-editor-workspace {
  display: grid;
  grid-template-columns: minmax(0, 1.15fr) minmax(280px, 0.85fr);
  gap: 16px;
  padding: 0 16px 16px;
  align-items: start;
}

.team-editor-workspace__right {
  position: sticky;
  top: 8px;
}

@media (max-width: 960px) {
  .team-editor-workspace {
    grid-template-columns: 1fr;
  }
  .team-editor-workspace__right {
    position: static;
  }
}

.min-width-0 {
  min-width: 0;
}

.team-control :deep(.q-field__control) {
  border-radius: 16px;
  background: var(--glass-elevated);
}

.member-editor-list {
  display: grid;
  gap: 8px;
}

.member-editor {
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.topology-preview {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.topology-node {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  border: 1px solid color-mix(in srgb, var(--color-accent) 18%, transparent);
  border-radius: 999px;
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface));
  color: var(--color-accent);
  font-size: 12px;
  font-weight: 700;
}

.definition-json {
  white-space: pre-wrap;
  max-height: 240px;
  overflow: auto;
  padding: 12px;
  border-radius: 14px;
  border: 1px solid var(--glass-border);
  background: var(--glass-elevated);
  color: var(--color-text-primary);
}

.team-editor.is-dark .team-control :deep(.q-field__control) {
  background: color-mix(in srgb, var(--glass-surface) 88%, transparent);
}
</style>
