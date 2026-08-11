<template>
  <article
    v-liquid-glow
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
              {{ isSystem ? '系统' : '自建' }}
            </q-chip>
            <q-chip v-if="!position.enabled" dense square size="sm" class="position-card__status-off">已停用</q-chip>
          </div>
          <div class="position-card__path ellipsis">{{ path }}</div>
          <div v-if="variantTags.length" class="position-card__variants row q-gutter-xs q-mt-xs">
            <q-chip v-for="tag in variantTags" :key="tag" dense square size="sm" class="position-card__variant-chip">
              {{ tag }}
            </q-chip>
          </div>
        </div>
        <div v-if="agentCount > 0" class="position-card__agent-count">
          <q-icon name="smart_toy" size="14px" />
          <span>{{ agentCount }}</span>
        </div>
      </div>

      <p v-if="description" class="position-card__desc">{{ description }}</p>
      <p v-else class="position-card__desc position-card__desc--empty">暂无职位描述</p>

      <div v-if="!readonly" class="position-card__foot row items-center justify-end q-gutter-xs">
        <q-btn flat dense round color="primary" icon="edit" aria-label="编辑职位" @click="$emit('edit', position)" />
        <q-btn
          v-if="!isSystem"
          flat
          dense
          round
          color="negative"
          icon="delete"
          aria-label="删除职位"
          @click="$emit('remove', position)"
        />
      </div>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import { parseIsSystem, trimmedDesc } from '../../features/platform/taxonomyTreeUtils';

const props = withDefaults(
  defineProps<{
    position: PlatformResourceTreeNode;
    path: string;
    readonly?: boolean;
    highlight?: boolean;
    agentCount?: number;
  }>(),
  {
    readonly: false,
    highlight: false,
    agentCount: 0,
  },
);

defineEmits<{
  edit: [node: PlatformResourceTreeNode];
  remove: [node: PlatformResourceTreeNode];
}>();

const description = computed(() => trimmedDesc(props.position.description));
const isSystem = computed(() => parseIsSystem(props.position));

const variantTags = computed(() => {
  try {
    const meta = JSON.parse(props.position.metadata_json || '{}');
    const variants: string[] = meta.variants ?? meta.variant_tags ?? [];
    return variants;
  } catch {
    return [];
  }
});
</script>
