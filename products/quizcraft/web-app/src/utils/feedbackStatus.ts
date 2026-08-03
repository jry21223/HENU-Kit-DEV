import { ApiError, type FeedbackStatus } from '@/generated/quizcraft-api';

export const feedbackStatusLabel = (status: FeedbackStatus['status']) => {
  switch (status) {
    case 'pending':
      return '已受理';
    case 'in_progress':
      return '处理中';
    case 'blocked':
      return '暂时受阻';
    case 'resolved':
      return '已解决';
    case 'archived':
      return '已归档';
  }
};

export const feedbackErrorMessage = (error: unknown, fallback: string) => {
  if (error instanceof ApiError) {
    return '服务暂时不可用，请稍后再来';
  }
  const message = error instanceof Error ? error.message.trim() : '';
  if (!message || message === 'Failed to fetch' || message === 'Network Error') {
    return '网络不通，请检查网络后重试';
  }
  return fallback;
};
