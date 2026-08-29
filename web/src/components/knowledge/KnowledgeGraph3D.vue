<template>
  <!-- G5 深空图谱（V12.8）：左自研 3D 画布 + 右 HUD 操作台（.kg-hud 作用域皮肤）。 -->
  <!-- SP2-8：fullscreen 覆盖模式（v-model:fullscreen；ESC / 关闭按钮退出）。 -->
  <div class="knowledge-graph kg-hud" :class="{ 'knowledge-graph--fullscreen': fullscreen }">
    <!-- 左：3D 画布区 -->
    <q-card flat class="app-pane-card knowledge-graph__stage">
      <div v-if="loading" class="knowledge-graph__overlay">
        <q-spinner-dots color="primary" size="32px" />
        <span>{{ t('knowledgePage.graphLoading') }}</span>
      </div>
      <div v-else-if="error" class="knowledge-graph__overlay knowledge-graph__overlay--error">
        <q-icon name="cloud_off" size="20px" />
        <span>{{ error }}</span>
      </div>
      <div v-else-if="!legendNodes.length" class="knowledge-graph__overlay">
        <q-icon name="hub" size="20px" />
        <span>{{ t('knowledgePage.graphEmpty') }}</span>
      </div>
      <knowledge-graph-canvas
        v-show="nodes.length && !error"
        ref="canvasRef"
        :nodes="nodes"
        :edges="edges"
        :selected-node-id="selectedNodeId"
        :focus-signal="focusSignal"
        :generation="generation"
        :auto-rotate="autoRotate"
        :show-labels="showLabels"
        :layout="layout"
        :color-mode="colorMode"
        @node-click="$emit('select-node', $event)"
        @background-click="$emit('select-node', '')"
        @node-dblclick="(p: { docId: string; relPath: string }) => $emit('open-in-explorer', p)"
        @focus-change="(id: string) => (focusedDocId = id)"
        @focus-group="$emit('focus-group', $event)"
        @hover-change="(id: string) => (hoveredDocId = id)"
      />
      <!-- M4：聚焦节点信息卡（真折射玻璃浮层，画布右侧） -->
      <FocusCard
        v-if="focusedNode"
        :node="focusedNode"
        :can-reembed="canReembed"
        @open-in-explorer="(p: { docId: string; relPath: string }) => $emit('open-in-explorer', p)"
        @reembed="(docId: string) => $emit('reembed-node', docId)"
        @close="onFocusClose"
      />
      <!-- M5：过滤图例（点击隐藏组 / 悬停透镜），HUD 左下（统计行上方）。键定未过滤 legendNodes：全组隐藏时仍保留恢复路径。 -->
      <GraphLegend
        v-if="legendNodes.length && !error"
        class="knowledge-graph__legend-panel"
        :groups="legendGroups"
        :hidden-groups="hiddenGroups"
        @toggle-group="(d: string) => $emit('toggle-group', d)"
        @lens-enter="(d: string) => canvasRef?.setLens(d)"
        @lens-leave="canvasRef?.setLens(null)"
      />
      <!-- D5：左上 SYS 状态灯（SYNCING 琥珀闪烁 / ONLINE 青 / OFFLINE 红） -->
      <div class="knowledge-graph__sys">
        <span class="knowledge-graph__sys-label">{{ t('knowledgePage.graphSysStatus') }}</span>
        <span class="knowledge-graph__sys-dot" :class="`knowledge-graph__sys-dot--${sysState}`" />
        <span class="knowledge-graph__sys-value" :class="`knowledge-graph__sys-value--${sysState}`">{{
          sysLabel
        }}</span>
        <!-- 邻域模式：返回全局（原右上工具条迁入） -->
        <button
          v-if="neighborhoodHops > 0"
          class="kg-hud__switch kg-hud__switch--accent knowledge-graph__sys-back"
          @click="$emit('reset-global-view')"
        >
          {{ t('knowledgePage.graphBackToGlobal') }}
        </button>
      </div>
      <!-- D5：顶部居中括号大标题 + 副标（点击=库切换下拉） -->
      <div class="knowledge-graph__title">
        <div class="knowledge-graph__title-main">[ {{ t('knowledgePage.graphTitle') }} ]</div>
        <q-btn-dropdown
          flat
          dense
          no-caps
          class="knowledge-graph__title-sub"
          :label="t('knowledgePage.graphSubtitle', { name: currentCollectionName })"
        >
          <q-list dense class="knowledge-graph__title-menu">
            <q-item
              v-for="opt in collectionOptions"
              :key="opt.value"
              v-close-popup
              clickable
              @click="$emit('select-collection', opt.value)"
            >
              <q-item-section>
                <q-item-label class="row items-center no-wrap q-gutter-xs">
                  <span class="ellipsis">{{ opt.label }}</span>
                  <span v-if="opt.backend === 'team'" class="knowledge-graph__team-badge">
                    {{ t('knowledgePage.vaultTeamBadge') }}
                  </span>
                </q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
        </q-btn-dropdown>
      </div>
      <!-- D5：左中竖排图标胶囊（替代原右上横排文字工具条） -->
      <hud-toolbar
        v-if="nodes.length && !error"
        class="knowledge-graph__hud-toolbar"
        :layout="layout"
        :color-mode="colorMode"
        :auto-rotate="autoRotate"
        :show-labels="showLabels"
        :fullscreen="!!fullscreen"
        :node-legend="nodeLegend"
        :link-types="graphLinkTypes"
        @fit="fitView"
        @toggle-layout="toggleLayout"
        @toggle-color-mode="toggleColorMode"
        @toggle-auto-rotate="$emit('update:auto-rotate', !autoRotate)"
        @toggle-labels="$emit('update:show-labels', !showLabels)"
        @toggle-fullscreen="$emit('update:fullscreen', !fullscreen)"
      />
      <!-- D5：左下按键提示（随 hover/聚焦上下文变化） -->
      <div v-if="nodes.length && !error" class="knowledge-graph__hints">
        <span class="knowledge-graph__hint">{{ t('knowledgePage.graphHintRotate') }}</span>
        <span class="knowledge-graph__hint">{{ t('knowledgePage.graphHintPan') }}</span>
        <span class="knowledge-graph__hint">{{ t('knowledgePage.graphHintZoom') }}</span>
        <template v-if="hoveredDocId && !focusedDocId">
          <span class="knowledge-graph__hint knowledge-graph__hint--ctx">{{ t('knowledgePage.graphHintFocus') }}</span>
          <span class="knowledge-graph__hint knowledge-graph__hint--ctx">{{ t('knowledgePage.graphHintOpen') }}</span>
        </template>
        <span v-if="focusedDocId" class="knowledge-graph__hint knowledge-graph__hint--ctx">
          {{ t('knowledgePage.graphHintExitFocus') }}
        </span>
      </div>
      <div class="knowledge-graph__stats">
        {{ t('knowledgePage.graphStats', { nodes: totalNodes, edges: totalEdges }) }}
        <span v-if="hiddenIsolated" class="knowledge-graph__stats-hidden">
          · {{ t('knowledgePage.graphIsolatedHidden', { count: hiddenIsolated }) }}
        </span>
        <span v-if="neighborhoodHops > 0" class="knowledge-graph__stats-hood">
          · {{ t('knowledgePage.graphNeighborhoodHint', { name: neighborhoodRootName, hops: neighborhoodHops }) }}
        </span>
      </div>
    </q-card>

    <!-- 右：操作台 -->
    <q-card flat class="app-pane-card knowledge-graph__console">
      <q-card-section class="knowledge-graph__console-body">
        <!-- 库选择（SP1-I：team 库选项带「团队」徽标） -->
        <q-select
          :model-value="collectionId"
          :options="collectionOptions"
          emit-value
          map-options
          dense
          outlined
          :label="t('knowledgePage.graphVaultLabel')"
          @update:model-value="(v: string) => $emit('select-collection', v)"
        >
          <template #option="scope">
            <q-item v-bind="scope.itemProps">
              <q-item-section>
                <q-item-label class="row items-center no-wrap q-gutter-xs">
                  <span class="ellipsis">{{ scope.opt.label }}</span>
                  <span v-if="scope.opt.backend === 'team'" class="knowledge-graph__team-badge">
                    {{ t('knowledgePage.vaultTeamBadge') }}
                  </span>
                </q-item-label>
              </q-item-section>
            </q-item>
          </template>
        </q-select>

        <!-- 边类型过滤 chips -->
        <div class="knowledge-graph__section">
          <div class="knowledge-graph__section-title">{{ t('knowledgePage.graphLinkTypes') }}</div>
          <div class="knowledge-graph__chips">
            <q-chip
              v-for="lt in graphLinkTypes"
              :key="lt"
              dense
              clickable
              :outline="!linkTypes.includes(lt)"
              :class="{ 'knowledge-graph__chip--off': !linkTypes.includes(lt) }"
              @click="$emit('toggle-link-type', lt)"
            >
              <span
                class="knowledge-graph__chip-dot"
                :style="{ background: graphLinkColor(lt), color: graphLinkColor(lt) }"
              />
              {{ t(`knowledgePage.linkType${lt.charAt(0).toUpperCase() + lt.slice(1)}`) }}
            </q-chip>
          </div>
        </div>

        <!-- 目录前缀过滤（复用 V12.6 范围选择器）+ 孤立节点开关 -->
        <div class="knowledge-graph__section knowledge-graph__section--row">
          <knowledge-scope-picker
            :scope-prefix="pathPrefix"
            :scope-nodes="scopeNodes"
            @update:scope-prefix="$emit('set-path-prefix', $event)"
            @scope-lazy-load="$emit('scope-lazy-load', $event)"
          />
          <q-toggle
            :model-value="showIsolated"
            dense
            size="sm"
            :label="t('knowledgePage.graphShowIsolated')"
            @update:model-value="(v: boolean) => $emit('update:show-isolated', v)"
          />
        </div>

        <!-- 局部图谱（G5-D D-5）：聚焦邻域跳数步进 + 返回全局 -->
        <div class="knowledge-graph__section">
          <div class="knowledge-graph__section-title">{{ t('knowledgePage.graphFocusNeighborhood') }}</div>
          <div class="kg-hud__hood">
            <div class="kg-hud__hops" role="group" :aria-label="t('knowledgePage.graphNeighborhoodHops')">
              <button
                v-for="h in [1, 2, 3, 4]"
                :key="h"
                class="kg-hud__hop"
                :class="{ 'kg-hud__hop--active': hopsDraft === h }"
                @click="setHops(h)"
              >
                {{ h }}
              </button>
            </div>
            <button
              v-if="neighborhoodHops === 0"
              class="kg-hud__switch kg-hud__switch--accent"
              :disabled="!selectedNode"
              :title="selectedNode ? '' : t('knowledgePage.graphFocusNeedSelection')"
              @click="$emit('focus-neighborhood', hopsDraft)"
            >
              {{ t('knowledgePage.graphFocusNeighborhood') }}
            </button>
            <button v-else class="kg-hud__switch" @click="$emit('reset-global-view')">
              {{ t('knowledgePage.graphBackToGlobal') }}
            </button>
          </div>
        </div>

        <!-- 实体治理（G5-G G-1）：合并建议列表 + 一键合并 + 重写条数内联反馈 -->
        <div class="knowledge-graph__section">
          <div class="knowledge-graph__section-title">{{ t('knowledgePage.graphEntityGovernance') }}</div>
          <q-list v-if="mergeSuggestions.length" dense class="knowledge-graph__merge-list">
            <q-item v-for="sg in mergeSuggestions" :key="`${sg.keeper_id}-${sg.mergee_id}`">
              <q-item-section>
                <q-item-label lines="1" class="knowledge-graph__merge-names">
                  {{ sg.keeper_name }} <span class="knowledge-graph__merge-arrow">←</span> {{ sg.mergee_name }}
                </q-item-label>
                <q-item-label caption class="knowledge-graph__merge-meta">
                  <span class="knowledge-graph__merge-badge" :class="`knowledge-graph__merge-badge--${sg.source}`">
                    {{ t(`knowledgePage.mergeSource.${sg.source}`) }}
                  </span>
                  <span v-if="sg.source === 'embedding'">{{ sg.similarity.toFixed(2) }}</span>
                </q-item-label>
              </q-item-section>
              <q-item-section side>
                <button
                  class="kg-hud__switch kg-hud__switch--accent"
                  :disabled="merging"
                  @click="$emit('merge-entities', { keeperId: sg.keeper_id, mergeeId: sg.mergee_id })"
                >
                  {{ t('knowledgePage.mergeAction') }}
                </button>
              </q-item-section>
            </q-item>
          </q-list>
          <div v-else class="knowledge-graph__nodes-empty">{{ t('knowledgePage.mergeNoSuggestions') }}</div>
          <div v-if="lastMergeResult" class="knowledge-graph__merge-feedback">
            {{
              t('knowledgePage.mergeFeedback', {
                mentions: lastMergeResult.rewritten_mentions,
                links: lastMergeResult.rewritten_links,
              })
            }}
          </div>
        </div>

        <!-- 节点搜索定位 -->
        <q-input
          :model-value="nodeQuery"
          dense
          outlined
          clearable
          :placeholder="t('knowledgePage.graphNodeSearchPlaceholder')"
          @update:model-value="(v: string | number | null) => $emit('update:node-query', v == null ? '' : String(v))"
        >
          <template #prepend><q-icon name="travel_explore" size="16px" /></template>
        </q-input>

        <!-- 节点列表（连接度排序，点击聚焦） -->
        <div class="knowledge-graph__section knowledge-graph__section--grow">
          <div class="knowledge-graph__section-title">{{ t('knowledgePage.graphNodeList') }}</div>
          <q-list v-if="nodeList.length" dense class="knowledge-graph__node-list">
            <q-item
              v-for="n in nodeList"
              :key="n.doc_id"
              clickable
              :active="n.doc_id === selectedNodeId"
              active-class="knowledge-graph__node--active"
              @click="$emit('focus-node', n.doc_id)"
            >
              <q-item-section>
                <q-item-label lines="1">{{ n.name }}</q-item-label>
                <q-item-label caption lines="1">{{ n.rel_path || n.name }}</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-badge
                  outline
                  color="accent"
                  :label="n.degree"
                  :title="t('knowledgePage.graphSelectedDegree', { count: n.degree })"
                />
              </q-item-section>
            </q-item>
          </q-list>
          <div v-else class="knowledge-graph__nodes-empty">{{ t('knowledgePage.graphNodesEmpty') }}</div>
        </div>

        <!-- 选中节点卡 -->
        <div class="knowledge-graph__selected">
          <template v-if="selectedNode">
            <div class="knowledge-graph__selected-name" :title="selectedNode.name">{{ selectedNode.name }}</div>
            <div class="knowledge-graph__selected-path" :title="selectedNode.rel_path">
              {{ selectedNode.rel_path || '—' }}
            </div>
            <div class="knowledge-graph__selected-meta">
              <q-chip v-if="selectedNode.doc_type" dense size="sm" outline>{{ selectedNode.doc_type }}</q-chip>
              <span class="knowledge-graph__selected-degree">
                {{ t('knowledgePage.graphSelectedDegree', { count: selectedNode.degree }) }}
              </span>
            </div>
            <q-btn
              unelevated
              rounded
              no-caps
              dense
              color="primary"
              icon="travel_explore"
              :label="t('knowledgePage.graphOpenInExplorer')"
              class="q-mt-sm"
              @click="$emit('open-in-explorer', { docId: selectedNode.doc_id, relPath: selectedNode.rel_path })"
            />
          </template>
          <div v-else class="knowledge-graph__selected-empty">{{ t('knowledgePage.graphSelectedEmpty') }}</div>
        </div>
      </q-card-section>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import KnowledgeGraphCanvas from './graph3d/KnowledgeGraph3DCanvas.vue';
