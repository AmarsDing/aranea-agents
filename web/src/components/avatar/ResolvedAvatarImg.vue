<template>
  <img v-if="displaySrc" class="resolved-avatar-img" :src="displaySrc" :alt="alt" />
</template>

<script setup lang="ts">
import { computed } from "vue";
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
  const v = props.icon?.trim() ?? "";
  if (!v) return "";
  if (/^(https?:|data:|blob:)/i.test(v)) return v;
  return resolved.value;
});
</script>
