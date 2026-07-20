/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ImportedQuestionInput } from './ImportedQuestionInput';
export type BankImportRequest = {
    /**
     * Workshop clients send the observed lifecycle version. Omission is retained for older import clients and serializes against the current version while still creating only an unpublished draft.
     */
    expected_version?: number;
    source_sha256: string;
    questions: Array<ImportedQuestionInput>;
};
