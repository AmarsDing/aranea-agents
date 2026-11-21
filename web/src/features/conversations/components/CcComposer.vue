<template>
  <!--
    新任务 / 交给下一位：占右栏（列表留在左边对照），四步——要办什么 → 给它的资料 → 交给谁 → 做完交回什么；
    底部一句话说明会发生什么 + 交接单预览（就是要发出去的原文）。设计：07-会话中心.md §12.3
  -->
  <section class="cc-np" :aria-label="heading" @keydown.esc.stop="emit('close')" @keydown.enter.ctrl.prevent="send">
    <header class="cc-np__h">
      <h2>{{ heading }}</h2>
      <small v-if="mode === 'handoff' && task">{{ task.title }}</small>
      <q-btn
        flat
        dense
        round
        icon="close"
        class="cc-np__close"
        :aria-label="t('cc.compose.close')"
        :title="t('cc.compose.close')"
        @click="emit('close')"
      />
    </header>

    <div class="cc-np__body">
      <div class="cc-np__in">
        <section class="cc-st">
          <div class="cc-st__k">
            <i>1</i>{{ t(mode === 'new' ? 'cc.compose.s1' : 'cc.compose.s1Hand')
            }}<span>{{ t(mode === 'new' ? 'cc.compose.s1Tip' : 'cc.compose.s1HandTip') }}</span>
          </div>
          <textarea
            ref="textEl"
            v-model="text"
            class="cc-field"
            rows="3"
            :aria-label="t(mode === 'new' ? 'cc.compose.s1' : 'cc.compose.s1Hand')"
            :placeholder="t(mode === 'new' ? 'cc.compose.s1Ph' : 'cc.compose.s1HandPh')"
          />
        </section>

        <section class="cc-st">
          <div class="cc-st__k">
            <i>2</i>{{ t('cc.compose.s2')
            }}<span>{{ t(mode === 'new' ? 'cc.compose.s2Tip' : 'cc.compose.s2HandTip') }}</span>
          </div>
          <div class="cc-drop">
            <q-icon name="attach_file" size="18px" aria-hidden="true" />
            <q-btn flat dense no-caps class="cc-np__mini" :label="t('cc.compose.pick')" @click="fileEl?.click()" />
            <span class="cc-drop__tip">{{ t('cc.compose.dropTip', { max: ARTIFACT_MAX_LABEL }) }}</span>
            <input ref="fileEl" type="file" multiple hidden @change="onPick" />
          </div>
          <ul v-if="carry.length || files.length" class="cc-mats">
            <li v-for="m in carry" :key="m.meta.id" class="cc-mat" :class="{ 'cc-mat--off': !m.on }">
              <input v-model="m.on" type="checkbox" :aria-label="t('cc.compose.carry', { name: m.meta.name })" />
              <span class="cc-mat__ic">{{ fileExt(m.meta.name) }}</span>
              <span class="cc-mat__n" :title="m.meta.name"
                >{{ m.meta.name }}<small>{{ m.from }}</small></span
              >
              <span class="cc-mat__s cc-num">{{ fmtSize(m.meta.size) }}</span>
              <span />
            </li>
            <li v-for="(f, i) in files" :key="`${i}-${f.name}`" class="cc-mat">
              <span />
              <span class="cc-mat__ic">{{ fileExt(f.name) }}</span>
              <span class="cc-mat__n" :title="f.name"
                >{{ f.name
                }}<small v-if="f.size > ARTIFACT_MAX_BYTES" class="cc-mat__bad">{{
                  t('cc.compose.tooLarge', { max: ARTIFACT_MAX_LABEL })
                }}</small></span
              >
              <span class="cc-mat__s cc-num">{{ fmtSize(f.size) }}</span>
              <q-btn
                flat
                dense
                round
                size="sm"
                icon="close"
                :aria-label="t('cc.compose.remove', { name: f.name })"
                @click="files.splice(i, 1)"
              />
            </li>
          </ul>
          <p v-if="isCode && materials.length" class="cc-np__note">{{ t('cc.compose.pathNote') }}</p>
          <p v-if="pick === 'flow' && materials.length" class="cc-np__note">{{ t('cc.compose.flowNoFiles') }}</p>
          <p v-if="capWarn" class="cc-np__warn" role="alert">{{ capWarn }}</p>
        </section>

        <section class="cc-st">
          <div class="cc-st__k">
            <i>3</i>{{ t('cc.compose.s3') }}<span>{{ t('cc.compose.s3Tip') }}</span>
          </div>
          <div class="cc-who" role="radiogroup" :aria-label="t('cc.compose.s3')">
            <button
              v-for="c in cards"
              :key="c.key"
              type="button"
              role="radio"
              class="cc-who__c"
              :class="{ 'cc-who__c--on': pick === c.key }"
              :aria-checked="pick === c.key"
              :disabled="c.disabled"
              @click="pick = c.key"
            >
              <q-icon v-if="c.icon" :name="c.icon" size="18px" class="cc-who__ico" aria-hidden="true" />
              <CcGlyph v-else :executor="c.glyph" :name="c.mono" size="lg" />
              <b
                >{{ c.title }}<span v-if="c.key === 'spirit'" class="cc-tag">{{ t('cc.compose.recommended') }}</span></b
              >
              <small>{{ c.sub }}</small>
            </button>
          </div>

          <div v-if="pick === 'agent'" class="cc-sub">
            <label class="cc-search cc-sub__search">
              <q-icon name="search" size="16px" aria-hidden="true" />
              <input
                v-model.trim="who"
                type="search"
                autocomplete="off"
                :aria-label="t('cc.compose.searchPeople', { n: people.length })"
                :placeholder="t('cc.compose.searchPeople', { n: people.length })"
              />
            </label>
            <select v-if="depts.length" v-model="dept" class="cc-np__sel" :aria-label="t('cc.compose.dept')">
              <option value="">{{ t('cc.compose.deptAll') }}</option>
              <option v-for="d in depts" :key="d.value" :value="d.value">{{ d.label }}</option>
            </select>
            <span class="cc-sub__k">{{ t(who || dept ? 'cc.compose.matches' : 'cc.compose.recent') }}</span>
            <button
              v-for="p in shownPeople"
              :key="`${p.kind}:${p.id}`"
              type="button"
              class="cc-opt"
              :class="{ 'cc-opt--on': sel?.id === p.id }"
              :aria-pressed="sel?.id === p.id"
              @click="sel = p"
            >
              <CcGlyph :executor="`${p.kind}:${p.id}`" :name="p.name" size="xs" />{{ p.name }}
            </button>
            <span v-if="!shownPeople.length" class="cc-sub__k">{{ t('cc.compose.noPeople') }}</span>
          </div>

          <div v-if="pick === 'flow'" class="cc-sub">
            <span class="cc-sub__k">{{ t('cc.compose.flow') }}</span>
            <select v-if="flows.length" v-model="flowSel" class="cc-np__sel" :aria-label="t('cc.compose.flow')">
              <option v-for="g in flows" :key="g.id" :value="g.id" :title="g.description">{{ g.name }}</option>
            </select>
            <span v-if="!flows.length" class="cc-sub__k">{{ t('cc.compose.noFlows') }}</span>
            <small v-else-if="flow?.description" class="cc-sub__k">{{ flow.description }}</small>
            <template v-if="flowFields.length">
              <hr />
              <label v-for="name in flowFields" :key="name" class="cc-search cc-sub__search">
                <span class="cc-sub__k">{{ name }}</span>
                <input
                  v-model.trim="flowState[name]"
                  type="text"
                  autocomplete="off"
                  :aria-label="t('cc.compose.flowField', { name })"
                  :placeholder="t('cc.compose.flowField', { name })"
                />
              </label>
            </template>
          </div>

          <div v-if="pick === 'tool'" class="cc-sub">
            <span class="cc-sub__k">{{ t('cc.compose.tool') }}</span>
            <button
              v-for="o in toolOptions"
              :key="o.key"
              type="button"
              class="cc-opt"
              :class="{ 'cc-opt--on': toolSel === o.key }"
              :aria-pressed="toolSel === o.key"
              :disabled="!o.ok"
              :title="o.ok ? '' : o.why || t('cc.compose.notReady')"
              @click="toolSel = o.key"
            >
              <CcGlyph :executor="o.glyph" :name="o.name" size="xs" />{{ o.name
              }}<small>{{ t(o.ok ? 'cc.compose.ok' : 'cc.compose.notReady') }}</small>
            </button>
            <span v-if="!toolOptions.length" class="cc-sub__k">{{ t('cc.compose.noTools') }}</span>
            <hr />
            <span class="cc-sub__k">{{ t('cc.compose.workdir') }}</span>
            <select v-model="where" class="cc-np__sel" :aria-label="t('cc.compose.workdir')">
              <option v-for="o in whereOptions" :key="o.value" :value="o.value" :title="o.title">{{ o.label }}</option>
            </select>
            <span v-if="!whereOptions.length" class="cc-sub__k">{{ t('cc.compose.noProjects') }}</span>
            <code v-else-if="wherePick" class="cc-sub__path cc-num" :title="wherePick.title">{{
              wherePick.title
            }}</code>
            <template v-if="canFleet && pro">
              <span class="cc-sub__k cc-sub__gap">{{ t('cc.compose.way') }}</span>
              <button
                type="button"
                class="cc-opt"
                :class="{ 'cc-opt--on': toolMode === 'direct' }"
                :aria-pressed="toolMode === 'direct'"
                @click="toolMode = 'direct'"
              >
                {{ t('cc.compose.direct') }}
              </button>
              <button
                type="button"
                class="cc-opt"
                :class="{ 'cc-opt--on': toolMode === 'terminal' }"
                :aria-pressed="toolMode === 'terminal'"
                @click="toolMode = 'terminal'"
              >
                <q-icon name="terminal" size="14px" aria-hidden="true" />{{ t('cc.compose.terminal') }}
              </button>
            </template>
          </div>

          <div v-if="pick === 'run'" class="cc-sub">
            <span class="cc-sub__k">{{ t('cc.compose.workdir') }}</span>
            <select v-model="where" class="cc-np__sel" :aria-label="t('cc.compose.workdir')">
              <option v-for="o in whereOptions" :key="o.value" :value="o.value" :title="o.title">{{ o.label }}</option>
            </select>
            <code v-if="wherePick" class="cc-sub__path cc-num" :title="wherePick.title">{{ wherePick.title }}</code>
            <span class="cc-sub__k cc-sub__gap">{{ t('cc.compose.coordinator') }}</span>
            <button
              v-for="o in toolOptions"
              :key="o.key"
              type="button"
              class="cc-opt"
              :class="{ 'cc-opt--on': toolSel === o.key }"
              :aria-pressed="toolSel === o.key"
              :disabled="!o.ok"
              @click="toolSel = o.key"
            >
              <CcGlyph :executor="o.glyph" :name="o.name" size="xs" />{{ o.name }}
            </button>
          </div>

          <div v-if="candidates.length" class="cc-sub" role="alert">
            <span class="cc-sub__k">{{ t('cc.compose.which') }}</span>
            <button
              v-for="c in candidates"
              :key="c.id"
              type="button"
              class="cc-opt"
              :title="c.path"
              @click="pickCandidate(c.name ?? '')"
            >
              {{ c.name }}<small>{{ c.path }}</small>
            </button>
          </div>
        </section>

        <section class="cc-st">
          <div class="cc-st__k">
            <i>4</i>{{ t('cc.compose.s4') }}<span>{{ t('cc.compose.s4Tip') }}</span>
          </div>
          <div class="cc-chipset">
            <button
              v-for="d in DELIVERABLES"
              :key="d"
              type="button"
              class="cc-opt"
              :class="{ 'cc-opt--on': deliver.includes(d) }"
              :aria-pressed="deliver.includes(d)"
              @click="toggleDeliver(d)"
            >
              {{ t(`cc.deliver.${d}`) }}
            </button>
          </div>
        </section>

        <details v-if="pro || mode === 'handoff'" class="cc-pv" :open="isCode || mode === 'handoff'">
          <summary>
            <q-icon name="chevron_right" size="16px" aria-hidden="true" />{{
              t('cc.compose.preview', { who: targetName })
            }}
          </summary>
          <pre>{{ preview }}</pre>
        </details>
      </div>
    </div>

    <footer class="cc-np__f">
      <p class="cc-np__what" aria-live="polite">
        <span v-if="error" class="cc-np__err" role="alert">{{ error }}</span>
        <template v-else>{{ what }}</template>
      </p>
      <q-btn
        v-if="mode === 'new'"
        flat
        no-caps
        :label="t('cc.compose.note')"
        :title="t('cc.compose.noteTip')"
        :disable="busy || !splitTitle(text).title"
        @click="note"
      />
      <q-btn
        unelevated
        no-caps
        color="primary"
        icon="east"
        :label="t('cc.compose.send', { who: targetName })"
        :title="t('cc.compose.sendKey')"
        :loading="busy"
        :disable="!canSend"
        @click="send"
      />
    </footer>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { Notify } from 'quasar';
