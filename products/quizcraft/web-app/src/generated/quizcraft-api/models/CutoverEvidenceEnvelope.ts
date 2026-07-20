/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { RequestID } from './RequestID';
export type CutoverEvidenceEnvelope = {
    request_id: RequestID;
    data: {
        database: string;
        writes_enabled: boolean;
        release_sha: string;
        migration_run_id: string;
        migration_cursor: number;
        shadow_gate_report_id: string;
    };
};
