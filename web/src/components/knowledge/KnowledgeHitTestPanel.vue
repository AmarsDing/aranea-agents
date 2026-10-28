<!-- G-19 hit-test 面板：同一 query 并行跑 dense/sparse/rrf 三策略对比检索。
     调试调用走后端 DebugKnowledgeSearch——不落 access_log（反复试探不污染热度信号），
     但复用完整检索链路（排序保真）。组件内消息（全局 locale 文件归并行会话维护）。 -->
<template>
  <div class="kht column q-gutter-md">
    <div class="kht__row row q-col-gutter-sm items-end">
      <q-input
        v-model="hit.query.value"
        class="col-12 col-md"
        dense
        outlined
        autofocus
        :label="tt('queryLabel')"
        :placeholder="tt('queryPh')"
        clearable
        @keyup.enter="runAll"
      />
      <q-input
        v-model.number="hit.topK.value"
        class="col-4 col-md-1"
        type="number"
        dense
        outlined
        :label="tt('topk')"
      />
      <q-input v-model="hit.queryPrefix.value" class="col-8 col-md-2" dense outlined :label="tt('prefix')" clearable />
      <q-toggle v-model="hit.useRerank.value" class="col-6 col-md-1" dense :label="tt('rerank')" />
      <q-btn
        unelevated
        no-caps
        color="primary"
        class="col-6 col-md-auto"
        :label="tt('run')"
        :loading="hit.running.value"
        :disable="!collectionId || !hit.query.value.trim()"
        @click="runAll"
      />
    </div>

    <!-- U1-2 用例条：选择回填 / 存为用例 / 删除 / 导出 / 导入 -->
    <div class="kht__cases row q-col-gutter-xs items-center">
      <q-select
        v-model="selectedCaseId"
        class="col-12 col-md-3"
        dense
        outlined
        emit-value
        map-options
        clearable
        :options="caseOptions"
        :label="t('knowledgePage.hitTest.caseSelect')"
        @update:model-value="onPickCase"
      />
      <q-btn
        flat
        dense
        no-caps
        icon="save"
        :label="t('knowledgePage.hitTest.caseSave')"
        :disable="!hit.query.value.trim()"
        @click="openSave"
      />
      <q-btn
        flat
        dense
        no-caps
        icon="delete"
        :label="t('knowledgePage.hitTest.caseDelete')"
        :disable="!selectedCase"
        @click="removeCase"
      />
      <q-btn
        flat
        dense
        no-caps
        icon="download"
        :label="t('knowledgePage.hitTest.caseExport')"
        :disable="!selectedCase"
        @click="exportCurrent"
      />
      <q-btn flat dense no-caps icon="upload" :label="t('knowledgePage.hitTest.caseImport')" @click="triggerImport" />
      <input ref="fileInput" type="file" accept=".json,application/json" class="hidden" @change="onImportFile" />
    </div>
    <div v-if="importError" class="text-caption text-negative">
      {{ t('knowledgePage.hitTest.caseImportBad', { msg: importError }) }}
    </div>

    <div class="kht__hint row items-center q-gutter-xs text-caption">
      <q-icon name="science" size="14px" />
      <span>{{ tt('skipLogHint') }}</span>
    </div>

    <!-- 回归对比：选中用例且至少有两次运行时展示最近两次 topScores delta -->
    <div v-if="selectedCase && selectedCase.runs.length" class="kht__history">
      <div class="text-caption kht__dim">
        {{ t('knowledgePage.hitTest.caseHistory') }} ·
        {{ t('knowledgePage.hitTest.historyRuns', { n: selectedCase.runs.length }) }}
      </div>
      <table v-if="compareRows.length" class="kht__compare text-caption">
        <thead>
          <tr>
            <th>#</th>
            <th>{{ t('knowledgePage.hitTest.prevRun') }} ({{ shortAt(prevRun?.at) }})</th>
            <th>{{ t('knowledgePage.hitTest.latestRun') }} ({{ shortAt(latestRun?.at) }})</th>
            <th>Δ</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in compareRows" :key="r.idx">
            <td class="kht__dim">#{{ r.idx + 1 }}</td>
            <td>{{ fmtMaybe(r.prev) }}</td>
            <td>{{ fmtMaybe(r.latest) }}</td>
            <td :class="deltaClass(r)">{{ deltaText(r) }}</td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="latestRun" class="text-caption kht__dim">
        {{ t('knowledgePage.hitTest.historyOne', { ms: latestRun.durationMs, mode: latestRun.modeUsed }) }}
      </div>
    </div>

    <div v-if="!hit.runs.value.length" class="kht__empty text-caption">{{ tt('empty') }}</div>

    <div v-else class="row q-col-gutter-md">
      <div v-for="row in hit.runs.value" :key="row.strategy" class="col-12 col-md-4 kht__col">
        <div class="kht__col-head row items-center q-gutter-xs">
          <q-badge color="cyan" class="kht__strategy">{{ tt(row.strategy) }}</q-badge>
          <span v-if="!row.loading && !row.error" class="text-caption kht__dim">
            {{ tt('mode', { m: row.result?.mode_used ?? '?' }) }}
          </span>
          <span v-if="!row.loading && row.result" class="text-caption kht__dim">
            {{ row.result.reranked ? tt('reranked') : tt('notReranked') }}
          </span>
          <span v-if="!row.loading" class="text-caption kht__dim kht__elapsed">{{ row.elapsed_ms }} ms</span>
        </div>

        <q-linear-progress v-if="row.loading" indeterminate color="cyan" class="q-mt-xs" />
        <q-banner v-else-if="row.error" rounded class="bg-negative text-white q-mt-xs">
          {{ tt('error', { msg: row.error }) }}
        </q-banner>
        <div v-else-if="row.result && !row.result.chunks.length" class="kht__empty q-mt-xs">{{ tt('noHits') }}</div>

        <div v-else-if="row.result" class="kht__list q-mt-xs">
          <div v-for="(c, i) in row.result.chunks" :key="c.id" class="kht__hit">
            <div class="row items-center q-gutter-xs">
              <span class="kht__rank">#{{ i + 1 }}</span>
              <span v-if="preRank(row, c) !== null" class="kht__prerank text-caption">
                {{ tt('preRank', { n: preRank(row, c) }) }}
              </span>
              <span class="kht__score">{{ scoreText(c.score) }}</span>
              <span class="kht__scores text-caption">{{ breakdown(c) }}</span>
              <span class="kht__doc text-caption ellipsis">{{ c.doc_id || '—' }}</span>
            </div>
            <div class="kht__content text-caption">{{ preview(c.content) }}</div>
          </div>
        </div>
      </div>
    </div>

    <q-dialog v-model="saveDialog" persistent>
      <q-card style="min-width: 320px">
        <q-card-section>
          <q-input
            v-model="saveName"
            dense
            outlined
            autofocus
            :label="t('knowledgePage.hitTest.caseSave')"
            :placeholder="t('knowledgePage.hitTest.caseNamePh')"
            @keyup.enter="confirmSave"
          />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat no-caps :label="t('knowledgePage.hitTest.caseCancel')" />
          <q-btn
            unelevated
            no-caps
            color="primary"
            :label="t('knowledgePage.hitTest.caseSave')"
            :disable="!saveName.trim()"
            @click="confirmSave"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useKnowledgeHitTest, type HitTestRun } from '../../features/knowledge/useKnowledgeHitTest';
