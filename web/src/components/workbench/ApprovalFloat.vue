<template>
  <div
    v-if="item"
    class="wb-afloat wb-await-frame"
    :class="{ 'wb-afloat--docked': docked }"
    role="dialog"
    :aria-label="t('workbench.float.title')"
  >
    <header class="wb-afloat__head">
      <span class="wb-afloat__kind">{{ t(`workbench.lanes.${item.lane}`) }}</span>
      <span class="wb-afloat__meta">{{ item.who }} · {{ shortTime(item.when) }}</span>
      <button type="button" class="wb-afloat__x" :title="t('workbench.you.dismiss')" @click="$emit('dismiss')">
        ×
      </button>
    </header>
    <div class="wb-afloat__summary" :title="item.summary">{{ item.summary }}</div>
    <input
      v-model="note"
      class="wb-afloat__note"
      :placeholder="t('workbench.you.notePlaceholder')"
      @keydown.enter.prevent="signPrimary"
    />
    <div class="wb-afloat__actions">
      <button
        v-if="item.action === 'approve'"
        type="button"
        class="wb-btn-wb wb-await-btn"
        :disabled="busy"
        @click="sign('approved')"
      >
        {{ t('workbench.you.approve') }}
      </button>
      <button
        v-if="item.action === 'ack'"
        type="button"
        class="wb-btn-wb wb-await-btn"
        :disabled="busy"
        @click="sign('acked')"
      >
        {{ t('workbench.you.ack') }}
      </button>
      <button
        v-if="item.action === 'approve'"
        type="button"
        class="wb-btn-wb wb-btn-wb--ghost"
        :disabled="busy"
        @click="sign('rejected')"
      >
        {{ t('workbench.you.reject') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * 非模态审批浮层（GrokBot/设计 §4.3 wrap)：吸附工作区右上、无遮罩不锁屏、可暂挂。
 * 金门呼吸按钮属事件态豁免（待签>0），非稳态辉光。
 */
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { YouItem } from '../../features/workbench/types';

const props = defineProps<{
  item: YouItem | null;
  busy: boolean;
  /** 钻取抽屉开启时右移让位（抽屉 420px + 28px 余距），避免遮挡抽屉关闭钮 */
  docked: boolean;
}>();

const emit = defineEmits<{
  sign: [item: YouItem, verb: 'approved' | 'rejected' | 'acked', note: string];
  dismiss: [];
}>();

const { t } = useI18n();
const note = ref('');

watch(
  () => props.item?.id,
  () => {
    note.value = '';
  },
);

function sign(verb: 'approved' | 'rejected' | 'acked'): void {
  if (!props.item) return;
  emit('sign', props.item, verb, note.value.trim());
}

function signPrimary(): void {
  if (!props.item) return;
  sign(props.item.action === 'ack' ? 'acked' : 'approved');
}

function shortTime(iso: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
}
</script>

<style scoped lang="sass">
.wb-afloat
  position: absolute
  top: 12px
  right: 12px
  z-index: 30
  width: 300px
  padding: 10px 12px
  border-radius: var(--wb-radius)
  background: color-mix(in srgb, var(--canvas-base) 88%, var(--color-text-primary))
  box-shadow: 0 12px 30px color-mix(in srgb, var(--color-shadow) 35%, transparent)
  transition: right var(--wb-dur-view) ease
  // 投影只给真正浮起物（总论铁律 2 例外条款）
.wb-afloat--docked
  right: 448px
.wb-afloat__head
  display: flex
  align-items: center
  gap: 8px
  font-size: var(--wb-micro-label)
  letter-spacing: 0.12em
  text-transform: uppercase
  color: var(--wb-await)
.wb-afloat__meta
  flex: 1
  text-align: right
  color: var(--color-text-secondary)
  letter-spacing: 0
  text-transform: none
.wb-afloat__x
  border: none
  background: transparent
  color: var(--color-text-secondary)
  font-size: 14px
  cursor: pointer
  &:hover
    color: var(--color-text-primary)
.wb-afloat__summary
  margin: 8px 0
  font-size: var(--wb-body)
  color: var(--color-text-primary)
.wb-afloat__note
  width: 100%
  margin-bottom: 8px
  padding: 4px 8px
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  background: transparent
  color: var(--color-text-primary)
  font-size: var(--wb-caption)
  &:focus
    outline: 1px solid var(--wb-await-line)
.wb-afloat__actions
  display: flex
  gap: 8px
.wb-btn-wb
  border: none
  border-radius: var(--wb-radius)
  padding: 5px 14px
  font-size: var(--wb-caption)
  cursor: pointer
  &:disabled
    opacity: 0.5
    cursor: wait
.wb-btn-wb--ghost
  border: 1px solid var(--wb-line-hairline)
  background: transparent
  color: var(--color-text-secondary)
</style>
