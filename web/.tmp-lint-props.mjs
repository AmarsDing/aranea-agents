import { readFileSync } from 'fs';
const r = JSON.parse(readFileSync('./lint-report.json', 'utf8'));
const map = {};
for (const f of r) {
  for (const m of f.messages) {
    if (m.ruleId !== 'vue/no-mutating-props') continue;
    if (!map[f.filePath]) map[f.filePath] = new Set();
    const match = m.message.match(/"(.+?)"/);
    map[f.filePath].add(match ? match[1] : m.message);
  }
}
for (const [k, v] of Object.entries(map)) {
  console.log(k.replace(/.*\\web\\src\\/, '').replace(/\\/g, '/'));
  console.log('  ' + [...v].join(', '));
}
console.log('TOTAL FILES:', Object.keys(map).length, 'TOTAL PROPS:', Object.values(map).reduce((a, s) => a + s.size, 0));
const tot = r.reduce((a, f) => a + f.messages.length, 0);
const by = {};
for (const f of r) {
  for (const m of f.messages) {
    by[m.ruleId] = (by[m.ruleId] || 0) + 1;
  }
}
console.log('ALL total', tot);
console.log(JSON.stringify(by, null, 2));
