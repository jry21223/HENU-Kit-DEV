import { isApiRequestError } from '../api/errors';
export { ApiRequestError, classifyApiErrorKind } from '../api/errors';

export type AnswerSubmissionFailureKind =
  | 'http'
  | 'timeout'
  | 'network'
  | 'unknown';

export type AnswerSubmissionFailure = {
  kind: AnswerSubmissionFailureKind;
  title: string;
  message: string;
};

export type AnswerSubmissionResult<T> =
  | { ok: true; response: T }
  | { ok: false; error: AnswerSubmissionFailure };

export const describeAnswerSubmissionFailure = (
  error: unknown,
): AnswerSubmissionFailure => {
  if (isApiRequestError(error)) {
    if (error.kind === 'timeout') {
      return {
        kind: 'timeout',
        title: '提交超时',
        message: '服务器响应超时，你的答案仍已保留，请重试。',
      };
    }

    if (error.kind === 'network') {
      return {
        kind: 'network',
        title: '网络连接失败',
        message: '暂时无法连接服务器，你的答案仍已保留，请检查网络后重试。',
      };
    }

    if (error.kind === 'http') {
      return {
        kind: 'http',
        title: '提交失败',
        message: `${error.message || '服务器拒绝了本次提交'}，你的答案仍已保留。`,
      };
    }
  }

  return {
    kind: 'unknown',
    title: '提交失败',
    message: '发生未知错误，你的答案仍已保留，请重试。',
  };
};

export const runAnswerSubmission = async <T>(
  request: () => Promise<T>,
): Promise<AnswerSubmissionResult<T>> => {
  try {
    return { ok: true, response: await request() };
  } catch (error) {
    return { ok: false, error: describeAnswerSubmissionFailure(error) };
  }
};
