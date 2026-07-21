<!--
  Team 域展示组件：仅 props / emits（aranea-frontend-guide SKILL §1 红线 #1）。
  路径约定：SKILL §3.3 → `web/src/components/teams/`。
-->
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card :class="['team-editor-dialog app-dialog-card app-glass-dialog', { 'is-dark': isDark }]">
      <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">{{ editingId ? '编辑 Team' : '新增 Team' }}</div>
          <div class="app-glass-dialog__subtitle">配置成员角色与编排模式；右侧实时编译预览拓扑结构。</div>
        </div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>

      <q-banner
        v-if="hasActiveRun"
        dense
        rounded
        class="bg-warning text-dark q-mx-md q-mt-sm"
      >
        存在运行中的 TeamRun，编排定义只读；结束后可再编辑。
      </q-banner>

      <div class="app-glass-dialog__scroll" :class="{ 'team-editor--readonly': hasActiveRun }">
        <div class="team-editor-workspace">
          <div class="team-editor-workspace__main">
            <section class="team-editor-section app-dialog-section">
              <header class="team-editor-section__head">
                <h3 class="team-editor-section__title">快速开始</h3>
                <p class="team-editor-section__hint">选择模板可快速生成成员角色和编排参数。</p>
              </header>
              <q-select
                class="team-control app-field-md"
                dense
                outlined
                clearable
                emit-value
                map-options
                label="Team 模板"
                :model-value="selectedTemplateKey"
                :options="teamTemplateOptions"
                @update:model-value="onTemplatePick"
              />
            </section>

            <section class="team-editor-section">
              <header class="team-editor-section__head">
                <h3 class="team-editor-section__title">基础信息</h3>
              </header>
              <div class="app-form-field-grid app-form-field-grid--2col">
                <q-input v-model.trim="form.display_name" class="team-control" dense outlined label="Team 名称 *" />
                <q-input
                  v-model.trim="form.team_key"
                  class="team-control"
                  dense
                  outlined
                  label="Team Key *"
                  hint="小写字母、数字、连字符"
                />
                <q-input
                  v-model.trim="form.app_name"
                  class="team-control"
                  dense
                  outlined
                  label="App Name"
                  hint="留空则使用 Team Key"
                />
                <q-select
                  v-model="form.status"
                  class="team-control"
                  dense
                  outlined
                  emit-value
                  map-options
                  label="状态"
                  :options="statusOptions"
                />
                <q-select
                  v-model="form.taxonomy_industry_id"
                  class="team-control"
                  dense
                  outlined
                  emit-value
                  map-options
                  clearable
                  label="行业归属"
                  :options="industryOptions"
                />
              </div>
            </section>

            <section class="team-editor-section">
              <header class="team-editor-section__head">
                <h3 class="team-editor-section__title">编排模式</h3>
              </header>
              <div class="app-form-field-grid app-form-field-grid--2col">
                <q-select
                  v-model="definition.mode"
                  class="team-control"
                  dense
                  outlined
                  emit-value
                  map-options
                  label="编排模式"
                  :options="modeOptions"
                />
                <q-input
                  v-if="definition.mode === 'parallel'"
                  v-model.number="definition.max_concurrency"
                  class="team-control"
                  dense
                  outlined
                  type="number"
                  min="1"
                  label="并行批大小"
                  hint="每批同时执行的 Worker 数"
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
                  hint="0 = 默认 1 轮；轮数×成员数会拉长耗时"
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
                <q-input
                  v-model="definition.description"
                  class="team-control app-grid-span-full"
                  dense
                  outlined
                  autogrow
                  type="textarea"
                  label="Team 说明"
                />
              </div>
            </section>

            <div class="team-editor-expansion">
              <q-expansion-item icon="settings" label="运行时 / 失败策略" caption="执行引擎与失败接管配置">
                <div class="team-editor-expansion__body">
                  <q-banner v-if="nativeLocked" dense rounded class="team-editor-notice q-mb-sm">
                    Native 执行引擎仅平台管理员可选；当前将使用 Graph。
                  </q-banner>
                  <div class="app-form-field-grid app-form-field-grid--2col">
                    <q-select
                      v-model="runtimeEngine"
                      class="team-control"
                      dense
                      outlined
                      emit-value
                      map-options
                      label="执行引擎"
                      hint="Native 需 admin + ARANEA_TEAM_NATIVE=1"
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
                      label="并行失败策略"
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
                      hint="0 = 禁用"
                    />
                    <q-select
                      v-model="failureOnError"
                      class="team-control"
                      dense
                      outlined
                      emit-value
                      map-options
                      clearable
                      label="错误接管"
                      :options="failureOnErrorOptions"
                    />
                  </div>
                </div>
              </q-expansion-item>
            </div>

            <section class="team-editor-section">
              <header class="team-editor-section__head">
                <h3 class="team-editor-section__title">运行参数</h3>
              </header>
              <div class="app-form-field-grid app-form-field-grid--2col">
                <q-input
                  v-model.number="definition.timeout_seconds"
                  class="team-control"
                  dense
                  outlined
                  type="number"
                  min="0"
                  max="7200"
                  label="单次运行超时（秒）"
                  hint="0 = 仅遵循 HTTP/反代超时；长任务建议 ≥600"
                />
                <q-select
                  v-model="definition.intent_anchor_agent_id"
                  class="team-control"
                  dense
                  outlined
                  clearable
                  emit-value
                  map-options
                  label="Intent 锚定成员（可选）"
                  hint="留空则用排序后首位启用成员"
                  :options="intentAnchorOptions"
                />
              </div>
            </section>

            <div class="team-editor-expansion">
              <q-expansion-item icon="sync_alt" label="A2A 协议" caption="信封格式与载荷限制">
                <div class="team-editor-expansion__body">
                  <div class="app-form-field-grid app-form-field-grid--2col">
                    <q-toggle v-model="a2aEnabled" class="app-grid-span-full" label="启用 A2A 信封" />
                    <q-input
                      v-model="a2aEnvelopeVersion"
                      class="team-control"
                      dense
                      outlined
                      label="Envelope Version"
                    />
                    <q-select
                      v-model="a2aMessageFormat"
                      class="team-control"
                      dense
                      outlined
                      emit-value
                      map-options
                      label="消息格式"
                      :options="a2aFormatOptions"
                    />
                    <q-input
                      v-model.number="a2aMaxPayloadChars"
                      class="team-control"
                      dense
                      outlined
                      type="number"
                      min="500"
                      label="最大载荷字符"
                    />
                    <q-toggle v-model="a2aIncludeTrace" class="app-grid-span-full" label="包含追踪元数据" />
                  </div>
                </div>
              </q-expansion-item>
            </div>

            <section class="team-editor-section">
              <header class="team-editor-section__head row items-center justify-between no-wrap">
                <div>
                  <h3 class="team-editor-section__title">成员 Agent</h3>
                  <p class="team-editor-section__hint">{{ definition.members.length }} 个成员 · 按顺序执行或并行编排</p>
                </div>
                <q-btn
                  flat
                  rounded
                  no-caps
                  icon="add"
                  label="添加成员"
                  class="team-editor-add-member"
                  @click="$emit('addMember')"
                />
              </header>

              <div v-if="definition.members.length === 0" class="team-member-empty">
                尚未添加成员，点击「添加成员」或选择模板快速填充。
              </div>
              <div v-else class="team-member-list">
                <div v-for="(member, index) in definition.members" :key="index" class="team-member-row">
                  <div class="team-member-row__toolbar">
                    <span class="team-member-row__index">成员 {{ index + 1 }}</span>
                    <q-space />
                    <q-toggle v-model="member.enabled" dense label="启用" />
                    <q-btn flat dense round color="negative" icon="delete" @click="$emit('removeMember', index)" />
                  </div>
                  <div class="team-member-row__grid">
                    <q-select
                      v-model="member.agent_id"
                      class="team-control"
                      dense
                      outlined
                      emit-value
                      map-options
                      label="Agent"
                      :options="agentOptions"
                    />
                    <q-select
                      v-model="member.role"
                      class="team-control"
                      dense
                      outlined
                      emit-value
                      map-options
                      label="角色"
                      :options="roleOptionsForMode(definition.mode || 'sequential')"
                    />
                    <q-input v-model="member.name" class="team-control" dense outlined label="成员名称" />
                    <q-input
                      v-model.number="member.sort_order"
                      class="team-control"
                      dense
                      outlined
                      type="number"
                      label="顺序"
                    />
                    <q-input
                      v-model="member.task_prompt"
                      class="team-control team-member-row__task"
                      dense
                      outlined
                      autogrow
                      type="textarea"
                      label="职责 / 任务说明"
                    />
                  </div>
                </div>
              </div>
            </section>

            <div class="team-editor-expansion team-editor-expansion--compact">
              <q-expansion-item icon="code" label="definition_json 预览" dense>
                <div class="team-editor-expansion__body">
                  <pre class="team-definition-json">{{ definitionJSON }}</pre>
                </div>
              </q-expansion-item>
            </div>
          </div>

          <aside class="team-editor-workspace__aside">
            <TeamCompilePreview
              :is-dark="isDark"
              :compiled="compileResult"
              :loading="compileLoading"
              :error="compileError"
              :issues="compileIssues"
            />
          </aside>
        </div>
      </div>

      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat rounded no-caps label="取消" />
        <q-btn
          class="team-dialog-save"
          rounded
          unelevated
          no-caps
          label="保存"
          :loading="saving"
          :disable="!canSave || hasActiveRun"
          @click="$emit('save')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
