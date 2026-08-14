<template>
  <tool-glass-panel v-if="!loading && rows.length === 0">
    <q-card-section class="column items-center text-center q-pa-xl">
      <q-avatar size="72px" color="primary" text-color="white" icon="history" />
      <div class="text-h6 q-mt-md">{{ $t('toolsPage.runsPage.emptyTitle') }}</div>
      <div class="text-body2 muted-caption q-mt-sm">{{ $t('toolsPage.runsPage.emptySubtitle') }}</div>
    </q-card-section>
  </tool-glass-panel>

  <tool-glass-panel v-else>
    <AppRegistryTable
      :shell="false"
      table-class="tool-runs-data-table"
      row-key="id"
      :rows="rows"
      :columns="columns"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-tool="props">
        <q-td :props="props">
          <AppRegistryHoverTip :text="toolRunHoverText(props.row)" :empty-label="$t('toolsPage.runsPage.hoverEmpty')">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">
                {{ props.row.tool_display_name || props.row.tool_key }}
              </div>
              <div class="app-registry-cell-sub ellipsis">{{ props.row.tool_key }}</div>
            </div>
          </AppRegistryHoverTip>
        </q-td>
      </template>

      <template #body-cell-agent="props">
        <q-td :props="props">
          <div class="app-registry-cell-primary ellipsis">{{ invocationAgentLine(props.row) }}</div>
          <div class="app-registry-cell-sub ellipsis">{{ props.row.agent_id }}</div>
        </q-td>
      </template>

      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge rounded :color="toolInvocationStatusColor(props.row.status)">{{
            toolInvocationStatusLabel(props.row.status)
          }}</q-badge>
        </q-td>
      </template>

      <template #body-cell-session_id="props">
        <q-td :props="props">
          <span class="app-registry-cell-sub ellipsis" :title="props.row.session_id">{{
            props.row.session_id || '—'
          }}</span>
        </q-td>
      </template>

      <template #body-cell-time="props">
        <q-td :props="props">
          <div>{{ formatInvocationWhen(props.row.started_at) }}</div>
          <div class="text-caption muted-caption">{{ formatInvocationDuration(props.row.duration_ms) }}</div>
        </q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="primary"
            icon="visibility"
            :aria-label="$t('toolsPage.runsPage.viewDetail')"
            @click="$emit('viewDetail', props.row)"
          >
            <q-tooltip>{{ $t('toolsPage.runsPage.viewDetail') }}</q-tooltip>
          </q-btn>
        </q-td>
      </template>
    </AppRegistryTable>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import ToolGlassPanel from './ToolGlassPanel.vue';
import type { ToolInvocation } from '../../features/tools/types';
import {
  toolRunsTableColumns,
  clipPreview,
  formatInvocationDuration,
  formatInvocationWhen,
  invocationAgentLine,
  toolInvocationStatusColor,
  toolInvocationStatusLabel,
} from './toolUi';

defineProps<{
  rows: ToolInvocation[];
  loading: boolean;
}>();

defineEmits<{
  viewDetail: [row: ToolInvocation];
}>();

const { t } = useI18n();
const columns = computed(() => toolRunsTableColumns());

function toolRunHoverText(row: ToolInvocation) {
  const input = clipPreview(row.input_preview);
  const output = clipPreview(row.output_preview || row.error_message);
  const parts = [];
  if (input) parts.push(t('toolsPage.runsPage.hoverInput', { text: input }));
  if (output) parts.push(t('toolsPage.runsPage.hoverOutput', { text: output }));
  return parts.join('\n\n');
}
</script>