import FocusCard from './graph3d/FocusCard.vue';
import GraphLegend, { type LegendGroup } from './graph3d/GraphLegend.vue';
import HudToolbar from './graph3d/HudToolbar.vue';
import KnowledgeScopePicker from './KnowledgeScopePicker.vue';
import { graphDocTypeColor, graphLinkColor, GRAPH_LINK_TYPES } from '../../features/knowledge/graphUi';
import {
  COLOR_MODE_STORAGE_KEY,
  resolveColorMode,
  type GraphColorMode,
} from '../../features/knowledge/graph3d/structurePalette';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../features/knowledge/useVaultExplorer';
import type {
  CollectionGraphEdge,
  CollectionGraphNode,
  EntityMergeSuggestion,
  KnowledgeCollection,
  MergeEntitiesResult,
} from '../../features/knowledge/types';

const props = defineProps<{
  collections: KnowledgeCollection[];
  /** 当前库 id。 */
  collectionId: string;
  /** 边类型过滤 chips（选中集）。 */
  linkTypes: string[];
  /** 目录前缀过滤（'' = 全库）。 */
  pathPrefix: string;
  /** 渲染节点/边（经孤立裁剪 + 邻域裁剪 + M5 组过滤）。 */
  nodes: CollectionGraphNode[];
  edges: CollectionGraphEdge[];
  /** M5：图例组统计源（未做组过滤的视图节点——隐藏组仍在图例中可恢复）。 */
  legendNodes: CollectionGraphNode[];
  /** M5：已隐藏的 doc_type 组列表（页面持有 + localStorage 持久化）。 */
  hiddenGroups: string[];
  /** 全量计数（统计行）。 */
  totalNodes: number;
  totalEdges: number;
  /** 被隐藏的孤立节点数。 */
  hiddenIsolated: number;
  loading: boolean;
  error: string;
  /** 数据代际（画布重置视野）。 */
  generation: number;
  showIsolated: boolean;
  nodeQuery: string;
  /** 节点列表（连接度降序 + 搜索过滤）。 */
  nodeList: CollectionGraphNode[];
  selectedNodeId: string;
  selectedNode: CollectionGraphNode | null;
  focusSignal: number;
  /** 范围选择器迷你树根节点。 */
  scopeNodes: VaultQTreeNode[];
  /** HUD：自动旋转开关。 */
  autoRotate: boolean;
  /** HUD：标签开关。 */
  showLabels: boolean;
  /** 局部图谱跳数（0 = 全局图）。 */
  neighborhoodHops: number;
  /** 局部图谱根节点名（统计行提示）。 */
  neighborhoodRootName: string;
  /** 实体治理：合并建议列表（G5-G）。 */
  mergeSuggestions: EntityMergeSuggestion[];
  /** 实体治理：合并进行中（按钮防重入）。 */
  merging: boolean;
  /** 实体治理：最近一次合并重写反馈（null = 无）。 */
  lastMergeResult: MergeEntitiesResult | null;
  /** SP2-8：全屏覆盖模式（v-model:fullscreen）。 */
  fullscreen?: boolean;
}>();

