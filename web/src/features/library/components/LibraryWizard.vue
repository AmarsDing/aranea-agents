<template>
  <!-- 新建资料库：① 放到哪个空间 + 名字 ② 文件放在哪（本机文件夹 / 只存在系统里）。10-资料.md §4「建库向导」 -->
  <AppDialog
    :model-value="modelValue"
    :title="t('library.wizard.title')"
    :subtitle="t('library.wizard.stepOf', { n: step })"
    icon="create_new_folder"
    size="lg"
    position="standard"
    :loading="creating"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="lb-wz">
      <template v-if="step === 1">
        <div class="lb-wz__label">{{ t('library.wizard.step1') }}</div>
        <div class="lb-wz__cards" role="radiogroup" :aria-label="t('library.wizard.step1')">
          <button
            v-for="s in SPACES"
            :key="s"
            type="button"
            role="radio"
            class="lb-wz__card"
            :class="{ 'lb-wz__card--on': space === s }"
            :aria-checked="space === s"
            @click="space = s"
          >
            <q-icon :name="s === 'mine' ? 'person' : 'groups'" size="20px" aria-hidden="true" />
            <b>{{ t(`library.space.${s}`) }}</b>
            <small>{{ t(`library.space.${s}Hint`) }}</small>
          </button>
        </div>
        <q-input
          v-model="name"
          dense
          outlined
          autofocus
          class="lb-wz__name"
          :label="t('library.wizard.name')"
          :placeholder="t('library.wizard.namePlaceholder')"
          :error="nameTouched && !name.trim()"
          :error-message="t('library.wizard.nameRequired')"
          @keydown.enter.prevent="goStep2"
        />
      </template>

      <template v-else>
        <div class="lb-wz__label">{{ t('library.wizard.step2') }}</div>
        <div class="lb-wz__cards" role="radiogroup" :aria-label="t('library.wizard.step2')">
          <button
            v-for="o in STORES"
            :key="o"
            type="button"
            role="radio"
            class="lb-wz__card"
            :class="{ 'lb-wz__card--on': store === o }"
            :aria-checked="store === o"
            @click="store = o"
          >
            <q-icon :name="o === 'local' ? 'folder_open' : 'cloud_queue'" size="20px" aria-hidden="true" />
            <b>{{ t(`library.wizard.${o}`) }}</b>
            <small>{{ t(`library.wizard.${o}Desc`) }}</small>
          </button>
        </div>

        <template v-if="store === 'local'">
          <template v-if="localEnabled">
            <FolderBrowser
              :path="dir.path || t('library.wizard.thisPc')"
              :dirs="dir.dirs"
              :can-up="!!dir.path"
              :loading="browsing"
              :up-label="t('library.wizard.up')"
              :empty-text="browseError || t('library.wizard.noSubdirs')"
              @open="browse"
              @up="browse(dir.parent)"
            />
            <div class="lb-wz__row">
              <q-input
                v-model="mkdirName"
                dense
                outlined
                class="lb-wz__grow"
                :disable="!dir.path"
                :placeholder="t('library.wizard.mkdirName')"
                :aria-label="t('library.wizard.mkdirName')"
                @keydown.enter.prevent="mkdir"
              />
              <q-btn
                flat
                no-caps
                icon="create_new_folder"
                :disable="!dir.path || !validFileName(mkdirName)"
                :label="t('library.wizard.mkdir')"
                @click="mkdir"
              />
              <q-btn
                unelevated
                no-caps
                color="primary"
                :disable="!dir.path"
                :label="t('library.wizard.choose')"
                @click="chosen = dir.path"
              />
            </div>
            <p v-if="chosen" class="lb-wz__chosen" aria-live="polite">
              <q-icon name="check_circle" size="16px" aria-hidden="true" />
              {{ t('library.wizard.chosen', { path: chosen }) }}
            </p>
          </template>
          <template v-else>
            <p class="lb-hint">{{ t('library.wizard.pickerOff') }}</p>
            <q-input
              v-model="chosen"
              dense
              outlined
              class="lb-mono"
              :label="t('library.wizard.pathLabel')"
              :placeholder="t('library.wizard.pathPlaceholder')"
            />
          </template>
        </template>
        <p v-if="error" class="lb-wz__err" role="alert">{{ error }}</p>
      </template>
    </div>

    <template #actions>
      <q-btn v-if="step === 2" flat no-caps :label="t('library.wizard.back')" @click="step = 1" />
      <q-space />
      <q-btn v-if="step === 1" unelevated no-caps color="primary" :label="t('library.wizard.next')" @click="goStep2" />
      <q-btn
        v-else
        unelevated
        no-caps
        color="primary"
        :loading="creating"
        :disable="store === 'local' && !chosen.trim()"
        :label="t('library.wizard.create')"
        @click="create"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../../../components/common/AppDialog.vue';
