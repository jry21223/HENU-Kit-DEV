import { expect, test } from 'playwright/test';

const bankId = '11111111-1111-5111-8111-111111111111';
const bankVersionId = '22222222-2222-5222-8222-222222222222';
const questionId = '33333333-3333-5333-8333-333333333333';
const questionVersionId = '44444444-4444-5444-8444-444444444444';
const sessionId = '55555555-5555-4555-8555-555555555555';
const feedbackId = '66666666-6666-4666-8666-666666666666';

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
      expect(request.postDataJSON()).toEqual({ bank_id: bankId, question_id: questionId, question_version_id: questionVersionId, category: 'wrong_answer', detail: '解析需要补充边界条件' });
      await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ request_id: 'req_feedback', data: { operation_id: questionId, state: 'succeeded', idempotency_key: request.headers()['idempotency-key'], request_id: 'req_feedback', resource_id: feedbackId } }) });
      return;
    }
    if (request.method() === 'GET' && request.url().endsWith(`/api/v1/feedback/${feedbackId}/status`)) {
      expect((await request.allHeaders()).cookie).toContain('quizcraft_anonymous=server-issued-anonymous-session');
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'req_feedback_status', data: { feedback_id: feedbackId, bank_id: bankId, question_id: questionId, question_version_id: questionVersionId, category: 'wrong_answer', status: 'pending', created_at: '2026-07-28T00:00:00Z', updated_at: '2026-07-28T00:00:00Z' } }) });
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
  await page.getByLabel('反馈类型').selectOption('wrong_answer');
  await page.getByLabel('反馈建议').fill('解析需要补充边界条件');
  await page.getByRole('button', { name: '提交反馈' }).click();
  await expect(page.getByText('反馈已保存，当前状态：已受理。')).toBeVisible();
  expect(calls.filter((call) => call.startsWith('POST '))).toEqual([
    'POST /api/v1/practice/sessions',
    `POST /api/v1/practice/sessions/${sessionId}/answers`,
    'POST /api/v1/feedback',
  ]);
  expect(calls).toContain(`GET /api/v1/feedback/${feedbackId}/status`);
  expect(calls.filter((call) => call === 'GET /api/v1/banks').length).toBeGreaterThanOrEqual(1);
  expect(calls.every((call) => call.includes('/api/v1/'))).toBe(true);
});

test('feedback retry keeps its idempotency key after a transient write failure and browser refresh', async ({ page }) => {
  const feedbackKeys: string[] = [];
  let feedbackRequests = 0;
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    if (request.method() === 'GET' && request.url().endsWith('/api/v1/banks')) {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_feedback_retry_banks',
          data: [{
            bank_id: bankId,
            bank_version_id: bankVersionId,
            bank_key: 'browser-bank',
            name: '反馈重试题库',
            content_sha256: 'c'.repeat(64),
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
        headers: { 'set-cookie': 'quizcraft_anonymous=feedback-retry-session; Path=/; HttpOnly; SameSite=Lax' },
        body: JSON.stringify({
          request_id: 'req_feedback_retry_session',
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
              content: '反馈失败后重试必须保持同一幂等键',
              options: ['1', '2'],
            }],
          },
        }),
      });
      return;
    }
    if (request.method() === 'POST' && request.url().endsWith('/api/v1/feedback')) {
      feedbackRequests += 1;
      feedbackKeys.push(request.headers()['idempotency-key']);
      expect((await request.allHeaders()).cookie).toContain('quizcraft_anonymous=feedback-retry-session');
      if (feedbackRequests === 1) {
        await route.fulfill({
          status: 503,
          contentType: 'application/json',
          body: JSON.stringify({ request_id: 'req_feedback_retry_failed', error: { code: 'database_unavailable', message: 'unavailable' } }),
        });
        return;
      }
      await route.fulfill({
        status: 202,
        contentType: 'application/json',
        body: JSON.stringify({ request_id: 'req_feedback_retry_accepted', data: { operation_id: questionId, state: 'succeeded', idempotency_key: request.headers()['idempotency-key'], request_id: 'req_feedback_retry_accepted', resource_id: feedbackId } }),
      });
      return;
    }
    if (request.method() === 'GET' && request.url().endsWith(`/api/v1/feedback/${feedbackId}/status`)) {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ request_id: 'req_feedback_retry_status', data: { feedback_id: feedbackId, bank_id: bankId, question_id: questionId, question_version_id: questionVersionId, category: 'other', status: 'pending', created_at: '2026-07-28T00:00:00Z', updated_at: '2026-07-28T00:00:00Z' } }),
      });
      return;
    }
    await route.abort();
  });

  await page.goto('/practice');
  await page.getByRole('button', { name: '开始练习' }).click();
  await expect(page.getByRole('heading', { name: '反馈失败后重试必须保持同一幂等键' })).toBeVisible();
  await page.getByRole('button', { name: '反馈本题' }).click();
  await page.getByLabel('反馈建议').fill('第一次请求暂时失败后必须安全重试');
  await page.getByRole('button', { name: '提交反馈' }).click();
  await expect(page.getByRole('alert')).toHaveText('服务暂时不可用，请稍后再来');

  await page.reload();
  await expect(page.getByRole('heading', { name: '反馈失败后重试必须保持同一幂等键' })).toBeVisible();
  await page.getByRole('button', { name: '反馈本题' }).click();
  await expect(page.getByLabel('反馈建议')).toHaveValue('第一次请求暂时失败后必须安全重试');
  await page.getByRole('button', { name: '提交反馈' }).click();
  await expect(page.getByText('反馈已保存，当前状态：已受理。')).toBeVisible();
  expect(feedbackKeys).toHaveLength(2);
  expect(feedbackKeys[0]).toBeTruthy();
  expect(feedbackKeys[1]).toBe(feedbackKeys[0]);
});

