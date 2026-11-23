<template>
  <q-page class="app-standard-page cap-page coding-tools-page">
    <Teleport defer to="#app-tab-actions">
      <CodingWorkspacePicker :projects="projects" @changed="loadAll()" />
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="loadAll(true)"
      />
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="add"
        :label="t('cap.coding.create')"
        @click="addOpen = true"
      />
    </Teleport>

    <AppStatePanel
      v-if="pageState === 'error'"
      variant="error"
      :description="error"
      :action-label="$t('shell.common.retry')"
      action-icon="refresh"
      @action="loadAll()"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />

    <template v-else>
      <AppStatePanel
        v-if="pageState === 'partial'"
        variant="partial"
        dense
        :description="error"
        :action-label="$t('shell.common.retry')"
        action-icon="refresh"
        @action="loadAll()"
      />

      <CapStats :stats="stats" :label="t('cap.common.statsLabel')" />

      <!-- 工具：一张卡一个，状态与动作都在卡上，测试 / 登录 / 编辑在弹窗里完成，不跳页 -->
      <section class="tools-grid" :aria-label="t('cap.coding.toolsLabel')">
        <CodingToolCard
          v-for="agent in agents"
          :key="agent.agentKey"
          :agent="agent"
          :preset="presetFor(agent)"
          :busy="busyFor(agent)"
          @toggle="toggleEnabled(agent, $event)"
          @login="openLogin(agent)"
          @test="openTest(agent)"
          @probe="probe(agent)"
          @update="update(agent)"
          @edit="openEdit(agent)"
          @remove="confirmDeleteAgent(agent)"
        />
        <AppStatePanel
          v-if="agents.length === 0"
          variant="empty"
          icon="construction"
          :title="t('cap.coding.emptyTitle')"
          :description="t('cap.coding.emptyDesc')"
          :action-label="t('cap.coding.create')"
          action-icon="add"
          @action="addOpen = true"
        />
      </section>
    </template>

    <CodingToolAddDialog v-model="addOpen" :agents="agents" @changed="loadAll()" @custom="openCustomCreate" />
    <CodingToolTestDialog
      v-model="testOpen"
      :agent="current"
      :preset="currentPreset"
      @changed="loadAll()"
      @login="switchToLogin"
    />
    <CodingToolLoginDialog v-model="loginOpen" :preset="currentPreset" @changed="loadAll()" />
    <CodingToolEditDialog
      v-model="toolEditOpen"
      :agent="current"
      :preset="currentPreset"
      @saved="onSaved"
      @login="switchToLogin"
    />

    <!-- 自定义接入（ACP / OpenAI 兼容）：命令、参数、环境变量都要自己填 -->
    <AppDialog
      v-model="editorOpen"
      persistent
      size="xl"
      :title="editingAgent ? t('cap.coding.form.editTitle') : t('cap.coding.form.createTitle')"
      :subtitle="t('cap.coding.form.subtitle')"
    >
      <div v-if="!editingAgent" class="q-pb-sm">
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          icon="auto_fix_high"
          :label="$t('codingTools.shimPresetLabel')"
          @click="applyBuiltinShimPreset"
        />
        <div class="tools-hint">{{ $t('codingTools.shimPresetHint') }}</div>
      </div>
      <q-form class="app-form-field-grid app-form-field-grid--2col" @submit.prevent="saveAgent">
        <q-input
          v-model="form.agentKey"
          dense
          outlined
          :label="t('cap.coding.form.agentKey')"
          :readonly="!!editingAgent"
          :rules="[(v) => !!v || t('cap.coding.form.required')]"
        />
        <q-input v-model="form.displayName" dense outlined :label="t('cap.coding.form.displayName')" />
        <q-input
          v-model="form.command"
          dense
          outlined
          :label="t('cap.coding.form.command')"
          placeholder="npx"
          :rules="[(v) => !!v || t('cap.coding.form.required')]"
        />
        <q-input
          v-model="form.argsText"
          dense
          outlined
          :label="t('cap.coding.form.args')"
          :hint="t('cap.coding.form.argsHint')"
        />
        <q-input
          v-model="form.iconUrl"
          dense
          outlined
          :label="t('cap.coding.form.iconUrl')"
          :hint="t('cap.coding.form.iconHint')"
        />
        <q-select
          v-model="form.defaultAutonomy"
          dense
          outlined
          emit-value
          map-options
          :label="t('cap.coding.form.autonomy')"
          :options="AUTONOMY_OPTIONS"
        />
        <div class="app-grid-span-full">
          <div class="section-label q-mb-sm">{{ t('cap.coding.form.capabilities') }}</div>
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
            <div class="section-label">{{ t('cap.coding.form.env') }}</div>
            <q-btn
              flat
              dense
              color="primary"
              icon="add"
              :label="t('cap.coding.form.addRow')"
              @click="form.envRows.push({ key: '', value: '', masked: false })"
            />
          </div>
          <div v-for="(row, i) in form.envRows" :key="i" class="row q-gutter-sm items-center q-mb-sm">
            <q-input
              v-model="row.key"
              dense
              outlined
              class="col-4"
              :label="t('cap.coding.form.envKey')"
              :readonly="row.masked"
            />
            <q-input
              v-model="row.value"
              dense
              outlined
              class="col"
              :label="t('cap.coding.form.envValue')"
              :readonly="row.masked"
              :hint="row.masked ? t('cap.coding.form.maskedHint') : ''"
            />
            <q-btn
              flat
              dense
              round
              icon="close"
              :aria-label="t('cap.coding.form.removeRow')"
              @click="form.envRows.splice(i, 1)"
            />
          </div>
        </div>
        <q-toggle v-model="form.enabled" color="primary" :label="t('cap.coding.form.enabled')" />
      </q-form>
      <template #actions>
        <q-btn flat rounded :label="t('cap.coding.form.cancel')" @click="editorOpen = false" />
        <q-btn
          color="primary"
          rounded
          unelevated
          :label="t('cap.coding.form.save')"
          :loading="saving"
          @click="saveAgent"
        />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { confirm } from '../composables/useAppConfirm';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapTone } from '../components/capability/capTypes';
