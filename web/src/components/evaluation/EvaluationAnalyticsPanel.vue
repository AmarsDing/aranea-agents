<template>
  <q-card flat class="app-entity-glass-panel evaluation-analytics-panel">
    <q-card-section class="row items-center justify-between q-pb-none">
      <div class="text-subtitle2 text-weight-bold">趋势与对比</div>
      <div class="app-form-field-grid items-end" style="grid-template-columns: minmax(180px, 240px) auto">
        <q-select
          v-model="localAgentId"
          dense
          outlined
          emit-value
          map-options
          :options="agentOptions"
          label="Agent"
          @update:model-value="onAgentChange"
        />
        <q-btn flat dense round icon="refresh" :loading="trendLoading" @click="emit('refresh-trend')" />
      </div>
    </q-card-section>

    <q-card-section>
      <div v-if="trendLoading" class="text-grey-7 q-py-md">加载趋势…</div>
      <div v-else-if="!trendPoints.length" class="text-grey-7 q-py-md">暂无已完成运行记录</div>
      <template v-else>
        <AppRegistryTable
          :shell="false"
          :data-shell="true"
          column-persist-key="eval-trend"
          :rows="pagedTrendPoints"
          :columns="trendColumns"
          row-key="run_id"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        />
        <AppRegistryPagination
          v-model:page="trendPage"
          v-model:page-size="trendPageSize"
          :page-max="trendPageMax"
          :total="trendPoints.length"
          :loading="trendLoading"
          label="条趋势"
        />
      </template>
    </q-card-section>

    <q-separator />

    <q-card-section>
      <div class="text-caption text-grey-7 q-mb-sm">勾选 2 条以上运行记录进行 A/B 对比（以最早一条为基线）</div>
      <AppRegistryTable
        v-model:selected="localSelected"
        :shell="false"
        :data-shell="true"
        column-persist-key="eval-compare-select"
        selection="multiple"
        :rows="pagedCompareRuns"
        :columns="compareSelectColumns"
        row-key="id"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      />
      <AppRegistryPagination
        v-model:page="comparePage"
        v-model:page-size="comparePageSize"
        :page-max="comparePageMax"
        :total="runs.length"
        label="条运行"
        class="q-mt-sm"
      />
      <div class="app-actions-bar app-actions-bar--start q-mt-sm">
        <q-btn
          outline
          no-caps
          color="primary"
          label="对比选中"
          :disable="localSelected.length < 2"
          :loading="compareLoading"
          @click="emitCompare"
        />
      </div>
      <template v-if="comparisons.length">
        <AppRegistryTable
          :shell="false"
          :data-shell="true"
          column-persist-key="eval-comparison"
          class="q-mt-md"
          :rows="pagedComparisons"
          :columns="comparisonColumns"
          row-key="run_id"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        />
        <AppRegistryPagination
          v-model:page="comparisonPage"
          v-model:page-size="comparisonPageSize"
          :page-max="comparisonPageMax"
          :total="comparisons.length"
          label="条对比"
        />
      </template>
    </q-card-section>

    <q-separator />

    <q-card-section>
      <div class="row items-center justify-between">
        <div>
          <div class="text-subtitle2 text-weight-bold">{{ $t('evaluationPage.divergenceTitle') }}</div>
          <div v-if="divergence" class="text-caption text-grey-7">
            {{
              $t('evaluationPage.divergenceMeta', {
                threshold: divergence.threshold.toFixed(2),
                total: divergence.annotated_total,
              })
            }}
          </div>
        </div>
        <q-btn flat dense round icon="refresh" :loading="divergenceLoading" @click="emit('refresh-divergence')" />
      </div>
      <div v-if="divergenceLoading && !divergence" class="text-grey-7 q-py-md">
        {{ $t('evaluationPage.divergenceLoading') }}
      </div>
      <div v-else-if="!divergence || !divergence.annotated_total" class="text-grey-7 q-py-md">
        {{ $t('evaluationPage.divergenceEmpty') }}
      </div>
      <template v-else>
        <div class="row q-col-gutter-md q-mt-xs q-mb-md">
          <div class="col-auto column items-center">
            <div class="text-h6 text-weight-bold" :class="`text-${agreementColor}`">
              {{ formatPercent(divergence.agreement_rate) }}
            </div>
            <div class="text-caption text-grey-7">{{ $t('evaluationPage.divergenceAgreeRate') }}</div>
          </div>
          <div class="col-auto column items-center">
            <div class="text-h6 text-weight-bold">{{ divergence.agree_count }}</div>
            <div class="text-caption text-grey-7">{{ $t('evaluationPage.divergenceAgree') }}</div>
          </div>
          <div class="col-auto column items-center">
            <div class="text-h6 text-weight-bold">{{ divergence.diverge_count }}</div>
            <div class="text-caption text-grey-7">{{ $t('evaluationPage.divergenceDiverge') }}</div>
          </div>
          <div class="col-auto column items-center">
            <div class="text-h6 text-weight-bold text-warning">{{ divergence.false_pass_count }}</div>
            <div class="text-caption text-grey-7">{{ $t('evaluationPage.divergenceFalsePass') }}</div>
          </div>
          <div class="col-auto column items-center">
            <div class="text-h6 text-weight-bold text-negative">{{ divergence.false_fail_count }}</div>
            <div class="text-caption text-grey-7">{{ $t('evaluationPage.divergenceFalseFail') }}</div>
          </div>
        </div>
        <template v-if="divergence.divergent_cases.length">
          <AppRegistryTable
            :shell="false"
            :data-shell="true"
            column-persist-key="eval-divergence"
            :rows="pagedDivergentCases"
            :columns="divergenceColumns"
            row-key="result_id"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
          >
            <template #body-cell-input="props">
              <q-td :props="props">
                <span class="app-registry-cell-sub ellipsis" :title="props.row.input">{{ props.row.input }}</span>
              </q-td>
            </template>
            <template #body-cell-human_pass="props">
              <q-td :props="props">
                <q-chip dense :color="props.row.human_pass ? 'positive' : 'negative'" text-color="white" size="sm">
                  {{ $t(props.row.human_pass ? 'evaluationPage.divergencePass' : 'evaluationPage.divergenceFail') }}
                </q-chip>
              </q-td>
            </template>
            <template #body-cell-divergence_kind="props">
              <q-td :props="props">
                <q-chip
                  dense
                  :color="props.row.divergence_kind === 'false_pass' ? 'warning' : 'negative'"
                  text-color="white"
                  size="sm"
                >
                  {{
                    $t(
                      props.row.divergence_kind === 'false_pass'
                        ? 'evaluationPage.divergenceFalsePass'
                        : 'evaluationPage.divergenceFalseFail',
                    )
                  }}
                </q-chip>
              </q-td>
            </template>
          </AppRegistryTable>
          <AppRegistryPagination
            v-model:page="divergencePage"
            v-model:page-size="divergencePageSize"
            :page-max="divergencePageMax"
            :total="divergence.divergent_cases.length"
            :loading="divergenceLoading"
            :label="$t('evaluationPage.divergencePagination')"
          />
        </template>
        <div v-else class="text-grey-7 q-py-sm">{{ $t('evaluationPage.divergenceNoCases') }}</div>
      </template>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';

