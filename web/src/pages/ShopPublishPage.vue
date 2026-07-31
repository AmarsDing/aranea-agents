<template>
  <q-page class="app-standard-page shop-publish-page">
    <AppPageHero
      :kicker="t('shopPage.kicker')"
      :title="t('shopPage.publishTitle')"
      :subtitle="t('shopPage.publishSubtitle')"
    />

    <q-card flat class="app-glass-panel shop-publish-page__wizard">
      <q-stepper v-model="step" color="primary" animated flat contracted>
        <!-- 步骤 1：类型 + 基本信息 -->
        <q-step :name="1" :title="t('shopPage.publish.stepBasic')" icon="info" :done="step > 1">
          <div class="text-weight-bold q-mb-sm">{{ t('shopPage.publish.pickType') }}</div>
          <publish-type-select v-model="draft.type" class="q-mb-lg" />

          <div class="row q-col-gutter-md">
            <div class="col-12 col-md-6">
              <q-input
                v-model="draft.name"
                outlined
                dense
                :label="t('shopPage.publish.fieldName')"
                :rules="[(v) => !!v || t('common.required')]"
              />
            </div>
            <div class="col-12 col-md-6">
              <q-input
                v-model="draft.slug"
                outlined
                dense
                :label="t('shopPage.publish.fieldSlug')"
                :hint="t('shopPage.publish.fieldSlugHint')"
              />
            </div>
            <div class="col-12">
              <q-input
                v-model="draft.tagline"
                outlined
                dense
                :label="t('shopPage.publish.fieldTagline')"
                :hint="t('shopPage.publish.fieldTaglineHint')"
                counter
                maxlength="60"
              />
            </div>
            <div class="col-12 col-md-6">
              <q-select
                v-model="draft.category"
                outlined
                dense
                emit-value
                map-options
                :label="t('shopPage.publish.fieldCategory')"
                :options="categoryOptions"
              />
            </div>
            <div class="col-6 col-md-3">
              <q-select
                v-model="draft.priceModel"
                outlined
                dense
                emit-value
                map-options
                :label="t('shopPage.publish.fieldPriceModel')"
                :options="priceModelOptions"
              />
            </div>
            <div class="col-6 col-md-3">
              <q-input
                v-model.number="draft.priceYuan"
                outlined
                dense
                type="number"
                min="0"
                :disable="draft.priceModel === 'free' || draft.priceModel === 'enterprise'"
                :label="t('shopPage.publish.fieldPrice')"
                prefix="¥"
              />
            </div>
          </div>
        </q-step>

        <!-- 步骤 2：详细内容 -->
        <q-step :name="2" :title="t('shopPage.publish.stepContent')" icon="description" :done="step > 2">
          <div class="row q-col-gutter-md">
            <div class="col-12">
              <q-input
                v-model="draft.description"
                outlined
                dense
                autogrow
                type="textarea"
                :label="t('shopPage.publish.fieldDescription')"
              />
            </div>
            <div class="col-12">
              <div class="row items-center justify-between q-mb-xs">
                <span class="text-weight-medium">{{ t('shopPage.publish.fieldReadme') }}</span>
                <q-toggle v-model="readmePreview" dense :label="t('shopPage.publish.readmePreview')" />
              </div>
              <q-input
                v-if="!readmePreview"
                v-model="draft.readme"
                outlined
                dense
                type="textarea"
                :input-style="{ minHeight: '220px', fontFamily: 'monospace' }"
                :placeholder="t('shopPage.publish.readmePlaceholder')"
              />
              <q-card v-else flat bordered class="q-pa-md shop-publish-page__readme-preview">
                <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->
                <div class="chat-message-prose" v-html="readmeHtml"></div>
              </q-card>
            </div>
            <div class="col-6 col-md-3">
              <q-input
                v-model="draft.version"
                outlined
                dense
                :label="t('shopPage.publish.fieldVersion')"
                placeholder="1.0.0"
              />
            </div>
            <div class="col-6 col-md-3">
              <q-input
                v-model="draft.compatibility"
                outlined
                dense
                :label="t('shopPage.publish.fieldCompat')"
                placeholder="aranea>=1.5"
              />
            </div>
            <div class="col-12 col-md-6">
              <q-select
                v-model="draft.tags"
                outlined
                dense
                multiple
                use-chips
                use-input
                new-value-mode="add-unique"
                :label="t('shopPage.publish.fieldTags')"
              />
            </div>
          </div>
        </q-step>

        <!-- 步骤 3：组织整包（仅 org_bundle） -->
        <q-step
          v-if="draft.type === 'org_bundle'"
          :name="3"
          :title="t('shopPage.publish.stepOrg')"
          icon="corporate_fare"
          :done="step > 3"
        >
          <org-bundle-picker :nodes="orgPickTree" @change="onOrgChange" />
        </q-step>

        <!-- 步骤 4：确认发布 -->
        <q-step :name="confirmStep" :title="t('shopPage.publish.stepConfirm')" icon="check_circle">
          <q-card flat bordered class="q-pa-md shop-publish-page__summary">
            <div class="row q-col-gutter-sm">
              <div class="col-12 row items-center q-gutter-sm">
                <asset-type-icon v-if="draft.type" :type="draft.type" size="40px" />
                <div>
                  <div class="text-h6 text-weight-bold">{{ draft.name || t('shopPage.publish.unnamed') }}</div>
                  <div class="text-caption text-grey-7">{{ draft.slug || '—' }}</div>
                </div>
              </div>
              <div class="col-12 col-md-6">
                <q-markup-table flat dense class="shop-publish-page__summary-table">
                  <tbody>
                    <tr>
                      <td>{{ t('shopPage.publish.fieldType') }}</td>
                      <td>{{ typeLabel }}</td>
                    </tr>
                    <tr>
                      <td>{{ t('shopPage.publish.fieldCategory') }}</td>
                      <td>{{ draft.category || '—' }}</td>
                    </tr>
                    <tr>
                      <td>{{ t('shopPage.publish.fieldPriceModel') }}</td>
                      <td>{{ priceLabel }}</td>
                    </tr>
                    <tr>
                      <td>{{ t('shopPage.publish.fieldVersion') }}</td>
                      <td>v{{ draft.version || '1.0.0' }}</td>
                    </tr>
                  </tbody>
                </q-markup-table>
              </div>
              <div v-if="draft.type === 'org_bundle'" class="col-12 col-md-6">
                <div class="text-weight-medium q-mb-xs">{{ t('shopPage.publish.orgSummary') }}</div>
                <div class="row q-gutter-sm">
                  <q-chip dense icon="corporate_fare" color="primary" text-color="white">{{
                    orgSummary.departments
                  }}</q-chip>
                  <q-chip dense icon="badge" color="teal" text-color="white">{{ orgSummary.positions }}</q-chip>
                  <q-chip dense icon="smart_toy" color="deep-purple" text-color="white">{{ orgSummary.agents }}</q-chip>
                </div>
              </div>
            </div>
          </q-card>
          <q-banner rounded class="app-info-banner q-mt-md">{{ t('shopPage.publish.confirmHint') }}</q-banner>
        </q-step>

        <template #navigation>
          <q-stepper-navigation class="row justify-end q-gutter-sm q-pa-md">
            <q-btn v-if="step > 1" flat no-caps :label="t('common.back')" @click="prevStep" />
            <q-btn
              v-if="!isConfirmStep"
              color="primary"
              unelevated
              no-caps
              :label="t('common.next')"
              :disable="!canNext"
              @click="nextStep"
            />
            <q-btn
              v-else
              color="primary"
              unelevated
              no-caps
              icon="publish"
              :label="t('shopPage.publish.submit')"
              :loading="submitting"
              @click="submit"
            />
          </q-stepper-navigation>
        </template>
      </q-stepper>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import MarkdownIt from 'markdown-it';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AssetTypeIcon from '../components/ecosystem/AssetTypeIcon.vue';
