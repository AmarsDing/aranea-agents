<template>
  <section class="hs-panel hs-credit">
    <header class="hs-head">
      <q-icon name="toll" size="18px" class="hs-head__icon" />
      <h2 class="hs-head__title">{{ t('harnessScreen.quota.title') }}</h2>
      <span class="hs-head__cap">{{ t('harnessScreen.credits.cap') }}</span>
    </header>
    <div v-if="loading && !rows.length" class="hs-empty">{{ t('harnessScreen.loading') }}</div>
    <ul v-else class="hs-credit__list">
      <li v-for="r in rows" :key="r.key" class="hs-credit__row" :class="{ 'hs-credit__row--missing': r.missing }">
        <div class="hs-credit__who" :title="r.sub">
          <span class="hs-credit__title">{{ r.title }}</span>
          <span v-if="r.sub" class="hs-credit__sub" :class="{ 'hs-credit__sub--stale': r.stale }">{{ r.sub }}</span>
        </div>
        <span class="hs-credit__value hs-num" :class="`hs-tone--${r.tone}`">{{ r.value }}</span>
        <q-btn
          v-if="r.editable && r.platform"
          class="hs-credit__edit"
          flat
          round
          dense
          size="xs"
          icon="edit"
          :aria-label="t('harnessScreen.credits.edit', { name: r.title })"
          @click="$emit('edit', r.platform)"
        />
        <span v-if="r.used !== null || r.windows.length || r.spark.length" class="hs-credit__meters">
          <!-- 近 30 天每天的剩余：往下走就是在消耗 -->
          <OverviewSparkline v-if="r.spark.length" class="hs-credit__spark" :points="r.spark" />
          <span
            v-if="r.used !== null"
            class="hs-credit__bar"
            role="img"
            :aria-label="t('harnessScreen.credits.usedAria', { name: r.title, pct: Math.round(r.used * 100) })"
          >
            <i :class="`hs-meter--${r.tone}`" :style="{ width: `${Math.max(r.used * 100, 2)}%` }" />
          </span>
          <span v-for="w in r.windows" :key="w.label" class="hs-credit__win hs-num" :class="`hs-tone--${w.tone}`">
            {{ w.label }} {{ Math.round(w.pct) }}%
          </span>
        </span>
      </li>
    </ul>
    <p v-if="unavailable" class="hs-quota__note">{{ t('harnessScreen.credits.unavailable') }}</p>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import OverviewSparkline from '../overview/OverviewSparkline.vue';
import type { HsCreditRow } from './hsTypes';

defineProps<{ rows: HsCreditRow[]; loading: boolean; unavailable: boolean }>();
defineEmits<{ edit: [platform: string] }>();
const { t } = useI18n();
</script>
