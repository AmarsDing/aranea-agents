<template>
  <tool-glass-panel>
    <AppRegistryTable
      :shell="false"
      table-class="tool-runs-data-table"
      row-key="id"
      :rows="rows"
      :columns="columns"
      :loading="loading"
      :error="error"
      :empty-title="$t('toolsPage.auditsPage.emptyTitle')"
      :empty-description="$t('toolsPage.auditsPage.emptySubtitle')"
      empty-icon="policy"
      :empty-action-label="$t('shell.runs.emptyAction')"
      empty-action-icon="restart_alt"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
      @retry="$emit('retry')"
      @empty-action="$emit('reset')"
    >
      <template #body-cell-tool="props">
        <q-td :props="props">
          <AppRegistryHoverTip
            :text="clipPreview(props.row.result_summary)"
            :empty-label="$t('toolsPage.auditsPage.hoverEmpty')"
          >
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">{{ props.row.tool_key }}</div>
              <div class="app-registry-cell-sub ellipsis">{{ props.row.action }}</div>
            </div>
          </AppRegistryHoverTip>
        </q-td>
      </template>

      <template #body-cell-actor="props">
        <q-td :props="props">
          <div class="app-registry-cell-primary ellipsis">{{ props.row.agent_id || '—' }}</div>
          <div class="app-registry-cell-sub ellipsis">{{ props.row.user_id || props.row.session_id || '—' }}</div>
        </q-td>
      </template>

      <template #body-cell-status="props">
        <q-td :props="props">
          <AppStatusChip :status="props.row.status" />
        </q-td>
      </template>

      <template #body-cell-time="props">
        <q-td :props="props">
          <div>{{ formatInvocationWhen(props.row.created_at) }}</div>
          <div class="text-caption muted-caption ellipsis">{{ props.row.source }}</div>
        </q-td>
      </template>
    </AppRegistryTable>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppStatusChip from '../common/AppStatusChip.vue';
import ToolGlassPanel from './ToolGlassPanel.vue';
import type { ToolInvocationAudit } from '../../features/tools/types';
import { toolAuditsTableColumns, clipPreview, formatInvocationWhen } from './toolUi';

defineProps<{
  rows: ToolInvocationAudit[];
  loading: boolean;
  error?: string;
}>();

defineEmits<{
  retry: [];
  reset: [];
}>();

const columns = computed(() => toolAuditsTableColumns());
</script>
