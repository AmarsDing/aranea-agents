<template>
  <div :class="['code-editor', { 'code-editor--dark': isDark, 'code-editor--readonly': readonly, 'code-editor--error': validationError }]">
    <label v-if="label" class="code-editor__label">{{ label }}</label>
    <div class="code-editor__wrapper">
      <div class="code-editor__line-numbers" ref="lineNumbersEl">
        <span v-for="n in lineCount" :key="n">{{ n }}</span>
      </div>
      <div class="code-editor__editor">
        <pre class="code-editor__highlight" aria-hidden="true"><code v-html="highlightedCode"></code></pre>
        <textarea
          ref="textareaEl"
          class="code-editor__textarea"
          :value="modelValue"
          :placeholder="placeholder"
          :readonly="readonly"
          :rows="rows"
          spellcheck="false"
          autocomplete="off"
          autocorrect="off"
          autocapitalize="off"
          @input="onInput"
          @scroll="syncScroll"
          @keydown="onKeydown"
        />
      </div>
    </div>
    <div v-if="validationError" class="code-editor__error">{{ validationError }}</div>
    <div v-else-if="hint" class="code-editor__hint">{{ hint }}</div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted } from 'vue';
import { useQuasar } from 'quasar';

const props = withDefaults(
  defineProps<{
    modelValue: string;
    language?: string;
    placeholder?: string;
    readonly?: boolean;
    rows?: number;
    label?: string;
    hint?: string;
    rules?: Array<(val: string) => boolean | string>;
  }>(),
  {
    language: 'json',
    placeholder: '',
    readonly: false,
    rows: 8,
    label: '',
    hint: '',
    rules: () => [],
  },
);

const emit = defineEmits<{
  'update:modelValue': [value: string];
}>();

const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);

const textareaEl = ref<HTMLTextAreaElement | null>(null);
const lineNumbersEl = ref<HTMLDivElement | null>(null);

const lineCount = computed(() => {
  const lines = (props.modelValue || '').split('\n').length;
  return Math.max(lines, props.rows);
});

function onInput(e: Event) {
  const val = (e.target as HTMLTextAreaElement).value;
  emit('update:modelValue', val);
}

function syncScroll() {
  if (!textareaEl.value) return;
  const pre = textareaEl.value.previousElementSibling as HTMLElement | null;
  if (pre) {
    pre.scrollTop = textareaEl.value.scrollTop;
    pre.scrollLeft = textareaEl.value.scrollLeft;
  }
  if (lineNumbersEl.value) {
    lineNumbersEl.value.scrollTop = textareaEl.value.scrollTop;
  }
}

function onKeydown(e: KeyboardEvent) {
  const ta = e.target as HTMLTextAreaElement;
  // Tab inserts 2 spaces
  if (e.key === 'Tab') {
    e.preventDefault();
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    const val = ta.value;
    const newVal = val.substring(0, start) + '  ' + val.substring(end);
    ta.value = newVal;
    ta.selectionStart = ta.selectionEnd = start + 2;
    emit('update:modelValue', newVal);
    return;
  }
  // Enter with auto-indent
  if (e.key === 'Enter') {
    e.preventDefault();
    const start = ta.selectionStart;
    const val = ta.value;
    const lineBefore = val.substring(0, start).split('\n').pop() ?? '';
    const indent = lineBefore.match(/^\s*/)?.[0] ?? '';
    // Extra indent after { or [
    const lastChar = val.substring(0, start).trimEnd().slice(-1);
    const extra = lastChar === '{' || lastChar === '[' ? '  ' : '';
    const insert = '\n' + indent + extra;
    const newVal = val.substring(0, start) + insert + val.substring(ta.selectionEnd);
    ta.value = newVal;
    ta.selectionStart = ta.selectionEnd = start + insert.length;
    emit('update:modelValue', newVal);
    return;
  }
  // Bracket matching: auto-close { } [ ] " "
  const pairs: Record<string, string> = { '{': '}', '[': ']', '"': '"' };
  if (pairs[e.key]) {
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    const val = ta.value;
    // If text is selected, wrap it
    if (start !== end) {
      e.preventDefault();
      const selected = val.substring(start, end);
      const newVal = val.substring(0, start) + e.key + selected + pairs[e.key] + val.substring(end);
      ta.value = newVal;
      ta.selectionStart = start + 1;
      ta.selectionEnd = end + 1;
      emit('update:modelValue', newVal);
      return;
    }
    // If next char is the closing bracket, skip over it
    if (val[start] === e.key && e.key === '"') {
      e.preventDefault();
      ta.selectionStart = ta.selectionEnd = start + 1;
      return;
    }
    // Auto-close
    e.preventDefault();
    const newVal = val.substring(0, start) + e.key + pairs[e.key] + val.substring(end);
    ta.value = newVal;
    ta.selectionStart = ta.selectionEnd = start + 1;
    emit('update:modelValue', newVal);
    return;
  }
  // Backspace: delete matching pair if adjacent
  if (e.key === 'Backspace') {
    const start = ta.selectionStart;
    if (start > 0 && start === ta.selectionEnd) {
      const val = ta.value;
      const before = val[start - 1];
      const after = val[start];
      const matchPairs: Record<string, string> = { '{': '}', '[': ']', '"': '"' };
      if (matchPairs[before] && matchPairs[before] === after) {
        e.preventDefault();
        const newVal = val.substring(0, start - 1) + val.substring(start + 1);
        ta.value = newVal;
        ta.selectionStart = ta.selectionEnd = start - 1;
        emit('update:modelValue', newVal);
        return;
      }
    }
  }
}

