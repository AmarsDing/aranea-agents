<template>
  <!-- 工作台 › 资料：三栏（空间与目录 · 文件列表 · 预览），设计 docs/aranea-agents/design/2026-11-17-三大模块重设计/10-资料.md §3.2 -->
  <q-page class="app-standard-page lb-page">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('common.refresh')"
        :aria-label="t('common.refresh')"
        :loading="refreshing"
        @click="refreshAll"
      />
    </Teleport>

    <AppStatePanel v-if="pageState === 'forbidden'" class="lb-full" variant="forbidden" />
    <AppStatePanel v-else-if="pageState === 'loading'" class="lb-full" variant="loading" :rows="6" />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      class="lb-full"
      variant="error"
      :action-label="t('common.retry')"
      @action="refreshAll"
    />
    <AppStatePanel
      v-else-if="!collections.length"
      class="lb-full"
      variant="empty"
      icon="folder_open"
      :title="t('library.empty.title')"
      :description="t('library.empty.desc')"
      :action-label="t('library.empty.action')"
      action-icon="add"
      :secondary-label="t('library.empty.artifacts')"
      @action="wizardOpen = true"
      @secondary="router.push('/artifacts')"
    />

    <template v-else>
      <!-- ============ 左：空间 + 资料库与目录树 ============ -->
      <nav class="lb-panel lb-nav" :aria-label="t('library.nav.aria')">
        <button
          v-for="s in SPACES"
          :key="s"
          type="button"
          class="lb-space"
          :class="{ 'lb-space--on': space === s }"
          :aria-pressed="space === s"
          @click="switchSpace(s)"
        >
          <span class="lb-space__ic"><q-icon :name="s === 'mine' ? 'person' : 'groups'" size="18px" /></span>
          <b>{{ t(`library.space.${s}`) }}</b>
          <small
            >{{ spaceCount(s) ? t('library.space.libCount', { n: spaceCount(s) }) : t('library.space.noLib') }} ·
            {{ t(`library.space.${s}Hint`) }}</small
          >
        </button>

        <div class="lb-sect">{{ t(`library.space.${space}`) }}</div>
        <div class="lb-tree" role="tree" :aria-label="t(`library.space.${space}`)">
          <div
            v-for="row in treeRows"
            :key="row.key"
            class="lb-tn"
            :class="{ 'lb-tn--on': row.libId === libId && row.path === prefix }"
            :style="{ paddingLeft: `${4 + row.depth * 16}px` }"
            role="treeitem"
            :aria-expanded="row.expandable ? !!expanded[row.key] : undefined"
            :aria-selected="row.libId === libId && row.path === prefix"
          >
            <button
              type="button"
              class="lb-tn__tog"
              :class="{ 'lb-tn__tog--hide': !row.expandable }"
              :aria-label="expanded[row.key] ? t('library.nav.collapse') : t('library.nav.expand')"
              :tabindex="row.expandable ? 0 : -1"
              @click="toggleTree(row)"
            >
              <q-icon :name="expanded[row.key] ? 'expand_more' : 'chevron_right'" size="16px" />
            </button>
            <button type="button" class="lb-tn__label" :title="row.title" @click="openTreeRow(row)">
              <q-icon
                :name="row.depth === 0 ? 'inventory_2' : 'folder'"
                size="16px"
                class="lb-tn__ic"
                aria-hidden="true"
              />
              <span>{{ row.label }}</span>
            </button>
            <q-btn
              v-if="row.depth === 0"
              flat
              dense
              round
              size="sm"
              icon="more_horiz"
              class="lb-tn__more"
              :aria-label="t('library.nav.libMenu', { name: row.label })"
            >
              <q-menu anchor="bottom right" self="top right">
                <q-list dense>
                  <q-item v-close-popup clickable @click="moveLibrary(row.libId)">
                    <q-item-section>{{
                      t('library.nav.moveTo', { space: t(`library.space.${otherSpace}`) })
                    }}</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>
            </q-btn>
          </div>
          <p v-if="!spaceLibs.length" class="lb-hint lb-tree__empty">{{ t('library.space.noLib') }}</p>
        </div>

        <q-btn
          flat
          no-caps
          dense
          icon="add"
          class="lb-nav__new"
          :label="t('library.nav.newLibrary')"
          @click="wizardOpen = true"
        />
        <router-link to="/knowledge/notes" class="lb-nav__kb" :title="t('library.nav.knowledgeBaseHint')">
          <q-icon name="hub" size="16px" aria-hidden="true" />{{ t('library.nav.knowledgeBase') }}
        </router-link>
      </nav>

      <!-- ============ 中：面包屑 · 搜索 · 新建 · 文件列表 ============ -->
      <section
        class="lb-panel lb-fl"
        :class="{ 'lb-fl--drop': dragDepth > 0 }"
        :aria-label="t('library.list.aria')"
        @dragenter.prevent="onDragEnter"
        @dragover.prevent
        @dragleave="onDragLeave"
        @drop.prevent="onDrop"
      >
        <div class="lb-fl__bar">
          <nav class="lb-bc" aria-label="breadcrumb">
            <template v-if="currentLib">
              <button type="button" class="lb-bc__seg" @click="goDir('')">{{ currentLib.name }}</button>
              <template v-for="c in crumbs" :key="c.path">
                <q-icon name="chevron_right" size="16px" aria-hidden="true" />
                <button type="button" class="lb-bc__seg" @click="goDir(c.path)">{{ c.name }}</button>
              </template>
            </template>
          </nav>
          <span class="lb-sp" />
          <span v-if="currentLib" class="lb-where lb-mono" :title="currentLib.root_path">
            {{
              isLocalLibrary(currentLib)
                ? t('library.nav.localAt', { path: currentLib.root_path })
                : t('library.nav.teamStore')
            }}
          </span>
        </div>

        <div class="lb-fl__tools">
          <!-- clearable 清空时发 null，这里统一成空串 -->
          <q-input
            :model-value="q"
            dense
            outlined
            clearable
            class="lb-fl__search"
            :placeholder="t('library.list.search')"
            :aria-label="t('library.list.searchAria')"
            @update:model-value="q = String($event ?? '')"
            @keydown.esc="q = ''"
          >
            <template #prepend><q-icon name="search" size="18px" /></template>
          </q-input>
          <q-toggle
            v-model="semantic"
            dense
            :label="t('library.list.semantic')"
            :title="t('library.list.semanticHint')"
          />
          <q-btn-dropdown
            unelevated
            no-caps
            dense
            color="primary"
            icon="add"
            class="lb-fl__new"
            :label="t('library.list.new')"
            :disable="!currentLib"
          >
            <q-list dense>
              <q-item v-close-popup clickable :disable="!localLib" @click="openNameDialog('folder')">
                <q-item-section avatar><q-icon name="create_new_folder" size="18px" /></q-item-section>
                <q-item-section>
                  <q-item-label>{{ t('library.list.newFolder') }}</q-item-label>
                  <q-item-label v-if="!localLib" caption>{{ t('library.list.teamNoFolder') }}</q-item-label>
                </q-item-section>
              </q-item>
              <q-item v-close-popup clickable :disable="!localLib" @click="openNameDialog('md')">
                <q-item-section avatar><q-icon name="note_add" size="18px" /></q-item-section>
                <q-item-section>{{ t('library.list.newMd') }}</q-item-section>
              </q-item>
              <q-item v-close-popup clickable @click="fileInput?.click()">
                <q-item-section avatar><q-icon name="upload_file" size="18px" /></q-item-section>
                <q-item-section>{{ t('library.list.upload') }}</q-item-section>
              </q-item>
            </q-list>
          </q-btn-dropdown>
          <input ref="fileInput" type="file" multiple class="lb-sr" tabindex="-1" @change="onPickFiles" />
        </div>

        <!-- 搜索结果：按文件聚合，跨两个空间 -->
        <div v-if="q.trim()" class="lb-tbl" aria-live="polite">
          <q-linear-progress v-if="searching" indeterminate size="2px" color="primary" />
          <p v-else-if="searchError" class="lb-hint lb-tbl__note" role="alert">{{ searchError }}</p>
          <template v-else>
            <p class="lb-hint lb-tbl__note">
              {{
                hits.length ? t('library.search.results', { q: q.trim(), n: hits.length }) : t('library.search.none')
              }}
              <template v-if="hits.length && semantic">{{ t('library.search.semanticNote') }}</template>
            </p>
            <button
              v-for="h in hits"
              :key="h.docId"
              type="button"
              class="lb-res"
              :class="{ 'lb-res--on': h.docId === selectedDocId }"
              @click="openHit(h)"
            >
              <span class="lb-res__t">
                <span class="lb-fi lb-fi--sm" :class="`lb-fi--${fileKind(h.name)}`" aria-hidden="true">{{
                  KIND_LABEL[fileKind(h.name)]
                }}</span>
                <span class="lb-res__n">{{ h.name }}</span>
                <span v-if="h.nameMatch" class="lb-chip">{{ t('library.search.nameHit') }}</span>
              </span>
              <span v-if="h.snippet" class="lb-res__p">{{ h.snippet }}</span>
              <span class="lb-res__m">{{ hitWhere(h) }}</span>
            </button>
          </template>
        </div>

        <AppStatePanel
          v-else-if="!currentLib"
          class="lb-fl__state"
          variant="empty"
          icon="inventory_2"
          :title="t('library.empty.spaceTitle', { space: t(`library.space.${space}`) })"
          :description="t('library.empty.spaceDesc')"
          :action-label="t('library.nav.newLibrary')"
          action-icon="add"
          @action="wizardOpen = true"
        />

        <div v-else class="lb-tbl" role="table" :aria-label="t('library.list.aria')" :aria-busy="listLoading">
          <div class="lb-th" role="row">
            <span role="columnheader">{{ t('library.list.colName') }}</span>
            <span role="columnheader">{{ t('library.list.colModified') }}</span>
            <span role="columnheader">{{ t('library.list.colSize') }}</span>
            <span role="columnheader">{{ t('library.list.colStatus') }}</span>
          </div>
          <q-linear-progress v-if="listLoading" indeterminate size="2px" color="primary" />
          <button v-if="prefix" type="button" class="lb-tr" role="row" @click="goDir(parentDir)">
            <span class="lb-tr__nm"
              ><q-icon name="arrow_upward" size="18px" class="lb-tr__up" />{{ t('library.list.up') }}</span
            >
          </button>
          <button
            v-for="n in sortedNodes"
            :key="n.kind + n.path + n.doc_id"
            type="button"
            class="lb-tr"
            :class="{ 'lb-tr--on': n.kind === 'file' && n.doc_id === selectedDocId }"
            role="row"
            :aria-selected="n.kind === 'file' ? n.doc_id === selectedDocId : undefined"
            @click="openNode(n)"
          >
            <span class="lb-tr__nm" role="cell">
              <span v-if="n.kind === 'dir'" class="lb-fi lb-fi--folder" aria-hidden="true" />
              <span v-else class="lb-fi" :class="`lb-fi--${fileKind(n.name)}`" aria-hidden="true">{{
                KIND_LABEL[fileKind(n.name)]
              }}</span>
              <span class="lb-tr__name">{{ n.name }}</span>
            </span>
            <span class="lb-tr__c lb-mono" role="cell">{{ n.updated_at ? listTime(n.updated_at, now) : '—' }}</span>
            <span class="lb-tr__c lb-mono" role="cell">{{ n.kind === 'dir' ? '—' : fileSize(n.size_bytes) }}</span>
            <span class="lb-tr__c" role="cell">
              <span
                v-if="n.kind === 'file'"
                class="lb-st"
                :class="`lb-st--${n.status}`"
                :title="n.error_message || statusText(n.status)"
              >
                <q-icon :name="STATUS_ICON[n.status] ?? 'help_outline'" size="16px" aria-hidden="true" />
                <span>{{ statusText(n.status) }}</span>
              </span>
            </span>
          </button>
          <AppStatePanel
            v-if="!listLoading && !sortedNodes.length"
            dense
            variant="empty"
            icon="folder_open"
            :title="t('library.list.emptyDir')"
            :description="t(localLib ? 'library.list.emptyDirDesc' : 'library.list.emptyDirDescTeam')"
          />
          <p v-else class="lb-hint lb-tbl__note">
            {{ prefix || localLib ? t('library.list.dropHint', { dir: dirLabel }) : t('library.list.dropHintRoot') }}
          </p>
        </div>
        <div v-if="dragDepth > 0" class="lb-drop" aria-hidden="true">
          <q-icon name="upload" size="28px" />{{ t('library.list.dropHere', { dir: dirLabel }) }}
        </div>
      </section>

      <!-- ============ 右：预览 ============ -->
      <LibraryPreview
        :node="selectedNode"
        :library="currentLib"
        :local-enabled="localEnabled"
        @move="openMove"
        @remove="removeSelected"
        @saved="loadNodes"
        @dirty="previewDirty = $event"
      />
    </template>

    <LibraryWizard
      v-model="wizardOpen"
      :default-space="space"
      :local-enabled="localEnabled"
      @created="onLibraryCreated"
    />

    <!-- 新建文件夹 / Markdown -->
    <AppDialog
      v-model="nameDlg.open"
      :title="nameDlg.kind === 'folder' ? t('library.dialog.newFolder') : t('library.dialog.newMd')"
      :subtitle="t('library.dialog.inDir', { dir: dirLabel })"
      size="sm"
      :loading="nameDlg.busy"
    >
      <q-input
        v-model="nameDlg.name"
        dense
        outlined
        autofocus
        :label="t('library.dialog.name')"
        :suffix="nameDlg.kind === 'md' ? '.md' : undefined"
        :error="!!nameDlg.error"
        :error-message="nameDlg.error"
        @keydown.enter.prevent="submitName"
      />
      <template #actions>
        <q-btn flat no-caps :label="t('library.dialog.cancel')" @click="nameDlg.open = false" />
        <q-btn
          unelevated
          no-caps
          color="primary"
          :loading="nameDlg.busy"
          :label="t('library.dialog.create')"
          @click="submitName"
        />
      </template>
    </AppDialog>

    <!-- 移动到同一资料库的另一个文件夹 -->
    <AppDialog
      v-model="moveDlg.open"
      :title="t('library.dialog.moveTitle', { name: selectedNode?.name ?? '' })"
      size="md"
      position="standard"
      :loading="moveDlg.busy"
    >
      <FolderBrowser
        :path="`${currentLib?.name ?? ''} / ${moveDlg.path || t('library.dialog.root')}`"
        :dirs="moveDlg.dirs"
        :can-up="!!moveDlg.path"
        :loading="moveDlg.loading"
        :up-label="t('library.list.up')"
        :empty-text="t('library.wizard.noSubdirs')"
        @open="browseMove"
        @up="browseMove(parentOf(moveDlg.path))"
      />
      <p v-if="moveDlg.conflict" class="lb-wz__err" role="alert">{{ t('library.dialog.moveConflict') }}</p>
      <template #actions>
        <q-btn flat no-caps :label="t('library.dialog.cancel')" @click="moveDlg.open = false" />
        <q-space />
        <template v-if="moveDlg.conflict">
          <q-btn flat no-caps :label="t('library.dialog.overwrite')" @click="submitMove('overwrite')" />
          <q-btn
            unelevated
            no-caps
            color="primary"
            :label="t('library.dialog.keepBoth')"
            @click="submitMove('rename')"
          />
        </template>
        <q-btn
          v-else
          unelevated
          no-caps
          color="primary"
          :loading="moveDlg.busy"
          :disable="moveDlg.path === dirOf(selectedNode?.path ?? '')"
          :label="t('library.dialog.moveHere')"
          @click="submitMove('')"
        />
      </template>
    </AppDialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppDialog from '../components/common/AppDialog.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import FolderBrowser from '../components/library/FolderBrowser.vue';
