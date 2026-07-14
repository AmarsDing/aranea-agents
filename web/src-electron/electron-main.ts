import { app, BrowserWindow, Menu } from 'electron';
import { createServer, type IncomingMessage, type ServerResponse } from 'http';
import { readFile, stat } from 'fs/promises';
import path from 'path';
import os from 'os';
import { fileURLToPath } from 'url';

// needed in case process is undefined under Linux
const platform = process.platform || os.platform();

const currentDir = fileURLToPath(new URL('.', import.meta.url));

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

async function serveStatic(rootDir: string, req: IncomingMessage, res: ServerResponse) {
  let urlPath = (req.url || '/').split('?')[0];
  urlPath = decodeURIComponent(urlPath);

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
    // File doesn't exist — SPA fallback to index.html
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
    staticServer.listen(0, '127.0.0.1', () => {
      const addr = staticServer!.address();
      const port = typeof addr === 'object' && addr ? addr.port : 0;
      resolve(port);
    });
    staticServer.on('error', reject);
  });
}

async function createWindow() {
  mainWindow = new BrowserWindow({
    icon: path.resolve(currentDir, 'icons/icon.png'),
    title: 'Aranea-Agents',
    width: 1400,
    height: 900,
    minWidth: 1024,
    minHeight: 600,
    useContentSize: true,
    autoHideMenuBar: true,
    webPreferences: {
      contextIsolation: true,
      preload: path.resolve(
        currentDir,
        path.join(process.env.QUASAR_ELECTRON_PRELOAD_FOLDER, 'electron-preload' + process.env.QUASAR_ELECTRON_PRELOAD_EXTENSION)
      ),
    },
  });

  if (process.env.DEV) {
    await mainWindow.loadURL(process.env.APP_URL);
  } else {
    // Serve the app from a local HTTP server so the page origin is
    // http://127.0.0.1:PORT/ — same-site as the backend at http://127.0.0.1:8000.
    // This ensures SameSite=Lax session cookies are sent with XHR/fetch/WS requests.
    // Using file:// makes the page cross-site, which blocks cookies and causes
    // a 401 redirect loop (login → overview → 401 → login → ...).
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

void app.whenReady().then(createWindow);

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
