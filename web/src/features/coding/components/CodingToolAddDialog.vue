<template>
  <AppDialog
    v-model="open"
    size="xl"
    :persistent="running"
    :title="t('cap.coding.tool.add.title')"
    :subtitle="t('cap.coding.tool.add.subtitle')"
  >
    <div class="tool-add">
      <p class="tool-add__dir">
        <q-icon name="folder_open" size="16px" aria-hidden="true" />
        <span class="mono">{{ t('cap.coding.tool.add.installTo', { dir: cliRoot || '—' }) }}</span>
      </p>
      <p v-if="error" class="tool-add__error" role="alert">{{ error }}</p>

      <ul class="tool-add__grid" role="list">
        <li v-for="p in tools" :key="p.id">
          <label class="tool-add__item" :class="{ 'is-added': added(p), 'is-picked': picked.has(p.id) }">
            <q-checkbox
              dense
              :model-value="picked.has(p.id) || added(p)"
              :disable="added(p) || running"
              :aria-label="p.label"
              @update:model-value="togglePick(p.id)"
            />
            <img v-if="toolIcon(p.id)" :src="toolIcon(p.id)" class="tool-add__logo" alt="" />
            <span v-else class="tool-add__logo tool-add__logo--letter" aria-hidden="true">{{
              p.label.slice(0, 1)
            }}</span>
            <span class="tool-add__name">{{ p.label }}</span>
            <span class="tool-add__state" :class="stateClass(p)">{{ stateText(p) }}</span>
          </label>
        </li>
      </ul>

      <!-- 处理进度：每个工具一个状态标识，成功 ✔ / 失败 ✖ -->
      <ol v-if="queue.length" class="tool-add__queue" aria-label="queue">
        <li v-for="q in queue" :key="q.id" :class="`is-${q.step}`">
          <q-icon :name="STEP_ICON[q.step]" size="16px" aria-hidden="true" />
          {{ q.label }} · {{ t(`cap.coding.tool.add.step.${q.step}`) }}
        </li>
      </ol>

      <CodingToolConsole
        :title="t('cap.coding.tool.add.console')"
        :text="log"
        :placeholder="t('cap.coding.tool.add.idle')"
        :busy="running"
      />
    </div>

    <template #actions>
      <q-btn flat rounded no-caps :disable="running" :label="t('cap.coding.tool.add.custom')" @click="emit('custom')" />
      <q-space />
      <q-btn flat rounded no-caps :disable="running" :label="t('cap.coding.form.cancel')" @click="open = false" />
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        :disable="picked.size === 0"
        :loading="running"
        :label="t('cap.coding.tool.add.start', { n: picked.size })"
        @click="run"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../../../components/common/AppDialog.vue';
import CodingToolConsole from './CodingToolConsole.vue';
import type { CodingAgent } from '../api';
import { probeCodingAgent } from '../api';
import type { AgentPreset } from '../../fleet/api';
import { addToCodingTools, errText, installAgent, listAgents, presetOfAgent, toolIcon } from '../../fleet/api';

const open = defineModel<boolean>({ default: false });
const props = defineProps<{ agents: CodingAgent[] }>();
const emit = defineEmits<{ changed: []; custom: [] }>();
const { t } = useI18n();

type Step = 'queued' | 'install' | 'register' | 'probe' | 'done' | 'failed';
const STEP_ICON: Record<Step, string> = {
  queued: 'schedule',
  install: 'downloading',
  register: 'how_to_reg',
  probe: 'network_check',
  done: 'check_circle',
  failed: 'cancel',
};

const presets = ref<AgentPreset[]>([]);
const cliRoot = ref('');
const error = ref('');
const picked = reactive(new Set<string>());
const queue = ref<{ id: string; label: string; step: Step }[]>([]);
const log = ref('');
const running = ref(false);

// 已装的排前面；终端（shell）不是工具
const tools = computed(() =>
  presets.value.filter((p) => p.id !== 'shell').sort((a, b) => Number(b.available) - Number(a.available)),
);
const added = (p: AgentPreset) => props.agents.some((a) => presetOfAgent(a, [p]) === p);

async function load() {
  if (running.value) return;
  picked.clear();
  queue.value = [];
  log.value = '';
  try {
    const res = await listAgents();
    presets.value = res.items;
    cliRoot.value = res.cliRoot;
    error.value = '';
  } catch (e) {
    error.value = errText(e);
  }
}

watch(open, (v) => v && void load());

function togglePick(id: string) {
  if (picked.has(id)) picked.delete(id);
  else picked.add(id);
}

