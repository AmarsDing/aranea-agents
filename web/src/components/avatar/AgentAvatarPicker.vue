<template>
  <q-dialog v-model="dialogModel">
    <q-card class="avatar-picker-card">
      <q-card-section class="avatar-picker-card__header">
        <div>
          <div class="text-h6">选择头像</div>
          <div class="text-caption text-grey-7">从数据库内置头像中选择，或上传自己的图片。</div>
        </div>
        <q-btn flat round icon="close" @click="dialogModel = false" />
      </q-card-section>

      <q-tabs v-model="tab" dense class="avatar-tabs">
        <q-tab name="system" label="内置" />
        <q-tab name="mine" label="我的上传" />
      </q-tabs>
      <q-separator />

      <q-card-section>
        <div v-if="loading" class="avatar-grid">
          <q-skeleton v-for="i in 10" :key="i" type="QAvatar" size="72px" />
        </div>
        <div v-else class="avatar-grid">
          <button
            v-for="asset in visibleAssets"
            :key="asset.id"
            class="avatar-option"
            :class="{ 'is-selected': selectedId === asset.id }"
            type="button"
            @click="selectedId = asset.id"
          >
            <q-avatar rounded size="72px">
              <resolved-avatar-img :icon="asset.id" :alt="asset.name" />
            </q-avatar>
            <span>{{ asset.name }}</span>
            <q-icon v-if="selectedId === asset.id" name="check_circle" class="avatar-option__check" />
          </button>
        </div>

        <q-banner v-if="!loading && visibleAssets.length === 0" rounded class="avatar-picker-empty-banner">
          暂无头像。可以先上传一张本地图片。
        </q-banner>
      </q-card-section>

      <q-separator />
      <q-card-section class="avatar-upload-row">
        <input ref="fileInput" class="hidden-input" type="file" accept="image/png,image/jpeg,image/webp" @change="onFileChange" />
        <q-btn outline rounded color="primary" icon="upload" label="从本地上传" :loading="uploading" @click="fileInput?.click()" />
        <div class="text-caption text-grey-7">支持 PNG / JPEG / WebP，最大 2MB。上传后图片写入数据库 BLOB。</div>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat rounded label="取消" @click="dialogModel = false" />
        <q-btn color="primary" rounded unelevated label="确定" :disable="!selectedId" @click="confirm" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, toRef } from "vue";
import { useAvatarPickerDialog } from "../../features/avatar/useAvatarPickerDialog";
import ResolvedAvatarImg from "./ResolvedAvatarImg.vue";

const props = defineProps<{
  modelValue: string;
  open: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "update:open": [value: boolean];
}>();

const dialogModel = computed({
  get: () => props.open,
  set: (value: boolean) => emit("update:open", value)
});

const { tab, loading, uploading, selectedId, fileInput, visibleAssets, uploadFromFile } = useAvatarPickerDialog({
  modelValue: toRef(props, "modelValue"),
  open: toRef(props, "open")
});

async function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  input.value = "";
  if (!file) return;
  await uploadFromFile(file);
}

function confirm() {
  emit("update:modelValue", selectedId.value);
  emit("update:open", false);
}
</script>

<style scoped>
.avatar-picker-card {
  width: 560px;
  max-width: 94vw;
  border: 1px solid var(--glass-border);
  border-radius: 26px;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-elevated));
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated));
  box-shadow: none;
  overflow: hidden;
}

.avatar-picker-card__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 22px 24px;
  background: var(--glass-elevated);
  box-shadow: var(--glass-inner-highlight);
}

.avatar-picker-card__header .text-h6 {
  color: var(--color-text-primary);
}

.avatar-tabs {
  padding: 0 16px;
  background: var(--glass-surface);
}

.avatar-tabs :deep(.q-tab) {
  font-weight: 700;
  color: var(--color-text-secondary);
}

.avatar-tabs :deep(.q-tab--active) {
  color: var(--color-text-primary);
  background: color-mix(in srgb, var(--color-accent) 14%, transparent);
}

.avatar-tabs :deep(.q-tab__indicator) {
  height: 3px;
  border-radius: 2px;
  background: var(--color-accent);
}

.avatar-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(96px, 1fr));
  gap: 14px;
  min-height: 188px;
}

.avatar-option {
  position: relative;
  display: grid;
  justify-items: center;
  gap: 8px;
  padding: 12px 8px;
  border: 1px solid var(--glass-border);
  border-radius: 18px;
  background: var(--glass-elevated);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  box-shadow: var(--glass-inner-highlight);
  color: var(--color-text-primary);
  cursor: pointer;
  font: inherit;
  transition:
    transform 180ms ease,
    border-color 180ms ease,
    background 180ms ease;
}

.avatar-option:hover,
.avatar-option.is-selected {
  transform: translateY(-2px);
  border-color: var(--glass-border-hover, var(--glass-border));
  background: var(--glass-surface-hover);
}

.avatar-option.is-selected {
  box-shadow: var(--glass-inner-highlight), 0 0 0 1px color-mix(in srgb, var(--color-accent) 35%, transparent);
}

.avatar-option span {
  max-width: 100%;
  overflow: hidden;
  font-size: 12px;
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-text-secondary);
}

.avatar-option__check {
  position: absolute;
  top: 8px;
  right: 8px;
  color: var(--color-accent);
  background: var(--glass-surface);
  border-radius: 999px;
}

.avatar-picker-empty-banner {
  border: 1px solid var(--glass-border);
  background: var(--glass-elevated);
  color: var(--color-text-primary);
}

.avatar-upload-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  background: var(--glass-surface);
}

.avatar-upload-row :deep(.q-btn) {
  min-height: 40px;
  font-weight: 700;
}

.avatar-picker-card :deep(.q-card__actions) {
  padding: 14px 22px 20px;
  background: var(--glass-elevated);
  border-top: 1px solid var(--glass-border);
}

.avatar-picker-card :deep(.q-card__actions .q-btn) {
  min-height: 40px;
  padding: 0 18px;
  font-weight: 700;
}

.avatar-picker-card :deep(.text-grey-7) {
  color: var(--color-text-secondary) !important;
}

.hidden-input {
  display: none;
}

body.body--dark .avatar-picker-card {
  border-color: var(--glass-border);
  background: var(--glass-surface);
}

body.body--dark .avatar-picker-card__header {
  background: var(--glass-elevated);
}

body.body--dark .avatar-tabs :deep(.q-tab--active) {
  background: color-mix(in srgb, var(--color-accent) 22%, transparent);
}

body.body--dark .avatar-option.is-selected {
  box-shadow: var(--glass-inner-highlight), 0 0 0 1px color-mix(in srgb, var(--color-accent) 42%, transparent);
}

body.body--dark .avatar-option__check {
  background: rgba(18, 24, 34, 0.92);
}
</style>
