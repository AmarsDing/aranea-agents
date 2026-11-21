<template>
  <!--
    工作台 › 对话：统一入口。界面上都是对话；执行层面每段对话都是一个任务——一件事交给了哪些执行者（同事、流程、编码工具），每次执行是一段过程，简单的聊天也一样。
    左：任务列表（搜索、视图页签、执行者胶囊、项目 / 排序）；右：任务详情（执行链 + 当前执行的对话）。
    设计：docs/aranea-agents/design/2026-11-17-三大模块重设计/07-会话中心.md §10
  -->
  <q-page
    class="app-standard-page cc-page"
    @dragenter="onDragEnter"
    @dragover.prevent
    @dragleave="onDragLeave"
    @drop.prevent="onDrop"
  >
    <Teleport defer to="#app-tab-actions">
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="add"
        :label="t('cc.compose.newBtn')"
        :title="t('cc.compose.newKey')"
        @click="openNew()"
      />
    </Teleport>
    <div v-if="dragging" class="cc-dropall" aria-hidden="true">
      <div>{{ t(composer?.mode === 'new' ? 'cc.compose.dropAdd' : 'cc.compose.dropNew') }}</div>
    </div>
    <section class="cc-panel cc-list" :class="{ 'cc-list--picking': picked.size }" :aria-label="t('cc.listLabel')">
      <div class="cc-lh">
        <label class="cc-search">
          <q-icon name="search" size="18px" aria-hidden="true" />
          <span class="cc-sr">{{ t('cc.search') }}</span>
          <input ref="searchEl" v-model.trim="q" type="search" autocomplete="off" :placeholder="t('cc.search')" />
          <kbd>/</kbd>
        </label>
        <nav class="cc-views" :aria-label="t('cc.viewsLabel')">
          <button
            v-for="v in MAIN_VIEWS"
            :key="v"
            type="button"
            :class="{ 'cc-views--on': view === v && !q }"
            :aria-pressed="view === v && !q"
            @click="setView(v)"
          >
            {{ t(`cc.view.${v}`) }}<span class="cc-num" :class="countTone(v)">{{ counts[v] }}</span>
          </button>
          <q-btn-dropdown
            flat
            dense
            no-caps
            class="cc-views__more"
            :class="{ 'cc-views--on': MORE_VIEWS.includes(view) && !q }"
            :label="MORE_VIEWS.includes(view) ? `${t(`cc.view.${view}`)} ${counts[view]}` : t('cc.view.more')"
          >
            <q-list dense>
              <q-item v-for="v in MORE_VIEWS" :key="v" v-close-popup clickable @click="setView(v)">
                <q-item-section>{{ t(`cc.view.${v}`) }}</q-item-section>
                <q-item-section side class="cc-num">{{ counts[v] }}</q-item-section>
              </q-item>
            </q-list>
          </q-btn-dropdown>
        </nav>
        <div class="cc-fl" :class="{ 'cc-fl--on': executor }">
          <button
            v-for="g in executors"
            :key="g.key"
            type="button"
            class="cc-xf"
            :class="{ 'cc-xf--on': executor === g.key }"
            :aria-pressed="executor === g.key"
            :title="t('cc.executorCount', { name: g.name, n: g.count })"
            @click="executor = executor === g.key ? '' : g.key"
          >
            <CcGlyph
              :executor="g.key === 'ag' ? 'agent:' : g.key"
              :name="g.key === 'ag' ? t('cc.platformMono') : g.name"
              size="xs"
            />{{ g.count }}
          </button>
          <span class="cc-xf cc-xf--off" :title="t('cc.traeOff')">
            <CcGlyph executor="tr" name="Trae" size="xs" />
          </span>
          <span class="cc-fl__tools">
            <label
              class="cc-sel"
              :class="{ 'cc-sel--on': project }"
              :title="t('cc.projectTitle', { p: project || t('cc.allProjects') })"
            >
              <q-icon name="folder_open" size="18px" aria-hidden="true" />
              <select v-model="project" :aria-label="t('cc.projectLabel')">
                <option value="">{{ t('cc.allProjects') }}</option>
                <option v-for="p in projects" :key="p" :value="p">{{ p }}</option>
              </select>
            </label>
            <label class="cc-sel" :class="{ 'cc-sel--on': sort !== 'last' }" :title="t(`cc.sort.${sort}`)">
              <q-icon name="sort" size="18px" aria-hidden="true" />
              <select v-model="sort" :aria-label="t('cc.sortLabel')">
                <option v-for="s in SORTS" :key="s" :value="s">{{ t(`cc.sort.${s}`) }}</option>
              </select>
            </label>
          </span>
        </div>
        <div v-if="chips.length" class="cc-chips">
          <span v-for="c in chips" :key="c.key" class="cc-chip"
            >{{ c.label
            }}<button type="button" :aria-label="t('cc.unset', { what: c.label })" @click="unset(c.key)">
              <q-icon name="close" size="14px" /></button
          ></span>
        </div>
        <p v-if="cc.errors.value.length" class="cc-warn" role="status">
          {{ t('cc.partial', { sources: cc.errors.value.map((s) => t(`cc.sourceName.${s}`)).join('、') }) }}
        </p>
      </div>

      <div ref="rowsEl" class="cc-lb" role="listbox" :aria-label="t('cc.listLabel')" tabindex="0">
        <p v-if="!cc.loaded.value" class="cc-empty">{{ t('cc.loading') }}</p>
        <div v-else-if="!visible.length" class="cc-empty">
          <p>{{ t(q ? 'cc.noMatch' : view === 'now' ? 'cc.emptyNow' : 'cc.empty') }}</p>
          <q-btn
            v-if="!q && view === 'now' && counts.all"
            flat
            dense
            no-caps
            :label="t('cc.showAll', { n: counts.all })"
            @click="setView('all')"
          />
          <q-btn
            v-if="!q && !counts.all"
            flat
            dense
            no-caps
            icon="add"
            :label="t('cc.compose.newBtn')"
            @click="openNew()"
          />
        </div>
        <template v-for="g in grouped" v-else :key="g.key">
          <div class="cc-grp">
            <b>{{ t(`cc.group.${g.key}`) }}</b
            ><span class="cc-num">{{ g.items.length }}</span>
          </div>
          <div
            v-for="row in g.items"
            :id="`cc-row-${row.id}`"
            :key="row.id"
            class="cc-row"
            :class="{ 'cc-row--on': row.id === selId, 'cc-row--picked': picked.has(row.id) }"
            role="option"
            :aria-selected="row.id === selId"
            @click="onRow(row, $event)"
          >
            <span class="cc-row__lead">
              <span class="cc-dot" :class="`cc-dot--${taskStatus(row)}`" />
              <input
                type="checkbox"
                class="cc-row__check"
                :checked="picked.has(row.id)"
                :aria-label="t('cc.pick', { title: row.title })"
                @click.stop="togglePick(row.id)"
              />
            </span>
            <div class="cc-row__main">
              <div class="cc-row__top">
                <span class="cc-row__title">{{ row.title }}</span>
                <q-icon
                  v-if="taskPinned(row)"
                  name="star"
                  size="14px"
                  class="cc-row__pin"
                  :aria-label="t('cc.pinned')"
                />
                <span class="cc-sr">{{ t(`cc.status.${taskStatus(row)}`) }}</span>
                <span class="cc-row__time cc-num">{{ when(taskLast(row)) }}</span>
              </div>
              <div v-if="keyLine(row).text" class="cc-row__line" :class="`cc-row__line--${keyLine(row).tone}`">
                {{ keyLine(row).text }}
              </div>
              <div class="cc-row__foot">
                <span class="cc-stack">
                  <CcGlyph
                    v-for="e in uniqueExecutors(row).slice(0, 4)"
                    :key="e.executor"
                    :executor="e.executor"
                    :name="e.executorName"
                    size="xs"
                  />
                  <span v-if="uniqueExecutors(row).length > 4" class="cc-stack__more"
                    >+{{ uniqueExecutors(row).length - 4 }}</span
                  >
                </span>
                <span class="cc-row__meta">{{ rowMeta(row) }}</span>
              </div>
            </div>
          </div>
        </template>
      </div>

      <div v-if="picked.size" class="cc-batch" role="toolbar" :aria-label="t('cc.batchLabel')">
        <b>{{ t('cc.picked', { n: picked.size }) }}</b>
        <q-btn
          v-if="!MORE_VIEWS.includes(view) || view === 'pinned'"
          flat
          dense
          no-caps
          icon="archive"
          :label="t('cc.action.archive')"
          :disable="busy"
          @click="archiveTasks(pickedTasks)"
        />
        <q-btn
          v-else
          flat
          dense
          no-caps
          :label="t('cc.action.restore')"
          :disable="busy"
          @click="restoreTasks(pickedTasks)"
        />
        <q-btn
          v-if="view !== 'trash'"
          flat
          dense
          no-caps
          icon="delete_outline"
          :label="t('cc.action.delete')"
          :disable="busy"
          @click="deleteTasks(pickedTasks)"
        />
        <q-btn flat dense no-caps :label="t('cc.action.cancel')" @click="picked = new Set()" />
      </div>
    </section>

    <section class="cc-panel cc-right">
      <!-- 新任务 / 交给下一位：占右栏；新任务切走时只是藏起来，草稿留着 -->
      <CcComposer
        v-if="composer"
        v-show="composerOpen"
        ref="composerEl"
        :key="composer.key"
        :mode="composer.mode"
        :task="composer.mode === 'handoff' ? task : undefined"
        :files="composer.files"
        :to="composer.to"
        :prefer="composer.prefer"
        :reject="composer.reject"
        :recent="recent"
        :inputs="taskFiles.inputs.value"
        :outputs="taskFiles.outputs.value"
        :can-fleet="cc.canFleet.value"
        :presets="cc.presets.value"
        :workspaces="cc.workspaces.value"
        @close="composer = null"
        @sent="onSent"
      />
      <CcTaskDetail
        v-if="task && !(composer && composerOpen)"
        :task="task"
        :exec-id="execId"
        :tab="tab"
        :busy="busy"
        @select-exec="selectExec"
        @tab="tab = $event"
        @handover="handover"
        @approve="approve"
        @reject="reject"
        @pin="pin"
        @archive="archiveTasks([task])"
        @restore="restoreTasks([task])"
        @delete="deleteTasks([task])"
      >
        <template #files>
          <CcFilesView
            :task="task"
            :inputs="taskFiles.inputs.value"
            :outputs="taskFiles.outputs.value"
            :loading="taskFiles.loading.value"
            :failed="taskFiles.failed.value"
          />
        </template>
        <!-- 本平台会话：嵌入原来的对话引擎（只留对话区），会话由地址里的 session / agent 决定；一旦打开就留着，切走只是隐藏 -->
        <div v-if="chatMounted" v-show="exec?.kind === 'session'" class="cc-chat">
          <ChatWorkspace embedded />
        </div>
        <CcBridgeView v-if="exec?.kind === 'bridge'" :exec="exec" @changed="cc.refresh" />
        <CcTerminalView v-else-if="exec?.kind === 'terminal'" :exec="exec" />
        <CcFlowView v-else-if="exec?.kind === 'flow'" :exec="exec" @changed="cc.refresh" />
      </CcTaskDetail>
      <div v-else-if="!(composer && composerOpen)" class="cc-empty cc-empty--detail">
        <p>{{ t(cc.loaded.value ? 'cc.pickOne' : 'cc.loading') }}</p>
        <q-btn
          v-if="cc.loaded.value"
          flat
          dense
          no-caps
          icon="add"
          :label="t('cc.compose.newBtn')"
          @click="openNew()"
        />
      </div>
    </section>
  </q-page>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';
