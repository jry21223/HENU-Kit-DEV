export type ApiErrorKind = 'http' | 'timeout' | 'network' | 'unknown';

type RequestErrorLike = {
  code?: string;
  request?: unknown;
  response?: unknown;
};

export const classifyApiErrorKind = (error: RequestErrorLike): ApiErrorKind => {
  if (error.code === 'ECONNABORTED' || error.code === 'ETIMEDOUT') {
    return 'timeout';
  }
  if (error.response) return 'http';
  if (error.request) return 'network';
  return 'unknown';
};

export class ApiRequestError extends Error {
  readonly kind: ApiErrorKind;
  readonly status?: number;

  constructor(kind: ApiErrorKind, message: string, status?: number) {
    super(message);
    this.name = 'ApiRequestError';
    this.kind = kind;
    this.status = status;
  }
}

export const isApiRequestError = (error: unknown): error is ApiRequestError =>
  error instanceof ApiRequestError;
