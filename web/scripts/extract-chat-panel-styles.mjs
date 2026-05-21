import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const vuePath = path.join(root, "src/components/chat/ChatMessagePanel.vue");
const outPath = path.join(root, "src/css/theme/_chat-message-panel.sass");

const src = fs.readFileSync(vuePath, "utf8");
const start = src.indexOf("<style scoped lang=\"sass\">");
const end = src.indexOf("</style>", start);
if (start < 0 || end < 0) {
  throw new Error("style block not found");
}

let sass = src.slice(start + "<style scoped lang=\"sass\">".length, end).trim();

const rgbaMap = [
  ["rgba(15, 23, 42, 0.04)", "var(--chat-shadow-4)"],
  ["rgba(15, 23, 42, 0.06)", "var(--chat-shadow-6)"],
  ["rgba(15, 23, 42, 0.08)", "var(--chat-shadow-8)"],
  ["rgba(15, 23, 42, 0.07)", "var(--chat-shadow-8)"],
  ["rgba(15, 23, 42, 0.35)", "var(--chat-surface-dark-35)"],
  ["rgba(15, 23, 42, 0.45)", "var(--chat-surface-dark-45)"],
  ["rgba(15, 23, 42, 0.62)", "var(--chat-surface-dark-62)"],
  ["rgba(15, 23, 42, 0.78)", "var(--chat-surface-dark-78)"],
  ["rgba(15, 23, 42, 0.92)", "var(--chat-surface-dark-92)"],
  ["rgba(30, 41, 59, 0.6)", "var(--chat-surface-muted-60)"],
  ["rgba(30, 41, 59, 0.76)", "var(--chat-surface-muted-76)"],
  ["rgba(30, 41, 59, 0.82)", "var(--chat-surface-muted-82)"],
  ["rgba(30, 41, 59, 0.94)", "var(--chat-surface-muted-94)"],
  ["rgba(71, 85, 105, 0.78)", "var(--chat-text-muted)"],
  ["rgba(71, 85, 105, 0.7)", "var(--chat-text-muted)"],
  ["rgba(71, 85, 105, 0.92)", "var(--chat-text-muted-strong)"],
  ["rgba(100, 116, 139, 0.28)", "var(--chat-scrollbar-thumb)"],
  ["rgba(100, 116, 139, 0.55)", "var(--chat-scrollbar-thumb-hover)"],
  ["rgba(148, 163, 184, 0.22)", "var(--chat-scrollbar-thumb-dark)"],
  ["rgba(148, 163, 184, 0.45)", "var(--chat-scrollbar-thumb-hover-dark)"],
  ["rgba(148, 163, 184, 0.16)", "var(--chat-border-muted)"],
  ["rgba(148, 163, 184, 0.18)", "var(--chat-border-muted)"],
  ["rgba(148, 163, 184, 0.14)", "var(--chat-border-subtle)"],
  ["rgba(148, 163, 184, 0.12)", "var(--chat-border-subtle)"],
  ["rgba(203, 213, 225, 0.7)", "var(--chat-text-muted)"],
  ["rgba(203, 213, 225, 0.78)", "var(--chat-text-muted)"],
  ["rgba(203, 213, 225, 0.18)", "var(--chat-border-muted)"],
  ["rgba(233, 162, 59, 0.14)", "var(--chat-accent-soft)"],
  ["rgba(233, 162, 59, 0.08)", "var(--chat-accent-soft)"],
  ["rgba(233, 162, 59, 0.55)", "var(--chat-accent-border)"],
  ["rgba(233, 162, 59, 0.4)", "var(--chat-accent-border)"],
  ["rgba(212, 140, 26, 0.85)", "var(--color-accent-hover)"],
  ["rgba(99, 102, 241, 0.18)", "var(--chat-indigo-soft)"],
  ["rgba(99, 102, 241, 0.28)", "var(--chat-indigo-border)"],
  ["rgba(99, 102, 241, 0.04)", "var(--chat-indigo-faint)"],
  ["rgba(99, 102, 241, 0.45)", "var(--chat-indigo-border-strong)"],
  ["rgba(99, 102, 241, 0.5)", "var(--chat-indigo-border-strong)"],
  ["rgba(79, 70, 229, 0.22)", "var(--chat-indigo-shadow)"],
  ["rgba(255, 255, 255, 0.78)", "var(--chat-file-tile-bg)"],
  ["rgba(255, 255, 255, 0.65)", "var(--chat-header-bg-top)"],
  ["rgba(255, 255, 255, 0.32)", "var(--chat-header-bg-bottom)"],
  ["rgba(255, 255, 255, 0.55)", "var(--chat-pending-item-bg)"],
  ["rgba(255, 255, 255, 0.14)", "var(--chat-bubble-highlight)"],
  ["rgba(255, 255, 255, 0.45)", "var(--chat-border-light)"],
  ["rgba(255, 255, 255, 0.92)", "var(--chat-text-on-accent)"],
  ["rgba(255, 255, 255, 0.82)", "var(--chat-text-on-accent-soft)"],
  ["rgba(255, 255, 255, 0.18)", "var(--chat-surface-light-18)"],
  ["rgba(255, 255, 255, 0.1)", "var(--chat-surface-light-10)"],
  ["rgba(255, 255, 255, 0.04)", "var(--chat-surface-light-4)"],
  ["rgba(255, 255, 255, 0.25)", "var(--chat-border-light)"],
  ["rgba(255, 255, 255, 0.28)", "var(--chat-border-light-strong)"],
  ["rgba(255, 255, 255, 0.22)", "var(--chat-border-light)"],
  ["rgba(255, 255, 255, 0.16)", "var(--chat-border-subtle)"],
  ["rgba(255, 255, 255, 0.65)", "var(--chat-border-light-strong)"],
  ["rgba(141, 110, 99, 0.2)", "var(--color-border-soft)"],
  ["rgba(141, 110, 99, 0.12)", "var(--color-border-soft)"],
  ["rgba(248, 250, 252, 0.55)", "var(--chat-canvas-tint-top)"],
  ["rgba(241, 245, 249, 0.0)", "transparent"],
  ["rgba(0, 0, 0, 0.32)", "var(--chat-shadow-dark-a)"],
  ["rgba(0, 0, 0, 0.18)", "var(--chat-shadow-dark-b)"],
  ["rgba(0, 0, 0, 0.5)", "var(--chat-shadow-overlay)"],
  ["rgba(251, 191, 36, 0.08)", "var(--color-warning-soft)"],
  ["rgba(251, 191, 36, 0.22)", "var(--chat-warning-border)"],
  ["rgba(14, 165, 233, 0.12)", "var(--color-info-soft)"],
  ["rgba(14, 165, 233, 0.04)", "var(--chat-info-faint)"],
  ["rgba(14, 165, 233, 0.42)", "var(--chat-info-border)"],
  ["rgba(14, 165, 233, 0.18)", "var(--chat-info-soft)"],
  ["rgba(56, 189, 248, 0.36)", "var(--chat-info-border-dark)"],
  ["rgba(56, 189, 248, 0.1)", "var(--chat-info-soft-dark)"],
  ["rgba(239, 68, 68, 0.12)", "var(--color-danger-soft)"],
  ["rgba(239, 68, 68, 0.04)", "var(--chat-danger-faint)"],
  ["rgba(239, 68, 68, 0.46)", "var(--chat-danger-border)"],
  ["rgba(239, 68, 68, 0.18)", "var(--chat-danger-soft)"],
  ["rgba(248, 113, 113, 0.42)", "var(--chat-danger-border-dark)"],
  ["rgba(34, 197, 94, 0.18)", "var(--color-success-soft)"],
  ["rgba(34, 197, 94, 0.5)", "var(--chat-success-border)"],
  ["rgba(92, 107, 192, 0.35)", "var(--chat-indigo-border)"],
  ["rgba(147, 197, 253, 0.45)", "var(--chat-info-border)"],
  ["rgba(191, 219, 254, 0.85)", "var(--chat-info-border-strong)"],
];

