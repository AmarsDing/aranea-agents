<!--
  定时任务页右侧详情：一个定时任务或一条运行台自动化。纯展示（props / emits），不调接口。
-->
<template>
  <aside class="cron-detail" :aria-label="item.name">
    <header class="cron-detail__head">
      <div class="cron-detail__title">
        <h2>{{ item.name }}</h2>
        <span class="cron-detail__sub mono">{{ item.kind === 'fleet' ? `运行台 · ${item.sub}` : item.sub }}</span>
      </div>
      <span class="cron-state" :class="`is-${item.state}`">{{ STATE_LABEL[item.state] }}</span>
      <q-btn flat dense round size="sm" icon="close" aria-label="关闭详情" @click="emit('close')" />
    </header>

    <div class="cron-detail__actions">
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="play_arrow"
        :label="item.kind === 'fleet' ? '立即运行' : '立即执行'"
        :loading="busy"
        @click="emit('run')"
      />
      <q-btn v-if="item.kind === 'cron'" flat dense no-caps icon="edit" label="编辑" @click="emit('edit')" />
      <q-btn v-else flat dense no-caps icon="open_in_new" label="在运行台编辑" @click="emit('open-fleet', '')" />
      <q-btn
        v-if="item.state === 'dead'"
        flat
        dense
        no-caps
        icon="restart_alt"
        color="warning"
        label="清零并恢复"
        :disable="busy"
        @click="emit('reset')"
      />
      <q-space />
      <q-toggle
        :model-value="item.enabled"
        dense
        :disable="busy || item.state === 'dead'"
        :label="item.enabled ? '启用中' : '已停用'"
        @update:model-value="(v: boolean) => emit('toggle', v)"
      />
      <q-btn
        v-if="item.kind === 'cron'"
        flat
        dense
        round
        size="sm"
        icon="delete"
        color="negative"
        aria-label="删除"
        @click="emit('delete')"
      />
    </div>

    <p v-if="item.state === 'dead'" class="cron-detail__alert" role="alert">
      连续失败 {{ item.failureCount }} 次，已自动停用。修好原因后点「清零并恢复」。
    </p>

    <dl class="cron-detail__facts">
      <dt>计划</dt>
      <dd>
        {{ item.schedule }}
        <span v-if="rawSchedule" class="cron-detail__muted mono">{{ rawSchedule }}</span>
      </dd>
      <dt>交给</dt>
      <dd>{{ item.target }}</dd>
      <dt>下次</dt>
      <dd class="mono">
        {{ item.nextRunAt ? formatCronShort(item.nextRunAt) : item.state === 'active' ? '计算中…' : '—' }}
      </dd>
      <dt>上次</dt>
      <dd>
        <span class="mono">{{ formatCronShort(item.lastRunAt) }}</span>
        <span v-if="item.lastOutcome" class="cron-outcome" :class="`is-${item.lastOutcome}`">
          {{ OUTCOME_LABEL[item.lastOutcome] }}
        </span>
      </dd>
      <template v-if="item.lastError">
        <dt>上次原因</dt>
        <dd class="cron-detail__error">{{ item.lastError }}</dd>
      </template>
      <dt>{{ item.kind === 'fleet' ? '提示' : '消息' }}</dt>
      <dd class="cron-detail__message">{{ message || '—' }}</dd>
    </dl>

    <template v-if="item.kind === 'cron' && meta">
      <div class="cron-detail__stats" aria-label="累计执行">
        <div>
          <b class="mono">{{ meta.run_count || 0 }}</b
          ><span>累计</span>
        </div>
        <div>
          <b class="mono is-ok">{{ meta.success_count || 0 }}</b
          ><span>成功</span>
        </div>
        <div>
          <b class="mono" :class="{ 'is-fail': (meta.failure_count || 0) > 0 }">{{ meta.failure_count || 0 }}</b
          ><span>连续失败</span>
        </div>
        <div>
          <b class="mono">{{ Number(meta.skip_count || 0) }}</b
          ><span>跳过</span>
        </div>
      </div>

      <section class="cron-detail__runs">
        <header>
          <h3>最近执行</h3>
          <q-space />
          <q-btn
            flat
            dense
            round
            size="sm"
            icon="refresh"
            aria-label="刷新最近执行"
            :loading="runsLoading"
            @click="emit('reload-runs')"
          />
          <q-btn flat dense no-caps size="sm" label="全部记录" @click="emit('history')" />
        </header>
        <ol v-if="runs.length" class="cron-runs">
          <li v-for="r in runs" :key="r.id" class="cron-run">
            <span class="cron-outcome" :class="`is-${runOutcome(r.status)}`">{{
              RUN_LABEL[r.status] ?? r.status
            }}</span>
            <span class="mono">{{ formatCronShort(r.started_at || r.created_at) }}</span>
            <span class="cron-detail__muted">{{ r.trigger === 'manual' ? '手动' : '定时' }}</span>
            <span class="cron-detail__muted mono">{{ duration(r) }}</span>
            <router-link
              v-if="r.run_id"
              class="cron-run__link"
              :to="{ name: 'session-detail', params: { sessionId: r.run_id } }"
              >会话</router-link
            >
            <span v-if="r.resolved_at" class="cron-run__tag" title="在工作台告警里忽略过">已忽略</span>
            <div v-if="r.error_message" class="cron-run__err" :title="r.error_message">{{ r.error_message }}</div>
          </li>
        </ol>
        <p v-else class="cron-detail__muted">{{ runsLoading ? '加载中…' : '还没有执行过。' }}</p>
      </section>
    </template>

    <section v-else-if="item.kind === 'fleet'" class="cron-detail__runs">
      <p class="cron-detail__muted">
        自动化到点会在运行台里起一个 agent 终端；agent 干得怎么样，在运行台对应的 worktree 里看。
      </p>
      <q-btn
        v-if="automation?.lastWorkspaceId"
        flat
        dense
        no-caps
        icon="terminal"
        label="打开上次运行的 worktree"
        @click="emit('open-fleet', automation.lastWorkspaceId)"
      />
    </section>
  </aside>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { CronTaskConfig, CronTaskMetadata, CronTaskRow, CronTaskRun } from '../../features/cron/types';
