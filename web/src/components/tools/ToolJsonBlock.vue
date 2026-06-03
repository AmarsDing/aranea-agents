<template>
  <pre class="app-code-block app-code-block--scroll">{{ text }}</pre>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    text?: string;
  }>(),
  { text: '' },
);
</script>
