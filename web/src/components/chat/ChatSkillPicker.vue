<template>
  <!-- Skill 选择器：输入框工具条入口按钮 + 弹出面板多选。
       点击卡片 toggle 选中（面板保持打开，可连续多选/取消）；
       catalog 为空时整个入口自隐藏。 -->
  <span v-if="skills.length > 0" class="skill-picker">
    <q-btn
      unelevated
      outline
      color="accent"
      :aria-label="t('chat.skillPickerTooltip')"
      class="composer-btn composer-btn--outline skill-picker__btn"
      :class="{ 'skill-picker__btn--active': selectedSlugs.length > 0 }"
    >
      <q-icon name="auto_awesome" size="22px" />
      <q-badge v-if="selectedSlugs.length > 0" color="accent" floating class="skill-picker__badge">
        {{ selectedSlugs.length }}
      </q-badge>
      <q-tooltip anchor="top middle" self="bottom middle">{{ t('chat.skillPickerTooltip') }}</q-tooltip>

      <q-menu anchor="top middle" self="bottom middle" transition-show="scale" class="skill-picker__menu">
        <div class="skill-picker__panel">
          <div class="skill-picker__header row items-center justify-between no-wrap">
            <span class="skill-picker__header-count">
              {{ t('chat.skillPickerSelected', { count: selectedSlugs.length }) }}
            </span>
            <q-btn
              v-if="selectedSlugs.length > 0"
              flat
              dense
              no-caps
              size="sm"
              color="accent"
              class="skill-picker__clear"
              :label="t('chat.skillPickerClear')"
              @click="emit('clear')"
            />
          </div>
          <div class="skill-picker__list">
            <div
              v-for="skill in skills"
              :key="skill.slug"
              class="skill-picker-card"
              :class="{ 'skill-picker-card--selected': isSelected(skill.slug) }"
              @click="emit('toggle', skill.slug)"
            >
              <q-icon
                :name="isSelected(skill.slug) ? 'check_circle' : 'radio_button_unchecked'"
                size="xs"
                class="skill-picker-card__check"
              />
              <div class="skill-picker-card__body">
                <div class="skill-picker-card__name">{{ skill.name || skill.slug }}</div>
                <div v-if="skill.description" class="skill-picker-card__desc">
                  {{ skill.description }}
                  <q-tooltip v-if="skill.description.length > descTooltipThreshold" :delay="500">
                    {{ skill.description }}
                  </q-tooltip>
                </div>
                <div v-if="skill.tags && skill.tags.length > 0" class="skill-picker-card__tags">
                  <span v-for="tag in skill.tags.slice(0, maxTags)" :key="tag" class="skill-picker-card__tag">
                    {{ tag }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </q-menu>
    </q-btn>
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { SkillCatalogEntry } from '../../features/skills/types';

const maxTags = 3;
const descTooltipThreshold = 80;

const props = defineProps<{
  skills: SkillCatalogEntry[];
  selectedSlugs: string[];
}>();

const emit = defineEmits<{
  toggle: [slug: string];
  clear: [];
}>();

const { t } = useI18n();

function isSelected(slug: string): boolean {
  return props.selectedSlugs.includes(slug);
}
</script>

<style scoped lang="sass">
.skill-picker
  display: inline-flex

.skill-picker__btn--active
  border-color: var(--color-accent)

.skill-picker__panel
  width: 320px
  max-width: 90vw
  background: var(--glass-elevated, var(--glass-surface))
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
  border: 1px solid var(--glass-border)
  border-radius: 12px
  overflow: hidden

.skill-picker__header
  padding: 8px 12px
  border-bottom: 1px solid var(--glass-border)

.skill-picker__header-count
  font-size: 12px
  color: var(--color-text-secondary)

.skill-picker__list
  max-height: 320px
  overflow-y: auto
  padding: 8px
  display: flex
  flex-direction: column
  gap: 6px

.skill-picker-card
  display: flex
  align-items: flex-start
  gap: 8px
  padding: 8px 10px
  border-radius: 8px
  border: 1px solid var(--glass-border)
  cursor: pointer
  transition: background 0.2s ease, border-color 0.2s ease

  &:hover
    background: var(--glass-surface)

.skill-picker-card--selected
  border-color: var(--color-success, var(--color-accent))
  background: var(--glass-surface)

  .skill-picker-card__check
    color: var(--color-success, var(--color-accent))

.skill-picker-card__check
  margin-top: 2px
  color: var(--color-text-secondary)
  flex-shrink: 0

.skill-picker-card__body
  min-width: 0
  flex: 1

.skill-picker-card__name
  font-size: 13px
  font-weight: 500
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis

.skill-picker-card__desc
  font-size: 12px
  color: var(--color-text-secondary)
  line-height: 1.4
  margin-top: 2px
  overflow: hidden
  text-overflow: ellipsis
  display: -webkit-box
  -webkit-line-clamp: 2
  -webkit-box-orient: vertical

.skill-picker-card__tags
  display: flex
  gap: 4px
  margin-top: 4px
  flex-wrap: wrap

.skill-picker-card__tag
  font-size: 10px
  padding: 0 4px
  border-radius: 3px
  background: var(--glass-border)
  color: var(--color-text-secondary)
</style>