const emit = defineEmits<{
  'select-collection': [id: string];
  'toggle-link-type': [type: string];
  'set-path-prefix': [prefix: string];
  'update:show-isolated': [value: boolean];
  'update:node-query': [value: string];
  'select-node': [docId: string];
  'focus-node': [docId: string];
  'open-in-explorer': [payload: { docId: string; relPath: string }];
  'scope-lazy-load': [payload: VaultLazyLoadPayload];
  'update:auto-rotate': [value: boolean];
  'update:show-labels': [value: boolean];
  /** 聚焦邻域：root 由页面决定（邻域模式下锁定原根）。 */
  'focus-neighborhood': [hops: number];
  'reset-global-view': [];
  /** 一键合并：mergee 并入 keeper（G5-G）。 */
  'merge-entities': [payload: { keeperId: number; mergeeId: number }];
  /** SP2-8：全屏覆盖模式开关。 */
  'update:fullscreen': [value: boolean];
  /** M4：FocusCard「重新向量化」（B1 入口②，API 接线在 B1-T4）。 */
  'reembed-node': [docId: string];
  /** M5：图例点击切换 doc_type 组隐藏（状态由页面持有）。 */
  'toggle-group': [docType: string];
  /** 缺口4：点击超点 → 组聚焦（过滤节点列表 + 相机飞组）。 */
  'focus-group': [groupName: string];
}>();

