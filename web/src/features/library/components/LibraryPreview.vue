<template>
  <!-- 资料右栏：文件名、位置、操作、预览（PDF / 图片原样，Markdown 渲染并可就地编辑，其余看抽出的文字）、AI 摘要、信息。10-资料.md §3.2 -->
  <aside class="lb-panel lb-pv" :aria-label="t('library.preview.aria')">
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
      <header class="lb-pv__head">
        <div class="lb-pv__name">
          <span class="lb-fi" :class="`lb-fi--${kind}`" aria-hidden="true">{{ KIND_LABEL[kind] }}</span>
          <b :title="node.name">{{ node.name }}</b>
        </div>
        <div class="lb-pv__path lb-mono">
          <template v-if="localPath">{{ t('library.preview.local') }} {{ localPath }}</template>
          <template v-else>{{ t('library.preview.library') }} {{ library?.name }}</template>
        </div>
        <div class="lb-pv__acts">
          <q-btn
            v-if="canOpenLocal"
            unelevated
            no-caps
            dense
            color="primary"
            icon="open_in_new"
            class="lb-pv__btn"
            :loading="opening"
            :label="t('library.preview.openLocal')"
            @click="openLocal"
          />
          <q-btn
            v-if="canEdit && !editing"
            flat
            no-caps
            dense
            icon="edit"
            class="lb-pv__btn"
            :label="t('library.preview.editHere')"
            @click="startEdit"
          />
          <template v-if="editing">
            <q-btn
              unelevated
              no-caps
              dense
              color="primary"
              icon="save"
              class="lb-pv__btn"
              :loading="saving"
              :disable="!dirty"
              :label="t('library.preview.save')"
              @click="save"
            />
            <q-btn flat no-caps dense class="lb-pv__btn" :label="t('library.preview.stopEdit')" @click="stopEdit" />
          </template>
          <q-btn
            :unelevated="!canOpenLocal"
            :flat="canOpenLocal"
            :color="canOpenLocal ? undefined : 'primary'"
            no-caps
            dense
            icon="download"
            class="lb-pv__btn"
            :loading="downloading"
            :label="t('library.preview.download')"
            @click="download"
          />
          <span class="lb-sp" />
          <q-btn flat dense round icon="more_horiz" :aria-label="t('library.preview.more')">
            <q-menu anchor="bottom right" self="top right">
              <q-list dense>
                <q-item v-if="movable" v-close-popup clickable @click="emit('move')">
                  <q-item-section avatar><q-icon name="drive_file_move" size="18px" /></q-item-section>
                  <q-item-section>{{ t('library.preview.move') }}</q-item-section>
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

      <div class="lb-pv__body">
        <q-linear-progress v-if="loading" indeterminate size="2px" color="primary" />
        <p v-else-if="loadError" class="lb-hint" role="alert">{{ loadError }}</p>
        <template v-else>
          <NoteEditor
            v-if="editing"
            class="lb-pv__editor"
            :content="draft"
            :document-id="node.doc_id"
            @update-content="onDraft"
            @save="save"
          />
          <NotePreview
            v-else-if="mode === 'markdown'"
            class="lb-pv__md"
            :content="content?.raw_content || content?.content_text || ''"
          />
          <iframe v-else-if="mode === 'pdf' && assetUrl" class="lb-pv__frame" :src="assetUrl" :title="node.name" />
          <img v-else-if="mode === 'image' && assetUrl" class="lb-pv__img" :src="assetUrl" :alt="node.name" />
          <video v-else-if="mode === 'media' && assetUrl && isVideo" class="lb-pv__img" :src="assetUrl" controls />
          <audio v-else-if="mode === 'media' && assetUrl" :src="assetUrl" controls />
          <template v-else-if="mode === 'text'">
            <p v-if="node.status === 'error'" class="lb-hint" role="alert">
              {{ t('library.preview.errorText', { reason: node.error_message || node.status }) }}
            </p>
            <template v-else-if="content?.content_text">
              <p class="lb-hint">{{ t('library.preview.textNote') }}</p>
              <div class="lb-pv__text">{{ content.content_text }}</div>
            </template>
            <p v-else class="lb-hint">{{ t('library.preview.noText') }}</p>
          </template>
        </template>

        <template v-if="node.summary && !editing">
          <!-- 摘要卡：配了大模型时是模型写的，没配时退化成标题，所以不标「AI」 -->
          <div class="lb-sect">{{ t('library.preview.summary') }}</div>
          <p class="lb-pv__ai">{{ node.summary }}</p>
        </template>

        <template v-if="!editing">
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
          </dl>
        </template>
      </div>
    </template>
  </aside>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppStatePanel from '../../../components/common/AppStatePanel.vue';
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
import { fileKind, isLocalLibrary, KIND_LABEL, localPathOf, previewMode } from '../model';

const props = defineProps<{
  node: VaultTreeNode | null;
  library: KnowledgeCollection | null;
  /** 后端开了本机动作（FEATURES_LOCAL_REVEAL_ENABLED） */
  localEnabled: boolean;
}>();

const emit = defineEmits<{
  move: [];
  remove: [];
  /** 保存成功：上层刷新列表 */
  saved: [];
  /** 有没保存的修改：上层切换文件前要确认 */
  dirty: [value: boolean];
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

function revokeAsset() {
  if (assetUrl.value) URL.revokeObjectURL(assetUrl.value);
  assetUrl.value = '';
}

let loadSeq = 0;
async function load() {
  const node = props.node;
  const seq = ++loadSeq;
  editing.value = false;
  content.value = null;
  loadError.value = '';
  revokeAsset();
  if (!node?.doc_id) return;
  loading.value = true;
  try {
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

function stopEdit() {
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
