<script setup lang="ts">
// 经营总览（整合 P2-8，自 ceo-dashboard HeroBanner.vue 移植）：
// 一句话结论 + 四个关键数字。CEO 打开页面 3 秒内要知道的全部。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CompanySnapshot, Overview } from '../../features/ceoboard/types';
import { overviewLevel } from '../../features/ceoboard/healthRank';

const props = defineProps<{ ov: Overview; snap?: CompanySnapshot | null; meName?: string }>();
const { t } = useI18n();

const wfs = computed(() => props.ov.lanes.flatMap((l) => l.workflows));
const redWfs = computed(() => wfs.value.filter((w) => w.health === 'red'));
const yellowWfs = computed(() => wfs.value.filter((w) => w.health === 'yellow'));
const downPlatforms = computed(() => props.ov.platforms.filter((p) => p.health === 'red'));
const totalVolume = computed(() => wfs.value.reduce((s, w) => s + (w.volume_1h ?? 0), 0));
const approvals = computed(() => props.ov.approvals.count);

const level = computed(() => {
  if (props.snap?.company?.health) return props.snap.company.health as 'red' | 'yellow' | 'green' | 'gray';
  return overviewLevel(props.ov);
});

const statusWord = computed(() => t(`ceoboard.hero.status.${level.value}`));

interface HeroTile {
  id: string;
  value: string;
  label: string;
  tone: '' | 'warn' | 'bad';
}

function kpi(id: string) {
  return props.snap?.kpis.find((k) => k.id === id);
}

const tiles = computed<HeroTile[]>(() => {
  if (props.snap) {
    return [
      { id: 'content_inflow', value: kpi('content_inflow')?.display ?? '—', label: t('ceoboard.ops.kpis.content_inflow'), tone: '' },
      {
        id: 'review_pending',
        value: kpi('review_pending')?.display ?? '—',
        label: t('ceoboard.ops.kpis.review_pending'),
        tone: kpi('review_pending')?.health === 'red' ? 'bad' : kpi('review_pending')?.health === 'yellow' ? 'warn' : '',
      },
      {
        id: 'publish_failed_24h',
        value: kpi('publish_failed_24h')?.display ?? '—',
        label: t('ceoboard.ops.kpis.publish_failed_24h'),
        tone: kpi('publish_failed_24h')?.health === 'red' ? 'bad' : '',
      },
      {
        id: 'opportunity_open',
        value: kpi('opportunity_open')?.display ?? '—',
        label: t('ceoboard.ops.kpis.opportunity_open'),
        tone: '',
      },
    ];
  }
  return [
    { id: 'volume', value: String(totalVolume.value), label: t('ceoboard.hero.tileVolume'), tone: '' },
    { id: 'approvals', value: String(approvals.value), label: t('ceoboard.hero.tileApprovals'), tone: approvals.value > 0 ? 'warn' : '' },
    {
      id: 'flows',
      value: String(redWfs.value.length + yellowWfs.value.length),
      label: t('ceoboard.hero.tileBadFlows'),
      tone: redWfs.value.length > 0 ? 'bad' : '',
    },
    {
      id: 'platforms',
      value: `${props.ov.platforms.length - downPlatforms.value.length}/${props.ov.platforms.length}`,
      label: t('ceoboard.hero.tileOnline'),
      tone: downPlatforms.value.length > 0 ? 'bad' : '',
    },
  ];
});

const subline = computed(() => {
  if (props.snap) {
    const issues = props.snap.anomalies ?? [];
    const red = issues.filter((a) => a.level === 'red').length;
    if (props.snap.company.health === 'gray' && !issues.length) {
      return t('ceoboard.hero.opsEmpty');
    }
    if (!issues.length) {
      return t('ceoboard.hero.opsCalm', { count: kpi('content_inflow')?.display ?? '0' });
    }
    return t('ceoboard.hero.opsIssues', { count: issues.length, red });
  }
  if (level.value === 'green') {
    return t('ceoboard.hero.subGreen', { count: wfs.value.length });
  }
  const parts: string[] = [];
  if (downPlatforms.value.length) parts.push(t('ceoboard.hero.partDown', { count: downPlatforms.value.length }));
  if (redWfs.value.length) parts.push(t('ceoboard.hero.partRed', { count: redWfs.value.length }));
  if (yellowWfs.value.length) parts.push(t('ceoboard.hero.partYellow', { count: yellowWfs.value.length }));
  if (approvals.value > 0) parts.push(t('ceoboard.hero.partApprovals', { count: approvals.value }));
  return parts.join('，') + '。';
});

const greeting = computed(() => {
  const h = new Date().getHours();
  const g =
    h < 6
      ? t('ceoboard.hero.greet.night')
      : h < 12
        ? t('ceoboard.hero.greet.morning')
        : h < 14
          ? t('ceoboard.hero.greet.noon')
          : h < 18
            ? t('ceoboard.hero.greet.afternoon')
            : t('ceoboard.hero.greet.evening');
  return props.meName ? t('ceoboard.hero.greetWithName', { g, name: props.meName }) : g;
});
</script>

<template>
  <section class="ceo-hero" aria-live="polite">
    <div>
      <div class="ceo-hero-greet">{{ greeting }}</div>
      <div class="ceo-hero-status" :class="level">{{ statusWord }}</div>
      <div class="ceo-hero-sub">{{ subline }}</div>
    </div>
    <div class="ceo-hero-tiles" role="list">
      <div v-for="tile in tiles" :key="tile.id" class="ceo-tile" :class="tile.tone" role="listitem">
        <span class="ceo-tile-label">{{ tile.label }}</span>
        <span class="ceo-tile-num">{{ tile.value }}</span>
      </div>
    </div>
  </section>
</template>