import { Notify } from 'quasar';
import ChatWorkspace from '../features/chat/ChatWorkspace.vue';
import { useProMode } from '../features/prefs/proMode';
import CcBridgeView from '../features/conversations/components/CcBridgeView.vue';
import CcFlowView from '../features/conversations/components/CcFlowView.vue';
import CcComposer, { type ComposerPick } from '../features/conversations/components/CcComposer.vue';
import CcFilesView from '../features/conversations/components/CcFilesView.vue';
import CcGlyph from '../components/conversations/CcGlyph.vue';
import CcTaskDetail, { type DetailTab } from '../components/conversations/CcTaskDetail.vue';
import CcTerminalView from '../features/conversations/components/CcTerminalView.vue';
import { useTaskFiles } from '../features/conversations/useTaskFiles';
import { confirm } from '../composables/useAppConfirm';
import {
  KNOWN_TOOLS,
  defaultExecution,
  executorGroup,
  flat,
  fromSession,
  inView,
  matches,
  needsReview,
  sameDay,
  sortTasks,
  sumOf,
  taskBucket,
  taskLast,
  taskPinned,
  taskStatus,
  type ConvTask,
  type Execution,
  type View,
} from '../features/conversations/model';
import { useConversationCenter, type ActionResult } from '../features/conversations/useConversationCenter';
import { getSession } from '../features/session/api';
import { dayOrClock } from '../features/today/logic';
import { parseKratosApiError } from '../utils/kratosError';

