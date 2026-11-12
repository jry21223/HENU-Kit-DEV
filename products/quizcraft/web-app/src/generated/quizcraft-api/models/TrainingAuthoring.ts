/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { TrainingClozeAuthoring } from './TrainingClozeAuthoring';
import type { TrainingListeningAuthoring } from './TrainingListeningAuthoring';
/**
 * Closed optional training input on the existing Workshop bank import only. Uses source IDs; owner resolves generated UUIDs within the import transaction. All fields participate in bank semantic hashing before unit-version binding.
 */
export type TrainingAuthoring = (TrainingListeningAuthoring | TrainingClozeAuthoring);
