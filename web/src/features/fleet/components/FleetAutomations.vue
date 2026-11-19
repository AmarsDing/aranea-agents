<template>
  <div class="fleet-auto">
    <header class="fleet-auto__bar">
      <h2>自动化</h2>
      <span class="fleet-auto__sub"
        >按计划在项目上启动 agent 执行一段提示：夜间升依赖、每周分拣、定期清理不稳定测试。</span
      >
      <q-space />
      <q-btn dense unelevated no-caps color="primary" icon="add" label="新建" @click="edit()" />
    </header>

    <table class="fleet-auto__table">
      <thead>
        <tr>
          <th>名称</th>
          <th>项目 / agent</th>
          <th>计划</th>
          <th>下次</th>
          <th>上次</th>
          <th>启用</th>
          <th />
        </tr>
      </thead>
      <tbody>
        <tr v-for="a in items" :key="a.id">
          <td>
            <div class="fleet-auto__name">{{ a.name }}</div>
            <div class="fleet-auto__muted">{{ a.prompt }}</div>
          </td>
          <td>
            {{ projectName(a.projectId) }}
            <div class="fleet-auto__muted">
              {{ agentLabel(a.agent) }} · {{ a.worktreeMode === 'new' ? '每次新建 worktree' : '主检出' }}
            </div>
          </td>
          <td class="mono">{{ a.schedule }}</td>
          <td class="mono">{{ fmt(a.nextRunAt) }}</td>
          <td>
            <span class="mono">{{ fmt(a.lastRunAt) }}</span>
            <div
              v-if="a.lastStatus"
              class="fleet-auto__muted"
              :class="{ 'is-fail': a.lastStatus.startsWith('failed') }"
            >
              {{ a.lastStatus }}
            </div>
          </td>
          <td>
            <q-toggle
              :model-value="a.enabled"
              dense
              :aria-label="`启用 ${a.name}`"
              @update:model-value="(v: boolean) => toggle(a, v)"
            />
          </td>
          <td class="fleet-auto__ops">
            <q-btn dense flat no-caps size="sm" label="立即运行" @click="runNow(a)" />
            <q-btn dense flat round size="sm" icon="edit" :aria-label="`编辑 ${a.name}`" @click="edit(a)" />
            <q-btn dense flat round size="sm" icon="delete" :aria-label="`删除 ${a.name}`" @click="remove(a)" />
          </td>
        </tr>
        <tr v-if="items.length === 0">
          <td colspan="7" class="fleet-auto__muted">还没有自动化。</td>
        </tr>
      </tbody>
    </table>

    <q-dialog v-model="open">
      <q-card class="fleet-auto__dialog">
        <q-card-section class="fleet-auto__dtitle">{{ form.id ? '编辑自动化' : '新建自动化' }}</q-card-section>
        <q-card-section class="fleet-auto__form">
          <q-input v-model="form.name" dense outlined label="名称" />
          <div class="fleet-auto__row">
            <q-select
              v-model="form.projectId"
              class="col"
              dense
              outlined
              emit-value
              map-options
              label="项目"
              :options="projectOptions"
            />
            <q-select
              v-model="form.agent"
              class="col"
              dense
              outlined
              emit-value
              map-options
              label="agent"
              :options="agentOptions"
            />
          </div>
          <q-input
            v-model="form.prompt"
            dense
            outlined
            autogrow
            type="textarea"
            label="提示"
            placeholder="例如：检查依赖更新，升级小版本并跑测试，失败就回滚"
          />
          <div class="fleet-auto__row">
            <q-select
              v-model="form.schedule"
              class="col"
              dense
              outlined
              use-input
              fill-input
              hide-selected
              new-value-mode="add-unique"
              label="计划"
              hint="every 6h / daily 02:00 / weekdays 09:30 / weekly mon 09:00（本地时间）"
              :options="SCHEDULES"
              @input-value="(v: string) => (form.schedule = v)"
            />
            <q-select
              v-model="form.worktreeMode"
              class="col"
              dense
              outlined
              emit-value
              map-options
              label="在哪里跑"
              :options="[
                { label: '每次新建 worktree', value: 'new' },
                { label: '项目主检出', value: 'primary' },
              ]"
            />
          </div>
          <div class="fleet-auto__row">
            <q-toggle v-model="form.enabled" dense label="启用" />
            <q-toggle v-model="form.fullAccess" dense label="完全放权" />
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps label="取消" />
          <q-btn
            unelevated
            no-caps
            color="primary"
            label="保存"
            :disable="!form.projectId || !form.prompt.trim() || !form.schedule.trim()"
            @click="save"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { Dialog, Notify } from 'quasar';
