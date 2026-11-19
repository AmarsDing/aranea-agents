<template>
  <aside class="fleet-side" aria-label="运行台导航">
    <button type="button" class="fleet-side__search" @click="state.jumpOpen = true">
      <q-icon name="search" size="16px" />
      <span>跳转</span>
      <kbd class="mono">Ctrl J</kbd>
    </button>

    <nav class="fleet-side__nav">
      <button
        v-for="n in navs"
        :key="n.view"
        type="button"
        class="fleet-side__navitem"
        :class="{ 'is-active': state.view === n.view }"
        @click="state.view = n.view"
      >
        <q-icon :name="n.icon" size="18px" />
        <span>{{ n.label }}</span>
        <span v-if="n.badge" class="fleet-side__badge" :class="{ 'is-warn': n.warn }">{{ n.badge }}</span>
      </button>
    </nav>

    <div class="fleet-side__head">
      <span>项目</span>
      <q-space />
      <q-input
        v-model="filter"
        dense
        borderless
        clearable
        class="fleet-side__filter"
        placeholder="筛选"
        aria-label="筛选 worktree"
      />
      <q-btn dense flat round size="sm" icon="create_new_folder" aria-label="添加项目" @click="addOpen = true" />
    </div>

    <div class="fleet-side__tree">
      <div v-if="state.loaded && state.projects.length === 0" class="fleet-side__hint">
        还没有项目。点右上角添加一个本机 git 仓库。
      </div>
      <section v-for="p in state.projects" :key="p.id" class="fleet-proj">
        <div class="fleet-proj__row">
          <button
            type="button"
            class="fleet-proj__name"
            :aria-expanded="!collapsed[p.id]"
            @click="collapsed[p.id] = !collapsed[p.id]"
          >
            <q-icon :name="collapsed[p.id] ? 'chevron_right' : 'expand_more'" size="16px" />
            <span>{{ p.name }}</span>
          </button>
          <q-btn
            dense
            flat
            round
            size="sm"
            icon="add"
            :aria-label="`在 ${p.name} 新建 worktree`"
            @click="openCreate(p.id)"
          />
        </div>
        <template v-if="!collapsed[p.id]">
          <div
            v-for="w in visibleWorkspaces(p.id)"
            :key="w.id"
            class="fleet-ws"
            :class="{ 'is-active': state.view === 'workspace' && state.currentWs === w.id, 'is-child': !!w.parentId }"
          >
            <button type="button" class="fleet-ws__main" @click="selectWorkspace(w.id)">
              <StatusGlyph
                :status="
                  w.status === 'creating' ? 'creating' : w.status === 'failed' ? 'failed' : workspaceStatus(w.id)
                "
              />
              <span class="fleet-ws__name">{{ w.name }}</span>
              <span v-if="w.kind === 'primary'" class="fleet-ws__tag">主检出</span>
              <q-icon v-if="w.pinned" name="push_pin" size="12px" class="fleet-ws__pin" />
              <span v-if="w.origin === 'orchestration'" class="fleet-ws__tag">编排</span>
              <span v-if="w.origin === 'automation'" class="fleet-ws__tag">自动化</span>
            </button>
            <div class="fleet-ws__branch mono">{{ w.status === 'failed' ? w.error : w.branch }}</div>
            <button
              v-for="t in terminalsOf(w.id)"
              :key="t.id"
              type="button"
              class="fleet-ws__agent"
              @click="focusTerminal(t.id)"
            >
              <StatusGlyph :status="t.status" />
              <span class="fleet-ws__agentname">{{ t.title || agentLabel(t.agent) }}</span>
              <span class="fleet-ws__age mono"
                >{{ agentLabel(t.agent) }} · {{ ago(t.lastActivityAt || t.startedAt) }}</span
              >
            </button>
            <q-menu touch-position context-menu>
              <q-list dense style="min-width: 180px">
                <q-item v-close-popup clickable @click="rename(w)"><q-item-section>重命名</q-item-section></q-item>
                <q-item v-close-popup clickable @click="togglePin(w)"
                  ><q-item-section>{{ w.pinned ? '取消置顶' : '置顶' }}</q-item-section></q-item
                >
                <template v-if="w.kind !== 'primary'">
                  <q-item v-close-popup clickable @click="toggleArchive(w)"
                    ><q-item-section>{{ w.archived ? '取消归档' : '归档' }}</q-item-section></q-item
                  >
                  <q-separator />
                  <q-item v-close-popup clickable class="text-negative" @click="remove(w)"
                    ><q-item-section>删除 worktree 与分支…</q-item-section></q-item
                  >
                </template>
              </q-list>
            </q-menu>
          </div>
          <button
            v-if="archivedCount(p.id)"
            type="button"
            class="fleet-side__more"
            @click="showArchived[p.id] = !showArchived[p.id]"
          >
            {{ showArchived[p.id] ? '隐藏' : '显示' }}已归档（{{ archivedCount(p.id) }}）
          </button>
        </template>
      </section>
    </div>

    <q-dialog v-model="addOpen">
      <q-card class="fleet-dialog">
        <q-card-section class="text-subtitle1">添加项目</q-card-section>
        <q-card-section class="q-gutter-sm">
          <q-input
            v-model="addPath"
            dense
            outlined
            autofocus
            label="本机仓库路径"
            placeholder="例如 E:\teamwork\aranea-agents"
          />
          <q-input v-model="addName" dense outlined label="显示名（可选）" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps label="取消" />
          <q-btn
            unelevated
            no-caps
            color="primary"
            label="添加"
            :loading="adding"
            :disable="!addPath.trim()"
            @click="doAdd"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </aside>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue';
