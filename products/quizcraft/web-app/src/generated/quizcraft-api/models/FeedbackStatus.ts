/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type FeedbackStatus = {
    feedback_id: string;
    bank_id: string;
    question_id: string;
    question_version_id: string;
    category: 'wrong_answer' | 'ambiguous' | 'typo' | 'outdated' | 'other';
    status: 'pending' | 'in_progress' | 'blocked' | 'resolved' | 'archived';
    created_at: string;
    updated_at: string;
};
