<template>
  <!--
    编码工具桥（ACP）的一次执行：你的提示 → 工具的回答、工具调用（收起）、计划、待批准、错误。
    在跑的时候每 3 秒接着读新事件；结束后可以验收或归档。待批准的去审批中心处理（P1 不在这里直接批）。
  -->
  <div class="cc-xview">
    <div class="cc-xview__body">
      <div class="cc-xview__in">
        <div class="cc-turn cc-turn--me">
          <span class="cc-glyph cc-glyph--me" aria-hidden="true">{{ t('cc.you')[0] }}</span>
          <div>
            <div class="cc-turn__who">
              <b>{{ t('cc.you') }}</b>
            </div>
            <div class="cc-turn__text">{{ prompt || exec.what }}</div>
          </div>
        </div>
        <template v-for="(block, i) in blocks" :key="i">
          <div class="cc-turn">
            <CcGlyph :executor="exec.executor" :name="exec.executorName" />
            <div>
              <template v-if="block.kind === 'ai'">
                <div class="cc-turn__who">
                  <b>{{ exec.executorName }}</b>
                </div>
                <div class="cc-turn__text">{{ block.text }}</div>
              </template>
              <details v-else-if="block.kind === 'tools'" class="cc-tools">
                <summary>{{ t('cc.bridge.tools', { n: block.items.length }) }} · {{ toolNames(block.items) }}</summary>
                <ol>
                  <li v-for="(item, j) in block.items" :key="j">
                    <b>{{ item.title }}</b> {{ item.text }}
                  </li>
                </ol>
              </details>
              <div v-else-if="block.kind === 'ask'" class="cc-ask" role="group" :aria-label="t('cc.bridge.askLabel')">
                <div class="cc-ask__q">
                  {{ t('cc.bridge.ask', { what: block.title }) }} <code v-if="block.text">{{ block.text }}</code>
                </div>
                <div class="cc-ask__acts">
                  <q-btn unelevated dense no-caps color="primary" to="/approvals" :label="t('cc.bridge.toApprovals')" />
                </div>
              </div>
              <div v-else-if="block.kind === 'err'" class="cc-err">{{ block.title }} {{ block.text }}</div>
              <div v-else class="cc-plan">
                <b>{{ block.title || t('cc.bridge.plan') }}</b>
                <div class="cc-turn__text">{{ block.text }}</div>
              </div>
            </div>
          </div>
        </template>
        <p v-if="exec.status === 'running'" class="cc-live">{{ t('cc.bridge.running') }}</p>
        <p v-else-if="loaded && !blocks.length" class="cc-xview__empty">{{ t('cc.bridge.noEvents') }}</p>
        <p v-if="exec.status === 'failed' && exec.detail" class="cc-err">{{ exec.detail }}</p>
      </div>
    </div>
    <footer class="cc-xview__dock">
      <span class="cc-xview__note">{{ t('cc.bridge.via') }}</span>
      <q-btn v-if="live" flat dense no-caps :label="t('cc.action.stop')" :loading="busy" @click="run(stop)" />
      <template v-else-if="exec.bucket === 'active'">
        <span v-if="reviewed" class="cc-xview__note">{{ t(`cc.bridge.review.${reviewed}`) }}</span>
        <q-btn
          v-else
          unelevated
          dense
          no-caps
          color="primary"
          :label="t('cc.bridge.approve')"
          :loading="busy"
          @click="run(approve)"
        />
        <q-btn flat dense no-caps :label="t('cc.action.archive')" :loading="busy" @click="run(archive)" />
      </template>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import CcGlyph from '../../../components/conversations/CcGlyph.vue';
import { cancelCodingTask, getCodingTask, listCodingTaskEvents, reviewCodingTask } from '../../coding/api';
import { groupBridgeEvents, type Execution } from '../model';
import type { CodingTaskEvent } from '../../../services/kratos/agentbridge/v1';
import { parseKratosApiError } from '../../../utils/kratosError';

const props = defineProps<{ exec: Execution }>();
const emit = defineEmits<{ changed: [] }>();
const { t } = useI18n();

const prompt = ref('');
const reviewed = ref('');
const events = ref<CodingTaskEvent[]>([]);
const loaded = ref(false);
const busy = ref(false);
const blocks = computed(() => groupBridgeEvents(events.value));
const live = computed(() => props.exec.status === 'running' || props.exec.status === 'waiting');
const toolNames = (items: { title: string }[]) =>
  [...new Set(items.map((i) => i.title).filter(Boolean))].slice(0, 3).join('、');

let timer: ReturnType<typeof setInterval> | null = null;
async function loadMore() {
  const after = events.value[events.value.length - 1]?.seq ?? 0;
  const more = await listCodingTaskEvents(props.exec.refId, after);
  if (more.length) events.value = [...events.value, ...more];
}
async function load(id: string) {
  events.value = [];
  loaded.value = false;
  try {
    const [task] = await Promise.all([getCodingTask(id), loadMore()]);
    prompt.value = task.prompt ?? '';
    reviewed.value = task.reviewStatus && task.reviewStatus !== 'archived' ? task.reviewStatus : '';
  } catch (err) {
    Notify.create({ type: 'negative', message: parseKratosApiError(err).message });
  } finally {
    loaded.value = true;
  }
}
watch(
  () => props.exec.refId,
  (id) => void load(id),
  { immediate: true },
);
// 在跑的时候接着读新事件
watch(
  live,
  (on) => {
    if (timer) clearInterval(timer);
    timer = on ? setInterval(() => void loadMore().catch(() => undefined), 3000) : null;
  },
  { immediate: true },
);
onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});

const stop = () => cancelCodingTask(props.exec.refId);
const approve = () => reviewCodingTask(props.exec.refId, 'approve');
const archive = () => reviewCodingTask(props.exec.refId, 'archive');
async function run(fn: () => Promise<unknown>) {
  busy.value = true;
  try {
    await fn();
    emit('changed');
    await load(props.exec.refId);
  } catch (err) {
    Notify.create({ type: 'negative', message: parseKratosApiError(err).message });
  } finally {
    busy.value = false;
  }
}
</script>
