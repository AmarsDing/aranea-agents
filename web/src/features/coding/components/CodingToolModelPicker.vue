<template>
  <div class="model-pick">
    <div class="model-pick__head">
      <strong>{{ t('cap.coding.tool.edit.model') }}</strong>
      <span class="model-pick__hint" :class="{ 'is-fail': !!error }" role="status">{{ hint }}</span>
      <q-btn
        flat
        dense
        round
        size="sm"
        icon="refresh"
        :loading="loading"
        :aria-label="t('cap.coding.tool.edit.modelsRefresh')"
        @click="load"
      >
        <q-tooltip>{{ t('cap.coding.tool.edit.modelsRefresh') }}</q-tooltip>
      </q-btn>
    </div>

    <q-input
      v-if="models.length > 8 || filter"
      v-model="filter"
      dense
      outlined
      clearable
      :placeholder="t('cap.coding.tool.edit.modelSearch')"
    >
      <template #prepend><q-icon name="search" size="18px" /></template>
    </q-input>

    <!-- 一行一个模型：名称（版本）· 默认标记 · ID · 说明 · 支持的强度，选中打勾 -->
    <q-list
      dense
      bordered
      separator
      class="model-pick__list"
      role="listbox"
      :aria-label="t('cap.coding.tool.edit.model')"
    >
      <q-item clickable :active="!model" role="option" :aria-selected="!model" @click="pick('')">
        <q-item-section>
          <q-item-label>{{ t('cap.coding.tool.edit.modelDefault') }}</q-item-label>
        </q-item-section>
        <q-item-section side><q-icon v-if="!model" name="check" color="primary" /></q-item-section>
      </q-item>
      <q-item
        v-for="m in shown"
        :key="m.id"
        clickable
        :active="m.id === model"
        role="option"
        :aria-selected="m.id === model"
        @click="pick(m.id)"
      >
        <q-item-section>
          <q-item-label class="model-pick__name">
            {{ m.name || m.id }}
            <q-badge v-if="m.isDefault" outline color="primary" :label="t('cap.coding.tool.edit.modelIsDefault')" />
          </q-item-label>
          <q-item-label v-if="m.name && m.name !== m.id" caption class="mono">{{ m.id }}</q-item-label>
          <q-item-label v-if="m.description" caption lines="2">{{ m.description }}</q-item-label>
        </q-item-section>
        <q-item-section v-if="effortsOf(m).length" side class="model-pick__range">
          {{ effortRange(effortsOf(m)) }}
        </q-item-section>
        <q-item-section side><q-icon v-if="m.id === model" name="check" color="primary" /></q-item-section>
      </q-item>
      <!-- 当前设置不在列表里（手填过、或厂商下架了）也要看得见 -->
      <q-item v-if="model && !known" active role="option" aria-selected="true">
        <q-item-section>
          <q-item-label class="mono">{{ model }}</q-item-label>
          <q-item-label caption>{{ t('cap.coding.tool.edit.modelNotListed') }}</q-item-label>
        </q-item-section>
        <q-item-section side><q-icon name="check" color="primary" /></q-item-section>
      </q-item>
      <q-item v-if="customId" clickable role="option" :aria-selected="false" @click="pick(customId)">
        <q-item-section>
          <q-item-label>{{ t('cap.coding.tool.edit.modelUseTyped', { id: customId }) }}</q-item-label>
        </q-item-section>
      </q-item>
    </q-list>

    <div v-if="preset.supportsEffort && effortOptions.length" class="model-pick__effort">
      <div class="model-pick__head">
        <strong>{{ t('cap.coding.tool.edit.effort') }}</strong>
        <span class="model-pick__hint">{{ t('cap.coding.tool.edit.effortHint') }}</span>
      </div>
      <q-btn-toggle
        :model-value="effort"
        no-caps
        unelevated
        dense
        toggle-color="primary"
        class="model-pick__toggle"
        :options="effortOptions"
        @update:model-value="effort = $event"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { AgentPreset, ToolModel } from '../../fleet/api';
import { errText, listToolModels } from '../../fleet/api';

