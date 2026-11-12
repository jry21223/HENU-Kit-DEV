/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingOutcome } from './TrainingOutcome';
import type { TrainingPhase } from './TrainingPhase';
/**
 * First-answer fact only. No expected answer, analysis, transcript, shared evidence, or reveal content, even when phase=completed. saved_count/phase are current at response time; all first-answer fields are immutable.
 */
export type TrainingAnswerReceipt = {
    session_id: string;
    unit_version_id: string;
    question_id: string;
    question_version_id: string;
    answer: number;
    correct: boolean;
    assisted: boolean;
    outcome: TrainingOutcome;
    review_required: boolean;
    submitted_at: string;
    replayed: boolean;
    saved_count: number;
    total_count: number;
    phase: TrainingPhase;
};