test('saved feedback remains recoverable when a status refresh temporarily fails', async ({ page }) => {
  let statusReads = 0;
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === 'GET' && path === '/api/v1/feedback') {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_feedback_history',
          data: {
            items: [{
              feedback_id: feedbackId,
              bank_id: bankId,
              question_id: questionId,
              question_version_id: questionVersionId,
              category: 'typo',
              status: 'pending',
              created_at: '2026-07-28T00:00:00Z',
              updated_at: '2026-07-28T00:00:00Z',
            }],
          },
        }),
      });
      return;
    }
    if (request.method() === 'GET' && path === `/api/v1/feedback/${feedbackId}/status`) {
      statusReads += 1;
      if (statusReads === 1) {
        await route.fulfill({
          status: 503,
          contentType: 'application/json',
          body: JSON.stringify({ request_id: 'req_feedback_status_unavailable', error: { code: 'feedback_status_unavailable', message: 'unavailable' } }),
        });
        return;
      }
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          request_id: 'req_feedback_status_resolved',
          data: {
            feedback_id: feedbackId,
            bank_id: bankId,
            question_id: questionId,
            question_version_id: questionVersionId,
            category: 'typo',
            status: 'resolved',
            created_at: '2026-07-28T00:00:00Z',
            updated_at: '2026-07-28T00:05:00Z',
          },
        }),
      });
      return;
    }
    await route.abort();
  });

  await page.goto('/feedback');
  await expect(page.getByRole('heading', { name: '我的反馈' })).toBeVisible();
  await expect(page.getByText(feedbackId)).toBeVisible();
  await expect(page.getByText('已受理')).toBeVisible();

  const refresh = page.getByRole('button', { name: `刷新反馈 ${feedbackId}` });
  await refresh.click();
  await expect(page.getByRole('alert')).toHaveText('服务暂时不可用，请稍后再来');
  await expect(page.getByText(feedbackId)).toBeVisible();
  await refresh.click();
  await expect(page.getByText('已解决')).toBeVisible();
  expect(statusReads).toBe(2);
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

test('retired standalone routes converge on the practice surface without legacy copy', async ({ page }) => {
  await page.route('http://127.0.0.1:18080/api/v1/banks', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'req_retired_routes', data: [] }) });
  });

  for (const [path, target] of [
    ['/', '/practice'],
    ['/extract', '/practice'],
    ['/wheel', '/practice'],
    [`/workshop/feedback/${feedbackId}`, '/feedback'],
  ]) {
    await page.goto(path);
    await expect(page).toHaveURL(new RegExp(`${target}$`));
    await expect(page.getByRole('link', { name: '刷题', exact: true })).toBeVisible();
    for (const retiredCopy of ['QuizCraft', '题库工坊', '随机大转盘', '管理令牌', '开源项目，可自行部署']) {
      await expect(page.locator('body')).not.toContainText(retiredCopy);
    }
    await expect(page.locator('a[href="/extract"], a[href="/wheel"], a[href^="/workshop"]')).toHaveCount(0);
  }
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

