<template>
  <q-page class="app-standard-page app-registry-page skill-runs-page">
    <AppPageHero
      kicker="Skill observability"
      title="Skill 运行记录"
      subtitle="按 Skill、Agent、结果筛选调用明细，用于追踪使用频率和执行质量。"
    >
      <template #actions>
        <q-btn outline rounded no-caps color="primary" icon="arrow_back" label="返回 Skill 管理" to="/skills" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input v-model="skillId" class="app-page-toolbar__field" dense outlined clearable debounce="350" label="Skill ID" />
      <q-input v-model="agentId" class="app-page-toolbar__field" dense outlined clearable debounce="350" label="Agent ID" />
      <q-select v-model="status" class="app-page-toolbar__field" dense outlined clearable emit-value map-options label="结果" :options="statusOptions" />
      <q-input v-model="from" class="app-page-toolbar__field" dense outlined clearable label="开始时间 ISO" />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
      </template>
    </AppPageToolbar>

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
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import SkillPagination from "../components/skills/SkillPagination.vue";
import SkillRunsTable from "../components/skills/SkillRunsTable.vue";
import { useSkillRunsPage } from "../features/skills/useSkillRunsPage";

const {
  skillId,
  agentId,
  status,
  from,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  statusOptions,
  pageMax,
  resetFilters,
  loadRows
} = useSkillRunsPage();
</script>
