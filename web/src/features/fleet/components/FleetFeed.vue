<template>
  <div class="fleet-feed">
    <header class="fleet-feed__bar">
      <h2>动态</h2>
      <q-input v-model="q" dense outlined clearable class="fleet-feed__search" placeholder="筛选" />
      <q-space />
      <q-btn
        v-if="canNotify"
        dense
        flat
        no-caps
        icon="notifications_active"
        label="开启桌面通知"
        @click="requestDesktopNotifications"
      />
      <q-btn dense flat no-caps icon="done_all" label="全部已读" :disable="state.unread === 0" @click="markAll" />
    </header>
    <ol class="fleet-feed__list" aria-live="polite">
      <li v-for="e in items" :key="e.id">
        <button type="button" class="fleet-ev" :class="{ 'is-unread': !e.read }" @click="jumpToEvent(e)">
          <q-icon :name="icon(e.kind)" size="18px" class="fleet-ev__icon" :class="`is-${e.kind}`" />
          <div class="fleet-ev__main">
            <div class="fleet-ev__title">{{ e.title }}</div>
            <pre v-if="e.body" class="fleet-ev__body">{{ e.body }}</pre>
          </div>
          <time class="fleet-ev__time mono" :datetime="e.createdAt">{{ ago(e.createdAt) }}</time>
        </button>
      </li>
      <li v-if="items.length === 0" class="fleet-feed__empty">
        还没有动态。agent 完成一轮、等你确认、worktree 建好时都会出现在这里。
      </li>
    </ol>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import * as api from '../api';
import { ago, useFleet } from '../useFleet';

const { state, jumpToEvent, refreshEvents, requestDesktopNotifications } = useFleet();
const q = ref('');
const canNotify = typeof Notification !== 'undefined' && Notification.permission === 'default';

const items = computed(() => {
  const s = q.value?.trim().toLowerCase() ?? '';
  return state.events.filter((e) => !s || `${e.title} ${e.body}`.toLowerCase().includes(s));
});

const ICONS: Record<string, string> = {
  agent_done: 'task_alt',
  needs_input: 'help',
  exited: 'stop_circle',
  worktree_ready: 'create_new_folder',
  worktree_failed: 'error',
  worker_done: 'assignment_turned_in',
  question: 'contact_support',
  automation: 'schedule',
};
const icon = (k: string) => ICONS[k] ?? 'circle';

async function markAll() {
  await api.markEventsRead([], true);
  await refreshEvents();
}
</script>

<style scoped lang="sass">
.fleet-feed
  display: flex
  flex-direction: column
  height: 100%
  min-height: 0
  padding: var(--nt-space-4)
  gap: var(--nt-space-3)
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.fleet-feed__bar
  display: flex
  align-items: center
  gap: var(--nt-space-3)
  h2
    margin: 0
    font-size: var(--nt-fs-title)
    color: var(--nt-text-0)
.fleet-feed__search
  width: 240px
.fleet-feed__list
  flex: 1
  min-height: 0
  overflow: auto
  margin: 0
  padding: 0
  list-style: none
.fleet-feed__empty
  padding: var(--nt-space-6)
  color: var(--nt-text-3)
  text-align: center
.fleet-ev
  display: flex
  align-items: flex-start
  gap: var(--nt-space-3)
  width: 100%
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 0
  border-bottom: 1px solid var(--nt-border-1)
  background: transparent
  color: var(--nt-text-2)
  text-align: left
  cursor: pointer
  &:hover
    background: var(--nt-bg-1)
  &.is-unread
    color: var(--nt-text-0)
    .fleet-ev__title
      font-weight: 600
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: -2px
.fleet-ev__icon
  margin-top: 2px
  color: var(--nt-text-3)
  &.is-needs_input, &.is-question
    color: var(--nt-warn)
  &.is-agent_done, &.is-worker_done, &.is-worktree_ready
    color: var(--nt-success)
  &.is-worktree_failed, &.is-exited
    color: var(--nt-danger)
.fleet-ev__main
  flex: 1
  min-width: 0
.fleet-ev__title
  font-size: var(--nt-fs-body)
.fleet-ev__body
  max-height: 54px
  margin: var(--nt-space-1) 0 0
  overflow: hidden
  color: var(--nt-text-3)
  font-family: var(--nt-font-mono)
  font-size: var(--nt-fs-micro)
  white-space: pre-wrap
.fleet-ev__time
  flex: none
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
</style>
