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
        <q-table
          flat
          dense
          :rows="rows"
          :columns="columns"
          row-key="tool_key"
          :pagination="{ rowsPerPage: 0 }"
          hide-pagination
          class="agent-tool-overrides-table"
        >
          <template #body-cell-effective_state="props">
            <q-td :props="props">
              <q-badge :color="props.row.enabled ? 'positive' : 'grey'" :label="props.row.effective_state" />
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
            <q-td :props="props" class="text-right">
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
                @click="$emit('remove', props.row)"
              >
                <q-tooltip>删除覆盖</q-tooltip>
              </q-btn>
            </q-td>
          </template>
        </q-table>
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
  </q-card>
</template>

<script setup lang="ts">
import AgentToolOverrideEditorDialog from "./AgentToolOverrideEditorDialog.vue";
import type {
  AgentToolOverrideForm,
  AgentToolOverrideRow
} from "../../features/agents/useAgentToolOverrides";

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
}>();

defineEmits<{
  refresh: [];
  edit: [row: AgentToolOverrideRow];
  remove: [row: AgentToolOverrideRow];
  save: [];
  "update:editorOpen": [value: boolean];
  "update:form": [value: AgentToolOverrideForm];
}>();

const columns = [
  { name: "tool_key", label: "工具 Key", field: "tool_key", align: "left" as const },
  { name: "display_name", label: "名称", field: "display_name", align: "left" as const },
  { name: "effective_state", label: "生效", field: "effective_state", align: "left" as const },
  { name: "requires_confirmation", label: "确认", field: "effective_requires_confirmation", align: "left" as const },
  { name: "override", label: "覆盖", field: "override", align: "left" as const },
  { name: "actions", label: "", field: "actions", align: "right" as const }
];
</script>
