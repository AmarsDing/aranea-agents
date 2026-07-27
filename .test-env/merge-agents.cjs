// 合并 agents 分页 JSON → TSV（UTF-8）
const fs = require('fs');
const path = require('path');
const dir = 'F:\\aranea-agents\\.test-env';
const files = fs.readdirSync(dir).filter(f => /^apage_\d+\.json$/.test(f))
  .sort((a, b) => parseInt(a.match(/\d+/)[0]) - parseInt(b.match(/\d+/)[0]));
const all = [];
for (const f of files) {
  try {
    const d = JSON.parse(fs.readFileSync(path.join(dir, f), 'utf8'));
    if (Array.isArray(d.items)) all.push(...d.items);
  } catch (e) { console.error('PARSE_FAIL', f, e.message); }
}
const esc = s => (s || '').replace(/[\t\r\n]+/g, ' ').trim();
const lines = all.map(a => [a.agentKey, esc(a.displayName), a.status, esc(a.agentDescription)].join('\t'));
fs.writeFileSync(path.join(dir, 'agents-all.tsv'), lines.join('\n'), 'utf8');
console.log('TOTAL=' + all.length);