import CcGlyph from '../../../components/conversations/CcGlyph.vue';
import { listAgents } from '../../agents/api';
import type { Agent } from '../../agents/types';
import { ARTIFACT_MAX_BYTES, ARTIFACT_MAX_LABEL } from '../../artifact/limits';
import type { ArtifactMeta } from '../../artifact/types';
import {
  isLikelyImageAttachmentName,
  modelSupportsFileInput,
  modelSupportsImageInput,
} from '../../chat/modelCapabilities';
import { listCodingAgents, listCodingProjects, upsertCodingProject } from '../../coding/api';
import { listGraphs } from '../../graph/api';
import { listCollections } from '../../knowledge/api';
import type { KnowledgeCollection } from '../../knowledge/types';
import type { GraphDefinition } from '../../graph/types';
import { listPlatformResourceTree } from '../../platform/api';
import type { PlatformResourceTreeNode } from '../../platform/types';
import { useProMode } from '../../prefs/proMode';
import type { AgentPreset, Workspace } from '../../fleet/api';
import { createOfficeTask } from '../../office/api';
import { SPIRIT_AGENT_KEY } from '../../spirit/types';
import { listTeams } from '../../teams/api';
import type { Team } from '../../teams/types';
import type { CodingAgent, CodingProject, ProjectCandidate } from '../../../services/kratos/agentbridge/v1';
import { parseKratosApiError } from '../../../utils/kratosError';
import { LIB_PREFIX, deptOptions, flowInputs, samePath, type AssignPick } from '../assign';
import {
  DELIVERABLES,
  buildHandText,
  fileExt,
  fmtSize,
  splitTitle,
  type Deliverable,
  type HandLabels,
} from '../handoff';
import { flat, toolKey, toolName, type ConvTask } from '../model';
import { dispatchTask, executorKey, type Material, type Target } from '../useTaskDispatch';
import type { ExecOutputs } from '../useTaskFiles';

