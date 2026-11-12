/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingPartText } from './TrainingPartText';
export type TrainingClozeLayer = {
    kind: 'cloze';
    layer: 'english' | 'chinese';
    parts: Array<TrainingPartText>;
};
