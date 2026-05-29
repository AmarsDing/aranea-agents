<template>
  <q-page :class="['app-standard-page graph-executions-page', { 'is-dark': isDark }]">
    <AppPageHero kicker="执行历史" :title="graphName" subtitle="查看此 Graph 的所有执行记录。">
      <template #actions>
        <q-btn flat rounded icon="arrow_back" label="返回列表" @click="goBack" />
      </template>
    </AppPageHero>

    <div v-if="executionHistory.length > 0 || statusFilter || timeRangeFilter" class="graph-executions-page__filters q-mt-md row q-gutter-sm items-center">
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

    <q-inner-loading :showing="loading && executionHistory.length === 0" />

    <section v-if="filteredHistory.length > 0" class="graph-executions-page__list q-mt-sm">
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
              <span v-if="execDuration(exec.startedAt, exec.finishedAt)" class="graph-executions-page__duration q-ml-sm">{{ execDuration(exec.startedAt, exec.finishedAt) }}</span>
            </div>
          </div>
          <q-badge
            :color="statusColor(exec.status)"
            :label="statusLabel(exec.status)"
            class="graph-executions-page__badge"
          />
        </q-card-section>
        <q-card-section v-if="exec.errorMessage" class="q-pt-none">
          <div class="text-caption text-negative ellipsis-2-lines">{{ exec.errorMessage }}</div>
        </q-card-section>
      </q-card>
    </section>

    <q-card v-else-if="!loading && executionHistory.length === 0" flat class="graph-executions-page__empty q-mt-md">
      <q-card-section class="text-center text-grey-7">
        <q-icon name="history" size="48px" class="q-mb-sm" />
        <div>暂无执行记录</div>
      </q-card-section>
    </q-card>

    <q-card v-else-if="!loading && filteredHistory.length === 0" flat class="graph-executions-page__empty q-mt-md">
      <q-card-section class="text-center text-grey-7">
        <q-icon name="filter_list_off" size="48px" class="q-mb-sm" />
        <div>当前筛选条件下无记录</div>
      </q-card-section>
    </q-card>

    <div v-if="hasNextPage" class="row justify-center q-py-lg">
      <q-btn flat rounded label="加载更多" :loading="loading" @click="loadMore" />
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { useQuasar } from "quasar";
import { useGraphExecutionsPage } from "../features/graph/useGraphExecutionsPage";
import { formatTime, execDuration } from "../features/graph/utils";
import { STATUS_FILTER_OPTIONS, TIME_RANGE_OPTIONS, statusColor, statusLabel } from "../features/graph/graphExecutionsUi";

const $q = useQuasar();
const isDark = $q.dark.isActive;

const {
  graphName,
  executionHistory,
  filteredHistory,
  loading,
  hasNextPage,
  statusFilter,
  timeRangeFilter,
  loadMore,
  goToRun,
  goBack,
} = useGraphExecutionsPage();
</script>
