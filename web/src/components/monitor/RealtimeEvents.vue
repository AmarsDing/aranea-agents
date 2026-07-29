<template>
  <div class="registry-table-card">
    <!-- ── Pulse：实时事件条（WS，不落库）── -->
    <div class="registry-table-card__header">
      <div>
        <div class="registry-table-card__title row items-center q-gutter-sm">
          <span>{{ t('monitorPage.events.panelTitle') }}</span>
          <q-badge outline color="primary">{{ t('monitorPage.events.count', { n: pulseEvents.length }) }}</q-badge>
          <q-badge :color="streamBadge.color" :label="streamBadge.label" outline />
        </div>
      </div>
      <div class="row items-center q-gutter-sm">
        <q-btn
          flat
          dense
          :icon="paused ? 'play_arrow' : 'pause'"
          :label="paused ? t('monitorPage.resume') : t('monitorPage.pause')"
          @click="emit('toggle-stream')"
        />
        <q-btn flat dense icon="delete_sweep" :label="t('monitorPage.events.clearPulse')" @click="emit('clear-pulse')" />
      </div>
    </div>

    <div class="pulse-strip q-px-md q-pb-md">
      <div v-if="pulseEvents.length === 0" class="text-caption text-grey pulse-strip__empty">
        {{ pulseEmptyHint }}
      </div>
      <div v-else class="pulse-strip__scroll row no-wrap items-center q-gutter-xs">
        <q-chip
          v-for="evt in pulseEvents"
          :key="evt.id"
          dense
          square
          clickable
          class="pulse-chip"
          @click="openDetail(evt)"
        >
          <q-icon name="circle" :color="severityColor(evt.severity)" size="8px" class="q-mr-xs" />
          <span class="pulse-chip__title">{{ evt.title }}</span>
          <q-tooltip max-width="420px">
            <div>{{ evt.time }}</div>
            <div v-if="evt.subtitle">{{ evt.subtitle }}</div>
          </q-tooltip>
        </q-chip>
      </div>
    </div>

    <q-separator />

    <!-- ── 历史：持久化事件（服务端分页 + 过滤）── -->
    <div class="registry-table-card__header">
      <div>
        <div class="registry-table-card__title row items-center q-gutter-sm">
          <span>{{ t('monitorPage.events.historyTitle') }}</span>
          <q-badge outline color="primary">{{ t('monitorPage.events.count', { n: historyTotal }) }}</q-badge>
        </div>
      </div>
      <div class="row items-center q-gutter-sm">
        <q-select
          :model-value="typeFilter"
          dense
          outlined
          emit-value
          map-options
          :options="typeOptions"
          :label="t('monitorPage.events.typeLabel')"
          style="min-width: 160px"
          @update:model-value="(v: string) => emit('update:typeFilter', v)"
        />
        <q-select
          :model-value="severityFilter"
          dense
          outlined
          emit-value
          map-options
          :options="severityOptions"
          :label="t('monitorPage.events.severityLabel')"
          style="min-width: 120px"
          @update:model-value="(v: string) => emit('update:severityFilter', v)"
        />
        <q-btn flat dense icon="refresh" :label="t('monitorPage.events.refresh')" @click="emit('refresh-history')" />
      </div>
    </div>

    <div class="app-registry-table-shell">
      <AppRegistryTable
        :shell="false"
        :rows="historyEvents"
        :columns="columns"
        row-key="id"
        flat
        dense
        :loading="historyLoading"
        :pagination="{ rowsPerPage: 0 }"
        hide-pagination
      >
      <template #body-cell-title="props">
        <q-td :props="props">
          <div class="row items-center no-wrap q-gutter-xs">
            <q-icon name="circle" :color="severityColor(props.row.severity)" size="8px">
              <q-tooltip>{{ t(`monitorPage.events.severity.${props.row.severity}`) }}</q-tooltip>
            </q-icon>
            <span>{{ props.row.title }}</span>
            <q-tooltip v-if="props.row.subtitle" max-width="420px">{{ props.row.subtitle }}</q-tooltip>
          </div>
        </q-td>
      </template>
      <template #body-cell-tags="props">
        <q-td :props="props">
          <q-chip dense square size="sm" :color="categoryChipColor(props.row.category)" text-color="white">
            {{ t(`monitorPage.events.category.${props.row.category}`) }}
            <q-tooltip>{{ props.row.type }}</q-tooltip>
          </q-chip>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <div class="row no-wrap q-gutter-xs">
            <q-btn
              v-if="props.row.canOpenInRuns"
              flat
              dense
              size="sm"
              icon="monitor_heart"
              :label="t('monitorPage.events.openInRuns')"
              @click="emit('open-in-runs', props.row)"
            />
            <q-btn
              v-if="props.row.completionSessionId || props.row.sessionId"
              flat
              dense
              size="sm"
              icon="chat"
              :label="t('monitorPage.events.openSession')"
              @click="emit('open-session', props.row)"
            />
            <q-btn
              flat
              dense
              size="sm"
              icon="visibility"
              :label="t('monitorPage.detail')"
              @click="openDetail(props.row)"
            />
          </div>
        </q-td>
      </template>
      </AppRegistryTable>

      <AppRegistryPagination
        :page="page"
        :page-size="pageSize"
        :page-max="pageMax"
        :total="historyTotal"
        :loading="historyLoading"
        :label="t('monitorPage.events.paginationLabel')"
        @update:page="(v: number) => emit('update:page', v)"
        @update:page-size="(v: number) => emit('update:pageSize', v)"
      />
    </div>

    <!-- ── 结构化详情 ── -->
    <q-dialog v-model="detailOpen">
      <q-card style="min-width: 480px; max-width: 720px">
        <q-card-section class="row items-center">
          <div class="text-h6">{{ t('monitorPage.events.dialogTitle') }}</div>
          <q-space />
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <q-card-section v-if="detailEvent" class="q-gutter-sm">
          <div class="row items-center q-gutter-sm">
            <q-chip dense square :color="severityColor(detailEvent.severity)" text-color="white">
              {{ t(`monitorPage.events.severity.${detailEvent.severity}`) }}
            </q-chip>
            <q-chip dense square :color="categoryChipColor(detailEvent.category)" text-color="white">
              {{ t(`monitorPage.events.category.${detailEvent.category}`) }}
            </q-chip>
            <span class="text-caption text-grey">{{ detailEvent.type }}</span>
          </div>
          <div class="text-subtitle2">{{ detailEvent.title }}</div>
          <div v-if="detailEvent.subtitle" class="text-body2">{{ detailEvent.subtitle }}</div>
          <q-list dense class="event-detail-list">
            <q-item v-if="detailEvent.actor">
              <q-item-section side class="event-detail-list__label">{{ t('monitorPage.events.detail.actor') }}</q-item-section>
              <q-item-section>{{ detailEvent.actor }}</q-item-section>
            </q-item>
            <q-item>
              <q-item-section side class="event-detail-list__label">{{ t('monitorPage.events.detail.time') }}</q-item-section>
              <q-item-section>{{ detailEvent.time }}</q-item-section>
            </q-item>
            <q-item v-if="detailEvent.completionSessionId || detailEvent.sessionId">
              <q-item-section side class="event-detail-list__label">{{ t('monitorPage.events.detail.session') }}</q-item-section>
              <q-item-section class="row items-center no-wrap q-gutter-xs">
                <span class="ellipsis">{{ detailEvent.completionSessionId || detailEvent.sessionId }}</span>
                <q-btn
                  flat
                  dense
                  size="sm"
                  icon="chat"
                  :label="t('monitorPage.events.openSession')"
                  @click="emit('open-session', detailEvent)"
                />
              </q-item-section>
            </q-item>
          </q-list>
          <q-expansion-item dense :label="t('monitorPage.events.detail.rawJson')" class="event-detail-raw">
            <pre class="event-json">{{ detailJson }}</pre>
          </q-expansion-item>
        </q-card-section>
        <q-separator />
        <q-card-actions align="right">
          <q-btn flat dense icon="content_copy" :label="t('monitorPage.events.detail.copyJson')" @click="copyDetailJson" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { copyToClipboard, Notify } from 'quasar';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import { createMonitorEventColumns } from './monitorTableUi';
