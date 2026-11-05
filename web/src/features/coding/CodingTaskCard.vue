<template>
  <q-card flat bordered class="coding-task-card q-mb-sm">
    <q-card-section class="cursor-pointer q-py-sm" @click="$emit('toggle', task.id)">
      <div class="row items-center q-gutter-sm no-wrap">
        <q-avatar size="32px" color="primary" text-color="white" class="text-weight-bold">
          <img v-if="agent?.iconUrl" :src="agent.iconUrl" :alt="agent.displayName || agent.agentKey" />
          <template v-else>{{ agentInitial(agent) }}</template>
        </q-avatar>
        <div class="col ellipsis">
          <div class="row items-center q-gutter-sm">
            <span class="text-weight-medium ellipsis">{{ title }}</span>
            <q-badge :color="statusColor(task.status)" text-color="white" :label="statusLabel(task.status)" />
            <q-badge v-if="silentBadge" color="orange-2" text-color="orange-10" :label="silentBadge" />
          </div>
          <div class="text-caption text-grey-7 ellipsis">
            {{ agent?.displayName || task.agentKey }} · {{ task.projectName }} · 开始于 {{ formatDateTime(task.createdAt) }} · 已耗时 {{ formatDuration(task, now) }}
          </div>
          <div v-if="task.lastExcerpt" class="text-caption text-grey-6 ellipsis">
            最近一步：{{ task.lastExcerpt }}
          </div>
        </div>
        <q-icon :name="expanded ? 'expand_less' : 'expand_more'" color="grey-6" />
      </div>
    </q-card-section>

    <template v-if="expanded">
      <q-separator />
      <q-card-section class="q-py-sm">
        <div class="row items-center q-gutter-sm q-mb-sm">
          <q-icon name="bolt" color="primary" size="sm" />
          <div class="col ellipsis text-body2">
            {{ currentAction || '等待工具输出…' }}
          </div>
          <span class="text-caption text-grey-7">已运行 {{ formatDuration(task, now) }}</span>
          <q-btn
            v-if="isActiveStatus(task.status)"
            flat
            dense
            color="negative"
            icon="stop"
            label="中止"
            :loading="cancelling"
            @click.stop="confirmCancel"
          />
        </div>

        <div v-if="events.length === 0" class="text-caption text-grey-6 q-pa-sm">暂无步骤事件。</div>
        <q-list v-else dense separator class="coding-task-events">
          <q-item v-for="ev in events" :key="ev.id || ev.seq" dense>
            <q-item-section side top>
              <q-badge :color="eventKindColor(ev.kind)" text-color="white" :label="eventKindLabel(ev.kind)" />
            </q-item-section>
            <q-item-section>
              <q-item-label v-if="ev.title" class="text-weight-medium">{{ ev.title }}</q-item-label>
              <q-item-label v-if="ev.text" caption class="coding-task-event-text">{{ ev.text }}</q-item-label>
            </q-item-section>
            <q-item-section side top>
              <span class="text-caption text-grey-6">#{{ ev.seq }} · {{ formatDateTime(ev.createdAt) }}</span>
            </q-item-section>
          </q-item>
        </q-list>

        <q-expansion-item
          dense
          dense-toggle
          expand-separator
          icon="article"
          label="原始日志"
          class="q-mt-sm"
          header-class="text-grey-7"
        >
          <pre class="coding-task-rawlog q-pa-sm">{{ rawLog }}</pre>
        </q-expansion-item>

        <template v-if="isTerminalStatus(task.status)">
          <q-banner v-if="task.summary" rounded dense class="bg-green-1 text-grey-9 q-mt-sm">
            <template #avatar><q-icon name="summarize" color="positive" /></template>
            {{ task.summary }}
          </q-banner>
          <q-banner v-if="task.error" rounded dense class="bg-red-1 text-negative q-mt-sm">
            <template #avatar><q-icon name="error" color="negative" /></template>
            {{ task.error }}
          </q-banner>

          <div v-if="!task.reviewStatus" class="row q-gutter-sm q-mt-sm">
            <q-btn outline dense color="positive" icon="check" label="通过" :loading="reviewing === 'approve'" @click.stop="doReview('approve')" />
            <q-btn outline dense color="warning" icon="undo" label="打回" :loading="reviewing === 'reject'" @click.stop="rejectOpen = true" />
            <q-btn flat dense color="grey-7" icon="archive" label="仅存档" :loading="reviewing === 'archive'" @click.stop="doReview('archive')" />
          </div>
          <div v-else class="row items-center q-gutter-sm q-mt-sm">
            <q-badge
              :color="task.reviewStatus === 'approved' ? 'positive' : task.reviewStatus === 'rejected' ? 'warning' : 'grey-6'"
              text-color="white"
              :label="reviewStatusLabel"
            />
            <span v-if="task.reviewComment" class="text-caption text-grey-7">{{ task.reviewComment }}</span>
            <q-btn
              v-if="task.reviewStatus === 'rejected'"
              flat
              dense
              color="primary"
              icon="replay"
              label="按意见重做"
              @click.stop="$emit('redo', task)"
            />
          </div>
        </template>
      </q-card-section>
    </template>

    <q-dialog v-model="rejectOpen" persistent>
      <q-card class="app-dialog-card app-glass-dialog" style="min-width: 420px">
        <q-card-section class="row items-start justify-between">
          <div class="text-h6">打回任务</div>
          <q-btn flat dense round icon="close" aria-label="关闭" @click="rejectOpen = false" />
        </q-card-section>
        <q-separator />
        <q-card-section>
          <q-input
            v-model="rejectComment"
            dense
            outlined
            type="textarea"
            autogrow
            label="打回意见 *"
            hint="将记录到任务验收结果；可选择按意见重做，把原任务描述与意见预填回发起条。"
          />
        </q-card-section>
        <q-card-actions align="right" class="q-pa-md">
          <q-btn flat rounded label="取消" @click="rejectOpen = false" />
          <q-btn color="warning" rounded unelevated label="确认打回" :disable="!rejectComment.trim()" :loading="reviewing === 'reject'" @click="doReview('reject')" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import {
  agentInitial,
  cancelCodingTask,
  eventKindColor,
  eventKindLabel,
  formatDateTime,
  formatDuration,
  isActiveStatus,
  isTerminalStatus,
  listCodingTaskEvents,
  reviewCodingTask,
  silentMinutes,
  statusColor,
  statusLabel,
} from './api';
import type { CodingAgent, CodingTask, CodingTaskEvent } from './api';

