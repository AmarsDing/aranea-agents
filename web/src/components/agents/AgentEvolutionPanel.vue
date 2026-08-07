<template>
  <div class="evolution-panel settings-grid settings-grid--wide">
    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">进化</span>
          </div>
          <p class="settings-section__hint">控制风格进化、技能进化、指标采集与建议生成。</p>
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
        <strong>沟通风格</strong
        >（SOUL.md）与下方<strong>自动提议流水线</strong>独立：前者控制是否改写语调，后者控制是否扫描并生成改进提议。
      </q-banner>
    </section>

    <section class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">自动提议流水线</span>
          </div>
          <p class="settings-section__hint">基于 Episode / 负反馈触发结构化进化提议（与 L4 图谱注入无关）。</p>
        </div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-toggle v-model="evolutionSettings.enabled" label="启用提议流水线" />
        <q-toggle v-model="evolutionSettings.auto_apply" label="低风险自动应用" />
        <q-input
          v-model.number="evolutionSettings.min_episodes"
          dense
          outlined
          type="number"
          label="触发 Episode 数"
          :min="1"
        />
        <q-input
          v-model.number="evolutionSettings.min_negative_feedback"
          dense
          outlined
          type="number"
          label="触发负反馈数"
          :min="1"
        />
        <q-input
          v-model.number="evolutionSettings.throttle_hours"
          dense
          outlined
          type="number"
          label="节流小时"
          :min="1"
        />
        <q-input
          v-model.number="evolutionSettings.proposal_ttl_days"
          dense
          outlined
          type="number"
          label="提议过期天数"
          :min="1"
        />
        <q-input
          v-model.number="evolutionSettings.persona_max_chars"
          dense
          outlined
          type="number"
          label="Persona 最大字符"
          :min="1"
        />
        <q-input
          v-model.number="evolutionSettings.system_prompt_max_appends"
          dense
          outlined
          type="number"
          label="Prompt 追加段上限"
          :min="0"
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
        />
        <q-input
          v-model.number="evalAuto.min_interval_sec"
          dense
          outlined
          type="number"
          :min="0"
          :label="$t('agentSettings.evalAutoMinInterval')"
          :hint="$t('agentSettings.evalAutoMinIntervalHint')"
          :disable="!evalAuto.auto_after_turn"
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
            <span class="section-title__text">指标与建议</span>
          </div>
          <p class="settings-section__hint">时间范围只影响看板读取，不写入 Agent 配置。</p>
        </div>
        <q-btn-toggle v-model="rangeModel" rounded unelevated toggle-color="primary" :options="rangeOptions" />
      </div>
      <q-inner-loading :showing="metricsLoading" label="加载指标..." />
      <div v-if="!metricsLoading" class="app-metrics-grid q-mt-sm">
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="query_stats" color="primary" size="26px" />
              <div class="overview-metric-card__label">工具成功率</div>
            </div>
            <div class="overview-metric-card__value">{{ formatPercent(metrics?.tool_success_rate) }}</div>
            <div class="overview-metric-card__caption">共 {{ metrics?.total_episodes ?? 0 }} 个会话</div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="travel_explore" color="primary" size="26px" />
              <div class="overview-metric-card__label">检索质量</div>
            </div>
            <div v-if="hasRetrievalData" class="overview-metric-card__value">
              {{ formatPercent(metrics?.retrieval_quality) }}
            </div>
            <div v-else class="overview-metric-card__value text-grey-6">暂无数据</div>
            <div class="overview-metric-card__caption">
              {{ hasRetrievalData ? '记忆工具调用成功率' : '时间范围内无记忆工具调用' }}
            </div>
          </q-card-section>
        </q-card>
        <q-card flat bordered class="overview-metric-card app-metrics-grid__item">
          <q-card-section class="overview-metric-card__body">
            <div class="row items-center q-gutter-sm">
              <q-icon name="tips_and_updates" color="primary" size="26px" />
              <div class="overview-metric-card__label">建议</div>
            </div>
            <div class="overview-metric-card__value">{{ pendingSuggestionsCount }}</div>
            <div class="overview-metric-card__caption">待处理改进建议</div>
          </q-card-section>
        </q-card>
      </div>
      <div v-if="metrics?.tool_success_series?.length" class="q-mt-md">
        <div class="settings-subsection__title q-mb-sm">工具成功率趋势</div>
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
    </section>

    <section v-if="suggestions.length > 0" class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">进化建议列表</span>
          </div>
          <p class="settings-section__hint">基于指标自动生成的改进建议，可应用或拒绝。</p>
        </div>
      </div>
      <q-list separator class="app-glass-list">
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
            <div v-if="s.status === 'pending'" class="row q-gutter-xs">
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
                <q-tooltip>应用</q-tooltip>
              </q-btn>
              <q-btn
                flat
                round
                dense
                icon="close"
                color="negative"
                size="sm"
                :loading="rejectingId === s.id"
                @click="onReject(s.id)"
              >
                <q-tooltip>拒绝</q-tooltip>
              </q-btn>
            </div>
            <q-badge
              v-else
              :color="s.status === 'applied' ? 'positive' : 'grey'"
              :label="suggestionStatusLabel(s.status)"
            />
          </q-item-section>
        </q-item>
      </q-list>
    </section>

    <section class="settings-section settings-section--success">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">适应护栏</span>
          </div>
          <p class="settings-section__hint">限制自动调整幅度，样本不足或表现下降时回滚。</p>
        </div>
      </div>
      <div class="app-form-field-grid app-form-field-grid--2col">
        <q-input
          v-model.number="guardrails.max_change_per_period"
          dense
          outlined
          type="number"
          step="0.01"
          label="每周期最大变化"
        />
        <q-input v-model.number="guardrails.min_data_points" dense outlined type="number" label="最少数据点" />
        <q-input
          v-model.number="guardrails.rollback_on_decline_percent"
          dense
          outlined
          type="number"
          suffix="%"
          label="下降时回滚"
        />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