const MAIN_VIEWS: View[] = ['now', 'waiting', 'today', 'all'];
const MORE_VIEWS: View[] = ['pinned', 'archived', 'trash'];
const SORTS = ['last', 'cost', 'turns'] as const;

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const cc = useConversationCenter();

const view = ref<View>('now');
const q = ref('');
const executor = ref('');
const project = ref('');
const sort = ref<(typeof SORTS)[number]>('last');
const selId = ref('');
const execId = ref('');
const { pro } = useProMode();
const tab = ref<DetailTab>('talk');
const picked = ref(new Set<string>());
const busy = ref(false);
const chatMounted = ref(false);
const searchEl = ref<HTMLInputElement | null>(null);
/** 从别处带着 ?session= 进来、但不在已加载列表里的会话，临时补成一个任务 */
const extra = ref<ConvTask[]>([]);

// 每分钟推进一次「现在」，驱动今天 / 时间显示
const now = ref(new Date());
let clockTimer: ReturnType<typeof setInterval> | null = null;

const allTasks = computed(() => [
  ...cc.tasks.value,
  ...extra.value.filter((x) => !cc.tasks.value.some((t) => t.id === x.id)),
]);
const counts = computed(
  () =>
    Object.fromEntries(
      [...MAIN_VIEWS, ...MORE_VIEWS].map((v) => [v, allTasks.value.filter((t) => inView(t, v, now.value)).length]),
    ) as Record<View, number>,
);
const countTone = (v: View) =>
  v === 'waiting' && counts.value.waiting ? 'cc-num--warn' : v === 'now' && counts.value.now ? 'cc-num--run' : '';
