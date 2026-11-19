<template>
  <aside class="evidence-rail" :aria-label="t('shell.approvalEvidence.title')">
    <template v-if="item">
      <header class="evidence-rail__head">
        <div class="evidence-rail__kicker">
          <span class="evidence-rail__kind">{{ t(`approvalsPage.kind_${item.kind}`) }}</span>
          <span v-if="backlog > 0" class="evidence-rail__backlog">
            {{ t('shell.approvalEvidence.sameKindBacklog', { n: backlog }) }}
          </span>
        </div>
        <h2 class="evidence-rail__title">{{ item.title || item.id }}</h2>
        <div class="evidence-rail__risk">
          <span class="evidence-rail__risk-label">{{ t('shell.approvalEvidence.riskLabel') }}</span>
          <span class="risk-meter" :data-risk="item.risk_level || 'none'" role="img" :aria-label="riskText">
            <i
              v-for="level in RISK_LEVELS"
              :key="level"
              class="risk-meter__tick"
              :class="{ 'risk-meter__tick--on': riskOn(level) }"
            />
          </span>
          <strong class="risk-meter__text" :data-risk="item.risk_level || 'none'">{{ riskText }}</strong>
        </div>
      </header>

      <p v-if="item.summary" class="evidence-rail__summary">{{ item.summary }}</p>

      <section class="evidence-rail__section">
        <h3 class="evidence-rail__h3">{{ t('shell.approvalEvidence.impact') }}</h3>
        <dl v-if="impacts.length" class="evidence-rail__dl">
          <div v-for="impact in impacts" :key="impact.label" class="evidence-rail__row">
            <dt>{{ impact.label }}</dt>
            <dd class="mono">{{ impact.value }}</dd>
          </div>
        </dl>
        <p v-else class="evidence-rail__muted">—</p>
      </section>

      <section class="evidence-rail__section">
        <h3 class="evidence-rail__h3">{{ t('shell.approvalEvidence.machineVerdict') }}</h3>
        <dl v-if="verdictEntries.length" class="evidence-rail__dl">
          <div v-for="entry in verdictEntries" :key="entry.key" class="evidence-rail__row">
            <dt class="mono">{{ entry.key }}</dt>
            <dd class="mono">{{ entry.value }}</dd>
          </div>
        </dl>
        <p v-else class="evidence-rail__muted">—</p>
      </section>

      <section class="evidence-rail__section">
        <h3 class="evidence-rail__h3">{{ t('shell.approvalEvidence.context') }}</h3>
        <dl v-if="contextEntries.length" class="evidence-rail__dl">
          <div v-for="entry in contextEntries" :key="entry.key" class="evidence-rail__row">
            <dt class="mono">{{ entry.key }}</dt>
            <dd class="mono">{{ entry.value }}</dd>
          </div>
        </dl>
        <pre v-else-if="item.context_raw" class="evidence-rail__raw mono">{{ item.context_raw }}</pre>
        <p v-else class="evidence-rail__muted">{{ t('shell.approvalEvidence.noContext') }}</p>
      </section>

      <footer class="evidence-rail__foot">
        <div class="evidence-rail__row">
          <span class="evidence-rail__foot-label">{{ t('shell.approvalEvidence.requester') }}</span>
          <span class="mono">{{ item.requester || '—' }}</span>
        </div>
        <div class="evidence-rail__row">
          <span class="evidence-rail__foot-label">{{ t('shell.approvalEvidence.attribution') }}</span>
          <span class="mono">{{ item.agent_key || '—' }}</span>
        </div>
        <div class="evidence-rail__row">
          <span class="evidence-rail__foot-label">{{ t('shell.approvalEvidence.createdAt') }}</span>
          <span class="mono">{{ fmtTime(item.created_at) }}</span>
        </div>
        <div class="evidence-rail__row">
          <span class="evidence-rail__foot-label">{{ t('shell.approvalEvidence.queuedFor') }}</span>
          <span class="mono">{{ queuedFor }}</span>
        </div>
      </footer>
    </template>

    <div v-else class="evidence-rail__empty">
      <span class="evidence-rail__empty-mark" aria-hidden="true">◎</span>
      <p>{{ t('shell.approvalEvidence.empty') }}</p>
    </div>
  </aside>
