// web/src/features/graph/__tests__/useGraphEditorAssets.import.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useGraphEditorAssets } from '../useGraphEditorAssets';
import type { GraphDefinition } from '../types';
import { graphDefinitionFixture } from './graphFixtures';

// Mock Quasar
const mockNotify = vi.fn();
vi.mock('quasar', () => ({
  useQuasar: () => ({
    notify: mockNotify,
  }),
}));

// 导入落点选择已从 $q.dialog(options: radio) 迁到 useAppPrompt
const mockPrompt = vi.fn();
vi.mock('../../../composables/useAppPrompt', () => ({
  prompt: (options: unknown) => mockPrompt(options),
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
    graphDef = graphDefinitionFixture({
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
    });
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

    // Mock user choosing "import to current"
    mockPrompt.mockResolvedValue('current');

    mockImportGraphDefinition.mockResolvedValue({
      id: 'imported-graph',
      name: 'Imported Graph',
      nodes: [{ id: 'node1', type: 'function' }],
    });

    await onImportFile(mockEvent);

    // Should show dialog with two options
    expect(mockPrompt).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'graphs.assetImportTitle',
        message: 'graphs.assetImportMessage',
        type: 'radio',
        defaultValue: 'current',
        items: [
          { value: 'current', label: 'graphs.assetImportToCurrent' },
          { value: 'new', label: 'graphs.assetImportCreateNew' },
        ],
      }),
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
    mockPrompt.mockResolvedValue('current');

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
      expect.objectContaining({ type: 'positive', message: 'graphs.assetImportedToCurrent' }),
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
    mockPrompt.mockResolvedValue('new');

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
      expect.objectContaining({ type: 'positive', message: 'graphs.assetImportSuccess' }),
    );
  });

  it('does nothing when user cancels dialog', async () => {
    const { onImportFile } = useGraphEditorAssets(graphDef, isNew);

    const mockFile = new File(['{"nodes":[]}'], 'test.json', { type: 'application/json' });
    const mockEvent = {
      target: { files: [mockFile], value: '' },
    } as unknown as Event;

    // Mock user canceling
    mockPrompt.mockResolvedValue(null);

    await onImportFile(mockEvent);

    // Should not import or navigate
    expect(mockImportGraphDefinition).not.toHaveBeenCalled();
    expect(mockReplace).not.toHaveBeenCalled();
  });
});
