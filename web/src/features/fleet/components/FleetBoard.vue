<template>
  <div class="fleet-board">
    <header class="fleet-board__bar">
      <h2>Agent 看板</h2>
      <q-input
        v-model="q"
        dense
        outlined
        clearable
        class="fleet-board__search"
        placeholder="搜索 worktree / 项目 / agent"
      />
      <q-toggle v-model="showIdle" dense label="显示空闲" />
    </header>
    <div class="fleet-board__cols">
      <section v-for="col in columns" :key="col.key" class="fleet-board__col" :aria-label="col.label">
        <h3>
          <StatusGlyph :status="col.glyph" />
          {{ col.label }}
          <span class="fleet-board__count">{{ col.items.length }}</span>
        </h3>
        <button
          v-for="t in col.items"
          :key="t.id"
          type="button"
          class="fleet-card"
          :class="`is-${col.key}`"
          @click="focusTerminal(t.id)"
        >
          <div class="fleet-card__head">
            <StatusGlyph :status="t.status" />
            <span class="fleet-card__title">{{ t.title || agentLabel(t.agent) }}</span>
          </div>
          <pre v-if="t.preview" class="fleet-card__preview">{{ t.preview }}</pre>
          <div class="fleet-card__foot mono">
            <span>{{ projectOf(t.workspaceId) }} / {{ wsName(t.workspaceId) }}</span>
            <span>{{ agentLabel(t.agent) }} · {{ ago(t.statusChangedAt || t.startedAt) }}</span>
          </div>
        </button>
        <div v-if="col.items.length === 0" class="fleet-board__empty">无</div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { Terminal } from '../api';
import { ago, useFleet } from '../useFleet';
import StatusGlyph from './StatusGlyph.vue';

const { state, focusTerminal, agentLabel } = useFleet();
const q = ref('');
const showIdle = ref(false);

const wsName = (id: string) => state.workspaces.find((w) => w.id === id)?.name ?? '';
const projectOf = (id: string) => {
  const pid = state.workspaces.find((w) => w.id === id)?.projectId;
  return state.projects.find((p) => p.id === pid)?.name ?? '';
};

const agents = computed(() => {
  const s = q.value?.trim().toLowerCase() ?? '';
  return state.terminals
    .filter((t) => t.agent !== 'shell')
    .filter(
      (t) =>
        !s ||
        `${wsName(t.workspaceId)} ${projectOf(t.workspaceId)} ${agentLabel(t.agent)} ${t.title}`
          .toLowerCase()
          .includes(s),
    );
});

const by = (pred: (t: Terminal) => boolean) =>
  agents.value.filter(pred).sort((a, b) => (a.statusChangedAt < b.statusChangedAt ? 1 : -1));

const columns = computed(() => {
  const cols = [
    { key: 'needs', label: '等你', glyph: 'needs_input', items: by((t) => t.status === 'needs_input') },
    {
      key: 'working',
      label: '工作中',
      glyph: 'working',
      items: by((t) => t.status === 'working' || t.status === 'starting'),
    },
    {
      key: 'done',
      label: '完成',
      glyph: 'done',
      items: by((t) => t.status === 'done' || t.status === 'exited' || t.status === 'failed'),
    },
  ];
  if (showIdle.value) cols.push({ key: 'idle', label: '空闲', glyph: 'idle', items: by((t) => t.status === 'idle') });
  return cols;
});
</script>

<style scoped lang="sass">
.fleet-board
  display: flex
  flex-direction: column
  height: 100%
  min-height: 0
  padding: var(--nt-space-4)
  gap: var(--nt-space-3)
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.fleet-board__bar
  display: flex
  align-items: center
  gap: var(--nt-space-3)
  h2
    margin: 0
    font-size: var(--nt-fs-title)
    color: var(--nt-text-0)
.fleet-board__search
  width: 280px
.fleet-board__cols
  display: grid
  grid-auto-flow: column
  grid-auto-columns: minmax(260px, 1fr)
  gap: var(--nt-space-3)
  flex: 1
  min-height: 0
  overflow-x: auto
.fleet-board__col
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  min-height: 0
  overflow-y: auto
  padding: var(--nt-space-2)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-l)
  background: var(--nt-bg-1)
  h3
    display: flex
    align-items: center
    gap: var(--nt-space-2)
    margin: 0 0 var(--nt-space-1)
    font-size: var(--nt-fs-body)
    color: var(--nt-text-1)
.fleet-board__count
  margin-left: auto
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
.fleet-board__empty
  padding: var(--nt-space-4)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
  text-align: center
.fleet-card
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-2)
  color: var(--nt-text-1)
  text-align: left
  cursor: pointer
  &:hover
    border-color: var(--nt-accent-border)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
  &.is-needs
    border-color: var(--nt-warn)
    background: color-mix(in srgb, var(--nt-warn) 10%, var(--nt-bg-2))
  &.is-done
    background: color-mix(in srgb, var(--nt-success) 8%, var(--nt-bg-2))
.fleet-card__head
  display: flex
  align-items: center
  gap: var(--nt-space-1)
  font-size: var(--nt-fs-body)
  color: var(--nt-text-0)
.fleet-card__title
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.fleet-card__preview
  max-height: 60px
  margin: 0
  overflow: hidden
  color: var(--nt-text-2)
  font-family: var(--nt-font-mono)
  font-size: var(--nt-fs-micro)
  white-space: pre-wrap
.fleet-card__foot
  display: flex
  justify-content: space-between
  gap: var(--nt-space-2)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
</style>
