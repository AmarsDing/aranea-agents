<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero
      kicker="Coding Tasks"
      title="编码任务"
      subtitle="向外部编程工具派发任务：选项目、写描述、挑工具（默认自动路由），在看板里跟踪每一步。"
    >
      <template #actions>
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadTasks(true)" />
        <q-btn outline rounded color="primary" icon="settings" label="管理工具" @click="router.push('/settings/coding-tools')" />
      </template>
    </AppPageHero>

    <q-banner v-if="agents.length === 0 && !loading && !error" rounded class="app-banner-info q-mb-md">
      <template #avatar><q-icon name="terminal" color="primary" /></template>
      还没有可用的编码工具。请先注册一个外部编程 CLI，再回来派发任务。
      <template #action>
        <q-btn flat color="primary" label="去添加工具" @click="router.push('/settings/coding-tools')" />
      </template>
    </q-banner>

    <!-- 顶部发起条 -->
    <q-card flat class="app-registry-panel q-mb-md">
      <q-card-section>
        <div class="row q-col-gutter-sm items-center">
          <q-select
            v-model="dispatchProject"
            class="col-12 col-md-3"
            dense
            outlined
            emit-value
            map-options
            label="项目"
            :options="projectOptions"
          >
            <template #prepend><q-icon name="folder_open" /></template>
          </q-select>
          <q-input
            v-model="dispatchPrompt"
            class="col-12 col-md"
            dense
            outlined
            label="任务描述"
            placeholder="想让它做什么？例如：修复登录页校验失败的问题"
            @keyup.enter="dispatch"
          />
          <q-select
            v-model="dispatchAgentKey"
            class="col-12 col-md-2"
            dense
            outlined
            emit-value
            map-options
            label="工具"
            :options="agentOptions"
          />
          <q-btn
            class="col-12 col-md-auto"
            color="primary"
            rounded
            unelevated
            icon="send"
            label="派发"
            :loading="dispatching"
            :disable="!dispatchProject || !dispatchPrompt.trim()"
            @click="dispatch"
          />
        </div>
        <q-expansion-item dense dense-toggle expand-separator label="高级选项" header-class="text-grey-7 q-pl-none" class="q-mt-xs">
          <div class="text-caption text-grey-7 q-pb-sm">
            所选工具的默认自治级别：{{ autonomyLabel(selectedAgent?.defaultAutonomy) }}。
            自治级别决定工具被允许的操作范围（只读浏览 / 修改工作区文件 / 执行任意命令）。
          </div>
        </q-expansion-item>
      </q-card-section>
    </q-card>

    <AppStatePanel
      v-if="pageState === 'error'"
      class="q-mb-md"
      variant="error"
      :description="error"
      action-label="重试"
      action-icon="refresh"
      @action="loadTasks()"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      variant="empty"
      title="还没有编码任务"
      description="在上方选择项目、输入任务描述，点击「派发」创建第一个编码任务。"
      icon="task_alt"
    />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        class="q-mb-md"
        variant="partial"
        dense
        :description="error"
        action-label="重试"
        action-icon="refresh"
        @action="loadTasks()"
      />

      <div class="row items-center justify-between q-mb-sm">
        <q-tabs v-model="viewMode" dense no-caps class="text-grey-8">
          <q-tab name="grouped" label="按工具分组" />
          <q-tab name="all" label="全部" />
        </q-tabs>
        <div class="row items-center q-gutter-sm">
          <q-checkbox
            dense
            data-test="select-all"
            :aria-label="t('shell.bulkBar.selectAllPage')"
            :model-value="codingPageAllSelected"
            @update:model-value="toggleAllCodingTasks"
          />
          <span class="text-caption text-grey-6">共 {{ tasks.length }} 个任务</span>
        </div>
      </div>

      <div class="coding-workspace" :class="{ 'coding-workspace--inspecting': inspectorOpen }">
      <div class="coding-workspace__main">
    <!-- 按工具分组泳道 -->
    <template v-if="viewMode === 'grouped'">
      <section v-for="lane in lanes" :key="lane.key" class="q-mb-md">
        <div class="row items-center q-gutter-sm q-mb-xs">
          <q-avatar size="28px" color="primary" text-color="white" class="text-weight-bold">
            <img v-if="lane.agent?.iconUrl" :src="lane.agent.iconUrl" :alt="lane.agent.displayName || lane.agent.agentKey" />
            <template v-else>{{ agentInitial(lane.agent) }}</template>
          </q-avatar>
          <span class="text-weight-bold">{{ lane.agent?.displayName || lane.key }}</span>
          <q-badge color="grey-4" text-color="grey-9" :label="String(lane.tasks.length)" />
        </div>
        <div v-if="lane.tasks.length === 0" class="text-caption text-grey-6 q-pl-lg q-pb-sm">暂无任务</div>
        <CodingTaskCard
          v-for="task in lane.tasks"
          :key="task.id"
          :task="task"
          :agent="lane.agent"
          :expanded="expandedIds.has(task.id ?? '')"
          :now="now"
          selectable
          :selected="isCodingTaskSelected(task.id || '')"
          @inspect="inspectTask"
          @toggle="toggleExpanded"
          @toggle-select="toggleCodingTaskSelected"
          @updated="loadTasks()"
          @redo="onRedo"
        />
      </section>
    </template>

    <!-- 全部 -->
    <template v-else>
      <CodingTaskCard
        v-for="task in sortedTasks"
        :key="task.id"
        :task="task"
        :agent="agentByKey(task.agentKey)"
        :expanded="expandedIds.has(task.id ?? '')"
        :now="now"
        selectable
        :selected="isCodingTaskSelected(task.id || '')"
        @inspect="inspectTask"
        @toggle="toggleExpanded"
        @toggle-select="toggleCodingTaskSelected"
        @updated="loadTasks()"
        @redo="onRedo"
      />
    </template>
      </div>
      <AppRegistryInspector
        v-model="inspectorOpen"
        :title="inspectedTask?.prompt?.split('\n')[0] || t('shell.registry.inspect')"
        :eyebrow="inspectedTask ? `TASK / ${inspectedTask.id || '—'}` : undefined"
      >
        <div v-if="inspectedTask" class="registry-inspector-stack">
          <q-badge :color="statusColor(inspectedTask.status)" text-color="white" :label="statusLabel(inspectedTask.status)" />
          <dl class="registry-facts">
            <div><dt>{{ t('shell.registry.identifier') }}</dt><dd class="mono">{{ inspectedTask.id || '—' }}</dd></div>
            <div><dt>{{ t('shell.registry.tool') }}</dt><dd>{{ agentByKey(inspectedTask.agentKey)?.displayName || inspectedTask.agentKey || '—' }}</dd></div>
            <div><dt>{{ t('shell.registry.project') }}</dt><dd>{{ inspectedTask.projectName || '—' }}</dd></div>
            <div><dt>{{ t('shell.registry.createdAt') }}</dt><dd class="mono">{{ formatDateTime(inspectedTask.createdAt) }}</dd></div>
          </dl>
          <section class="registry-inspector-section">
            <h3>{{ t('shell.registry.description') }}</h3>
            <p class="registry-inspector-description registry-inspector-preline">{{ inspectedTask.prompt || '—' }}</p>
          </section>
          <section v-if="inspectedTask.lastExcerpt" class="registry-inspector-section">
            <h3>{{ t('shell.registry.latestStep') }}</h3>
            <p class="registry-inspector-description">{{ inspectedTask.lastExcerpt }}</p>
          </section>
          <section v-if="inspectedTask.summary || inspectedTask.error" class="registry-inspector-section">
            <h3>{{ t('shell.registry.summary') }}</h3>
            <p class="registry-inspector-description">{{ inspectedTask.summary || inspectedTask.error }}</p>
          </section>
        </div>
      </AppRegistryInspector>
    </div>

    <BulkActionBar
      v-if="codingSelectedCount > 0"
      :count="codingSelectedCount"
      :total="tasks.length"
      :actions="codingBulkActions"
      :busy="codingBulkBusy"
      :error="codingBulkError"
      @run="runCodingBulk"
      @clear="clearCodingSelection"
    />
    </template>

    <!-- 候选项目选择 -->
    <AppDialog
      v-model="candidatesOpen"
      persistent
      size="sm"
      title="匹配到多个项目"
      subtitle="请选择一个项目重新派发。"
    >
      <q-list separator>
        <q-item v-for="c in candidates" :key="c.id" v-ripple clickable @click="pickCandidate(c)">
          <q-item-section avatar><q-icon name="folder_open" color="amber-8" /></q-item-section>
          <q-item-section>
            <q-item-label class="text-weight-medium">{{ c.name }}</q-item-label>
            <q-item-label caption class="text-grey-7">{{ c.path }}</q-item-label>
          </q-item-section>
          <q-item-section side><q-icon name="chevron_right" /></q-item-section>
        </q-item>
      </q-list>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import BulkActionBar from '../components/common/BulkActionBar.vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import CodingTaskCard from '../features/coding/CodingTaskCard.vue';
