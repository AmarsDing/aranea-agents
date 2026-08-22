<!--
  Pure presentation: all data via props, all actions via emits.
  No composable / Store access. 服务端分页/筛选：查询变更通过 load 事件上抛。
-->
<template>
  <div>
    <q-card flat bordered class="monitor-card">
      <q-card-section class="q-pb-none">
        <div class="text-h6 text-weight-bold">{{ t('monitorPage.audit.title') }}</div>
        <div class="text-caption text-grey-7">{{ t('monitorPage.audit.subtitle') }}</div>
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
          :label="t('monitorPage.audit.filterAction')"
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
          :label="t('monitorPage.audit.filterResource')"
          :options="resourceOptions"
        />
        <q-input
          v-model="keyword"
          class="app-page-toolbar__search"
          dense
          outlined
          clearable
          debounce="300"
          :label="t('monitorPage.audit.searchPlaceholder')"
        >
          <template #prepend>
            <q-icon name="search" />
          </template>
        </q-input>
        <q-toggle v-model="hideSystem" :label="t('monitorPage.audit.hideSystem')">
          <q-tooltip>{{ t('monitorPage.audit.hideSystemHint') }}</q-tooltip>
        </q-toggle>
        <template #actions>
          <q-btn flat rounded no-caps icon="restart_alt" :label="t('monitorPage.audit.reset')" @click="resetFilters" />
          <q-btn
            flat
            rounded
            no-caps
            icon="refresh"
            :label="t('monitorPage.audit.refresh')"
            :loading="loading"
            @click="emitLoad"
          />
          <q-btn
            flat
            rounded
            no-caps
            icon="delete_sweep"
            color="negative"
            :label="t('monitorPage.audit.clearAll')"
            :disable="total === 0"
            @click="confirmClear = true"
          />
        </template>
      </AppPageToolbar>

      <div class="app-registry-table-shell">
        <AppRegistryTable
          :shell="false"
          :rows="rows"
          :columns="columns"
          row-key="id"
          :loading="loading"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-event="slotProps">
            <q-td :props="slotProps" class="cursor-pointer" @click="openDetail(slotProps.row)">
              <span class="audit-event-cell">
                <span class="apm-status-dot" :class="`apm-status-dot--${eventTone(slotProps.row.action)}`" />
                <span class="audit-event-cell__action">{{ slotProps.row.action }}</span>
              </span>
            </q-td>
          </template>
          <template #body-cell-resource="slotProps">
            <q-td :props="slotProps">
              <div class="min-width-0">
                <div class="app-registry-cell-primary ellipsis">{{ slotProps.row.resource }}</div>
                <div class="app-registry-cell-sub ellipsis">{{ slotProps.row.resource_id || '—' }}</div>
              </div>
            </q-td>
          </template>
          <template #body-cell-actor="slotProps">
            <q-td :props="slotProps">
              <div class="app-registry-cell-primary ellipsis">{{ slotProps.row.actor || 'system' }}</div>
              <div v-if="slotProps.row.ip" class="app-registry-cell-sub">{{ slotProps.row.ip }}</div>
            </q-td>
          </template>
          <template #body-cell-request="slotProps">
            <q-td :props="slotProps">
              <code class="monitor-code ellipsis">{{ slotProps.row.request_id || '—' }}</code>
            </q-td>
          </template>
          <template #body-cell-detail="slotProps">
            <q-td :props="slotProps">
              <AppRegistryHoverTip v-if="slotProps.row.detail" :text="prettyDetail(slotProps.row.detail)">
                <div class="app-registry-cell-sub ellipsis">{{ auditDetailSummary(slotProps.row.detail) || '—' }}</div>
              </AppRegistryHoverTip>
              <span v-else class="app-registry-cell-sub">—</span>
            </q-td>
          </template>
          <template #body-cell-created="slotProps">
            <q-td :props="slotProps">
              <span class="app-registry-cell-sub">{{ formatDate(slotProps.row.created_at) }}</span>
            </q-td>
          </template>
        </AppRegistryTable>

        <AppRegistryPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-max="pageMax"
          :total="total"
          :loading="loading"
          :label="t('monitorPage.audit.paginationLabel')"
          :page-size-options="[12, 25, 50]"
        />
      </div>
    </q-card>

    <q-dialog v-model="confirmClear" persistent>
      <q-card class="app-dialog-card">
        <q-card-section class="row items-center">
          <q-icon name="warning" color="negative" size="md" class="q-mr-sm" />
          <span class="text-h6">{{ t('monitorPage.audit.clearTitle') }}</span>
        </q-card-section>
        <q-card-section>
          {{ t('monitorPage.audit.clearConfirm', { total }) }}
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps :label="t('monitorPage.audit.cancel')" />
          <q-btn
            flat
            no-caps
            color="negative"
            :label="t('monitorPage.audit.clearConfirmBtn')"
            :loading="clearing"
            @click="doClear"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="detailOpen">
      <q-card class="app-dialog-card app-dialog-card--lg app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-start justify-between">
          <div class="audit-detail-head">
            <span
              class="apm-status-dot audit-detail-head__dot"
              :class="`apm-status-dot--${eventTone(selected?.action || '')}`"
            />
            <div>
              <div class="app-glass-dialog__title">{{ t('monitorPage.audit.dialogTitle') }}</div>
              <div class="app-glass-dialog__subtitle">{{ selected?.action }}</div>
            </div>
          </div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-glass-dialog__body">
            <div class="apm-meta-grid q-mb-md">
              <div class="apm-meta-item">
                <div class="apm-meta-item__label">{{ t('monitorPage.audit.labelResource') }}</div>
                <div class="apm-meta-item__value ellipsis">
                  {{ selected?.resource }}<span v-if="selected?.resource_id"> / {{ selected?.resource_id }}</span>
                </div>
              </div>
              <div class="apm-meta-item">
                <div class="apm-meta-item__label">{{ t('monitorPage.audit.labelTime') }}</div>
                <div class="apm-meta-item__value">{{ formatDate(selected?.created_at) }}</div>
              </div>
              <div v-if="selected?.actor" class="apm-meta-item">
                <div class="apm-meta-item__label">{{ t('monitorPage.audit.labelActor') }}</div>
                <div class="apm-meta-item__value ellipsis">{{ selected?.actor }}</div>
              </div>
              <div v-if="selected?.ip" class="apm-meta-item">
                <div class="apm-meta-item__label">IP</div>
                <div class="apm-meta-item__value">{{ selected?.ip }}</div>
              </div>
              <div v-if="selected?.user_agent" class="apm-meta-item">
                <div class="apm-meta-item__label">User Agent</div>
                <div class="apm-meta-item__value ellipsis">{{ selected?.user_agent }}</div>
              </div>
              <div v-if="selected?.severity" class="apm-meta-item">
                <div class="apm-meta-item__label">{{ t('monitorPage.audit.labelSeverity') }}</div>
                <div class="apm-meta-item__value">
                  <q-badge :color="severityColor(selected!.severity)">{{ selected?.severity }}</q-badge>
                </div>
              </div>
            </div>
            <pre class="monitor-json app-code-block">{{ selectedJSON }}</pre>
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn flat no-caps :label="t('monitorPage.audit.copyJson')" icon="content_copy" @click="copyJSON" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { copyToClipboard } from 'quasar';
import { useI18n } from 'vue-i18n';
import AppPageToolbar from '../layout/AppPageToolbar.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';

