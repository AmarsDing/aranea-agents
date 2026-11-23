<template>
  <q-btn flat dense round icon="folder_special" :aria-label="t('cap.coding.tool.workspace.title')">
    <q-badge v-if="projects.length" floating rounded color="primary" :label="projects.length" />
    <q-tooltip>{{ t('cap.coding.tool.workspace.hint') }}</q-tooltip>
    <q-menu anchor="bottom right" self="top right" class="ws-pick" @before-show="load">
      <header class="ws-pick__head">
        <strong>{{ t('cap.coding.tool.workspace.title') }}</strong>
        <q-icon name="help_outline" size="16px" class="ws-pick__help" :aria-label="t('cap.coding.tool.workspace.hint')">
          <q-tooltip max-width="280px">{{ t('cap.coding.tool.workspace.hint') }}</q-tooltip>
        </q-icon>
        <q-space />
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="open_in_new"
          :to="{ path: '/knowledge' }"
          :aria-label="t('cap.coding.tool.workspace.open')"
        >
          <q-tooltip>{{ t('cap.coding.tool.workspace.open') }}</q-tooltip>
        </q-btn>
      </header>
      <q-linear-progress v-if="loading" indeterminate color="primary" />
      <p v-if="error" class="ws-pick__note ws-pick__note--alert" role="alert">{{ error }}</p>
      <q-list dense class="ws-pick__list">
        <q-item v-for="c in vaults" :key="c.id" tag="label" clickable :disable="busy !== ''">
          <q-item-section side>
            <q-checkbox
              dense
              :model-value="!!projectFor(c.root_path)"
              :disable="busy !== ''"
              :aria-label="c.name"
              @update:model-value="toggle(c, Boolean($event))"
            />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ c.name }}</q-item-label>
            <q-item-label caption class="ws-pick__path" :title="c.root_path">{{ c.root_path }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-icon :name="c.space === 'company' ? 'apartment' : 'person'" size="16px">
              <q-tooltip>{{ t(`cap.coding.tool.workspace.${c.space === 'company' ? 'company' : 'mine'}`) }}</q-tooltip>
            </q-icon>
          </q-item-section>
        </q-item>
        <!-- 早先手填登记、资料里没有对应库的：只能取消 -->
        <q-item v-for="p in orphans" :key="p.id" tag="label" clickable :disable="busy !== ''">
          <q-item-section side>
            <q-checkbox
              dense
              :model-value="true"
              :disable="busy !== ''"
              :aria-label="p.name"
              @update:model-value="remove(p)"
            />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ p.name }}</q-item-label>
            <q-item-label caption class="ws-pick__path" :title="p.path">{{ p.path }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-icon name="link_off" size="16px">
              <q-tooltip>{{ t('cap.coding.tool.workspace.notInKnowledge') }}</q-tooltip>
            </q-icon>
          </q-item-section>
        </q-item>
      </q-list>
      <div v-if="!loading && vaults.length === 0" class="ws-pick__note">
        {{ t('cap.coding.tool.workspace.empty') }}
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          :to="{ path: '/knowledge' }"
          :label="t('cap.coding.tool.workspace.add')"
        />
      </div>
    </q-menu>
  </q-btn>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { CodingProject } from '../api';
import { deleteCodingProject, upsertCodingProject } from '../api';
import { listCollections } from '../../knowledge/api';
import type { KnowledgeCollection } from '../../knowledge/types';
import { errText } from '../../fleet/api';

// 工作空间 = 资料里的本机资料库（有本地目录的）；勾上即登记为工具与 Agent 可执行的目录，不手填路径
const props = defineProps<{ projects: CodingProject[] }>();
const emit = defineEmits<{ changed: [] }>();
const { t } = useI18n();

const collections = ref<KnowledgeCollection[]>([]);
const loading = ref(false);
const busy = ref('');
const error = ref('');

const norm = (p: string) =>
  p
    .replace(/[\\/]+$/, '')
    .replace(/\//g, '\\')
    .toLowerCase();
const vaults = computed(() => collections.value.filter((c) => c.vault_backend === 'local' && c.root_path));
const projectFor = (path: string) => props.projects.find((p) => norm(p.path ?? '') === norm(path));
const orphans = computed(() =>
  props.projects.filter((p) => !vaults.value.some((c) => norm(c.root_path) === norm(p.path ?? ''))),
);

async function load() {
  loading.value = true;
  error.value = '';
  try {
    collections.value = (await listCollections({ limit: 200 })).items;
  } catch (e) {
    error.value = errText(e);
  } finally {
    loading.value = false;
  }
}

// 目录登记按名称去重：资料库重名时加序号，免得覆盖另一个目录
function uniqueName(name: string) {
  const taken = new Set(props.projects.map((p) => p.name));
  let n = name;
  for (let i = 2; taken.has(n); i++) n = `${name} (${i})`;
  return n;
}

async function toggle(c: KnowledgeCollection, on: boolean) {
  const existing = projectFor(c.root_path);
  if (!on) return existing && remove(existing);
  busy.value = c.id;
  error.value = '';
  try {
    await upsertCodingProject({ name: uniqueName(c.name), path: c.root_path, description: c.description });
    emit('changed');
  } catch (e) {
    error.value = errText(e);
  } finally {
    busy.value = '';
  }
}

async function remove(p: CodingProject) {
  busy.value = p.id ?? 'x';
  error.value = '';
  try {
    await deleteCodingProject(p.id ?? '');
    emit('changed');
  } catch (e) {
    error.value = errText(e);
  } finally {
    busy.value = '';
  }
}
</script>

<style lang="sass">
// q-menu 渲染到 body 外层，不能用 scoped
.ws-pick
  width: min(420px, calc(100vw - 32px))
  padding: var(--nt-space-2)
.ws-pick__head
  display: flex
  align-items: center
  gap: var(--nt-space-1)
  padding: var(--nt-space-1) var(--nt-space-2)
  color: var(--nt-text-1)
.ws-pick__help
  color: var(--nt-text-3)
.ws-pick__list
  max-height: 50vh
  overflow: auto
.ws-pick__path
  overflow: hidden
  font-family: var(--nt-font-mono)
  text-overflow: ellipsis
  white-space: nowrap
.ws-pick__note
  margin: 0
  padding: var(--nt-space-2)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
.ws-pick__note--alert
  color: var(--nt-danger)
</style>