import { resolvePageState } from './pageState';
import {
  agentInitial,
  autonomyLabel,
  dispatchCodingTask,
  formatDateTime,
  isActiveStatus,
  listCodingAgents,
  listCodingProjects,
  listCodingTasks,
  parseTime,
  statusColor,
  statusLabel,
} from '../features/coding/api';
import type { CodingAgent, CodingTask, ProjectCandidate } from '../features/coding/api';
import { useCodingTasksBulk } from '../features/coding/useCodingTasksBulk';
import { isForbiddenApiError } from '../utils/kratosError';

const LAST_PROJECT_KEY = 'coding-tasks:last-project';

const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const { t } = useI18n();

const loading = ref(false);
const error = ref('');
/** 403 / PERMISSION_DENIED：页面渲染 forbidden 态，不提供重试。 */
const forbidden = ref(false);
const agents = ref<CodingAgent[]>([]);
const tasks = ref<CodingTask[]>([]);
const viewMode = ref<'grouped' | 'all'>('grouped');
const expandedIds = ref<Set<string>>(new Set());
const now = ref(Date.now());

const dispatchProject = ref('');
const dispatchPrompt = ref('');
const dispatchAgentKey = ref('');
const dispatching = ref(false);

/** 五态判定：错误优先于空态，有数据时失败降级为 partial 横幅。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: tasks.value.length,
    forbidden: forbidden.value,
  }),
);

const candidatesOpen = ref(false);
const candidates = ref<ProjectCandidate[]>([]);
const inspectorOpen = ref(false);
const inspectedTask = ref<CodingTask | null>(null);

let taskPollTimer: ReturnType<typeof setInterval> | null = null;
let clockTimer: ReturnType<typeof setInterval> | null = null;

const projectList = ref<Awaited<ReturnType<typeof listCodingProjects>>>([]);

const projectOptions = computed(() => {
  const last = dispatchProject.value;
  const items = [...projectList.value];
  // 最近使用的项目排在最前。
  items.sort((a, b) => (a.name === last ? -1 : b.name === last ? 1 : 0));
  return items.map((p) => ({ label: `${p.name}（${p.path}）`, value: p.name ?? '' }));
});

const agentOptions = computed(() => [
  { label: '自动', value: '' },
  ...agents.value
    .filter((a) => a.enabled)
    .map((a) => ({ label: a.displayName || a.agentKey || '', value: a.agentKey ?? '' })),
]);

const selectedAgent = computed(() => agents.value.find((a) => a.agentKey === dispatchAgentKey.value));

const sortedTasks = computed(() =>
  [...tasks.value].sort((a, b) => parseTime(b.createdAt) - parseTime(a.createdAt)),
);

const lanes = computed(() => {
  const result: { key: string; agent: CodingAgent | undefined; tasks: CodingTask[] }[] = [];
  for (const agent of agents.value.filter((a) => a.enabled)) {
    result.push({
      key: agent.agentKey ?? '',
      agent,
      tasks: sortedTasks.value.filter((t) => t.agentKey === agent.agentKey),
    });
  }
  // 兜底：任务里出现但不在启用工具列表中的 agentKey 也成组，避免任务被隐藏。
  const known = new Set(result.map((l) => l.key));
  const orphans = sortedTasks.value.filter((t) => !known.has(t.agentKey ?? ''));
  if (orphans.length > 0) {
    const byKey = new Map<string, CodingTask[]>();
    for (const t of orphans) {
      const k = t.agentKey ?? '';
      if (!byKey.has(k)) byKey.set(k, []);
      byKey.get(k)?.push(t);
    }
    for (const [key, list] of byKey) {
      result.push({ key, agent: agentByKey(key), tasks: list });
    }
  }
  return result;
});

function agentByKey(key: string | undefined): CodingAgent | undefined {
  return agents.value.find((a) => a.agentKey === key);
}

const {
  count: codingSelectedCount,
  pageAllSelected: codingPageAllSelected,
  busy: codingBulkBusy,
  error: codingBulkError,
  actions: codingBulkActions,
  isSelected: isCodingTaskSelected,
  toggleRow: toggleCodingTaskSelected,
  toggleAll: toggleAllCodingTasks,
  clear: clearCodingSelection,
  run: runCodingBulk,
} = useCodingTasksBulk(sortedTasks, loadTasks);

// 视图切换（按工具分组 / 全部）会改变可见任务集合，必须清空选择，避免误操作不可见任务。
watch(viewMode, () => clearCodingSelection());

function inspectTask(task: CodingTask) {
  inspectedTask.value = task;
  inspectorOpen.value = true;
}

const hasActiveTasks = computed(() => tasks.value.some((t) => isActiveStatus(t.status)));

async function loadTasks(manual = false) {
  loading.value = true;
  error.value = '';
  forbidden.value = false;
  try {
    tasks.value = await listCodingTasks(100);
    if (manual) $q.notify({ type: 'positive', message: `已刷新，共 ${tasks.value.length} 个任务`, timeout: 2000 });
  } catch (err) {
    error.value = err instanceof Error ? err.message : '任务加载失败';
    forbidden.value = isForbiddenApiError(err);
  } finally {
    loading.value = false;
  }
  syncTaskPolling();
}

async function loadMeta() {
  try {
    const [a, p] = await Promise.all([listCodingAgents(), listCodingProjects()]);
    agents.value = a;
    projectList.value = p;
    const last = localStorage.getItem(LAST_PROJECT_KEY) ?? '';
    if (!dispatchProject.value) {
      dispatchProject.value = p.find((x) => x.name === last)?.name ?? p[0]?.name ?? '';
    }
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载工具/项目失败';
  }
}

function syncTaskPolling() {
  if (shouldPoll() && !taskPollTimer) {
    taskPollTimer = setInterval(() => {
      if (document.hidden) return;
      if (!hasActiveTasks.value) {
        stopTaskPolling();
        return;
      }
      void loadTasks();
    }, 3000);
  } else if (!shouldPoll() && taskPollTimer) {
    stopTaskPolling();
  }
}

function shouldPoll(): boolean {
  return hasActiveTasks.value && !document.hidden;
}

function stopTaskPolling() {
  if (taskPollTimer) {
    clearInterval(taskPollTimer);
    taskPollTimer = null;
  }
}

function onVisibility() {
  if (document.hidden) {
    stopTaskPolling();
  } else {
    syncTaskPolling();
    if (hasActiveTasks.value) void loadTasks();
  }
}

async function dispatch() {
  if (!dispatchProject.value || !dispatchPrompt.value.trim()) return;
  dispatching.value = true;
  try {
    const res = await dispatchCodingTask({
      agentKey: dispatchAgentKey.value,
      projectQuery: dispatchProject.value,
      prompt: dispatchPrompt.value.trim(),
    });
    if (res.task?.id) {
      localStorage.setItem(LAST_PROJECT_KEY, dispatchProject.value);
      $q.notify({ type: 'positive', message: '任务已派发' });
      const id = res.task.id;
      dispatchPrompt.value = '';
      await loadTasks();
      expandedIds.value = new Set([...expandedIds.value, id]);
    } else if ((res.candidates ?? []).length > 0) {
      candidates.value = res.candidates ?? [];
      candidatesOpen.value = true;
    } else {
      $q.notify({ type: 'warning', message: '派发失败：未返回任务' });
    }
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '派发失败' });
  } finally {
    dispatching.value = false;
  }
}

async function pickCandidate(c: ProjectCandidate) {
  candidatesOpen.value = false;
  if (!c.name) return;
  dispatchProject.value = c.name;
  await dispatch();
}

function toggleExpanded(id: string | undefined) {
  if (!id) return;
  const next = new Set(expandedIds.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  expandedIds.value = next;
}

function onRedo(task: CodingTask) {
  dispatchPrompt.value = `${task.prompt ?? ''}\n\n打回意见：${task.reviewComment ?? ''}`.trim();
  if (task.projectName) dispatchProject.value = task.projectName;
  if (task.agentKey) dispatchAgentKey.value = task.agentKey;
  $q.notify({ type: 'info', message: '已把原任务描述与打回意见预填回发起条' });
}

onMounted(async () => {
  await loadMeta();
  await loadTasks();
  const focus = typeof route.query.focus === 'string' ? route.query.focus : '';
  if (focus && tasks.value.some((t) => t.id === focus)) {
    expandedIds.value = new Set([focus]);
  }
  clockTimer = setInterval(() => {
    now.value = Date.now();
  }, 1000);
  document.addEventListener('visibilitychange', onVisibility);
});

onBeforeUnmount(() => {
  stopTaskPolling();
  if (clockTimer) clearInterval(clockTimer);
  document.removeEventListener('visibilitychange', onVisibility);
});
</script>
