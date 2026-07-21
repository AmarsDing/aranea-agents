import { app, BrowserWindow, Menu } from 'electron';
import { createServer, request as httpRequest, type IncomingMessage, type ServerResponse } from 'http';
import { readFile, stat } from 'fs/promises';
import net from 'net';
import path from 'path';
import os from 'os';
import { fileURLToPath } from 'url';

// needed in case process is undefined under Linux
const platform = process.platform || os.platform();

const currentDir = fileURLToPath(new URL('.', import.meta.url));

/** Portable install backend (see installer staging config). */
const BACKEND_HOST = '127.0.0.1';
const BACKEND_PORT = 8000;

// Remove the default application menu (File/Edit/View/Window/Help).
Menu.setApplicationMenu(null);

let mainWindow: BrowserWindow | undefined;
let staticServer: ReturnType<typeof createServer> | null = null;

const MIME_TYPES: Record<string, string> = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.eot': 'application/vnd.ms-fontobject',
  '.map': 'application/json',
  '.webp': 'image/webp',
};

function shouldProxy(urlPath: string): boolean {
  return (
    urlPath === '/healthz' ||
    urlPath.startsWith('/v1/') ||
    urlPath.startsWith('/api/') ||
    urlPath.startsWith('/openapi')
  );
}

function proxyHttp(req: IncomingMessage, res: ServerResponse) {
  const headers = { ...req.headers, host: `${BACKEND_HOST}:${BACKEND_PORT}` };
  const opts = {
    hostname: BACKEND_HOST,
    port: BACKEND_PORT,
    path: req.url,
    method: req.method,
    headers,
  };
  const upstream = httpRequest(opts, (upRes) => {
    res.writeHead(upRes.statusCode || 502, upRes.headers);
    upRes.pipe(res);
  });
  upstream.on('error', () => {
    if (!res.headersSent) {
      res.writeHead(502, { 'Content-Type': 'text/plain; charset=utf-8' });
    }
    res.end('Backend unavailable. Start Aranea via desktop shortcut / AraneaLauncher.exe');
  });
  req.pipe(upstream);
}

function proxyUpgrade(req: IncomingMessage, socket: net.Socket, head: Buffer) {
  const upstream = net.connect(BACKEND_PORT, BACKEND_HOST, () => {
    const pathAndQuery = req.url || '/';
    let payload = `GET ${pathAndQuery} HTTP/1.1\r\n`;
    for (const [k, v] of Object.entries(req.headers)) {
      if (v === undefined) continue;
      if (Array.isArray(v)) {
        for (const item of v) payload += `${k}: ${item}\r\n`;
      } else {
        payload += `${k}: ${v}\r\n`;
      }
    }
    payload += '\r\n';
    upstream.write(payload);
    if (head.length) upstream.write(head);
    socket.pipe(upstream);
    upstream.pipe(socket);
  });
  upstream.on('error', () => {
    socket.destroy();
  });
  socket.on('error', () => {
    upstream.destroy();
  });
}

async function serveStatic(rootDir: string, req: IncomingMessage, res: ServerResponse) {
  let urlPath = (req.url || '/').split('?')[0];
  urlPath = decodeURIComponent(urlPath);

  if (shouldProxy(urlPath)) {
    proxyHttp(req, res);
    return;
  }

  // Prevent path traversal
  if (urlPath.includes('..')) {
    res.writeHead(403);
    res.end('Forbidden');
    return;
  }

  let filePath = path.join(rootDir, urlPath);

  // If the path doesn't have an extension or is a directory, fall back to index.html (SPA routing)
  try {
    const s = await stat(filePath);
    if (s.isDirectory()) {
      filePath = path.join(filePath, 'index.html');
    }
  } catch {
    // File doesn't exist 鈥?SPA fallback to index.html
    filePath = path.join(rootDir, 'index.html');
  }

  const ext = path.extname(filePath);
  const contentType = MIME_TYPES[ext] || 'application/octet-stream';

  try {
    const data = await readFile(filePath);
    res.writeHead(200, { 'Content-Type': contentType });
    res.end(data);
  } catch {
    res.writeHead(404);
    res.end('Not found');
  }
}

function startStaticServer(rootDir: string): Promise<number> {
  return new Promise((resolve, reject) => {
    staticServer = createServer((req, res) => {
      serveStatic(rootDir, req, res).catch(() => {
        if (!res.headersSent) {
          res.writeHead(500);
          res.end('Internal server error');
        }
      });
    });
    staticServer.on('upgrade', (req, socket, head) => {
      const urlPath = (req.url || '').split('?')[0];
      if (shouldProxy(urlPath) || urlPath.startsWith('/v1/ws')) {
        proxyUpgrade(req, socket as net.Socket, head);
        return;
      }
      socket.destroy();
    });
    staticServer.listen(0, '127.0.0.1', () => {
      const addr = staticServer!.address();
      const port = typeof addr === 'object' && addr ? addr.port : 0;
      resolve(port);
    });
    staticServer.on('error', reject);
  });
}

async function createWindow() {
  const preloadPath = resolvePreloadPath();

  mainWindow = new BrowserWindow({
    icon: path.resolve(currentDir, 'icons/icon.png'),
    title: 'Aranea-Agents',
    width: 1400,
    height: 900,
    minWidth: 1024,
    minHeight: 600,
    useContentSize: true,
    autoHideMenuBar: true,
    show: false,
    webPreferences: {
      contextIsolation: true,
      preload: preloadPath,
    },
  });

  if (process.env.DEV) {
    await mainWindow.loadURL(process.env.APP_URL);
  } else {
    // Local HTTP origin + reverse-proxy /v1|/api|/healthz|/v1/ws 鈫?backend :8000.
    // Same-origin cookies work without cross-port CORS; login no longer 401-loops.
    const port = await startStaticServer(currentDir);
    await mainWindow.loadURL(`http://127.0.0.1:${port}/`);
  }

  // Explicitly show and focus the window (fixes blank window on some Windows setups)
  mainWindow.show();
  mainWindow.focus();

  if (process.env.DEBUGGING) {
    mainWindow.webContents.openDevTools();
  } else {
    mainWindow.webContents.on('devtools-opened', () => {
      mainWindow?.webContents.closeDevTools();
    });
  }

  mainWindow.on('closed', () => {
    mainWindow = undefined;
  });
}

function resolvePreloadPath(): string {
  // Quasar injects these only in `quasar dev -m electron`. Portable builds must
  // fall back to the packaged preload written by scripts/build-electron.mjs.
  const folder = process.env.QUASAR_ELECTRON_PRELOAD_FOLDER;
  const ext = process.env.QUASAR_ELECTRON_PRELOAD_EXTENSION;
  if (folder && ext) {
    return path.resolve(currentDir, folder, 'electron-preload' + ext);
  }
  return path.resolve(currentDir, 'preload', 'electron-preload.cjs');
}

void app.whenReady().then(() =>
  createWindow().catch((err) => {
    console.error('Failed to create window:', err);
    // Keep process from becoming a headless zombie with no UI.
    app.quit();
  })
);

app.on('window-all-closed', () => {
  if (staticServer) {
    staticServer.close();
    staticServer = null;
  }
  if (platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', () => {
  if (mainWindow === undefined) {
    void createWindow();
  }
});
