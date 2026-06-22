import { readFileSync } from 'fs';
const report = JSON.parse(readFileSync('lint-report.json', 'utf8'));
const ruleCounts = {};
const fileCounts = {};
let total = 0;
for (const file of report) {
  const filePath = file.filePath.replace(/\\\\/g, '/');
  if (!file.messages) continue;
  for (const msg of file.messages) {
    if (msg.severity !== 1 && msg.severity !== 2) continue;
    total++;
    const rule = msg.ruleId || 'unknown';
    ruleCounts[rule] = (ruleCounts[rule] || 0) + 1;
    fileCounts[filePath] = (fileCounts[filePath] || 0) + 1;
  }
}
console.log('Total messages: ' + total);
console.log('By rule:');
for (const [rule, count] of Object.entries(ruleCounts).sort((a, b) => b[1] - a[1])) {
  console.log('  ' + rule + ': ' + count);
}
console.log('Top files:');
for (const [file, count] of Object.entries(fileCounts).sort((a, b) => b[1] - a[1]).slice(0, 40)) {
  console.log('  ' + count +