const evolution = defineModel<Record<EvolutionKey, boolean>>('evolution', { required: true });
const evolutionSettings = defineModel<AgentRuntimeConfigForm['evolutionSettings']>('evolutionSettings', {
  required: true,
});
// Container: approved — evolution Tab 内指标/建议编排；内部调用 useAgentEvolutionPanel。
import { computed, ref } from 'vue';
import type { EvolutionKey } from './agentUi';
import type { AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';
import { useAgentEvolutionPanel } from '../../features/agents/useAgentEvolutionPanel';

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

const evolutionRange = ref('30d');

const { metricsLoading, metrics, suggestions, applyingId, rejectingId, pendingSuggestionsCount, onApply, onReject } =
  useAgentEvolutionPanel(
    () => props.agentId,
    () => evolutionRange.value,
  );

// 检索质量：series 为空 ⟺ 时间范围内无记忆工具调用，0.0% 会误导用户。
const hasRetrievalData = computed(() => (metrics.value?.retrieval_quality_series?.length ?? 0) > 0);

const rangeModel = computed({
  get: () => evolutionRange.value,
  set: (value: string) => {
    evolutionRange.value = value;
  },
});

const rangeOptions = ['7d', '30d', '90d'].map((value) => ({ label: value, value }));
const evolutionToggles: Array<{ key: EvolutionKey; title: string; caption: string }> = [
  { key: 'self_evolve', title: '允许 Agent 进化其沟通风格', caption: '允许随时间更新 SOUL.md 中的语调与风格。' },
  { key: 'skill_evolve', title: '允许从经验中创建和管理技能', caption: '提示用户将有效工作流保存为 Skill。' },
  { key: 'evolution_metrics_enabled', title: '进化指标', caption: '记录工具效果、检索质量、反馈等。' },
  { key: 'evolution_suggestions_enabled', title: '进化建议', caption: '基于指标生成改进建议。' },
];

function formatPercent(v: number | undefined): string {
  if (v === undefined || v === null) return '—';
  return (v * 100).toFixed(1) + '%';
}

function formatDate(iso: string): string {
  if (!iso) return '';
  try {
    return new Date(iso).toLocaleDateString();
  } catch {
    return iso;
  }
}

function suggestionTypeLabel(type: string): string {
  switch (type) {
    case 'persona':
      return '沟通风格';
    case 'prompt':
      return '系统提示';
    case 'skill':
      return '技能调整';
    case 'orchestration_optimization':
      return '编排优化';
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
      return '已应用';
    case 'rejected':
      return '已拒绝';
    case 'rolled_back':
      return '已回滚';
    case 'pending':
      return '待处理';
    case 'expired':
      return '已过期';
    default:
      return status;
  }
}
</script>
