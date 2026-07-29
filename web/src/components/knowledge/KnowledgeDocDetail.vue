<template>
  <q-card flat class="app-pane-card knowledge-doc-detail">
    <template v-if="node">
      <!-- 一级密度：摘要卡（无需二次请求，列表数据直接渲染） -->
      <div class="app-pane-card__header knowledge-doc-detail__header">
        <div class="row items-center no-wrap ellipsis">
          <q-icon name="description" size="18px" class="q-mr-xs" />
          <span class="ellipsis" :title="node.name">{{ node.name }}</span>
        </div>
        <div class="row items-center no-wrap q-gutter-xs">
          <q-chip dense size="sm" :color="statusColor(node.status)" text-color="white">{{ node.status }}</q-chip>
          <q-chip v-if="node.doc_type" dense size="sm" outline>{{ node.doc_type }}</q-chip>
        </div>
      </div>

      <div class="app-pane-card__body knowledge-doc-detail__body">
        <q-banner v-if="node.status === 'error' && node.error_message" dense rounded class="app-banner-warning q-mb-sm">
          <div class="text-caption text-weight-medium">{{ t('knowledgePage.detailError') }}</div>
          <div class="text-caption">{{ node.error_message }}</div>
        </q-banner>
        <div v-if="node.tags?.length" class="q-mb-sm">
          <q-chip v-for="tag in node.tags" :key="tag" dense size="sm" outline class="q-mr-xs">{{ tag }}</q-chip>
        </div>
        <div class="knowledge-doc-detail__summary">
          {{ node.summary || t('knowledgePage.detailSummaryEmpty') }}
        </div>
        <div class="row q-gutter-md q-mt-sm text-caption text-grey-7">
          <span>{{ formatKnowledgeDocSize(node.size_bytes) }}</span>
          <span>{{ formatKnowledgeTime(node.updated_at) }}</span>
          <span v-if="node.path" class="ellipsis" :title="node.path">{{ node.path }}</span>
        </div>

        <div class="row q-gutter-xs q-mt-md">
          <q-btn
            flat
            dense
            no-caps
            size="sm"
            color="primary"
            :icon="expanded ? 'expand_less' : 'expand_more'"
            :label="expanded ? t('knowledgePage.detailCollapse') : t('knowledgePage.detailExpand')"
            @click="$emit('toggle-expand')"
          />
          <q-space />
          <q-btn flat dense round size="sm" icon="drive_file_move_outline" @click="$emit('move')">
            <q-tooltip>{{ t('knowledgePage.moveTo') }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" icon="delete_outline" color="negative" @click="$emit('delete')">
            <q-tooltip>{{ t('knowledgePage.detailDeleteAria') }}</q-tooltip>
          </q-btn>
        </div>

        <!-- 二级密度：正文预览 + 关联区 -->
        <q-slide-transition>
          <div v-if="expanded" class="q-mt-md">
            <div class="knowledge-doc-detail__section-title">
              {{ t('knowledgePage.detailPreview') }}
              <q-chip v-if="previewOrganized" dense size="sm" color="positive" text-color="white" class="q-ml-xs">
                {{ t('knowledgePage.previewOrganizedBadge') }}
              </q-chip>
            </div>
            <q-linear-progress v-if="previewLoading" indeterminate color="primary" />
            <pre v-else-if="previewContent" class="knowledge-doc-detail__preview">{{ previewContent }}</pre>
            <div v-else class="text-caption text-grey-6">{{ t('knowledgePage.previewEmpty') }}</div>

            <div class="knowledge-doc-detail__section-title q-mt-md">
              {{ t('knowledgePage.detailLinks') }}
            </div>
            <q-linear-progress v-if="linksLoading" indeterminate color="primary" />
            <q-list v-else-if="links.length" dense separator>
              <q-item
                v-for="(l, i) in links"
                :key="`${l.target_doc_id}-${l.direction}-${i}`"
                clickable
                @click="$emit('navigate', { docId: l.target_doc_id, relPath: l.target_rel_path })"
              >
                <q-item-section avatar>
                  <q-icon :name="l.direction === 'out' ? 'north_east' : 'south_west'" size="16px" />
                </q-item-section>
                <q-item-section>
                  <q-item-label lines="1">{{ l.target_source }}</q-item-label>
                  <q-item-label v-if="l.context" caption lines="1">{{ l.context }}</q-item-label>
                </q-item-section>
                <q-item-section side class="row items-center q-gutter-xs">
                  <span class="text-caption text-grey-6">
                    {{ l.direction === 'out' ? t('knowledgePage.linkDirOut') : t('knowledgePage.linkDirIn') }}
                  </span>
                  <!-- R-3：关联来源类型徽标（显式/实体/语义），避免用户误以为全是可靠关联 -->
                  <q-chip dense size="sm" :color="linkTypeColor(l.link_type)" text-color="white">
                    {{ linkTypeLabel(l.link_type) }}
                  </q-chip>
                </q-item-section>
              </q-item>
            </q-list>
            <div v-else class="text-caption text-grey-6">{{ t('knowledgePage.detailLinksEmpty') }}</div>
          </div>
        </q-slide-transition>
      </div>
    </template>

    <div v-else class="app-registry-empty app-pane-card__body">
      <q-icon name="find_in_page" size="48px" color="grey-6" />
      <div class="text-h6">{{ t('knowledgePage.detailEmptyTitle') }}</div>
      <div class="text-body2">{{ t('knowledgePage.detailEmptyHint') }}</div>
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { KnowledgeLink, VaultTreeNode } from '../../features/knowledge/types';
import { formatKnowledgeDocSize, formatKnowledgeTime, knowledgeStatusColor } from '../../features/knowledge/knowledgeUi';

defineProps<{
  node: VaultTreeNode | null;
  expanded: boolean;
  previewContent: string;
  previewOrganized: boolean;
  previewLoading: boolean;
  links: KnowledgeLink[];
  linksLoading: boolean;
}>();

defineEmits<{
  'toggle-expand': [];
  delete: [];
  move: [];
  navigate: [payload: { docId: string; relPath: string }];
}>();

const { t } = useI18n();
const statusColor = knowledgeStatusColor;

function linkTypeColor(linkType: string): string {
  if (linkType === 'explicit') return 'primary';
  if (linkType === 'entity') return 'deep-purple';
  if (linkType === 'semantic') return 'teal';
  return 'grey';
}

function linkTypeLabel(linkType: string): string {
  if (linkType === 'explicit') return t('knowledgePage.linkTypeExplicit');
  if (linkType === 'entity') return t('knowledgePage.linkTypeEntity');
  if (linkType === 'semantic') return t('knowledgePage.linkTypeSemantic');
  return linkType;
}
</script>

<style lang="scss" scoped>
.knowledge-doc-detail {
  display: flex;
  flex-direction: column;
  min-height: 0;

  &__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  &__body {
    overflow-y: auto;
  }

  &__summary {
    font-size: 13px;
    line-height: 1.6;
    color: var(--q-grey-8, #424242);
    white-space: pre-wrap;
  }

  &__section-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--q-grey-7, #616161);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin-bottom: 6px;
  }

  &__preview {
    margin: 0;
    padding: 10px 12px;
    max-height: 300px;
    overflow: auto;
    font-size: 12px;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-word;
    background: rgba(0, 0, 0, 0.03);
    border-radius: 8px;
  }
}
</style>