import CodingToolCard from '../features/coding/components/CodingToolCard.vue';
import CodingToolAddDialog from '../features/coding/components/CodingToolAddDialog.vue';
import CodingToolTestDialog from '../features/coding/components/CodingToolTestDialog.vue';
import CodingToolLoginDialog from '../features/coding/components/CodingToolLoginDialog.vue';
import CodingToolEditDialog from '../features/coding/components/CodingToolEditDialog.vue';
import CodingWorkspacePicker from '../features/coding/components/CodingWorkspacePicker.vue';
import { resolvePageState } from './pageState';
import {
  AUTONOMY_OPTIONS,
  CAPABILITY_OPTIONS,
  deleteCodingAgent,
  listCodingAgents,
  listCodingProjects,
  probeCodingAgent,
  upsertCodingAgent,
} from '../features/coding/api';
import type { CodingAgent, CodingProject } from '../features/coding/api';
import { checkToolUpdate, installAgent, listPresets, presetOfAgent } from '../features/fleet/api';
import type { AgentPreset } from '../features/fleet/api';
import { isForbiddenApiError, parseKratosApiError } from '../utils/kratosError';

const $q = useQuasar();
const { t } = useI18n();

const loading = ref(false);
const error = ref('');
/** 403 / PERMISSION_DENIED：页面渲染 forbidden 态，不提供重试。 */
const forbidden = ref(false);
const agents = ref<CodingAgent[]>([]);
const projects = ref<CodingProject[]>([]);
const presets = ref<AgentPreset[]>([]);
const togglingKey = ref('');
const probingKey = ref('');
const updatingId = ref('');

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: agents.value.length + projects.value.length,
    forbidden: forbidden.value,
  }),
);

