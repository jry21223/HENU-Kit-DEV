import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

const proxy = {
  '/api': {
    target: 'http://127.0.0.1:10086',
    changeOrigin: true,
  },
  '/ws': {
    target: 'ws://127.0.0.1:10086',
    changeOrigin: true,
    ws: true,
  },
}

function normalizeBase(raw: string | undefined): string {
  const value = (raw ?? '/').trim() || '/'
  if (value === '/') return '/'
  const withLeading = value.startsWith('/') ? value : `/${value}`
  return withLeading.endsWith('/') ? withLeading : `${withLeading}/`
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const base = normalizeBase(env.VITE_BASE_PATH ?? process.env.VITE_BASE_PATH)

  return {
    base,
    plugins: [react()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
    },
    server: {
      port: 5173,
      strictPort: true,
      proxy,
    },
    preview: {
      port: 5173,
      strictPort: true,
      proxy,
    },
  }
})
