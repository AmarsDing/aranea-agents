<template>
  <q-page class="app-standard-page tool-runs-page">
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
import SkillPagination from "../components/skills/SkillPagination.vue";
import ToolAuditsFilters from "../components/tools/ToolAuditsFilters.vue";
import ToolAuditsTable from "../components/tools/ToolAuditsTable.vue";
import ToolHeroSection from "../components/tools/ToolHeroSection.vue";
import { useToolAuditsPage } from "../features/tools/useToolAuditsPage";

const {
  toolKey,
  agentId,
  userId,
  status,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  pageMax,
  statusOptions,
  loadRows,
  resetFilters
} = useToolAuditsPage();
</script>
