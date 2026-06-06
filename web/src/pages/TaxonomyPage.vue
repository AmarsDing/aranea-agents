<template>
  <q-page :class="['app-standard-page app-entity-page app-entity-page--warm taxonomy-page', { 'is-dark': isDark }]">
    <section class="taxonomy-page__hero" aria-label="行业分类管理">
      <AppPageHero
        kicker="Taxonomy"
        title="行业分类"
        subtitle="按行业、部门、职位三层组织 Agent 业务画像。创建 Agent 时仅绑定职位叶子，列表筛选同源读取数据库。"
      >
        <template #actions>
          <div class="taxonomy-actions">
            <q-btn
              outline
              rounded
              color="primary"
              icon="refresh"
              label="刷新"
              :loading="loading"
              @click="() => loadTree()"
            />
            <q-btn color="primary" rounded unelevated icon="add" label="新增行业" @click="openCreate('industry')" />
          </div>
        </template>
      </AppPageHero>
    </section>

    <section class="taxonomy-page__toolbar" aria-label="筛选与统计">
      <AppPageToolbar variant="entity" offset class="taxonomy-toolbar">
        <q-input
          v-model="keyword"
          class="app-page-toolbar__search taxonomy-control"
          dense
          outlined
          clearable
          debounce="200"
          placeholder="搜索行业、部门或职位..."
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <template #actions>
          <div class="taxonomy-stats">
            <div class="app-entity-stat">
              <strong>{{ stats.industries }}</strong
              ><span>行业</span>
            </div>
            <div class="app-entity-stat">
              <strong>{{ stats.departments }}</strong
              ><span>部门</span>
            </div>
            <div class="app-entity-stat">
              <strong>{{ stats.positions }}</strong
              ><span>职位</span>
            </div>
          </div>
          <q-btn-toggle
            v-model="viewMode"
            class="taxonomy-view-toggle"
            no-caps
            rounded
            dense
            toggle-color="primary"
            :options="[
              { label: '树形', value: 'tree', icon: 'account_tree' },
              { label: '卡片', value: 'card', icon: 'dashboard' },
            ]"
          />
          <q-toggle v-model="onlyCustom" class="taxonomy-toolbar__toggle" color="primary" label="仅看自建" />
        </template>
      </AppPageToolbar>
    </section>

    <section class="taxonomy-page__content" aria-label="分类列表">
      <q-card v-if="loading" flat bordered class="app-entity-glass-panel taxonomy-tree-shell q-mt-lg">
        <q-card-section>
          <q-skeleton type="text" width="32%" />
          <q-skeleton class="q-mt-md" height="240px" />
        </q-card-section>
      </q-card>

      <q-card
        v-else-if="filteredTree.length === 0"
        flat
        bordered
        class="app-entity-glass-panel taxonomy-empty q-mt-lg"
      >
        <q-card-section class="column items-center text-center">
          <div class="taxonomy-empty__visual"><q-icon name="account_tree" size="44px" color="primary" /></div>
          <div class="text-h6 q-mt-md">暂无匹配分类</div>
          <div class="text-body2 text-grey-7 q-mt-sm">创建第一个行业，再添加部门与职位。</div>
          <q-btn
            class="q-mt-md"
            color="primary"
            rounded
            unelevated
            icon="add"
            label="新增行业"
            @click="openCreate('industry')"
          />
        </q-card-section>
      </q-card>

      <div v-else class="taxonomy-grid q-mt-lg">
        <taxonomy-tree
          v-if="viewMode === 'tree'"
          :tree="filteredTree"
          :keyword="keyword"
          :default-expand-all="true"
          :toggling-ids="togglingIds"
          @edit="openEdit"
          @create-child="openCreate"
          @remove="removeNode"
          @toggle-enabled="toggleNodeEnabled"
          @reorder-positions="onReorderPositions"
        />
        <div v-else class="taxonomy-card-grid">
          <taxonomy-industry-card
            v-for="industry in filteredTree"
            :key="industry.id"
            :industry="industry"
            :is-dark="isDark"
            :toggling-ids="togglingIds"
            @edit="openEdit"
            @create-child="openCreate"
            @remove="removeNode"
            @toggle-enabled="toggleNodeEnabled"
          />
        </div>
      </div>
    </section>

    <TaxonomyNodeDialog
      v-model="dialogOpen"
      :editing-id="editingId"
      :form="form"
      :parent-name="parentName"
      :saving="saving"
      @submit="saveNode"
      @refine-error="onRefineError"
    />
  </q-page>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import TaxonomyTree from '../components/agents/TaxonomyTree.vue';
import TaxonomyIndustryCard from '../components/agents/TaxonomyIndustryCard.vue';
import TaxonomyNodeDialog from '../components/agents/TaxonomyNodeDialog.vue';
import { useTaxonomyPage } from '../features/platform/useTaxonomyPage';

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
  togglingIds,
  loadTree,
  openCreate,
  openEdit,
  saveNode,
  removeNode,
  toggleNodeEnabled,
  onRefineError,
  onReorderPositions,
} = useTaxonomyPage();

const viewMode = ref<'tree' | 'card'>('tree');
</script>
