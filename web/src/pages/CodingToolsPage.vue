<template>
  <q-page class="app-standard-page app-registry-page">
    <AppPageHero
      kicker="Coding Agents"
      title="编码工具"
      subtitle="注册并管理外部编程 CLI 工具（ACP 子进程），维护可派发任务的项目目录白名单。"
    >
      <template #actions>
        <q-btn color="primary" rounded unelevated icon="add" label="添加工具" @click="openCreate" />
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadAll(true)" />
      </template>
    </AppPageHero>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadAll(true)" />
      </template>
    </q-banner>

    <!-- 工具列表 -->
    <q-card v-if="!loading && agents.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-icon name="terminal" size="48px" color="grey-5" />
        <div class="text-subtitle1 text-weight-bold">还没有编码工具</div>
        <div class="text-caption text-grey-7">
          注册一个外部编程 CLI（命令行工具），即可在「编码任务」看板中派发只读或改代码任务。
        </div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="添加第一个工具" @click="openCreate" />
      </q-card-section>
    </q-card>

    <q-card v-else flat class="app-registry-panel q-mb-lg">
      <q-list separator>
        <q-item v-for="agent in agents" :key="agent.agentKey" class="q-py-md">
          <q-item-section avatar>
            <q-avatar size="40px" color="primary" text-color="white" class="text-weight-bold">
              <img v-if="agent.iconUrl" :src="agent.iconUrl" :alt="agent.displayName || agent.agentKey" />
              <template v-else>{{ agentInitial(agent) }}</template>
            </q-avatar>
          </q-item-section>
          <q-item-section>
            <q-item-label class="text-weight-bold">
              {{ agent.displayName || agent.agentKey }}
              <span class="text-caption text-grey-6 q-ml-sm">{{ agent.agentKey }}</span>
            </q-item-label>
            <q-item-label caption class="text-grey-7 ellipsis">
              {{ agent.command }} {{ (agent.args || []).join(' ') }}
            </q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row items-center q-gutter-sm">
              <q-badge
                :color="probeBadge(agent).color"
                :text-color="probeBadge(agent).textColor"
                :label="probeBadge(agent).label"
                class="q-px-sm"
              >
                <q-tooltip v-if="agent.lastProbeError" max-width="320px">{{ agent.lastProbeError }}</q-tooltip>
              </q-badge>
              <q-toggle
                :model-value="!!agent.enabled"
                color="primary"
                :disable="togglingKey === agent.agentKey"
                :aria-label="agent.enabled ? '停用工具' : '启用工具'"
                @update:model-value="toggleEnabled(agent, $event)"
              />
              <q-btn flat dense round icon="edit" aria-label="编辑" @click="openEdit(agent)" />
              <q-btn
                flat
                dense
                round
                icon="network_check"
                aria-label="探测可用性"
                :loading="probingKey === agent.agentKey"
                @click="probe(agent)"
              />
              <q-btn flat dense round icon="play_arrow" color="primary" aria-label="试跑" @click="openTryRun(agent)" />
            </div>
          </q-item-section>
        </q-item>
      </q-list>
    </q-card>

    <!-- 项目目录 -->
    <q-card flat class="app-registry-panel">
      <q-card-section class="row items-center justify-between">
        <div>
          <div class="text-h6">项目目录</div>
          <div class="text-caption text-grey-7">可派发任务的本机项目（cwd 白名单），派发任务前请先在此登记。</div>
        </div>
        <q-btn color="primary" rounded unelevated dense icon="add" label="添加项目" @click="openProjectCreate" />
      </q-card-section>
      <q-separator />
      <q-card-section v-if="projects.length === 0" class="text-center text-grey-6 q-pa-lg">
        暂无项目，点击右上角「添加项目」登记本机目录。
      </q-card-section>
      <q-list v-else separator>
        <q-item v-for="p in projects" :key="p.id">
          <q-item-section avatar>
            <q-icon name="folder_open" color="amber-8" />
          </q-item-section>
          <q-item-section>
            <q-item-label class="text-weight-medium">{{ p.name }}</q-item-label>
            <q-item-label caption class="text-grey-7">{{ p.path }}</q-item-label>
            <q-item-label v-if="p.description" caption class="text-grey-6">{{ p.description }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-btn flat dense round icon="delete" color="negative" aria-label="删除项目" @click="confirmDeleteProject(p)" />
          </q-item-section>
        </q-item>
      </q-list>
    </q-card>

    <!-- 编辑/新增工具对话框 -->
    <q-dialog v-model="editorOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--xl app-glass-dialog">
        <q-card-section class="row items-start justify-between q-gutter-md">
          <div>
            <div class="text-h6">{{ editingAgent ? '编辑编码工具' : '添加编码工具' }}</div>
            <div class="text-caption text-grey-7">配置工具命令、环境变量、能力声明与默认自治级别。</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭" @click="editorOpen = false" />
        </q-card-section>
        <q-separator />
        <q-card-section class="mcp-form-scroll">
          <q-form class="app-form-field-grid app-form-field-grid--2col" @submit.prevent="saveAgent">
            <q-input v-model="form.agentKey" dense outlined label="工具标识 (agentKey) *" :readonly="!!editingAgent" :rules="[(v) => !!v || '必填']" />
            <q-input v-model="form.displayName" dense outlined label="显示名称" />
            <q-input v-model="form.command" dense outlined label="命令 (command) *" placeholder="npx" :rules="[(v) => !!v || '必填']" />
            <q-input v-model="form.argsText" dense outlined label="参数 (args)" hint="逗号分隔，例如 acp, --stdio" />
            <q-input v-model="form.iconUrl" dense outlined label="图标 URL" hint="留空时使用工具名首字母占位" />
            <q-select
              v-model="form.defaultAutonomy"
              dense
              outlined
              emit-value
              map-options
              label="默认自治级别"
              :options="AUTONOMY_OPTIONS"
            />
            <div class="app-grid-span-full">
              <div class="section-label q-mb-sm">能力声明</div>
              <div class="row q-gutter-md">
                <q-checkbox
                  v-for="opt in CAPABILITY_OPTIONS"
                  :key="opt.value"
                  v-model="form.capabilities"
                  :val="opt.value"
                  :label="opt.label"
                  dense
                />
              </div>
            </div>
            <div class="app-grid-span-full">
              <div class="row items-center justify-between q-mb-sm">
                <div class="section-label">环境变量</div>
                <q-btn flat dense color="primary" icon="add" label="添加一行" @click="form.envRows.push({ key: '', value: '', masked: false })" />
              </div>
              <div v-for="(row, i) in form.envRows" :key="i" class="row q-gutter-sm items-center q-mb-sm">
                <q-input v-model="row.key" dense outlined class="col-4" label="键" :readonly="row.masked" />
                <q-input
                  v-model="row.value"
                  dense
                  outlined
                  class="col"
                  label="值"
                  :readonly="row.masked"
                  :hint="row.masked ? '服务端已掩码，留空保持不变' : ''"
                />
                <q-btn flat dense round icon="close" aria-label="删除该行" @click="form.envRows.splice(i, 1)" />
              </div>
            </div>
            <q-toggle v-model="form.enabled" color="primary" label="启用" />
          </q-form>
        </q-card-section>
        <q-separator />
        <q-card-actions align="right" class="q-pa-md">
          <q-btn flat rounded label="取消" @click="editorOpen = false" />
          <q-btn color="primary" rounded unelevated label="保存" :loading="saving" @click="saveAgent" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 项目新增对话框 -->
    <q-dialog v-model="projectDialogOpen" persistent>
      <q-card class="app-dialog-card app-glass-dialog" style="min-width: 420px">
        <q-card-section class="row items-start justify-between">
          <div class="text-h6">添加项目</div>
          <q-btn flat dense round icon="close" aria-label="关闭" @click="projectDialogOpen = false" />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-form class="q-gutter-md" @submit.prevent="saveProject">
            <q-input v-model="projectForm.name" dense outlined label="项目名称 *" :rules="[(v) => !!v || '必填']" />
            <q-input v-model="projectForm.path" dense outlined label="本机路径 *" placeholder="F:\project\demo" :rules="[(v) => !!v || '必填']" />
            <q-input v-model="projectForm.description" dense outlined type="textarea" autogrow label="描述" />
          </q-form>
        </q-card-section>
        <q-card-actions align="right" class="q-pa-md">
          <q-btn flat rounded label="取消" @click="projectDialogOpen = false" />
          <q-btn color="primary" rounded unelevated label="保存" :loading="projectSaving" @click="saveProject" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 试跑：选择项目 -->
    <q-dialog v-model="tryRunOpen" persistent>
      <q-card class="app-dialog-card app-glass-dialog" style="min-width: 420px">
        <q-card-section class="row items-start justify-between">
          <div>
            <div class="text-h6">试跑 {{ tryRunAgent?.displayName || tryRunAgent?.agentKey }}</div>
            <div class="text-caption text-grey-7">以固定只读指令试跑：浏览项目顶层结构并说明用途，不修改任何文件。</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭" @click="tryRunOpen = false" />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-select
            v-model="tryRunProject"
            dense
            outlined
            emit-value
            map-options
            label="选择项目 *"
            :options="projectOptions"
            :rules="[(v) => !!v || '请选择项目']"
          />
        </q-card-section>
        <q-card-actions align="right" class="q-pa-md">
          <q-btn flat rounded label="取消" @click="tryRunOpen = false" />
          <q-btn color="primary" rounded unelevated icon="play_arrow" label="开始试跑" :loading="tryRunning" :disable="!tryRunProject" @click="runTryRun" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- 候选项目选择（项目名歧义时） -->
    <q-dialog v-model="candidatesOpen" persistent>
      <q-card class="app-dialog-card app-glass-dialog" style="min-width: 480px">
        <q-card-section class="row items-start justify-between">
          <div>
            <div class="text-h6">匹配到多个项目</div>
            <div class="text-caption text-grey-7">请选择一个项目重新派发。</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭" @click="closeCandidates" />
        </q-card-section>
        <q-separator />
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
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import AppPageHero from '../components/layout/AppPageHero.vue';
import {
  AUTONOMY_OPTIONS,
  CAPABILITY_OPTIONS,
  agentInitial,
  deleteCodingProject,
  dispatchCodingTask,
  listCodingAgents,
  listCodingProjects,
  probeCodingAgent,
  upsertCodingAgent,
  upsertCodingProject,
} from '../features/coding/api';
import type { CodingAgent, CodingProject, ProjectCandidate } from '../features/coding/api';

const TRY_RUN_PROMPT = '只读浏览当前项目：列出顶层目录结构并一句话说明项目用途。不要修改任何文件。';

const $q = useQuasar();
const router = useRouter();

const loading = ref(false);
const error = ref('');
const agents = ref<CodingAgent[]>([]);
const projects = ref<CodingProject[]>([]);
const togglingKey = ref('');
const probingKey = ref('');

const editorOpen = ref(false);
const editingAgent = ref<CodingAgent | null>(null);
const saving = ref(false);

type EnvRow = { key: string; value: string; masked: boolean };
const form = reactive({
  agentKey: '',
  displayName: '',
  command: '',
  argsText: '',
  iconUrl: '',
  defaultAutonomy: 'readonly',
  capabilities: [] as string[],
  envRows: [] as EnvRow[],
  enabled: true,
});

const projectDialogOpen = ref(false);
const projectSaving = ref(false);
const projectForm = reactive({ name: '', path: '', description: '' });

const tryRunOpen = ref(false);
const tryRunAgent = ref<CodingAgent | null>(null);
const tryRunProject = ref('');
const tryRunning = ref(false);

const candidatesOpen = ref(false);
const candidates = ref<ProjectCandidate[]>([]);
// 候选重发时保留上一次派发上下文。
const pendingDispatch = ref<{ agentKey: string; prompt: string } | null>(null);

const projectOptions = computed(() => projects.value.map((p) => ({ label: `${p.name}（${p.path}）`, value: p.name ?? '' })));

function isMaskedValue(v: string): boolean {
  return v.length > 0 && /^[*•]+$/.test(v);
}

async function loadAll(manual = false) {
  loading.value = true;
  error.value = '';
  try {
    const [a, p] = await Promise.all([listCodingAgents(), listCodingProjects()]);
    agents.value = a;
    projects.value = p;
    if (manual) $q.notify({ type: 'positive', message: `已刷新，共 ${a.length} 个工具、${p.length} 个项目`, timeout: 2000 });
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败';
  } finally {
    loading.value = false;
  }
}

onMounted(() => void loadAll());

function probeBadge(agent: CodingAgent): { label: string; color: string; textColor: string } {
  if (agent.lastProbeOk === true) return { label: '可用', color: 'positive', textColor: 'white' };
  if (agent.lastProbeError) return { label: '不可用', color: 'negative', textColor: 'white' };
  return { label: '未探测', color: 'grey-4', textColor: 'grey-9' };
}

function openCreate() {
  editingAgent.value = null;
  Object.assign(form, {
    agentKey: '',
    displayName: '',
    command: '',
    argsText: '',
    iconUrl: '',
    defaultAutonomy: 'readonly',
    capabilities: [],
    envRows: [],
    enabled: true,
  });
  editorOpen.value = true;
}

function openEdit(agent: CodingAgent) {
  editingAgent.value = agent;
  form.agentKey = agent.agentKey ?? '';
  form.displayName = agent.displayName ?? '';
  form.command = agent.command ?? '';
  form.argsText = (agent.args ?? []).join(', ');
  form.iconUrl = agent.iconUrl ?? '';
  form.defaultAutonomy = agent.defaultAutonomy || 'readonly';
  form.capabilities = [...(agent.capabilities ?? [])];
  form.enabled = !!agent.enabled;
  form.envRows = Object.entries(agent.env ?? {}).map(([key, value]) => ({
    key,
    value,
    masked: isMaskedValue(value),
  }));
  editorOpen.value = true;
}

async function saveAgent() {
  if (!form.agentKey || !form.command) {
    $q.notify({ type: 'warning', message: '请填写工具标识与命令' });
    return;
  }
  saving.value = true;
  try {
    const env: Record<string, string> = {};
    const oldEnv = editingAgent.value?.env ?? {};
    for (const row of form.envRows) {
      if (!row.key) continue;
      // 掩码行留空 = 保留服务端旧值；填新值 = 覆盖。
      if (row.masked && !row.value) {
        if (oldEnv[row.key] !== undefined) env[row.key] = oldEnv[row.key];
        continue;
      }
      env[row.key] = row.value;
    }
    await upsertCodingAgent({
      agentKey: form.agentKey,
      displayName: form.displayName,
      command: form.command,
      args: form.argsText
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
      env,
      enabled: form.enabled,
      livenessOverrideJson: undefined,
      iconUrl: form.iconUrl,
      capabilities: form.capabilities,
      defaultAutonomy: form.defaultAutonomy,
    });
    $q.notify({ type: 'positive', message: '已保存' });
    editorOpen.value = false;
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
  } finally {
    saving.value = false;
  }
}

async function toggleEnabled(agent: CodingAgent, enabled: boolean) {
  togglingKey.value = agent.agentKey ?? '';
  try {
    await upsertCodingAgent({
      agentKey: agent.agentKey,
      displayName: agent.displayName,
      command: agent.command,
      args: agent.args,
      env: agent.env,
      enabled,
      livenessOverrideJson: undefined,
      iconUrl: agent.iconUrl,
      capabilities: agent.capabilities,
      defaultAutonomy: agent.defaultAutonomy,
    });
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '切换启用状态失败' });
  } finally {
    togglingKey.value = '';
  }
}

