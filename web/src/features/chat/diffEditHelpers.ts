import type { DiffEditHunk, DiffEditArguments, PatchFileArguments } from "./types";

const FILE_EDIT_TOOLS = new Set(["diff_edit", "edit_file", "patch_file", "save_file"]);

export function isFileEditTool(toolName: string): boolean {
  return FILE_EDIT_TOOLS.has(toolName);
}

export function extractDiffHunks(
  toolName: string,
  args?: Record<string, unknown>,
): DiffEditHunk[] {
  if (!args) return [];

  if (toolName === "diff_edit" || toolName === "edit_file") {
    const typed = args as unknown as DiffEditArguments;
    if (!Array.isArray(typed.edits)) return [];
    return typed.edits.map((e) => ({
      search: String(e.search ?? ""),
      replace: String(e.replace ?? ""),
      replace_all: e.replace_all,
    }));
  }

  if (toolName === "patch_file") {
    const typed = args as unknown as PatchFileArguments;
    if (typed.patch) {
      return parseUnifiedDiff(typed.patch);
    }
    if (Array.isArray(typed.hunks)) {
      return typed.hunks.map((h) => ({
        search: String(h.old_lines ?? h.search ?? ""),
        replace: String(h.new_lines ?? h.replace ?? ""),
      }));
    }
    return [];
  }

  return [];
}

export function extractFileName(args?: Record<string, unknown>): string {
  if (!args) return "";
  return String(args.file_name ?? args.path ?? "");
}

function parseUnifiedDiff(patch: string): DiffEditHunk[] {
  const hunks: DiffEditHunk[] = [];
  const lines = patch.split("\n");
  let searchLines: string[] = [];
  let replaceLines: string[] = [];
  let inHunk = false;

  for (const line of lines) {
    if (line.startsWith("@@")) {
      if (inHunk && (searchLines.length > 0 || replaceLines.length > 0)) {
        hunks.push({ search: searchLines.join("\n"), replace: replaceLines.join("\n") });
      }
      searchLines = [];
      replaceLines = [];
      inHunk = true;
      continue;
    }
    if (!inHunk) continue;

    if (line.startsWith("-")) {
      searchLines.push(line.slice(1));
    } else if (line.startsWith("+")) {
      replaceLines.push(line.slice(1));
    } else if (line.startsWith(" ")) {
      searchLines.push(line.slice(1));
      replaceLines.push(line.slice(1));
    }
  }

  if (inHunk && (searchLines.length > 0 || replaceLines.length > 0)) {
    hunks.push({ search: searchLines.join("\n"), replace: replaceLines.join("\n") });
  }

  return hunks;
}
