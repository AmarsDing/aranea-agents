<template>
  <q-dialog :model-value="open" maximized @update:model-value="onDialogUpdate">
    <q-card class="app-dialog-card app-glass-dialog app-maximized-dialog tool-editor-shell">
      <div class="tool-editor-shell__head">
        <div class="tool-editor-shell__head-left">
          <q-btn flat dense round icon="arrow_back" class="app-registry-icon-btn" :disable="saving" @click="tryClose">
            <q-tooltip>{{ t('toolsPage.editor.back') }}</q-tooltip>
          </q-btn>
          <div class="tool-editor-shell__breadcrumb">
            <span class="tool-editor-shell__breadcrumb-prefix">Tools</span>
            <q-icon name="chevron_right" size="xs" color="grey-6" />
            <span class="tool-editor-shell__breadcrumb-current">{{
              editingId ? t('toolsPage.editor.editTitle') : t('toolsPage.editor.createTitle')
            }}</span>
          </div>
          <div v-if="editingId && form.display_name" class="tool-editor-shell__entity">{{ form.display_name }}</div>
        </div>
        <div class="tool-editor-shell__head-right">
          <q-btn flat dense round icon="help_outline" class="app-registry-icon-btn" @click="helpOpen = true">
            <q-tooltip>{{ t('toolsPage.editor.help') }}</q-tooltip>
          </q-btn>
        </div>
      </div>

      <div class="tool-editor-shell__body">
        <aside class="tool-editor-aside">
          <div class="tool-editor-aside__nav">
            <div class="tool-editor-aside__nav-title">配置步骤</div>
            <div
              v-for="(section, idx) in navSections"
              :key="section.id"
              class="tool-editor-aside__nav-item"
              :class="{ 'tool-editor-aside__nav-item--active': activeSection === section.id }"
              @click="scrollToSection(section.id)"
            >
              <div class="tool-editor-aside__nav-num">{{ idx + 1 }}</div>
              <div class="tool-editor-aside__nav-text">
                <div class="tool-editor-aside__nav-label">{{ section.label }}</div>
                <div class="tool-editor-aside__nav-hint">{{ section.hint }}</div>
              </div>
            </div>
          </div>

          <div class="tool-editor-aside__validation">
            <div class="tool-editor-aside__nav-title">校验状态</div>
            <div
              v-for="check in validationChecks"
              :key="check.key"
              class="tool-editor-aside__check row items-center q-gutter-xs"
            >
              <q-icon
                :name="check.ok ? 'check_circle' : 'error'"
                :color="check.ok ? 'positive' : 'warning'"
                size="xs"
              />
              <span class="text-caption">{{ check.label }}</span>
            </div>
          </div>

          <div v-if="editingId && diffLines.length" class="tool-editor-aside__diff">
            <div class="tool-editor-aside__nav-title">{{ t('toolsPage.editor.diffTitle') }}</div>
            <div class="tool-editor-aside__diff-list">
              <div
                v-for="(line, i) in diffLines.slice(0, 6)"
                :key="i"
                class="tool-editor-aside__diff-line text-caption"
              >
                {{ line }}
              </div>
              <div v-if="diffLines.length > 6" class="text-caption text-grey">…还有 {{ diffLines.length - 6 }} 项</div>
            </div>
          </div>
        </aside>

        <main class="tool-editor-main">
          <div ref="scrollContainer" class="tool-editor-main__scroll" @scroll="onScroll">
            <div class="tool-editor-main__content">
              <section id="section-basic" class="tool-editor-section">
                <div class="tool-editor-section__header">
                  <div class="tool-editor-section__step">1</div>
                  <div>
                    <h3 class="tool-editor-section__title">基础信息</h3>
                    <p class="tool-editor-section__desc">
                      填写工具的名称和用途说明，让团队成员一眼理解这个工具做什么。
                    </p>
                  </div>
                </div>

                <div v-if="!editingId" class="tool-template-grid q-mb-md">
                  <button
                    v-for="tpl in templates"
                    :key="tpl.id"
                    type="button"
                    class="tool-template-card"
                    :class="{ 'tool-template-card--active': selectedTemplate === tpl.id }"
                    @click="$emit('apply-template', tpl.id)"
                  >
                    <span class="tool-template-card__label">{{ tpl.label }}</span>
                    <span class="tool-template-card__caption">{{ tpl.caption }}</span>
                  </button>
                </div>

                <div class="app-form-field-grid app-form-field-grid--2col">
                  <tool-field-hint-input
                    :model-value="form.key"
                    label="工具标识 (Key)"
                    :hint="hints.key"
                    :disable="Boolean(editingId)"
                    @update:model-value="$emit('patch-form', { key: $event })"
                  />
                  <tool-field-hint-input
                    :model-value="form.display_name"
                    label="显示名称"
                    :hint="hints.display_name"
                    @update:model-value="$emit('patch-form', { display_name: $event })"
                  />
                  <q-input
                    class="app-grid-span-full app-field-long"
                    :model-value="form.description"
                    dense
                    outlined
                    autogrow
                    type="textarea"
                    label="描述"
                    :hint="hints.description"
                    @update:model-value="$emit('patch-form', { description: String($event ?? '') })"
                  />
                  <q-input
                    :model-value="form.category"
                    dense
                    outlined
                    label="分类"
                    :hint="hints.category"
                    @update:model-value="$emit('patch-form', { category: String($event ?? 'custom') })"
                  />
                  <q-select
                    v-if="!editingId"
                    :model-value="form.source"
                    dense
                    outlined
                    emit-value
                    map-options
                    label="来源"
                    :hint="hints.source"
                    :options="sourceOptions"
                    @update:model-value="$emit('patch-form', { source: String($event ?? 'external') })"
                  />
                  <q-input
                    v-else
                    :model-value="form.source"
                    dense
                    outlined
                    readonly
                    label="来源"
                    :hint="hints.source"
                  />
                  <q-select
                    :model-value="form.risk_level"
                    dense
                    outlined
                    emit-value
                    map-options
                    label="风险级别"
                    :hint="hints.risk_level"
                    :options="riskLevelOptions"
                    @update:model-value="$emit('patch-form', { risk_level: String($event ?? 'low') })"
                  />
                </div>
              </section>

              <section id="section-policy" class="tool-editor-section">
                <div class="tool-editor-section__header">
                  <div class="tool-editor-section__step">2</div>
                  <div>
                    <h3 class="tool-editor-section__title">运行策略</h3>
                    <p class="tool-editor-section__desc">控制工具的启用状态、访问权限和运行时行为。</p>
                  </div>
                </div>

                <q-banner v-if="registryLocked" rounded class="settings-warning-banner q-mb-sm">
                  内置 / 只读工具：「需确认 / 流式 / 并发」由代码 registry
                  维护，保存后重启可能被同步覆盖。日常启停请用列表页开关。
                </q-banner>

                <tool-policy-toggle-list
                  :form="form"
                  :registry-locked="registryLocked"
                  @patch-form="$emit('patch-form', $event)"
                />

                <q-banner rounded dense class="settings-info-banner q-mt-sm">
                  Agent 级并行、流式、重试在
                  <router-link :to="{ name: 'agents' }" class="text-primary">Agent 列表</router-link>
                  进入对应 Agent → 能力 Tab 配置；此处为 Tool 目录级标记。
                </q-banner>
              </section>

              <section id="section-params" class="tool-editor-section">
                <div class="tool-editor-section__header">
                  <div class="tool-editor-section__step">3</div>
                  <div>
                    <h3 class="tool-editor-section__title">调用参数</h3>
                    <p class="tool-editor-section__desc">定义 AI 调用此工具时需要传递的参数结构。</p>
                    <p class="tool-editor-section__desc-detail">
                      <strong>何时配置：</strong>想让 AI 知道该传什么信息给工具（搜索关键词、文件路径等）。
                      <strong>配置后：</strong>AI 按参数结构生成调用请求，缺少必填参数时会自动补充或向用户询问。
                    </p>
                  </div>
                </div>
                <tool-schema-builder
                  :model-value="form.parameters_schema_json"
                  title="调用参数定义"
                  :hint="hints.parameters_schema_json"
                  :readonly="schemaReadonly"
                  @update:model-value="$emit('patch-form', { parameters_schema_json: $event })"
                />
                <q-expansion-item dense-toggle label="返回结构说明（可选）" class="q-mt-md">
                  <div class="q-pt-sm">
                    <p class="tool-editor-section__desc-detail">
                      定义工具返回数据的结构，供 AI 根据返回结果做后续决策（如判断搜索是否有结果）。
                      不配置不影响工具运行，但 AI 无法预知返回格式。
                    </p>
                    <tool-schema-builder
                      :model-value="form.result_schema_json"
                      title="返回结构定义"
                      :hint="hints.result_schema_json"
                      :readonly="schemaReadonly"
                      @update:model-value="$emit('patch-form', { result_schema_json: $event })"
                    />
                  </div>
                </q-expansion-item>
              </section>

              <section id="section-config" class="tool-editor-section">
                <div class="tool-editor-section__header">
                  <div class="tool-editor-section__step">4</div>
                  <div>
                    <h3 class="tool-editor-section__title">管理员配置</h3>
                    <p class="tool-editor-section__desc">
                      配置工具运行所需的密钥、超时等参数。AI 不可见，仅管理员可编辑。
                    </p>
                    <p class="tool-editor-section__desc-detail">
                      <strong>何时配置：</strong>工具需要 API Key、连接地址、超时时间等运行时参数时。
                      <strong>配置后：</strong>工具调用时自动携带这些配置值，无需 AI 传递敏感信息。
                    </p>
                  </div>
                </div>

                <q-expansion-item
                  dense-toggle
                  :default-opened="!hasConfigSchema"
                  label="配置项定义（管理员维护）"
                  class="q-mb-md"
                >
                  <div class="q-pt-sm">
                    <p class="tool-editor-section__desc-detail">
                      定义此工具接受哪些配置项（如 api_key、timeout）。首次创建工具或需要新增配置项时修改；
                      已有配置项的工具通常无需改动此区域。
                    </p>
                    <tool-schema-builder
                      :model-value="form.config_schema_json"
                      title="配置项定义"
                      :hint="hints.config_schema_json"
                      :readonly="schemaReadonly"
                      @update:model-value="$emit('patch-form', { config_schema_json: $event })"
                    />
                  </div>
                </q-expansion-item>

                <div class="tool-editor-section__subtitle">当前配置值</div>
                <q-banner v-if="form.key === 'web_research'" rounded dense class="settings-info-banner q-mb-sm">
                  API Key 留空时使用
                  <router-link :to="{ name: 'settings' }" class="text-primary">系统设置 → Web 研究</router-link>
                  或环境变量 TAVILY_API_KEY。
                </q-banner>
                <template v-if="hasConfigSchema">
                  <tool-schema-form
                    :schema-json="form.config_schema_json"
                    :model-value="form.config_json"
                    @update:model-value="$emit('patch-form', { config_json: $event })"
                  />
                </template>
                <q-input
                  v-else
                  :model-value="form.config_json"
                  type="textarea"
                  outlined
                  autogrow
                  dense
                  class="app-field-long"
                  label="配置 JSON"
                  :hint="hints.config_json"
                  :readonly="schemaReadonly"
                  :error="Boolean(jsonErrors.config_json)"
                  :error-message="jsonErrors.config_json"
                  @update:model-value="$emit('patch-form', { config_json: String($event ?? '{}') })"
                />
                <q-banner v-if="extraConfigKeys.length" rounded class="settings-warning-banner q-mt-sm">
                  以下字段不在配置 Schema 中，保存时可能被后端拒绝：{{ extraConfigKeys.join(', ') }}
                </q-banner>
              </section>

              <section id="section-advanced" class="tool-editor-section">
                <div class="tool-editor-section__header">
                  <div class="tool-editor-section__step">5</div>
                  <div>
                    <h3 class="tool-editor-section__title">高级选项</h3>
                    <p class="tool-editor-section__desc">出厂默认配置和元数据，通常无需修改。</p>
                    <p class="tool-editor-section__desc-detail">
                      <strong>何时配置：</strong>需要为新工具设定默认配置值，或添加供其他系统读取的扩展信息时。
                      <strong>配置后：</strong>新 Agent 绑定此工具时会继承出厂默认配置。
                    </p>
                  </div>
                </div>
                <q-banner v-if="registryLocked" rounded dense class="settings-info-banner q-mb-sm">
                  只读内置工具：高级 JSON 可查看；修改可能在重启后被 registry 覆盖。
                </q-banner>
                <q-expansion-item dense-toggle label="出厂默认配置">
                  <div class="q-pt-sm column q-gutter-sm">
                    <ul v-if="defaultDiffLines.length" class="app-registry-muted-caption q-pl-md q-ma-none">
                      <li v-for="(line, i) in defaultDiffLines" :key="i">{{ line }}</li>
                    </ul>
                    <q-input
                      :model-value="form.default_config_json"
                      type="textarea"
                      outlined
                      autogrow
                      dense
                      class="app-field-long"
                      label="默认配置 JSON"
                      :readonly="registryLocked"
                      :error="Boolean(jsonErrors.default_config_json)"
                      :error-message="jsonErrors.default_config_json"
                      @update:model-value="$emit('patch-form', { default_config_json: String($event ?? '{}') })"
                    />
                    <q-btn
                      v-if="!registryLocked"
                      flat
                      dense
                      no-caps
                      size="sm"
                      label="从当前配置复制"
                      @click="$emit('patch-form', { default_config_json: form.config_json })"
                    />
                  </div>
                </q-expansion-item>
                <q-expansion-item dense-toggle label="扩展元数据">
                  <div class="q-pt-sm">
                    <q-input
                      :model-value="form.metadata_json"
                      type="textarea"
                      outlined
                      autogrow
                      dense
                      class="app-field-long"
                      label="元数据 JSON"
                      :readonly="registryLocked"
                      :error="Boolean(jsonErrors.metadata_json)"
                      :error-message="jsonErrors.metadata_json"
                      @update:model-value="$emit('patch-form', { metadata_json: String($event ?? '{}') })"
                    />
                  </div>
                </q-expansion-item>
                <q-expansion-item dense-toggle label="Raw JSON（全部 Schema）">
                  <div class="q-pt-sm column q-gutter-sm">
                    <q-input
                      v-for="field in rawFields"
                      :key="field.key"
                      :model-value="form[field.key]"
                      type="textarea"
                      outlined
                      autogrow
                      dense
                      class="app-field-long"
                      :label="field.label"
                      :readonly="registryLocked"
                      :error="Boolean(jsonErrors[field.key])"
                      :error-message="jsonErrors[field.key]"
                      @update:model-value="$emit('patch-form', { [field.key]: String($event ?? '{}') })"
                    />
                  </div>
                </q-expansion-item>
              </section>
            </div>
          </div>

          <div class="tool-editor-main__footer">
            <q-btn outline no-caps label="取消" @click="tryClose" />
            <q-btn
              no-caps
              unelevated
              class="app-registry-primary-btn"
              icon="save"
              label="保存"
              :loading="saving"
              @click="$emit('save')"
            />
          </div>
        </main>
      </div>

      <tool-editor-help-drawer v-model:open="helpOpen" />
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { TOOL_CREATE_TEMPLATES, TOOL_FIELD_HINTS, isRegistryLockedTool } from '../../features/tools/toolEditorCopy';
import { configExtraKeys, configDiffSummary } from '../../features/tools/jsonSchemaBuilder';
import type { ToolUpsertInput } from '../../features/tools/types';
import { toolEditorJsonKeys, validateToolJsonFields, riskLevelOptions, sourceSuggestions, diffToolFormLines } from './toolUi';
import ToolFieldHintInput from './editor/ToolFieldHintInput.vue';
import ToolEditorHelpDrawer from './editor/ToolEditorHelpDrawer.vue';
import ToolPolicyToggleList from './editor/ToolPolicyToggleList.vue';
import ToolSchemaBuilder from './editor/ToolSchemaBuilder.vue';
import ToolSchemaForm from './ToolSchemaForm.vue';

