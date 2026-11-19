// Container: approved — 「笔记」Tab：kind=note 的 user/workspace 域列表 // （spec §6.3：Quasar 重写 wb Memory.vue 双
tab 形态）。 // 自包含本地状态：不经 memoryStore.loadFacts（避免与 browse Tab 共享 facts // 状态的竞态，见
__tests__/memoryStoreRace.spec.ts），直接调 listMemoryFacts。 // 无权限域不可见由服务端 scope ACL
保证（internal/service/memory_scope.go // authorizeMemoryScope）：scope_type=user 强制 caller uid、workspace 强制 //
caller workspace，跨用户/跨租户访问直接 Forbidden。
<template>
  <div class="column q-gutter-md">
    <q-banner v-if="!endpointReady" rounded class="memory-info-banner">
      {{ t('memory.notes.unavailableBanner') }}
    </q-banner>

    <q-card flat bordered class="memory-card">
      <div class="row items-center q-gutter-x-sm q-px-md q-pt-md">
        <span class="text-caption text-grey-7">{{ t('memory.notes.scopeLabel') }}</span>
        <q-chip
          v-for="opt in scopeOptions"
          :key="opt.value"
          clickable
          dense
          :outline="scope !== opt.value"
          color="primary"
          @click="scope = opt.value"
        >
          {{ opt.label }}
        </q-chip>
      </div>

      <q-card-section class="row items-center q-gutter-sm">
        <q-input
          v-model="keyword"
          class="col-grow"
          dense
          outlined
          clearable
          debounce="300"
          :label="t('memory.notes.searchPlaceholder')"
          @update:model-value="onKeywordInput"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-btn
          unelevated
          rounded
          no-caps
          color="primary"
          icon="refresh"
          :label="t('memory.notes.refresh')"
          :loading="loading"
          @click="load"
        />
      </q-card-section>

      <q-card-section v-if="!loading && rows.length === 0" class="app-registry-empty app-empty-state-center">
        <q-icon name="sticky_note_2" size="44px" color="grey-6" />
        <div class="text-subtitle1 q-mt-sm">{{ t('memory.notes.emptyTitle') }}</div>
        <div class="text-caption text-grey-7">{{ t('memory.notes.emptyCaption') }}</div>
      </q-card-section>

      <template v-else>
        <q-list separator>
          <q-item v-for="row in rows" :key="row.id">
            <q-item-section>
              <q-item-label class="memory-note-statement ellipsis-2-lines">{{ row.statement }}</q-item-label>
              <q-item-label caption>
                <q-chip v-if="row.status && row.status !== 'active'" dense square size="sm" outline color="blue-grey">
                  {{ t(`memory.knowledge.status.${row.status}`) }}
                </q-chip>
                <span>{{ formatDate(row.created_at) }}</span>
              </q-item-label>
            </q-item-section>
          </q-item>
        </q-list>

        <q-card-section v-if="pageMax > 1" class="row items-center justify-end q-gutter-sm">
          <span class="text-caption text-grey-7">{{ t('memory.notes.total', { count: filteredCount }) }}</span>
          <q-pagination
            v-model="page"
            :max="pageMax"
            :max-pages="6"
            boundary-links
            direction-links
            dense
            color="primary"
          />
        </q-card-section>
      </template>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { listMemoryFacts } from '../api';
import type { MemoryFact } from '../types';

type NotesScope = 'user' | 'workspace';

const { t } = useI18n();

/** 笔记域（wb 双 tab 形态）：个人 = user 域；公司空间 = workspace 域。 */
const scope = ref<NotesScope>('user');
const keyword = ref('');
const status = ref('active');
const rows = ref<MemoryFact[]>([]);
const total = ref(0);
const filteredCount = ref(0);
const loading = ref(false);
const endpointReady = ref(true);
const page = ref(1);
const pageSize = 10;
const pageMax = computed(() => Math.max(1, Math.ceil(filteredCount.value / pageSize)));
let loadSeq = 0;

const scopeOptions = computed(() => [
  { value: 'user' as NotesScope, label: t('memory.notes.scopePersonal') },
  { value: 'workspace' as NotesScope, label: t('memory.notes.scopeWorkspace') },
]);

watch([scope, page], () => {
  void load();
});

function onKeywordInput() {
  if (page.value !== 1) {
    page.value = 1;
    return;
  }
  void load();
}

async function load() {
  const seq = ++loadSeq;
  loading.value = true;
  try {
    const result = await listMemoryFacts({
      kind: 'note',
      scope_type: scope.value,
      status: status.value || undefined,
      keyword: keyword.value || undefined,
      limit: pageSize,
      offset: (page.value - 1) * pageSize,
    });
    if (seq !== loadSeq) return;
    rows.value = result.items ?? [];
    total.value = result.total;
    filteredCount.value = result.filtered_count ?? result.total;
    endpointReady.value = true;
    if (page.value > pageMax.value) {
      page.value = pageMax.value;
    }
  } catch {
    if (seq !== loadSeq) return;
    rows.value = [];
    total.value = 0;
    filteredCount.value = 0;
    endpointReady.value = false;
  } finally {
    if (seq === loadSeq) loading.value = false;
  }
}

function formatDate(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

void load();
</script>

<style scoped>
.memory-note-statement {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
