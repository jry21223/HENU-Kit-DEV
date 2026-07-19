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

test('favorites use the generated server client and never merge guest browser state', async ({ page }) => {
  let favoriteWrites = 0;
  const favoriteMethods: string[] = [];
  let authenticated = false;
  await page.context().addCookies([{ name: 'quizcraft_anonymous', value: 'server-issued-anonymous-session', domain: '127.0.0.1', path: '/', httpOnly: true, secure: false, sameSite: 'Lax' }]);
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    if (request.url().endsWith('/api/v1/banks')) {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'banks', data: [{ bank_id: bankId, bank_version_id: bankVersionId, bank_key: 'browser-bank', name: '浏览器影子题库', content_sha256: 'a'.repeat(64), question_count: 1, chapters: [{ id: 'ch01', name: '第一章' }] }] }) });
      return;
    }
    if (request.url().endsWith('/api/v1/practice/sessions')) {
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ request_id: 'session', data: { session_id: sessionId, bank_id: bankId, bank_version_id: bankVersionId, mode: 'random', excluded_unavailable_count: 0, questions: [{ question_id: questionId, question_version_id: questionVersionId, type: 'single', chapter_id: 'ch01', chapter: '第一章', content: '需要登录收藏的问题', options: ['是', '否'] }] } }) });
      return;
    }
    if (request.url().endsWith(`/api/v1/banks/${bankId}/favorites`)) {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'list', data: [] }) });
      return;
    }
    if (request.url().endsWith(`/api/v1/banks/${bankId}/favorites/${questionId}`)) {
      favoriteWrites += 1;
      favoriteMethods.push(request.method());
      if (!authenticated) {
        await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ request_id: 'auth', error: { code: 'authentication_required', message: 'sign in' } }) });
        return;
      }
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'favorite', data: { operation_id: questionId, state: 'succeeded', idempotency_key: request.headers()['idempotency-key'], request_id: 'favorite', resource_id: questionId } }) });
      return;
    }
    if (request.url().endsWith(`/api/v1/practice/sessions/${sessionId}/answers`)) {
      expect((await request.allHeaders()).cookie).toContain('quizcraft_anonymous=server-issued-anonymous-session');
      expect((await request.allHeaders()).cookie).toContain('quizcraft_session=signed-in');
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'answer', data: { question_id: questionId, question_version_id: questionVersionId, correct: true, replayed: false, expected_answer: 1, analysis: '登录回跳后仍可作答' } }) });
      return;
    }
    await route.abort();
  });
  await page.route('http://127.0.0.1:18080/auth/login**', async (route) => {
    const returnTo = new URL(route.request().url()).searchParams.get('return_to');
    expect(returnTo).toContain('/quiz?favorite_question_id=');
    authenticated = true;
    await route.fulfill({ status: 302, headers: { location: `http://127.0.0.1:4173${returnTo}`, 'set-cookie': 'quizcraft_session=signed-in; Path=/; HttpOnly; SameSite=Lax' } });
  });

  await page.goto('/practice');
  await page.getByRole('button', { name: '开始练习' }).click();
  await page.getByRole('button', { name: '收藏本题' }).click();
  await expect(page).toHaveURL(/\/quiz$/);
  await expect(page.getByRole('button', { name: '取消收藏本题' })).toBeVisible();
  await page.getByRole('button', { name: /B.*否/ }).click();
  await page.getByRole('button', { name: '提交答案' }).click();
  await expect(page.getByText('登录回跳后仍可作答')).toBeVisible();

  authenticated = false;
  await page.getByRole('button', { name: '取消收藏本题' }).click();
  await expect(page).toHaveURL(/\/quiz$/);
  await expect.poll(() => favoriteWrites).toBe(4);
  await expect(page.getByRole('button', { name: '收藏本题' })).toBeVisible();
  expect(favoriteMethods).toEqual(['PUT', 'PUT', 'DELETE', 'DELETE']);
  const persistedStars = await page.evaluate(() => JSON.parse(localStorage.getItem('quiz-storage') || '{}')?.state?.starredQuestions || []);
  expect(persistedStars).toEqual([]);
});

