import { i18n } from '../../i18n';
import { defineStore } from 'pinia';
import { ref } from 'vue';
import { mvpApi } from '../../features/mvp/api';
import type { ContentItem, Opportunity, PublishJob, ReviewTask, Supplier } from '../../features/mvp/api';
import { createMvpRequestState } from '../../features/mvp/requestState';

export const useContentInboxStore = defineStore('mvp-content-inbox', () => {
  const rows = ref<ContentItem[]>([]);
  const feedback = ref('');
  const state = createMvpRequestState();
  async function load(sourceType = '', status = '') {
    const query = new URLSearchParams();
    if (sourceType) query.set('source_type', sourceType);
    if (status) query.set('status', status);
    await state.load(
      () => mvpApi.listContent(query.size ? `?${query}` : ''),
      (result) => {
        rows.value = result.items ?? [];
      },
    );
  }
  async function create(input: Partial<ContentItem>) {
    const result = await state.change(() => mvpApi.createContent(input));
    if (result) feedback.value = i18n.global.t('uiSystem.operations.contentAdded');
    return result !== null;
  }
  async function ingestRSS(url: string) {
    const result = await state.change(() => mvpApi.ingestRSS(url));
    if (result)
      feedback.value =
        i18n.global.t('uiSystem.operations.ingestResult', result) +
        (result.errors?.length
          ? i18n.global.t('uiSystem.operations.partialFailure', { errors: result.errors.join('; ') })
          : '');
    return result !== null;
  }
  async function ingestSearch(query: string, url: string, snippet: string) {
    const result = await state.change(() =>
      mvpApi.ingestSearch(query, [{ URL: url, Title: query, Snippet: snippet || query }]),
    );
    if (result)
      feedback.value =
        i18n.global.t('uiSystem.operations.ingestResult', result) +
        (result.errors?.length
          ? i18n.global.t('uiSystem.operations.partialFailure', { errors: result.errors.join('; ') })
          : '');
    return result !== null;
  }
  return {
    rows,
    feedback,
    loading: state.loading,
    saving: state.saving,
    error: state.error,
    load,
    create,
    ingestRSS,
    ingestSearch,
  };
});

export const useReviewQueueStore = defineStore('mvp-review-queue', () => {
  const rows = ref<ReviewTask[]>([]);
  const lastJob = ref<PublishJob | null>(null);
  const state = createMvpRequestState();
  async function load() {
    await state.load(mvpApi.listReview, (result) => {
      rows.value = result.items ?? [];
    });
  }
  async function approve(id: string) {
    const result = await state.change(() => mvpApi.approve(id));
    if (!result) return false;
    lastJob.value = result.publish_job;
    rows.value = rows.value.filter((item) => item.ID !== id);
    await load();
    return true;
  }
  async function reject(id: string, note: string) {
    if (!note.trim()) return false;
    const result = await state.change(() => mvpApi.reject(id, note.trim()));
    if (!result) return false;
    rows.value = rows.value.filter((item) => item.ID !== id);
    await load();
    return true;
  }
  return { rows, lastJob, loading: state.loading, saving: state.saving, error: state.error, load, approve, reject };
});

export const useOpportunityBoardStore = defineStore('mvp-opportunities', () => {
  const rows = ref<Opportunity[]>([]);
  const state = createMvpRequestState();
  async function load(audience = '') {
    await state.load(
      () => mvpApi.listOpp(audience),
      (result) => {
        rows.value = result.items ?? [];
      },
    );
  }
  async function create(title: string, audience: string) {
    return (
      (await state.change(() =>
        mvpApi.createOpp({ Title: title, Audience: audience || 'both', Summary: '', Score: 0.5, Status: 'open' }),
      )) !== null
    );
  }
  async function acknowledge(id: string) {
    return (await state.change(() => mvpApi.ackOpp(id))) !== null;
  }
  return { rows, loading: state.loading, saving: state.saving, error: state.error, load, create, acknowledge };
});

export const useSourcingBoardStore = defineStore('mvp-sourcing', () => {
  const rows = ref<Supplier[]>([]);
  const state = createMvpRequestState();
  async function load(opportunityId = '') {
    await state.load(
      () => mvpApi.listSourcing(opportunityId),
      (result) => {
        rows.value = result.items ?? [];
      },
    );
  }
  async function create(input: Partial<Supplier>) {
    return (await state.change(() => mvpApi.createSupplier(input))) !== null;
  }
  return { rows, loading: state.loading, saving: state.saving, error: state.error, load, create };
});
