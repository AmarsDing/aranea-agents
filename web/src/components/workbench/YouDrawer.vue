<template>
  <aside class="wb-you">
    <div class="wb-you__head">
      <span class="wb-you__title">{{ t('workbench.you.title') }}</span>
      <span v-if="total > 0" class="wb-await-badge" aria-live="polite" aria-atomic="true">{{ total }}</span>
    </div>
    <div class="wb-you__scroll">
      <section v-for="lane in lanes" :key="lane.lane" class="wb-you__lane">
        <header class="wb-you__lane-head">
          <span class="wb-you__lane-name">{{ t(`workbench.lanes.${lane.lane}`) }}</span>
          <span v-if="lane.count > 0" class="wb-await-badge wb-await-badge--sm">{{ lane.count }}</span>
          <span v-else class="wb-you__lane-clear">·</span>
          <button
            v-if="lane.degraded"
            type="button"
            class="wb-you__lane-degraded"
            :title="t('workbench.you.degradedTip')"
            :aria-label="t('workbench.you.retry')"
            @click="$emit('retry-lane', lane.lane)"
          >
            ! {{ t('workbench.you.retry') }}
          </button>
          <span class="wb-you__lane-fresh" :title="t('workbench.you.freshTip')">{{ freshness(lane) }}</span>
        </header>

        <div v-if="lane.denied" class="wb-you__denied">
          <span>{{ t('workbench.you.noPermission') }}</span>
          <q-btn
            dense
            flat
            no-caps
            size="sm"
            color="primary"
            :label="t('workbench.you.openApprovals')"
            @click="$emit('open-route', '/approvals')"
          />
        </div>

        <div v-else-if="lane.detailFailed" class="wb-you__detail-failed" role="status">
          <span>{{ t('workbench.you.detailFailed') }}</span>
          <q-btn
            dense
            flat
            no-caps
            size="sm"
            color="primary"
            :label="t('workbench.you.refresh')"
            @click="$emit('retry-lane', lane.lane)"
          />
        </div>

        <div v-else-if="laneItems(lane).length === 0" class="wb-you__empty">{{ t('workbench.you.laneEmpty') }}</div>

        <div
          v-if="lane.lane === 'alerts' && lane.count > 0"
          class="wb-you__filters"
          role="group"
          :aria-label="t('workbench.you.sourceFilter')"
        >
          <button
            v-for="f in alertFilters"
            :key="f"
            type="button"
            class="wb-you__filter"
            :class="{ 'is-on': alertFilter === f }"
            :aria-pressed="alertFilter === f"
            @click="alertFilter = f"
          >
            {{ t(`workbench.you.source.${f}`) }}
          </button>
        </div>

        <template v-for="item in laneItems(lane)" :key="item.id">
          <div v-if="groupHeaderFor(item)" class="wb-you__group-head">
            <span class="wb-you__group-name">{{ groupHeaderFor(item)?.label }}</span>
            <span class="wb-you__group-count">{{ groupHeaderFor(item)?.count }}</span>
          </div>
          <div
            class="wb-you__item wb-await-frame"
            :class="{ 'wb-you__item--selected': selectedId === item.id, 'wb-you__item--stale': item.stale }"
          >
            <div class="wb-you__row">
              <input
                v-if="canSelect(item)"
                type="checkbox"
                class="wb-you__check"
                :checked="selected.has(item.id)"
                :aria-label="t('workbench.you.selectItem', { title: item.summary })"
                @change="toggleSelect(item.id)"
              />
              <button
                type="button"
                class="wb-you__main"
                :aria-label="t('workbench.you.focusItem', { title: item.summary })"
                @click="$emit('focus-item', item)"
              >
                <span class="wb-you__meta">
                  <span class="wb-you__who">{{ item.who }}</span>
                  <span class="wb-you__tags">
                    <span v-if="kindLabel(item)" class="wb-you__tag">{{ kindLabel(item) }}</span>
                    <span v-if="item.stale" class="wb-you__tag wb-you__tag--warn">{{ t('workbench.you.stale') }}</span>
                    <span class="wb-you__when">{{ shortTime(item.when) }}</span>
                  </span>
                </span>
                <span class="wb-you__summary" :title="item.summary">{{ item.summary }}</span>
              </button>
            </div>

            <div class="wb-you__actions">
              <template v-if="item.action === 'approve'">
                <q-btn
                  dense
                  unelevated
                  no-caps
                  size="sm"
                  color="primary"
                  :label="t('workbench.you.approve')"
                  :loading="isBusy(item.id)"
                  :disable="busyAny(item.id)"
                  @click="$emit('sign', item, 'approved')"
                />
                <q-btn
                  dense
                  flat
                  no-caps
                  size="sm"
                  color="negative"
                  :label="t('workbench.you.reject')"
                  :disable="busyAny(item.id)"
                  @click="startReject(item.id)"
                />
              </template>
              <q-btn
                v-else-if="item.action === 'ack'"
                dense
                unelevated
                no-caps
                size="sm"
                color="primary"
                :label="t('workbench.you.ack')"
                :loading="isBusy(item.id)"
                :disable="busyAny(item.id)"
                @click="$emit('sign', item, 'acked')"
              />
              <q-btn
                v-else-if="item.action === 'resolve'"
                dense
                outline
                no-caps
                size="sm"
                color="primary"
                :label="t('workbench.you.resolve')"
                :loading="isBusy(item.id)"
                :disable="busyAny(item.id)"
                @click="$emit('sign', item, 'resolved')"
              />
              <q-btn
                v-else
                dense
                flat
                no-caps
                size="sm"
                color="grey-6"
                :label="t('workbench.you.goto')"
                @click="$emit('goto-item', item)"
              />
            </div>

            <div v-if="rejectingId === item.id" class="wb-you__reason">
              <textarea
                v-model="rejectNote"
                class="wb-you__reason-input"
                rows="2"
                :placeholder="t('workbench.you.reasonPlaceholder')"
                :aria-label="t('workbench.you.reasonPlaceholder')"
                :disabled="isBusy('reject:' + item.id)"
                @keydown.esc.stop="cancelReject"
              />
              <div class="wb-you__reason-actions">
                <q-btn
                  dense
                  unelevated
                  no-caps
                  size="sm"
                  color="negative"
                  :label="t('workbench.you.reasonSend')"
                  :loading="isBusy('reject:' + item.id)"
                  :disable="!rejectNote.trim() || isBusy('reject:' + item.id)"
                  @click="confirmReject(item)"
                />
                <q-btn
                  dense
                  flat
                  no-caps
                  size="sm"
                  color="grey-6"
                  :label="t('workbench.you.reasonCancel')"
                  @click="cancelReject"
                />
              </div>
              <p v-if="!rejectNote.trim()" class="wb-you__hint">{{ t('workbench.you.reasonRequired') }}</p>
            </div>

            <p v-if="errorMap[item.id]" class="wb-you__error" role="alert">{{ errorMap[item.id] }}</p>
          </div>
        </template>
      </section>
    </div>

    <div v-if="selected.size > 0" class="wb-you__batch" role="region" :aria-label="t('workbench.you.batchRegion')">
      <span class="wb-you__batch-count">{{ t('workbench.you.batchCount', { n: selected.size }) }}</span>
      <q-btn
        v-if="selectedApprove.length > 0"
        dense
        unelevated
        no-caps
        size="sm"
        color="primary"
        :label="t('workbench.you.batchApprove', { n: selectedApprove.length })"
        :disable="batchBusy"
        @click="emitBatch(selectedApprove, 'approved')"
      />
      <q-btn
        v-if="selectedAck.length > 0"
        dense
        unelevated
        no-caps
        size="sm"
        color="primary"
        :label="t('workbench.you.batchAck', { n: selectedAck.length })"
        :disable="batchBusy"
        @click="emitBatch(selectedAck, 'acked')"
      />
      <q-btn
        dense
        flat
        no-caps
        size="sm"
        color="grey-6"
        :label="t('workbench.you.batchClear')"
        @click="selected.clear()"
      />
    </div>
    <footer class="wb-you__foot">{{ t('workbench.you.footer') }}</footer>
  </aside>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { YouItem, YouLaneId, YouLaneState } from '../../features/workbench/types';