import OrgBundlePicker from '../components/ecosystem/OrgBundlePicker.vue';
import PublishTypeSelect from '../components/ecosystem/PublishTypeSelect.vue';
import { ASSET_TYPE_META, PRICE_MODELS } from '../features/ecosystem/marketUi';
import type { MarketAssetType, PriceModel } from '../features/ecosystem/types';
import { useEcosystemStore } from '../stores/ecosystem';

const { t } = useI18n();
const router = useRouter();
const $q = useQuasar();
const store = useEcosystemStore();
const { categories, orgPickTree } = storeToRefs(store);

const step = ref(1);
const submitting = ref(false);
const readmePreview = ref(false);

const draft = reactive({
  type: '' as MarketAssetType | '',
  name: '',
  slug: '',
  tagline: '',
  category: '',
  priceModel: 'free' as PriceModel,
  priceYuan: 0,
  description: '',
  readme: '',
  version: '1.0.0',
  compatibility: 'aranea>=1.5',
  tags: [] as string[],
});

const orgSummary = reactive({ departments: 0, positions: 0, agents: 0 });

function onOrgChange(s: { departments: number; positions: number; agents: number }) {
  Object.assign(orgSummary, s);
}

const md = new MarkdownIt({ html: false, linkify: true, breaks: false });
const readmeHtml = computed(() => md.render(draft.readme || `*${t('shopPage.publish.readmeEmpty')}*`));