import type { AuditLog, AuditQuery } from '../../features/monitor/types';
import { auditDetailSummary, compactJSON, formatDate, parseJSON } from '../../features/monitor/utils';
import { AUDIT_DEFAULT_PAGE_SIZE } from '../../features/constants/queryLimits';
import { createAuditColumns } from './monitorTableUi';

const { t } = useI18n();
const columns = computed(() => createAuditColumns(t));

const props = defineProps<{
  rows: AuditLog[];
  total: number;
  loading: boolean;
}>();

const emit = defineEmits<{
  load: [query: AuditQuery];
  notify: [payload: { message: string; type: 'positive' | 'negative' | 'warning' }];
  clear: [];
}>();

// 事件类型（动词级，后端按 "<verb>.%" 前缀匹配）与实体类型为固定枚举，
// 与 18-monitor.md §2.1 及后端审计点覆盖范围对齐。
const ACTION_VERBS = ['create', 'update', 'delete', 'toggle', 'credentials', 'archive', 'sync'] as const;
const RESOURCE_TYPES = [
  'agent',
  'team',
  'channel',
  'provider',
  'config',
  'session',
  'tool',
  'mcp_server',
  'skill',
] as const;

const actionOptions = ACTION_VERBS.map((v) => ({ label: v, value: v }));
const resourceOptions = RESOURCE_TYPES.map((r) => ({ label: r, value: r }));

