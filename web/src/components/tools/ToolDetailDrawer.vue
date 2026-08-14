<template>
  <teleport to="body">
    <transition name="drawer-backdrop">
      <div v-if="open" class="tool-detail-backdrop" @click="$emit('close')" />
    </transition>
  </teleport>
  <q-drawer
    :model-value="open"
    :width="640"
    :breakpoint="0"
    side="right"
    overlay
    bordered
    class="tool-detail-drawer"
    @update:model-value="onDrawerUpdate"
  >
    <template v-if="tool">
      <div class="tool-detail-drawer__head">
        <div class="tool-detail-drawer__head-info">
          <div class="tool-detail-drawer__title">{{ tool.display_name }}</div>
          <div class="tool-detail-drawer__subtitle app-registry-muted-caption">{{ tool.key }}</div>
        </div>
        <div class="tool-detail-drawer__head-actions row q-gutter-xs">
          <q-btn flat dense round icon="edit" class="app-registry-icon-btn" @click="onEdit">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="delete"
            color="negative"
            class="app-registry-icon-btn"
            :disable="tool.readonly"
            @click="$emit('remove-tool', tool)"
          >
            <q-tooltip>{{ tool.readonly ? '内置/只读工具不可删除' : '删除' }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="close" class="app-registry-icon-btn" @click="$emit('close')">
            <q-tooltip>关闭</q-tooltip>
          </q-btn>
        </div>
      </div>
      <q-separator />

      <div class="tool-detail-drawer__meta">
        <q-chip dense :color="tool.enabled ? 'positive' : 'grey'" text-color="white">
          {{ tool.enabled ? '已启用' : '已停用' }}
        </q-chip>
        <q-chip dense :color="riskQuasarColor(tool.risk_level)" text-color="white">
          {{ riskLabel(tool.risk_level) }}
        </q-chip>
        <q-chip v-if="tool.requires_confirmation" dense color="warning" text-color="dark">
          {{ policyChip.requires_confirmation.label }}
          <q-tooltip>{{ policyChip.requires_confirmation.tooltip }}</q-tooltip>
        </q-chip>
        <q-chip v-if="tool.supports_streaming" dense outline>
          {{ policyChip.supports_streaming.label }}
          <q-tooltip>{{ policyChip.supports_streaming.tooltip }}</q-tooltip>
        </q-chip>
        <q-chip v-if="tool.supports_concurrency" dense outline>
          {{ policyChip.supports_concurrency.label }}
          <q-tooltip>{{ policyChip.supports_concurrency.tooltip }}</q-tooltip>
        </q-chip>
        <q-chip dense outline :color="runtimeStatusColor(tool.runtime_status)">{{
          runtimeStatusLabel(tool.runtime_status)
        }}</q-chip>
      </div>

      <q-tabs
        :model-value="activeTab"
        dense
        class="app-registry-detail-tabs"
        active-color="primary"
        indicator-color="primary"
        align="left"
        no-caps
        @update:model-value="$emit('update:activeTab', $event)"
      >
        <q-tab name="overview" label="概览" />
        <q-tab name="config" label="配置" />
        <q-tab name="agents" :label="agentTabLabel" />
      </q-tabs>

      <div class="tool-detail-drawer__body">
        <q-tab-panels
          :model-value="activeTab"
          class="app-registry-detail-panels bg-transparent"
          @update:model-value="onActiveTabChange"
        >
          <q-tab-panel name="overview" class="q-pa-none">
            <q-banner rounded class="app-registry-detail-banner q-mb-md">
              {{ tool.description || '暂无描述' }}
            </q-banner>

            <div class="tool-detail-metrics q-mb-md">
              <div class="tool-detail-metrics__card">
                <div class="tool-detail-metrics__value">{{ tool.invoke_count }}</div>
                <div class="tool-detail-metrics__label">总调用</div>
              </div>
              <div class="tool-detail-metrics__card">
                <div class="tool-detail-metrics__value" :class="successRateClass">{{ successRate }}</div>
                <div class="tool-detail-metrics__label">成功率</div>
              </div>
              <div class="tool-detail-metrics__card">
                <div class="tool-detail-metrics__value">{{ tool.success_count }}</div>
                <div class="tool-detail-metrics__label">成功</div>
              </div>
              <div class="tool-detail-metrics__card">
                <div class="tool-detail-metrics__value" :class="tool.failure_count > 0 ? 'text-negative' : ''">
                  {{ tool.failure_count }}
                </div>
                <div class="tool-detail-metrics__label">失败</div>
              </div>
              <div class="tool-detail-metrics__card">
                <div class="tool-detail-metrics__value" :class="`text-${toolArgsFirstPassRateColor(tool)}`">
                  {{ formatToolArgsFirstPassRate(tool) }}
                </div>
                <div class="tool-detail-metrics__label">
                  一次合法率
                  <q-tooltip>
                    参数一次合法率 = 1 − (修复成功 {{ tool.repaired_count }} + 不可修复 {{ tool.invalid_count }}) / 调用
                    {{ tool.invoke_count }}（90 天窗口）
                  </q-tooltip>
                </div>
              </div>
            </div>

            <div class="row q-col-gutter-sm text-body2 q-mb-md">
              <div class="col-6"><span class="text-weight-medium">Key：</span>{{ tool.key }}</div>
              <div class="col-6"><span class="text-weight-medium">分类：</span>{{ tool.category }}</div>
              <div class="col-6"><span class="text-weight-medium">来源：</span>{{ tool.source }}</div>
              <div class="col-6"><span class="text-weight-medium">风险：</span>{{ riskLabel(tool.risk_level) }}</div>
            </div>

            <q-expansion-item dense-toggle default-opened label="参数 Schema" class="q-mb-md">
              <div class="q-pt-sm">
                <p class="app-registry-muted-caption">LLM 调用此工具时可传的参数（JSON Schema）。</p>
                <tool-json-block :text="prettyParamsSchema" />
                <q-expansion-item v-if="hasResultSchema" dense-toggle label="返回结构" class="q-mt-sm">
                  <tool-json-block class="q-mt-sm" :text="prettyResultSchema" />
                </q-expansion-item>
              </div>
            </q-expansion-item>

            <q-card v-if="tool.source !== 'mcp'" flat bordered class="q-mb-md tool-detail-test-card">
              <q-card-section class="q-pb-sm">
                <div class="text-subtitle2">在线测试</div>
                <div class="text-caption text-grey-7">单次调用验证（默认超时 30s，写入 tool_test 调用记录）。</div>
              </q-card-section>
              <q-card-section class="q-pt-none q-gutter-sm">
                <q-input
                  :model-value="testArgsJson"
                  type="textarea"
                  dense
                  outlined
                  autogrow
                  label="参数 JSON"
                  @update:model-value="$emit('update:testArgsJson', String($event ?? '{}'))"
                />
                <div class="row q-gutter-sm items-center">
                  <q-input
                    :model-value="testTimeoutSec"
                    class="col-4"
                    dense
                    outlined
                    type="number"
                    label="超时 (秒)"
                    :min="1"
                    :max="120"
                    @update:model-value="$emit('update:testTimeoutSec', Number($event) || 30)"
                  />
                  <q-btn
                    no-caps
                    unelevated
                    class="app-registry-primary-btn"
                    label="运行测试"
                    icon="play_arrow"
                    :loading="testRunning"
                    @click="$emit('run-test')"
                  />
                </div>
                <q-banner
                  v-if="testResult"
                  rounded
                  :class="testResult.status === 'success' ? 'bg-green-1' : 'bg-red-1'"
                >
                  <template #avatar>
                    <q-icon
                      :name="testResult.status === 'success' ? 'check_circle' : 'error'"
                      :color="testResult.status === 'success' ? 'positive' : 'negative'"
                    />
                  </template>
                  <div class="text-body2">{{ testResult.status }} · {{ testResult.duration_ms }}ms</div>
                  <div v-if="testResult.error_message" class="text-caption q-mt-xs">{{ testResult.error_message }}</div>
                  <tool-json-block
                    v-else-if="testResult.result_preview"
                    class="q-mt-xs"
                    :text="testResult.result_preview"
                  />
                </q-banner>
              </q-card-section>
            </q-card>

            <q-expansion-item dense-toggle label="最近调用记录" class="q-mb-md">
              <div class="q-pt-sm">
                <div v-if="runsLoading" class="text-center q-pa-md">
                  <q-spinner-dots size="28px" />
                </div>
                <template v-else>
                  <q-list v-if="recentRuns.length" separator dense class="rounded-borders">
                    <q-item v-for="r in recentRuns.slice(0, 5)" :key="r.id" class="app-registry-list-item">
                      <q-item-section avatar>
                        <q-icon :name="runStatusIcon(r.status)" :color="runStatusColor(r.status)" size="sm" />
                      </q-item-section>
                      <q-item-section>
                        <q-item-label>{{ r.agent_display_name || r.agent_id }}</q-item-label>
                        <q-item-label caption>{{ r.started_at }} · {{ r.duration_ms }}ms</q-item-label>
                      </q-item-section>
                      <q-item-section side>
                        <q-badge :color="runStatusColor(r.status)" :label="r.status" />
                      </q-item-section>
                    </q-item>
                  </q-list>
                  <div v-else class="text-caption q-pa-sm">暂无调用记录</div>
                  <q-btn
                    flat
                    no-caps
                    class="app-registry-accent-btn q-mt-sm"
                    icon="history"
                    label="查看全部调用记录"
                    :to="{ name: 'tool-runs', query: { tool_key: tool.key } }"
                  />
                </template>
              </div>
            </q-expansion-item>
          </q-tab-panel>

          <q-tab-panel name="config" class="q-pa-none">
            <tool-detail-config-panel
              :tool="tool"
              :config-json="configJson"
              :saving="configSaving"
              @update:config-json="$emit('update:configJson', $event)"
              @update:config-schema-json="$emit('update:configSchemaJson', $event)"
              @save="$emit('save-config')"
            />
          </q-tab-panel>

          <q-tab-panel name="agents" class="q-pa-none">
            <div v-if="agentBindingLoading" class="text-center q-pa-md">
              <q-spinner-dots size="28px" />
              <div class="text-caption q-mt-sm">{{ t('toolsPage.agentBinding.loading') }}</div>
            </div>
            <q-banner v-else-if="agentBindingError" rounded class="settings-warning-banner q-mb-md">
              {{ t('toolsPage.agentBinding.loadFailed') }}
              <template #action>
                <q-btn
                  flat
                  dense
                  no-caps
                  color="primary"
                  :label="t('toolsPage.agentBinding.retry')"
                  @click="$emit('retry-agent-bindings')"
                />
              </template>
            </q-banner>
            <template v-else>
              <q-banner v-if="agentBindingSummary" rounded dense class="settings-info-banner q-mb-md">
                <strong>{{ t('toolsPage.agentBinding.summaryTitle') }}</strong
                >{{ bindingSummaryLine(agentBindingSummary, t) }}{{ t('toolsPage.agentBinding.configHintPre')
                }}<router-link :to="{ name: 'agents' }" class="text-primary">{{
                  t('toolsPage.agentBinding.configHintLink')
                }}</router-link
                >{{ t('toolsPage.agentBinding.configHintPost') }}
              </q-banner>

              <AppRegistryTable
                v-if="priorityBindingRows.length"
                :shell="false"
                :data-shell="true"
                :rows="priorityBindingRows"
                :columns="agentBindingColumns"
                row-key="agent_id"
                hide-pagination
                :pagination="{ rowsPerPage: 0 }"
              >
                <template #body-cell-agent_name="slotProps">
                  <q-td :props="slotProps">
                    <div>{{ slotProps.row.agent_name }}</div>
                    <div class="text-caption text-grey-7">{{ slotProps.row.agent_key }}</div>
                  </q-td>
                </template>
                <template #body-cell-profile="slotProps">
                  <q-td :props="slotProps">
                    <span class="text-caption">{{ toolProfileLabel(slotProps.row.profile) }}</span>
                  </q-td>
                </template>
                <template #body-cell-state="slotProps">
                  <q-td :props="slotProps">
                    <q-icon
                      :name="slotProps.row.effective_state === 'allowed' ? 'check_circle' : 'cancel'"
                      :color="slotProps.row.effective_state === 'allowed' ? 'positive' : 'grey'"
                      size="sm"
                    />
                    {{
                      slotProps.row.effective_state === 'allowed'
                        ? t('toolsPage.agentBinding.stateAllowed')
                        : t('toolsPage.agentBinding.stateDenied')
                    }}
                    <q-badge
                      v-if="slotProps.row.override_mode"
                      outline
                      color="primary"
                      class="q-ml-xs"
                      :label="overrideModeLabel(slotProps.row.override_mode)"
                    />
                  </q-td>
                </template>
                <template #body-cell-reason="slotProps">
                  <q-td :props="slotProps">
                    <span class="text-caption">{{ bindingReasonLabel(slotProps.row.reason) }}</span>
                  </q-td>
                </template>
              </AppRegistryTable>

              <q-expansion-item
                v-if="deniedBindingRows.length"
                dense
                dense-toggle
                class="q-mb-md"
                :label="t('toolsPage.agentBinding.groupDenied', { count: deniedBindingRows.length })"
                header-class="text-grey-7"
              >
                <AppRegistryTable
                  :shell="false"
                  :data-shell="true"
                  :rows="deniedBindingRows"
                  :columns="agentBindingColumns"
                  row-key="agent_id"
                  hide-pagination
                  :pagination="{ rowsPerPage: 0 }"
                >
                  <template #body-cell-agent_name="slotProps">
                    <q-td :props="slotProps">
                      <div>{{ slotProps.row.agent_name }}</div>
                      <div class="text-caption text-grey-7">{{ slotProps.row.agent_key }}</div>
                    </q-td>
                  </template>
                  <template #body-cell-profile="slotProps">
                    <q-td :props="slotProps">
                      <span class="text-caption">{{ toolProfileLabel(slotProps.row.profile) }}</span>
                    </q-td>
                  </template>
                  <template #body-cell-state="slotProps">
                    <q-td :props="slotProps">
                      <q-icon name="cancel" color="grey" size="sm" />
                      {{ t('toolsPage.agentBinding.stateDenied') }}
                      <q-badge
                        v-if="slotProps.row.override_mode"
                        outline
                        color="primary"
                        class="q-ml-xs"
                        :label="overrideModeLabel(slotProps.row.override_mode)"
                      />
                    </q-td>
                  </template>
                  <template #body-cell-reason="slotProps">
                    <q-td :props="slotProps">
                      <span class="text-caption">{{ bindingReasonLabel(slotProps.row.reason) }}</span>
                    </q-td>
                  </template>
                </AppRegistryTable>
              </q-expansion-item>

              <q-expansion-item
                v-if="toolsDisabledBindingRows.length"
                dense
                dense-toggle
                class="q-mb-md"
                :label="t('toolsPage.agentBinding.groupToolsDisabled', { count: toolsDisabledBindingRows.length })"
                header-class="text-grey-7"
              >
                <AppRegistryTable
                  :shell="false"
                  :data-shell="true"
                  :rows="toolsDisabledBindingRows"
                  :columns="agentBindingColumns"
                  row-key="agent_id"
                  hide-pagination
                  :pagination="{ rowsPerPage: 0 }"
                >
                  <template #body-cell-agent_name="slotProps">
                    <q-td :props="slotProps">
                      <div>{{ slotProps.row.agent_name }}</div>
                      <div class="text-caption text-grey-7">{{ slotProps.row.agent_key }}</div>
                    </q-td>
                  </template>
                  <template #body-cell-profile="slotProps">
                    <q-td :props="slotProps">
                      <span class="text-caption">{{ toolProfileLabel(slotProps.row.profile) }}</span>
                    </q-td>
                  </template>
                  <template #body-cell-state="slotProps">
                    <q-td :props="slotProps">
                      <q-icon name="cancel" color="grey" size="sm" />
                      {{ t('toolsPage.agentBinding.stateDenied') }}
                      <q-badge
                        v-if="slotProps.row.override_mode"
                        outline
                        color="primary"
                        class="q-ml-xs"
                        :label="overrideModeLabel(slotProps.row.override_mode)"
                      />
                    </q-td>
                  </template>
                  <template #body-cell-reason="slotProps">
                    <q-td :props="slotProps">
                      <span class="text-caption">{{ bindingReasonLabel(slotProps.row.reason) }}</span>
                    </q-td>
                  </template>
                </AppRegistryTable>
              </q-expansion-item>

              <div v-if="agentBindingSummary && !agentBindingSummary.rows.length" class="text-caption q-pa-sm">
                {{ t('toolsPage.agentBinding.emptyAgents') }}
              </div>

              <div class="text-subtitle2 q-mb-xs">{{ t('toolsPage.agentBinding.overridesTitle') }}</div>
            </template>

            <div v-if="overridesLoading && !agentBindingLoading" class="text-center q-pa-md">
              <q-spinner-dots size="28px" />
            </div>
            <template v-else-if="!agentBindingLoading">
              <q-list v-if="overrides.length" separator dense class="rounded-borders">
                <q-item v-for="o in overrides" :key="o.id" class="app-registry-list-item">
                  <q-item-section avatar>
                    <q-icon :name="overrideModeIcon(o.mode)" :color="overrideModeColor(o.mode)" size="sm" />
                  </q-item-section>
                  <q-item-section>
                    <q-item-label>{{ agentNameById(o.agent_id) }}</q-item-label>
                    <q-item-label caption>
                      {{ t('toolsPage.agentBinding.overrideRow', { mode: modeLabel(o.mode) })
                      }}<span v-if="o.requires_confirmation">
                        · {{ t('toolsPage.agentBinding.requiresConfirmation') }}</span
                      >
                    </q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row q-gutter-xs">
                      <q-btn
                        flat
                        dense
                        round
                        icon="edit"
                        size="sm"
                        class="app-registry-icon-btn"
                        @click="$emit('edit-override', o)"
                      >
                        <q-tooltip>{{ t('toolsPage.agentBinding.editOverride') }}</q-tooltip>
                      </q-btn>
                      <q-btn
                        flat
                        dense
                        round
                        icon="delete"
                        size="sm"
                        class="app-registry-icon-btn"
                        @click="$emit('delete-override', o)"
                      >
                        <q-tooltip>{{ t('toolsPage.agentBinding.deleteOverride') }}</q-tooltip>
                      </q-btn>
                    </div>
                  </q-item-section>
                </q-item>
              </q-list>
              <div v-else class="text-caption q-pa-sm">{{ t('toolsPage.agentBinding.noOverrides') }}</div>
              <q-btn
                flat
                no-caps
                icon="add"
                :label="t('toolsPage.overrideMode.add')"
                class="app-registry-accent-btn q-mt-sm"
                @click="$emit('edit-override', null)"
              />
            </template>
          </q-tab-panel>
        </q-tab-panels>
      </div>
    </template>

    <tool-override-editor-dialog
      :open="overrideEditorOpen"
      :form="overrideForm"
      :editing="Boolean(editingOverride)"
      :saving="overrideSaving"
      :agent-options="agentOptions"
      :agents-loading="agentsLoading"
      @update:open="$emit('update:overrideEditorOpen', $event)"
      @update:form="$emit('update:overrideForm', $event)"
      @save="$emit('save-override')"
    />
  </q-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { TOOL_POLICY_CHIP_COPY } from '../../features/tools/toolEditorCopy';
import { bindingSummaryLine } from '../../features/tools/toolAgentBindingSummary';
import type { ToolAgentBindingSummary } from '../../features/tools/toolAgentBindingSummary';
import type { Tool, ToolAgentOverride, ToolInvocation, ToolTestResult } from '../../features/tools/types';
import type { ToolOverrideForm } from '../../stores/tools/toolDetail';
import {
  agentBindingColumns as buildAgentBindingColumns,
  riskLabel,
  riskQuasarColor,
  runtimeStatusLabel,
  runtimeStatusColor,
  prettyJSON,
  toolProfileLabel,
  bindingReasonLabel,
  overrideModeLabel,
  formatToolArgsFirstPassRate,
  toolArgsFirstPassRateColor,
} from './toolUi';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import ToolDetailConfigPanel from './ToolDetailConfigPanel.vue';
import ToolJsonBlock from './ToolJsonBlock.vue';
import ToolOverrideEditorDialog from './ToolOverrideEditorDialog.vue';

const props = defineProps<{
  open: boolean;
  tool: Tool | null;
  activeTab: string;
  overrides: ToolAgentOverride[];
  overridesLoading: boolean;
  recentRuns: ToolInvocation[];
  runsLoading: boolean;
  testArgsJson: string;
  testTimeoutSec: number;
  testRunning: boolean;
  testResult: ToolTestResult | null;
  configJson: string;
  configSaving: boolean;
  overrideEditorOpen: boolean;
  editingOverride: ToolAgentOverride | null;
  overrideSaving: boolean;
  overrideForm: ToolOverrideForm;
  agentBindingSummary: ToolAgentBindingSummary | null;
  agentBindingLoading: boolean;
  agentBindingError: boolean;
  agentOptions: { label: string; value: string }[];
  agentsLoading: boolean;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  'update:activeTab': [value: string];
  'update:testArgsJson': [value: string];
  'update:testTimeoutSec': [value: number];
  'run-test': [];
  'update:configJson': [value: string];
  'update:configSchemaJson': [value: string];
  'save-config': [];
  'edit-override': [row: ToolAgentOverride | null];
  'delete-override': [row: ToolAgentOverride];
  'update:overrideEditorOpen': [value: boolean];
  'update:overrideForm': [value: ToolOverrideForm];
  'save-override': [];
  'retry-agent-bindings': [];
  'edit-tool': [tool: Tool];
  'remove-tool': [tool: Tool];
  close: [];
}>();

const policyChip = TOOL_POLICY_CHIP_COPY;

const { t } = useI18n();

const modeOptions = computed(() => [
  { label: t('toolsPage.overrideMode.shortInherit'), value: 'inherit' },
  { label: t('toolsPage.overrideMode.shortAllow'), value: 'allow' },
  { label: t('toolsPage.overrideMode.shortDeny'), value: 'deny' },
]);

function modeLabel(mode: string): string {
  const m = modeOptions.value.find((o) => o.value === mode);
  return m ? m.label : mode;
}

// 覆盖行的启停由 mode 决定（enabled 列运行时不参与判定），图标按 mode 语义推导。
function overrideModeIcon(mode: string): string {
  if (mode === 'allow') return 'check_circle';
  if (mode === 'deny') return 'cancel';
  return 'adjust';
}

function overrideModeColor(mode: string): string {
  if (mode === 'allow') return 'positive';
  if (mode === 'deny') return 'negative';
  return 'grey';
}

function agentNameById(id: string): string {
  // 覆盖列表先于覆盖编辑器渲染（agentOptions 尚未加载），
  // 优先用聚合摘要里的 agent 名称，避免直接展示 UUID。
  const bindingRow = props.agentBindingSummary?.rows.find((r) => r.agent_id === id);
  if (bindingRow) return bindingRow.agent_name;
  const found = props.agentOptions.find((o) => o.value === id);
  return found ? found.label : id;
}

function runStatusIcon(status: string): string {
  if (status === 'success') return 'check_circle';
  if (status === 'error') return 'error';
  if (status === 'blocked') return 'block';
  return 'help';
}

function runStatusColor(status: string): string {
  if (status === 'success') return 'positive';
  if (status === 'error') return 'negative';
  if (status === 'blocked') return 'warning';
  return 'grey';
}

const successRate = computed(() => {
  const t = props.tool;
  if (!t || t.success_count + t.failure_count === 0) return '—';
  return ((t.success_count / (t.success_count + t.failure_count)) * 100).toFixed(1) + '%';
});

const successRateClass = computed(() => {
  const t = props.tool;
  if (!t || t.success_count + t.failure_count === 0) return '';
  const rate = t.success_count / (t.success_count + t.failure_count);
  if (rate >= 0.95) return 'text-positive';
  if (rate >= 0.8) return 'text-warning';
  return 'text-negative';
});

const prettyParamsSchema = computed(() => prettyJSON(props.tool?.parameters_schema_json || '{}'));
const prettyResultSchema = computed(() => prettyJSON(props.tool?.result_schema_json || '{}'));
const hasResultSchema = computed(() => {
  const raw = props.tool?.result_schema_json;
  return raw && raw.trim() !== '{}';
});

const agentBindingColumns = buildAgentBindingColumns();

/** 高价值行优先：可用或有显式覆盖的 Agent 进主表，其余按口径折叠分组。 */
const priorityBindingRows = computed(() => {
  const rows = props.agentBindingSummary?.rows ?? [];
  return rows.filter((r) => r.effective_state === 'allowed' || Boolean(r.override_mode));
});

/** 策略拒绝（Agent 已启用工具调用但未生效）。带覆盖的拒绝行在主表展示，故组内数量可能少于摘要计数。 */
const deniedBindingRows = computed(() => {
  const rows = props.agentBindingSummary?.rows ?? [];
  return rows.filter((r) => r.tools_enabled && r.effective_state !== 'allowed' && !r.override_mode);
});

/** Agent 级关闭工具调用。带覆盖的行在主表展示，故组内数量可能少于摘要计数。 */
const toolsDisabledBindingRows = computed(() => {
  const rows = props.agentBindingSummary?.rows ?? [];
  return rows.filter((r) => !r.tools_enabled && !r.override_mode);
});

/** Tab 计数展示可用数/总数；数据未加载时只显示 Agent。 */
const agentTabLabel = computed(() => {
  const s = props.agentBindingSummary;
  if (!s || !s.total_agents) return 'Agent';
  return `Agent (${s.allowed}/${s.total_agents})`;
});

function onDrawerUpdate(val: boolean) {
  if (!val) emit('close');
}

function onActiveTabChange(val: string | number) {
  emit('update:activeTab', String(val));
}

function onEdit() {
  if (props.tool) {
    emit('edit-tool', props.tool);
  }
}
</script>
