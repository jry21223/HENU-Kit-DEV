/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ImportedQuestionReport } from './ImportedQuestionReport';
export type ImportReport = {
    accepted: boolean;
    bank_id?: string;
    bank_version_id?: string;
    source_sha256: string;
    content_sha256?: string;
    question_count: number;
    answered_count: number;
    unanswered_count: number;
    type_counts: Record<string, number>;
    chapter_counts: Record<string, number>;
    questions: Array<ImportedQuestionReport>;
    errors: Array<{
        path: string;
        code: string;
        message: string;
    }>;
};
