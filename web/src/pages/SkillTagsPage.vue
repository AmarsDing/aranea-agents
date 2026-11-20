<template>
  <q-page class="app-standard-page cap-page app-registry-page skill-tags-page">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="$t('common.refresh')"
        :aria-label="$t('common.refresh')"
        :loading="tagsLoading"
        @click="reload"
      />
      <q-btn unelevated dense no-caps color="primary" icon="add" :label="$t('skillTags.create')" @click="openCreate" />
    </Teleport>
    <AppPageToolbar class="skill-tags-filter">
      <q-input
        v-model="search"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="250"
        :label="$t('skillTags.searchLabel')"
        :placeholder="$t('skillTags.searchPlaceholder')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        v-model="sourceFilter"
        class="app-page-toolbar__field"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="$t('skillTags.sourceFilter')"
        :options="sourceOptions"
      />
      <template #actions>
        <span class="skill-tags-stat">
          <q-icon name="sell" size="13px" />
          {{ $t('skillTags.totalChip', { count: filteredTags.length }) }}
        </span>
        <span v-if="orphanCount > 0" class="skill-tags-stat skill-tags-stat--warning">
          <q-icon name="warning_amber" size="13px" />
          {{ $t('skillTags.orphanChip', { count: orphanCount }) }}
        </span>
      </template>
    </AppPageToolbar>

    <!-- 五态：已有标签但刷新失败 → 局部失败横幅，列表照常渲染 -->
    <AppStatePanel
      v-if="(loadError || forbidden) && filteredTags.length > 0"
      variant="partial"
      dense
      class="q-mb-md"
      :description="loadError"
      :action-label="$t('common.retry')"
      action-icon="refresh"
      @action="reload"
    />

    <!-- 五态：无权限 → 只渲染 forbidden 面板，不提供重试 -->
    <AppStatePanel v-else-if="forbidden" variant="forbidden" class="q-mb-md" />

    <!-- 五态：加载失败且无标签 → 只渲染可重试错误面板，不与空态并存。 -->
    <AppStatePanel
      v-else-if="loadError"
      variant="error"
      class="q-mb-md"
      :description="loadError"
      :action-label="$t('common.retry')"
      action-icon="refresh"
      @action="reload"
    />

    <!-- 五态：首屏加载无标签 → 骨架面板 -->
    <AppStatePanel
      v-if="tagsLoading && !loadError && !forbidden && filteredTags.length === 0"
      variant="loading"
      :rows="4"
    />

    <!-- 五态：成功但无标签 → 空态 -->
    <AppStatePanel
      v-else-if="!tagsLoading && !loadError && !forbidden && filteredGroups.length === 0"
      variant="empty"
      :title="search ? $t('skillTags.emptyTitleSearch') : $t('skillTags.emptyTitle')"
      :description="$t('skillTags.emptyHint')"
      icon="sell"
    />

    <template v-else-if="filteredGroups.length > 0">
      <section v-for="group in filteredGroups" :key="group.dimension" class="skill-tags-group q-mb-md">
        <div class="skill-tags-group__header row items-center q-gutter-sm q-mb-sm">
          <q-icon :name="group.dimension ? 'folder_open' : 'label_important'" color="primary" size="20px" />
          <span class="text-subtitle1 text-weight-medium">{{ group.label }}</span>
          <span class="skill-tags-count">{{ group.items.length }}</span>
        </div>
        <q-card flat class="app-glass-panel">
          <q-list separator>
            <q-item
              v-for="tag in group.items"
              :key="tag.name"
              :class="{ 'skill-tag-row--orphan': tag.source === 'orphan' }"
            >
              <q-item-section>
                <q-item-label class="row items-center q-gutter-xs">
                  <span class="skill-tag-pill" :class="{ 'skill-tag-pill--system': tag.source === 'system' }">
                    {{ tag.name }}
                  </span>
                  <span v-if="tag.source === 'system'" class="skill-tag-badge">
                    {{ $t('skillTags.badgeSystem') }}
                  </span>
                  <span v-else-if="tag.source === 'orphan'" class="skill-tag-badge skill-tag-badge--warning">
                    {{ $t('skillTags.badgeOrphan') }}
                  </span>
                </q-item-label>
                <q-item-label v-if="tag.source === 'orphan'" caption class="q-mt-xs">
                  <span class="skill-tag-row__orphan-hint">{{ $t('skillTags.orphanHint') }}</span>
                </q-item-label>
              </q-item-section>
              <q-item-section side class="skill-tag-row__usage">
                <span class="skill-tag-row__usage-text">
                  {{ $t('skillTags.usageCount', { count: tag.used_count }) }}
                </span>
              </q-item-section>
              <q-item-section side>
                <div class="row q-gutter-xs">
                  <q-btn
                    v-if="tag.source === 'orphan'"
                    flat
                    dense
                    round
                    icon="library_add"
                    color="positive"
                    @click="onAdopt(tag)"
                  >
                    <q-tooltip>{{ $t('skillTags.adoptTooltip') }}</q-tooltip>
                  </q-btn>
                  <q-btn flat dense round icon="edit" color="primary" @click="openRename(tag)">
                    <q-tooltip>{{ $t('skillTags.renameTooltip') }}</q-tooltip>
                  </q-btn>
                  <q-btn flat dense round icon="delete" color="negative" @click="onDelete(tag)">
                    <q-tooltip>{{ $t('skillTags.deleteTooltip') }}</q-tooltip>
                  </q-btn>
                </div>
              </q-item-section>
            </q-item>
          </q-list>
        </q-card>
      </section>
    </template>

    <!-- 新建标签 -->
    <AppDialog v-model="createOpen" persistent size="sm" :title="$t('skillTags.createDialogTitle')">
      <q-input
        v-model="createName"
        dense
        outlined
        autofocus
        :label="$t('skillTags.nameLabel')"
        :hint="$t('skillTags.nameHint')"
        :error="!!createError"
        :error-message="createError"
        @keyup.enter="onCreate"
      />
      <template #actions>
        <q-btn flat no-caps :label="$t('common.cancel')" :disable="creating" @click="createOpen = false" />
        <q-btn color="primary" unelevated no-caps :label="$t('common.create')" :loading="creating" @click="onCreate" />
      </template>
    </AppDialog>

    <!-- 改名 -->
    <AppDialog v-model="renameOpen" persistent size="sm" :title="$t('skillTags.renameDialogTitle')">
      <div class="text-body2 text-grey-7 q-mb-sm">
        {{ $t('skillTags.renameDesc', { name: renameTarget?.name ?? '' }) }}
      </div>
      <q-input
        v-model="renameName"
        dense
        outlined
        autofocus
        :label="$t('skillTags.newNameLabel')"
        :hint="$t('skillTags.newNameHint')"
        :error="!!renameError"
        :error-message="renameError"
        @keyup.enter="onRename"
      />
      <template #actions>
        <q-btn flat no-caps :label="$t('common.cancel')" :disable="renaming" @click="renameOpen = false" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="$t('skillTags.renameAction')"
          :loading="renaming"
          @click="onRename"
        />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import { useSkillTagsPage } from '../features/skills/useSkillTagsPage';

