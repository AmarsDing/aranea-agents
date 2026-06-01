<template>
  <q-dialog v-model="dialogModel">
    <q-card class="avatar-picker-card app-dialog-card app-dialog-card--md">
      <q-card-section class="avatar-picker-card__header">
        <div>
          <div class="text-h6">{{ title }}</div>
          <div class="text-caption text-grey-7">{{ subtitle }}</div>
        </div>
        <q-btn flat round icon="close" aria-label="关闭" @click="dialogModel = false" />
      </q-card-section>

      <q-tabs v-model="tab" dense class="avatar-tabs">
        <q-tab name="system" :label="isChannel ? '平台内置' : '内置'" />
        <q-tab name="mine" :label="isChannel ? '自定义' : '我的上传'" />
      </q-tabs>
      <q-separator />

      <div class="avatar-picker-scroll">
        <div v-if="loading" class="avatar-grid">
          <q-skeleton v-for="i in 10" :key="i" type="QAvatar" size="72px" />
        </div>
        <template v-else-if="tab === 'system'">
          <div v-for="group in systemGroups" :key="group.key" class="avatar-group">
            <div class="avatar-group__label text-subtitle2">{{ group.label }}</div>
            <div class="avatar-grid">
              <button
                v-for="asset in group.items"
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
          </div>
        </template>
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

        <q-banner v-if="isEmpty" rounded class="avatar-picker-empty-banner">
          {{ emptyHint }}
        </q-banner>
      </div>

      <q-separator />
      <q-card-section class="avatar-upload-row">
        <input ref="fileInput" class="hidden-input" type="file" accept="image/png,image/jpeg,image/webp" @change="onFileChange" />
        <q-btn outline rounded color="primary" icon="upload" :label="uploadLabel" :loading="uploading" @click="fileInput?.click()" />
        <div class="text-caption text-grey-7">{{ uploadHint }}</div>
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
import { useAvatarPickerDialog, type AvatarPickerScope } from "../../features/avatar/useAvatarPickerDialog";
import ResolvedAvatarImg from "./ResolvedAvatarImg.vue";
const props = withDefaults(
  defineProps<{
    modelValue: string;
    open: boolean;
    scope?: AvatarPickerScope;
  }>(),
  { scope: "agent" }
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "update:open": [value: boolean];
}>();

const dialogModel = computed({
  get: () => props.open,
  set: (value: boolean) => emit("update:open", value)
});

const isChannel = computed(() => props.scope === "channel");

const { tab, loading, uploading, selectedId, fileInput, visibleAssets, systemGroups, uploadFromFile } = useAvatarPickerDialog({
  modelValue: toRef(props, "modelValue"),
  open: toRef(props, "open"),
  scope: props.scope
});

const isEmpty = computed(() => {
  if (loading.value) return false;
  if (tab.value === "system") return systemGroups.value.length === 0;
  return visibleAssets.value.length === 0;
});

const title = computed(() => (isChannel.value ? "选择平台图标" : "选择头像"));
const subtitle = computed(() =>
  isChannel.value
    ? "内置平台图标来自 avatar_assets；也可上传自定义图标。"
    : "从数据库内置头像中选择，或上传自己的图片。"
);
const emptyHint = computed(() => (isChannel.value ? "暂无平台图标，请重启后端完成 seed 或上传自定义图标。" : "暂无头像。可以先上传一张本地图片。"));
const uploadLabel = computed(() => (isChannel.value ? "上传自定义图标" : "从本地上传"));
const uploadHint = computed(() => "支持 PNG / JPEG / WebP，最大 2MB。上传后图片写入数据库 BLOB。");

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