const definition = defineModel<TeamDefinition>('definition', { required: true });
import { computed, watch } from 'vue';
import { useQuasar } from 'quasar';
import type { TeamDefinition } from '../../features/teams/types';
import { useTeamCompilePreview } from '../../features/teams/useTeamCompilePreview';
import TeamCompilePreview from './TeamCompilePreview.vue';
import {
  failureDefaultOptions,
  failureOnErrorOptions,
  modeOptions,
  parallelFailOptions,
  roleOptionsForMode,
  runtimeEngineOptions,
  statusOptions,
  teamRoleLabel,
  teamTemplateOptions,
  type TeamTemplateKey,
} from './teamUtils';

const $q = useQuasar();

const form = defineModel<{
  team_key: string;
  display_name: string;
  status: string;
  app_name: string;
  taxonomy_industry_id: string;
}>('form', { required: true });
const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    selectedTemplateKey?: TeamTemplateKey | null;
    editingId: string;
    definitionJSON?: string;
    agentOptions: Array<{ label: string; value: string }>;
    industryOptions: Array<{ label: string; value: string }>;
    saving: boolean;
    canSave: boolean;
    isDark: boolean;
    isPlatformAdmin?: boolean;
    hasActiveRun?: boolean;
  }>(),
  { definitionJSON: '{}', selectedTemplateKey: null, isPlatformAdmin: false, hasActiveRun: false },
);

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  'update:selectedTemplateKey': [value: TeamTemplateKey | null];
  addMember: [];
  removeMember: [index: number];
  applyTemplate: [template: TeamTemplateKey];
  save: [];
}>();