import { confirm } from '../composables/useAppConfirm';
import { isForbiddenApiError, parseKratosApiError } from '../utils/kratosError';
import { fetchLocalRevealEnabled } from '../features/artifact/api';
import {
  createVaultDir,
  createVaultDocument,
  deleteDocument,
  getDocument,
  ingestDocument,
  listCollections,
  listDocuments,
  listVaultTree,
  moveDocumentToDir,
  searchKnowledge,
  updateCollectionSpace,
} from '../features/knowledge/api';
import { inferUploadMime, readFileAsBase64 } from '../features/knowledge/knowledgeUploadCodec';
import type { KnowledgeCollection, KnowledgeDocument, VaultTreeNode } from '../features/knowledge/types';
import LibraryPreview from '../features/library/components/LibraryPreview.vue';
import LibraryWizard from '../features/library/components/LibraryWizard.vue';
import { fileSize, listTime } from '../features/mail/model';
import {
  crumbsOf,
  dirOf,
  fileKind,
  isLocalLibrary,
  KIND_LABEL,
  mergeSearchHits,
  nameMatches,
  sortNodes,
  spaceOf,
  SPACES,
  validFileName,
  type LibrarySpace,
  type SearchHit,
} from '../features/library/model';

const { t } = useI18n();
const $q = useQuasar();
const route = useRoute();
const router = useRouter();

