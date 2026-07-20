/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { OperationEnvelope } from '../models/OperationEnvelope';
import type { QuestionFeedback } from '../models/QuestionFeedback';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class FeedbackService {
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
                409: `Idempotency payload or optimistic version conflict`,
                503: `PostgreSQL or a required service is unavailable`,
            },
        });
    }
}
