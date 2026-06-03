<template>
  <div v-if="progress.active" class="sessions-bulk-progress q-mb-md">
    <div class="row items-center q-gutter-sm q-mb-xs">
      <span class="text-caption text-weight-medium">{{ progress.label }}</span>
    </div>
    <q-linear-progress rounded size="8px" indeterminate color="primary" />
  </div>
</template>

<script setup lang="ts">
import type { BulkProgress } from '../../features/session/types';

defineProps<{
  progress: BulkProgress;
}>();
</script>
