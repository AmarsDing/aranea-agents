<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--xl app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between q-gutter-md">
        <div>
          <div class="app-glass-dialog__title">{{ tool?.display_name }}</div>
          <div class="app-glass-dialog__subtitle app-registry-muted-caption">{{ tool?.key }}</div>
        </div>
        <q-btn flat dense round icon="close" aria-label="关闭详情" class="app-registry-icon-btn" @click="$emit('update:open', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-glass-dialog__body">
        <slot />
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { Tool } from "../../features/tools/types";

defineProps<{ open: boolean; tool: Tool | null }>();
defineEmits<{ "update:open": [value: boolean] }>();
</script>
