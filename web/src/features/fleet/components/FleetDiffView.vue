<template>
  <div class="fleet-diff">
    <header class="fleet-diff__bar">
      <span
        class="fleet-diff__base mono"
        :title="`对比起点 ${diff?.baseRef || ''}（merge-base ${short(diff?.mergeBase)}）`"
      >
        对比 {{ diff?.baseRef || '…' }}
      </span>
      <span v-if="diff" class="fleet-diff__stat mono">
        <span class="is-add">+{{ diff.additions }}</span>
        <span class="is-del">−{{ diff.deletions }}</span>
        · {{ diff.files.length }} 个文件
      </span>
      <q-space />
      <q-btn
        dense
        flat
        no-caps
        size="sm"
        :icon="wrap ? 'wrap_text' : 'notes'"
        :label="wrap ? '自动换行' : '不换行'"
        @click="wrap = !wrap"
      />
      <q-btn dense flat no-caps size="sm" icon="refresh" label="刷新" :loading="loading" @click="load" />
      <q-btn-dropdown
        dense
        unelevated
        no-caps
        size="sm"
        color="primary"
        icon="send"
        :label="`发送批注 ${pendingNotes.length}`"
        :disable="pendingNotes.length === 0"
      >
        <q-list dense>
          <q-item-label header>发给这个 worktree 里的 agent</q-item-label>
          <q-item v-for="t in agentTerminals" :key="t.id" v-close-popup clickable @click="send(t.id)">
            <q-item-section avatar><StatusGlyph :status="t.status" /></q-item-section>
            <q-item-section>{{ agentLabel(t.agent) }} · {{ t.title || t.id.slice(0, 6) }}</q-item-section>
          </q-item>
          <q-item v-if="agentTerminals.length === 0" disable>
            <q-item-section>这里还没有运行中的 agent</q-item-section>
          </q-item>
          <q-separator />
          <q-item-label header>或新起一个 agent</q-item-label>
          <q-item v-for="a in availableAgents" :key="a.id" v-close-popup clickable @click="send('', a.id)">
            <q-item-section avatar><q-icon name="add" /></q-item-section>
            <q-item-section>新起 {{ a.label }}</q-item-section>
          </q-item>
        </q-list>
      </q-btn-dropdown>
    </header>

    <div v-if="error" class="fleet-diff__empty" role="alert">{{ error }}</div>
    <div v-else-if="diff && diff.files.length === 0" class="fleet-diff__empty">没有改动。</div>

    <div v-else class="fleet-diff__body">
      <nav class="fleet-diff__files" aria-label="改动文件">
        <button
          v-for="f in diff?.files ?? []"
          :key="f.path"
          type="button"
          class="fleet-diff__file"
          :class="{ 'is-active': f.path === activeFile }"
          :title="f.path"
          @click="scrollTo(f.path)"
        >
          <span class="fleet-diff__badge" :class="`is-${f.status}`">{{ statusLetter(f.status) }}</span>
          <span class="fleet-diff__fname">{{ baseName(f.path) }}</span>
          <span v-if="noteCount(f.path)" class="fleet-diff__ncount">{{ noteCount(f.path) }}</span>
          <span class="fleet-diff__fstat mono"
            ><span class="is-add">+{{ f.additions }}</span> <span class="is-del">−{{ f.deletions }}</span></span
          >
        </button>
      </nav>

      <div ref="scroller" class="fleet-diff__hunks">
        <section
          v-for="f in diff?.files ?? []"
          :key="f.path"
          :ref="(el) => setFileRef(f.path, el)"
          class="fleet-diff__section"
        >
          <h3 class="fleet-diff__fhead mono">
            <span class="fleet-diff__badge" :class="`is-${f.status}`">{{ statusLetter(f.status) }}</span>
            <span v-if="f.oldPath && f.oldPath !== f.path">{{ f.oldPath }} → </span>{{ f.path }}
          </h3>
          <div v-if="f.binary" class="fleet-diff__note-muted">二进制文件，不显示内容。</div>
          <div v-if="f.truncated" class="fleet-diff__note-muted">内容过长，已截断显示。</div>
          <table class="fleet-diff__table mono" :class="{ 'is-wrap': wrap }">
            <tbody v-for="(h, hi) in f.hunks" :key="hi">
              <tr class="fleet-diff__hunkhead">
                <td colspan="4">{{ h.header }}</td>
              </tr>
              <template v-for="(l, li) in h.lines" :key="`${hi}-${li}`">
                <tr class="fleet-diff__line" :class="`is-${l.kind}`">
                  <td class="fleet-diff__no">{{ l.oldNo || '' }}</td>
                  <td class="fleet-diff__no">
                    <button
                      type="button"
                      class="fleet-diff__add"
                      :aria-label="`在第 ${lineNo(l)} 行写批注`"
                      @click="startNote(f.path, l)"
                    >
                      +
                    </button>
                    <span>{{ l.newNo || '' }}</span>
                  </td>
                  <td class="fleet-diff__sign">{{ l.kind === 'add' ? '+' : l.kind === 'del' ? '−' : '' }}</td>
                  <td class="fleet-diff__code">{{ l.text }}</td>
                </tr>
                <tr v-for="n in notesAt(f.path, l)" :key="n.id" class="fleet-diff__noterow">
                  <td colspan="4">
                    <div class="fleet-diff__notecard" :class="{ 'is-resolved': n.resolved }">
                      <div class="fleet-diff__notehead">
                        <span>第 {{ n.line }} 行批注</span>
                        <span v-if="n.sentAt" class="fleet-diff__sent">已发送</span>
                        <span v-if="n.resolved" class="fleet-diff__sent">已解决</span>
                        <q-space />
                        <q-btn dense flat round size="xs" icon="edit" aria-label="编辑批注" @click="editNote(n)" />
                        <q-btn
                          dense
                          flat
                          round
                          size="xs"
                          :icon="n.resolved ? 'undo' : 'check'"
                          :aria-label="n.resolved ? '重新打开' : '标记已解决'"
                          @click="toggleResolved(n)"
                        />
                        <q-btn dense flat round size="xs" icon="delete" aria-label="删除批注" @click="removeNote(n)" />
                      </div>
                      <div class="fleet-diff__notebody">{{ n.body }}</div>
                    </div>
                  </td>
                </tr>
                <tr
                  v-if="draft && draft.path === f.path && draft.line === lineNo(l) && draft.side === sideOf(l)"
                  class="fleet-diff__noterow"
                >
                  <td colspan="4">
                    <div class="fleet-diff__notecard is-editing">
                      <q-input
                        v-model="draft.body"
                        autofocus
                        autogrow
                        dense
                        outlined
                        placeholder="写下对这一行的意见（Ctrl+Enter 保存，Esc 取消）"
                        @keydown.ctrl.enter.prevent="saveDraft"
                        @keydown.meta.enter.prevent="saveDraft"
                        @keydown.esc.prevent="draft = null"
                      />
                      <div class="fleet-diff__noteactions">
                        <q-btn dense flat no-caps size="sm" label="取消" @click="draft = null" />
                        <q-btn
                          dense
                          unelevated
                          no-caps
                          size="sm"
                          color="primary"
                          label="保存"
                          :disable="!draft.body.trim()"
                          @click="saveDraft"
                        />
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </section>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch, type ComponentPublicInstance } from 'vue';
import { Notify } from 'quasar';
import * as api from '../api';
import type { DiffLine, DiffResult, Note } from '../api';
import { useFleet } from '../useFleet';
import StatusGlyph from './StatusGlyph.vue';