export type ComposerPick = AssignPick;

const props = defineProps<{
  mode: 'new' | 'handoff';
  task?: ConvTask;
  /** 打开时预填的「要办什么」（从指令条、别的页面带过来） */
  text?: string;
  /** 打开时带进来的文件（拖到页面上） */
  files?: File[];
  to?: ComposerPick;
  /** 退回重做：输入框预填「退回意见」 */
  reject?: boolean;
  /** 预选的执行者：`agent:<id>` / `team:<id>` / `flow:<graphId>` / 工具键（cc、cx…） */
  prefer?: string;
  /** 最近用过的执行者（`agent:<id>` / `team:<id>`），按最近排 */
  recent: string[];
  inputs: ArtifactMeta[];
  outputs: ExecOutputs[];
  canFleet: boolean;
  presets: AgentPreset[];
  workspaces: Workspace[];
}>();
const emit = defineEmits<{ close: []; sent: [r: { anchorId: string; sessionId: string; executor: string }] }>();
// 专业模式才摆出「直接执行 / 开终端」和交接单预览；否则按默认（直接执行）走
const { pro } = useProMode();
const { t } = useI18n();

const LAST_WHERE = 'cc.compose.where';
const readLast = () => {
  try {
    return localStorage.getItem(LAST_WHERE) ?? '';
  } catch {
    return '';
  }
};