import { Dialog, Notify } from 'quasar';
import * as api from '../api';
import type { Workspace } from '../api';
import { ago, useFleet, type FleetView } from '../useFleet';
import StatusGlyph from './StatusGlyph.vue';

const {
  state,
  selectWorkspace,
  focusTerminal,
  workspaceStatus,
  agentLabel,
  refreshAll,
  refreshWorkspaces,
  refreshTerminals,
} = useFleet();

const filter = ref('');
const collapsed = reactive<Record<string, boolean>>({});
const showArchived = reactive<Record<string, boolean>>({});
const addOpen = ref(false);
const addPath = ref('');
const addName = ref('');
const adding = ref(false);

const needsYou = computed(() => state.terminals.filter((t) => t.status === 'needs_input').length);
const runningRuns = computed(
  () => state.terminals.filter((t) => t.role === 'coordinator' && t.status !== 'exited').length,
);

const navs = computed<{ view: FleetView; icon: string; label: string; badge?: number; warn?: boolean }[]>(() => [
  { view: 'feed', icon: 'notifications', label: '动态', badge: state.unread },
  { view: 'board', icon: 'view_kanban', label: 'Agent 看板', badge: needsYou.value, warn: needsYou.value > 0 },
  { view: 'runs', icon: 'account_tree', label: '编排', badge: runningRuns.value },
  { view: 'automations', icon: 'schedule', label: '自动化' },
]);

function visibleWorkspaces(projectId: string): Workspace[] {
  const q = filter.value.trim().toLowerCase();
  return state.workspaces
    .filter((w) => w.projectId === projectId && (!w.archived || showArchived[projectId]))
    .filter((w) => !q || w.name.toLowerCase().includes(q) || w.branch.toLowerCase().includes(q))
    .sort(
      (a, b) =>
        Number(b.kind === 'primary') - Number(a.kind === 'primary') ||
        Number(b.pinned) - Number(a.pinned) ||
        (a.createdAt < b.createdAt ? 1 : -1),
    );
}

const archivedCount = (projectId: string) =>
  state.workspaces.filter((w) => w.projectId === projectId && w.archived).length;
const terminalsOf = (wsId: string) => state.terminals.filter((t) => t.workspaceId === wsId && t.agent !== 'shell');

function openCreate(projectId: string) {
  state.createProjectId = projectId;
  state.createOpen = true;
}

async function doAdd() {
  adding.value = true;
  try {
    await api.addProject(addPath.value.trim(), addName.value.trim());
    addOpen.value = false;
    addPath.value = '';
    addName.value = '';
    await refreshAll();
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  } finally {
    adding.value = false;
  }
}

function rename(w: Workspace) {
  Dialog.create({ title: '重命名', prompt: { model: w.name, type: 'text' }, cancel: true }).onOk(
    async (name: string) => {
      await api.updateWorkspace(w.id, { name });
      await refreshWorkspaces();
    },
  );
}

async function togglePin(w: Workspace) {
  await api.updateWorkspace(w.id, { pinned: !w.pinned });
  await refreshWorkspaces();
}

async function toggleArchive(w: Workspace) {
  await api.updateWorkspace(w.id, { archived: !w.archived });
  await refreshWorkspaces();
}