import type { FleetAutomation } from '../../features/cron/types';
import { STATE_LABEL, type ScheduleItem, type ScheduleOutcome } from '../../features/cron/scheduleItems';
import { formatCronShort, parseCronJSON } from './cronTaskUtils';

const props = defineProps<{
  item: ScheduleItem;
  row: CronTaskRow | null;
  automation: FleetAutomation | null;
  runs: CronTaskRun[];
  runsLoading: boolean;
  busy: boolean;
}>();

const emit = defineEmits<{
  close: [];
  run: [];
  edit: [];
  toggle: [enabled: boolean];
  reset: [];
  delete: [];
  history: [];
  'reload-runs': [];
  'open-fleet': [workspaceId: string];
}>();

const OUTCOME_LABEL: Record<Exclude<ScheduleOutcome, ''>, string> = {
  success: '成功',
  failure: '失败',
  skipped: '跳过',
  running: '执行中',
};
const RUN_LABEL: Record<string, string> = {
  success: '成功',
  failure: '失败',
  skipped: '跳过',
  skipped_missed: '错过',
  pending: '执行中',
};

const cfg = computed(() => parseCronJSON<CronTaskConfig>(props.row?.config_json, {}));
const meta = computed(() =>
  props.row ? parseCronJSON<CronTaskMetadata & { skip_count?: number }>(props.row.metadata_json, {}) : null,
);
const message = computed(() => (props.item.kind === 'fleet' ? props.automation?.prompt : cfg.value.message) ?? '');

/** 人话旁边附原始写法，方便核对（cron 表达式 / 时区 / 运行台的计划串）。 */
const rawSchedule = computed(() => {
  if (props.item.kind === 'fleet') return props.automation?.schedule ?? '';
  const c = cfg.value;
  const tz = c.timezone && c.timezone !== 'Asia/Shanghai' ? ` · ${c.timezone}` : '';
  return c.schedule_type === 'cron' ? `${c.cron_expression ?? ''}${tz}` : tz.replace(' · ', '');
});

function runOutcome(status: string): string {
  if (status === 'success') return 'success';
  if (status === 'failure') return 'failure';
  if (status === 'pending') return 'running';
  return 'skipped';
}