const { t } = useI18n();
const graphLinkTypes = GRAPH_LINK_TYPES;

// ---------- SP2-8：全屏覆盖模式（ESC 退出；仅全屏时监听，避免截获页面其他 ESC） ----------

function onFullscreenKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('update:fullscreen', false);
}

watch(
  () => props.fullscreen,
  (on) => {
    if (on) window.addEventListener('keydown', onFullscreenKeydown);
    else window.removeEventListener('keydown', onFullscreenKeydown);
  },
  // immediate：组件经 v-if 挂载时 fullscreen 已为 true，无值变化 watch 不触发，ESC 监听会漏注册
  { immediate: true },
);

onBeforeUnmount(() => window.removeEventListener('keydown', onFullscreenKeydown));

/** 邻域跳数草稿（步进器）；进入邻域后与 prop 同步。 */
const hopsDraft = ref(2);

watch(
  () => props.neighborhoodHops,
  (h) => {
    if (h > 0) hopsDraft.value = h;
  },
);

/** 步进选跳数；邻域模式下即时重聚焦（根锁定）。 */
function setHops(h: number) {
  hopsDraft.value = h;
  if (props.neighborhoodHops > 0) emit('focus-neighborhood', h);
}

const collectionOptions = computed(() =>
  props.collections.map((c) => ({ label: c.name || c.id, value: c.id, backend: c.vault_backend })),
);

