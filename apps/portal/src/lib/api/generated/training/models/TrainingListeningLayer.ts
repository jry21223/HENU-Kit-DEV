/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingSegmentText } from './TrainingSegmentText';
export type TrainingListeningLayer = {
    kind: 'listening';
    layer: 'english' | 'chinese';
    segments: Array<TrainingSegmentText>;
};
