<template>
  <teleport to="body">
    <transition name="drawer-backdrop">
      <div v-if="open" class="si-run-drawer-backdrop" @click="emit('update:open', false)" />
    </transition>
  </teleport>
  <q-drawer
    :model-value="open"
    :width="720"
    :breakpoint="0"
    side="right"
    overlay
    bordered
    class="si-run-drawer"
    @update:model-value="emit('update:open', $event)"
  >
    <div class="si-run-drawer__head">
      <div class="si-run-drawer__head-info">
        <div class="si-run-drawer__title ellipsis" :title="run?.id">{{ run?.id || '—' }}</div>
        <div class="si-run-drawer__subtitle app-registry-muted-caption ellipsis">
          {{ run?.suggestionId || '—' }}
        </div>
      </div>
      <q-btn flat dense round icon="close" class="app-registry-icon-btn" @click="emit('update:open', false)">
        <q-tooltip>{{ t('selfImprovementPage.drawerClose') }}</q-tooltip>
      </q-btn>
    </div>

    <div v-if="run" class="si-run-drawer__meta">
      <q-chip dense square :color="siStatusColor(run.status)" text-color="white">
        {{ siStatusLabel(t, run.status) }}
      </q-chip>
      <q-chip dense square :color="siRiskColor(run.riskLevel)" text-color="white">
        {{ siRiskLabel(t, run.riskLevel) }}
      </q-chip>
      <q-chip dense square outline :color="siTriggerColor(run.triggerSource)">
        {{ siTriggerLabel(t, run.triggerSource) }}
      </q-chip>
      <q-chip dense square outline>{{ siKindLabel(t, run.patchKind) }}</q-chip>
    </div>

    <q-tabs
      v-model="tab"
      dense
      class="app-registry-detail-tabs"
      active-color="primary"
      indicator-color="primary"
      align="left"
      no-caps
    >
      <q-tab name="overview" :label="t('selfImprovementPage.tabOverview')" />
      <q-tab name="diagnosis" :label="t('selfImprovementPage.tabDiagnosis')" />
      <q-tab name="gates" :label="t('selfImprovementPage.tabGates')" />
      <q-tab name="diff" :label="t('selfImprovementPage.tabDiff')" />
    </q-tabs>

    <div class="si-run-drawer__body">
      <q-inner-loading :showing="loading" />

      <q-tab-panels v-model="tab" class="app-registry-detail-panels bg-transparent">
        <!-- 概览：元信息 + 治理决策 -->
        <q-tab-panel name="overview" class="q-pa-md">
          <template v-if="run">
            <div class="si-run-kv">
              <div class="si-run-kv__item">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldBaseRef') }}</div>
                <div class="si-run-kv__value">{{ run.baseRef || '—' }}</div>
              </div>
              <div class="si-run-kv__item">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldBranch') }}</div>
                <div class="si-run-kv__value ellipsis" :title="run.branch">{{ run.branch || '—' }}</div>
              </div>
              <div class="si-run-kv__item">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldAttempts') }}</div>
                <div class="si-run-kv__value">{{ run.attempts }}</div>
              </div>
              <div class="si-run-kv__item">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldDiff') }}</div>
                <div class="si-run-kv__value">
                  {{
                    t('selfImprovementPage.diffSummary', {
                      files: run.diffStats.files,
                      additions: run.diffStats.additions,
                      deletions: run.diffStats.deletions,
                    })
                  }}
                </div>
              </div>
              <div class="si-run-kv__item">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldAppliedCommit') }}</div>
                <div class="si-run-kv__value">{{ run.appliedCommit || '—' }}</div>
              </div>
              <div class="si-run-kv__item">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldApprovedBy') }}</div>
                <div class="si-run-kv__value">{{ run.approvedBy || '—' }}</div>
              </div>
              <div class="si-run-kv__item">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldObserveUntil') }}</div>
                <div class="si-run-kv__value">{{ formatSITime(run.observeUntil) }}</div>
              </div>
              <div class="si-run-kv__item">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldCreated') }}</div>
                <div class="si-run-kv__value">{{ formatSITime(run.createdAt) }}</div>
              </div>
              <div v-if="run.closedReason" class="si-run-kv__item si-run-kv__item--wide">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.fieldClosedReason') }}</div>
                <div class="si-run-kv__value">{{ run.closedReason }}</div>
              </div>
            </div>

            <div class="overview-section-title q-mt-lg q-mb-sm">{{ t('selfImprovementPage.govTitle') }}</div>
            <template v-if="run.governance">
              <div class="row items-center q-gutter-sm q-mb-sm">
                <q-chip dense square :color="siRiskColor(run.governance.riskLevel)" text-color="white">
                  {{ siRiskLabel(t, run.governance.riskLevel) }}
                </q-chip>
                <q-chip dense square outline>{{ siChannelLabel(t, run.governance.channel) }}</q-chip>
              </div>
              <div class="row q-gutter-xs">
                <q-chip v-for="rule in run.governance.ruleHits" :key="rule" dense outline>{{ rule }}</q-chip>
              </div>
            </template>
            <div v-else class="overview-section-caption">{{ t('selfImprovementPage.govEmpty') }}</div>
          </template>
        </q-tab-panel>

        <!-- 诊断：Analyst 归因 + Critic 审查 -->
        <q-tab-panel name="diagnosis" class="q-pa-md">
          <template v-if="run">
            <div class="overview-section-title q-mb-sm">{{ t('selfImprovementPage.diagTitle') }}</div>
            <template v-if="run.diagnosis">
              <div class="si-run-block">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.diagRootCause') }}</div>
                <div class="si-run-block__text">{{ run.diagnosis.rootCause || '—' }}</div>
              </div>
              <div class="si-run-block">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.diagFixStrategy') }}</div>
                <div class="si-run-block__text">{{ run.diagnosis.fixStrategy || '—' }}</div>
              </div>
              <div class="si-run-block">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.diagImpact') }}</div>
                <div class="si-run-block__text">{{ run.diagnosis.impactScope || '—' }}</div>
              </div>
              <div class="si-run-block">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.diagConfidence') }}</div>
                <div class="si-run-block__text">{{ Math.round(run.diagnosis.confidence * 100) }}%</div>
              </div>
              <div v-if="run.diagnosis.affectedFiles.length" class="si-run-block">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.diagFiles') }}</div>
                <div class="si-run-files">
                  <code v-for="f in run.diagnosis.affectedFiles" :key="f" class="si-run-file">{{ f }}</code>
                </div>
              </div>
            </template>
            <div v-else class="overview-section-caption">{{ t('selfImprovementPage.diagEmpty') }}</div>

            <div class="overview-section-title q-mt-lg q-mb-sm">{{ t('selfImprovementPage.criticTitle') }}</div>
            <template v-if="run.criticReport">
              <div class="row items-center q-gutter-sm q-mb-sm">
                <q-chip dense square :color="run.criticReport.isSafe ? 'positive' : 'negative'" text-color="white">
                  {{
                    run.criticReport.isSafe
                      ? t('selfImprovementPage.criticSafe')
                      : t('selfImprovementPage.criticUnsafe')
                  }}
                </q-chip>
                <q-chip dense square outline :color="siRiskColor(run.criticReport.riskLevel)">
                  {{ siRiskLabel(t, run.criticReport.riskLevel) }}
                </q-chip>
              </div>
              <div v-if="run.criticReport.concerns.length" class="si-run-block">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.criticConcerns') }}</div>
                <ul class="si-run-concerns">
                  <li v-for="(c, i) in run.criticReport.concerns" :key="i">{{ c }}</li>
                </ul>
              </div>
              <div v-if="run.criticReport.suggestion" class="si-run-block">
                <div class="si-run-kv__label">{{ t('selfImprovementPage.criticSuggestion') }}</div>
                <div class="si-run-block__text">{{ run.criticReport.suggestion }}</div>
              </div>
            </template>
            <div v-else class="overview-section-caption">{{ t('selfImprovementPage.criticEmpty') }}</div>
          </template>
        </q-tab-panel>

        <!-- 验证：沙盒 Gate 结果 -->
        <q-tab-panel name="gates" class="q-pa-md">
          <template v-if="run">
            <div v-if="!run.verificationReport.length" class="overview-section-caption">
              {{ t('selfImprovementPage.gatesEmpty') }}
            </div>
            <q-list v-else separator>
              <q-item v-for="gate in run.verificationReport" :key="gate.gate">
                <q-item-section avatar>
                  <q-icon
                    :name="gate.passed ? 'check_circle' : 'cancel'"
                    :color="gate.passed ? 'positive' : 'negative'"
                    size="sm"
                  />
                </q-item-section>
                <q-item-section>
                  <q-item-label>{{ siGateLabel(t, gate.gate) }}</q-item-label>
                  <q-item-label v-if="gate.output" caption lines="3" class="si-run-gate-output">
                    {{ gate.output }}
                  </q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-item-label caption>{{ gate.durationMs }}ms</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </template>
        </q-tab-panel>

        <!-- Diff -->
        <q-tab-panel name="diff" class="q-pa-none">
          <div v-if="!run?.diff" class="overview-section-caption q-pa-md">
            {{ t('selfImprovementPage.diffEmpty') }}
          </div>
          <pre v-else class="si-run-diff"><code
            v-for="(line, i) in diffLines"
            :key="i"
            class="si-run-diff__line"
            :class="line.kind"
          >{{ line.text }}
