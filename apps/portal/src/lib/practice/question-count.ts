/** Question-count bounds accepted by the Portal-to-QuizCraft session contract. */
export const MIN_QUESTION_COUNT = 1;
export const MAX_QUESTION_COUNT = 500;

/** True when a committed setup count can cross the command boundary unchanged. */
export function isValidQuestionCount(count: number): boolean {
  return Number.isInteger(count) && count >= MIN_QUESTION_COUNT && count <= MAX_QUESTION_COUNT;
}