import { useHitTestCases } from '../../features/knowledge/useHitTestCases';
import type { KnowledgeChunk } from '../../features/knowledge/types';

const props = defineProps<{ collectionId: string }>();

// 用例管理文案走全局 locale（knowledgePage.hitTest.*）；其余 G-19 文案保留组件内消息
// （全局 locale 文件归并行会话维护的区域不动）。测试环境无 i18n 实例时回退为返回 key。
// 全局 t 与本地 tt 同样守在 try/catch 内：测试环境无 i18n 实例时回退为返回 key。
let t: (key: string, named?: Record<string, unknown>) => string;
try {
  t = useI18n().t;
} catch {
  t = (key, named) => (named ? `${key}:${JSON.stringify(named)}` : key);
}
let tt: (key: string, named?: Record<string, unknown>) => string;
try {
  tt = useI18n({
    useScope: 'local',
    inheritLocale: true,
    messages: {
      'zh-CN': {
        queryLabel: '排查查询',
        queryPh: '输入出问题的查询语句，回车并行对比…',
        topk: 'TopK',
        prefix: '路径前缀（可选）',
        rerank: 'Rerank',
        run: '运行对比',
        skipLogHint: '调试流量不落 access_log：反复试探不污染热度信号，排序与线上链路一致。',
        empty: '先输入查询运行一次三策略对比',
        noHits: '无命中',
        mode: '执行：{m}',
        reranked: '已重排',
        notReranked: '未重排',
        error: '该策略失败：{msg}',
        preRank: '重排前 #{n}',
        dense: '稠密',
        sparse: '稀疏',
        rrf: '融合 RRF',
      },
      'en-US': {
        queryLabel: 'Query to debug',
        queryPh: 'Type the misbehaving query, Enter to compare…',
        topk: 'TopK',
        prefix: 'Path prefix (optional)',
        rerank: 'Rerank',
        run: 'Compare',
        skipLogHint:
          'Debug traffic skips access_log: probes never pollute popularity signals; ranking matches production.',
        empty: 'Run a three-strategy comparison to see hits',
        noHits: 'No hits',
        mode: 'mode: {m}',
        reranked: 'reranked',
        notReranked: 'not reranked',
        error: 'Strategy failed: {msg}',
        preRank: 'pre-rerank #{n}',
        dense: 'dense',
        sparse: 'sparse',
        rrf: 'RRF fused',
      },
    },
  }).t;
} catch {
  tt = (key, named) => (named ? `${key}:${JSON.stringify(named)}` : key);
}

