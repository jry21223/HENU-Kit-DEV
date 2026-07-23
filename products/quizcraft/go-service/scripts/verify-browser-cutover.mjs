#!/usr/bin/env node
import { createRequire } from 'node:module';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const baseURL = process.env.CUTOVER_E2E_BASE_URL;
const viewportName = process.env.CUTOVER_VIEWPORT;
const operatorSession = process.env.QUIZCRAFT_OPERATOR_SESSION;
const webAppDirectory = process.env.CUTOVER_WEB_APP_DIR;
if (!baseURL || !operatorSession || !webAppDirectory || !['desktop', 'mobile_390'].includes(viewportName)) {
  throw new Error('CUTOVER_E2E_BASE_URL, CUTOVER_WEB_APP_DIR, QUIZCRAFT_OPERATOR_SESSION, and a supported CUTOVER_VIEWPORT are required');
}
const require = createRequire(pathToFileURL(path.join(webAppDirectory, 'package.json')));
const { chromium } = require('playwright');
const origin = new URL(baseURL);
if (origin.protocol !== 'https:') throw new Error('cutover browser verification requires HTTPS');
const viewport = viewportName === 'mobile_390' ? { width: 390, height: 844 } : { width: 1440, height: 1000 };
const browser = await chromium.launch({ headless: true });
try {
  const context = await browser.newContext({ viewport });
  await context.addCookies([{
    name: '__Host-quizcraft_session', value: operatorSession, url: origin.origin,
    secure: true, httpOnly: true, sameSite: 'Lax',
  }]);
  const page = await context.newPage();
  const failures = [];
  page.on('pageerror', (error) => failures.push(`page:${error.message}`));
  page.on('response', (response) => {
    if (response.status() >= 500) failures.push(`http:${response.status()}:${response.url()}`);
  });

  await page.goto(new URL('/practice', origin).href, { waitUntil: 'networkidle' });
  await page.getByRole('button', { name: '开始练习' }).first().click();
  await page.waitForURL(/\/quiz(?:\?|$)/);
  const options = page.getByRole('button').filter({ hasText: /^[A-Z][.、\s]/ });
  if (await options.count() === 0) throw new Error('practice did not render answer options');
  await options.first().click();
  await page.getByRole('button', { name: '提交答案' }).click();
  await page.getByRole('button', { name: '收藏本题' }).click();
  await page.getByRole('button', { name: '取消收藏本题' }).click();
  await page.getByRole('button', { name: '反馈本题' }).click();
  await page.getByLabel('反馈建议').fill(`HC-22 ${viewportName} cutover verification`);
  await page.getByRole('button', { name: '提交反馈' }).click();
  await page.getByText('反馈提交成功，感谢你的建议！').waitFor();

  await page.goto(new URL('/ranking', origin).href, { waitUntil: 'networkidle' });
  await page.getByRole('heading', { name: '排行榜' }).waitFor();
  await page.goto(new URL('/extract', origin).href, { waitUntil: 'networkidle' });
  await page.getByRole('heading', { name: '题库工坊' }).waitFor();
  if (failures.length) throw new Error(`browser failures: ${failures.join(', ')}`);
} finally {
  await browser.close();
}
