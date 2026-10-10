/**
 * How one answer option reads after the server has scored the question.
 *
 * - correct: the learner picked it and it is part of the answer.
 * - wrong:   the learner picked it but it is not part of the answer.
 * - missed:  the learner did not pick it but it is part of the answer.
 * - neutral: not picked, not part of the answer.
 *
 * Whether the whole question counts as right is never derived here: it is the
 * server's `correct` verdict, so a partly right multi-choice pick stays wrong.
 */
export type OptionFeedbackState = "correct" | "wrong" | "missed" | "neutral";

function stateFor(selected: boolean, expected: boolean): OptionFeedbackState {
  if (expected) return selected ? "correct" : "missed";
  return selected ? "wrong" : "neutral";
}

/** True when the server's expected answer names this option by index or text. */
export function optionIsExpected(expectedAnswer: unknown, option: string, index: number): boolean {
  if (Array.isArray(expectedAnswer)) return expectedAnswer.some((item) => optionIsExpected(item, option, index));
  return expectedAnswer === index || expectedAnswer === option;
}

export function choiceOptionState(input: {
  selected: boolean;
  expectedAnswer: unknown;
  option: string;
  index: number;
}): OptionFeedbackState {
  return stateFor(input.selected, optionIsExpected(input.expectedAnswer, input.option, input.index));
}

export function judgeOptionState(input: {
  selected: boolean;
  expectedAnswer: unknown;
  value: boolean;
}): OptionFeedbackState {
  return stateFor(input.selected, input.expectedAnswer === input.value);
}