const hit = useKnowledgeHitTest();
const casesApi = useHitTestCases();

// ---------- U1-2 用例条 ----------
const selectedCaseId = ref<string | null>(null);
const saveDialog = ref(false);
const saveName = ref('');
const importError = ref('');
const fileInput = ref<HTMLInputElement | null>(null);

// 跨库切换：用例按 collectionId 命名空间存取，旧库选择对新库无意义——清空选中与导入错误。
watch(
  () => props.collectionId,
  () => {
    selectedCaseId.value = null;
    importError.value = '';
  },
);

const selectedCase = computed(() =>
  selectedCaseId.value ? casesApi.getCase(props.collectionId, selectedCaseId.value) : null,
);
const caseOptions = computed(() => casesApi.listCases(props.collectionId).map((c) => ({ label: c.name, value: c.id })));

/** 选择用例：回填 query 与参数行。 */
function onPickCase(id: string | null) {
  if (!id) return;
  const c = casesApi.getCase(props.collectionId, id);
  if (!c) return;
  hit.query.value = c.query;
  hit.topK.value = c.params.topK;
  hit.useRerank.value = c.params.useRerank;
  hit.queryPrefix.value = c.params.pathPrefix;
}

function openSave() {
  saveName.value = '';
  saveDialog.value = true;
}

function confirmSave() {
  const name = saveName.value.trim();
  if (!name || !hit.query.value.trim()) return;
  const c = casesApi.addCase(props.collectionId, {
    name,
    query: hit.query.value.trim(),
    params: {
      topK: hit.topK.value > 0 ? hit.topK.value : 10,
      useRerank: hit.useRerank.value,
      pathPrefix: hit.queryPrefix.value.trim(),
    },
  });
  selectedCaseId.value = c.id;
  saveDialog.value = false;
}

function removeCase() {
  if (!selectedCaseId.value) return;
  casesApi.deleteCase(props.collectionId, selectedCaseId.value);
  selectedCaseId.value = null;
}