// 搜索时不受视图限制（找回的东西可能已经归档），只排除回收站
const pool = computed(() =>
  allTasks.value.filter(
    (t) =>
      (q.value ? taskBucket(t) !== 'deleted' && matches(t, q.value) : inView(t, view.value, now.value)) &&
      (!project.value || t.project === project.value),
  ),
);
const usesExecutor = (t: ConvTask, key: string) => flat(t).some((e) => executorGroup(e.executor) === key);
const visible = computed(() =>
  sortTasks(
    pool.value.filter((t) => !executor.value || usesExecutor(t, executor.value)),
    sort.value,
  ),
);

// 执行者胶囊：只列当前范围里出现过的；本平台智能体在前，然后是常见编码工具，其余按名字
const executors = computed(() => {
  const seen = new Map<string, { key: string; name: string; count: number }>();
  for (const task of pool.value) {
    const keys = new Set(flat(task).map((e) => executorGroup(e.executor)));
    for (const key of keys) {
      const e = flat(task).find((x) => executorGroup(x.executor) === key)!;
      const name = key === 'ag' ? t('cc.platform') : key === 'flow' ? t('cc.flow.group') : e.executorName;
      const item = seen.get(key) ?? { key, name, count: 0 };
      item.count++;
      seen.set(key, item);
    }
  }
  const order = ['ag', ...KNOWN_TOOLS];
  const rank = (k: string) => (order.includes(k) ? order.indexOf(k) : order.length);
  return [...seen.values()].sort((a, b) => rank(a.key) - rank(b.key) || a.name.localeCompare(b.name));
});
const projects = computed(() => [...new Set(allTasks.value.map((t) => t.project).filter(Boolean))].sort());

