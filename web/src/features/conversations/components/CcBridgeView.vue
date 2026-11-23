<template>
  <!--
    编码工具桥（ACP / 运行台里的命令行工具）的一次执行：你的提示 → 工具的回答、工具调用（收起）、计划、错误。
    它要你确认时，确认卡就在对话里（照 Claude Code 的做法：说清它要做什么，1 允许 / 2 这个任务里都允许 / 3 拒绝）。
    在跑的时候每 3 秒接着读新事件。验收、归档在列表行和标题栏的状态灯上做，这里不再有底栏。
  -->
  <div class="cc-xview" @keydown="onKey">
    <div ref="bodyEl" class="cc-xview__body">
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
          <p v-if="block.kind === 'note'" class="cc-note">{{ block.text }}</p>
          <!-- 之前的确认请求（已经回答过的）：收成一行 -->
          <p v-else-if="block.kind === 'ask' && i !== pendingAsk" class="cc-note">
            <q-icon name="how_to_reg" size="14px" aria-hidden="true" />{{
              t('cc.bridge.asked', { what: askLine(block.text) })
            }}
          </p>
          <div v-else-if="block.kind !== 'ask'" class="cc-turn">
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
              <div v-else-if="block.kind === 'err'" class="cc-err">{{ block.title }} {{ block.text }}</div>
              <div v-else-if="block.kind === 'plan'" class="cc-plan">
                <b>{{ block.title || t('cc.bridge.plan') }}</b>
                <div class="cc-turn__text">{{ block.text }}</div>
              </div>
            </div>
          </div>
        </template>

        <!-- 等你确认：就在对话里回答 -->
        <section
          v-if="asking"
          ref="askEl"
          class="cc-confirm"
          role="group"
          tabindex="-1"
          :aria-label="t('cc.bridge.askLabel', { who: exec.executorName })"
        >
          <div class="cc-confirm__h">
            <CcGlyph :executor="exec.executor" :name="exec.executorName" size="xs" />
            <b>{{ t('cc.bridge.askLabel', { who: exec.executorName }) }}</b>
          </div>
          <pre class="cc-confirm__what">{{ askBody }}</pre>
          <div class="cc-confirm__opts">
            <button
              v-for="(o, k) in OPTIONS"
              :key="o"
              type="button"
              class="cc-confirm__opt"
              :class="{ 'cc-confirm__opt--deny': o === 'deny' }"
              :disabled="busy"
              @click="decide(o)"
            >
              <kbd>{{ k + 1 }}</kbd
              >{{ t(`cc.bridge.decide.${o}`) }}
            </button>
          </div>
          <p class="cc-confirm__hint">{{ t('cc.bridge.decideHint') }}</p>
        </section>

        <p v-if="live && !asking" class="cc-live">
          <span>{{ excerpt || t('cc.bridge.running') }}</span>
          <button type="button" class="cc-live__stop" :disabled="busy" @click="run(stop)">
            {{ t('cc.action.stop') }}
          </button>
        </p>
        <p v-else-if="loaded && !blocks.length && !live" class="cc-xview__empty">{{ t('cc.bridge.noEvents') }}</p>
        <p v-if="exec.status === 'failed' && exec.detail" class="cc-err">{{ exec.detail }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import CcGlyph from '../../../components/conversations/CcGlyph.vue';
import {
  cancelCodingTask,
  confirmCodingPermission,
  getCodingTask,
  listCodingTaskEvents,
  type PermissionDecision,
} from '../../coding/api';
import { groupBridgeEvents, type Execution } from '../model';
import type { CodingTaskEvent } from '../../../services/kratos/agentbridge/v1';
import { parseKratosApiError } from '../../../utils/kratosError';

const OPTIONS: PermissionDecision[] = ['approve', 'always', 'deny'];

const props = defineProps<{ exec: Execution }>();
const emit = defineEmits<{ changed: [] }>();
const { t } = useI18n();

const prompt = ref('');
const excerpt = ref('');
const taskStatus = ref('');
const events = ref<CodingTaskEvent[]>([]);
const loaded = ref(false);
const busy = ref(false);
const askEl = ref<HTMLElement | null>(null);
const bodyEl = ref<HTMLElement | null>(null);
const blocks = computed(() => groupBridgeEvents(events.value));
const live = computed(() => props.exec.status === 'running' || props.exec.status === 'waiting');
const toolNames = (items: { title: string }[]) =>
  [...new Set(items.map((i) => i.title).filter(Boolean))].slice(0, 3).join('、');

// 还挂着的确认请求：任务在等批准时，最后一个确认块
const waiting = computed(() => taskStatus.value === 'awaiting_approval' || props.exec.status === 'waiting');
const pendingAsk = computed(() => {
  if (!waiting.value) return -1;
  for (let i = blocks.value.length - 1; i >= 0; i--) if (blocks.value[i]!.kind === 'ask') return i;
  return -1;
});
const asking = computed(() => pendingAsk.value >= 0);
// 确认请求的原文第一行是「××请求确认：」，正文是终端里那个确认框
const askBody = computed(() => {
  const b = blocks.value[pendingAsk.value];
  const text = b && b.kind === 'ask' ? b.text : '';
  return text.split('\n').slice(1).join('\n').trim() || text;
});
const askLine = (text: string) =>
  text
    .split('\n')
    .slice(1)
    .map((l) => l.trim())
    .find(Boolean) ?? '';

let timer: ReturnType<typeof setInterval> | null = null;
async function loadMore() {
  const after = events.value[events.value.length - 1]?.seq ?? 0;
  const [more, task] = await Promise.all([
    listCodingTaskEvents(props.exec.refId, after),
    getCodingTask(props.exec.refId),
  ]);
  if (more.length) events.value = [...events.value, ...more];
  excerpt.value = (task.lastExcerpt ?? '').split('\n').filter(Boolean).pop() ?? '';
  if (task.status !== taskStatus.value) {
    taskStatus.value = task.status ?? '';
    emit('changed');
  }
}
async function load(id: string) {
  events.value = [];
  loaded.value = false;
  try {
    const [task] = await Promise.all([getCodingTask(id), loadMore()]);
    prompt.value = task.prompt ?? '';
    taskStatus.value = task.status ?? '';
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
// 出现确认请求时滚到它、把焦点给它（键盘 1 / 2 / 3 直接回答）
watch(asking, async (on) => {
  if (!on) return;
  await nextTick();
  askEl.value?.scrollIntoView({ block: 'nearest' });
  askEl.value?.focus({ preventScroll: true });
});
onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});

const stop = () => cancelCodingTask(props.exec.refId);
async function decide(d: PermissionDecision) {
  await run(() => confirmCodingPermission(props.exec.refId, d));
}
function onKey(e: KeyboardEvent) {
  if (!asking.value || busy.value || e.ctrlKey || e.metaKey || e.altKey) return;
  const pick = e.key === 'Escape' ? 'deny' : OPTIONS[Number(e.key) - 1];
  if (!pick) return;
  e.preventDefault();
  void decide(pick);
}
async function run(fn: () => Promise<unknown>) {
  busy.value = true;
  try {
    await fn();
    await loadMore();
    emit('changed');
  } catch (err) {
    Notify.create({ type: 'negative', message: parseKratosApiError(err).message });
    // 已经被别处回答过（审批中心、另一个窗口）：重新读一次
    await loadMore().catch(() => undefined);
  } finally {
    busy.value = false;
  }
}
</script>
