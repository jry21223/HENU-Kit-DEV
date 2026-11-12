/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingKind } from './TrainingKind';
import type { TrainingPhase } from './TrainingPhase';
export type TrainingReviewItem = {
    unit_id: string;
    unit_version_id: string;
    session_id: string;
    title: string;
    kind: TrainingKind;
    phase: TrainingPhase;
    review_required_count: number;
    last_attempt_at: string;
};