function stateText(p: AgentPreset) {
  if (added(p)) return t('cap.coding.tool.add.added');
  return p.available ? t('cap.coding.tool.add.installed') : t('cap.coding.tool.add.missing');
}
function stateClass(p: AgentPreset) {
  return added(p) ? 'is-added' : p.available ? 'is-ok' : 'is-missing';
}

const write = (line: string) => (log.value += (log.value ? '\n' : '') + line);
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

// 依次处理：没装的先装（实时转写安装日志）→ 登记成编码工具 → 探测
async function run() {
  running.value = true;
  const ids = tools.value.filter((p) => picked.has(p.id)).map((p) => p.id);
  queue.value = ids.map((id) => ({ id, label: tools.value.find((p) => p.id === id)?.label ?? id, step: 'queued' }));
  let ok = 0;
  let fail = 0;
  for (const q of queue.value) {
    write(`━━ ${q.label}`);
    try {
      let p = presets.value.find((x) => x.id === q.id) as AgentPreset;
      if (!p.available) {
        q.step = 'install';
        write(`▶ ${t('cap.coding.tool.add.step.install')} → ${cliRoot.value}`);
        await installAgent(q.id);
        let seen = 0;
        for (;;) {
          await sleep(1500);
          const res = await listAgents();
          presets.value = res.items;
          p = res.items.find((x) => x.id === q.id) as AgentPreset;
          const text = p.installLog;
          if (text.length < seen) seen = 0; // 日志只留尾部，被截断时从头再看
          if (text.length > seen) {
            write(text.slice(seen).trimEnd());
            seen = text.length;
          }
          if (p.installState !== 'running') break;
        }
        if (p.installState === 'failed' || !p.available) throw new Error(t('cap.coding.tool.add.step.failed'));
      } else {
        write(`✔ ${t('cap.coding.tool.add.installed')}：${p.resolvedPath}${p.version ? ` · ${p.version}` : ''}`);
      }
      q.step = 'register';
      await addToCodingTools(p);
      write(`✔ ${t('cap.coding.tool.add.step.register')}：cli_${p.id}`);
      q.step = 'probe';
      const r = await probeCodingAgent(`cli_${p.id}`);
      if (!r.ok) throw new Error(r.error || t('cap.coding.tool.status.failed'));
      write(`✔ ${t('cap.coding.tool.add.step.probe')}：${t('cap.coding.tool.status.ok')}`);
      q.step = 'done';
      ok++;
    } catch (e) {
      q.step = 'failed';
      fail++;
      write(`✖ ${errText(e)}`);
    }
    picked.delete(q.id);
    emit('changed');
  }
  write('');
  write(t('cap.coding.tool.add.allDone', { ok, fail }));
  running.value = false;
}
</script>

<style scoped lang="sass">
.tool-add
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
.tool-add__dir
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  margin: 0
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
.tool-add__error
  margin: 0
  color: var(--nt-danger)
  font-size: var(--nt-fs-caption)
.tool-add__grid
  display: grid
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 220px), 1fr))
  gap: var(--nt-space-2)
  margin: 0
  padding: 0
  list-style: none
.tool-add__item
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  min-height: 48px
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-tile-bg, var(--nt-bg-2))
  cursor: pointer
  &.is-picked
    border-color: var(--nt-accent)
  &.is-added
    cursor: default
    opacity: 0.7
  &:focus-within
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.tool-add__logo
  flex: none
  width: 24px
  height: 24px
  border-radius: var(--nt-radius-s)
  object-fit: cover
.tool-add__logo--letter
  display: inline-flex
  align-items: center
  justify-content: center
  background: var(--nt-accent-soft)
  color: var(--nt-accent)
  font-size: var(--nt-fs-caption)
  font-weight: 600
.tool-add__name
  flex: 1
  min-width: 0
  overflow: hidden
  color: var(--nt-text-1)
  font-weight: 600
  text-overflow: ellipsis
  white-space: nowrap
.tool-add__state
  flex: none
  font-size: var(--nt-fs-micro)
  &.is-ok
    color: var(--nt-success)
  &.is-missing
    color: var(--nt-text-3)
  &.is-added
    color: var(--nt-text-2)
.tool-add__queue
  display: flex
  flex-wrap: wrap
  gap: var(--nt-space-2) var(--nt-space-4)
  margin: 0
  padding: 0
  list-style: none
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  li
    display: inline-flex
    align-items: center
    gap: var(--nt-space-1)
  .is-done
    color: var(--nt-success)
  .is-failed
    color: var(--nt-danger)
  .is-install, .is-register, .is-probe
    color: var(--nt-accent)
.mono
  font-family: var(--nt-font-mono)
</style>