test('favorites overview starts a bank-scoped favorites practice', async ({ page }) => {
  const calls: string[] = [];
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    calls.push(`${request.method()} ${new URL(request.url()).pathname}`);
    if (request.url().endsWith('/api/v1/banks')) {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'banks', data: [{ bank_id: bankId, bank_version_id: bankVersionId, bank_key: 'browser-bank', name: '浏览器影子题库', content_sha256: 'a'.repeat(64), question_count: 1, chapters: [] }] }) });
      return;
    }
    if (request.url().endsWith('/api/v1/favorites')) {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'folders', data: [{ bank_id: bankId, bank_name: '浏览器影子题库', available_count: 1, unavailable_count: 2 }] }) });
      return;
    }
    if (request.url().endsWith(`/api/v1/banks/${bankId}/favorites/practice-sessions`)) {
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ request_id: 'favorites-session', data: { session_id: sessionId, bank_id: bankId, bank_version_id: bankVersionId, mode: 'favorites', excluded_unavailable_count: 2, questions: [{ question_id: questionId, question_version_id: questionVersionId, type: 'single', chapter_id: 'ch01', chapter: '第一章', content: '只来自当前题库', options: ['是', '否'] }] } }) });
      return;
    }
    if (request.url().endsWith(`/api/v1/banks/${bankId}/favorites`)) {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'list', data: [{ bank_id: bankId, question_id: questionId, available: true, question_version_id: questionVersionId }] }) });
      return;
    }
    await route.abort();
  });
  await page.goto('/favorites');
  await expect(page.getByText('可练习 1 题')).toBeVisible();
  await expect(page.getByText('另有 2 题暂不可用')).toBeVisible();
  await page.getByRole('button', { name: '练习这个题库' }).click();
  await expect(page).toHaveURL(/\/quiz$/);
  await expect(page.getByRole('heading', { name: '只来自当前题库' })).toBeVisible();
  expect(calls).toContain(`POST /api/v1/banks/${bankId}/favorites/practice-sessions`);
});

test('ranking defaults to overall weekly, exposes four views, and supports opt out', async ({ page }) => {
  const rankingCalls: string[] = [];
  let profileWrites = 0;
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname === '/api/v1/banks') {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'banks', data: [{ bank_id: bankId, bank_version_id: bankVersionId, bank_key: 'browser-bank', name: '浏览器影子题库', content_sha256: 'a'.repeat(64), question_count: 1, chapters: [] }] }) });
      return;
    }
    if (url.pathname.includes('/rankings')) {
      rankingCalls.push(`${url.pathname}?${url.searchParams.toString()}`);
      const bankScope = url.pathname.includes(`/banks/${bankId}/`);
      const responsePeriod = url.searchParams.get('period') || 'weekly';
      if (!bankScope && responsePeriod === 'lifetime') await new Promise((resolve) => setTimeout(resolve, 300));
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'ranking', data: { scope: bankScope ? 'bank' : 'overall', ...(bankScope ? { bank_id: bankId } : {}), period: responsePeriod, metric: 'correct_answer_count', entries: [{ rank: 1, nickname: `${bankScope ? 'bank' : 'overall'}-${responsePeriod}`, system_avatar: 'scholar-blue', correct_answer_count: 7 }] } }) });
      return;
    }
    if (url.pathname === '/api/v1/ranking-profile' && request.method() === 'PATCH') {
      profileWrites += 1;
      expect(request.postDataJSON()).toEqual({ visible: false, nickname: '公开昵称', system_avatar: 'scholar-blue' });
      await new Promise((resolve) => setTimeout(resolve, 300));
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'profile', data: { operation_id: questionId, state: 'succeeded', idempotency_key: request.headers()['idempotency-key'], request_id: 'profile' } }) });
      return;
    }
    await route.abort();
  });
  await page.goto('/ranking');
  await expect(page.getByText('overall-weekly')).toBeVisible();
  expect(rankingCalls[0]).toBe('/api/v1/rankings/overall?period=weekly');
  await page.getByRole('button', { name: '历史' }).click();
  await expect.poll(() => rankingCalls.some((call) => call === '/api/v1/rankings/overall?period=lifetime')).toBe(true);
  await page.getByRole('button', { name: '按题库' }).click();
  await expect.poll(() => rankingCalls.some((call) => call === `/api/v1/banks/${bankId}/rankings?period=lifetime`)).toBe(true);
  await expect(page.getByText('bank-lifetime')).toBeVisible();
  await page.waitForTimeout(350);
  await expect(page.getByText('bank-lifetime')).toBeVisible();
  await page.getByRole('button', { name: '本周' }).click();
  await expect.poll(() => rankingCalls.some((call) => call === `/api/v1/banks/${bankId}/rankings?period=weekly`)).toBe(true);
  await page.getByLabel('公开昵称').fill('公开昵称');
  await page.getByLabel('参与公开排行').uncheck();
  await page.getByRole('button', { name: '保存公开资料' }).click();
  await expect(page.getByRole('button', { name: '保存中…' })).toBeDisabled();
  await page.getByRole('button', { name: '保存中…' }).click({ force: true });
  await page.getByRole('button', { name: '全站' }).click();
  await expect(page.getByText('overall-weekly')).toBeVisible();
  await expect(page.getByText('已退出公开排行')).toBeVisible();
  await expect(page.getByText('overall-weekly')).toBeVisible();
  expect(profileWrites).toBe(1);
});
