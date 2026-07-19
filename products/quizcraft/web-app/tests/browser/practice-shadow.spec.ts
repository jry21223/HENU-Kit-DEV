import { expect, test } from 'playwright/test';

const bankId = '11111111-1111-5111-8111-111111111111';
const bankVersionId = '22222222-2222-5222-8222-222222222222';
const questionId = '33333333-3333-5333-8333-333333333333';
const questionVersionId = '44444444-4444-5444-8444-444444444444';
const sessionId = '55555555-5555-4555-8555-555555555555';

test('React uses the generated Practice client for a guest session', async ({ page }) => {
  const calls: string[] = [];
  await page.addInitScript(() => {
    localStorage.setItem('quizcraft_access_token', 'browser-controlled-token');
  });
  await page.context().addCookies([{
    name: 'quizcraft_session',
    value: 'server-issued-http-only-session',
    domain: '127.0.0.1',
    path: '/',
    httpOnly: true,
    secure: false,
    sameSite: 'Lax',
  }]);
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    calls.push(`${request.method()} ${new URL(request.url()).pathname}`);
    expect(request.headers().authorization).toBeUndefined();
    expect((await request.allHeaders()).cookie).toContain('quizcraft_session=server-issued-http-only-session');
    if (request.method() === 'GET' && request.url().endsWith('/api/v1/banks')) {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_browser_banks',
          data: [{
            bank_id: bankId,
            bank_version_id: bankVersionId,
            bank_key: 'browser-bank',
            name: '浏览器影子题库',
            content_sha256: 'a'.repeat(64),
            question_count: 1,
            chapters: [{ id: 'ch01', name: '第一章' }],
          }],
        }),
      });
      return;
    }
    if (request.method() === 'POST' && request.url().endsWith('/api/v1/practice/sessions')) {
      expect(request.headers()['idempotency-key']).toBeTruthy();
      expect(request.postDataJSON()).toMatchObject({
        bank_id: bankId,
        bank_version_id: bankVersionId,
        mode: 'random',
      });
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_browser_session',
          data: {
            session_id: sessionId,
            bank_id: bankId,
            bank_version_id: bankVersionId,
            mode: 'random',
            excluded_unavailable_count: 0,
            questions: [{
              question_id: questionId,
              question_version_id: questionVersionId,
              type: 'single',
              chapter_id: 'ch01',
              chapter: '第一章',
              content: '浏览器中的 1 + 1 等于？',
              options: ['1', '2'],
            }],
          },
        }),
      });
      return;
    }
    if (request.method() === 'POST' && request.url().endsWith(`/api/v1/practice/sessions/${sessionId}/answers`)) {
      expect(request.headers()['idempotency-key']).toBeTruthy();
      expect(request.postDataJSON()).toEqual({
        question_id: questionId,
        question_version_id: questionVersionId,
        answer: 1,
      });
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_browser_answer',
          data: {
            question_id: questionId,
            question_version_id: questionVersionId,
            correct: true,
            replayed: false,
            expected_answer: 1,
            analysis: '服务端判题',
          },
        }),
      });
      return;
    }
    await route.abort();
  });

  await page.goto('/practice');
  await expect(page.getByText('浏览器影子题库')).toBeVisible();
  await page.getByRole('button', { name: '开始练习' }).click();
  await expect(page).toHaveURL(/\/quiz$/);
  await expect(page.getByRole('heading', { name: '浏览器中的 1 + 1 等于？' })).toBeVisible();
  await page.getByRole('button', { name: /B.*2/ }).click();
  await page.getByRole('button', { name: '提交答案' }).click();
  await expect(page.getByText('服务端判题')).toBeVisible();
  expect(calls.filter((call) => call.startsWith('POST '))).toEqual([
    'POST /api/v1/practice/sessions',
    `POST /api/v1/practice/sessions/${sessionId}/answers`,
  ]);
  expect(calls.filter((call) => call === 'GET /api/v1/banks').length).toBeGreaterThanOrEqual(1);
  expect(calls.every((call) => call.includes('/api/v1/'))).toBe(true);
});

test('shadow bank failure does not fall back to browser-owned mock banks', async ({ page }) => {
  await page.route('http://127.0.0.1:18080/api/v1/banks', async (route) => {
    await route.fulfill({
      status: 503,
      contentType: 'application/json',
      body: JSON.stringify({ request_id: 'req_unavailable', error: { code: 'database_unavailable', message: 'unavailable' } }),
    });
  });
  await page.goto('/practice');
  await expect(page.getByText('Java 题库')).toHaveCount(0);
  await expect(page.getByText('Web 前端')).toHaveCount(0);
  await page.getByRole('button', { name: '开始练习' }).click();
  await expect(page).toHaveURL(/\/practice$/);
});
