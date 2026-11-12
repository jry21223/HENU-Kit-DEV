/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingAttribution } from './TrainingAttribution';
import type { TrainingAuthoredMedia } from './TrainingAuthoredMedia';
import type { TrainingListeningLayer } from './TrainingListeningLayer';
import type { TrainingSentenceSegment } from './TrainingSentenceSegment';
import type { TrainingSourceReview } from './TrainingSourceReview';
import type { TrainingStableID } from './TrainingStableID';
export type TrainingListeningAuthoring = {
    source_unit_id: TrainingStableID;
    title: string;
    attributions: Array<TrainingAttribution>;
    question_reviews: Array<TrainingSourceReview>;
    kind: 'listening';
    media: TrainingAuthoredMedia;
    segments: Array<TrainingSentenceSegment>;
    layers: Array<TrainingListeningLayer>;
};
