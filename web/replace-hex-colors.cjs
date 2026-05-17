const fs = require('fs');
const path = require('path');

const HEX_MAP = {
  '#ffffff': 'var(--color-on-accent)',
  '#fff': 'var(--color-on-accent)',
  '#fefbf4': 'var(--canvas-base)',
  '#fefdf5': 'var(--canvas-base)',
  '#090d14': 'var(--canvas-base)',
  '#0b1120': 'var(--canvas-base)',
  '#0b1220': 'var(--canvas-base)',
  '#0b1224': 'var(--canvas-base)',
  '#0f172a': 'var(--color-surface-solid)',
  '#111827': 'var(--color-surface-elevated)',
  '#1e293b': 'var(--color-surface-soft)',
  '#1d2939': 'var(--color-text-heading)',
  '#101828': 'var(--color-text-dark)',
  '#3a322c': 'var(--color-text-primary)',
  '#8b7a6b': 'var(--color-text-secondary)',
  '#667085': 'var(--color-text-tertiary)',
  '#64748b': 'var(--color-text-tertiary)',
  '#5f6b7a': 'var(--color-text-tertiary)',
  '#475467': 'var(--color-text-tertiary)',
  '#94a3b8': 'var(--color-text-slate-400)',
  '#b8a590': 'var(--color-icon-muted)',
  '#9ca0b0': 'var(--color-text-secondary)',
  '#ebebf0': 'var(--color-text-primary)',
  '#e2e8f0': 'var(--color-text-dark)',
  '#f8fafc': 'var(--color-surface-soft)',
  '#e5e7eb': 'var(--color-border-soft)',
  '#d1d5db': 'var(--color-text-gray-300)',
  '#e9a23b': 'var(--color-accent)',
  '#d48c1a': 'var(--color-accent-hover)',
  '#00e5ff': 'var(--color-accent)',
  '#5aebff': 'var(--color-accent-hover)',
  '#1976d2': 'var(--color-info)',
  '#155ebc': 'var(--color-link)',
  '#2563eb': 'var(--color-accent-blue)',
  '#3b82f6': 'var(--color-accent-blue)',
  '#60a5fa': 'var(--color-info)',
  '#93c5fd': 'var(--color-link)',
  '#4caf7c': 'var(--color-success)',
  '#3fe0a0': 'var(--color-success)',
  '#4caf50': 'var(--color-success)',
  '#027a48': 'var(--color-accent-green)',
  '#86efac': 'var(--color-accent-green)',
  '#f09b54': 'var(--color-warning)',
  '#ffaf4d': 'var(--color-warning)',
  '#e55c5c': 'var(--color-danger)',
  '#ff5e7a': 'var(--color-danger)',
  '#f44336': 'var(--color-danger)',
  '#eef6ff': 'var(--color-info-soft)',
  '#ecfdf3': 'var(--color-success-soft)',
  '#a855f7': 'var(--color-neon-violet)',
  '#fef3e4': 'var(--interaction-surface-hover)',
  '#fff8e7': 'var(--color-surface-soft)',
  '#fffbf2': 'var(--canvas-base)',
  '#5d4037': 'var(--color-cream-text)',
  '#795548': 'var(--color-cream-text-muted)',
  '#8d6e63': 'var(--color-cream-text-light)',
  '#fbfcff': 'var(--color-page-tint)',
  '#f7f9fc': 'var(--color-page-tint-alt)',
  '#f3f6ff': 'var(--color-page-tint-blue)',
  '#f1f5f9': 'var(--color-page-tint-cool)',
  '#f7f8fb': 'var(--color-page-tint-warm)',
  '#f8faff': 'var(--color-status-blue-bg)',
  '#eef2f6': 'var(--color-status-info-bg)',
  '#e7f8ef': 'var(--color-status-success-bg)',
  '#fff7ed': 'var(--color-status-warning-bg)',
  '#fff7f7': 'var(--color-status-danger-bg)',
  '#fff4e5': 'var(--color-status-warning-bg-alt)',
  '#fff8ed': 'var(--color-status-warning-bg-warm)',
  '#eff6ff': 'var(--color-status-info-bg-alt)',
  '#f4f9ff': 'var(--color-status-blue-bg)',
  '#edf5ff': 'var(--color-info-soft)',
  '#fff1f2': 'var(--color-danger-soft)',
  '#b45309': 'var(--color-status-warning-text)',
  '#9a3412': 'var(--color-status-warning-text-dark)',
  '#1e40af': 'var(--color-status-info-text)',
  '#175cd3': 'var(--color-status-info-text-light)',
  '#334155': 'var(--color-text-slate-700)',
  '#475569': 'var(--color-text-slate-600)',
  '#cbd5e1': 'var(--color-text-slate-300)',
  '#374151': 'var(--color-text-gray-700)',
  '#4b5563': 'var(--color-text-gray-600)',
  '#6b7280': 'var(--color-text-gray-500)',
  '#9ca3af': 'var(--color-text-gray-400)',
  '#1f2937': 'var(--color-text-gray-800)',
  '#344054': 'var(--color-text-gray-600-alt)',
  '#4f46e5': 'var(--color-accent-indigo)',
  '#6366f1': 'var(--color-accent-indigo-light)',
  '#818cf8': 'var(--color-accent-indigo-lighter)',
  '#4338ca': 'var(--color-accent-indigo-dark)',
  '#3730a3': 'var(--color-accent-indigo-darker)',
  '#fbbf24': 'var(--color-accent-amber)',
  '#fcd34d': 'var(--color-accent-amber-light)',
  '#92400e': 'var(--color-accent-amber-dark)',
  '#fdba74': 'var(--color-accent-orange-light)',
  '#fed7aa': 'var(--color-accent-orange-bg)',
  '#22c55e': 'var(--color-accent-green-light)',
  '#bbf7d0': 'var(--color-accent-green-bright)',
  '#bfdbfe': 'var(--color-accent-blue-light)',
  '#dbeafe': 'var(--color-accent-blue-bright)',
  '#21ba45': 'var(--color-quasar-positive)',
  '#c10015': 'var(--color-quasar-negative)',
  '#f2c037': 'var(--color-quasar-warning)',
  '#9e9e9e': 'var(--color-quasar-grey)',
  '#9a6a4f': 'var(--color-cream-accent)',
  '#4e342e': 'var(--color-cream-accent-dark)',
  '#b42318': 'var(--color-danger-text)',
  '#16a34a': 'var(--color-success)',
  '#f2f4f7': 'var(--color-interaction-surface-alt)',
  '#757575': 'var(--color-text-tertiary)',
  '#5c6bc0': 'var(--color-accent-indigo)',
  '#26c6da': 'var(--color-accent-blue)',
  '#ab47bc': 'var(--color-neon-violet)',
  '#26a69a': 'var(--color-success)',
  '#7e57c2': 'var(--color-neon-violet)',
  '#ff7043': 'var(--color-warning)',
  '#42a5f5': 'var(--color-accent-blue)',
  '#ec407a': 'var(--color-danger)',
  '#66bb6a': 'var(--color-success)',
  '#ffa726': 'var(--color-warning)',
  '#666': 'var(--color-text-tertiary)',
};

