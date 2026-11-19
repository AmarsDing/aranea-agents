<template>
  <div
    v-if="item"
    ref="rootEl"
    class="wb-afloat wb-await-frame"
    :class="{ 'wb-afloat--docked': docked }"
    role="dialog"
    aria-modal="false"
    :aria-label="t('workbench.float.title')"
    tabindex="-1"
  >
    <header class="wb-afloat__head">
      <span class="wb-afloat__kind">{{ t(`workbench.lanes.${item.lane}`) }}</span>
      <span class="wb-afloat__meta">{{ item.who }} · {{ shortTime(item.when) }}</span>
      <q-btn
        dense
        flat
        round
        size="sm"
        icon="close"
        color="grey-6"
        :aria-label="t('workbench.you.dismiss')"
        @click="$emit('dismiss')"
      />
    </header>
    <div class="wb-afloat__summary" :title="item.summary">{{ item.summary }}</div>
    <p v-if="item.detail" class="wb-afloat__detail">{{ item.detail }}</p>
    <input
      ref="noteEl"
      v-model="note"
      class="wb-afloat__note"
      :placeholder="t('workbench.you.notePlaceholder')"
      :aria-label="t('workbench.you.notePlaceholder')"
      @keydown.enter.prevent="signPrimary"
    />
    <div class="wb-afloat__actions">
      <q-btn
        v-if="item.action === 'approve'"
        dense
        unelevated
        no-caps
        size="sm"
        color="primary"
        :label="t('workbench.you.approve')"
        :loading="busy"
        :disable="busy"
        @click="sign('approved')"
      />
      <q-btn
        v-if="item.action === 'ack'"
        dense
        unelevated
        no-caps
        size="sm"
        color="primary"
        :label="t('workbench.you.ack')"
        :loading="busy"
        :disable="busy"
        @click="sign('acked')"
      />
      <q-btn
        v-if="item.action === 'resolve'"
        dense
        outline
        no-caps
        size="sm"
        color="primary"
        :label="t('workbench.you.resolve')"
        :loading="busy"
        :disable="busy"
        @click="sign('resolved')"
      />
      <q-btn
        v-if="item.action === 'approve'"
        dense
        flat
        no-caps
        size="sm"
        color="negative"
        :label="t('workbench.you.reject')"
        :disable="busy || !note.trim()"
        :title="!note.trim() ? t('workbench.sign.reasonRequired') : ''"
        @click="sign('rejected')"
      />
    </div>
    <p v-if="item.action === 'approve' && !note.trim()" class="wb-afloat__hint">
      {{ t('workbench.float.rejectHint') }}
    </p>
    <p v-if="error" class="wb-afloat__error" role="alert">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
/**
 * 非模态审批浮层（设计 §4.3 wrap)：吸附工作区右上、无遮罩不锁屏、可暂挂。
 * 拒绝理由必填（sign 层同样强校验，防绕过 UI 提交假理由）；Esc 关闭并回焦触发点。
 */
import { nextTick, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { YouItem } from '../../features/workbench/types';

const props = defineProps<{
  item: YouItem | null;
  busy: boolean;
  /** 钻取抽屉开启时右移让位（抽屉 420px + 28px 余距），避免遮挡抽屉关闭钮 */
  docked: boolean;
  error?: string;
}>();

const emit = defineEmits<{
  sign: [item: YouItem, verb: 'approved' | 'rejected' | 'acked' | 'resolved', note: string];
  dismiss: [];
}>();

const { t } = useI18n();
const note = ref('');
const rootEl = ref<HTMLElement | null>(null);
const noteEl = ref<HTMLInputElement | null>(null);

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape' && props.item) emit('dismiss');
}

let returnFocusEl: HTMLElement | null = null;

watch(
  () => props.item?.id,
  async (id) => {
    note.value = '';
    if (!id) {
      window.removeEventListener('keydown', onKeydown);
      // Esc/关闭后回焦触发点（无障碍底线：弹层关闭回焦）
      returnFocusEl?.focus();
      returnFocusEl = null;
      return;
    }
    returnFocusEl = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    window.addEventListener('keydown', onKeydown);
    await nextTick();
    rootEl.value?.focus();
  },
  { immediate: true },
);

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown);
  returnFocusEl?.focus();
  returnFocusEl = null;
});

function sign(verb: 'approved' | 'rejected' | 'acked' | 'resolved'): void {
  if (!props.item || props.busy) return;
  emit('sign', props.item, verb, note.value.trim());
}

function signPrimary(): void {
  if (!props.item) return;
  sign(props.item.action === 'ack' ? 'acked' : props.item.action === 'resolve' ? 'resolved' : 'approved');
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
  box-shadow: var(--nt-shadow-depth)
  transition: right var(--wb-dur-view) ease
  &:focus-visible
    outline: 2px solid var(--wb-await)
    outline-offset: 4px
  // 投影只给真正浮起物（总论铁律 2 例外条款）
.wb-afloat--docked
  right: 448px
.wb-afloat__head
  display: flex
  align-items: center
  gap: var(--nt-space-2)
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
.wb-afloat__summary
  margin: var(--nt-space-2) 0
  font-size: var(--wb-body)
  color: var(--color-text-primary)
.wb-afloat__detail
  margin: 0 0 var(--nt-space-2)
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
  word-break: break-word
.wb-afloat__note
  width: 100%
  margin-bottom: var(--nt-space-2)
  padding: var(--nt-space-1) var(--nt-space-2)
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  background: transparent
  color: var(--color-text-primary)
  font-size: var(--wb-caption)
  &:focus-visible
    outline: 2px solid var(--wb-await)
    outline-offset: 2px
.wb-afloat__actions
  display: flex
  gap: var(--nt-space-2)
.wb-afloat__hint
  margin: 6px 0 0
  font-size: 10px
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-afloat__error
  margin: 6px 0 0
  font-size: 10px
  color: var(--wb-block)
  word-break: break-word
</style>
