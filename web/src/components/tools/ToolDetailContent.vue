<template>
  <div v-if="tool" class="tool-detail-content column q-gutter-md">
    <q-banner rounded class="app-registry-detail-banner">{{ tool.description || "暂无描述" }}</q-banner>

    <div class="tool-detail-meta">
      <q-chip dense :color="tool.enabled ? 'positive' : 'grey'" text-color="white">
        {{ tool.enabled ? "已启用" : "已停用" }}
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
      <q-chip dense outline>{{ runtimeStatusLabel(tool.runtime_status) }}</q-chip>
    </div>

    <q-tabs v-model="activeTab" dense class="app-registry-detail-tabs" active-color="primary" indicator-color="primary" align="left" no-caps>
      <q-tab name="overview" label="概览" />
      <q-tab name="params" label="参数" />
      <q-tab name="config" label="配置" />
      <q-tab name="agents" :label="'Agent (' + overrides.length + ')'" />
      <q-tab name="runs" label="调用记录" />
    </q-tabs>

    <q-tab-panels v-model="activeTab" class="app-registry-detail-panels">
      <q-tab-panel name="overview" class="q-pa-none">
        <div class="row q-col-gutter-sm text-body2 q-mb-md">
          <div class="col-6"><span class="text-weight-medium">Key：</span>{{ tool.key }}</div>
          <div class="col-6"><span class="text-weight-medium">分类：</span>{{ tool.category }}</div>
          <div class="col-6"><span class="text-weight-medium">来源：</span>{{ tool.source }}</div>
          <div class="col-6"><span class="text-weight-medium">风险：</span>{{ riskLabel(tool.risk_level) }}</div>
          <div class="col-6"><span class="text-weight-medium">调用次数：</span>{{ tool.invoke_count }}</div>
          <div class="col-6">
            <span class="text-weight-medium">成功 / 失败：</span>{{ tool.success_count }} / {{ tool.failure_count }}
          </div>
        </div>
        <q-card v-if="tool.source !== 'mcp'" flat bordered class="q-mt-sm">
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
            <q-banner v-if="testResult" rounded :class="testResult.status === 'success' ? 'bg-green-1' : 'bg-red-1'">
              <template #avatar>
                <q-icon
                  :name="testResult.status === 'success' ? 'check_circle' : 'error'"
                  :color="testResult.status === 'success' ? 'positive' : 'negative'"
                />
              </template>
              <div class="text-body2">{{ testResult.status }} · {{ testResult.duration_ms }}ms</div>
              <div v-if="testResult.error_message" class="text-caption q-mt-xs">{{ testResult.error_message }}</div>
              <tool-json-block v-else-if="testResult.result_preview" class="q-mt-xs" :text="testResult.result_preview" />
            </q-banner>
          </q-card-section>
        </q-card>
      </q-tab-panel>

      <q-tab-panel name="params" class="q-pa-none">
        <tool-detail-params-panel :tool="tool" />
      </q-tab-panel>

      <q-tab-panel name="config" class="q-pa-none">
        <tool-detail-config-panel
          :tool="tool"
          :config-json="configJson"
          :saving="configSaving"
          @update:config-json="$emit('update:configJson', $event)"
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
            <strong>生效摘要：</strong>{{ bindingSummaryLine(agentBindingSummary) }}。
            Profile / allow / deny 在
            <router-link :to="{ name: 'agents' }" class="text-primary">Agent 列表</router-link>
            → 能力 Tab 配置。
          </q-banner>

          <q-expansion-item
            v-if="agentBindingSummary?.rows.length"
            dense-toggle
            expand-separator
            label="各 Agent 生效明细"
            class="q-mb-md"
          >
            <q-list dense separator class="rounded-borders">
              <q-item v-for="row in agentBindingSummary.rows" :key="row.agent_id">
                <q-item-section avatar>
                  <q-icon
                    :name="row.effective_state === 'allowed' ? 'check_circle' : 'cancel'"
                    :color="row.effective_state === 'allowed' ? 'positive' : 'grey'"
                    size="sm"
                  />
                </q-item-section>
                <q-item-section>
                  <q-item-label>{{ row.agent_name }}</q-item-label>
                  <q-item-label caption>{{ row.effective_state }} · {{ row.reason }}</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </q-expansion-item>

          <div class="text-subtitle2 q-mb-xs">显式覆盖（tool_agent_overrides）</div>
        </template>

        <div v-if="overridesLoading && !agentBindingLoading" class="text-center q-pa-md">
          <q-spinner-dots size="28px" />
        </div>
        <template v-else-if="!agentBindingLoading">
          <q-list v-if="overrides.length" separator dense class="rounded-borders">
            <q-item v-for="o in overrides" :key="o.id" class="app-registry-list-item">
              <q-item-section avatar>
                <q-icon :name="o.enabled ? 'check_circle' : 'cancel'" :color="o.enabled ? 'positive' : 'negative'" size="sm" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ o.agent_id }}</q-item-label>
                <q-item-label caption>
                  模式：{{ modeLabel(o.mode) }}<span v-if="o.requires_confirmation"> · 需确认</span>
                </q-item-label>
              </q-item-section>
              <q-item-section side>
                <div class="row q-gutter-xs">
                  <q-btn flat dense round icon="edit" size="sm" class="app-registry-icon-btn" @click="$emit('edit-override', o)">
                    <q-tooltip>编辑覆盖</q-tooltip>
                  </q-btn>
                  <q-btn flat dense round icon="delete" size="sm" class="app-registry-icon-btn" @click="$emit('delete-override', o)">
                    <q-tooltip>删除覆盖</q-tooltip>
                  </q-btn>
                </div>
              </q-item-section>
            </q-item>
          </q-list>
          <div v-else class="text-caption q-pa-sm">暂无 Agent 覆盖配置</div>
          <q-btn flat no-caps icon="add" label="添加覆盖" class="app-registry-accent-btn q-mt-sm" @click="$emit('edit-override', null)" />
        </template>
      </q-tab-panel>

      <q-tab-panel name="runs" class="q-pa-none">
        <div v-if="runsLoading" class="text-center q-pa-md">
          <q-spinner-dots size="28px" />
        </div>
        <template v-else>
          <q-list v-if="recentRuns.length" separator dense class="rounded-borders">
            <q-item v-for="r in recentRuns" :key="r.id" class="app-registry-list-item">
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
            v-close-popup
          />
        </template>
      </q-tab-panel>
    </q-tab-panels>

    <tool-override-editor-dialog
      :open="overrideEditorOpen"
      :form="overrideForm"
      :editing="Boolean(editingOverride)"
      :saving="overrideSaving"
      @update:open="$emit('update:overrideEditorOpen', $event)"
      @update:form="$emit('update:overrideForm', $event)"
      @save="$emit('save-override')"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import type { Tool, ToolAgentOverride, ToolInvocation } from "../../features/tools/types";
