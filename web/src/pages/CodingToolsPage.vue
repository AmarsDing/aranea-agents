<template>
  <q-page class="app-standard-page cap-page coding-tools-page">
    <Teleport defer to="#app-tab-actions">
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
      <q-btn unelevated dense no-caps color="primary" icon="add" :label="t('cap.coding.create')" @click="openCreate" />
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

      <!-- 工具：一张构件卡一个；探测失败的原因直接写在卡上，不藏在悬停提示里 -->
      <section class="cap-grid" :aria-label="t('cap.coding.toolsLabel')">
        <CapCard
          v-for="agent in agents"
          :key="agent.agentKey"
          icon="terminal"
          :icon-url="agent.iconUrl || undefined"
          :code="agent.agentKey || ''"
          :title="agent.displayName || agent.agentKey || ''"
          :description="commandLine(agent)"
          :status="agentStatus(agent)"
          :facts="agentFacts(agent)"
          :tags="agentTags(agent)"
          @open="openEdit(agent)"
        >
          <template #switch>
            <q-toggle
              dense
              color="primary"
              :model-value="!!agent.enabled"
              :disable="togglingKey === agent.agentKey"
              :aria-label="agent.enabled ? t('cap.common.batchDisable') : t('cap.common.batchEnable')"
              @update:model-value="toggleEnabled(agent, Boolean($event))"
            />
          </template>
          <p v-if="agent.lastProbeError" class="cap-note cap-note--alert" role="note">
            <q-icon name="error_outline" size="16px" aria-hidden="true" />
            <span class="cap-note__body">{{ agent.lastProbeError }}</span>
          </p>
          <!-- 同一个工具在运行台（终端）里能不能用：两处接入方式不同，结论可能不一样，并排写清 -->
          <p v-if="fleetPresetFor(agent)" class="cap-note cap-note--info" role="note">
            <q-icon name="terminal" size="16px" aria-hidden="true" />
            <span class="cap-note__body">{{ fleetLine(agent) }}</span>
          </p>
          <template #relation>{{
            agent.updatedAt ? t('cap.coding.updatedAt', { at: formatWhen(agent.updatedAt) }) : ''
          }}</template>
          <template #actions>
            <q-btn
              flat
              dense
              no-caps
              size="sm"
              color="primary"
              icon="network_check"
              :label="t('cap.coding.probe')"
              :loading="probingKey === agent.agentKey"
              @click="probe(agent)"
            />
            <q-btn flat dense round size="sm" icon="more_horiz" :aria-label="t('cap.common.more')">
              <q-menu anchor="bottom right" self="top right">
                <q-list dense class="cap-menu">
                  <q-item v-close-popup clickable :disable="!agent.enabled" @click="openTryRun(agent)">
                    <q-item-section avatar><q-icon name="play_arrow" size="18px" /></q-item-section>
                    <q-item-section>{{ t('cap.coding.tryRun') }}</q-item-section>
                  </q-item>
                  <q-item v-close-popup clickable @click="openEdit(agent)">
                    <q-item-section avatar><q-icon name="edit" size="18px" /></q-item-section>
                    <q-item-section>{{ t('cap.common.edit') }}</q-item-section>
                  </q-item>
                  <q-item clickable :to="{ path: '/coding-tasks' }">
                    <q-item-section avatar><q-icon name="history" size="18px" /></q-item-section>
                    <q-item-section>{{ t('cap.coding.tasks') }}</q-item-section>
                  </q-item>
                  <q-separator />
                  <q-item v-close-popup clickable class="text-negative" @click="confirmDeleteAgent(agent)">
                    <q-item-section avatar><q-icon name="delete" size="18px" /></q-item-section>
                    <q-item-section>{{ t('cap.common.remove') }}</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>
            </q-btn>
          </template>
        </CapCard>
        <AppStatePanel
          v-if="agents.length === 0"
          variant="empty"
          icon="terminal"
          :title="t('cap.coding.emptyTitle')"
          :description="t('cap.coding.emptyDesc')"
          :action-label="t('cap.coding.create')"
          action-icon="add"
          @action="openCreate"
        />
      </section>

      <!-- 命令行工具：运行台装好的命令行，一键接入成编码工具（派活在运行台终端里执行） -->
      <section class="cap-section" :aria-label="t('cap.coding.cli.title')">
        <div class="cap-section__head">
          <h2 class="cap-section__title">{{ t('cap.coding.cli.title') }}</h2>
          <p class="cap-section__hint">{{ t('cap.coding.cli.hint') }}</p>
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            icon="open_in_new"
            :label="t('cap.coding.fleet.open')"
            :to="{ path: '/fleet', query: { panel: 'cli' } }"
          />
        </div>
        <p v-if="presetsError" class="cap-section__hint">{{ t('cap.coding.fleet.unavailable') }}</p>
        <template v-else>
          <ul class="cli-grid" role="list">
            <li v-for="p in visibleCli" :key="p.id" class="cli-row" :class="{ 'cli-row--off': !p.available }">
              <i class="cap-dot" :class="cliDot(p)" aria-hidden="true" />
              <div class="cli-row__main">
                <span class="cli-row__name">{{ p.label }}</span>
                <span class="cli-row__meta">
                  <code class="cap-mono">{{ p.command }}</code> · {{ cliState(p) }}
                </span>
              </div>
              <q-btn
                v-if="!p.available"
                flat
                dense
                no-caps
                size="sm"
                color="primary"
                :label="t('cap.coding.cli.install')"
                :to="{ path: '/fleet', query: { panel: 'cli' } }"
              />
              <span v-else-if="linkedAgent(p)" class="cli-row__linked">
                <q-icon name="check_circle" size="16px" aria-hidden="true" />{{ t('cap.coding.cli.connected') }}
              </span>
              <q-btn
                v-else
                dense
                unelevated
                no-caps
                size="sm"
                color="primary"
                :label="t('cap.coding.cli.connect')"
                :loading="connectingId === p.id"
                :aria-label="`${t('cap.coding.cli.connect')} ${p.label}`"
                @click="connectCli(p)"
              />
            </li>
          </ul>
          <p v-if="installedCli.length === 0" class="cap-section__hint">{{ t('cap.coding.cli.noneInstalled') }}</p>
          <q-btn
            v-if="missingCli.length"
            flat
            dense
            no-caps
            size="sm"
            class="cli-toggle"
            :icon="showMissing ? 'expand_less' : 'expand_more'"
            :label="
              showMissing ? t('cap.coding.cli.hideMissing') : t('cap.coding.cli.showMissing', { n: missingCli.length })
            "
            @click="showMissing = !showMissing"
          />
        </template>
      </section>

      <!-- 项目目录：可派发任务的本机目录白名单 -->
      <section class="cap-section" :aria-label="t('cap.coding.projects')">
        <div class="cap-section__head">
          <h2 class="cap-section__title">{{ t('cap.coding.projects') }}</h2>
          <p class="cap-section__hint">{{ t('cap.coding.projectsHint') }}</p>
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            icon="add"
            :label="t('cap.coding.addProject')"
            @click="openProjectCreate"
          />
        </div>
        <ul v-if="projects.length" class="cap-rows" role="list">
          <li v-for="p in projects" :key="p.id" class="cap-row">
            <q-icon name="folder_open" size="20px" class="cap-row__icon" aria-hidden="true" />
            <div class="cap-row__main">
              <strong class="cap-row__title">{{ p.name }}</strong>
              <code class="cap-mono cap-row__sub">{{ p.path }}</code>
              <span v-if="p.description" class="cap-row__desc">{{ p.description }}</span>
            </div>
            <q-btn
              flat
              dense
              round
              size="sm"
              icon="edit"
              :aria-label="t('cap.common.edit')"
              @click="openProjectEdit(p)"
            />
            <q-btn
              flat
              dense
              round
              size="sm"
              icon="delete"
              color="negative"
              :aria-label="t('cap.common.remove')"
              @click="confirmDeleteProject(p)"
            />
          </li>
        </ul>
        <p v-else class="cap-section__hint">{{ t('cap.coding.projectsEmpty') }}</p>
      </section>
    </template>

    <!-- 编辑/新增工具对话框 -->
    <!-- 表单页签有未保存输入才挡 Esc / 点外面关闭；命令行页签没有要丢的东西 -->
    <AppDialog
      v-model="editorOpen"
      :persistent="!!editingAgent || addTab === 'custom'"
      size="xl"
      :title="editingAgent ? t('cap.coding.form.editTitle') : t('cap.coding.form.createTitle')"
      :subtitle="editingAgent || addTab === 'custom' ? t('cap.coding.form.subtitle') : t('cap.coding.cli.dialogHint')"
    >
      <q-tabs
        v-if="!editingAgent"
        v-model="addTab"
        dense
        no-caps
        align="left"
        narrow-indicator
        active-color="primary"
        indicator-color="primary"
        class="cli-tabs"
      >
        <q-tab name="cli" :label="t('cap.coding.addTabs.cli')" />
        <q-tab name="custom" :label="t('cap.coding.addTabs.custom')" />
      </q-tabs>
      <div v-if="!editingAgent && addTab === 'cli'" class="cli-pick">
        <ul v-if="connectableCli.length" class="cli-grid" role="list">
          <li v-for="p in connectableCli" :key="p.id" class="cli-row">
            <i class="cap-dot" :class="cliDot(p)" aria-hidden="true" />
            <div class="cli-row__main">
              <span class="cli-row__name">{{ p.label }}</span>
              <span class="cli-row__meta">
                <code class="cap-mono">{{ p.command }}</code> · {{ cliState(p) }}
              </span>
            </div>
            <q-btn
              dense
              unelevated
              no-caps
              size="sm"
              color="primary"
              :label="t('cap.coding.cli.connect')"
              :loading="connectingId === p.id"
              :aria-label="`${t('cap.coding.cli.connect')} ${p.label}`"
              @click="connectCli(p)"
            />
          </li>
        </ul>
        <p v-else class="cli-pick__hint">
          {{ installedCli.length ? t('cap.coding.cli.allConnected') : t('cap.coding.cli.noneInstalled') }}
        </p>
      </div>
      <div v-if="!editingAgent && addTab === 'custom'" class="q-pb-sm">
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          icon="auto_fix_high"
          :label="$t('codingTools.shimPresetLabel')"
          @click="applyBuiltinShimPreset"
        />
        <div class="cli-pick__hint">
          {{ $t('codingTools.shimPresetHint') }}
        </div>
      </div>
      <q-form
        v-if="editingAgent || addTab === 'custom'"
        class="app-form-field-grid app-form-field-grid--2col"
        @submit.prevent="saveAgent"
      >
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
          v-if="editingAgent || addTab === 'custom'"
          color="primary"
          rounded
          unelevated
          :label="t('cap.coding.form.save')"
          :loading="saving"
          @click="saveAgent"
        />
      </template>
    </AppDialog>

    <!-- 项目新增对话框 -->
    <AppDialog
      v-model="projectDialogOpen"
      persistent
      size="sm"
      :title="projectEditing ? t('cap.coding.editProject') : t('cap.coding.addProject')"
    >
      <q-form class="q-gutter-md" @submit.prevent="saveProject">
        <q-input
          v-model="projectForm.name"
          dense
          outlined
          :label="t('cap.coding.form.projectName')"
          :readonly="projectEditing"
          :rules="[(v) => !!v || t('cap.coding.form.required')]"
        />
        <q-input
          v-model="projectForm.path"
          dense
          outlined
          :label="t('cap.coding.form.projectPath')"
          placeholder="F:\project\demo"
          :rules="[(v) => !!v || t('cap.coding.form.required')]"
        />
        <q-input
          v-model="projectForm.description"
          dense
          outlined
          type="textarea"
          autogrow
          :label="t('cap.coding.form.projectDesc')"
        />
      </q-form>
      <template #actions>
        <q-btn flat rounded :label="t('cap.coding.form.cancel')" @click="projectDialogOpen = false" />
        <q-btn
          color="primary"
          rounded
          unelevated
          :label="t('cap.coding.form.save')"
          :loading="projectSaving"
          @click="saveProject"
        />
      </template>
    </AppDialog>

    <!-- 试跑：选择项目 -->
    <AppDialog
      v-model="tryRunOpen"
      persistent
      size="sm"
      :title="t('cap.coding.form.tryRunTitle', { name: tryRunAgent?.displayName || tryRunAgent?.agentKey || '' })"
      :subtitle="t('cap.coding.form.tryRunSubtitle')"
    >
      <q-select
        v-model="tryRunProject"
        dense
        outlined
        emit-value
        map-options
        :label="t('cap.coding.form.pickProject')"
        :options="projectOptions"
        :rules="[(v) => !!v || t('cap.coding.form.pickProjectRequired')]"
      />
      <template #actions>
        <q-btn flat rounded :label="t('cap.coding.form.cancel')" @click="tryRunOpen = false" />
        <q-btn
          color="primary"
          rounded
          unelevated
          icon="play_arrow"
          :label="t('cap.coding.form.startTryRun')"
          :loading="tryRunning"
          :disable="!tryRunProject"
          @click="runTryRun"
        />
      </template>
    </AppDialog>

    <!-- 候选项目选择（项目名歧义时） -->
    <AppDialog
      v-model="candidatesOpen"
      persistent
      size="sm"
      :title="t('cap.coding.form.candidatesTitle')"
      :subtitle="t('cap.coding.form.candidatesSubtitle')"
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
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { confirm } from '../composables/useAppConfirm';
import { useRouter } from 'vue-router';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import CapCard from '../components/capability/CapCard.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapFact, CapStatus, CapTag, CapTone } from '../components/capability/capTypes';
import { resolvePageState } from './pageState';
import {
  AUTONOMY_OPTIONS,
  CAPABILITY_OPTIONS,
  deleteCodingAgent,
  deleteCodingProject,
  dispatchCodingTask,
  listCodingAgents,
  listCodingProjects,
  probeCodingAgent,
  upsertCodingAgent,
  upsertCodingProject,
} from '../features/coding/api';
import type { CodingAgent, CodingProject, ProjectCandidate } from '../features/coding/api';
import { addToCodingTools, listPresets } from '../features/fleet/api';
import type { AgentPreset } from '../features/fleet/api';
import { isForbiddenApiError, parseKratosApiError } from '../utils/kratosError';

