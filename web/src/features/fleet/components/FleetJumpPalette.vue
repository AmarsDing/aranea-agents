<template>
  <!-- 不要开场动画：Quasar 等动画结束才给输入框焦点，期间按键会丢，键盘面板等不起。 -->
  <q-dialog v-model="state.jumpOpen" position="top" :transition-duration="0" @before-show="onShow">
    <q-card class="fleet-jump" role="dialog" aria-label="跳转">
      <q-input
        v-model="query"
        autofocus
        dense
        borderless
        class="fleet-jump__input"
        placeholder="项目/worktree、agent 或终端…"
        @keydown.down.prevent="move(1)"
        @keydown.up.prevent="move(-1)"
        @keydown.enter.prevent="choose(items[cursor])"
        @keydown.esc="state.jumpOpen = false"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-list dense class="fleet-jump__list" role="listbox">
        <template v-for="(it, i) in items" :key="it.key">
          <q-item-label v-if="it.section" header class="fleet-jump__section">{{ it.section }}</q-item-label>
          <q-item
            clickable
            :active="i === cursor"
            active-class="fleet-jump__active"
            role="option"
            :aria-selected="i === cursor"
            @click="choose(it)"
            @mousemove="cursor = i"
          >
            <q-item-section avatar>
              <StatusGlyph v-if="it.status" :status="it.status" />
              <q-icon v-else :name="it.icon" size="16px" />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ it.label }}</q-item-label>
              <q-item-label caption class="mono">{{ it.caption }}</q-item-label>
            </q-item-section>
          </q-item>
        </template>
        <q-item v-if="items.length === 0" disable><q-item-section>没有匹配项</q-item-section></q-item>
      </q-list>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { ago, useFleet, STATUS_TEXT } from '../useFleet';
import StatusGlyph from './StatusGlyph.vue';

interface Item {
  key: string;
  section?: string;
  label: string;
  caption: string;
  icon?: string;
  status?: string;
  run: () => void;
}

const { state, selectWorkspace, focusTerminal, agentLabel, workspaceStatus } = useFleet();
const query = ref('');
const cursor = ref(0);

const RANK: Record<string, number> = {
  needs_input: 0,
  working: 1,
  done: 2,
  starting: 3,
  idle: 4,
  failed: 5,
  exited: 6,
};

const items = computed<Item[]>(() => {
  const q = query.value.trim().toLowerCase();
  const out: Item[] = [];
  const wsName = (id: string) => state.workspaces.find((w) => w.id === id)?.name ?? '';
  const projName = (id: string) => state.projects.find((p) => p.id === id)?.name ?? '';
  const terms = state.terminals
    .filter((t) => !q || `${agentLabel(t.agent)} ${t.title} ${wsName(t.workspaceId)}`.toLowerCase().includes(q))
    .sort((a, b) => (RANK[a.status] ?? 9) - (RANK[b.status] ?? 9) || (a.lastActivityAt < b.lastActivityAt ? 1 : -1))
    .slice(0, q ? 20 : 6);
  terms.forEach((t, i) =>
    out.push({
      key: `t-${t.id}`,
      section: i === 0 ? '终端与 agent' : undefined,
      label: `${agentLabel(t.agent)}${t.title ? ' · ' + t.title : ''}`,
      caption: `${wsName(t.workspaceId)} · ${STATUS_TEXT[t.status] ?? t.status} · ${ago(t.lastActivityAt || t.startedAt)}`,
      status: t.status,
      run: () => focusTerminal(t.id),
    }),
  );
  const wss = state.workspaces
    .filter((w) => !w.archived || q)
    .filter((w) => !q || `${projName(w.projectId)}/${w.name} ${w.branch}`.toLowerCase().includes(q))
    .sort((a, b) => ((a.lastActiveAt || a.createdAt) < (b.lastActiveAt || b.createdAt) ? 1 : -1))
    .slice(0, q ? 30 : 8);
  wss.forEach((w, i) =>
    out.push({
      key: `w-${w.id}`,
      section: i === 0 ? 'worktree' : undefined,
      label: `${projName(w.projectId)} / ${w.name}`,
      caption: w.branch,
      status: workspaceStatus(w.id) || undefined,
      icon: 'folder',
      run: () => selectWorkspace(w.id),
    }),
  );
  if (q && !wss.some((w) => w.name.toLowerCase() === q)) {
    out.push({
      key: 'create',
      section: '新建',
      label: `新建 worktree「${query.value.trim()}」`,
      caption: '在当前项目',
      icon: 'add',
      run: () => {
        state.createProjectId =
          state.workspaces.find((w) => w.id === state.currentWs)?.projectId ?? state.projects[0]?.id ?? '';
        state.createOpen = true;
      },
    });
  }
  return out;
});

watch(query, () => (cursor.value = 0));

function onShow() {
  query.value = '';
  cursor.value = 0;
}

function move(d: number) {
  const n = items.value.length;
  if (n) cursor.value = (cursor.value + d + n) % n;
}

function choose(it?: Item) {
  if (!it) return;
  state.jumpOpen = false;
  it.run();
}
</script>

<style scoped lang="sass">
.fleet-jump
  width: 620px
  max-width: 94vw
  margin-top: 10vh
  background: var(--nt-bg-2)
.fleet-jump__input
  padding: var(--nt-space-1) var(--nt-space-3)
  border-bottom: 1px solid var(--nt-border-1)
  font-size: var(--nt-fs-body)
.fleet-jump__list
  max-height: 60vh
  overflow: auto
.fleet-jump__section
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.fleet-jump__active
  background: var(--nt-accent-soft)
  color: var(--nt-text-0)
.mono
  font-family: var(--nt-font-mono)
</style>
