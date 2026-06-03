<template>
  <div>
    <q-card flat bordered class="monitor-card">
      <q-card-section class="q-pb-none">
        <div class="text-h6 text-weight-bold">活动日志</div>
        <div class="text-caption text-grey-7">管理与配置变更审计记录</div>
      </q-card-section>

      <AppPageToolbar class="monitor-audit-toolbar">
        <q-select
          v-model="actionFilter"
          class="app-page-toolbar__field"
          dense
          outlined
          emit-value
          map-options
          clearable
          label="事件类型"
          :options="actionOptions"
        />
        <q-select
          v-model="resourceFilter"
          class="app-page-toolbar__field"
          dense
          outlined
          emit-value
          map-options
          clearable
          label="实体类型"
          :options="resourceOptions"
        />
        <q-input
          v-model="keyword"
          class="app-page-toolbar__search"
          dense
          outlined
          clearable
          debounce="200"
          label="搜索事件 / 资源 / 详情"
        >
          <template #prepend>
            <q-icon name="search" />
          </template>
        </q-input>
        <template #actions>
          <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
          <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('reload')" />
        </template>
      </AppPageToolbar>

      <div class="app-registry-table-shell">
        <AppRegistryTable
          :shell="false"
          :rows="pagedRows"
          :columns="AUDIT_TABLE_COLUMNS"
          row-key="id"
          :loading="loading"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-event="props">
            <q-td :props="props" class="cursor-pointer" @click="openDetail(props.row)">
              <q-chip dense square :color="eventColor(props.row.action)" text-color="white">
                {{ props.row.action }}.{{ props.row.resource }}
              </q-chip>
            </q-td>
          </template>
          <template #body-cell-resource="props">
            <q-td :props="props">
              <AppRegistryHoverTip :text="props.row.detail">
                <div class="min-width-0">
                  <div class="app-registry-cell-primary ellipsis">{{ props.row.resource }}</div>
                  <div class="app-registry-cell-sub ellipsis">{{ props.row.resource_id || '—' }}</div>
                </div>
              </AppRegistryHoverTip>
            </q-td>
          </template>
          <template #body-cell-actor="props">
            <q-td :props="props">
              <div class="app-registry-cell-primary ellipsis">{{ props.row.actor || 'system' }}</div>
              <div v-if="props.row.ip" class="app-registry-cell-sub">{{ props.row.ip }}</div>
            </q-td>
          </template>
          <template #body-cell-request="props">
            <q-td :props="props">
              <code class="monitor-code ellipsis">{{ props.row.request_id || '—' }}</code>
            </q-td>
          </template>
          <template #body-cell-created="props">
            <q-td :props="props">
              <span class="app-registry-cell-sub">{{ formatDate(props.row.created_at) }}</span>
            </q-td>
          </template>
        </AppRegistryTable>

        <AppRegistryPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-max="pageMax"
          :total="filteredRows.length"
          :loading="loading"
          label="条审计"
          :page-size-options="[12, 25, 50]"
        />
      </div>
    </q-card>

    <q-dialog v-model="detailOpen">
      <q-card class="app-dialog-card app-dialog-card--lg app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-start justify-between">
          <div>
            <div class="app-glass-dialog__title">Audit 详情</div>
            <div class="app-glass-dialog__subtitle">{{ selected?.action }}.{{ selected?.resource }}</div>
          </div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-glass-dialog__body">
            <q-list dense>
              <q-item v-if="selected?.actor">
                <q-item-section>操作者</q-item-section>
                <q-item-section side>{{ selected?.actor }}</q-item-section>
              </q-item>
              <q-item v-if="selected?.ip">
                <q-item-section>IP</q-item-section>
                <q-item-section side>{{ selected?.ip }}</q-item-section>
              </q-item>
              <q-item v-if="selected?.severity">
                <q-item-section>严重级别</q-item-section>
                <q-item-section side>
                  <q-badge :color="severityColor(selected!.severity)">{{ selected?.severity }}</q-badge>
                </q-item-section>
              </q-item>
            </q-list>
            <pre class="monitor-json app-code-block">{{ selectedJSON }}</pre>
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn flat no-caps label="复制 JSON" icon="content_copy" @click="copyJSON" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { copyToClipboard } from 'quasar';
import AppPageToolbar from '../layout/AppPageToolbar.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';

import type { AuditLog } from '../../features/monitor/types';
import { compactJSON, formatDate } from '../../features/monitor/utils';
import { AUDIT_TABLE_COLUMNS } from './monitorTableUi';

const props = defineProps<{
  rows: AuditLog[];
  loading: boolean;
}>();

const emit = defineEmits<{
  reload: [];
  notify: [payload: { message: string; type: 'positive' | 'negative' | 'warning' }];
}>();

const keyword = ref('');
const actionFilter = ref<string | null>(null);
const resourceFilter = ref<string | null>(null);
const selected = ref<AuditLog | null>(null);
const detailOpen = ref(false);
const page = ref(1);
const pageSize = ref(12);

const actionOptions = computed(() => {
  const actions = new Set(props.rows.map((r) => r.action).filter(Boolean));
  return [...actions].map((a) => ({ label: a, value: a }));
});

const resourceOptions = computed(() => {
  const resources = new Set(props.rows.map((r) => r.resource).filter(Boolean));
  return [...resources].map((r) => ({ label: r, value: r }));
});

const filteredRows = computed(() => {
  let result = props.rows;
  if (actionFilter.value) {
    result = result.filter((row) => row.action === actionFilter.value);
  }
  if (resourceFilter.value) {
    result = result.filter((row) => row.resource === resourceFilter.value);
  }
  const q = keyword.value.trim().toLowerCase();
  if (q) {
    result = result.filter((row) =>
      [row.action, row.resource, row.resource_id, row.request_id, row.detail, row.actor].some((value) =>
        String(value || '')
          .toLowerCase()
          .includes(q),
      ),
    );
  }
  return result;
});

const pageMax = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filteredRows.value.slice(start, start + pageSize.value);
});

watch([keyword, actionFilter, resourceFilter], () => {
  page.value = 1;
});

const selectedJSON = computed(() => compactJSON(selected.value ?? {}));

function resetFilters() {
  keyword.value = '';
  actionFilter.value = null;
  resourceFilter.value = null;
  page.value = 1;
}

function openDetail(row: AuditLog) {
  selected.value = row;
  detailOpen.value = true;
}

function eventColor(action: string) {
  if (action.includes('delete')) return 'negative';
  if (action.includes('create')) return 'positive';
  if (action.includes('toggle')) return 'orange';
  if (action.includes('credentials')) return 'purple';
  if (action.includes('update')) return 'primary';
  return 'grey';
}

function severityColor(severity: string) {
  if (severity === 'critical' || severity === 'high') return 'negative';
  if (severity === 'warning' || severity === 'medium') return 'orange';
  return 'positive';
}

async function copyJSON() {
  await copyToClipboard(selectedJSON.value);
  emit('notify', { message: '已复制', type: 'positive' });
}
</script>

<style scoped>
.monitor-audit-toolbar {
  padding: var(--space-2) var(--space-4) 0;
  border-bottom: none;
}
</style>
