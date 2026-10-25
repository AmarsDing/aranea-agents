<template>
  <footer class="ov-footer">
    <div class="ov-footer__sources">
      <span class="ov-footer__title">
        <q-icon name="schedule" size="13px" />
        {{ t('overviewPage.v2.freshnessTitle') }}
      </span>
      <span v-for="s in sources" :key="s.label" class="ov-footer__src">
        {{ s.label }} <strong>{{ s.time }}</strong>
      </span>
    </div>
    <ul class="ov-footer__notes">
      <li v-for="(note, i) in notes" :key="i">{{ note }}</li>
    </ul>
  </footer>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { OvSource } from './ovTypes';

defineProps<{ sources: OvSource[]; notes: string[] }>();

const { t } = useI18n();
</script>