const props = defineProps<{
  open: boolean;
  editingId: string;
  form: ToolUpsertInput;
  /** 打开编辑器时的表单快照（store.originalForm），用于「变更预览」真差分。 */
  originalForm: ToolUpsertInput;
  saving: boolean;
  dirty: boolean;
  jsonErrors: Record<string, string>;
  selectedTemplate: string;
  activeSection?: string;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  save: [];
  close: [];
  'request-close': [];
  'apply-template': [id: string];
  'patch-form': [p: Record<string, unknown>];
  'update:activeSection': [value: string];
}>();

const { t } = useI18n();

const helpOpen = ref(false);
const localSection = ref('basic');
const activeSection = computed({
  get: () => props.activeSection || localSection.value,
  set: (v: string) => {
    localSection.value = v;
    emit('update:activeSection', v);
  },
});
const scrollContainer = ref<HTMLElement | null>(null);

watch(
  () => props.activeSection,
  (id) => {
    if (!id || !props.open) return;
    // 变化源自本地（onScroll 滚动监听 / scrollToSection 已处理）时不重复滚动，
    // 否则手动滚动会被 watch 回写"吸回"到分区顶部。
    if (id === localSection.value) return;
    scrollToSection(id);
  },
);

const hints = TOOL_FIELD_HINTS;
const templates = TOOL_CREATE_TEMPLATES;
const sourceOptions = sourceSuggestions;