/** 画布实例（工具条「适应视图」+ M5 透镜）。 */
const canvasRef = ref<{
  zoomToFit: (ms?: number) => void;
  clearFocus: () => void;
  setLens: (docType: string | null) => void;
} | null>(null);

function fitView() {
  canvasRef.value?.zoomToFit();
}

// ---------- M4：聚焦锁定信息卡（FocusCard） ----------

/** 聚焦节点 docId（独立局部状态，不与 selectedNodeId 耦合；Canvas focus-change 驱动）。 */
const focusedDocId = ref('');

/** FocusCard 节点数据（标题/doc_type/度数/路径）；节点不在当前裁剪集内时卸载卡片。 */
const focusedNode = computed(() => {
  if (!focusedDocId.value) return null;
  const n = props.nodes.find((x) => x.doc_id === focusedDocId.value);
  if (!n) return null;
  return { docId: n.doc_id, name: n.name, relPath: n.rel_path, docType: n.doc_type, degree: n.degree };
});

/** B1 入口②置灰逻辑：当前集合有语义层（embedding_model 非空）才可重新向量化。 */
const canReembed = computed(() => {
  const c = props.collections.find((x) => x.id === props.collectionId);
  return Boolean(c?.embedding_model);
});

/** 关闭卡片 = 解除 Canvas 聚焦锁定（clearFocus 回抛 focus-change '' 同步 focusedDocId）。 */
function onFocusClose() {
  focusedDocId.value = '';
  canvasRef.value?.clearFocus();
}

// ---------- M2：布局切换（力导向/星系盘，localStorage 持久化，刷新保持） ----------

const layout = ref<'force' | 'galaxy'>(
  typeof localStorage !== 'undefined' && localStorage.getItem('kg3d-layout') === 'galaxy' ? 'galaxy' : 'force',
);

watch(layout, (v) => {
  try {
    localStorage.setItem('kg3d-layout', v);
  } catch {
    /* 隐私模式忽略 */
  }
});

function toggleLayout() {
  layout.value = layout.value === 'galaxy' ? 'force' : 'galaxy';
}

// ---------- D5：HUD 状态（SYS 状态灯 / 着色模式 / hover 按键提示） ----------

/** 节点着色模式 A/B（structure=结构层级默认 / group=doc_type 分类），localStorage 持久化。 */
const colorMode = ref<GraphColorMode>(
  resolveColorMode(typeof localStorage !== 'undefined' ? localStorage.getItem(COLOR_MODE_STORAGE_KEY) : null),
);

watch(colorMode, (v) => {
  try {
    localStorage.setItem(COLOR_MODE_STORAGE_KEY, v);
  } catch {
    /* 隐私模式忽略 */
  }
});

function toggleColorMode() {
  colorMode.value = colorMode.value === 'structure' ? 'group' : 'structure';
}