const props = defineProps<{ workspaceId: string; active: boolean }>();
const { state, availableAgents, agentLabel, focusTerminal, refreshTerminals } = useFleet();

const diff = ref<DiffResult | null>(null);
const notes = ref<Note[]>([]);
const loading = ref(false);
const error = ref('');
const wrap = ref(false);
const activeFile = ref('');
const scroller = ref<HTMLElement | null>(null);
const fileRefs = new Map<string, HTMLElement>();
const draft = ref<{ id?: string; path: string; line: number; side: string; body: string; lineText: string } | null>(
  null,
);

const agentTerminals = computed(() =>
  state.terminals.filter(
    (t) => t.workspaceId === props.workspaceId && t.agent !== 'shell' && t.status !== 'exited' && t.status !== 'failed',
  ),
);
const pendingNotes = computed(() => notes.value.filter((n) => !n.resolved));

async function load() {
  loading.value = true;
  try {
    const [d, ns] = await Promise.all([api.getDiff(props.workspaceId), api.listNotes(props.workspaceId)]);
    diff.value = d;
    notes.value = ns;
    error.value = '';
    if (!activeFile.value && d.files[0]) activeFile.value = d.files[0].path;
  } catch (e) {
    error.value = api.errText(e);
  } finally {
    loading.value = false;
  }
}

