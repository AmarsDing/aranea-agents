<template>
  <div v-if="sessionId || agentId" class="chat-composer-toolbar-slot">
    <q-btn
      dense
      unelevated
      outline
      color="primary"
      class="chat-toolbar-btn chat-toolbar-btn--outline"
      :aria-label="panelLabel"
      @click="openDialog"
    >
      <q-icon name="work_history" size="20px" />
      <q-badge v-if="badgeCount" color="negative" floating transparent>{{ badgeCount }}</q-badge>
      <q-tooltip>{{ panelLabel }}</q-tooltip>
    </q-btn>

    <q-dialog v-model="dialogOpen" transition-show="scale" transition-hide="scale">
      <q-card
        class="app-dialog-card app-dialog-card--md app-glass-dialog chat-jobs-dialog"
        :class="{ 'is-dark': isDark }"
      >
        <q-card-section class="row items-center q-pb-none">
          <div class="text-h6 col">{{ t('chat.job.title', '后台任务') }}</div>
          <q-btn v-close-popup flat round dense icon="close" :aria-label="t('chat.cancel')" />
        </q-card-section>
        <q-card-section class="app-dialog-body q-pt-sm">
          <q-tabs v-model="tab" dense align="left" class="q-mb-sm" active-color="primary">
            <q-tab name="jobs" :label="jobsTabLabel" />
            <q-tab name="deadLetters" :label="deadLettersTabLabel" />
          </q-tabs>

          <q-tab-panels v-model="tab" animated>
            <q-tab-panel name="jobs" class="q-pa-none">
              <div v-if="jobsLoading" class="row items-center q-gutter-sm q-pa-sm">
                <q-spinner size="20px" color="primary" />
                <span class="text-caption">{{ t('chat.job.loading', '加载后台任务…') }}</span>
              </div>
              <q-banner v-else-if="jobsError" dense rounded class="app-banner-warning q-mb-sm">{{
                jobsError
              }}</q-banner>
              <div v-else-if="!jobRows.length" class="text-caption text-grey q-pa-sm">
                {{ t('chat.job.empty', '暂无后台任务') }}
              </div>
              <q-list v-else dense separator class="chat-jobs-panel__list">
                <q-item v-for="job in jobRows" :key="job.id">
                  <q-item-section avatar>
                    <q-badge :color="statusColor(job.status)" :label="job.status" />
                  </q-item-section>
                  <q-item-section>
                    <q-item-label class="ellipsis">{{ job.summary || job.id }}</q-item-label>
                    <q-item-label caption>
                      {{ job.source === 'session_run' ? jobPhaseLabel(job) || job.status : job.target_type || 'sync' }}
                      <span v-if="job.target_id && job.source !== 'session_run'"> · {{ job.target_id }}</span>
                      <span v-if="job.turn_id"> · turn {{ job.turn_id.slice(0, 8) }}</span>
                      <span v-if="jobElapsed(job)"> · {{ jobElapsed(job) }}</span>
                    </q-item-label>
                  </q-item-section>
                  <q-item-section side>
                    <div class="row items-center q-gutter-xs">
                      <q-btn
                        v-if="isJobActive(job)"
                        flat
                        dense
                        round
                        icon="cancel"
                        color="negative"
                        :aria-label="t('chat.job.cancel', '取消任务')"
                        @click.stop="cancelJob(job)"
                      >
                        <q-tooltip>{{ t('chat.job.cancel', '取消任务') }}</q-tooltip>
                      </q-btn>
                      <q-btn
                        v-if="turnBlockLink(job)"
                        flat
                        dense
                        round
                        icon="forum"
                        :aria-label="t('chat.job.openTurn', '跳转到对话轮次')"
                        @click.stop="openTurnBlock(job)"
                      />
                      <q-btn
                        v-if="graphRunLink(job)"
                        flat
                        dense
                        round
                        icon="open_in_new"
                        :aria-label="t('chat.job.openGraph')"
                        @click.stop="openGraphRun(job)"
                      />
                      <q-item-label caption class="text-grey">{{ formatTime(job.updated_at) }}</q-item-label>
                    </div>
                  </q-item-section>
                </q-item>
              </q-list>
            </q-tab-panel>

            <q-tab-panel name="deadLetters" class="q-pa-none">
              <div v-if="dlLoading" class="row items-center q-gutter-sm q-pa-sm">
                <q-spinner size="20px" color="primary" />
                <span class="text-caption">{{ t('chat.deadLetter.loading', '加载死信…') }}</span>
              </div>
              <q-banner v-else-if="dlError" dense rounded class="app-banner-warning q-mb-sm">{{ dlError }}</q-banner>
              <div v-else-if="!deadLetterRows.length" class="text-caption text-grey q-pa-sm">
                {{ t('chat.deadLetter.empty', '暂无待处理死信') }}
              </div>
              <q-list v-else dense separator class="chat-jobs-panel__list">
                <q-expansion-item
                  v-for="row in deadLetterRows"
                  :key="row.id"
                  dense
                  expand-separator
                  class="chat-jobs-panel__dl-item"
                >
                  <template #header>
                    <q-item-section avatar>
                      <q-badge :color="deadLetterStatusColor(row.status)" :label="row.status" />
                    </q-item-section>
                    <q-item-section>
                      <q-item-label class="ellipsis">{{ row.error_message || row.source_type || row.id }}</q-item-label>
                      <q-item-label caption>
                        {{ row.source_type }}
                        <span v-if="row.team_run_id"> · {{ row.team_run_id }}</span>
                      </q-item-label>
                    </q-item-section>
                    <q-item-section side>
                      <div class="row items-center q-gutter-xs">
                        <q-btn
                          v-if="teamRunObservatoryLink(row)"
                          flat
                          dense
                          round
                          icon="insights"
                          :aria-label="t('chat.deadLetter.openObservatory', '打开编排观测')"
                          @click.stop="openTeamRunObservatory(row)"
                        />
                        <q-btn
                          v-if="row.status === 'pending'"
                          flat
                          dense
                          no-caps
                          size="sm"
                          color="primary"
                          :label="t('chat.deadLetter.resolve', '标记已处理')"
                          @click.stop="emit('resolve-dead-letter', row.id)"
                        />
                      </div>
                    </q-item-section>
                  </template>
                  <q-card flat class="q-pa-sm bg-transparent">
                    <div class="text-caption text-grey q-mb-xs">{{ formatTime(row.created_at) }}</div>
                    <pre v-if="payloadPreview(row)" class="chat-jobs-panel__payload">{{ payloadPreview(row) }}</pre>
                  </q-card>
                </q-expansion-item>
              </q-list>
            </q-tab-panel>
          </q-tab-panels>

          <div class="row justify-end q-pt-xs">
            <q-btn
              flat
              dense
              no-caps
              size="sm"
              icon="refresh"
              :label="t('chat.job.refresh', '刷新')"
              @click="refreshActiveTab"
            />
          </div>
        </q-card-section>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, toRef, watch, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useChatBackgroundJobs } from '../../features/chat/useChatBackgroundJobs';