// ——— 可选的执行者 ———
type Person = {
  kind: 'agent' | 'team';
  id: string;
  key: string;
  name: string;
  provider: string;
  model: string;
  /** 挂在组织树的哪个节点上（团队没有） */
  node: string;
};
const agents = ref<Agent[]>([]);
const teams = ref<Team[]>([]);
const codingAgents = ref<CodingAgent[]>([]);
const codingProjects = ref<CodingProject[]>([]);
const graphs = ref<GraphDefinition[]>([]);
const orgTree = ref<PlatformResourceTreeNode[]>([]);
const libraries = ref<KnowledgeCollection[]>([]);
onMounted(async () => {
  // 资料里的本机资料库：还没登记成工作目录的也列出来，选了发出时自动登记
  void listCollections({ limit: 200 })
    .then((r) => (libraries.value = r.items.filter((c) => c.vault_backend === 'local' && c.root_path)))
    .catch(() => undefined);
  const [a, tm, ca, cp, gs, org] = await Promise.allSettled([
    listAgents({ limit: 500 }),
    listTeams(),
    listCodingAgents(),
    listCodingProjects(),
    listGraphs(100),
    listPlatformResourceTree('organization'),
  ]);
  if (gs.status === 'fulfilled') graphs.value = gs.value.items;
  if (org.status === 'fulfilled') orgTree.value = org.value;
  if (a.status === 'fulfilled') agents.value = a.value;
  if (tm.status === 'fulfilled') teams.value = tm.value;
  if (ca.status === 'fulfilled') codingAgents.value = ca.value;
  if (cp.status === 'fulfilled') codingProjects.value = cp.value;
});
const spirit = computed(() => agents.value.find((a) => a.agent_key === SPIRIT_AGENT_KEY));
const people = computed<Person[]>(() => [
  ...agents.value
    .filter((a) => a.id && a.status !== 'inactive' && a.agent_key !== SPIRIT_AGENT_KEY)
    .map((a) => ({
      kind: 'agent' as const,
      id: a.id,
      key: a.agent_key,
      name: a.display_name || a.agent_key,
      provider: a.provider,
      model: a.model,
      node: a.taxonomy_position_id,
    })),
  ...teams.value
    .filter((x) => x.id && x.status !== 'inactive')
    .map((x) => ({
      kind: 'team' as const,
      id: x.id,
      key: x.team_key,
      name: x.display_name || x.team_key,
      provider: '',
      model: '',
      node: '',
    })),
]);
const who = ref('');
// 按组织找人：公司 › 部门（组织树读不到就只剩搜索）
const depts = computed(() => deptOptions(orgTree.value));
const dept = ref('');
const shownPeople = computed(() => {
  const q = who.value.toLowerCase();
  const nodes = depts.value.find((d) => d.value === dept.value)?.nodes;
  if (q || nodes)
    return people.value
      .filter((p) => (!nodes || nodes.has(p.node)) && (!q || `${p.name} ${p.key}`.toLowerCase().includes(q)))
      .slice(0, nodes ? 40 : 12);
  const recent = props.recent
    .map((r) => people.value.find((p) => `${p.kind}:${p.id}` === r))
    .filter((p): p is Person => Boolean(p));
  return (recent.length ? recent : people.value).slice(0, 6);
});

