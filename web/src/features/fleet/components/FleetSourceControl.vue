<template>
  <div class="fleet-scm">
    <header class="fleet-scm__head">
      <div class="fleet-scm__branch mono">
        <q-icon name="call_split" size="16px" />
        <span>{{ st?.branch || '…' }}</span>
        <span class="fleet-scm__muted">→ {{ st?.baseRef || '' }}</span>
      </div>
      <div v-if="st" class="fleet-scm__chips mono">
        <span v-if="st.additions || st.deletions" title="相对起点的总增删">
          <span class="is-add">+{{ st.additions }}</span> <span class="is-del">−{{ st.deletions }}</span>
        </span>
        <span v-if="st.ahead" title="领先上游">↑{{ st.ahead }}</span>
        <span v-if="st.behind" title="落后上游">↓{{ st.behind }}</span>
        <span v-if="!st.upstream" class="fleet-scm__muted">未推送</span>
      </div>
      <q-space />
      <q-btn dense flat round size="sm" icon="refresh" aria-label="刷新" :loading="loading" @click="load" />
    </header>

    <div class="fleet-scm__list">
      <div class="fleet-scm__group">
        <span>已暂存（{{ staged.length }}）</span>
        <q-btn v-if="staged.length" dense flat no-caps size="sm" label="全部取消" @click="unstage([])" />
      </div>
      <button
        v-for="f in staged"
        :key="`s-${f.path}`"
        type="button"
        class="fleet-scm__file"
        :title="`取消暂存 ${f.path}`"
        @click="unstage([f.path])"
      >
        <span class="fleet-scm__code mono is-staged">{{ f.index }}</span>
        <span class="fleet-scm__path">{{ f.path }}</span>
        <q-icon name="remove" size="14px" />
      </button>
      <div class="fleet-scm__group">
        <span>改动（{{ changed.length }}）</span>
        <q-btn v-if="changed.length" dense flat no-caps size="sm" label="全部暂存" @click="stage([])" />
      </div>
      <button
        v-for="f in changed"
        :key="`c-${f.path}`"
        type="button"
        class="fleet-scm__file"
        :title="`暂存 ${f.path}`"
        @click="stage([f.path])"
      >
        <span class="fleet-scm__code mono">{{ f.untracked ? 'U' : f.worktree }}</span>
        <span class="fleet-scm__path">{{ f.path }}</span>
        <q-icon name="add" size="14px" />
      </button>
      <div v-if="st && st.files.length === 0" class="fleet-scm__muted fleet-scm__empty">工作区干净。</div>
    </div>

    <footer class="fleet-scm__foot">
      <q-input
        v-model="message"
        dense
        outlined
        autogrow
        type="textarea"
        :input-style="{ maxHeight: '160px' }"
        placeholder="提交说明（Ctrl+Enter 提交）"
        @keydown.ctrl.enter.prevent="doCommit"
        @keydown.meta.enter.prevent="doCommit"
      />
      <div class="fleet-scm__actions">
        <q-checkbox v-model="amend" dense size="sm" label="修改上一次提交" />
        <q-space />
        <q-btn
          v-if="staged.length || amend"
          dense
          unelevated
          no-caps
          color="primary"
          icon="check"
          label="提交"
          :loading="busy"
          :disable="!message.trim() && !amend"
          @click="doCommit"
        />
        <q-btn
          v-else-if="changed.length"
          dense
          unelevated
          no-caps
          color="primary"
          icon="playlist_add"
          label="暂存全部"
          :loading="busy"
          @click="stage([])"
        />
        <q-btn
          v-else
          dense
          outline
          no-caps
          color="primary"
          icon="cloud_upload"
          label="推送"
          :loading="busy"
          @click="doPush"
        />
      </div>
      <pre
        v-if="output"
        class="fleet-scm__out mono"
        :class="{ 'is-error': !outputOk }"
        :role="outputOk ? 'status' : 'alert'"
        >{{ output }}</pre
      >
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { Notify } from 'quasar';
import * as api from '../api';
import type { GitOp, GitStatus } from '../api';

const props = defineProps<{ workspaceId: string; active: boolean }>();

