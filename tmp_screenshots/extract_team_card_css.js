const fs = require('fs');
const path = 'F:\\aranea-agents\\web\\dist\\assets\\index-f0k246fP.css';
const css = fs.readFileSync(path, 'utf8');

// Find all .team-card rules
const matches = css.match(/\.team-card[^}]*}[^}]*}/g) || [];
for (const m of matches.slice(0, 10)) {
  console.log('---');
  console.log(m.substring(0, 800));
}

console.log('\nTotal .team-card matches:', matches.length);