import type { EvalRun, EvalRunComparison, EvalTrendPoint, JudgeDivergence } from '../../features/evaluation/types';
import {
  buildEvalDivergenceColumns,
  EVAL_COMPARE_TABLE_COLUMNS,
  EVAL_RECENT_RUN_TABLE_COLUMNS,
  EVAL_TREND_TABLE_COLUMNS,
} from '../../features/evaluation/evaluationTableUi';

const props = defineProps<{
  agentId: string;
  agentOptions: { label: string; value: string }[];
  runs: EvalRun[];
  trendPoints: EvalTrendPoint[];
  comparisons: EvalRunComparison[];
  divergence: JudgeDivergence | null;
  trendLoading: boolean;
  compareLoading: boolean;
  divergenceLoading: boolean;
}>();

const emit = defineEmits<{
  'update:agentId': [value: string];
  'refresh-trend': [];
  'refresh-divergence': [];
  compare: [runIds: string[]];
}>();

const localAgentId = ref(props.agentId);
const localSelected = ref<EvalRun[]>([]);

const trendPage = ref(1);
const trendPageSize = ref(5);
const comparePage = ref(1);
const comparePageSize = ref(5);
const comparisonPage = ref(1);
const comparisonPageSize = ref(10);
const divergencePage = ref(1);
const divergencePageSize = ref(5);

const trendPageMax = computed(() => Math.max(1, Math.ceil(props.trendPoints.length / trendPageSize.value)));
const comparePageMax = computed(() => Math.max(1, Math.ceil(props.runs.length / comparePageSize.value)));
const comparisonPageMax = computed(() => Math.max(1, Math.ceil(props.comparisons.length / comparisonPageSize.value)));
const divergencePageMax = computed(() =>
  Math.max(1, Math.ceil((props.divergence?.divergent_cases.length ?? 0) / divergencePageSize.value)),
);

const pagedTrendPoints = computed(() => {
  const start = (trendPage.value - 1) * trendPageSize.value;
  return props.trendPoints.slice(start, start + trendPageSize.value);
});
const pagedCompareRuns = computed(() => {
  const start = (comparePage.value - 1) * comparePageSize.value;
  return props.runs.slice(start, start + comparePageSize.value);
});
const pagedComparisons = computed(() => {
  const start = (comparisonPage.value - 1) * comparisonPageSize.value;
  return props.comparisons.slice(start, start + comparisonPageSize.value);
});
const pagedDivergentCases = computed(() => {
  const cases = props.divergence?.divergent_cases ?? [];
  const start = (divergencePage.value - 1) * divergencePageSize.value;
  return cases.slice(start, start + divergencePageSize.value);
});

watch(
  () => props.agentId,
  (v) => {
    localAgentId.value = v;
  },
);

watch(
  () => props.runs,
  () => {
    localSelected.value = [];
    comparePage.value = 1;
  },
);

watch(
  () => props.trendPoints,
  () => {
    trendPage.value = 1;
  },
);

watch(
  () => props.comparisons,
  () => {
    comparisonPage.value = 1;
  },
);

watch(
  () => props.divergence,
  () => {
    divergencePage.value = 1;
  },
);

const { t } = useI18n();

const trendColumns = EVAL_TREND_TABLE_COLUMNS;
const compareSelectColumns = EVAL_RECENT_RUN_TABLE_COLUMNS;
const comparisonColumns = EVAL_COMPARE_TABLE_COLUMNS;
const divergenceColumns = computed(() => buildEvalDivergenceColumns(t));

function formatPercent(v: number): string {
  return `${(v * 100).toFixed(1)}%`;
}

const agreementColor = computed(() => {
  const rate = props.divergence?.agreement_rate ?? 0;
  if (rate >= 0.8) return 'positive';
  if (rate >= 0.6) return 'warning';
  return 'negative';
});

function onAgentChange(v: string) {
  emit('update:agentId', v);
  emit('refresh-trend');
}

function emitCompare() {
  emit(
    'compare',
    localSelected.value.map((r) => r.id),
  );
}
</script>
