<template>
  <section class="hs-panel hs-task">
    <header class="hs-head">
      <q-icon name="assignment_turned_in" size="18px" class="hs-head__icon" />
      <h2 class="hs-head__title">{{ t('harnessScreen.task.title') }}</h2>
      <span class="hs-head__cap">{{ t('harnessScreen.task.cap') }}</span>
      <q-btn
        v-if="to"
        class="hs-head__link"
        flat
        dense
        no-caps
        size="sm"
        icon-right="chevron_right"
        :to="to"
        :label="t('harnessScreen.detail')"
      />
    </header>
    <div class="hs-task__stats">
      <div v-for="s in stats" :key="s.key" class="hs-task__stat" :class="s.tone && `hs-tone--${s.tone}`">
        <span class="hs-task__stat-value hs-num">{{ s.value }}</span>
        <span class="hs-task__stat-label">{{ s.label }}</span>
      </div>
    </div>
    <div class="hs-task__dist">
      <div class="hs-task__dist-head">
        <span>{{ t('harnessScreen.task.byStatus') }}</span>
        <span class="hs-num">{{ bars.length ? t('harnessScreen.task.total', { count: total }) : '—' }}</span>
      </div>
      <div v-if="total > 0" class="hs-task__stack" aria-hidden="true">
        <i v-for="b in bars" :key="b.key" :class="`hs-st--${b.key}`" :style="{ flexGrow: b.share }" />
      </div>
      <ul class="hs-task__legend">
        <li v-for="b in bars" :key="b.key">
          <i class="hs-legend__swatch" :class="`hs-st--${b.key}`" aria-hidden="true" />
          <span>{{ b.label }}</span>
          <span class="hs-num">{{ b.count }}</span>
        </li>
      </ul>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { HsStat, HsTaskBar } from './hsTypes';

defineProps<{ stats: HsStat[]; bars: HsTaskBar[]; total: number; to?: string }>();
const { t } = useI18n();
</script>