import { groupByRequester } from '../../features/workbench/logic';

const props = defineProps<{
  lanes: YouLaneState[];
  total: number;
  selectedId: string;
  busyIds?: string[];
  errors?: Record<string, string>;
  /** 页面秒级时钟（ISO）：驱动 lane 新鲜度文案随秒刷新，避免两次轮询之间显示冻结 */
  nowIso?: string;
}>();

const emit = defineEmits<{
  sign: [item: YouItem, verb: 'approved' | 'rejected' | 'acked' | 'resolved', note?: string];
  'batch-sign': [items: YouItem[], verb: 'approved' | 'acked'];
  'goto-item': [item: YouItem];
  'focus-item': [item: YouItem];
  'retry-lane': [lane: YouLaneId];
  'open-route': [route: string];
}>();

const { t, te } = useI18n();

const alertFilter = ref<'all' | 'dead' | 'cron'>('all');
const alertFilters = ['all', 'dead', 'cron'] as const;
const rejectingId = ref('');
const rejectNote = ref('');
const selected = ref(new Set<string>());

const busySet = computed(() => new Set(props.busyIds ?? []));
const errorMap = computed(() => props.errors ?? {});
const batchBusy = computed(() => (props.busyIds?.length ?? 0) > 0);

function isBusy(id: string): boolean {
  return busySet.value.has(id);
}