const {
  search,
  sourceFilter,
  sourceOptions,
  tagsLoading,
  loadError,
  forbidden,
  filteredTags,
  orphanCount,
  filteredGroups,
  createOpen,
  createName,
  createError,
  creating,
  renameOpen,
  renameTarget,
  renameName,
  renameError,
  renaming,
  reload,
  openCreate,
  onCreate,
  onAdopt,
  openRename,
  onRename,
  onDelete,
} = useSkillTagsPage();
</script>

<style scoped lang="sass">
.skill-tags-filter
  margin-bottom: 16px

.skill-tags-dialog-card
  min-width: 420px

// ── 工具栏统计 pill（中性玻璃 + 强调图标）──
.skill-tags-stat
  display: inline-flex
  align-items: center
  gap: 5px
  padding: 4px 12px
  border-radius: 999px
  font-size: var(--text-xs)
  font-weight: 600
  line-height: 1.4
  color: var(--color-text-secondary)
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)

  .q-icon
    color: var(--color-accent)

  &--warning
    color: var(--color-warning)
    border-color: color-mix(in srgb, var(--color-warning) 32%, transparent)
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)

    .q-icon
      color: var(--color-warning)

// ── 分组计数 pill ──
.skill-tags-count
  display: inline-flex
  align-items: center
  justify-content: center
  min-width: 22px
  padding: 1px 8px
  border-radius: 999px
  font-size: 11px
  font-weight: 700
  line-height: 1.5
  color: var(--color-text-secondary)
  background: color-mix(in srgb, var(--color-text-secondary) 10%, transparent)

// ── 标签主体 pill：背景锚定主题玻璃色（与卡片同族），强调色只用于文字/描边 ──
// 注意：禁止 color-mix(accent, transparent) 作背景——透明薄纱叠加在玻璃上
// 暗色下会渲染成灰绿浑浊色块，与主题脱节
.skill-tag-pill
  display: inline-flex
  align-items: center
  padding: 3px 10px
  border-radius: 999px
  font-size: var(--text-xs)
  font-weight: 600
  letter-spacing: 0.02em
  line-height: 1.5
  color: var(--color-accent-hover)
  border: 1px solid color-mix(in srgb, var(--color-accent) 38%, var(--glass-border))
  background: var(--glass-elevated)

  &--system
    color: var(--color-text-secondary)
    border-color: var(--glass-border)
    background: transparent

body.body--dark .skill-tag-pill
  color: var(--color-accent)
  border-color: color-mix(in srgb, var(--color-accent) 48%, var(--glass-border))

  &--system
    color: var(--color-text-secondary)
    border-color: var(--glass-border)
    background: transparent

// ── 来源角标 ──
.skill-tag-badge
  display: inline-flex
  align-items: center
  padding: 1px 7px
  border-radius: 999px
  font-size: 10px
  font-weight: 700
  line-height: 1.5
  letter-spacing: 0.04em
  color: var(--color-text-tertiary)
  border: 1px solid var(--glass-border)

  &--warning
    color: var(--color-warning)
    border-color: color-mix(in srgb, var(--color-warning) 30%, transparent)
    background: color-mix(in srgb, var(--color-warning) 8%, transparent)

// ── 行 ──
.skill-tag-row--orphan
  background: var(--color-warning-soft)

.skill-tag-row__orphan-hint
  color: var(--color-warning)

.skill-tag-row__usage
  min-width: 96px

// 注意：必须作用于内层 span——Quasar 对 .q-item__section--side 的颜色规则
// 特异性高于本文件 scoped 类，直接挂在 section 上会被覆盖
.skill-tag-row__usage-text
  font-size: var(--text-sm)
  color: var(--color-text-secondary)
  font-variant-numeric: tabular-nums
</style>
