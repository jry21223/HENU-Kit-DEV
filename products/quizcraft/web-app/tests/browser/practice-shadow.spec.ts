import { expect, test } from 'playwright/test';

const bankId = '11111111-1111-5111-8111-111111111111';
const bankVersionId = '22222222-2222-5222-8222-222222222222';
const questionId = '33333333-3333-5333-8333-333333333333';
const questionVersionId = '44444444-4444-5444-8444-444444444444';
const sessionId = '55555555-5555-4555-8555-555555555555';

test('React uses the generated Practice client for a guest session', async ({ page }) => {
  const calls: string[] = [];
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    calls.push(`${request.method()} ${new URL(request.url()).pathname}`);
    expect(request.headers().authorization).toBeUndefined();
    if (request.method() === 'GET' && request.url().endsWith('/api/v1/banks')) {
      expect((await request.allHeaders()).cookie).toBeUndefined();
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
      expect((await request.allHeaders()).cookie).toBeUndefined();
      expect(request.postDataJSON()).toMatchObject({
        bank_id: bankId,
        bank_version_id: bankVersionId,
        mode: 'random',
      });
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        headers: { 'set-cookie': 'quizcraft_anonymous=server-issued-anonymous-session; Path=/; HttpOnly; SameSite=Lax' },
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
      expect((await request.allHeaders()).cookie).toContain('quizcraft_anonymous=server-issued-anonymous-session');
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
    if (request.method() === 'POST' && request.url().endsWith('/api/v1/feedback')) {
      expect(request.headers()['idempotency-key']).toBeTruthy();
      expect(request.postDataJSON()).toEqual({ bank_id: bankId, question_id: questionId, question_version_id: questionVersionId, category: 'other', detail: '解析需要补充边界条件' });
      await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ request_id: 'req_feedback', data: { operation_id: questionId, state: 'succeeded', idempotency_key: request.headers()['idempotency-key'], request_id: 'req_feedback', resource_id: sessionId } }) });
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
  await page.getByRole('button', { name: '反馈本题' }).click();
  await page.getByLabel('反馈建议').fill('解析需要补充边界条件');
  await page.getByRole('button', { name: '提交反馈' }).click();
  await expect(page.getByText('反馈提交成功，感谢你的建议！')).toBeVisible();
  expect(calls.filter((call) => call.startsWith('POST '))).toEqual([
    'POST /api/v1/practice/sessions',
    `POST /api/v1/practice/sessions/${sessionId}/answers`,
    'POST /api/v1/feedback',
  ]);
  expect(calls.filter((call) => call === 'GET /api/v1/banks').length).toBeGreaterThanOrEqual(1);
  expect(calls.every((call) => call.includes('/api/v1/'))).toBe(true);
});

test('answer retry keeps its idempotency key after a browser refresh', async ({ page }) => {
  const answerKeys: string[] = [];
  let answerRequests = 0;
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    if (request.method() === 'GET' && request.url().endsWith('/api/v1/banks')) {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_retry_banks',
          data: [{
            bank_id: bankId,
            bank_version_id: bankVersionId,
            bank_key: 'browser-bank',
            name: '浏览器重试题库',
            content_sha256: 'b'.repeat(64),
            question_count: 1,
            chapters: [{ id: 'ch01', name: '第一章' }],
          }],
        }),
      });
      return;
    }
    if (request.method() === 'POST' && request.url().endsWith('/api/v1/practice/sessions')) {
      await route.fulfill({
        status: 201,
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_retry_session',
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
              content: '刷新后仍需使用同一个重试键',
              options: ['1', '2'],
            }],
          },
        }),
      });
      return;
    }
    if (request.method() === 'POST' && request.url().endsWith(`/api/v1/practice/sessions/${sessionId}/answers`)) {
      answerRequests += 1;
      answerKeys.push(request.headers()['idempotency-key']);
      if (answerRequests === 1) {
        await route.fulfill({
          status: 503,
          contentType: 'application/json',
          body: JSON.stringify({ request_id: 'req_retry_failed', error: { code: 'database_unavailable', message: 'unavailable' } }),
        });
        return;
      }
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_retry_answer',
          data: {
            question_id: questionId,
            question_version_id: questionVersionId,
            correct: true,
            replayed: false,
            expected_answer: 1,
            analysis: '重试由服务端确认',
          },
        }),
      });
      return;
    }
    await route.abort();
  });

  await page.goto('/practice');
  await page.getByRole('button', { name: '开始练习' }).click();
  await expect(page.getByRole('heading', { name: '刷新后仍需使用同一个重试键' })).toBeVisible();
  await page.getByRole('button', { name: /B.*2/ }).click();
  await page.getByRole('button', { name: '提交答案' }).click();
  await expect(page.getByRole('alert')).toBeVisible();

  await page.reload();
  await expect(page.getByRole('heading', { name: '刷新后仍需使用同一个重试键' })).toBeVisible();
  await page.getByRole('button', { name: /B.*2/ }).click();
  await page.getByRole('button', { name: '提交答案' }).click();
  await expect(page.getByText('重试由服务端确认')).toBeVisible();
  expect(answerKeys).toHaveLength(2);
  expect(answerKeys[1]).toBe(answerKeys[0]);
});

