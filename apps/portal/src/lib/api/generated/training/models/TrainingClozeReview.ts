/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingLayerDisclosure } from './TrainingLayerDisclosure';
import type { TrainingQuestionReview } from './TrainingQuestionReview';
export type TrainingClozeReview = {
    session_id: string;
    unit_version_id: string;
    kind: 'cloze';
    questions: Array<TrainingQuestionReview>;
    layers: Array<TrainingLayerDisclosure>;
};
