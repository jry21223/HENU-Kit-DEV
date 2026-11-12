/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingEvidence } from './TrainingEvidence';
export type TrainingQuestionReview = {
    question_id: string;
    question_version_id: string;
    expected_answer: number;
    analysis: string;
    error_category: 'vocabulary' | 'collocation' | 'logic' | 'grammar' | 'detail' | 'inference';
    evidence: TrainingEvidence;
};
