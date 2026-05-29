<template>
  <q-page class="app-standard-page tool-runs-page">
    <tool-hero-section kicker="Tool observability" title="Tool 调用记录" subtitle="查看工具调用参数摘要、结果摘要、耗时、状态和错误信息。">
      <template #actions>
        <q-btn outline rounded no-caps class="app-outline-btn" icon="arrow_back" label="返回 Tools 管理" :to="{ name: 'tools' }" />
      </template>
    </tool-hero-section>

    <tool-runs-filters
      :tool-key="toolKey"
      :agent-id="agentId"
      :status="status"
      :from="from"
      :status-options="statusOptions"
      :loading="loading"
      @update:tool-key="toolKey = $event ?? ''"
      @update:agent-id="agentId = $event ?? ''"
      @update:status="status = $event ?? ''"
      @update:from="from = $event ?? ''"
      @reset="resetFilters"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense label="重试" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <tool-runs-table :rows="rows" :loading="loading" />

    <app-registry-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="条调用记录" />
  </q-page>
</template>

<script setup lang="ts">
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import ToolHeroSection from "../components/tools/ToolHeroSection.vue";
import ToolRunsFilters from "../components/tools/ToolRunsFilters.vue";
import ToolRunsTable from "../components/tools/ToolRunsTable.vue";
import { useToolRunsPage } from "../features/tools/useToolRunsPage";

const {
  toolKey,
  agentId,
  status,
  from,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  pageMax,
  statusOptions,
  resetFilters,
  loadRows
} = useToolRunsPage();
</script>
