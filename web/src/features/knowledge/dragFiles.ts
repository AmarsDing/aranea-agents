/**
 * U0-3：OS 文件拖拽收集（纯函数，可测）。
 * 拖拽上传与按钮上传共用同一入队入口（onUploadFilesPicked → enqueueUploadFiles）；
 * 本模块只负责从 DataTransfer 提取文件并过滤目录项。
 */

/** 拖拽收集结果：可入库文件 + 被跳过的目录数（用于外层 notify 提示）。 */
export type CollectedDropFiles = {
  files: File[];
  skippedDirs: number;
};

/** webkitGetAsEntry 返回值的最小结构（目录判定只需 isFile/isDirectory）。 */
type WebkitEntryLike = {
  isFile: boolean;
  isDirectory: boolean;
};

type DataTransferItemWithEntry = DataTransferItem & {
  webkitGetAsEntry?: () => WebkitEntryLike | null;
};

/**
 * 拖拽载荷是否来自 OS 文件。
 * 树内移动/Tab 排序拖拽只 setData('text/plain')，types 不含 'Files'，天然分流。
 */
export function isOsFileDrag(dt: DataTransfer | null): boolean {
  if (!dt) return false;
  return Array.from(dt.types ?? []).includes('Files');
}

/**
 * 遍历 DataTransfer.items 过滤目录项（不支持目录上传，计数提示）。
 * - 仅 item.kind === 'file' 进入候选（string 等拖拽文本忽略）；
 * - webkitGetAsEntry 可用时按 entry 判定目录并计数跳过；
 * - entry 为 null（个别浏览器/来源）或 API 缺失时，退化为直接收文件；
 * - items 为空的环境（少数浏览器 drop 只暴露 files）退化为 files 全收。
 */
export function collectDropFiles(dt: DataTransfer): CollectedDropFiles {
  const files: File[] = [];
  let skippedDirs = 0;
  const items = Array.from(dt.items ?? []);
  if (!items.length) {
    return { files: Array.from(dt.files ?? []), skippedDirs };
  }
  for (const item of items) {
    if (item.kind !== 'file') continue;
    const entry = (item as DataTransferItemWithEntry).webkitGetAsEntry?.() ?? null;
    if (entry && entry.isDirectory) {
      skippedDirs += 1;
      continue;
    }
    const file = item.getAsFile();
    if (file) files.push(file);
  }
  return { files, skippedDirs };
}
