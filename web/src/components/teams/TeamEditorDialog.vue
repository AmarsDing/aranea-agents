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

      <q-banner v-if="hasActiveRun" dense rounded class="bg-warning text-dark q-mx-md q-mt-sm">
        存在运行中的 TeamRun，编排定义只读；结束后可再编辑。
      </q-banner>
      <q-banner v-else-if="isSpiritTeam" dense rounded class="team-editor-spirit-banner q-mx-md q-mt-sm">
        <q-icon name="auto_awesome" size="16px" class="q-mr-xs" />
        此团队由任务编排自动生成；App Name 与 Team Key 已锁定，成员与参数调整将影响后续编排运行。
      </q-banner>
      <q-banner
        v-else-if="isCustomSource"
        dense
        rounded
        class="team-editor-custom-banner q-mx-md q-mt-sm"
        data-test="custom-source-banner"
      >
        <q-icon name="account_tree" size="16px" class="q-mr-xs" />
        拓扑已在 Graph 编辑器中自定义，可能与表单字段不一致；修改编排模式/成员后保存将按表单重建并覆盖自定义拓扑。
        <template #action>
          <q-btn flat no-caps dense label="重置为派生" data-test="reset-to-derived" @click="onResetToDerived" />
        </template>
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
                  :readonly="isSpiritTeam"
                  :hint="isSpiritTeam ? '编排生成的团队不可修改' : '小写字母、数字、连字符'"
                />
                <q-input
                  v-model.trim="form.app_name"
                  class="team-control"
                  dense
                  outlined
                  label="App Name"
                  :readonly="isSpiritTeam"
                  :hint="isSpiritTeam ? '编排生成的团队不可修改' : '留空则使用 Team Key'"
                />
                <q-field class="team-control team-status-field" dense outlined label="状态" stack-label>
                  <template #control>
                    <div class="team-status-field__body row items-center no-wrap">
                      <q-badge rounded :color="statusMeta.color" :label="statusMeta.label" />
                      <span class="team-status-field__hint">自动流转</span>
                      <q-btn
                        v-if="canRetryStatus"
                        flat
                        dense
                        no-caps
                        rounded
                        size="sm"
                        color="warning"
                        icon="replay"
                        label="重试"
                        @click="$emit('retry')"
                      >
                        <q-tooltip>重置为待执行，可重新发起运行</q-tooltip>
                      </q-btn>
                    </div>
                  </template>
                </q-field>
                <q-select
                  v-model="form.taxonomy_industry_id"
                  class="team-control"
                  dense
                  outlined
                  emit-value
                  map-options
                  clearable
                  label="所属部门"
                  :options="departmentOptions"
                />
                <q-input
                  v-model="definition.description"
                  class="team-control app-grid-span-full"
                  dense
                  outlined
                  autogrow
                  type="textarea"
                  label="Team 说明"
                  :hint="t('teamsPage.descriptionHint')"
                />
              </div>
            </section>

            <section class="team-editor-section">
              <header class="team-editor-section__head">
                <h3 class="team-editor-section__title">编排模式</h3>
                <p class="team-editor-section__hint">选择模式即按模板生成图谱；成员角色由模式与顺序自动派生。</p>
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
                  :hint="modeHint"
                  :options="modeOptions"
                >
                  <template #option="{ itemProps, opt }">
                    <q-item v-bind="itemProps">
                      <q-item-section>
                        <q-item-label>{{ opt.label }}</q-item-label>
                        <q-item-label caption>{{ opt.description }}</q-item-label>
                      </q-item-section>
                    </q-item>
                  </template>
                </q-select>
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
                <q-field
                  v-if="definition.mode === 'parallel'"
                  class="team-control"
                  dense
                  outlined
                  readonly
                  stack-label
                  label="汇总 Agent（派生）"
                  hint="排序最后的启用成员；调整成员顺序可变更"
                >
                  <template #control>
                    <span class="team-derived-value">{{ synthesizerLabel }}</span>
                  </template>
                </q-field>
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
                <q-select
                  class="team-control app-grid-span-full"
                  dense
                  outlined
                  clearable
                  emit-value
                  map-options
                  label="关联 Graph（可选）"
                  :hint="
                    isLinkedExternal
                      ? '拓扑来自关联的独立图资产，表单中的模式/成员不再驱动执行拓扑'
                      : '选择独立图资产作为拓扑来源；清空则恢复表单派生'
                  "
                  :options="graphOptions"
                  :model-value="linkedGraphSelection"
                  data-test="linked-graph-select"
                  @update:model-value="onLinkedGraphPick"
                />
              </div>
            </section>

            <div class="team-editor-expansion">
              <q-expansion-item icon="settings" label="失败策略" caption="失败接管与重试配置（Graph 运行时）">
                <div class="team-editor-expansion__body">
                  <div class="app-form-field-grid app-form-field-grid--2col">
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
                  hint="意图识别锁定由哪个成员承接；留空则用排序后首位启用成员"
                  :options="intentAnchorOptions"
                />
                <q-toggle
                  v-model="checkpointEnabled"
                  class="team-control"
                  label="启用 Checkpoint"
                  data-test="checkpoint-toggle"
                />
              </div>
            </section>

            <div class="team-editor-expansion">
              <q-expansion-item icon="sync_alt" label="A2A 协议" caption="信封格式与载荷限制">
                <div class="team-editor-expansion__body">
                  <div class="app-form-field-grid app-form-field-grid--2col">
                    <q-toggle v-model="a2aEnabled" class="app-grid-span-full" label="启用 A2A 信封" />
                    <q-select
                      v-model="a2aEnvelopeVersion"
                      class="team-control"
                      dense
                      outlined
                      emit-value
                      map-options
                      label="Envelope Version"
                      hint="信封协议版本，当前支持 a2a.v1"
                      :options="a2aVersionOptions"
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
                <div>尚未添加成员，点击「添加成员」或选择模板快速填充。</div>
                <div v-if="agentOptions.length === 0" class="q-mt-xs">
                  {{ $t('teamsPage.editorNoAvailableAgents') }}
                </div>
              </div>
              <div v-else class="team-member-list">
                <div v-for="(member, index) in definition.members" :key="index" class="team-member-row">
                  <div class="team-member-row__toolbar">
                    <span class="team-member-row__index">{{ t('teamsPage.memberIndex', { n: index + 1 }) }}</span>
                    <q-space />
                    <q-toggle v-model="member.enabled" dense :label="t('teamsPage.memberEnabled')" />
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
                    <q-field
                      v-if="roleDerived"
                      class="team-control"
                      dense
                      outlined
                      readonly
                      stack-label
                      :label="t('teamsPage.memberRoleDerived')"
                      :hint="t('teamsPage.memberRoleDerivedHint')"
                    >
                      <template #control>
                        <span class="team-derived-value">{{ teamRoleLabel(member.role) }}</span>
                      </template>
                    </q-field>
                    <q-select
                      v-else
                      v-model="member.role"
                      class="team-control"
                      dense
                      outlined
                      emit-value
                      map-options
                      :label="t('teamsPage.memberRole')"
                      :options="roleOptionsForMode(definition.mode || 'sequential')"
                    />
                    <q-input
                      v-model="member.name"
                      class="team-control"
                      dense
                      outlined
                      :label="t('teamsPage.memberName')"
                    />
                    <q-input
                      v-model.number="member.sort_order"
                      class="team-control"
                      dense
                      outlined
                      type="number"
                      :label="t('teamsPage.memberSortOrder')"
                    />
                    <q-input
                      v-model="member.task_prompt"
                      class="team-control team-member-row__task"
                      dense
                      outlined
                      autogrow
                      type="textarea"
                      :label="t('teamsPage.memberTaskPrompt')"
                    />
                  </div>
                </div>
              </div>
            </section>

            <div class="team-editor-expansion team-editor-expansion--compact">
              <q-expansion-item icon="code" :label="t('teamsPage.definitionJsonPreview')" dense>
                <div class="team-editor-expansion__body">
                  <pre class="team-definition-json">{{ definitionJson }}</pre>
                </div>
              </q-expansion-item>
            </div>
          </div>

          <aside class="team-editor-workspace__aside">
            <TeamCompilePreview
              :is-dark="isDark"
              :compiled="compileResult"
              :definition="definition"
              :loading="compileLoading"
              :error="compileError"
              :issues="compileIssues"
            />
          </aside>
        </div>
      </div>

      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat rounded no-caps :label="t('common.cancel')" />
        <q-btn
          class="team-dialog-save"
          rounded
          unelevated
          no-caps
          :label="editingId ? t('teamsPage.save') : t('teamsPage.createTeam')"
          :loading="saving"
          :disable="!canSave || hasActiveRun"
          data-test="team-save"
          @click="onSaveClick"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