const TRY_RUN_PROMPT = '只读浏览当前项目：列出顶层目录结构并一句话说明项目用途。不要修改任何文件。'; // i18n-exempt 发给编码工具的指令，不是界面文字

const $q = useQuasar();
const router = useRouter();
const { t } = useI18n();

const loading = ref(false);
const error = ref('');
/** 403 / PERMISSION_DENIED：页面渲染 forbidden 态，不提供重试。 */
const forbidden = ref(false);
const agents = ref<CodingAgent[]>([]);
const projects = ref<CodingProject[]>([]);
const togglingKey = ref('');
const probingKey = ref('');

/** 五态判定：工具与项目目录共用一次加载，任一侧有数据时失败降级为 partial 横幅。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: agents.value.length + projects.value.length,
    forbidden: forbidden.value,
  }),
);

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
/** 编辑已有项目：名称是唯一键（后端按名称 upsert），只能改路径和描述 */
const projectEditing = ref(false);
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

const projectOptions = computed(() =>
  projects.value.map((p) => ({ label: `${p.name}（${p.path}）`, value: p.name ?? '' })),
);

function isMaskedValue(v: string): boolean {
  return v.length > 0 && /^[*•]+$/.test(v);
}

async function loadAll(manual = false) {
  loading.value = true;
  error.value = '';
  forbidden.value = false;
  try {
    const [a, p, presetList] = await Promise.all([
      listCodingAgents(),
      listCodingProjects(),
      // 运行台接口取不到（如容器模式没开运行台）时不影响本页
      listPresets().catch(() => null),
    ]);
    agents.value = a;
    projects.value = p;
    presets.value = presetList ?? [];
    presetsError.value = presetList === null;
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

// ——— 运行台（终端）对照 ———
const presets = ref<AgentPreset[]>([]);
const presetsError = ref(false);
const terminalPresets = computed(() => presets.value.filter((p) => p.id !== 'shell'));

// ——— 命令行工具：运行台装好的命令行，一键接入（command 登记为 fleet:<工具 id>） ———
const addTab = ref<'cli' | 'custom'>('cli');
const showMissing = ref(false);
const connectingId = ref('');
const linkedAgent = (p: AgentPreset) => agents.value.find((a) => (a.command ?? '') === 'fleet:' + p.id);
const installedCli = computed(() => terminalPresets.value.filter((p) => p.available));
const missingCli = computed(() => terminalPresets.value.filter((p) => !p.available));
const visibleCli = computed(() => [...installedCli.value, ...(showMissing.value ? missingCli.value : [])]);
const connectableCli = computed(() => installedCli.value.filter((p) => !linkedAgent(p)));
function cliState(p: AgentPreset): string {
  return p.available ? t('cap.coding.cli.ready.' + (p.readiness || 'unknown')) : t('cap.coding.cli.ready.missing');
}
function cliDot(p: AgentPreset): string {
  if (!p.available) return 'cap-dot--off';
  if (p.readiness === 'ready') return 'cap-dot--ok';
  return p.readiness ? 'cap-dot--watch' : 'cap-dot--pending';
}
async function connectCli(p: AgentPreset) {
  connectingId.value = p.id;
  try {
    await addToCodingTools(p);
    $q.notify({ type: 'positive', message: t('cap.coding.cli.connectedMsg', { name: p.label }) });
    await loadAll();
    if (connectableCli.value.length === 0) editorOpen.value = false;
  } catch (err) {
    $q.notify({ type: 'negative', message: parseKratosApiError(err).message || t('cap.coding.msg.saveFailed') });
  } finally {
    connectingId.value = '';
  }
}
// 编码工具（ACP 派发）的 agentKey → 运行台终端清单里的同一个工具
const FLEET_ID_BY_AGENT: Record<string, string> = {
  claude_code: 'claude',
  codex: 'codex',
  codebuddy: 'codebuddy',
  workbuddy: 'codebuddy',
  gemini: 'gemini',
  qwen: 'qwen',
  kimi: 'kimi',
  opencode: 'opencode',
};
function fleetPresetFor(agent: CodingAgent): AgentPreset | undefined {
  // command 登记为 fleet:<工具 id> 的编码 agent 直接在运行台终端里执行
  const cmd = agent.command ?? '';
  const id = cmd.startsWith('fleet:') ? cmd.slice(6) : FLEET_ID_BY_AGENT[agent.agentKey ?? ''];
  return id ? presets.value.find((p) => p.id === id) : undefined;
}
function fleetLine(agent: CodingAgent): string {
  const p = fleetPresetFor(agent);
  if (!p) return '';
  if (!p.available) return t('cap.coding.fleet.lineMissing', { label: p.label, cmd: p.command });
  if ((agent.command ?? '').startsWith('fleet:')) return t('cap.coding.fleet.lineViaFleet', { label: p.label });
  return agent.lastProbeOk
    ? t('cap.coding.fleet.lineBoth', { label: p.label })
    : t('cap.coding.fleet.lineTerminalOnly', { label: p.label, cmd: agent.command ?? '' });
}

// ——— 构件卡取值 ———
function commandLine(agent: CodingAgent): string {
  return [agent.command, ...(agent.args ?? [])].filter(Boolean).join(' ');
}

function agentStatus(agent: CodingAgent): CapStatus {
  if (!agent.enabled) return { tone: 'off', label: t('cap.common.disabled') };
  if (agent.lastProbeOk === true) return { tone: 'ok', label: t('cap.coding.probeOk') };
  if (agent.lastProbeError) return { tone: 'alert', label: t('cap.coding.probeFailed') };
  return { tone: 'pending', label: t('cap.coding.probeNever') };
}

const AUTONOMY_SHORT: Record<string, string> = {
  readonly: 'autonomyReadonly',
  workspace: 'autonomyWorkspace',
  full: 'autonomyFull',
};

function agentFacts(agent: CodingAgent): CapFact[] {
  const cmd = agent.command ?? '';
  // 运行台终端接入：真实权限由运行台里该工具的档位决定，显示「跟随运行台」而不是 ACP 的自治级别
  if (cmd.startsWith('fleet:')) {
    const mode = fleetPresetFor(agent)?.mode || 'manual';
    return [
      { label: t('cap.coding.access'), value: t('cap.coding.accessFleet') },
      {
        label: t('cap.coding.permission'),
        value: t(`cap.coding.modes.${mode}`),
        tone: mode === 'full' ? 'alert' : mode === 'auto_edit' ? 'watch' : undefined,
      },
      { label: t('cap.coding.capabilities'), value: (agent.capabilities ?? []).length },
    ];
  }
  const autonomy = agent.defaultAutonomy || 'readonly';
  const tone: CapTone | undefined = autonomy === 'full' ? 'alert' : autonomy === 'workspace' ? 'watch' : undefined;
  return [
    {
      label: t('cap.coding.access'),
      value: t(cmd.trim() === '@self' ? 'cap.coding.accessShim' : 'cap.coding.accessAcp'),
    },
    { label: t('cap.coding.autonomy'), value: t(`cap.coding.${AUTONOMY_SHORT[autonomy] ?? 'autonomyReadonly'}`), tone },
    { label: t('cap.coding.env'), value: Object.keys(agent.env ?? {}).length },
  ];
}

function agentTags(agent: CodingAgent): CapTag[] {
  return (agent.capabilities ?? []).map((c) => ({ label: CAPABILITY_OPTIONS.find((o) => o.value === c)?.label ?? c }));
}

function formatWhen(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
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

function openCreate() {
  editingAgent.value = null;
  addTab.value = connectableCli.value.length ? 'cli' : 'custom';
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

function openTryRun(agent: CodingAgent) {
  if (projects.value.length === 0) {
    $q.notify({ type: 'warning', message: t('cap.coding.needProject') });
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
      $q.notify({ type: 'positive', message: t('cap.coding.msg.dispatched') });
      void router.push({ path: '/coding-tasks', query: { focus: res.task.id } });
    } else if ((res.candidates ?? []).length > 0) {
      candidates.value = res.candidates ?? [];
      pendingDispatch.value = { agentKey: tryRunAgent.value.agentKey ?? '', prompt: TRY_RUN_PROMPT };
      candidatesOpen.value = true;
    } else {
      $q.notify({ type: 'warning', message: t('cap.coding.msg.noTask') });
    }
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('cap.coding.msg.dispatchFailed') });
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
      $q.notify({ type: 'positive', message: t('cap.coding.msg.dispatched') });
      void router.push({ path: '/coding-tasks', query: { focus: res.task.id } });
    }
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('cap.coding.msg.dispatchFailed') });
  }
}

