const explicitWriteSetting = import.meta.env.VITE_QUIZCRAFT_GO_WRITES;

// The maintenance-window release is atomic: the served bundle either keeps all
// traffic on legacy (0) or moves all reads and writes to Go (1). There is no
// browser cohort, percentage rollout, or shadow flag in the production path.
export const QUIZCRAFT_GO_WRITES_ENABLED = explicitWriteSetting === '1';
export const QUIZCRAFT_GO_READ_ENABLED = QUIZCRAFT_GO_WRITES_ENABLED;
