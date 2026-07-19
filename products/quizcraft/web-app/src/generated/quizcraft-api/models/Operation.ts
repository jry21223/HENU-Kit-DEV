/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { RequestID } from './RequestID';
export type Operation = {
    operation_id: string;
    state: 'pending' | 'succeeded' | 'rejected' | 'unknown';
    idempotency_key: string;
    request_id: RequestID;
    resource_id?: string;
};
