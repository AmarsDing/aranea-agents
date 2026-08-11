<template>
  <div class="kb-glass kb-tabs">
    <!-- 标签页条 -->
    <div v-if="tabs.length" class="kb-tabs__bar">
      <div
        v-for="(tab, i) in tabs"
        :key="tab.docId"
        class="kb-tabs__tab"
        :class="{
          'kb-tabs__tab--active': tab.docId === activeTabId,
          'kb-tabs__tab--dirty': tab.dirty,
          'kb-tabs__tab--dragging': i === dragIndex,
          'kb-tabs__tab--drop-target': i === dropIndex && i !== dragIndex,
        }"
        role="tab"
        :aria-selected="tab.docId === activeTabId"
        draggable="true"
        @click="$emit('activate', tab.docId)"
        @mousedown.middle.prevent="$emit('close', tab.docId)"
        @dragstart="onDragStart(i, $event)"
        @dragover.prevent="onDragOver(i)"
        @drop.prevent="onDrop(i)"
        @dragend="onDragEnd"
      >
        <span class="kb-tabs__tab-title ellipsis" :title="tab.relPath">{{ tab.title }}</span>
        <span v-if="tab.dirty" class="kb-tabs__dirty-dot" />
        <q-icon
          v-if="tab.conflict"
          name="warning_amber"
          size="14px"
          class="kb-tabs__conflict"
          :title="t('knowledgePage.workbench.conflictHint')"
        />
        <q-btn flat dense round size="xs" icon="close" class="kb-tabs__close" @click.stop="$emit('close', tab.docId)" />
      </div>

      <div class="kb-tabs__bar-spacer" />

      <!-- 活动 tab 工具 -->
      <template v-if="activeTab">
        <q-btn
          v-if="activeTab.editable"
          flat
          dense
          no-caps
          size="sm"
          :icon="activeTab.mode === 'edit' ? 'visibility' : 'edit'"
          :label="
            activeTab.mode === 'edit' ? t('knowledgePage.workbench.tabPreview') : t('knowledgePage.workbench.tabEdit')
          "
          class="kb-tabs__tool"
          @click="$emit('toggle-mode', activeTab.docId)"
        >
          <q-tooltip>Ctrl+E</q-tooltip>
        </q-btn>
        <q-btn
          v-if="activeTab.editable"
          flat
          dense
          no-caps
          size="sm"
          icon="save"
          :label="t('knowledgePage.workbench.save')"
          :loading="activeTab.saving"
          :disable="!activeTab.dirty"
          class="kb-tabs__tool"
          @click="$emit('save', activeTab.docId)"
        >
          <q-tooltip>Ctrl+S</q-tooltip>
        </q-btn>
      </template>
    </div>

    <!-- 内容区 -->
    <div class="kb-tabs__body">
      <template v-if="activeTab">
        <div v-if="activeTab.conflict" class="kb-tabs__conflict-banner">
          <q-icon name="warning_amber" size="16px" />
          <span>{{ t('knowledgePage.workbench.conflictHint') }}</span>
        </div>
        <NoteEditor
          ref="editorRef"
          :key="`${activeTab.docId}:${activeTab.mode}`"
          :content="activeTab.content"
          :read-only="activeTab.mode === 'preview'"
          :candidates="candidates"
          :get-headings="getHeadings"
          :link-recency-rank="linkRecencyRank"
          @update-content="(c: string) => $emit('update-content', activeTab!.docId, c)"
          @save="$emit('save', activeTab!.docId)"
          @open-doc="(target: string, heading?: string) => $emit('open-doc', target, heading)"
          @create-doc="(target: string) => $emit('create-doc', target)"
          @pick-link="(target: string) => $emit('pick-link', target)"
        />
      </template>

      <!-- 空态（SP2-7）：3D 环形近期笔记 + 提示 + V3 主 CTA（流体 GlowButton） -->
      <div v-else class="kb-tabs__empty">
        <RingCarousel v-if="ringItems.length" :items="ringItems" @select="(it) => $emit('open-doc-id', it.key)" />
        <q-icon v-else name="auto_stories" size="48px" class="kb-tabs__empty-icon" />
        <div class="kb-tabs__empty-title">{{ t('knowledgePage.workbench.emptyTitle') }}</div>
        <div class="kb-tabs__empty-hint">{{ t('knowledgePage.workbench.emptyHint') }}</div>
        <GlowButton
          class="kb-tabs__empty-cta"
          icon="add"
          :label="t('knowledgePage.workbench.commands.new-note')"
          @click="$emit('create-note')"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// SP2 §SP2-1 中栏：笔记标签页条 + 编辑/预览内容区（编辑器本体 SP2-4 接入 CM6）。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import NoteEditor from './NoteEditor.vue';
import RingCarousel, { type RingItem } from '../effects/RingCarousel.vue';
import GlowButton from '../effects/GlowButton.vue';
import type { WorkbenchTab } from '../../../features/knowledge/useKnowledgeWorkbench';
import type { KnowledgeDocument } from '../../../features/knowledge/types';

