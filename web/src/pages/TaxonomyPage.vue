<template>
  <q-page :class="['app-standard-page app-entity-page app-entity-page--warm taxonomy-page', { 'is-dark': isDark }]">
    <AppPageHero
      kicker="Agent Type"
      title="Agent 行业分类"
      subtitle="按行业、部门、职位三层组织 Agent 业务画像。创建 Agent 时仅绑定职位叶子，列表筛选同源读取数据库。"
    >
      <template #actions>
        <div class="category-actions">
          <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="() => loadTree()" />
          <q-btn color="primary" rounded unelevated icon="add" label="新增行业" @click="openCreate('industry')" />
        </div>
      </template>
    </AppPageHero>

    <AppPageToolbar variant="entity" offset class="category-toolbar">
      <q-input
        v-model="keyword"
        class="app-page-toolbar__search category-control"
        dense
        outlined
        clearable
        debounce="200"
        placeholder="搜索行业、部门或职位..."
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <template #actions>
        <div class="category-stats">
          <div class="app-entity-stat"><strong>{{ stats.industries }}</strong><span>行业</span></div>
          <div class="app-entity-stat"><strong>{{ stats.departments }}</strong><span>部门</span></div>
          <div class="app-entity-stat"><strong>{{ stats.positions }}</strong><span>职位</span></div>
        </div>
        <q-toggle v-model="onlyCustom" class="category-toolbar__toggle" color="primary" label="仅看自建" />
      </template>
    </AppPageToolbar>

    <q-card v-if="loading" flat bordered class="app-entity-glass-panel taxonomy-tree-shell q-mt-lg">
      <q-card-section>
        <q-skeleton type="text" width="32%" />
        <q-skeleton class="q-mt-md" height="240px" />
      </q-card-section>
    </q-card>

    <q-card v-else-if="filteredTree.length === 0" flat bordered class="app-entity-glass-panel category-empty q-mt-lg">
      <q-card-section class="column items-center text-center">
        <div class="category-empty__visual"><q-icon name="account_tree" size="44px" color="primary" /></div>
        <div class="text-h6 q-mt-md">暂无匹配分类</div>
        <div class="text-body2 text-grey-7 q-mt-sm">创建第一个行业，再添加部门与职位。</div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="新增行业" @click="openCreate('industry')" />
      </q-card-section>
    </q-card>

    <section v-else class="taxonomy-tree-shell q-mt-lg">
      <taxonomy-tree
        :tree="filteredTree"
        :keyword="keyword"
        :toggling-ids="togglingIds"
        @edit="openEdit"
        @create-child="openCreate"
        @remove="removeNode"
        @toggle-enabled="toggleNodeEnabled"
        @reorder="reorderNodes"
      />
    </section>

    <q-dialog v-model="dialogOpen" persistent>
      <q-card class="category-dialog app-dialog-card app-dialog-card--md app-glass-dialog">
        <q-card-section class="app-glass-dialog__head category-dialog__head row items-start justify-between no-wrap">
          <div class="min-width-0">
            <div class="app-glass-dialog__title category-dialog__title">{{ editingId ? "编辑分类" : `新增${levelLabel(form.level)}` }}</div>
            <div class="app-glass-dialog__subtitle category-dialog__subtitle">固定三层结构：行业 → 部门 → 职位</div>
          </div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="app-glass-dialog__body category-dialog__body">
          <div class="app-form-field-grid app-form-field-grid--2col category-dialog__form">
            <q-input v-model.trim="form.name" class="category-control" dense outlined label="名称 *" />
            <q-input v-model.number="form.sort_order" class="category-control" dense outlined type="number" label="排序" />
          </div>

          <div class="category-dialog__meta">
            <div class="category-meta-item">
              <span class="category-meta-item__label">层级</span>
              <span class="category-meta-item__value">{{ levelLabel(form.level) }}</span>
            </div>
            <div class="category-meta-item">
              <span class="category-meta-item__label">父级</span>
              <span class="category-meta-item__value ellipsis">{{ parentName }}</span>
            </div>
          </div>

          <div class="category-dialog__desc">
            <!-- PGO-1-WEB-03: label and placeholder switch by level. -->
            <div class="category-dialog__desc-label">{{ currentDescLabel }}</div>
            <q-input
              v-model="form.description"
              class="category-control category-dialog__desc-input"
              dense
              outlined
              type="textarea"
              :rows="4"
              :placeholder="currentDescPlaceholder"
            />
            <!-- PGO-3-WEB-03: AI Refine button for category description (3 levels). -->
            <div class="row justify-end q-mt-xs">
              <AIRefineButton
                :scope="levelScope(currentLevelNum)"
                :resource-id="editingId || undefined"
                :text="form.description ?? ''"
                flat
                size="sm"
                label="AI 优化描述"
                @apply="(v: string) => { form.description = v }"
                @error="onRefineError"
              />
            </div>
          </div>

          <div class="category-dialog__enabled row items-center q-mt-md">
            <q-toggle v-model="form.enabled" color="primary" label="启用" />
            <span class="category-dialog__enabled-hint text-caption text-grey-7 q-ml-sm">
              停用后 Agent / Team 筛选与分组仍会保留数据，但默认不再出现在选择器中。
            </span>
          </div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions category-dialog__actions">
          <q-btn flat rounded no-caps label="取消" v-close-popup />
          <q-btn color="primary" rounded unelevated no-caps label="保存" :loading="saving" @click="saveNode" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useQuasar } from "quasar";
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import TaxonomyTree from "../components/agents/TaxonomyTree.vue";
import AiRefineButton from "../components/agents/AIRefineButton.vue";
import { useTaxonomyPage } from "../features/platform/useTaxonomyPage";
import { descriptionLabel, descriptionPlaceholder, levelScope, parseLevelNumber } from "../features/platform/taxonomyLabels";

const {
  isDark,
  loading,
  saving,
  togglingIds,
  keyword,
  onlyCustom,
  dialogOpen,
  editingId,
  form,
  filteredTree,
  stats,
  parentName,
  loadTree,
  openCreate,
  openEdit,
  saveNode,
  removeNode,
  toggleNodeEnabled,
  reorderNodes,
  levelLabel
} = useTaxonomyPage();

const $q = useQuasar();

function onRefineError(msg: string) {
  $q.notify({ type: 'negative', message: msg });
}

// PGO-1-WEB-03: dynamic labels based on category level.
const currentLevelNum = computed(() => parseLevelNumber(form.level));
const currentDescLabel = computed(() => descriptionLabel(currentLevelNum.value));
const currentDescPlaceholder = computed(() => descriptionPlaceholder(currentLevelNum.value));
</script>
