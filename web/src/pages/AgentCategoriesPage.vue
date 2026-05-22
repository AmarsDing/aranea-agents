<template>
  <q-page :class="['app-entity-page app-entity-page--warm agent-categories-page', { 'is-dark': isDark }]">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Agent Type</div>
        <h1 class="app-page-title">Agent 行业分类</h1>
        <p class="app-page-subtitle">按行业、部门、职位三层组织 Agent 业务画像。创建 Agent 时仅绑定职位叶子，列表筛选同源读取数据库。</p>
      </div>
      <div class="category-actions">
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadTree" />
        <q-btn color="primary" rounded unelevated icon="add" label="新增行业" @click="openCreate('industry')" />
      </div>
    </section>

    <q-card flat bordered class="app-entity-glass-panel category-toolbar">
      <q-card-section class="category-toolbar__inner">
        <q-input
          v-model="keyword"
          class="category-toolbar__search category-control"
          dense
          outlined
          clearable
          debounce="200"
          placeholder="搜索行业、部门或职位..."
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <div class="category-toolbar__aside">
          <div class="category-stats">
            <div class="app-entity-stat"><strong>{{ stats.industries }}</strong><span>行业</span></div>
            <div class="app-entity-stat"><strong>{{ stats.departments }}</strong><span>部门</span></div>
            <div class="app-entity-stat"><strong>{{ stats.positions }}</strong><span>职位</span></div>
          </div>
          <q-toggle v-model="onlyCustom" class="category-toolbar__toggle" color="primary" label="仅看自建" />
        </div>
      </q-card-section>
    </q-card>

    <div v-if="loading" class="app-entity-grid category-grid q-mt-lg">
      <q-card v-for="i in 3" :key="i" flat bordered class="app-entity-glass-panel industry-card">
        <q-card-section>
          <q-skeleton type="text" width="42%" />
          <q-skeleton class="q-mt-md" height="80px" />
        </q-card-section>
      </q-card>
    </div>

    <q-card v-else-if="filteredTree.length === 0" flat bordered class="app-entity-glass-panel category-empty q-mt-lg">
      <q-card-section class="column items-center text-center">
        <div class="category-empty__visual"><q-icon name="account_tree" size="44px" color="primary" /></div>
        <div class="text-h6 q-mt-md">暂无匹配分类</div>
        <div class="text-body2 text-grey-7 q-mt-sm">创建第一个行业，再添加部门与职位。</div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="新增行业" @click="openCreate('industry')" />
      </q-card-section>
    </q-card>

    <section v-else class="app-entity-grid category-grid q-mt-lg">
      <q-card v-for="industry in filteredTree" :key="industry.id" flat bordered class="app-entity-glass-panel industry-card">
        <q-card-section class="industry-card__header">
          <div class="row items-start q-gutter-md no-wrap">
            <q-avatar rounded color="primary" text-color="white" icon="domain" class="industry-avatar" />
            <div class="col min-width-0">
              <div class="row items-center q-gutter-sm">
                <div class="text-h6 ellipsis">{{ industry.name }}</div>
                <q-chip dense square :class="isSystem(industry) ? 'system-chip' : 'custom-chip'">{{ isSystem(industry) ? "系统" : "自建" }}</q-chip>
              </div>
              <div v-if="trimmedDesc(industry.description)" class="text-body2 text-grey-8 q-mt-xs category-desc-line">{{ trimmedDesc(industry.description) }}</div>
              <div v-else class="text-caption text-grey-7">暂无行业描述 · 可在编辑中补充</div>
            </div>
          </div>
          <div class="row q-gutter-xs">
            <q-btn flat dense round color="primary" icon="edit" @click="openEdit(industry)" />
            <q-btn flat dense rounded color="primary" icon="add" label="部门" @click="openCreate('department', industry)" />
            <q-btn flat dense round color="negative" icon="delete" @click="removeNode(industry)" />
          </div>
        </q-card-section>

        <q-card-section class="department-list">
          <q-expansion-item
            v-for="department in industry.children"
            :key="department.id"
            default-opened
            expand-icon="keyboard_arrow_down"
            class="department-item"
          >
            <template #header>
              <q-item-section avatar><q-icon name="lan" color="primary" /></q-item-section>
              <q-item-section>
                <q-item-label class="text-weight-bold">{{ department.name }}</q-item-label>
                <q-item-label v-if="trimmedDesc(department.description)" caption class="category-dept-desc">{{ trimmedDesc(department.description) }}</q-item-label>
                <q-item-label v-else caption class="text-grey-6">暂无部门描述</q-item-label>
              </q-item-section>
              <q-item-section side>
                <div class="row q-gutter-xs">
                  <q-btn flat dense round color="primary" icon="edit" @click.stop="openEdit(department)" />
                  <q-btn flat dense rounded color="primary" icon="add" label="职位" @click.stop="openCreate('position', department)" />
                  <q-btn flat dense round color="negative" icon="delete" @click.stop="removeNode(department)" />
                </div>
              </q-item-section>
            </template>

            <div class="position-list">
              <div v-for="position in department.children" :key="position.id" class="position-item">
                <div class="row items-center q-gutter-sm">
                  <q-icon name="badge" color="primary" />
                  <div class="col min-width-0">
                    <div class="text-weight-medium">{{ position.name }}</div>
                    <div class="text-caption text-grey-7 position-path">{{ fullPath(industry, department, position) }}</div>
                    <div v-if="positionDescChain(industry, department, position)" class="text-caption position-desc-chain q-mt-xs">
                      {{ positionDescChain(industry, department, position) }}
                    </div>
                  </div>
                </div>
                <div class="row q-gutter-xs position-item__actions">
                  <q-btn flat dense round color="primary" icon="edit" @click="openEdit(position)" />
                  <q-btn flat dense round color="negative" icon="delete" @click="removeNode(position)" />
                </div>
              </div>
              <q-btn flat rounded color="primary" icon="add" label="新增职位" class="q-mt-sm" @click="openCreate('position', department)" />
            </div>
          </q-expansion-item>

          <q-btn flat rounded color="primary" icon="add" label="新增部门" class="q-mt-sm" @click="openCreate('department', industry)" />
        </q-card-section>
      </q-card>
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
            <div class="category-dialog__desc-label">描述</div>
            <q-input
              v-model="form.description"
              class="category-control category-dialog__desc-input"
              dense
              outlined
              type="textarea"
              :rows="4"
              placeholder="可选，补充该分类的业务说明…"
            />
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
import { useAgentCategoriesPage } from "../features/platform/useAgentCategoriesPage";

const {
  isDark,
  loading,
  saving,
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
  isSystem,
  levelLabel,
  fullPath,
  positionDescChain,
  trimmedDesc
} = useAgentCategoriesPage();
</script>
