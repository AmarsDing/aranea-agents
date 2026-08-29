<template>
  <div class="overview-stats-row q-mb-md">
    <div v-for="card in cards" :key="card.label" class="overview-stat-card">
      <div class="overview-stat-card__header">
        <div class="overview-stat-card__icon-wrap" :class="card.iconClass">
          <q-icon :name="card.icon" size="18px" />
        </div>
        <div class="overview-stat-card__label">{{ card.label }}</div>
      </div>
      <div class="overview-stat-card__body">
        <span class="overview-stat-card__value" :class="card.valueClass">{{ card.value }}</span>
      </div>
      <div class="overview-stat-card__bottom">
        <span class="overview-stat-card__bottom-left">{{ card.sub }}</span>
        <span class="overview-stat-card__bottom-right">{{ card.caption }}</span>
      </div>
      <div v-if="card.bar" class="overview-stat-card__bar">
        <div class="overview-stat-card__bar-fill" :class="card.bar.fillClass" :style="{ width: card.bar.width }" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

const props = defineProps<{
  total: number;
  successCount: number;
  failureCount: number;
  avgScore: number;
  /** 已有根因分析的失败报告条数（后端按最近 N 条返回） */
  rcaCount: number;
}>();

interface StatCard {
  label: string;
  icon: string;
  iconClass?: string;
  value: string;
  valueClass?: string;
  sub?: string;
  caption?: string;
  bar?: { width: string; fillClass: string };
}

/** 无记录时比率为 null（显示 —），避免 0/0 误报为 0%。 */
const successRate = computed<number | null>(() => {
  const total = props.successCount + props.failureCount;
  if (total === 0) return null;
  return props.successCount / total;
});

const cards = computed<StatCard[]>(() => {
  const rate = successRate.value;
  const ratePct = rate == null ? null : Math.round(rate * 1000) / 10;

  let rateValueClass = 'overview-stat-card__value--tech-green';
  let rateFill = 'overview-stat-card__bar-fill--ok';
  if (ratePct != null && ratePct < 70) {
    rateValueClass = 'overview-stat-card__value--tech-red';
    rateFill = 'overview-stat-card__bar-fill--danger';
  } else if (ratePct != null && ratePct < 90) {
    rateValueClass = 'overview-stat-card__value--tech-amber';
    rateFill = 'overview-stat-card__bar-fill--warn';
  }

  return [
    {
      label: t('skillsPage.reportStatTotal'),
      icon: 'assessment',
      value: String(props.total),
      valueClass: 'overview-stat-card__value--tech-blue',
      sub: t('skillsPage.reportStatTotalSub', { success: props.successCount, failure: props.failureCount }),
      caption: t('skillsPage.reportStatTotalCaption'),
    },
    {
      label: t('skillsPage.statsSuccessRate'),
      icon: 'verified',
      iconClass: 'overview-stat-card__icon-wrap--accent',
      value: ratePct == null ? '—' : `${ratePct}%`,
      valueClass: rateValueClass,
      sub: t('skillsPage.reportStatRateSub', { count: props.failureCount }),
      bar: ratePct == null ? undefined : { width: `${Math.min(ratePct, 100)}%`, fillClass: rateFill },
    },
    {
      label: t('skillsPage.reportStatAvgScore'),
      icon: 'grade',
      value: props.total > 0 ? props.avgScore.toFixed(1) : '—',
      valueClass: 'overview-stat-card__value--tech-cyan',
      sub: t('skillsPage.reportStatAvgScoreSub'),
    },
    {
      label: t('skillsPage.reportStatFailure'),
      icon: 'error_outline',
      iconClass: props.failureCount > 0 ? 'overview-stat-card__icon-wrap--danger' : undefined,
      value: String(props.failureCount),
      valueClass: props.failureCount > 0 ? 'overview-stat-card__value--tech-red' : undefined,
      sub: t('skillsPage.reportStatFailureSub', { count: props.rcaCount }),
    },
  ];
});
</script>
