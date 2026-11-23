<template>
  <AppDialog
    v-model="open"
    size="lg"
    position="standard"
    :persistent="running"
    :title="t('cap.coding.tool.test.title', { name })"
    :subtitle="t('cap.coding.tool.test.subtitle')"
  >
    <div class="tool-test">
      <ol class="tool-test__steps">
        <li v-for="k in STEPS" :key="k" :class="`is-${steps[k]}`">
          <q-spinner v-if="steps[k] === 'run'" size="18px" />
          <q-icon v-else :name="ICON[steps[k]]" size="18px" aria-hidden="true" />
          <span>{{ t(`cap.coding.tool.test.steps.${k}`) }}</span>
        </li>
      </ol>

      <section v-if="verdict" class="tool-test__result" :class="passed ? 'is-ok' : 'is-fail'" role="status">
        <q-icon :name="passed ? 'check_circle' : 'error'" size="22px" aria-hidden="true" />
        <div class="tool-test__result-body">
          <strong>{{ verdict }}</strong>
          <span v-if="ms" class="mono">{{ t('cap.coding.tool.test.duration', { s: (ms / 1000).toFixed(1) }) }}</span>
          <pre v-if="reply" class="tool-test__reply mono">{{ t('cap.coding.tool.test.reply') }}：{{ reply }}</pre>
        </div>
      </section>

      <CodingToolConsole :title="t('cap.coding.tool.add.console')" :text="logText" :busy="running" />
    </div>

    <template #actions>
      <q-btn
        v-if="needLogin"
        flat
        rounded
        no-caps
        icon="login"
        :label="t('cap.coding.tool.test.goLogin')"
        @click="emit('login')"
      />
      <q-space />
      <q-btn flat rounded no-caps :disable="running" :label="t('cap.coding.tool.login.done')" @click="open = false" />
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        :loading="running"
        :label="t('cap.coding.tool.test.rerun')"
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
import { errText, listAgents, testAgent } from '../../fleet/api';

const open = defineModel<boolean>({ default: false });
const props = defineProps<{ agent: CodingAgent | null; preset?: AgentPreset }>();
const emit = defineEmits<{ changed: []; login: [] }>();
const { t } = useI18n();

type StepKey = 'installed' | 'probe' | 'chat' | 'verdict';
type StepState = 'wait' | 'run' | 'ok' | 'fail' | 'skip';
const STEPS: StepKey[] = ['installed', 'probe', 'chat', 'verdict'];
const ICON: Record<StepState, string> = {
  wait: 'radio_button_unchecked',
  run: 'pending',
  ok: 'check_circle',
  fail: 'cancel',
  skip: 'remove_circle_outline',
};

const steps = reactive<Record<StepKey, StepState>>({ installed: 'wait', probe: 'wait', chat: 'wait', verdict: 'wait' });
const running = ref(false);
const head = ref('');
const toolLog = ref('');
const verdict = ref('');
const reply = ref('');
const ms = ref(0);
const passed = ref(false);
const needLogin = ref(false);

const name = computed(() => props.preset?.label || props.agent?.displayName || props.agent?.agentKey || '');
const logText = computed(() => [head.value, toolLog.value].filter(Boolean).join('\n'));
const write = (line: string) => (head.value += (head.value ? '\n' : '') + line);
const sleep = (n: number) => new Promise((r) => setTimeout(r, n));

function finish(ok: boolean, text: string) {
  steps.verdict = ok ? 'ok' : 'fail';
  passed.value = ok;
  verdict.value = text;
}

watch(open, (v) => v && void run());

// 四步：已安装 → 能启动（探测）→ 能对话（非交互真发一句）→ 结论；全程留在弹窗里
async function run() {
  const agent = props.agent;
  if (!agent || running.value) return;
  running.value = true;
  Object.assign(steps, { installed: 'wait', probe: 'wait', chat: 'wait', verdict: 'wait' });
  head.value = toolLog.value = verdict.value = reply.value = '';
  ms.value = 0;
  needLogin.value = false;
  let p = props.preset;
  try {
    if (p) {
      steps.installed = p.available ? 'ok' : 'fail';
      write(
        p.available
          ? `✔ ${p.resolvedPath}${p.version ? ` · ${p.version}` : ''}`
          : `✖ ${t('cap.coding.tool.add.missing')}`,
      );
      if (!p.available) return finish(false, t('cap.coding.tool.add.missing'));
    } else {
      steps.installed = 'skip';
    }

    steps.probe = 'run';
    const r = await probeCodingAgent(agent.agentKey ?? '');
    emit('changed');
    steps.probe = r.ok ? 'ok' : 'fail';
    write(r.ok ? `✔ ${t('cap.coding.tool.test.steps.probe')}` : `✖ ${r.error}`);
    if (!r.ok) return finish(false, r.error || t('cap.coding.tool.status.failed'));

    if (!p?.supportsTest) {
      steps.chat = 'skip';
      return finish(true, t('cap.coding.tool.test.notSupported'));
    }

    steps.chat = 'run';
    await testAgent(p.id);
    for (;;) {
      await sleep(1200);
      const res = await listAgents();
      p = res.items.find((x) => x.id === props.preset?.id) ?? p;
      toolLog.value = p.testLog;
      if (p.testState !== 'running') break;
    }
    const ok = p.testState === 'ok';
    steps.chat = ok ? 'ok' : 'fail';
    reply.value = p.testReply;
    ms.value = p.testMs;
    needLogin.value = !ok && p.readiness === 'need_login';
    finish(ok, ok ? t('cap.coding.tool.test.pass') : p.testVerdict);
  } catch (e) {
    for (const k of STEPS) if (steps[k] === 'run') steps[k] = 'fail';
    finish(false, errText(e));
  } finally {
    running.value = false;
    emit('changed');
  }
}
</script>

<style scoped lang="sass">
.tool-test
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
.tool-test__steps
  display: grid
  grid-template-columns: repeat(4, minmax(0, 1fr))
  gap: var(--nt-space-2)
  margin: 0
  padding: 0
  list-style: none
  li
    display: flex
    align-items: center
    gap: var(--nt-space-2)
    min-height: 44px
    padding: var(--nt-space-2) var(--nt-space-3)
    border: 1px solid var(--nt-border-1)
    border-radius: var(--nt-radius-m)
    color: var(--nt-text-3)
    font-size: var(--nt-fs-caption)
  .is-run
    border-color: var(--nt-accent)
    color: var(--nt-accent)
  .is-ok
    color: var(--nt-success)
  .is-fail
    border-color: var(--nt-danger)
    color: var(--nt-danger)
  .is-skip
    color: var(--nt-text-3)
  @media (max-width: 559px)
    grid-template-columns: repeat(2, minmax(0, 1fr))
.tool-test__result
  display: flex
  align-items: flex-start
  gap: var(--nt-space-3)
  padding: var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  &.is-ok
    border-color: var(--nt-success)
    color: var(--nt-success)
  &.is-fail
    border-color: var(--nt-danger)
    color: var(--nt-danger)
.tool-test__result-body
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)
  min-width: 0
  color: var(--nt-text-1)
  span
    color: var(--nt-text-2)
    font-size: var(--nt-fs-caption)
.tool-test__reply
  margin: 0
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  white-space: pre-wrap
  overflow-wrap: anywhere
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
</style>