import { severityColor, type MonitorViewEvent } from '../../features/monitor/eventView';
import { EVENT_TYPE_FILTERS } from '../../features/monitor/useMonitorRealtimeEvents';
import type { StreamState } from '../../features/monitor/types';

const props = defineProps<{
  pulseEvents: MonitorViewEvent[];
  streamState: StreamState;
  paused: boolean;
  historyEvents: MonitorViewEvent[];
  historyTotal: number;
  historyLoading: boolean;
  page: number;
  pageSize: number;
  typeFilter: string;
  severityFilter: string;
}>();

const emit = defineEmits<{
  'toggle-stream': [];
  'clear-pulse': [];
  'update:page': [value: number];
  'update:pageSize': [value: number];
  'update:typeFilter': [value: string];
  'update:severityFilter': [value: string];
  'refresh-history': [];
  'open-session': [event: MonitorViewEvent];
  'open-in-runs': [event: MonitorViewEvent];
}>();

const { t } = useI18n();
const columns = createMonitorEventColumns(t);

const pageMax = computed(() => Math.max(1, Math.ceil(props.historyTotal / props.pageSize)));

/** 类型筛选：选项值即 event_type 前缀（与落库 keyspace 对齐） */
const TYPE_OPTION_I18N: Record<string, string> = {
  all: 'monitorPage.events.typeOption.all',
  'runner.completion': 'monitorPage.events.typeOption.runnerCompletion',
  'alert.': 'monitorPage.events.typeOption.alert',
  'skill.filesystem.': 'monitorPage.events.typeOption.skillFilesystem',
  'usage.budget_alert': 'monitorPage.events.typeOption.usageBudget',
  'chat.user_feedback': 'monitorPage.events.typeOption.userFeedback',
};
const typeOptions = computed(() =>
  EVENT_TYPE_FILTERS.map((value) => ({ value, label: t(TYPE_OPTION_I18N[value] ?? value) })),
);

