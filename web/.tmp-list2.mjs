import fs from 'fs';
const r = JSON.parse(fs.readFileSync('./lint-report.json', 'utf8'));
for (const f of r) {
  for (const m of f.messages || []) {
    if (m.ruleId === '@typescript-eslint/no-unused-vars') {
      const name = m.message.match(/'([^']+)'/)?.[1] || m.message;
      console.log(f.filePath.replace(process.cwd() + '/', ''), m.line + ':' + m.column, name);
    }
  }