</code></pre>
        </q-tab-panel>
      </q-tab-panels>
    </div>

    <!-- 治理操作（按状态机可用性渲染） -->
    <div v-if="run && hasActions" class="si-run-drawer__actions">
      <!-- 在途介入指令（ControlRun）：流水线在阶段边界异步消费 -->
      <template v-if="canControl(run.status)">
        <q-btn
          outline
          no-caps
          color="primary"
          icon="pause"
          :label="t('selfImprovementPage.controlPause')"
          :loading="actionRunning === 'control:pause'"
          @click="emit('control', run, 'pause')"
        />
        <q-btn
          outline
          no-caps
          color="warning"
          icon="skip_next"
          :label="t('selfImprovementPage.controlSkipRetry')"
          :loading="actionRunning === 'control:skip_retry'"
          @click="emit('control', run, 'skip_retry')"
        />
        <q-btn
          outline
          no-caps
          color="negative"
          icon="stop_circle"
          :label="t('selfImprovementPage.controlRollback')"
          :loading="actionRunning === 'control:rollback'"
          @click="emit('control', run, 'rollback')"
        />
      </template>
      <q-btn
        v-if="canApprove(run.status)"
        color="positive"
        unelevated
        no-caps
        icon="check"
        :label="t('selfImprovementPage.actionApprove')"
        :loading="actionRunning === 'approve'"
        @click="emit('approve', run)"
      />
      <q-btn
        v-if="canReject(run.status)"
        color="negative"
        outline
        no-caps
        icon="close"
        :label="t('selfImprovementPage.actionReject')"
        :loading="actionRunning === 'reject'"
        @click="emit('reject', run)"
      />
      <q-btn
        v-if="canRollback(run.status)"
        color="warning"
        outline
        no-caps
        icon="undo"
        :label="t('selfImprovementPage.actionRollback')"
        :loading="actionRunning === 'rollback'"
        @click="emit('rollback', run)"
      />
      <q-btn
        v-if="canClose(run.status)"
        color="primary"
        unelevated
        no-caps
        icon="task_alt"
        :label="t('selfImprovementPage.actionClose')"
        :loading="actionRunning === 'close'"
        @click="emit('close', run)"
      />
    </div>
  </q-drawer>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SIControlCommand, SIRunDetail } from '../../features/self-improvement/types';
