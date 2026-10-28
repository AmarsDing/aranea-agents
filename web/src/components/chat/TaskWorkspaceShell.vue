<template>
  <ChatWorkspaceShell
    class="task-workspace"
    :class="{ 'task-workspace--reading': readingExpanded }"
    :data-task-pane="pane"
    @keydown="onKeydown"
  >
    <template #toolbar>
      <header class="task-workspace-toolbar row items-center" :class="{ 'task-workspace-toolbar--mobile': mobileMode }">
        <q-resize-observer :debounce="0" @resize="(size) => (contentWidth = size.width)" />
        <q-btn
          v-if="mobileMode"
          flat
          round
          dense
          icon="arrow_back"
          :aria-label="t('taskWorkspace.returnToChat')"
          @click="emit('return-chat')"
        />
        <q-icon name="view_quilt" color="accent" size="24px" />
        <div class="task-workspace-heading">
          <h1 class="ellipsis" :title="sessionTitle">{{ sessionTitle || t('taskWorkspace.title') }}</h1>
          <div class="text-caption text-cream-muted ellipsis">
            {{ t('taskWorkspace.title') }}
          </div>
        </div>
        <q-space />
        <q-btn
          flat
          dense
          no-caps
          icon="checklist"
          data-task-action="activity"
          :label="t(mobileMode ? 'taskWorkspace.activityShort' : 'taskWorkspace.activity')"
          :aria-label="t('taskWorkspace.activity')"
          :aria-expanded="contextVisible && contextTab === 'activity'"
          aria-controls="task-workspace-context"
          aria-keyshortcuts="Alt+4"
          @click="contextVisible && contextTab === 'activity' ? emit('close-context') : emit('show-activity')"
        />
        <q-btn
          flat
          dense
          no-caps
          icon="chat_bubble_outline"
          :label="t('taskWorkspace.chat')"
          :aria-pressed="contentWidth >= 944 || pane === 'chat'"
          aria-keyshortcuts="Alt+1"
          @click="emit('show-chat')"
        />
        <q-btn-dropdown
          flat
          dense
          no-caps
          :icon="mobileMode ? undefined : 'history'"
          :label="t(mobileMode ? 'taskWorkspace.navigationShort' : 'taskWorkspace.navigation')"
          :aria-label="t('taskWorkspace.navigation')"
        >
          <q-list>
            <q-item v-close-popup clickable @click="emit('show-navigation', 'sessions')"
              ><q-item-section>{{ t('taskWorkspace.sessions') }}</q-item-section></q-item
            >
            <q-item v-close-popup clickable @click="emit('show-navigation', 'members')"
              ><q-item-section>{{ t('taskWorkspace.members') }}</q-item-section></q-item
            >
            <q-item v-close-popup clickable @click="emit('show-navigation', 'tree')"
              ><q-item-section>{{ t('taskWorkspace.tree') }}</q-item-section></q-item
            >
            <q-item
              ><q-item-section class="text-caption">{{ t('taskWorkspace.navigationKeys') }}</q-item-section></q-item
            >
          </q-list>
        </q-btn-dropdown>
        <q-btn
          outline
          dense
          no-caps
          color="accent"
          icon="inventory_2"
          data-task-action="artifacts"
          :label="t(mobileMode ? 'taskWorkspace.artifactsShort' : 'taskWorkspace.artifacts')"
          :aria-label="t('taskWorkspace.artifacts')"
          :aria-expanded="contextVisible && contextTab === 'artifacts'"
          aria-controls="task-workspace-context"
          aria-keyshortcuts="Alt+3"
          @click="contextVisible && contextTab === 'artifacts' ? emit('close-context') : emit('show-artifacts')"
        />
      </header>
    </template>
    <slot />
    <template #dialogs><slot name="dialogs" /></template>
  </ChatWorkspaceShell>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { computed, nextTick, ref, watch } from 'vue';
