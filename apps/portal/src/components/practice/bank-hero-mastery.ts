/** 0–100 subject mastery row from the Portal practice-stats contract. */
export type MasterySubject = {
  label: string;
  value: number;
};

/** Snapshot that drives the knowledge mesh. */
export type MasterySnapshot = {
  subjects: MasterySubject[];
  /** Overall accuracy 0–100 → nucleus size / brightness */
  accuracy: number;
  /** Streak days → orbiting cube count */
  streakDays: number;
  /** Total answered → orbit radius slightly */
  totalQuestions: number;
};

export type MasteryVisuals = {
  ringSubjects: MasterySubject[];
  coverage: number;
  cubeCount: number;
  orbitRadius: number;
};

export const EMPTY_MASTERY: MasterySnapshot = {
  subjects: [],
  accuracy: 0,
  streakDays: 0,
  totalQuestions: 0,
};

export function clamp01(value: number) {
  return Math.min(1, Math.max(0, value));
}

export function masteryPercent(value: number) {
  return clamp01(value / 100);
}

export function averageMastery(subjects: MasterySubject[]) {
  if (subjects.length === 0) return 0;
  return (
    subjects.reduce((total, subject) => total + masteryPercent(subject.value), 0) /
    subjects.length
  );
}

export function deriveMasteryVisuals(
  mastery: MasterySnapshot
): MasteryVisuals {
  return {
    ringSubjects: mastery.subjects.slice(0, 3).map((subject) => ({
      ...subject,
      value: clamp01(subject.value / 100) * 100,
    })),
    coverage: averageMastery(mastery.subjects),
    cubeCount: Math.min(
      4,
      Math.max(
        0,
        Math.floor(mastery.streakDays / 10) +
          (mastery.streakDays > 0 ? 1 : 0)
      )
    ),
    orbitRadius:
      2.2 + Math.min(0.35, Math.max(0, mastery.totalQuestions) / 2500),
  };
}
