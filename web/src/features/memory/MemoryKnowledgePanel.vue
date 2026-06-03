// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div>
    <q-banner v-if="!factsEndpointReady" rounded class="memory-info-banner q-mb-md">
      L3 facts 暂时不可用。请检查 **`memory/v1`** 网关（**`GET /v1/memory/l3/facts`**）或筛选条件。
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
          label="搜索知识、偏好或规则"
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
          label="Scope"
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
          label="状态"
          :options="factStatusOptions"
          @update:model-value="$emit('update:factStatus', $event as string | null)"
        />
        <template #actions>
          <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="$emit('reset')" />
          <q-btn
            unelevated
            rounded
            no-caps
            color="primary"
            icon="manage_search"
            label="查询"
            :loading="loadingFacts"
            @click="$emit('search')"
          />
        </template>
      </AppPageToolbar>

      <q-card-section v-if="!loadingFacts && factRows.length === 0" class="app-registry-empty app-empty-state-center">
        <q-icon name="psychology_alt" size="44px" color="grey-6" />
        <div class="text-subtitle1 q-mt-sm">暂无长期知识</div>
        <div class="text-caption text-grey-7">用户确认的偏好、规则和经验会在 L3 facts 写入后出现在这里。</div>
      </q-card-section>

      <template v-else>
        <div class="app-registry-table-shell">
          <AppRegistryTable
            :shell="false"
            table-class="memory-facts-table"
            :rows="pagedRows"
            :columns="factColumns"
            row-key="id"
            :loading="loadingFacts"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
          >
            <template #body-cell-scope="props">
              <q-td :props="props">
                <AppRegistryHoverTip :text="factHoverText(props.row)">
                  <q-chip dense square color="primary" text-color="white">{{ props.row.scope_type || 'agent' }}</q-chip>
                </AppRegistryHoverTip>
              </q-td>
            </template>
            <template #body-cell-confidence="props">
              <q-td :props="props">
                <q-linear-progress
                  rounded
                  size="9px"
                  :value="bounded(props.row.confidence)"
                  :color="scoreColor(props.row.confidence)"
                />
                <div class="text-caption q-mt-xs">{{ formatPercent(props.row.confidence) }}</div>
              </q-td>
            </template>
            <template #body-cell-source="props">
              <q-td :props="props">
                <span class="app-registry-cell-sub ellipsis">{{ props.row.source_kind || '—' }}</span>
              </q-td>
            </template>
            <template #body-cell-updated="props">
              <q-td :props="props">
                <span class="app-registry-cell-sub">{{ formatFactDate(props.row.updated_at) }}</span>
              </q-td>
            </template>
            <template #body-cell-actions="props">
              <q-td :props="props">
                <div class="app-registry-cell-actions">
                  <q-btn
                    flat
                    dense
                    round
                    icon="visibility"
                    color="primary"
                    aria-label="查看知识详情"
                    @click="$emit('openFact', props.row)"
                  />
                </div>
              </q-td>
            </template>
          </AppRegistryTable>

          <AppRegistryPagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :page-max="pageMax"
            :total="factRows.length"
            :loading="loadingFacts"
            label="条知识"
          />
        </div>
      </template>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { QTableProps } from 'quasar';
import AppPageToolbar from '../../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../../components/layout/AppRegistryHoverTip.vue';
import AppRegistryPagination from '../../components/layout/AppRegistryPagination.vue';
import type { MemoryFact } from './types';

const props = defineProps<{
  factsEndpointReady: boolean;
  factKeyword: string;
  factScope: string | null;
  factStatus: string | null;
  scopeOptions: Array<{ label: string; value: string }>;
  factStatusOptions: Array<{ label: string; value: string }>;
  factRows: MemoryFact[];
  factColumns: QTableProps['columns'];
  loadingFacts: boolean;
}>();

defineEmits<{
  'update:factKeyword': [value: string];
  'update:factScope': [value: string | null];
  'update:factStatus': [value: string | null];
  reset: [];
  search: [];
  openFact: [fact: MemoryFact];
}>();

const page = ref(1);
const pageSize = ref(12);
const pageMax = computed(() => Math.max(1, Math.ceil(props.factRows.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return props.factRows.slice(start, start + pageSize.value);
});

watch(
  () => props.factRows,
  () => {
    page.value = 1;
  },
);

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
</script>

<style scoped>
.memory-knowledge-toolbar {
  padding: var(--space-3) var(--space-4) 0;
  border-bottom: none;
}
</style>
