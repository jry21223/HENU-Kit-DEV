/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingAttribution } from './TrainingAttribution';
import type { TrainingBlank } from './TrainingBlank';
import type { TrainingHintDescriptor } from './TrainingHintDescriptor';
import type { TrainingPassagePart } from './TrainingPassagePart';
import type { TrainingQuestion } from './TrainingQuestion';
import type { TrainingSHA256 } from './TrainingSHA256';
export type TrainingClozeUnit = {
    unit_id: string;
    unit_version_id: string;
    bank_id: string;
    bank_version_id: string;
    content_sha256: TrainingSHA256;
    title: string;
    attributions: Array<TrainingAttribution>;
    questions: Array<TrainingQuestion>;
    kind: 'cloze';
    parts: Array<TrainingPassagePart>;
    blanks: Array<TrainingBlank>;
    hints: Array<TrainingHintDescriptor>;
};