import {
  canApprove,
  canClose,
  canControl,
  canReject,
  canRollback,
  formatSITime,
  siChannelLabel,
  siGateLabel,
  siKindLabel,
  siRiskColor,
  siRiskLabel,
  siStatusColor,
  siStatusLabel,
  siTriggerColor,
  siTriggerLabel,
} from './selfImprovementUi';

// RunDetailDrawer（73-self-iteration-v3 design §八）：run 详情抽屉 —
// 概览/诊断/验证/Diff 四 Tab + 治理操作（approve/reject/rollback/close）。
// 纯展示组件：数据经 props 注入，操作经 emits 上抛给 Page composable。

const props = defineProps<{
  open: boolean;
  run: SIRunDetail | null;
  loading: boolean;
  actionRunning: string;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  approve: [run: SIRunDetail];
  reject: [run: SIRunDetail];
  rollback: [run: SIRunDetail];
  close: [run: SIRunDetail];
  control: [run: SIRunDetail, command: SIControlCommand];
}>();

const { t } = useI18n();

const tab = ref('overview');

const hasActions = computed(() => {
  const s = props.run?.status ?? '';
  return canApprove(s) || canReject(s) || canRollback(s) || canClose(s) || canControl(s);
});

type DiffLine = { text: string; kind: string };

