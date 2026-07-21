// Container: approved — feature-local panel; recall debug + composite search for admin tooling.
<template>
  <div class="column q-gutter-md">
    <q-card flat bordered class="memory-card">
      <q-card-section>
        <div class="text-h6">{{ t('memory.recall.title') }}</div>
        <div class="text-caption text-grey-7">
          {{ t('memory.recall.subtitle') }}
        </div>
      </q-card-section>
      <q-card-section class="q-gutter-md">
        <div class="row q-col-gutter-md">
          <div class="col-12 col-md-8">
            <q-input v-model="query" :label="t('memory.recall.queryLabel')" dense outlined clearable @keyup.enter="runDebug" />
          </div>
          <div class="col-6 col-md-2">
            <q-input v-model.number="l2Limit" type="number" :label="t('memory.recall.l2Limit')" dense outlined />
          </div>
          <div class="col-6 col-md-2">
            <q-input v-model.number="l3Limit" type="number" :label="t('memory.recall.l3Limit')" dense outlined />
          </div>
        </div>
        <div class="row q-gutter-sm">
          <q-btn
            color="primary"
            :label="t('memory.recall.debugButton')"
            :loading="loadingDebug"
            :disable="!agentId || !query.trim()"
            @click="runDebug"
          />
          <q-btn
            outline
            color="secondary"
            :label="t('memory.recall.compositeButton')"
            :loading="loadingComposite"
            :disable="!agentId || !query.trim()"
            @click="runComposite"
          />
        </div>
        <q-banner v-if="error" rounded class="bg-negative text-white">{{ error }}</q-banner>
      </q-card-section>
    </q-card>

    <div v-if="compositeHits.length" class="row q-col-gutter-md">
      <div class="col-12">
        <q-card flat bordered class="memory-card">
          <q-card-section>
            <div class="text-subtitle1">{{ t('memory.recall.compositeTitle') }}</div>
          </q-card-section>
          <AppRegistryMarkupTable
            :rows="compositeRows"
            :columns="compositeColumns as RegistryTableColumn[]"
            row-key="row_uid"
          >
            <template #cell-layer="{ row }">
              <AppRegistryHoverTip :text="String(row.text || row.id)" :empty-label="t('memory.recall.emptyText')">
                <q-badge :color="row.layer === 'L2' ? 'teal' : 'deep-purple'">{{ row.layer }}</q-badge>
              </AppRegistryHoverTip>
            </template>
            <template #cell-score="{ row }">
              {{ formatScore(Number(row.score)) }}
            </template>
          </AppRegistryMarkupTable>
        </q-card>
      </div>
    </div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-md-6">
        <recall-hit-table :title="t('memory.recall.l2Title')" color="teal" :rows="l2Hits" />
      </div>
      <div class="col-12 col-md-6">
        <recall-hit-table :title="t('memory.recall.l3Title')" color="deep-purple" :rows="l3Hits" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CompositeSearchHit } from './types';
import { useMemoryRecallTester } from './composables/useMemoryRecallTester';
import RecallHitTable from '../../components/memory/RecallHitTable.vue';
import AppRegistryHoverTip from '../../components/layout/AppRegistryHoverTip.vue';
import AppRegistryMarkupTable from '../../components/layout/AppRegistryMarkupTable.vue';
import { buildCompositeColumns } from './memoryTableUi';
import type { RegistryTableColumn } from '../ui/registryTableColumns';

const { t } = useI18n();

const compositeColumns = computed(() => buildCompositeColumns(t));

const props = defineProps<{
  agentId: string | null;
  sessionId?: string | null;
}>();

const {
  l2Hits,
  l3Hits,
  compositeHits,
  loadingDebug,
  loadingComposite,
  error,
  runDebug: runDebugFetch,
  runComposite: runCompositeFetch,
} = useMemoryRecallTester(
  () => props.agentId,
  () => props.sessionId,
);

const query = ref('');
const l2Limit = ref(5);
const l3Limit = ref(8);

const compositeRows = computed(() =>
  compositeHits.value.map((row: CompositeSearchHit) => ({ ...row, row_uid: `${row.layer}-${row.id}` })),
);

async function runDebug() {
  await runDebugFetch(query.value, l2Limit.value, l3Limit.value);
}

async function runComposite() {
  await runCompositeFetch(query.value);
}

function formatScore(v: number) {
  return (Number(v) || 0).toFixed(3);
}
</script>
