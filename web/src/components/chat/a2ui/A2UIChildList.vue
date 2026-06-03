<template>
  <A2UIComponentNode
    v-for="childId in childIds"
    :key="childId"
    :component-id="childId"
    :surface="surface"
    @user-action="(p) => emit('user-action', p)"
  />
</template>

<script setup lang="ts">
import type { A2UISurfaceState } from '../../../features/chat/a2uiSurfaceState';
import type { A2UIUserActionPayload } from '../../../features/chat/a2uiUserAction';
import A2UIComponentNode from '../A2UIComponentNode.vue';

defineProps<{
  childIds: string[];
  surface: A2UISurfaceState;
}>();

const emit = defineEmits<{
  'user-action': [payload: A2UIUserActionPayload];
}>();
</script>