</template>

<script setup lang="ts">
/**
 * EvidenceRail：审批证据与影响面同屏（替代「详情卡片堆字段」）。
 * 业务动作是「在证据之间比较后拍板」，因此必须同屏给出：风险刻度、影响对象、
 * 机器判定、请求上下文、排队时长；原始 context 解析失败时按原文展示，不吞形状。
 */
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { RISK_LEVELS, type ApprovalItemRow, type RiskLevelKey } from '../../features/approval/types';

const props = defineProps<{
  item: ApprovalItemRow | null;
  /** 同类型待办总数（含当前条），用于判断积压规模。 */
  backlog?: number;
}>();

const { t } = useI18n();

const IMPACT_GROUPS: { keys: string[]; labelKey: string }[] = [
  { keys: ['task_id', 'taskId', 'taskID'], labelKey: 'impactTask' },
  { keys: ['agent_id', 'agentId', 'agent_key', 'agentKey', 'target_id', 'targetId'], labelKey: 'impactAgent' },
  { keys: ['skill_id', 'skillId', 'skill_key', 'skillKey'], labelKey: 'impactSkill' },
  { keys: ['schedule_id', 'scheduleId', 'draft_id', 'draftId'], labelKey: 'impactSchedule' },
];

const VERDICT_PATTERN = /verdict|decision|machine|recommend|risk|score|confidence|policy|auto|reason/i;

function formatValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—';
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

const contextObject = computed<Record<string, unknown>>(() => props.item?.context ?? {});

const impacts = computed(() => {
  const context = contextObject.value;
  const rows: { label: string; value: string }[] = [];
  for (const group of IMPACT_GROUPS) {
    const key = group.keys.find((candidate) => context[candidate] !== undefined && context[candidate] !== '');
    if (!key) continue;
    rows.push({
      label: t(`shell.approvalEvidence.${group.labelKey}`),
      value: formatValue(context[key]),
    });
  }
  return rows;
});

const classifiedKeys = computed(() => {
  const context = contextObject.value;
  const impactKeys = new Set(
    IMPACT_GROUPS.flatMap((group) => {
      const key = group.keys.find((candidate) => context[candidate] !== undefined && context[candidate] !== '');
      return key ? [key] : [];
    }),
  );
  const verdict: { key: string; value: string }[] = [];
  const rest: { key: string; value: string }[] = [];
  for (const [key, value] of Object.entries(context)) {
    if (impactKeys.has(key)) continue;
    const entry = { key, value: formatValue(value) };
    (VERDICT_PATTERN.test(key) ? verdict : rest).push(entry);
  }
  return { verdict, rest };
});

const verdictEntries = computed(() => classifiedKeys.value.verdict);
const contextEntries = computed(() => classifiedKeys.value.rest);

const riskText = computed(() => {
  const risk = props.item?.risk_level;
  return risk ? t(`shell.approvalEvidence.risk${risk[0].toUpperCase()}${risk.slice(1)}`) : '—';
});

function riskOn(level: RiskLevelKey): boolean {
  const order: RiskLevelKey[] = ['low', 'medium', 'high'];
  const current = props.item?.risk_level;
  if (!current) return false;
  return order.indexOf(level) <= order.indexOf(current);
}

function fmtTime(iso: string): string {
  if (!iso) return '—';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  return date.toLocaleString();
}

const queuedFor = computed(() => {
  const iso = props.item?.created_at;
  if (!iso) return '—';
  const start = new Date(iso).getTime();
  if (Number.isNaN(start)) return '—';
  const minutes = Math.max(0, Math.round((Date.now() - start) / 60000));
  if (minutes < 60) return t('shell.approvalEvidence.durationMinutes', { n: minutes });
  const hours = Math.round(minutes / 60);
  if (hours < 48) return t('shell.approvalEvidence.durationHours', { n: hours });
  return t('shell.approvalEvidence.durationDays', { n: Math.round(hours / 24) });
});