// ── Compile preview (delegated to composable) ──
const { compileResult, compileLoading, compileError, compileIssues } = useTeamCompilePreview(
  () => props.editingId,
  () => props.definitionJSON,
);

const intentAnchorOptions = computed(() =>
  definition.value.members
    .filter((m) => m.enabled !== false && String(m.agent_id || '').trim() !== '')
    .map((m) => ({
      label: [m.name, teamRoleLabel(m.role)].filter(Boolean).join(' · ') || String(m.agent_id).slice(0, 8),
      value: m.agent_id,
    })),
);

function onTemplatePick(key: TeamTemplateKey | null) {
  emit('update:selectedTemplateKey', key);
  if (!key) return;
  $q.dialog({
    title: '应用模板',
    message: '应用模板将覆盖当前配置，确定继续？',
    cancel: true,
    persistent: true,
  }).onOk(() => {
    emit('applyTemplate', key);
  });
}

const filteredRuntimeEngineOptions = computed(() =>
  props.isPlatformAdmin ? runtimeEngineOptions : runtimeEngineOptions.filter((o) => o.value !== 'native'),
);

const nativeLocked = computed(
  () => !props.isPlatformAdmin && String(definition.value.runtime_engine || 'graph').toLowerCase() === 'native',
);

watch(
  nativeLocked,
  (locked) => {
    if (locked) {
      definition.value.runtime_engine = 'graph';
      definition.value.team_graph_runtime = true;
    }
  },
  { immediate: true },
);

