<template>
  <div v-if="node.type === 'split'" ref="splitEl" class="fleet-split" :class="`is-${node.dir}`">
    <div class="fleet-split__child" :style="{ flexBasis: `${node.ratio * 100}%` }">
      <FleetPaneNode :node="node.a" :layout="layout" :workspace-id="workspaceId" />
    </div>
    <div
      class="fleet-split__bar"
      role="separator"
      :aria-orientation="node.dir === 'row' ? 'vertical' : 'horizontal'"
      tabindex="0"
      @pointerdown="startResize"
      @keydown.left.prevent="nudge(-0.05)"
      @keydown.up.prevent="nudge(-0.05)"
      @keydown.right.prevent="nudge(0.05)"
      @keydown.down.prevent="nudge(0.05)"
    />
    <div class="fleet-split__child" :style="{ flexBasis: `${(1 - node.ratio) * 100}%` }">
      <FleetPaneNode :node="node.b" :layout="layout" :workspace-id="workspaceId" />
    </div>
  </div>

  <div
    v-else
    class="fleet-leaf"
    :class="{ 'is-focused': layout.focus === node.id }"
    @pointerdown.capture="layout.focus = node.id"
  >
    <div class="fleet-tabs" role="tablist">
      <div
        v-for="tid in node.tabs"
        :key="tid"
        class="fleet-tab"
        :class="{ 'is-active': node.active === tid }"
        role="tab"
        :aria-selected="node.active === tid"
        tabindex="0"
        draggable="true"
        @click="activate(tid)"
        @keydown.enter="activate(tid)"
        @dragstart="onDragStart($event, tid)"
        @dragend="dragging.tab = ''"
      >
        <StatusGlyph v-if="tabTerminal(tid)" :status="tabTerminal(tid)!.status" />
        <q-icon v-else :name="layout.tabs[tid]?.kind === 'diff' ? 'difference' : 'account_tree'" size="14px" />
        <span class="fleet-tab__label">{{ tabLabel(tid) }}</span>
        <q-btn
          v-if="tabTerminal(tid) && ['exited', 'failed'].includes(tabTerminal(tid)!.status)"
          dense
          flat
          no-caps
          size="xs"
          label="重启"
          @click.stop="restart(tabTerminal(tid)!.id)"
        />
        <q-btn dense flat round size="xs" icon="close" :aria-label="`关闭 ${tabLabel(tid)}`" @click.stop="close(tid)" />
        <q-menu touch-position context-menu>
          <q-list dense style="min-width: 160px">
            <template v-if="tabTerminal(tid)">
              <q-item v-close-popup clickable @click="restart(tabTerminal(tid)!.id)"
                ><q-item-section>重启</q-item-section></q-item
              >
              <q-item v-close-popup clickable @click="kill(tabTerminal(tid)!.id)"
                ><q-item-section>结束进程（保留标签）</q-item-section></q-item
              >
            </template>
            <q-item v-close-popup clickable @click="splitTab(tid, 'right')"
              ><q-item-section>向右分屏</q-item-section></q-item
            >
            <q-item v-close-popup clickable @click="splitTab(tid, 'bottom')"
              ><q-item-section>向下分屏</q-item-section></q-item
            >
            <q-separator />
            <q-item v-close-popup clickable @click="close(tid)"><q-item-section>关闭</q-item-section></q-item>
          </q-list>
        </q-menu>
      </div>
      <q-btn dense flat round size="sm" icon="add" class="fleet-tabs__add" aria-label="新建标签">
        <q-menu>
          <q-list dense style="min-width: 200px">
            <q-item-label header>新开 agent</q-item-label>
            <q-item v-for="a in availableAgents" :key="a.id" v-close-popup clickable @click="openAgent(a.id)">
              <q-item-section>{{ a.label }}</q-item-section>
            </q-item>
            <q-item v-close-popup clickable @click="openAgent('shell')"
              ><q-item-section>普通终端</q-item-section></q-item
            >
            <q-separator />
            <q-item v-close-popup clickable @click="openPanel('diff', workspaceId)"
              ><q-item-section>改动 Diff</q-item-section></q-item
            >
            <q-item v-close-popup clickable @click="openPanel('scm', workspaceId)"
              ><q-item-section>源码管理</q-item-section></q-item
            >
          </q-list>
        </q-menu>
      </q-btn>
    </div>

    <div class="fleet-leaf__body">
      <template v-for="tid in node.tabs" :key="tid">
        <div v-show="node.active === tid" class="fleet-leaf__pane">
          <FleetTerminalView
            v-if="layout.tabs[tid]?.kind === 'terminal'"
            :ref="(el) => setTermRef(tid, el)"
            :terminal-id="layout.tabs[tid]!.ref!"
            :active="node.active === tid"
            @restart="restart(layout.tabs[tid]!.ref!)"
          />
          <FleetDiffView
            v-else-if="layout.tabs[tid]?.kind === 'diff'"
            :workspace-id="workspaceId"
            :active="node.active === tid"
          />
          <FleetSourceControl
            v-else-if="layout.tabs[tid]?.kind === 'scm'"
            :workspace-id="workspaceId"
            :active="node.active === tid"
          />
        </div>
      </template>
      <div v-if="node.tabs.length === 0" class="fleet-leaf__empty">
        <p>这个 worktree 还没有打开任何东西。</p>
        <div class="fleet-leaf__starts">
          <q-btn
            v-for="a in availableAgents.slice(0, 4)"
            :key="a.id"
            outline
            no-caps
            color="primary"
            :label="`启动 ${a.label}`"
            @click="openAgent(a.id)"
          />
          <q-btn outline no-caps color="primary" label="普通终端" @click="openAgent('shell')" />
        </div>
      </div>

      <div v-if="dragging.tab" class="fleet-drop" @dragover.prevent>
        <div
          v-for="z in zones"
          :key="z"
          class="fleet-drop__zone"
          :class="[`is-${z}`, { 'is-hot': hot === z }]"
          @dragenter.prevent="hot = z"
          @dragleave="hot = hot === z ? '' : hot"
          @dragover.prevent
          @drop.prevent="onDrop(z)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, type ComponentPublicInstance } from 'vue';