// ——— 表单状态 ———
const latest = computed(() =>
  props.task ? [...flat(props.task)].sort((a, b) => a.last.localeCompare(b.last)).pop() : undefined,
);
const text = ref(
  props.mode !== 'handoff'
    ? (props.text ?? '')
    : props.reject
      ? t('cc.compose.prefillReject')
      : latest.value?.status === 'failed'
        ? t('cc.compose.prefillFailed', { why: latest.value.detail || t('cc.status.failed') })
        : '',
);
const files = ref<File[]>([...(props.files ?? [])]);
const carry = ref<{ meta: ArtifactMeta; from: string; on: boolean }[]>([]);
// 交接：默认带上「你给的」和前面交出来的；资料是异步读的，到了再补，已经取消勾选的保持不勾
watch(
  () => [props.inputs, props.outputs] as const,
  ([inputs, outputs]) => {
    if (props.mode !== 'handoff') return;
    const off = new Set(carry.value.filter((m) => !m.on).map((m) => m.meta.id));
    carry.value = [
      ...inputs.map((meta) => ({ meta, from: t('cc.files.gave') })),
      ...outputs.flatMap((o) =>
        o.files.map((meta) => ({ meta, from: t('cc.compose.fromExec', { who: o.exec.executorName }) })),
      ),
    ].map((m) => ({ ...m, on: !off.has(m.meta.id) }));
  },
  { immediate: true },
);
const pick = ref<ComposerPick>(props.to ?? 'spirit');
const sel = ref<Person | null>(null);
const toolMode = ref<'direct' | 'terminal'>('direct');
const toolSel = ref('');
// 流程：模板不能直接执行，只列正式的流程
const flows = computed(() => graphs.value.filter((g) => g.id && !g.isTemplate));
const flowSel = ref('');
const flowState = ref<Record<string, string>>({});
const flow = computed(() => flows.value.find((g) => g.id === flowSel.value));
const flowFields = computed(() => flowInputs(flow.value?.stateFields ?? []));
watch(
  flows,
  (list) => {
    if (list.some((g) => g.id === flowSel.value)) return;
    flowSel.value = (list.find((g) => `flow:${g.id}` === props.prefer) ?? list[0])?.id ?? '';
  },
  { immediate: true },
);
const where = ref(readLast());
const deliver = ref<Deliverable[]>([...(props.task?.deliver ?? [])]);
const busy = ref(false);
const error = ref('');
const candidates = ref<ProjectCandidate[]>([]);
const textEl = ref<HTMLTextAreaElement | null>(null);
const fileEl = ref<HTMLInputElement | null>(null);
onMounted(() => void nextTick(() => textEl.value?.focus()));

