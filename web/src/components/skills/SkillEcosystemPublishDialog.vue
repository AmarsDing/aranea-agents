<template>
  <AppDialog
    :model-value="modelValue"
    :title="t('skillsPage.ecosystemPublishTitle')"
    size="sm"
    persistent
    :loading="Boolean(loading)"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="skill-ecosystem-publish__skill">
      <q-avatar size="40px" color="primary" text-color="white" icon="psychology" />
      <div class="skill-ecosystem-publish__skill-meta">
        <div class="skill-ecosystem-publish__skill-name">{{ skill?.name }}</div>
        <div class="skill-ecosystem-publish__skill-slug">{{ skill?.slug }}</div>
      </div>
    </div>
    <div v-if="skill?.description" class="skill-ecosystem-publish__desc">{{ skill.description }}</div>
    <div class="skill-ecosystem-publish__note">
      <q-icon name="storefront" size="16px" />
      <span>{{ t('skillsPage.ecosystemPublishMessage') }}</span>
    </div>
    <template #actions>
      <q-btn
        flat
        rounded
        no-caps
        :label="t('common.cancel')"
        :disable="loading"
        @click="emit('update:modelValue', false)"
      />
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        icon="storefront"
        :label="t('skillsPage.ecosystemPublishAction')"
        :loading="loading"
        @click="emit('confirm')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { Skill } from '../../features/skills/types';
import AppDialog from '../common/AppDialog.vue';

defineProps<{
  modelValue: boolean;
  skill: Skill | null;
  loading?: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  confirm: [];
}>();

const { t } = useI18n();
</script>

<style scoped lang="sass">
.skill-ecosystem-publish__skill
  display: flex
  align-items: center
  gap: 12px

.skill-ecosystem-publish__skill-meta
  min-width: 0

.skill-ecosystem-publish__skill-name
  font-size: 15px
  font-weight: 700
  color: var(--color-text-heading)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.skill-ecosystem-publish__skill-slug
  font-size: 12px
  color: var(--color-text-tertiary)
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.skill-ecosystem-publish__desc
  margin-top: 10px
  padding: 10px 12px
  border: 1px solid var(--glass-border)
  border-radius: 8px
  background: color-mix(in srgb, var(--glass-elevated) 72%, transparent)
  font-size: 12px
  line-height: 1.6
  color: var(--color-text-secondary)
  display: -webkit-box
  -webkit-line-clamp: 3
  -webkit-box-orient: vertical
  overflow: hidden

.skill-ecosystem-publish__note
  display: flex
  align-items: flex-start
  gap: 6px
  margin-top: 12px
  font-size: 12px
  line-height: 1.6
  color: var(--color-text-secondary)

  .q-icon
    margin-top: 2px
    color: var(--color-accent)
</style>