import { useTaskDeadLetters } from '../../features/chat/useTaskDeadLetters';
import { formatElapsed, phaseLabel } from '../../features/chat/jobFormatters';
import { ACTIVE_RUN_STATUSES } from '../../features/chat/sessionRunStatus';

type ChatBackgroundJobRow = ReturnType<typeof useChatBackgroundJobs>['rows']['value'][number];
type TaskDeadLetterRow = ReturnType<typeof useTaskDeadLetters>['rows']['value'][number];

const emit = defineEmits<{
  'focus-turn': [turnId: string];
  navigate: [route: { name: string; params: Record<string, string> }];
  'cancel-job': [job: { id: string; source: string }];
  'resolve-dead-letter': [id: string];
}>();

const props = defineProps<{
  sessionId?: string;
  agentId?: string;
  refreshNonce?: number;
  isDark?: boolean;
}>();

const refreshRef = toRef(props, 'refreshNonce') as Ref<number | undefined>;

const { t } = useI18n();
const dialogOpen = ref(false);
const tab = ref<'jobs' | 'deadLetters'>('jobs');

const {
  loading: jobsLoading,
  error: jobsError,
  rows: jobRows,
  load: loadJobs,
  runningCount,
} = useChatBackgroundJobs(toRef(props, 'sessionId'), toRef(props, 'agentId'), refreshRef);

const {
  loading: dlLoading,
  error: dlError,
  rows: deadLetterRows,
  load: loadDeadLetters,
  resolve: resolveDeadLetter,
  pendingCount,
} = useTaskDeadLetters(toRef(props, 'sessionId'), refreshRef);