async function loadAll(manual = false) {
  loading.value = true;
  error.value = '';
  forbidden.value = false;
  try {
    const [a, p, presetList] = await Promise.all([
      listCodingAgents(),
      listCodingProjects(),
      // 运行台接口取不到（如容器模式没开运行台）时不影响本页
      listPresets().catch(() => [] as AgentPreset[]),
    ]);
    agents.value = a;
    projects.value = p;
    presets.value = presetList;
    // 别处（自动更新、运行台、添加工具）发起的安装也要在卡片上看到进度
    if (presetList.some((p) => p.installState === 'running')) {
      clearTimeout(pollTimer);
      pollTimer = setTimeout(() => void pollInstalls(), 2000);
    }
    if (manual)
      $q.notify({
        type: 'positive',
        message: t('cap.coding.msg.refreshed', { tools: a.length, projects: p.length }),
        timeout: 2000,
      });
  } catch (err) {
    error.value = err instanceof Error ? err.message : t('cap.coding.msg.loadFailed');
    forbidden.value = isForbiddenApiError(err);
  } finally {
    loading.value = false;
  }
}

onMounted(() => void loadAll());

// 运行台清单里的工具（fleet:<id> 在运行台终端执行，DeepSeek Harness 走 ACP），状态与设置以运行台清单为准
function presetFor(agent: CodingAgent): AgentPreset | undefined {
  return presetOfAgent(agent, presets.value);
}

function busyFor(agent: CodingAgent): string {
  if (probingKey.value === agent.agentKey) return 'probe';
  if (togglingKey.value === agent.agentKey) return 'toggle';
  if (updatingId.value && updatingId.value === presetFor(agent)?.id) return 'update';
  return '';
}

const stats = computed(() => {
  const on = agents.value.filter((a) => a.enabled);
  const ok = on.filter((a) => a.lastProbeOk === true).length;
  const failed = on.filter((a) => !a.lastProbeOk && a.lastProbeError).length;
  return [
    { key: 'tools', label: t('cap.coding.stats.tools'), value: agents.value.length },
    { key: 'enabled', label: t('cap.coding.stats.enabled'), value: on.length },
    {
      key: 'probe',
      label: t('cap.coding.stats.probe'),
      value: `${ok} / ${on.length}`,
      tone: (failed > 0 ? 'alert' : on.length && ok === on.length ? 'ok' : 'pending') as CapTone,
    },
    { key: 'projects', label: t('cap.coding.stats.projects'), value: projects.value.length },
  ];
});

// ——— 卡片动作：测试 / 登录 / 编辑都开弹窗 ———
const addOpen = ref(false);
const testOpen = ref(false);
const loginOpen = ref(false);
const toolEditOpen = ref(false);
const current = ref<CodingAgent | null>(null);
const currentPreset = computed(() => (current.value ? presetFor(current.value) : undefined));

function openTest(agent: CodingAgent) {
  current.value = agent;
  testOpen.value = true;
}
function openLogin(agent: CodingAgent) {
  current.value = agent;
  loginOpen.value = true;
}
function switchToLogin() {
  testOpen.value = false;
  toolEditOpen.value = false;
  loginOpen.value = true;
}
function openEdit(agent: CodingAgent) {
  current.value = agent;
  if (presetFor(agent)) toolEditOpen.value = true;
  else openCustomEdit(agent);
}
function onSaved() {
  $q.notify({ type: 'positive', message: t('cap.coding.tool.edit.saved') });
  void loadAll();
}

async function probe(agent: CodingAgent) {
  probingKey.value = agent.agentKey ?? '';
  try {
    const res = await probeCodingAgent(agent.agentKey ?? '');
    $q.notify(
      res.ok
        ? { type: 'positive', message: t('cap.coding.msg.probeOk', { name: agent.displayName || agent.agentKey }) }
        : { type: 'negative', message: res.error || t('cap.coding.msg.probeFailed') },
    );
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('cap.coding.msg.probeFailed') });
  } finally {
    probingKey.value = '';
  }
}

// 更新 = 重新安装最新版（后台跑）；装完前每 2 秒刷新一次清单，卡上的版本与更新时间随之变化
let pollTimer: ReturnType<typeof setTimeout> | undefined;
// 装完前每 2 秒刷新一次清单（卡片上的进度条跟着走）；装完提示结果
async function pollInstalls(watchId = '') {
  presets.value = await listPresets().catch(() => presets.value);
  if (presets.value.some((p) => p.installState === 'running')) {
    pollTimer = setTimeout(() => void pollInstalls(watchId), 2000);
    return;
  }
  updatingId.value = '';
  const p = presets.value.find((x) => x.id === watchId);
  if (!p) return;
  $q.notify(
    p.installState === 'ok'
      ? { type: 'positive', message: t('cap.coding.tool.update.done', { name: p.label, v: p.version }) }
      : { type: 'negative', message: t('cap.coding.tool.update.failed', { name: p.label }) },
  );
}
onBeforeUnmount(() => clearTimeout(pollTimer));

