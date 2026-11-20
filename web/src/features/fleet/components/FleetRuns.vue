<template>
  <div class="fleet-runs">
    <aside class="fleet-runs__list">
      <header class="fleet-runs__bar">
        <span class="fleet-runs__count">{{ runs.length }} 个协调任务</span>
        <q-space />
        <q-btn dense unelevated no-caps color="primary" icon="add" label="新建" @click="openNew" />
      </header>
      <button
        v-for="r in runs"
        :key="r.id"
        type="button"
        class="fleet-run"
        :class="{ 'is-active': r.id === currentId }"
        @click="select(r.id)"
      >
        <div class="fleet-run__obj">{{ r.objective }}</div>
        <div class="fleet-run__meta mono">
          <span :class="`is-${r.status}`">{{ RUN_TEXT[r.status] ?? r.status }}</span>
          <span>{{ r.doneCount }}/{{ r.taskCount }} 任务</span>
          <span v-if="r.unread" class="fleet-run__q">{{ r.unread }} 个待答</span>
          <span>{{ ago(r.createdAt) }}</span>
        </div>
      </button>
      <div v-if="runs.length === 0" class="fleet-runs__empty">
        还没有协调任务。把一个大目标交给协调者 agent，它会拆任务、在各自的 worktree 里派 worker，并把结果收回来。
      </div>
    </aside>

    <section class="fleet-runs__detail">
      <template v-if="creating">
        <h3>新建协调任务</h3>
        <div class="fleet-runs__form">
          <q-select
            v-model="form.projectId"
            dense
            outlined
            emit-value
            map-options
            label="项目"
            :options="projectOptions"
          />
          <q-input
            v-model="form.objective"
            dense
            outlined
            autogrow
            type="textarea"
            label="目标"
            placeholder="例如：给 office 模块补单元测试，并修复测试中发现的问题"
          />
          <div class="fleet-runs__row">
            <q-select
              v-model="form.agent"
              class="col"
              dense
              outlined
              emit-value
              map-options
              label="协调者 agent"
              :options="agentOptions"
            />
            <q-toggle v-model="form.fullAccess" dense label="完全放权（worker 同样生效）" />
          </div>
          <p class="fleet-runs__hint">
            协调者在项目主检出里启动，用 <code>fleetctl</code> 建任务、派 worker；每个 worker 在自己的 worktree
            和分支里干活，汇报或提问时会自动唤醒协调者。全部完成后由你审阅各分支再合并，不会自动推送。
          </p>
          <div class="fleet-runs__row">
            <q-space />
            <q-btn flat no-caps label="取消" @click="creating = false" />
            <q-btn
              unelevated
              no-caps
              color="primary"
              label="启动协调者"
              :loading="busy"
              :disable="!form.projectId || !form.objective.trim()"
              @click="create"
            />
          </div>
        </div>
      </template>

      <template v-else-if="detail">
        <header class="fleet-runs__dhead">
          <div>
            <h3>{{ detail.run.objective }}</h3>
            <div class="fleet-run__meta mono">
              <span :class="`is-${detail.run.status}`">{{ RUN_TEXT[detail.run.status] ?? detail.run.status }}</span>
              <span>协调者 {{ agentLabel(detail.run.agent) }}</span>
              <span>{{ detail.run.id }}</span>
            </div>
          </div>
          <q-space />
          <q-btn
            dense
            flat
            no-caps
            icon="terminal"
            label="协调者终端"
            :disable="!hasTerminal(detail.run.coordinatorTerminalId)"
            @click="focusTerminal(detail.run.coordinatorTerminalId)"
          />
          <q-btn
            v-if="detail.run.status === 'running'"
            dense
            flat
            no-caps
            color="negative"
            icon="stop"
            label="停止"
            @click="stop"
          />
        </header>

        <h4>任务（{{ detail.tasks.length }}）</h4>
        <table class="fleet-runs__table">
          <thead>
            <tr>
              <th>任务</th>
              <th>状态</th>
              <th>worker</th>
              <th>结果</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="t in detail.tasks" :key="t.id">
              <td>
                <div class="fleet-runs__ttitle">{{ t.title }}</div>
                <div class="fleet-runs__muted mono">
                  {{ t.id }}<template v-if="t.deps.length"> · 依赖 {{ t.deps.join(', ') }}</template>
                </div>
              </td>
              <td>
                <span class="fleet-runs__status" :class="`is-${t.status}`">{{ TASK_TEXT[t.status] ?? t.status }}</span>
              </td>
              <td>
                <button v-if="t.workerTerminalId" type="button" class="fleet-runs__link" @click="openWorker(t)">
                  <StatusGlyph :status="workerStatus(t.workerTerminalId)" />
                  {{ agentLabel(t.workerAgent) }} · {{ wsName(t.workerWorkspaceId) }}
                </button>
                <span v-else class="fleet-runs__muted">{{ t.workerWorkspaceId ? '创建中…' : '未派发' }}</span>
              </td>
              <td class="fleet-runs__summary">{{ t.summary }}</td>
            </tr>
            <tr v-if="detail.tasks.length === 0">
              <td colspan="4" class="fleet-runs__muted">协调者还没有建任务。</td>
            </tr>
          </tbody>
        </table>

        <h4>收件箱</h4>
        <ol class="fleet-runs__msgs">
          <li
            v-for="m in detail.messages"
            :key="m.id"
            class="fleet-msg"
            :class="{ 'is-question': m.type === 'question' && !m.answer }"
          >
            <div class="fleet-msg__head mono">
              <span>{{ MSG_TEXT[m.type] ?? m.type }}</span>
              <span>{{ m.from }}</span>
              <span>{{ ago(m.createdAt) }}</span>
            </div>
            <div class="fleet-msg__body">{{ m.body || m.subject }}</div>
            <div v-if="m.type === 'question'" class="fleet-msg__answer">
              <template v-if="m.answer">答复：{{ m.answer }}</template>
              <template v-else>
                <q-btn
                  v-for="o in m.options"
                  :key="o"
                  dense
                  outline
                  no-caps
                  size="sm"
                  color="primary"
                  :label="o"
                  @click="answer(m.id, o)"
                />
                <q-input
                  v-model="answers[m.id]"
                  dense
                  outlined
                  class="col"
                  placeholder="直接回答 worker"
                  @keydown.enter.prevent="answer(m.id, answers[m.id] ?? '')"
                />
                <q-btn
                  dense
                  unelevated
                  no-caps
                  size="sm"
                  color="primary"
                  label="回答"
                  :disable="!(answers[m.id] ?? '').trim()"
                  @click="answer(m.id, answers[m.id] ?? '')"
                />
              </template>
            </div>
          </li>
          <li v-if="detail.messages.length === 0" class="fleet-runs__muted">暂无消息。</li>
        </ol>
      </template>

      <div v-else class="fleet-runs__empty">选择左侧一个协调任务，或新建一个。</div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { Dialog, Notify } from 'quasar';
