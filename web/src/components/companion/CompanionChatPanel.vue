<template>
  <!-- 伴侣聊天窗滑出壳（需求 §2.6：从 HUD 右侧滑出/收起）；内容由 Page 经 slot 注入 -->
  <transition name="companion-panel">
    <aside
      v-if="open"
      class="companion-chat-panel column no-wrap"
      role="complementary"
      :aria-label="t('companion.chatPanelTitle')"
    >
      <div class="companion-chat-panel__header row items-center no-wrap">
        <div class="col text-subtitle2 ellipsis">{{ t('companion.chatPanelTitle') }}</div>
        <q-btn flat round dense icon="chevron_right" :aria-label="t('companion.closeChat')" @click="emit('close')" />
      </div>
      <div class="col companion-chat-panel__body">
        <slot />
      </div>
    </aside>
  </transition>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

defineProps<{
  open: boolean;
}>();

const emit = defineEmits<{
  close: [];
}>();

const { t } = useI18n();
</script>

<style scoped lang="sass">
.companion-chat-panel
  position: absolute
  top: 0
  right: 0
  bottom: 0
  width: min(440px, 92vw)
  background: var(--glass-surface)
  backdrop-filter: blur(var(--glass-blur-elevated))
  border-left: 1px solid var(--glass-border)
  z-index: 10

  &__header
    min-height: 48px
    padding: 4px 8px 4px 16px
    border-bottom: 1px solid var(--glass-border)
    color: var(--color-text-primary)

  &__body
    min-height: 0
    display: flex
    flex-direction: column

.companion-panel-enter-active,
.companion-panel-leave-active
  transition: transform 0.28s ease, opacity 0.28s ease

.companion-panel-enter-from,
.companion-panel-leave-to
  transform: translateX(100%)
  opacity: 0
</style>
