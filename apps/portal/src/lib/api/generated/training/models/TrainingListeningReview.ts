/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingLayerDisclosure } from './TrainingLayerDisclosure';
import type { TrainingQuestionReview } from './TrainingQuestionReview';
import type { TrainingSentenceSegment } from './TrainingSentenceSegment';
export type TrainingListeningReview = {
    session_id: string;
    unit_version_id: string;
    kind: 'listening';
    questions: Array<TrainingQuestionReview>;
    segments: Array<TrainingSentenceSegment>;
    layers: Array<TrainingLayerDisclosure>;
};
