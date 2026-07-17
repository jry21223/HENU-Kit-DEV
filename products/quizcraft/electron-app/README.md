# Electron App

这个目录是桌面端壳工程，复用 `../web-app` 的 React 页面。

## 目录说明

- `main.js`: Electron 主进程入口
- `preload.js`: 安全桥接层（`contextIsolation: true`）
- `package.json`: Electron 启动脚本

## 开发模式

1. 启动后端（项目根目录）

```bash
python server.py
```

2. 启动前端（`web-app`）

```bash
cd web-app
npm run dev
```

3. 启动 Electron（`electron-app`）

```bash
cd electron-app
npm install
npm run dev
```

## 生产预览

在 `electron-app` 目录执行：

```bash
npm install
npm run start:prod
```

`start:prod` 会先构建 `web-app`，再加载 `web-app/dist/index.html`。
