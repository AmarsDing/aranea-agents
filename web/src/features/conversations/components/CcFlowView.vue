<template>
  <!-- 流程（Graph）的一次运行：走到哪一步、每一步成没成；画布实况、检查点去流程的运行页看 -->
  <div class="cc-xview">
    <div class="cc-xview__body">
      <div class="cc-xview__in">
        <p v-if="error" class="cc-xview__empty" role="alert">{{ error }}</p>
        <p v-else-if="!run" class="cc-xview__empty">{{ t('cc.loading') }}</p>
        <template v-else>
          <p class="cc-flow__now" aria-live="polite">
            <span class="cc-dot" :class="`cc-dot--${exec.status}`" aria-hidden="true" />
            {{ now }}
          </p>
          <p v-if="run.errorMessage" class="cc-np__warn" role="alert">{{ run.errorMessage }}</p>
          <ol v-if="run.steps.length" class="cc-flow__steps">
            <li v-for="s in run.steps" :key="`${s.stepIndex}-${s.nodeId}`" class="cc-flow__step">
              <span class="cc-num">{{ s.stepIndex + 1 }}</span>
              <b>{{ s.nodeId }}</b>
              <span :class="{ 'cc-flow__bad': s.status === 'failed' }">{{ stepStatus(s.status) }}</span>
              <time class="cc-num">{{ clock(s.timestamp) }}</time>
              <small v-if="s.error">{{ s.error }}</small>
            </li>
          </ol>
          <p v-else class="cc-xview__empty">{{ t('cc.flow.noSteps') }}</p>
        </template>
      </div>
    </div>
    <footer class="cc-xview__dock">
      <span class="cc-xview__note">{{ t('cc.flow.via') }}</span>
      <q-btn
        v-if="exec.status === 'running' || exec.status === 'waiting'"
        flat
        dense
        no-caps
        icon="stop"
        :label="t('cc.flow.stop')"
        :loading="stopping"
        @click="stop"
      />
      <q-btn flat dense no-caps icon="open_in_new" :label="t('cc.flow.open')" :to="runLink" />
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { cancelGraphExecution, getGraphExecution } from '../../graph/api';
import type { GraphExecution } from '../../graph/types';
import { parseKratosApiError } from '../../../utils/kratosError';
import type { Execution } from '../model';

const POLL_MS = 5000;

const props = defineProps<{ exec: Execution }>();
const emit = defineEmits<{ changed: [] }>();
const { t, te } = useI18n();

const run = ref<GraphExecution | null>(null);
const error = ref('');
const stopping = ref(false);
let timer: ReturnType<typeof setTimeout> | null = null;

const graphId = computed(() => props.exec.executor.replace(/^flow:/, ''));
const runLink = computed(
  () => `/graphs/${encodeURIComponent(graphId.value)}/run/${encodeURIComponent(props.exec.refId)}`,
);
const live = (status: string) => status === 'running' || status === 'pending';

async function load() {
  if (timer) clearTimeout(timer);
  const id = props.exec.refId;
  try {
    const got = await getGraphExecution(id);
    if (id !== props.exec.refId) return;
    const was = run.value?.status;
    run.value = got;
    error.value = '';
    if (was && was !== got.status) emit('changed');
    if (live(got.status)) timer = setTimeout(() => void load(), POLL_MS);
  } catch (err) {
    error.value = parseKratosApiError(err).message || t('cc.flow.gone');
  }
}
watch(
  () => props.exec.refId,
  () => {
    run.value = null;
    void load();
  },
  { immediate: true },
);
onUnmounted(() => {
  if (timer) clearTimeout(timer);
});

const stepStatus = (s: string) => (te(`cc.flow.step.${s}`) ? t(`cc.flow.step.${s}`) : s);
const now = computed(() => {
  const x = run.value;
  if (!x) return '';
  if (x.status === 'interrupted') return t('cc.flow.nowWaiting', { node: x.interruptNode || x.currentNode });
  if (live(x.status)) return t('cc.flow.nowRunning', { node: x.currentNode || '…' });
  if (x.status === 'failed') return t('cc.flow.nowFailed', { node: x.currentNode || '…' });
  if (x.status === 'completed') return t('cc.flow.nowDone', { n: x.steps.length });
  return stepStatus(x.status);
});
const clock = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString([], { hour12: false });
};

async function stop() {
  stopping.value = true;
  try {
    await cancelGraphExecution(props.exec.refId);
    await load();
    emit('changed');
  } catch (err) {
    error.value = parseKratosApiError(err).message;
  } finally {
    stopping.value = false;
  }
}
</script>