import FolderBrowser from '../../../components/library/FolderBrowser.vue';
import { createCollection, createLocalDir, listLocalDirs, type LocalDirListing } from '../../knowledge/api';
import type { KnowledgeCollection } from '../../knowledge/types';
import { parseKratosApiError } from '../../../utils/kratosError';
import { SPACES, validFileName, type LibrarySpace } from '../model';

const props = defineProps<{
  modelValue: boolean;
  defaultSpace: LibrarySpace;
  /** 后端开了本机文件夹浏览（FEATURES_LOCAL_REVEAL_ENABLED） */
  localEnabled: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  created: [collection: KnowledgeCollection];
}>();

const { t } = useI18n();
const STORES = ['local', 'team'] as const;

const step = ref<1 | 2>(1);
const space = ref<LibrarySpace>('mine');
const name = ref('');
const nameTouched = ref(false);
const store = ref<(typeof STORES)[number]>('local');
const dir = reactive<LocalDirListing>({ path: '', parent: '', dirs: [] });
const browsing = ref(false);
const browseError = ref('');
const chosen = ref('');
const mkdirName = ref('');
const creating = ref(false);
const error = ref('');

watch(
  () => props.modelValue,
  (open) => {
    if (!open) return;
    step.value = 1;
    space.value = props.defaultSpace;
    name.value = '';
    nameTouched.value = false;
    store.value = 'local';
    chosen.value = '';
    mkdirName.value = '';
    error.value = '';
  },
);

async function browse(path: string) {
  browsing.value = true;
  browseError.value = '';
  try {
    Object.assign(dir, await listLocalDirs(path));
  } catch {
    browseError.value = t('library.wizard.loadFailed');
    dir.dirs = [];
  } finally {
    browsing.value = false;
  }
}

function goStep2() {
  nameTouched.value = true;
  if (!name.value.trim()) return;
  step.value = 2;
  if (props.localEnabled && !dir.dirs.length && !dir.path) void browse('');
}

async function mkdir() {
  if (!dir.path || !validFileName(mkdirName.value)) return;
  error.value = '';
  try {
    const created = await createLocalDir(dir.path, mkdirName.value.trim());
    mkdirName.value = '';
    await browse(created);
    chosen.value = created;
  } catch (e) {
    error.value = t('library.wizard.failed', { reason: parseKratosApiError(e).message });
  }
}

async function create() {
  error.value = '';
  creating.value = true;
  try {
    const c = await createCollection({
      name: name.value.trim(),
      root_path: store.value === 'local' ? chosen.value.trim() : '',
      vault_backend: store.value,
      space: space.value,
    });
    emit('created', c);
    emit('update:modelValue', false);
  } catch (e) {
    const err = parseKratosApiError(e);
    const msg = err.message.toLowerCase();
    if (msg.includes('root_path not found') || msg.includes('not a directory'))
      error.value = t('library.wizard.rootMissing');
    else if (err.status === 409 || msg.includes('duplicate') || msg.includes('already'))
      error.value = t('library.wizard.pathTaken');
    else error.value = t('library.wizard.failed', { reason: err.message });
  } finally {
    creating.value = false;
  }
}
</script>
