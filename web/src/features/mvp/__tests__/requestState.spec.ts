import { describe, expect, it } from 'vitest';
import { createMvpRequestState } from '../requestState';

describe('MVP operation request coordination', () => {
  it('keeps the current filter result when an older request completes late', async () => {
    const state = createMvpRequestState();
    let releaseOld!: (value: string) => void;
    let displayed = '';
    const oldRequest = state.load(
      () =>
        new Promise<string>((resolve) => {
          releaseOld = resolve;
        }),
      (value) => {
        displayed = value;
      },
    );
    await state.load(
      async () => 'new filter',
      (value) => {
        displayed = value;
      },
    );
    releaseOld('old filter');
    await oldRequest;
    expect(displayed).toBe('new filter');
    expect(state.loading.value).toBe(false);
  });
  it('does not send a duplicate mutation while the first submission is pending', async () => {
    const state = createMvpRequestState();
    let release!: (value: string) => void;
    const first = state.change(
      () =>
        new Promise<string>((resolve) => {
          release = resolve;
        }),
    );
    let duplicates = 0;
    expect(
      await state.change(async () => {
        duplicates++;
        return 'duplicate';
      }),
    ).toBeNull();
    release('saved');
    expect(await first).toBe('saved');
    expect(duplicates).toBe(0);
  });
  it('keeps a visible failure and allows retry after a rejected submission', async () => {
    const state = createMvpRequestState();
    expect(
      await state.change(async () => {
        throw new Error('网络暂时不可用');
      }),
    ).toBeNull();
    expect(state.error.value).toBe('网络暂时不可用');
    expect(state.saving.value).toBe(false);
    expect(await state.change(async () => 'retried')).toBe('retried');
    expect(state.error.value).toBe('');
  });
});
