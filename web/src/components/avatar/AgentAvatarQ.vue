<template>
  <q-avatar
    :color="emoji ? undefined : 'primary'"
    :text-color="emoji ? undefined : 'white'"
    :rounded="rounded"
    :size="size"
    :icon="resolvedIcon"
    :class="['app-avatar-cover', { 'app-avatar--emoji': emoji }, avatarClass]"
    @click="$emit('click', $event)"
  >
    <img v-if="avatarSrc" :src="avatarSrc" :alt="alt" />
    <span v-else-if="emoji" class="app-avatar-emoji" role="img" :aria-label="alt">{{ emoji }}</span>
    <slot />
  </q-avatar>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useAgentAvatarPreview } from '../../features/avatar/useAgentAvatarPreview';

const props = withDefaults(
  defineProps<{
    icon: string;
    alt?: string;
    size?: string;
    rounded?: boolean;
    avatarClass?: string;
  }>(),
  { alt: '', size: '56px', rounded: false, avatarClass: '' },
);

defineEmits<{
  click: [evt: MouseEvent];
}>();

// 行业包导入的 Agent 头像是 emoji（如 💰）：直接当文字画，不去头像库取缩略图
const emoji = computed(() => {
  const v = props.icon?.trim() ?? '';
  return v.length <= 16 && /^\p{Extended_Pictographic}/u.test(v) ? v : '';
});
const iconRef = computed(() => (emoji.value ? '' : props.icon));
const { avatarSrc, avatarIcon } = useAgentAvatarPreview(iconRef);

/** 有 `<img>` 时不传 `icon`；缩略图加载前显示占位图标 */
const resolvedIcon = computed(() => (avatarSrc.value || emoji.value ? undefined : (avatarIcon.value ?? 'smart_toy')));
</script>
