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
      <DocumentSummaryCard
        v-if="summary || docType || tags.length"
        class="kb-side-panels__summary"
        :summary="summary"
        :tags="tags"
        :doc-type="docType"
      />
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
// 右栏五面板（SP2 §SP2-8）：纯展示；联动数据由 useWorkbenchSidePanels 收口（store 缓存 + 竞态守卫 + 防抖解析）。
// 折叠态持久化 localStorage（纯 UI 状态，不进 composable）。
import { computed, ref, toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import GlassPanel from '../effects/GlassPanel.vue';
import DocumentSummaryCard from '../DocumentSummaryCard.vue';
import PanelBacklinks from '../panels/PanelBacklinks.vue';
import PanelOutlinks from '../panels/PanelOutlinks.vue';
import PanelOutline from '../panels/PanelOutline.vue';
import PanelProperties from '../panels/PanelProperties.vue';
import PanelLocalGraph from '../panels/PanelLocalGraph.vue';
import { useWorkbenchSidePanels } from '../../../features/knowledge/useWorkbenchSidePanels';
import type { WorkbenchTab } from '../../../features/knowledge/useKnowledgeWorkbench';

const props = defineProps<{
  activeTab: WorkbenchTab | null;
  collectionId: string;
  /** SP2-8：图谱增量刷新信号（knowledge.graph.delta → 页面失效缓存后 +1，本栏据此重拉）。 */
  refreshNonce?: number;
  summary?: string;
  tags?: string[];
  docType?: string;
}>();

defineEmits<{
  'open-doc-id': [docId: string];
  'expand-graph': [docId: string];
  'jump-outline': [offset: number];
  'apply-autolink': [];
}>();

const { t } = useI18n();
const tags = computed(() => props.tags ?? []);
const summary = computed(() => props.summary ?? '');
const docType = computed(() => props.docType ?? '');

const { outlinks, mentions, hops, localNodes, localEdges, backlinkGroups, danglingHere, outlineItems, frontmatter } =
  useWorkbenchSidePanels({
    activeTab: toRef(props, 'activeTab'),
    collectionId: toRef(props, 'collectionId'),
    refreshNonce: toRef(props, 'refreshNonce'),
  });

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
