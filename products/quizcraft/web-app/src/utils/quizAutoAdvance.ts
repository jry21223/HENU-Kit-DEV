type AutoAdvanceInput = {
  isCorrect: boolean;
  currentIndex: number;
  questionCount: number;
};

export const shouldAutoAdvanceAfterAnswer = ({
  isCorrect,
  currentIndex,
  questionCount,
}: AutoAdvanceInput) => (
  isCorrect &&
  currentIndex >= 0 &&
  currentIndex < questionCount - 1
);
