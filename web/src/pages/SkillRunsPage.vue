<template>
  <q-page class="app-page-cream app-registry-page skill-runs-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Skill observability</div>
        <h1 class="app-page-title">Skill 运行记录</h1>
        <p class="app-page-subtitle">按 Skill、Agent、结果筛选调用明细，用于追踪使用频率和执行质量。</p>
      </div>
      <div class="app-actions-bar">
        <q-btn outline rounded no-caps color="primary" icon="arrow_back" label="返回 Skill 管理" to="/skills" />
      </div>
    </section>

    <q-card flat class="app-registry-panel">
      <q-card-section class="app-form-field-grid app-registry-toolbar items-end">
        <q-input v-model="skillId" class="app-field-md" dense outlined clearable debounce="350" label="Skill ID" />
        <q-input v-model="agentId" class="app-field-md" dense outlined clearable debounce="350" label="Agent ID" />
        <q-select v-model="status" class="app-field-sm" dense outlined clearable emit-value map-options label="结果" :options="statusOptions" />
        <q-input v-model="from" class="app-field-md" dense outlined clearable label="开始时间 ISO" />
        <div class="app-actions-bar app-actions-bar--start">
          <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <div class="app-registry-table-shell">
      <skill-runs-table :rows="rows" :loading="loading" />
    </div>

    <skill-pagination v-model:page="page" v-model:page-size="pageSize" :page-max="pageMax" :total="total" :loading="loading" label="条运行记录" />
  </q-page>
</template>

<script setup lang="ts">
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
  resetFilters
} = useSkillRunsPage();
</script>
