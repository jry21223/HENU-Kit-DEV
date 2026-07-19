/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { Chapter } from './Chapter';
export type BankVersion = {
    /**
     * Stable across every version of this bank.
     */
    bank_id: string;
    /**
     * Immutable identifier for exact bank content.
     */
    bank_version_id: string;
    bank_key: string;
    name: string;
    content_sha256: string;
    question_count: number;
    chapters: Array<Chapter>;
};