function closeCandidates() {
  candidatesOpen.value = false;
  candidates.value = [];
  pendingDispatch.value = null;
}

// 删除编码工具：只有从没跑过任务的能删，后端会拒绝其余的，把原因原样告诉用户
async function confirmDeleteAgent(agent: CodingAgent) {
  const name = agent.displayName || agent.agentKey || '';
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

function openProjectCreate() {
  Object.assign(projectForm, { name: '', path: '', description: '' });
  projectEditing.value = false;
  projectDialogOpen.value = true;
}

function openProjectEdit(p: CodingProject) {
  Object.assign(projectForm, { name: p.name ?? '', path: p.path ?? '', description: p.description ?? '' });
  projectEditing.value = true;
  projectDialogOpen.value = true;
}

async function saveProject() {
  if (!projectForm.name || !projectForm.path) {
    $q.notify({ type: 'warning', message: t('cap.coding.msg.needProjectFields') });
    return;
  }
  projectSaving.value = true;
  try {
    await upsertCodingProject({
      name: projectForm.name,
      path: projectForm.path,
      description: projectForm.description,
    });
    $q.notify({ type: 'positive', message: t('cap.coding.msg.projectSaved') });
    projectDialogOpen.value = false;
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('cap.coding.msg.saveFailed') });
  } finally {
    projectSaving.value = false;
  }
}

