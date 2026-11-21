<template>
  <!-- 回收站（整个资料库删掉的文件）/ 历史版本（一个文件被改写前的旧版本）：都来自本机资料库的 .aranea/trash -->
  <AppDialog
    :model-value="modelValue"
    size="lg"
    position="standard"
    :title="history ? t('library.trash.historyTitle', { name: history.name }) : t('library.trash.title')"
    :subtitle="history ? t('library.trash.historyHint') : t('library.trash.hint')"
    :loading="busy"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="lb-trash">
      <div class="lb-trash__list" role="listbox" :aria-label="t('library.trash.title')">
        <p v-if="!busy && !items.length" class="lb-hint lb-trash__empty">
          {{ history ? t('library.trash.noHistory') : t('library.trash.empty') }}
        </p>
        <button
          v-for="it in items"
          :key="it.path"
          type="button"
          role="option"
          class="lb-trash__it"
          :class="{ 'lb-trash__it--on': it.path === picked?.path }"
          :aria-selected="it.path === picked?.path"
          @click="pick(it)"
        >
          <span class="lb-fi lb-fi--sm" :class="`lb-fi--${fileKind(it.orig_path)}`" aria-hidden="true">{{
            KIND_LABEL[fileKind(it.orig_path)]
          }}</span>
          <span class="lb-trash__name">
            <b>{{ history ? fullTime(it.modified_at) : baseName(it.orig_path) }}</b>
            <small class="lb-mono">{{ history ? fileSize(it.size_bytes) : it.orig_path }}</small>
          </span>
          <span v-if="!history" class="lb-hint lb-mono">{{ fullTime(it.modified_at) }}</span>
        </button>
      </div>
      <div class="lb-trash__pv">
        <p v-if="!picked" class="lb-hint">{{ t('library.trash.pick') }}</p>
        <template v-else>
          <pre v-if="text !== null" class="lb-trash__text">{{ text }}</pre>
          <p v-else class="lb-hint">{{ t('library.trash.binary', { size: fileSize(picked.size_bytes) }) }}</p>
        </template>
      </div>
    </div>
    <template #actions>
      <q-btn
        v-if="!history"
        flat
        no-caps
        color="negative"
        icon="delete_forever"
        :label="t('library.trash.empty_all')"
        :disable="!items.length"
        @click="purge([])"
      />
      <q-space />
      <q-btn
        flat
        no-caps
        color="negative"
        :label="t('library.trash.purge')"
        :disable="!picked"
        @click="picked && purge([picked.path])"
      />
      <q-btn
        unelevated
        no-caps
        color="primary"
        icon="restore"
        :label="history && isMd ? t('library.trash.useVersion') : t('library.trash.restore')"
        :disable="!picked"
        @click="restore"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppDialog from '../../../components/common/AppDialog.vue';
import { confirm } from '../../../composables/useAppConfirm';
import {
  getDocumentContent,
  getVaultTrashContent,
  listVaultTrash,
  purgeVaultTrash,
  restoreVaultTrash,
  updateDocumentContent,
  type VaultTrashItem,
} from '../../knowledge/api';
import { fileSize, fullTime } from '../../mail/model';
import { baseName, fileKind, KIND_LABEL } from '../model';

const props = defineProps<{
  modelValue: boolean;
  collectionId: string;
  /** 有 = 看这个文件的历史版本；无 = 整个回收站 */
  history?: { docId: string; relPath: string; name: string } | null;
}>();
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; changed: [] }>();

const { t } = useI18n();
const $q = useQuasar();

const items = ref<VaultTrashItem[]>([]);
const picked = ref<VaultTrashItem | null>(null);
const text = ref<string | null>(null);
const busy = ref(false);
const TEXT_KINDS = new Set(['md', 'txt']);
const isMd = computed(() => !!props.history && fileKind(props.history.name) === 'md');

function fail(e: unknown) {
  $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
}

async function load() {
  busy.value = true;
  picked.value = null;
  text.value = null;
  try {
    items.value = await listVaultTrash(props.collectionId, props.history?.relPath ?? '');
  } catch (e) {
    items.value = [];
    fail(e);
  } finally {
    busy.value = false;
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) void load();
  },
);

async function pick(it: VaultTrashItem) {
  picked.value = it;
  text.value = null;
  if (!TEXT_KINDS.has(fileKind(it.orig_path))) return;
  try {
    const c = await getVaultTrashContent(props.collectionId, it.path);
    if (picked.value?.path === it.path) text.value = c;
  } catch (e) {
    fail(e);
  }
}

/** 恢复：Markdown 的历史版本直接替换当前内容（当前的会先备份）；其他放回原处（重名留两份）。 */
async function restore() {
  const it = picked.value;
  if (!it) return;
  busy.value = true;
  try {
    if (props.history && isMd.value && text.value !== null) {
      const cur = await getDocumentContent(props.history.docId);
      await updateDocumentContent(props.history.docId, text.value, cur.base_hash);
      $q.notify({ type: 'positive', message: t('library.trash.versionUsed') });
    } else {
      const rel = await restoreVaultTrash(props.collectionId, it.path);
      $q.notify({ type: 'positive', message: t('library.trash.restored', { path: rel }) });
    }
    emit('changed');
    await load();
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
  }
}

async function purge(paths: string[]) {
  const ok = await confirm({
    title: paths.length ? t('library.trash.purgeTitle') : t('library.trash.emptyTitle'),
    message: paths.length ? t('library.trash.purgeBody') : t('library.trash.emptyBody', { n: items.value.length }),
    tone: 'danger',
    confirmLabel: t('library.trash.purge'),
  });
  if (!ok) return;
  busy.value = true;
  try {
    const n = await purgeVaultTrash(props.collectionId, paths);
    $q.notify({ message: t('library.trash.purged', { n }) });
    await load();
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
  }
}
</script>