// 运行台（终端 / 编排）用预设，直接执行用编码工具桥登记的工具
const useFleetTools = computed(() => pick.value === 'run' || toolMode.value === 'terminal');
const toolOptions = computed(() =>
  useFleetTools.value
    ? props.presets
        .filter((p) => p.id && p.id !== 'shell')
        .map((p) => ({ key: p.id, glyph: toolKey(p.id), name: p.label || p.id, ok: p.available, why: '' }))
    : codingAgents.value
        .filter((a) => a.enabled && a.agentKey)
        .map((a) => ({
          key: a.agentKey!,
          glyph: toolKey(a.agentKey!),
          name: a.displayName || toolName(toolKey(a.agentKey!)),
          ok: a.lastProbeOk !== false,
          why: a.lastProbeError ?? '',
        })),
);
// 工作目录：登记过的目录本身排前（工具就在这个目录里干活）；运行台另有按任务建的隔离副本（改动在副本的分支上）；
// 资料里还没登记的本机资料库排最后，选了发出时自动登记。
const whereOptions = computed(() => {
  const known = useFleetTools.value
    ? props.workspaces
        .filter((w) => w.status === 'ready' && !w.archived)
        .sort((a, b) => Number(b.kind === 'primary') - Number(a.kind === 'primary'))
        .map((w) => ({
          value: w.id,
          label: w.kind === 'primary' ? w.name : t('cc.compose.copyOf', { name: w.name, branch: w.branch }),
          title: w.path,
        }))
    : codingProjects.value.filter((p) => p.name).map((p) => ({ value: p.name!, label: p.name!, title: p.path ?? '' }));
  const paths = [...codingProjects.value.map((p) => p.path ?? ''), ...known.map((o) => o.title)];
  const fresh = libraries.value
    .filter((c) => !paths.some((p) => samePath(p, c.root_path)))
    .map((c) => ({
      value: `${LIB_PREFIX}${c.id}`,
      label: t('cc.compose.fromLibrary', { name: c.name }),
      title: c.root_path,
    }));
  return [...known, ...fresh];
});
const wherePick = computed(() => whereOptions.value.find((o) => o.value === where.value));
watch(
  toolOptions,
  (opts) => {
    if (opts.some((o) => o.key === toolSel.value && o.ok)) return;
    const ok = opts.filter((o) => o.ok);
    toolSel.value = (ok.find((o) => o.glyph === props.prefer) ?? ok[0])?.key ?? '';
  },
  { immediate: true },
);
// 退回重做时预选原来那位（名册是异步读的）
watch(
  people,
  (list) => {
    if (!sel.value && props.prefer) sel.value = list.find((p) => `${p.kind}:${p.id}` === props.prefer) ?? null;
  },
  { immediate: true },
);
watch(
  whereOptions,
  (opts) => {
    if (opts.length && !opts.some((o) => o.value === where.value)) where.value = opts[0]!.value;
  },
  { immediate: true },
);

const toolLabel = computed(() => toolOptions.value.find((o) => o.key === toolSel.value)?.name ?? '');
const cards = computed(() => {
  const out: {
    key: ComposerPick;
    glyph: string;
    mono: string;
    title: string;
    sub: string;
    icon?: string;
    disabled?: boolean;
  }[] = [
    {
      key: 'spirit',
      glyph: 'agent:spirit',
      mono: t('cc.compose.spiritMono'),
      title: t('cc.compose.spirit'),
      sub: spirit.value ? t('cc.compose.spiritSub') : t('cc.compose.noSpirit'),
      disabled: !spirit.value && agents.value.length > 0,
    },
    {
      key: 'agent',
      glyph: sel.value ? `${sel.value.kind}:${sel.value.id}` : 'agent:',
      mono: sel.value?.name ?? t('cc.compose.peopleMono'),
      title: sel.value?.name ?? t('cc.compose.people'),
      sub: sel.value ? sel.value.model || t('cc.compose.teamSub') : t('cc.compose.peopleSub'),
    },
    {
      key: 'flow',
      glyph: 'flow',
      mono: '',
      icon: 'schema',
      title: pick.value === 'flow' && flow.value ? flow.value.name : t('cc.compose.flows'),
      sub: t('cc.compose.flowsSub'),
    },
    {
      key: 'tool',
      glyph: pick.value === 'tool' && toolSel.value ? toolKey(toolSel.value) : 'cc',
      mono: toolLabel.value,
      title: pick.value === 'tool' && toolLabel.value ? toolLabel.value : t('cc.compose.tools'),
      sub: t('cc.compose.toolsSub'),
    },
  ];
  if (props.canFleet)
    out.push({
      key: 'run',
      glyph: 'run',
      mono: '',
      icon: 'account_tree',
      title: t('cc.compose.run'),
      sub: t('cc.compose.runSub'),
    });
  return out;
});

const target = computed<Target | null>(() => {
  if (pick.value === 'spirit')
    return spirit.value
      ? { kind: 'agent', id: spirit.value.id, key: spirit.value.agent_key, name: t('cc.compose.spirit') }
      : null;
  if (pick.value === 'agent')
    return sel.value ? { kind: sel.value.kind, id: sel.value.id, key: sel.value.key, name: sel.value.name } : null;
  if (pick.value === 'flow') {
    if (!flow.value || flowFields.value.some((k) => !flowState.value[k])) return null;
    const state = Object.fromEntries(flowFields.value.map((k) => [k, flowState.value[k]!]));
    const inputKeys = ['user_input', ...(flow.value.stateFields.some((x) => x.name === 'input') ? ['input'] : [])];
    return { kind: 'flow', graphId: flow.value.id, name: flow.value.name, inputKeys, state };
  }
  if (!toolSel.value || !where.value) return null;
  if (pick.value === 'run') {
    const ws = props.workspaces.find((w) => w.id === where.value);
    if (!ws && !where.value.startsWith(LIB_PREFIX)) return null;
    const name = t('cc.compose.runName', { who: toolLabel.value });
    return { kind: 'run', preset: toolSel.value, name, projectId: ws?.projectId ?? '', workspaceId: ws?.id ?? '' };
  }
  return toolMode.value === 'terminal'
    ? { kind: 'terminal', preset: toolSel.value, name: toolLabel.value, workspaceId: where.value }
    : { kind: 'tool', agentKey: toolSel.value, name: toolLabel.value, project: where.value };
});
const targetName = computed(() => target.value?.name ?? t('cc.compose.nobody'));
const isCode = computed(() => pick.value === 'tool' || pick.value === 'run');
const whereLabel = computed(() => wherePick.value?.title || wherePick.value?.label || where.value);

