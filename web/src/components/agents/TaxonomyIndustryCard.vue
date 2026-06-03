<template>
  <q-card
    flat
    bordered
    :class="['taxonomy-industry-card', { 'is-dark': isDark, 'taxonomy-industry-card--disabled': !industry.enabled }]"
  >
    <div class="taxonomy-industry-card__inner">
      <header class="taxonomy-industry-card__head">
        <div class="taxonomy-industry-card__head-main min-width-0">
          <div class="taxonomy-industry-card__title-row">
            <q-avatar
              rounded
              color="primary"
              text-color="white"
              icon="domain"
              size="36px"
              class="taxonomy-industry-card__avatar"
            />
            <h3 class="taxonomy-industry-card__name ellipsis">{{ industry.name }}</h3>
            <q-chip dense square size="sm" :class="isSystem ? 'system-chip' : 'custom-chip'">
              {{ isSystem ? '系统' : '自建' }}
            </q-chip>
            <q-chip v-if="!industry.enabled" dense square size="sm" class="taxonomy-industry-card__status-off"
              >已停用</q-chip
            >
          </div>
          <p v-if="description" class="taxonomy-industry-card__desc">{{ description }}</p>
        </div>
      </header>

      <div class="taxonomy-industry-card__stats">
        <div class="taxonomy-industry-card__stat">
          <strong>{{ deptCount }}</strong
          ><span>部门</span>
        </div>
        <div class="taxonomy-industry-card__stat">
          <strong>{{ posCount }}</strong
          ><span>职位</span>
        </div>
      </div>

      <div v-if="departments.length" class="taxonomy-industry-card__departments">
        <div v-for="dept in departments" :key="dept.id" class="taxonomy-industry-card__dept">
          <div class="taxonomy-industry-card__dept-head">
            <q-icon name="lan" size="16px" color="primary" />
            <span class="taxonomy-industry-card__dept-name ellipsis">{{ dept.name }}</span>
            <q-chip dense square size="sm" :class="parseIsSystem(dept) ? 'system-chip' : 'custom-chip'">{{
              parseIsSystem(dept) ? '系统' : '自建'
            }}</q-chip>
            <span class="taxonomy-industry-card__dept-count">{{ positionNodes(dept).length }} 职位</span>
          </div>
          <div v-if="positionNodes(dept).length" class="taxonomy-industry-card__positions">
            <div v-for="pos in positionNodes(dept)" :key="pos.id" class="taxonomy-industry-card__position">
              <q-icon name="badge" size="14px" />
              <span class="ellipsis">{{ pos.name }}</span>
              <q-chip v-if="!pos.enabled" dense square size="sm" class="taxonomy-industry-card__pos-off">停用</q-chip>
            </div>
          </div>
        </div>
      </div>

      <footer class="taxonomy-industry-card__foot">
        <div class="taxonomy-industry-card__action-group">
          <q-btn
            flat
            dense
            round
            size="sm"
            color="primary"
            icon="add"
            @click.stop="$emit('createChild', 'department', industry)"
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
            @click.stop="$emit('toggleEnabled', industry, !industry.enabled)"
          >
            <q-tooltip>{{ industry.enabled ? '停用' : '启用' }}</q-tooltip>
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
            <q-tooltip>删除</q-tooltip>
          </q-btn>
        </div>
      </footer>
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import {
  parseIsSystem,
  trimmedDesc,
  departmentPositions,
  type TaxonomyLevel,
} from '../../features/platform/taxonomyTreeUtils';

const props = defineProps<{
  industry: PlatformResourceTreeNode;
  isDark: boolean;
}>();

defineEmits<{
  edit: [node: PlatformResourceTreeNode];
  createChild: [level: TaxonomyLevel, parent: PlatformResourceTreeNode];
  remove: [node: PlatformResourceTreeNode];
  toggleEnabled: [node: PlatformResourceTreeNode, enabled: boolean];
}>();

const isSystem = computed(() => parseIsSystem(props.industry));
const description = computed(() => trimmedDesc(props.industry.description));
const departments = computed(() => (props.industry.children ?? []).filter((node) => node.level === 'department'));
const deptCount = computed(() => departments.value.length);
const posCount = computed(() => departments.value.reduce((sum, dept) => sum + positionNodes(dept).length, 0));

function positionNodes(dept: PlatformResourceTreeNode) {
  return departmentPositions(dept);
}
</script>