const diffLines = computed<DiffLine[]>(() => {
  const diff = props.run?.diff ?? '';
  if (!diff) return [];
  return diff.split('\n').map((text) => {
    let kind = 'si-run-diff__line--ctx';
    if (text.startsWith('+++') || text.startsWith('---')) kind = 'si-run-diff__line--meta';
    else if (text.startsWith('@@')) kind = 'si-run-diff__line--hunk';
    else if (text.startsWith('diff ') || text.startsWith('index ')) kind = 'si-run-diff__line--meta';
    else if (text.startsWith('+')) kind = 'si-run-diff__line--add';
    else if (text.startsWith('-')) kind = 'si-run-diff__line--del';
    return { text, kind };
  });
});
</script>

<style scoped lang="sass">
.si-run-drawer
  display: flex
  flex-direction: column

.si-run-drawer__head
  display: flex
  align-items: center
  justify-content: space-between
  padding: 16px 16px 8px

.si-run-drawer__head-info
  min-width: 0

.si-run-drawer__title
  font-weight: 600
  font-size: 0.95rem
  color: var(--color-text-primary)

.si-run-drawer__subtitle
  font-size: 0.76rem

.si-run-drawer__meta
  display: flex
  flex-wrap: wrap
  gap: 6px
  padding: 0 16px 8px

.si-run-drawer__body
  flex: 1
  overflow-y: auto
  position: relative

.si-run-drawer__actions
  display: flex
  gap: 10px
  padding: 12px 16px
  border-top: 1px solid var(--glass-border)

.si-run-kv
  display: grid
  grid-template-columns: repeat(2, minmax(0, 1fr))
  gap: 12px 16px

.si-run-kv__item--wide
  grid-column: 1 / -1

.si-run-kv__label
  font-size: 0.72rem
  color: var(--color-text-secondary)
  margin-bottom: 2px

.si-run-kv__value
  font-size: 0.85rem
  color: var(--color-text-primary)
  word-break: break-all

.si-run-block
  margin-bottom: 12px

.si-run-block__text
  font-size: 0.85rem
  color: var(--color-text-primary)
  white-space: pre-wrap
  word-break: break-word

.si-run-files
  display: flex
  flex-direction: column
  gap: 4px

.si-run-file
  font-size: 0.78rem
  color: var(--color-accent)
  word-break: break-all

.si-run-concerns
  margin: 0
  padding-left: 18px
  font-size: 0.85rem
  color: var(--color-text-primary)

.si-run-gate-output
  white-space: pre-wrap
  word-break: break-all
  font-family: monospace

.si-run-diff
  margin: 0
  padding: 12px 0
  font-family: monospace
  font-size: 0.76rem
  line-height: 1.5
  overflow-x: auto

.si-run-diff__line
  display: block
  padding: 0 16px
  white-space: pre
  color: var(--color-text-primary)

.si-run-diff__line--add
  background: rgba(76, 175, 124, 0.12)
  color: var(--color-success)

.si-run-diff__line--del
  background: rgba(229, 92, 92, 0.12)
  color: var(--color-danger)

.si-run-diff__line--hunk
  color: var(--color-accent)
  font-weight: 600

.si-run-diff__line--meta
  color: var(--color-text-secondary)
</style>
