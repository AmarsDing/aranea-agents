<template>
  <div v-if="ui.comp" :class="ui.wrapperClass" :style="ui.wrapperStyle">
    <A2UIKindContent
      :component-id="componentId"
      :surface="surface"
      :kind="ui.kind"
      :ctx="ui"
      @user-action="(p) => emit('user-action', p)"
    />
  </div>
</template>

<script setup lang="ts">
import { toRef } from "vue";
import { useA2UIComponent } from "../../features/chat/a2ui/useA2UIComponent";
import type { A2UISurfaceState } from "../../features/chat/a2uiSurfaceState";
import type { A2UIUserActionPayload } from "../../features/chat/a2uiUserAction";
import A2UIKindContent from "./a2ui/A2UIKindContent.vue";

const props = defineProps<{
  componentId: string;
  surface: A2UISurfaceState;
}>();

const emit = defineEmits<{
  "user-action": [payload: A2UIUserActionPayload];
}>();

const ui = useA2UIComponent(
  toRef(props, "componentId"),
  toRef(props, "surface"),
  (p) => emit("user-action", p)
);
</script>

<script lang="ts">
export default { name: "A2UIComponentNode" };
</script>
