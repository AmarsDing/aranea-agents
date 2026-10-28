<script setup lang="ts">
import { useI18n } from 'vue-i18n';
const { t } = useI18n();
</script>

<template>
  <section class="ec-summary-scene">
    <h1>
      {{ t('ecommerceLoop.summary.title') }}<strong>{{ t('ecommerceLoop.summary.accent') }}</strong>
    </h1>
    <div
      v-for="(tone, index) in ['data', 'ceo', 'owner']"
      :key="tone"
      class="ec-summary-row ec-panel"
      :class="`ec-tone-${tone}`"
    >
      <b>0{{ index + 1 }}</b>
      <div>
        <h2>{{ t(`ecommerceLoop.summary.${index + 1}.title`) }}</h2>
        <p>{{ t(`ecommerceLoop.summary.${index + 1}.body`) }}</p>
      </div>
    </div>
    <p class="ec-summary-note">{{ t('ecommerceLoop.summary.note') }}</p>
  </section>
</template>
