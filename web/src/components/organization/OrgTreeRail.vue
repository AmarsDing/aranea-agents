<template>
  <nav class="bz-rail" :aria-label="title">
    <div class="bz-rail__head">
      <span>{{ title }}</span>
      <span v-if="total !== undefined" class="bz-num">{{ total }}</span>
      <slot name="head-action" />
    </div>

    <button
      v-for="entry in entries"
      :key="entry.key"
      type="button"
      class="bz-nv"
      :class="{ 'bz-nv--on': selected === entry.key }"
      :aria-current="selected === entry.key ? 'true' : undefined"
      @click="emit('select', entry.key)"
    >
      <q-icon :name="entry.icon" size="18px" aria-hidden="true" />
      <span class="bz-nv__name">{{ entry.label }}</span>
      <span v-if="entry.count !== undefined" class="bz-nv__n bz-num">{{ entry.count }}</span>
    </button>

    <div v-if="sectionLabel && companies.length" class="bz-sect">{{ sectionLabel }}</div>

    <div v-for="company in companies" :key="company.id" class="bz-co">
      <div class="bz-co__row">
        <q-btn
          flat
          dense
          round
          size="sm"
          class="bz-co__toggle"
          :icon="isOpen(company.id) ? 'expand_more' : 'chevron_right'"
          :aria-expanded="isOpen(company.id)"
          :aria-label="t(isOpen(company.id) ? 'agentRoster.collapse' : 'agentRoster.expand', { name: company.name })"
          @click="toggle(company.id)"
        />
        <button
          type="button"
          class="bz-nv bz-nv--co"
          :class="{ 'bz-nv--on': selected === company.id, 'bz-nv--off': !company.enabled }"
          :aria-current="selected === company.id ? 'true' : undefined"
          @click="pickCompany(company.id)"
        >
          <span class="bz-nv__name">{{ company.name }}</span>
          <span v-if="company.count !== undefined" class="bz-nv__n bz-num">{{ company.count }}</span>
        </button>
      </div>
      <div v-if="isOpen(company.id)" class="bz-co__depts">
        <button
          v-for="dept in company.departments"
          :key="dept.id"
          type="button"
          class="bz-nv bz-nv--dept"
          :class="{ 'bz-nv--on': selected === dept.id, 'bz-nv--off': !dept.enabled }"
          :aria-current="selected === dept.id ? 'true' : undefined"
          @click="emit('select', dept.id)"
        >
          <q-icon v-if="dept.icon" :name="dept.icon" size="14px" class="bz-nv__icon" aria-hidden="true" />
          <span class="bz-nv__name">{{ dept.name }}</span>
          <span v-if="dept.tag" class="bz-vac" :title="dept.tagTitle">{{ dept.tag }}</span>
          <span v-if="dept.count !== undefined" class="bz-nv__n bz-num">{{ dept.count }}</span>
        </button>
        <p v-if="!company.departments.length" class="bz-rail__empty">{{ emptyDeptText }}</p>
      </div>
    </div>

    <div v-if="$slots.foot" class="bz-rail__foot"><slot name="foot" /></div>
  </nav>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { TreeRailCompany, TreeRailEntry } from './treeRail';

const props = withDefaults(
  defineProps<{
    title: string;
    total?: number;
    entries?: TreeRailEntry[];
    sectionLabel?: string;
    companies: TreeRailCompany[];
    selected: string;
    emptyDeptText?: string;
  }>(),
  { total: undefined, entries: () => [], sectionLabel: '', emptyDeptText: '' },
);
const emit = defineEmits<{ select: [key: string] }>();
const { t } = useI18n();

// 只展开当前选中项所在的公司（四家公司全展开有四十多行），手动展开的保留
const open = ref(new Set<string>());
watch(
  () => [props.selected, props.companies.length] as const,
  () => {
    const id = props.companies.find(
      (c) => c.id === props.selected || c.departments.some((d) => d.id === props.selected),
    )?.id;
    if (id && !open.value.has(id)) open.value = new Set([...open.value, id]);
  },
  { immediate: true },
);
const isOpen = (id: string) => open.value.has(id);
function toggle(id: string) {
  const next = new Set(open.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  open.value = next;
}
function pickCompany(id: string) {
  if (!open.value.has(id)) toggle(id);
  emit('select', id);
}
</script>
