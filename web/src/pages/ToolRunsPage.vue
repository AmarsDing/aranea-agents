<template>
  <q-page class="app-page-cream tool-runs-page">
    <tool-hero-section kicker="Tool observability" title="Tool 调用记录" subtitle="查看工具调用参数摘要、结果摘要、耗时、状态和错误信息。">
      <template #actions>
        <q-btn outline rounded no-caps class="tool-runs-outline-btn" icon="arrow_back" label="返回 Tools 管理" :to="{ name: 'tools' }" />
      </template>
    </tool-hero-section>

    <tool-runs-filters
      :tool-key="toolKey"
      :agent-id="agentId"
      :status="status"
      :from="from"
      :status-options="statusOptions"
      :loading="loading"
      @update:tool-key="toolKey = $event"
      @update:agent-id="agentId = $event"
      @update:status="status = $event"
      @update:from="from = $event"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="tools-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense label="重试" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <tool-runs-table :rows="rows" :loading="loading" />

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="条调用记录" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import SkillPagination from "../components/skills/SkillPagination.vue";
import ToolHeroSection from "../components/tools/ToolHeroSection.vue";
import ToolRunsFilters from "../components/tools/ToolRunsFilters.vue";
import ToolRunsTable from "../components/tools/ToolRunsTable.vue";
import { toolInvocationStatusOptions } from "../components/tools/toolUi";
import { listToolRuns } from "../features/tools/api";
import type { ToolInvocation } from "../features/tools/types";

const toolKey = ref("");
const agentId = ref("");
const status = ref("");
const from = ref("");
const page = ref(1);
const pageSize = ref(20);
const rows = ref<ToolInvocation[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");
const route = useRoute();

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const statusOptions = [...toolInvocationStatusOptions, { label: "失败", value: "failed" }];

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listToolRuns({
      tool_key: toolKey.value,
      agent_id: agentId.value,
      status: status.value,
      from: from.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载调用记录失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  toolKey.value = "";
  agentId.value = "";
  status.value = "";
  from.value = "";
  page.value = 1;
  void loadRows();
}

watch([toolKey, agentId, status, from], () => {
  page.value = 1;
  void loadRows();
});
watch([page, pageSize], () => {
  void loadRows();
});

onMounted(() => {
  if (typeof route.query.tool_key === "string") {
    toolKey.value = route.query.tool_key;
  }
  void loadRows();
});
</script>

<style scoped lang="sass">
.tool-runs-page
  padding: 24px

.tools-error-banner
  background: rgba(229, 92, 92, 0.92)
  color: #fff
  border: 1px solid rgba(255, 255, 255, 0.25)

body.body--dark .tools-error-banner
  background: rgba(255, 94, 122, 0.22)
  color: var(--color-text-primary)
  border-color: rgba(255, 255, 255, 0.12)

.tool-runs-outline-btn
  border-color: rgba(208, 192, 168, 0.85)
  color: var(--color-text-primary)

body:not(.body--dark) .tool-runs-outline-btn:hover
  background: var(--interaction-surface-hover)
</style>
