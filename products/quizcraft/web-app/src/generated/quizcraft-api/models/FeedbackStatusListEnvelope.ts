/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { FeedbackStatus } from './FeedbackStatus';
import type { RequestID } from './RequestID';
export type FeedbackStatusListEnvelope = {
    request_id: RequestID;
    data: {
        items: Array<FeedbackStatus>;
    };
};
