<template>
  <article v-liquid-glow :class="['taxonomy-industry-card', { 'is-dark': isDark, 'is-disabled': !industry.enabled }]">
    <!-- Header: monogram + title + status -->
    <header class="taxonomy-industry-card__head">
      <div class="taxonomy-industry-card__mono" :style="{ background: monoBg }" :aria-label="`行业 ${industry.name}`">
        <span>{{ monoLetters }}</span>
      </div>
      <div class="taxonomy-industry-card__title">
        <h3 class="taxonomy-industry-card__name">{{ industry.name }}</h3>
        <div class="taxonomy-industry-card__key">
          <span class="taxonomy-industry-card__key-prefix">key</span>
          <span class="app-mono">{{ industry.key }}</span>
        </div>
      </div>
      <span :class="['taxonomy-industry-card__status', industry.enabled ? 'is-on' : 'is-off']">
        <span class="taxonomy-industry-card__status-dot" />
        {{ industry.enabled ? '已启用' : '已停用' }}
      </span>
    </header>

    <!-- Description -->
    <p v-if="description" class="taxonomy-industry-card__desc">{{ description }}</p>

    <hr class="taxonomy-industry-card__divider" />

    <!-- Metrics -->
    <div class="taxonomy-industry-card__metrics">
      <div class="taxonomy-industry-card__metric">
        <span class="taxonomy-industry-card__metric-value app-mono">{{ deptCount }}</span>
        <span class="taxonomy-industry-card__metric-label">部门</span>
      </div>
      <div class="taxonomy-industry-card__metric">
        <span class="taxonomy-industry-card__metric-value app-mono">{{ posCount }}</span>
        <span class="taxonomy-industry-card__metric-label">职位</span>
      </div>
      <div class="taxonomy-industry-card__metric">
        <span class="taxonomy-industry-card__metric-value app-mono">{{ isSystem ? '系统' : '自建' }}</span>
        <span class="taxonomy-industry-card__metric-label">来源</span>
      </div>
    </div>

    <!-- Collapsible departments -->
    <div v-if="departments.length" class="taxonomy-industry-card__depts">
      <button type="button" class="taxonomy-industry-card__depts-toggle" @click="deptsExpanded = !deptsExpanded">
        <q-icon :name="deptsExpanded ? 'expand_less' : 'expand_more'" size="18px" />
        <span>{{ deptsExpanded ? '收起部门' : `查看 ${departments.length} 个部门` }}</span>
      </button>
      <div v-if="deptsExpanded" class="taxonomy-industry-card__depts-list">
        <div v-for="dept in departments" :key="dept.id" class="taxonomy-industry-card__dept">
          <div class="taxonomy-industry-card__dept-head">
            <q-icon name="lan" size="14px" />
            <span class="taxonomy-industry-card__dept-name ellipsis">{{ dept.name }}</span>
            <span class="taxonomy-industry-card__dept-count">{{ positionNodes(dept).length }} 职位</span>
            <q-chip dense square size="sm" :class="parseIsSystem(dept) ? 'system-chip' : 'custom-chip'">
              {{ parseIsSystem(dept) ? '系统' : '自建' }}
            </q-chip>
            <q-btn flat dense round size="sm" color="primary" icon="edit" @click.stop="$emit('edit', dept)">
              <q-tooltip>编辑部门</q-tooltip>
            </q-btn>
            <q-btn
              flat
              dense
              round
              size="sm"
              :color="dept.enabled ? 'negative' : 'positive'"
              :icon="dept.enabled ? 'pause' : 'play_arrow'"
              :disable="isToggling(dept.id)"
              @click.stop="$emit('toggle-enabled', dept, !dept.enabled)"
            >
              <q-tooltip>{{ dept.enabled ? '停用部门' : '启用部门' }}</q-tooltip>
            </q-btn>
            <q-btn
              v-if="!parseIsSystem(dept)"
              flat
              dense
              round
              size="sm"
              color="negative"
              icon="delete"
              @click.stop="$emit('remove', dept)"
            >
              <q-tooltip>删除部门</q-tooltip>
            </q-btn>
          </div>
          <div v-if="positionNodes(dept).length" class="taxonomy-industry-card__positions">
            <div v-for="pos in positionNodes(dept)" :key="pos.id" class="taxonomy-industry-card__position">
              <q-icon name="badge" size="12px" />
              <span class="ellipsis">{{ pos.name }}</span>
              <q-chip v-if="!pos.enabled" dense square size="sm" class="taxonomy-industry-card__pos-off">停用</q-chip>
              <q-btn flat dense round size="sm" color="primary" icon="edit" @click.stop="$emit('edit', pos)">
                <q-tooltip>编辑职位</q-tooltip>
              </q-btn>
              <q-btn
                v-if="!parseIsSystem(pos)"
                flat
                dense
                round
                size="sm"
                color="negative"
                icon="delete"
                @click.stop="$emit('remove', pos)"
              >
                <q-tooltip>删除职位</q-tooltip>
              </q-btn>
            </div>
            <q-btn
              flat
              rounded
              color="primary"
              icon="add"
              label="新增职位"
              size="sm"
              class="q-mt-xs"
              @click.stop="$emit('create-child', 'position', dept)"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Footer: actions -->
    <footer class="taxonomy-industry-card__foot">
      <div class="taxonomy-industry-card__action-group">
        <q-btn
          flat
          dense
          round
          size="sm"
          color="primary"
          icon="add"
          @click.stop="$emit('create-child', 'department', industry)"
        >
          <q-tooltip>新增部门</q-tooltip>
        </q-btn>
        <q-btn flat dense round size="sm" color="primary" icon="edit" @click.stop="$emit('edit', industry)">
          <q-tooltip>编辑行业</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          size="sm"
          :color="industry.enabled ? 'negative' : 'positive'"
          :icon="industry.enabled ? 'pause' : 'play_arrow'"
          :disable="isToggling(industry.id)"
          @click.stop="$emit('toggle-enabled', industry, !industry.enabled)"
        >
          <q-tooltip>{{ industry.enabled ? '停用公司' : '启用公司' }}</q-tooltip>
        </q-btn>
        <q-btn
          v-if="!isSystem"
          flat
          dense
          round
          size="sm"
          color="negative"
          icon="delete"
          @click.stop="$emit('remove', industry)"
        >
          <q-tooltip>删除公司</q-tooltip>
        </q-btn>
      </div>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import {
  parseIsSystem,
  trimmedDesc,
  departmentPositions,
  type TaxonomyLevel,
} from '../../features/platform/taxonomyTreeUtils';
import { monoBgForKey, monoLettersForKey } from './monogram';

