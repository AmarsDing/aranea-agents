<template>
  <q-page :class="['graphs-page', { 'is-dark': isDark }]">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Graph 工作流</div>
        <h1 class="app-page-title">Graph 管理</h1>
        <p class="app-page-subtitle">可视化构建可观测、可干预、可回溯的确定性工作流，支持条件路由、人工审批和状态回溯。</p>
      </div>
      <q-btn color="primary" rounded unelevated icon="add" label="新增 Graph" @click="openCreate" />
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
        bordered
        :class="['graph-card', { 'is-dark': isDark }]"
        @click="openEditor(graph.id)"
      >
        <q-card-section>
          <div class="row items-center justify-between">
            <div class="text-h6 graph-card__name">{{ graph.name }}</div>
            <q-icon name="account_tree" size="20px" color="primary" />
          </div>
          <div v-if="graph.description" class="text-caption app-text-secondary q-mt-xs">{{ graph.description }}</div>
          <div class="row q-gutter-sm q-mt-sm">
            <q-badge rounded color="blue-grey">{{ graph.nodes?.length ?? 0 }} 节点</q-badge>
            <q-badge rounded color="blue-grey">{{ graph.edges?.length ?? 0 }} 连线</q-badge>
            <q-badge v-if="graph.executionEngine === 'dag'" rounded color="purple">DAG</q-badge>
            <q-badge v-if="graph.enableCheckpoint" rounded color="teal">检查点</q-badge>
          </div>
        </q-card-section>
        <q-separator />
        <q-card-actions align="right">
          <q-btn flat dense round icon="play_arrow" color="primary" @click.stop="openRunDialog(graph)">
            <q-tooltip>执行</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="content_copy" @click.stop="duplicateGraph(graph)">
            <q-tooltip>复制</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="delete" color="negative" @click.stop="removeGraph(graph)">
            <q-tooltip>删除</q-tooltip>
          </q-btn>
        </q-card-actions>
      </q-card>
    </section>

    <q-card v-if="!loading && rows.length === 0" flat bordered :class="['graphs-empty', { 'is-dark': isDark }, 'q-mt-lg']">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="account_tree" />
        <div class="text-h6 q-mt-md">暂无 Graph</div>
        <div class="text-body2 app-text-secondary q-mt-sm">创建一个 Graph 工作流，可视化编排 Agent、条件路由和并行分支。</div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="新增 Graph" @click="openCreate" />
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
          <q-btn color="primary" rounded unelevated label="执行" :loading="runLoading" @click="executeRun" />
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
.graphs-page {
  min-height: 100%;
  padding: 28px;
  background:
    radial-gradient(circle at 86% 0%, rgb(25 118 210 / 12%), transparent 28%),
    linear-gradient(180deg, var(--color-page-tint) 0%, var(--color-page-tint-alt) 46%, var(--color-on-accent) 100%);
}

.graphs-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 18px;
}

.graph-card {
  cursor: pointer;
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 20px;
  background: var(--glass-surface, rgb(255 253 245 / 65%));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
  transition: transform 0.15s;
}

.graph-card:hover {
  transform: translateY(-2px);
}

.graph-card__name {
  font-weight: 700;
}

.graphs-empty {
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 24px;
  background: rgb(255 255 255 / 86%);
  backdrop-filter: blur(16px);
}

.graphs-page.is-dark {
  background:
    radial-gradient(circle at 86% 0%, rgb(59 130 246 / 16%), transparent 30%),
    linear-gradient(160deg, var(--canvas-base) 0%, var(--color-surface-elevated) 48%, var(--color-surface-solid) 100%);
  color: var(--color-border-soft);
}

.graph-card.is-dark {
  border-color: rgb(148 163 184 / 16%);
  background: rgb(17 24 39 / 90%);
}

.graphs-empty.is-dark {
  border-color: rgb(148 163 184 / 16%);
  background: rgb(17 24 39 / 90%);
}

@media (width <= 599px) {
  .graphs-page {
    padding: 18px;
  }

  .graphs-grid {
    grid-template-columns: 1fr;
  }
}
</style>