async function confirmDeleteProject(p: CodingProject) {
  const ok = await confirm({
    title: t('cap.coding.msg.deleteProjectTitle'),
    message: t('cap.coding.msg.deleteProjectMessage', { name: p.name }),
    tone: 'danger',
  });
  if (!ok) return;
  try {
    await deleteCodingProject(p.id ?? '');
    $q.notify({ type: 'positive', message: t('cap.coding.msg.projectDeleted') });
    await loadAll();
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('cap.coding.msg.deleteFailed') });
  }
}
</script>

<style scoped lang="sass">
// 命令行工具：三列紧凑行（宽屏自动增减列），每行一个状态点 + 名称 / 命令 · 状态 + 一个动作
.cli-grid
  display: grid
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 300px), 1fr))
  gap: var(--nt-space-2)
  margin: 0
  padding: 0
  list-style: none
.cli-row
  display: flex
  align-items: center
  gap: var(--nt-space-3)
  min-height: 52px
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-tile-bg, var(--nt-bg-2))
.cli-row--off
  border-style: dashed
  background: transparent
.cli-row__main
  display: flex
  flex: 1
  flex-direction: column
  gap: 2px
  min-width: 0
.cli-row__name
  overflow: hidden
  color: var(--nt-text-1)
  font-weight: 600
  text-overflow: ellipsis
  white-space: nowrap
.cli-row--off .cli-row__name
  color: var(--nt-text-2)
  font-weight: 500
.cli-row__meta
  overflow: hidden
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
  text-overflow: ellipsis
  white-space: nowrap
.cli-row__linked
  display: inline-flex
  flex: none
  align-items: center
  gap: var(--nt-space-1)
  color: var(--nt-success)
  font-size: var(--nt-fs-caption)
.cli-toggle
  align-self: flex-start
  color: var(--nt-text-2)
.cli-tabs
  margin-bottom: var(--nt-space-3)
  border-bottom: 1px solid var(--nt-border-1)
.cli-pick
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
.cli-pick__hint
  max-width: 72ch
  margin: 0
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
</style>