import type { ToolAgentBindingSummary } from "../../features/tools/toolAgentBindingSummary";
import { bindingSummaryLine } from "../../features/tools/toolAgentBindingSummary";
import { TOOL_POLICY_CHIP_COPY } from "../../features/tools/toolEditorCopy";
import type { ToolOverrideForm } from "../../features/tools/useToolDetailPanel";
import type { ToolTestResult } from "../../features/tools/types";
import ToolDetailConfigPanel from "./ToolDetailConfigPanel.vue";
import ToolDetailParamsPanel from "./ToolDetailParamsPanel.vue";
import ToolJsonBlock from "./ToolJsonBlock.vue";
import ToolOverrideEditorDialog from "./ToolOverrideEditorDialog.vue";
import { riskLabel, runtimeStatusLabel } from "./toolUi";

defineProps<{
  tool: Tool | null;
  overrides: ToolAgentOverride[];
  overridesLoading: boolean;
  recentRuns: ToolInvocation[];
  runsLoading: boolean;
  testArgsJson: string;
  testTimeoutSec: number;
  testRunning: boolean;
  testResult: ToolTestResult | null;
  overrideEditorOpen: boolean;
  editingOverride: ToolAgentOverride | null;
  overrideSaving: boolean;
  overrideForm: ToolOverrideForm;
  configJson: string;
  configSaving: boolean;
  agentBindingSummary: ToolAgentBindingSummary | null;
  agentBindingLoading: boolean;
}>();

const policyChip = TOOL_POLICY_CHIP_COPY;

defineEmits<{
  "update:testArgsJson": [value: string];
  "update:testTimeoutSec": [value: number];
  "run-test": [];
  "edit-override": [row: ToolAgentOverride | null];
  "delete-override": [row: ToolAgentOverride];
  "update:overrideEditorOpen": [value: boolean];
  "update:overrideForm": [value: ToolOverrideForm];
  "save-override": [];
  "update:configJson": [value: string];
  "save-config": [];
}>();

const activeTab = ref("overview");

const modeOptions = [
  { label: "继承 (inherit)", value: "inherit" },
  { label: "允许 (allow)", value: "allow" },
  { label: "拒绝 (deny)", value: "deny" }
];

function modeLabel(mode: string): string {
  const m = modeOptions.find((o) => o.value === mode);
  return m ? m.label : mode;
}

function runStatusIcon(status: string): string {
  if (status === "success") return "check_circle";
  if (status === "error") return "error";
  if (status === "blocked") return "block";
  return "help";
}

function runStatusColor(status: string): string {
  if (status === "success") return "positive";
  if (status === "error") return "negative";
  if (status === "blocked") return "warning";
  return "grey";
}
</script>