const a2aFormatOptions = [
  { label: 'Markdown + JSON', value: 'markdown_json' },
  { label: 'Plain', value: 'plain' },
];
const a2aEnabled = computed({
  get: () => definition.value.a2a?.enabled ?? true,
  set: (value: boolean) => {
    definition.value.a2a = { ...definition.value.a2a, enabled: value };
  },
});
const a2aEnvelopeVersion = computed({
  get: () => definition.value.a2a?.envelope_version || 'a2a.v1',
  set: (value: string) => {
    definition.value.a2a = { ...definition.value.a2a, envelope_version: value };
  },
});
const a2aMessageFormat = computed({
  get: () => definition.value.a2a?.message_format || 'markdown_json',
  set: (value: string) => {
    definition.value.a2a = { ...definition.value.a2a, message_format: value };
  },
});
const a2aMaxPayloadChars = computed({
  get: () => definition.value.a2a?.max_payload_chars || 6000,
  set: (value: number) => {
    definition.value.a2a = { ...definition.value.a2a, max_payload_chars: value };
  },
});
const a2aIncludeTrace = computed({
  get: () => definition.value.a2a?.include_trace ?? true,
  set: (value: boolean) => {
    definition.value.a2a = { ...definition.value.a2a, include_trace: value };
  },
});

const criticLoopMaxIterations = computed({
  get: () => definition.value.critic_loop?.max_iterations ?? 2,
  set: (value: number) => {
    const n = Number.isFinite(value) ? Math.min(32, Math.max(1, Math.floor(value))) : 2;
    const prev = definition.value.critic_loop ?? { max_iterations: 2, score_threshold: 0.8 };
    definition.value.critic_loop = { ...prev, max_iterations: n };
  },
});

const runtimeEngine = computed({
  get: () => (String(definition.value.runtime_engine || 'graph').toLowerCase() === 'native' ? 'native' : 'graph'),
  set: (value: 'native' | 'graph') => {
    definition.value.runtime_engine = value;
    definition.value.team_graph_runtime = value === 'graph';
  },
});

function ensureFailurePolicy() {
  if (!definition.value.failure_policy) {
    definition.value.failure_policy = { default: 'retry_then_block', parallel_fail: 'continue' };
  }
  return definition.value.failure_policy;
}

const failureDefault = computed({
  get: () => definition.value.failure_policy?.default ?? 'retry_then_block',
  set: (value: string | null) => {
    const policy = ensureFailurePolicy();
    policy.default = value || 'retry_then_block';
  },
});

const parallelFail = computed({
  get: () => definition.value.failure_policy?.parallel_fail ?? 'continue',
  set: (value: string | null) => {
    const policy = ensureFailurePolicy();
    policy.parallel_fail = value || 'continue';
  },
});

const failureRetryMax = computed({
  get: () => definition.value.failure_policy?.retry?.max_attempts ?? 3,
  set: (value: number) => {
    const policy = ensureFailurePolicy();
    policy.retry = { ...(policy.retry ?? {}), max_attempts: Math.max(0, Math.floor(Number(value) || 0)) };
  },
});

const circuitFailureThreshold = computed({
  get: () => definition.value.failure_policy?.circuit_breaker?.failure_threshold ?? 0,
  set: (value: number) => {
    const policy = ensureFailurePolicy();
    const n = Math.max(0, Math.floor(Number(value) || 0));
    if (n <= 0) {
      delete policy.circuit_breaker;
      return;
    }
    policy.circuit_breaker = {
      ...(policy.circuit_breaker ?? {}),
      failure_threshold: n,
      reset_timeout_seconds: policy.circuit_breaker?.reset_timeout_seconds ?? 60,
    };
  },
});

const failureOnError = computed({
  get: () => definition.value.failure_policy?.on_error ?? '',
  set: (value: string | null) => {
    const policy = ensureFailurePolicy();
    policy.on_error = value || undefined;
  },
});
</script>

<style scoped>
.team-editor--readonly {
  pointer-events: none;
  opacity: 0.72;
}
</style>
