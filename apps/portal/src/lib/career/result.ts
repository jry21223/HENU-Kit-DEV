import type { CareerJob, CareerSearchResult } from "../api/types";

const MATCH_THRESHOLD = 50;

/** Return only real matches, ordered by the service-confirmed score. */
export function visibleCareerMatches(result: CareerSearchResult): CareerJob[] {
  return result.jobs
    .filter((job) => job.match_score >= MATCH_THRESHOLD)
    .toSorted((left, right) => right.match_score - left.match_score);
}
