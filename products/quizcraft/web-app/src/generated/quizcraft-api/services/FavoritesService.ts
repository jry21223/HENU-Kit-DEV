/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { FavoriteListEnvelope } from '../models/FavoriteListEnvelope';
import type { FavoritesOverviewEnvelope } from '../models/FavoritesOverviewEnvelope';
import type { OperationEnvelope } from '../models/OperationEnvelope';
import type { PracticeSessionEnvelope } from '../models/PracticeSessionEnvelope';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class FavoritesService {
    /**
     * List per-bank favorites counts and unavailable counts
     * @returns FavoritesOverviewEnvelope Automatic per-bank folders
     * @throws ApiError
     */
    public static getFavoritesOverview(): CancelablePromise<FavoritesOverviewEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/favorites',
            errors: {
                401: `Missing or invalid actor credentials`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * List one bank's favorite references
     * @returns FavoriteListEnvelope Available items include content versions; unavailable items expose references only
     * @throws ApiError
     */
    public static listFavoriteQuestions({
        bankId,
    }: {
        bankId: string,
    }): CancelablePromise<FavoriteListEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/banks/{bank_id}/favorites',
            path: {
                'bank_id': bankId,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Favorite a stable question reference idempotently
     * @returns OperationEnvelope Favorite write result
     * @throws ApiError
     */
    public static favoriteQuestion({
        bankId,
        questionId,
        idempotencyKey,
    }: {
        bankId: string,
        questionId: string,
        idempotencyKey: string,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/banks/{bank_id}/favorites/{question_id}',
            path: {
                'bank_id': bankId,
                'question_id': questionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                404: `Resource or operation is unknown to this actor`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Remove a stable favorite idempotently
     * @returns OperationEnvelope Unfavorite write result
     * @throws ApiError
     */
    public static unfavoriteQuestion({
        bankId,
        questionId,
        idempotencyKey,
    }: {
        bankId: string,
        questionId: string,
        idempotencyKey: string,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/api/v1/banks/{bank_id}/favorites/{question_id}',
            path: {
                'bank_id': bankId,
                'question_id': questionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Create a Practice Core session from one bank's available favorites
     * @returns PracticeSessionEnvelope Session excludes unavailable favorites and reports the excluded count
     * @throws ApiError
     */
    public static createFavoritesPracticeSession({
        bankId,
        idempotencyKey,
    }: {
        bankId: string,
        idempotencyKey: string,
    }): CancelablePromise<PracticeSessionEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/banks/{bank_id}/favorites/practice-sessions',
            path: {
                'bank_id': bankId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                404: `Resource or operation is unknown to this actor`,
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
}