async function probe(agent: CodingAgent) {
  probingKey.value = agent.agentKey ?? '';
  try {
    const res = await probeCodingAgent(agent.agentKey ?? '');
    $q.notify(
      res.ok
        ? { type: 'positive', message: `${agent.displayName || agent.agentKey} 探测通过` }
        : { type: 'negative', message: res.error || '探测失败' },
    );
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '探测失败' });
  } finally {
    probingKey.value = '';
  }
}

function openTryRun(agent: CodingAgent) {
  if (projects.value.length === 0) {
    $q.notify({ type: 'warning', message: '请先在下方「项目目录」中添加项目' });
    return;
  }
  tryRunAgent.value = agent;
  tryRunProject.value = projects.value[0]?.name ?? '';
  tryRunOpen.value = true;
}

async function runTryRun() {
  if (!tryRunAgent.value || !tryRunProject.value) return;
  tryRunning.value = true;
  try {
    const res = await dispatchCodingTask({
      agentKey: tryRunAgent.value.agentKey ?? '',
      projectQuery: tryRunProject.value,
      prompt: TRY_RUN_PROMPT,
    });
    tryRunOpen.value = false;
    if (res.task?.id) {
      $q.notify({ type: 'positive', message: '试跑任务已派发' });
      void router.push({ path: '/coding-tasks', query: { focus: res.task.id } });
    } else if ((res.candidates ?? []).length > 0) {
      candidates.value = res.candidates ?? [];
      pendingDispatch.value = { agentKey: tryRunAgent.value.agentKey ?? '', prompt: TRY_RUN_PROMPT };
      candidatesOpen.value = true;
    } else {
      $q.notify({ type: 'warning', message: '派发失败：未返回任务' });
    }
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '派发失败' });
  } finally {
    tryRunning.value = false;
  }
}