const STATUS_ICON: Record<string, string> = {
  indexed: 'check_circle',
  pending: 'hourglass_top',
  indexing: 'hourglass_top',
  error: 'error_outline',
};
// shortcut: 文件名搜索每个资料库只拉前 500 份文档在前端过滤；资料库更大时改成后端按名字查
const NAME_INDEX_LIMIT = 500;
const UPLOAD_MAX_MB = 50;

const collections = ref<KnowledgeCollection[]>([]);
const pageState = ref<'loading' | 'ready' | 'error' | 'forbidden'>('loading');
const refreshing = ref(false);
const localEnabled = ref(false);
const space = ref<LibrarySpace>('mine');
const libId = ref('');
const prefix = ref('');
const nodes = ref<VaultTreeNode[]>([]);
const listLoading = ref(false);
const selectedDocId = ref('');
const previewDirty = ref(false);
const treeKids = reactive<Record<string, VaultTreeNode[]>>({});
const expanded = reactive<Record<string, boolean>>({});
const wizardOpen = ref(false);
const fileInput = ref<HTMLInputElement | null>(null);
const dragDepth = ref(0);
const now = ref(new Date());

const q = ref('');
const semantic = ref(false);
const searching = ref(false);
const searchError = ref('');
const hits = ref<SearchHit[]>([]);
const docCache = new Map<string, KnowledgeDocument[]>();