const props = defineProps<{
  industry: PlatformResourceTreeNode;
  isDark: boolean;
  togglingIds?: Set<string>;
}>();

defineEmits<{
  edit: [node: PlatformResourceTreeNode];
  'create-child': [level: TaxonomyLevel, parent: PlatformResourceTreeNode];
  remove: [node: PlatformResourceTreeNode];
  'toggle-enabled': [node: PlatformResourceTreeNode, enabled: boolean];
}>();

const deptsExpanded = ref(false);

const isSystem = computed(() => parseIsSystem(props.industry));
const description = computed(() => trimmedDesc(props.industry.description));
const departments = computed(() => (props.industry.children ?? []).filter((node) => node.level === 'department'));
const deptCount = computed(() => departments.value.length);
const posCount = computed(() => departments.value.reduce((sum, dept) => sum + positionNodes(dept).length, 0));

const monoBg = computed(() => monoBgForKey(props.industry.key));
const monoLetters = computed(() => monoLettersForKey(props.industry.key, props.industry.name));

function isToggling(id: string) {
  return props.togglingIds?.has(id) ?? false;
}

function positionNodes(dept: PlatformResourceTreeNode) {
  return departmentPositions(dept);
}
</script>

<style lang="sass" scoped>
.taxonomy-industry-card
  position: relative
  display: flex
  flex-direction: column
  padding: 18px 20px 16px
  border: 1px solid var(--glass-border)
  border-radius: 18px
  background: var(--glass-surface)
  backdrop-filter: blur(18px) saturate(var(--liquid-saturate))
  box-shadow: var(--glass-inner-highlight)
  transition: transform 340ms cubic-bezier(0.34, 1.4, 0.64, 1), box-shadow 280ms ease, border-color 220ms ease

  // 液态玻璃光学层（与 _liquid-card.sass 同配方；scoped 样式无法复用全局规则，需保持同步）
  &::after
    content: ""
    position: absolute
    inset: 0
    border: 1px solid transparent
    border-radius: inherit
    pointer-events: none
    background-image: linear-gradient(115deg, transparent 32%, var(--liquid-sheen-band) 47%, var(--liquid-sheen-peak) 50%, var(--liquid-sheen-band) 53%, transparent 68%), linear-gradient(135deg, var(--liquid-sheen-static), transparent 42%), linear-gradient(155deg, var(--liquid-rim-hi), var(--liquid-rim-mid) 42%, var(--liquid-rim-lo) 74%, var(--liquid-rim-hi))
    background-size: 280% 100%, 100% 100%, 100% 100%
    background-position: 130% 0, 0 0, 0 0
    background-repeat: no-repeat
    background-clip: padding-box, padding-box, border-box
    opacity: 0.7
    transition: background-position 720ms cubic-bezier(0.22, 0.8, 0.36, 1), opacity 240ms ease, box-shadow 220ms ease

  &:hover::after
    background-position: -40% 0, 0 0, 0 0
    opacity: 1
    // 菜单选中态描边：主题强调色内缘
    box-shadow: inset 0 0 0 1px var(--liquid-accent-edge)

  &.is-dark::after
    background-image: radial-gradient(220px circle at var(--liquid-mx, 50%) var(--liquid-my, -20%), var(--liquid-spot), transparent 62%), linear-gradient(115deg, transparent 32%, var(--liquid-sheen-band) 47%, var(--liquid-sheen-peak) 50%, var(--liquid-sheen-band) 53%, transparent 68%), linear-gradient(135deg, var(--liquid-sheen-static), transparent 42%), linear-gradient(155deg, var(--liquid-rim-hi), var(--liquid-rim-mid) 42%, var(--liquid-rim-lo) 74%, var(--liquid-rim-hi))
    background-size: 100% 100%, 280% 100%, 100% 100%, 100% 100%
    background-position: 0 0, 130% 0, 0 0, 0 0
    background-clip: padding-box, padding-box, padding-box, border-box
    opacity: 0.85

  &.is-dark:hover::after
    background-position: 0 0, -40% 0, 0 0, 0 0
    opacity: 1
    box-shadow: inset 0 0 0 1px var(--liquid-accent-edge)

  &.is-disabled::after
    opacity: 0

  &:hover
    transform: translateY(-2px)
    border-color: color-mix(in srgb, var(--color-accent) 30%, var(--glass-border))
    box-shadow: 0 8px 24px rgba(93, 64, 55, 0.08), var(--glass-inner-highlight)

  &.is-disabled
    background: color-mix(in srgb, var(--glass-surface) 60%, var(--color-page-tint))

    .taxonomy-industry-card__name,
    .taxonomy-industry-card__metric-value
      color: var(--color-text-tertiary)

    .taxonomy-industry-card__desc,
    .taxonomy-industry-card__dept-name,
    .taxonomy-industry-card__position
      color: var(--color-text-tertiary)

    .taxonomy-industry-card__mono
      opacity: 0.6

