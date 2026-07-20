/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type WorkshopBankVersion = {
    bank_version_id: string;
    content_sha256: string;
    question_count: number;
    state: 'draft' | 'validated' | 'legacy';
    active: boolean;
    validated_at?: string;
};