const definition = defineModel<TeamDefinition>('definition', { required: true });
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import type { TeamDefinition } from '../../features/teams/types';
import { useTeamCompilePreview } from '../../features/teams/useTeamCompilePreview';
import TeamCompilePreview from './TeamCompilePreview.vue';
import {
  derivedRoleModes,
  definitionGraphSource,
  definitionTopologyOverwriteKey,
  failureDefaultOptions,
  failureOnErrorOptions,
  modeOptions,
  parallelFailOptions,
  roleOptionsForMode,
  teamRoleLabel,
  teamStatusMap,
  teamTemplateOptions,
  type TeamTemplateKey,
} from './teamUtils';

const $q = useQuasar();
const { t } = useI18n();

const form = defineModel<{
  team_key: string;
  display_name: string;
  status: string;
  app_name: string;
  taxonomy_industry_id: string;
  spirit_session_id: string;
}>('form', { required: true });
const props = withDefaults(
  defineProps<{
    modelValue: boolean;
    selectedTemplateKey?: TeamTemplateKey | null;
    editingId: string;
    definitionJson?: string;
    agentOptions: Array<{ label: string; value: string }>;
    departmentOptions: Array<{ label: string; value: string }>; // 部门选项（公司 / 部门）
    /** 「关联 Graph」选择器选项（仅独立图资产，父级已排除 team-owned） */
    graphOptions?: Array<{ label: string; value: string }>;
    /** 打开编辑器时的覆盖确认基线（definitionTopologyOverwriteKey）；custom 团队指纹漂移即触发覆盖确认 */
    overwriteBaselineKey?: string;
    saving: boolean;
    canSave: boolean;
    isDark: boolean;
    hasActiveRun?: boolean;
  }>(),
  {
    definitionJson: '{}',
    selectedTemplateKey: null,
    hasActiveRun: false,
    graphOptions: () => [],
    overwriteBaselineKey: '',
  },
);

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  'update:selectedTemplateKey': [value: TeamTemplateKey | null];
  addMember: [];
  removeMember: [index: number];
  applyTemplate: [template: TeamTemplateKey];
  save: [];
  retry: [];
  resetToDerived: [];
}>();

