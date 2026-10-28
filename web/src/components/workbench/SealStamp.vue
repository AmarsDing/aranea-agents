<template>
  <transition name="wb-seal-fade">
    <div v-if="receipt" class="wb-seal" aria-live="polite">
      <div class="wb-seal__stamp">{{ t('workbench.seal.you') }}</div>
      <div class="wb-seal__text">{{ verbLine }}</div>
    </div>
  </transition>
</template>

<script setup lang="ts">
/**
 * 金色落款戳 THE SEAL（配角①，设计 §2.5.2）：
 * 回执到达帧 44px 金「你」戳，180ms snap 过冲落印（一次性事件动画，非持续辉光），
 * 900ms 后收起（条目坍缩为绿动词行由页面写日志承担）。
 * 落印时机=真实回执到达；回放态不加（P0 无回放）。
 */
import { computed, onUnmounted, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SignReceipt } from '../../features/workbench/types';

const props = defineProps<{
  receipt: SignReceipt | null;
}>();

const emit = defineEmits<{
  done: [receipt: SignReceipt];
}>();

const { t } = useI18n();
let timer: ReturnType<typeof setTimeout> | null = null;

const verbLine = computed(() => {
  if (!props.receipt) return '';
  return t(`workbench.seal.${props.receipt.verb}`, { title: props.receipt.title.slice(0, 24) });
});

watch(
  () => props.receipt,
  (r) => {
    if (timer) clearTimeout(timer);
    if (r) {
      timer = setTimeout(() => {
        if (props.receipt) emit('done', props.receipt);
      }, 900);
    }
  },
  { immediate: true },
);

onUnmounted(() => {
  if (timer) clearTimeout(timer);
});
</script>

<style scoped lang="sass">
.wb-seal
  position: absolute
  top: 64px
  right: 330px
  z-index: 40
  display: flex
  align-items: center
  gap: 10px
  pointer-events: none
.wb-seal__stamp
  width: 44px
  height: 44px
  border-radius: 8px
  background: var(--wb-await)
  color: var(--color-on-accent)
  font-size: 22px
  font-weight: 800
  display: flex
  align-items: center
  justify-content: center
  // 金色恒取 token（沉淀约：组件内禁止散写金），透明度用 color-mix 派生
  box-shadow: 0 0 16px color-mix(in srgb, var(--wb-await) 50%, transparent)
  animation: wb-seal-snap var(--wb-dur-seal) cubic-bezier(0.2, 1.6, 0.4, 1)
.wb-seal__text
  font-size: var(--wb-caption)
  color: var(--wb-ok)
.wb-seal-fade-leave-active
  transition: opacity 220ms ease
.wb-seal-fade-leave-to
  opacity: 0
</style>
