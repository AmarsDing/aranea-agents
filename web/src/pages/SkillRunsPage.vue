<template>
  <q-page class="app-standard-page cap-page app-registry-page skill-runs-page">
    <AppPageToolbar>
      <q-input
        v-model="skillId"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        debounce="350"
        :label="t('shell.runs.filterSkillId')"
      />
      <q-input
        v-model="agentId"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        debounce="350"
        :label="t('shell.runs.filterAgentId')"
      />
      <q-select
        v-model="status"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('shell.runs.filterStatus')"
        :options="statusOptions"
      />
      <q-input
        v-model="from"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        type="date"
        :label="t('shell.runs.filterFrom')"
      />
      <q-input
        v-model="to"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        type="date"
        :label="t('shell.runs.filterTo')"
      />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('shell.runs.reset')" @click="resetFilters" />
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('shell.runs.refresh')"
          :loading="loading"
          @click="loadRows"
        />
      </template>
    </AppPageToolbar>

    <!-- 五态：已有数据但刷新失败 → 局部失败横幅，列表照常渲染 -->
    <AppStatePanel
      v-if="(error || forbidden) && rows.length > 0"
      variant="partial"
      dense
      class="q-mb-md"
      :description="t('shell.runs.staleError', { message: error })"
      :action-label="t('common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />

    <!-- 五态：无权限 → 只渲染 forbidden 面板，不提供重试 -->
    <AppStatePanel v-if="forbidden && rows.length === 0" variant="forbidden" class="q-mb-md" />

    <!-- 五态：加载失败且无数据 → 只渲染可重试错误面板，不与空态并存。 -->
    <AppStatePanel
      v-else-if="error && rows.length === 0"
      variant="error"
      class="q-mb-md"
      :description="error"
      :action-label="t('common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />

    <!-- 五态：首屏加载无数据 → 骨架面板 -->
    <AppStatePanel v-else-if="loading && rows.length === 0" variant="loading" :rows="4" />

    <skill-runs-table v-else :rows="rows" :loading="loading" :error="error" @retry="loadRows" @reset="resetFilters" />

    <AppRegistryPagination
      v-if="rows.length > 0"
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-max="pageMax"
      :total="total"
      :loading="loading"
      :label="t('shell.runs.pageUnit')"
    />
  </q-page>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import SkillRunsTable from '../components/skills/SkillRunsTable.vue';
import { useSkillRunsPage } from '../features/skills/useSkillRunsPage';

const { t } = useI18n();

const {
  skillId,
  agentId,
  status,
  from,
  to,
  page,
  pageSize,
  rows,
  total,
  loading,
  error,
  forbidden,
  statusOptions,
  pageMax,
  resetFilters,
  loadRows,
} = useSkillRunsPage();
</script>