/** hover 节点 docId（Canvas hover-change 驱动，''=离开）；按键提示上下文用。 */
const hoveredDocId = ref('');

/** SYS 状态灯：加载中 syncing / 错误 offline / 就绪 online。 */
const sysState = computed<'syncing' | 'offline' | 'online'>(() => {
  if (props.loading) return 'syncing';
  if (props.error) return 'offline';
  return 'online';
});

const sysLabel = computed(() =>
  sysState.value === 'syncing'
    ? t('knowledgePage.graphSysSyncing')
    : sysState.value === 'offline'
      ? t('knowledgePage.graphSysOffline')
      : t('knowledgePage.graphSysOnline'),
);

/** 副标库名（顶部标题区下拉当前值）。 */
const currentCollectionName = computed(() => {
  const c = props.collections.find((x) => x.id === props.collectionId);
  return c?.name || props.collectionId || '—';
});

/** M5 图例组统计：未过滤视图节点按 doc_type 聚合计数（取色与画布 palette 同源，排序稳定）。 */
const legendGroups = computed<LegendGroup[]>(() => {
  const counts = new Map<string, number>();
  for (const n of props.legendNodes) {
    const k = n.doc_type || '';
    counts.set(k, (counts.get(k) ?? 0) + 1);
  }
  return [...counts.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([docType, count]) => ({ docType, color: graphDocTypeColor(docType), count }));
});

/** 节点图例：doc_type 按频次降序取前 8，色板与画布一致。 */
const nodeLegend = computed(() => {
  const counts = new Map<string, number>();
  for (const n of props.nodes) {
    const k = n.doc_type || '';
    counts.set(k, (counts.get(k) ?? 0) + 1);
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 8)
    .map(([type, count]) => ({
      type: type || t('knowledgePage.graphLegendUntyped'),
      count,
      color: graphDocTypeColor(type),
    }));
});
</script>