for (const [from, to] of rgbaMap) {
  sass = sass.split(from).join(to);
}

sass = sass
  .replace(/:global\(\.body--dark\)/g, "body.body--dark")
  .replace(/:deep\(([^)]+)\)/g, " $1")
  .replace(/\$canvas-light/g, "var(--chat-canvas-light)")
  .replace(/\$canvas-dark/g, "var(--chat-canvas-dark)")
  .replace(/\$msg-shadow-sm/g, "var(--chat-shadow-sm)")
  .replace(/\$msg-shadow-md/g, "var(--chat-shadow-md)")
  .replace(/\$msg-shadow-dark/g, "var(--chat-shadow-dark)")
  .replace(/\$msg-shadow-sent/g, "var(--chat-shadow-sent)")
  .replace(/\$msg-radius/g, "var(--chat-bubble-radius)")
  .replace(/\$msg-radius-sm/g, "var(--chat-bubble-radius-sm)")
  .replace(/\$msg-user-max/g, "var(--chat-user-max-width)")
  .replace(/\$msg-opposite-gutter/g, "var(--chat-opposite-gutter)")
  .replace(/\$msg-edge-gutter/g, "var(--chat-edge-gutter)")
  .replace(/\$msg-block-gap/g, "var(--chat-block-gap)")
  .replace(/\$msg-continued-gap/g, "var(--chat-continued-gap)");

const header = `// ChatMessagePanel 样式（自 ChatMessagePanel.vue 提取，使用 --chat-* token）
// stylelint-disable color-no-hex -- token 引用文件

`;

fs.writeFileSync(outPath, header + sass + "\n", "utf8");

const newVue = src.slice(0, start) + src.slice(end + "</style>".length);
fs.writeFileSync(vuePath, newVue.replace(/\n{3,}/g, "\n\n"), "utf8");
console.log("Wrote", outPath);
