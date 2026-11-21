<template>
  <q-dialog v-model="open" @before-show="load" @hide="stopPoll">
    <q-card class="fleet-agents">
      <q-card-section class="fleet-agents__title">CLI 工具</q-card-section>
      <q-card-section class="fleet-agents__body">
        <ol class="fleet-agents__steps">
          <li>
            <b>安装</b>：一键装到 <span class="mono">{{ cliRoot || '（未配置）' }}</span
            >，运行台自己找得到，不改系统 PATH。
          </li>
          <li><b>登录</b>：点「打开」在当前 worktree 里启动它，按提示登录（每家只需一次）。</li>
          <li>
            <b>权限</b>：选「自动改文件」或「完全放权」可让它不停下来问；默认逐项审批，确认框会在看板「等你」里给批准 /
            拒绝按钮。
          </li>
          <li>
            <b>派活</b>：「加入编码工具」后，aranea 的 Agent、Graph 的 Agent 节点、工作台「派给
            AI」都能把任务派给它，结果自动回流。
          </li>
        </ol>
        <p v-if="error" class="fleet-agents__error" role="alert">{{ error }}</p>
        <ul class="fleet-agents__list" aria-live="polite">
          <li v-for="p in items" :key="p.id" class="fleet-agents__item">
            <div class="fleet-agents__head">
              <span class="fleet-agents__name">{{ p.label }}</span>
              <span class="mono fleet-agents__cmd">{{ p.command }}</span>
              <span class="fleet-agents__state" :class="`is-${stateKey(p)}`">{{ stateText(p) }}</span>
              <span v-if="p.available" class="fleet-agents__state" :class="`is-ready-${p.readiness || 'unknown'}`">{{
                readyText(p)
              }}</span>
              <q-space />
              <q-btn
                v-if="p.available"
                dense
                flat
                no-caps
                label="打开"
                :disable="!state.currentWs"
                :aria-label="`在当前 worktree 打开 ${p.label}`"
                @click="openIn(p)"
              />
              <q-btn
                v-if="p.install"
                dense
                no-caps
                :unelevated="!p.available"
                :outline="p.available"
                :color="p.available ? undefined : 'primary'"
                :label="p.available ? '重装' : '安装'"
                :loading="p.installState === 'running'"
                :aria-label="`${p.available ? '重装' : '安装'} ${p.label}`"
                @click="install(p)"
              />
              <span v-else class="fleet-agents__manual">需手动安装</span>
            </div>
            <div v-if="p.available" class="mono fleet-agents__path">{{ p.resolvedPath }}</div>
            <p v-if="p.available && p.readiness !== 'ready' && p.loginHint" class="fleet-agents__login">
              登录：{{ p.loginHint }}
            </p>
            <div v-if="p.available" class="fleet-agents__settings">
              <q-btn-toggle
                v-model="drafts[p.id].mode"
                dense
                no-caps
                unelevated
                toggle-color="primary"
                :options="modeOptions(p)"
                :aria-label="`${p.label} 的默认权限`"
              />
              <q-input
                v-model="drafts[p.id].args"
                dense
                outlined
                class="fleet-agents__args mono"
                label="附加参数"
                placeholder="如 -m gpt-6-luna"
              />
              <q-btn
                dense
                outline
                no-caps
                label="保存"
                :disable="!dirty(p)"
                :loading="saving === p.id"
                :aria-label="`保存 ${p.label} 的设置`"
                @click="save(p)"
              />
              <span v-if="links[p.id]" class="fleet-agents__linked">已加入编码工具（{{ links[p.id] }}）</span>
              <q-btn
                v-else
                dense
                flat
                no-caps
                color="primary"
                label="加入编码工具"
                :aria-label="`把 ${p.label} 加入编码工具`"
                @click="link(p)"
              />
            </div>
            <q-expansion-item
              v-if="p.installLog"
              dense
              dense-toggle
              label="安装日志"
              :default-opened="p.installState === 'failed'"
              class="fleet-agents__logwrap"
            >
              <pre class="mono fleet-agents__log">{{ p.installLog }}</pre>
            </q-expansion-item>
          </li>
        </ul>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn v-close-popup flat no-caps label="关闭" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, reactive, ref } from 'vue';
import * as api from '../api';
import type { AgentPreset, PermissionMode } from '../api';
import { useFleet } from '../useFleet';

const open = defineModel<boolean>({ default: false });
const { state, newTerminal } = useFleet();

const items = ref<AgentPreset[]>([]);
const cliRoot = ref('');
const error = ref('');
const saving = ref('');
const links = ref<Record<string, string>>({});
const drafts = reactive<Record<string, { mode: PermissionMode; args: string }>>({});
let timer: ReturnType<typeof setTimeout> | null = null;

const argsText = (p: AgentPreset) => p.args.join(' ');

