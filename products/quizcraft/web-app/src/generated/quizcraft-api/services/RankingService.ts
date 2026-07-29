/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { LegacyRankingEnvelope } from '../models/LegacyRankingEnvelope';
import type { OperationEnvelope } from '../models/OperationEnvelope';
import type { RankingEnvelope } from '../models/RankingEnvelope';
import type { RankingProfileUpdate } from '../models/RankingProfileUpdate';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class RankingService {
    /**
     * Get the public overall ranking through the internal Portal read contract
     * @returns RankingEnvelope Overall weekly by default or lifetime ranking
     * @throws ApiError
     */
    public static getOverallRanking({
        period = 'weekly',
    }: {
        period?: 'weekly' | 'lifetime',
    }): CancelablePromise<RankingEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/rankings/overall',
            query: {
                'period': period,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Dedicated service request nonce was already used`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Get the immutable pre-migration ranking snapshot without legacy identifiers
     * @returns LegacyRankingEnvelope Read-only pre-migration standings
     * @throws ApiError
     */
    public static getLegacyRanking(): CancelablePromise<LegacyRankingEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/rankings/legacy',
            errors: {
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Get one bank's public ranking through the internal Portal read contract
     * @returns RankingEnvelope Bank weekly or lifetime ranking
     * @throws ApiError
     */
    public static getBankRanking({
        bankId,
        period = 'weekly',
    }: {
        bankId: string,
        period?: 'weekly' | 'lifetime',
    }): CancelablePromise<RankingEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/banks/{bank_id}/rankings',
            path: {
                'bank_id': bankId,
            },
            query: {
                'period': period,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                409: `Dedicated service request nonce was already used`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Set controlled public nickname, system avatar, and ranking visibility
     * @returns OperationEnvelope Profile write result
     * @throws ApiError
     */
    public static updateRankingProfile({
        idempotencyKey,
        requestBody,
    }: {
        idempotencyKey: string,
        requestBody: RankingProfileUpdate,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'PATCH',
            url: '/api/v1/ranking-profile',
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
}
