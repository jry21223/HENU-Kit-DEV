/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingReviewItem } from './TrainingReviewItem';
/**
 * Actor-owned current review index, grouped by immutable unit version. Empty next_cursor means final page. Each item links a pinned session, not a browser question bank; fresh independent success updates the Core learning-state projection and may remove it.
 */
export type TrainingReviewItems = {
    items: Array<TrainingReviewItem>;
    next_cursor: string;
};
