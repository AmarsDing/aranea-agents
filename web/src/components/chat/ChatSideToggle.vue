<template>
  <div class="chat-side__toggle column flex-center">
    <q-btn
      round
      dense
      :icon="icon"
      color="accent"
      v-bind="$attrs"
      class="chat-side__toggle-btn shadow-2"
      @click="$emit('toggle')"
    />
  </div>
</template>

<script setup lang="ts">
defineOptions({ inheritAttrs: false });

defineProps<{
  open: boolean;
  icon: string;
}>();

defineEmits<{
  toggle: [];
}>();
</script>
