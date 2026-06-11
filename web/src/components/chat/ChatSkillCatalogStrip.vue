<template>
  <div v-if="visibleSkills.length > 0" class="skill-catalog-strip">
    <div class="skill-catalog-strip__scroll row no-wrap q-gutter-x-xs">
      <div
        v-for="skill in visibleSkills"
        :key="skill.slug"
        class="skill-catalog-strip__card"
        :class="{ 'skill-catalog-strip__card--loaded': isLoaded(skill.slug) }"
        @click="onSkillClick(skill)"
      >
        <div class="skill-catalog-strip__card-header">
          <q-icon :name="isLoaded(skill.slug) ? 'check_circle' : 'download'" size="xs" class="q-mr-xs" />
          <span class="skill-catalog-strip__card-name">{{ skill.name || skill.slug }}</span>
        </div>
        <div v-if="skill.description" class="skill-catalog-strip__card-desc">
          {{ truncateDesc(skill.description) }}
        </div>
        <div v-if="skill.tags && skill.tags.length > 0" class="skill-catalog-strip__card-tags">
          <span v-for="tag in visibleTags(skill.tags)" :key="tag" class="skill-catalog-strip__tag">{{ tag }}</span>
        </div>
        <q-tooltip v-if="skill.description && skill.description.length > descMaxLen" :delay="500">{{ skill.description }}</q-tooltip>
      </div>
    </div>
    <q-btn
      v-if="props.skills.length > maxVisible"
      flat
      dense
      size="sm"
      :icon="expanded ? 'expand_less' : 'expand_more'"
      :label="expanded ? '' : `+${props.skills.length - maxVisible}`"
      class="q-ml-xs"
      @click="expanded = !expanded"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { SkillCatalogEntry } from '../../features/skills/types';

const descMaxLen = 80;
const maxTags = 3;

const props = defineProps<{
  skills: SkillCatalogEntry[];
}>();

const expanded = ref(false);
const maxVisible = 8;

const visibleSkills = computed(() =>
  expanded.value ? props.skills : props.skills.slice(0, maxVisible),
);

const loadedSet = ref<Set<string>>(new Set());

function isLoaded(slug: string): boolean {
  return loadedSet.value.has(slug);
}

function truncateDesc(desc: string): string {
  if (desc.length <= descMaxLen) return desc;
  return desc.slice(0, descMaxLen - 3) + '...';
}

function visibleTags(tags: string[]): string[] {
  return tags.slice(0, maxTags);
}

function onSkillClick(skill: SkillCatalogEntry) {
  if (isLoaded(skill.slug)) return;
  loadedSet.value = new Set([...loadedSet.value, skill.slug]);
  emit('loadSkill', skill.slug);
}

const emit = defineEmits<{
  loadSkill: [slug: string];
}>();
</script>

<style scoped>
.skill-catalog-strip {
  display: flex;
  align-items: center;
  padding: 4px 8px;
  min-height: 36px;
  max-height: 96px;
  overflow: hidden;
}

.skill-catalog-strip__scroll {
  overflow-x: auto;
  flex: 1;
  scrollbar-width: thin;
}

.skill-catalog-strip__card {
  display: flex;
  flex-direction: column;
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--glass-surface, rgba(255, 255, 255, 0.06));
  border: 1px solid var(--glass-border, rgba(255, 255, 255, 0.1));
  cursor: pointer;
  flex-shrink: 0;
  min-width: 120px;
  max-width: 200px;
  transition: background 0.2s ease;
}

.skill-catalog-strip__card:hover {
  background: var(--glass-elevated, rgba(255, 255, 255, 0.1));
}

.skill-catalog-strip__card--loaded {
  border-color: var(--color-success);
}

.skill-catalog-strip__card-header {
  display: flex;
  align-items: center;
  gap: 2px;
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.skill-catalog-strip__card-name {
  overflow: hidden;
  text-overflow: ellipsis;
}

.skill-catalog-strip__card-desc {
  font-size: 11px;
  color: var(--color-text-secondary, rgba(255, 255, 255, 0.6));
  line-height: 1.3;
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.skill-catalog-strip__card-tags {
  display: flex;
  gap: 4px;
  margin-top: 2px;
  flex-wrap: wrap;
}

.skill-catalog-strip__tag {
  font-size: 10px;
  padding: 0 4px;
  border-radius: 3px;
  background: var(--glass-border, rgba(255, 255, 255, 0.08));
  color: var(--color-text-secondary, rgba(255, 255, 255, 0.5));
}
</style>