const nameDlg = reactive({ open: false, kind: 'folder' as 'folder' | 'md', name: '', busy: false, error: '' });
const moveDlg = reactive({
  open: false,
  path: '',
  dirs: [] as Array<{ name: string; path: string }>,
  loading: false,
  busy: false,
  conflict: false,
});

const otherSpace = computed<LibrarySpace>(() => (space.value === 'mine' ? 'company' : 'mine'));
const spaceLibs = computed(() => collections.value.filter((c) => spaceOf(c) === space.value));
const currentLib = computed(() => collections.value.find((c) => c.id === libId.value) ?? null);
const localLib = computed(() => !!currentLib.value && isLocalLibrary(currentLib.value));
const sortedNodes = computed(() => sortNodes(nodes.value));
const selectedNode = computed(
  () => nodes.value.find((n) => n.kind === 'file' && n.doc_id === selectedDocId.value) ?? null,
);
const crumbs = computed(() => crumbsOf(prefix.value));
const parentDir = computed(() => parentOf(prefix.value));
const dirLabel = computed(() =>
  crumbs.value.length ? crumbs.value[crumbs.value.length - 1].name : (currentLib.value?.name ?? ''),
);

type TreeRow = {
  key: string;
  libId: string;
  path: string;
  label: string;
  title: string;
  depth: number;
  expandable: boolean;
};

