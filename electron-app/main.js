const { app, BrowserWindow } = require('electron');
const fs = require('node:fs');
const path = require('node:path');

const DEV_URL = process.env.ELECTRON_START_URL;
const PROD_ENTRY = path.resolve(__dirname, '../web-app/dist/index.html');

function createWindow() {
  const window = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 1000,
    minHeight: 680,
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  if (DEV_URL) {
    window.loadURL(DEV_URL);
    window.webContents.openDevTools({ mode: 'detach' });
    return;
  }

  if (fs.existsSync(PROD_ENTRY)) {
    window.loadFile(PROD_ENTRY);
    return;
  }

  const html = `
    <html>
      <body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; padding: 32px;">
        <h2>QuizCraft Electron</h2>
        <p>Cannot find web build output: <code>${PROD_ENTRY}</code></p>
        <p>Run <code>npm run build</code> in <code>web-app</code> first, or start with <code>npm run dev</code> in <code>electron-app</code> while Vite is running.</p>
      </body>
    </html>
  `;
  window.loadURL(`data:text/html;charset=utf-8,${encodeURIComponent(html)}`);
}

app.whenReady().then(() => {
  createWindow();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});