const sideOf = (l: DiffLine) => (l.kind === 'del' ? 'old' : 'new');
const lineNo = (l: DiffLine) => (l.kind === 'del' ? l.oldNo : l.newNo);
const notesAt = (path: string, l: DiffLine) =>
  notes.value.filter((n) => n.path === path && n.line === lineNo(l) && n.side === sideOf(l));
const noteCount = (path: string) => notes.value.filter((n) => n.path === path && !n.resolved).length;
const baseName = (p: string) => p.split('/').pop() || p;
const short = (sha?: string) => (sha ? sha.slice(0, 8) : '');
const statusLetter = (s: string) => ({ added: 'A', deleted: 'D', renamed: 'R', untracked: 'U' })[s] ?? 'M';

function setFileRef(path: string, el: Element | ComponentPublicInstance | null) {
  if (el instanceof HTMLElement) fileRefs.set(path, el);
}

function scrollTo(path: string) {
  activeFile.value = path;
  fileRefs.get(path)?.scrollIntoView({ block: 'start' });
}

function startNote(path: string, l: DiffLine) {
  draft.value = { path, line: lineNo(l), side: sideOf(l), body: '', lineText: l.text };
}

function editNote(n: Note) {
  draft.value = { id: n.id, path: n.path, line: n.line, side: n.side, body: n.body, lineText: n.lineText };
}

async function saveDraft() {
  const d = draft.value;
  if (!d || !d.body.trim()) return;
  try {
    await api.upsertNote({ ...d, workspaceId: props.workspaceId, body: d.body.trim() });
    draft.value = null;
    notes.value = await api.listNotes(props.workspaceId);
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  }
}

async function toggleResolved(n: Note) {
  await api.upsertNote({ ...n, resolved: !n.resolved });
  notes.value = await api.listNotes(props.workspaceId);
}

async function removeNote(n: Note) {
  await api.deleteNote(n.id);
  notes.value = notes.value.filter((x) => x.id !== n.id);
}

async function send(terminalId: string, agent = '') {
  try {
    const res = await api.sendNotes(props.workspaceId, terminalId, agent);
    Notify.create({ type: 'positive', message: `已把 ${res.sent} 条批注整批发给 agent` });
    notes.value = await api.listNotes(props.workspaceId);
    await refreshTerminals();
    focusTerminal(res.terminalId);
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  }
}

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
.fleet-diff
  display: flex
  flex-direction: column
  height: 100%
  min-height: 0
  background: var(--nt-data-inset)
.fleet-diff__bar
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  padding: var(--nt-space-1) var(--nt-space-3)
  border-bottom: 1px solid var(--nt-border-1)
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.is-add
  color: var(--nt-success)
.is-del
  color: var(--nt-danger)
.fleet-diff__empty
  padding: var(--nt-space-6)
  color: var(--nt-text-2)
  text-align: center
.fleet-diff__body
  display: flex
  flex: 1
  min-height: 0
