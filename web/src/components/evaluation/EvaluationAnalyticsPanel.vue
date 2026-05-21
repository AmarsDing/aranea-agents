<template>
  <q-card flat bordered>
    <q-card-section class="row items-center justify-between q-pb-none">
      <div class="text-subtitle2">趋势与对比</div>
      <div class="row q-gutter-sm items-center">
        <q-select
          v-model="localAgentId"
          dense
          outlined
          emit-value
          map-options
          :options="agentOptions"
          label="Agent"
          style="min-width: 180px"
          @update:model-value="onAgentChange"
        />
        <q-btn flat dense icon="refresh" :loading="trendLoading" @click="emit('refresh-trend')" />
      </div>
    </q-card-section>

    <q-card-section>
      <div v-if="trendLoading" class="text-grey-7 q-py-md">加载趋势…</div>
      <div v-else-if="!trendPoints.length" class="text-grey-7 q-py-md">暂无已完成运行记录</div>
      <q-table
        v-else
        flat
        dense
        :rows="trendPoints"
        :columns="trendColumns"
        row-key="run_id"
        :pagination="{ rowsPerPage: 5 }"
      />
    </q-card-section>

    <q-separator />

    <q-card-section>
      <div class="text-caption text-grey-7 q-mb-sm">勾选 2 条以上运行记录进行 A/B 对比（以最早一条为基线）</div>
      <q-table
        flat
        dense
        selection="multiple"
        v-model:selected="localSelected"
        :rows="runs"
        :columns="compareSelectColumns"
        row-key="id"
        :pagination="{ rowsPerPage: 5 }"
      />
      <div class="row q-gutter-sm q-mt-sm">
        <q-btn
          outline
          color="primary"
          label="对比选中"
          :disable="localSelected.length < 2"
          :loading="compareLoading"
          @click="emitCompare"
        />
      </div>
      <q-table
        v-if="comparisons.length"
        flat
        dense
        class="q-mt-md"
        :rows="comparisons"
        :columns="comparisonColumns"
        row-key="run_id"
        :pagination="{ rowsPerPage: 10 }"
      />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
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

watch(
  () => props.agentId,
  (v) => {
    localAgentId.value = v;
  }
);

const trendColumns = [
  { name: "created_at", label: "时间", field: "created_at", align: "left" as const },
  { name: "trigger_source", label: "触发", field: "trigger_source", align: "left" as const },
  { name: "exact_match_score", label: "Exact", field: "exact_match_score", align: "right" as const },
  { name: "contains_match_score", label: "Contains", field: "contains_match_score", align: "right" as const },
  { name: "llm_judge_score", label: "LLM", field: "llm_judge_score", align: "right" as const },
  { name: "pass_at_k", label: "pass@k", field: "pass_at_k", align: "right" as const }
];

const compareSelectColumns = [
  { name: "id", label: "Run", field: "id", align: "left" as const },
  { name: "status", label: "状态", field: "status", align: "left" as const },
  { name: "exact_match_score", label: "Exact", field: "exact_match_score", align: "right" as const },
  { name: "created_at", label: "时间", field: "created_at", align: "left" as const }
];

const comparisonColumns = [
  { name: "run_id", label: "Run", field: "run_id", align: "left" as const },
  { name: "exact_match_score", label: "Exact", field: "exact_match_score", align: "right" as const },
  { name: "delta_exact_match", label: "Δ Exact", field: "delta_exact_match", align: "right" as const },
  { name: "delta_llm_judge", label: "Δ LLM", field: "delta_llm_judge", align: "right" as const },
  { name: "delta_tool_call_accuracy", label: "Δ Tool", field: "delta_tool_call_accuracy", align: "right" as const }
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
