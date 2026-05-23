<template>
  <q-page :class="['graphs-page', { 'is-dark': isDark }]">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Graph 工作流</div>
        <h1 class="app-page-title">Graph 管理</h1>
        <p class="app-page-subtitle">可视化构建可观测、可干预、可回溯的确定性工作流，支持条件路由、人工审批和状态回溯。</p>
      </div>
      <q-btn class="graphs-page__create-btn" rounded unelevated icon="add" label="新增 Graph" @click="openCreate" />
    </section>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mt-md">
      {{ error }}
      <template #action><q-btn flat color="white" label="重试" @click="loadRows" /></template>
    </q-banner>

    <section class="graphs-grid q-mt-lg">
      <q-card
        v-for="graph in rows"
        :key="graph.id"
        flat
        :class="['graph-card', { 'is-dark': isDark }]"
        @click="openEditor(graph.id)"
      >
        <div class="graph-card__inner">
          <div class="row items-start justify-between no-wrap">
            <h3 class="graph-card__name col min-width-0 ellipsis">{{ graph.name }}</h3>
            <q-icon name="account_tree" size="18px" class="text-grey-6" />
          </div>
          <p v-if="graph.description" class="graph-card__desc ellipsis-2-lines">{{ graph.description }}</p>
          <div class="graph-card__tags">
            <span class="graph-card__tag">{{ graph.nodes?.length ?? 0 }} 节点</span>
            <span class="graph-card__tag">{{ graph.edges?.length ?? 0 }} 连线</span>
            <span v-if="graph.executionEngine === 'dag'" class="graph-card__tag">DAG</span>
            <span v-if="graph.enableCheckpoint" class="graph-card__tag">检查点</span>
          </div>
          <footer class="graph-card__foot">
            <q-btn flat dense round icon="play_arrow" @click.stop="openRunDialog(graph)">
              <q-tooltip>执行</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="content_copy" @click.stop="duplicateGraph(graph)">
              <q-tooltip>复制</q-tooltip>
            </q-btn>
            <q-btn flat dense round icon="delete" color="negative" @click.stop="removeGraph(graph)">
              <q-tooltip>删除</q-tooltip>
            </q-btn>
          </footer>
        </div>
      </q-card>
    </section>

    <q-card v-if="!loading && rows.length === 0" flat :class="['graphs-empty', { 'is-dark': isDark }, 'q-mt-lg']">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="account_tree" />
        <div class="text-h6 q-mt-md">暂无 Graph</div>
        <div class="text-body2 app-text-secondary q-mt-sm">创建一个 Graph 工作流，可视化编排 Agent、条件路由和并行分支。</div>
        <q-btn class="q-mt-md graphs-page__create-btn" rounded unelevated icon="add" label="新增 Graph" @click="openCreate" />
      </q-card-section>
    </q-card>

    <q-dialog v-model="runDialogOpen" persistent>
      <q-card :class="['graph-run-dialog app-dialog-card app-dialog-card--sm app-glass-dialog', { 'is-dark': isDark }]">
        <q-card-section class="app-glass-dialog__head">
          <div class="app-glass-dialog__title">执行 Graph</div>
          <div class="app-glass-dialog__subtitle">为 {{ runDialogGraph?.name }} 启动一次执行</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <q-input v-model="runSessionId" class="app-field-md" dense outlined label="Session ID" hint="关联的会话 ID" />
          <q-input v-model="runInitialState" class="app-field-long" dense outlined autogrow type="textarea" label="初始状态 (JSON)" hint="可选，JSON 格式的初始状态" />
        </q-card-section>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn flat rounded label="取消" @click="runDialogOpen = false" />
          <q-btn class="graphs-page__create-btn" rounded unelevated label="执行" :loading="runLoading" @click="executeRun" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { useGraphsPage } from "../features/graph/useGraphsPage";

const {
  isDark,
  rows,
  loading,
  error,
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
  removeGraph
} = useGraphsPage();
</script>

<style scoped>
.graphs-page__create-btn {
  background: var(--color-accent);
  color: #fff;
}

.graphs-page__create-btn:hover {
  background: var(--color-accent-hover);
}

.min-width-0 {
  min-width: 0;
}

.ellipsis-2-lines {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
</style>
