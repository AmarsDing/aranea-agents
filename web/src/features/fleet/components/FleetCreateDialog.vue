<template>
  <!-- 重置表单必须在 before-show：@show 在开场动画之后才触发，会清掉用户已经打的字。 -->
  <q-dialog v-model="state.createOpen" @before-show="onShow">
    <q-card class="fleet-create">
      <q-card-section class="fleet-create__title">新建 worktree</q-card-section>
      <q-card-section class="fleet-create__body">
        <div class="fleet-create__row">
          <q-select
            v-model="projectId"
            class="col"
            dense
            outlined
            emit-value
            map-options
            label="项目"
            :options="projectOptions"
          />
          <q-select
            v-model="startFrom"
            class="col"
            dense
            outlined
            use-input
            fill-input
            hide-selected
            input-debounce="0"
            label="起点"
            :options="refOptions"
            @filter="filterRefs"
            @input-value="(v: string) => (startFrom = v)"
          />
        </div>
        <q-input
          v-model="name"
          dense
          outlined
          autofocus
          label="任务名"
          placeholder="例如 fix-login-race（留空自动命名）"
          @keydown.enter.prevent="submit"
        />
        <q-select v-model="agent" dense outlined emit-value map-options label="Agent" :options="agentOptions">
          <template #after>
            <q-toggle v-model="fullAccess" dense size="sm" label="完全放权" :disable="!agentSupportsFull" />
          </template>
        </q-select>
        <div v-if="fullAccess" class="fleet-create__warn" role="note">
          完全放权会让 agent 不经逐项批准就执行命令（{{ fullFlag }}）。worktree 只隔离文件，不隔离网络与本机其他目录。
        </div>
        <q-input
          v-if="agent"
          v-model="prompt"
          dense
          outlined
          autogrow
          type="textarea"
          label="第一句话（可选）"
          placeholder="agent 启动后自动发给它；留空就只打开 agent"
        />
        <q-expansion-item dense dense-toggle label="高级" class="fleet-create__adv">
          <div class="fleet-create__advbody">
            <q-input
              v-model="branch"
              dense
              outlined
              label="分支名（留空由任务名推导）"
              placeholder="feature/my-branch"
            />
            <q-toggle v-model="fetch" dense label="创建前先 git fetch" />
          </div>
        </q-expansion-item>
      </q-card-section>
      <q-card-actions class="fleet-create__actions">
        <q-toggle v-model="more" dense size="sm" label="继续创建" />
        <q-space />
        <q-btn v-close-popup flat no-caps label="取消" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          label="创建（Enter）"
          :loading="busy"
          :disable="!projectId"
          @click="submit"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { Notify } from 'quasar';
import * as api from '../api';
import { useFleet } from '../useFleet';

const { state, availableAgents, refreshWorkspaces, selectWorkspace } = useFleet();

const projectId = ref('');
const name = ref('');
const startFrom = ref('');
const agent = ref('');
const prompt = ref('');
const fullAccess = ref(false);
const branch = ref('');
const fetch = ref(false);
const more = ref(false);
const busy = ref(false);
const refs = ref<string[]>([]);
const refOptions = ref<string[]>([]);

const AGENT_KEY = 'fleet:create:agent';

const projectOptions = computed(() =>
  state.projects.filter((p) => p.isGit).map((p) => ({ label: p.name, value: p.id })),
);
const agentOptions = computed(() => [
  { label: '不启动 agent（空 worktree）', value: '' },
  ...availableAgents.value.map((a) => ({ label: a.label, value: a.id })),
]);
const preset = computed(() => state.presets.find((p) => p.id === agent.value));
const agentSupportsFull = computed(() => !!preset.value?.supportsFullAccess);
const fullFlag = computed(() => (preset.value?.supportsFullAccess ? `${preset.value.command} 的免批准参数` : ''));

function onShow() {
  projectId.value = state.createProjectId || projectId.value || state.projects[0]?.id || '';
  name.value = '';
  prompt.value = '';
  branch.value = '';
  try {
    agent.value = localStorage.getItem(AGENT_KEY) ?? availableAgents.value[0]?.id ?? '';
  } catch {
    agent.value = availableAgents.value[0]?.id ?? '';
  }
  if (agent.value && !availableAgents.value.some((a) => a.id === agent.value))
    agent.value = availableAgents.value[0]?.id ?? '';
}

watch(projectId, async (id) => {
  if (!id) return;
  try {
    const r = await api.listRefs(id);
    refs.value = [...r.local, ...r.remote];
    refOptions.value = refs.value;
    startFrom.value = r.defaultRef;
  } catch {
    refs.value = [];
  }
});

watch(agent, (a) => {
  if (!preset.value?.supportsFullAccess) fullAccess.value = false;
  try {
    localStorage.setItem(AGENT_KEY, a);
  } catch {
    /* 只是记住上次选择 */
  }
});

function filterRefs(val: string, update: (fn: () => void) => void) {
  update(() => {
    const q = val.toLowerCase();
    refOptions.value = q ? refs.value.filter((r) => r.toLowerCase().includes(q)) : refs.value;
  });
}

async function submit() {
  if (!projectId.value || busy.value) return;
  busy.value = true;
  try {
    const w = await api.createWorkspace({
      projectId: projectId.value,
      name: name.value.trim(),
      branch: branch.value.trim(),
      startFrom: startFrom.value.trim(),
      agent: agent.value,
      prompt: prompt.value.trim(),
      fullAccess: fullAccess.value,
      fetch: fetch.value,
    });
    Notify.create({ type: 'info', message: `正在创建 worktree「${w.name}」（分支 ${w.branch}）` });
    await refreshWorkspaces();
    selectWorkspace(w.id);
    if (more.value) {
      name.value = '';
      prompt.value = '';
      branch.value = '';
    } else {
      state.createOpen = false;
    }
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  } finally {
    busy.value = false;
  }
}
</script>

<style scoped lang="sass">
.fleet-create
  width: 560px
  max-width: 94vw
  background: var(--nt-bg-2)
.fleet-create__title
  font-size: var(--nt-fs-panel)
  font-weight: 600
  color: var(--nt-text-0)
.fleet-create__body
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
.fleet-create__row
  display: flex
  gap: var(--nt-space-2)
.fleet-create__warn
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-warn)
  border-radius: var(--nt-radius-m)
  color: var(--nt-warn)
  font-size: var(--nt-fs-caption)
.fleet-create__adv
  border-radius: var(--nt-radius-m)
  color: var(--nt-text-2)
.fleet-create__advbody
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  padding: var(--nt-space-2) 0
.fleet-create__actions
  padding: var(--nt-space-2) var(--nt-space-4) var(--nt-space-4)
</style>
