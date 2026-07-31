<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="onDialogUpdate">
    <q-card class="app-dialog-card app-glass-dialog app-dialog-card--xl">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">{{ isCreate ? '新建 Skill' : '编辑 Skill' }}</div>
        <q-btn flat round dense icon="close" :disable="saving" @click="tryClose" />
      </q-card-section>
      <!-- 吸顶元数据行：名称 / Slug / 标签 一行；滚动描述/正文时始终可见 -->
      <q-card-section class="skill-meta-dialog__meta">
        <div class="skill-meta-dialog__meta-grid">
          <q-input
            v-model="name"
            dense
            outlined
            label="名称"
            :rules="[(v) => (!!String(v || '').trim() && String(v).trim().length <= 80) || '必填，1–80 字符']"
            :disable="saving"
          />
          <q-input
            v-model="slug"
            dense
            outlined
            label="Slug"
            hint="小写字母、数字、短横线、下划线"
            :disable="saving || !isCreate"
            :rules="isCreate ? [slugRule] : undefined"
          />
          <q-select
            v-model="tags"
            dense
            outlined
            multiple
            use-chips
            use-input
            new-value-mode="add-unique"
            input-debounce="0"
            label="标签"
            :hint="$t('skillTags.selectHint')"
            :options="filteredTagOptions"
            :disable="saving"
            @filter="onTagFilter"
          />
        </div>
      </q-card-section>
      <q-card-section class="app-glass-dialog__scroll skill-meta-dialog__scroll">
        <div class="skill-meta-dialog__fields">
          <q-input
            v-model="description"
            dense
            outlined
            type="textarea"
            autogrow
            label="描述"
            :disable="saving"
            :rules="[(v) => (!!String(v || '').trim() && String(v).trim().length <= 500) || '必填，1–500 字符']"
          />
          <q-input
            v-model="body"
            dense
            outlined
            type="textarea"
            autogrow
            label="正文（SKILL.md）"
            input-style="min-height: 220px; font-family: var(--font-mono, ui-monospace, monospace)"
            :disable="saving"
            :rules="[(v) => !!String(v || '').trim() || '发布前正文必填；草稿也建议填写']"
          />
        </div>
      </q-card-section>
      <q-card-actions align="right" class="q-px-md q-pb-md">
        <q-btn flat rounded no-caps label="取消" :disable="saving" @click="tryClose" />
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          :label="isCreate ? '创建草稿' : '保存草稿'"
          :loading="saving"
          @click="submit"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { Skill } from '../../features/skills/types';

const props = defineProps<{
  modelValue: boolean;
  skill: Skill | null;
  /** 编辑时预填正文（由 Page/composable 拉取） */
  initialBody?: string;
  /** 标签字典选项源（规范标签名，由 Page/composable 拉取）。 */
  tagOptions?: string[];
  saving?: boolean;
  notify: (opts: { type: string; message: string }) => void;
  confirm: (opts: {
    title: string;
    message: string;
    okLabel?: string;
    cancelLabel?: string;
    okColor?: string;
  }) => Promise<boolean>;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  submit: [
    payload: {
      id?: string;
      name: string;
      slug: string;
      description: string;
      tags: string[];
      bodyMarkdown: string;
    },
  ];
}>();

const name = ref('');
const slug = ref('');
const description = ref('');
const tags = ref<string[]>([]);
const body = ref('');
const snapshot = ref('');
const tagNeedle = ref('');

const isCreate = computed(() => !props.skill?.id);
const dirty = computed(() => snapshot.value !== serializeForm());

/** 字典选项按输入关键字过滤；new-value-mode 仍允许添加字典外新标签。 */
const filteredTagOptions = computed(() => {
  const all = props.tagOptions ?? [];
  const kw = tagNeedle.value.trim().toLowerCase();
  if (!kw) return all;
  return all.filter((t) => t.toLowerCase().includes(kw));
});

function onTagFilter(val: string, update: (fn: () => void) => void) {
  update(() => {
    tagNeedle.value = val;
  });
}

function serializeForm() {
  return JSON.stringify({
    name: name.value,
    slug: slug.value,
    description: description.value,
    tags: tags.value,
    body: body.value,
  });
}

function slugify(raw: string) {
  return String(raw || '')
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64);
}

function slugRule(v: string) {
  const s = String(v || '').trim();
  if (!s) return 'Slug 必填';
  if (!/^[a-z0-9][a-z0-9_-]*$/.test(s)) return '仅允许小写字母、数字、短横线、下划线';
  return true;
}

function defaultBody(n: string, d: string) {
  const title = n.trim() || 'New Skill';
  const desc = d.trim() || 'Describe what this skill does.';
  return `---\nname: ${title}\ndescription: ${desc}\n---\n\n# ${title}\n\n## When to use\n\n## Steps\n\n1. \n`;
}

function hydrate() {
  tagNeedle.value = '';
  if (props.skill) {
    name.value = props.skill.name || '';
    slug.value = props.skill.slug || '';
    description.value = props.skill.description || '';
    tags.value = (props.skill.tags || []).map((t) => t.name).filter(Boolean);
    body.value = props.initialBody || '';
  } else {
    name.value = '';
    slug.value = '';
    description.value = '';
    tags.value = [];
    body.value = defaultBody('', '');
  }
  snapshot.value = serializeForm();
}

watch(
  () => [props.modelValue, props.skill?.id, props.initialBody] as const,
  ([open]) => {
    if (open) hydrate();
  },
);

watch(name, (n) => {
  if (isCreate.value && !slug.value) {
    slug.value = slugify(n);
  }
});

async function submit() {
  const n = name.value.trim();
  const d = description.value.trim();
  const s = (isCreate.value ? slug.value : props.skill?.slug || slug.value).trim();
  const b = body.value.trim();
  if (!n || n.length > 80) {
    props.notify({ type: 'negative', message: '请填写有效名称（1–80 字符）' });
    return;
  }
  if (!d || d.length > 500) {
    props.notify({ type: 'negative', message: '请填写有效描述（1–500 字符）' });
    return;
  }
  if (isCreate.value && slugRule(s) !== true) {
    props.notify({ type: 'negative', message: String(slugRule(s)) });
    return;
  }
  if (!b) {
    props.notify({ type: 'negative', message: '正文不能为空' });
    return;
  }
  emit('submit', {
    id: props.skill?.id,
    name: n,
    slug: s,
    description: d,
    tags: [...tags.value],
    bodyMarkdown: b,
  });
}

async function tryClose() {
  if (dirty.value) {
    const ok = await props.confirm({
      title: '未保存的更改',
      message: '关闭将丢失当前编辑内容，确定继续？',
      okLabel: '放弃更改',
      okColor: 'negative',
      cancelLabel: '继续编辑',
    });
    if (!ok) return;
  }
  emit('update:modelValue', false);
}

function onDialogUpdate(val: boolean) {
  if (!val) void tryClose();
}
</script>
