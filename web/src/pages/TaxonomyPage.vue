<template>
  <q-page :class="['app-standard-page app-entity-page app-entity-page--warm taxonomy-page', { 'is-dark': isDark }]">
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
        <q-toggle v-model="onlyCustom" class="taxonomy-toolbar__toggle" color="primary" label="仅看自建" />
      </template>
    </AppPageToolbar>

    <q-card v-if="loading" flat bordered class="app-entity-glass-panel taxonomy-tree-shell q-mt-lg">
      <q-card-section>
        <q-skeleton type="text" width="32%" />
        <q-skeleton class="q-mt-md" height="240px" />
      </q-card-section>
    </q-card>

    <q-card v-else-if="filteredTree.length === 0" flat bordered class="app-entity-glass-panel taxonomy-empty q-mt-lg">
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

    <section v-else class="taxonomy-grid q-mt-lg">
      <taxonomy-industry-card
        v-for="industry in filteredTree"
        :key="industry.id"
        :industry="industry"
        :is-dark="isDark"
        @edit="openEdit"
        @create-child="openCreate"
        @remove="removeNode"
        @toggle-enabled="toggleNodeEnabled"
      />
      <button type="button" class="taxonomy-industry-card-add" @click="openCreate('industry')">
        <q-icon name="add" size="32px" color="primary" />
        <span>新增行业</span>
      </button>
    </section>

    <q-dialog v-model="dialogOpen" persistent>
      <q-card class="taxonomy-dialog app-dialog-card app-dialog-card--md app-glass-dialog">
        <q-card-section class="app-glass-dialog__head taxonomy-dialog__head row items-start justify-between no-wrap">
          <div class="min-width-0">
            <div class="app-glass-dialog__title taxonomy-dialog__title">
              {{ editingId ? '编辑分类' : `新增${levelLabel(form.level)}` }}
            </div>
            <div class="app-glass-dialog__subtitle taxonomy-dialog__subtitle">固定三层结构：行业 → 部门 → 职位</div>
          </div>
          <q-btn v-close-popup flat round dense icon="close" />
        </q-card-section>
        <q-separator />
        <q-card-section class="app-glass-dialog__body taxonomy-dialog__body">
          <div class="app-form-field-grid app-form-field-grid--2col taxonomy-dialog__form">
            <q-input v-model.trim="form.name" class="taxonomy-control" dense outlined label="名称 *" />
            <q-input
              v-model.number="form.sort_order"
              class="taxonomy-control"
              dense
              outlined
              type="number"
              label="排序"
            />
          </div>

          <div class="taxonomy-dialog__meta">
            <div class="taxonomy-meta-item">
              <span class="taxonomy-meta-item__label">层级</span>
              <span class="taxonomy-meta-item__value">{{ levelLabel(form.level) }}</span>
            </div>
            <div class="taxonomy-meta-item">
              <span class="taxonomy-meta-item__label">父级</span>
              <span class="taxonomy-meta-item__value ellipsis">{{ parentName }}</span>
            </div>
          </div>

          <div class="taxonomy-dialog__desc">
            <div class="taxonomy-dialog__desc-label">{{ currentDescLabel }}</div>
            <q-input
              v-model="form.description"
              class="taxonomy-control taxonomy-dialog__desc-input"
              dense
              outlined
              type="textarea"
              :rows="4"
              :placeholder="currentDescPlaceholder"
            />
            <div class="row justify-end q-mt-xs">
              <AiRefineButton
                :scope="levelScope(currentLevelNum)"
                :resource-id="editingId || undefined"
                :text="form.description ?? ''"
                flat
                size="sm"
                label="AI 优化描述"
                @apply="
                  (v: string) => {
                    form.description = v;
                  }
                "
              />
            </div>
          </div>

          <div class="taxonomy-dialog__enabled row items-center q-mt-md">
            <q-toggle v-model="form.enabled" color="primary" label="启用" />
            <span class="taxonomy-dialog__enabled-hint text-caption text-grey-7 q-ml-sm">
              停用后 Agent / Team 筛选与分组仍会保留数据，但默认不再出现在选择器中。
            </span>
          </div>
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions taxonomy-dialog__actions">
          <q-btn v-close-popup flat rounded no-caps label="取消" />
          <q-btn color="primary" rounded unelevated no-caps label="保存" :loading="saving" @click="saveNode" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import TaxonomyIndustryCard from '../components/agents/TaxonomyIndustryCard.vue';
import AiRefineButton from '../components/agents/AIRefineButton.vue';
import { useTaxonomyPage } from '../features/platform/useTaxonomyPage';
import {
  descriptionLabel,
  descriptionPlaceholder,
  levelScope,
  parseLevelNumber,
} from '../features/platform/taxonomyLabels';

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
  toggleNodeEnabled,
  levelLabel,
} = useTaxonomyPage();

const currentLevelNum = computed(() => parseLevelNumber(form.level));
const currentDescLabel = computed(() => descriptionLabel(currentLevelNum.value));
const currentDescPlaceholder = computed(() => descriptionPlaceholder(currentLevelNum.value));
</script>
