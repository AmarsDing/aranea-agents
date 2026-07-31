<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-glass-dialog app-dialog-card--sm">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">{{ t('skillsPage.ecosystemPublishTitle') }}</div>
        <q-btn v-close-popup flat round dense icon="close" :disable="loading" />
      </q-card-section>
      <q-card-section class="app-glass-dialog__scroll skill-ecosystem-publish__body">
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
      </q-card-section>
      <q-card-actions align="right">
        <q-btn v-close-popup flat rounded no-caps :label="t('common.cancel')" :disable="loading" />
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
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { Skill } from '../../features/skills/types';

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
.skill-ecosystem-publish__body
  padding: 16px 18px

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
  border-radius: 10px
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
