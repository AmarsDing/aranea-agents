<template>
  <q-page class="q-pa-md system-settings-page">
    <q-card flat bordered>
      <q-card-section class="text-h6">{{ t("settingsPage.title") }}</q-card-section>
      <q-separator />
      <q-card-section>
        <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">{{ error }}</q-banner>
        <q-input
          v-model="rootDir"
          :label="t('settingsPage.rootDir')"
          :hint="t('settingsPage.rootDirHint')"
          outlined
          dense
          class="q-mb-sm"
        />
        <q-input
          v-model="workDir"
          :label="t('settingsPage.workDir')"
          :hint="t('settingsPage.workDirHint')"
          outlined
          dense
          class="q-mb-sm"
        />
        <div v-if="lastSavedLabel" class="text-caption text-grey-7 q-mb-md">{{ lastSavedLabel }}</div>
        <div class="row q-gutter-sm">
          <q-btn color="primary" unelevated no-caps :loading="saving" :label="t('settingsPage.save')" @click="save" />
          <q-btn outline color="primary" no-caps :loading="loading" :label="t('settingsPage.reload')" @click="load" />
        </div>
      </q-card-section>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { getSystemSettings, updateSystemSettings } from "../features/system-settings/api";

const { t } = useI18n();
const $q = useQuasar();
const rootDir = ref("");
const workDir = ref("");
const updateTime = ref<string | undefined>(undefined);
const loading = ref(false);
const saving = ref(false);
const error = ref("");

const lastSavedLabel = computed(() => {
  const ts = updateTime.value;
  if (!ts) return "";
  return t("settingsPage.lastSaved", { time: ts });
});

onMounted(load);

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const res = await getSystemSettings();
    rootDir.value = res.rootDirectory ?? "";
    workDir.value = res.workDirectory ?? "";
    updateTime.value = res.updateTime;
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  error.value = "";
  try {
    const res = await updateSystemSettings(rootDir.value, workDir.value);
    rootDir.value = res.rootDirectory ?? "";
    workDir.value = res.workDirectory ?? "";
    updateTime.value = res.updateTime;
    $q.notify({ type: "positive", message: t("settingsPage.saveOk") });
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : String(e);
    $q.notify({ type: "negative", message: t("settingsPage.saveFailed") });
  } finally {
    saving.value = false;
  }
}
</script>
