// web/src/features/graph/__tests__/useGraphEditorAssets.import.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useGraphEditorAssets } from '../useGraphEditorAssets';
import type { GraphDefinition } from '../types';

// Mock Quasar
const mockDialog = vi.fn();
const mockNotify = vi.fn();
vi.mock('quasar', () => ({
  useQuasar: () => ({
    dialog: mockDialog,
    notify: mockNotify,
  }),
}));

// Mock vue-i18n (t returns the key)
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}));

// Mock router
const mockReplace = vi.fn();
vi.mock('vue-router', () => ({
  useRouter: () => ({
    replace: mockReplace,
  }),
}));

// Mock graph store
const mockImportGraphDefinition = vi.fn();
vi.mock('../../../stores/graph', () => ({
  useGraphStore: () => ({
    importGraphDefinition: mockImportGraphDefinition,
  }),
}));

describe('useGraphEditorAssets - R2-3 Import Dialog', () => {
  let graphDef: GraphDefinition;
  let isNew: () => boolean;

  beforeEach(() => {
    vi.clearAllMocks();
    graphDef = {
      id: 'current-graph',
      name: 'Current Graph',
      version: 1,
      nodes: [],
      edges: [],
      conditionalEdges: [],
      entryPoint: '',
      finishPoint: '',
      stateFields: [],
      description: 'Current description',
    };
    isNew = () => false;
  });

  it('shows dialog asking user to choose import destination', async () => {
    const { onImportFile } = useGraphEditorAssets(graphDef, isNew);

    // Mock file input event
    const mockFile = new File(['{"nodes":[]}'], 'test.json', { type: 'application/json' });
    const mockEvent = {
      target: {
        files: [mockFile],
        value: '',
      },
    } as unknown as Event;

    // Mock dialog to return immediately
    mockDialog.mockReturnValue({
      onOk: (cb: (choice: string) => void) => {
        cb('current'); // Simulate user choosing "import to current"
        return { onCancel: () => {} };
      },
    });

    mockImportGraphDefinition.mockResolvedValue({
      id: 'imported-graph',
      name: 'Imported Graph',
      nodes: [{ id: 'node1', type: 'function' }],
    });

    await onImportFile(mockEvent);

    // Should show dialog with two options
    expect(mockDialog).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'graphs.assetImportTitle',
        message: 'graphs.assetImportMessage',
        options: expect.objectContaining({
          type: 'radio',
          model: 'current',
          items: [
            { value: 'current', label: 'graphs.assetImportToCurrent' },
            { value: 'new', label: 'graphs.assetImportCreateNew' },
          ],
        }),
        cancel: true,
        persistent: true,
      })
    );
  });

  it('imports to current canvas when user selects "current"', async () => {
    const { onImportFile } = useGraphEditorAssets(graphDef, isNew);

    const mockFile = new File(['{"nodes":[{"id":"node1","type":"function"}]}'], 'test.json', {
      type: 'application/json',
    });
    const mockEvent = {
      target: { files: [mockFile], value: '' },
    } as unknown as Event;

    // Mock user choosing "current"
    mockDialog.mockReturnValue({
      onOk: (cb: (choice: string) => void) => {
        cb('current');
        return { onCancel: () => {} };
      },
    });

    const importedData = {
      id: 'imported-graph',
      name: 'Imported Graph',
      nodes: [{ id: 'node1', type: 'function' }],
    };
    mockImportGraphDefinition.mockResolvedValue(importedData);

    await onImportFile(mockEvent);

    // Should import and merge into current graphDef
    expect(mockImportGraphDefinition).toHaveBeenCalled();
    expect(graphDef.nodes).toHaveLength(1);
    expect(graphDef.nodes[0].id).toBe('node1');

    // Should NOT navigate away
    expect(mockReplace).not.toHaveBeenCalled();

    // Should show success message
    expect(mockNotify).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'positive', message: 'graphs.assetImportedToCurrent' })
    );
  });

  it('creates new graph when user selects "new"', async () => {
    const { onImportFile } = useGraphEditorAssets(graphDef, isNew);

    const mockFile = new File(['{"nodes":[{"id":"node1","type":"function"}]}'], 'test.json', {
      type: 'application/json',
    });
    const mockEvent = {
      target: { files: [mockFile], value: '' },
    } as unknown as Event;

    // Mock user choosing "new"
    mockDialog.mockReturnValue({
      onOk: (cb: (choice: string) => void) => {
        cb('new');
        return { onCancel: () => {} };
      },
    });

    const importedData = {
      id: 'imported-graph',
      name: 'Imported Graph',
      nodes: [{ id: 'node1', type: 'function' }],
    };
    mockImportGraphDefinition.mockResolvedValue(importedData);

    await onImportFile(mockEvent);

    // Should import and navigate to new graph
    expect(mockImportGraphDefinition).toHaveBeenCalled();
    expect(mockReplace).toHaveBeenCalledWith({
      name: 'graph-editor',
      params: { id: 'imported-graph' },
    });

    // Should show success message
    expect(mockNotify).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'positive', message: 'graphs.assetImportSuccess' })
    );
  });

  it('does nothing when user cancels dialog', async () => {
    const { onImportFile } = useGraphEditorAssets(graphDef, isNew);

    const mockFile = new File(['{"nodes":[]}'], 'test.json', { type: 'application/json' });
    const mockEvent = {
      target: { files: [mockFile], value: '' },
    } as unknown as Event;

    // Mock user canceling
    mockDialog.mockReturnValue({
      onOk: () => ({ onCancel: (cb: () => void) => cb() }),
    });

    await onImportFile(mockEvent);

    // Should not import or navigate
    expect(mockImportGraphDefinition).not.toHaveBeenCalled();
    expect(mockReplace).not.toHaveBeenCalled();
  });
});
