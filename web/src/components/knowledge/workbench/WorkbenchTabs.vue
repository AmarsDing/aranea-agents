<template>
  <div class="kb-glass kb-tabs">
    <!-- 标签页条 -->
    <div v-if="tabs.length" class="kb-tabs__bar">
      <div
        v-for="tab in tabs"
        :key="tab.docId"
        class="kb-tabs__tab"
        :class="{
          'kb-tabs__tab--active': tab.docId === activeTabId,
          'kb-tabs__tab--dirty': tab.dirty,
        }"
        role="tab"
        :aria-selected="tab.docId === activeTabId"
        @click="$emit('activate', tab.docId)"
        @mousedown.middle.prevent="$emit('close', tab.docId)"
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
          :key="`${activeTab.docId}:${activeTab.mode}`"
          :content="activeTab.content"
          :read-only="activeTab.mode === 'preview'"
          :candidates="candidates"
          @update-content="(c: string) => $emit('update-content', activeTab!.docId, c)"
          @save="$emit('save', activeTab!.docId)"
          @open-doc="(target: string) => $emit('open-doc', target)"
          @create-doc="(target: string) => $emit('create-doc', target)"
        />
      </template>

      <!-- 空态（SP2-7 换 RingCarousel） -->
      <div v-else class="kb-tabs__empty">
        <q-icon name="auto_stories" size="48px" class="kb-tabs__empty-icon" />
        <div class="kb-tabs__empty-title">{{ t('knowledgePage.workbench.emptyTitle') }}</div>
        <div class="kb-tabs__empty-hint">{{ t('knowledgePage.workbench.emptyHint') }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// SP2 §SP2-1 中栏：笔记标签页条 + 编辑/预览内容区（编辑器本体 SP2-4 接入 CM6）。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import NoteEditor from './NoteEditor.vue';
import type { WorkbenchTab } from '../../../features/knowledge/useKnowledgeWorkbench';

const props = defineProps<{
  tabs: WorkbenchTab[];
  activeTabId: string;
  /** wikilink 补全/存在性判定候选（当前库文档 relPath） */
  candidates: string[];
}>();

const emit = defineEmits<{
  activate: [docId: string];
  close: [docId: string];
  save: [docId: string];
  'toggle-mode': [docId: string];
  'update-content': [docId: string, content: string];
  'open-doc': [target: string];
  'create-doc': [target: string];
}>();

const { t } = useI18n();

const activeTab = computed(() => props.tabs.find((x) => x.docId === props.activeTabId) ?? null);
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
      background: rgba(79, 216, 255, 0.12)
      color: var(--kb-text-primary)
      box-shadow: inset 0 -2px 0 var(--kb-accent-cyan)

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
    color: #ffc857

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
    color: #ffc857
    background: rgba(255, 200, 87, 0.08)
    border-bottom: 1px solid rgba(255, 200, 87, 0.2)
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
</style>
