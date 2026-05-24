// Container: approved — feature-local panel; recall debug + composite search for admin tooling.
<template>
  <div class="column q-gutter-md">
    <q-card flat bordered class="memory-card">
      <q-card-section>
        <div class="text-h6">Recall 调试器</div>
        <div class="text-caption text-grey-7">输入 query 查看 L2/L3 rerank 分数分解（keyword / vector / importance / recency / cross-encoder）。</div>
      </q-card-section>
      <q-card-section class="q-gutter-md">
        <div class="row q-col-gutter-md">
          <div class="col-12 col-md-8">
            <q-input v-model="query" label="Query" dense outlined clearable @keyup.enter="runDebug" />
          </div>
          <div class="col-6 col-md-2">
            <q-input v-model.number="l2Limit" type="number" label="L2 limit" dense outlined />
          </div>
          <div class="col-6 col-md-2">
            <q-input v-model.number="l3Limit" type="number" label="L3 limit" dense outlined />
          </div>
        </div>
        <div class="row q-gutter-sm">
          <q-btn color="primary" label="Debug Recall" :loading="loadingDebug" :disable="!agentId || !query.trim()" @click="runDebug" />
          <q-btn outline color="secondary" label="Composite Search" :loading="loadingComposite" :disable="!agentId || !query.trim()" @click="runComposite" />
        </div>
        <q-banner v-if="error" rounded class="bg-negative text-white">{{ error }}</q-banner>
      </q-card-section>
    </q-card>

    <div v-if="compositeHits.length" class="row q-col-gutter-md">
      <div class="col-12">
        <q-card flat bordered class="memory-card">
          <q-card-section>
            <div class="text-subtitle1">Composite Search（L2 + L3 融合）</div>
          </q-card-section>
          <q-markup-table flat dense wrap-cells>
            <thead>
              <tr>
                <th>Layer</th>
                <th>Text</th>
                <th class="text-right">Score</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in compositeHits" :key="`${row.layer}-${row.id}`">
                <td><q-badge :color="row.layer === 'L2' ? 'teal' : 'deep-purple'">{{ row.layer }}</q-badge></td>
                <td class="ellipsis" style="max-width: 480px">{{ row.text || row.id }}</td>
                <td class="text-right">{{ formatScore(row.score) }}</td>
              </tr>
            </tbody>
          </q-markup-table>
        </q-card>
      </div>
    </div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-md-6">
        <recall-hit-table title="L2 Episodes" color="teal" :rows="l2Hits" />
      </div>
      <div class="col-12 col-md-6">
        <recall-hit-table title="L3 Facts" color="deep-purple" :rows="l3Hits" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import type { CompositeSearchHit, MemoryRecallHit } from "./types";
import { compositeSearchMemories, debugMemoryRecall } from "./api";
import RecallHitTable from "./RecallHitTable.vue";

const props = defineProps<{
  agentId: string | null;
  sessionId?: string | null;
}>();

const query = ref("");
const l2Limit = ref(5);
const l3Limit = ref(8);
const l2Hits = ref<MemoryRecallHit[]>([]);
const l3Hits = ref<MemoryRecallHit[]>([]);
const compositeHits = ref<CompositeSearchHit[]>([]);
const loadingDebug = ref(false);
const loadingComposite = ref(false);
const error = ref("");

watch(
  () => props.agentId,
  () => {
    l2Hits.value = [];
    l3Hits.value = [];
    compositeHits.value = [];
    error.value = "";
  }
);

async function runDebug() {
  if (!props.agentId || !query.value.trim()) return;
  loadingDebug.value = true;
  error.value = "";
  try {
    const res = await debugMemoryRecall({
      agent_id: props.agentId,
      session_id: props.sessionId || undefined,
      query: query.value.trim(),
      l2_limit: l2Limit.value,
      l3_limit: l3Limit.value
    });
    l2Hits.value = res.l2_hits;
    l3Hits.value = res.l3_hits;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "Recall debug failed";
  } finally {
    loadingDebug.value = false;
  }
}

async function runComposite() {
  if (!props.agentId || !query.value.trim()) return;
  loadingComposite.value = true;
  error.value = "";
  try {
    compositeHits.value = await compositeSearchMemories({
      agent_id: props.agentId,
      session_id: props.sessionId || undefined,
      query: query.value.trim(),
      limit: 10
    });
  } catch (err) {
    error.value = err instanceof Error ? err.message : "Composite search failed";
  } finally {
    loadingComposite.value = false;
  }
}

function formatScore(v: number) {
  return (Number(v) || 0).toFixed(3);
}
</script>