import { Dialog, Notify } from 'quasar';
import * as api from '../api';
import { closeTab, moveTab, type DropZone, type Layout, type PaneNode } from '../layout';
import { useFleet, STATUS_TEXT } from '../useFleet';
import { dragging } from '../dragState';
import FleetTerminalView from './FleetTerminalView.vue';
import FleetDiffView from './FleetDiffView.vue';
import FleetSourceControl from './FleetSourceControl.vue';
import StatusGlyph from './StatusGlyph.vue';

defineOptions({ name: 'FleetPaneNode' });

const props = defineProps<{ node: PaneNode; layout: Layout; workspaceId: string }>();
const { state, availableAgents, agentLabel, saveLayouts, openPanel, newTerminal, refreshTerminals } = useFleet();

const zones: DropZone[] = ['top', 'left', 'center', 'right', 'bottom'];
const hot = ref<DropZone | ''>('');
const splitEl = ref<HTMLElement | null>(null);
const termRefs = new Map<string, { reconnect: () => void }>();

function setTermRef(tid: string, el: Element | ComponentPublicInstance | null) {
  if (el && 'reconnect' in (el as object)) termRefs.set(tid, el as unknown as { reconnect: () => void });
}

function tabTerminal(tid: string) {
  const t = props.layout.tabs[tid];
  return t?.kind === 'terminal' ? state.terminals.find((x) => x.id === t.ref) : undefined;
}

function tabLabel(tid: string): string {
  const tab = props.layout.tabs[tid];
  if (!tab) return '';
  if (tab.kind === 'diff') return 'Diff';
  if (tab.kind === 'scm') return '源码管理';
  const t = tabTerminal(tid);
  if (!t) return '终端';
  const name = agentLabel(t.agent);
  if (t.agent === 'shell') return t.title ? `${name} · ${t.title}` : name;
  return t.title ? `${name} · ${t.title}` : `${name} · ${STATUS_TEXT[t.status] ?? t.status}`;
}

function activate(tid: string) {
  if (props.node.type !== 'leaf') return;
  props.node.active = tid;
  props.layout.focus = props.node.id;
  saveLayouts();
}

function onDragStart(ev: DragEvent, tid: string) {
  dragging.tab = tid;
  ev.dataTransfer?.setData('application/x-fleet-tab', tid);
  if (ev.dataTransfer) ev.dataTransfer.effectAllowed = 'move';
}

function onDrop(zone: DropZone) {
  const tid = dragging.tab;
  dragging.tab = '';
  hot.value = '';
  if (!tid || props.node.type !== 'leaf') return;
  moveTab(props.layout, tid, props.node.id, zone);
  saveLayouts();
}

function splitTab(tid: string, zone: DropZone) {
  if (props.node.type !== 'leaf') return;
  moveTab(props.layout, tid, props.node.id, zone);
  saveLayouts();
}