async function load() {
  try {
    const res = await api.listAgents();
    items.value = res.items.filter((p) => p.id !== 'shell');
    cliRoot.value = res.cliRoot;
    state.presets = res.items; // 新建 worktree 等处立刻能选到刚装好的工具
    for (const p of items.value) {
      if (!drafts[p.id] || !dirty(p)) drafts[p.id] = { mode: p.mode, args: argsText(p) };
    }
    error.value = '';
  } catch (e) {
    error.value = api.errText(e);
  }
  // 编码工具登记失败不挡面板（可能没有 coding 权限）
  links.value = await api.listCodingToolLinks().catch(() => ({}));
  stopPoll();
  if (open.value && items.value.some((p) => p.installState === 'running')) timer = setTimeout(load, 2000);
}

function stopPoll() {
  if (timer) clearTimeout(timer);
  timer = null;
}

async function install(p: AgentPreset) {
  try {
    await api.installAgent(p.id);
  } catch (e) {
    error.value = `${p.label}：${api.errText(e)}`;
  }
  await load();
}

async function openIn(p: AgentPreset) {
  open.value = false;
  await newTerminal(state.currentWs, p.id);
}

function modeOptions(p: AgentPreset) {
  return [
    { label: '逐项审批', value: 'manual' },
    { label: '自动改文件', value: 'auto_edit', disable: !p.autoEditFlag },
    { label: '完全放权', value: 'full', disable: !p.supportsFullAccess },
  ];
}

function dirty(p: AgentPreset) {
  const d = drafts[p.id];
  return !!d && (d.mode !== p.mode || d.args.trim() !== argsText(p));
}

async function save(p: AgentPreset) {
  const d = drafts[p.id];
  if (!d) return;
  saving.value = p.id;
  try {
    const args = d.args.trim() ? d.args.trim().split(/\s+/) : [];
    const next = await api.updateAgentSetting(p.id, d.mode, args);
    items.value = items.value.map((x) => (x.id === p.id ? next : x));
    drafts[p.id] = { mode: next.mode, args: argsText(next) };
    error.value = '';
  } catch (e) {
    error.value = `${p.label}：${api.errText(e)}`;
  } finally {
    saving.value = '';
  }
}

async function link(p: AgentPreset) {
  try {
    await api.addToCodingTools(p);
    links.value = { ...links.value, [p.id]: `cli_${p.id}` };
  } catch (e) {
    error.value = `${p.label}：${api.errText(e)}`;
  }
}

function stateKey(p: AgentPreset): string {
  if (p.installState === 'running') return 'running';
  if (p.installState === 'failed') return 'failed';
  return p.available ? 'ok' : 'missing';
}

function stateText(p: AgentPreset): string {
  return { running: '安装中', failed: '安装失败', ok: '已安装', missing: '未安装' }[stateKey(p)] ?? '';
}

function readyText(p: AgentPreset): string {
  return { need_login: '要登录', need_trust: '要信任目录', ready: '能对话' }[p.readiness] ?? '还没在运行台用过';
}

onBeforeUnmount(stopPoll);
</script>

<style scoped lang="sass">
.fleet-agents
  width: 760px
  max-width: 94vw
  background: var(--nt-bg-2)
.fleet-agents__title
  font-size: var(--nt-fs-panel)
  font-weight: 600
  color: var(--nt-text-0)
.fleet-agents__body
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
  max-height: 72vh
  overflow-y: auto
.fleet-agents__steps
  max-width: 72ch
  margin: 0
  padding-left: var(--nt-space-5)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  b
    color: var(--nt-text-1)
.fleet-agents__error
  margin: 0
  color: var(--nt-danger)
  font-size: var(--nt-fs-caption)
.fleet-agents__list
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  margin: 0
  padding: 0
  list-style: none
.fleet-agents__item
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-1)
.fleet-agents__head, .fleet-agents__settings
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-2)
.fleet-agents__settings
  margin-top: var(--nt-space-2)
.fleet-agents__args
  flex: 1
  min-width: 160px
.fleet-agents__name
  color: var(--nt-text-1)
  font-weight: 600
.fleet-agents__cmd, .fleet-agents__path, .fleet-agents__manual
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.fleet-agents__path
  margin-top: var(--nt-space-1)
  overflow-wrap: anywhere
.fleet-agents__login
  max-width: 72ch
  margin: var(--nt-space-1) 0 0
  color: var(--nt-warn)
  font-size: var(--nt-fs-caption)
.fleet-agents__linked
  color: var(--nt-success)
  font-size: var(--nt-fs-micro)
.fleet-agents__state
  font-size: var(--nt-fs-micro)
  &.is-ok, &.is-ready-ready
    color: var(--nt-success)
  &.is-running
    color: var(--nt-accent)
  &.is-failed
    color: var(--nt-danger)
  &.is-missing, &.is-ready-unknown
    color: var(--nt-text-3)
  &.is-ready-need_login, &.is-ready-need_trust
    color: var(--nt-warn)
.fleet-agents__logwrap
  margin-top: var(--nt-space-1)
  color: var(--nt-text-2)
.fleet-agents__log
  max-height: 220px
  margin: 0
  padding: var(--nt-space-2)
  overflow: auto
  border-radius: var(--nt-radius-s)
  background: var(--nt-data-bg)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-micro)
  white-space: pre-wrap
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
</style>
