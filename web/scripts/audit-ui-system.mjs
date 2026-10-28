import { readdir, readFile, mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Inventory source coverage separately from browser acceptance. No API calls.
const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const srcRoot = path.join(webRoot, 'src');
async function walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const nested = await Promise.all(entries.map((entry) => {
    const target = path.join(dir, entry.name);
    return entry.isDirectory() ? walk(target) : [target];
  }));
  return nested.flat();
}
const files = (await walk(srcRoot)).filter((file) => file.endsWith('.vue'));
const count = (source, pattern) => [...source.matchAll(pattern)].length;
const inventory = [];
for (const file of files.sort()) {
  const source = await readFile(file, 'utf8');
  const template = source.match(/<template>([\s\S]*?)<script/)?.[1] ?? source;
  const styles = [...source.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
  const rel = path.relative(webRoot, file).replaceAll('\\', '/');
  inventory.push({
    file: rel,
    page: rel.startsWith('src/pages/'),
    root: template.match(/<(?:q-page|div|section|ChatWorkspace)\b[^>]*>/)?.[0] ?? '',
    buttons: count(template, /<q-btn\b/g),
    fields: count(template, /<q-(?:input|select)\b/g),
    dialogs: count(template, /<q-dialog\b/g),
    tables: count(template, /<(?:q-table|q-markup-table|table)\b/g),
    registryTables: count(template, /<(?:AppRegistryTable|app-registry-table|AppRegistryMarkupTable)\b/g),
    localColors: count(styles.replace(/\/\/[^\n]*|\/\*[\s\S]*?\*\//g, ''), /#[\da-f]{3,8}\b|\brgba?\(\s*\d/gi),
    inlineStyles: count(template, /\s(?::)?style=/g),
    fixedWidths: count(styles, /\b(?:min-)?width:\s*(?:[6-9]\d{2}|\d{4,})px/g),
    actions: [...new Set([...template.matchAll(/@(click|submit|delete|save|retry|refresh)[\w:.-]*=/g)].map((m) => m[1]))],
  });
}
const routes = await readFile(path.join(srcRoot, 'router/routes.ts'), 'utf8');
const routePages = [...new Set([...routes.matchAll(/(?:\.\.\/pages\/)([^'"]+\.vue)/g)].map((m) => `src/pages/${m[1]}`))].sort();
const totals = Object.fromEntries(['buttons', 'fields', 'dialogs', 'tables', 'registryTables', 'localColors', 'inlineStyles', 'fixedWidths']
  .map((key) => [key, inventory.reduce((sum, entry) => sum + entry[key], 0)]));
const report = {
  scope: 'Static Vue source inventory; counts do not prove working interactions or visual acceptance.',
  vueFiles: inventory.length,
  pageFiles: inventory.filter((entry) => entry.page).length,
  routedPageFiles: routePages.length,
  totals,
  routePages,
  pages: inventory.filter((entry) => entry.page),
  componentsWithLocalColors: inventory.filter((entry) => entry.localColors).sort((a, b) => b.localColors - a.localColors),
};
const outputIndex = process.argv.indexOf('--output');
if (outputIndex >= 0 && process.argv[outputIndex + 1]) {
  const output = path.resolve(webRoot, process.argv[outputIndex + 1]);
  await mkdir(path.dirname(output), { recursive: true });
  await writeFile(output, JSON.stringify(report, null, 2) + '\n');
  console.log(`Inventory: ${output}`);
}
console.log(JSON.stringify({ vueFiles: report.vueFiles, pageFiles: report.pageFiles, routedPageFiles: report.routedPageFiles, ...totals }, null, 2));
if (process.argv.includes('--check')) {
  const failures = [];
  const entryHTML = await readFile(path.join(webRoot, 'index.html'), 'utf8');
  if (/fonts\.(?:googleapis|gstatic)\.com/i.test(entryHTML)) failures.push('Entry fonts must use the shared local font stack; remove the blocking external font stylesheet.');
  if (totals.localColors) failures.push(`${totals.localColors} literal colors in Vue styles; use theme tokens.`);
  const required = ['canvas-base', 'surface-base', 'surface-raised', 'surface-muted', 'color-text-primary', 'color-text-secondary', 'color-text-tertiary', 'color-accent', 'q-primary', 'button-primary-bg', 'button-primary-text', 'radius-control', 'radius-card', 'focus-ring', 'font-body'];
  for (const theme of ['light', 'dark']) {
    const source = await readFile(path.join(srcRoot, `css/theme/_css-vars-${theme}.sass`), 'utf8');
    const entries = [...source.matchAll(/^  --([\w-]+): (.+)$/gm)].map((m) => [m[1], m[2]]);
    const tokens = new Map(entries);
    if (tokens.size !== entries.length) failures.push(`${theme}: duplicate token declarations`);
    for (const key of required) if (!tokens.has(key)) failures.push(`${theme}: missing --${key}`);
    const resolve = (key, chain = []) => {
      if (chain.includes(key)) throw new Error(`${theme}: token cycle ${[...chain, key].join(' -> ')}`);
      return tokens.get(key)?.replace(/var\(--([\w-]+)\)/g, (_, ref) => resolve(ref, [...chain, key]) ?? `var(--${ref})`);
    };
    for (const key of tokens.keys()) {
      try { resolve(key); } catch (error) { failures.push(error.message); }
    }
    if (resolve('q-primary') !== resolve('color-accent')) failures.push(`${theme}: Quasar primary differs from accent`);
    const rgb = (hex) => /^#[0-9a-f]{6}$/i.test(hex) ? hex.slice(1).match(/../g).map((part) => parseInt(part, 16)) : null;
    const luminance = (channels) => channels.map((value) => value / 255).map((value) => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
    const contrast = (a, b) => (Math.max(luminance(a), luminance(b)) + 0.05) / (Math.min(luminance(a), luminance(b)) + 0.05);
    for (const foreground of ['color-text-primary', 'color-text-secondary', 'color-text-tertiary', 'color-accent']) {
      for (const surface of ['surface-base', 'surface-raised', 'surface-muted']) {
        const a = rgb(resolve(foreground) ?? '');
        const b = rgb(resolve(surface) ?? '');
        if (!a || !b || contrast(a, b) < 4.5) failures.push(`${theme}: --${foreground} on --${surface} must reach 4.5:1`);
      }
    }
    const buttonText = rgb(resolve('button-primary-text') ?? '');
    for (const stop of resolve('button-primary-bg')?.match(/#[0-9a-f]{6}/gi) ?? []) {
      if (buttonText && contrast(buttonText, rgb(stop)) < 4.5) failures.push(`${theme}: primary action text contrast is below 4.5:1`);
    }
  }
  if (failures.length) {
    console.error(failures.join('\n'));
    process.exitCode = 1;
  } else console.log('UI contract passed: token coverage, no duplicate/circular tokens, shared primary, no literal Vue style colors, core text/button contrast >= 4.5:1.');
}
