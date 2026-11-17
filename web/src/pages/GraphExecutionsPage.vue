<template>
  <q-page :class="['app-standard-page graph-executions-page', { 'is-dark': isDark }]">
    <AppPageHero kicker="执行历史" :title="graphName" subtitle="查看此 Graph 的所有执行记录。">
      <template #actions>
        <q-btn flat rounded icon="arrow_back" label="返回列表" @click="goBack" />
      </template>
    </AppPageHero>

    <div
      v-if="executionHistory.length > 0 || statusFilter || timeRangeFilter"
      class="graph-executions-page__filters q-mt-md row q-gutter-sm items-center"
    >
      <q-btn-toggle
        v-model="statusFilter"
        flat
        no-caps
        dense
        toggle-color="accent"
        :options="STATUS_FILTER_OPTIONS"
        class="graph-executions-page__filter-toggle"
      />
      <q-btn-toggle
        v-model="timeRangeFilter"
        flat
        no-caps
        dense
        toggle-color="accent"
        :options="TIME_RANGE_OPTIONS"
        class="graph-executions-page__filter-toggle"
      />
      <q-space />
      <div class="text-caption text-grey-7">{{ filteredHistory.length }} 条记录</div>
    </div>

    <!-- 五态：已有记录但刷新失败 → 局部失败横幅，列表照常渲染 -->
    <AppStatePanel
      v-if="error && executionHistory.length > 0"
      variant="partial"
      dense
      class="q-mt-md"
      :description="error"
      action-label="重试"
      action-icon="refresh"
      @action="reload"
    />

    <!-- 五态：加载失败且无记录 → 只渲染可重试错误面板。
         403 错误码未透出（useGraphExecutionsPage 仅保留 message），forbidden 分支未接上。 -->
    <AppStatePanel
      v-else-if="error"
      variant="error"
      class="q-mt-md"
      :description="error"
      action-label="重试"
      action-icon="refresh"
      @action="reload"
    />

    <AppStatePanel v-if="loading && executionHistory.length === 0 && !error" variant="loading" :rows="4" class="q-mt-md" />

    <section v-else-if="filteredHistory.length > 0" class="graph-executions-page__list q-mt-sm">
      <q-card
        v-for="exec in filteredHistory"
        :key="exec.executionId"
        flat
        class="graph-executions-page__card"
        @click="goToRun(exec.executionId)"
      >
        <q-card-section class="row items-center no-wrap">
          <div class="col min-width-0">
            <div class="graph-executions-page__card-id text-body2 text-weight-medium ellipsis">
              {{ exec.executionId.slice(0, 12) }}…
            </div>
            <div class="graph-executions-page__card-meta text-caption text-grey-7">
              {{ formatTime(exec.startedAt) }}
              <span v-if="exec.finishedAt"> → {{ formatTime(exec.finishedAt) }}</span>
              <span
                v-if="execDuration(exec.startedAt, exec.finishedAt)"
                class="graph-executions-page__duration q-ml-sm"
                >{{ execDuration(exec.startedAt, exec.finishedAt) }}</span
              >
            </div>
          </div>
          <AppStatusChip :status="exec.status" class="graph-executions-page__badge" />
        </q-card-section>
        <q-card-section v-if="exec.errorMessage" class="q-pt-none">
          <div class="text-caption text-negative ellipsis-2-lines">{{ exec.errorMessage }}</div>
        </q-card-section>
      </q-card>
    </section>

    <AppStatePanel
      v-else-if="!loading && !error && executionHistory.length === 0"
      variant="empty"
      title="暂无执行记录"
      icon="history"
      class="q-mt-md"
    />

    <AppStatePanel
      v-else-if="!loading && !error && filteredHistory.length === 0"
      variant="empty"
      title="当前筛选条件下无记录"
      icon="filter_list_off"
      class="q-mt-md"
    />

    <div v-if="hasNextPage" class="row justify-center q-py-lg">
      <q-btn flat rounded label="加载更多" :loading="loading" @click="loadMore" />
    </div>
  </q-page>
</template>

<script setup lang="ts">
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import { useGraphExecutionsPage } from '../features/graph/useGraphExecutionsPage';
import { formatTime, execDuration } from '../features/graph/utils';
import { STATUS_FILTER_OPTIONS, TIME_RANGE_OPTIONS } from '../features/graph/graphExecutionsUi';

const {
  isDark,
  graphName,
  executionHistory,
  filteredHistory,
  loading,
  error,
  hasNextPage,
  statusFilter,
  timeRangeFilter,
  reload,
  loadMore,
  goToRun,
  goBack,
} = useGraphExecutionsPage();
</script>
