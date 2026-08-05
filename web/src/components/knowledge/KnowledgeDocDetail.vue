<template>
  <q-card flat class="app-pane-card knowledge-doc-detail">
    <template v-if="node">
      <div class="app-pane-card__header knowledge-doc-detail__header">
        <div class="row items-center no-wrap ellipsis">
          <q-icon :name="headerIcon" size="18px" class="q-mr-xs" />
          <span class="ellipsis" :title="node.name">{{ node.name }}</span>
        </div>
        <div class="row items-center no-wrap q-gutter-xs">
          <q-chip dense size="sm" :color="statusColor(node.status)" text-color="white">{{
            statusLabel(node.status)
          }}</q-chip>
          <q-chip v-if="node.doc_type" dense size="sm" outline>{{ node.doc_type }}</q-chip>
        </div>
      </div>

      <div class="app-pane-card__body knowledge-doc-detail__body">
        <q-banner v-if="node.status === 'error' && node.error_message" dense rounded class="app-banner-warning q-mb-sm">
          <div class="text-caption text-weight-medium">{{ t('knowledgePage.detailError') }}</div>
          <div class="text-caption">{{ node.error_message }}</div>
        </q-banner>

        <!-- 副标题：路径（V12.4：名称上移标题，路径缩小为副标题） -->
        <div v-if="node.path" class="knowledge-doc-detail__path ellipsis" :title="node.path">{{ node.path }}</div>

        <!-- 第一行：摘要（一行省略）+ hover 360px 大号浮层卡（完整摘要 + 元信息） -->
        <div class="knowledge-doc-detail__summary-hover">
          <div class="knowledge-doc-detail__summary ellipsis">
            {{ node.summary || t('knowledgePage.detailSummaryEmpty') }}
          </div>
          <div class="knowledge-doc-detail__hover-card">
            <div class="knowledge-doc-detail__hover-summary">
              {{ node.summary || t('knowledgePage.detailSummaryEmpty') }}
            </div>
            <div v-if="node.tags?.length" class="q-mt-xs">
              <q-chip v-for="tag in node.tags" :key="tag" dense size="sm" outline class="q-mr-xs">{{ tag }}</q-chip>
            </div>
            <div class="knowledge-doc-detail__hover-meta">
              <div>
                <span>{{ t('knowledgePage.hoverMetaSize') }}</span
                >{{ formatKnowledgeDocSize(node.size_bytes) }}
              </div>
              <div>
                <span>{{ t('knowledgePage.hoverMetaUpdated') }}</span
                >{{ formatKnowledgeTime(node.updated_at) }}
              </div>
              <div v-if="node.path">
                <span>{{ t('knowledgePage.hoverMetaPath') }}</span
                >{{ node.path }}
              </div>
              <div v-if="node.doc_type">
                <span>{{ t('knowledgePage.hoverMetaType') }}</span
                >{{ node.doc_type }}
              </div>
            </div>
          </div>
        </div>

        <!-- 第二行：关联计数 chips（显式/实体/语义），点击锚滚关联区；零计数禁用点击（UX-006） -->
        <div class="row items-center q-gutter-xs q-mt-sm">
          <q-chip
            v-for="c in linkChips"
            :key="c.type"
            dense
            size="sm"
            :color="c.count ? c.color : 'grey-4'"
            :text-color="c.count ? 'white' : 'grey-7'"
            :clickable="c.count > 0"
            @click="c.count > 0 && scrollToLinks()"
          >
            {{ c.label }} {{ c.count }}
          </q-chip>
          <q-space />
          <!-- 操作：编辑（md/txt vault）/ 下载原文（word 等）/ 移动 / 删除 -->
          <q-btn
            v-if="editable && !editing"
            flat
            dense
            round
            size="sm"
            icon="edit_outlined"
            @click="$emit('start-edit')"
          >
            <q-tooltip>{{ t('knowledgePage.detailEdit') }}</q-tooltip>
          </q-btn>
          <q-btn v-if="showDownload" flat dense round size="sm" icon="download" @click="$emit('download-asset')">
            <q-tooltip>{{ t('knowledgePage.detailDownloadOriginal') }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" icon="drive_file_move_outline" @click="$emit('move')">
            <q-tooltip>{{ t('knowledgePage.moveTo') }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" icon="delete_outline" color="negative" @click="$emit('delete')">
            <q-tooltip>{{ t('knowledgePage.detailDeleteAria') }}</q-tooltip>
          </q-btn>
        </div>

        <!-- 正文/媒体区：固定高 420px，主题化滚动条 -->
        <div class="knowledge-doc-detail__section-title q-mt-md">
          {{ mediaSectionTitle }}
          <q-chip
            v-if="previewOrganized && showTextPreview"
            dense
            size="sm"
            color="positive"
            text-color="white"
            class="q-ml-xs"
          >
            {{ t('knowledgePage.previewOrganizedBadge') }}
          </q-chip>
          <q-space />
          <template v-if="editing">
            <q-btn
              flat
              dense
              no-caps
              size="sm"
              color="primary"
              :label="t('knowledgePage.detailCancel')"
              @click="$emit('cancel-edit')"
            />
            <q-btn
              unelevated
              dense
              no-caps
              size="sm"
              color="primary"
              :label="editSaving ? t('knowledgePage.detailSaving') : t('knowledgePage.detailSave')"
              :loading="editSaving"
              @click="$emit('save-edit')"
            />
          </template>
        </div>
        <div class="knowledge-doc-detail__media">
          <q-linear-progress v-if="previewLoading || assetLoading" indeterminate color="primary" />
          <template v-else>
            <!-- 编辑态：等宽 textarea（G2-B5） -->
            <textarea
              v-if="editing"
              class="knowledge-doc-detail__editor"
              :value="editDraft"
              spellcheck="false"
              @input="$emit('update:edit-draft', ($event.target as HTMLTextAreaElement).value)"
            />
            <!-- 图片/音频/视频：B6 原始流内联渲染 -->
            <img
              v-else-if="mediaKind === 'image' && assetUrl"
              :src="assetUrl"
              :alt="node.name"
              class="knowledge-doc-detail__img"
            />
            <audio
              v-else-if="mediaKind === 'audio' && assetUrl"
              :src="assetUrl"
              controls
              class="knowledge-doc-detail__audio"
            />
            <video
              v-else-if="mediaKind === 'video' && assetUrl"
              :src="assetUrl"
              controls
              class="knowledge-doc-detail__video"
            />
            <!-- md/txt/word/其他：解析后文本预览 -->
            <pre v-else-if="previewContent" class="knowledge-doc-detail__preview">{{ previewContent }}</pre>
            <!-- UX-003：加载失败错误态（不再复用「解析中」占位文案），内联重试 -->
            <div v-else-if="previewError" class="knowledge-doc-detail__error">
              <q-icon name="error_outline" size="20px" color="warning" />
              <span class="text-caption">{{ t('knowledgePage.contentLoadError') }}</span>
              <q-btn
                flat
                dense
                no-caps
                size="sm"
                color="primary"
                :label="t('knowledgePage.retry')"
                @click="$emit('retry')"
              />
            </div>
            <div v-else class="text-caption text-grey-6 q-pa-sm">{{ t('knowledgePage.previewEmpty') }}</div>
          </template>
        </div>

        <!-- 关联区（chips 锚滚目标） -->
        <div ref="linksSection" class="knowledge-doc-detail__section-title q-mt-md">
          {{ t('knowledgePage.detailLinks') }}
        </div>
        <q-linear-progress v-if="linksLoading" indeterminate color="primary" />
        <!-- UX-003：关联加载失败错误态 + 内联重试 -->
        <div v-else-if="linksError" class="knowledge-doc-detail__links-error">
          <q-icon name="error_outline" size="16px" color="warning" />
          <span class="text-caption text-grey-6">{{ t('knowledgePage.linksLoadError') }}</span>
          <q-btn
            flat
            dense
            no-caps
            size="sm"
            color="primary"
            :label="t('knowledgePage.retry')"
            @click="$emit('retry')"
          />
        </div>
        <!-- UX-007：按目标文档聚合（一篇一行），双向合并标注「互引」，多类型并列 chips -->
        <q-list v-else-if="groupedLinks.length" dense separator>
          <q-item
            v-for="g in groupedLinks"
            :key="g.docId"
            clickable
            @click="$emit('navigate', { docId: g.docId, relPath: g.relPath })"
          >
            <q-item-section avatar>
              <q-icon :name="g.out && g.in ? 'swap_horiz' : g.out ? 'north_east' : 'south_west'" size="16px" />
            </q-item-section>
            <q-item-section>
              <q-item-label lines="1">{{ g.source }}</q-item-label>
              <q-item-label v-if="g.context" caption lines="1">{{ g.context }}</q-item-label>
            </q-item-section>
            <q-item-section side class="row items-center q-gutter-xs">
              <span class="text-caption text-grey-6">{{ dirLabel(g) }}</span>
              <q-chip v-for="tp in g.types" :key="tp" dense size="sm" :color="linkTypeColor(tp)" text-color="white">
                {{ linkTypeLabel(tp) }}
              </q-chip>
            </q-item-section>
          </q-item>
        </q-list>
        <div v-else class="text-caption text-grey-6">{{ t('knowledgePage.detailLinksEmpty') }}</div>
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
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { KnowledgeLink, VaultTreeNode } from '../../features/knowledge/types';
import type { KnowledgeMediaKind } from '../../features/knowledge/knowledgeUi';
import {
  formatKnowledgeDocSize,
  formatKnowledgeTime,
  knowledgeStatusColor,
  knowledgeStatusLabelKey,
} from '../../features/knowledge/knowledgeUi';

const props = defineProps<{
  node: VaultTreeNode | null;
  previewContent: string;
  previewOrganized: boolean;
  previewLoading: boolean;
  /** UX-003：正文加载失败错误态（内联展示 + 重试）。 */
  previewError: boolean;
  /** UX-003：关联加载失败错误态。 */
  linksError: boolean;
  links: KnowledgeLink[];
  linksLoading: boolean;
  linkCounts: { explicit: number; entity: number; semantic: number };
  mediaKind: KnowledgeMediaKind;
  editable: boolean;
  editing: boolean;
  editDraft: string;
  editSaving: boolean;
  assetUrl: string;
  assetLoading: boolean;
}>();

defineEmits<{
  delete: [];
  move: [];
  navigate: [payload: { docId: string; relPath: string }];
  'start-edit': [];
  'cancel-edit': [];
  'save-edit': [];
  'update:edit-draft': [value: string];
  'download-asset': [];
  /** UX-003：错误态重试（正文 + 关联一并重拉）。 */
  retry: [];
}>();

const { t } = useI18n();
const statusColor = knowledgeStatusColor;

/** 状态 chip 本地化标签（未知状态回退原始值）。 */
function statusLabel(status: string): string {
  const key = knowledgeStatusLabelKey(status);
  return key ? t(key) : status;
}

const linksSection = ref<HTMLElement | null>(null);

const headerIcon = computed(() => {
  switch (props.mediaKind) {
    case 'image':
      return 'image';
    case 'audio':
      return 'audiotrack';
    case 'video':
      return 'movie';
    case 'word':
      return 'article';
    default:
      return 'description';
  }
});

const mediaSectionTitle = computed(() => {
  if (props.editing) return t('knowledgePage.detailEditing');
  switch (props.mediaKind) {
    case 'image':
      return t('knowledgePage.detailMediaImage');
    case 'audio':
      return t('knowledgePage.detailMediaAudio');
    case 'video':
      return t('knowledgePage.detailMediaVideo');
    default:
      return t('knowledgePage.detailPreview');
  }
});

/** 文本预览徽标（已整理）仅在非编辑、非纯媒体渲染时展示。 */
const showTextPreview = computed(
  () => !props.editing && !(props.assetUrl && ['image', 'audio', 'video'].includes(props.mediaKind)),
);

/** 下载原文按钮：word/其他二进制（md/txt 就地编辑，媒体区已是原件）。 */
const showDownload = computed(() => props.mediaKind === 'word' || props.mediaKind === 'other');

const linkChips = computed(() => [
  { type: 'explicit', label: t('knowledgePage.linkTypeExplicit'), count: props.linkCounts.explicit, color: 'primary' },
  { type: 'entity', label: t('knowledgePage.linkTypeEntity'), count: props.linkCounts.entity, color: 'deep-purple' },
  { type: 'semantic', label: t('knowledgePage.linkTypeSemantic'), count: props.linkCounts.semantic, color: 'teal' },
]);

/** UX-007：按目标文档聚合后的关联行（一篇文档一行，方向/类型合并）。 */
interface GroupedLink {
  docId: string;
  relPath: string;
  source: string;
  context: string;
  out: boolean;
  in: boolean;
  types: string[];
}

const groupedLinks = computed<GroupedLink[]>(() => {
  const map = new Map<string, GroupedLink>();
  for (const l of props.links) {
    let g = map.get(l.target_doc_id);
    if (!g) {
      g = {
        docId: l.target_doc_id,
        relPath: l.target_rel_path,
        source: l.target_source,
        context: l.context,
        out: false,
        in: false,
        types: [],
      };
      map.set(l.target_doc_id, g);
    }
    if (l.direction === 'out') g.out = true;
    else g.in = true;
    if (!g.types.includes(l.link_type)) g.types.push(l.link_type);
    if (!g.context && l.context) g.context = l.context;
  }
  return [...map.values()];
});

function dirLabel(g: GroupedLink): string {
  if (g.out && g.in) return t('knowledgePage.linkDirBoth');
  return g.out ? t('knowledgePage.linkDirOut') : t('knowledgePage.linkDirIn');
}

/** UX-001：仅在详情列内部滚动到关联区（scrollIntoView 会把整页推走，丢失左/中栏上下文）。 */
function scrollToLinks() {
  const section = linksSection.value;
  const scroller = section?.closest('.knowledge-doc-detail__body');
  if (!section || !scroller) return;
  const top = section.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop - 4;
  scroller.scrollTo({ top, behavior: 'smooth' });
}

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

  &__path {
    font-size: 11px;
    color: var(--color-text-secondary);
    margin-bottom: 6px;
  }

  // 第一行：摘要一行省略 + hover 360px 浮层卡（V12.4）
  &__summary-hover {
    position: relative;
  }

  &__summary {
    font-size: 13px;
    line-height: 1.6;
    color: var(--color-text-primary);
    cursor: default;
  }

  &__hover-card {
    position: absolute;
    top: calc(100% + 6px);
    left: 0;
    width: 360px;
    max-width: 90vw;
    padding: 12px 14px;
    background: var(--color-surface-solid);
    border: 1px solid var(--color-border-soft);
    border-radius: 10px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
    z-index: 20;
    opacity: 0;
    visibility: hidden;
    transform: translateY(-4px);
    transition:
      opacity 0.15s ease,
      transform 0.15s ease,
      visibility 0.15s;
  }

  &__summary-hover:hover &__hover-card {
    opacity: 1;
    visibility: visible;
    transform: translateY(0);
  }

  &__hover-summary {
    font-size: 13px;
    line-height: 1.7;
    color: var(--color-text-primary);
    white-space: pre-wrap;
  }

  &__hover-meta {
    margin-top: 8px;
    font-size: 11px;
    color: var(--color-text-secondary);
    display: flex;
    flex-direction: column;
    gap: 2px;

    span {
      display: inline-block;
      min-width: 56px;
      color: var(--color-text-primary);
      font-weight: 600;
    }
  }

  &__section-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--color-text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    margin-bottom: 6px;
    display: flex;
    align-items: center;
    gap: 6px;
  }

  // 正文/媒体区：固定高 420px + 主题化滚动条（V12.4）
  &__media {
    height: 420px;
    overflow: auto;
    background: var(--color-surface-soft);
    border-radius: 8px;
    display: flex;
    flex-direction: column;

    &::-webkit-scrollbar {
      width: 8px;
      height: 8px;
    }
    &::-webkit-scrollbar-track {
      background: transparent;
    }
    &::-webkit-scrollbar-thumb {
      background: var(--color-border-soft);
      border-radius: 4px;
    }
    &::-webkit-scrollbar-thumb:hover {
      background: var(--color-primary, var(--q-primary));
    }
  }

  &__editor {
    flex: 1;
    min-height: 0;
    resize: none;
    border: none;
    outline: none;
    padding: 10px 12px;
    background: transparent;
    color: var(--color-text-primary);
    font-family: 'JetBrains Mono', 'Cascadia Code', Consolas, monospace;
    font-size: 12px;
    line-height: 1.6;
  }

  &__img {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
    margin: auto;
  }

  &__audio {
    width: 100%;
    margin: auto 0;
  }

  &__video {
    max-width: 100%;
    max-height: 100%;
    margin: auto;
    background: #000;
  }

  &__preview {
    margin: 0;
    padding: 10px 12px;
    font-size: 12px;
    line-height: 1.6;
    white-space: pre-wrap;
    word-break: break-word;
    color: var(--color-text-primary);
  }

  // UX-003：加载失败错误态（正文区居中 / 关联区横排）。
  &__error {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    color: var(--color-text-secondary);
  }

  &__links-error {
    display: flex;
    align-items: center;
    gap: 6px;
  }
}
</style>