const navSections = [
  { id: 'basic', label: '基础信息', hint: '名称、分类、描述', icon: 'info' },
  { id: 'policy', label: '运行策略', hint: '启用、确认、流式', icon: 'policy' },
  { id: 'params', label: '调用参数', hint: 'AI 传递的参数', icon: 'data_object' },
  { id: 'config', label: '管理员配置', hint: '密钥、超时等', icon: 'tune' },
  { id: 'advanced', label: '高级选项', hint: '默认值、元数据', icon: 'more_horiz' },
];

const registryLocked = computed(() => isRegistryLockedTool(props.form));
const schemaReadonly = computed(() => registryLocked.value);

const hasConfigSchema = computed(() => {
  try {
    const s = JSON.parse(props.form.config_schema_json || '{}');
    return s.properties && Object.keys(s.properties).length > 0;
  } catch {
    return false;
  }
});

const extraConfigKeys = computed(() => configExtraKeys(props.form.config_json, props.form.config_schema_json));

const defaultDiffLines = computed(() => configDiffSummary(props.form.config_json, props.form.default_config_json));

// 变更预览：当前表单 vs 打开时快照（真差分，与 dirty 判定同口径）。
const diffLines = computed(() => {
  if (!props.editingId) return [];
  return diffToolFormLines(
    props.form as unknown as Record<string, unknown>,
    props.originalForm as unknown as Record<string, unknown>,
  );
});

