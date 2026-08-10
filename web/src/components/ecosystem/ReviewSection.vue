<template>
  <div class="review-section">
    <!-- 评分分布 -->
    <div class="row q-col-gutter-md items-center q-mb-lg">
      <div class="col-auto text-center">
        <div class="review-section__score">{{ asset.rating.toFixed(1) }}</div>
        <rating-stars :rating="asset.rating" size="16px" />
        <div class="text-caption text-grey-7 q-mt-xs">
          {{ t('shopPage.ratingCount', { count: asset.ratingCount }) }}
        </div>
      </div>
      <div class="col">
        <div
          v-for="(count, i) in asset.ratingDist"
          :key="i"
          class="row items-center q-gutter-sm review-section__dist-row"
        >
          <span class="text-caption review-section__dist-label">{{ 5 - i }}★</span>
          <q-linear-progress
            :value="distRatio(count)"
            rounded
            size="6px"
            color="amber"
            track-color="grey-3"
            class="col"
          />
          <span class="text-caption text-grey-7 review-section__dist-count">{{ count }}</span>
        </div>
      </div>
    </div>

    <!-- 我要评分 -->
    <q-card flat class="app-glass-panel q-pa-md q-mb-lg">
      <div class="text-weight-medium q-mb-sm">{{ t('shopPage.writeReview') }}</div>
      <div class="row items-center q-gutter-sm q-mb-sm">
        <q-rating v-model="draftRating" size="22px" color="amber" icon="star_border" icon-selected="star" />
        <span v-if="draftRating > 0" class="text-caption text-grey-7">{{ draftRating }}.0</span>
      </div>
      <q-input
        v-model="draftContent"
        outlined
        dense
        autogrow
        type="textarea"
        :placeholder="t('shopPage.reviewPlaceholder')"
        class="q-mb-sm"
      />
      <div class="row justify-end">
        <q-btn
          color="primary"
          unelevated
          rounded
          no-caps
          dense
          :label="t('shopPage.submitReview')"
          :disable="draftRating === 0 || !draftContent.trim()"
          :loading="submitting"
          @click="submit"
        />
      </div>
    </q-card>

    <!-- 评论列表 -->
    <div v-for="review in asset.reviews" :key="review.id" class="review-section__item">
      <div class="row items-center q-gutter-sm q-mb-xs">
        <q-avatar size="28px" color="primary" text-color="white" class="review-section__avatar">
          {{ review.author.slice(0, 1) }}
        </q-avatar>
        <span class="text-weight-medium">{{ review.author }}</span>
        <rating-stars :rating="review.rating" size="13px" />
        <q-space />
        <span class="text-caption text-grey-7">{{ review.createdAt }}</span>
      </div>
      <div class="review-section__content">{{ review.content }}</div>
      <div class="row items-center q-gutter-sm q-mt-xs">
        <q-btn flat dense no-caps size="sm" icon="thumb_up_off_alt" :label="String(review.likes)" class="text-grey-7" />
      </div>
      <div v-if="review.reply" class="review-section__reply">
        <div class="row items-center q-gutter-xs q-mb-xs">
          <q-badge color="primary" :label="t('shopPage.creatorReply')" />
          <span class="text-caption text-weight-medium">{{ review.reply.author }}</span>
          <span class="text-caption text-grey-7">{{ review.reply.createdAt }}</span>
        </div>
        <div class="text-body2">{{ review.reply.content }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MarketAsset } from '../../features/ecosystem/types';
import RatingStars from './RatingStars.vue';

const props = defineProps<{
  asset: MarketAsset;
  submitting?: boolean;
}>();

const emit = defineEmits<{
  submit: [rating: number, content: string];
}>();

const { t } = useI18n();
const draftRating = ref(0);
const draftContent = ref('');

function distRatio(count: number): number {
  const max = Math.max(...props.asset.ratingDist, 1);
  return count / max;
}

function submit() {
  emit('submit', draftRating.value, draftContent.value.trim());
  draftRating.value = 0;
  draftContent.value = '';
}
</script>

<style scoped>
.review-section__score {
  font-size: 40px;
  font-weight: 800;
  line-height: 1;
  color: var(--color-text-primary);
}

.review-section__dist-row {
  margin-bottom: 3px;
}

.review-section__dist-label {
  width: 24px;
  text-align: right;
  color: var(--color-text-secondary);
}

.review-section__dist-count {
  width: 32px;
}

.review-section__item {
  padding: 14px 0;
  border-top: 1px solid var(--glass-border);
}

.review-section__avatar {
  font-size: 12px;
  font-weight: 700;
}

.review-section__content {
  font-size: 14px;
  line-height: 1.65;
  color: var(--color-text-primary);
  padding-left: 40px;
}

.review-section__reply {
  margin: 10px 0 0 40px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--interaction-surface-hover);
}

body.body--dark .review-section__reply {
  background: rgb(255 255 255 / 6%);
}
</style>
