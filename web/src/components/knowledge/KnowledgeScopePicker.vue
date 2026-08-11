<template>
  <!-- G3-F2 搜索范围选择器（V12.6）：弹出迷你目录树（仅目录单选）。
       G4 抽取为独立组件：3D 图谱操作台复用。 -->
  <q-btn
    flat
    dense
    no-caps
    rounded
    class="knowledge-scope-picker"
    :class="{ 'knowledge-scope-picker--active': scopePrefix !== '' }"
    :aria-label="t('knowledgePage.searchScopeTitle')"
  >
    <q-icon name="filter_alt" size="16px" class="q-mr-xs" />
    <span class="knowledge-scope-picker__label">{{ scopeLabel }}</span>
    <q-icon
      v-if="scopePrefix !== ''"
      name="close"
      size="14px"
      class="q-ml-xs"
      :aria-label="t('knowledgePage.searchScopeClear')"
      @click.stop="$emit('update:scope-prefix', '')"
    />
    <q-menu ref="scopeMenu" class="knowledge-scope-picker__menu" @show="onScopeMenuShow">
      <div class="knowledge-scope-picker__panel">
        <div class="knowledge-scope-picker__title">{{ t('knowledgePage.searchScopeTitle') }}</div>
        <q-tree
          v-if="scopeNodes.length"
          :nodes="scopeNodes"
          node-key="key"
          label-key="label"
          dense
          :selected="scopeSelectedKey"
          :expanded="scopeExpanded"
          @update:selected="onScopeSelect"
          @update:expanded="(keys: string[]) => (scopeExpanded = keys)"
          @lazy-load="(p: VaultLazyLoadPayload) => $emit('scope-lazy-load', p)"
        />
      </div>
    </q-menu>
  </q-btn>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { QMenu } from 'quasar';
import { dirNodeKey, parseVaultTreeKey, vaultNodeKey } from '../../features/knowledge/vaultTreeUi';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../features/knowledge/useVaultExplorer';

const props = defineProps<{
  /** 当前范围（vault 相对目录前缀，带尾斜杠；'' = 全库）。 */
  scopePrefix: string;
  /** 迷你树根节点（当前 vault；目录经 lazy-load 仅目录）。 */
  scopeNodes: VaultQTreeNode[];
}>();

const emit = defineEmits<{
  /** 范围变更（vault 根 = '' 全库；× 清除 = ''）。 */
  'update:scope-prefix': [prefix: string];
  'scope-lazy-load': [payload: VaultLazyLoadPayload];
}>();

const { t } = useI18n();

const scopeMenu = ref<QMenu | null>(null);
/** 迷你树展开节点（菜单打开时默认展开库根）。 */
const scopeExpanded = ref<string[]>([]);

/** 范围按钮标签：全库 / 末段目录名。 */
const scopeLabel = computed(() => {
  if (!props.scopePrefix) return t('knowledgePage.searchScopeAllVault');
  const segs = props.scopePrefix.split('/').filter(Boolean);
  return segs[segs.length - 1] ?? props.scopePrefix;
});

/** 迷你树选中 key：全库 = 库节点；目录 = 目录节点。 */
const scopeSelectedKey = computed<string | null>(() => {
  const vault = props.scopeNodes[0];
  if (!vault) return null;
  return props.scopePrefix ? dirNodeKey(vault.vaultId, props.scopePrefix) : vaultNodeKey(vault.vaultId);
});

function onScopeMenuShow() {
  const vault = props.scopeNodes[0];
  if (!vault) return;
  // UX-002：每次打开都确保库根展开（而非仅首次）。菜单关闭期间 scopeNodes 可能随父级
  // computed 重建，已加载子节点随旧节点对象丢失；重新赋展开数组让 q-tree 重新评估 lazy 节点。
  scopeExpanded.value = [...scopeExpanded.value.filter((k) => k !== vault.key), vault.key];
}

function onScopeSelect(key: string | null) {
  if (!key) return;
  const ref0 = parseVaultTreeKey(key);
  if (!ref0) return;
  emit('update:scope-prefix', ref0.prefix);
  scopeMenu.value?.hide();
}
</script>

<style lang="scss" scoped>
.knowledge-scope-picker {
  margin-right: 4px;
  padding: 0 8px;
  color: var(--color-text-secondary);
  font-size: 12px;

  &:hover {
    color: var(--color-accent);
  }

  &--active {
    color: var(--color-accent);
    background: var(--interaction-surface-hover);
  }

  &__label {
    max-width: 120px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__panel {
    min-width: 220px;
    max-width: 320px;
    max-height: 320px;
    overflow-y: auto;
    padding: 8px;
  }

  &__title {
    padding: 2px 8px 6px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--color-text-secondary);
    text-transform: uppercase;
  }
}
</style>
