/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingUnitVersion } from './TrainingUnitVersion';
import type { WorkshopQuestionDetail } from './WorkshopQuestionDetail';
export type WorkshopVersionDetail = {
    bank_id: string;
    bank_version_id: string;
    state: 'draft' | 'validated';
    content_sha256: string;
    questions: Array<WorkshopQuestionDetail>;
    /**
     * Owner-only resolved authored training binding for operator review; absent for ordinary versions. Never forwarded to learner routes.
     */
    training?: TrainingUnitVersion;
};
