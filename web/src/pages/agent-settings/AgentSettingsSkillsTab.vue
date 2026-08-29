<template>
  <div class="settings-grid settings-grid--wide skills-tab">
    <skills-tab-nav :items="navItems" :active-id="activeId" @select="selectSection" />

    <!-- Skill 挂载 -->
    <section :id="SECTION_IDS.mount" class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">Skill 挂载策略</span>
          </div>
          <p class="settings-section__hint">
            控制 ADK 可见的已发布
            Skill：白名单/黑名单、必选标签与意图收窄。运行时仅挂载<strong>已发布且已启用</strong>的平台 Skill。
          </p>
        </div>
        <div class="settings-section__actions">
          <q-btn
            flat
            rounded
            dense
            no-caps
            label="刷新列表"
            :loading="loadingSkillSlugs"
            @click="$emit('load-skill-slugs')"
          />
          <q-btn flat rounded dense no-caps label="恢复默认" @click="confirmReset" />
        </div>
      </div>

      <div class="settings-subsection-grid">
        <div class="settings-subsection">
          <div class="settings-subsection__head">
            <div class="settings-subsection__title">加载模式</div>
            <p class="settings-subsection__hint">
              Progressive：Prompt 仅注入 Skill 清单（L0），正文经
              <code>skill_load</code> 按需加载（L1），引用文档经 <code>skill_select_docs</code>（L2）。
            </p>
          </div>
          <q-select
            v-model="config.skill_load_mode"
            dense
            outlined
            emit-value
            map-options
            :options="skillLoadModeOptions"
            label="skill_load_mode"
            hint="默认 progressive（对齐渐进披露：先 skill_load 再执行）；需 Skill 正文常驻上下文时改 session"
          />
        </div>

        <div class="settings-subsection">
          <div class="settings-subsection__head row items-center justify-between">
            <div>
              <div class="settings-subsection__title">意图收窄</div>
              <p class="settings-subsection__hint">根据用户消息关键词匹配意图路径，缩小 Skill 候选集。</p>
            </div>
            <q-toggle v-model="config.skillRuntime.intent_routing_enabled" />
          </div>
          <div class="app-form-field-grid app-form-field-grid--2col">
            <q-input
              v-model.number="config.skillRuntime.intent_max_paths"
              dense
              outlined
              type="number"
              label="最多意图路径数"
              :min="1"
              :max="32"
            />
            <q-input
              v-model.number="config.skillRuntime.max_skills_in_toolset"
              dense
              outlined
              type="number"
              label="工具集内 Skill 上限"
              :min="1"
              :max="256"
            />
          </div>
        </div>

        <div class="settings-subsection settings-subsection--span">
          <div class="settings-subsection__head">
            <div class="settings-subsection__title">Slug 与标签</div>
            <p class="settings-subsection__hint">
              允许与拒绝同一 slug 互斥；历史重叠配置保存时按<strong>拒绝优先</strong>规整。
            </p>
          </div>
          <div class="app-form-field-grid app-form-field-grid--3col">
            <q-select
              v-model="config.skillRuntime.allowed_slugs"
              dense
              outlined
              multiple
              use-chips
              color="primary"
              use-input
              new-value-mode="add-unique"
              input-debounce="0"
              :options="skillSlugOptions"
              option-label="label"
              option-value="value"
              emit-value
              map-options
              label="允许的 Skill slug"
              hint="留空 = 不启用 slug 白名单"
            />
            <q-select
              v-model="config.skillRuntime.denied_slugs"
              dense
              outlined
              multiple
              use-chips
              color="primary"
              use-input
              new-value-mode="add-unique"
              input-debounce="0"
              :options="skillSlugOptions"
              option-label="label"
              option-value="value"
              emit-value
              map-options
              label="拒绝的 Skill slug"
            />
            <q-select
              v-model="config.skillRuntime.allowed_tags"
              dense
              outlined
              multiple
              use-chips
              color="primary"
              use-input
              new-value-mode="add-unique"
              input-debounce="0"
              label="要求的标签（AND）"
              hint="如 file_type:xlsx；可与用户话术中的 domain:/file_type: 合并"
              :options="skillTagOptions"
              :loading="loadingSkillTags"
            />
          </div>
        </div>
      </div>
    </section>

    <!-- 平台工具 -->
    <section :id="SECTION_IDS.policy" class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">平台工具策略</span>
          </div>
          <p class="settings-section__hint">全局 allow/deny 与 profile；逐工具覆盖见下方「工具覆盖」分区。</p>
        </div>
        <div class="row items-center q-gutter-sm">
          <span v-if="!config.tools.enabled" class="settings-subsection__hint">
            工具总开关关闭时，逐工具覆盖不会生效。
          </span>
          <q-toggle v-model="config.tools.enabled" label="启用工具" />
        </div>
      </div>

      <template v-if="config.tools.enabled">
        <div class="app-form-field-grid app-form-field-grid--2col q-mb-md">
          <q-select
            v-model="config.tools.profile"
            dense
            outlined
            emit-value
            map-options
            label="工具配置文件"
            hint="chat_only / read_only / coding / research / full（另：minimal·safe 别名，system_admin·spirit 系统内部）"
            :options="toolProfileOptions"
          />
          <q-banner v-if="config.tools.profile === 'full'" class="app-grid-span-full settings-warning-banner" rounded>
            <strong>full</strong> 配置文件会暴露平台全部工具，仅建议在受信环境使用。
          </q-banner>
          <q-input
            v-model="config.tools.tool_call_prefix"
            dense
            outlined
            label="工具调用前缀"
            hint="如 proxy_，解析前会从工具名剥离"
          />
          <q-select
            v-model="config.tools.allow"
            class="app-grid-span-full"
            dense
            outlined
            multiple
            use-chips
            color="primary"
            emit-value
            map-options
            label="允许"
            :options="toolSelectOptions"
            :loading="loadingCatalogTools"
          />
          <q-select
            v-model="config.tools.deny"
            dense
            outlined
            multiple
            use-chips
            color="primary"
            emit-value
            map-options
            label="拒绝"
            :options="toolSelectOptions"
            :loading="loadingCatalogTools"
          />
          <q-select
            v-model="config.tools.concurrent_allow"
            dense
            outlined
            multiple
            use-chips
            color="primary"
            emit-value
            map-options
            label="并行白名单"
            :options="toolSelectOptions"
            :loading="loadingCatalogTools"
          />
        </div>
        <q-banner v-if="toolConflicts.length" rounded class="settings-warning-banner q-mb-md">
          以下工具同时出现在允许与拒绝中，运行时按拒绝优先：{{ toolConflicts.join(', ') }}
        </q-banner>

        <div class="settings-subsection settings-subsection--flat q-mb-md">
          <div class="settings-subsection__head row items-center justify-between">
            <div>
              <div class="settings-subsection__title">工具重试</div>
              <p class="settings-subsection__hint">失败时指数退避 + 随机抖动。</p>
            </div>
            <q-toggle v-model="config.tools.retry.enabled" />
          </div>
          <div v-if="config.tools.retry.enabled" class="app-form-field-grid app-form-field-grid--2col">
            <q-input
              v-model.number="config.tools.retry.max_attempts"
              dense
              outlined
              type="number"
              label="最大重试次数"
              hint="含首次调用"
            />
            <q-input
              v-model.number="config.tools.retry.initial_interval_ms"
              dense
              outlined
              type="number"
              label="初始间隔 (ms)"
            />
            <q-input
              v-model.number="config.tools.retry.backoff_factor"
              dense
              outlined
              type="number"
              step="0.1"
              label="退避因子"
            />
            <q-input
              v-model.number="config.tools.retry.max_interval_ms"
              dense
              outlined
              type="number"
              label="最大间隔 (ms)"
            />
            <q-toggle v-model="config.tools.retry.jitter" label="随机抖动" />
          </div>
        </div>

        <div class="settings-subsection settings-subsection--flat q-mb-md">
          <div class="settings-subsection__title q-mb-sm">并行与流式</div>
          <div class="app-form-field-grid app-form-field-grid--2col">
            <q-toggle v-model="config.tools.parallel_enabled" label="并行工具调用" />
            <q-toggle v-model="config.tools.streaming_enabled" label="流式工具（StreamableCall）" />
          </div>
        </div>
      </template>
    </section>

    <!-- 工具覆盖 -->
    <section :id="SECTION_IDS.overrides" class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">工具覆盖</span>
          </div>
          <p class="settings-section__hint">
            按工具覆盖启用、模式与确认策略，<strong>更改即时生效</strong>；未覆盖的工具跟随上方全局策略。
          </p>
        </div>
      </div>
      <agent-tools-section :agent-id="agentId" />
    </section>

    <!-- 代码执行 -->
    <section :id="SECTION_IDS.executor" class="settings-section">
      <div class="section-heading">
        <div class="section-heading__main">
          <div class="section-title">
            <span class="section-title__text">Skill 代码执行</span>
          </div>
          <p class="settings-section__hint">
            <code>skill_run</code> 沙箱类型。生产环境建议使用 <strong>docker</strong>；local 无隔离，仅适合开发。
          </p>
        </div>
      </div>
      <q-select
        v-model="config.code_executor_type"
        class="app-field-md"
        dense
        outlined
        emit-value
        map-options
        :options="codeExecutorOptions"
        label="执行器类型"
      />
      <q-banner v-if="fallbackHint" rounded dense class="q-mt-sm settings-info-banner">
        {{ fallbackHint }}
      </q-banner>
    </section>
  </div>
