<template>
  <q-page :class="['app-standard-page graphs-page', { 'is-dark': isDark }]">
    <AppPageHero :kicker="t('graphs.kicker')" :title="t('graphs.title')" :subtitle="t('graphs.subtitle')">
      <template #actions>
        <q-btn
          outline
          rounded
          icon="dashboard"
          :label="t('graphs.createFromTemplate')"
          class="q-mr-sm"
          @click="templateDialogOpen = true"
        />
        <q-btn
          class="graphs-page__create-btn"
          rounded
          unelevated
          icon="add"
          :label="t('graphs.createGraph')"
          @click="openCreate"
        />
      </template>
    </AppPageHero>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mt-md">
      {{ error }}
      <template #action><q-btn flat color="white" :label="t('common.retry')" @click="loadRows" /></template>
    </q-banner>

    <div class="graphs-page__body">
      <div class="graphs-page__list">
        <section class="graphs-filter-bar q-mt-md">
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

        <div v-if="!loading && rows.length === 0" class="graphs-page__empty">
          <div class="graphs-page__empty-head column items-center q-pb-lg">
            <q-icon name="hub" size="48px" color="grey-6" class="q-mb-sm" />
            <div class="text-h6 text-grey-7">{{ t('graphs.emptyTitle') }}</div>
            <div class="text-body2 text-grey-6 q-mt-xs">{{ t('graphs.emptyHint') }}</div>
          </div>
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
          v-else
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
          <template #item="{ element: graph, index }">
            <q-card
              flat
              :class="[
                'graph-card',
                `graph-card--status-${cardStatus(graph)}`,
                { 'is-dark': isDark, 'graph-card--selected': selectedGraphId === graph.id },
              ]"
              :style="{ animationDelay: `${Math.min(index, 12) * 30}ms` }"
              @click="selectGraph(graph.id)"
              @contextmenu="onCardContextMenu($event, graph)"
            >
              <div class="graph-card__inner">
                <div class="row items-center justify-between no-wrap">
                  <h3 class="graph-card__name col min-width-0 ellipsis">{{ graph.name }}</h3>
                  <span :class="['graph-card__status', `graph-card__status--${cardStatus(graph)}`]">
                    <i class="graph-card__status-dot" />{{ t(GRAPH_CARD_STATUS_LABEL_KEYS[cardStatus(graph)]) }}
                  </span>
                </div>
                <div v-if="(graph.nodes?.length ?? 0) > 0" class="graph-card__chips">
                  <span
                    v-for="chip in compositionChips(graph).chips"
                    :key="chip.type"
                    class="graph-card__chip"
                    :title="t(NODE_TYPE_STYLES[chip.type as NodeType]?.labelKey ?? '')"
                  >
                    <i class="graph-card__chip-dot" :style="{ background: nodeTypeBorderColor(chip.type) }" />{{
                      chip.count
                    }}
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

      <GraphDetailPanel
        :graph="selectedGraph"
        :is-dark="isDark"
        :node-counts="selectedGraphNodeCounts"
        :node-type-border-color="nodeTypeBorderColor"
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

    <q-dialog v-model="templateDialogOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-center justify-between no-wrap">
          <div class="min-width-0">
            <div class="app-glass-dialog__title">{{ t('graphs.createFromTemplateTitle') }}</div>
            <div class="app-glass-dialog__subtitle">{{ t('graphs.createFromTemplateSubtitle') }}</div>
          </div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <q-card-section class="app-glass-dialog__body">
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
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat rounded no-caps :label="t('common.cancel')" />
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
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import draggable from 'vuedraggable';
import AppPageHero from '../components/layout/AppPageHero.vue';
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
  searchQuery,
  engineFilter,
  teamFilter,
  sortKey,
  sortOrder,
  SORT_OPTIONS,
  ENGINE_FILTER_OPTIONS,
  TEAM_FILTER_OPTIONS,
  isTeamOwned,
  teamDisplayName,
  nodeTypeBorderColor,
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
