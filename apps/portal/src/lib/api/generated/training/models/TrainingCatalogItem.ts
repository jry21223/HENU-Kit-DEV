/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingAttribution } from './TrainingAttribution';
import type { TrainingKind } from './TrainingKind';
import type { TrainingSHA256 } from './TrainingSHA256';
export type TrainingCatalogItem = {
    unit_id: string;
    unit_version_id: string;
    bank_id: string;
    bank_version_id: string;
    kind: TrainingKind;
    title: string;
    question_count: number;
    content_sha256: TrainingSHA256;
    attributions: Array<TrainingAttribution>;
};
