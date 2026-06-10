<template>
  <A2UIKindPrimitive v-if="route === 'primitive'" :kind="kind" :ctx="ctx" />
  <A2UIKindForm v-else-if="route === 'form'" :kind="kind" :ctx="ctx" />
  <A2UIKindLayout
    v-else-if="route === 'layout'"
    :kind="kind"
    :ctx="ctx"
    :surface="surface"
    @user-action="(p) => emit('user-action', p)"
  />
  <A2UIKindContainer
    v-else-if="route === 'container'"
    :kind="kind"
    :ctx="ctx"
    :surface="surface"
    @user-action="(p) => emit('user-action', p)"
  />
  <div v-else class="text-caption text-grey">{{ kind || 'Unknown' }} ({{ componentId }})</div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { resolveA2UIKindRoute } from '../../../features/chat/a2ui/a2uiKindRegistry';
import type { A2UIComponentCtx } from '../../../features/chat/a2ui/useA2UIComponent';
import type { A2UISurfaceState } from '../../../features/chat/a2uiSurfaceState';
import type { A2UIUserActionPayload } from '../../../features/chat/a2uiUserAction';
import A2UIKindContainer from './kinds/A2UIKindContainer.vue';
import A2UIKindForm from './kinds/A2UIKindForm.vue';
import A2UIKindLayout from './kinds/A2UIKindLayout.vue';
import A2UIKindPrimitive from './kinds/A2UIKindPrimitive.vue';

const props = defineProps<{
  componentId: string;
  surface: A2UISurfaceState;
  kind: string;
  ctx: A2UIComponentCtx;
}>();

const emit = defineEmits<{
  'user-action': [payload: A2UIUserActionPayload];
}>();

const route = computed(() => resolveA2UIKindRoute(props.kind));
</script>
