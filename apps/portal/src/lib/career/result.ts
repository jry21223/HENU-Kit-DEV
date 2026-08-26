import type { CareerJob, CareerSearchResult } from "../api/types";

/** Keep scanned jobs inspectable, ordered by the service-confirmed relevance. */
export function visibleCareerMatches(result: CareerSearchResult): CareerJob[] {
  return result.jobs.toSorted((left, right) => right.match_score - left.match_score);
}