const chips = computed(() => {
  const out: { key: 'q' | 'executor' | 'project'; label: string }[] = [];
  if (q.value) out.push({ key: 'q', label: t('cc.chipSearch', { q: q.value }) });
  if (executor.value)
    out.push({ key: 'executor', label: executors.value.find((g) => g.key === executor.value)?.name ?? executor.value });
  if (project.value) out.push({ key: 'project', label: project.value });
  return out;
});
function unset(key: 'q' | 'executor' | 'project') {
  if (key === 'q') q.value = '';
  else if (key === 'executor') executor.value = '';
  else project.value = '';
}

// 分组：进行中 · 等你 / 今天 / 昨天 / 本周 / 更早
function groupOf(task: ConvTask): string {
  if (!MORE_VIEWS.includes(view.value) && ['waiting', 'running'].includes(taskStatus(task))) return 'live';
  const last = taskLast(task);
  if (sameDay(last, now.value)) return 'today';
  const days = (now.value.getTime() - Date.parse(last)) / 864e5;
  return days < 2 ? 'yesterday' : days < 7 ? 'week' : 'older';
}
const grouped = computed(() => {
  const out: { key: string; items: ConvTask[] }[] = [];
  for (const task of visible.value) {
    const key = groupOf(task);
    const g = out.find((x) => x.key === key);
    if (g) g.items.push(task);
    else out.push({ key, items: [task] });
  }
  return out;
});

const task = computed(() => allTasks.value.find((x) => x.id === selId.value));
const exec = computed<Execution | undefined>(() =>
  task.value ? flat(task.value).find((e) => e.id === execId.value) : undefined,
);
const pickedTasks = computed(() => allTasks.value.filter((x) => picked.value.has(x.id)));

const when = (iso: string) => (iso ? dayOrClock(iso, now.value) : '');
function keyLine(task: ConvTask): { text: string; tone: string } {
  const xs = flat(task);
  const waiting = xs.find((e) => e.status === 'waiting');
  if (waiting)
    return {
      text: t('cc.waitingLine', { who: waiting.executorName, what: t(`cc.reason.${waiting.reason || 'input'}`) }),
      tone: 'wait',
    };
  const latest = [...xs].sort((a, b) => a.last.localeCompare(b.last)).pop();
  if (needsReview(task)) return { text: t('cc.review.line', { who: latest?.executorName ?? '' }), tone: 'wait' };
  if (taskStatus(task) === 'failed') return { text: latest?.detail || t('cc.status.failed'), tone: 'fail' };
  return { text: latest?.detail || '', tone: '' };
}
function uniqueExecutors(task: ConvTask) {
  const seen = new Set<string>();
  return flat(task).filter((e) => !seen.has(e.executor) && seen.add(e.executor));
}
function rowMeta(task: ConvTask) {
  const xs = flat(task);
  const who = uniqueExecutors(task).length === 1 ? xs[0]!.executorName : t('cc.meta.execs', { n: xs.length });
  const turns = sumOf(task, 'turns');
  const cost = sumOf(task, 'costUsd');
  return [
    who,
    task.project,
    pro.value && turns ? t('cc.meta.turns', { n: turns }) : '',
    pro.value && cost ? `$${cost.toFixed(2)}` : '',
  ]
    .filter(Boolean)
    .join(' · ');
}

