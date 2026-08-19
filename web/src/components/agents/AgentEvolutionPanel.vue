<template>
  <div class="evolution-panel settings-grid settings-grid--wide">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('agentSettings.evolution.sectionTitle') }}</span>
          </div>
          <p class="settings-section__hint">{{ $t('agentSettings.evolution.sectionHint') }}</p>
        </div>
      </div>
      <q-list separator class="app-glass-list">
        <q-item v-for="item in evolutionToggles" :key="item.key" class="app-glass-list__item--md">
          <q-item-section>
            <q-item-label>{{ item.title }}</q-item-label>
            <q-item-label caption>{{ item.caption }}</q-item-label>
          </q-item-section>
          <q-item-section side><q-toggle v-model="evolution[item.key]" /></q-item-section>
        </q-item>
      </q-list>
      <q-banner rounded class="settings-info-banner settings-info-banner--bordered q-mt-md">
        {{ $t('agentSettings.evolution.styleVsPipelineBanner') }}
      </q-banner>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('agentSettings.evolution.pipelineSectionTitle') }}</span>
          </div>
          <p class="settings-section__hint">{{ $t('agentSettings.evolution.pipelineSectionHint') }}</p>
        </div>
      </div>
      <q-list separator class="app-glass-list q-mb-md">
        <q-item class="app-glass-list__item--md">
          <q-item-section>
            <q-item-label>{{ $t('agentSettings.evolution.pipelineEnableLabel') }}</q-item-label>
            <q-item-label caption>{{ $t('agentSettings.evolution.pipelineEnableCaption') }}</q-item-label>
          </q-item-section>
          <q-item-section side><q-toggle v-model="evolutionSettings.enabled" /></q-item-section>
        </q-item>
        <q-item class="app-glass-list__item--md">
          <q-item-section>
            <q-item-label>{{ $t('agentSettings.evolution.autoApplyLabel') }}</q-item-label>
            <q-item-label caption>{{ $t('agentSettings.evolution.autoApplyCaption') }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-toggle v-model="evolutionSettings.auto_apply" :disable="!evolutionSettings.enabled" />
          </q-item-section>
        </q-item>
      </q-list>
      <q-banner rounded class="settings-info-banner settings-info-banner--bordered q-mb-md">
        {{ $t('agentSettings.evolution.pipelineNotLiveBanner') }}
      </q-banner>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input
          v-model.number="evolutionSettings.min_episodes"
          dense
          outlined
          type="number"
          :label="$t('agentSettings.evolution.minEpisodesLabel')"
          :min="1"
          :max="10000"
          :hint="$t('agentSettings.evolution.minEpisodesHint')"
          @blur="clampIntField(evolutionSettings, 'min_episodes', 1, 10000)"
        />
        <q-input
          v-model.number="evolutionSettings.min_negative_feedback"
          dense
          outlined
          type="number"
          :label="$t('agentSettings.evolution.minNegativeFeedbackLabel')"
          :min="1"
          :max="10000"
          :hint="$t('agentSettings.evolution.minNegativeFeedbackHint')"
          @blur="clampIntField(evolutionSettings, 'min_negative_feedback', 1, 10000)"
        />
        <q-input
          v-model.number="evolutionSettings.throttle_hours"
          dense
          outlined
          type="number"
          :label="$t('agentSettings.evolution.throttleHoursLabel')"
          :min="1"
          :max="8760"
          :hint="$t('agentSettings.evolution.throttleHoursHint')"
          @blur="clampIntField(evolutionSettings, 'throttle_hours', 1, 8760)"
        />
        <q-input
          v-model.number="evolutionSettings.proposal_ttl_days"
          dense
          outlined
          type="number"
          :label="$t('agentSettings.evolution.proposalTtlDaysLabel')"
          :min="1"
          :max="365"
          :hint="$t('agentSettings.evolution.proposalTtlDaysHint')"
          @blur="clampIntField(evolutionSettings, 'proposal_ttl_days', 1, 365)"
        />
        <q-input
          v-model.number="evolutionSettings.persona_max_chars"
          dense
          outlined
          type="number"
          :label="$t('agentSettings.evolution.personaMaxCharsLabel')"
          :min="1"
          :max="20000"
          :hint="$t('agentSettings.evolution.personaMaxCharsHint')"
          @blur="clampIntField(evolutionSettings, 'persona_max_chars', 1, 20000)"
        />
        <q-input
          v-model.number="evolutionSettings.system_prompt_max_appends"
          dense
          outlined
          type="number"
          :label="$t('agentSettings.evolution.maxAppendsLabel')"
          :min="0"
          :max="50"
          :hint="$t('agentSettings.evolution.maxAppendsHint')"
          @blur="clampIntField(evolutionSettings, 'system_prompt_max_appends', 0, 50)"
        />
      </div>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('agentSettings.evalAutoTitle') }}</span>
          </div>
          <p class="settings-section__hint">
            {{ $t('agentSettings.evalAutoHint') }}
          </p>
        </div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-toggle v-model="evalAuto.auto_after_turn" :label="$t('agentSettings.evalAutoEnable')" />
        <q-select
          v-model="evalAuto.dataset_id"
          dense
          outlined
          emit-value
          map-options
          clearable
          :label="$t('agentSettings.evalAutoDataset')"
          :options="evalDatasetOptions"
          :loading="loadingEvalDatasets"
          :disable="!evalAuto.auto_after_turn"
          @popup-show="$emit('load-eval-datasets')"
        />
        <q-input
          v-model="evalAuto.metrics"
          dense
          outlined
          :label="$t('agentSettings.evalAutoMetrics')"
          :disable="!evalAuto.auto_after_turn"
        />
        <q-input
          v-model.number="evalAuto.num_runs"
          dense
          outlined
          type="number"
          :min="1"
          :max="10"
          :label="$t('agentSettings.evalAutoNumRuns')"
          :disable="!evalAuto.auto_after_turn"
          @blur="clampIntField(evalAuto, 'num_runs', 1, 10)"
        />
        <q-input
          v-model.number="evalAuto.min_interval_sec"
          dense
          outlined
          type="number"
          :min="0"
          :max="86400"
          :label="$t('agentSettings.evalAutoMinInterval')"
          :hint="$t('agentSettings.evalAutoMinIntervalHint')"
          :disable="!evalAuto.auto_after_turn"
          @blur="clampIntField(evalAuto, 'min_interval_sec', 0, 86400)"
        />
      </div>
      <q-banner
        v-if="evalAuto.auto_after_turn && !evalAuto.dataset_id"
        rounded
        class="settings-info-banner settings-info-banner--bordered q-mt-md"
      >
        {{ $t('agentSettings.evalAutoNoDatasetWarning') }}
      </q-banner>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('agentSettings.evolution.metricsSectionTitle') }}</span>
          </div>
          <p class="settings-section__hint">{{ $t('agentSettings.evolution.metricsSectionHint') }}</p>
        </div>
        <q-btn-toggle
          v-model="rangeModel"
          rounded
          unelevated
          toggle-color="primary"
          :options="rangeOptions"
          :disable="!metricsEnabled"
        />
      </div>
      <q-banner
        v-if="!metricsEnabled"
        rounded
        class="settings-info-banner settings-info-banner--bordered q-mt-sm"
      >
        {{ $t('agentSettings.evolution.metricsDisabledBanner') }}
      </q-banner>
      <template v-else>
        <q-inner-loading :showing="metricsLoading" :label="$t('agentSettings.evolution.metricsLoading')" />
        <div v-if="!metricsLoading" class="app-metrics-grid q-mt-sm">
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="query_stats" color="primary" size="26px" />
              <div class="overview-metric-card__label">{{ $t('agentSettings.evolution.toolSuccessRateLabel') }}</div>
            </div>
            <div class="overview-metric-card__value">{{ formatPercent(metrics?.tool_success_rate) }}</div>
            <div class="overview-metric-card__caption">{{ $t('agentSettings.evolution.totalEpisodesCaption', { count: metrics?.total_episodes ?? 0 }) }}</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="travel_explore" color="primary" size="26px" />
              <div class="overview-metric-card__label">{{ $t('agentSettings.evolution.retrievalQualityLabel') }}</div>
            </div>
            <div v-if="hasRetrievalData" class="overview-metric-card__value">
              {{ formatPercent(metrics?.retrieval_quality) }}
            </div>
            <div v-else class="overview-metric-card__value text-grey-6">{{ $t('agentSettings.evolution.noData') }}</div>
            <div class="overview-metric-card__caption">
              {{ hasRetrievalData ? $t('agentSettings.evolution.retrievalQualityCaption') : $t('agentSettings.evolution.retrievalNoCallsCaption') }}
            </div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="tips_and_updates" color="primary" size="26px" />
              <div class="overview-metric-card__label">{{ $t('agentSettings.evolution.suggestionsCardLabel') }}</div>
            </div>
            <div class="overview-metric-card__value">{{ pendingSuggestionsCount }}</div>
            <div class="overview-metric-card__caption">{{ $t('agentSettings.evolution.pendingSuggestionsCaption') }}</div>
          </q-card-section>
        </q-card>
      </div>
      <div v-if="metrics?.tool_success_series?.length" class="q-mt-md">
        <div class="settings-subsection__title q-mb-sm">{{ $t('agentSettings.evolution.toolSuccessTrendTitle') }}</div>
        <div class="app-mini-bar-chart">
          <div
            v-for="(pt, idx) in metrics.tool_success_series"
            :key="idx"
            class="app-mini-bar-chart__bar"
            :style="{ height: `${Math.max(4, Math.round((pt.value ?? 0) * 100))}%` }"
            :title="`${pt.date}: ${formatPercent(pt.value)}`"
          />
        </div>
      </div>
      </template>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('agentSettings.evolution.suggestionsSectionTitle') }}</span>
          </div>
          <p class="settings-section__hint">{{ $t('agentSettings.evolution.suggestionsSectionHint') }}</p>
        </div>
      </div>
      <div v-if="suggestions.length === 0" class="app-empty-state-center app-empty-state-center--sm">
        <q-icon name="tips_and_updates" size="36px" color="grey-5" />
        <div class="text-body1">{{ $t('agentSettings.evolution.suggestionsEmptyTitle') }}</div>
        <div class="text-caption">{{ $t('agentSettings.evolution.suggestionsEmptyCaption') }}</div>
      </div>
      <q-list v-else separator class="app-glass-list">
        <q-item v-for="s in suggestions" :key="s.id" class="app-glass-list__item--lg">
          <q-item-section>
            <q-item-label class="text-weight-medium">
              <q-badge :color="suggestionTypeColor(s.type)" class="q-mr-sm" :label="suggestionTypeLabel(s.type)" />
              {{ s.title }}
            </q-item-label>
            <q-item-label caption class="q-mt-xs">{{ s.content }}</q-item-label>
            <q-item-label caption class="q-mt-xs text-grey-5">{{ formatDate(s.created_at) }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="row q-gutter-xs items-center">
              <q-btn flat round dense icon="visibility" size="sm" @click="onShowDetail(s.id)">
                <q-tooltip>{{ $t('agentSettings.evolution.viewDetail') }}</q-tooltip>
              </q-btn>
              <template v-if="s.status === 'pending'">
                <q-btn
                  v-if="s.applicable"
                  flat
                  round
                  dense
                  icon="check"
                  color="positive"
                  size="sm"
                  :loading="applyingId === s.id"
                  @click="onApply(s.id)"
                >
                  <q-tooltip>{{ $t('agentSettings.evolution.applyTooltip') }}</q-tooltip>
                </q-btn>
                <q-btn flat round dense icon="close" color="negative" size="sm" @click="onReject(s.id)">
                  <q-tooltip>{{ $t('agentSettings.evolution.rejectTooltip') }}</q-tooltip>
                </q-btn>
              </template>
              <template v-else>
                <q-btn
                  v-if="s.status === 'applied'"
                  flat
                  round
                  dense
                  icon="undo"
                  color="warning"
                  size="sm"
                  :loading="rollbackingId === s.id"
                  @click="onRollback(s.id)"
                >
                  <q-tooltip>{{ $t('agentSettings.evolution.rollbackTooltip') }}</q-tooltip>
                </q-btn>
                <q-badge
                  :color="s.status === 'applied' ? 'positive' : 'grey'"
                  :label="suggestionStatusLabel(s.status)"
                />
              </template>
            </div>
          </q-item-section>
        </q-item>
      </q-list>
    </section>

    <section class="settings-section settings-section--success">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">{{ $t('agentSettings.evolution.guardrailsSectionTitle') }}</span>
          </div>
          <p class="settings-section__hint">{{ $t('agentSettings.evolution.guardrailsSectionHint') }}</p>
        </div>
      </div>
      <q-banner rounded class="settings-info-banner settings-info-banner--bordered q-mb-md">
        {{ $t('agentSettings.evolution.guardrailsNotLiveBanner') }}
      </q-banner>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input
          v-model.number="guardrails.max_change_per_period"
          dense
          outlined
          type="number"
          step="0.01"
          :label="$t('agentSettings.evolution.maxChangeLabel')"
          :min="0"
          :max="1"
          :hint="$t('agentSettings.evolution.maxChangeHint')"
          @blur="clampFloatField(guardrails, 'max_change_per_period', 0, 1)"
        />
        <q-input
          v-model.number="guardrails.min_data_points"
          dense
          outlined
          type="number"
          :label="$t('agentSettings.evolution.minDataPointsLabel')"
          :min="1"
          :max="100000"
          :hint="$t('agentSettings.evolution.minDataPointsHint')"
          @blur="clampIntField(guardrails, 'min_data_points', 1, 100000)"
        />
        <q-input
          v-model.number="guardrails.rollback_on_decline_percent"
          dense
          outlined
          type="number"
          suffix="%"
          :label="$t('agentSettings.evolution.rollbackDeclineLabel')"
          :min="0"
          :max="100"
          :hint="$t('agentSettings.evolution.rollbackDeclineHint')"
          @blur="clampIntField(guardrails, 'rollback_on_decline_percent', 0, 100)"
        />
      </div>
    </section>

    <AgentEvolutionRejectDialog
      v-model:open="rejectDialogOpen"
      v-model:reason="rejectReason"
      :suggestion-title="rejectTargetTitle"
      :loading="rejectLoading"
      @confirm="confirmReject"
    />
    <AgentEvolutionSuggestionDialog
      v-model:open="detailDialogOpen"
      :suggestion="detailSuggestion"
      :type-label-of="suggestionTypeLabel"
      :status-label-of="suggestionStatusLabel"
    />
  </div>
</template>

<script setup lang="ts">
const evolution = defineModel<Record<EvolutionKey, boolean>>('evolution', { required: true });
const evolutionSettings = defineModel<AgentRuntimeConfigForm['evolutionSettings']>('evolutionSettings', {
  required: true,
});
// Container: approved — evolution Tab 内指标/建议编排；内部调用 useAgentEvolutionPanel。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { EvolutionKey } from './agentUi';
import type { AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';
import { useAgentEvolutionPanel } from '../../features/agents/useAgentEvolutionPanel';
import { formatDate } from '../../shared/format';
import AgentEvolutionRejectDialog from './AgentEvolutionRejectDialog.vue';
import AgentEvolutionSuggestionDialog from './AgentEvolutionSuggestionDialog.vue';

const guardrails = defineModel<{
  max_change_per_period: number;
  min_data_points: number;
  rollback_on_decline_percent: number;
}>('guardrails', { required: true });
const evalAuto = defineModel<AgentRuntimeConfigForm['evaluation']>('evalAuto', { required: true });
const props = defineProps<{
  agentId: string;
  evalDatasetOptions?: { label: string; value: string }[];
  loadingEvalDatasets?: boolean;
}>();
defineEmits<{ 'load-eval-datasets': [] }>();

const { t } = useI18n();

const evolutionRange = ref('30d');

const {
  metricsLoading,
  metrics,
  suggestions,
  applyingId,
  rollbackingId,
  rejectDialogOpen,
  rejectReason,
  rejectLoading,
  rejectingId,
  detailDialogOpen,
  detailSuggestion,
  pendingSuggestionsCount,
  onApply,
  onReject,
  confirmReject,
  onShowDetail,
  onRollback,
} = useAgentEvolutionPanel(
  () => props.agentId,
  () => evolutionRange.value,
);

const rejectTargetTitle = computed(() => suggestions.value.find((s) => s.id === rejectingId.value)?.title ?? '');

// 进化指标开关关闭时，指标看板用遮罩提示替代（U2）。
const metricsEnabled = computed(() => evolution.value.evolution_metrics_enabled);

// 检索质量：series 为空 ⟺ 时间范围内无记忆工具调用，0.0% 会误导用户。
const hasRetrievalData = computed(() => (metrics.value?.retrieval_quality_series?.length ?? 0) > 0);

const rangeModel = computed({
  get: () => evolutionRange.value,
  set: (value: string) => {
    evolutionRange.value = value;
  },
});

const rangeOptions = ['7d', '30d', '90d'].map((value) => ({ label: value, value }));
const evolutionToggles = computed<Array<{ key: EvolutionKey; title: string; caption: string }>>(() => [
  {
    key: 'self_evolve',
    title: t('agentSettings.evolution.toggleSelfEvolveTitle'),
    caption: t('agentSettings.evolution.toggleSelfEvolveCaption'),
  },
  {
    key: 'skill_evolve',
    title: t('agentSettings.evolution.toggleSkillEvolveTitle'),
    caption: t('agentSettings.evolution.toggleSkillEvolveCaption'),
  },
  {
    key: 'evolution_metrics_enabled',
    title: t('agentSettings.evolution.toggleMetricsTitle'),
    caption: t('agentSettings.evolution.toggleMetricsCaption'),
  },
  {
    key: 'evolution_suggestions_enabled',
    title: t('agentSettings.evolution.toggleSuggestionsTitle'),
    caption: t('agentSettings.evolution.toggleSuggestionsCaption'),
  },
]);

function formatPercent(v: number | undefined): string {
  if (v === undefined || v === null) return '—';
  return (v * 100).toFixed(1) + '%';
}

// 数字输入失焦钳位：空值/非法值回落到 min，超出边界截断，避免静默依赖后端默认值。
function clampIntField(obj: Record<string, unknown>, key: string, min: number, max: number): void {
  const raw = Number(obj[key]);
  const rounded = Number.isFinite(raw) ? Math.round(raw) : min;
  obj[key] = Math.min(max, Math.max(min, rounded));
}

function clampFloatField(obj: Record<string, unknown>, key: string, min: number, max: number): void {
  const raw = Number(obj[key]);
  const v = Number.isFinite(raw) ? raw : min;
  obj[key] = Math.min(max, Math.max(min, Math.round(v * 100) / 100));
}

function suggestionTypeLabel(type: string): string {
  switch (type) {
    case 'persona':
      return t('agentSettings.evolution.typePersona');
    case 'prompt':
      return t('agentSettings.evolution.typePrompt');
    case 'skill':
      return t('agentSettings.evolution.typeSkill');
    case 'orchestration_optimization':
      return t('agentSettings.evolution.typeOrchestration');
    default:
      return type || '—';
  }
}

function suggestionTypeColor(type: string): string {
  switch (type) {
    case 'persona':
      return 'deep-purple';
    case 'prompt':
      return 'blue';
    case 'skill':
      return 'teal';
    case 'orchestration_optimization':
      return 'orange';
    default:
      return 'grey';
  }
}

function suggestionStatusLabel(status: string): string {
  switch (status) {
    case 'applied':
      return t('agentSettings.suggestionStatus.applied');
    case 'rejected':
      return t('agentSettings.suggestionStatus.rejected');
    case 'rolled_back':
      return t('agentSettings.suggestionStatus.rolledBack');
    case 'pending':
      return t('agentSettings.suggestionStatus.pending');
    case 'expired':
      return t('agentSettings.suggestionStatus.expired');
    default:
      return status;
  }
}
</script>