function findVueFiles(dir) {
  const results = [];
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (['node_modules', '.quasar', 'dist', '.git'].includes(entry.name)) continue;
      results.push(...findVueFiles(fullPath));
    } else if (entry.name.endsWith('.vue')) {
      results.push(fullPath);
    }
  }
  return results;
}

function replaceHexInStyle(content) {
  const styleMatch = content.match(/<style[^>]*>([\s\S]*?)<\/style>/);
  if (!styleMatch) return { content, count: 0 };

  let styleContent = styleMatch[1];
  let count = 0;

  const hexPattern = /#[0-9a-fA-F]{3,8}\b/g;
  const newStyle = styleContent.replace(hexPattern, (hex) => {
    const lower = hex.toLowerCase();
    if (HEX_MAP[lower]) {
      count++;
      return HEX_MAP[lower];
    }
    if (hex.length === 4) {
      const expanded = '#' + hex[1] + hex[1] + hex[2] + hex[2] + hex[3] + hex[3];
      if (HEX_MAP[expanded]) {
        count++;
        return HEX_MAP[expanded];
      }
    }
    return hex;
  });

  if (count > 0) {
    content = content.replace(styleMatch[1], newStyle);
  }
  return { content, count };
}

const srcDir = path.join(__dirname, 'src');
const files = findVueFiles(srcDir);
let totalReplacements = 0;
let filesModified = 0;

for (const file of files) {
  const original = fs.readFileSync(file, 'utf8');
  const { content: updated, count } = replaceHexInStyle(original);
  if (count > 0) {
    fs.writeFileSync(file, updated, 'utf8');
    totalReplacements += count;
    filesModified++;
    const short = file.replace(/\\/g, '/').replace(/.*\/web\/src\//, 'src/');
    console.log(`${count} replacements in ${short}`);
  }
}

console.log(`\nTotal: ${totalReplacements} replacements in ${filesModified} files`);
