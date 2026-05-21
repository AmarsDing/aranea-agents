<template>
  <q-page class="app-page-cream tool-runs-page">
    <tool-hero-section
      kicker="Tool governance"
      title="工具调用审计"
      subtitle="结构化审计谁在何时调用了什么工具；默认保留 90 天。"
    >
      <template #actions>
        <q-btn outline rounded no-caps class="tool-runs-outline-btn" icon="arrow_back" label="返回 Tools 管理" :to="{ name: 'tools' }" />
      </template>
    </tool-hero-section>

    <tool-audits-filters
      :tool-key="toolKey"
      :agent-id="agentId"
      :user-id="userId"
      :status="status"
      :status-options="statusOptions"
      :loading="loading"
      @update:tool-key="toolKey = $event ?? ''"
      @update:agent-id="agentId = $event ?? ''"
      @update:user-id="userId = $event ?? ''"
      @update:status="status = $event ?? ''"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="tools-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense label="重试" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <tool-audits-table :rows="rows" :loading="loading" />

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="条审计记录" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import SkillPagination from "../components/skills/SkillPagination.vue";
import ToolAuditsFilters from "../components/tools/ToolAuditsFilters.vue";
import ToolAuditsTable from "../components/tools/ToolAuditsTable.vue";
import ToolHeroSection from "../components/tools/ToolHeroSection.vue";
import { toolInvocationStatusOptions } from "../components/tools/toolUi";
import { listToolInvocationAudits } from "../features/tools/api";
import type { ToolInvocationAudit } from "../features/tools/types";

const toolKey = ref("");
const agentId = ref("");
const userId = ref("");
const status = ref("");
const page = ref(1);
const pageSize = ref(20);
const rows = ref<ToolInvocationAudit[]>([]);
const total = ref(0);
const loading = ref(false);
const error = ref("");

const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const statusOptions = [...toolInvocationStatusOptions];

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const data = await listToolInvocationAudits({
      tool_key: toolKey.value,
      agent_id: agentId.value,
      user_id: userId.value,
      status: status.value,
      page: page.value,
      page_size: pageSize.value
    });
    rows.value = data.items;
    total.value = data.total;
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载审计日志失败";
  } finally {
    loading.value = false;
  }
}

function resetFilters() {
  toolKey.value = "";
  agentId.value = "";
  userId.value = "";
  status.value = "";
  page.value = 1;
  void loadRows();
}

watch([page, pageSize], () => {
  void loadRows();
});

onMounted(() => {
  void loadRows();
});
</script>

<style scoped lang="scss">
.tool-runs-page {
  padding: 1.25rem 1.5rem 2rem;
}

.tool-runs-outline-btn {
  border-color: rgba(var(--q-primary-rgb), 0.35);
}
</style>
