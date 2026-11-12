/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingAttribution } from './TrainingAttribution';
import type { TrainingMedia } from './TrainingMedia';
import type { TrainingQuestion } from './TrainingQuestion';
import type { TrainingSHA256 } from './TrainingSHA256';
export type TrainingListeningUnit = {
    unit_id: string;
    unit_version_id: string;
    bank_id: string;
    bank_version_id: string;
    content_sha256: TrainingSHA256;
    title: string;
    attributions: Array<TrainingAttribution>;
    questions: Array<TrainingQuestion>;
    kind: 'listening';
    media: TrainingMedia;
};
