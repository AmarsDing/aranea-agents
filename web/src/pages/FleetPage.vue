<template>
  <q-page class="fleet-page" :style-fn="pageStyle">
    <div class="fleet-shell">
      <FleetSidebar />
      <main class="fleet-main">
        <!-- 视图切换放在内容区顶部一行：左栏只留「项目 → worktree」树，不再上下挤两套导航 -->
        <nav class="fleet-tabs" aria-label="运行台视图">
          <button
            v-for="v in viewTabs"
            :key="v.view"
            type="button"
            class="fleet-tabs__item"
            :class="{ 'is-active': state.view === v.view }"
            :aria-current="state.view === v.view ? 'page' : undefined"
            @click="state.view = v.view"
          >
            <q-icon :name="v.icon" size="16px" />
            <span>{{ v.label }}</span>
            <span v-if="v.badge" class="fleet-tabs__badge mono" :class="{ 'is-warn': v.warn }">{{ v.badge }}</span>
          </button>
        </nav>
        <template v-if="state.view === 'workspace'">
          <div v-if="state.error" class="fleet-main__empty" role="alert">
            <q-icon name="cloud_off" size="32px" />
            <p>{{ state.error }}</p>
            <q-btn outline no-caps color="primary" label="重试" @click="refreshAll" />
          </div>
          <template v-else-if="ws">
            <header class="fleet-wsbar">
              <div class="fleet-wsbar__title">
                <h1>{{ ws.name }}</h1>
                <span class="fleet-wsbar__branch mono">
                  <q-icon name="call_split" size="14px" /> {{ ws.branch }}
                  <template v-if="ws.kind !== 'primary'"> ← {{ ws.baseRef }}</template>
                </span>
                <button
                  type="button"
                  class="fleet-wsbar__path mono"
                  :title="`复制路径 ${ws.path}`"
                  @click="copyPath(ws.path)"
                >
                  {{ ws.path }}
                </button>
              </div>
              <q-space />
              <template v-if="ws.status === 'ready'">
                <q-btn dense flat no-caps no-wrap icon="difference" label="Diff" @click="openPanel('diff')" />
                <q-btn dense flat no-caps no-wrap icon="account_tree" label="源码管理" @click="openPanel('scm')" />
                <q-btn-dropdown dense unelevated no-caps color="primary" icon="smart_toy" label="新开 agent">
                  <q-list dense>
                    <q-item
                      v-for="a in availableAgents"
                      :key="a.id"
                      v-close-popup
                      clickable
                      @click="newTerminal(ws.id, a.id)"
                    >
                      <q-item-section>{{ a.label }}</q-item-section>
                    </q-item>
                    <q-item v-close-popup clickable @click="newTerminal(ws.id, 'shell')"
                      ><q-item-section>普通终端</q-item-section></q-item
                    >
                    <q-item v-if="availableAgents.length === 0" disable
                      ><q-item-section>本机 PATH 上没找到编码 agent</q-item-section></q-item
                    >
                  </q-list>
                </q-btn-dropdown>
              </template>
            </header>
            <div v-if="ws.status === 'creating'" class="fleet-main__empty" role="status" aria-live="polite">
              <q-spinner size="28px" />
              <p>正在创建 worktree（git worktree add {{ ws.branch }} ← {{ ws.baseRef }}）…</p>
            </div>
            <div v-else-if="ws.status === 'failed'" class="fleet-main__empty" role="alert">
              <q-icon name="error" size="32px" />
              <p>创建失败：{{ ws.error }}</p>
            </div>
            <div v-else-if="layout" class="fleet-panes">
              <FleetPaneNode :node="layout.root" :layout="layout" :workspace-id="ws.id" />
            </div>
          </template>
          <div v-else-if="state.loaded" class="fleet-main__empty">
            <q-icon name="account_tree" size="40px" />
            <p>运行台：每个任务一个 git worktree，里面跑一个或多个编码 agent，互不干扰。</p>
            <p class="fleet-main__muted">先在左侧添加一个本机 git 仓库，再点项目旁的 + 新建 worktree。</p>
          </div>
        </template>
        <FleetFeed v-else-if="state.view === 'feed'" />
        <FleetBoard v-else-if="state.view === 'board'" />
        <FleetRuns v-else-if="state.view === 'runs'" />
        <FleetAutomations v-else-if="state.view === 'automations'" />
      </main>
    </div>
    <FleetStatusBar />
    <FleetCreateDialog />
    <FleetJumpPalette />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue';
import { useRoute } from 'vue-router';
import { copyToClipboard, Notify } from 'quasar';
import { useFleet, type FleetView } from '../features/fleet/useFleet';
import FleetSidebar from '../features/fleet/components/FleetSidebar.vue';
import FleetPaneNode from '../features/fleet/components/FleetPaneNode.vue';
import FleetFeed from '../features/fleet/components/FleetFeed.vue';
import FleetBoard from '../features/fleet/components/FleetBoard.vue';
import FleetRuns from '../features/fleet/components/FleetRuns.vue';
import FleetAutomations from '../features/fleet/components/FleetAutomations.vue';
import FleetStatusBar from '../features/fleet/components/FleetStatusBar.vue';
import FleetCreateDialog from '../features/fleet/components/FleetCreateDialog.vue';
import FleetJumpPalette from '../features/fleet/components/FleetJumpPalette.vue';

const {
  state,
  currentWorkspace: ws,
  availableAgents,
  layoutFor,
  openPanel,
  newTerminal,
  refreshAll,
  startPolling,
  stopPolling,
  selectWorkspace,
} = useFleet();

