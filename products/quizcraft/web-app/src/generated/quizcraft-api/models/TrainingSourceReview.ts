/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingEvidence } from './TrainingEvidence';
export type TrainingSourceReview = {
    source_question_id: string;
    error_category: 'vocabulary' | 'collocation' | 'logic' | 'grammar' | 'detail' | 'inference';
    evidence: TrainingEvidence;
};
