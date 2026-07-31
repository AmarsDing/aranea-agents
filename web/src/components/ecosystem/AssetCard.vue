<template>
  <q-card
    flat
    class="asset-card app-glass-panel full-height column"
    tabindex="0"
    @click="emit('open')"
    @keyup.enter="emit('open')"
  >
    <q-card-section class="col q-pb-sm">
      <div class="row items-start no-wrap q-gutter-sm">
        <asset-type-icon :type="asset.type" size="44px" />
        <div class="col ellipsis">
          <div class="row items-center no-wrap q-gutter-xs">
            <span class="asset-card__name ellipsis">{{ asset.name }}</span>
            <q-icon v-if="asset.creator.verified" name="verified" size="14px" color="primary">
              <q-tooltip>{{ t('shopPage.verifiedCreator') }}</q-tooltip>
            </q-icon>
          </div>
          <div class="asset-card__type text-caption">
            {{ t(`shopPage.type.${typeMeta.labelKey}`) }} · v{{ asset.version }}
          </div>
        </div>
        <price-tag :price-model="asset.priceModel" :price-cents="asset.priceCents" />
      </div>

      <div class="asset-card__tagline q-mt-sm">{{ asset.tagline }}</div>

      <div class="row items-center q-gutter-xs q-mt-sm">
        <q-avatar size="18px" :style="{ background: asset.creator.avatarColor }" class="asset-card__creator-avatar">
          <span class="asset-card__creator-initial">{{ creatorInitial }}</span>
        </q-avatar>
        <span class="asset-card__creator text-caption ellipsis" @click.stop="emit('openCreator')">
          {{ asset.creator.name }}
        </span>
      </div>
    </q-card-section>

    <q-separator inset />

    <q-card-section class="q-py-sm row items-center justify-between no-wrap">
      <rating-stars :rating="asset.rating" :count="asset.ratingCount" show-value />
      <span class="asset-card__installs text-caption">
        <q-icon name="download" size="13px" class="q-mr-xs" />{{ formatInstalls(asset.installCount) }}
      </span>
    </q-card-section>

    <q-card-actions class="q-pt-none q-px-md q-pb-md" @click.stop>
      <q-btn
        v-if="!asset.installed"
        color="primary"
        unelevated
        rounded
        no-caps
        dense
        class="full-width"
        :label="installLabel"
        :loading="installing"
        @click="emit('install')"
      />
      <q-btn v-else outline rounded no-caps dense class="full-width asset-card__installed-btn" disable>
        <q-icon name="check_circle" size="14px" class="q-mr-xs" />{{ t('shopPage.installed') }}
      </q-btn>
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { ASSET_TYPE_META, formatInstalls } from '../../features/ecosystem/marketUi';
import type { MarketAsset } from '../../features/ecosystem/types';
import AssetTypeIcon from './AssetTypeIcon.vue';
import PriceTag from './PriceTag.vue';
import RatingStars from './RatingStars.vue';

const props = defineProps<{
  asset: MarketAsset;
  installing?: boolean;
}>();

const emit = defineEmits<{
  open: [];
  openCreator: [];
  install: [];
}>();

const { t } = useI18n();

const typeMeta = computed(() => ASSET_TYPE_META[props.asset.type]);
const creatorInitial = computed(() => props.asset.creator.name.slice(0, 1));
const installLabel = computed(() =>
  props.asset.priceModel === 'free' ? t('shopPage.install') : t('shopPage.buyAndInstall'),
);
</script>

<style scoped>
.asset-card {
  cursor: pointer;
  transition:
    transform 0.18s ease,
    box-shadow 0.18s ease,
    background 0.18s ease;
  border-radius: 16px;
}
.asset-card:hover,
.asset-card:focus-visible {
  transform: translateY(-3px);
  background: var(--glass-surface-hover);
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.1);
}
body.body--dark .asset-card:hover {
  box-shadow: 0 8px 28px rgba(0, 0, 0, 0.4);
}
.asset-card__name {
  font-weight: 700;
  font-size: 15px;
  color: var(--color-text-primary);
}
.asset-card__type {
  color: var(--color-text-secondary);
  margin-top: 1px;
}
.asset-card__tagline {
  font-size: 13px;
  line-height: 1.55;
  color: var(--color-text-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  min-height: 2.6em;
}
.asset-card__creator-avatar {
  flex: none;
}
.asset-card__creator-initial {
  color: var(--color-on-accent);
  font-size: 10px;
  font-weight: 700;
}
.asset-card__creator {
  color: var(--color-text-secondary);
  cursor: pointer;
}
.asset-card__creator:hover {
  color: var(--color-accent);
  text-decoration: underline;
}
.asset-card__installs {
  color: var(--color-text-secondary);
  display: inline-flex;
  align-items: center;
}
.asset-card__installed-btn {
  color: var(--color-success);
  border-color: var(--color-success);
}
</style>
