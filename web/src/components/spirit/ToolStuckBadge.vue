<template>
  <q-chip
    v-if="count > 0"
    dense
    size="sm"
    class="tool-stuck-badge"
    icon="warning"
  >
    {{ t('chat.activity.stuckToolBadge', { count }, '⚠ {count} 工具未返回') }}
  </q-chip>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

defineProps<{
  count: number;
}>();

const { t } = useI18n();
</script>

<style scoped lang="sass">
.tool-stuck-badge
  font-size: 12px
  background: var(--color-warning-soft)
  color: var(--color-warning)
</style>
