import { expect, test } from 'playwright/test';

test('ranking stays fail-closed before #166 even when the V2 write flag is set', async ({ page }) => {
  let v2RankingRequests = 0;
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.includes('/rankings') || path === '/api/v1/ranking-profile') v2RankingRequests += 1;
    await route.abort();
  });

  await page.goto('/ranking');
  await expect(page.getByRole('alert')).toHaveText('旧排行榜正在迁移，暂不可用');
  expect(v2RankingRequests).toBe(0);
});

test('ranking stays explicit and sends no V2 ranking request at a narrow viewport', async ({ page }) => {
  let v2RankingRequests = 0;
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.includes('/rankings') || path === '/api/v1/ranking-profile') v2RankingRequests += 1;
    await route.abort();
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/ranking');
  await expect(page.getByRole('alert')).toHaveText('旧排行榜正在迁移，暂不可用');
  expect(v2RankingRequests).toBe(0);
});
