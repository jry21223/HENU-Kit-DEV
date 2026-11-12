/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingAuthoring } from './TrainingAuthoring';
import type { TrainingImportedQuestion } from './TrainingImportedQuestion';
import type { TrainingSHA256 } from './TrainingSHA256';
export type TrainingBankImportRequest = {
    expected_version: number;
    source_sha256: TrainingSHA256;
    questions: Array<TrainingImportedQuestion>;
    training: TrainingAuthoring;
};
