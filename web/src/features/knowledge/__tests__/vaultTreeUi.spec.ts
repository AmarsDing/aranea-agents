import { describe, expect, it } from 'vitest';
import {
  dirNodeKey,
  isValidDropTarget,
  parseVaultTreeKey,
  vaultNodeKey,
  vaultNodeVisual,
  vaultRootVisual,
  type DragFileRef,
} from '../vaultTreeUi';

describe('vaultTreeUi key 编解码', () => {
  it('库节点 key 往返', () => {
    const key = vaultNodeKey('col-1');
    expect(key).toBe('v|col-1');
    expect(parseVaultTreeKey(key)).toEqual({ collectionId: 'col-1', prefix: '' });
  });

  it('目录节点 key 往返（prefix 含多级斜杠）', () => {
    const key = dirNodeKey('col-1', 'guides/setup/');
    expect(key).toBe('d|col-1|guides/setup/');
    expect(parseVaultTreeKey(key)).toEqual({ collectionId: 'col-1', prefix: 'guides/setup/' });
  });

  it('非法 key 返回 null', () => {
    expect(parseVaultTreeKey('')).toBeNull();
    expect(parseVaultTreeKey('v|')).toBeNull();
    expect(parseVaultTreeKey('d|colonly')).toBeNull();
    expect(parseVaultTreeKey('d||x/')).toBeNull();
    expect(parseVaultTreeKey('random')).toBeNull();
  });
});

describe('vaultTreeUi 节点视觉', () => {
  it('库节点固定 cyan inventory_2', () => {
    expect(vaultRootVisual()).toEqual({ icon: 'inventory_2', cls: 'kv-icon--cyan', pulse: false });
  });

  it('目录 violet folder', () => {
    expect(vaultNodeVisual({ kind: 'dir' })).toEqual({ icon: 'folder', cls: 'kv-icon--violet', pulse: false });
  });

  it('md=teal / 图片=magenta / 音频=orange / 视频=orange', () => {
    expect(vaultNodeVisual({ kind: 'file', name: 'a.md' }).cls).toBe('kv-icon--teal');
    expect(vaultNodeVisual({ kind: 'file', name: 'p.PNG' }).cls).toBe('kv-icon--magenta');
    expect(vaultNodeVisual({ kind: 'file', name: 's.mp3' })).toMatchObject({ icon: 'graphic_eq', cls: 'kv-icon--orange' });
    expect(vaultNodeVisual({ kind: 'file', name: 'v.mp4' })).toMatchObject({ icon: 'movie', cls: 'kv-icon--orange' });
  });

  it('未知扩展回落 muted 通用文件图标', () => {
    expect(vaultNodeVisual({ kind: 'file', name: 'data.xyz' })).toEqual({
      icon: 'insert_drive_file',
      cls: 'kv-icon--muted',
      pulse: false,
    });
    expect(vaultNodeVisual({ kind: 'file', name: 'noext' }).cls).toBe('kv-icon--muted');
  });

  it('error 状态红色脉冲且保留类型图标', () => {
    const v = vaultNodeVisual({ kind: 'file', name: 'a.md', status: 'error' });
    expect(v).toEqual({ icon: 'article', cls: 'kv-icon--red', pulse: true });
  });
});

describe('G3-F1 isValidDropTarget', () => {
  const drag: DragFileRef = { docId: 'd1', name: 'a.md', fromPrefix: 'notes/', vaultId: 'c1' };

  it('无拖拽中 = 非法', () => {
    expect(isValidDropTarget(null, { vaultId: 'c1', prefix: '' })).toBe(false);
  });

  it('同 vault 不同目录 = 合法（含库根）', () => {
    expect(isValidDropTarget(drag, { vaultId: 'c1', prefix: '' })).toBe(true);
    expect(isValidDropTarget(drag, { vaultId: 'c1', prefix: 'archive/' })).toBe(true);
    expect(isValidDropTarget(drag, { vaultId: 'c1', prefix: 'notes/sub/' })).toBe(true);
  });

  it('原地（文件当前目录）= 非法（noop）', () => {
    expect(isValidDropTarget(drag, { vaultId: 'c1', prefix: 'notes/' })).toBe(false);
  });

  it('根目录文件拖入库根 = 非法', () => {
    const rootDrag: DragFileRef = { ...drag, fromPrefix: '' };
    expect(isValidDropTarget(rootDrag, { vaultId: 'c1', prefix: '' })).toBe(false);
    expect(isValidDropTarget(rootDrag, { vaultId: 'c1', prefix: 'notes/' })).toBe(true);
  });

  it('跨库 = 非法（V12.5 跨库拖拽本期禁止）', () => {
    expect(isValidDropTarget(drag, { vaultId: 'c2', prefix: '' })).toBe(false);
    expect(isValidDropTarget(drag, { vaultId: 'c2', prefix: 'notes/' })).toBe(false);
  });
});
