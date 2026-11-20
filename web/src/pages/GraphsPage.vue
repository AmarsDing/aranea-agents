<template>
  <q-page :class="['app-standard-page graphs-page', { 'is-dark': isDark }]">
    <!-- 页名在顶栏面包屑里，这里不再单占一行页头：筛选在左、操作在右，一行放下 -->
    <section class="graphs-filter-bar" :aria-label="t('graphs.title')">
      <q-input
        v-model="searchQuery"
        dense
        outlined
        :placeholder="t('graphs.searchPlaceholder')"
        class="graphs-filter-bar__search"
      >
        <template #prepend><q-icon name="search" /></template>
        <template v-if="searchQuery" #append
          ><q-icon name="close" class="cursor-pointer" @click="searchQuery = ''"
        /></template>
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
        v-model="teamFilter"
        :options="TEAM_FILTER_OPTIONS"
        dense
        outlined
        emit-value
        map-options
        class="graphs-filter-bar__select"
        data-test="graphs-team-filter"
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
      <q-btn
        flat
        dense
        round
        :icon="sortOrder === 'asc' ? 'arrow_upward' : 'arrow_downward'"
        :aria-label="sortOrder === 'asc' ? t('graphs.sortAsc') : t('graphs.sortDesc')"
        @click="sortOrder = sortOrder === 'asc' ? 'desc' : 'asc'"
      >
        <q-tooltip>{{ sortOrder === 'asc' ? t('graphs.sortAsc') : t('graphs.sortDesc') }}</q-tooltip>
      </q-btn>
      <q-toggle
        v-model="showOrchestrated"
        dense
        :label="t('graphs.showOrchestrated')"
        data-test="graphs-show-orchestrated"
      >
        <q-tooltip>{{ t('graphs.showOrchestratedHint') }}</q-tooltip>
      </q-toggle>
      <q-space />
      <q-btn
        flat
        dense
        no-caps
        icon="dashboard"
        :label="t('graphs.createFromTemplate')"
        @click="templateDialogOpen = true"
      />
      <q-btn unelevated dense no-caps color="primary" icon="add" :label="t('graphs.createGraph')" @click="openCreate" />
    </section>

    <!-- 五态：已有列表但刷新失败 → 局部失败横幅，列表照常渲染 -->
    <AppStatePanel
      v-if="(error || forbidden) && rows.length > 0"
      variant="partial"
      dense
      class="q-mt-md"
      :description="error"
      :action-label="t('common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />

    <!-- 五态：无权限 → 只渲染 forbidden 面板，不提供重试 -->
    <AppStatePanel v-else-if="forbidden" variant="forbidden" class="q-mt-md" />

    <!-- 五态：加载失败且列表为空 → 只渲染可重试错误面板（不与空态并存）。 -->
    <AppStatePanel
      v-else-if="error"
      variant="error"
      class="q-mt-md"
      :description="error"
      :action-label="t('common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />

    <div class="graphs-page__body">
      <div class="graphs-page__list">
        <div
          v-if="!showOrchestrated && hiddenOrchestratedCount > 0"
          class="q-mt-sm row items-center no-wrap rounded-borders q-px-sm q-py-xs text-caption app-text-secondary graphs-page__hint-banner"
        >
          <q-icon name="visibility_off" size="16px" class="q-mr-xs" />
          <span>{{ t('graphs.hiddenOrchestrated', { count: hiddenOrchestratedCount }) }}</span>
          <q-btn
            flat
            dense
            no-caps
            rounded
            color="primary"
            :label="t('graphs.showOrchestratedAction')"
            @click="showOrchestrated = true"
          />
        </div>

        <!-- 五态：首屏加载无数据 → 骨架面板（不与空态并存） -->
        <AppStatePanel v-if="loading && rows.length === 0 && !error" variant="loading" :rows="4" class="q-mt-md" />

        <div v-else-if="!loading && !error && rows.length === 0" class="graphs-page__empty">
          <AppStatePanel
            variant="empty"
            :title="t('graphs.emptyTitle')"
            :description="t('graphs.emptyHint')"
            icon="hub"
          />
          <q-inner-loading :showing="templatesLoading" />
          <div v-if="!templatesLoading && templates.length > 0" class="graphs-page__templates-grid">
            <q-card
              v-for="tpl in templates"
              :key="tpl.id"
              flat
              class="template-example-card cursor-pointer"
              :loading="templateCreating"
              @click="quickCreateFromTemplate(tpl)"
            >
              <div class="template-example-card__inner">
                <div class="row items-center no-wrap">
                  <q-icon
                    :name="TEMPLATE_CATEGORY_ICON[tpl.category] || 'dashboard'"
                    size="20px"
                    color="primary"
                    class="q-mr-sm"
                  />
                  <div class="col min-width-0">
                    <div class="template-example-card__name">{{ tpl.name }}</div>
                  </div>
                </div>
                <div class="template-example-card__desc">{{ tpl.description }}</div>
                <div class="template-example-card__meta">
                  <span>{{ tpl.nodes?.length ?? 0 }} {{ t('graphs.nodesUnit') }}</span>
                  <span>{{ tpl.edges?.length ?? 0 }} {{ t('graphs.edgesUnit') }}</span>
                </div>
                <q-btn
                  flat
                  dense
                  no-caps
                  color="primary"
                  icon="add_circle_outline"
                  :label="t('graphs.useTemplate')"
                  class="template-example-card__action"
                  :loading="templateCreating"
                  @click.stop="quickCreateFromTemplate(tpl)"
                />
              </div>
            </q-card>
          </div>
        </div>

        <draggable
          v-else-if="rows.length > 0"
          v-model="localGraphs"
          item-key="id"
          tag="section"
          class="graphs-grid q-mt-md"
          ghost-class="graph-card--ghost"
          chosen-class="graph-card--chosen"
          drag-class="graph-card--dragging"
          :disabled="!isDefaultSort"
          @end="onDragEnd"
        >
          <template #item="{ element: graph }">
            <q-card
              flat
              :class="[
                'graph-card',
                `graph-card--status-${cardStatus(graph)}`,
                { 'is-dark': isDark, 'graph-card--selected': selectedGraphId === graph.id },
              ]"
              :title="t('graphs.cardOpenHint')"
              @click="selectGraph(graph.id)"
              @dblclick="openEditor(graph.id)"
              @contextmenu="onCardContextMenu($event, graph)"
            >
              <div class="graph-card__inner">
                <div class="row items-center justify-between no-wrap">
                  <h3 class="graph-card__name col min-width-0 ellipsis">{{ graph.name }}</h3>
                  <!-- 「就绪」对每张有节点的卡都一样，等于没说；只标还空着的草稿 -->
                  <span
                    v-if="cardStatus(graph) === 'draft'"
                    :class="['graph-card__status', `graph-card__status--${cardStatus(graph)}`]"
                  >
                    <i class="graph-card__status-dot" />{{ t(GRAPH_CARD_STATUS_LABEL_KEYS[cardStatus(graph)]) }}
                  </span>
                </div>
                <div v-if="(graph.nodes?.length ?? 0) > 0" class="graph-card__chips">
                  <span v-for="chip in compositionChips(graph).chips" :key="chip.type" class="graph-card__chip">
                    <i class="graph-card__chip-dot" :class="`graph-card__chip-dot--${chip.type}`" />{{
                      t(NODE_TYPE_STYLES[chip.type as NodeType]?.labelKey ?? '')
                    }}
                    {{ chip.count }}
                  </span>
                  <span v-if="compositionChips(graph).overflow > 0" class="graph-card__chip graph-card__chip--overflow"
                    >+{{ compositionChips(graph).overflow }}</span
                  >
                </div>
                <p :class="['graph-card__desc', { 'graph-card__desc--empty': !graph.description }]">
                  {{ graph.description || t('graphs.noDescription') }}
                </p>
                <div class="graph-card__meta">
                  <span
                    v-if="isTeamOwned(graph)"
                    class="graph-card__team-badge"
                    :title="t('graphs.teamOwnedBadgeTip')"
                    data-test="graph-team-badge"
                  >
                    <q-icon name="groups" size="11px" />{{ t('graphs.teamOwnedBadge') }} ·
                    {{ teamDisplayName(graph.teamId ?? '') }}
                  </span>
                  <span>v{{ graph.version || 0 }}</span>
                  <span>{{ relativeTime(graph.updatedAt) }}</span>
                  <span>{{ graph.executionEngine === 'dag' ? t('graphs.cardDAG') : t('graphs.cardBSP') }}</span>
                  <span v-if="graph.enableCheckpoint">{{ t('graphs.checkpoint') }}</span>
                  <span class="graph-card__meta-total"
                    >{{ graph.nodes?.length ?? 0 }}{{ t('graphs.nodesUnit') }}
                    {{ (graph.edges?.length ?? 0) + (graph.conditionalEdges?.length ?? 0)
                    }}{{ t('graphs.edgesUnit') }}</span
                  >
                </div>
              </div>
            </q-card>
          </template>
          <template #footer>
            <div class="graph-card graph-card--add" @click="openCreate">
              <div class="graph-card__inner column items-center justify-center">
                <q-icon name="add" size="28px" color="grey-6" />
                <span class="graph-card--add__label">{{ t('graphs.createGraph') }}</span>
              </div>
            </div>
          </template>
        </draggable>

        <div v-if="hasNextPage" class="row justify-center q-py-md">
          <q-btn
            flat
            rounded
            no-caps
            icon="expand_more"
            :label="t('graphs.loadMore')"
            :loading="loadingMore"
            @click="loadMore"
          />
        </div>
      </div>

      <!-- 只在选中卡片时出现，不再常驻一块「点卡片看详情」的占位 -->
      <GraphDetailPanel
        v-if="selectedGraph"
        :graph="selectedGraph"
        :is-dark="isDark"
        :node-counts="selectedGraphNodeCounts"
        :executions="selectedExecutions"
        :executions-has-more="selectedExecutionsHasMore"
        @close="selectGraph('')"
        @edit="openEditor"
        @run="openRunDialog"
        @duplicate="duplicateGraph"
        @export="exportGraphJson"
        @delete="confirmRemoveGraph"
        @locate-node="locateNodeInEditor"
        @manage-schema="manageSchemaInEditor"
        @view-executions="viewSelectedExecutions"
      />
    </div>

    <GraphCardContextMenu
      :visible="ctxMenuVisible"
      :x="ctxMenuX"
      :y="ctxMenuY"
      :items="ctxMenuItems"
      @select="onCtxMenuAction"
      @close="closeCtxMenu"
    />

    <GraphRunDialog
      v-model="runDialogOpen"
      v-model:session-id="runSessionId"
      v-model:initial-state="runInitialState"
      :graph-name="runDialogGraph?.name"
      :loading="runLoading"
      @submit="executeRun"
    />

    <AppDialog
      v-model="templateDialogOpen"
      persistent
      size="md"
      :title="t('graphs.createFromTemplateTitle')"
      :subtitle="t('graphs.createFromTemplateSubtitle')"
    >
      <q-inner-loading :showing="templatesLoading" />
      <div v-if="!templatesLoading && templates.length === 0" class="text-center text-grey-7 q-pa-md">
        {{ t('graphs.noTemplates') }}
      </div>
      <div v-else class="q-gutter-sm">
        <q-card
          v-for="tpl in templates"
          :key="tpl.id"
          flat
          bordered
          class="cursor-pointer template-card"
          :class="{ 'template-card--selected': selectedTemplateId === tpl.id }"
          @click="selectedTemplateId = tpl.id"
        >
          <q-card-section class="row items-center no-wrap">
            <q-icon name="dashboard" size="24px" color="primary" class="q-mr-md" />
            <div class="col min-width-0">
              <div class="text-subtitle2">{{ tpl.name }}</div>
              <div class="text-caption text-grey-7 ellipsis">
                {{ tpl.description || t('graphs.noDescription') }}
              </div>
            </div>
          </q-card-section>
        </q-card>
      </div>
      <template #actions>
        <q-btn flat rounded no-caps :label="t('common.cancel')" @click="templateDialogOpen = false" />
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          :label="t('common.create')"
          :disable="!selectedTemplateId"
          :loading="templateCreating"
          @click="createFromTemplate"
        />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import draggable from 'vuedraggable';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import GraphRunDialog from '../components/graph/GraphRunDialog.vue';
