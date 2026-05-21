<!-- @deprecated use ChatExecutionCard.vue -->
<script setup lang="ts">
import ChatExecutionCard from "./ChatExecutionCard.vue";
import type { ToolUseEvent } from "../../features/chat/types";

defineProps<{
  event: ToolUseEvent;
  showMemberLabel?: boolean;
}>();
</script>

<template>
  <ChatExecutionCard v-bind="$props" />
</template>