const treeRows = computed<TreeRow[]>(() => {
  const rows: TreeRow[] = [];
  const walk = (lib: KnowledgeCollection, path: string, depth: number) => {
    for (const d of sortNodes(treeKids[`${lib.id}|${path}`] ?? [])) {
      const key = `${lib.id}|${d.path}`;
      rows.push({ key, libId: lib.id, path: d.path, label: d.name, title: d.path, depth, expandable: true });
      if (expanded[key]) walk(lib, d.path, depth + 1);
    }
  };
  for (const lib of spaceLibs.value) {
    const key = `${lib.id}|`;
    rows.push({
      key,
      libId: lib.id,
      path: '',
      label: lib.name,
      title: isLocalLibrary(lib) ? lib.root_path : t('library.nav.teamStore'),
      depth: 0,
      expandable: isLocalLibrary(lib),
    });
    if (expanded[key]) walk(lib, '', 1);
  }
  return rows;
});

function parentOf(path: string): string {
  return dirOf(path.replace(/\/$/, ''));
}

function spaceCount(s: LibrarySpace): number {
  return collections.value.filter((c) => spaceOf(c) === s).length;
}

function statusText(s: string): string {
  return ['indexed', 'pending', 'indexing', 'error'].includes(s) ? t(`library.status.${s}`) : s;
}

function hitWhere(h: SearchHit): string {
  const lib = collections.value.find((c) => c.id === h.collectionId);
  const dir = dirOf(h.relPath).replace(/\/$/, '').split('/').filter(Boolean).join(' / ');
  return [lib ? t(`library.space.${spaceOf(lib)}`) : '', lib?.name ?? '', dir].filter(Boolean).join(' / ');
}

// ——— 地址栏：?space=&lib=&dir=&file=（file 是 doc id），刷新和分享都能回到同一处 ———
function syncQuery() {
  const query: Record<string, string> = { space: space.value };
  if (libId.value) query.lib = libId.value;
  if (prefix.value) query.dir = prefix.value;
  if (selectedDocId.value) query.file = selectedDocId.value;
  void router.replace({ query });
}

async function guardLeave(): Promise<boolean> {
  if (!previewDirty.value) return true;
  const ok = await confirm({
    title: t('library.preview.leaveTitle'),
    message: t('library.preview.leaveMsg'),
    tone: 'danger',
    confirmLabel: t('library.preview.leaveOk'),
  });
  if (ok) previewDirty.value = false;
  return ok;
}

onBeforeRouteLeave(() => guardLeave());

async function loadCollections() {
  const res = await listCollections({ limit: 200 });
  collections.value = res.items;
}

// 这一层有「整理中」的文件时隔 4 秒静默刷新，同一位置最多 20 次
let pollTimer: ReturnType<typeof setTimeout> | undefined;
let pollKey = '';
let pollLeft = 0;

function schedulePoll(key: string, items: VaultTreeNode[]) {
  clearTimeout(pollTimer);
  if (key !== pollKey) {
    pollKey = key;
    pollLeft = 20;
  }
  if (pollLeft <= 0 || !items.some((n) => n.status === 'pending' || n.status === 'indexing')) return;
  pollLeft -= 1;
  pollTimer = setTimeout(() => void loadNodes(true), 4000);
}