import GraphDetailPanel from '../components/graph/GraphDetailPanel.vue';
import GraphCardContextMenu from '../components/graph/GraphCardContextMenu.vue';
import { useGraphsPage } from '../features/graph/useGraphsPage';
import type { GraphDefinition, NodeType } from '../features/graph/types';
import { NODE_TYPE_STYLES } from '../features/graph/types';
import { GRAPH_CARD_STATUS_LABEL_KEYS } from '../features/graph/utils';

const { t } = useI18n();

const {
  isDark,
  rows,
  filteredRows,
  loading,
  loadingMore,
  hasNextPage,
  error,
  forbidden,
  searchQuery,
  engineFilter,
  teamFilter,
  showOrchestrated,
  hiddenOrchestratedCount,
  sortKey,
  sortOrder,
  SORT_OPTIONS,
  ENGINE_FILTER_OPTIONS,
  TEAM_FILTER_OPTIONS,
  isTeamOwned,
  teamDisplayName,
  countNodesByType,
  cardStatus,
  compositionChips,
  relativeTime,
  runDialogOpen,
  runDialogGraph,
  runSessionId,
  runInitialState,
  runLoading,
  selectedGraphId,
  selectedGraph,
  selectedExecutions,
  selectedExecutionsHasMore,
  exportGraphJson,
  locateNodeInEditor,
  manageSchemaInEditor,
  viewSelectedExecutions,
  ctxMenuVisible,
  ctxMenuX,
  ctxMenuY,
  ctxMenuItems,
  loadRows,
  loadMore,
  openCreate,
  openEditor,
  openRunDialog,
  executeRun,
  duplicateGraph,
  confirmRemoveGraph,
  reorderGraphs,
  selectGraph,
  onCardContextMenu,
  closeCtxMenu,
  onCtxMenuAction,
  templateDialogOpen,
  selectedTemplateId,
  templateCreating,
  templatesLoading,
  templates,
  createFromTemplate,
  quickCreateFromTemplate,
  loadTemplates,
} = useGraphsPage();

const isDefaultSort = computed(() => sortKey.value === 'updatedAt' && sortOrder.value === 'desc');

const localGraphs = ref<GraphDefinition[]>([]);

watch(
  filteredRows,
  (val) => {
    localGraphs.value = val.slice();
  },
  { immediate: true },
);

// 空状态时自动加载模板
const isEmpty = computed(() => !loading.value && rows.value.length === 0);
watch(
  isEmpty,
  (empty) => {
    if (empty && templates.value.length === 0) {
      void loadTemplates();
    }
  },
  { immediate: true },
);

const TEMPLATE_CATEGORY_ICON: Record<string, string> = {
  pipeline: 'linear_scale',
  approval: 'approval',
  parallel: 'call_split',
  loop: 'loop',
  dispatch: 'alt_route',
  nested: 'account_tree',
};

function onDragEnd() {
  const ids = localGraphs.value.map((g) => g.id);
  reorderGraphs(ids);
}

const selectedGraphNodeCounts = computed(() => {
  if (!selectedGraph.value) return {};
  return countNodesByType(selectedGraph.value);
});
</script>
