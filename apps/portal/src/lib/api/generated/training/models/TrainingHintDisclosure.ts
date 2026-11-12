/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingStableID } from './TrainingStableID';
/**
 * Persisted before text is returned. All unanswered pinned questions under the session lock become assisted. Replay returns the same event; it cannot undo assistance.
 */
export type TrainingHintDisclosure = {
    disclosure_id: string;
    hint_id: TrainingStableID;
    level: number;
    text: string;
    affected_question_ids: Array<string>;
    disclosed_at: string;
};
