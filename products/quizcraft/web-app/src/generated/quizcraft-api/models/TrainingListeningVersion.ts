/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingListeningLayer } from './TrainingListeningLayer';
import type { TrainingListeningUnit } from './TrainingListeningUnit';
import type { TrainingQuestionReview } from './TrainingQuestionReview';
import type { TrainingSentenceSegment } from './TrainingSentenceSegment';
export type TrainingListeningVersion = {
    unit: TrainingListeningUnit;
    review_questions: Array<TrainingQuestionReview>;
    segments: Array<TrainingSentenceSegment>;
    layers: Array<TrainingListeningLayer>;
};