/** 同一行任一动作在途 → 该行所有写动作都禁用（拒绝在途时批准不能被点） */
function busyAny(id: string): boolean {
  return isBusy(id) || isBusy(`reject:${id}`);
}

function visibleItems(lane: YouLaneState): YouItem[] {
  if (lane.lane !== 'alerts' || alertFilter.value === 'all') return lane.items;
  return lane.items.filter((i) => i.source === alertFilter.value);
}

/**
 * 审批 lane 按请求人分组：组间按组内最高风险级排序，组内仍按风险级 → 时间。
 * 只有一个请求人时不显示分组头（避免单人场景多一层噪声）。
 */
const approvalGrouping = computed(() => {
  const lane = props.lanes.find((l) => l.lane === 'approval');
  const items = lane?.items ?? [];
  const groups = groupByRequester(items);
  const head = new Map<string, { label: string; count: number }>();
  if (groups.length <= 1) return { items, head };
  const ordered: YouItem[] = [];
  for (const g of groups) {
    if (g.items.length === 0) continue;
    head.set(g.items[0].id, { label: g.requester, count: g.items.length });
    ordered.push(...g.items);
  }
  return { items: ordered, head };
});

/** 审批 lane 用分组重排后的顺序，其余 lane 沿用来源筛选结果 */
function laneItems(lane: YouLaneState): YouItem[] {
  if (lane.lane === 'approval') return approvalGrouping.value.items;
  return visibleItems(lane);
}

function groupHeaderFor(item: YouItem): { label: string; count: number } | null {
  return approvalGrouping.value.head.get(item.id) ?? null;
}

/** 批量只允许非高危的批准/签收项（高危单独确认） */
function canSelect(item: YouItem): boolean {
  return (item.action === 'approve' || item.action === 'ack') && !item.danger && !item.stale;
}