/* ── Header ── */
.taxonomy-industry-card__head
  display: flex
  align-items: flex-start
  gap: 12px
  margin-bottom: 12px

.taxonomy-industry-card__mono
  width: 40px
  height: 40px
  flex-shrink: 0
  border-radius: 10px
  display: grid
  place-items: center
  font-weight: 700
  font-size: 14px
  letter-spacing: 0.02em
  color: var(--color-on-accent)
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.3), 0 2px 6px rgba(0, 0, 0, 0.06)
  user-select: none

.taxonomy-industry-card__title
  flex: 1
  min-width: 0

.taxonomy-industry-card__name
  margin: 0
  font-size: 15px
  font-weight: 600
  letter-spacing: -0.01em
  color: var(--color-text-primary)

.taxonomy-industry-card__key
  font-size: 11.5px
  color: var(--color-text-tertiary)
  margin-top: 2px
  display: flex
  gap: 4px

.taxonomy-industry-card__key-prefix
  color: var(--color-text-tertiary)

/* ── Status badge ── */
.taxonomy-industry-card__status
  flex-shrink: 0
  display: flex
  align-items: center
  gap: 4px
  font-size: 11.5px
  padding: 3px 8px
  border-radius: 999px
  white-space: nowrap

  &.is-on
    background: var(--color-success-soft)
    color: var(--color-accent-green)

  &.is-off
    background: rgba(229, 92, 92, 0.1)
    color: var(--color-danger)

