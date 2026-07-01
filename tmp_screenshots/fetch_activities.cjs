const http = require('http');
const sessionId = 'a7ed74d1-bc98-4f48-ade4-67115b94f45f';
const url = `http://127.0.0.1:8000/v1/sessions/${sessionId}/activities?limit=200`;

http.get(url, { headers: { 'Accept': 'application/json' } }, (res) => {
  let data = '';
  res.on('data', chunk => data += chunk);
  res.on('end', () => {
    try {
      const json = JSON.parse(data);
      const items = json.activities || json.items || json.data || [];
      console.log('total activities:', items.length);
      for (const a of items) {
        console.log(`${a.id?.slice(0,8)} | ${a.kind?.padEnd(12)} | ${a.status?.padEnd(12)} | ${a.parent_activity_id?.slice(0,8) || 'root'} | sid=${a.session_id?.slice(0,8)} | ${(a.agent_key || '').padEnd(16)} | ${(a.content || a.title || '').slice(0, 30)}`);
      }
    } catch (e) {
      console.error('parse error:', e.message);
      console.log('raw:', data.slice(0, 500));
    }
  });
}).on('error', e => console.error('http error:', e.message));