const backlog = computed(() => props.backlog ?? 0);
</script>

<style scoped lang="sass">
.evidence-rail
  display: flex
  flex-direction: column
  gap: var(--nt-space-4)
  padding: var(--nt-space-4)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-l)
  background: var(--nt-bg-1)
  color: var(--nt-text-1)
  min-height: 320px

.evidence-rail__head
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)

.evidence-rail__kicker
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)

.evidence-rail__kind
  padding: var(--nt-space-1) var(--nt-space-2)
  border-radius: var(--nt-radius-pill)
  border: 1px solid var(--nt-accent-border)
  color: var(--nt-accent)

.evidence-rail__title
  margin: 0
  font-size: var(--nt-fs-title)
  font-weight: 600
  color: var(--nt-text-0)
  word-break: break-word

.evidence-rail__risk
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)

.risk-meter
  display: inline-flex
  gap: var(--nt-space-1)

.risk-meter__tick
  width: 22px
  height: 6px
  border-radius: var(--nt-radius-pill)
  background: var(--nt-bg-3)

.risk-meter__tick--on
  background: var(--nt-warn)

.risk-meter[data-risk='high'] .risk-meter__tick--on
  background: var(--nt-danger)

.risk-meter[data-risk='medium'] .risk-meter__tick--on
  background: var(--nt-warn)

.risk-meter[data-risk='low'] .risk-meter__tick--on
  background: var(--nt-success)

.risk-meter__text[data-risk='high']
  color: var(--nt-danger)

.risk-meter__text[data-risk='medium']
  color: var(--nt-warn)

.risk-meter__text[data-risk='low']
  color: var(--nt-success)

.evidence-rail__summary
  margin: 0
  padding: var(--nt-space-3)
  border-radius: var(--nt-radius-m)
  background: var(--nt-bg-2)
  color: var(--nt-text-1)
  font-size: var(--nt-fs-body)
  line-height: 1.5
  white-space: pre-wrap
  word-break: break-word

.evidence-rail__section
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  padding-top: var(--nt-space-3)
  border-top: 1px solid var(--nt-border-1)

.evidence-rail__h3
  margin: 0
  font-size: var(--nt-fs-caption)
  font-weight: 600
  letter-spacing: 0.04em
  text-transform: uppercase
  color: var(--nt-text-2)

.evidence-rail__dl
  margin: 0
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)

.evidence-rail__row
  display: flex
  justify-content: space-between
  gap: var(--nt-space-3)
  font-size: var(--nt-fs-body)

.evidence-rail__row dt
  color: var(--nt-text-2)
  word-break: break-all

.evidence-rail__row dd
  margin: 0
  color: var(--nt-text-0)
  text-align: right
  word-break: break-all

.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums

.evidence-rail__raw
  margin: 0
  padding: var(--nt-space-2)
  border-radius: var(--nt-radius-s)
  background: var(--nt-bg-2)
  color: var(--nt-text-1)
  white-space: pre-wrap
  word-break: break-all
  max-height: 220px
  overflow: auto

.evidence-rail__muted
  margin: 0
  color: var(--nt-text-2)

.evidence-rail__foot
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)
  padding-top: var(--nt-space-3)
  border-top: 1px solid var(--nt-border-1)
  font-size: var(--nt-fs-caption)
  color: var(--nt-text-2)

.evidence-rail__foot-label
  color: var(--nt-text-2)

.evidence-rail__empty
  margin: auto
  display: flex
  flex-direction: column
  align-items: center
  gap: var(--nt-space-3)
  max-width: 260px
  text-align: center
  color: var(--nt-text-2)

.evidence-rail__empty-mark
  font-size: 40px
  line-height: 1
  opacity: 0.6
</style>