.taxonomy-industry-card__status-dot
  width: 6px
  height: 6px
  border-radius: 50%
  background: currentColor

/* ── Description ── */
.taxonomy-industry-card__desc
  font-size: 12.5px
  color: var(--color-text-secondary)
  margin: 0 0 14px
  min-height: 36px
  line-height: 1.5
  display: -webkit-box
  -webkit-line-clamp: 2
  -webkit-box-orient: vertical
  overflow: hidden

/* ── Divider ── */
.taxonomy-industry-card__divider
  border: 0
  border-top: 1px solid var(--color-border-soft)
  margin: 0 0 12px

/* ── Metrics ── */
.taxonomy-industry-card__metrics
  display: grid
  grid-template-columns: repeat(3, 1fr)
  gap: 4px
  margin-bottom: 12px

.taxonomy-industry-card__metric
  display: flex
  flex-direction: column
  gap: 2px

.taxonomy-industry-card__metric-value
  font-size: 16px
  font-weight: 600
  letter-spacing: -0.01em
  line-height: 1.1
  color: var(--color-text-primary)

.taxonomy-industry-card__metric-label
  font-size: 10.5px
  color: var(--color-text-tertiary)
  letter-spacing: 0.02em

/* ── Departments (collapsible) ── */
.taxonomy-industry-card__depts
  margin-bottom: 12px

.taxonomy-industry-card__depts-toggle
  display: inline-flex
  align-items: center
  gap: 4px
  padding: 5px 10px
  border-radius: 8px
  font-size: 12px
  font-weight: 500
  background: transparent
  border: 0
  cursor: pointer
  color: var(--color-text-secondary)
  transition: background 0.12s, color 0.12s

  &:hover
    background: var(--interaction-surface-hover)
    color: var(--color-text-primary)

.taxonomy-industry-card__depts-list
  margin-top: 8px
  display: flex
  flex-direction: column
  gap: 6px

.taxonomy-industry-card__dept
  border: 1px solid var(--glass-border)
  border-radius: 12px
  background: color-mix(in srgb, var(--color-page-tint) 20%, var(--glass-surface))
  overflow: hidden

.taxonomy-industry-card__dept-head
  display: flex
  align-items: center
  gap: 6px
  padding: 5px 10px
  font-size: 12.5px
  font-weight: 600
  color: var(--color-text-primary)

.taxonomy-industry-card__dept-name
  flex: 1
  min-width: 0

.taxonomy-industry-card__dept-count
  font-size: 11px
  color: var(--color-text-tertiary)
  white-space: nowrap

.taxonomy-industry-card__positions
  padding: 2px 10px 6px 26px
  display: flex
  flex-direction: column
  gap: 2px

.taxonomy-industry-card__position
  display: flex
  align-items: center
  gap: 5px
  font-size: 12px
  color: var(--color-text-secondary)

.taxonomy-industry-card__pos-off
  background: color-mix(in srgb, var(--color-negative) 12%, transparent)
  color: var(--color-negative)

/* ── Footer ── */
.taxonomy-industry-card__foot
  display: flex
  align-items: center
  justify-content: space-between
  padding-top: 10px
  border-top: 1px solid var(--color-border-soft)

.taxonomy-industry-card__action-group
  display: flex
  align-items: center
  gap: 2px

/* ── Dark mode ── */
body.body--dark .taxonomy-industry-card
  background: color-mix(in srgb, var(--glass-elevated) 60%, transparent)

  .taxonomy-industry-card__dept
    background: color-mix(in srgb, var(--color-page-tint) 20%, var(--glass-elevated))

  .taxonomy-industry-card__status.is-on
    background: rgba(63, 224, 160, 0.12)
    color: var(--color-success, #3FE0A0)

  .taxonomy-industry-card__status.is-off
    background: rgba(255, 94, 122, 0.12)
    color: var(--color-danger, #FF5E7A)

.app-mono
  font-family: 'JetBrains Mono', 'SF Mono', Menlo, monospace
  font-feature-settings: 'tnum' 1
</style>