import * as api from '../api';
import type { Run, RunDetail, Task } from '../api';
import { ago, useFleet } from '../useFleet';
import StatusGlyph from './StatusGlyph.vue';

const { state, availableAgents, agentLabel, focusTerminal, selectWorkspace, refreshTerminals, refreshWorkspaces } =
  useFleet();

const RUN_TEXT: Record<string, string> = { running: '进行中', done: '已完成', stopped: '已停止' };
const TASK_TEXT: Record<string, string> = {
  pending: '等依赖',
  ready: '就绪',
  dispatched: '进行中',
  completed: '完成',
  failed: '失败',
  blocked: '受阻',
};
const MSG_TEXT: Record<string, string> = {
  worker_done: '汇报',
  question: '提问',
  answer: '答复',
  status: '状态',
  escalation: '升级',
};

const runs = ref<Run[]>([]);
const currentId = ref('');
const detail = ref<RunDetail | null>(null);
const creating = ref(false);
const busy = ref(false);
const answers = reactive<Record<string, string>>({});
const form = reactive({ projectId: '', objective: '', agent: '', fullAccess: false });

const projectOptions = computed(() =>
  state.projects.filter((p) => p.isGit).map((p) => ({ label: p.name, value: p.id })),
);
const agentOptions = computed(() => availableAgents.value.map((a) => ({ label: a.label, value: a.id })));
const wsName = (id: string) => state.workspaces.find((w) => w.id === id)?.name ?? '';
const hasTerminal = (id: string) => state.terminals.some((t) => t.id === id);
const workerStatus = (id: string) => state.terminals.find((t) => t.id === id)?.status ?? 'exited';

async function load() {
  try {
    runs.value = await api.listRuns();
    if (!currentId.value && runs.value[0] && !creating.value) currentId.value = runs.value[0].id;
    if (currentId.value) detail.value = await api.getRun(currentId.value);
  } catch {
    /* 轮询失败下一轮再试 */
  }
}

function select(id: string) {
  creating.value = false;
  currentId.value = id;
  void load();
}

function openNew() {
  creating.value = true;
  form.projectId =
    form.projectId || state.workspaces.find((w) => w.id === state.currentWs)?.projectId || state.projects[0]?.id || '';
  form.agent = form.agent || availableAgents.value[0]?.id || '';
}

async function create() {
  busy.value = true;
  try {
    const r = await api.createRun({ ...form, objective: form.objective.trim() });
    creating.value = false;
    form.objective = '';
    currentId.value = r.id;
    await Promise.all([load(), refreshTerminals()]);
    Notify.create({ type: 'positive', message: '协调者已启动' });
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  } finally {
    busy.value = false;
  }
}

