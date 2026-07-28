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
  const message = error instanceof Error ? error.message.trim() : '';
  return message && message !== 'Failed to fetch' && !(error instanceof ApiError)
    ? message
    : fallback;
};
