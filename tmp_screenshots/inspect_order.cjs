// Inspect DOM order of activity blocks: plan -> graph_stage -> team-card -> agent-card
// Connects to running Chrome via CDP (agent-browser daemon).
const http = require('http');

function cdpSend(method, params = {}) {
  return new Promise((resolve, reject) => {
    const body = JSON.stringify({ id: 1, method, params });
    const req = http.request(
      {
        host: '127.0.0.1',
        port: 9222,
        path: '/json/protocol',
        method: 'GET',
      },
      (res) => {
        let data = '';
        res.on('data', (c) => (data += c));
        res.on('end', () => resolve(data));
      }
    );
    req.on('error', reject);
    req.end();
  });
}

// Use WebSocket instead - check available targets
http.get('http://127.0.0.1:9222/json', (res) => {
  let data = '';
  res.on('data', (c) => (data += c));
  res.on('end', () => {
    try {
      const targets = JSON.parse(data);
      const page = targets.find((t) => t.type === 'page');
      if (!page) {
        console.log('No page target found. Targets:', targets.map((t) => t.type));
        return;
      }
      console.log('Page target webSocketDebuggerUrl:', page.webSocketDebuggerUrl);
    } catch (e) {
      console.log('Parse error:', e.message);
      console.log('Raw:', data.slice(0, 200));
    }
  });
}).on('error', (e) => {
  console.log('HTTP error (port 9222):', e.message);
  // Try common CDP ports
  [9223, 9224, 9225, 9229].forEach((port) => {
    http.get(`http://127.0.0.1:${port}/json`, (res) => {
      let d = '';
      res.on('data', (c) => (d += c));
      res.on('end', () => console.log(`Port ${port}:`, d.slice(0, 100)));
    }).on('error', () => {});
  });
});
