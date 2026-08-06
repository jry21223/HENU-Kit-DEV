/** Question-count bounds for composing one practice session. */
export const MIN_QUESTION_COUNT = 1;
export const MAX_QUESTION_COUNT = 500;

/** True for an integer question count within the 1..500 session bounds. */
export function isValidQuestionCount(count: number): boolean {
  return (
    Number.isInteger(count) &&
    count >= MIN_QUESTION_COUNT &&
    count <= MAX_QUESTION_COUNT
  );
}
