<template>
  <div class="chat-entity-group">
    <ChatSectionHeader
      :icon="icon"
      :label="label"
      :count="items.length"
      :collapsed="collapsed"
      @update:collapsed="$emit('update:collapsed', $event)"
    />
    <transition name="chat-group-collapse">
      <div v-show="!collapsed" class="chat-entity-group__items">
        <draggable
          v-model="localItems"
          item-key="id"
          :animation="200"
          :delay="300"
          :delay-on-touch-only="false"
          ghost-class="chat-entity-item--ghost"
          chosen-class="chat-entity-item--chosen"
          drag-class="chat-entity-item--dragging"
          :move="onMove"
          @end="onDragEnd"
        >
          <template #item="{ element: item }">
            <ChatEntityItem
              :name="item.display_name"
              :active="activeId === item.id"
              :status-icon="statusIconFor(item)"
              :status-color="statusColorFor(item)"
              :status-label="statusLabelFor(item)"
              :settings-aria-label="settingsAriaLabel"
              :delete-aria-label="deleteAriaLabel"
              @click="$emit('select', item)"
              @settings="$emit('settings', item.id)"
              @delete="$emit('delete', item.id)"
            />
          </template>
        </draggable>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import draggable from 'vuedraggable';
import ChatSectionHeader from './ChatSectionHeader.vue';
import ChatEntityItem from './ChatEntityItem.vue';
import {
  entityStatusIconFor as statusIconFor,
  entityStatusColorFor as statusColorFor,
  entityStatusLabelFor as statusLabelFor,
} from './chatUi';

export type EntityItem = {
  id: string;
  display_name: string;
  status?: string;
  is_default?: boolean;
};

type DragMoveContext = {
  draggedContext: {
    element: EntityItem;
    index: number;
  };
  relatedContext: {
    element?: EntityItem;
    children: EntityItem[];
    list: EntityItem[];
  };
  draggedElement: EntityItem;
  from: HTMLElement;
  to: HTMLElement;
  newIndex: number;
  oldIndex: number;
};

const props = defineProps<{
  items: EntityItem[];
  label: string;
  icon: string;
  collapsed: boolean;
  activeId?: string | null;
  pinnedId?: string | null;
  settingsAriaLabel?: string;
  deleteAriaLabel?: string;
}>();

const emit = defineEmits<{
  'update:collapsed': [value: boolean];
  select: [item: EntityItem];
  settings: [id: string];
  delete: [id: string];
  reorder: [ids: string[]];
}>();

const localItems = computed({
  get: () => props.items,
  set: (value: EntityItem[]) => {
    emit(
      'reorder',
      value.map((item) => item.id),
    );
  },
});

function onMove(evt: DragMoveContext): boolean {
  if (props.pinnedId && evt.relatedContext?.element?.id === props.pinnedId && evt.newIndex <= 0) {
    return false;
  }
  return true;
}

function onDragEnd() {
  if (props.pinnedId) {
    const current = localItems.value;
    const pinnedIndex = current.findIndex((item) => item.id === props.pinnedId);
    if (pinnedIndex > 0) {
      const [pinned] = current.splice(pinnedIndex, 1);
      current.unshift(pinned);
      emit(
        'reorder',
        current.map((item) => item.id),
      );
    }
  }
}
</script>

<style scoped>
.chat-entity-group {
  margin-bottom: var(--space-3);
}

.chat-entity-group__items {
  overflow: hidden;
}

.chat-group-collapse-enter-active,
.chat-group-collapse-leave-active {
  transition:
    max-height 0.25s ease,
    opacity 0.2s ease;
  max-height: 2000px;
  opacity: 100%;
}

.chat-group-collapse-enter-from,
.chat-group-collapse-leave-to {
  max-height: 0;
  opacity: 0%;
}

.chat-entity-item--ghost {
  opacity: 40%;
  background: var(--glass-surface-hover);
}

.chat-entity-item--chosen {
  box-shadow: 0 4px 12px var(--glass-border, rgb(0 0 0 / 15%));
  transform: translateY(-2px);
}

.chat-entity-item--dragging {
  opacity: 80%;
  cursor: grabbing !important;
}
</style>