const badgeCount = computed(() => runningCount() + pendingCount());

const panelLabel = computed(() => {
  const base = t('chat.job.title', '后台任务');
  const n = badgeCount.value;
  return n > 0 ? `${base} (${n})` : base;
});

const jobsTabLabel = computed(() => {
  const n = runningCount();
  const base = t('chat.job.tab', '任务');
  return n > 0 ? `${base} (${n})` : base;
});

const deadLettersTabLabel = computed(() => {
  const n = pendingCount();
  const base = t('chat.deadLetter.tab', '死信');
  return n > 0 ? `${base} (${n})` : base;
});

watch(dialogOpen, (open) => {
  if (open) refreshActiveTab();
});

function openDialog() {
  dialogOpen.value = true;
}

function refreshActiveTab() {
  if (tab.value === 'deadLetters') {
    void loadDeadLetters();
  } else {
    void loadJobs();
  }
}

function isJobActive(job: ChatBackgroundJobRow) {
  return ACTIVE_RUN_STATUSES.has(job.status);
}

function jobElapsed(job: ChatBackgroundJobRow): string {
  if (!isJobActive(job)) return '';
  return formatElapsed(job.created_at, job.updated_at);
}

function jobPhaseLabel(job: ChatBackgroundJobRow): string {
  if (!job.phase) return '';
  return phaseLabel(job.phase);
}

const $q = useQuasar();

async function cancelJob(job: ChatBackgroundJobRow) {
  $q.dialog({
    title: t('chat.job.cancelConfirm', '取消任务'),
    message: t('chat.job.cancelConfirmMsg', '确定要取消此任务吗？此操作不可撤销。'),
    cancel: true,
    persistent: false,
  }).onOk(() => {
    emit('cancel-job', { id: job.id, source: job.source });
  });
}

function statusColor(status: string) {
  switch (status) {
    case 'running':
    case 'accepted':
    case 'interactive':
    case 'escalating':
    case 'durable':
      return 'info';
    case 'completed':
      return 'positive';
    case 'failed':
    case 'timeout':
      return 'negative';
    case 'async_queued':
    case 'queued':
      return 'purple';
    case 'cancelled':
      return 'warning';
    default:
      return 'grey';
  }
}

function deadLetterStatusColor(status: string) {
  switch (status) {
    case 'pending':
      return 'negative';
    case 'resolved':
      return 'positive';
    default:
      return 'grey';
  }
}

function formatTime(iso: string) {
  if (!iso) return '';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function turnBlockLink(job: ChatBackgroundJobRow) {
  const turnId = job.turn_id?.trim();
  if (!turnId) return null;
  if (job.source !== 'session_run') return null;
  return turnId;
}

function openTurnBlock(job: ChatBackgroundJobRow) {
  const turnId = turnBlockLink(job);
  if (turnId) {
    dialogOpen.value = false;
    emit('focus-turn', turnId);
  }
}

function graphRunLink(job: ChatBackgroundJobRow) {
  const graphId = job.graph_id?.trim();
  const execId = job.target_id?.trim();
  if (!graphId || !execId) return null;
  const tt = (job.target_type ?? '').toLowerCase();
  if (!tt.includes('graph')) return null;
  return { name: 'graph-run', params: { id: graphId, execId } };
}

function openGraphRun(job: ChatBackgroundJobRow) {
  const link = graphRunLink(job);
  if (link) emit('navigate', link);
}

function teamRunObservatoryLink(row: TaskDeadLetterRow) {
  const teamId = row.team_id?.trim();
  const runId = row.team_run_id?.trim();
  if (!teamId || !runId) return null;
  return { name: 'team-run-observatory', params: { teamId, runId } };
}

function openTeamRunObservatory(row: TaskDeadLetterRow) {
  const link = teamRunObservatoryLink(row);
  if (link) emit('navigate', link);
}

function payloadPreview(row: TaskDeadLetterRow) {
  const raw = row.payload_json?.trim();
  if (!raw || raw === '{}') return '';
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}
</script>

<style scoped>
.chat-jobs-panel__list {
  max-height: min(52vh, 360px);
  overflow-y: auto;
}

.chat-jobs-panel__dl-item {
  border-radius: 8px;
}

.chat-jobs-panel__payload {
  margin: 0;
  max-height: 120px;
  overflow: auto;
  font-size: var(--text-xs);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