async function loadNodes(quiet = false) {
  if (!libId.value) {
    nodes.value = [];
    return;
  }
  const lib = libId.value;
  const dir = prefix.value;
  if (!quiet) listLoading.value = true;
  try {
    const items = await listVaultTree(lib, dir);
    if (lib !== libId.value || dir !== prefix.value) return;
    nodes.value = items;
    treeKids[`${lib}|${dir}`] = items.filter((n) => n.kind === 'dir');
    now.value = new Date();
    schedulePoll(`${lib}|${dir}`, items);
  } catch (e) {
    if (quiet) return;
    nodes.value = [];
    $q.notify({ type: 'negative', message: parseKratosApiError(e).message });
  } finally {
    if (!quiet) listLoading.value = false;
  }
}

async function selectLibrary(id: string, dir = '', fileId = '') {
  if (!(await guardLeave())) return;
  const lib = collections.value.find((c) => c.id === id);
  if (lib) space.value = spaceOf(lib);
  libId.value = id;
  prefix.value = dir;
  selectedDocId.value = fileId;
  expanded[`${id}|`] = true;
  // 展开到当前目录，左边的树和中间的位置对得上
  for (const c of crumbsOf(dir)) {
    expanded[`${id}|${c.path}`] = true;
    if (!treeKids[`${id}|${parentOf(c.path)}`]) void loadTreeKids(id, parentOf(c.path));
  }
  syncQuery();
  await loadNodes();
}

async function loadTreeKids(id: string, path: string) {
  try {
    treeKids[`${id}|${path}`] = (await listVaultTree(id, path)).filter((n) => n.kind === 'dir');
  } catch {
    treeKids[`${id}|${path}`] = [];
  }
}

async function toggleTree(row: TreeRow) {
  if (!row.expandable) return;
  expanded[row.key] = !expanded[row.key];
  if (expanded[row.key] && !treeKids[row.key]) await loadTreeKids(row.libId, row.path);
}

function openTreeRow(row: TreeRow) {
  void selectLibrary(row.libId, row.path);
}

function goDir(path: string) {
  if (currentLib.value) void selectLibrary(currentLib.value.id, path);
}

async function openNode(n: VaultTreeNode) {
  if (n.kind === 'dir') return goDir(n.path);
  if (n.doc_id === selectedDocId.value || !(await guardLeave())) return;
  selectedDocId.value = n.doc_id;
  syncQuery();
}

function switchSpace(s: LibrarySpace) {
  if (s === space.value) return;
  space.value = s;
  const first = collections.value.find((c) => spaceOf(c) === s);
  if (first) void selectLibrary(first.id);
  else {
    libId.value = '';
    prefix.value = '';
    selectedDocId.value = '';
    nodes.value = [];
    syncQuery();
  }
}

async function moveLibrary(id: string) {
  const target = otherSpace.value;
  try {
    const updated = await updateCollectionSpace(id, target);
    collections.value = collections.value.map((c) => (c.id === id ? updated : c));
    $q.notify({ type: 'positive', message: t('library.nav.moved', { space: t(`library.space.${target}`) }) });
    if (id === libId.value) {
      space.value = target;
      syncQuery();
    }
  } catch (e) {
    $q.notify({ type: 'negative', message: parseKratosApiError(e).message });
  }
}

async function onLibraryCreated(c: KnowledgeCollection) {
  $q.notify({ type: 'positive', message: t('library.wizard.created', { name: c.name }) });
  await loadCollections();
  await selectLibrary(c.id);
}

async function refreshAll() {
  refreshing.value = true;
  docCache.clear();
  try {
    await loadCollections();
    pageState.value = 'ready';
    if (libId.value && !currentLib.value) libId.value = '';
    await loadNodes();
  } catch (e) {
    pageState.value = isForbiddenApiError(e) ? 'forbidden' : 'error';
  } finally {
    refreshing.value = false;
  }
}

// ——— 搜索：文件名（前端过滤）+ 内容（关键字或按意思，跨全部资料库），结果按文件聚合 ———
let searchTimer: ReturnType<typeof setTimeout> | undefined;
let searchSeq = 0;

async function docsOf(id: string): Promise<KnowledgeDocument[]> {
  if (!docCache.has(id)) {
    const res = await listDocuments(id, { limit: NAME_INDEX_LIMIT }).catch(() => ({
      items: [] as KnowledgeDocument[],
    }));
    docCache.set(id, res.items);
  }
  return docCache.get(id)!;
}

