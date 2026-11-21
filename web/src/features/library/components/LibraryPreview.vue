<template>
  <!-- 资料右栏：文件名、位置、操作、预览（PDF / 图片原样，Markdown 渲染并可就地编辑，其余看抽出的文字）、AI 摘要、信息。10-资料.md §3.2 -->
  <aside
    class="lb-panel lb-pv"
    :class="{ 'lb-pv--full': full }"
    :aria-label="t('library.preview.aria')"
    :aria-modal="full ? 'true' : undefined"
    :role="full ? 'dialog' : undefined"
  >
    <AppStatePanel
      v-if="!node"
      class="lb-pv__pick"
      variant="empty"
      icon="description"
      dense
      :title="t('library.preview.pick')"
      :description="t('library.preview.pickDesc')"
    />
    <template v-else>
      <!-- 标题栏：一行——文件名与完整路径在左，操作全是小图标（悬停看说明）在右 -->
      <header class="lb-pv__head">
        <span class="lb-fi" :class="`lb-fi--${kind}`" aria-hidden="true">{{ KIND_LABEL[kind] }}</span>
        <div class="lb-pv__title">
          <b :title="node.name">{{ node.name }}</b>
          <span class="lb-pv__path lb-mono" :title="fullPath">{{ fullPath }}</span>
        </div>
        <div class="lb-pv__acts" role="toolbar" :aria-label="t('library.preview.toolbar')">
          <span v-if="editing && dirty" class="lb-pv__dirty" role="status">{{ t('library.preview.unsaved') }}</span>
          <q-btn
            v-for="a in actions"
            :key="a.key"
            flat
            dense
            round
            size="sm"
            :icon="a.icon"
            :color="a.color"
            :loading="a.loading"
            :disable="a.disable"
            :aria-label="a.label"
            :aria-pressed="a.pressed"
            @click="a.run"
          >
            <q-tooltip>{{ a.label }}</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" icon="info_outline" :aria-label="t('library.preview.info')">
            <q-tooltip>{{ t('library.preview.info') }}</q-tooltip>
            <q-menu anchor="bottom right" self="top right" class="lb-pv-info-menu">
              <div class="lb-pv__info">
                <template v-if="node.summary">
                  <!-- 摘要：配了大模型时是模型写的，没配时退化成标题，所以不标「AI」 -->
                  <div class="lb-sect">{{ t('library.preview.summary') }}</div>
                  <p class="lb-pv__ai">{{ node.summary }}</p>
                </template>
                <div class="lb-sect">{{ t('library.preview.info') }}</div>
                <dl class="lb-kv">
                  <dt>{{ t('library.preview.size') }}</dt>
                  <dd class="lb-mono">{{ fileSize(node.size_bytes) }}</dd>
                  <dt>{{ t('library.preview.modified') }}</dt>
                  <dd class="lb-mono">{{ fullTime(node.updated_at) || '—' }}</dd>
                  <dt>{{ t('library.preview.type') }}</dt>
                  <dd>{{ KIND_LABEL[kind] }}</dd>
                  <dt>{{ t('library.preview.status') }}</dt>
                  <dd>{{ statusLabel }}</dd>
                  <template v-if="node.tags.length">
                    <dt>{{ t('library.preview.tags') }}</dt>
                    <dd class="lb-tags">
                      <span v-for="tag in node.tags" :key="tag" class="lb-chip">{{ tag }}</span>
                    </dd>
                  </template>
                  <dt>{{ t('library.preview.library') }}</dt>
                  <dd>{{ library?.name }}</dd>
                  <dt>{{ t('library.preview.location') }}</dt>
                  <dd class="lb-mono lb-kv__path">{{ fullPath }}</dd>
                </dl>
              </div>
            </q-menu>
          </q-btn>
          <q-btn flat dense round size="sm" icon="more_horiz" :aria-label="t('library.preview.more')">
            <q-tooltip>{{ t('library.preview.more') }}</q-tooltip>
            <q-menu anchor="bottom right" self="top right">
              <q-list dense>
                <q-item v-if="movable" v-close-popup clickable @click="emit('rename')">
                  <q-item-section avatar><q-icon name="drive_file_rename_outline" size="18px" /></q-item-section>
                  <q-item-section>{{ t('library.preview.rename') }}</q-item-section>
                </q-item>
                <q-item v-if="movable" v-close-popup clickable @click="emit('move')">
                  <q-item-section avatar><q-icon name="drive_file_move" size="18px" /></q-item-section>
                  <q-item-section>{{ t('library.preview.move') }}</q-item-section>
                </q-item>
                <q-item v-if="movable" v-close-popup clickable @click="emit('history')">
                  <q-item-section avatar><q-icon name="history" size="18px" /></q-item-section>
                  <q-item-section>{{ t('library.preview.history') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable @click="emit('visibility')">
                  <q-item-section avatar><q-icon name="visibility" size="18px" /></q-item-section>
                  <q-item-section>{{ t('library.preview.visibility') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable class="lb-danger" @click="emit('remove')">
                  <q-item-section avatar><q-icon name="delete_outline" size="18px" /></q-item-section>
                  <q-item-section>{{ t('library.preview.remove') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </div>
      </header>

      <div class="lb-pv__body" :class="{ 'lb-pv__body--fill': fills }">
        <q-linear-progress v-if="loading" indeterminate size="2px" color="primary" />
        <p v-else-if="loadError" class="lb-hint lb-pv__msg" role="alert">{{ loadError }}</p>
        <template v-else>
          <NoteEditor
            v-if="editing"
            class="lb-pv__editor lb-pv__doc"
            :content="draft"
            :document-id="node.doc_id"
            @update-content="onDraft"
            @save="save"
          />
          <div v-else-if="mode === 'markdown'" class="lb-pv__mdwrap">
            <!-- 目录：点标题滚到对应位置 -->
            <nav v-if="showToc && headings.length" class="lb-pv__toc" :aria-label="t('library.preview.toc')">
              <button
                v-for="(h, i) in headings"
                :key="i"
                type="button"
                class="lb-pv__toc-it"
                :style="{ paddingLeft: `${(h.level - 1) * 12 + 8}px` }"
                @click="scrollToHeading(i)"
              >
                {{ h.text }}
              </button>
            </nav>
            <NotePreview
              ref="mdRef"
              class="lb-pv__md lb-pv__doc"
              :content="content?.raw_content || content?.content_text || ''"
            />
          </div>
          <!-- Word / Excel / PPT：浏览器里解开文件按原结构出（标题、表格、工作表、每页幻灯片），只含转义过的文字 -->
          <!-- eslint-disable-next-line vue/no-v-html -->
          <div v-else-if="officeHtml" class="lb-office lb-pv__doc lb-pv__doc--wide" v-html="officeHtml" />
          <iframe v-else-if="mode === 'pdf' && assetUrl" class="lb-pv__frame" :src="assetUrl" :title="node.name" />
          <img
            v-else-if="mode === 'image' && assetUrl"
            class="lb-pv__img"
            :class="{ 'lb-pv__img--full': zoomed }"
            :src="assetUrl"
            :alt="node.name"
            :title="zoomed ? t('library.preview.zoomFit') : t('library.preview.zoomFull')"
            @click="zoomed = !zoomed"
          />
          <video v-else-if="mode === 'media' && assetUrl && isVideo" class="lb-pv__img" :src="assetUrl" controls />
          <audio v-else-if="mode === 'media' && assetUrl" :src="assetUrl" controls />
          <template v-else-if="mode === 'text'">
            <p v-if="node.status === 'error'" class="lb-hint lb-pv__msg" role="alert">
              {{ t('library.preview.errorText', { reason: node.error_message || node.status }) }}
            </p>
            <div v-else-if="content?.content_text" class="lb-pv__doc">
              <p class="lb-hint">{{ t('library.preview.textNote') }}</p>
              <div class="lb-pv__text">{{ content.content_text }}</div>
            </div>
            <p v-else class="lb-hint lb-pv__msg">{{ t('library.preview.noText') }}</p>
          </template>
        </template>
      </div>
    </template>
  </aside>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { copyToClipboard, useQuasar } from 'quasar';
import AppStatePanel from '../../../components/common/AppStatePanel.vue';
import { confirm } from '../../../composables/useAppConfirm';
import NoteEditor from '../../../components/knowledge/workbench/NoteEditor.vue';
import NotePreview from '../../../components/knowledge/workbench/NotePreview.vue';
import {
  fetchDocumentAsset,
  getDocumentContent,
  openDocumentLocally,
  updateDocumentContent,
} from '../../knowledge/api';
import type { KnowledgeCollection, KnowledgeDocumentContent, VaultTreeNode } from '../../knowledge/types';
import { fileSize, fullTime } from '../../mail/model';
import { parseKratosApiError } from '../../../utils/kratosError';
import { fileKind, isLocalLibrary, KIND_LABEL, localPathOf, mdHeadings, previewMode } from '../model';
import { officeKindOf, officeToHtml } from '../office';

const props = defineProps<{
  node: VaultTreeNode | null;
  library: KnowledgeCollection | null;
  /** 后端开了本机动作（FEATURES_LOCAL_REVEAL_ENABLED） */
  localEnabled: boolean;
  /** 已收藏 */
  favorite: boolean;
}>();

const emit = defineEmits<{
  move: [];
  remove: [];
  /** 保存成功：上层刷新列表 */
  saved: [];
  /** 有没保存的修改：上层切换文件前要确认 */
  dirty: [value: boolean];
  favorite: [];
  rename: [];
  history: [];
  visibility: [];
  copyLink: [];
}>();

const { t } = useI18n();
const $q = useQuasar();

const content = ref<KnowledgeDocumentContent | null>(null);
const assetUrl = ref('');
const loading = ref(false);
const loadError = ref('');
const editing = ref(false);
const draft = ref('');
const baseHash = ref('');
const saving = ref(false);
const opening = ref(false);
const downloading = ref(false);

const kind = computed(() => fileKind(props.node?.name ?? ''));
const mode = computed(() => previewMode(kind.value));
const isVideo = computed(() => /\.(mp4|mov|mkv|webm|avi)$/i.test(props.node?.name ?? ''));
const local = computed(() => !!props.library && isLocalLibrary(props.library));
const localPath = computed(() =>
  local.value && props.node ? localPathOf(props.library!.root_path, props.node.path) : '',
);
const canOpenLocal = computed(() => local.value && props.localEnabled);
const movable = computed(() => local.value);
const canEdit = computed(() => mode.value === 'markdown' && !!(content.value?.raw_content || content.value?.base_hash));
const dirty = computed(() => editing.value && draft.value !== (content.value?.raw_content ?? ''));
const statusLabel = computed(() => {
  const s = props.node?.status ?? '';
  return ['indexed', 'pending', 'indexing', 'error'].includes(s) ? t(`library.status.${s}`) : s || '—';
});

watch(dirty, (v) => emit('dirty', v));

/** 标题栏里显示、可复制的完整位置：本机资料库是磁盘路径，团队资料库是「资料库 / 相对路径」。 */
const fullPath = computed(() => {
  if (!props.node) return '';
  if (localPath.value) return localPath.value;
  return [props.library?.name ?? '', props.node.path.replace(/^\/+/, '')].filter(Boolean).join(' / ');
});

/** PDF / 图片 / 音视频铺满预览区，不留内边距。 */
const fills = computed(() => !editing.value && ['pdf', 'image', 'media'].includes(mode.value));

// 全屏：预览（含编辑）盖住整个窗口；Esc 退出（弹窗、菜单开着时 Esc 先关它们）
const full = ref(false);
watch(
  () => props.node,
  (n) => {
    if (!n) full.value = false;
  },
);
function onKey(e: KeyboardEvent) {
  if (e.key !== 'Escape' || !full.value || document.querySelector('.q-dialog, .q-menu')) return;
  full.value = false;
}
onMounted(() => window.addEventListener('keydown', onKey));
onBeforeUnmount(() => window.removeEventListener('keydown', onKey));

type PreviewAction = {
  key: string;
  icon: string;
  label: string;
  run: () => void;
  color?: string;
  loading?: boolean;
  disable?: boolean;
  pressed?: boolean;
};

const actions = computed<PreviewAction[]>(() => {
  const out: PreviewAction[] = [];
  if (editing.value) {
    out.push(
      {
        key: 'save',
        icon: 'save',
        color: 'primary',
        label: t('library.preview.saveKey'),
        loading: saving.value,
        disable: !dirty.value,
        run: () => void save(),
      },
      { key: 'stop', icon: 'edit_off', label: t('library.preview.stopEdit'), run: () => void stopEdit() },
    );
  } else if (canEdit.value) {
    out.push({ key: 'edit', icon: 'edit', label: t('library.preview.editHere'), run: startEdit });
  }
  if (canOpenLocal.value)
    out.push({
      key: 'local',
      icon: 'open_in_new',
      label: t('library.preview.openLocal'),
      loading: opening.value,
      run: () => void openLocal(),
    });
  out.push(
    {
      key: 'download',
      icon: 'download',
      label: t('library.preview.download'),
      loading: downloading.value,
      run: () => void download(),
    },
    { key: 'copy', icon: 'content_copy', label: t('library.preview.copyPath'), run: () => void copyPath() },
    { key: 'link', icon: 'link', label: t('library.preview.copyLink'), run: () => emit('copyLink') },
    {
      key: 'fav',
      icon: props.favorite ? 'star' : 'star_border',
      color: props.favorite ? 'warning' : undefined,
      label: props.favorite ? t('library.preview.unfavorite') : t('library.preview.favorite'),
      pressed: props.favorite,
      run: () => emit('favorite'),
    },
  );
  if (mode.value === 'markdown' && !editing.value && headings.value.length)
    out.push({
      key: 'toc',
      icon: 'toc',
      label: t('library.preview.toc'),
      pressed: showToc.value,
      color: showToc.value ? 'primary' : undefined,
      run: () => (showToc.value = !showToc.value),
    });
  out.push({
    key: 'full',
    icon: full.value ? 'fullscreen_exit' : 'fullscreen',
    label: full.value ? t('library.preview.exitFull') : t('library.preview.full'),
    pressed: full.value,
    run: () => (full.value = !full.value),
  });
  return out;
});

async function copyPath() {
  try {
    await copyToClipboard(fullPath.value);
    $q.notify({ type: 'positive', message: t('library.preview.copied') });
  } catch {
    $q.notify({ type: 'negative', message: t('library.preview.copyFailed') });
  }
}

function revokeAsset() {
  if (assetUrl.value) URL.revokeObjectURL(assetUrl.value);
  assetUrl.value = '';
}

// ---------- 目录 / Office / 图片缩放 ----------

const showToc = ref(true);
const headings = computed(() => mdHeadings(content.value?.raw_content || content.value?.content_text || ''));
const mdRef = ref<{ $el?: HTMLElement } | null>(null);

/** 第 i 个标题 = 渲染结果里第 i 个 h1–h6。 */
function scrollToHeading(i: number) {
  const el = mdRef.value?.$el?.querySelectorAll('h1, h2, h3, h4, h5, h6')[i];
  el?.scrollIntoView({ behavior: 'smooth', block: 'start' });
}

const officeHtml = ref('');
const zoomed = ref(false);

let loadSeq = 0;
async function load() {
  const node = props.node;
  const seq = ++loadSeq;
  editing.value = false;
  content.value = null;
  loadError.value = '';
  officeHtml.value = '';
  zoomed.value = false;
  revokeAsset();
  if (!node?.doc_id) return;
  loading.value = true;
  try {
    const office = officeKindOf(node.name);
    if (office) {
      // 先按原结构出；文件坏了 / 加密了就退回看抽出来的文字
      try {
        const { blob } = await fetchDocumentAsset(node.doc_id);
        const html = await officeToHtml(office, blob);
        if (seq === loadSeq && html) {
          officeHtml.value = html;
          return;
        }
      } catch {
        // 往下走抽出来的文字
      }
    }
    if (mode.value === 'pdf' || mode.value === 'image' || mode.value === 'media') {
      const { blob } = await fetchDocumentAsset(node.doc_id);
      if (seq === loadSeq) assetUrl.value = URL.createObjectURL(blob);
    } else {
      const c = await getDocumentContent(node.doc_id);
      if (seq === loadSeq) {
        content.value = c;
        baseHash.value = c.base_hash;
      }
    }
  } catch {
    if (seq === loadSeq) loadError.value = t('library.preview.loadFailed');
  } finally {
    if (seq === loadSeq) loading.value = false;
  }
}

watch(() => props.node?.doc_id, load, { immediate: true });
onBeforeUnmount(revokeAsset);

function startEdit() {
  draft.value = content.value?.raw_content ?? '';
  editing.value = true;
}

async function stopEdit() {
  if (dirty.value) {
    const ok = await confirm({
      title: t('library.preview.leaveTitle'),
      message: t('library.preview.leaveMsg'),
      tone: 'danger',
      confirmLabel: t('library.preview.leaveOk'),
    });
    if (!ok) return;
  }
  editing.value = false;
}

function onDraft(text: string) {
  draft.value = text;
}

async function save() {
  if (!props.node || !dirty.value || saving.value) return;
  saving.value = true;
  try {
    const res = await updateDocumentContent(props.node.doc_id, draft.value, baseHash.value);
    baseHash.value = res.base_hash || baseHash.value;
    content.value = {
      ...(content.value as KnowledgeDocumentContent),
      raw_content: draft.value,
      base_hash: baseHash.value,
    };
    $q.notify({
      type: res.conflict ? 'warning' : 'positive',
      message: t(res.conflict ? 'library.preview.savedConflict' : 'library.preview.saved'),
    });
    emit('saved');
  } catch (e) {
    $q.notify({ type: 'negative', message: parseKratosApiError(e).message });
  } finally {
    saving.value = false;
  }
}

async function download() {
  if (!props.node) return;
  downloading.value = true;
  try {
    const { blob, filename } = await fetchDocumentAsset(props.node.doc_id);
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename || props.node.name;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 30_000);
  } catch {
    $q.notify({ type: 'negative', message: t('library.preview.loadFailed') });
  } finally {
    downloading.value = false;
  }
}

async function openLocal() {
  if (!props.node) return;
  opening.value = true;
  try {
    await openDocumentLocally(props.node.doc_id);
    $q.notify({ type: 'positive', message: t('library.preview.openedLocal') });
  } catch (e) {
    const { status } = parseKratosApiError(e);
    if (status === 400) {
      $q.notify({ type: 'info', message: t('library.preview.blockedType') });
      await download();
    } else if (status === 412 || status === 501 || status === 404) {
      $q.notify({ type: 'info', message: t('library.preview.fallbackDownload') });
      await download();
    } else {
      $q.notify({ type: 'negative', message: parseKratosApiError(e).message });
    }
  } finally {
    opening.value = false;
  }
}
</script>
