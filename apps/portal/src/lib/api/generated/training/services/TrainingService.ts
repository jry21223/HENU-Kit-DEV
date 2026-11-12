/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
import type { TrainingAnswerReceiptEnvelope } from '../models/TrainingAnswerReceiptEnvelope';
import type { TrainingAnswerSubmission } from '../models/TrainingAnswerSubmission';
import type { TrainingCatalogEnvelope } from '../models/TrainingCatalogEnvelope';
import type { TrainingCreateSession } from '../models/TrainingCreateSession';
import type { TrainingHintDisclosureEnvelope } from '../models/TrainingHintDisclosureEnvelope';
import type { TrainingHintRequest } from '../models/TrainingHintRequest';
import type { TrainingLayerDisclosureEnvelope } from '../models/TrainingLayerDisclosureEnvelope';
import type { TrainingRevealRequest } from '../models/TrainingRevealRequest';
import type { TrainingReviewEnvelope } from '../models/TrainingReviewEnvelope';
import type { TrainingReviewItemsEnvelope } from '../models/TrainingReviewItemsEnvelope';
import type { TrainingSessionEnvelope } from '../models/TrainingSessionEnvelope';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class TrainingService {
    /**
     * Public safe catalog
     * Public safe catalog; no session identity, hints, keys or private material.
     * @returns TrainingCatalogEnvelope Owner-confirmed typed result.
     * @throws ApiError
     */
    public static listTrainingCatalog(): CancelablePromise<TrainingCatalogEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/practice/training/catalog',
            errors: {
                400: `Invalid closed request, option index, or pinned member/version.`,
                401: `Missing/invalid service signature or invalid Portal session; never downgraded to guest.`,
                403: `Service permission or product scope is invalid.`,
                404: `Gate disabled, missing resource, or resource belongs to another actor; no existence leak.`,
                409: `Incomplete unit, locked layer, unavailable version, immutable answer, idempotency conflict, or nonce replay. error.code distinguishes the cause.`,
                502: `Owner response invalid, owner authentication rejected, or owner network failure.`,
                503: `Write/command/read gate closed, stop-write in force, or required owner/dependency unavailable.`,
            },
        });
    }
    /**
     * Actor-owned review index
     * Actor-owned review index; recover the returned pinned session for complete unit context. Current Core learning state decides review membership.
     * @returns TrainingReviewItemsEnvelope Owner-confirmed typed result.
     * @throws ApiError
     */
    public static listTrainingReviewItems({
        cursor,
        pageSize = 20,
    }: {
        cursor?: string,
        pageSize?: number,
    }): CancelablePromise<TrainingReviewItemsEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/practice/training/review-items',
            query: {
                'cursor': cursor,
                'page_size': pageSize,
            },
            errors: {
                400: `Invalid closed request, option index, or pinned member/version.`,
                401: `Missing/invalid service signature or invalid Portal session; never downgraded to guest.`,
                403: `Service permission or product scope is invalid.`,
                404: `Gate disabled, missing resource, or resource belongs to another actor; no existence leak.`,
                409: `Incomplete unit, locked layer, unavailable version, immutable answer, idempotency conflict, or nonce replay. error.code distinguishes the cause.`,
                502: `Owner response invalid, owner authentication rejected, or owner network failure.`,
                503: `Write/command/read gate closed, stop-write in force, or required owner/dependency unavailable.`,
            },
        });
    }
    /**
     * Pin exactly the selected currently published immutable unit version. Never substitute a newer version.
     * Pin exactly the selected currently published immutable unit version. Never substitute a newer version.
     * @returns TrainingSessionEnvelope Owner-confirmed typed result.
     * @throws ApiError
     */
    public static createTrainingSession({
        idempotencyKey,
        requestBody,
    }: {
        idempotencyKey: string,
        requestBody: TrainingCreateSession,
    }): CancelablePromise<TrainingSessionEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/practice/training/sessions',
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Invalid closed request, option index, or pinned member/version.`,
                401: `Missing/invalid service signature or invalid Portal session; never downgraded to guest.`,
                403: `Service permission or product scope is invalid.`,
                404: `Gate disabled, missing resource, or resource belongs to another actor; no existence leak.`,
                409: `Incomplete unit, locked layer, unavailable version, immutable answer, idempotency conflict, or nonce replay. error.code distinguishes the cause.`,
                502: `Owner response invalid, owner authentication rejected, or owner network failure.`,
                503: `Write/command/read gate closed, stop-write in force, or required owner/dependency unavailable.`,
            },
        });
    }
    /**
     * Owner-bound recovery of safe pinned content, saved first answers, hints and disclosed layer names. Does not return shared review.
     * Owner-bound recovery of safe pinned content, saved first answers, hints and disclosed layer names. Does not return shared review.
     * @returns TrainingSessionEnvelope Owner-confirmed typed result.
     * @throws ApiError
     */
    public static getTrainingSession({
        sessionId,
    }: {
        sessionId: string,
    }): CancelablePromise<TrainingSessionEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/practice/training/sessions/{session_id}',
            path: {
                'session_id': sessionId,
            },
            errors: {
                400: `Invalid closed request, option index, or pinned member/version.`,
                401: `Missing/invalid service signature or invalid Portal session; never downgraded to guest.`,
                403: `Service permission or product scope is invalid.`,
                404: `Gate disabled, missing resource, or resource belongs to another actor; no existence leak.`,
                409: `Incomplete unit, locked layer, unavailable version, immutable answer, idempotency conflict, or nonce replay. error.code distinguishes the cause.`,
                502: `Owner response invalid, owner authentication rejected, or owner network failure.`,
                503: `Write/command/read gate closed, stop-write in force, or required owner/dependency unavailable.`,
            },
        });
    }
    /**
     * One immutable first answer per pinned question
     * One immutable first answer per pinned question; stable per-question idempotency key. Never releases shared review.
     * @returns TrainingAnswerReceiptEnvelope Owner-confirmed typed result.
     * @throws ApiError
     */
    public static submitTrainingAnswer({
        sessionId,
        idempotencyKey,
        requestBody,
    }: {
        sessionId: string,
        idempotencyKey: string,
        requestBody: TrainingAnswerSubmission,
    }): CancelablePromise<TrainingAnswerReceiptEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/practice/training/sessions/{session_id}/answers',
            path: {
                'session_id': sessionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Invalid closed request, option index, or pinned member/version.`,
                401: `Missing/invalid service signature or invalid Portal session; never downgraded to guest.`,
                403: `Service permission or product scope is invalid.`,
                404: `Gate disabled, missing resource, or resource belongs to another actor; no existence leak.`,
                409: `Incomplete unit, locked layer, unavailable version, immutable answer, idempotency conflict, or nonce replay. error.code distinguishes the cause.`,
                502: `Owner response invalid, owner authentication rejected, or owner network failure.`,
                503: `Write/command/read gate closed, stop-write in force, or required owner/dependency unavailable.`,
            },
        });
    }
    /**
     * Cloze only
     * Cloze only; persist a sequential authored hint layer before disclosure and mark all still-unanswered questions assisted under the same session lock.
     * @returns TrainingHintDisclosureEnvelope Owner-confirmed typed result.
     * @throws ApiError
     */
    public static discloseTrainingHint({
        sessionId,
        idempotencyKey,
        requestBody,
    }: {
        sessionId: string,
        idempotencyKey: string,
        requestBody: TrainingHintRequest,
    }): CancelablePromise<TrainingHintDisclosureEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/practice/training/sessions/{session_id}/hints',
            path: {
                'session_id': sessionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Invalid closed request, option index, or pinned member/version.`,
                401: `Missing/invalid service signature or invalid Portal session; never downgraded to guest.`,
                403: `Service permission or product scope is invalid.`,
                404: `Gate disabled, missing resource, or resource belongs to another actor; no existence leak.`,
                409: `Incomplete unit, locked layer, unavailable version, immutable answer, idempotency conflict, or nonce replay. error.code distinguishes the cause.`,
                502: `Owner response invalid, owner authentication rejected, or owner network failure.`,
                503: `Write/command/read gate closed, stop-write in force, or required owner/dependency unavailable.`,
            },
        });
    }
    /**
     * Owner-bound
     * Owner-bound; UNIT_INCOMPLETE until all exact pinned questions have first answers. Contains keys, analysis and evidence plus only already disclosed text layers.
     * @returns TrainingReviewEnvelope Owner-confirmed typed result.
     * @throws ApiError
     */
    public static getTrainingReview({
        sessionId,
    }: {
        sessionId: string,
    }): CancelablePromise<TrainingReviewEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/practice/training/sessions/{session_id}/review',
            path: {
                'session_id': sessionId,
            },
            errors: {
                400: `Invalid closed request, option index, or pinned member/version.`,
                401: `Missing/invalid service signature or invalid Portal session; never downgraded to guest.`,
                403: `Service permission or product scope is invalid.`,
                404: `Gate disabled, missing resource, or resource belongs to another actor; no existence leak.`,
                409: `Incomplete unit, locked layer, unavailable version, immutable answer, idempotency conflict, or nonce replay. error.code distinguishes the cause.`,
                502: `Owner response invalid, owner authentication rejected, or owner network failure.`,
                503: `Write/command/read gate closed, stop-write in force, or required owner/dependency unavailable.`,
            },
        });
    }
    /**
     * Owner-bound complete-session reveal
     * Owner-bound complete-session reveal; persist English before Chinese. Recovery/review replays only committed layers.
     * @returns TrainingLayerDisclosureEnvelope Owner-confirmed typed result.
     * @throws ApiError
     */
    public static revealTrainingLayer({
        sessionId,
        idempotencyKey,
        requestBody,
    }: {
        sessionId: string,
        idempotencyKey: string,
        requestBody: TrainingRevealRequest,
    }): CancelablePromise<TrainingLayerDisclosureEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/practice/training/sessions/{session_id}/reveals',
            path: {
                'session_id': sessionId,
            },
            headers: {
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Invalid closed request, option index, or pinned member/version.`,
                401: `Missing/invalid service signature or invalid Portal session; never downgraded to guest.`,
                403: `Service permission or product scope is invalid.`,
                404: `Gate disabled, missing resource, or resource belongs to another actor; no existence leak.`,
                409: `Incomplete unit, locked layer, unavailable version, immutable answer, idempotency conflict, or nonce replay. error.code distinguishes the cause.`,
                502: `Owner response invalid, owner authentication rejected, or owner network failure.`,
                503: `Write/command/read gate closed, stop-write in force, or required owner/dependency unavailable.`,
            },
        });
    }
}