/** 步骤编号：org_bundle 为 1→2→3→4，其它类型 1→2→4（跳过步骤 3） */
const confirmStep = computed(() => (draft.type === 'org_bundle' ? 4 : 3));
const isConfirmStep = computed(() => step.value === confirmStep.value);

const categoryOptions = computed(() => {
  const opts: { label: string; value: string }[] = [];
  const walk = (nodes: typeof categories.value, depth: number) => {
    for (const n of nodes) {
      opts.push({ label: `${'　'.repeat(depth)}${n.label}`, value: n.key });
      if (n.children) walk(n.children, depth + 1);
    }
  };
  walk(categories.value, 0);
  return opts;
});

const priceModelOptions = computed(() => PRICE_MODELS.map((m) => ({ value: m, label: t(`shopPage.priceModel.${m}`) })));
const typeLabel = computed(() => (draft.type ? t(`shopPage.type.${ASSET_TYPE_META[draft.type].labelKey}`) : '—'));
const priceLabel = computed(() => {
  if (draft.priceModel === 'free') return t('shopPage.priceFree');
  if (draft.priceModel === 'enterprise') return t('shopPage.priceEnterprise');
  return `¥${draft.priceYuan}${draft.priceModel === 'subscription' ? t('shopPage.pricePerMonth') : ''}`;
});

const canNext = computed(() => {
  if (step.value === 1) return !!draft.type && !!draft.name.trim() && !!draft.tagline.trim() && !!draft.category;
  if (step.value === 2) return !!draft.description.trim() && !!draft.readme.trim();
  if (step.value === 3 && draft.type === 'org_bundle') return orgSummary.agents > 0;
  return true;
});

/** slug 自动从名称推导（用户已填写时不覆盖） */
function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48);
}

function nextStep() {
  if (step.value === 1 && !draft.slug.trim()) draft.slug = slugify(draft.name);
  step.value += 1;
}

function prevStep() {
  step.value = Math.max(1, step.value - 1);
}

async function submit() {
  submitting.value = true;
  try {
    // 骨架期：模拟提交审核
    await new Promise((r) => setTimeout(r, 600));
    $q.notify({ type: 'positive', message: t('shopPage.publish.submitted') });
    void router.push('/shop/studio');
  } finally {
    submitting.value = false;
  }
}

onMounted(async () => {
  try {
    await Promise.all([store.loadCategories(), store.loadOrgPickTree()]);
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('shopPage.notifyLoadFailed') });
  }
});
</script>

<style scoped>
.shop-publish-page__wizard {
  border-radius: 14px;
}
.shop-publish-page__readme-preview {
  min-height: 220px;
  max-height: 400px;
  overflow-y: auto;
  border-color: var(--glass-border);
}
.shop-publish-page__summary {
  border-color: var(--glass-border);
  border-radius: 12px;
}
.shop-publish-page__summary-table td:first-child {
  color: var(--color-text-secondary);
  width: 40%;
}
</style>
