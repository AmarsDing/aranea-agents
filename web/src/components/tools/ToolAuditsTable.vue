<template>
  <tool-glass-panel v-if="!loading && rows.length === 0">
    <q-card-section class="column items-center text-center q-pa-xl">
      <q-avatar size="72px" color="primary" text-color="white" icon="policy" />
      <div class="text-h6 q-mt-md">{{ $t('toolsPage.auditsPage.emptyTitle') }}</div>
      <div class="text-body2 muted-caption q-mt-sm">{{ $t('toolsPage.auditsPage.emptySubtitle') }}</div>
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
          <AppRegistryHoverTip :text="clipPreview(props.row.result_summary)" :empty-label="$t('toolsPage.auditsPage.hoverEmpty')">
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
          <q-badge rounded :color="toolInvocationStatusColor(props.row.status)">{{
            toolInvocationStatusLabel(props.row.status)
          }}</q-badge>
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
import ToolGlassPanel from './ToolGlassPanel.vue';
import type { ToolInvocationAudit } from '../../features/tools/types';
import {
  toolAuditsTableColumns,
  clipPreview,
  formatInvocationWhen,
  toolInvocationStatusColor,
  toolInvocationStatusLabel,
} from './toolUi';

defineProps<{
  rows: ToolInvocationAudit[];
  loading: boolean;
}>();

const columns = computed(() => toolAuditsTableColumns());
</script>
