<template>
  <div class="kb-side-panels">
    <!-- 无活动 tab：玻璃空态 -->
    <GlassPanel v-if="!activeTab" class="kb-side-panels__empty">
      <div class="kb-side-panels__empty-inner">
        <q-icon name="dashboard_customize" size="36px" />
        <div>{{ t('knowledgePage.workbench.panels.empty') }}</div>
      </div>
    </GlassPanel>

    <template v-else>
      <GlassPanel
        v-for="p in panelDefs"
        :key="p.key"
        :title="t(`knowledgePage.workbench.panels.${p.key}`)"
        :icon="p.icon"
        :flush="p.key !== 'localGraph'"
        class="kb-side-panels__panel"
        :class="{ 'kb-side-panels__panel--collapsed': collapsed.has(p.key) }"
      >
        <template #header-actions>
          <q-btn
            flat
            dense
            round
            size="xs"
            :icon="collapsed.has(p.key) ? 'expand_more' : 'expand_less'"
            class="kb-side-panels__fold"
            @click="toggle(p.key)"
          />
        </template>
        <template v-if="!collapsed.has(p.key)">
          <PanelBacklinks
            v-if="p.key === 'backlinks'"
            :groups="backlinkGroups"
            :dangling="danglingHere"
            :mentions="mentions"
            @open-doc-id="(id: string) => $emit('open-doc-id', id)"
            @apply-autolink="$emit('apply-autolink')"
          />
          <PanelOutlinks
            v-else-if="p.key === 'outlinks'"
            :links="outlinks"
            @open-doc-id="(id: string) => $emit('open-doc-id', id)"
          />
          <PanelOutline
            v-else-if="p.key === 'outline'"
            :items="outlineItems"
            @jump="(offset: number) => $emit('jump-outline', offset)"
          />
          <PanelProperties v-else-if="p.key === 'properties'" :frontmatter="frontmatter" />
          <PanelLocalGraph
            v-else-if="p.key === 'localGraph'"
            :nodes="localNodes"
            :edges="localEdges"
            :root-id="activeTab.docId"
            :hops="hops"
            @update:hops="hops = $event"
            @open-doc-id="(id: string) => $emit('open-doc-id', id)"
            @expand="$emit('expand-graph', activeTab.docId)"
          />
        </template>
      </GlassPanel>
    </template>
  </div>
</template>

<script setup lang="ts">
// 右栏五面板容器（SP2 §SP2-8）：统一拉取/解析，面板纯展示；折叠态持久化 localStorage。
// Container: approved because 五面板共享同一 activeTab 数据源，收口拉取避免 5 处重复 fetch（FD3）。
// 性能（2026-08-17）：反链/出链/悬空链走 store 缓存 + graph delta 失效链，切 tab 不重复拉取；
// 局部图走 ListDocumentNeighborhood 服务端 BFS（小载荷），不再为右栏拉全库图谱。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import GlassPanel from '../effects/GlassPanel.vue';
import PanelBacklinks, { type BacklinkGroup } from '../panels/PanelBacklinks.vue';
import PanelOutlinks from '../panels/PanelOutlinks.vue';
import PanelOutline from '../panels/PanelOutline.vue';
import PanelProperties from '../panels/PanelProperties.vue';
import PanelLocalGraph from '../panels/PanelLocalGraph.vue';
import { listUnlinkedMentions } from '../../../features/knowledge/api';
import { useKnowledgeStore } from '../../../stores/knowledge';
import { parseOutline, type OutlineItem } from '../../../features/knowledge/outline';
import { parseFrontmatter, type Frontmatter } from '../../../features/knowledge/frontmatter';
import type { WorkbenchTab } from '../../../features/knowledge/useKnowledgeWorkbench';
import type {
  BlockBacklink,
  CollectionGraphEdge,
  CollectionGraphNode,
  DanglingLink,
  KnowledgeLink,
  UnlinkedMention,
} from '../../../features/knowledge/types';

const props = defineProps<{
  activeTab: WorkbenchTab | null;
  collectionId: string;
  /** SP2-8：图谱增量刷新信号（knowledge.graph.delta → 页面失效缓存后 +1，本栏据此重拉）。 */
  refreshNonce?: number;
}>();

defineEmits<{
  'open-doc-id': [docId: string];
  'expand-graph': [docId: string];
  'jump-outline': [offset: number];
  'apply-autolink': [];
}>();

const { t } = useI18n();
const knowledgeStore = useKnowledgeStore();

const panelDefs = [
  { key: 'backlinks', icon: 'link' },
  { key: 'outlinks', icon: 'call_made' },
  { key: 'outline', icon: 'format_list_bulleted' },
  { key: 'properties', icon: 'sell' },
  { key: 'localGraph', icon: 'hub' },
] as const;

type PanelKey = (typeof panelDefs)[number]['key'];

// ---------- 折叠态持久化 ----------
const collapsed = ref<Set<PanelKey>>(loadCollapsed());

