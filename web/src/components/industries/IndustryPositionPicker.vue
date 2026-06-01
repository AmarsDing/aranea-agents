<template>
  <div class="industry-position-picker">
    <div class="text-subtitle2 q-mb-sm">行业模板</div>
    <div class="row q-col-gutter-sm">
      <q-select
        v-model="industryKeyModel"
        class="col-12 col-sm-6"
        dense outlined
        emit-value map-options
        label="行业"
        :options="industryOptions"
        :loading="loadingIndustries"
        clearable
      >
        <template #prepend><q-icon name="business" /></template>
      </q-select>
      <q-select
        v-model="departmentKeyModel"
        class="col-12 col-sm-6"
        dense outlined
        emit-value map-options
        label="部门"
        :options="departmentOptions"
        :loading="loadingDepartments"
        :disable="!industryKey"
        clearable
      >
        <template #prepend><q-icon name="account_tree" /></template>
      </q-select>
    </div>
    <div class="row q-col-gutter-sm q-mt-sm">
      <q-select
        v-model="positionKeyModel"
        class="col-12 col-sm-6"
        dense outlined
        emit-value map-options
        label="岗位"
        :options="positionOptions"
        :loading="loadingPositions"
        :disable="!departmentKey"
        clearable
      >
        <template #prepend><q-icon name="badge" /></template>
      </q-select>
      <q-select
        v-model="variantModel"
        class="col-12 col-sm-6"
        dense outlined
        emit-value map-options
        label="方向"
        :options="variantOptions"
        :disable="!positionKey"
      >
        <template #prepend><q-icon name="alt_route" /></template>
      </q-select>
    </div>
    <div v-if="previewSummary" class="industry-position-picker__preview q-mt-sm">
      <q-icon name="info" size="xs" class="q-mr-xs" />
      <span class="text-caption">{{ previewSummary }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue"
import type { Industry, Department, Position, PositionPromptResult } from "../../features/industries/types"

const props = defineProps<{
  industryKey: string
  departmentKey: string
  positionKey: string
  variant: string
  industries: Industry[]
  departments: Department[]
  positions: Position[]
  loadingIndustries: boolean
  loadingDepartments: boolean
  loadingPositions: boolean
  variantOptions: Array<{ label: string; value: string }>
  promptResult: PositionPromptResult | null
}>()

const emit = defineEmits<{
  "update:industryKey": [value: string]
  "update:departmentKey": [value: string]
  "update:positionKey": [value: string]
  "update:variant": [value: string]
}>()

const industryKeyModel = computed({
  get: () => props.industryKey,
  set: (v: string) => emit("update:industryKey", v),
})
const departmentKeyModel = computed({
  get: () => props.departmentKey,
  set: (v: string) => emit("update:departmentKey", v),
})
const positionKeyModel = computed({
  get: () => props.positionKey,
  set: (v: string) => emit("update:positionKey", v),
})
const variantModel = computed({
  get: () => props.variant,
  set: (v: string) => emit("update:variant", v),
})

const industryOptions = computed(() =>
  props.industries.map(i => ({ label: i.name, value: i.key }))
)
const departmentOptions = computed(() =>
  props.departments.map(d => ({ label: d.name, value: d.key }))
)
const positionOptions = computed(() =>
  props.positions.map(p => ({ label: p.name, value: p.key }))
)

const previewSummary = computed(() => {
  if (!props.positionKey) return ""
  const ind = props.industries.find(i => i.key === props.industryKey)
  const dept = props.departments.find(d => d.key === props.departmentKey)
  const pos = props.positions.find(p => p.key === props.positionKey)
  const parts: string[] = []
  if (ind) parts.push(ind.name)
  if (dept) parts.push(dept.name)
  if (pos) parts.push(pos.name)
  const variantLabel = props.variantOptions.find(v => v.value === props.variant)?.label
  if (variantLabel && variantLabel !== "通用") parts.push(variantLabel)
  const breadcrumb = parts.join(" / ")
  if (!props.promptResult) return breadcrumb
  const desc = props.promptResult.positionDescription || props.promptResult.departmentDescription || props.promptResult.industryDescription
  if (!desc) return breadcrumb
  const short = desc.length > 80 ? desc.slice(0, 80) + "…" : desc
  return `${breadcrumb} — ${short}`
})
</script>
