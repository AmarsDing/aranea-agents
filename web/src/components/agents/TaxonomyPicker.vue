<template>
  <div class="taxonomy-picker" @click="!disable && (menuOpen = !menuOpen)">
    <q-field
      :model-value="displayLabel"
      :class="['taxonomy-field', controlClass]"
      dense
      outlined
      :label="label"
      :disable="disable"
    >
      <template #control>
        <div
          class="taxonomy-field__hit row items-center full-width cursor-pointer"
          :class="{ 'is-placeholder': !displayLabel }"
        >
          <span class="col ellipsis">{{ displayLabel || placeholder }}</span>
          <q-icon :name="icon" size="18px" color="primary" />
        </div>
      </template>

      <template #append>
        <q-icon
          v-if="clearable && displayLabel"
          name="close"
          size="16px"
          class="cursor-pointer taxonomy-field__clear"
          @click.stop="clearSelection"
        />
      </template>

      <q-menu
        v-model="menuOpen"
        no-parent-event
        anchor="bottom left"
        self="top left"
        fit
        :offset="[0, 6]"
        class="taxonomy-field-menu"
      >
        <q-card flat class="taxonomy-field-menu__card">
          <q-card-section class="q-pb-sm">
            <q-input
              v-model="menuKeyword"
              dense
              outlined
              clearable
              debounce="150"
              placeholder="搜索组织、部门或职位..."
              class="taxonomy-control"
            >
              <template #prepend><q-icon name="search" /></template>
            </q-input>
          </q-card-section>
          <q-separator />
          <q-scroll-area class="taxonomy-field-menu__scroll">
            <div class="q-pa-sm">
              <q-tree
                :nodes="menuNodes"
                node-key="id"
                :expanded="expanded"
                dense
                no-connectors
                @update:expanded="onExpandedUpdate"
              >
                <template #default-header="prop">
                  <div
                    class="app-taxonomy-tree-node row items-center no-wrap full-width"
                    :class="{
                      'app-taxonomy-tree-node--selectable': prop.node.selectable || selectableLevel === 'any',
                      'app-taxonomy-tree-node--selected': modelValue === prop.node.id,
                      'cursor-pointer': prop.node.selectable || selectableLevel === 'any',
                    }"
                    @click.stop="onPick(prop.node)"
                  >
                    <q-icon :name="prop.node.icon" color="primary" size="16px" class="q-mr-sm" />
                    <div class="col min-width-0">
                      <div class="ellipsis">{{ prop.node.label }}</div>
                      <div v-if="captionMode === 'level'" class="app-taxonomy-tree-node__caption">
                        {{ levelLabel(prop.node.level) }}
                      </div>
                      <div v-else-if="prop.node.caption" class="app-taxonomy-tree-node__caption ellipsis">
                        {{ prop.node.caption }}
                      </div>
                    </div>
                    <q-icon v-if="modelValue === prop.node.id" name="check_circle" color="primary" size="18px" />
                  </div>
                </template>
              </q-tree>
              <div v-if="menuNodes.length === 0" class="text-caption text-grey-7 q-pa-md text-center">
                {{ tree.length === 0 ? $t('agentsPage.taxonomy.emptyTree') : $t('agentsPage.taxonomy.noMatch') }}
              </div>
            </div>
          </q-scroll-area>
        </q-card>
      </q-menu>
    </q-field>
  </div>
</template>

<script setup lang="ts">
import { toRef } from 'vue';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import { useTaxonomyTreeField } from '../../features/platform/useTaxonomyTreeField';
import type { TaxonomyLevel } from '../../features/platform/taxonomyTreeUtils';

const props = withDefaults(
  defineProps<{
    modelValue: string | null;
    tree: PlatformResourceTreeNode[];
    label?: string;
    placeholder?: string;
    disable?: boolean;
    clearable?: boolean;
    /** position：创建 Agent 绑定职位；any：列表按行业/部门/职位筛选 */
    selectableLevel?: TaxonomyLevel | 'any';
    captionMode?: 'level' | 'description';
    icon?: string;
    controlClass?: string;
  }>(),
  {
    label: '组织架构',
    placeholder: '选择组织 / 部门 / 职位',
    disable: false,
    clearable: true,
    selectableLevel: 'position',
    captionMode: 'description',
    icon: 'account_tree',
    controlClass: 'agent-dialog-control',
  },
);

const emit = defineEmits<{
  'update:modelValue': [value: string | null];
}>();

const {
  menuOpen,
  menuKeyword,
  expanded,
  menuNodes,
  displayLabel,
  levelLabel,
  onPick,
  clearSelection,
  onExpandedUpdate,
} = useTaxonomyTreeField({
  modelValue: toRef(props, 'modelValue'),
  tree: toRef(props, 'tree'),
  selectableLevel: toRef(props, 'selectableLevel'),
  onUpdate: (value) => emit('update:modelValue', value),
});
</script>
