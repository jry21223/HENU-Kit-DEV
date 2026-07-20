/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { WorkshopBankVersion } from './WorkshopBankVersion';
export type WorkshopBank = {
    bank_id: string;
    bank_key: string;
    name: string;
    lifecycle_version: number;
    active_version_id?: string;
    versions: Array<WorkshopBankVersion>;
};
