<template>
  <q-page :class="['app-standard-page graphs-page', { 'is-dark': isDark }]">
    <AppPageHero
      kicker="Graph 工作流"
      title="Graph 管理"
      subtitle="可视化构建可观测、可干预、可回溯的确定性工作流，支持条件路由、人工审批和状态回溯。"
    >
      <template #actions>
        <q-btn class="graphs-page__create-btn" rounded unelevated icon="add" label="新增 Graph" @click="openCreate" />
      </template>
    </AppPageHero>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mt-md">
      {{ error }}
      <template #action><q-btn flat color="white" label="重试" @click="loadRows" /></template>
    </q-banner>

    <section class="graphs-filter-bar q-mt-md">
      <q-input
        v-model="searchQuery"
        dense
        outlined
        placeholder="搜索 Graph..."
        class="graphs-filter-bar__search"
      >
        <template #prepend><q-icon name="search" /></template>
        <template v-if="searchQuery" #append><q-icon name="close" class="cursor-pointer" @click="searchQuery = ''" /></template>
      </q-input>
      <q-select
        v-model="engineFilter"
        :options="ENGINE_FILTER_OPTIONS"
        dense
        outlined
        emit-value
        map-options
        class="graphs-filter-bar__select"
      />
      <q-select
        v-model="sortKey"
        :options="SORT_OPTIONS"
        dense
        outlined
        emit-value
        map-options
        class="graphs-filter-bar__select"
      />
      <q-btn-toggle
        v-model="sortOrder"
        no-caps
        flat
        dense
        toggle-color="primary"
        :options="[
          { value: 'asc', icon: 'arrow_upward' },
          { value: 'desc', icon: 'arrow_downward' },
        ]"
      />
    </section>

    <q-inner-loading :showing="loading && rows.length === 0" />

    <section class="graphs-grid q-mt-md">
      <q-card
        v-for="graph in filteredRows"
        :key="graph.id"
        flat
        :class="['graph-card', { 'is-dark': isDark }]"
        @click="openEditor(graph.id)"
      >
        <div class="graph-card__inner">
          <div class="row items-center justify-between no-wrap">
            <h3 class="graph-card__name col min-width-0 ellipsis">{{ graph.name }}</h3>
            <span class="graph-card__time">{{ relativeTime(graph.updatedAt) }}</span>
          </div>
          <p v-if="graph.description" class="graph-card__desc ellipsis">{{ graph.description }}</p>
          <div class="graph-card__chips">
            <template v-for="(count, type) in countNodesByType(graph)" :key="type">
              <span
                v-if="count"
                class="graph-card__chip"
                :style="{ borderColor: nodeTypeBorderColor(type as string), color: nodeTypeBorderColor(type as string) }"
              >{{ (NODE_TYPE_EMOJI as any)[type] }}×{{ count }}</span>
            </template>
          </div>
          <div class="row items-center justify-between no-wrap">
            <div class="graph-card__tags">
              <span v-if="graph.executionEngine === 'dag'" class="graph-card__tag">DAG</span>
              <span v-else class="graph-card__tag">BSP</span>
              <span v-if="graph.enableCheckpoint" class="graph-card__tag">检查点</span>
            </div>
            <span class="graph-card__summary">{{ graph.nodes?.length ?? 0 }}节点·{{ graph.edges?.length ?? 0 }}线</span>
          </div>
          <footer class="graph-card__foot">
            <q-btn flat dense round icon="play_arrow" size="sm" @click.stop="openRunDialog(graph)">
              <q-tooltip>执行</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="edit" size="sm" @click.stop="openEditor(graph.id)">
              <q-tooltip>编辑</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="content_copy" size="sm" @click.stop="duplicateGraph(graph)">
              <q-tooltip>复制</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="delete" size="sm" color="negative" @click.stop="confirmRemoveGraph(graph)">
              <q-tooltip>删除</q-tooltip>
            </q-btn>
          </footer>
        </div>
      </q-card>

      <div class="graph-card graph-card--add" @click="openCreate">
        <div class="graph-card__inner column items-center justify-center">
          <q-icon name="add" size="28px" color="grey-6" />
          <span class="graph-card--add__label">新增 Graph</span>
        </div>
      </div>
    </section>

    <q-card v-if="!loading && rows.length === 0" flat :class="['graphs-empty', { 'is-dark': isDark }, 'q-mt-lg']">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="account_tree" />
        <div class="text-h6 q-mt-md">暂无 Graph</div>
        <div class="text-body2 app-text-secondary q-mt-sm">创建一个 Graph 工作流，可视化编排 Agent、条件路由和并行分支。</div>
        <q-btn class="q-mt-md graphs-page__create-btn" rounded unelevated icon="add" label="新增 Graph" @click="openCreate" />
      </q-card-section>
    </q-card>

    <GraphRunDialog
      v-model="runDialogOpen"
      v-model:session-id="runSessionId"
      v-model:initial-state="runInitialState"
      :graph-name="runDialogGraph?.name"
      :loading="runLoading"
      @submit="executeRun"
    />
  </q-page>
</template>

<script setup lang="ts">
import AppPageHero from "../components/layout/AppPageHero.vue";
import GraphRunDialog from "../components/graph/GraphRunDialog.vue";
import { useGraphsPage } from "../features/graph/useGraphsPage";

const {
  isDark,
  rows,
  filteredRows,
  loading,
  error,
  searchQuery,
  engineFilter,
  sortKey,
  sortOrder,
  SORT_OPTIONS,
  ENGINE_FILTER_OPTIONS,
  NODE_TYPE_EMOJI,
  nodeTypeBorderColor,
  countNodesByType,
  relativeTime,
  runDialogOpen,
  runDialogGraph,
  runSessionId,
  runInitialState,
  runLoading,
  loadRows,
  openCreate,
  openEditor,
  openRunDialog,
  executeRun,
  duplicateGraph,
  confirmRemoveGraph,
} = useGraphsPage();
</script>