function loadCollapsed(): Set<PanelKey> {
  try {
    const raw = localStorage.getItem('kb-panels-collapsed');
    const arr = raw ? (JSON.parse(raw) as string[]) : [];
    return new Set(arr.filter((x): x is PanelKey => panelDefs.some((p) => p.key === x)));
  } catch {
    return new Set();
  }
}

function toggle(key: PanelKey) {
  const next = new Set(collapsed.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  collapsed.value = next;
  try {
    localStorage.setItem('kb-panels-collapsed', JSON.stringify([...next]));
  } catch {
    // localStorage 不可用时静默（隐私模式）
  }
}

// ---------- 联动数据（活动 tab 切换/保存后刷新） ----------
const backlinks = ref<BlockBacklink[]>([]);
const dangling = ref<DanglingLink[]>([]);
const outlinks = ref<KnowledgeLink[]>([]);
const mentions = ref<UnlinkedMention[]>([]);
const hops = ref(2);

let fetchSeq = 0;

async function fetchAll(docId: string, collectionId: string) {
  const seq = ++fetchSeq;
  const [bl, dl, ol, um] = await Promise.allSettled([
    knowledgeStore.loadBlockBacklinks(docId),
    collectionId ? knowledgeStore.loadDanglingLinks(collectionId) : Promise.resolve([]),
    knowledgeStore.loadDocumentLinks(docId),
    listUnlinkedMentions(docId),
  ]);
  if (seq !== fetchSeq) return; // 竞态守卫：旧响应丢弃
  backlinks.value = bl.status === 'fulfilled' ? bl.value : [];
  dangling.value = dl.status === 'fulfilled' ? dl.value : [];
  outlinks.value = ol.status === 'fulfilled' ? ol.value : [];
  mentions.value = um.status === 'fulfilled' ? um.value : [];
}

watch(
  () => [props.activeTab?.docId, props.activeTab?.baseHash, props.collectionId, props.refreshNonce] as const,
  ([docId, , colId]) => {
    if (docId) void fetchAll(docId, colId ?? '');
    else {
      backlinks.value = [];
      dangling.value = [];
      outlinks.value = [];
      mentions.value = [];
    }
  },
  { immediate: true },
);

// ---------- 局部图（B4：服务端 BFS 邻域 RPC；doc/hops/delta 失效任一变化即重拉） ----------
const localNodes = ref<CollectionGraphNode[]>([]);
const localEdges = ref<CollectionGraphEdge[]>([]);
let graphSeq = 0;

async function fetchNeighborhood(docId: string, h: number) {
  const seq = ++graphSeq;
  try {
    const g = await knowledgeStore.loadDocumentNeighborhood(docId, h);
    if (seq !== graphSeq) return; // 竞态守卫：旧响应丢弃
    localNodes.value = g.nodes;
    localEdges.value = g.edges;
  } catch {
    if (seq !== graphSeq) return;
    localNodes.value = [];
    localEdges.value = [];
  }
}

watch(
  () => [props.activeTab?.docId, hops.value, props.refreshNonce] as const,
  ([docId, h]) => {
    if (docId) void fetchNeighborhood(docId, h ?? 2);
    else {
      graphSeq++; // 使进行中的旧响应失效
      localNodes.value = [];
      localEdges.value = [];
    }
  },
  { immediate: true },
);

// ---------- 派生 ----------
const backlinkGroups = computed<BacklinkGroup[]>(() => {
  const byDoc = new Map<string, BacklinkGroup>();
  for (const b of backlinks.value) {
    let g = byDoc.get(b.src_doc_id);
    if (!g) {
      g = { docId: b.src_doc_id, docName: b.src_doc_name || b.src_doc_id, items: [] };
      byDoc.set(b.src_doc_id, g);
    }
    g.items.push(b);
  }
  return [...byDoc.values()];
});

/** dangling：引用来源含当前文档的未创建目标。 */
const danglingHere = computed(() => {
  const id = props.activeTab?.docId;
  if (!id) return [];
  return dangling.value.filter((d) => d.refs.some((r) => r.src_doc_id === id));
});

// 大纲/属性：content 300ms 防抖解析（编辑期每 keystroke 不重算）
const debouncedContent = ref('');
let debounceTimer: ReturnType<typeof setTimeout> | null = null;
watch(
  () => props.activeTab?.content ?? '',
  (c) => {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      debouncedContent.value = c;
    }, 300);
  },
  { immediate: true },
);

const outlineItems = computed<OutlineItem[]>(() => parseOutline(debouncedContent.value));
const frontmatter = computed<Frontmatter | null>(() => parseFrontmatter(debouncedContent.value));
</script>

<style lang="sass" scoped>
.kb-side-panels
  display: flex
  flex-direction: column
  gap: 10px
  height: 100%
  min-height: 0
  overflow-y: auto

  &__empty
    flex: 1

  &__empty-inner
    display: flex
    flex-direction: column
    align-items: center
    justify-content: center
    gap: 10px
    height: 100%
    color: var(--kb-text-dim)
    font-size: 13px

  &__panel
    flex: none
    max-height: 320px

    &--collapsed
      max-height: none

  &__fold
    color: var(--kb-text-dim)
</style>
