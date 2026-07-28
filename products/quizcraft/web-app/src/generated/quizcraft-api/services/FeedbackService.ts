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