function exportCurrent() {
  const c = selectedCase.value;
  if (!c) return;
  const json = casesApi.exportCase(props.collectionId, c.id);
  if (!json) return;
  const blob = new Blob([json], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${c.name.replace(/[\\/:*?"<>|]/g, '_') || 'hit-test-case'}.json`;
  a.click();
  // 同 tick revoke 在个别浏览器会截断下载，延迟到下一拍释放。
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

function triggerImport() {
  fileInput.value?.click();
}

async function onImportFile(ev: Event) {
  const input = ev.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = '';
  if (!file) return;
  importError.value = '';
  try {
    const res = casesApi.importCase(props.collectionId, await file.text());
    if (!res.ok) {
      importError.value = res.error;
      console.warn('[hit-test] import case failed:', res.error);
      return;
    }
    selectedCaseId.value = res.case.id;
    onPickCase(res.case.id);
  } catch (e) {
    importError.value = e instanceof Error ? e.message : String(e);
    console.warn('[hit-test] import case failed:', e);
  }
}

/** 三策略对比结束后：选中用例则追加一条 RunRecord（modeUsed/reranked/耗时/各策略 top1..3）。 */
function recordRun() {
  const c = selectedCase.value;
  if (!c) return;
  const rows = hit.runs.value;
  if (!rows.length) return;
  const topScores: number[] = [];
  const modes = new Set<string>();
  let reranked = false;
  let durationMs = 0;
  let any = false;
  for (const row of rows) {
    if (!row.result) continue;
    any = true;
    if (row.result.reranked) reranked = true;
    if (row.result.mode_used) modes.add(row.result.mode_used);
    durationMs = Math.max(durationMs, row.elapsed_ms);
    for (const ch of row.result.chunks.slice(0, 3)) topScores.push(Number(ch.score) || 0);
  }
  if (!any) return;
  casesApi.addRun(props.collectionId, c.id, {
    at: new Date().toISOString(),
    modeUsed: [...modes].join('/') || '?',
    reranked,
    durationMs,
    topScores,
  });
}

// ---------- 回归对比表 ----------
type CompareRow = { idx: number; prev: number | undefined; latest: number | undefined };

const latestRun = computed(() => selectedCase.value?.runs[0] ?? null);
const prevRun = computed(() => selectedCase.value?.runs[1] ?? null);
const compareRows = computed<CompareRow[]>(() => {
  const l = latestRun.value;
  const p = prevRun.value;
  if (!l || !p) return [];
  const n = Math.max(l.topScores.length, p.topScores.length);
  const rows: CompareRow[] = [];
  for (let i = 0; i < n; i += 1) rows.push({ idx: i, prev: p.topScores[i], latest: l.topScores[i] });
  return rows;
});

function fmtMaybe(v: number | undefined): string {
  return v === undefined ? '—' : scoreText(v);
}

function delta(r: CompareRow): number | null {
  if (r.prev === undefined || r.latest === undefined) return null;
  return r.latest - r.prev;
}

function deltaText(r: CompareRow): string {
  const d = delta(r);
  if (d === null) return '—';
  if (d === 0) return '±0.000';
  return `${d > 0 ? '↑' : '↓'} ${Math.abs(d).toFixed(3)}`;
}

function deltaClass(r: CompareRow): string {
  const d = delta(r);
  if (d === null || d === 0) return 'kht__dim';
  return d > 0 ? 'kht__delta--up' : 'kht__delta--down';
}

const ISO_LEN_DATE = 16; // "YYYY-MM-DDTHH:mm"
function shortAt(at: string | undefined): string {
  if (!at) return '?';
  return at.replace('T', ' ').slice(0, ISO_LEN_DATE);
}

async function runAll() {
  // 运行中回车/连点：hit.run 会提前返回且保留旧结果，此时再 recordRun 会把
  // 上一次结果重复记成一条 RunRecord，污染「最近两次对比」——直接拒绝本轮。
  if (hit.running.value) return;
  await hit.run(props.collectionId);
  recordRun();
}

function preRank(row: HitTestRun, c: KnowledgeChunk): number | null {
  return hit.preRerankRank(row, c);
}

function scoreText(v: number): string {
  return (Number(v) || 0).toFixed(3);
}

/** G-12 分路分解：仅展示非零的侧分 / 融合分 / 重排分。 */
function breakdown(c: KnowledgeChunk): string {
  const parts: string[] = [];
  if (c.dense_score) parts.push(`dense ${scoreText(c.dense_score)}`);
  if (c.sparse_score) parts.push(`sparse ${scoreText(c.sparse_score)}`);
  if (c.rrf_score) parts.push(`rrf ${c.rrf_score.toFixed(4)}`);
  if (c.rerank_score) parts.push(`rerank ${scoreText(c.rerank_score)}`);
  return parts.join(' · ');
}

function preview(content: string): string {
  const compact = content.replace(/\s+/g, ' ').trim();
  return compact.length > 160 ? `${compact.slice(0, 160)}…` : compact;
}
</script>

<style lang="sass" scoped>
.kht
  &__hint
    color: var(--kb-text-dim)
  &__dim
    color: var(--kb-text-dim)
    font-variant-numeric: tabular-nums
  &__elapsed
    margin-left: auto
  &__empty
    color: var(--kb-text-dim)
    padding: 12px 0
  &__col
    min-width: 0
  &__list
    max-height: 420px
    overflow-y: auto
  &__hit
    padding: 6px 4px
    border-bottom: 1px solid var(--glass-border)
  &__rank
    font-weight: 600
    color: var(--color-accent)
    min-width: 24px
  &__prerank
    color: var(--kb-text-dim)
  &__score
    font-variant-numeric: tabular-nums
    font-weight: 600
  &__scores
    color: var(--kb-text-dim)
    font-variant-numeric: tabular-nums
  &__doc
    color: var(--kb-text-dim)
    margin-left: auto
    max-width: 40%
  &__content
    color: var(--kb-text-primary, inherit)
    opacity: 0.85
    margin-top: 2px
    white-space: pre-wrap
    word-break: break-word
  &__cases
    margin-top: -4px
  &__history
    border-top: 1px dashed var(--glass-border)
    padding-top: 8px
  &__compare
    border-collapse: collapse
    margin-top: 4px
    font-variant-numeric: tabular-nums
    th, td
      padding: 2px 10px 2px 0
      text-align: left
      font-weight: 400
    thead th
      color: var(--kb-text-dim)
  &__delta--up
    color: var(--color-success)
    font-weight: 600
  &__delta--down
    color: var(--color-danger)
    font-weight: 600
</style>
