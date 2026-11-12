/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingAnswerReceipt } from './TrainingAnswerReceipt';
import type { TrainingHintDisclosure } from './TrainingHintDisclosure';
import type { TrainingPhase } from './TrainingPhase';
import type { TrainingUnit } from './TrainingUnit';
/**
 * Recovery always returns only safe unit, receipts and already persisted cloze hints. Pending has 0 receipts, partial has 1..N-1, completed has N. Actual review/revealed text is read separately after Core proves completeness; never inferred from client flags.
 */
export type TrainingSession = {
    session_id: string;
    unit: TrainingUnit;
    phase: TrainingPhase;
    saved_count: number;
    total_count: number;
    receipts: Array<TrainingAnswerReceipt>;
    hint_disclosures: Array<TrainingHintDisclosure>;
    revealed_layers: Array<'english' | 'chinese'>;
    recovery_scope: 'browser' | 'account';
};