const needsYou = computed(() => state.terminals.filter((t) => t.status === 'needs_input').length);
const runningRuns = computed(
  () =>
    state.terminals.filter((t) => t.role === 'coordinator' && t.status !== 'exited' && t.status !== 'failed').length,
);
const viewTabs = computed<{ view: FleetView; icon: string; label: string; badge?: number; warn?: boolean }[]>(() => [
  { view: 'workspace', icon: 'terminal', label: ws.value ? `工作区 · ${ws.value.name}` : '工作区' },
  { view: 'feed', icon: 'notifications', label: '动态', badge: state.unread },
  { view: 'board', icon: 'view_kanban', label: 'Agent 看板', badge: needsYou.value, warn: needsYou.value > 0 },
  // 叫「协调任务」而不是「编排」：外层领域已经叫编排，同名会以为是一回事
  { view: 'runs', icon: 'account_tree', label: '协调任务', badge: runningRuns.value },
  { view: 'automations', icon: 'schedule', label: '自动化' },
]);

// 从别处直达：/fleet?view=automations（定时任务页「在运行台编辑」）、/fleet?ws=<worktree>
const route = useRoute();
function applyRouteQuery() {
  const view = String(route.query.view ?? '');
  const wsId = String(route.query.ws ?? '');
  if (wsId) selectWorkspace(wsId);
  else if (viewTabs.value.some((v) => v.view === view)) state.view = view as FleetView;
}
watch(() => [route.query.view, route.query.ws], applyRouteQuery);

// 布局在 watcher 里按需创建（computed 里不做写操作）。
watch(
  () => (ws.value && ws.value.status === 'ready' ? ws.value.id : ''),
  (id) => {
    if (id) layoutFor(id);
  },
  { immediate: true },
);
const layout = computed(() => (ws.value ? (state.layouts[ws.value.id] ?? null) : null));

const pageStyle = (offset: number) => ({ height: offset ? `calc(100vh - ${offset}px)` : '100vh' });

function copyPath(p: string) {
  void copyToClipboard(p).then(() => Notify.create({ message: '路径已复制', timeout: 1200 }));
}

// Ctrl+J 打开跳转面板。不用 Orca 的 Ctrl+Shift+J：浏览器里那是开发者工具控制台。
function onKey(e: KeyboardEvent) {
  if (e.ctrlKey && !e.shiftKey && !e.altKey && (e.key === 'j' || e.key === 'J')) {
    e.preventDefault();
    state.jumpOpen = !state.jumpOpen;
  }
}

onMounted(() => {
  applyRouteQuery();
  startPolling();
  // 捕获阶段监听：Quasar 按钮加载中会拦掉冒泡的键盘事件，焦点停在上面时快捷键会失灵。
  window.addEventListener('keydown', onKey, true);
});
onBeforeUnmount(() => {
  stopPolling();
  window.removeEventListener('keydown', onKey, true);
});
</script>

<style scoped lang="sass">
.fleet-page
  display: flex
  flex-direction: column
  min-height: 0
  overflow: hidden
  background: var(--nt-bg-0)
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.fleet-shell
  display: flex
  flex: 1
  min-height: 0
.fleet-main
  display: flex
  flex: 1
  flex-direction: column
  min-width: 0
  min-height: 0
.fleet-tabs
  display: flex
  flex: none
  align-items: stretch
  gap: var(--nt-space-1)
  min-height: 40px
  padding: 0 var(--nt-space-3)
  overflow-x: auto
  border-bottom: 1px solid var(--nt-border-1)
  background: var(--nt-bg-1)
.fleet-tabs__item
  display: inline-flex
  align-items: center
  gap: var(--nt-space-1)
  max-width: 280px
  padding: 0 var(--nt-space-3)
  border: 0
  border-bottom: 2px solid transparent
  background: transparent
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  white-space: nowrap
  cursor: pointer
  span:not(.fleet-tabs__badge)
    overflow: hidden
    text-overflow: ellipsis
  &:hover
    color: var(--nt-text-0)
  &.is-active
    border-bottom-color: var(--nt-accent)
    color: var(--nt-accent)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: -2px
.fleet-tabs__badge
  min-width: 18px
  padding: 0 var(--nt-space-1)
  border-radius: var(--nt-radius-pill)
  background: var(--nt-bg-3)
  color: var(--nt-text-1)
  font-size: var(--nt-fs-micro)
  text-align: center
  &.is-warn
    background: var(--nt-warn)
    color: var(--nt-ink-inverse)
.fleet-wsbar
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  min-height: 48px
  padding: 0 var(--nt-space-3)
  border-bottom: 1px solid var(--nt-border-1)
  background: var(--nt-bg-1)
.fleet-wsbar__title
  display: flex
  align-items: baseline
  gap: var(--nt-space-3)
  min-width: 0
  h1
    flex: 0 1 auto
    min-width: 6ch
    max-width: 40%
    margin: 0
    overflow: hidden
    color: var(--nt-text-0)
    font-size: var(--nt-fs-panel)
    font-weight: 600
    line-height: 1.3
    text-overflow: ellipsis
    white-space: nowrap
.fleet-wsbar__branch
  display: inline-flex
  align-items: center
  gap: 2px
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  white-space: nowrap
.fleet-wsbar__path
  flex: 1 100 0
  min-width: 0
  overflow: hidden
  text-align: left
  border: 0
  background: transparent
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
  text-overflow: ellipsis
  white-space: nowrap
  cursor: copy
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-panes
  display: flex
  flex: 1
  min-height: 0
.fleet-main__empty
  display: flex
  flex: 1
  flex-direction: column
  align-items: center
  justify-content: center
  gap: var(--nt-space-2)
  padding: var(--nt-space-6)
  color: var(--nt-text-2)
  text-align: center
.fleet-main__muted
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
@media (max-width: 799px)
  .fleet-shell :deep(.fleet-side)
    width: 220px
    min-width: 200px
</style>