.fleet-diff__files
  width: 220px
  flex: none
  overflow: auto
  border-right: 1px solid var(--nt-border-1)
  padding: var(--nt-space-1)
.fleet-diff__file
  display: flex
  align-items: center
  gap: var(--nt-space-1)
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
  &:hover, &.is-active
    background: var(--nt-bg-2)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-diff__fname
  flex: 1
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.fleet-diff__fstat
  font-size: var(--nt-fs-micro)
.fleet-diff__ncount
  padding: 0 var(--nt-space-1)
  border-radius: var(--nt-radius-pill)
  background: var(--nt-accent-soft)
  color: var(--nt-accent)
  font-size: var(--nt-fs-micro)
.fleet-diff__badge
  width: 16px
  flex: none
  font-family: var(--nt-font-mono)
  font-size: var(--nt-fs-micro)
  text-align: center
  color: var(--nt-warn)
  &.is-added, &.is-untracked
    color: var(--nt-success)
  &.is-deleted
    color: var(--nt-danger)
  &.is-renamed
    color: var(--nt-info)
.fleet-diff__hunks
  flex: 1
  min-width: 0
  overflow: auto
.fleet-diff__section
  margin-bottom: var(--nt-space-3)
.fleet-diff__fhead
  position: sticky
  top: 0
  z-index: 1
  display: flex
  align-items: center
  gap: var(--nt-space-1)
  margin: 0
  padding: var(--nt-space-1) var(--nt-space-3)
  border-bottom: 1px solid var(--nt-border-1)
  background: var(--nt-bg-2)
  color: var(--nt-text-1)
  font-size: var(--nt-fs-caption)
  font-weight: 600
.fleet-diff__note-muted
  padding: var(--nt-space-2) var(--nt-space-3)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
.fleet-diff__table
  width: 100%
  border-collapse: collapse
  font-size: var(--nt-fs-caption)
  line-height: 1.5
.fleet-diff__hunkhead td
  padding: var(--nt-space-1) var(--nt-space-3)
  color: var(--nt-info)
  background: var(--nt-bg-1)
.fleet-diff__line
  &.is-add
    background: color-mix(in srgb, var(--nt-success) 12%, transparent)
  &.is-del
    background: color-mix(in srgb, var(--nt-danger) 12%, transparent)
  &:hover .fleet-diff__add
    opacity: 1
.fleet-diff__no
  position: relative
  width: 48px
  padding: 0 var(--nt-space-1)
  color: var(--nt-text-3)
  text-align: right
  user-select: none
  white-space: nowrap
.fleet-diff__add
  position: absolute
  left: 0
  top: 0
  width: 18px
  height: 100%
  padding: 0
  border: 0
  border-radius: var(--nt-radius-s)
  background: var(--nt-accent)
  color: var(--nt-on-accent)
  font-weight: 700
  cursor: pointer
  opacity: 0
  &:focus-visible
    opacity: 1
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-diff__sign
  width: 16px
  color: var(--nt-text-3)
  user-select: none
.fleet-diff__code
  padding-right: var(--nt-space-3)
  color: var(--nt-text-1)
  white-space: pre
.is-wrap .fleet-diff__code
  white-space: pre-wrap
  word-break: break-all
.fleet-diff__noterow td
  padding: var(--nt-space-1) var(--nt-space-3) var(--nt-space-2) var(--nt-space-6)
.fleet-diff__notecard
  min-width: 240px
  max-width: 640px
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-accent-border)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-2)
  font-family: var(--nt-font-sans)
  &.is-resolved
    opacity: .6
.fleet-diff__notehead
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-micro)
  white-space: nowrap
.fleet-diff__sent
  color: var(--nt-success)
.fleet-diff__notebody
  color: var(--nt-text-1)
  white-space: pre-wrap
.fleet-diff__noteactions
  display: flex
  justify-content: flex-end
  gap: var(--nt-space-2)
  margin-top: var(--nt-space-2)
</style>