async function runSearch() {
  const query = q.value.trim();
  const seq = ++searchSeq;
  if (!query) {
    hits.value = [];
    return;
  }
  searching.value = true;
  searchError.value = '';
  try {
    // 不传 collection_id 的联邦检索只路由到最像的一个库，资料页要两个空间都搜，所以逐库查再合并
    const hybrid = semantic.value ? '' : 'sparse';
    const [lists, perLib] = await Promise.all([
      Promise.all(collections.value.map((c) => docsOf(c.id))),
      Promise.all(
        collections.value.map((c) =>
          searchKnowledge({ collection_id: c.id, query, top_k: 10, hybrid_search: hybrid }).catch(() => []),
        ),
      ),
    ]);
    const chunks = perLib.flat();
    const all = lists.flat();
    const byId = new Map(all.map((d) => [d.id, d]));
    const missing = [...new Set(chunks.map((c) => c.doc_id))].filter((id) => !byId.has(id));
    const extra = await Promise.all(missing.map((id) => getDocument(id).catch(() => null)));
    for (const d of extra) if (d) byId.set(d.id, d);
    if (seq !== searchSeq) return;
    hits.value = mergeSearchHits(
      all.filter((d) => nameMatches(query, d)),
      chunks,
      byId,
    );
  } catch {
    if (seq === searchSeq) searchError.value = t('library.search.failed');
  } finally {
    if (seq === searchSeq) searching.value = false;
  }
}

watch([q, semantic], () => {
  clearTimeout(searchTimer);
  searchTimer = setTimeout(() => void runSearch(), 300);
});

async function openHit(h: SearchHit) {
  await selectLibrary(h.collectionId, dirOf(h.relPath), h.docId);
}

// ——— 新建文件夹 / Markdown ———
function openNameDialog(kind: 'folder' | 'md') {
  Object.assign(nameDlg, { open: true, kind, name: '', busy: false, error: '' });
}

async function submitName() {
  const name = nameDlg.name.trim().replace(/\.md$/i, '');
  if (!validFileName(name)) {
    nameDlg.error = t('library.dialog.invalidName');
    return;
  }
  if (!currentLib.value) return;
  nameDlg.busy = true;
  nameDlg.error = '';
  try {
    if (nameDlg.kind === 'folder') {
      await createVaultDir(currentLib.value.id, prefix.value + name);
      delete treeKids[`${currentLib.value.id}|${prefix.value}`];
    } else {
      const doc = await createVaultDocument(currentLib.value.id, `${prefix.value}${name}.md`);
      selectedDocId.value = doc.id;
    }
    nameDlg.open = false;
    $q.notify({ type: 'positive', message: t('library.dialog.created', { name }) });
    docCache.delete(currentLib.value.id);
    await loadNodes();
    syncQuery();
  } catch (e) {
    nameDlg.error = parseKratosApiError(e).status === 409 ? t('library.dialog.exists') : t('library.dialog.failed');
  } finally {
    nameDlg.busy = false;
  }
}

// ——— 上传：拖进列表或从「新建」里选；本机资料库写进当前文件夹 ———
function onDragEnter(e: DragEvent) {
  if (currentLib.value && e.dataTransfer?.types.includes('Files')) dragDepth.value += 1;
}
function onDragLeave() {
  dragDepth.value = Math.max(0, dragDepth.value - 1);
}
function onDrop(e: DragEvent) {
  dragDepth.value = 0;
  if (e.dataTransfer?.files.length) void uploadFiles(Array.from(e.dataTransfer.files));
}
function onPickFiles(e: Event) {
  const input = e.target as HTMLInputElement;
  if (input.files?.length) void uploadFiles(Array.from(input.files));
  input.value = '';
}

async function uploadFiles(files: File[]) {
  const lib = currentLib.value;
  if (!lib) return;
  const targetDir = isLocalLibrary(lib) ? prefix.value || '/' : '';
  const dismiss = $q.notify({
    type: 'ongoing',
    message: t('library.upload.uploading', { n: files.length }),
    timeout: 0,
  });
  let ok = 0;
  for (const f of files) {
    if (f.size > UPLOAD_MAX_MB * 1024 * 1024) {
      $q.notify({
        type: 'negative',
        message: t('library.upload.failed', {
          name: f.name,
          reason: t('library.upload.tooLarge', { mb: UPLOAD_MAX_MB }),
        }),
      });
      continue;
    }
    try {
      await ingestDocument({
        collection_id: lib.id,
        source: f.name,
        mime_type: inferUploadMime(f),
        content_base64: await readFileAsBase64(f),
        // 文件资料库只抽文字、不让大模型改写正文（也免得内部文件被发给外部模型）
        organize_to_markdown: false,
        target_dir: targetDir || undefined,
      });
      ok += 1;
    } catch (err) {
      const e = parseKratosApiError(err);
      const reason = e.status === 409 ? t('library.upload.exists') : e.message;
      $q.notify({ type: 'negative', message: t('library.upload.failed', { name: f.name, reason }) });
    }
  }
  dismiss();
  if (ok) $q.notify({ type: 'positive', message: t('library.upload.done', { n: ok }) });
  docCache.delete(lib.id);
  await loadNodes();
}