// --- JSON Syntax Highlighting ---
function escapeHtml(str: string): string {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function highlightJSON(code: string): string {
  if (!code) return '';
  const escaped = escapeHtml(code);
  // Tokenize and highlight JSON
  return escaped.replace(
    /("(?:\\.|[^"\\])*")\s*(:)?|(\b(?:true|false|null)\b)|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g,
    (match, str, colon, bool, num) => {
      if (str) {
        if (colon) {
          // Key
          return `<span class="ce-key">${str}</span>${colon}`;
        }
        // String value
        return `<span class="ce-string">${str}</span>`;
      }
      if (bool) {
        return `<span class="ce-boolean">${bool}</span>`;
      }
      if (num) {
        return `<span class="ce-number">${num}</span>`;
      }
      return match;
    },
  );
}

const highlightedCode = computed(() => highlightJSON(props.modelValue || ''));

const validationError = computed(() => {
  for (const rule of props.rules) {
    const result = rule(props.modelValue);
    if (result !== true) return typeof result === 'string' ? result : '';
  }
  return '';
});

// Sync scroll on value change (e.g. programmatic update)
watch(
  () => props.modelValue,
  () => nextTick(syncScroll),
);
</script>

<style lang="scss">
.code-editor {
  --ce-bg: #fff;
  --ce-text: #1d1d1d;
  --ce-line-bg: #f5f5f5;
  --ce-line-text: #999;
  --ce-border: #c0c0c0;
  --ce-key-color: #0451a5;
  --ce-string-color: #a31515;
  --ce-number-color: #098658;
  --ce-boolean-color: #0000ff;
  --ce-null-color: #0000ff;
  --ce-placeholder: #aaa;

  &--dark {
    --ce-bg: #1e1e1e;
    --ce-text: #d4d4d4;
    --ce-line-bg: #252526;
    --ce-line-text: #858585;
    --ce-border: #3c3c3c;
    --ce-key-color: #9cdcfe;
    --ce-string-color: #ce9178;
    --ce-number-color: #b5cea8;
    --ce-boolean-color: #569cd6;
    --ce-null-color: #569cd6;
    --ce-placeholder: #555;
  }

  &__label {
    display: block;
    font-size: 12px;
    color: var(--ce-line-text);
    margin-bottom: 4px;
    line-height: 1;
  }

  &__wrapper {
    display: flex;
    border: 1px solid var(--ce-border);
    border-radius: 4px;
    overflow: hidden;
    background: var(--ce-bg);
  }

  &__line-numbers {
    flex-shrink: 0;
    width: 36px;
    padding: 6px 4px;
    text-align: right;
    background: var(--ce-line-bg);
    color: var(--ce-line-text);
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
    font-size: 13px;
    line-height: 1.5;
    user-select: none;
    overflow: hidden;

    span {
      display: block;
    }
  }

  &__editor {
    flex: 1;
    position: relative;
    overflow: hidden;
  }

  &__highlight,
  &__textarea {
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
    font-size: 13px;
    line-height: 1.5;
    padding: 6px 8px;
    margin: 0;
    border: none;
    outline: none;
    white-space: pre-wrap;
    word-wrap: break-word;
    overflow-wrap: break-word;
    tab-size: 2;
  }

  &__highlight {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    pointer-events: none;
    overflow: auto;
    color: var(--ce-text);
    z-index: 0;

    code {
      font-family: inherit;
      font-size: inherit;
      line-height: inherit;
    }

    .ce-key {
      color: var(--ce-key-color);
    }
    .ce-string {
      color: var(--ce-string-color);
    }
    .ce-number {
      color: var(--ce-number-color);
    }
    .ce-boolean {
      color: var(--ce-boolean-color);
    }
    .ce-null {
      color: var(--ce-null-color);
    }
  }

  &__textarea {
    position: relative;
    z-index: 1;
    width: 100%;
    display: block;
    resize: vertical;
    background: transparent;
    color: transparent;
    caret-color: var(--ce-text);
    min-height: calc(1.5em * v-bind(rows) + 12px);

    &::placeholder {
      color: var(--ce-placeholder);
    }

    &:focus {
      outline: none;
    }
  }

  &--readonly &__textarea {
    caret-color: transparent;
    cursor: default;
  }

  &__hint {
    font-size: 12px;
    color: var(--ce-line-text);
    margin-top: 2px;
    line-height: 1.3;
  }

  &__error {
    font-size: 12px;
    color: #c10015;
    margin-top: 2px;
    line-height: 1.3;
  }

  &--error &__wrapper {
    border-color: #c10015;
  }

  &--dark &__error {
    color: #ff6b6b;
  }

  &--dark.code-editor--error &__wrapper {
    border-color: #ff6b6b;
  }
}
</style>
