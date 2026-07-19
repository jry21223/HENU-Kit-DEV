/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { RankingPeriod } from './RankingPeriod';
export type RankingPage = {
    scope: 'overall' | 'bank';
    bank_id?: string;
    period: RankingPeriod;
    metric: string;
    entries: Array<{
        rank: number;
        nickname: string;
        system_avatar: string;
        correct_answer_count: number;
    }>;
};
