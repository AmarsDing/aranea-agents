<template>
  <q-card flat bordered class="capability-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-subtitle2">工具覆盖</div>
        <div class="text-caption text-grey-7">
          按工具粒度覆盖启用、模式、需确认与配置 JSON（运行时与 Tools 详情页一致）。
        </div>
      </div>
      <q-btn flat dense no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
    </q-card-section>
    <q-separator />
    <q-card-section>
      <div v-if="loading" class="text-center q-pa-md">
        <q-spinner-dots size="28px" />
      </div>
      <template v-else>
        <q-banner v-if="!toolsEnabled" rounded class="settings-warning-banner q-mb-sm">
          Agent 工具总开关已关闭；以下矩阵为策略计算结果，运行时不注入工具。
        </q-banner>
        <AppRegistryTable
          :shell="false"
          :data-shell="true"
          table-class="agent-tool-overrides-table"
          :rows="rows"
          :columns="AGENT_TOOL_OVERRIDE_TABLE_COLUMNS"
          row-key="tool_key"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-tool_key="props">
            <q-td :props="props">
              <div class="app-registry-cell-primary ellipsis" :title="props.row.tool_key">{{ props.row.tool_key }}</div>
            </q-td>
          </template>
          <template #body-cell-display_name="props">
            <q-td :props="props">
              <span class="app-registry-cell-sub ellipsis" :title="props.row.display_name">{{ props.row.display_name || "—" }}</span>
            </q-td>
          </template>
          <template #body-cell-effective_state="props">
            <q-td :props="props">
              <q-badge :color="props.row.enabled ? 'positive' : 'grey'" :label="effectiveStateLabel(props.row.effective_state)" />
            </q-td>
          </template>
          <template #body-cell-requires_confirmation="props">
            <q-td :props="props">
              <q-icon v-if="props.row.effective_requires_confirmation" name="warning" color="warning" size="xs" />
              <span v-if="props.row.effective_requires_confirmation" class="q-ml-xs">需确认</span>
              <span v-else class="text-grey-6">—</span>
            </q-td>
          </template>
          <template #body-cell-override="props">
            <q-td :props="props">
              <span v-if="props.row.override">{{ modeLabel(props.row.override.mode) }}</span>
              <span v-else class="text-grey-6">无</span>
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div class="app-registry-cell-actions">
                <q-btn flat dense round icon="edit" size="sm" @click="$emit('edit', props.row)">
                  <q-tooltip>{{ props.row.override ? "编辑覆盖" : "添加覆盖" }}</q-tooltip>
                </q-btn>
                <q-btn
                  v-if="props.row.override"
                  flat
                  dense
                  round
                  icon="delete"
                  size="sm"
                  @click="$emit('request-remove', props.row)"
                >
                  <q-tooltip>删除覆盖</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
        </AppRegistryTable>
      </template>
    </q-card-section>

    <agent-tool-override-editor-dialog
      :open="editorOpen"
      :editing="Boolean(editingRow?.override)"
      :saving="saving"
      :row="editingRow"
      :form="form"
      :mode-options="modeOptions"
      @update:open="$emit('update:editorOpen', $event)"
      @update:form="$emit('update:form', $event)"
      @save="$emit('save')"
    />

    <q-dialog :model-value="confirmRemoveOpen" persistent @update:model-value="$emit('cancel-remove')">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section>
          <div class="text-h6">删除覆盖</div>
          <div class="text-body2 text-grey-7 q-mt-sm">确定删除 {{ pendingRemoveRow?.tool_key }} 的 Agent 覆盖？</div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat rounded no-caps label="取消" @click="$emit('cancel-remove')" />
          <q-btn color="negative" rounded unelevated no-caps label="删除" @click="$emit('confirm-remove')" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-card>
</template>

<script setup lang="ts">
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AgentToolOverrideEditorDialog from "./AgentToolOverrideEditorDialog.vue";

import type {
  AgentToolOverrideForm,
  AgentToolOverrideRow
} from "../../features/agents/useAgentToolOverrides";
import { AGENT_TOOL_OVERRIDE_TABLE_COLUMNS } from "./agentTableUi";

defineProps<{
  loading: boolean;
  saving: boolean;
  toolsEnabled: boolean;
  rows: AgentToolOverrideRow[];
  editorOpen: boolean;
  editingRow: AgentToolOverrideRow | null;
  form: AgentToolOverrideForm;
  modeOptions: { label: string; value: string }[];
  modeLabel: (mode: string) => string;
  effectiveStateLabel: (state: string) => string;
  confirmRemoveOpen: boolean;
  pendingRemoveRow: AgentToolOverrideRow | null;
}>();

defineEmits<{
  refresh: [];
  edit: [row: AgentToolOverrideRow];
  "request-remove": [row: AgentToolOverrideRow];
  "confirm-remove": [];
  "cancel-remove": [];
  save: [];
  "update:editorOpen": [value: boolean];
  "update:form": [value: AgentToolOverrideForm];
}>();
</script>
