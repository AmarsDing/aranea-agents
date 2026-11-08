<script setup lang="ts">
// 经营总览（整合 P2-8，自 ceo-dashboard HeroBanner.vue 移植）：
// 一句话结论 + 四个关键数字。CEO 打开页面 3 秒内要知道的全部。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Overview } from '../../features/ceoboard/types';

const props = defineProps<{ ov: Overview; meName?: string }>();
const { t } = useI18n();

const wfs = computed(() => props.ov.lanes.flatMap((l) => l.workflows));
const redWfs = computed(() => wfs.value.filter((w) => w.health === 'red'));
const yellowWfs = computed(() => wfs.value.filter((w) => w.health === 'yellow'));
const downPlatforms = computed(() => props.ov.platforms.filter((p) => p.health === 'red'));
const totalVolume = computed(() => wfs.value.reduce((s, w) => s + (w.volume_1h ?? 0), 0));
const approvals = computed(() => props.ov.approvals.count);

const level = computed<'green' | 'yellow' | 'red'>(() => {
  if (downPlatforms.value.length || redWfs.value.length) return 'red';
  if (yellowWfs.value.length || approvals.value > 0) return 'yellow';
  return 'green';
});

const statusWord = computed(() => t(`ceoboard.hero.status.${level.value}`));

const subline = computed(() => {
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
    <div class="ceo-hero-tiles">
      <div class="ceo-tile">
        <div class="ceo-tile-num">{{ totalVolume }}</div>
        <div class="ceo-tile-label">{{ t('ceoboard.hero.tileVolume') }}</div>
      </div>
      <div class="ceo-tile" :class="{ warn: approvals > 0 }">
        <div class="ceo-tile-num">{{ approvals }}</div>
        <div class="ceo-tile-label">{{ t('ceoboard.hero.tileApprovals') }}</div>
      </div>
      <div class="ceo-tile" :class="{ bad: redWfs.length > 0 }">
        <div class="ceo-tile-num">{{ redWfs.length + yellowWfs.length }}</div>
        <div class="ceo-tile-label">{{ t('ceoboard.hero.tileBadFlows') }}</div>
      </div>
      <div class="ceo-tile" :class="{ bad: downPlatforms.length > 0 }">
        <div class="ceo-tile-num">{{ ov.platforms.length - downPlatforms.length }}/{{ ov.platforms.length }}</div>
        <div class="ceo-tile-label">{{ t('ceoboard.hero.tileOnline') }}</div>
      </div>
    </div>
  </section>
</template>
