// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div>
    <q-banner v-if="!factsEndpointReady" rounded class="memory-info-banner q-mb-md">
      {{ t('memory.knowledge.unavailableBanner') }}
    </q-banner>
    <q-card flat bordered class="memory-card">
      <AppPageToolbar class="memory-knowledge-toolbar">
        <q-input
          :model-value="factKeyword"
          class="app-page-toolbar__search"
          dense
          outlined
          clearable
          debounce="300"
          :label="t('memory.knowledge.searchPlaceholder')"
          @update:model-value="$emit('update:factKeyword', String($event ?? ''))"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-select
          :model-value="factScope"
          class="app-page-toolbar__field"
          dense
          outlined
          clearable
          emit-value
          map-options
          :label="t('memory.knowledge.scopeLabel')"
          :options="scopeOptions"
          @update:model-value="$emit('update:factScope', $event as string | null)"
        />
        <q-select
          :model-value="factStatus"
          class="app-page-toolbar__field"
          dense
          outlined
          clearable
          emit-value
          map-options
          :label="t('memory.knowledge.statusLabel')"
          :options="factStatusOptions"
          @update:model-value="$emit('update:factStatus', $event as string | null)"
        />
        <template #actions>
          <q-btn flat rounded no-caps icon="restart_alt" :label="t('memory.knowledge.reset')" @click="$emit('reset')" />
          <q-btn
            flat
            rounded
            no-caps
            icon="add"
            color="primary"
            :label="t('memory.knowledge.create')"
            @click="$emit('createFact')"
          />
          <q-btn
            unelevated
            rounded
            no-caps
            color="primary"
            icon="manage_search"
            :label="t('memory.knowledge.search')"
            :loading="loadingFacts"
            @click="$emit('search')"
          />
        </template>
      </AppPageToolbar>

      <div v-if="factsEndpointReady" class="memory-knowledge-stats row q-gutter-lg q-px-md q-pt-sm">
        <div class="memory-knowledge-stats__item">
          <span class="memory-knowledge-stats__value text-positive">{{ factsActiveCount }}</span>
          <span class="memory-knowledge-stats__label">{{ t('memory.knowledge.statsActive') }}</span>
        </div>
        <div class="memory-knowledge-stats__item">
          <span class="memory-knowledge-stats__value text-blue-grey">{{ factsArchivedCount }}</span>
          <span class="memory-knowledge-stats__label">{{ t('memory.knowledge.statsArchived') }}</span>
        </div>
        <div class="memory-knowledge-stats__item">
          <span class="memory-knowledge-stats__value">{{ factRows.length }}</span>
          <span class="memory-knowledge-stats__label">{{ t('memory.knowledge.statsListed') }}</span>
        </div>
      </div>

      <q-card-section v-if="!loadingFacts && factRows.length === 0" class="app-registry-empty app-empty-state-center">
        <q-icon name="psychology_alt" size="44px" color="grey-6" />
        <div class="text-subtitle1 q-mt-sm">{{ t('memory.knowledge.emptyTitle') }}</div>
        <div class="text-caption text-grey-7">{{ t('memory.knowledge.emptyCaption') }}</div>
      </q-card-section>

      <template v-else>
        <div class="app-registry-table-shell">
          <AppRegistryTable
            :shell="false"
            table-class="memory-facts-table"
            :rows="factRows"
            :columns="factColumns"
            row-key="id"
            :loading="loadingFacts"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
          >
            <template #body-cell-statement="slotProps">
              <q-td :props="slotProps">
                <AppRegistryHoverTip :text="factHoverText(slotProps.row)">
                  <div class="memory-fact-statement">
                    <q-chip
                      v-if="slotProps.row.fact_kind"
                      dense
                      square
                      size="sm"
                      outline
                      color="blue-grey"
                      class="memory-fact-statement__kind"
                    >
                      {{ kindLabel(slotProps.row.fact_kind) }}
                    </q-chip>
                    <span class="memory-fact-statement__text ellipsis-2-lines">{{ slotProps.row.statement }}</span>
                  </div>
                </AppRegistryHoverTip>
              </q-td>
            </template>
            <template #body-cell-scope="slotProps">
              <q-td :props="slotProps">
                <q-chip dense square color="primary" text-color="white">{{
                  scopeLabel(slotProps.row.scope_type)
                }}</q-chip>
              </q-td>
            </template>
            <template #body-cell-confidence="slotProps">
              <q-td :props="slotProps">
                <q-linear-progress
                  rounded
                  size="9px"
                  :value="bounded(slotProps.row.confidence)"
                  :color="scoreColor(slotProps.row.confidence)"
                />
                <div class="text-caption q-mt-xs">{{ formatPercent(slotProps.row.confidence) }}</div>
              </q-td>
            </template>
            <template #body-cell-source="slotProps">
              <q-td :props="slotProps">
                <span class="app-registry-cell-sub ellipsis">{{ slotProps.row.source_kind || '—' }}</span>
              </q-td>
            </template>
            <template #body-cell-updated="slotProps">
              <q-td :props="slotProps">
                <span class="app-registry-cell-sub">{{ formatFactDate(slotProps.row.updated_at) }}</span>
              </q-td>
            </template>
            <template #body-cell-actions="slotProps">
              <q-td :props="slotProps">
                <div class="app-registry-cell-actions">
                  <q-btn
                    flat
                    dense
                    round
                    icon="visibility"
                    color="primary"
                    :aria-label="t('memory.knowledge.detailAria')"
                    @click="$emit('openFact', slotProps.row)"
                  />
                </div>
              </q-td>
            </template>
          </AppRegistryTable>

          <AppRegistryPagination
            :page="page"
            :page-size="pageSize"
            :page-max="pageMax"
            :total="factsTotal"
            :loading="loadingFacts"
            :label="t('memory.knowledge.paginationUnit')"
            @update:page="$emit('update:page', $event)"
            @update:page-size="$emit('update:pageSize', $event)"
          />
        </div>
      </template>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { QTableProps } from 'quasar';
