/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingAuthoredHint } from './TrainingAuthoredHint';
import type { TrainingClozeLayer } from './TrainingClozeLayer';
import type { TrainingClozeUnit } from './TrainingClozeUnit';
import type { TrainingQuestionReview } from './TrainingQuestionReview';
export type TrainingClozeVersion = {
    unit: TrainingClozeUnit;
    review_questions: Array<TrainingQuestionReview>;
    hints: Array<TrainingAuthoredHint>;
    layers: Array<TrainingClozeLayer>;
};
