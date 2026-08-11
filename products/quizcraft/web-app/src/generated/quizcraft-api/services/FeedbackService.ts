/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { FeedbackStatusEnvelope } from '../models/FeedbackStatusEnvelope';
import type { FeedbackStatusListEnvelope } from '../models/FeedbackStatusListEnvelope';
import type { OperationEnvelope } from '../models/OperationEnvelope';
import type { QuestionFeedback } from '../models/QuestionFeedback';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class FeedbackService {
    /**
     * Submit a correction through the narrow Portal Gateway command boundary
     * Internal-only service route. Portal Gateway signs the complete request body and binds the signed-in Portal Session subject as X-Actor-User-Id into the HMAC canonical form; corrections are a signed-in-only command.
     *
     * @returns OperationEnvelope Feedback write result
     * @throws ApiError
     */
    public static createPortalPracticeFeedback({
        idempotencyKey,
        requestBody,
    }: {
        idempotencyKey: string,
        requestBody: QuestionFeedback,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/portal/practice/feedback',
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
    /**
     * Read one authenticated user's correction processing status for Portal
     * The trusted Portal actor header is bound as the sixth line of the signed service request and cannot be substituted in transit.
     * @returns FeedbackStatusEnvelope Caller-owned correction status
     * @throws ApiError
     */
    public static getPortalPracticeFeedbackStatus({
        feedbackId,
        xActorUserId,
    }: {
        feedbackId: string,
        /**
         * UUID of the Portal Session subject; it is the sixth line of the HMAC canonical request.
         */
        xActorUserId: string,
    }): CancelablePromise<FeedbackStatusEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/portal/practice/feedback/{feedback_id}/status',
            path: {
                'feedback_id': feedbackId,
            },
            headers: {
                'X-Actor-User-Id': xActorUserId,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                403: `Permission code or product Scope denied`,
                404: `Resource or operation is unknown to this actor`,
                409: `Dedicated service request nonce was already used`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * List the caller-owned correction processing statuses
     * @returns FeedbackStatusListEnvelope Persisted caller-owned correction statuses
     * @throws ApiError
     */
    public static listQuestionFeedbackStatuses(): CancelablePromise<FeedbackStatusListEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/feedback',
            errors: {
                401: `Missing or invalid actor credentials`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
    /**
     * Submit a correction linked to stable content identity
     * @returns OperationEnvelope Feedback write result
     * @throws ApiError
     */
    public static createQuestionFeedback({
        idempotencyKey,
        requestBody,
    }: {
        idempotencyKey: string,
        requestBody: QuestionFeedback,
    }): CancelablePromise<OperationEnvelope> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/feedback',
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
    /**
     * Read the caller-owned correction processing status
     * @returns FeedbackStatusEnvelope Caller-owned correction status
     * @throws ApiError
     */
    public static getQuestionFeedbackStatus({
        feedbackId,
    }: {
        feedbackId: string,
    }): CancelablePromise<FeedbackStatusEnvelope> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/feedback/{feedback_id}/status',
            path: {
                'feedback_id': feedbackId,
            },
            errors: {
                400: `Invalid request`,
                401: `Missing or invalid actor credentials`,
                404: `Resource or operation is unknown to this actor`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
}
