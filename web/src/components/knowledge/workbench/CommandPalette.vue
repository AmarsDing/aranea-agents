<template>
  <PaletteModal
    v-if="open"
    :open="open"
    :title="t('knowledgePage.workbench.commandPalette')"
    :icon="mode === 'vaults' ? 'inventory_2' : 'terminal'"
    :placeholder="
      mode === 'vaults'
        ? t('knowledgePage.workbench.palette.pickVault')
        : t('knowledgePage.workbench.palette.placeholder')
    "
    :query="query"
    @close="close"
    @update:query="query = $event"
    @keydown="onKeydown"
  >
    <!-- 命令模式 -->
    <template v-if="mode === 'commands'">
      <template v-if="filtered.length">
        <button
          v-for="(c, i) in filtered"
          :key="c.def.id"
          type="button"
          class="kb-cmd__item"
          :class="{ 'kb-cmd__item--active': i === activeIndex, 'kb-cmd__item--disabled': c.disabled }"
          :disabled="c.disabled"
          @mouseenter="activeIndex = i"
          @click="run(c)"
        >
          <q-icon :name="c.def.icon" size="16px" class="kb-cmd__icon" />
          <span class="kb-cmd__title ellipsis">{{ c.title }}</span>
          <q-icon v-if="isMru(c.def.id)" name="history" size="13px" class="kb-cmd__mru" />
          <kbd v-if="c.def.shortcut" class="kb-cmd__shortcut">{{ c.def.shortcut }}</kbd>
        </button>
      </template>
      <div v-else class="kb-cmd__empty">{{ t('knowledgePage.workbench.palette.noResults') }}</div>
    </template>

    <!-- Vault 二级选择（switch-vault 命令进入） -->
    <template v-else>
      <template v-if="filteredVaults.length">
        <button
          v-for="(v, i) in filteredVaults"
          :key="v.id"
          type="button"
          class="kb-cmd__item"
          :class="{ 'kb-cmd__item--active': i === activeIndex }"
          @mouseenter="activeIndex = i"
          @click="pickVault(v.id)"
        >
          <q-icon :name="v.vault_backend === 'team' ? 'groups' : 'inventory_2'" size="16px" class="kb-cmd__icon" />
          <span class="kb-cmd__title ellipsis">{{ v.name || v.id }}</span>
          <q-icon v-if="v.id === currentVaultId" name="check" size="14px" class="kb-cmd__icon" />
        </button>
      </template>
      <div v-else class="kb-cmd__empty">{{ t('knowledgePage.workbench.palette.noResults') }}</div>
    </template>
  </PaletteModal>
</template>

<script setup lang="ts">
// CommandPalette（⌘K，SP2 §SP2-7）：命令注册表 + 模糊过滤 + vault 二级选择。
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import PaletteModal from './PaletteModal.vue';
import { filterCommands, type CommandId, type CommandItem } from '../../../features/knowledge/commands';
import { instantFilter } from '../../../features/knowledge/instantMatch';
import type { KnowledgeCollection } from '../../../features/knowledge/types';

const props = defineProps<{
  open: boolean;
  /** 命令项（标题已由父组件经 i18n 装配） */
  commands: CommandItem[];
  collections: KnowledgeCollection[];
  currentVaultId: string;
  /** P2-6：MRU 命令 id（近→远），空查询时置顶并显示 history 角标 */
  mru?: CommandId[];
}>();

const emit = defineEmits<{
  'update:open': [v: boolean];
  run: [id: CommandId];
  'switch-vault': [id: string];
}>();

const { t } = useI18n();

const query = ref('');
const activeIndex = ref(0);
const mode = ref<'commands' | 'vaults'>('commands');

const filtered = computed(() => filterCommands(props.commands, query.value, props.mru ?? []));
const mruSet = computed(() => new Set(props.mru ?? []));
/** MRU 角标：仅空查询（默认列表）时提示，键入后由相关性排序接管。 */
function isMru(id: CommandId) {
  return !query.value.trim() && mruSet.value.has(id);
}
const filteredVaults = computed(() => {
  const q = query.value.trim();
  if (!q) return props.collections;
  return instantFilter(props.collections, q, (c) => [c.name, c.id]);
});

watch(
  () => props.open,
  (on) => {
    if (on) {
      query.value = '';
      activeIndex.value = 0;
      mode.value = 'commands';
    }
  },
);

watch(query, () => {
  activeIndex.value = 0;
});

function close() {
  emit('update:open', false);
}

function run(c: CommandItem) {
  if (c.disabled) return;
  if (c.def.id === 'switch-vault') {
    // 二级选择：进入 vault 列表（不关闭浮层）
    mode.value = 'vaults';
    query.value = '';
    activeIndex.value = 0;
    return;
  }
  emit('run', c.def.id);
  close();
}

function pickVault(id: string) {
  emit('switch-vault', id);
  close();
}

function onKeydown(e: KeyboardEvent) {
  const list = mode.value === 'commands' ? filtered.value : filteredVaults.value;
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    if (list.length) activeIndex.value = (activeIndex.value + 1) % list.length;
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    if (list.length) activeIndex.value = (activeIndex.value - 1 + list.length) % list.length;
  } else if (e.key === 'Enter') {
    e.preventDefault();
    if (mode.value === 'commands') {
      const c = filtered.value[activeIndex.value];
      if (c) run(c);
    } else {
      const v = filteredVaults.value[activeIndex.value];
      if (v) pickVault(v.id);
    }
  } else if (e.key === 'Escape') {
    e.preventDefault();
    if (mode.value === 'vaults') {
      mode.value = 'commands';
      query.value = '';
    } else {
      close();
    }
  }
}
</script>

<style lang="sass" scoped>
.kb-cmd__item
  display: flex
  align-items: center
  gap: 10px
  width: 100%
  padding: 8px 10px
  border: 0
  border-radius: 8px
  background: transparent
  color: var(--kb-text-primary)
  font-size: 13.5px
  text-align: left
  cursor: pointer

  &--active
    background: rgba(79, 216, 255, 0.1)

  &--disabled
    opacity: 0.45
    cursor: default

.kb-cmd__icon
  color: var(--kb-accent-cyan)
  flex: none

.kb-cmd__title
  flex: 1
  min-width: 0

.kb-cmd__mru
  flex: none
  color: var(--kb-text-dim)
  opacity: 0.7

.kb-cmd__shortcut
  flex: none
  font-size: 11px
  font-family: inherit
  padding: 1px 7px
  border-radius: 6px
  color: var(--kb-text-dim)
  border: 1px solid var(--kb-glass-border)
  background: rgba(122, 138, 165, 0.08)

.kb-cmd__empty
  padding: 24px
  text-align: center
  color: var(--kb-text-dim)
  font-size: 13px
</style>
