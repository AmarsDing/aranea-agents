<template>
  <template v-if="ctx.isContainerKind">
    <A2UIChildList :child-ids="ctx.childIds" :surface="surface" @user-action="(p) => emit('user-action', p)" />
  </template>
  <template v-else-if="kind === 'List'">
    <div :class="ctx.listClass">
      <A2UIChildList :child-ids="ctx.childIds" :surface="surface" @user-action="(p) => emit('user-action', p)" />
    </div>
  </template>
</template>

<script setup lang="ts">
import type { A2UIComponentCtx } from '../../../../features/chat/a2ui/useA2UIComponent';
import type { A2UISurfaceState } from '../../../../features/chat/a2uiSurfaceState';
import type { A2UIUserActionPayload } from '../../../../features/chat/a2uiUserAction';
import A2UIChildList from '../A2UIChildList.vue';

defineProps<{
  kind: string;
  ctx: A2UIComponentCtx;
  surface: A2UISurfaceState;
}>();

const emit = defineEmits<{
  'user-action': [payload: A2UIUserActionPayload];
}>();
</script>
