/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { PracticeQuestion } from './PracticeQuestion';
import type { PracticeSessionMode } from './PracticeSessionMode';
export type PracticeSession = {
    session_id: string;
    bank_id: string;
    bank_version_id: string;
    mode: PracticeSessionMode;
    excluded_unavailable_count: number;
    questions: Array<PracticeQuestion>;
};