function duration(r: CronTaskRun): string {
  const a = Date.parse(r.started_at || r.created_at);
  const b = Date.parse(r.finished_at);
  if (Number.isNaN(a) || Number.isNaN(b) || b < a) return '';
  const s = Math.round((b - a) / 1000);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s`;
}
</script>

<style scoped lang="sass">
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
.cron-detail
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
  min-width: 0
  padding: var(--nt-space-4)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-l)
  background: var(--nt-glass-panel, var(--nt-bg-1))
  overflow: auto
.cron-detail__head
  display: flex
  align-items: flex-start
  gap: var(--nt-space-2)
.cron-detail__title
  flex: 1
  min-width: 0
  h2
    margin: 0
    color: var(--nt-text-0)
    font-size: var(--nt-fs-panel)
    font-weight: 600
    line-height: 1.35
    overflow-wrap: anywhere
.cron-detail__sub
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.cron-detail__actions
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-2)
.cron-detail__alert
  margin: 0
  padding: var(--nt-space-2) var(--nt-space-3)
  border: 1px solid var(--nt-danger)
  border-radius: var(--nt-radius-m)
  color: var(--nt-danger)
  font-size: var(--nt-fs-caption)
.cron-detail__facts
  display: grid
  grid-template-columns: 4.5em 1fr
  gap: var(--nt-space-2) var(--nt-space-3)
  margin: 0
  font-size: var(--nt-fs-caption)
  dt
    color: var(--nt-text-3)
  dd
    display: flex
    flex-wrap: wrap
    align-items: baseline
    gap: var(--nt-space-2)
    margin: 0
    min-width: 0
    color: var(--nt-text-1)
.cron-detail__message
  max-height: 9em
  overflow: auto
  white-space: pre-wrap
  max-width: 72ch
.cron-detail__error
  color: var(--nt-danger)
  overflow-wrap: anywhere
.cron-detail__muted
  margin: 0
  color: var(--nt-text-3)
  font-size: var(--nt-fs-caption)
.cron-detail__stats
  display: grid
  grid-template-columns: repeat(4, 1fr)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  > div
    display: flex
    flex-direction: column
    align-items: center
    gap: 2px
    padding: var(--nt-space-2) 0
    & + div
      border-left: 1px solid var(--nt-border-1)
  b
    color: var(--nt-text-0)
    font-size: var(--nt-fs-panel)
    &.is-ok
      color: var(--nt-success)
    &.is-fail
      color: var(--nt-danger)
  span
    color: var(--nt-text-3)
    font-size: var(--nt-fs-micro)
.cron-detail__runs
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  header
    display: flex
    align-items: center
    gap: var(--nt-space-1)
  h3
    margin: 0
    color: var(--nt-text-2)
    font-size: var(--nt-fs-caption)
    font-weight: 600
.cron-runs
  margin: 0
  padding: 0
  list-style: none
.cron-run
  display: flex
  flex-wrap: wrap
  align-items: baseline
  gap: var(--nt-space-1) var(--nt-space-3)
  padding: var(--nt-space-2) 0
  border-top: 1px solid var(--nt-border-1)
  font-size: var(--nt-fs-caption)
.cron-run__link
  color: var(--nt-accent)
  &:focus-visible
    outline: 2px solid var(--nt-accent)
    outline-offset: 2px
.cron-run__tag
  padding: 0 var(--nt-space-1)
  border: 1px solid var(--nt-border-2)
  border-radius: var(--nt-radius-s)
  color: var(--nt-text-3)
  font-size: var(--nt-fs-micro)
.cron-run__err
  flex-basis: 100%
  overflow: hidden
  color: var(--nt-danger)
  text-overflow: ellipsis
  white-space: nowrap
.cron-state
  flex: none
  padding: 1px var(--nt-space-2)
  border-radius: var(--nt-radius-pill)
  font-size: var(--nt-fs-micro)
  &.is-active
    background: color-mix(in srgb, var(--nt-success) 14%, transparent)
    color: var(--nt-success)
  &.is-paused
    background: var(--nt-bg-3)
    color: var(--nt-text-2)
  &.is-dead
    background: color-mix(in srgb, var(--nt-danger) 14%, transparent)
    color: var(--nt-danger)
.cron-outcome
  font-size: var(--nt-fs-micro)
  &.is-success
    color: var(--nt-success)
  &.is-failure
    color: var(--nt-danger)
  &.is-skipped
    color: var(--nt-warn)
  &.is-running
    color: var(--nt-accent)
</style>
