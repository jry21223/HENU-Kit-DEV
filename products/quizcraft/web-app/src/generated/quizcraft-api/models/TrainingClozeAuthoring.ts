/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingAttribution } from './TrainingAttribution';
import type { TrainingAuthoredHint } from './TrainingAuthoredHint';
import type { TrainingClozeLayer } from './TrainingClozeLayer';
import type { TrainingPassagePart } from './TrainingPassagePart';
import type { TrainingSourceBlank } from './TrainingSourceBlank';
import type { TrainingSourceReview } from './TrainingSourceReview';
import type { TrainingStableID } from './TrainingStableID';
export type TrainingClozeAuthoring = {
    source_unit_id: TrainingStableID;
    title: string;
    attributions: Array<TrainingAttribution>;
    question_reviews: Array<TrainingSourceReview>;
    kind: 'cloze';
    parts: Array<TrainingPassagePart>;
    blanks: Array<TrainingSourceBlank>;
    hints: Array<TrainingAuthoredHint>;
    layers: Array<TrainingClozeLayer>;
};
