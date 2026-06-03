<template>
  <q-popup-proxy
    v-model="open"
    anchor="bottom left"
    self="top left"
    :offset="[0, 4]"
    no-ref-focus
    class="chat-mention-popup-proxy"
  >
    <div class="chat-mention-popup" :class="{ 'chat-mention-popup--dark': isDark }">
      <div class="chat-mention-popup__header text-caption text-weight-medium">
        {{ t('chat.mentionTitle', '引用上下文') }}
      </div>
      <q-input
        v-model="filter"
        dense
        outlined
        :placeholder="t('chat.mentionSearch', '搜索…')"
        class="chat-mention-popup__search"
        autofocus
      />
      <q-list dense class="chat-mention-popup__list">
        <q-item
          v-for="item in filteredItems"
          :key="item.key"
          v-ripple
          clickable
          class="chat-mention-popup__item"
          @click="onSelect(item)"
        >
          <q-item-section side>
            <q-icon :name="item.icon" size="18px" :color="item.iconColor" />
          </q-item-section>
          <q-item-section>
            <q-item-label class="text-weight-medium">{{ item.label }}</q-item-label>
            <q-item-label caption>{{ item.description }}</q-item-label>
          </q-item-section>
        </q-item>
        <q-item v-if="filteredItems.length === 0" class="chat-mention-popup__empty">
          <q-item-section>
            <q-item-label caption>{{ t('chat.mentionEmpty', '无匹配项') }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </div>
  </q-popup-proxy>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ContextRefItem } from '../../features/chat/types';

const props = withDefaults(
  defineProps<{
    open: boolean;
    items: ContextRefItem[];
    isDark?: boolean;
  }>(),
  { isDark: false },
);

const emit = defineEmits<{
  'update:open': [value: boolean];
  select: [item: ContextRefItem];
}>();

const { t } = useI18n();
const filter = ref('');

const filteredItems = computed(() => {
  const q = filter.value.trim().toLowerCase();
  if (!q) return props.items;
  return props.items.filter(
    (item) =>
      item.label.toLowerCase().includes(q) ||
      item.description.toLowerCase().includes(q) ||
      item.key.toLowerCase().includes(q),
  );
});

function onSelect(item: ContextRefItem) {
  emit('select', item);
  emit('update:open', false);
  filter.value = '';
}
</script>

<style scoped>
.chat-mention-popup {
  min-width: 260px;
  max-width: 320px;
  max-height: 320px;
  padding: 8px;
  border-radius: 12px;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  border: 1px solid var(--glass-border);
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.chat-mention-popup__header {
  padding: 4px 8px;
  color: var(--color-text-secondary);
  font-size: 11px;
}

.chat-mention-popup__search {
  margin: 4px 0;
}

.chat-mention-popup__list {
  overflow-y: auto;
  flex: 1;
  min-height: 0;
}

.chat-mention-popup__item {
  border-radius: 6px;
  padding: 6px 8px;
  min-height: 36px;
}

.chat-mention-popup__empty {
  padding: 12px 8px;
}
</style>
