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
        <q-separator class="q-my-md" />
        <div class="text-subtitle2 q-mb-xs">{{ t("settingsPage.a2aPublicBaseTitle") }}</div>
        <div class="text-caption text-grey-7 q-mb-sm">{{ t("settingsPage.a2aPublicBaseHint") }}</div>
        <q-input
          v-model.trim="a2aPublicBaseUrl"
          :label="t('settingsPage.a2aPublicBaseUrl')"
          :hint="effectiveA2AHint"
          outlined
          dense
          class="q-mb-sm"
        />
        <q-separator class="q-my-md" />
        <div class="text-subtitle2 q-mb-xs">{{ t("settingsPage.globalQuotaTitle") }}</div>
        <div class="text-caption text-grey-7 q-mb-sm">{{ t("settingsPage.globalQuotaHint") }}</div>
        <q-input
          v-model.number="globalMonthlyUsd"
          :label="t('settingsPage.globalQuotaUsd')"
          outlined
          dense
          type="number"
          min="0"
          step="0.01"
          prefix="$"
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
import { getA2AConfig } from "../features/a2a/api";

const { t } = useI18n();
const $q = useQuasar();
const rootDir = ref("");
const workDir = ref("");
const a2aPublicBaseUrl = ref("");
const effectiveA2AUrl = ref("");
const globalMonthlyUsd = ref<number | null>(null);
const updateTime = ref<string | undefined>(undefined);
const loading = ref(false);
const saving = ref(false);
const error = ref("");

const lastSavedLabel = computed(() => {
  const ts = updateTime.value;
  if (!ts) return "";
  return t("settingsPage.lastSaved", { time: ts });
});

const effectiveA2AHint = computed(() => {
  if (!effectiveA2AUrl.value) return t("settingsPage.a2aPublicBaseEmptyHint");
  return t("settingsPage.a2aPublicBaseEffective", { url: effectiveA2AUrl.value });
});

function usdToMicroUsd(usd: number | null | undefined): number {
  if (usd == null || !Number.isFinite(usd) || usd <= 0) return 0;
  return Math.round(usd * 1_000_000);
}

function microUsdToUsd(micro: number | undefined): number | null {
  if (micro == null || !Number.isFinite(micro) || micro <= 0) return null;
  return micro / 1_000_000;
}

onMounted(load);

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [res, a2aCfg] = await Promise.all([getSystemSettings(), getA2AConfig().catch(() => null)]);
    rootDir.value = res.rootDirectory ?? "";
    workDir.value = res.workDirectory ?? "";
    a2aPublicBaseUrl.value = res.a2aPublicBaseUrl ?? "";
    effectiveA2AUrl.value = a2aCfg?.public_base_url ?? "";
    globalMonthlyUsd.value = microUsdToUsd(res.globalMonthlyMicroUsd);
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
    const res = await updateSystemSettings(
      rootDir.value,
      workDir.value,
      usdToMicroUsd(globalMonthlyUsd.value),
      a2aPublicBaseUrl.value
    );
    rootDir.value = res.rootDirectory ?? "";
    workDir.value = res.workDirectory ?? "";
    a2aPublicBaseUrl.value = res.a2aPublicBaseUrl ?? "";
    const a2aCfg = await getA2AConfig().catch(() => null);
    effectiveA2AUrl.value = a2aCfg?.public_base_url ?? "";
    globalMonthlyUsd.value = microUsdToUsd(res.globalMonthlyMicroUsd);
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
