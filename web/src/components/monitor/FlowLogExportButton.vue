<template>
  <q-btn
    flat
    icon="download"
    :label="t('monitorPage.logs.exportJsonl')"
    :disable="!lines.length"
    @click="$emit('export')"
  />
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { MonitorLogLine } from '../../features/monitor/types';

const { t } = useI18n();

defineProps<{
  traceId: string;
  lines: MonitorLogLine[];
}>();

defineEmits<{
  export: [];
}>();
</script>
