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
        /**
         * Internal-only stable actor key; null for guest learners. Portal Gateway MUST strip this field before any external response (ADR-0036 privacy contract).
         *
         */
        user_id: string | null;
        /**
         * Internal-only stable anonymous identity key (the session's immutable actor_key text) for guest learners; null for signed-in learners. Portal Gateway uses it only to derive a stable 游客x display label and MUST never expose it before any external response (ADR-0038).
         *
         */
        guest_key?: string | null;
        correct_answer_count: number;
    }>;
};