import * as api from '../api';
import type { Automation } from '../api';
import { useFleet } from '../useFleet';

const { state, availableAgents, agentLabel, refreshWorkspaces, refreshTerminals } = useFleet();
const SCHEDULES = ['every 6h', 'daily 02:00', 'weekdays 09:30', 'weekly mon 09:00'];

const items = ref<Automation[]>([]);
const open = ref(false);
const form = reactive({
  id: '',
  name: '',
  projectId: '',
  agent: '',
  prompt: '',
  schedule: 'daily 02:00',
  worktreeMode: 'new',
  enabled: true,
  fullAccess: false,
});

const projectOptions = computed(() =>
  state.projects.filter((p) => p.isGit).map((p) => ({ label: p.name, value: p.id })),
);
const agentOptions = computed(() => availableAgents.value.map((a) => ({ label: a.label, value: a.id })));
const projectName = (id: string) => state.projects.find((p) => p.id === id)?.name ?? id;
const fmt = (iso: string) =>
  iso
    ? new Date(iso).toLocaleString('zh-CN', {
        hour12: false,
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      })
    : '—';

async function load() {
  items.value = await api.listAutomations();
}

function edit(a?: Automation) {
  Object.assign(
    form,
    a
      ? {
          id: a.id,
          name: a.name,
          projectId: a.projectId,
          agent: a.agent,
          prompt: a.prompt,
          schedule: a.schedule,
          worktreeMode: a.worktreeMode,
          enabled: a.enabled,
          fullAccess: a.fullAccess,
        }
      : {
          id: '',
          name: '',
          projectId: state.projects[0]?.id ?? '',
          agent: availableAgents.value[0]?.id ?? '',
          prompt: '',
          schedule: 'daily 02:00',
          worktreeMode: 'new',
          enabled: true,
          fullAccess: false,
        },
  );
  open.value = true;
}

async function save() {
  try {
    await api.upsertAutomation({ ...form, prompt: form.prompt.trim(), schedule: form.schedule.trim() });
    open.value = false;
    await load();
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  }
}

async function toggle(a: Automation, enabled: boolean) {
  await api.upsertAutomation({ ...a, enabled });
  await load();
}

async function runNow(a: Automation) {
  try {
    await api.runAutomationNow(a.id);
    Notify.create({ type: 'info', message: `已启动「${a.name}」` });
    await Promise.all([load(), refreshWorkspaces(), refreshTerminals()]);
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
  }
}

function remove(a: Automation) {
  Dialog.create({
    title: `删除自动化「${a.name}」？`,
    message: '已经跑出来的 worktree 不受影响。',
    cancel: true,
    ok: { label: '删除', color: 'negative' },
  }).onOk(async () => {
    await api.deleteAutomation(a.id);
    await load();
  });
}

onMounted(load);
</script>

<style scoped lang="sass">
.fleet-auto
  height: 100%
  overflow: auto
  padding: var(--nt-space-4)
  color: var(--nt-text-1)
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.fleet-auto__bar
  display: flex
  align-items: center
  gap: var(--nt-space-3)
  margin-bottom: var(--nt-space-3)
  h2
    margin: 0
    font-size: var(--nt-fs-title)
    color: var(--nt-text-0)
.fleet-auto__sub
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
.fleet-auto__table
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
.fleet-auto__name
  color: var(--nt-text-0)
.fleet-auto__muted
  max-width: 420px
  overflow: hidden
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
  text-overflow: ellipsis
  white-space: nowrap
  &.is-fail
    color: var(--nt-danger)
.fleet-auto__ops
  white-space: nowrap
.fleet-auto__dialog
  width: 620px
  max-width: 94vw
  background: var(--nt-bg-2)
.fleet-auto__dtitle
  font-size: var(--nt-fs-panel)
  font-weight: 600
.fleet-auto__form
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
.fleet-auto__row
  display: flex
  align-items: flex-start
  gap: var(--nt-space-3)
</style>
