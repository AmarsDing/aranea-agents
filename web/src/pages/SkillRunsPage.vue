<template>
  <q-page class="app-page-cream skill-runs-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Skill observability</div>
        <h1 class="app-page-title">Skill 运行记录</h1>
        <p class="app-page-subtitle">按 Skill、Agent、结果筛选调用明细，用于追踪使用频率和执行质量。</p>
      </div>
      <q-btn outline rounded color="primary" icon="arrow_back" label="返回 Skill 管理" to="/skills" />
    </section>

    <q-card flat bordered class="skill-runs-filter q-mb-md">
      <q-card-section class="row q-col-gutter-sm items-center">
        <div class="col-12 col-md-3">
          <q-input v-model="skillId" dense outlined clearable debounce="350" label="Skill ID" />
        </div>
        <div class="col-12 col-md-3">
          <q-input v-model="agentId" dense outlined clearable debounce="350" label="Agent ID" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-select v-model="status" dense outlined clearable emit-value map-options label="结果" :options="statusOptions" />
        </div>
        <div class="col-12 col-sm-6 col-md-2">
          <q-input v-model="from" dense outlined clearable label="开始时间 ISO" />
        </div>
        <div class="col-12 col-sm-6 col-md-2 row justify-end">
          <q-btn flat rounded icon="restart_alt" label="重置" @click="resetFilters" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <skill-runs-table :rows="rows" :loading="loading" />

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="条运行记录" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import SkillPagination from "../components/skills/SkillPagination.vue";
import SkillRunsTable from "../components/skills/SkillRunsTable.vue";
import { listSkillRuns } from "../features/skills/api";
import type { SkillInvocation } from "../features/skills/types";

const skillId = ref("");
const agentId = ref("");
const status = ref("");
const from = ref("");
const page = ref(1);
const pageSize = ref(20);
const rows = ref<SkillInvocation[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");

const statusOptions = [
  { label: "成功", value: "success" },
  { label: "失败", value: "failure" }
];
const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listSkillRuns({
      skill_id: skillId.value,
      agent_id: agentId.value,
      status: status.value,
      from: from.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载运行记录失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  skillId.value = "";
  agentId.value = "";
  status.value = "";
  from.value = "";
  page.value = 1;
  void loadRows();
}

watch([skillId, agentId, status, from], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => {
  void loadRows();
});

onMounted(loadRows);
</script>

<style scoped lang="sass">
.skill-runs-page
  padding: var(--space-6)

.skill-runs-filter
  border-radius: 22px
</style>