// ── M53 Phase 11 F2：拓扑来源（preset/custom/linked_external）──
const graphSource = computed(() => definitionGraphSource(definition.value));
const isCustomSource = computed(() => graphSource.value === 'custom');
const isLinkedExternal = computed(() => graphSource.value === 'linked_external');
/** custom 团队拓扑/checkpoint 指纹相对打开时漂移 → 保存将按表单重建，需覆盖确认（D.4）。 */
const overwriteDirty = computed(() => definitionTopologyOverwriteKey(definition.value) !== props.overwriteBaselineKey);

/** 关联 Graph 选择器仅管理 external 关联；preset/custom 的 owned 资产 id 不在此展示。 */
const linkedGraphSelection = computed(() =>
  isLinkedExternal.value ? String(definition.value.linked_graph_id ?? '') : '',
);
function onLinkedGraphPick(value: string | null) {
  const id = String(value ?? '').trim();
  if (id) {
    definition.value.linked_graph_id = id;
    definition.value.source = 'linked_external';
    return;
  }
  delete definition.value.linked_graph_id;
  if (isLinkedExternal.value) {
    definition.value.source = 'preset';
  }
}

/** checkpoint 缺省镜像后端 parseRuntimeOptions 默认 true；开关显式写布尔。 */
const checkpointEnabled = computed({
  get: () => definition.value.enable_checkpoint ?? true,
  set: (value: boolean) => {
    definition.value.enable_checkpoint = value;
  },
});

