<template>
  <img v-if="displaySrc" :src="displaySrc" :alt="alt" />
</template>

<script setup lang="ts">
import { computed } from "vue";
import { shouldRenderAgentAvatarImage } from "../../features/avatar/iconModel";
import { useAvatarThumbnailSrc } from "../../features/avatar/useAvatarThumbnailSrc";

const props = withDefaults(
  defineProps<{
    icon: string;
    alt?: string;
  }>(),
  { alt: "" }
);

const iconRef = computed(() => props.icon);
const resolved = useAvatarThumbnailSrc(iconRef);

const displaySrc = computed(() => {
  if (!shouldRenderAgentAvatarImage(props.icon)) return "";
  const v = props.icon?.trim() ?? "";
  if (/^(https?:|data:|blob:)/i.test(v)) return v;
  return resolved.value;
});
</script>