// ——— 选择 ———
function selectTask(id: string) {
  selId.value = id;
  const target = allTasks.value.find((x) => x.id === id);
  selectExec(target ? (defaultExecution(target)?.id ?? '') : '');
  tab.value = 'talk';
}
function selectExec(id: string) {
  execId.value = id;
  const e = task.value ? flat(task.value).find((x) => x.id === id) : undefined;
  if (e?.kind !== 'session') return;
  chatMounted.value = true;
  // 嵌入的对话引擎看地址里的 session / agent 决定显示哪段会话
  if (route.query.session !== e.refId)
    void router.replace({ query: { ...route.query, session: e.refId, agent: e.agentId || undefined } });
}
function onRow(task: ConvTask, event: MouseEvent) {
  if (event.shiftKey || picked.value.size) return togglePick(task.id);
  // 新任务的草稿藏起来留着；交给下一位是跟着当前任务的，换任务就不要了
  if (composer.value?.mode === 'handoff') composer.value = null;
  composerOpen.value = false;
  selectTask(task.id);
}
function togglePick(id: string) {
  const next = new Set(picked.value);
  if (!next.delete(id)) next.add(id);
  picked.value = next;
}
function setView(v: View) {
  view.value = v;
  q.value = '';
  picked.value = new Set();
}
// 选中的任务不在当前范围里了（切视图、归档了）→ 换成第一个
watch(visible, (list) => {
  if (!list.some((x) => x.id === selId.value)) selectTask(list[0]?.id ?? '');
  else if (task.value && !flat(task.value).some((e) => e.id === execId.value))
    selectExec(defaultExecution(task.value)?.id ?? '');
});
watch(view, (v) => {
  if (v === 'archived') void cc.ensureBucket('archived');
  if (v === 'trash') void cc.ensureBucket('deleted');
});
watch(q, (v) => {
  if (v) void cc.ensureBucket('archived');
});

// 从别处带着 ?session= 进来：选中它所在的任务；列表里没有就临时补一个
const linked = typeof route.query.session === 'string' ? route.query.session : '';
const stopLinkWatch = watch(
  () => cc.loaded.value,
  async (ok) => {
    if (!ok || !linked) return;
    stopLinkWatch();
    // 派给流程 / 编码工具的任务，登记的会话是资料袋（不算一次执行）：按任务的锚点认
    let found = allTasks.value.find((x) => x.anchorId === linked || flat(x).some((e) => e.id === `session:${linked}`));
    if (!found) {
      try {
        const s = await getSession(linked);
        const e = fromSession(s, { agentNames: cc.agentNames.value, teamNames: cc.teamNames.value });
        found = { id: `auto:${e.id}`, title: e.what || e.executorName, origin: 'auto', project: '', chain: [e] };
        extra.value = [found];
        if (e.bucket !== 'active') view.value = e.bucket === 'archived' ? 'archived' : 'trash';
        else view.value = 'all';
      } catch {
        return;
      }
    } else if (!visible.value.some((x) => x.id === found!.id)) {
      view.value = 'all';
    }
    await nextTick();
    // 这段会话本身是一次执行就选它；否则（资料袋）选任务里该看的那次执行
    if (flat(found).some((e) => e.id === `session:${linked}`)) {
      selId.value = found.id;
      selectExec(`session:${linked}`);
    } else selectTask(found.id);
  },
);

// 从任务页、看板带着 ?task= 进来（没有承载会话的任务，如派给流程 / 运行台的）：选中这条任务
const linkedTask = typeof route.query.task === 'string' ? route.query.task : '';
if (linkedTask) {
  const stop = watch(
    () => cc.loaded.value && allTasks.value.some((x) => x.id === linkedTask),
    async (ok) => {
      if (!ok) return;
      stop();
      if (!visible.value.some((x) => x.id === linkedTask)) view.value = 'all';
      await nextTick();
      selectTask(linkedTask);
    },
  );
}

