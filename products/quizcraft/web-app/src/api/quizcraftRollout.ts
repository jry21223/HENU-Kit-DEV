const parseReadPercent = (value: string | undefined) => {
  const parsed = Number(value || '0');
  if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100) return 0;
  return parsed;
};

const hashPercent = (value: string) => {
  let hash = 2166136261;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0) % 10000 / 100;
};

const rolloutSubject = () => {
  if (typeof window === 'undefined') return 'server-render';
  const storageKey = 'quizcraft_go_read_cohort';
  try {
    const existing = window.localStorage.getItem(storageKey);
    if (existing) return existing;
    const created = typeof crypto.randomUUID === 'function'
      ? crypto.randomUUID()
      : `${Date.now()}-${Math.random()}`;
    window.localStorage.setItem(storageKey, created);
    return created;
  } catch {
    return 'storage-unavailable';
  }
};

const legacyAllTraffic = import.meta.env.VITE_QUIZCRAFT_GO_SHADOW === '1';
const explicitWriteSetting = import.meta.env.VITE_QUIZCRAFT_GO_WRITES;
const explicitReadPercent = import.meta.env.VITE_QUIZCRAFT_GO_READ_PERCENT;

export const QUIZCRAFT_GO_WRITES_ENABLED =
  explicitWriteSetting === '1' || (explicitWriteSetting === undefined && legacyAllTraffic);

export const QUIZCRAFT_GO_READ_PERCENT = parseReadPercent(
  explicitReadPercent === undefined && legacyAllTraffic ? '100' : explicitReadPercent,
);

export const QUIZCRAFT_GO_READ_ENABLED =
  QUIZCRAFT_GO_WRITES_ENABLED || hashPercent(rolloutSubject()) < QUIZCRAFT_GO_READ_PERCENT;