</template>

<script setup lang="ts">
const config = defineModel<AgentRuntimeConfigForm>('config', { required: true });
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AgentToolsSection from '../../components/agents/AgentToolsSection.vue';
import SkillsTabNav from '../../components/agents/SkillsTabNav.vue';
import { useSkillsTabNav } from '../../features/agents/useSkillsTabNav';
import type { CodeExecutorCapability } from '../../features/monitor/types';
import type { AgentRuntimeConfigForm } from '../../features/agents/agentRuntimeConfig';

/** 四分区锚点 id，与 SkillsTabNav 项一一对应。 */
const SECTION_IDS = {
  mount: 'skills-section-mount',
  policy: 'skills-section-policy',
  overrides: 'skills-section-overrides',
  executor: 'skills-section-executor',
} as const;

const { t } = useI18n();

const baseExecutorOptions = [
  { label: 'Local（子进程，开发用）', value: 'local' },
  { label: 'Docker（容器隔离，推荐生产）', value: 'docker' },
  { label: 'E2B（云端沙箱，需 E2B_API_KEY）', value: 'e2b' },
  { label: 'Container（框架引擎，需 build tag）', value: 'container' },
];

const skillLoadModeOptions = [
  { label: 'progressive（L0→L1→L2 按需加载，默认）', value: 'progressive' },
  { label: 'turn（当前轮次）', value: 'turn' },
  { label: 'once（单次请求）', value: 'once' },
  { label: t('agentSettings.skillLoadModeSession'), value: 'session' },
];