// ——— 操作 ———
function report(kind: 'archive' | 'restore' | 'delete', r: ActionResult, undo?: () => void) {
  const parts = [t(`cc.done.${kind}`, { n: r.done })];
  if (r.skipped) parts.push(t('cc.done.skipped', { n: r.skipped }));
  if (r.failed) parts.push(t('cc.done.failed', { n: r.failed }));
  Notify.create({
    type: r.failed ? 'warning' : 'positive',
    message: parts.join('；'),
    actions: undo && r.done ? [{ label: t('cc.action.undo'), color: 'white', handler: undo }] : [],
  });
}
async function guarded<T>(fn: () => Promise<T>): Promise<T | undefined> {
  busy.value = true;
  try {
    return await fn();
  } catch (err) {
    Notify.create({ type: 'negative', message: parseKratosApiError(err).message });
    return undefined;
  } finally {
    busy.value = false;
  }
}
async function archiveTasks(list: ConvTask[]) {
  const r = await guarded(() => cc.archive(list));
  picked.value = new Set();
  if (r) report('archive', r, () => void cc.restore(list));
}
async function restoreTasks(list: ConvTask[]) {
  const r = await guarded(() => cc.restore(list));
  picked.value = new Set();
  if (r) report('restore', r);
}
async function deleteTasks(list: ConvTask[]) {
  const xs = list.flatMap(flat);
  const n = (kind: Execution['kind']) => xs.filter((e) => e.kind === kind).length;
  const live = xs.filter((e) => e.kind === 'terminal' && (e.status === 'running' || e.status === 'waiting')).length;
  const lines = [
    n('session') ? t('cc.confirm.session', { n: n('session') }) : '',
    n('terminal') ? t('cc.confirm.terminal', { n: n('terminal') }) : '',
    live ? t('cc.confirm.live', { n: live }) : '',
    n('bridge') ? t('cc.confirm.bridge', { n: n('bridge') }) : '',
    n('flow') ? t('cc.confirm.flow', { n: n('flow') }) : '',
  ].filter(Boolean);
  const ok = await confirm({
    title: t('cc.confirm.title', { n: list.length, execs: xs.length }),
    message: lines.join('\n'),
    tone: 'danger',
    confirmLabel: t('cc.action.delete'),
  });
  if (!ok) return;
  const r = await guarded(() => cc.remove(list));
  picked.value = new Set();
  if (r) report('delete', r, () => void cc.restore(list));
}
async function pin() {
  if (task.value) await guarded(() => cc.togglePin(task.value!));
}
// ——— 新任务 / 交给下一位（右栏面板） ———
type ComposerState = {
  key: number;
  mode: 'new' | 'handoff';
  files?: File[];
  to?: ComposerPick;
  /** 预选的执行者：`agent:<id>` / `team:<id>` / 工具键 */
  prefer?: string;
  reject?: boolean;
};
const composer = ref<ComposerState | null>(null);
const composerOpen = ref(false);
const composerEl = ref<InstanceType<typeof CcComposer> | null>(null);
let composerSeq = 0;
const taskFiles = useTaskFiles(task);
// 最近用过的同事：任务里出现过的本平台智能体 / 团队，按最近动静排
const recent = computed(() => {
  const seen = new Map<string, string>();
  for (const e of allTasks.value.flatMap(flat))
    if (executorGroup(e.executor) === 'ag' && e.last > (seen.get(e.executor) ?? '')) seen.set(e.executor, e.last);
  return [...seen.entries()].sort((a, b) => b[1].localeCompare(a[1])).map(([k]) => k);
});

function openNew(files?: File[]) {
  // 藏起来的新任务草稿还在就接着写
  if (composer.value?.mode === 'new') {
    if (files?.length) composerEl.value?.addFiles(files);
  } else composer.value = { key: ++composerSeq, mode: 'new', files };
  composerOpen.value = true;
}
function openHandoff(to: ComposerPick, opts: { reject?: boolean; prefer?: string } = {}) {
  if (!task.value) return;
  composer.value = { key: ++composerSeq, mode: 'handoff', to, ...opts };
  composerOpen.value = true;
}
function handover(to: 'spirit' | 'agent' | 'tool') {
  openHandoff(to);
}
async function onSent(r: { anchorId: string; sessionId: string }) {
  composer.value = null;
  composerOpen.value = false;
  await cc.refresh();
  const hit = allTasks.value.find(
    (x) => x.anchorId === r.anchorId || flat(x).some((e) => e.kind === 'session' && e.refId === r.sessionId),
  );
  if (!hit) return;
  q.value = '';
  if (!visible.value.some((x) => x.id === hit.id)) view.value = 'all';
  await nextTick();
  selectTask(hit.id);
  if (r.sessionId && flat(hit).some((e) => e.id === `session:${r.sessionId}`)) selectExec(`session:${r.sessionId}`);
}
async function approve() {
  if (!task.value) return;
  const ok = await guarded(async () => {
    await cc.approve(task.value!);
    return true;
  });
  if (ok) Notify.create({ type: 'positive', message: t('cc.review.approved') });
}
// 退回重做：交回给同一位，输入框预填「退回意见」
function reject() {
  const last = task.value ? [...flat(task.value)].sort((a, b) => a.last.localeCompare(b.last)).pop() : undefined;
  if (!last) return;
  const group = executorGroup(last.executor);
  openHandoff(group === 'ag' ? 'agent' : group === 'flow' ? 'flow' : 'tool', { reject: true, prefer: last.executor });
}