// 已知有新版就直接更新；否则先对比本机与远端版本：旧了就更新，已是最新就提示
async function update(agent: CodingAgent) {
  const p = presetFor(agent);
  if (!p) return;
  updatingId.value = p.id;
  try {
    let cur = p;
    if (!p.updateAvailable) {
      cur = await checkToolUpdate(p.id);
      presets.value = presets.value.map((x) => (x.id === cur.id ? cur : x));
      if (!cur.updateAvailable) {
        updatingId.value = '';
        $q.notify({
          type: 'positive',
          message: t('cap.coding.tool.update.upToDate', {
            local: cur.version || '—',
            remote: cur.latestVersion || '—',
          }),
        });
        return;
      }
    }
    await installAgent(p.id);
    $q.notify({
      type: 'info',
      message: t('cap.coding.tool.update.started', { name: p.label, remote: cur.latestVersion || '—' }),
    });
    clearTimeout(pollTimer);
    await pollInstalls(p.id);
  } catch (err) {
    updatingId.value = '';
    $q.notify({ type: 'negative', message: parseKratosApiError(err).message || t('cap.coding.msg.saveFailed') });
  }
}

async function toggleEnabled(agent: CodingAgent, enabled: boolean) {
  togglingKey.value = agent.agentKey ?? '';
  try {
    await upsertCodingAgent({
      agentKey: agent.agentKey,
      displayName: agent.displayName,
      command: agent.command,
      // 只改启停：env / args 省略 = 后端保留旧值（列表里的密钥是掩码，不能回传）
      args: undefined,
      env: undefined,
      enabled,
      livenessOverrideJson: undefined,
      iconUrl: agent.iconUrl,
      capabilities: agent.capabilities,
      defaultAutonomy: agent.defaultAutonomy,
    });
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('cap.coding.msg.toggleFailed') });
  } finally {
    togglingKey.value = '';
  }
}

// 删除编码工具：只有从没跑过任务的能删，后端会拒绝其余的，把原因原样告诉用户
async function confirmDeleteAgent(agent: CodingAgent) {
  const name = presetFor(agent)?.label || agent.displayName || agent.agentKey || '';
  const ok = await confirm({
    title: t('cap.coding.msg.deleteAgentTitle', { name }),
    message: t('cap.coding.msg.deleteAgentMessage'),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    await deleteCodingAgent(agent.agentKey ?? '');
    $q.notify({ type: 'positive', message: t('cap.coding.msg.agentDeleted', { name }) });
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'warning', message: parseKratosApiError(err).message || t('cap.coding.msg.deleteFailed') });
  }
}

// ——— 自定义接入（ACP / OpenAI 兼容）表单 ———
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

function isMaskedValue(v: string): boolean {
  return v.length > 0 && /^[*•]+$/.test(v);
}

function openCustomCreate() {
  addOpen.value = false;
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

// 内置通用 shim 预设：command=@self 解析为宿主进程自身（admin），配合
// acp-shim 子命令把任意 OpenAI 兼容端点包成 ACP 工具——新增厂商零代码。
function applyBuiltinShimPreset() {
  form.command = '@self';
  form.argsText = 'acp-shim, --base-url=https://api.example.com/v1, --model=your-model, --api-key-env=OPENAI_API_KEY';
}

function openCustomEdit(agent: CodingAgent) {
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
    $q.notify({ type: 'warning', message: t('cap.coding.msg.needKeyCommand') });
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
    $q.notify({ type: 'positive', message: t('cap.coding.msg.saved') });
    editorOpen.value = false;
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('cap.coding.msg.saveFailed') });
  } finally {
    saving.value = false;
  }
}
</script>

<style scoped lang="sass">
.tools-grid
  display: grid
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 380px), 1fr))
  gap: var(--nt-space-3)
.tools-hint
  max-width: 72ch
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
</style>