const materials = computed<Material[]>(() => [
  ...carry.value.filter((m) => m.on).map((m) => ({ meta: m.meta })),
  ...files.value.map((file) => ({ file })),
]);
const materialNames = computed(() => materials.value.map((m) => ('meta' in m ? m.meta.name : m.file.name)));

// 交给本平台智能体时，模型读不了文件 / 图片就在发出之前拦下（团队由成员各自的模型决定，不拦）
const capWarn = computed(() => {
  if (!materials.value.length || (pick.value !== 'spirit' && pick.value !== 'agent')) return '';
  const p =
    pick.value === 'spirit'
      ? spirit.value && { name: t('cc.compose.spirit'), provider: spirit.value.provider, model: spirit.value.model }
      : sel.value?.kind === 'agent' && sel.value;
  if (!p) return '';
  const desc = { provider: p.provider, model: p.model };
  if (!modelSupportsFileInput(desc)) return t('cc.compose.capNoFile', { who: p.name, model: p.model });
  const imgs = materialNames.value.filter(isLikelyImageAttachmentName);
  if (imgs.length && !modelSupportsImageInput(desc))
    return t('cc.compose.capNoImage', { who: p.name, model: p.model, names: imgs.join(t('cc.hand.sep')) });
  return '';
});

// ——— 交接单 ———
const labels = computed<HandLabels>(() => ({
  task: t('cc.hand.task'),
  require: t('cc.hand.require'),
  before: t('cc.hand.before'),
  mats: t('cc.hand.mats'),
  ask: t('cc.hand.ask'),
  deliver: t('cc.hand.deliver'),
  tail: t('cc.hand.tail'),
  attached: t('cc.hand.attached'),
  sep: t('cc.hand.sep'),
  semi: t('cc.hand.semi'),
}));
function buildHand(mats: { name: string; path?: string }[]) {
  const dl = deliver.value.map((d) => t(`cc.deliver.${d}`));
  if (props.mode === 'new') {
    // 简单的对话：没带资料、没要求交回什么，交给助手 / 同事——原话发出，不套交接单的格式
    if (plainTalk.value && !mats.length) return text.value.trim();
    const { title, body } = splitTitle(text.value);
    return buildHandText(
      { title: title || t('cc.compose.untitled'), body, materials: mats, deliver: dl },
      labels.value,
    );
  }
  const before = props.task
    ? flat(props.task).map((e) =>
        t(e.detail ? 'cc.hand.beforeLineD' : 'cc.hand.beforeLine', {
          who: e.executorName,
          status: t(`cc.status.${e.status}`),
          detail: e.detail.slice(0, 200),
        }),
      )
    : [];
  return buildHandText(
    { title: props.task?.title ?? '', before, ask: text.value, materials: mats, deliver: dl },
    labels.value,
  );
}
const plainTalk = computed(
  () => props.mode === 'new' && (pick.value === 'spirit' || pick.value === 'agent') && !deliver.value.length,
);
const preview = computed(() =>
  buildHand(materialNames.value.map((name) => ({ name, path: isCode.value ? t('cc.hand.pathLater') : undefined }))),
);

const what = computed(() => {
  const n = materials.value.length;
  const tail = t(deliver.value.length ? 'cc.compose.whatReview' : 'cc.compose.whatDone');
  const x = target.value;
  if (!x)
    return t(
      pick.value === 'agent'
        ? 'cc.compose.whatPickPerson'
        : pick.value === 'flow'
          ? 'cc.compose.whatPickFlow'
          : 'cc.compose.whatPickTool',
    );
  const mats = n ? t('cc.compose.whatMats', { n }) : '';
  if (pick.value === 'spirit') return t('cc.compose.whatSpirit', { mats }) + tail;
  if (x.kind === 'agent' || x.kind === 'team') return t('cc.compose.whatPerson', { who: x.name, mats }) + tail;
  if (x.kind === 'flow') return t('cc.compose.whatFlow', { who: x.name }) + tail;
  const paths = n ? t('cc.compose.whatPaths', { n }) : '';
  if (x.kind === 'run') return t('cc.compose.whatRun', { who: toolLabel.value, where: whereLabel.value, paths }) + tail;
  if (x.kind === 'terminal')
    return t('cc.compose.whatTerminal', { who: x.name, where: whereLabel.value, paths }) + tail;
  return t('cc.compose.whatTool', { who: x.name, where: whereLabel.value, paths }) + tail;
});

