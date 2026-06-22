import { readFileSync } from 'fs';
const r = JSON.parse(readFileSync('./lint-report.json', 'utf8'));
const tot = r.reduce((a, f) => a + f.messages.length, 0);
const by = {};
for (const f of r) {
  for (const m of f.messages) {
    by[m.ruleId] = (by[m.ruleId] || 0) + 1;
  }
}
console.log('total', tot);
console.log(JSON.stringify(by, null,