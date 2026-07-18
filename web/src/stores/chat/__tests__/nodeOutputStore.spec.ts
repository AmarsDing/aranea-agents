import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useNodeOutputStore } from '../nodeOutputStore';

describe('nodeOutputStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('starts empty', () => {
    const store = useNodeOutputStore();
    expect(store.getNodeOutput('node1')).toEqual([]);
  });

  it('setNodeOutput stores artifacts', () => {
    const store = useNodeOutputStore();
    const artifacts = [{ artifact_id: 'a1', url: 'https://example.com/img.png', mime_type: 'image/png' }];
    store.setNodeOutput('node1', artifacts);
    expect(store.getNodeOutput('node1')).toEqual(artifacts);
  });

  it('appendNodeOutput adds to existing artifacts', () => {
    const store = useNodeOutputStore();
    store.setNodeOutput('node1', [{ artifact_id: 'a1', url: 'url1', mime_type: 'image/png' }]);
    store.appendNodeOutput('node1', { artifact_id: 'a2', url: 'url2', mime_type: 'video/mp4' });
    expect(store.getNodeOutput('node1')).toHaveLength(2);
  });

  it('clearSession removes all outputs', () => {
    const store = useNodeOutputStore();
    store.setNodeOutput('node1', [{ artifact_id: 'a1', url: 'u', mime_type: 'image/png' }]);
    store.clearSession();
    expect(store.getNodeOutput('node1')).toEqual([]);
  });
});