const st = ref<GitStatus | null>(null);
const loading = ref(false);
const busy = ref(false);
const message = ref('');
const amend = ref(false);
const output = ref('');
const outputOk = ref(true);

const staged = computed(() => st.value?.files.filter((f) => f.staged) ?? []);
const changed = computed(() => st.value?.files.filter((f) => f.untracked || (f.worktree && f.worktree !== ' ')) ?? []);

async function load() {
  loading.value = true;
  try {
    st.value = await api.getGitStatus(props.workspaceId);
  } catch (e) {
    output.value = api.errText(e);
    outputOk.value = false;
  } finally {
    loading.value = false;
  }
}

function show(r: GitOp) {
  output.value = r.output;
  outputOk.value = r.ok;
}

async function run(fn: () => Promise<GitOp>, okMsg?: string) {
  busy.value = true;
  try {
    const r = await fn();
    show(r);
    if (r.ok && okMsg) Notify.create({ type: 'positive', message: okMsg });
    await load();
    return r.ok;
  } catch (e) {
    output.value = api.errText(e);
    outputOk.value = false;
    return false;
  } finally {
    busy.value = false;
  }
}

const stage = (paths: string[]) => run(() => api.stageFiles(props.workspaceId, paths));
const unstage = (paths: string[]) => run(() => api.unstageFiles(props.workspaceId, paths));

async function doCommit() {
  if (!message.value.trim() && !amend.value) return;
  const ok = await run(() => api.commit(props.workspaceId, message.value.trim(), amend.value), '已提交');
  if (ok) {
    message.value = '';
    amend.value = false;
  }
}

const doPush = () => run(() => api.push(props.workspaceId), '已推送');

onMounted(load);
watch(() => props.workspaceId, load);
watch(
  () => props.active,
  (a) => {
    if (a) void load();
  },
);
</script>

<style scoped lang="sass">
.fleet-scm
  display: flex
  flex-direction: column
  height: 100%
  min-height: 0
  background: var(--nt-data-inset)
  color: var(--nt-text-1)
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.is-add
  color: var(--nt-success)
.is-del
  color: var(--nt-danger)
.fleet-scm__head
  display: flex
  align-items: center
  gap: var(--nt-space-3)
  padding: var(--nt-space-2) var(--nt-space-3)
  border-bottom: 1px solid var(--nt-border-1)
  font-size: var(--nt-fs-caption)
.fleet-scm__branch
  display: flex
  align-items: center
  gap: var(--nt-space-1)
.fleet-scm__chips
  display: flex
  gap: var(--nt-space-2)
.fleet-scm__muted
  color: var(--nt-text-3)
.fleet-scm__list
  flex: 1
  overflow: auto
  padding: var(--nt-space-1)
.fleet-scm__group
  display: flex
  align-items: center
  justify-content: space-between
  padding: var(--nt-space-2) var(--nt-space-2) var(--nt-space-1)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-micro)
.fleet-scm__file
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  width: 100%
  min-height: 30px
  padding: 0 var(--nt-space-2)
  border: 0
  border-radius: var(--nt-radius-s)
  background: transparent
  color: var(--nt-text-1)
  font-size: var(--nt-fs-caption)
  text-align: left
  cursor: pointer
  &:hover
    background: var(--nt-bg-2)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-scm__code
  width: 14px
  color: var(--nt-warn)
  &.is-staged
    color: var(--nt-success)
.fleet-scm__path
  flex: 1
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.fleet-scm__empty
  padding: var(--nt-space-4)
  text-align: center
.fleet-scm__foot
  padding: var(--nt-space-2) var(--nt-space-3) var(--nt-space-3)
  border-top: 1px solid var(--nt-border-1)
.fleet-scm__actions
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  margin-top: var(--nt-space-2)
.fleet-scm__out
  max-height: 180px
  margin: var(--nt-space-2) 0 0
  padding: var(--nt-space-2)
  overflow: auto
  border-radius: var(--nt-radius-s)
  background: var(--nt-bg-1)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-micro)
  white-space: pre-wrap
  &.is-error
    color: var(--nt-danger)
</style>
