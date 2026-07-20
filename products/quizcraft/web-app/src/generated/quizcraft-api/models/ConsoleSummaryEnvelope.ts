/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { RequestID } from './RequestID';
export type ConsoleSummaryEnvelope = {
    request_id: RequestID;
    data: {
        id: string;
        status: 'ok' | 'empty' | 'partial';
        status_message: string;
        as_of: string;
        metrics: Array<{
            label: string;
            value: string;
            hint?: string;
        }>;
    };
};