function onResetToDerived() {
  $q.dialog({
    title: t('teamsPage.resetToDerivedTitle'),
    message: t('teamsPage.resetToDerivedMessage'),
    cancel: true,
    persistent: true,
  }).onOk(() => emit('resetToDerived'));
}

function onSaveClick() {
  if (isCustomSource.value && overwriteDirty.value) {
    $q.dialog({
      title: t('teamsPage.overwriteCustomTitle'),
      message: t('teamsPage.overwriteCustomMessage'),
      cancel: true,
      persistent: true,
    }).onOk(() => emit('save'));
    return;
  }
  emit('save');
}

// ── Spirit (orchestration-generated) team guards ──
const isSpiritTeam = computed(() => String(form.value.spirit_session_id || '').trim() !== '');

// ── Status display (readonly; transitions via lifecycle / RetryTeam RPC) ──
const statusMeta = computed(
  () => teamStatusMap[form.value.status] ?? { label: form.value.status || '—', color: 'grey' },
);
const canRetryStatus = computed(() => ['failed', 'cancelled'].includes(form.value.status));

// ── Compile preview (delegated to composable) ──
const { compileResult, compileLoading, compileError, compileIssues } = useTeamCompilePreview(
  () => props.editingId,
  () => props.definitionJson,
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
    title: t('teamsPage.applyTemplate'),
    message: t('teamsPage.applyTemplateMsg'),
    cancel: true,
    persistent: true,
  }).onOk(() => {
    emit('applyTemplate', key);
  });
}

// ── Mode guidance: help users pick an orchestration mode ──
const modeHintKeys: Record<string, string> = {
  sequential: 'teamsPage.modeHintSequential',
  parallel: 'teamsPage.modeHintParallel',
  coordinator: 'teamsPage.modeHintCoordinator',
  critic_loop: 'teamsPage.modeHintCriticLoop',
  adaptive: 'teamsPage.modeHintAdaptive',
};
const modeHint = computed(() => {
  const key = modeHintKeys[definition.value.mode || 'sequential'];
  return key ? t(key) : '';
});

// ADR-08 A3：派生模式下角色由 mode + 成员顺序自动派生（拓扑 watcher 回写），编辑器只读展示。
const roleDerived = computed(() => derivedRoleModes.has(String(definition.value.mode || 'sequential').toLowerCase()));

const a2aFormatOptions = [
  { label: 'Markdown + JSON', value: 'markdown_json' },
  { label: 'Plain', value: 'plain' },
];
// C-1：信封版本枚举化，防自由文本 typo；后端支持新版本时在此扩展。
const a2aVersionOptions = [{ label: 'a2a.v1', value: 'a2a.v1' }];
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
  opacity: 72%;
}

/* 运行中只读：仅禁用内部表单交互元素，保留折叠面板展开、滚动查看能力 */
.team-editor--readonly :where(input, select, textarea, [contenteditable]),
.team-editor--readonly :where(.q-btn:not(.q-expansion-item__container > .q-expansion-item__header *)),
.team-editor--readonly :where(.q-field, .q-select) {
  pointer-events: none;
}

.team-editor-spirit-banner {
  background: color-mix(in srgb, var(--color-accent) 14%, transparent);
  color: var(--color-text-primary);
  border: 1px solid color-mix(in srgb, var(--color-accent) 32%, transparent);
}

.team-editor-custom-banner {
  background: color-mix(in srgb, var(--color-warning) 12%, transparent);
  color: var(--color-text-primary);
  border: 1px solid color-mix(in srgb, var(--color-warning) 32%, transparent);
}

.team-status-field__body {
  gap: 8px;
  min-height: 24px;
}

.team-status-field__hint {
  font-size: 12px;
  color: var(--color-text-secondary);
}
</style>