function remove(w: Workspace) {
  Dialog.create({
    title: `删除 worktree「${w.name}」？`,
    message: `会结束其中的终端、删除目录 ${w.path}，并删除分支 ${w.branch}。分支上有未合并的提交时会保留分支，不会丢提交。`,
    cancel: { label: '取消', flat: true },
    ok: { label: '删除', color: 'negative', unelevated: true },
  }).onOk(async () => {
    try {
      const res = await api.deleteWorkspace(w.id, false);
      Notify.create({
        type: res.branchKept ? 'warning' : 'positive',
        message: res.message,
        timeout: res.branchKept ? 8000 : 4000,
      });
      await Promise.all([refreshWorkspaces(), refreshTerminals()]);
    } catch (e) {
      Notify.create({ type: 'negative', message: api.errText(e) });
    }
  });
}
</script>

<style scoped lang="sass">
.fleet-side
  display: flex
  flex-direction: column
  width: 300px
  min-width: 240px
  height: 100%
  border-right: 1px solid var(--nt-border-1)
  background: var(--nt-bg-1)
  color: var(--nt-text-1)
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.fleet-side__search
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  margin: var(--nt-space-3) var(--nt-space-3) var(--nt-space-2)
  min-height: 36px
  padding: 0 var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-2)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  cursor: pointer
  kbd
    margin-left: auto
    color: var(--nt-text-3)
    font-size: var(--nt-fs-micro)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-side__nav
  display: flex
  flex-direction: column
  padding: 0 var(--nt-space-2)
.fleet-side__navitem
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  min-height: 34px
  padding: 0 var(--nt-space-2)
  border: 0
  border-radius: var(--nt-radius-s)
  background: transparent
  color: var(--nt-text-1)
  font-size: var(--nt-fs-body)
  text-align: left
  cursor: pointer
  &:hover
    background: var(--nt-bg-2)
  &.is-active
    background: var(--nt-accent-soft)
    color: var(--nt-accent)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-side__badge
  margin-left: auto
  min-width: 20px
  padding: 0 var(--nt-space-1)
  border-radius: var(--nt-radius-pill)
  background: var(--nt-bg-3)
  color: var(--nt-text-1)
  font-size: var(--nt-fs-micro)
  text-align: center
  &.is-warn
    background: var(--nt-warn)
    color: var(--nt-ink-inverse)
.fleet-side__head
  display: flex
  align-items: center
  gap: var(--nt-space-1)
  margin-top: var(--nt-space-3)
  padding: 0 var(--nt-space-2) 0 var(--nt-space-4)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.fleet-side__filter
  width: 90px
  font-size: var(--nt-fs-caption)
.fleet-side__tree
  flex: 1
  overflow: auto
  padding: var(--nt-space-1) var(--nt-space-2) var(--nt-space-4)
.fleet-side__hint
  padding: var(--nt-space-4) var(--nt-space-2)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
.fleet-proj__row
  display: flex
  align-items: center
.fleet-proj__name
  display: flex
  flex: 1
  align-items: center
  gap: var(--nt-space-1)
  min-height: 32px
  border: 0
  background: transparent
  color: var(--nt-text-0)
  font-size: var(--nt-fs-body)
  font-weight: 600
  text-align: left
  cursor: pointer
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-ws
  margin: 2px 0 2px var(--nt-space-2)
  padding: var(--nt-space-1)
  border: 1px solid transparent
  border-radius: var(--nt-radius-m)
  &.is-active
    border-color: var(--nt-accent-border)
    background: var(--nt-bg-2)
  &.is-child
    margin-left: var(--nt-space-5)
.fleet-ws__main
  display: flex
  align-items: center
  gap: var(--nt-space-1)
  width: 100%
  min-height: 28px
  border: 0
  background: transparent
  color: var(--nt-text-0)
  font-size: var(--nt-fs-caption)
  text-align: left
  cursor: pointer
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-ws__name
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.fleet-ws__tag
  padding: 0 var(--nt-space-1)
  border: 1px solid var(--nt-border-2)
  border-radius: var(--nt-radius-s)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.fleet-ws__pin
  color: var(--nt-text-3)
.fleet-ws__branch
  padding-left: 21px
  overflow: hidden
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
  text-overflow: ellipsis
  white-space: nowrap
.fleet-ws__agent
  display: flex
  align-items: center
  gap: var(--nt-space-1)
  width: 100%
  min-height: 26px
  padding-left: var(--nt-space-3)
  border: 0
  border-radius: var(--nt-radius-s)
  background: transparent
  color: var(--nt-text-1)
  font-size: var(--nt-fs-caption)
  text-align: left
  cursor: pointer
  &:hover
    background: var(--nt-bg-3)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-ws__agentname
  flex: 1
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.fleet-ws__age
  flex: none
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.fleet-side__more
  margin: 2px 0 0 var(--nt-space-4)
  border: 0
  background: transparent
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
  cursor: pointer
.fleet-dialog
  width: 480px
  max-width: 92vw
</style>
