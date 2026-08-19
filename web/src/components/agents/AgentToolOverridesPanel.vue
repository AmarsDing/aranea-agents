<template>
  <q-card flat bordered class="capability-card">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-subtitle2">{{ $t('toolsPage.agentTools.panelTitle') }}</div>
        <div class="text-caption text-grey-7">
          {{ $t('toolsPage.agentTools.panelSubtitle') }}
        </div>
      </div>
      <div class="row items-center q-gutter-sm">
        <q-select
          v-model="stateFilter"
          dense
          outlined
          emit-value
          map-options
          :options="stateFilterOptions"
          :label="$t('toolsPage.agentTools.filterState')"
          style="min-width: 120px"
        />
        <q-input
          v-model="search"
          dense
          outlined
          clearable
          :placeholder="$t('agentSettings.toolOverrideSearchPlaceholder')"
          style="min-width: 220px"
        />
        <q-btn flat dense no-caps icon="refresh" :label="$t('common.refresh')" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>
    <q-separator />
    <q-card-section>
      <div v-if="loading" class="text-center q-pa-md">
        <q-spinner-dots size="28px" />
      </div>
      <template v-else>
        <q-banner v-if="!toolsEnabled" rounded class="settings-warning-banner q-mb-sm">
          {{ $t('toolsPage.agentTools.masterOff') }}
        </q-banner>
        <AppRegistryTable
          :shell="false"
          :data-shell="true"
          table-class="agent-tool-overrides-table"
          :rows="pagedRows"
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
              <span class="app-registry-cell-sub ellipsis" :title="props.row.display_name">{{
                props.row.display_name || '—'
              }}</span>
            </q-td>
          </template>
          <template #body-cell-effective_state="props">
            <q-td :props="props">
              <q-badge
                :color="props.row.effective_state === 'allowed' ? 'positive' : 'grey'"
                :label="effectiveStateLabel(props.row.effective_state)"
              />
            </q-td>
          </template>
          <template #body-cell-requires_confirmation="props">
            <q-td :props="props">
              <q-icon v-if="props.row.effective_requires_confirmation" name="warning" color="warning" size="xs" />
              <span v-if="props.row.effective_requires_confirmation" class="q-ml-xs">{{
                $t('toolsPage.agentTools.requiresConfirmation')
              }}</span>
              <span v-else class="text-grey-6">—</span>
            </q-td>
          </template>
          <template #body-cell-override="props">
            <q-td :props="props">
              <span v-if="props.row.override">{{ modeLabel(props.row.override.mode) }}</span>
              <span v-else class="text-grey-6">{{ $t('toolsPage.agentTools.none') }}</span>
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div class="app-registry-cell-actions">
                <q-btn
                  flat
                  dense
                  round
                  icon="edit"
                  size="sm"
                  :aria-label="props.row.override ? $t('toolsPage.agentTools.editOverride') : $t('toolsPage.agentTools.addOverride')"
                  @click="$emit('edit', props.row)"
                >
                  <q-tooltip>{{ props.row.override ? $t('toolsPage.agentTools.editOverride') : $t('toolsPage.agentTools.addOverride') }}</q-tooltip>
                </q-btn>
                <q-btn
                  v-if="props.row.override"
                  flat
                  dense
                  round
                  icon="delete"
                  size="sm"
                  :aria-label="$t('toolsPage.agentTools.removeTitle')"
                  @click="$emit('request-remove', props.row)"
                >
                  <q-tooltip>{{ $t('toolsPage.agentTools.removeTitle') }}</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
        </AppRegistryTable>
        <AppRegistryPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-max="pageMax"
          :total="filteredRows.length"
          :loading="loading"
          :label="$t('toolsPage.list.pageUnit')"
        />
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
          <div class="text-h6">{{ $t('toolsPage.agentTools.removeTitle') }}</div>
          <div class="text-body2 text-grey-7 q-mt-sm">
            {{ $t('toolsPage.agentTools.removeMessage', { key: pendingRemoveRow?.tool_key }) }}
          </div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat rounded no-caps :label="$t('common.cancel')" @click="$emit('cancel-remove')" />
          <q-btn color="negative" rounded unelevated no-caps :label="$t('common.delete')" @click="$emit('confirm-remove')" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-card>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import AgentToolOverrideEditorDialog from './AgentToolOverrideEditorDialog.vue';

import type { AgentToolOverrideForm, AgentToolOverrideRow } from '../../features/agents/useAgentToolOverrides';
import { AGENT_TOOL_OVERRIDE_TABLE_COLUMNS } from './agentTableUi';

const panelProps = defineProps<{
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
  'request-remove': [row: AgentToolOverrideRow];
  'confirm-remove': [];
  'cancel-remove': [];
  save: [];
  'update:editorOpen': [value: boolean];
  'update:form': [value: AgentToolOverrideForm];
}>();

const { t } = useI18n();

const search = ref('');
const stateFilter = ref('');
const stateFilterOptions = computed(() => [
  { label: t('toolsPage.agentTools.stateAll'), value: '' },
  { label: t('toolsPage.agentTools.stateAllowed'), value: 'allowed' },
  { label: t('toolsPage.agentTools.stateDenied'), value: 'denied' },
]);

const filteredRows = computed(() => {
  const q = search.value.trim().toLowerCase();
  let list = panelProps.rows;
  if (stateFilter.value) {
    list = list.filter((r) => r.effective_state === stateFilter.value);
  }
  if (!q) return list;
  return list.filter(
    (r) => r.tool_key.toLowerCase().includes(q) || r.display_name.toLowerCase().includes(q),
  );
});

// 客户端分页：与全局 Tools 页（AppRegistryPagination）交互模式一致
const page = ref(1);
const pageSize = ref(20);
const pageMax = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filteredRows.value.slice(start, start + pageSize.value);
});

watch([search, stateFilter], () => {
  page.value = 1;
});
watch(pageMax, (max) => {
  if (page.value > max) page.value = max;
});
</script>