async function close(tid: string) {
  const t = tabTerminal(tid);
  const doClose = async () => {
    if (t) {
      try {
        await api.closeTerminal(t.id);
      } catch (e) {
        Notify.create({ type: 'negative', message: api.errText(e) });
      }
      await refreshTerminals();
    }
    closeTab(props.layout, tid);
    saveLayouts();
  };
  if (t && t.agent !== 'shell' && ['working', 'needs_input', 'starting'].includes(t.status)) {
    Dialog.create({
      title: '关闭正在运行的 agent？',
      message: `${agentLabel(t.agent)} 还在${STATUS_TEXT[t.status]}，关闭会结束进程。`,
      cancel: { label: '取消', flat: true },
      ok: { label: '结束并关闭', color: 'negative', unelevated: true },
    }).onOk(() => void doClose());
    return;
  }
  await doClose();
}

async function restart(terminalId: string) {
  try {
    await api.restartTerminal(terminalId);
    await refreshTerminals();
    const tid = Object.values(props.layout.tabs).find((t) => t.ref === terminalId)?.id;
    if (tid) termRefs.get(tid)?.reconnect();
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  }
}

async function kill(terminalId: string) {
  await api.killTerminal(terminalId);
  await refreshTerminals();
}

function openAgent(agent: string) {
  props.layout.focus = props.node.id;
  void newTerminal(props.workspaceId, agent);
}

// ---------- 分隔条 ----------

function startResize(ev: PointerEvent) {
  const node = props.node;
  const el = splitEl.value;
  if (node.type !== 'split' || !el) return;
  const rect = el.getBoundingClientRect();
  const target = ev.currentTarget as HTMLElement;
  target.setPointerCapture(ev.pointerId);
  const move = (e: PointerEvent) => {
    const r = node.dir === 'row' ? (e.clientX - rect.left) / rect.width : (e.clientY - rect.top) / rect.height;
    node.ratio = Math.min(0.85, Math.max(0.15, r));
  };
  const up = () => {
    target.removeEventListener('pointermove', move);
    target.removeEventListener('pointerup', up);
    saveLayouts();
  };
  target.addEventListener('pointermove', move);
  target.addEventListener('pointerup', up);
}

function nudge(d: number) {
  if (props.node.type !== 'split') return;
  props.node.ratio = Math.min(0.85, Math.max(0.15, props.node.ratio + d));
  saveLayouts();
}
</script>

<style scoped lang="sass">
.fleet-split
  display: flex
  width: 100%
  height: 100%
  min-width: 0
  min-height: 0
  &.is-row
    flex-direction: row
  &.is-col
    flex-direction: column
.fleet-split__child
  display: flex
  min-width: 0
  min-height: 0
  flex-grow: 0
  flex-shrink: 1
.fleet-split__bar
  flex: none
  background: var(--nt-border-1)
  .is-row > &
    width: 4px
    cursor: col-resize
  .is-col > &
    height: 4px
    cursor: row-resize
  &:hover, &:focus-visible
    background: var(--nt-accent)
    outline: none
.fleet-leaf
  display: flex
  flex-direction: column
  width: 100%
  height: 100%
  min-width: 0
  min-height: 0
  border-top: 2px solid transparent
  &.is-focused
    border-top-color: var(--nt-accent-border)
.fleet-tabs
  display: flex
  align-items: center
  gap: 2px
  min-height: 36px
  padding: 0 var(--nt-space-1)
  overflow-x: auto
  border-bottom: 1px solid var(--nt-border-1)
  background: var(--nt-bg-1)
.fleet-tab
  display: flex
  align-items: center
  gap: var(--nt-space-1)
  max-width: 260px
  height: 30px
  padding: 0 var(--nt-space-1) 0 var(--nt-space-2)
  border-radius: var(--nt-radius-s)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  cursor: pointer
  user-select: none
  &:hover
    background: var(--nt-bg-2)
  &.is-active
    background: var(--nt-bg-3)
    color: var(--nt-text-0)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-tab__label
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.fleet-tabs__add
  flex: none
.fleet-leaf__body
  position: relative
  flex: 1
  min-height: 0
.fleet-leaf__pane
  position: absolute
  inset: 0
.fleet-leaf__empty
  display: flex
  flex-direction: column
  align-items: center
  justify-content: center
  gap: var(--nt-space-3)
  height: 100%
  color: var(--nt-text-2)
.fleet-leaf__starts
  display: flex
  flex-wrap: wrap
  justify-content: center
  gap: var(--nt-space-2)
.fleet-drop
  position: absolute
  inset: 0
  z-index: 5
  display: grid
  grid-template: "top top top" 22% "left center right" 1fr "bottom bottom bottom" 22% / 22% 1fr 22%
.fleet-drop__zone
  border-radius: var(--nt-radius-m)
  &.is-top
    grid-area: top
  &.is-left
    grid-area: left
  &.is-center
    grid-area: center
  &.is-right
    grid-area: right
  &.is-bottom
    grid-area: bottom
  &.is-hot
    background: var(--nt-accent-soft)
    outline: 2px dashed var(--nt-accent)
    outline-offset: -4px
</style>
