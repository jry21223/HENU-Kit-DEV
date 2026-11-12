/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/**
 * Zero-based single-choice option index. Core also checks index < pinned options length and exact pinned membership; no client correctness or assistance fields.
 */
export type TrainingAnswerSubmission = {
    unit_version_id: string;
    question_id: string;
    question_version_id: string;
    answer: number;
};
