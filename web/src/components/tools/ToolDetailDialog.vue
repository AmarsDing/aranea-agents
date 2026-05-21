<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="tool-dialog-card tool-dialog-card--detail app-dialog-card app-dialog-card--lg">
      <q-card-section class="row items-start justify-between q-gutter-md">
        <div>
          <div class="text-h6">{{ tool?.display_name }}</div>
          <div class="text-caption muted-caption">{{ tool?.key }}</div>
        </div>
        <q-btn flat dense round icon="close" aria-label="关闭详情" class="tool-icon-btn" @click="$emit('update:open', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section>
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

<style scoped lang="sass">
.tool-dialog-card
  max-width: 960px
  width: 92vw
  border-radius: 22px
  border: 1px solid var(--glass-border)
  background: var(--glass-elevated)
  backdrop-filter: blur(var(--glass-blur-elevated))
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated))
  box-shadow: none

.muted-caption
  color: var(--color-text-secondary)

.tool-icon-btn
  color: var(--color-icon-muted)
</style>
