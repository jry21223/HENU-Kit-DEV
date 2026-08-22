import { defineConfig } from 'playwright/test';

const rankingDark = process.env.QUIZCRAFT_E2E_RANKING_DARK === '1';
const productionPreview = process.env.QUIZCRAFT_E2E_PREVIEW === '1';

export default defineConfig({
  testDir: './tests/browser',
  timeout: 30_000,
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
  },
  webServer: {
    command:
      rankingDark
        ? 'VITE_QUIZCRAFT_GO_WRITES=1 VITE_QUIZCRAFT_GO_API_BASE_URL=http://127.0.0.1:18080 npm run dev -- --host 127.0.0.1 --port 4173'
        : productionPreview
          ? 'npm run preview -- --host 127.0.0.1 --port 4173'
        : 'VITE_QUIZCRAFT_GO_WRITES=1 VITE_QUIZCRAFT_WORKSHOP=1 VITE_QUIZCRAFT_GO_API_BASE_URL=http://127.0.0.1:18080 VITE_QUIZCRAFT_LOGIN_URL=http://127.0.0.1:18080/auth/login npm run dev -- --host 127.0.0.1 --port 4173',
    url: 'http://127.0.0.1:4173/practice',
    reuseExistingServer: false,
    timeout: 120_000,
  },
});
