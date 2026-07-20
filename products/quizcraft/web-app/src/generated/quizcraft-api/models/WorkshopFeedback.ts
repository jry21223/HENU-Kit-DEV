/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type WorkshopFeedback = {
    feedback_id: string;
    bank_id: string;
    question_id: string;
    question_version_id: string;
    category: 'wrong_answer' | 'ambiguous' | 'typo' | 'outdated' | 'other';
    detail: string;
    created_at: string;
};
