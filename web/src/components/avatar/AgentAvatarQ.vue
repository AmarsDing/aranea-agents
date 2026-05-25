<template>
  <q-avatar
    color="primary"
    text-color="white"
    :rounded="rounded"
    :size="size"
    :icon="resolvedIcon"
    :class="['app-avatar-cover', avatarClass]"
    @click="$emit('click', $event)"
  >
    <img v-if="avatarSrc" :src="avatarSrc" :alt="alt" />
    <slot />
  </q-avatar>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useAgentAvatarPreview } from "../../features/avatar/useAgentAvatarPreview";

const props = withDefaults(
  defineProps<{
    icon: string;
    alt?: string;
    size?: string;
    rounded?: boolean;
    avatarClass?: string;
  }>(),
  { alt: "", size: "56px", rounded: false, avatarClass: "" }
);

defineEmits<{
  click: [evt: MouseEvent];
}>();

const iconRef = computed(() => props.icon);
const { avatarSrc, avatarIcon } = useAgentAvatarPreview(iconRef);

/** 有 `<img>` 时不传 `icon`；缩略图加载前显示占位图标 */
const resolvedIcon = computed(() => (avatarSrc.value ? undefined : avatarIcon.value ?? "smart_toy"));
</script>
