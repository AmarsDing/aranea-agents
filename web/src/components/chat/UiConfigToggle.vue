<template>
  <q-btn flat dense :icon="showToolCalls ? 'build_circle' : 'build_circle_outlined'" @click="toggle">
    <q-tooltip>{{
      showToolCalls ? t('chat.uiConfig.hideToolCallsTooltip') : t('chat.uiConfig.showToolCallsTooltip')
    }}</q-tooltip>
  </q-btn>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

defineProps<{
  showToolCalls: boolean;
}>();

const emit = defineEmits<{
  toggle: [];
}>();

const { t } = useI18n();

function toggle() {
  emit('toggle');
}
</script>
