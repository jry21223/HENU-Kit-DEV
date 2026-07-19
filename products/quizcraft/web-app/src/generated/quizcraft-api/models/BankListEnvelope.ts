/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { BankVersion } from './BankVersion';
import type { RequestID } from './RequestID';
export type BankListEnvelope = {
    request_id: RequestID;
    data: Array<BankVersion>;
};