function stop() {
  const run = detail.value?.run;
  if (!run) return;
  Dialog.create({
    title: '停止这个协调任务？',
    message: '会结束协调者和仍在干活的 worker 终端（终端标签保留，可以看输出）。已经提交到各分支的改动不受影响。',
    cancel: { label: '取消', flat: true },
    ok: { label: '停止', color: 'negative', unelevated: true },
  }).onOk(async () => {
    try {
      await api.stopRun(run.id);
      await Promise.all([load(), refreshTerminals()]);
    } catch (e) {
      Notify.create({ type: 'negative', message: api.errText(e) });
    }
  });
}

async function answer(messageId: string, text: string) {
  if (!text.trim()) return;
  try {
    await api.answerQuestion(messageId, text.trim());
    answers[messageId] = '';
    await load();
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  }
}

async function openWorker(t: Task) {
  await refreshWorkspaces();
  if (hasTerminal(t.workerTerminalId)) focusTerminal(t.workerTerminalId);
  else if (t.workerWorkspaceId) selectWorkspace(t.workerWorkspaceId);
}

let timer: ReturnType<typeof setInterval> | null = null;
onMounted(() => {
  void load();
  timer = setInterval(() => void load(), 3000);
});
onBeforeUnmount(() => timer && clearInterval(timer));
</script>

<style scoped lang="sass">
.fleet-runs
  display: flex
  height: 100%
  min-height: 0
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.fleet-runs__list
  display: flex
  flex-direction: column
  width: 320px
  flex: none
  overflow: auto
  padding: var(--nt-space-4) var(--nt-space-3)
  border-right: 1px solid var(--nt-border-1)
  gap: var(--nt-space-2)
.fleet-runs__bar
  display: flex
  align-items: center
.fleet-runs__count
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
.fleet-run
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-1)
  color: var(--nt-text-1)
  text-align: left
  cursor: pointer
  &.is-active
    border-color: var(--nt-accent-border)
    background: var(--nt-bg-2)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-run__obj
  display: -webkit-box
  overflow: hidden
  -webkit-line-clamp: 2
  -webkit-box-orient: vertical
  color: var(--nt-text-0)
  font-size: var(--nt-fs-body)
.fleet-run__meta
  display: flex
  flex-wrap: wrap
  gap: var(--nt-space-2)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
  .is-running
    color: var(--nt-accent)
  .is-done
    color: var(--nt-success)
.fleet-run__q
  color: var(--nt-warn)
.fleet-runs__empty
  padding: var(--nt-space-5)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
.fleet-runs__detail
  flex: 1
  min-width: 0
  overflow: auto
  padding: var(--nt-space-4) var(--nt-space-5)
  color: var(--nt-text-1)
  h3
    margin: 0 0 var(--nt-space-1)
    font-size: var(--nt-fs-panel)
    color: var(--nt-text-0)
  h4
    margin: var(--nt-space-4) 0 var(--nt-space-2)
    font-size: var(--nt-fs-body)
    color: var(--nt-text-2)
.fleet-runs__form
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
  max-width: 720px
.fleet-runs__row
  display: flex
  align-items: center
  gap: var(--nt-space-3)
.fleet-runs__hint
  margin: 0
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
  code
    font-family: var(--nt-font-mono)
    color: var(--nt-accent)
.fleet-runs__dhead
  display: flex
  align-items: flex-start
  gap: var(--nt-space-2)
.fleet-runs__table
  width: 100%
  border-collapse: collapse
  font-size: var(--nt-fs-caption)
  th
    padding: var(--nt-space-1) var(--nt-space-2)
    border-bottom: 1px solid var(--nt-border-2)
    color: var(--nt-text-3)
    font-weight: 500
    text-align: left
  td
    padding: var(--nt-space-2)
    border-bottom: 1px solid var(--nt-border-1)
    vertical-align: top
.fleet-runs__ttitle
  color: var(--nt-text-0)
.fleet-runs__muted
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.fleet-runs__status
  &.is-dispatched
    color: var(--nt-accent)
  &.is-completed
    color: var(--nt-success)
  &.is-failed
    color: var(--nt-danger)
  &.is-pending
    color: var(--nt-text-3)
.fleet-runs__link
  display: inline-flex
  align-items: center
  gap: var(--nt-space-1)
  border: 0
  background: transparent
  color: var(--nt-accent)
  cursor: pointer
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.fleet-runs__summary
  max-width: 360px
  color: var(--nt-text-2)
  white-space: pre-wrap
.fleet-runs__msgs
  margin: 0
  padding: 0
  list-style: none
.fleet-msg
  margin-bottom: var(--nt-space-2)
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-1)
  &.is-question
    border-color: var(--nt-warn)
.fleet-msg__head
  display: flex
  gap: var(--nt-space-2)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.fleet-msg__body
  margin-top: var(--nt-space-1)
  white-space: pre-wrap
.fleet-msg__answer
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-2)
  margin-top: var(--nt-space-2)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
</style>
