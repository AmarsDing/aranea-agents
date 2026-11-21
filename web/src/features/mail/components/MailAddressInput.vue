<template>
  <!-- 收件人输入：照旧是逗号分隔的一串；正在输入的那一段按来往记录联想，↑↓ 选、回车 / Tab 填入 -->
  <div class="ml-addr">
    <q-input
      ref="inputRef"
      :model-value="modelValue"
      class="ml-cmp__field"
      borderless
      dense
      :placeholder="placeholder"
      :aria-label="label"
      aria-autocomplete="list"
      :aria-expanded="open"
      @update:model-value="onInput(String($event ?? ''))"
      @keydown="onKey"
      @blur="closeSoon"
    />
    <ul v-if="open" class="ml-addr__list" role="listbox" :aria-label="label">
      <li
        v-for="(c, i) in hits"
        :key="c.address"
        role="option"
        class="ml-addr__opt"
        :class="{ 'ml-addr__opt--on': i === active }"
        :aria-selected="i === active"
        @mousedown.prevent="pick(c)"
      >
        <b>{{ c.name || c.address }}</b>
        <span v-if="c.name" class="ml-num">{{ c.address }}</span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { suggestMailContacts, type MailContactRow } from '../../office/api';

const props = defineProps<{ modelValue: string; placeholder?: string; label: string }>();
const emit = defineEmits<{ 'update:modelValue': [value: string] }>();

const inputRef = ref<{ focus: () => void } | null>(null);
const hits = ref<MailContactRow[]>([]);
const open = ref(false);
const active = ref(0);
let timer: ReturnType<typeof setTimeout> | undefined;
let seq = 0;

/** 正在输入的最后一段（逗号 / 分号之后）。 */
function current(v: string): string {
  return (v.split(/[,，;；]/).pop() ?? '').trim();
}

function onInput(v: string) {
  emit('update:modelValue', v);
  clearTimeout(timer);
  const q = current(v);
  if (q.length < 1) {
    open.value = false;
    return;
  }
  timer = setTimeout(async () => {
    const mine = ++seq;
    try {
      const list = await suggestMailContacts(q, 6);
      if (mine !== seq) return;
      hits.value = list;
      active.value = 0;
      open.value = list.length > 0;
    } catch {
      open.value = false;
    }
  }, 200);
}

function pick(c: MailContactRow) {
  const parts = props.modelValue.split(/[,，;；]/);
  parts.pop();
  const label = c.name ? `${c.name} <${c.address}>` : c.address;
  emit('update:modelValue', [...parts.map((p) => p.trim()).filter(Boolean), label].join(', ') + ', ');
  open.value = false;
  inputRef.value?.focus();
}

function onKey(e: KeyboardEvent) {
  if (!open.value) return;
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault();
    const n = hits.value.length;
    active.value = (active.value + (e.key === 'ArrowDown' ? 1 : n - 1)) % n;
  } else if (e.key === 'Enter' || e.key === 'Tab') {
    const c = hits.value[active.value];
    if (c) {
      e.preventDefault();
      pick(c);
    }
  } else if (e.key === 'Escape') {
    e.stopPropagation();
    open.value = false;
  }
}

function closeSoon() {
  setTimeout(() => (open.value = false), 150);
}

defineExpose({ focus: () => inputRef.value?.focus() });
</script>
