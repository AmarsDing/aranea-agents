<template>
  <q-page :class="['app-standard-page app-entity-page app-entity-page--warm taxonomy-page', { 'is-dark': isDark }]">
    <section class="taxonomy-page__hero" aria-label="组织架构管理">
      <AppPageHero
        kicker="Organization"
        title="组织架构"
        subtitle="按公司、部门、职位三层组织 Agent 业务画像。创建 Agent 时仅绑定职位叶子，列表筛选同源读取数据库。"
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
            <q-btn color="primary" rounded unelevated icon="add" label="新增公司" @click="openCreate('company')" />
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
          placeholder="搜索公司、部门或职位..."
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <template #actions>
          <div class="taxonomy-stats">
            <div class="app-entity-stat">
              <strong>{{ stats.companies }}</strong
              ><span>公司</span>
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

    <section class="taxonomy-page__content" aria-label="组织列表">
      <!-- 五态：无权限 → 只渲染 forbidden 面板，不提供重试。
           403 时缓存的组织树可能来自其他权限范围，优先于局部失败横幅。 -->
      <AppStatePanel v-if="forbidden" variant="forbidden" class="q-mb-md" />

      <!-- 五态：已有组织但刷新失败 → 局部失败横幅，树/卡片照常渲染 -->
      <AppStatePanel
        v-else-if="loadError && filteredTree.length > 0"
        variant="partial"
        dense
        class="q-mb-md"
        :description="loadError"
        action-label="重试"
        action-icon="refresh"
        @action="loadTree()"
      />

      <!-- 五态：加载失败且无组织 → 只渲染可重试错误面板，不与空态并存。 -->
      <AppStatePanel
        v-else-if="loadError"
        variant="error"
        class="q-mb-md"
        :description="loadError"
        action-label="重试"
        action-icon="refresh"
        @action="loadTree()"
      />

      <AppStatePanel
        v-if="loading && filteredTree.length === 0 && !loadError && !forbidden"
        variant="loading"
        :rows="4"
      />

      <AppStatePanel
        v-else-if="!loading && !loadError && !forbidden && filteredTree.length === 0"
        variant="empty"
        title="暂无匹配组织"
        description="创建第一个公司，再添加部门与职位。"
        icon="account_tree"
        action-label="新增公司"
        action-icon="add"
        @action="openCreate('company')"
      />

      <template v-else-if="filteredTree.length > 0 && !forbidden">
        <taxonomy-tree
          v-if="viewMode === 'tree'"
          :tree="filteredTree"
          :keyword="keyword"
          :default-expand-all="false"
          :toggling-ids="togglingIds"
          @edit="openEdit"
          @create-child="openCreate"
          @remove="removeNode"
          @toggle-enabled="toggleNodeEnabled"
          @reorder-positions="onReorderPositions"
        />
        <div v-else class="taxonomy-card-grid">
          <taxonomy-industry-card
            v-for="company in filteredTree"
            :key="company.id"
            :industry="company"
            :is-dark="isDark"
            :toggling-ids="togglingIds"
            @edit="openEdit"
            @create-child="openCreate"
            @remove="removeNode"
            @toggle-enabled="toggleNodeEnabled"
          />
        </div>
      </template>
    </section>

    <TaxonomyNodeDialog
      v-model="dialogOpen"
      v-model:form="form"
      :editing-id="editingId"
      :parent-name="parentName"
      :saving="saving"
      :refine-fn="refinePromptField"
      @submit="saveNode"
      @refine-error="onRefineError"
    />
  </q-page>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import TaxonomyTree from '../components/agents/TaxonomyTree.vue';
import TaxonomyIndustryCard from '../components/agents/TaxonomyIndustryCard.vue';
import TaxonomyNodeDialog from '../components/agents/TaxonomyNodeDialog.vue';
import { refinePromptField } from '../features/agents/aiRefine';
import { useTaxonomyPage } from '../features/platform/useTaxonomyPage';

const {
  isDark,
  loading,
  loadError,
  forbidden,
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
