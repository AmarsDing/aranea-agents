import { describe, expect, it } from 'vitest';

import {
  PROVIDER_MODEL_TABLE_COLUMNS,
  isLocalChannelModel,
} from '../../../components/platform/providerModelUi';
import type { PlatformResource } from '../../platform/types';

function rowWith(channelClass?: string): PlatformResource {
  return { resource: 'llm-provider-models', channel_class: channelClass } as PlatformResource;
}

describe('isLocalChannelModel', () => {
  it('treats explicit local (case/space-insensitive) as local', () => {
    expect(isLocalChannelModel(rowWith('local'))).toBe(true);
    expect(isLocalChannelModel(rowWith(' LOCAL '))).toBe(true);
    expect(isLocalChannelModel(rowWith('Local'))).toBe(true);
  });

  it('treats empty/missing/cloud as cloud (conservative default)', () => {
    expect(isLocalChannelModel(rowWith(''))).toBe(false);
    expect(isLocalChannelModel(rowWith(undefined))).toBe(false);
    expect(isLocalChannelModel(rowWith('cloud'))).toBe(false);
    expect(isLocalChannelModel(rowWith('CLOUD'))).toBe(false);
  });

  it('rejects near-miss spellings (backend validates; UI must not guess)', () => {
    expect(isLocalChannelModel(rowWith('loca'))).toBe(false);
    expect(isLocalChannelModel(rowWith('onprem'))).toBe(false);
  });
});

describe('PROVIDER_MODEL_TABLE_COLUMNS', () => {
  it('includes the D6 L-1 channel marker column', () => {
    const names = PROVIDER_MODEL_TABLE_COLUMNS.map((c) => c.name);
    expect(names).toContain('channel');
  });
});