const keyword = ref('');
const actionFilter = ref<string | null>(null);
const resourceFilter = ref<string | null>(null);
// 默认隐藏系统噪音（sync.skill 等文件同步审计），用户可手动放开查看。
const hideSystem = ref(true);
const selected = ref<AuditLog | null>(null);
const detailOpen = ref(false);
const confirmClear = ref(false);
const clearing = ref(false);
const page = ref(1);
const pageSize = ref(AUDIT_DEFAULT_PAGE_SIZE);

const pageMax = computed(() => Math.max(1, Math.ceil(props.total / pageSize.value)));

const query = computed<AuditQuery>(() => ({
  limit: pageSize.value,
  offset: (page.value - 1) * pageSize.value,
  action: actionFilter.value ?? '',
  resource: resourceFilter.value ?? '',
  keyword: keyword.value.trim(),
  // 显式按动作过滤时由后端忽略 exclude_system（否则选 sync 会恒为空）。
  exclude_system: hideSystem.value && !actionFilter.value,
}));

// 筛选变化回到第一页；query 变化统一触发服务端加载（初始加载由父级负责，与默认值一致）。
watch([keyword, actionFilter, resourceFilter, hideSystem], () => {
  page.value = 1;
});
watch(query, (q) => emit('load', q));
// 外部数据收缩（如删除）导致页码越界时收敛到末页。
watch(pageMax, (max) => {
  if (page.value > max) page.value = max;
});

const selectedJSON = computed(() => compactJSON(parseJSON(selected.value?.detail ?? '')));

function emitLoad() {
  emit('load', query.value);
}

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

function doClear() {
  clearing.value = true;
  emit('clear');
  // Parent handles the actual clear + notify; close dialog immediately.
  confirmClear.value = false;
  clearing.value = false;
}

function prettyDetail(detail: string): string {
  return compactJSON(parseJSON(detail));
}

/** 动作 → APM 状态点色调（删除红 / 新建绿 / 开关橙 / 凭据与更新蓝） */
function eventTone(action: string): string {
  if (action.includes('delete')) return 'error';
  if (action.includes('create')) return 'ok';
  if (action.includes('toggle')) return 'warn';
  if (action.includes('credentials')) return 'info';
  if (action.includes('update')) return 'info';
  return 'idle';
}

function severityColor(severity: string) {
  if (severity === 'critical' || severity === 'high') return 'negative';
  if (severity === 'warning' || severity === 'medium') return 'orange';
  return 'positive';
}

async function copyJSON() {
  await copyToClipboard(selectedJSON.value);
  emit('notify', { message: t('monitorPage.audit.copied'), type: 'positive' });
}
</script>

<style scoped>
.monitor-audit-toolbar {
  padding: var(--space-2) var(--space-4) 0;
  border-bottom: none;
}

/* 事件列：状态点 + 等宽动作名 */
.audit-event-cell {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.audit-event-cell .apm-status-dot {
  width: 8px;
  height: 8px;
}

.audit-event-cell__action {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  font-weight: 600;
}

/* 详情弹窗头部状态点 */
.audit-detail-head {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
}

.audit-detail-head__dot {
  width: 12px;
  height: 12px;
  margin-top: 6px;
}
</style>
