import { describe, expect, it } from 'vitest';
import { collectDropFiles, isOsFileDrag } from '../dragFiles';

/** 构造 mock DataTransferItem（happy-dom 无完整 DnD 实现）。 */
function fileItem(file: File | null, entry?: { isFile: boolean; isDirectory: boolean } | null): DataTransferItem {
  return {
    kind: 'file',
    type: file?.type ?? '',
    getAsFile: () => file,
    // entry === undefined 模拟浏览器无 webkitGetAsEntry；null 模拟 API 存在但取不到 entry
    webkitGetAsEntry: entry === undefined ? undefined : () => entry,
  } as unknown as DataTransferItem;
}

function stringItem(): DataTransferItem {
  return { kind: 'string', type: 'text/plain', getAsFile: () => null } as unknown as DataTransferItem;
}

function makeDataTransfer(opts: { items?: DataTransferItem[]; files?: File[]; types?: string[] }): DataTransfer {
  return {
    items: opts.items ?? [],
    files: opts.files ?? [],
    types: opts.types ?? ['Files'],
  } as unknown as DataTransfer;
}

describe('dragFiles', () => {
  describe('isOsFileDrag', () => {
    it('returns false for null dataTransfer', () => {
      expect(isOsFileDrag(null)).toBe(false);
    });

    it('detects OS file drags by Files type', () => {
      expect(isOsFileDrag(makeDataTransfer({ types: ['Files'] }))).toBe(true);
    });

    it('rejects internal drags carrying only text types (tree move / tab sort)', () => {
      expect(isOsFileDrag(makeDataTransfer({ types: ['text/plain'] }))).toBe(false);
    });
  });

  describe('collectDropFiles', () => {
    it('collects plain files via items', () => {
      const a = new File(['a'], 'a.md', { type: 'text/markdown' });
      const b = new File(['b'], 'b.txt', { type: 'text/plain' });
      const { files, skippedDirs } = collectDropFiles(makeDataTransfer({ items: [fileItem(a), fileItem(b)] }));
      expect(files.map((f) => f.name)).toEqual(['a.md', 'b.txt']);
      expect(skippedDirs).toBe(0);
    });

    it('skips directory entries via webkitGetAsEntry and counts them', () => {
      const f = new File(['a'], 'a.md', { type: 'text/markdown' });
      const dirEntry = { isFile: false, isDirectory: true };
      const dt = makeDataTransfer({
        items: [fileItem(f), fileItem(null, dirEntry), fileItem(null, dirEntry)],
      });
      const { files, skippedDirs } = collectDropFiles(dt);
      expect(files.map((x) => x.name)).toEqual(['a.md']);
      expect(skippedDirs).toBe(2);
    });

    it('accepts the file when the entry is null (API present but no entry)', () => {
      const f = new File(['a'], 'a.md', { type: 'text/markdown' });
      const { files, skippedDirs } = collectDropFiles(makeDataTransfer({ items: [fileItem(f, null)] }));
      expect(files).toHaveLength(1);
      expect(skippedDirs).toBe(0);
    });

    it('ignores non-file items (dragged text snippets)', () => {
      const { files, skippedDirs } = collectDropFiles(
        makeDataTransfer({ items: [stringItem()], types: ['text/plain'] }),
      );
      expect(files).toHaveLength(0);
      expect(skippedDirs).toBe(0);
    });

    it('tolerates items whose getAsFile returns null', () => {
      const { files, skippedDirs } = collectDropFiles(makeDataTransfer({ items: [fileItem(null, null)] }));
      expect(files).toEqual([]);
      expect(skippedDirs).toBe(0);
    });

    it('falls back to dt.files when items are empty', () => {
      const f = new File(['a'], 'a.md', { type: 'text/markdown' });
      const { files, skippedDirs } = collectDropFiles(makeDataTransfer({ items: [], files: [f] }));
      expect(files.map((x) => x.name)).toEqual(['a.md']);
      expect(skippedDirs).toBe(0);
    });
  });
});
