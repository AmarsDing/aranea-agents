<template>
  <!--
    看板 › 总览：一张图管全部公司（我 → 公司 → 部门 → Agent），画布占满整页。
    点节点在右侧看详情；公司可收起部门、部门可展开 Agent，收放状态记在本机。
    数据只用已有的：组织、名册、收件箱、交办与执行态、定时任务、今日用量、经营信号；每 60 秒和切回页面时刷新。
  -->
  <q-page class="app-standard-page kb-page" :class="{ 'kb-page--insp': selected }">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="unfold_less"
        :title="t('company.foldAll')"
        :aria-label="t('company.foldAll')"
        @click="foldAll"
      />
      <q-btn
        flat
        dense
        round
        icon="unfold_more"
        :title="t('company.unfoldAll')"
        :aria-label="t('company.unfoldAll')"
        @click="unfoldAll"
      />
    </Teleport>

    <p v-if="status === 'error'" class="kb-note kb-note--danger" role="alert">{{ t('company.loadFailed') }}</p>
    <p v-else-if="status === 'loading'" class="kb-note" role="status">{{ t('company.loading') }}</p>
    <template v-else>
      <div class="kb-canvas">
        <CompanyTree
          :board="board"
          :now="now"
          :selected="selected"
          :folded="folded"
          :opened="opened"
          @select="onSelect"
          @toggle-company="folded = flip(folded, $event)"
          @toggle-dept="opened = flip(opened, $event)"
        />
      </div>
      <BoardInspector
        v-if="selected"
        :board="board"
        :selected="selected"
        :now="now"
        :signals="signals"
        :partial="partial"
        @close="close"
        @select="onSelect"
      />
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import BoardInspector from '../components/company/BoardInspector.vue';
import CompanyTree from '../components/company/CompanyTree.vue';
import { useCompanyTree } from '../features/company/useCompanyTree';

const FOLD_KEY = 'aranea:board:fold:v1';

const { t } = useI18n();
const { status, partial, nowIso, board, signals } = useCompanyTree();
const now = computed(() => new Date(nowIso.value));

/** '' = 右侧检视关着；me / co:<id> / dept:<coId>:<deptId> / agent:<coId>:<deptId>:<agentId> */
const selected = ref('');
let trigger: HTMLElement | null = null;

function onSelect(id: string) {
  if (!selected.value) trigger = document.activeElement as HTMLElement | null;
  // 再点一次同一个节点就关上
  if (selected.value === id) close();
  else selected.value = id;
}

function close() {
  selected.value = '';
  const back = trigger;
  trigger = null;
  void nextTick(() => back?.isConnected && back.focus());
}

// ——— 收放：公司收起部门、部门展开 Agent（本机记住，读写失败不影响使用） ———
function loadFold(): { folded: string[]; opened: string[] } {
  try {
    const raw = JSON.parse(localStorage.getItem(FOLD_KEY) ?? '{}') as { folded?: unknown; opened?: unknown };
    const ids = (v: unknown) => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []);
    return { folded: ids(raw.folded), opened: ids(raw.opened) };
  } catch {
    return { folded: [], opened: [] };
  }
}
const saved = loadFold();
const folded = ref(new Set(saved.folded));
const opened = ref(new Set(saved.opened));

function flip(set: ReadonlySet<string>, id: string): Set<string> {
  const next = new Set(set);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  return next;
}

function foldAll() {
  opened.value = new Set();
  folded.value = new Set(board.value.companies.map((c) => c.id));
}

function unfoldAll() {
  folded.value = new Set();
  opened.value = new Set(board.value.companies.flatMap((c) => c.depts.map((d) => d.id)));
}

watch([folded, opened], () => {
  try {
    localStorage.setItem(FOLD_KEY, JSON.stringify({ folded: [...folded.value], opened: [...opened.value] }));
  } catch {
    // 存不了不影响收放
  }
});
</script>
