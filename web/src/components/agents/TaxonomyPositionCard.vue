<template>
  <article
    class="position-card app-entity-glass-panel"
    :class="{ 'position-card--highlight': highlight, 'position-card--disabled': !position.enabled }"
  >
    <div class="position-card__accent" aria-hidden="true" />
    <div class="position-card__body">
      <div class="position-card__head row items-start no-wrap q-gutter-sm">
        <q-avatar rounded color="primary" text-color="white" icon="badge" size="36px" class="position-card__avatar" />
        <div class="col min-width-0">
          <div class="row items-center q-gutter-xs no-wrap">
            <span class="position-card__title ellipsis">{{ position.name }}</span>
            <q-chip dense square size="sm" :class="isSystem ? 'system-chip' : 'custom-chip'">
              {{ isSystem ? "系统" : "自建" }}
            </q-chip>
            <q-chip v-if="!position.enabled" dense square size="sm" class="position-card__status-off">已停用</q-chip>
          </div>
          <div class="position-card__path ellipsis">{{ path }}</div>
        </div>
      </div>

      <p v-if="description" class="position-card__desc">{{ description }}</p>
      <p v-else class="position-card__desc position-card__desc--empty">暂无职位描述</p>

      <div v-if="!readonly" class="position-card__foot row items-center justify-end q-gutter-xs">
        <q-btn flat dense round color="primary" icon="edit" aria-label="编辑职位" @click="$emit('edit', position)" />
        <q-btn flat dense round color="negative" icon="delete" aria-label="删除职位" @click="$emit('remove', position)" />
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { PlatformResourceTreeNode } from "../../features/platform/types";
import { parseIsSystem, trimmedDesc } from "../../features/platform/taxonomyTreeUtils";

const props = defineProps<{
  position: PlatformResourceTreeNode;
  path: string;
  readonly?: boolean;
  highlight?: boolean;
}>();

defineEmits<{
  edit: [node: PlatformResourceTreeNode];
  remove: [node: PlatformResourceTreeNode];
}>();

const description = computed(() => trimmedDesc(props.position));
const isSystem = computed(() => parseIsSystem(props.position));
</script>