const heading = computed(() => t(props.mode === 'new' ? 'cc.compose.newTitle' : 'cc.compose.handTitle'));
const canSend = computed(
  () =>
    !busy.value &&
    Boolean(target.value) &&
    !capWarn.value &&
    Boolean(text.value.trim()) &&
    !files.value.some((f) => f.size > ARTIFACT_MAX_BYTES) &&
    (props.mode === 'new' || Boolean(props.task)),
);

// ——— 操作 ———
function onPick(e: Event) {
  const input = e.target as HTMLInputElement;
  files.value.push(...Array.from(input.files ?? []));
  input.value = '';
}
function addFiles(list: File[]) {
  files.value.push(...list);
}
defineExpose({ addFiles });
function toggleDeliver(d: Deliverable) {
  deliver.value = deliver.value.includes(d) ? deliver.value.filter((x) => x !== d) : [...deliver.value, d];
}

/** 选的是资料里还没登记的本机资料库：先登记成工作目录，再把目标换成登记后的目录本身 */
async function registerLibrary(x: Target): Promise<Target> {
  const lib = libraries.value.find((c) => `${LIB_PREFIX}${c.id}` === where.value);
  if (!lib || (x.kind !== 'tool' && x.kind !== 'terminal' && x.kind !== 'run')) return x;
  const taken = new Set(codingProjects.value.map((p) => p.name));
  let name = lib.name;
  for (let n = 2; taken.has(name); n++) name = `${lib.name} (${n})`;
  const p = await upsertCodingProject({ name, path: lib.root_path, description: lib.description });
  codingProjects.value = [...codingProjects.value, p];
  // 登记后用目录本身（运行台里是这个项目的主检出），不是隔离副本
  const primary = `primary-${p.id ?? ''}`;
  if (x.kind === 'tool') {
    where.value = p.name ?? name;
    return { ...x, project: where.value };
  }
  where.value = primary;
  if (x.kind === 'terminal') return { ...x, workspaceId: primary };
  return { ...x, projectId: p.id ?? '', workspaceId: primary };
}

async function send() {
  if (!canSend.value || !target.value) return;
  if (props.mode === 'new' && !spirit.value) {
    error.value = t('cc.compose.noSpirit');
    return;
  }
  busy.value = true;
  error.value = '';
  candidates.value = [];
  const { title, body } = props.mode === 'new' ? splitTitle(text.value) : { title: props.task!.title, body: '' };
  let to = target.value;
  if (where.value.startsWith(LIB_PREFIX)) {
    try {
      to = await registerLibrary(to);
    } catch (err) {
      busy.value = false;
      error.value = t('cc.compose.err.register', { msg: parseKratosApiError(err).message });
      return;
    }
  }
  const res = await dispatchTask({
    mode: props.mode,
    title: title || t('cc.compose.untitled'),
    body,
    target: to,
    materials: materials.value,
    deliver: deliver.value,
    buildHand,
    spiritId: spirit.value?.id ?? '',
    recordTitle: t('cc.compose.recordTitle', { title }),
    anchorId: props.mode === 'handoff' ? props.task?.anchorId : undefined,
    officeId: props.mode === 'handoff' ? props.task?.officeId : undefined,
  });
  busy.value = false;
  if (res.ok) {
    if (isCode.value) {
      try {
        localStorage.setItem(LAST_WHERE, where.value);
      } catch {
        /* 只是记住上次的选择 */
      }
    }
    Notify.create({
      type: res.registered ? 'positive' : 'warning',
      message: t(res.registered ? 'cc.compose.sent' : 'cc.compose.sentNotRegistered', { who: targetName.value }),
    });
    emit('sent', { anchorId: res.anchorId, sessionId: res.sessionId, executor: executorKey(to) });
    return;
  }
  if (res.candidates?.length) {
    candidates.value = res.candidates;
    error.value = t('cc.compose.which');
    return;
  }
  error.value = t(`cc.compose.err.${res.code}`, { msg: res.message });
}
function pickCandidate(name: string) {
  where.value = name;
  void send();
}
async function note() {
  const { title, body } = splitTitle(text.value);
  if (!title) return;
  busy.value = true;
  try {
    await createOfficeTask(title, body);
    Notify.create({ type: 'positive', message: t('cc.compose.noted') });
    emit('close');
  } catch (err) {
    error.value = parseKratosApiError(err).message;
  } finally {
    busy.value = false;
  }
}
</script>