import ChatWorkspaceShell from './ChatWorkspaceShell.vue';
const props = withDefaults(
  defineProps<{
    pane: 'chat' | 'navigation' | 'artifacts';
    sessionTitle?: string;
    readingExpanded?: boolean;
    mobileMode?: boolean;
    contextOpen?: boolean;
    contextTab?: 'artifacts' | 'activity';
  }>(),
  { contextOpen: true, contextTab: 'artifacts', sessionTitle: '' },
);
const emit = defineEmits<{
  'return-chat': [];
  'show-chat': [];
  'show-artifacts': [];
  'show-activity': [];
  'close-context': [];
  'show-navigation': [tab: 'sessions' | 'members' | 'tree'];
  'context-visible': [visible: boolean];
}>();
const { t } = useI18n();
const contentWidth = ref(0);
const contextVisible = computed(
  () => props.contextOpen !== false && (contentWidth.value >= 1184 || props.pane === 'artifacts'),
);
async function onKeydown(event: KeyboardEvent) {
  if (event.defaultPrevented || event.isComposing) return;
  if (event.altKey && !event.ctrlKey && !event.metaKey) {
    const root = event.currentTarget as HTMLElement;
    if (event.key === '1') emit('show-chat');
    else if (event.key === '2') emit('show-navigation', 'sessions');
    else if (event.key === '3') emit('show-artifacts');
    else if (event.key === '4') emit('show-activity');
    else return;
    event.preventDefault();
    await nextTick();
    const selector =
      event.key === '1'
        ? '.task-conversation [role="region"]'
        : event.key === '2'
          ? '.task-navigation input, .task-navigation [tabindex="0"], .task-navigation button'
          : '.task-context';
    const target =
      (event.key === '1' ? root.querySelector<HTMLElement>('.task-conversation textarea:not(:disabled)') : null) ??
      root.querySelector<HTMLElement>(selector);
    target?.focus();
  } else if (event.key === 'Escape' && !(event.target instanceof HTMLTextAreaElement)) emit('close-context');
}
watch(contextVisible, (visible) => emit('context-visible', visible), { immediate: true });
</script>

<style scoped lang="sass">
.task-workspace
  container-type: inline-size
  container-name: task-workspace
  :deep(.chat-workspace-shell__main)
    gap: 12px
    min-height: 0
  :deep(.task-navigation)
    order: 0
    width: 240px !important
    min-width: 240px !important
    max-width: 240px !important
  :deep(.task-conversation)
    order: 1
    min-width: 0
    background: var(--glass-elevated)
  :deep([tabindex='-1']:focus)
    outline: 2px solid var(--color-accent-text)
    outline-offset: 2px
  :deep(.task-context)
    order: 2
    flex: 0 0 360px
    min-width: 0
    height: auto
    align-self: stretch
.task-workspace-toolbar
  flex: 0 0 auto
  gap: var(--space-2)
  flex-wrap: wrap
  padding: var(--space-2) 0 var(--space-3)
.task-workspace-heading
  min-width: 0
  max-width: 32rem
  flex: 1 1 200px
  h1
    margin: 0
    font-size: var(--text-lg, 18px)
    line-height: 1.5
.task-workspace-toolbar--mobile
  display: grid
  grid-template-columns: repeat(4, minmax(0, 1fr))
  gap: var(--space-1)
  > .q-icon, > .q-space
    display: none
  .task-workspace-heading
    grid-column: 2 / -1
    max-width: none
  > .q-btn
    min-width: 0
    min-height: var(--control-height-touch, 44px)
  > .q-btn:first-of-type
    justify-self: start
.task-workspace--reading
  :deep(.task-navigation)
    display: none !important
  :deep(.task-context)
    flex-basis: 55%
@container task-workspace (max-width: 1183px)
  .task-workspace :deep(.task-context)
    flex-basis: 320px
  .task-workspace:not([data-task-pane='artifacts']) :deep(.task-context)
    display: none
  .task-workspace[data-task-pane='artifacts'] :deep(.task-navigation)
    display: none !important
@container task-workspace (max-width: 943px)
  .task-workspace :deep(.task-navigation)
    display: none !important
  .task-workspace[data-task-pane='navigation'] :deep(.task-navigation)
    display: flex !important
    flex: 1
    width: 100% !important
    max-width: none !important
  .task-workspace[data-task-pane='navigation'] :deep(.task-conversation), .task-workspace[data-task-pane='artifacts'] :deep(.task-conversation)
    display: none
  .task-workspace[data-task-pane='artifacts'] :deep(.task-context)
    flex: 1
</style>