test('favorite authentication failure hands off to HENU Kit without restoring standalone OAuth', async ({ page }) => {
  let favoriteWrites = 0;
  let retiredLoginRequests = 0;
  await page.context().addCookies([{ name: 'quizcraft_anonymous', value: 'server-issued-anonymous-session', domain: '127.0.0.1', path: '/', httpOnly: true, secure: false, sameSite: 'Lax' }]);
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/auth/login') retiredLoginRequests += 1;
  });
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
      await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ request_id: 'auth', error: { code: 'authentication_required', message: 'sign in' } }) });
      return;
    }
    await route.abort();
  });
  await page.route('https://henukit.cn/practice/favorites', async (route) => {
    await route.fulfill({ contentType: 'text/html', body: '<main>练习服务收藏夹</main>' });
  });

  await page.goto('/practice');
  await page.getByRole('button', { name: '开始练习' }).click();
  await page.getByRole('button', { name: '收藏本题' }).click();
  await expect(page).toHaveURL('https://henukit.cn/practice/favorites');
  expect(favoriteWrites).toBe(1);
  expect(retiredLoginRequests).toBe(0);
  const persistedStars = await page.evaluate(() => JSON.parse(localStorage.getItem('quiz-storage') || '{}')?.state?.starredQuestions || []);
  expect(persistedStars).toEqual([]);
});

test('favorite write failure stays visible and never falls back to browser storage', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (url.pathname === '/api/v1/banks') {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'banks', data: [{ bank_id: bankId, bank_version_id: bankVersionId, bank_key: 'browser-bank', name: '浏览器影子题库', content_sha256: 'a'.repeat(64), question_count: 1, chapters: [{ id: 'ch01', name: '第一章' }] }] }) });
      return;
    }
    if (url.pathname === '/api/v1/practice/sessions') {
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ request_id: 'session', data: { session_id: sessionId, bank_id: bankId, bank_version_id: bankVersionId, mode: 'random', excluded_unavailable_count: 0, questions: [{ question_id: questionId, question_version_id: questionVersionId, type: 'single', chapter_id: 'ch01', chapter: '第一章', content: '失败后不能伪造收藏', options: ['是', '否'] }] } }) });
      return;
    }
    if (url.pathname === `/api/v1/banks/${bankId}/favorites`) {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'favorites', data: [] }) });
      return;
    }
    if (url.pathname === `/api/v1/banks/${bankId}/favorites/${questionId}`) {
      expect(request.method()).toBe('PUT');
      await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ request_id: 'unavailable', error: { code: 'database_unavailable', message: 'unavailable' } }) });
      return;
    }
    await route.abort();
  });

  await page.goto('/practice');
  await page.getByRole('button', { name: '开始练习' }).click();
  await expect(page.getByRole('heading', { name: '失败后不能伪造收藏' })).toBeVisible();
  await page.getByRole('button', { name: '收藏本题' }).click();
  await expect(page.getByRole('status')).toHaveText('收藏失败，请稍后重试');
  await expect(page.getByRole('button', { name: '收藏本题' })).toBeVisible();
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

test('favorites overview distinguishes an unavailable service from a login requirement', async ({ page }) => {
  let favoritesStatus = 503;
  await page.route('http://127.0.0.1:18080/api/v1/**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/v1/banks') {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ request_id: 'banks', data: [] }) });
      return;
    }
    if (url.pathname === '/api/v1/favorites') {
      await route.fulfill({ status: favoritesStatus, contentType: 'application/json', body: JSON.stringify({ request_id: 'favorites_state', error: { code: favoritesStatus === 401 ? 'authentication_required' : 'database_unavailable', message: 'unavailable' } }) });
      return;
    }
    await route.abort();
  });

  await page.goto('/favorites');
  await expect(page.getByRole('alert')).toHaveText('收藏夹暂时加载不出来，请检查网络后重试');
  await expect(page.getByRole('link', { name: '登录后查看收藏夹' })).toHaveCount(0);

  favoritesStatus = 401;
  await page.goto('/favorites');
  await expect(page.getByRole('alert')).toHaveText('收藏夹请在练习服务中查看');
  await expect(page.getByRole('link', { name: '前往练习服务' })).toHaveAttribute('href', 'https://henukit.cn/practice/favorites');
});
