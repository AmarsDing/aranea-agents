// 右栏五面板联动数据（SP2 §SP2-8）：统一拉取/解析，WorkbenchSidePanels 纯展示。
// 收口拉取避免 5 处重复 fetch（FD3）；反链/出链/悬空链/未链接提及走 store 缓存 + graph delta 失效链，
// 切 tab 不重复拉取；局部图走 ListDocumentNeighborhood 服务端 BFS（小载荷），不为右栏拉全库图谱。
import { computed, ref, watch, type Ref } from 'vue';
import { useKnowledgeStore } from '../../stores/knowledge';
import { parseOutline, type OutlineItem } from './outline';
import { parseFrontmatter, type Frontmatter } from './frontmatter';
import type { WorkbenchTab } from './useKnowledgeWorkbench';
import type {
  BlockBacklink,
  CollectionGraphEdge,
  CollectionGraphNode,
  DanglingLink,
  KnowledgeLink,
  UnlinkedMention,
} from './types';

/** 反链分组（PanelBacklinks 展示模型）。 */
export interface SidePanelBacklinkGroup {
  docId: string;
  docName: string;
  items: BlockBacklink[];
}

export interface WorkbenchSidePanelsDeps {
  activeTab: Ref<WorkbenchTab | null>;
  collectionId: Ref<string>;
  /** SP2-8：图谱增量刷新信号（knowledge.graph.delta → 页面失效缓存后 +1，本栏据此重拉）。 */
  refreshNonce: Ref<number | undefined>;
}

export function useWorkbenchSidePanels(deps: WorkbenchSidePanelsDeps) {
  const knowledgeStore = useKnowledgeStore();

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
      knowledgeStore.loadUnlinkedMentions(docId),
    ]);
    if (seq !== fetchSeq) return; // 竞态守卫：旧响应丢弃
    backlinks.value = bl.status === 'fulfilled' ? bl.value : [];
    dangling.value = dl.status === 'fulfilled' ? dl.value : [];
    outlinks.value = ol.status === 'fulfilled' ? ol.value : [];
    mentions.value = um.status === 'fulfilled' ? um.value : [];
  }

  watch(
    () =>
      [
        deps.activeTab.value?.docId,
        deps.activeTab.value?.baseHash,
        deps.collectionId.value,
        deps.refreshNonce.value,
      ] as const,
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
    () => [deps.activeTab.value?.docId, hops.value, deps.refreshNonce.value] as const,
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
  const backlinkGroups = computed<SidePanelBacklinkGroup[]>(() => {
    const byDoc = new Map<string, SidePanelBacklinkGroup>();
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
    const id = deps.activeTab.value?.docId;
    if (!id) return [];
    return dangling.value.filter((d) => d.refs.some((r) => r.src_doc_id === id));
  });

  // 大纲/属性：content 300ms 防抖解析（编辑期每 keystroke 不重算）
  const debouncedContent = ref('');
  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  watch(
    () => deps.activeTab.value?.content ?? '',
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

  return {
    backlinks,
    dangling,
    outlinks,
    mentions,
    hops,
    localNodes,
    localEdges,
    backlinkGroups,
    danglingHere,
    outlineItems,
    frontmatter,
  };
}
