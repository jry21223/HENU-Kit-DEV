#!/usr/bin/env node
import { createRequire } from 'node:module';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const baseURL = process.env.CUTOVER_E2E_BASE_URL;
const viewportName = process.env.CUTOVER_VIEWPORT;
const webAppDirectory = process.env.CUTOVER_WEB_APP_DIR;
if (!baseURL || !webAppDirectory || !['desktop', 'mobile_390'].includes(viewportName)) {
  throw new Error('CUTOVER_E2E_BASE_URL, CUTOVER_WEB_APP_DIR, and a supported CUTOVER_VIEWPORT are required');
}
const require = createRequire(pathToFileURL(path.join(webAppDirectory, 'package.json')));
const { chromium } = require('playwright');
const origin = new URL(baseURL);
if (origin.protocol !== 'https:') throw new Error('cutover browser verification requires HTTPS');
const viewport = viewportName === 'mobile_390' ? { width: 390, height: 844 } : { width: 1440, height: 1000 };
const browser = await chromium.launch({ headless: true });
try {
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();
  const failures = [];
  const retiredCopy = ['QuizCraft', '题库工坊', 'QC-', 'V2 排行榜', '管理令牌', '随机大转盘', '开源项目，可自行部署'];
  const assertPublicCopy = async () => {
    const visibleText = await page.locator('body').innerText();
    const title = await page.title();
    for (const copy of retiredCopy) {
      if (visibleText.includes(copy) || title.includes(copy)) throw new Error(`retired public copy is visible: ${copy}`);
    }
  };
  page.on('pageerror', (error) => failures.push(`page:${error.message}`));
  page.on('response', (response) => {
    if (response.status() >= 500) failures.push(`http:${response.status()}:${response.url()}`);
  });

  await page.goto(new URL('/practice', origin).href, { waitUntil: 'networkidle' });
  await assertPublicCopy();
  await page.getByRole('button', { name: '开始练习' }).first().click();
  await page.waitForURL(/\/quiz(?:\?|$)/);
  const options = page.getByRole('button').filter({ hasText: /^[A-Z][.、\s]/ });
  if (await options.count() === 0) throw new Error('practice did not render answer options');
  await options.first().click();
  await page.getByRole('button', { name: '提交答案' }).click();
  await page.getByRole('button', { name: '反馈本题' }).click();
  await page.getByLabel('反馈建议').fill(`HC-22 ${viewportName} cutover verification`);
  await page.getByRole('button', { name: '提交反馈' }).click();
  await page.getByText('反馈提交成功，感谢你的建议！').waitFor();

  await page.goto(new URL('/ranking', origin).href, { waitUntil: 'networkidle' });
  await page.getByRole('heading', { name: '排行榜' }).waitFor();
  await assertPublicCopy();
  await page.goto(new URL('/favorites', origin).href, { waitUntil: 'networkidle' });
  await assertPublicCopy();
  for (const [path, expectedPath] of [['/extract', '/practice'], ['/wheel', '/practice'], ['/workshop/feedback/retired', '/feedback']]) {
    await page.goto(new URL(path, origin).href, { waitUntil: 'networkidle' });
    if (new URL(page.url()).pathname !== expectedPath) throw new Error(`${path} did not converge on ${expectedPath}`);
    await assertPublicCopy();
  }
  if (failures.length) throw new Error(`browser failures: ${failures.join(', ')}`);
} finally {
  await browser.close();
}
