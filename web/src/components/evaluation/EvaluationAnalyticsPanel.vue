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
        :shell="false"
        :data-shell="true"
        column-persist-key="eval-compare-select"
        selection="multiple"
        v-model:selected="localSelected"
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
  </q-card>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AppRegistryPagination from "../layout/AppRegistryPagination.vue";
import { registryColWidth } from "../../features/ui/registryTableColumns";
import type { EvalRun, EvalRunComparison, EvalTrendPoint } from "../../features/evaluation/types";

const props = defineProps<{
  agentId: string;
  agentOptions: { label: string; value: string }[];
  runs: EvalRun[];
  trendPoints: EvalTrendPoint[];
  comparisons: EvalRunComparison[];
  trendLoading: boolean;
  compareLoading: boolean;
}>();

const emit = defineEmits<{
  "update:agentId": [value: string];
  "refresh-trend": [];
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

const trendPageMax = computed(() => Math.max(1, Math.ceil(props.trendPoints.length / trendPageSize.value)));
const comparePageMax = computed(() => Math.max(1, Math.ceil(props.runs.length / comparePageSize.value)));
const comparisonPageMax = computed(() => Math.max(1, Math.ceil(props.comparisons.length / comparisonPageSize.value)));

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

watch(
  () => props.agentId,
  (v) => {
    localAgentId.value = v;
  }
);

const trendColumns = [
  { name: "created_at", label: "时间", field: "created_at", align: "left" as const, ...registryColWidth("11%") },
  { name: "trigger_source", label: "触发", field: "trigger_source", align: "left" as const, ...registryColWidth("72px") },
  { name: "exact_match_score", label: "Exact", field: "exact_match_score", align: "right" as const, ...registryColWidth("64px") },
  { name: "contains_match_score", label: "Contains", field: "contains_match_score", align: "right" as const, ...registryColWidth("64px") },
  { name: "llm_judge_score", label: "LLM", field: "llm_judge_score", align: "right" as const, ...registryColWidth("64px") },
  { name: "pass_at_k", label: "pass@k", field: "pass_at_k", align: "right" as const, ...registryColWidth("64px") }
];

const compareSelectColumns = [
  { name: "id", label: "Run", field: "id", align: "left" as const, ...registryColWidth("10%") },
  { name: "status", label: "状态", field: "status", align: "left" as const, ...registryColWidth("9%") },
  { name: "exact_match_score", label: "Exact", field: "exact_match_score", align: "right" as const, ...registryColWidth("64px") },
  { name: "created_at", label: "时间", field: "created_at", align: "left" as const, ...registryColWidth("11%") }
];

const comparisonColumns = [
  { name: "run_id", label: "Run", field: "run_id", align: "left" as const, ...registryColWidth("10%") },
  { name: "exact_match_score", label: "Exact", field: "exact_match_score", align: "right" as const, ...registryColWidth("64px") },
  { name: "delta_exact_match", label: "Δ Exact", field: "delta_exact_match", align: "right" as const, ...registryColWidth("64px") },
  { name: "delta_llm_judge", label: "Δ LLM", field: "delta_llm_judge", align: "right" as const, ...registryColWidth("64px") },
  { name: "delta_tool_call_accuracy", label: "Δ Tool", field: "delta_tool_call_accuracy", align: "right" as const, ...registryColWidth("64px") }
];

function onAgentChange(v: string) {
  emit("update:agentId", v);
  emit("refresh-trend");
}

function emitCompare() {
  emit(
    "compare",
    localSelected.value.map((r) => r.id)
  );
}
</script>
