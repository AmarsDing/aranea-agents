import http from 'http';
import fs from 'fs';
import path from 'path';

const BACKEND = '127.0.0.1';
const BACKEND_PORT = 8800;
const PORT = 9304;

function proxyRequest(req, res) {
  const options = {
    hostname: BACKEND,
    port: BACKEND_PORT,
    path: req.url,
    method: req.method,
    headers: { ...req.headers, host: `${BACKEND}:${BACKEND_PORT}` }
  };
  const proxyReq = http.request(options, (proxyRes) => {
    res.writeHead(proxyRes.statusCode, proxyRes.headers);
    proxyRes.pipe(res);
  });
  proxyReq.on('error', () => {
    res.writeHead(502);
    res.end('Proxy error');
  });
  req.pipe(proxyReq);
}

http.createServer((req, res) => {
  if (req.url.startsWith('/v1/') || req.url.startsWith('/v2/') || req.url.startsWith('/api/') || req.url.startsWith('/healthz')) {
    return proxyRequest(req, res);
  }
  const filePath = req.url === '/' ? 'index.html' : req.url;
  const fullPath = path.join('dist/spa', filePath);
  fs.readFile(fullPath, (err, data) => {
    if (err) {
      fs.readFile('dist/spa/index.html', (err2, data2) => {
        if (err2) { res.writeHead(404); res.end(); return; }
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(data2);
      });
      return;
    }
    const ext = path.extname(fullPath).toLowerCase();
    const types = {
      '.html': 'text/html', '.js': 'application/javascript', '.css': 'text/css',
      '.ico': 'image/x-icon', '.woff': 'font/woff', '.woff2': 'font/woff2',
      '.wasm': 'application/wasm', '.json': 'application/json'
    };
    res.writeHead(200, { 'Content-Type': types[ext] || 'application/octet-stream' });
    res.end(data);
  });
}).listen(PORT, () => console.log(`http://localhost:${PORT}`));
