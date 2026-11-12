/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingClozeVersion } from './TrainingClozeVersion';
import type { TrainingListeningVersion } from './TrainingListeningVersion';
/**
 * Owner-only import/version representation, validated atomically by the existing explicit publication lifecycle. Not exposed on any learner route. See ADR-0060 for cross-reference and canonical-hash invariants.
 */
export type TrainingUnitVersion = (TrainingListeningVersion | TrainingClozeVersion);