const severityOptions = computed(() =>
  ['all', 'critical', 'warn', 'success', 'info'].map((value) => ({
    value,
    label: t(`monitorPage.events.severity.${value}`),
  })),
);

const streamBadge = computed(() => {
  switch (props.streamState) {
    case 'live':
      return { color: 'positive', label: t('monitorPage.events.stream.live') };
    case 'connected':
      return { color: 'positive', label: t('monitorPage.events.stream.connected') };
    case 'connecting':
      return { color: 'info', label: t('monitorPage.events.stream.connecting') };
    case 'paused':
      return { color: 'grey', label: t('monitorPage.events.stream.paused') };
    default:
      return { color: 'negative', label: t('monitorPage.events.stream.error') };
  }
});

const pulseEmptyHint = computed(() => {
  if (props.streamState === 'paused') return t('monitorPage.events.stream.paused');
  if (props.streamState === 'connected' || props.streamState === 'live') {
    return t('monitorPage.events.pulseEmpty');
  }
  return t('monitorPage.events.stream.connecting');
});

// ── 详情弹窗 ──
const detailOpen = ref(false);
const detailEvent = ref<MonitorViewEvent | null>(null);
const detailJson = computed(() => (detailEvent.value ? JSON.stringify(detailEvent.value.raw, null, 2) : ''));

function openDetail(evt: MonitorViewEvent) {
  detailEvent.value = evt;
  detailOpen.value = true;
}

async function copyDetailJson() {
  await copyToClipboard(detailJson.value);
  Notify.create({ message: t('monitorPage.events.detail.copied'), color: 'positive', position: 'top' });
}

function categoryChipColor(category: string): string {
  switch (category) {
    case 'task':
      return 'primary';
    case 'message':
      return 'secondary';
    case 'agent':
      return 'accent';
    case 'tool':
      return 'teal';
    case 'system':
      return 'grey-7';
    default:
      return 'grey';
  }
}
</script>

<style scoped>
.pulse-strip__scroll {
  overflow-x: auto;
  padding-bottom: 4px;
}

.pulse-strip__empty {
  padding: 8px 0;
}

.pulse-chip {
  cursor: pointer;
}

.pulse-chip__title {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.event-json {
  max-height: 320px;
  overflow: auto;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}

.event-detail-list__label {
  min-width: 64px;
  color: var(--q-grey-7);
  font-size: 12px;
}
</style>
