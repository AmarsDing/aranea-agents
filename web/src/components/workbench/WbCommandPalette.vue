<template>
  <q-dialog :model-value="open" position="top" seamless @update:model-value="$emit('close')">
    <div class="wb-palette">
      <input
        ref="inputEl"
        v-model="query"
        class="wb-palette__input"
        :placeholder="t('workbench.cmdk.placeholder')"
        @keydown.esc.prevent="$emit('close')"
        @keydown.enter.prevent="runFirst"
        @keydown.down.prevent="move(1)"
        @keydown.up.prevent="move(-1)"
      />
      <div class="wb-palette__list">
        <button
          v-for="(item, i) in items"
          :key="item.def.id"
          type="button"
          class="wb-palette__item"
          :class="{ on: i === cursor, dis: item.disabled }"
          :disabled="item.disabled"
          @click="$emit('run', item.def.id)"
        >
          <q-icon :name="item.def.icon" size="14px" />
          <span class="wb-palette__title">{{ item.title }}</span>
          <span v-if="item.def.shortcut" class="wb-palette__sc">{{ item.def.shortcut }}</span>
        </button>
        <div v-if="!items.length" class="wb-palette__empty">{{ t('workbench.cmdk.empty') }}</div>
      </div>
    </div>
  </q-dialog>
</template>

<script setup lang="ts">
/**
 * ⌘K 命令面板（P0 基础集；动作全集 P1）。过滤在页面（数据流铁律），
 * 本组件只导航与触发。
 */
import { nextTick, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { WbCommandItem } from '../../features/workbench/commands';

const props = defineProps<{
  open: boolean;
  items: WbCommandItem[];
}>();

const emit = defineEmits<{
  close: [];
  run: [id: WbCommandItem['def']['id']];
  'update:query': [q: string];
}>();

const { t } = useI18n();
const inputEl = ref<HTMLInputElement | null>(null);
const cursor = ref(0);
const query = ref('');

watch(query, (q) => emit('update:query', q));

watch(
  () => props.open,
  async (open) => {
    if (open) {
      cursor.value = 0;
      await nextTick();
      inputEl.value?.focus();
    }
  },
);

watch(
  () => props.items.length,
  () => {
    cursor.value = 0;
  },
);

function move(delta: number): void {
  const n = props.items.length;
  if (!n) return;
  cursor.value = (cursor.value + delta + n) % n;
}

function runFirst(): void {
  const item = props.items[cursor.value];
  if (item && !item.disabled) emit('run', item.def.id);
}
</script>

<style scoped lang="sass">
.wb-palette
  width: 440px
  max-width: 92vw
  margin-top: 12vh
  border: 1px solid var(--wb-line-hairline)
  border-radius: 8px
  background: var(--color-surface-solid, var(--canvas-base))
  box-shadow: 0 18px 44px color-mix(in srgb, var(--color-shadow) 40%, transparent)
.wb-palette__input
  width: 100%
  padding: 10px 14px
  border: none
  border-bottom: 1px solid var(--wb-line-hairline)
  background: transparent
  color: var(--color-text-primary)
  font-size: 13px
  &:focus
    outline: none
.wb-palette__list
  max-height: 300px
  overflow-y: auto
  padding: 4px
.wb-palette__item
  display: flex
  align-items: center
  gap: 8px
  width: 100%
  padding: 7px 10px
  border: none
  border-radius: var(--wb-radius)
  background: transparent
  color: var(--color-text-primary)
  font-size: var(--wb-caption)
  cursor: pointer
  &.on
    background: color-mix(in srgb, var(--wb-flow) 14%, transparent)
  &.dis
    opacity: 0.45
.wb-palette__title
  flex: 1
  text-align: left
.wb-palette__sc
  color: var(--color-text-secondary)
  font-size: 10px
.wb-palette__empty
  padding: 12px
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
  text-align: center
</style>