const { activeId, selectSection } = useSkillsTabNav([
  SECTION_IDS.mount,
  SECTION_IDS.policy,
  SECTION_IDS.overrides,
  SECTION_IDS.executor,
]);

const navItems = computed(() => [
  { id: SECTION_IDS.mount, label: t('agentSettings.skillsNavMount') },
  { id: SECTION_IDS.policy, label: t('agentSettings.skillsNavPolicy') },
  { id: SECTION_IDS.overrides, label: t('agentSettings.skillsNavOverrides') },
  { id: SECTION_IDS.executor, label: t('agentSettings.skillsNavExecutor') },
]);

const props = withDefaults(
  defineProps<{
    agentId?: string;
    skillSlugOptions: { label: string; value: string }[];
    loadingSkillSlugs: boolean;
    /** 标签字典选项源（规范标签名）。 */
    skillTagOptions?: string[];
    loadingSkillTags?: boolean;
    codeExecutorCapabilities?: CodeExecutorCapability[];
    toolProfileOptions?: { label: string; value: string }[];
    toolSelectOptions?: { label: string; value: string }[];
    loadingCatalogTools?: boolean;
    toolConflicts?: string[];
  }>(),
  {
    agentId: '',
    skillTagOptions: () => [],
    loadingSkillTags: false,
    codeExecutorCapabilities: () => [],
    toolProfileOptions: () => [],
    toolSelectOptions: () => [],
    loadingCatalogTools: false,
    toolConflicts: () => [],
  },
);

const emit = defineEmits<{
  'load-skill-slugs': [];
  'reset-skill-defaults': [];
}>();

function confirmReset() {
  emit('reset-skill-defaults');
}

const capabilityByType = computed(() => {
  const map = new Map<string, CodeExecutorCapability>();
  for (const c of props.codeExecutorCapabilities ?? []) {
    map.set(c.type, c);
  }
  return map;
});

const codeExecutorOptions = computed(() =>
  baseExecutorOptions.map((opt) => {
    const cap = capabilityByType.value.get(opt.value);
    const unavailable = cap && !cap.available;
    return {
      ...opt,
      disable: unavailable,
      label:
        unavailable && cap?.reason
          ? t('agentSettings.skillExecutorOptionUnavailable', { label: opt.label, reason: cap.reason })
          : opt.label,
    };
  }),
);

const fallbackHint = computed(() => {
  const selected = String(config.value.code_executor_type ?? 'local');
  const cap = capabilityByType.value.get(selected);
  if (!cap || cap.available) {
    return '';
  }
  const reason = cap.reason ? `（${cap.reason}）` : '';
  return t('agentSettings.skillsExecutorUnavailable', { selected, reason });
});
</script>
