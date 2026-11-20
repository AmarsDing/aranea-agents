<template>
  <section class="hs-panel hs-tools">
    <header class="hs-head">
      <q-icon name="handyman" size="18px" class="hs-head__icon" />
      <h2 class="hs-head__title">{{ t('harnessScreen.tools.title') }}</h2>
      <span class="hs-head__cap">{{ t('harnessScreen.tools.cap') }}</span>
      <span class="hs-legend hs-head__tail" aria-hidden="true">
        <i class="hs-legend__swatch hs-seg--ok" />{{ t('harnessScreen.tools.ok') }}
        <i class="hs-legend__swatch hs-seg--fail" />{{ t('harnessScreen.tools.fail') }}
        <i class="hs-legend__swatch hs-seg--block" />{{ t('harnessScreen.tools.block') }}
      </span>
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
    <div v-if="rows === undefined" class="hs-empty">{{ t('harnessScreen.loading') }}</div>
    <div v-else-if="rows === null" class="hs-empty">{{ t('harnessScreen.tools.unavailable') }}</div>
    <div v-else-if="!rows.length" class="hs-empty">{{ t('harnessScreen.tools.empty') }}</div>
    <ol v-else class="hs-tools__list">
      <li v-for="r in rows" :key="r.key" class="hs-tools__row">
        <span class="hs-tools__name" :title="r.key">{{ r.label }}</span>
        <span
          class="hs-tools__track"
          role="img"
          :aria-label="t('harnessScreen.tools.rowAria', { name: r.label, calls: r.calls, fail: r.failRate })"
        >
          <span class="hs-tools__bar" :style="{ width: `${Math.max(r.ratio * 100, 2)}%` }">
            <i class="hs-seg--ok" :style="{ flexGrow: r.okPart }" />
            <i class="hs-seg--fail" :style="{ flexGrow: r.failPart }" />
            <i class="hs-seg--block" :style="{ flexGrow: r.blockPart }" />
            <i class="hs-seg--other" :style="{ flexGrow: Math.max(1 - r.okPart - r.failPart - r.blockPart, 0) }" />
          </span>
        </span>
        <span class="hs-tools__calls hs-num">{{ r.calls }}</span>
        <span class="hs-tools__fail hs-num">{{ r.failRate }}</span>
      </li>
    </ol>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { HsToolRow } from './hsTypes';

defineProps<{ rows: HsToolRow[] | null | undefined; to?: string }>();
const { t } = useI18n();
</script>