const props = defineProps<{
  task: CodingTask;
  agent?: CodingAgent | undefined;
  expanded: boolean;
  now: number;
}>();

const emit = defineEmits<{
  (e: 'toggle', id: string | undefined): void;
  (e: 'updated'): void;
  (e: 'redo', task: CodingTask): void;
}>();

const $q = useQuasar();

const events = ref<CodingTaskEvent[]>([]);
const cancelling = ref(false);
const reviewing = ref('');
const rejectOpen = ref(false);
const rejectComment = ref('');

let pollTimer: ReturnType<typeof setInterval> | null = null;

const title = computed(() => {
  const firstLine = (props.task.prompt ?? '').split('\n')[0] ?? '';
  return firstLine.length > 60 ? `${firstLine.slice(0, 60)}…` : firstLine || '（无描述）';
});

const silentBadge = computed(() => {
  if (props.task.status !== 'running') return '';
  const mins = silentMinutes(props.task, props.now);
  return mins >= 2 ? `已 ${mins} 分钟无输出` : '';
});

const currentAction = computed(() => {
  const last = events.value[events.value.length - 1];
  if (!last) return '';
  return [last.title, last.text].filter(Boolean).join('：');
});

const rawLog = computed(() =>
  events.value.map((ev) => `#${ev.seq} [${ev.kind}] ${ev.title ?? ''} ${ev.text ?? ''}`.trim()).join('\n'),
);

const reviewStatusLabel = computed(() => {
  switch (props.task.reviewStatus) {
    case 'approved':
      return '已通过';
    case 'rejected':
      return '已打回';
    case 'archived':
      return '已存档';
    default:
      return props.task.reviewStatus || '';
  }
});

async function fetchEvents() {
  try {
    const lastSeq = events.value.length ? (events.value[events.value.length - 1]?.seq ?? 0) : 0;
    const items = await listCodingTaskEvents(props.task.id ?? '', lastSeq);
    if (items.length > 0) {
      events.value = [...events.value, ...items].sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0));
    }
  } catch {
    // 轮询失败静默，下一轮重试。
  }
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

function startPolling() {
  stopPolling();
  events.value = [];
  void fetchEvents();
  if (isTerminalStatus(props.task.status)) return;
  pollTimer = setInterval(() => {
    if (document.hidden) return;
    void fetchEvents();
    if (isTerminalStatus(props.task.status)) stopPolling();
  }, 2000);
}

watch(
  () => [props.expanded, props.task.status] as const,
  ([expanded]) => {
    if (expanded) startPolling();
    else stopPolling();
  },
  { immediate: true },
);

onBeforeUnmount(stopPolling);

function confirmCancel() {
  $q.dialog({
    title: '确认中止该任务？',
    message: '中止后工具进程将被终止，已产生的修改不会自动回滚。',
    cancel: true,
    persistent: true,
  }).onOk(async () => {
    cancelling.value = true;
    try {
      await cancelCodingTask(props.task.id ?? '');
      $q.notify({ type: 'positive', message: '已请求中止' });
      emit('updated');
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '中止失败' });
    } finally {
      cancelling.value = false;
    }
  });
}

async function doReview(action: string) {
  if (action === 'reject' && !rejectComment.value.trim()) return;
  reviewing.value = action;
  try {
    await reviewCodingTask(props.task.id ?? '', action, action === 'reject' ? rejectComment.value.trim() : '');
    $q.notify({ type: 'positive', message: action === 'approve' ? '已通过验收' : action === 'reject' ? '已打回' : '已存档' });
    if (action === 'reject') {
      rejectOpen.value = false;
      // 打回成功后提供「按意见重做」入口（由页面预填发起条）。
      emit('redo', { ...props.task, reviewComment: rejectComment.value.trim() });
      rejectComment.value = '';
    }
    emit('updated');
  } catch (err) {
    $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '操作失败' });
  } finally {
    reviewing.value = '';
  }
}
</script>

<style scoped>
.coding-task-event-text {
  white-space: pre-wrap;
  word-break: break-word;
}

.coding-task-rawlog {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
