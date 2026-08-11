<template>
  <div class="kb-panel-list">
    <template v-if="groups.length || dangling.length || mentions.length">
      <div v-for="g in groups" :key="g.docId" class="kb-panel-group">
        <button type="button" class="kb-panel-group__head" @click="$emit('open-doc-id', g.docId)">
          <q-icon name="description" size="14px" />
          <span class="ellipsis">{{ g.docName }}</span>
          <span class="kb-panel-group__count">{{ g.items.length }}</span>
        </button>
        <div v-for="(it, i) in g.items" :key="i" class="kb-panel-item kb-panel-item--static" :title="it.context">
          {{ it.context || it.raw_target }}
        </div>
      </div>
      <div v-if="dangling.length" class="kb-panel-group">
        <div class="kb-panel-group__head kb-panel-group__head--dangling">
          <q-icon name="link_off" size="14px" />
          <span>{{ t('knowledgePage.workbench.panels.danglingGroup') }}</span>
        </div>
        <div v-for="d in dangling" :key="d.raw_target" class="kb-panel-item kb-panel-item--dangling">
          <span class="ellipsis">{{ d.raw_target }}</span>
          <span class="kb-panel-group__count">{{ d.ref_count }}</span>
        </div>
      </div>
      <!-- P2-7：未链接提及（纯文本出现但未成链），点击跳来源文档 -->
      <div v-if="mentions.length" class="kb-panel-group">
        <div class="kb-panel-group__head kb-panel-group__head--static">
          <q-icon name="chat_bubble_outline" size="14px" />
          <span>{{ t('knowledgePage.workbench.panels.mentionsGroup') }}</span>
        </div>
        <button
          v-for="m in mentions"
          :key="m.src_doc_id"
          type="button"
          class="kb-panel-item"
          :title="m.snippet"
          @click="$emit('open-doc-id', m.src_doc_id)"
        >
          <span class="ellipsis">{{ m.src_doc_name }}</span>
          <span class="kb-panel-group__count">{{ m.count }}</span>
        </button>
      </div>
    </template>
    <div v-else class="kb-panel-empty">{{ t('knowledgePage.workbench.panels.noBacklinks') }}</div>
  </div>
</template>

<script setup lang="ts">
// 反向链接面板（SP2 §SP2-8）：按来源文档分组的块级反链 + dangling 组 + 未链接提及（P2-7）。
import { useI18n } from 'vue-i18n';
import type { BlockBacklink, DanglingLink, UnlinkedMention } from '../../../features/knowledge/types';

export type BacklinkGroup = { docId: string; docName: string; items: BlockBacklink[] };

withDefaults(
  defineProps<{
    groups: BacklinkGroup[];
    dangling: DanglingLink[];
    /** P2-7：未链接提及 */
    mentions?: UnlinkedMention[];
  }>(),
  { mentions: () => [] },
);

defineEmits<{
  'open-doc-id': [docId: string];
}>();

const { t } = useI18n();
</script>

<style lang="sass" scoped>
@use './panel-shared'
</style>