async function pickCandidate(c: ProjectCandidate) {
  const pending = pendingDispatch.value;
  if (!pending) return;
  try {
    const res = await dispatchCodingTask({
      agentKey: pending.agentKey,
      projectQuery: c.name ?? '',
      prompt: pending.prompt,
    });
    closeCandidates();
    if (res.task?.id) {
      $q.notify({ type: 'positive', message: '试跑任务已派发' });
      void router.push({ path: '/coding-tasks', query: { focus: res.task.id } });
    }
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '派发失败' });
  }
}

function closeCandidates() {
  candidatesOpen.value = false;
  candidates.value = [];
  pendingDispatch.value = null;
}

function openProjectCreate() {
  Object.assign(projectForm, { name: '', path: '', description: '' });
  projectDialogOpen.value = true;
}

async function saveProject() {
  if (!projectForm.name || !projectForm.path) {
    $q.notify({ type: 'warning', message: '请填写项目名称与路径' });
    return;
  }
  projectSaving.value = true;
  try {
    await upsertCodingProject({
      name: projectForm.name,
      path: projectForm.path,
      description: projectForm.description,
    });
    $q.notify({ type: 'positive', message: '项目已保存' });
    projectDialogOpen.value = false;
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '保存失败' });
  } finally {
    projectSaving.value = false;
  }
}

function confirmDeleteProject(p: CodingProject) {
  $q.dialog({
    title: '确认删除该项目？',
    message: `删除「${p.name}」后将无法向该目录派发新任务，进行中的任务不受影响。`,
    cancel: true,
    persistent: true,
  }).onOk(async () => {
    try {
      await deleteCodingProject(p.id ?? '');
      $q.notify({ type: 'positive', message: '项目已删除' });
      await loadAll();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '删除失败' });
    }
  });
}
</script>