<style lang="scss" scoped>
// SP2-8：全屏覆盖模式——fixed 覆盖视口，HUD 布局/交互零改动；z 低于 Quasar dialog（6000）。
.knowledge-graph--fullscreen {
  position: fixed;
  inset: 0;
  z-index: 3000;
  padding: 16px;
  // 覆盖层底色：工作台深空（组件在 .kb-workbench 内时取令牌，独立使用时退化为同值常量）。
  background: var(--kb-bg-deep, #0a0e1a);
  grid-template-rows: minmax(0, 1fr);

  .knowledge-graph__stage {
    min-height: 0;
  }
}

.knowledge-graph {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: 16px;
  align-items: stretch;

  &__stage {
    position: relative;
    min-height: 560px;
    padding: 8px;
    display: flex;
  }

  &__overlay {
    position: absolute;
    inset: 0;
    z-index: 5;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    font-size: 13px;
    color: var(--color-text-secondary);

    &--error {
      color: var(--q-negative);
    }
  }

  // D5：统计行移到画布下缘居中。
  &__stats {
    position: absolute;
    left: 50%;
    transform: translateX(-50%);
    bottom: 12px;
    z-index: 4;
    font-size: 11px;
    color: var(--color-text-secondary);
    background: var(--interaction-surface-hover);
    border-radius: 6px;
    padding: 2px 8px;
    white-space: nowrap;
  }

  // D5：左上 SYS 状态灯
  &__sys {
    position: absolute;
    top: 12px;
    left: 14px;
    z-index: 4;
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11px;
    letter-spacing: 0.1em;
  }

  &__sys-label {
    color: var(--color-text-secondary);
  }

  &__sys-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;

    &--online {
      background: #35e0d0;
      box-shadow: 0 0 8px #35e0d0;
    }

    &--syncing {
      background: #f5c542;
      box-shadow: 0 0 8px #f5c542;
      animation: kg-sys-blink 0.9s ease-in-out infinite;
    }

    &--offline {
      background: #ff6b81;
      box-shadow: 0 0 8px #ff6b81;
    }
  }

  &__sys-value {
    font-weight: 600;

    &--online {
      color: #35e0d0;
    }

    &--syncing {
      color: #f5c542;
    }

    &--offline {
      color: #ff6b81;
    }
  }

  &__sys-back {
    margin-left: 8px;
  }

  // D5：顶部居中括号大标题 + 副标
  &__title {
    position: absolute;
    top: 10px;
    left: 50%;
    transform: translateX(-50%);
    z-index: 4;
    display: flex;
    flex-direction: column;
    align-items: center;
    pointer-events: none;
  }

  &__title-main {
    font-size: 15px;
    font-weight: 600;
    letter-spacing: 0.22em;
    white-space: nowrap;
  }

  &__title-sub {
    pointer-events: auto;
    font-size: 10px;
    letter-spacing: 0.18em;

    :deep(.q-btn__content) {
      letter-spacing: 0.18em;
    }
  }

  &__title-menu {
    min-width: 180px;
    max-width: 260px;
  }

  // D5：左中竖排图标胶囊（HudToolbar）
  &__hud-toolbar {
    position: absolute;
    left: 12px;
    top: 50%;
    transform: translateY(-50%);
    z-index: 4;
  }

  // D5：左下按键提示 chips
  &__hints {
    position: absolute;
    left: 14px;
    bottom: 12px;
    z-index: 4;
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    max-width: 46%;
  }

  &__hint {
    font-size: 10px;
    letter-spacing: 0.06em;
    padding: 2px 7px;
    border-radius: 4px;
    border: 1px solid var(--color-border-soft);
    color: var(--color-text-secondary);
    background: var(--interaction-surface-hover);
    white-space: nowrap;
  }

  // M5 图例面板：GraphLegend 默认 left:16/bottom:16，抬高避开左下按键提示行。
  &__legend-panel {
    bottom: 44px;
    max-width: 240px;
  }

  &__console-body {
    display: flex;
    flex-direction: column;
    gap: 12px;
    height: 100%;
    // UX：不再限死 560px——全屏模式下跟stage 拉满视口高，内容超出时整板滚动兜底；
    // 旧值 560 会把 --grow 节点列表压成细条、选中卡溢出盖住列表（点不中列表项）。
    max-height: 100%;
    overflow-y: auto;
  }

  &__section {
    display: flex;
    flex-direction: column;
    gap: 6px;

    &--row {
      flex-direction: row;
      align-items: center;
      justify-content: space-between;
    }

    &--grow {
      flex: 1;
      min-height: 0;
    }
  }

  &__section-title {
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--color-text-secondary);
    text-transform: uppercase;
  }

  &__chips {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  &__chip-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 4px;
  }

  &__chip--off {
    opacity: 0.55;
  }

  &__node-list {
    flex: 1;
    // UX 保底 ~3 行：容器空间不足时宁可整板滚动，也不把列表挤成细条（原来点不中列表项）。
    min-height: 96px;
    max-height: 220px;
    overflow-y: auto;
  }

  &__node--active {
    color: var(--q-primary);
    background: var(--interaction-surface-hover);
    border-radius: 6px;
  }

  &__nodes-empty {
    font-size: 12px;
    color: var(--color-text-secondary);
    padding: 8px 0;
  }

  // 实体治理（G5-G）：合并建议列表限高滚动，避免挤压节点列表。
  &__merge-list {
    max-height: 148px;
    overflow-y: auto;
  }

  &__merge-names {
    font-size: 12px;
  }

  &__merge-arrow {
    opacity: 0.6;
  }

  &__merge-meta {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  &__merge-badge {
    font-size: 10px;
    line-height: 1.4;
    padding: 0 5px;
    border-radius: 3px;
    border: 1px solid var(--color-border-soft);
    color: var(--color-text-secondary);
    white-space: nowrap;
  }

  // SP1-I（I-2）：库选择器 team 徽标（与树节点团队徽标同语言）。
  &__team-badge {
    flex: none;
    font-size: 10px;
    line-height: 16px;
    padding: 0 6px;
    border-radius: 8px;
    color: var(--color-primary);
    border: 1px solid color-mix(in srgb, var(--color-primary) 55%, transparent);
  }

  &__merge-feedback {
    font-size: 11px;
    color: var(--q-positive);
  }

  &__selected {
    border-top: 1px solid var(--color-border-soft);
    padding-top: 10px;
  }

  &__selected-name {
    font-size: 14px;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__selected-path {
    font-size: 12px;
    color: var(--color-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__selected-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 6px;
  }

  &__selected-degree {
    font-size: 12px;
    color: var(--color-text-secondary);
  }

  &__selected-empty {
    font-size: 12px;
    color: var(--color-text-secondary);
  }
}

// ---------------------------------------------------------------- G5-E HUD 皮肤
// 作用域限定 .kg-hud（NFR-G5-4 不改全局 token；亮/暗主题下图谱 Tab 均为深空风）。
.kg-hud {
  --kg-cyan: #00d4ff;
  --kg-edge: #1a3a4a;
  --kg-panel: rgba(5, 8, 16, 0.88);
  --kg-text: #cfe8f5;
  --kg-text-dim: #7fa3b8;

  font-family: 'JetBrains Mono', Consolas, monospace;
  letter-spacing: 0.08em;

  // 面板：深空玻璃 + 青色描边发光
  :deep(.app-pane-card) {
    background: var(--kg-panel);
    border: 1px solid var(--kg-edge);
    box-shadow: 0 0 15px #00d4ff22;
    color: var(--kg-text);
  }

  // 覆盖 G4 依赖全局 token 的颜色（深空底上保证可读）
  .knowledge-graph__section-title {
    color: var(--kg-cyan);
    text-shadow: 0 0 6px #00d4ff55;
  }

  .knowledge-graph__stats {
    background: rgba(5, 8, 16, 0.72);
    border: 1px solid var(--kg-edge);
    color: var(--kg-text-dim);
  }

  // D5：SYS 状态灯 / 括号标题 / 按键提示 深空皮肤
  .knowledge-graph__sys-label {
    color: var(--kg-text-dim);
    text-shadow: 0 0 6px #00d4ff33;
  }

  .knowledge-graph__title-main {
    color: var(--kg-cyan);
    text-shadow: 0 0 12px #00d4ff66;
  }

  .knowledge-graph__title-sub {
    color: var(--kg-text-dim);

    :deep(.q-btn__content) {
      color: var(--kg-text-dim);
    }
  }

  .knowledge-graph__title-menu {
    background: var(--kg-panel);
    border: 1px solid var(--kg-edge);
    box-shadow: 0 0 15px #00d4ff22;
    color: var(--kg-text);
  }

  .knowledge-graph__hint {
    background: rgba(5, 8, 16, 0.72);
    border-color: var(--kg-edge);
    color: var(--kg-text-dim);

    &--ctx {
      color: var(--kg-cyan);
      border-color: var(--kg-cyan);
    }
  }

  .knowledge-graph__overlay {
    color: var(--kg-text-dim);

    &--error {
      color: #ff6b81;
    }
  }

  .knowledge-graph__nodes-empty,
  .knowledge-graph__selected-empty,
  .knowledge-graph__selected-path,
  .knowledge-graph__selected-degree {
    color: var(--kg-text-dim);
  }

  .knowledge-graph__selected {
    border-top-color: var(--kg-edge);
  }

  .knowledge-graph__selected-name {
    color: var(--kg-text);
  }

  // 图例发光色块（currentColor = 色板色，见模板 style 绑定）
  .knowledge-graph__chip-dot {
    box-shadow: 0 0 6px currentColor;
  }

  .knowledge-graph__chip--off {
    opacity: 0.45;
  }

  .knowledge-graph__node--active {
    background: rgba(0, 212, 255, 0.12);
    color: var(--kg-cyan);
  }

  // 实体治理深空适配：norm 确定性冲突用主青，embedding 语义建议用紫（区分来源可信度）。
  .knowledge-graph__merge-badge {
    border-color: var(--kg-edge);
    color: var(--kg-text-dim);

    &--norm {
      color: var(--kg-cyan);
      border-color: var(--kg-cyan);
    }

    &--embedding {
      color: #c792ea;
      border-color: #c792ea66;
    }
  }

  .knowledge-graph__merge-feedback {
    color: #5ce8a0;
    text-shadow: 0 0 6px #5ce8a044;
  }

  // Quasar 表单件深空适配（select/input/toggle/chip）
  :deep(.q-field--outlined .q-field__control):before {
    border-color: var(--kg-edge);
  }

  :deep(.q-field--outlined .q-field__control):hover:before {
    border-color: var(--kg-cyan);
  }

  :deep(.q-field__native),
  :deep(.q-field__input),
  :deep(.q-field__label) {
    color: var(--kg-text);
  }

  :deep(.q-chip) {
    color: var(--kg-text);
    border-color: var(--kg-edge);
  }

  :deep(.q-toggle__label) {
    color: var(--kg-text-dim);
  }

  // HUD 括号式开关
  &__switch {
    font: inherit;
    font-size: 11px;
    letter-spacing: 0.08em;
    background: transparent;
    border: 1px solid var(--kg-edge);
    border-radius: 4px;
    color: var(--kg-text-dim);
    padding: 3px 8px;
    cursor: pointer;
    white-space: nowrap;

    &:hover:not(:disabled) {
      border-color: var(--kg-cyan);
      color: var(--kg-text);
    }

    &:disabled {
      opacity: 0.4;
      cursor: not-allowed;
    }

    &--on {
      color: var(--kg-cyan);
      text-shadow: 0 0 6px #00d4ff55;
    }

    &--accent {
      color: var(--kg-cyan);
      border-color: var(--kg-cyan);
    }
  }

  &__bracket {
    font-weight: 600;
  }

  // 邻域跳数步进
  &__hood {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__hops {
    display: flex;
    gap: 2px;
  }

  &__hop {
    font: inherit;
    font-size: 11px;
    width: 24px;
    height: 24px;
    background: transparent;
    border: 1px solid var(--kg-edge);
    border-radius: 4px;
    color: var(--kg-text-dim);
    cursor: pointer;

    &:hover {
      border-color: var(--kg-cyan);
      color: var(--kg-text);
    }

    &--active {
      color: #050810;
      background: var(--kg-cyan);
      border-color: var(--kg-cyan);
      box-shadow: 0 0 8px #00d4ff66;
    }
  }
}

// D5：SYS SYNCING 琥珀闪烁（reduced-motion 用户降级为常亮）
@keyframes kg-sys-blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.25;
  }
}

@media (prefers-reduced-motion: reduce) {
  .knowledge-graph__sys-dot--syncing {
    animation: none;
  }
}

@media (max-width: 1100px) {
  .knowledge-graph {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