// ——— 把文件拖到页面上：带着这些文件新建任务（新任务面板开着就加进去） ———
const dragging = ref(false);
let dragDepth = 0;
const hasFiles = (e: DragEvent) => Array.from(e.dataTransfer?.types ?? []).includes('Files');
function onDragEnter(e: DragEvent) {
  if (!hasFiles(e)) return;
  dragDepth++;
  dragging.value = true;
}
function onDragLeave(e: DragEvent) {
  if (!hasFiles(e)) return;
  dragDepth = Math.max(0, dragDepth - 1);
  if (!dragDepth) dragging.value = false;
}
function onDrop(e: DragEvent) {
  dragDepth = 0;
  dragging.value = false;
  const files = Array.from(e.dataTransfer?.files ?? []);
  if (files.length) openNew(files);
}

// ——— 键盘：/ 搜索，J / K 换任务，[ ] 换执行，X 勾选，E 归档，Esc 退出勾选 ———
function onKey(event: KeyboardEvent) {
  if (event.ctrlKey || event.metaKey || event.altKey) return;
  const tag = (document.activeElement as HTMLElement | null)?.tagName ?? '';
  if (/INPUT|TEXTAREA|SELECT/.test(tag) || (document.activeElement as HTMLElement | null)?.isContentEditable) {
    if (event.key === 'Escape' && document.activeElement === searchEl.value) searchEl.value?.blur();
    return;
  }
  // 对话区、新任务面板里的按键不抢
  if ((document.activeElement as HTMLElement | null)?.closest('.cc-chat, .cc-xview__term, .cc-np')) return;
  if (event.key === 'n') {
    event.preventDefault();
    openNew();
    return;
  }
  if (event.key === 'Escape' && composer.value && composerOpen.value) {
    composer.value = null;
    return;
  }
  const list = visible.value;
  const i = list.findIndex((x) => x.id === selId.value);
  const move = (d: number) => {
    const next = list[Math.min(list.length - 1, Math.max(0, i + d))];
    if (!next) return;
    selectTask(next.id);
    document.getElementById(`cc-row-${next.id}`)?.scrollIntoView({ block: 'nearest' });
  };
  if (event.key === '/') {
    event.preventDefault();
    searchEl.value?.focus();
  } else if (event.key === 'j' || event.key === 'ArrowDown') move(1);
  else if (event.key === 'k' || event.key === 'ArrowUp') move(-1);
  else if ((event.key === '[' || event.key === ']') && task.value) {
    const xs = flat(task.value);
    const j = xs.findIndex((e) => e.id === execId.value) + (event.key === ']' ? 1 : -1);
    if (xs[j]) selectExec(xs[j].id);
  } else if (event.key === 'x' && selId.value) togglePick(selId.value);
  else if (event.key === 'e' && task.value && taskBucket(task.value) === 'active') void archiveTasks([task.value]);
  else if (event.key === 'Escape' && picked.value.size) picked.value = new Set();
}
onMounted(() => {
  window.addEventListener('keydown', onKey);
  clockTimer = setInterval(() => (now.value = new Date()), 60_000);
});
onUnmounted(() => {
  window.removeEventListener('keydown', onKey);
  if (clockTimer) clearInterval(clockTimer);
});
</script>