// ——— 移动（同一资料库内换文件夹） ———
async function browseMove(path: string) {
  if (!currentLib.value) return;
  moveDlg.path = path;
  moveDlg.loading = true;
  moveDlg.conflict = false;
  try {
    moveDlg.dirs = sortNodes((await listVaultTree(currentLib.value.id, path)).filter((n) => n.kind === 'dir')).map(
      (d) => ({
        name: d.name,
        path: d.path,
      }),
    );
  } catch {
    moveDlg.dirs = [];
  } finally {
    moveDlg.loading = false;
  }
}

function openMove() {
  moveDlg.open = true;
  void browseMove(dirOf(selectedNode.value?.path ?? ''));
}

async function submitMove(policy: '' | 'overwrite' | 'rename') {
  const node = selectedNode.value;
  if (!node || !currentLib.value) return;
  moveDlg.busy = true;
  try {
    await moveDocumentToDir(node.doc_id, moveDlg.path || '/', policy);
    moveDlg.open = false;
    $q.notify({
      type: 'positive',
      message: t('library.dialog.moved', { dir: moveDlg.path.replace(/\/$/, '') || t('library.dialog.root') }),
    });
    docCache.delete(currentLib.value.id);
    await selectLibrary(currentLib.value.id, moveDlg.path, node.doc_id);
  } catch (e) {
    if (parseKratosApiError(e).status === 409) moveDlg.conflict = true;
    else $q.notify({ type: 'negative', message: t('library.dialog.moveFailed') });
  } finally {
    moveDlg.busy = false;
  }
}

// ——— 删除 ———
async function removeSelected() {
  const node = selectedNode.value;
  if (!node || !currentLib.value) return;
  const ok = await confirm({
    title: t('library.dialog.removeTitle', { name: node.name }),
    message: localLib.value ? t('library.dialog.removeLocal') : t('library.dialog.removeTeam'),
    tone: 'danger',
    confirmLabel: t('library.dialog.removeOk'),
  });
  if (!ok) return;
  try {
    await deleteDocument(node.doc_id);
    $q.notify({ type: 'positive', message: t('library.dialog.removed', { name: node.name }) });
    previewDirty.value = false;
    selectedDocId.value = '';
    docCache.delete(currentLib.value.id);
    syncQuery();
    await loadNodes();
  } catch {
    $q.notify({ type: 'negative', message: t('library.dialog.removeFailed') });
  }
}

// ——— 进页 ———
async function applyRoute() {
  const query = route.query;
  // 对话里的引用跳链带 line / evidence_kind：要精确定位到段落，交给 Harness 的知识库页
  if (typeof query.doc === 'string' && (query.line || query.evidence_kind)) {
    void router.replace({ path: '/knowledge/notes', query });
    return;
  }
  if (typeof query.doc === 'string') {
    try {
      const d = await getDocument(query.doc);
      await selectLibrary(d.collection_id, dirOf(d.rel_path), d.id);
      return;
    } catch {
      // 文档没了或没权限：落到默认位置
    }
  }
  const lib = typeof query.lib === 'string' ? collections.value.find((c) => c.id === query.lib) : undefined;
  if (lib) {
    await selectLibrary(
      lib.id,
      typeof query.dir === 'string' ? query.dir : '',
      typeof query.file === 'string' ? query.file : '',
    );
    return;
  }
  // 地址里点名了空间就停在那（哪怕是空的）；没点名时，「我的资料」空着就落到有资料库的空间
  const named = query.space === 'company' || query.space === 'mine' ? query.space : '';
  const wanted = named || space.value;
  const first = collections.value.find((c) => spaceOf(c) === wanted) ?? (named ? undefined : collections.value[0]);
  if (first) await selectLibrary(first.id);
  else space.value = wanted;
}

onMounted(async () => {
  void fetchLocalRevealEnabled().then((v) => (localEnabled.value = v));
  try {
    await loadCollections();
    pageState.value = 'ready';
    await applyRoute();
  } catch (e) {
    pageState.value = isForbiddenApiError(e) ? 'forbidden' : 'error';
  }
});

onBeforeUnmount(() => {
  clearTimeout(searchTimer);
  clearTimeout(pollTimer);
});
</script>