// 模型与思考强度：列表实时取（厂商接口 / 工具自己的列表命令 / 内置在售型号），像客户端的模型菜单那样
// 显示名称版本、说明、默认标记与各型号支持的强度；强度档位跟着所选模型走
const model = defineModel<string>('model', { default: '' });
const effort = defineModel<string>('effort', { default: '' });
const props = defineProps<{ preset: AgentPreset }>();
const { t, te } = useI18n();

const models = ref<ToolModel[]>([]);
const loading = ref(false);
const error = ref('');
const source = ref('');
const at = ref('');
const filter = ref('');

const known = computed(() => models.value.some((m) => m.id === model.value));
const shown = computed(() => {
  const f = (filter.value ?? '').trim().toLowerCase();
  if (!f) return models.value;
  return models.value.filter((m) => `${m.id} ${m.name} ${m.description}`.toLowerCase().includes(f));
});
// 搜索框里输的不是列表里的模型：允许直接用它（列表可能不全）
const customId = computed(() => {
  const f = (filter.value ?? '').trim();
  return f && /^[A-Za-z0-9][\w.:/\-[\]=,]*$/.test(f) && !models.value.some((m) => m.id === f) ? f : '';
});
const selected = computed(() => models.value.find((m) => m.id === model.value));
const effortsOf = (m: ToolModel) => (m.efforts.length ? m.efforts : props.preset.efforts);

const effortName = (e: string) => (te(`cap.coding.tool.effortLevel.${e}`) ? t(`cap.coding.tool.effortLevel.${e}`) : e);
const effortRange = (list: string[]) =>
  list.length > 1 ? `${effortName(list[0])}–${effortName(list[list.length - 1])}` : effortName(list[0]);
const effortOptions = computed(() => {
  const list = selected.value ? effortsOf(selected.value) : props.preset.efforts;
  const def = selected.value?.defaultEffort;
  return [
    {
      label: def
        ? t('cap.coding.tool.edit.effortDefaultIs', { e: effortName(def) })
        : t('cap.coding.tool.edit.effortDefault'),
      value: '',
    },
    ...list.map((e) => ({ label: effortName(e), value: e })),
  ];
});
// 换了模型、新模型不支持当前强度时回到默认
watch(selected, (m) => {
  if (m && effort.value && !effortsOf(m).includes(effort.value)) effort.value = '';
});

const hint = computed(() => {
  if (error.value) return error.value;
  if (!source.value) return t('cap.coding.tool.edit.modelHint');
  const src = source.value === 'static' && !props.preset.keyEnv ? 'staticNoKey' : source.value;
  return t(`cap.coding.tool.edit.modelsFrom.${src}`, {
    name: props.preset.label,
    n: models.value.length,
    at: at.value,
  });
});

function pick(id: string) {
  model.value = id;
  filter.value = '';
}

async function load() {
  loading.value = true;
  error.value = source.value = '';
  try {
    const r = await listToolModels(props.preset.id);
    models.value = r.models;
    error.value = r.error;
    source.value = r.source;
    at.value = new Date().toLocaleTimeString();
  } catch (e) {
    error.value = errText(e);
  } finally {
    loading.value = false;
  }
}

watch(
  () => props.preset.id,
  () => {
    models.value = [];
    filter.value = '';
    void load();
  },
  { immediate: true },
);
</script>

<style scoped lang="sass">
.model-pick
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
.model-pick__head
  display: flex
  flex-wrap: wrap
  align-items: center
  gap: var(--nt-space-1) var(--nt-space-2)
  color: var(--nt-text-1)
  strong
    font-size: var(--nt-fs-caption)
.model-pick__hint
  flex: 1
  min-width: 0
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
  &.is-fail
    color: var(--nt-danger)
.model-pick__list
  max-height: 320px
  overflow: auto
  border-radius: var(--nt-radius-m)
.model-pick__name
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  color: var(--nt-text-1)
  font-weight: 600
.model-pick__range
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
.model-pick__effort
  display: flex
  flex-direction: column
  gap: var(--nt-space-1)
.model-pick__toggle
  flex-wrap: wrap
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
.mono
  font-family: var(--nt-font-mono)
</style>
