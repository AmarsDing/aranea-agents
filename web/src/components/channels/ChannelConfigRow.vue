<template>
  <div class="app-musebot-row">
    <label class="app-musebot-row__label" :for="inputId">{{ label }}</label>
    <div class="app-musebot-row__control">
      <slot />
    </div>
    <span v-if="status" class="app-musebot-row__status">{{ status }}</span>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  label: string;
  status?: string;
  fieldKey?: string;
}>();

const inputId = computed(() => (props.fieldKey ? `channel-field-${props.fieldKey}` : undefined));
</script>
