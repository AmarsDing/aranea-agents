<template>
  <q-btn
    flat
    dense
    :icon="showToolCalls ? 'build_circle' : 'build_circle_outlined'"
    @click="toggle"
  >
    <q-tooltip>{{ showToolCalls ? t('chat.uiConfig.hideToolCallsTooltip') : t('chat.uiConfig.showToolCallsTooltip') }}</q-tooltip>
  </q-btn>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUiConfigStore } from 'src/stores/uiConfig'

const { t } = useI18n()
const uiConfig = useUiConfigStore()

const showToolCalls = computed(() => uiConfig.showToolCalls)

function toggle() {
  uiConfig.setShowToolCalls(!uiConfig.showToolCalls)
}
</script>