test('Workshop uses scoped generated APIs and requires human validation before publish', async ({ page }) => {
  let lifecycleVersion = 1;
  let state: 'none' | 'draft' | 'validated' = 'none';
  let active = false;
  const versionId = '66666666-6666-4666-8666-666666666666';
  const writes: string[] = [];
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === 'GET' && url.pathname === '/api/v1/workshop/catalog') {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'req_workshop', data: [{ bank_id: bankId, bank_key: 'browser-bank', name: '浏览器题库', lifecycle_version: lifecycleVersion, ...(active ? { active_version_id: versionId } : {}), versions: state === 'none' ? [] : [{ bank_version_id: versionId, content_sha256: 'b'.repeat(64), question_count: 1, state, active }] }] }) });
      return;
    }
    if (request.method() === 'GET' && url.pathname === `/api/v1/workshop/banks/${bankId}/versions/${versionId}`) {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'req_detail', data: { bank_id: bankId, bank_version_id: versionId, state: 'draft', content_sha256: 'b'.repeat(64), questions: [{ question_id: questionId, question_version_id: versionId, source_question_id: 'q0001', type: 'single', chapter_id: 'ch01', chapter: '第一章', content: '1 + 1 = ?', options: ['1', '2'], answer: 1, analysis: '2', position: 1 }] } }) });
      return;
    }
    if (request.method() === 'POST') {
      expect(request.headers()['idempotency-key']).toBeTruthy();
      writes.push(url.pathname);
      if (url.pathname.endsWith('/versions')) {
        expect(request.postDataJSON().expected_version).toBe(1);
        state = 'draft'; lifecycleVersion = 2;
      } else if (url.pathname.endsWith('/validate')) {
        expect(request.postDataJSON().expected_version).toBe(2);
        state = 'validated'; lifecycleVersion = 3;
      } else if (url.pathname.endsWith('/publish')) {
        expect(request.postDataJSON().expected_version).toBe(3);
        active = true; lifecycleVersion = 4;
      }
      await route.fulfill({ status: url.pathname.endsWith('/versions') ? 201 : 200, contentType: 'application/json', body: JSON.stringify({ request_id: 'req_write', data: { operation_id: questionId, state: 'succeeded', idempotency_key: request.headers()['idempotency-key'], request_id: 'req_write', resource_id: versionId } }) });
      return;
    }
    await route.abort();
  });
  await page.goto('/extract');
  await expect(page.getByRole('heading', { name: '题库工坊' })).toBeVisible();
  await expect(page.getByText(/ADMIN_TOKEN/)).toHaveCount(0);
  await page.getByRole('button', { name: '创建草稿版本' }).click();
  await expect(page.getByText('draft', { exact: false })).toBeVisible();
  await expect(page.getByRole('button', { name: '发布' })).toHaveCount(0);
  await page.getByRole('button', { name: '查看并校验题目' }).click();
  await expect(page.getByText('1. 1 + 1 = ?', { exact: true })).toBeVisible();
  await expect(page.locator('ol li')).toHaveText(['1', '2']);
  await page.getByRole('button', { name: '人工校验通过' }).click();
  await page.getByRole('button', { name: '发布' }).click();
  await expect(page.getByRole('status').getByText('已发布校验版本')).toBeVisible();
  expect(writes).toEqual([
    `/api/v1/workshop/banks/${bankId}/versions`,
    `/api/v1/workshop/banks/${bankId}/versions/${versionId}/validate`,
    `/api/v1/workshop/banks/${bankId}/versions/${versionId}/publish`,
  ]);
});

test('Operations Inbox deep link reads full feedback only from QuizCraft', async ({ page }) => {
  const feedbackId = '77777777-7777-4777-8777-777777777777';
  await page.route(`http://127.0.0.1:18080/api/v1/workshop/feedback/${feedbackId}`, async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'req_feedback_detail', data: { feedback_id: feedbackId, bank_id: bankId, question_id: questionId, question_version_id: '66666666-6666-4666-8666-666666666666', category: 'wrong_answer', detail: '正确答案与解析矛盾', created_at: '2026-07-20T00:00:00Z' } }) });
  });
  await page.goto(`/workshop/feedback/${feedbackId}`);
  await expect(page.getByRole('heading', { name: 'QuizCraft 纠错反馈' })).toBeVisible();
  await expect(page.getByText('正确答案与解析矛盾')).toBeVisible();
  await expect(page.getByText('正文仅从 QuizCraft 读取')).toBeVisible();
});

test('Workshop deep link offers Platform Core login and preserves return path on 401', async ({ page }) => {
  await page.route('http://127.0.0.1:18080/api/v1/workshop/catalog', (route) => route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ request_id: 'req_login', error: { code: 'platform_session_required', message: 'sign in' } }) }));
  await page.goto('/extract');
  const login = page.getByRole('link', { name: '通过 Platform Core 登录并返回工坊' });
  await expect(login).toBeVisible();
  await expect(login).toHaveAttribute('href', 'http://127.0.0.1:18080/auth/login?return_to=%2Fextract');
});

test('Workshop offers login when a detail read or mutation loses its session', async ({ page }) => {
  const versionId = '66666666-6666-4666-8666-666666666666';
  await page.route('http://127.0.0.1:18080/api/v1/workshop/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === '/api/v1/workshop/catalog') {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'req_catalog', data: [{ bank_id: bankId, bank_key: 'session-bank', name: 'Session Bank', lifecycle_version: 1, versions: [{ bank_version_id: versionId, content_sha256: 'd'.repeat(64), question_count: 1, state: 'draft', active: false }] }] }) });
      return;
    }
    await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ request_id: 'req_expired', error: { code: 'invalid_session', message: 'expired' } }) });
  });
  await page.goto('/extract');
  await page.getByRole('button', { name: '查看并校验题目' }).click();
  await expect(page.getByRole('link', { name: '通过 Platform Core 登录并返回工坊' })).toBeVisible();
  await page.reload();
  await page.getByRole('button', { name: '创建草稿版本' }).click();
  await expect(page.getByRole('link', { name: '通过 Platform Core 登录并返回工坊' })).toBeVisible();
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
