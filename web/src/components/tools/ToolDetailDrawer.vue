<template>
  <teleport to=".q-layout">
    <transition name="drawer-backdrop">
      <div v-if="open" class="tool-detail-backdrop" @click="$emit('close')" />
    </transition>
  </teleport>
  <q-drawer
    :model-value="open"
    :width="640"
    :breakpoint="1024"
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
            @click="$emit('remove-tool', tool)"
          >
            <q-tooltip>删除</q-tooltip>
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
        <q-tab name="agents" :label="'Agent (' + overrides.length + ')'" />
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
              <div class="text-caption q-mt-sm">正在汇总各 Agent 生效状态…</div>
            </div>
            <template v-else>
              <q-banner v-if="agentBindingSummary" rounded dense class="settings-info-banner q-mb-md">
                <strong>生效摘要：</strong>{{ bindingSummaryLine(agentBindingSummary) }}。 Profile / allow / deny 在
                <router-link :to="{ name: 'agents' }" class="text-primary">Agent 列表</router-link>
                → 能力 Tab 配置。
              </q-banner>

              <AppRegistryTable
                v-if="agentBindingSummary?.rows.length"
                :shell="false"
                :data-shell="true"
                :rows="agentBindingSummary.rows"
                :columns="agentBindingColumns"
                row-key="agent_id"
                hide-pagination
                :pagination="{ rowsPerPage: 0 }"
              >
                <template #body-cell-state="props">
                  <q-td :props="props">
                    <q-icon
                      :name="props.row.effective_state === 'allowed' ? 'check_circle' : 'cancel'"
                      :color="props.row.effective_state === 'allowed' ? 'positive' : 'grey'"
                      size="sm"
                    />
                    {{ props.row.effective_state }}
                  </q-td>
                </template>
                <template #body-cell-reason="props">
                  <q-td :props="props">
                    <span class="text-caption">{{ props.row.reason }}</span>
                  </q-td>
                </template>
              </AppRegistryTable>

              <div class="text-subtitle2 q-mb-xs">显式覆盖（tool_agent_overrides）</div>
            </template>

            <div v-if="overridesLoading && !agentBindingLoading" class="text-center q-pa-md">
              <q-spinner-dots size="28px" />
            </div>
            <template v-else-if="!agentBindingLoading">
              <q-list v-if="overrides.length" separator dense class="rounded-borders">
                <q-item v-for="o in overrides" :key="o.id" class="app-registry-list-item">
                  <q-item-section avatar>
                    <q-icon
                      :name="o.enabled ? 'check_circle' : 'cancel'"
                      :color="o.enabled ? 'positive' : 'negative'"
                      size="sm"
                    />
                  </q-item-section>
                  <q-item-section>
                    <q-item-label>{{ agentNameById(o.agent_id) }}</q-item-label>
                    <q-item-label caption>
                      模式：{{ modeLabel(o.mode) }}<span v-if="o.requires_confirmation"> · 需确认</span>
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
                        <q-tooltip>编辑覆盖</q-tooltip>
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
                        <q-tooltip>删除覆盖</q-tooltip>
                      </q-btn>
                    </div>
                  </q-item-section>
                </q-item>
              </q-list>
              <div v-else class="text-caption q-pa-sm">暂无 Agent 覆盖配置</div>
              <q-btn
                flat
                no-caps
                icon="add"
                label="添加覆盖"
                class="app-registry-accent-btn q-mt-sm"
                @click="$emit('edit-override', null)"
              />
            </template>
          </q-tab-panel>
        </q-tab-panels>
      </div>
    </template>

    <q-inner-loading :showing="loading" />

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
import { TOOL_POLICY_CHIP_COPY } from '../../features/tools/toolEditorCopy';
import { bindingSummaryLine } from '../../features/tools/toolAgentBindingSummary';
import type { ToolAgentBindingRow, ToolAgentBindingSummary } from '../../features/tools/toolAgentBindingSummary';
import type { Tool, ToolAgentOverride, ToolInvocation, ToolTestResult } from '../../features/tools/types';
import type { ToolOverrideForm } from '../../stores/tools/toolDetail';
import { AGENT_BINDING_COLUMNS, riskLabel, riskQuasarColor, runtimeStatusLabel, runtimeStatusColor, prettyJSON } from './toolUi';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import ToolDetailConfigPanel from './ToolDetailConfigPanel.vue';
import ToolJsonBlock from './ToolJsonBlock.vue';
import ToolOverrideEditorDialog from './ToolOverrideEditorDialog.vue';

const props = defineProps<{
  open: boolean;
  tool: Tool | null;
  activeTab: string;
  loading: boolean;
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
  'edit-tool': [tool: Tool];
  'remove-tool': [tool: Tool];
  close: [];
}>();

const policyChip = TOOL_POLICY_CHIP_COPY;

const modeOptions = [
  { label: '继承 (inherit)', value: 'inherit' },
  { label: '允许 (allow)', value: 'allow' },
  { label: '拒绝 (deny)', value: 'deny' },
];

function modeLabel(mode: string): string {
  const m = modeOptions.find((o) => o.value === mode);
  return m ? m.label : mode;
}

function agentNameById(id: string): string {
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

const agentBindingColumns = AGENT_BINDING_COLUMNS;

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