import AppPageToolbar from '../../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../../components/layout/AppRegistryPagination.vue';
import type { MemoryFact } from './types';

const { t } = useI18n();

defineProps<{
  factsEndpointReady: boolean;
  factKeyword: string;
  factScope: string | null;
  factStatus: string | null;
  scopeOptions: Array<{ label: string; value: string }>;
  factStatusOptions: Array<{ label: string; value: string }>;
  factRows: MemoryFact[];
  factColumns: QTableProps['columns'];
  loadingFacts: boolean;
  factsActiveCount: number;
  factsArchivedCount: number;
  /** 与当前 status 过滤同口径的事实总数（服务端分页 total）。 */
  factsTotal: number;
  page: number;
  pageSize: number;
  pageMax: number;
}>();

defineEmits<{
  'update:factKeyword': [value: string];
  'update:factScope': [value: string | null];
  'update:factStatus': [value: string | null];
  'update:page': [value: number];
  'update:pageSize': [value: number];
  reset: [];
  search: [];
  openFact: [fact: MemoryFact];
  createFact: [];
}>();

function bounded(value?: number) {
  const numeric = Number(value) || 0;
  return Math.max(0, Math.min(1, numeric));
}

function scoreColor(value?: number) {
  const score = bounded(value);
  if (score >= 0.75) return 'positive';
  if (score >= 0.45) return 'warning';
  return 'negative';
}

function formatPercent(value?: number) {
  return `${Math.round((Number(value) || 0) * 100)}%`;
}

function formatFactDate(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function factHoverText(row: MemoryFact) {
  const parts = [];
  if (row.statement?.trim()) parts.push(row.statement.trim());
  if (row.details_markdown?.trim()) parts.push(row.details_markdown.trim());
  return parts.join('\n\n');
}

function scopeLabel(scopeType?: string) {
  const key = `memory.knowledge.scope.${scopeType || 'agent'}`;
  const translated = t(key);
  return translated !== key ? translated : scopeType || 'agent';
}

function kindLabel(factKind?: string) {
  const key = `memory.factEdit.kind.${factKind || 'fact'}`;
  const translated = t(key);
  return translated !== key ? translated : factKind || 'fact';
}
</script>

<style scoped>
.memory-fact-statement {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  min-width: 0;
}

.memory-fact-statement__kind {
  flex-shrink: 0;
  margin-top: 1px;
}

.memory-fact-statement__text {
  color: var(--color-text-primary);
  font-size: 13px;
  line-height: 1.5;
  word-break: break-word;
}

.memory-knowledge-toolbar {
  padding: var(--space-3) var(--space-4) 0;
  border-bottom: none;
}

.memory-knowledge-stats {
  border-bottom: 1px solid var(--glass-border);
  padding-bottom: var(--space-3);
}

.memory-knowledge-stats__item {
  display: flex;
  align-items: baseline;
  gap: var(--space-2);
}

.memory-knowledge-stats__value {
  font-size: 20px;
  font-weight: 700;
  color: var(--color-text-primary);
}

.memory-knowledge-stats__label {
  font-size: 12px;
  color: var(--color-text-secondary);
}
</style>
