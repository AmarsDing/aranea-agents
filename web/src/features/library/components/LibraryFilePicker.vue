<template>
  <!-- 从资料里选文件（写信加附件用）：选资料库 → 按名字筛 → 勾选几个 → 取回原文件 -->
  <AppDialog
    :model-value="modelValue"
    size="md"
    position="standard"
    :title="t('library.picker.title')"
    :loading="loading"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="lb-pick">
      <div class="lb-pick__bar">
        <q-select
          v-model="libId"
          dense
          outlined
          emit-value
          map-options
          class="lb-pick__lib"
          :options="libOptions"
          :label="t('library.picker.library')"
        />
        <q-input
          v-model="q"
          dense
          outlined
          clearable
          class="lb-pick__q"
          :placeholder="t('library.picker.filter')"
          @update:model-value="q = String($event ?? '')"
        >
          <template #prepend><q-icon name="search" size="18px" /></template>
        </q-input>
      </div>
      <div class="lb-pick__list" role="listbox" aria-multiselectable="true" :aria-label="t('library.picker.title')">
        <p v-if="!loading && !shown.length" class="lb-hint">{{ t('library.picker.none') }}</p>
        <label v-for="d in shown" :key="d.id" class="lb-pick__item">
          <q-checkbox v-model="chosen" dense size="sm" :val="d.id" />
          <span class="lb-fi lb-fi--sm" :class="`lb-fi--${fileKind(name(d))}`" aria-hidden="true">{{
            KIND_LABEL[fileKind(name(d))]
          }}</span>
          <span class="lb-pick__name" :title="d.rel_path || d.source">{{ name(d) }}</span>
          <span class="lb-hint lb-mono">{{ fileSize(d.size_bytes) }}</span>
        </label>
      </div>
    </div>
    <template #actions>
      <q-btn flat no-caps :label="t('library.dialog.cancel')" @click="emit('update:modelValue', false)" />
      <q-btn
        unelevated
        no-caps
        color="primary"
        :disable="!chosen.length"
        :loading="fetching"
        :label="t('library.picker.add', { n: chosen.length })"
        @click="confirmPick"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppDialog from '../../../components/common/AppDialog.vue';
import { fetchDocumentAsset, listCollections, listDocuments } from '../../knowledge/api';
import type { KnowledgeCollection, KnowledgeDocument } from '../../knowledge/types';
import { fileSize } from '../../mail/model';
import { fileKind, KIND_LABEL, spaceOf } from '../model';

const props = defineProps<{ modelValue: boolean }>();
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; picked: [files: File[]] }>();

const { t } = useI18n();
const $q = useQuasar();

const libs = ref<KnowledgeCollection[]>([]);
const libId = ref('');
const docs = ref<KnowledgeDocument[]>([]);
const q = ref('');
const chosen = ref<string[]>([]);
const loading = ref(false);
const fetching = ref(false);

const libOptions = computed(() =>
  libs.value.map((c) => ({ value: c.id, label: `${t(`library.space.${spaceOf(c)}`)} / ${c.name}` })),
);
const name = (d: KnowledgeDocument) => (d.rel_path || d.source || d.id).split('/').pop() ?? d.id;
const shown = computed(() => {
  const k = q.value.trim().toLowerCase();
  return k ? docs.value.filter((d) => name(d).toLowerCase().includes(k)) : docs.value;
});

watch(
  () => props.modelValue,
  async (open) => {
    if (!open || libs.value.length) return;
    loading.value = true;
    try {
      libs.value = (await listCollections({ limit: 200 })).items;
      libId.value = libs.value[0]?.id ?? '';
    } finally {
      loading.value = false;
    }
  },
);

watch(libId, async (id) => {
  chosen.value = [];
  docs.value = [];
  if (!id) return;
  loading.value = true;
  try {
    docs.value = (await listDocuments(id, { limit: 500 })).items;
  } catch {
    docs.value = [];
  } finally {
    loading.value = false;
  }
});

async function confirmPick() {
  fetching.value = true;
  try {
    const files = await Promise.all(
      chosen.value.map(async (id) => {
        const d = docs.value.find((x) => x.id === id);
        const { blob, filename } = await fetchDocumentAsset(id);
        return new File([blob], filename || (d ? name(d) : id), { type: blob.type });
      }),
    );
    emit('picked', files);
    emit('update:modelValue', false);
    chosen.value = [];
  } catch {
    $q.notify({ type: 'negative', message: t('library.preview.loadFailed') });
  } finally {
    fetching.value = false;
  }
}
</script>