const validationChecks = computed(() => {
  const keys = [...toolEditorJsonKeys];
  const fieldObj = keys.reduce(
    (acc, k) => {
      acc[k] = props.form[k];
      return acc;
    },
    {} as Record<string, string>,
  );
  const errors = validateToolJsonFields(fieldObj, keys);
  return [
    // 与 store save() 的客户端校验口径一致：必填 → JSON → 多余字段。
    {
      key: 'required',
      label: t('toolsPage.editor.checkRequired'),
      ok: Boolean(props.form.key.trim() && props.form.display_name.trim()),
    },
    { key: 'json', label: 'JSON 格式', ok: Object.keys(errors).length === 0 },
    { key: 'extra', label: '多余配置字段', ok: extraConfigKeys.value.length === 0 },
  ];
});

const rawFields = [
  { key: 'parameters_schema_json' as const, label: '参数 Schema JSON' },
  { key: 'result_schema_json' as const, label: '返回 Schema JSON' },
  { key: 'config_schema_json' as const, label: '配置 Schema JSON' },
];

const scrollSpySuppressUntil = ref(0);
let scrollSpyTimer: ReturnType<typeof setTimeout> | undefined;

function scrollToSection(id: string) {
  activeSection.value = id;
  const el = document.getElementById(`section-${id}`);
  const scroller = scrollContainer.value;
  if (!el || !scroller) return;
  // 平滑滚动期间抑制滚动监听：否则 onScroll 会把 activeSection 抢回当前位置，
  // 经 watch 回写后把滚动目标又拉回顶部，导致导航失效（实测停在 scrollTop≈75）。
  scrollSpySuppressUntil.value = Date.now() + 800;
  clearTimeout(scrollSpyTimer);
  scrollSpyTimer = setTimeout(() => {
    scrollSpySuppressUntil.value = 0;
  }, 850);
  // rect 坐标系：section 的 offsetParent 是 q-card 而非滚动容器，offsetTop 不能直接用于 scrollTo。
  const top = el.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop - 16;
  scroller.scrollTo({ top, behavior: 'smooth' });
}

function onScroll() {
  if (Date.now() < scrollSpySuppressUntil.value) return;
  const scroller = scrollContainer.value;
  if (!scroller) return;
  const scrollerTop = scroller.getBoundingClientRect().top;
  for (let i = navSections.length - 1; i >= 0; i--) {
    const el = document.getElementById(`section-${navSections[i].id}`);
    if (el && el.getBoundingClientRect().top - scrollerTop - 32 <= 0) {
      activeSection.value = navSections[i].id;
      break;
    }
  }
}

function tryClose() {
  // 未保存确认弹窗上收 Page 层（红线 #4）：由父级决定放弃或继续编辑。
  if (props.dirty) {
    emit('request-close');
  } else {
    emit('close');
  }
}

function onDialogUpdate(val: boolean) {
  if (!val) {
    tryClose();
  }
}
</script>
