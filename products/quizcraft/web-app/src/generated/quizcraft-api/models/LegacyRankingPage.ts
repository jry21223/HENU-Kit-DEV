/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type LegacyRankingPage = {
    captured_at: string | null;
    content_sha256: string;
    entries: Array<{
        rank: number;
        name: string;
        correct: number;
        total: number;
    }>;
};