function toggleSelect(id: string): void {
  const next = new Set(selected.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  selected.value = next;
}

const allItems = computed(() => props.lanes.flatMap((l) => l.items));
const selectedApprove = computed(() =>
  allItems.value.filter((i) => selected.value.has(i.id) && i.action === 'approve'),
);
const selectedAck = computed(() => allItems.value.filter((i) => selected.value.has(i.id) && i.action === 'ack'));

function emitBatch(items: YouItem[], verb: 'approved' | 'acked'): void {
  if (items.length === 0 || batchBusy.value) return;
  emit('batch-sign', items, verb);
  // 失败项会行内反馈；选择清空避免对已消失条目反复提交
  selected.value = new Set();
}

function startReject(id: string): void {
  rejectingId.value = rejectingId.value === id ? '' : id;
  rejectNote.value = '';
}

function cancelReject(): void {
  rejectingId.value = '';
  rejectNote.value = '';
}

function confirmReject(item: YouItem): void {
  const note = rejectNote.value.trim();
  if (!note) return;
  emit('sign', item, 'rejected', note);
  // 保留输入让失败时可直接重试；成功后行会随刷新消失
}

function kindLabel(item: YouItem): string {
  if (item.lane !== 'approval' || !item.source) return '';
  const key = `workbench.approval.kinds.${item.source}`;
  return te(key) ? t(key) : '';
}

function shortTime(iso: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}

function freshness(lane: YouLaneState): string {
  if (!lane.updatedAt) return '—';
  const nowMs = props.nowIso ? Date.parse(props.nowIso) : Date.now();
  const at = Date.parse(lane.updatedAt);
  if (Number.isNaN(at) || Number.isNaN(nowMs)) return '—';
  const ms = Math.max(0, nowMs - at);
  const s = Math.floor(ms / 1000);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m`;
}
</script>

<style scoped lang="sass">
.wb-you
  display: flex
  flex-direction: column
  height: 100%
  min-height: 0
  border-left: 1px solid var(--wb-line-hairline)
  background: color-mix(in srgb, var(--wb-await) 4%, var(--canvas-base))
.wb-you__head
  flex: none
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  padding: 10px 12px 8px
  border-bottom: 1px solid var(--wb-line-hairline)
.wb-you__title
  font-size: 12px
  font-weight: 700
  letter-spacing: 0.1em
  text-transform: uppercase
.wb-you__scroll
  flex: 1
  min-height: 0
  overflow-y: auto
  padding: var(--nt-space-2)
.wb-you__lane
  margin-bottom: 10px
.wb-you__lane-head
  display: flex
  align-items: center
  gap: 6px
  padding: 2px 2px 4px
  font-size: var(--wb-micro-label)
  letter-spacing: 0.12em
  text-transform: uppercase
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-you__lane-name
  flex: 1
.wb-await-badge--sm
  min-width: 16px
  height: 15px
  font-size: 10px
.wb-you__lane-clear
  color: var(--wb-ok)
.wb-you__lane-degraded
  border: 0
  padding: 0
  background: transparent
  color: var(--wb-skip)
  font-weight: 700
  font-size: 10px
  cursor: pointer
  &:hover
    text-decoration: underline
  &:focus-visible
    outline: 2px solid var(--wb-await)
    outline-offset: 4px
.wb-you__lane-fresh
  font-variant-numeric: tabular-nums
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-you__empty
  padding: 6px 4px
  font-size: var(--wb-caption)
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-you__detail-failed
  display: flex
  align-items: center
  justify-content: space-between
  gap: 6px
  padding: 6px 8px
  border-radius: var(--wb-radius)
  border: 1px dashed color-mix(in srgb, var(--wb-skip) 55%, transparent)
  background: color-mix(in srgb, var(--wb-skip) 8%, transparent)
  font-size: var(--wb-caption)
  color: var(--wb-skip)
.wb-you__group-head
  display: flex
  align-items: center
  gap: 5px
  margin: 8px 2px 3px
  font-size: 10px
  letter-spacing: 0.08em
  text-transform: uppercase
  color: var(--color-text-secondary)
  &:first-child
    margin-top: 0
.wb-you__group-name
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.wb-you__group-count
  min-width: 15px
  height: 14px
  display: inline-flex
  align-items: center
  justify-content: center
  border-radius: 999px
  border: 1px solid var(--wb-line-hairline)
  font-size: 9px
  font-variant-numeric: tabular-nums
.wb-you__denied
  display: flex
  align-items: center
  justify-content: space-between
  gap: 6px
  padding: 6px 4px
  font-size: var(--wb-caption)
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-you__filters
  display: flex
  gap: var(--nt-space-1)
  padding: 0 2px 6px
.wb-you__filter
  border: 1px solid var(--wb-line-hairline)
  border-radius: 999px
  padding: 1px 8px
  background: transparent
  color: var(--color-text-secondary)
  font-size: 10px
  cursor: pointer
  &.is-on
    border-color: color-mix(in srgb, var(--wb-await) 60%, transparent)
    color: var(--wb-await)
  &:focus-visible
    outline: 2px solid var(--wb-await)
    outline-offset: 4px
.wb-you__item
  position: relative
  padding: var(--nt-space-2)
  margin-bottom: 6px
  border-radius: var(--wb-radius)
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 6%, transparent)
.wb-you__item--selected
  outline: 1px solid var(--wb-await)
  outline-offset: 1px
.wb-you__item--stale
  opacity: 0.72
.wb-you__row
  display: flex
  align-items: flex-start
  gap: 6px
.wb-you__check
  margin: 2px 0 0
  accent-color: var(--wb-await)
.wb-you__main
  flex: 1
  min-width: 0
  display: block
  border: 0
  padding: 0
  background: transparent
  text-align: left
  color: inherit
  cursor: pointer
  border-radius: 6px
  &:focus-visible
    outline: 2px solid var(--wb-await)
    outline-offset: 4px
.wb-you__meta
  display: flex
  justify-content: space-between
  gap: var(--nt-space-2)
  font-size: 10px
  color: var(--color-text-secondary)
.wb-you__who
  font-weight: 600
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
.wb-you__tags
  display: flex
  align-items: center
  gap: var(--nt-space-1)
  flex: none
.wb-you__tag
  border-radius: 999px
  padding: 0 6px
  border: 1px solid var(--wb-line-hairline)
  font-size: 9px
  letter-spacing: 0.04em
.wb-you__tag--warn
  border-color: color-mix(in srgb, var(--wb-skip) 60%, transparent)
  color: var(--wb-skip)
.wb-you__when
  font-variant-numeric: tabular-nums
.wb-you__summary
  display: block
  margin: 3px 0 6px
  font-size: var(--wb-caption)
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
.wb-you__actions
  display: flex
  gap: 6px
  padding-left: 2px
.wb-you__reason
  margin-top: 6px
  padding-top: 6px
  border-top: 1px dashed var(--wb-line-hairline)
.wb-you__reason-input
  width: 100%
  resize: vertical
  border: 1px solid var(--wb-line-hairline)
  border-radius: 6px
  padding: 4px 6px
  background: color-mix(in srgb, var(--canvas-base) 80%, transparent)
  color: var(--color-text-primary)
  font-size: var(--wb-caption)
  &:focus-visible
    outline: 2px solid var(--wb-await)
    outline-offset: 2px
.wb-you__reason-actions
  display: flex
  gap: 6px
  margin-top: var(--nt-space-1)
.wb-you__hint
  margin: var(--nt-space-1) 0 0
  font-size: 10px
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-you__error
  margin: 6px 0 0
  font-size: 10px
  color: var(--wb-block)
  word-break: break-word
.wb-you__batch
  flex: none
  display: flex
  align-items: center
  gap: 6px
  padding: 6px 8px
  border-top: 1px solid var(--wb-line-hairline)
  background: color-mix(in srgb, var(--wb-await) 8%, var(--canvas-base))
.wb-you__batch-count
  flex: 1
  font-size: 10px
  font-variant-numeric: tabular-nums
.wb-you__foot
  flex: none
  padding: var(--nt-space-2) var(--nt-space-3)
  border-top: 1px solid var(--wb-line-hairline)
  font-size: 10px
  color: var(--color-text-tertiary, var(--color-text-secondary))
  letter-spacing: 0.04em
</style>
