<template>
  <q-avatar
    color="primary"
    text-color="white"
    :rounded="rounded"
    :size="size"
    :icon="avatarIcon"
    :class="avatarClass"
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
  { alt: "", size: "56px", rounded: true, avatarClass: "" }
);

defineEmits<{
  click: [evt: MouseEvent];
}>();

const iconRef = computed(() => props.icon);
const { avatarSrc, avatarIcon } = useAgentAvatarPreview(iconRef);
</script>