const props = defineProps<{
  tabs: WorkbenchTab[];
  activeTabId: string;
  /** wikilink 补全/存在性判定候选（当前库文档 relPath） */
  candidates: string[];
  /** 空态 RingCarousel 卡片（近期文档，SP2-7） */
  recentDocs?: KnowledgeDocument[];
  /** P2-5：`[[target#` 标题补全数据源 */
  getHeadings?: (target: string) => string[];
  /** B4 #8：空查询 [[ 补全的最近引用名次（归一化名 → 名次，0=最近） */
  linkRecencyRank?: ReadonlyMap<string, number>;
}>();

const emit = defineEmits<{
  activate: [docId: string];
  close: [docId: string];
  save: [docId: string];
  'toggle-mode': [docId: string];
  'update-content': [docId: string, content: string];
  'open-doc': [target: string, heading?: string];
  'create-doc': [target: string];
  'open-doc-id': [docId: string];
  reorder: [from: number, to: number];
  /** B4 #8：wikilink 补全落链（原始候选 relPath，供上报 recency） */
  'pick-link': [target: string];
  /** V3：空态主 CTA「新建笔记」（父级弹新建对话框） */
  'create-note': [];
}>();

const { t } = useI18n();

const activeTab = computed(() => props.tabs.find((x) => x.docId === props.activeTabId) ?? null);

// ---------- 标签页拖拽重排（P1-4，原生 HTML5 DnD，零依赖） ----------
const dragIndex = ref(-1);
const dropIndex = ref(-1);

function onDragStart(i: number, e: DragEvent) {
  dragIndex.value = i;
  e.dataTransfer?.setData('text/plain', String(i)); // Firefox 要求 setData 才触发拖拽
  if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
}

function onDragOver(i: number) {
  dropIndex.value = i;
}

function onDrop(i: number) {
  const from = dragIndex.value;
  onDragEnd();
  if (from >= 0 && from !== i) emit('reorder', from, i);
}

function onDragEnd() {
  dragIndex.value = -1;
  dropIndex.value = -1;
}

/** 空态环形卡片：文档名 + 所在目录。 */
const ringItems = computed<RingItem[]>(() =>
  (props.recentDocs ?? []).map((d) => {
    const rel = d.rel_path || d.source;
    const i = rel.lastIndexOf('/');
    return {
      key: d.id,
      title: i >= 0 ? rel.slice(i + 1) : rel,
      subtitle: i >= 0 ? rel.slice(0, i) : '',
      icon: 'description',
    };
  }),
);

/** 右栏大纲跳转透传（SP2-5）：NoteEditor 实例滚动定位。 */
const editorRef = ref<InstanceType<typeof NoteEditor> | null>(null);

function scrollToOffset(offset: number) {
  editorRef.value?.scrollToOffset(offset);
}

defineExpose({ scrollToOffset });
</script>

<style lang="sass" scoped>
.kb-tabs
  display: flex
  flex-direction: column
  min-height: 0
  height: 100%

  &__bar
    display: flex
    align-items: center
    gap: 2px
    padding: 6px 8px
    border-bottom: 1px solid var(--kb-glass-border)
    overflow-x: auto
    flex: none

    &::-webkit-scrollbar
      height: 4px

  &__bar-spacer
    flex: 1

  &__tab
    display: flex
    align-items: center
    gap: 6px
    max-width: 200px
    padding: 5px 8px 5px 12px
    border-radius: 9px
    color: var(--kb-text-dim)
    font-size: 13px
    cursor: pointer
    user-select: none
    flex: none
    transition: background 0.15s ease, color 0.15s ease

    &:hover
      background: var(--kb-glass-highlight)

    &--active
      background: color-mix(in srgb, var(--color-accent) 12%, transparent)
      color: var(--kb-text-primary)
      box-shadow: inset 0 -2px 0 var(--kb-accent-cyan)

    &--dragging
      opacity: 0.45

    &--drop-target
      box-shadow: inset 2px 0 0 var(--kb-accent-cyan)

  &__tab-title
    min-width: 0

  &__dirty-dot
    flex: none
    width: 6px
    height: 6px
    border-radius: 50%
    background: var(--kb-accent-cyan)
    box-shadow: 0 0 6px var(--kb-accent-cyan)

  &__conflict
    flex: none
    color: var(--kb-warn)

  &__close
    flex: none
    opacity: 0
    transition: opacity 0.15s ease
    color: var(--kb-text-dim)

  &__tab:hover &__close,
  &__tab--active &__close
    opacity: 1

  &__tool
    color: var(--kb-text-dim)
    flex: none

    &:hover
      color: var(--kb-accent-cyan)

  &__body
    flex: 1
    min-height: 0
    display: flex
    flex-direction: column

  &__conflict-banner
    display: flex
    align-items: center
    gap: 8px
    padding: 8px 14px
    font-size: 12px
    color: var(--kb-warn)
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)
    border-bottom: 1px solid color-mix(in srgb, var(--color-warning) 20%, transparent)
    flex: none

  &__empty
    flex: 1
    display: flex
    flex-direction: column
    align-items: center
    justify-content: center
    gap: 8px
    color: var(--kb-text-dim)

  &__empty-icon
    color: var(--kb-accent-cyan)
    opacity: 0.6

  &__empty-title
    font-size: 16px
    color: var(--kb-text-primary)

  &__empty-hint
    font-size: 12px

  &__empty-cta
    margin-top: 10px
</style>
