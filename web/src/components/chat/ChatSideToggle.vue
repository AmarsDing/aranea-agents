<template>
  <div class="chat-side__toggle column flex-center">
    <q-btn
      round
      dense
      :icon="icon"
      color="primary"
      :aria-label="ariaLabel"
      class="chat-side__toggle-btn shadow-2"
      @click="$emit('toggle')"
    />
  </div>
</template>

<script setup lang="ts">
defineProps<{
  open: boolean;
  icon: string;
  ariaLabel: string;
}>();

defineEmits<{
  toggle: [];
}>();
</script>
