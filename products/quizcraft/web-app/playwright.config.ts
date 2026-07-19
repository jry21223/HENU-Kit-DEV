import { defineConfig } from 'playwright/test';

export default defineConfig({
  testDir: './tests/browser',
  timeout: 30_000,
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
  },
  webServer: {
    command:
      'VITE_QUIZCRAFT_GO_SHADOW=1 VITE_QUIZCRAFT_GO_API_BASE_URL=http://127.0.0.1:18080 VITE_QUIZCRAFT_LOGIN_URL=http://127.0.0.1:18080/auth/login npm run dev -- --host 127.0.0.1 --port 4173',
    url: 'http://127.0.0.1:4173/practice',
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
