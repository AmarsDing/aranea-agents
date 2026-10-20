<template>
  <transition name="kb-bulk" appear>
    <div v-if="count > 0" class="kb-bulk-bar" role="toolbar" :aria-label="t('knowledgePage.workbench.bulk.barAria')">
      <span class="kb-bulk-bar__count ellipsis" data-test="bulk-count">
        {{ t('knowledgePage.workbench.bulk.selectedCount', { n: count }) }}
      </span>
      <div class="kb-bulk-bar__actions">
        <q-btn
          dense
          flat
          no-caps
          size="sm"
          class="kb-bulk-bar__btn"
          icon="drive_file_move"
          :label="t('knowledgePage.workbench.bulk.move')"
          data-test="bulk-move"
          @click="openMove"
        />
        <q-btn
          dense
          flat
          no-caps
          size="sm"
          class="kb-bulk-bar__btn"
          icon="psychology"
          :label="t('knowledgePage.workbench.bulk.reembed')"
          data-test="bulk-reembed"
          :disable="!hasSemantic"
          @click="$emit('reembed')"
        >
          <q-tooltip v-if="!hasSemantic">{{ t('knowledgePage.reembedNoSemantic') }}</q-tooltip>
        </q-btn>
        <q-btn
          dense
          flat
          no-caps
          size="sm"
          class="kb-bulk-bar__btn kb-bulk-bar__btn--danger"
          icon="delete_outline"
          :label="t('knowledgePage.workbench.bulk.delete')"
          data-test="bulk-delete"
          @click="$emit('delete')"
        />
        <q-btn
          dense
          flat
          round
          size="xs"
          class="kb-bulk-bar__close"
          icon="close"
          :title="t('knowledgePage.workbench.bulk.cancel')"
          data-test="bulk-clear"
          @click="$emit('clear')"
        />
      </div>
    </div>
  </transition>

  <!-- 移动目标选择：KnowledgeMoveDialog 为单文档口径，批量侧栏循环逐文档提交（由外层 composable 汇总 toast）。 -->
  <KnowledgeMoveDialog
    v-model:open="moveOpen"
    v-model:target-id="moveTargetId"
    :doc-source="t('knowledgePage.workbench.bulk.selectedDocs', { n: count })"
    :options="moveOptions"
    :loading="false"
    @submit="onMoveSubmit"
  />
</template>

<script setup lang="ts">
// U0-4 批量操作条：选中 >0 浮现；移动/删除/重嵌入/取消选择。
// 组件本身零状态副作用——动作全部 emit 给侧栏（useBulkDocActions 执行），保证可测与单一数据源。
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import KnowledgeMoveDialog from '../KnowledgeMoveDialog.vue';
import type { MoveTargetOption } from '../../../features/knowledge/useBulkDocActions';

defineProps<{
  /** 当前选中数（>0 时浮现） */
  count: number;
  /** 当前库有语义层（embedding_model 非空）；否则重嵌入置灰（与行菜单同口径） */
  hasSemantic: boolean;
  /** 移动目标库选项（moveTargetsFor 计算结果） */
  moveOptions: MoveTargetOption[];
}>();

const emit = defineEmits<{
  /** 确认移动（targetId 已在对话框中选定） */
  move: [targetId: string];
  delete: [];
  reembed: [];
  clear: [];
}>();

const { t } = useI18n();

const moveOpen = ref(false);
const moveTargetId = ref('');

function openMove() {
  moveTargetId.value = '';
  moveOpen.value = true;
}

function onMoveSubmit() {
  const target = moveTargetId.value;
  moveOpen.value = false;
  moveTargetId.value = '';
  if (target) emit('move', target);
}
</script>

<style lang="sass" scoped>
.kb-bulk-bar
  display: flex
  align-items: center
  gap: 8px
  margin: 4px
  padding: 6px 10px
  border-radius: var(--kb-radius-glass)
  background: rgba(var(--hud-sky-rgb), 0.10)
  box-shadow: inset 0 0 0 1px var(--kb-accent-cyan)
  color: var(--kb-text-primary)
  font-size: 12px

  &__count
    flex: none
    font-weight: 600
    color: var(--kb-accent-cyan)

  &__actions
    display: flex
    align-items: center
    gap: 4px
    margin-left: auto

  &__btn
    color: var(--kb-text-primary)

    &--danger
      color: var(--kb-danger)

  &__close
    color: var(--kb-text-dim)

    &:hover
      color: var(--kb-accent-cyan)

// 浮入/浮出（与玻璃面板一致的轻动效）
.kb-bulk-enter-active,
.kb-bulk-leave-active
  transition: opacity 0.18s ease, transform 0.18s ease

.kb-bulk-enter-from,
.kb-bulk-leave-to
  opacity: 0
  transform: translateY(6px)
</style>
