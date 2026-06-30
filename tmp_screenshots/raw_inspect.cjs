const fs = require('fs');
const d = JSON.parse(fs.readFileSync('tmp_screenshots/activities_new.json', 'utf8'));
console.log('Top-level type:', Array.isArray(d) ? 'array' : 'object');
console.log('Top-level keys:', Object.keys(d));
const arr = d.activities || d.items || (Array.isArray(d) ? d : []);
console.log('Activities count:', arr.length);
if (arr.length > 0) {
  console.log('\nFirst activity keys:', Object.keys(arr[0]));
  console.log('\nFirst activity full:');
  console.log(JSON.stringify(arr[0], null, 2).substring(0, 3000));
}
// Find a team_stage activity
const ts = arr.find(a => a.kind === 'team_stage');
if (ts) {
  console.log('\n=== TEAM_STAGE FULL ===');
  console.log(JSON.stringify(ts, null, 2));
}
