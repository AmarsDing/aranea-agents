<template>
  <div v-if="surface.ready && !surface.deleted" class="chat-a2ui-surface">
    <A2UIComponentNode
      v-if="surface.rootId"
      :component-id="surface.rootId"
      :surface="surface"
      @user-action="(p) => emit('user-action', p)"
    />
  </div>
  <q-banner v-else-if="surface.deleted" rounded dense class="settings-info-banner">
    Surface 已删除
  </q-banner>
</template>

<script setup lang="ts">
import type { A2UISurfaceState } from "../../features/chat/a2uiSurfaceState";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";
import A2UIComponentNode from "./A2UIComponentNode.vue";

defineProps<{
  surface: A2UISurfaceState;
}>();

const emit = defineEmits<{
  "user-action": [payload: A2UIUserActionPayload];
}>();
</script>

<style scoped>
.chat-a2ui-surface {
  padding: var(--space-2) var(--space-1);
}
</style>
